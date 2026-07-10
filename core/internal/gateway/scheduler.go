package gateway

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/store"
)

const (
	StrategyFailover   = "failover"
	StrategyRoundRobin = "round_robin"
	StrategyQuotaAware = "quota_aware"
)

type Scheduler struct {
	mu       sync.Mutex
	round    uint64
	inFlight map[int64]int
}

func NewScheduler() *Scheduler {
	return &Scheduler{inFlight: make(map[int64]int)}
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
			s.mu.Unlock()
		})
	}
}

func (s *Scheduler) InFlight(accountID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inFlight[accountID]
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
