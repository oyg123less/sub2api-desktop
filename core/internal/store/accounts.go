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
	)
	if err := row.Scan(&a.ID, &a.Email, &a.ChatGPTAccountID, &a.PlanType,
		&accessEnc, &refreshEnc, &idTokenEnc, &expiresAt, &status, &a.StatusReason,
		&rateUntil, &proxyID, &lastUsed, &createdAt, &updatedAt, &usageJSON,
		&a.CredentialFingerprint, &lastSuccess, &a.ConsecutiveFailures, &nextRetry); err != nil {
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
	a.Status = AccountStatus(status)
	a.ExpiresAt = unixToTime(expiresAt)
	a.CreatedAt = unixToTime(createdAt)
	a.UpdatedAt = unixToTime(updatedAt)
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

const accountCols = `id, email, chatgpt_account_id, plan_type, access_token, refresh_token, id_token, expires_at, status, status_reason, rate_limited_until, proxy_id, last_used_at, created_at, updated_at, usage_snapshot, credential_fingerprint, last_success_at, consecutive_failures, next_retry_at`

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
	if a.Status == "" {
		a.Status = AccountActive
	}
	if a.CredentialFingerprint == "" {
		a.CredentialFingerprint = CredentialFingerprint(a.AccessToken, a.RefreshToken)
	}
	res, err := s.db.Exec(`INSERT INTO accounts
		(email, chatgpt_account_id, plan_type, access_token, refresh_token, id_token, expires_at, status, status_reason, rate_limited_until, proxy_id, last_used_at, created_at, updated_at, usage_snapshot, credential_fingerprint, last_success_at, consecutive_failures, next_retry_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,'',?,?,?,?)`,
		a.Email, a.ChatGPTAccountID, a.PlanType, accessEnc, refreshEnc, idTokenEnc,
		timeToUnix(a.ExpiresAt), string(a.Status), a.StatusReason, int64(0), a.ProxyID, int64(0),
		now.Unix(), now.Unix(), a.CredentialFingerprint, timeToUnixPtr(a.LastSuccessAt), a.ConsecutiveFailures, timeToUnixPtr(a.NextRetryAt))
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

func timeToUnixPtr(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return timeToUnix(*t)
}

// SetAccountStatus updates status + reason.
func (s *Store) SetAccountStatus(id int64, status AccountStatus, reason string) error {
	_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason=?, updated_at=? WHERE id=?`,
		string(status), reason, time.Now().Unix(), id)
	return err
}

// SetRateLimited marks an account rate-limited until the given time.
func (s *Store) SetRateLimited(id int64, until time.Time) error {
	_, err := s.db.Exec(`UPDATE accounts SET status=?, rate_limited_until=?, updated_at=? WHERE id=?`,
		string(AccountRateLimited), timeToUnix(until), time.Now().Unix(), id)
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
	_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason='', rate_limited_until=0,
		last_success_at=?, consecutive_failures=0, next_retry_at=0, updated_at=? WHERE id=?`,
		string(AccountActive), now, now, id)
	return err
}

// RecordAccountFailure records an authentication/refresh failure with bounded
// exponential backoff. Network and upstream protocol errors must not call it.
func (s *Store) RecordAccountFailure(id int64, reason string) error {
	var failures int
	if err := s.db.QueryRow(`SELECT consecutive_failures FROM accounts WHERE id=?`, id).Scan(&failures); err != nil {
		return err
	}
	failures++
	backoffs := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute}
	idx := failures - 1
	if idx >= len(backoffs) {
		idx = len(backoffs) - 1
	}
	now := time.Now()
	_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason=?, consecutive_failures=?, next_retry_at=?, updated_at=? WHERE id=?`,
		string(AccountRefreshFailed), reason, failures, now.Add(backoffs[idx]).Unix(), now.Unix(), id)
	return err
}

// RecoverExpiredRateLimits makes accounts whose retry window elapsed eligible
// before the scheduler takes its next snapshot.
func (s *Store) RecoverExpiredRateLimits(now time.Time) error {
	_, err := s.db.Exec(`UPDATE accounts SET status=?, status_reason='', rate_limited_until=0, updated_at=?
		WHERE status=? AND rate_limited_until > 0 AND rate_limited_until <= ?`,
		string(AccountActive), now.Unix(), string(AccountRateLimited), now.Unix())
	return err
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
