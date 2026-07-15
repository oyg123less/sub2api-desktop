package gateway

import (
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
