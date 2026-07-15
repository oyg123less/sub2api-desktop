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

func TestConcurrentAccountFailuresIncrementAtomically(t *testing.T) {
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
	if updated.ConsecutiveFailures != failures {
		t.Fatalf("consecutive failures = %d, want %d", updated.ConsecutiveFailures, failures)
	}
	if updated.NextRetryAt == nil || updated.NextRetryAt.Before(time.Now().Add(29*time.Minute)) {
		t.Fatal("concurrent failure backoff was not capped at 30 minutes")
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
	if err := s.SetRateLimited(account.ID, until); err != nil {
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
