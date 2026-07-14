package account

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

const MaxImportBytes = 10 << 20

type ParsedImportEntry struct {
	Index    int
	Entry    ImportEntry
	Source   string
	Warnings []string
	Err      error
}

// ParseImportDocument normalizes text encoding and parses all supported import
// shapes without silently treating malformed JSON as an access token.
func ParseImportDocument(raw []byte) ([]ParsedImportEntry, error) {
	if len(raw) > MaxImportBytes {
		return nil, fmt.Errorf("import exceeds %d bytes", MaxImportBytes)
	}
	content, encodingName, err := decodeImportText(raw)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, errors.New("empty import content")
	}

	values := []any{}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		values, err = decodeJSONStream(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON import: %w", err)
		}
	} else {
		values, err = parseImportLines(trimmed)
		if err != nil {
			return nil, err
		}
	}

	entries := []ParsedImportEntry{}
	collectImportValues(values, "document", &entries)
	if len(entries) == 0 {
		return nil, errors.New("no importable accounts found")
	}
	if encodingName != "utf-8" {
		for index := range entries {
			entries[index].Warnings = append(entries[index].Warnings, "input converted from "+encodingName)
		}
	}
	for index := range entries {
		entries[index].Index = index + 1
		if entries[index].Err == nil {
			entry := entries[index].Entry
			entry.AccountType = strings.ToLower(strings.TrimSpace(entry.AccountType))
			entry.BaseURL = strings.TrimSpace(entry.BaseURL)
			entry.APIKey = strings.TrimSpace(entry.APIKey)
			if entry.AccountType == "" {
				entry.AccountType = string(store.AccountTypeOAuth)
				if entry.APIKey != "" && strings.TrimSpace(entry.AccessToken) == "" && strings.TrimSpace(entry.RefreshToken) == "" {
					entry.AccountType = string(store.AccountTypeAPIKey)
				}
			}
			entries[index].Entry = entry
			switch store.AccountType(entry.AccountType) {
			case store.AccountTypeAPIKey:
				if entry.APIKey == "" {
					entries[index].Err = errors.New("missing api_key")
				} else if entry.BaseURL == "" {
					entry.BaseURL = openai.CodexResponsesURL
					entries[index].Entry = entry
				}
			case store.AccountTypeOAuth:
				if strings.TrimSpace(entry.AccessToken) == "" && strings.TrimSpace(entry.RefreshToken) == "" {
					entries[index].Err = errors.New("missing access_token and refresh_token")
				} else if entry.AccessToken != "" && !validBareToken(entry.AccessToken) {
					entries[index].Err = errors.New("access_token has an invalid format")
				} else if entry.RefreshToken != "" && !validOpaqueToken(entry.RefreshToken) {
					entries[index].Err = errors.New("refresh_token has an invalid format")
				} else if entry.IDToken != "" && !validJWT(entry.IDToken) {
					entries[index].Err = errors.New("id_token has an invalid JWT format")
				}
			default:
				entries[index].Err = fmt.Errorf("unsupported account_type %q", entry.AccountType)
			}
		}
	}
	return entries, nil
}

// ParseImportPayload keeps the v0.1.x parser entry point for compatibility.
// New code should use ParseImportDocument so row-level errors remain visible.
func ParseImportPayload(raw string) ([]ImportEntry, error) {
	parsed, err := ParseImportDocument([]byte(raw))
	if err != nil {
		return nil, err
	}
	entries := make([]ImportEntry, 0, len(parsed))
	for _, item := range parsed {
		if item.Err != nil {
			return nil, fmt.Errorf("row %d: %w", item.Index, item.Err)
		}
		entries = append(entries, item.Entry)
	}
	return entries, nil
}

func decodeImportText(raw []byte) (string, string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}):
		raw = raw[3:]
		if !utf8.Valid(raw) {
			return "", "", errors.New("invalid UTF-8 import")
		}
		return string(raw), "utf-8", nil
	case bytes.HasPrefix(raw, []byte{0xff, 0xfe}):
		text, err := decodeUTF16(raw[2:], false)
		return text, "utf-16le", err
	case bytes.HasPrefix(raw, []byte{0xfe, 0xff}):
		text, err := decodeUTF16(raw[2:], true)
		return text, "utf-16be", err
	case utf8.Valid(raw):
		return string(raw), "utf-8", nil
	case looksUTF16(raw, false):
		text, err := decodeUTF16(raw, false)
		return text, "utf-16le", err
	case looksUTF16(raw, true):
		text, err := decodeUTF16(raw, true)
		return text, "utf-16be", err
	default:
		return "", "", errors.New("unsupported import encoding; use UTF-8 or UTF-16")
	}
}

func looksUTF16(raw []byte, bigEndian bool) bool {
	if len(raw) < 4 || len(raw)%2 != 0 {
		return false
	}
	zeros := 0
	for index := 0; index < len(raw); index += 2 {
		position := index + 1
		if bigEndian {
			position = index
		}
		if raw[position] == 0 {
			zeros++
		}
	}
	return zeros*4 >= len(raw)
}

func decodeUTF16(raw []byte, bigEndian bool) (string, error) {
	if len(raw)%2 != 0 {
		return "", errors.New("invalid UTF-16 byte length")
	}
	units := make([]uint16, len(raw)/2)
	for index := range units {
		first, second := raw[index*2], raw[index*2+1]
		if bigEndian {
			units[index] = uint16(first)<<8 | uint16(second)
		} else {
			units[index] = uint16(second)<<8 | uint16(first)
		}
	}
	return string(utf16.Decode(units)), nil
}

func decodeJSONStream(content string) ([]any, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	values := []any{}
	for {
		var value any
		err := decoder.Decode(&value)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("empty JSON content")
	}
	return values, nil
}

func parseImportLines(content string) ([]any, error) {
	values := []any{}
	for lineNumber, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			decoded, err := decodeJSONStream(line)
			if err != nil {
				return nil, fmt.Errorf("line %d contains invalid JSON: %w", lineNumber+1, err)
			}
			values = append(values, decoded...)
			continue
		}
		values = append(values, bareToken{value: line, line: lineNumber + 1})
	}
	return values, nil
}

type bareToken struct {
	value string
	line  int
}

func collectImportValues(values []any, source string, output *[]ParsedImportEntry) {
	for _, value := range values {
		switch item := value.(type) {
		case []any:
			collectImportValues(item, source, output)
		case bareToken:
			entry := ParsedImportEntry{Source: "line"}
			if !validBareToken(item.value) {
				entry.Err = fmt.Errorf("line %d is not a recognized token", item.line)
			} else if looksRefreshToken(item.value) {
				entry.Entry.RefreshToken = item.value
			} else {
				entry.Entry.AccessToken = item.value
			}
			*output = append(*output, entry)
		case string:
			entry := ParsedImportEntry{Source: source}
			if !validBareToken(item) {
				entry.Err = errors.New("string value is not a recognized token")
			} else if looksRefreshToken(item) {
				entry.Entry.RefreshToken = strings.TrimSpace(item)
			} else {
				entry.Entry.AccessToken = strings.TrimSpace(item)
			}
			*output = append(*output, entry)
		case map[string]any:
			if inner, ok := item["accounts"].([]any); ok {
				collectImportValues(inner, "accounts", output)
				continue
			}
			entry, warnings := entryFromMap(item)
			*output = append(*output, ParsedImportEntry{Entry: entry, Source: detectSource(item), Warnings: warnings})
		default:
			*output = append(*output, ParsedImportEntry{Source: source, Err: fmt.Errorf("unsupported value type %T", value)})
		}
	}
}

func validBareToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t") {
		return false
	}
	if looksRefreshToken(value) || strings.HasPrefix(value, "sk-") {
		return len(value) >= 20
	}
	return validJWT(value)
}

func validJWT(value string) bool {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts[:2] {
		if len(part) < 4 {
			return false
		}
		if _, err := base64.RawURLEncoding.DecodeString(part); err != nil {
			return false
		}
	}
	return len(parts[2]) >= 8
}

func validOpaqueToken(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 20 && !strings.ContainsAny(value, " \t\r\n")
}

func looksRefreshToken(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "rt_") || strings.HasPrefix(value, "refresh_")
}

func detectSource(value map[string]any) string {
	if _, ok := value["credentials"].(map[string]any); ok {
		return "sub2api_backup"
	}
	if _, ok := value["tokens"].(map[string]any); ok {
		return "codex_auth"
	}
	return "account_object"
}

func entryFromMap(value map[string]any) (ImportEntry, []string) {
	entry := ImportEntry{}
	entry.AccountType = firstString(value, []string{"account_type"}, []string{"accountType"})
	entry.BaseURL = firstString(value, []string{"base_url"}, []string{"baseUrl"}, []string{"BaseURL"})
	entry.APIKey = firstString(value,
		[]string{"api_key"}, []string{"apiKey"}, []string{"credentials", "api_key"}, []string{"credentials", "apiKey"},
	)
	entry.AccessToken = firstString(value,
		[]string{"tokens", "access_token"}, []string{"tokens", "accessToken"},
		[]string{"credentials", "access_token"}, []string{"credentials", "accessToken"},
		[]string{"access_token"}, []string{"accessToken"}, []string{"token"},
	)
	entry.RefreshToken = firstString(value,
		[]string{"tokens", "refresh_token"}, []string{"tokens", "refreshToken"},
		[]string{"credentials", "refresh_token"}, []string{"credentials", "refreshToken"},
		[]string{"refresh_token"}, []string{"refreshToken"},
	)
	entry.IDToken = firstString(value,
		[]string{"tokens", "id_token"}, []string{"tokens", "idToken"},
		[]string{"credentials", "id_token"}, []string{"credentials", "idToken"},
		[]string{"id_token"}, []string{"idToken"},
	)
	entry.Email = firstString(value, []string{"email"}, []string{"user", "email"}, []string{"name"})
	entry.ChatGPTAccountID = firstString(value,
		[]string{"chatgpt_account_id"}, []string{"chatgptAccountId"},
		[]string{"credentials", "chatgpt_account_id"},
	)
	warnings := []string{}
	if entry.ChatGPTAccountID == "" {
		if ambiguous := firstString(value,
			[]string{"account_id"}, []string{"accountId"}, []string{"account", "id"}, []string{"account", "account_id"},
		); ambiguous != "" {
			warnings = append(warnings, "ambiguous account_id ignored until identity verification")
		}
	}
	entry.PlanType = firstString(value,
		[]string{"plan_type"}, []string{"planType"}, []string{"account", "plan_type"}, []string{"account", "planType"},
	)
	entry.ExpiresAt = firstScalar(value,
		[]string{"tokens", "expires_at"}, []string{"tokens", "expiresAt"},
		[]string{"credentials", "expires_at"}, []string{"expires_at"}, []string{"expiresAt"},
	)
	return entry, warnings
}

func lookupPath(value map[string]any, path []string) (any, bool) {
	var current any = value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func firstString(value map[string]any, paths ...[]string) string {
	for _, path := range paths {
		if candidate, ok := lookupPath(value, path); ok {
			if text, ok := candidate.(string); ok {
				if text = strings.TrimSpace(text); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func firstScalar(value map[string]any, paths ...[]string) string {
	for _, path := range paths {
		candidate, ok := lookupPath(value, path)
		if !ok {
			continue
		}
		switch scalar := candidate.(type) {
		case string:
			if text := strings.TrimSpace(scalar); text != "" {
				return text
			}
		case json.Number:
			return scalar.String()
		case float64:
			return strconv.FormatInt(int64(scalar), 10)
		}
	}
	return ""
}
