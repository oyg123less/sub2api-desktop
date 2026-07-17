package account

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/redact"
	"sub2api-desktop/core/internal/store"
)

type ImportEntry struct {
	AccountType      string `json:"account_type"`
	BaseURL          string `json:"base_url"`
	APIKey           string `json:"api_key"`
	Email            string `json:"email"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	PlanType         string `json:"plan_type"`
	ExpiresAt        string `json:"expires_at"`
	ProxyID          *int64 `json:"proxy_id,omitempty"`
	ProxySpecified   bool   `json:"-"`
	ProxyError       string `json:"-"`
}

type ImportProxyMode string

const (
	ImportProxyPreserve ImportProxyMode = "preserve"
	ImportProxyDirect   ImportProxyMode = "direct"
	ImportProxyOverride ImportProxyMode = "override"
)

type ImportOptions struct {
	ProxyMode ImportProxyMode
	ProxyID   *int64
}

type ImportAction string

const (
	ImportCreate   ImportAction = "create"
	ImportUpdate   ImportAction = "update"
	ImportSkip     ImportAction = "skip"
	ImportError    ImportAction = "error"
	ImportConflict ImportAction = "conflict"
)

type ImportPreviewSummary struct {
	Total    int `json:"total"`
	Create   int `json:"create"`
	Update   int `json:"update"`
	Skip     int `json:"skip"`
	Error    int `json:"error"`
	Conflict int `json:"conflict"`
}

type ImportPreviewRow struct {
	Index                  int           `json:"index"`
	Action                 ImportAction  `json:"action"`
	AccountType            string        `json:"account_type"`
	MatchedAccountID       int64         `json:"matched_account_id,omitempty"`
	EmailMasked            string        `json:"email_masked,omitempty"`
	ChatGPTAccountIDMasked string        `json:"chatgpt_account_id_masked,omitempty"`
	HasAccessToken         bool          `json:"has_access_token"`
	HasRefreshToken        bool          `json:"has_refresh_token"`
	HasIDToken             bool          `json:"has_id_token"`
	HasAPIKey              bool          `json:"has_api_key"`
	IdentityLevel          IdentityLevel `json:"identity_level"`
	IdentityVerified       bool          `json:"identity_verified"`
	Warnings               []string      `json:"warnings"`
	WarningCodes           []string      `json:"warning_codes"`
	ErrorCode              string        `json:"error_code,omitempty"`
	ErrorMessage           string        `json:"error_message,omitempty"`
	ProxyID                *int64        `json:"proxy_id,omitempty"`
	ProxySpecified         bool          `json:"proxy_specified"`
}

type ImportPreview struct {
	ContentSHA256 string               `json:"content_sha256"`
	Summary       ImportPreviewSummary `json:"summary"`
	Rows          []ImportPreviewRow   `json:"rows"`
	plan          []store.AccountImportMutation
}

const importPreviewTTL = 10 * time.Minute

type cachedImportPreview struct {
	preview   *ImportPreview
	expiresAt time.Time
}

type ImportCommitResult struct {
	ContentSHA256 string               `json:"content_sha256"`
	Imported      int                  `json:"imported"`
	Updated       int                  `json:"updated"`
	Skipped       int                  `json:"skipped"`
	Failed        int                  `json:"failed"`
	Validated     int                  `json:"validated"`
	Warnings      []string             `json:"warnings,omitempty"`
	Rows          []ImportPreviewRow   `json:"rows"`
	Summary       ImportPreviewSummary `json:"summary"`
}

type ImportServiceError struct {
	Code      string
	Message   string
	Retryable bool
	Details   map[string]any
}

func (e *ImportServiceError) Error() string { return e.Message }

func contentHash(raw []byte) string {
	return contentHashWithOptions(raw, ImportOptions{})
}

func contentHashWithOptions(raw []byte, options ImportOptions) string {
	sum := sha256.Sum256(raw)
	if options.ProxyMode != "" && options.ProxyMode != ImportProxyPreserve {
		value := hex.EncodeToString(sum[:]) + "\x00proxy_mode=" + string(options.ProxyMode)
		if options.ProxyID != nil {
			value += "\x00proxy_id=" + strconv.FormatInt(*options.ProxyID, 10)
		}
		sum = sha256.Sum256([]byte(value))
	}
	return hex.EncodeToString(sum[:])
}

func (m *Manager) PreviewImport(ctx context.Context, raw []byte) (*ImportPreview, error) {
	return m.PreviewImportWithOptions(ctx, raw, ImportOptions{})
}

func (m *Manager) PreviewImportWithOptions(ctx context.Context, raw []byte, options ImportOptions) (*ImportPreview, error) {
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	preview, err := m.buildImportPreview(ctx, raw, options)
	if err != nil {
		return nil, err
	}
	m.cacheImportPreview(preview)
	return preview, nil
}

func (m *Manager) buildImportPreview(ctx context.Context, raw []byte, options ImportOptions) (*ImportPreview, error) {
	parsed, err := ParseImportDocument(raw)
	if err != nil {
		return nil, &ImportServiceError{Code: "import_parse_failed", Message: err.Error()}
	}
	preview := &ImportPreview{ContentSHA256: contentHashWithOptions(raw, options), Rows: make([]ImportPreviewRow, 0, len(parsed))}
	preview.Summary.Total = len(parsed)
	seenIdentity := map[string]struct {
		fingerprint string
		index       int
	}{}
	seenFingerprint := map[string]struct {
		verifiedID string
		index      int
	}{}

	for _, parsedEntry := range parsed {
		entry := parsedEntry.Entry
		applyImportProxyOptions(&entry, options)
		entry.AccessToken = strings.TrimSpace(entry.AccessToken)
		entry.RefreshToken = strings.TrimSpace(entry.RefreshToken)
		entry.IDToken = strings.TrimSpace(entry.IDToken)
		entry.APIKey = strings.TrimSpace(entry.APIKey)
		entry.BaseURL = strings.TrimSpace(entry.BaseURL)
		row := ImportPreviewRow{
			AccountType: entry.AccountType,
			Index:       parsedEntry.Index, HasAccessToken: entry.AccessToken != "", HasRefreshToken: entry.RefreshToken != "",
			HasIDToken: entry.IDToken != "", HasAPIKey: entry.APIKey != "", IdentityLevel: IdentityUnparsed,
			Warnings: append([]string(nil), parsedEntry.Warnings...),
		}
		row.ProxyID, row.ProxySpecified = entry.ProxyID, entry.ProxySpecified
		if parsedEntry.Err != nil {
			row.Action, row.ErrorCode, row.ErrorMessage = ImportError, "import_invalid_row", parsedEntry.Err.Error()
			preview.addRow(row)
			continue
		}
		if entry.ProxyError != "" {
			row.Action, row.ErrorCode, row.ErrorMessage = ImportError, "import_invalid_proxy", entry.ProxyError
			preview.addRow(row)
			continue
		}
		if entry.ProxySpecified && entry.ProxyID != nil {
			if _, err := m.store.GetProxy(*entry.ProxyID); err != nil {
				row.Action, row.ErrorCode, row.ErrorMessage = ImportError, "import_proxy_not_found", "selected proxy does not exist"
				preview.addRow(row)
				continue
			}
		}

		identity := VerifiedIdentity{Level: IdentityUnparsed}
		if store.AccountType(entry.AccountType) != store.AccountTypeAPIKey {
			var identityWarnings, identityWarningCodes []string
			identity, identityWarnings, identityWarningCodes = m.resolveImportIdentity(ctx, entry)
			row.Warnings = append(row.Warnings, identityWarnings...)
			row.WarningCodes = append(row.WarningCodes, identityWarningCodes...)
			row.IdentityLevel = identity.Level
			row.IdentityVerified = identity.Level == IdentitySigned
		}
		displayEmail, displayID := entry.Email, entry.ChatGPTAccountID
		if identity.Email != "" {
			displayEmail = identity.Email
		}
		if identity.ChatGPTAccountID != "" {
			displayID = identity.ChatGPTAccountID
		}
		row.EmailMasked = redact.MaskEmail(displayEmail)
		if displayEmail == "" {
			row.EmailMasked = ""
		}
		row.ChatGPTAccountIDMasked = maskAccountID(displayID)

		verifiedID := ""
		if row.IdentityVerified {
			verifiedID = identity.ChatGPTAccountID
			entry.ChatGPTAccountID = verifiedID
			if identity.Email != "" {
				entry.Email = identity.Email
			}
			if identity.PlanType != "" {
				entry.PlanType = identity.PlanType
			}
		} else {
			if entry.ChatGPTAccountID == "" {
				entry.ChatGPTAccountID = identity.ChatGPTAccountID
			}
			if entry.Email == "" {
				entry.Email = identity.Email
			}
			if entry.PlanType == "" {
				entry.PlanType = identity.PlanType
			}
			if entry.ChatGPTAccountID != "" {
				row.Warnings = append(row.Warnings, "unverified account identity is used for forwarding only and will not be used for matching")
			}
		}

		accountType := store.AccountType(entry.AccountType)
		fingerprint := store.AccountCredentialFingerprint(accountType, entry.AccessToken, entry.RefreshToken, entry.BaseURL, entry.APIKey)
		if previous, duplicate := seenFingerprint[fingerprint]; duplicate && fingerprint != "" {
			if previous.verifiedID != "" && verifiedID != "" && previous.verifiedID != verifiedID {
				row.Action, row.ErrorCode = ImportConflict, "import_duplicate_conflict"
				row.ErrorMessage = fmt.Sprintf("credential fingerprint is shared by different verified identities in row %d", previous.index)
			} else {
				row.Action = ImportSkip
				switch {
				case previous.verifiedID != "" && previous.verifiedID == verifiedID:
					row.Warnings = append(row.Warnings, fmt.Sprintf("same verified identity and credentials as row %d", previous.index))
				case previous.verifiedID == "" && verifiedID == "":
					row.Warnings = append(row.Warnings, fmt.Sprintf("credentials match row %d but neither row has a verified identity; conservatively deduplicated", previous.index))
				default:
					row.Warnings = append(row.Warnings, fmt.Sprintf("exact credential duplicate of row %d", previous.index))
				}
			}
			preview.addRow(row)
			continue
		}

		if previous, duplicate := seenIdentity[verifiedID]; duplicate && verifiedID != "" {
			if previous.fingerprint == fingerprint {
				row.Action = ImportSkip
				row.Warnings = append(row.Warnings, fmt.Sprintf("same verified identity and credentials as row %d", previous.index))
			} else {
				row.Action, row.ErrorCode = ImportConflict, "import_duplicate_conflict"
				row.ErrorMessage = fmt.Sprintf("verified identity conflicts with different credentials in row %d", previous.index)
			}
			preview.addRow(row)
			continue
		}

		matched, matchErr := m.matchImportAccount(verifiedID, fingerprint)
		if matchErr != nil {
			row.Action, row.ErrorCode, row.ErrorMessage = ImportError, "import_match_failed", matchErr.Error()
			preview.addRow(row)
			continue
		}
		if matched != nil && verifiedID != "" && matched.ChatGPTAccountID != "" && matched.ChatGPTAccountID != verifiedID {
			row.Action, row.ErrorCode, row.ErrorMessage = ImportConflict, "import_duplicate_conflict", "credential fingerprint belongs to a different verified identity"
			preview.addRow(row)
			continue
		}

		mutation := store.AccountImportMutation{
			AccountType: accountType, BaseURL: entry.BaseURL, APIKey: entry.APIKey,
			Index: parsedEntry.Index, Email: entry.Email, ChatGPTAccountID: entry.ChatGPTAccountID, PlanType: entry.PlanType,
			AccessToken: entry.AccessToken, RefreshToken: entry.RefreshToken, IDToken: entry.IDToken,
			ExpiresAt: parseExpiry(entry.ExpiresAt), IdentityVerified: row.IdentityVerified,
			ProxyID: entry.ProxyID, ProxySpecified: entry.ProxySpecified,
		}
		if matched != nil {
			mutation.ExistingID = matched.ID
			row.Action = ImportUpdate
			row.MatchedAccountID = matched.ID
		} else {
			row.Action = ImportCreate
		}
		preview.plan = append(preview.plan, mutation)
		if fingerprint != "" {
			seenFingerprint[fingerprint] = struct {
				verifiedID string
				index      int
			}{verifiedID: verifiedID, index: parsedEntry.Index}
		}
		if verifiedID != "" {
			seenIdentity[verifiedID] = struct {
				fingerprint string
				index       int
			}{fingerprint: fingerprint, index: parsedEntry.Index}
		}
		preview.addRow(row)
	}
	return preview, nil
}

func (m *Manager) cacheImportPreview(preview *ImportPreview) {
	now := time.Now()
	m.previewMu.Lock()
	defer m.previewMu.Unlock()
	for sha, cached := range m.previewPlans {
		if !now.Before(cached.expiresAt) {
			delete(m.previewPlans, sha)
		}
	}
	m.previewPlans[preview.ContentSHA256] = cachedImportPreview{
		preview: cloneImportPreview(preview), expiresAt: now.Add(importPreviewTTL),
	}
}

func (m *Manager) takeCachedImportPreview(sha string) (*ImportPreview, bool) {
	m.previewMu.Lock()
	defer m.previewMu.Unlock()
	cached, ok := m.previewPlans[sha]
	if !ok {
		return nil, false
	}
	delete(m.previewPlans, sha)
	if !time.Now().Before(cached.expiresAt) {
		return nil, false
	}
	return cloneImportPreview(cached.preview), true
}

func cloneImportPreview(source *ImportPreview) *ImportPreview {
	clone := *source
	clone.plan = append([]store.AccountImportMutation(nil), source.plan...)
	clone.Rows = append([]ImportPreviewRow(nil), source.Rows...)
	for index := range clone.Rows {
		clone.Rows[index].Warnings = append([]string(nil), source.Rows[index].Warnings...)
		clone.Rows[index].WarningCodes = append([]string(nil), source.Rows[index].WarningCodes...)
	}
	return &clone
}

func (p *ImportPreview) addRow(row ImportPreviewRow) {
	p.Rows = append(p.Rows, row)
	switch row.Action {
	case ImportCreate:
		p.Summary.Create++
	case ImportUpdate:
		p.Summary.Update++
	case ImportSkip:
		p.Summary.Skip++
	case ImportConflict:
		p.Summary.Conflict++
	case ImportError:
		p.Summary.Error++
	}
}

func (m *Manager) resolveImportIdentity(ctx context.Context, entry ImportEntry) (VerifiedIdentity, []string, []string) {
	warnings := []string{}
	warningCodes := []string{}
	if entry.IDToken != "" {
		identity, err := m.identityVerifier.VerifyIDToken(ctx, entry.IDToken)
		if err == nil {
			if identity.Expired {
				warnings = append(warnings, "verified ID token is expired")
			}
			return identity, warnings, warningCodes
		}
		identity = decodeIdentity(entry.IDToken)
		warningCodes = append(warningCodes, identityVerificationWarningCode(err))
		if identity.Expired {
			warnings = append(warnings, "ID token is expired")
		}
		return identity, warnings, warningCodes
	}
	if entry.AccessToken != "" {
		identity := decodeIdentity(entry.AccessToken)
		if identity.Level == IdentityDecoded {
			warnings = append(warnings, "access token identity is decoded but unverified")
		}
		return identity, warnings, warningCodes
	}
	return VerifiedIdentity{Level: IdentityUnparsed}, warnings, warningCodes
}

func (m *Manager) matchImportAccount(verifiedID, fingerprint string) (*store.Account, error) {
	var byIdentity, byFingerprint *store.Account
	var err error
	if verifiedID != "" {
		byIdentity, err = m.store.GetAccountByChatGPTID(verifiedID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	if fingerprint != "" {
		byFingerprint, err = m.store.GetAccountByFingerprint(fingerprint)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	if byIdentity != nil && byFingerprint != nil && byIdentity.ID != byFingerprint.ID {
		return nil, &ImportServiceError{Code: "import_duplicate_conflict", Message: "identity and credential fingerprint match different accounts"}
	}
	if byIdentity != nil {
		return byIdentity, nil
	}
	return byFingerprint, nil
}

func (m *Manager) CommitImport(ctx context.Context, raw []byte, expectedSHA string, validate bool) (*ImportCommitResult, error) {
	return m.CommitImportWithOptions(ctx, raw, expectedSHA, validate, ImportOptions{})
}

func (m *Manager) CommitImportWithOptions(ctx context.Context, raw []byte, expectedSHA string, validate bool, options ImportOptions) (*ImportCommitResult, error) {
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	actualSHA := contentHashWithOptions(raw, options)
	if !strings.EqualFold(strings.TrimSpace(expectedSHA), actualSHA) {
		return nil, &ImportServiceError{
			Code: "import_preview_mismatch", Message: "import content differs from the preview", Details: map[string]any{
				"expected_sha256": strings.TrimSpace(expectedSHA), "actual_sha256": actualSHA,
			},
		}
	}
	preview, ok := m.takeCachedImportPreview(actualSHA)
	if !ok {
		var err error
		preview, err = m.buildImportPreview(ctx, raw, options)
		if err != nil {
			return nil, err
		}
	}
	if preview.Summary.Conflict > 0 {
		return nil, &ImportServiceError{Code: "import_duplicate_conflict", Message: "import contains conflicting credentials for the same verified identity", Details: map[string]any{"conflicts": preview.Summary.Conflict}}
	}
	applied, err := m.store.ApplyAccountImports(preview.plan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ImportServiceError{Code: "import_account_not_found", Message: err.Error()}
		}
		return nil, &ImportServiceError{Code: "import_commit_failed", Message: err.Error()}
	}
	result := &ImportCommitResult{
		ContentSHA256: actualSHA, Imported: preview.Summary.Create, Updated: preview.Summary.Update,
		Skipped: preview.Summary.Skip, Failed: preview.Summary.Error, Rows: preview.Rows, Summary: preview.Summary,
	}
	if validate {
		result.Validated, result.Warnings = m.validateImportedAccounts(ctx, applied)
	}
	return result, nil
}

func normalizeImportOptions(options ImportOptions) (ImportOptions, error) {
	if options.ProxyMode == "" {
		options.ProxyMode = ImportProxyPreserve
	}
	switch options.ProxyMode {
	case ImportProxyPreserve:
		options.ProxyID = nil
	case ImportProxyDirect:
		options.ProxyID = nil
	case ImportProxyOverride:
		if options.ProxyID == nil || *options.ProxyID <= 0 {
			return options, &ImportServiceError{Code: "import_invalid_proxy", Message: "a valid proxy is required for proxy override"}
		}
	default:
		return options, &ImportServiceError{Code: "import_invalid_proxy", Message: "invalid import proxy mode"}
	}
	return options, nil
}

func applyImportProxyOptions(entry *ImportEntry, options ImportOptions) {
	switch options.ProxyMode {
	case ImportProxyDirect:
		entry.ProxyID = nil
		entry.ProxySpecified = true
	case ImportProxyOverride:
		entry.ProxyID = options.ProxyID
		entry.ProxySpecified = true
	}
}

func (m *Manager) validateImportedAccounts(ctx context.Context, applied []store.AppliedAccountImport) (int, []string) {
	overallCtx, cancelOverall := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelOverall()
	type validationWarning struct {
		index   int
		message string
	}
	jobs := make(chan store.AppliedAccountImport)
	var wait sync.WaitGroup
	var mu sync.Mutex
	validated := 0
	warningRows := []validationWarning{}
	addWarning := func(index int, message string) {
		mu.Lock()
		warningRows = append(warningRows, validationWarning{index: index, message: message})
		mu.Unlock()
	}
	worker := func() {
		defer wait.Done()
		for item := range jobs {
			if overallCtx.Err() != nil {
				addWarning(item.Index, fmt.Sprintf("row %d validation skipped: overall timeout", item.Index))
				continue
			}
			account, err := m.store.GetAccount(item.AccountID)
			if err != nil {
				addWarning(item.Index, fmt.Sprintf("row %d validation: %v", item.Index, err))
				continue
			}
			if account.AccountType == store.AccountTypeAPIKey {
				continue
			}
			if account.RefreshToken == "" {
				addWarning(item.Index, fmt.Sprintf("row %d validation skipped: no refresh token", item.Index))
				continue
			}
			if account.ProxyID != nil {
				addWarning(item.Index, fmt.Sprintf("row %d validation deferred: account uses a proxy", item.Index))
				continue
			}
			validationCtx, cancel := context.WithTimeout(overallCtx, 30*time.Second)
			err = m.Refresh(validationCtx, &http.Client{Timeout: 30 * time.Second}, item.AccountID)
			cancel()
			if err != nil {
				addWarning(item.Index, fmt.Sprintf("row %d validation failed: %s", item.Index, redact.Sanitize(err.Error())))
				continue
			}
			_ = m.store.RecordAccountSuccess(item.AccountID)
			mu.Lock()
			validated++
			mu.Unlock()
		}
	}
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go worker()
	}
	for _, item := range applied {
		select {
		case jobs <- item:
		case <-overallCtx.Done():
			addWarning(item.Index, fmt.Sprintf("row %d validation skipped: overall timeout", item.Index))
		}
	}
	close(jobs)
	wait.Wait()
	sort.Slice(warningRows, func(i, j int) bool { return warningRows[i].index < warningRows[j].index })
	warnings := make([]string, 0, len(warningRows))
	for _, warning := range warningRows {
		warnings = append(warnings, warning.message)
	}
	return validated, warnings
}

// ImportResult and Import preserve the v0.1.x control endpoint while routing
// the operation through the same preview and transactional commit path.
type ImportResult struct {
	Imported int      `json:"imported"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

func (m *Manager) Import(entries []ImportEntry) ImportResult {
	raw, err := json.Marshal(entries)
	if err != nil {
		return ImportResult{Skipped: len(entries), Errors: []string{err.Error()}}
	}
	result, err := m.CommitImport(context.Background(), raw, contentHash(raw), false)
	if err != nil {
		return ImportResult{Skipped: len(entries), Errors: []string{err.Error()}}
	}
	return ImportResult{Imported: result.Imported, Updated: result.Updated, Skipped: result.Skipped + result.Failed}
}

func parseExpiry(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil && unix > 0 {
		if unix > 1e12 {
			return time.UnixMilli(unix)
		}
		return time.Unix(unix, 0)
	}
	return time.Time{}
}

func maskAccountID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 10 {
		if value == "" {
			return ""
		}
		return value[:min(2, len(value))] + "***"
	}
	return value[:6] + "..." + value[len(value)-4:]
}
