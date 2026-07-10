package store

import (
	"database/sql"
	"fmt"
	"time"
)

type AccountImportMutation struct {
	Index            int
	ExistingID       int64
	Email            string
	ChatGPTAccountID string
	PlanType         string
	AccessToken      string
	RefreshToken     string
	IDToken          string
	ExpiresAt        time.Time
	IdentityVerified bool
	LiveValidated    bool
}

type AppliedAccountImport struct {
	Index     int   `json:"index"`
	AccountID int64 `json:"account_id"`
	Created   bool  `json:"created"`
}

// ApplyAccountImports commits all selected preview rows atomically. Empty
// incoming fields preserve existing credentials and metadata.
func (s *Store) ApplyAccountImports(mutations []AccountImportMutation) ([]AppliedAccountImport, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	rollback := func(err error) ([]AppliedAccountImport, error) {
		_ = tx.Rollback()
		return nil, err
	}

	applied := make([]AppliedAccountImport, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.ExistingID == 0 {
			id, err := s.insertImportedAccount(tx, mutation)
			if err != nil {
				return rollback(fmt.Errorf("import row %d: %w", mutation.Index, err))
			}
			applied = append(applied, AppliedAccountImport{Index: mutation.Index, AccountID: id, Created: true})
			continue
		}
		if err := s.updateImportedAccount(tx, mutation); err != nil {
			return rollback(fmt.Errorf("import row %d: %w", mutation.Index, err))
		}
		applied = append(applied, AppliedAccountImport{Index: mutation.Index, AccountID: mutation.ExistingID})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return applied, nil
}

func (s *Store) insertImportedAccount(tx *sql.Tx, mutation AccountImportMutation) (int64, error) {
	accessEnc, err := s.cipher.Encrypt(mutation.AccessToken)
	if err != nil {
		return 0, err
	}
	refreshEnc, err := s.cipher.Encrypt(mutation.RefreshToken)
	if err != nil {
		return 0, err
	}
	idEnc, err := s.cipher.Encrypt(mutation.IDToken)
	if err != nil {
		return 0, err
	}
	chatGPTID := ""
	if mutation.IdentityVerified {
		chatGPTID = mutation.ChatGPTAccountID
	}
	status := AccountPending
	lastSuccess := int64(0)
	if mutation.LiveValidated {
		status = AccountActive
		lastSuccess = time.Now().Unix()
	}
	now := time.Now().Unix()
	result, err := tx.Exec(`INSERT INTO accounts
		(email, chatgpt_account_id, plan_type, access_token, refresh_token, id_token, expires_at, status, status_reason,
		 rate_limited_until, proxy_id, last_used_at, created_at, updated_at, usage_snapshot, credential_fingerprint,
		 last_success_at, consecutive_failures, next_retry_at)
		VALUES (?,?,?,?,?,?,?,?,?,0,NULL,0,?,?,'',?,?,0,0)`,
		mutation.Email, chatGPTID, mutation.PlanType, accessEnc, refreshEnc, idEnc, timeToUnix(mutation.ExpiresAt),
		string(status), "", now, now, CredentialFingerprint(mutation.AccessToken, mutation.RefreshToken), lastSuccess)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) updateImportedAccount(tx *sql.Tx, mutation AccountImportMutation) error {
	existing, err := s.scanAccount(tx.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE id=?`, mutation.ExistingID))
	if err != nil {
		return err
	}
	accessChanged := mutation.AccessToken != ""
	if mutation.AccessToken == "" {
		mutation.AccessToken = existing.AccessToken
	}
	if mutation.RefreshToken == "" {
		mutation.RefreshToken = existing.RefreshToken
	}
	if mutation.IDToken == "" {
		mutation.IDToken = existing.IDToken
	}
	if mutation.ExpiresAt.IsZero() {
		if accessChanged {
			mutation.ExpiresAt = time.Time{}
		} else {
			mutation.ExpiresAt = existing.ExpiresAt
		}
	}
	if !mutation.IdentityVerified {
		mutation.Email = existing.Email
		mutation.ChatGPTAccountID = existing.ChatGPTAccountID
		mutation.PlanType = existing.PlanType
	} else {
		if mutation.Email == "" {
			mutation.Email = existing.Email
		}
		if mutation.ChatGPTAccountID == "" {
			mutation.ChatGPTAccountID = existing.ChatGPTAccountID
		}
		if mutation.PlanType == "" {
			mutation.PlanType = existing.PlanType
		}
	}
	accessEnc, err := s.cipher.Encrypt(mutation.AccessToken)
	if err != nil {
		return err
	}
	refreshEnc, err := s.cipher.Encrypt(mutation.RefreshToken)
	if err != nil {
		return err
	}
	idEnc, err := s.cipher.Encrypt(mutation.IDToken)
	if err != nil {
		return err
	}
	status := existing.Status
	statusReason := existing.StatusReason
	lastSuccess := timeToUnixPtr(existing.LastSuccessAt)
	if mutation.LiveValidated {
		status = AccountActive
		statusReason = ""
		lastSuccess = time.Now().Unix()
	}
	result, err := tx.Exec(`UPDATE accounts SET email=?, chatgpt_account_id=?, plan_type=?, access_token=?, refresh_token=?,
		id_token=?, expires_at=?, status=?, status_reason=?, credential_fingerprint=?, last_success_at=?, updated_at=? WHERE id=?`,
		mutation.Email, mutation.ChatGPTAccountID, mutation.PlanType, accessEnc, refreshEnc, idEnc, timeToUnix(mutation.ExpiresAt),
		string(status), statusReason, CredentialFingerprint(mutation.AccessToken, mutation.RefreshToken), lastSuccess, time.Now().Unix(), mutation.ExistingID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return ErrNotFound
	}
	return nil
}
