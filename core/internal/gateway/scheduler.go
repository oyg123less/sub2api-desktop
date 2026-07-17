package gateway

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/store"
)

var ErrAccountQueueFull = errors.New("all account waiting queues are full")

const (
	StrategyFailover   = "failover"
	StrategyRoundRobin = "round_robin"
	StrategyQuotaAware = "quota_aware"
)

type Scheduler struct {
	mu       sync.Mutex
	round    uint64
	inFlight map[int64]int
	waiting  map[int64]int
	changed  chan struct{}
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		inFlight: make(map[int64]int),
		waiting:  make(map[int64]int),
		changed:  make(chan struct{}),
	}
}

func (s *Scheduler) Order(accounts []*store.Account, strategy string) []*store.Account {
	result := append([]*store.Account(nil), accounts...)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.orderLocked(result, strategy)
}

// OrderAndAcquire atomically chooses and reserves the first candidate, closing
// the window where concurrent requests could all observe zero in-flight work.
func (s *Scheduler) OrderAndAcquire(accounts []*store.Account, strategy string) ([]*store.Account, func()) {
	result := append([]*store.Account(nil), accounts...)
	s.mu.Lock()
	result = s.orderLocked(result, strategy)
	if len(result) == 0 {
		s.mu.Unlock()
		return result, func() {}
	}
	accountID := result[0].ID
	s.inFlight[accountID]++
	s.mu.Unlock()
	return result, s.releaseFunc(accountID)
}

// OrderAndWait acquires a slot without exceeding an account's concurrency
// limit. When every account is saturated, the request occupies one bounded
// waiting-queue position until any account releases a slot.
func (s *Scheduler) OrderAndWait(ctx context.Context, accounts []*store.Account, strategy string) ([]*store.Account, func(), error) {
	var queueOwner int64
	for {
		result := append([]*store.Account(nil), accounts...)
		s.mu.Lock()
		result = s.orderLocked(result, strategy)
		selected := -1
		for index, account := range result {
			if account.MaxConcurrency <= 0 || s.inFlight[account.ID] < account.MaxConcurrency {
				selected = index
				break
			}
		}
		if selected >= 0 {
			if queueOwner != 0 {
				s.decrementWaitingLocked(queueOwner)
				queueOwner = 0
			}
			accountID := result[selected].ID
			s.inFlight[accountID]++
			if selected > 0 {
				selectedAccount := result[selected]
				copy(result[1:selected+1], result[0:selected])
				result[0] = selectedAccount
			}
			s.signalLocked()
			s.mu.Unlock()
			return result, s.releaseFunc(accountID), nil
		}

		if queueOwner == 0 {
			for _, account := range result {
				if account.QueueCapacity > 0 && s.waiting[account.ID] < account.QueueCapacity {
					queueOwner = account.ID
					s.waiting[account.ID]++
					break
				}
			}
			if queueOwner == 0 {
				s.mu.Unlock()
				return nil, func() {}, ErrAccountQueueFull
			}
		}
		changed := s.changed
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.decrementWaitingLocked(queueOwner)
			s.signalLocked()
			s.mu.Unlock()
			return nil, func() {}, ctx.Err()
		case <-changed:
		}
	}
}

func (s *Scheduler) orderLocked(result []*store.Account, strategy string) []*store.Account {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case StrategyFailover:
		return result
	case StrategyRoundRobin:
		if len(result) > 1 {
			start := int(s.round % uint64(len(result)))
			s.round++
			result = append(result[start:], result[:start]...)
		}
		return result
	default:
		sort.SliceStable(result, func(i, j int) bool {
			a, b := result[i], result[j]
			if x, y := accountStatusRank(a.Status), accountStatusRank(b.Status); x != y {
				return x < y
			}
			aUsage, aKnown := accountQuota(a)
			bUsage, bKnown := accountQuota(b)
			if aKnown != bKnown {
				return aKnown
			}
			if aKnown && aUsage != bUsage {
				return aUsage < bUsage
			}
			if x, y := s.inFlight[a.ID], s.inFlight[b.ID]; x != y {
				return x < y
			}
			if x, y := lastUsedUnix(a), lastUsedUnix(b); x != y {
				return x < y
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.Before(b.CreatedAt)
			}
			return a.ID < b.ID
		})
		return result
	}
}

func (s *Scheduler) Acquire(accountID int64) func() {
	s.mu.Lock()
	s.inFlight[accountID]++
	s.mu.Unlock()
	return s.releaseFunc(accountID)
}

func (s *Scheduler) TryAcquire(accountID int64, maxConcurrency int) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxConcurrency > 0 && s.inFlight[accountID] >= maxConcurrency {
		return func() {}, false
	}
	s.inFlight[accountID]++
	return s.releaseFunc(accountID), true
}

func (s *Scheduler) releaseFunc(accountID int64) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			if s.inFlight[accountID] <= 1 {
				delete(s.inFlight, accountID)
			} else {
				s.inFlight[accountID]--
			}
			s.signalLocked()
			s.mu.Unlock()
		})
	}
}

func (s *Scheduler) InFlight(accountID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inFlight[accountID]
}

func (s *Scheduler) Runtime(accountID int64) (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inFlight[accountID], s.waiting[accountID]
}

func (s *Scheduler) decrementWaitingLocked(accountID int64) {
	if s.waiting[accountID] <= 1 {
		delete(s.waiting, accountID)
	} else {
		s.waiting[accountID]--
	}
}

func (s *Scheduler) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func accountStatusRank(status store.AccountStatus) int {
	switch status {
	case store.AccountActive:
		return 0
	case store.AccountPending:
		return 1
	default:
		return 2
	}
}

func accountQuota(account *store.Account) (float64, bool) {
	if account.CodexUsage == nil {
		return 0, false
	}
	value := -math.MaxFloat64
	if account.CodexUsage.PrimaryUsedPercent != nil {
		value = math.Max(value, *account.CodexUsage.PrimaryUsedPercent)
	}
	if account.CodexUsage.SecondaryUsedPercent != nil {
		value = math.Max(value, *account.CodexUsage.SecondaryUsedPercent)
	}
	if value == -math.MaxFloat64 {
		return 0, false
	}
	return value, true
}

func lastUsedUnix(account *store.Account) int64 {
	if account.LastUsedAt == nil {
		return time.Time{}.Unix()
	}
	return account.LastUsedAt.Unix()
}
