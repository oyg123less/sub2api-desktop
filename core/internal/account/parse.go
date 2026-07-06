package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ParseImportPayload converts raw pasted/imported text into ImportEntry values.
// It accepts the same shapes as the upstream sub2api importer:
//   - a bare array of entries, or an object with an "accounts" array
//     (including sub2api-data backup exports where token fields live in a
//     "credentials" object);
//   - a single account object, including Codex CLI auth.json where tokens are
//     nested under "tokens";
//   - snake_case and camelCase key variants;
//   - a JSON stream / one JSON document or bare access token per line.
func ParseImportPayload(raw string) ([]ImportEntry, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("空内容")
	}

	var values []any
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		vs, err := decodeJSONStream(trimmed)
		if err != nil {
			if lineValues, lineErr := parseLines(trimmed); lineErr == nil && len(lineValues) > 0 {
				values = lineValues
			} else {
				return nil, fmt.Errorf("JSON 解析失败: %w", err)
			}
		} else {
			values = vs
		}
	} else {
		vs, err := parseLines(trimmed)
		if err != nil {
			return nil, err
		}
		values = vs
	}

	var entries []ImportEntry
	collectImportValues(values, &entries)
	if len(entries) == 0 {
		return nil, errors.New("未找到可导入的账号")
	}
	return entries, nil
}

func decodeJSONStream(content string) ([]any, error) {
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber()
	var values []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	if len(values) == 0 {
		return nil, errors.New("空 JSON 内容")
	}
	return values, nil
}

func parseLines(content string) ([]any, error) {
	var values []any
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			vs, err := decodeJSONStream(line)
			if err != nil {
				return nil, fmt.Errorf("第 %d 行 JSON 解析失败: %w", len(values)+1, err)
			}
			values = append(values, vs...)
			continue
		}
		values = append(values, line)
	}
	return values, nil
}

func collectImportValues(values []any, out *[]ImportEntry) {
	for _, v := range values {
		switch item := v.(type) {
		case []any:
			collectImportValues(item, out)
		case string:
			tok := strings.TrimSpace(item)
			if tok != "" {
				*out = append(*out, ImportEntry{AccessToken: tok})
			}
		case map[string]any:
			// Wrapper objects: {"accounts": [...]} (Amber & sub2api-data backups).
			if inner, ok := item["accounts"].([]any); ok {
				collectImportValues(inner, out)
				continue
			}
			*out = append(*out, entryFromMap(item))
		}
	}
}

// entryFromMap extracts token/identity fields from one account object,
// tolerating the upstream key variants: values nested under "tokens" (Codex
// CLI auth.json) or "credentials" (sub2api-data backups), and camelCase names.
func entryFromMap(m map[string]any) ImportEntry {
	var e ImportEntry
	e.AccessToken = firstString(m,
		[]string{"tokens", "access_token"}, []string{"tokens", "accessToken"},
		[]string{"credentials", "access_token"}, []string{"credentials", "accessToken"},
		[]string{"access_token"}, []string{"accessToken"}, []string{"token"},
	)
	e.RefreshToken = firstString(m,
		[]string{"tokens", "refresh_token"}, []string{"tokens", "refreshToken"},
		[]string{"credentials", "refresh_token"}, []string{"credentials", "refreshToken"},
		[]string{"refresh_token"}, []string{"refreshToken"},
	)
	e.IDToken = firstString(m,
		[]string{"tokens", "id_token"}, []string{"tokens", "idToken"},
		[]string{"credentials", "id_token"}, []string{"credentials", "idToken"},
		[]string{"id_token"}, []string{"idToken"},
	)
	e.Email = firstString(m, []string{"email"}, []string{"user", "email"}, []string{"name"})
	e.ChatGPTAccountID = firstString(m,
		[]string{"chatgpt_account_id"}, []string{"chatgptAccountId"},
		[]string{"account_id"}, []string{"accountId"},
		[]string{"account", "id"}, []string{"account", "account_id"},
		[]string{"account", "chatgpt_account_id"},
		[]string{"credentials", "chatgpt_account_id"},
	)
	e.PlanType = firstString(m,
		[]string{"plan_type"}, []string{"planType"},
		[]string{"account", "plan_type"}, []string{"account", "planType"},
	)
	e.ExpiresAt = firstScalar(m,
		[]string{"tokens", "expires_at"}, []string{"tokens", "expiresAt"},
		[]string{"credentials", "expires_at"},
		[]string{"expires_at"}, []string{"expiresAt"},
	)
	return e
}

func lookupPath(m map[string]any, path []string) (any, bool) {
	var cur any = m
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[key]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func firstString(m map[string]any, paths ...[]string) string {
	for _, p := range paths {
		if v, ok := lookupPath(m, p); ok {
			if s, ok := v.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// firstScalar returns the first string or numeric value at the given paths,
// formatted as a string (unix seconds keep their integer form).
func firstScalar(m map[string]any, paths ...[]string) string {
	for _, p := range paths {
		v, ok := lookupPath(m, p)
		if !ok {
			continue
		}
		switch n := v.(type) {
		case string:
			if s := strings.TrimSpace(n); s != "" {
				return s
			}
		case json.Number:
			return n.String()
		case float64:
			return strconv.FormatInt(int64(n), 10)
		}
	}
	return ""
}
