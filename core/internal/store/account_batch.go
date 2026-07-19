package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const MaxAccountBatchSize = 500

type BatchDeleteResult struct {
	Requested int     `json:"requested"`
	Deleted   []int64 `json:"deleted"`
	Missing   []int64 `json:"missing"`
	Failed    []int64 `json:"failed"`
}

type BatchProxyResult struct {
	Matched   int    `json:"matched"`
	Updated   int    `json:"updated"`
	Unchanged int    `json:"unchanged"`
	ProxyID   *int64 `json:"proxy_id"`
}

type ProxyBindingCount struct {
	ProxyID int64 `json:"proxy_id"`
	Count   int   `json:"count"`
}

type AccountProxySummary struct {
	Total          int                 `json:"total"`
	Bound          int                 `json:"bound"`
	Unbound        int                 `json:"unbound"`
	UniformProxyID *int64              `json:"uniform_proxy_id,omitempty"`
	Mixed          bool                `json:"mixed"`
	Bindings       []ProxyBindingCount `json:"bindings"`
}

func normalizeAccountIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > MaxAccountBatchSize {
		return nil, fmt.Errorf("account_ids must contain between 1 and %d entries", MaxAccountBatchSize)
	}
	unique := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, errors.New("account_ids must contain positive integers")
		}
		unique[id] = struct{}{}
	}
	result := make([]int64, 0, len(unique))
	for id := range unique {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

func (s *Store) DeleteAccounts(ids []int64) (result BatchDeleteResult, resultErr error) {
	normalized, err := normalizeAccountIDs(ids)
	if err != nil {
		return result, err
	}
	result = BatchDeleteResult{
		Requested: len(normalized),
		Deleted:   make([]int64, 0, len(normalized)),
		Missing:   make([]int64, 0),
		Failed:    make([]int64, 0),
	}
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback()
			result.Failed = append([]int64{}, normalized...)
			result.Deleted = []int64{}
		}
	}()
	rows, err := tx.Query(`SELECT id FROM accounts WHERE id IN (`+placeholders(len(normalized))+`) ORDER BY id`, int64Args(normalized)...)
	if err != nil {
		return result, err
	}
	existing := make(map[int64]struct{}, len(normalized))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return result, err
		}
		existing[id] = struct{}{}
		result.Deleted = append(result.Deleted, id)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	for _, id := range normalized {
		if _, ok := existing[id]; !ok {
			result.Missing = append(result.Missing, id)
		}
	}
	if len(result.Deleted) > 0 {
		deleteResult, err := tx.Exec(`DELETE FROM accounts WHERE id IN (`+placeholders(len(result.Deleted))+`)`, int64Args(result.Deleted)...)
		if err != nil {
			return result, err
		}
		if affected, err := deleteResult.RowsAffected(); err != nil || affected != int64(len(result.Deleted)) {
			if err != nil {
				return result, err
			}
			return result, errors.New("account batch deletion affected an unexpected number of rows")
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) SetAllAccountsProxy(proxyID *int64) (result BatchProxyResult, resultErr error) {
	if proxyID != nil && *proxyID <= 0 {
		return result, errors.New("proxy_id must be a positive integer or null")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback()
		}
	}()
	if proxyID != nil {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM proxies WHERE id=?`, *proxyID).Scan(&exists); err != nil {
			return result, err
		}
		if exists == 0 {
			return result, ErrNotFound
		}
	}
	if err := tx.QueryRow(`SELECT
		(SELECT COUNT(*) FROM accounts) +
		(SELECT COUNT(*) FROM cloud_received_account_links l
		 JOIN cloud_received_keys k ON k.user_id=l.user_id AND k.grant_public_id=l.grant_public_id
		 JOIN cloud_session cs ON cs.user_id=l.user_id
		 WHERE l.remote_status IN ('active','paused'))`).Scan(&result.Matched); err != nil {
		return result, err
	}
	now := time.Now().Unix()
	updateResult, err := tx.Exec(`UPDATE accounts SET proxy_id=?,updated_at=? WHERE proxy_id IS NOT ?`, proxyID, now, proxyID)
	if err != nil {
		return result, err
	}
	affected, err := updateResult.RowsAffected()
	if err != nil {
		return result, err
	}
	result.Updated = int(affected)
	cloudUpdate, err := tx.Exec(`UPDATE cloud_received_account_links SET proxy_id=?,updated_at=?
		WHERE remote_status IN ('active','paused') AND proxy_id IS NOT ?
		AND EXISTS (SELECT 1 FROM cloud_received_keys k WHERE k.user_id=cloud_received_account_links.user_id
			AND k.grant_public_id=cloud_received_account_links.grant_public_id)
		AND EXISTS (SELECT 1 FROM cloud_session cs WHERE cs.user_id=cloud_received_account_links.user_id)`, proxyID, now, proxyID)
	if err != nil {
		return result, err
	}
	cloudAffected, err := cloudUpdate.RowsAffected()
	if err != nil {
		return result, err
	}
	result.Updated += int(cloudAffected)
	result.Unchanged = result.Matched - result.Updated
	result.ProxyID = proxyID
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) AccountProxySummary() (AccountProxySummary, error) {
	result := AccountProxySummary{Bindings: make([]ProxyBindingCount, 0)}
	rows, err := s.db.Query(`SELECT proxy_id,COUNT(*) FROM (
		SELECT proxy_id FROM accounts
		UNION ALL
		SELECT l.proxy_id FROM cloud_received_account_links l
		JOIN cloud_received_keys k ON k.user_id=l.user_id AND k.grant_public_id=l.grant_public_id
		JOIN cloud_session cs ON cs.user_id=l.user_id
		WHERE l.remote_status IN ('active','paused')
	) GROUP BY proxy_id ORDER BY proxy_id`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var distinct int
	for rows.Next() {
		var proxyID sql.NullInt64
		var count int
		if err := rows.Scan(&proxyID, &count); err != nil {
			return result, err
		}
		result.Total += count
		distinct++
		if proxyID.Valid {
			result.Bound += count
			result.Bindings = append(result.Bindings, ProxyBindingCount{ProxyID: proxyID.Int64, Count: count})
			value := proxyID.Int64
			result.UniformProxyID = &value
		} else {
			result.Unbound += count
			result.UniformProxyID = nil
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Mixed = distinct > 1
	if result.Mixed || result.Unbound > 0 {
		result.UniformProxyID = nil
	}
	return result, nil
}
