package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	WorkspaceStateLocal    = "local"
	WorkspaceStateBound    = "bound"
	WorkspaceStateRecovery = "recovery"
)

type WorkspaceMeta struct {
	WorkspaceID          string `json:"workspace_id"`
	State                string `json:"state"`
	BoundCloudUserID     int64  `json:"bound_cloud_user_id,omitempty"`
	BoundEmail           string `json:"bound_email,omitempty"`
	SuggestedCloudUserID int64  `json:"suggested_cloud_user_id,omitempty"`
	RecoveryReason       string `json:"recovery_reason,omitempty"`
	AccountCount         int    `json:"account_count"`
	ProxyCount           int    `json:"proxy_count"`
	PendingOutbox        int    `json:"pending_outbox"`
	QuarantinedItems     int    `json:"quarantined_items"`
}

type WorkspaceOwnershipError struct {
	Code string
	Meta WorkspaceMeta
}

func (e *WorkspaceOwnershipError) Error() string {
	switch e.Code {
	case "workspace_binding_required":
		return "Confirm that this local workspace should belong to the cloud account before continuing."
	case "workspace_account_mismatch":
		return "This workspace belongs to a different cloud account. Switch or create a workspace before signing in."
	case "legacy_workspace_ambiguous":
		return "This legacy workspace contains data from multiple or unknown cloud users and is available only for recovery."
	case "cloud_workspace_owner_mismatch":
		return "Cloud sync ownership validation failed for this workspace."
	default:
		return "The cloud account does not match this workspace."
	}
}

func (s *Store) InitializeWorkspace(workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return errors.New("workspace id is required")
	}
	result, err := s.db.Exec(`UPDATE workspace_meta SET workspace_id=?,updated_at=?
		WHERE id=1 AND workspace_id=''`, workspaceID, time.Now().Unix())
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 1 {
		return nil
	}
	var current string
	if err := s.db.QueryRow(`SELECT workspace_id FROM workspace_meta WHERE id=1`).Scan(&current); err != nil {
		return err
	}
	if current != workspaceID {
		return fmt.Errorf("workspace database belongs to %q, not %q", current, workspaceID)
	}
	return nil
}

func (s *Store) WorkspaceMeta() (WorkspaceMeta, error) {
	var meta WorkspaceMeta
	var boundUser, suggestedUser sql.NullInt64
	err := s.db.QueryRow(`SELECT workspace_id,state,bound_cloud_user_id,bound_email,
		suggested_cloud_user_id,recovery_reason FROM workspace_meta WHERE id=1`).Scan(
		&meta.WorkspaceID, &meta.State, &boundUser, &meta.BoundEmail, &suggestedUser, &meta.RecoveryReason)
	if err != nil {
		return meta, err
	}
	if boundUser.Valid {
		meta.BoundCloudUserID = boundUser.Int64
	}
	if suggestedUser.Valid {
		meta.SuggestedCloudUserID = suggestedUser.Int64
	}
	if err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM accounts),(SELECT COUNT(*) FROM proxies),
		(SELECT COUNT(*) FROM cloud_sync_outbox WHERE quarantined=0),
		((SELECT COUNT(*) FROM cloud_sync_outbox WHERE quarantined=1)+
		 (SELECT COUNT(*) FROM cloud_sync_tombstones WHERE quarantined=1)+
		 (SELECT COUNT(*) FROM cloud_sync_conflicts WHERE quarantined=1))`).Scan(
		&meta.AccountCount, &meta.ProxyCount, &meta.PendingOutbox, &meta.QuarantinedItems); err != nil {
		return meta, err
	}
	return meta, nil
}

func (s *Store) BindCloudUser(userID int64, email string, confirmed bool) error {
	if userID <= 0 || strings.TrimSpace(email) == "" {
		return errors.New("invalid cloud workspace owner")
	}
	meta, err := s.WorkspaceMeta()
	if err != nil {
		return err
	}
	switch meta.State {
	case WorkspaceStateBound:
		if meta.BoundCloudUserID != userID {
			return &WorkspaceOwnershipError{Code: "workspace_account_mismatch", Meta: meta}
		}
		return nil
	case WorkspaceStateRecovery:
		if meta.RecoveryReason != "legacy_owner_confirmation_required" || meta.SuggestedCloudUserID != userID {
			return &WorkspaceOwnershipError{Code: "legacy_workspace_ambiguous", Meta: meta}
		}
		if !confirmed {
			return &WorkspaceOwnershipError{Code: "workspace_binding_required", Meta: meta}
		}
	case WorkspaceStateLocal:
		if !confirmed && (meta.AccountCount > 0 || meta.ProxyCount > 0 || meta.PendingOutbox > 0) {
			return &WorkspaceOwnershipError{Code: "workspace_binding_required", Meta: meta}
		}
	default:
		return errors.New("invalid workspace state")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE workspace_meta SET state='bound',bound_cloud_user_id=?,bound_email=?,
		suggested_cloud_user_id=NULL,recovery_reason='',updated_at=? WHERE id=1`, userID,
		strings.ToLower(strings.TrimSpace(email)), time.Now().Unix()); err != nil {
		return err
	}
	for _, statement := range []string{
		`UPDATE cloud_sync_outbox SET owner_user_id=?,quarantined=0 WHERE owner_user_id=0`,
		`UPDATE cloud_sync_tombstones SET owner_user_id=?,quarantined=0 WHERE owner_user_id=0`,
		`UPDATE cloud_sync_conflicts SET owner_user_id=?,quarantined=0 WHERE owner_user_id=0`,
		`UPDATE cloud_sync_runtime SET owner_user_id=? WHERE owner_user_id=0`,
	} {
		if _, err := tx.Exec(statement, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AssertCloudOwner(userID int64) error {
	meta, err := s.WorkspaceMeta()
	if err != nil {
		return err
	}
	if meta.State != WorkspaceStateBound || meta.BoundCloudUserID != userID || userID <= 0 {
		return &WorkspaceOwnershipError{Code: "cloud_workspace_owner_mismatch", Meta: meta}
	}
	return nil
}

func (s *Store) boundCloudUserID() (int64, error) {
	var userID sql.NullInt64
	var state string
	if err := s.db.QueryRow(`SELECT state,bound_cloud_user_id FROM workspace_meta WHERE id=1`).Scan(&state, &userID); err != nil {
		return 0, err
	}
	if state != WorkspaceStateBound || !userID.Valid {
		return 0, nil
	}
	return userID.Int64, nil
}
