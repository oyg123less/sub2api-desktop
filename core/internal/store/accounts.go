package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Store) scanAccount(row interface {
	Scan(dest ...any) error
}) (*Account, error) {
	var (
		a           Account
		accessEnc   string
		refreshEnc  string
		idTokenEnc  string
		apiKeyEnc   string
		expiresAt   int64
		rateUntil   int64
		lastUsed    int64
		createdAt   int64
		updatedAt   int64
		proxyID     sql.NullInt64
		status      string
		usageJSON   string
		lastSuccess int64
		nextRetry   int64
		syncDirty   int
	)
	if err := row.Scan(&a.ID, &a.AccountType, &a.BaseURL, &apiKeyEnc, &a.Email, &a.ChatGPTAccountID, &a.PlanType,
		&accessEnc, &refreshEnc, &idTokenEnc, &expiresAt, &status, &a.StatusReason,
		&rateUntil, &proxyID, &lastUsed, &createdAt, &updatedAt, &usageJSON,
		&a.CredentialFingerprint, &lastSuccess, &a.ConsecutiveFailures, &nextRetry,
		&a.MaxConcurrency, &a.QueueCapacity, &a.ClientUID, &a.SyncVersion, &syncDirty); err != nil {
		return nil, err
	}
	if usageJSON != "" {
		var u CodexUsage
		if err := json.Unmarshal([]byte(usageJSON), &u); err == nil {
			a.CodexUsage = &u
		}
	}
	var err error
	if a.AccessToken, err = s.cipher.Decrypt(accessEnc); err != nil {
		return nil, err
	}
	if a.RefreshToken, err = s.cipher.Decrypt(refreshEnc); err != nil {
		return nil, err
	}
	if a.IDToken, err = s.cipher.Decrypt(idTokenEnc); err != nil {
		return nil, err
	}
	if a.APIKey, err = s.cipher.Decrypt(apiKeyEnc); err != nil {
		return nil, err
	}
	if a.AccountType == "" {
		a.AccountType = AccountTypeOAuth
	}
	a.Status = AccountStatus(status)
	a.ExpiresAt = unixToTime(expiresAt)
	a.CreatedAt = unixToTime(createdAt)
	a.UpdatedAt = unixToTime(updatedAt)
	a.SyncDirty = syncDirty != 0
	if proxyID.Valid {
		v := proxyID.Int64
		a.ProxyID = &v
	}
	if rateUntil != 0 {
		t := unixToTime(rateUntil)
		a.RateLimitedUntil = &t
	}
	if lastUsed != 0 {
		t := unixToTime(lastUsed)
		a.LastUsedAt = &t
	}
	if lastSuccess != 0 {
		t := unixToTime(lastSuccess)
		a.LastSuccessAt = &t
	}
	if nextRetry != 0 {
		t := unixToTime(nextRetry)
		a.NextRetryAt = &t
	}
	return &a, nil
}

const accountCols = `id, account_type, base_url, api_key, email, chatgpt_account_id, plan_type, access_token, refresh_token, id_token, expires_at, status, status_reason, rate_limited_until, proxy_id, last_used_at, created_at, updated_at, usage_snapshot, credential_fingerprint, last_success_at, consecutive_failures, next_retry_at, max_concurrency, queue_capacity, client_uid, sync_version, sync_dirty`

// CreateAccount inserts a new account (tokens encrypted).
func (s *Store) CreateAccount(a *Account) (*Account, error) {
	now := time.Now()
	accessEnc, err := s.cipher.Encrypt(a.AccessToken)
	if err != nil {
		return nil, err
	}
	refreshEnc, err := s.cipher.Encrypt(a.RefreshToken)
	if err != nil {
		return nil, err
	}
	idTokenEnc, err := s.cipher.Encrypt(a.IDToken)
	if err != nil {
		return nil, err
	}
	apiKeyEnc, err := s.cipher.Encrypt(a.APIKey)
	if err != nil {
		return nil, err
	}
	if a.AccountType == "" {
		a.AccountType = AccountTypeOAuth
	}
	if a.Status == "" {
		a.Status = AccountActive
	}
	if a.CredentialFingerprint == "" {
		a.CredentialFingerprint = AccountCredentialFingerprint(a.AccountType, a.AccessToken, a.RefreshToken, a.BaseURL, a.APIKey)
	}
	res, err := s.db.Exec(`INSERT INTO accounts
		(account_type, base_url, api_key, email, chatgpt_account_id, plan_type, access_token, refresh_token, id_token, expires_at, status, status_reason, rate_limited_until, proxy_id, last_used_at, created_at, updated_at, usage_snapshot, credential_fingerprint, last_success_at, consecutive_failures, next_retry_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		string(a.AccountType), a.BaseURL, apiKeyEnc, a.Email, a.ChatGPTAccountID, a.PlanType, accessEnc, refreshEnc, idTokenEnc,
		timeToUnix(a.ExpiresAt), string(a.Status), a.StatusReason, int64(0), a.ProxyID, int64(0),
		now.Unix(), now.Unix(), "", a.CredentialFingerprint, timeToUnixPtr(a.LastSuccessAt), a.ConsecutiveFailures, timeToUnixPtr(a.NextRetryAt))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetAccount(id)
}

// GetAccount fetches an account by id.
func (s *Store) GetAccount(id int64) (*Account, error) {
	row := s.db.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE id = ?`, id)
	a, err := s.scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// GetAccountByChatGPTID finds an account by its ChatGPT account id.
func (s *Store) GetAccountByChatGPTID(cid string) (*Account, error) {
	row := s.db.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE chatgpt_account_id = ? LIMIT 1`, cid)
	a, err := s.scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// GetAccountByFingerprint finds an exact credential match without exposing the
// token material used to derive the fingerprint.
func (s *Store) GetAccountByFingerprint(fingerprint string) (*Account, error) {
	if fingerprint == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE credential_fingerprint = ? LIMIT 1`, fingerprint)
	a, err := s.scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// ListAccounts returns all accounts ordered by creation time.
func (s *Store) ListAccounts() ([]*Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountCols + ` FROM accounts ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Account
	for rows.Next() {
		a, err := s.scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateTokens updates the token set and expiry for an account and marks it active.
func (s *Store) UpdateTokens(id int64, access, refresh, idToken string, expiresAt time.Time) error {
	accessEnc, err := s.cipher.Encrypt(access)
	if err != nil {
		return err
	}
	refreshEnc, err := s.cipher.Encrypt(refresh)
	if err != nil {
		return err
	}
	idTokenEnc, err := s.cipher.Encrypt(idToken)
	if err != nil {
		return err
	}
	fingerprint := CredentialFingerprint(access, refresh)
	_, err = s.db.Exec(`UPDATE accounts SET access_token=?, refresh_token=?, id_token=?, expires_at=?, credential_fingerprint=?, status=?, status_reason='', consecutive_failures=0, next_retry_at=0, updated_at=? WHERE id=?`,
		accessEnc, refreshEnc, idTokenEnc, timeToUnix(expiresAt), fingerprint, string(AccountActive), time.Now().Unix(), id)
	return err
}

// BackfillAccountIdentity fills identity metadata that was unavailable when the
// account was created. Existing non-empty values are never overwritten.
func (s *Store) BackfillAccountIdentity(id int64, email, chatGPTAccountID, planType string) error {
	_, err := s.db.Exec(`UPDATE accounts SET
		email=CASE WHEN email='' THEN ? ELSE email END,
		chatgpt_account_id=CASE WHEN chatgpt_account_id='' THEN ? ELSE chatgpt_account_id END,
		plan_type=CASE WHEN plan_type='' THEN ? ELSE plan_type END,
		updated_at=? WHERE id=?`, email, chatGPTAccountID, planType, time.Now().Unix(), id)
	return err
}

func timeToUnixPtr(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return timeToUnix(*t)
}

// SetAccountStatus updates status + reason.
func (s *Store) SetAccountStatus(id int64, status AccountStatus, reason string) error {
	if status == AccountActive {
		_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason='', rate_limited_until=0,
			consecutive_failures=0, next_retry_at=0, updated_at=? WHERE id=?`,
			string(status), time.Now().Unix(), id)
		return err
	}
	_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason=?, updated_at=? WHERE id=?`,
		string(status), reason, time.Now().Unix(), id)
	return err
}

func ValidateAccountLimits(maxConcurrency, queueCapacity int) error {
	if maxConcurrency < MinAccountMaxConcurrency || maxConcurrency > MaxAccountMaxConcurrency {
		return errors.New("max concurrency must be between 1 and 100")
	}
	if queueCapacity < MinAccountQueueCapacity || queueCapacity > MaxAccountQueueCapacity {
		return errors.New("queue capacity must be between 0 and 1000")
	}
	return nil
}

func (s *Store) SetAccountLimits(id int64, maxConcurrency, queueCapacity int) error {
	if err := ValidateAccountLimits(maxConcurrency, queueCapacity); err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE accounts SET max_concurrency=?, queue_capacity=?, updated_at=? WHERE id=?`,
		maxConcurrency, queueCapacity, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRateLimited marks an account unavailable until the given time and stores
// whether the upstream reported transient throttling or exhausted quota.
func (s *Store) SetRateLimited(id int64, until time.Time, reason string) error {
	_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason=?, rate_limited_until=?, updated_at=? WHERE id=?`,
		string(AccountRateLimited), reason, timeToUnix(until), time.Now().Unix(), id)
	return err
}

// TouchAccount records last-used time.
func (s *Store) TouchAccount(id int64) error {
	_, err := s.db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// RecordAccountSuccess resets transient failures and records a successful
// end-to-end upstream response.
func (s *Store) RecordAccountSuccess(id int64) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`UPDATE accounts SET
		status=CASE
			WHEN status=? THEN status
			WHEN status=? AND rate_limited_until>? THEN status
			ELSE ? END,
		status_reason=CASE
			WHEN status=? OR (status=? AND rate_limited_until>?) THEN status_reason
			ELSE '' END,
		rate_limited_until=CASE
			WHEN status=? AND rate_limited_until>? THEN rate_limited_until
			ELSE 0 END,
		last_success_at=?, consecutive_failures=0, next_retry_at=0, updated_at=? WHERE id=?`,
		string(AccountDisabled), string(AccountRateLimited), now, string(AccountActive),
		string(AccountDisabled), string(AccountRateLimited), now,
		string(AccountRateLimited), now, now, now, id)
	return err
}

// RecordAccountTestSuccess clears transient backend states after an explicit
// successful probe while preserving a user's manual disabled state.
func (s *Store) RecordAccountTestSuccess(id int64) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`UPDATE accounts SET
		status=CASE WHEN status=? THEN status ELSE ? END,
		status_reason=CASE WHEN status=? THEN status_reason ELSE '' END,
		rate_limited_until=CASE WHEN status=? THEN rate_limited_until ELSE 0 END,
		last_success_at=?, consecutive_failures=CASE WHEN status=? THEN consecutive_failures ELSE 0 END,
		next_retry_at=CASE WHEN status=? THEN next_retry_at ELSE 0 END, updated_at=? WHERE id=?`,
		string(AccountDisabled), string(AccountActive),
		string(AccountDisabled), string(AccountDisabled), now,
		string(AccountDisabled), string(AccountDisabled), now, id)
	return err
}

// RecordAccountFailure records an authentication/refresh failure with bounded
// exponential backoff. Network and upstream protocol errors must not call it.
func (s *Store) RecordAccountFailure(id int64, reason string) error {
	now := time.Now()
	result, err := s.db.Exec(`UPDATE accounts SET
		status=CASE WHEN status=? THEN status WHEN consecutive_failures>=2 THEN ? ELSE ? END,
		status_reason=CASE WHEN status=? THEN status_reason WHEN consecutive_failures>=2 THEN 'auto_disabled_auth_failures' ELSE ? END,
		next_retry_at=CASE WHEN status=? THEN next_retry_at WHEN consecutive_failures>=2 THEN 0 ELSE ?+CASE consecutive_failures
			WHEN 0 THEN 60
			WHEN 1 THEN 300
			WHEN 2 THEN 900
			ELSE 1800 END END,
		consecutive_failures=CASE WHEN status=? THEN consecutive_failures ELSE consecutive_failures+1 END,
		updated_at=? WHERE id=?`,
		string(AccountDisabled), string(AccountDisabled), string(AccountRefreshFailed), string(AccountDisabled), reason,
		string(AccountDisabled), now.Unix(), string(AccountDisabled), now.Unix(), id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RecordSevereAccountFailure(id int64, reason string) error {
	if reason == "" {
		reason = "auto_disabled_account_inactive"
	}
	result, err := s.db.Exec(`UPDATE accounts SET status=?,
		status_reason=CASE WHEN status=? THEN status_reason ELSE ? END,
		next_retry_at=CASE WHEN status=? THEN next_retry_at ELSE 0 END,
		consecutive_failures=CASE WHEN status=? THEN consecutive_failures ELSE consecutive_failures+1 END,
		updated_at=? WHERE id=?`, string(AccountDisabled), string(AccountDisabled), reason,
		string(AccountDisabled), string(AccountDisabled), time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// RecoverExpiredRateLimits makes accounts whose retry window elapsed eligible
// before the scheduler takes its next snapshot.
func (s *Store) RecoverExpiredRateLimits(now time.Time) error {
	_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason='', rate_limited_until=0, updated_at=?
		WHERE status=? AND rate_limited_until > 0 AND rate_limited_until <= ?`,
		string(AccountActive), now.Unix(), string(AccountRateLimited), now.Unix())
	return err
}

// ListAccountRuntimeStates reads only fields needed for live account polling.
// It avoids decrypting credentials or calculating accumulated usage.
func (s *Store) ListAccountRuntimeStates() ([]*AccountRuntimeState, error) {
	rows, err := s.db.Query(`SELECT id, status, status_reason, rate_limited_until FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]*AccountRuntimeState, 0)
	for rows.Next() {
		var state AccountRuntimeState
		var status string
		var limitedUntil int64
		if err := rows.Scan(&state.ID, &status, &state.StatusReason, &limitedUntil); err != nil {
			return nil, err
		}
		state.Status = AccountStatus(status)
		if limitedUntil > 0 {
			value := unixToTime(limitedUntil)
			state.RateLimitedUntil = &value
		}
		states = append(states, &state)
	}
	return states, rows.Err()
}

// SetAccountCodexUsage stores the latest Codex rate-limit window snapshot.
func (s *Store) SetAccountCodexUsage(id int64, u *CodexUsage) error {
	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE accounts SET usage_snapshot=? WHERE id=?`, string(data), id)
	return err
}

// SetAccountProxy binds (or clears) a proxy for an account.
func (s *Store) SetAccountProxy(id int64, proxyID *int64) error {
	_, err := s.db.Exec(`UPDATE accounts SET proxy_id=?, updated_at=? WHERE id=?`, proxyID, time.Now().Unix(), id)
	return err
}

// DeleteAccount removes an account.
func (s *Store) DeleteAccount(id int64) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, id)
	return err
}
