package store

import (
	"time"
)

// InsertLog records a request log entry.
func (s *Store) InsertLog(l *RequestLog) error {
	streamInt := 0
	if l.Stream {
		streamInt = 1
	}
	_, err := s.db.Exec(`INSERT INTO request_logs
		(account_id, account_email, model, status_code, prompt_tokens, completion_tokens, total_tokens, latency_ms, stream, error, created_at,
		 request_id, requested_model, resolved_model, error_kind, attempt_count, terminal_event)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.AccountID, l.AccountEmail, l.Model, l.StatusCode, l.PromptTokens, l.CompletionTokens,
		l.TotalTokens, l.LatencyMS, streamInt, l.Error, time.Now().Unix(), l.RequestID, l.RequestedModel,
		l.ResolvedModel, l.ErrorKind, max(l.AttemptCount, 1), l.TerminalEvent)
	return err
}

// RecentLogs returns the most recent request logs (up to limit).
func (s *Store) RecentLogs(limit int) ([]*RequestLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, account_id, account_email, model, status_code, prompt_tokens, completion_tokens, total_tokens, latency_ms, stream, error, created_at,
		request_id, requested_model, resolved_model, error_kind, attempt_count, terminal_event
		FROM request_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RequestLog
	for rows.Next() {
		l, err := scanLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LogsForExport returns logs in chronological order for deterministic JSON and
// CSV exports. A zero since value exports the full retained range.
func (s *Store) LogsForExport(since time.Time) ([]*RequestLog, error) {
	rows, err := s.db.Query(`SELECT id, account_id, account_email, model, status_code, prompt_tokens, completion_tokens, total_tokens, latency_ms, stream, error, created_at,
		request_id, requested_model, resolved_model, error_kind, attempt_count, terminal_event
		FROM request_logs WHERE created_at >= ? ORDER BY id ASC`, timeToUnix(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RequestLog
	for rows.Next() {
		entry, err := scanLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *Store) ClearLogs() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM request_logs`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func scanLog(rows interface {
	Scan(dest ...any) error
}) (*RequestLog, error) {
	var (
		l         RequestLog
		accID     *int64
		streamInt int
		createdAt int64
	)
	if err := rows.Scan(&l.ID, &accID, &l.AccountEmail, &l.Model, &l.StatusCode,
		&l.PromptTokens, &l.CompletionTokens, &l.TotalTokens, &l.LatencyMS, &streamInt,
		&l.Error, &createdAt, &l.RequestID, &l.RequestedModel, &l.ResolvedModel, &l.ErrorKind,
		&l.AttemptCount, &l.TerminalEvent); err != nil {
		return nil, err
	}
	l.AccountID = accID
	l.Stream = streamInt == 1
	l.CreatedAt = unixToTime(createdAt)
	return &l, nil
}

// StatsSummary is an aggregate view over request logs.
type StatsSummary struct {
	TotalRequests   int64 `json:"total_requests"`
	SuccessRequests int64 `json:"success_requests"`
	FailedRequests  int64 `json:"failed_requests"`
	TotalTokens     int64 `json:"total_tokens"`
	PromptTokens    int64 `json:"prompt_tokens"`
	CompletionTok   int64 `json:"completion_tokens"`
	AvgLatencyMS    int64 `json:"avg_latency_ms"`
}

// Summary computes an aggregate over logs newer than `since` (zero = all time).
func (s *Store) Summary(since time.Time) (*StatsSummary, error) {
	var sum StatsSummary
	row := s.db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status_code < 200 OR status_code >= 300 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(prompt_tokens),0),
		COALESCE(SUM(completion_tokens),0),
		COALESCE(CAST(AVG(latency_ms) AS INTEGER),0)
		FROM request_logs WHERE created_at >= ?`, timeToUnix(since))
	if err := row.Scan(&sum.TotalRequests, &sum.SuccessRequests, &sum.FailedRequests,
		&sum.TotalTokens, &sum.PromptTokens, &sum.CompletionTok, &sum.AvgLatencyMS); err != nil {
		return nil, err
	}
	return &sum, nil
}

// DailyPoint is a per-day aggregate.
type DailyPoint struct {
	Date        string `json:"date"`
	Requests    int64  `json:"requests"`
	TotalTokens int64  `json:"total_tokens"`
}

// Daily returns per-day aggregates for the last n days.
func (s *Store) Daily(days int) ([]DailyPoint, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	rows, err := s.db.Query(`SELECT date(created_at, 'unixepoch', 'localtime') AS d,
		COUNT(*), COALESCE(SUM(total_tokens),0)
		FROM request_logs WHERE created_at >= ?
		GROUP BY d ORDER BY d ASC`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyPoint
	for rows.Next() {
		var p DailyPoint
		if err := rows.Scan(&p.Date, &p.Requests, &p.TotalTokens); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AccountModelUsage aggregates token usage for one (account, model) pair.
type AccountModelUsage struct {
	AccountID        int64  `json:"account_id"`
	Model            string `json:"model"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// UsageByAccountModel returns per-account, per-model token aggregates over all
// logs that carry an account_id. Used to compute per-account usage and cost.
func (s *Store) UsageByAccountModel() ([]AccountModelUsage, error) {
	rows, err := s.db.Query(`SELECT account_id, model,
		COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0), COALESCE(SUM(total_tokens),0)
		FROM request_logs WHERE account_id IS NOT NULL
		GROUP BY account_id, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountModelUsage
	for rows.Next() {
		var u AccountModelUsage
		if err := rows.Scan(&u.AccountID, &u.Model, &u.Requests, &u.PromptTokens, &u.CompletionTokens, &u.TotalTokens); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ModelPoint aggregates usage per model.
type ModelPoint struct {
	Model       string `json:"model"`
	Requests    int64  `json:"requests"`
	TotalTokens int64  `json:"total_tokens"`
}

// ByModel returns per-model aggregates.
func (s *Store) ByModel(since time.Time) ([]ModelPoint, error) {
	rows, err := s.db.Query(`SELECT model, COUNT(*), COALESCE(SUM(total_tokens),0)
		FROM request_logs WHERE created_at >= ?
		GROUP BY model ORDER BY COUNT(*) DESC`, timeToUnix(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelPoint
	for rows.Next() {
		var p ModelPoint
		if err := rows.Scan(&p.Model, &p.Requests, &p.TotalTokens); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
