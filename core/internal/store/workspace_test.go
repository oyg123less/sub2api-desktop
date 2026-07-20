package store

import (
	"errors"
	"testing"
)

func TestWorkspaceBindingCannotBeTakenOverByAnotherCloudUser(t *testing.T) {
	st := openCloudTestStore(t)
	if err := st.InitializeWorkspace("ws_owner_test"); err != nil {
		t.Fatal(err)
	}
	if err := st.BindCloudUser(101, "owner-a@example.test", true); err != nil {
		t.Fatal(err)
	}
	if err := st.BindCloudUser(202, "owner-b@example.test", true); err == nil {
		t.Fatal("a second cloud user took over the bound workspace")
	} else {
		var ownership *WorkspaceOwnershipError
		if !errors.As(err, &ownership) || ownership.Code != "workspace_account_mismatch" {
			t.Fatalf("unexpected mismatch error: %#v", err)
		}
	}
	meta, err := st.WorkspaceMeta()
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != WorkspaceStateBound || meta.BoundCloudUserID != 101 || meta.BoundEmail != "owner-a@example.test" {
		t.Fatalf("workspace owner changed after rejected login: %+v", meta)
	}
	if err := st.InitializeWorkspace("ws_different"); err == nil {
		t.Fatal("the same database accepted a different workspace id")
	}
}

func TestAmbiguousRecoveryWorkspaceCannotAutoBindOrReleaseQuarantine(t *testing.T) {
	st := openCloudTestStore(t)
	if _, err := st.db.Exec(`UPDATE workspace_meta SET state='recovery',bound_cloud_user_id=NULL,
		suggested_cloud_user_id=NULL,recovery_reason='legacy_multiple_cloud_users' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`INSERT INTO cloud_sync_outbox(owner_user_id,idempotency_key,payload_json,created_at,quarantined)
		VALUES(0,'recovery-batch','ciphertext',unixepoch(),1)`); err != nil {
		t.Fatal(err)
	}
	err := st.BindCloudUser(303, "recovery@example.test", true)
	var ownership *WorkspaceOwnershipError
	if !errors.As(err, &ownership) || ownership.Code != "legacy_workspace_ambiguous" {
		t.Fatalf("ambiguous recovery bind error: %#v", err)
	}
	meta, metaErr := st.WorkspaceMeta()
	if metaErr != nil {
		t.Fatal(metaErr)
	}
	if meta.State != WorkspaceStateRecovery || meta.BoundCloudUserID != 0 || meta.QuarantinedItems != 1 {
		t.Fatalf("ambiguous recovery data was claimed automatically: %+v", meta)
	}
}
