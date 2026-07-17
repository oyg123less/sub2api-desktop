package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	appcrypto "sub2api-desktop/core/internal/crypto"
)

func TestAccountFailureBackoffAndSuccessRecovery(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	account, err := s.CreateAccount(&Account{AccessToken: "token", Status: AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAccountFailure(account.ID, "refresh failed"); err != nil {
		t.Fatal(err)
	}
	failed, _ := s.GetAccount(account.ID)
	if failed.Status != AccountRefreshFailed || failed.ConsecutiveFailures != 1 || failed.NextRetryAt == nil {
		t.Fatalf("unexpected failed state: %+v", failed)
	}
	if failed.NextRetryAt.Before(time.Now().Add(50 * time.Second)) {
		t.Fatalf("retry backoff too short: %v", failed.NextRetryAt)
	}
	if err := s.RecordAccountSuccess(account.ID); err != nil {
		t.Fatal(err)
	}
	recovered, _ := s.GetAccount(account.ID)
	if recovered.Status != AccountActive || recovered.ConsecutiveFailures != 0 || recovered.NextRetryAt != nil || recovered.LastSuccessAt == nil {
		t.Fatalf("unexpected recovered state: %+v", recovered)
	}
}

func TestConcurrentAccountFailuresAutoDisableAtomically(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	account, err := s.CreateAccount(&Account{AccessToken: "token", Status: AccountActive})
	if err != nil {
		t.Fatal(err)
	}

	const failures = 32
	start := make(chan struct{})
	errors := make(chan error, failures)
	var wait sync.WaitGroup
	for index := 0; index < failures; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errors <- s.RecordAccountFailure(account.ID, "concurrent failure")
		}()
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	updated, err := s.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ConsecutiveFailures != 3 {
		t.Fatalf("consecutive failures = %d, want auto-disable threshold 3", updated.ConsecutiveFailures)
	}
	if updated.Status != AccountDisabled || updated.NextRetryAt != nil || updated.StatusReason != "auto_disabled_auth_failures" {
		t.Fatalf("concurrent auth failures did not auto-disable account: %+v", updated)
	}
}

func TestThirdAccountFailureAutoDisablesAndActiveRecovers(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	account, err := s.CreateAccount(&Account{AccessToken: "token", Status: AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if err := s.RecordAccountFailure(account.ID, "authentication failed"); err != nil {
			t.Fatal(err)
		}
		updated, _ := s.GetAccount(account.ID)
		if attempt < 3 && updated.Status != AccountRefreshFailed {
			t.Fatalf("attempt %d status=%s, want refresh_failed", attempt, updated.Status)
		}
		if attempt == 3 && updated.Status != AccountDisabled {
			t.Fatalf("third failure status=%s, want disabled", updated.Status)
		}
	}
	if err := s.SetAccountStatus(account.ID, AccountActive, ""); err != nil {
		t.Fatal(err)
	}
	recovered, _ := s.GetAccount(account.ID)
	if recovered.Status != AccountActive || recovered.ConsecutiveFailures != 0 || recovered.NextRetryAt != nil {
		t.Fatalf("manual recovery did not reset health: %+v", recovered)
	}
}

func TestAccountLimitsPersist(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	account, err := s.CreateAccount(&Account{AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if account.MaxConcurrency != DefaultAccountMaxConcurrency || account.QueueCapacity != DefaultAccountQueueCapacity {
		t.Fatalf("unexpected default limits: %+v", account)
	}
	if err := s.SetAccountLimits(account.ID, 7, 42); err != nil {
		t.Fatal(err)
	}
	updated, _ := s.GetAccount(account.ID)
	if updated.MaxConcurrency != 7 || updated.QueueCapacity != 42 {
		t.Fatalf("limits were not persisted: %+v", updated)
	}
	if err := s.SetAccountLimits(account.ID, 0, 42); err == nil {
		t.Fatal("invalid concurrency was accepted")
	}
}

func TestAccountSuccessPreservesUnexpiredRateLimit(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	account, err := s.CreateAccount(&Account{AccessToken: "token", Status: AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Hour)
	if err := s.SetRateLimited(account.ID, until, "transient_rate_limit"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAccountSuccess(account.ID); err != nil {
		t.Fatal(err)
	}

	updated, err := s.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != AccountRateLimited || updated.RateLimitedUntil == nil {
		t.Fatal("successful request cleared an unexpired rate limit")
	}
	if updated.RateLimitedUntil.Before(until.Add(-2 * time.Second)) {
		t.Fatal("successful request shortened the rate-limit window")
	}
	if updated.LastSuccessAt == nil {
		t.Fatal("successful request did not record last success")
	}
}

func TestAccountTestSuccessClearsTransientStateButPreservesManualDisable(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	rateLimited, err := s.CreateAccount(&Account{AccessToken: "limited-token", Status: AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRateLimited(rateLimited.ID, time.Now().Add(time.Hour), "transient_rate_limit"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAccountTestSuccess(rateLimited.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.GetAccount(rateLimited.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != AccountActive || recovered.StatusReason != "" || recovered.RateLimitedUntil != nil {
		t.Fatalf("manual test did not clear transient state: %+v", recovered)
	}

	disabled, err := s.CreateAccount(&Account{AccessToken: "disabled-token", Status: AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountStatus(disabled.ID, AccountDisabled, "manually_disabled"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAccountTestSuccess(disabled.ID); err != nil {
		t.Fatal(err)
	}
	preserved, err := s.GetAccount(disabled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Status != AccountDisabled || preserved.StatusReason != "manually_disabled" {
		t.Fatalf("manual test re-enabled a manually disabled account: %+v", preserved)
	}
}

func TestCleanupLogsUsesBatchesAndRowCap(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 2505; i++ {
		if err := s.InsertLog(&RequestLog{Model: "gpt-5.4", StatusCode: 200}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := s.CleanupLogs(0, 100)
	if err != nil || deleted != 2405 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	health, _ := s.LogHealth()
	if health.Rows != 100 {
		t.Fatalf("rows=%d", health.Rows)
	}
	if _, err := s.db.Exec(`UPDATE request_logs SET created_at=?`, time.Now().AddDate(0, 0, -8).Unix()); err != nil {
		t.Fatal(err)
	}
	deleted, err = s.CleanupLogs(7, 100000)
	if err != nil || deleted != 100 {
		t.Fatalf("retention deleted=%d err=%v", deleted, err)
	}
}
