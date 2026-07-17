package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sub2api-desktop/core/internal/store"
)

func TestSchedulerRoundRobin(t *testing.T) {
	scheduler := NewScheduler()
	accounts := []*store.Account{{ID: 1}, {ID: 2}, {ID: 3}}
	wants := [][]int64{{1, 2, 3}, {2, 3, 1}, {3, 1, 2}, {1, 2, 3}}
	for i, want := range wants {
		got := scheduler.Order(accounts, StrategyRoundRobin)
		for j := range want {
			if got[j].ID != want[j] {
				t.Fatalf("round %d index %d = %d, want %d", i, j, got[j].ID, want[j])
			}
		}
	}
}

func TestSchedulerWaitQueueWakesOnRelease(t *testing.T) {
	scheduler := NewScheduler()
	accounts := []*store.Account{{ID: 1, Status: store.AccountActive, MaxConcurrency: 1, QueueCapacity: 2}}
	_, firstRelease, err := scheduler.OrderAndWait(context.Background(), accounts, StrategyQuotaAware)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		accounts []*store.Account
		release  func()
		err      error
	}
	done := make(chan result, 1)
	go func() {
		ordered, release, err := scheduler.OrderAndWait(context.Background(), accounts, StrategyQuotaAware)
		done <- result{accounts: ordered, release: release, err: err}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		_, waiting := scheduler.Runtime(1)
		if waiting == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request did not enter wait queue")
		}
		time.Sleep(time.Millisecond)
	}
	firstRelease()
	select {
	case acquired := <-done:
		if acquired.err != nil || len(acquired.accounts) != 1 || acquired.accounts[0].ID != 1 {
			t.Fatalf("unexpected queued acquisition: %+v", acquired)
		}
		acquired.release()
	case <-time.After(time.Second):
		t.Fatal("queued request did not wake")
	}
	if inFlight, waiting := scheduler.Runtime(1); inFlight != 0 || waiting != 0 {
		t.Fatalf("runtime leaked: in_flight=%d waiting=%d", inFlight, waiting)
	}
}

func TestSchedulerQueueFullAndCancellationCleanup(t *testing.T) {
	scheduler := NewScheduler()
	accounts := []*store.Account{{ID: 1, Status: store.AccountActive, MaxConcurrency: 1, QueueCapacity: 1}}
	_, release, err := scheduler.OrderAndWait(context.Background(), accounts, StrategyQuotaAware)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waitDone := make(chan error, 1)
	go func() {
		_, _, err := scheduler.OrderAndWait(ctx, accounts, StrategyQuotaAware)
		waitDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		_, waiting := scheduler.Runtime(1)
		if waiting == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request did not enter wait queue")
		}
		time.Sleep(time.Millisecond)
	}
	if _, _, err := scheduler.OrderAndWait(context.Background(), accounts, StrategyQuotaAware); !errors.Is(err, ErrAccountQueueFull) {
		t.Fatalf("queue-full error=%v", err)
	}
	cancel()
	if err := <-waitDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	if _, waiting := scheduler.Runtime(1); waiting != 0 {
		t.Fatalf("waiting count leaked after cancel: %d", waiting)
	}
	release()
}

func TestSevereAccountFailureClassification(t *testing.T) {
	if !isSevereAccountFailure(`Personal access token owner is inactive.`) {
		t.Fatal("inactive token owner was not classified as severe")
	}
	if isSevereAccountFailure("temporary proxy connection failure") {
		t.Fatal("network failure was classified as severe")
	}
}

func TestSchedulerConcurrentInFlightAccounting(t *testing.T) {
	scheduler := NewScheduler()
	const workers = 64
	releaseAll := make(chan struct{})
	var acquired sync.WaitGroup
	var finished sync.WaitGroup
	acquired.Add(workers)
	finished.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer finished.Done()
			release := scheduler.Acquire(7)
			acquired.Done()
			<-releaseAll
			release()
		}()
	}
	acquired.Wait()
	if got := scheduler.InFlight(7); got != workers {
		t.Fatalf("in-flight = %d, want %d", got, workers)
	}
	close(releaseAll)
	finished.Wait()
	if got := scheduler.InFlight(7); got != 0 {
		t.Fatalf("in-flight after release = %d, want 0", got)
	}
}

func TestSchedulerQuotaAwareOrdering(t *testing.T) {
	now := time.Now()
	low, high := 15.0, 90.0
	old, recent := now.Add(-time.Hour), now.Add(-time.Minute)
	accounts := []*store.Account{
		{ID: 1, Status: store.AccountActive, CodexUsage: &store.CodexUsage{PrimaryUsedPercent: &high}, LastUsedAt: &old, CreatedAt: now},
		{ID: 2, Status: store.AccountActive, CodexUsage: &store.CodexUsage{SecondaryUsedPercent: &low}, LastUsedAt: &recent, CreatedAt: now},
		{ID: 3, Status: store.AccountPending, CodexUsage: &store.CodexUsage{PrimaryUsedPercent: &low}, CreatedAt: now},
		{ID: 4, Status: store.AccountActive, LastUsedAt: &old, CreatedAt: now},
	}
	scheduler := NewScheduler()
	got := scheduler.Order(accounts, StrategyQuotaAware)
	want := []int64{2, 1, 4, 3}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("index %d = %d, want %d", i, got[i].ID, want[i])
		}
	}
}

func TestSchedulerInFlightBreaksUnknownQuotaTie(t *testing.T) {
	scheduler := NewScheduler()
	accounts := []*store.Account{
		{ID: 1, Status: store.AccountActive},
		{ID: 2, Status: store.AccountActive},
	}
	release := scheduler.Acquire(1)
	if got := scheduler.Order(accounts, StrategyQuotaAware); got[0].ID != 2 {
		t.Fatalf("account %d selected while account 1 was in flight", got[0].ID)
	}
	release()
	release()
	if scheduler.InFlight(1) != 0 {
		t.Fatal("release must be idempotent")
	}
}

func TestSchedulerOrderAndAcquireDistributesConcurrentSelections(t *testing.T) {
	scheduler := NewScheduler()
	accounts := []*store.Account{
		{ID: 1, Status: store.AccountActive},
		{ID: 2, Status: store.AccountActive},
		{ID: 3, Status: store.AccountActive},
	}
	var releases []func()
	var selected []int64
	for i := 0; i < 3; i++ {
		ordered, release := scheduler.OrderAndAcquire(accounts, StrategyQuotaAware)
		selected = append(selected, ordered[0].ID)
		releases = append(releases, release)
	}
	for _, release := range releases {
		release()
	}
	if selected[0] != 1 || selected[1] != 2 || selected[2] != 3 {
		t.Fatalf("selected=%v", selected)
	}
}
