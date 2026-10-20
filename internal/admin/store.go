package admin

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Store holds the console's read models. It talks to SQLite directly: every
// query here joins several aggregates at once, which is exactly the shape the
// transactional Store port was not built for.
type Store struct {
	db *sql.DB
}

// NewStore wraps an open database handle.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

// where accumulates AND-ed conditions and their arguments in lockstep.
type where struct {
	conds []string
	args  []any
}

func (w *where) add(cond string, args ...any) {
	w.conds = append(w.conds, cond)
	w.args = append(w.args, args...)
}

// like builds a case-insensitive contains match, escaping the wildcards so a
// literal '%' typed by an operator does not turn into a full table scan.
func (w *where) like(columns []string, term string) {
	term = strings.TrimSpace(term)
	if term == "" {
		return
	}
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(term)
	pattern := "%" + escaped + "%"
	parts := make([]string, 0, len(columns))
	for _, c := range columns {
		parts = append(parts, c+" LIKE ? ESCAPE '\\'")
	}
	w.add("("+strings.Join(parts, " OR ")+")", repeat(pattern, len(columns))...)
}

func (w *where) clause() string {
	if len(w.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.conds, " AND ")
}

func repeat(v string, n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = v
	}
	return out
}

type rowScanner interface {
	Scan(dest ...any) error
}

// listPage runs a count and a page query sharing the same filters.
func listPage[T any](ctx context.Context, db *sql.DB, countQ, listQ string, args []any, p Page,
	scan func(rowScanner) (T, error)) (List[T], error) {
	p = p.Normalise()

	var total int
	if err := db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return List[T]{}, fmt.Errorf("count rows: %w", err)
	}
	if total == 0 {
		return NewList([]T{}, 0, p), nil
	}

	pageArgs := make([]any, 0, len(args)+2)
	pageArgs = append(pageArgs, args...)
	pageArgs = append(pageArgs, p.PageSize, p.Offset())

	rows, err := db.QueryContext(ctx, listQ+" LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return List[T]{}, fmt.Errorf("query rows: %w", err)
	}
	defer rows.Close()

	items := make([]T, 0, p.PageSize)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return List[T]{}, fmt.Errorf("scan row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[T]{}, fmt.Errorf("iterate rows: %w", err)
	}
	return NewList(items, total, p), nil
}

func nullStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func nullInt(v sql.NullInt64) *int64 {
	if v.Valid {
		n := v.Int64
		return &n
	}
	return nil
}

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------

// Overview builds the dashboard in a fixed number of queries, independent of how
// many rows the dataset has.
func (s *Store) Overview(ctx context.Context, health Health) (Overview, error) {
	now := time.Now().UTC()
	var out Overview

	var (
		convTotal, conv7d         int
		msgTotal, msg7d           int
		transferConv              int
		openTickets, urgent       int
		pendingActions, stale     int
		refundPending, invPending int
		toolCalls, toolFailed     int
		casesOpen                 int
	)
	const q = `
SELECT
  (SELECT COUNT(*) FROM conversations),
  (SELECT COUNT(*) FROM conversations WHERE created_at >= ?),
  (SELECT COUNT(*) FROM messages),
  (SELECT COUNT(*) FROM messages WHERE created_at >= ?),
  (SELECT COUNT(DISTINCT t.conversation_id) FROM service_tickets t
     WHERE t.ticket_type = 'human_transfer' AND t.conversation_id IS NOT NULL),
  (SELECT COUNT(*) FROM service_tickets WHERE status IN ('open','assigned')),
  (SELECT COUNT(*) FROM service_tickets WHERE status IN ('open','assigned') AND priority IN ('high','urgent')),
  (SELECT COUNT(*) FROM pending_actions WHERE status = 'pending'),
  (SELECT COUNT(*) FROM pending_actions WHERE status = 'pending' AND expires_at < ?),
  (SELECT COUNT(*) FROM refunds WHERE refund_status IN ('requested','pending_review','approved','processing')),
  (SELECT COUNT(*) FROM invoices WHERE invoice_status IN ('requested','processing')),
  (SELECT COUNT(*) FROM tool_executions),
  (SELECT COUNT(*) FROM tool_executions WHERE status IN ('failed','rejected')),
  (SELECT COUNT(*) FROM service_cases WHERE case_status NOT IN ('completed','cancelled','rejected'))`
	weekAgo := now.AddDate(0, 0, -7).Format(time.RFC3339)
	if err := s.db.QueryRowContext(ctx, q, weekAgo, weekAgo, now.Format(time.RFC3339)).
		Scan(&convTotal, &conv7d, &msgTotal, &msg7d, &transferConv, &openTickets, &urgent,
			&pendingActions, &stale, &refundPending, &invPending, &toolCalls, &toolFailed, &casesOpen); err != nil {
		return Overview{}, fmt.Errorf("overview counters: %w", err)
	}

	selfService := 0.0
	if convTotal > 0 {
		selfService = float64(convTotal-transferConv) / float64(convTotal) * 100
	}
	toolOK := 100.0
	if toolCalls > 0 {
		toolOK = float64(toolCalls-toolFailed) / float64(toolCalls) * 100
	}

	out.Metrics = []Metric{
		{Key: "conversations", Label: "会话总量", Value: itoa(convTotal), Hint: "近 7 天新增 " + itoa(conv7d),
			Tone: "neutral", Link: "#/conversations"},
		{Key: "self_service", Label: "自助解决率", Value: fmt.Sprintf("%.1f%%", selfService),
			Hint: itoa(convTotal-transferConv) + " / " + itoa(convTotal) + " 未转人工", Tone: toneFor(selfService, 70, 50),
			Link: "#/analytics"},
		{Key: "messages", Label: "消息总量", Value: itoa(msgTotal), Hint: "近 7 天 " + itoa(msg7d),
			Tone: "neutral", Link: "#/conversations"},
		{Key: "tool_success", Label: "工具成功率", Value: fmt.Sprintf("%.1f%%", toolOK),
			Hint: itoa(toolCalls-toolFailed) + " / " + itoa(toolCalls) + " 次调用", Tone: toneFor(toolOK, 97, 90),
			Link: "#/tools"},
		{Key: "tickets", Label: "待处理工单", Value: itoa(openTickets),
			Hint: "高优 " + itoa(urgent) + " 张", Tone: toneForNegative(openTickets),
			Link: "#/tickets"},
		{Key: "pending", Label: "待确认动作", Value: itoa(pendingActions),
			Hint: "已过期未清理 " + itoa(stale), Tone: toneForNegative(stale),
			Link: "#/pending-actions"},
	}

	out.Queues = []QueueRow{
		{Key: "tickets", Label: "工单待处理", Count: openTickets, Overdue: urgent,
			Link: "#/tickets?status=open", Severity: severity(openTickets)},
		{Key: "pending_actions", Label: "待客户确认", Count: pendingActions, Overdue: stale,
			Link: "#/pending-actions?status=pending", Severity: severity(stale)},
		{Key: "refunds", Label: "退款待处理", Count: refundPending,
			Link: "#/finance?tab=refunds", Severity: severity(refundPending)},
		{Key: "service_cases", Label: "售后进行中", Count: casesOpen,
			Link: "#/aftersales", Severity: severity(casesOpen)},
		{Key: "invoices", Label: "发票待开", Count: invPending,
			Link: "#/finance?tab=invoices", Severity: severity(invPending)},
	}

	activity, err := s.activity(ctx, 14)
	if err != nil {
		return Overview{}, err
	}
	out.Activity = activity

	intents, err := s.IntentStats(ctx, 8)
	if err != nil {
		return Overview{}, err
	}
	out.Intents = intents

	tools, err := s.ToolStats(ctx)
	if err != nil {
		return Overview{}, err
	}
	if len(tools) > 5 {
		tools = tools[:5]
	}
	out.TopTools = tools

	failures, err := s.RecentFailures(ctx, 6)
	if err != nil {
		return Overview{}, err
	}
	out.RecentFailed = failures

	out.Health = health
	out.GeneratedAt = now.Format(time.RFC3339)
	return out, nil
}

// activity returns one point per day for the last n days, including zero days so
// the sparkline has no gaps.
func (s *Store) activity(ctx context.Context, days int) ([]DayPoint, error) {
	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)
	rows, err := s.db.QueryContext(ctx, `SELECT substr(created_at, 1, 10) AS d, COUNT(*)
		FROM conversations WHERE created_at >= ? GROUP BY d`, since.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("activity: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		counts[d] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]DayPoint, 0, days)
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, DayPoint{Date: d, Count: counts[d]})
	}
	return out, nil
}

// RecentFailures lists the latest failed or rejected tool calls.
func (s *Store) RecentFailures(ctx context.Context, limit int) ([]ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, conversation_id, tool_call_id, tool_name, status,
		COALESCE(input_json, ''), COALESCE(output_json, ''), COALESCE(error_code, ''),
		COALESCE(error_message_safe, ''), latency_ms, created_at, COALESCE(finished_at, '')
		FROM tool_executions WHERE status IN ('failed','rejected')
		ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent failures: %w", err)
	}
	defer rows.Close()

	out := []ToolCall{}
	for rows.Next() {
		var t ToolCall
		var latency sql.NullInt64
		if err := rows.Scan(&t.ID, &t.ConversationID, &t.ToolCallID, &t.ToolName, &t.Status,
			&t.Input, &t.Output, &t.ErrorCode, &t.ErrorMessage, &latency, &t.CreatedAt, &t.FinishedAt); err != nil {
			return nil, err
		}
		t.LatencyMS = nullInt(latency)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ToolStats aggregates every tool. Latencies are collected in memory because a
// percentiles-on-SQLite query costs more to read than the whole column does at
// this dataset size; the row cap keeps that trade-off bounded.
func (s *Store) ToolStats(ctx context.Context) ([]ToolStat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tool_name, status, COALESCE(error_code, ''), latency_ms
		FROM tool_executions ORDER BY created_at DESC LIMIT 50000`)
	if err != nil {
		return nil, fmt.Errorf("tool stats: %w", err)
	}
	defer rows.Close()

	type acc struct {
		stat      ToolStat
		latencies []int64
		errors    map[string]int
	}
	byTool := map[string]*acc{}
	order := []string{}

	for rows.Next() {
		var name, status, errCode string
		var latency sql.NullInt64
		if err := rows.Scan(&name, &status, &errCode, &latency); err != nil {
			return nil, err
		}
		a, ok := byTool[name]
		if !ok {
			a = &acc{stat: ToolStat{ToolName: name}, errors: map[string]int{}}
			byTool[name] = a
			order = append(order, name)
		}
		a.stat.Calls++
		switch status {
		case "succeeded":
			a.stat.Succeeded++
		case "failed":
			a.stat.Failed++
		case "rejected":
			a.stat.Rejected++
		}
		if errCode != "" {
			a.errors[errCode]++
		}
		if latency.Valid {
			a.latencies = append(a.latencies, latency.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ToolStat, 0, len(order))
	for _, name := range order {
		a := byTool[name]
		st := a.stat
		if st.Calls > 0 {
			st.SuccessRate = float64(st.Succeeded) / float64(st.Calls) * 100
		}
		if len(a.latencies) > 0 {
			sort.Slice(a.latencies, func(i, j int) bool { return a.latencies[i] < a.latencies[j] })
			var sum int64
			for _, v := range a.latencies {
				sum += v
			}
			st.AvgLatency = sum / int64(len(a.latencies))
			st.P95Latency = a.latencies[p95Index(len(a.latencies))]
		}
		var topErr string
		var topN int
		for code, n := range a.errors {
			if n > topN {
				topErr, topN = code, n
			}
		}
		st.TopError = topErr
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Calls > out[j].Calls })
	return out, nil
}

// p95Index is the nearest-rank position for the 95th percentile.
func p95Index(n int) int {
	if n <= 1 {
		return 0
	}
	idx := (95*n + 99) / 100
	if idx >= n {
		idx = n - 1
	}
	return idx
}

// IntentStats aggregates annotations by primary intent.
func (s *Store) IntentStats(ctx context.Context, limit int) ([]IntentRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		COALESCE(intent_primary, '未标注') AS intent,
		COUNT(*),
		SUM(CASE WHEN resolution_status = 'resolved' THEN 1 ELSE 0 END),
		SUM(CASE WHEN resolution_status = 'unresolved' THEN 1 ELSE 0 END),
		SUM(CASE WHEN resolution_status = 'transferred' THEN 1 ELSE 0 END),
		COALESCE(AVG(confidence), 0),
		SUM(CASE WHEN sentiment = 'positive' THEN 1 ELSE 0 END),
		SUM(CASE WHEN sentiment = 'negative' THEN 1 ELSE 0 END),
		SUM(CASE WHEN sentiment = 'neutral' THEN 1 ELSE 0 END)
		FROM message_annotations
		GROUP BY intent ORDER BY COUNT(*) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("intent stats: %w", err)
	}
	defer rows.Close()

	out := []IntentRow{}
	for rows.Next() {
		var r IntentRow
		if err := rows.Scan(&r.IntentPrimary, &r.Samples, &r.Resolved, &r.Unresolved,
			&r.Transferred, &r.AvgConfidence, &r.Positive, &r.Negative, &r.Neutral); err != nil {
			return nil, err
		}
		total := r.Resolved + r.Unresolved + r.Transferred
		if total > 0 {
			r.ResolveRate = float64(r.Resolved) / float64(total) * 100
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

// ConversationFilter narrows the conversation list.
type ConversationFilter struct {
	Query   string
	Status  string
	Channel string
	Page    Page
}

// ListConversations pages across every customer.
func (s *Store) ListConversations(ctx context.Context, f ConversationFilter) (List[ConversationRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("c.status = ?", f.Status)
	}
	if f.Channel != "" {
		w.add("c.channel = ?", f.Channel)
	}
	w.like([]string{"c.conversation_no", "cu.customer_no", "cu.display_name", "c.summary"}, f.Query)

	from := ` FROM conversations c JOIN customers cu ON cu.id = c.customer_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT c.id, c.conversation_no, c.customer_id, cu.customer_no, COALESCE(cu.display_name, ''),
		c.status, c.channel, COALESCE(c.summary, ''),
		(SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id),
		(SELECT COUNT(*) FROM tool_executions t WHERE t.conversation_id = c.id),
		(SELECT COUNT(*) FROM pending_actions p WHERE p.conversation_id = c.id AND p.status = 'pending'),
		c.created_at, COALESCE(c.last_message_at, '')` + from + w.clause() +
		` ORDER BY COALESCE(c.last_message_at, c.created_at) DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (ConversationRow, error) {
		var c ConversationRow
		err := r.Scan(&c.ID, &c.ConversationNo, &c.CustomerID, &c.CustomerNo, &c.CustomerName,
			&c.Status, &c.Channel, &c.Summary, &c.MessageCount, &c.ToolCount, &c.PendingCount,
			&c.CreatedAt, &c.LastMessageAt)
		return c, err
	})
}

// GetConversation loads a conversation row by id.
func (s *Store) GetConversation(ctx context.Context, id string) (ConversationRow, error) {
	var c ConversationRow
	err := s.db.QueryRowContext(ctx, `SELECT c.id, c.conversation_no, c.customer_id, cu.customer_no,
		COALESCE(cu.display_name, ''), c.status, c.channel, COALESCE(c.summary, ''),
		(SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id),
		(SELECT COUNT(*) FROM tool_executions t WHERE t.conversation_id = c.id),
		(SELECT COUNT(*) FROM pending_actions p WHERE p.conversation_id = c.id AND p.status = 'pending'),
		c.created_at, COALESCE(c.last_message_at, '')
		FROM conversations c JOIN customers cu ON cu.id = c.customer_id WHERE c.id = ?`, id).
		Scan(&c.ID, &c.ConversationNo, &c.CustomerID, &c.CustomerNo, &c.CustomerName,
			&c.Status, &c.Channel, &c.Summary, &c.MessageCount, &c.ToolCount, &c.PendingCount,
			&c.CreatedAt, &c.LastMessageAt)
	if err == sql.ErrNoRows {
		return ConversationRow{}, errNotFound("会话不存在")
	}
	if err != nil {
		return ConversationRow{}, err
	}
	return c, nil
}

// ListMessages returns the full transcript including tool turns, because the
// console exists to explain what the assistant actually did.
func (s *Store) ListMessages(ctx context.Context, conversationID string, limit int) ([]MessageRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id, m.role, COALESCE(m.content, ''),
		COALESCE(m.tool_name, ''), COALESCE(m.tool_arguments_json, ''), COALESCE(m.tool_result_json, ''),
		m.created_at,
		COALESCE(a.intent_primary, ''), COALESCE(a.intent_secondary, ''), COALESCE(a.sentiment, ''),
		COALESCE(a.resolution_status, ''), a.confidence
		FROM messages m LEFT JOIN message_annotations a ON a.message_id = m.id
		WHERE m.conversation_id = ?
		ORDER BY m.created_at, m.id LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	out := []MessageRow{}
	for rows.Next() {
		var m MessageRow
		var conf sql.NullFloat64
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.ToolName, &m.ToolArguments, &m.ToolResult,
			&m.CreatedAt, &m.IntentPrimary, &m.IntentSecondary, &m.Sentiment, &m.Resolution, &conf); err != nil {
			return nil, err
		}
		if conf.Valid {
			v := conf.Float64
			m.Confidence = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ConversationDetail assembles the detail drawer.
func (s *Store) ConversationDetail(ctx context.Context, id string) (ConversationDetail, error) {
	conv, err := s.GetConversation(ctx, id)
	if err != nil {
		return ConversationDetail{}, err
	}
	messages, err := s.ListMessages(ctx, id, 500)
	if err != nil {
		return ConversationDetail{}, err
	}
	tools, err := s.ToolCalls(ctx, ToolCallFilter{ConversationID: id, Page: Page{Page: 1, PageSize: 100}})
	if err != nil {
		return ConversationDetail{}, err
	}
	pending, err := s.PendingActions(ctx, PendingFilter{ConversationID: id, Page: Page{Page: 1, PageSize: 50}})
	if err != nil {
		return ConversationDetail{}, err
	}
	audit, err := s.AuditLogs(ctx, AuditFilter{ConversationID: id, Page: Page{Page: 1, PageSize: 50}})
	if err != nil {
		return ConversationDetail{}, err
	}
	orders, err := s.Orders(ctx, OrderFilter{CustomerID: conv.CustomerID, Page: Page{Page: 1, PageSize: 10}})
	if err != nil {
		return ConversationDetail{}, err
	}
	return ConversationDetail{
		Conversation: conv, Messages: messages, ToolCalls: tools.Items,
		Pending: pending.Items, Audit: audit.Items, Orders: orders.Items,
	}, nil
}

// ---------------------------------------------------------------------------
// Pending actions
// ---------------------------------------------------------------------------

// PendingFilter narrows the pending-action list.
type PendingFilter struct {
	Status         string
	ActionType     string
	ConversationID string
	Query          string
	Page           Page
}

// PendingActions lists proposals across all customers.
func (s *Store) PendingActions(ctx context.Context, f PendingFilter) (List[PendingAction], error) {
	w := &where{}
	if f.Status != "" {
		w.add("p.status = ?", f.Status)
	}
	if f.ActionType != "" {
		w.add("p.action_type = ?", f.ActionType)
	}
	if f.ConversationID != "" {
		w.add("p.conversation_id = ?", f.ConversationID)
	}
	w.like([]string{"p.action_no", "cu.customer_no", "cu.display_name", "p.target_id"}, f.Query)

	from := ` FROM pending_actions p
		JOIN conversations c ON c.id = p.conversation_id
		JOIN customers cu ON cu.id = p.customer_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT p.id, p.action_no, p.conversation_id, cu.customer_no, COALESCE(cu.display_name, ''),
		p.action_type, p.target_type, p.target_id, p.status, COALESCE(p.preview_json, ''),
		COALESCE(p.payload_json, '{}'), p.created_at, p.expires_at,
		COALESCE(p.confirmed_at, ''), COALESCE(p.executed_at, ''), COALESCE(p.result_json, '')` +
		from + w.clause() + ` ORDER BY p.created_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (PendingAction, error) {
		var a PendingAction
		err := r.Scan(&a.ID, &a.ActionNo, &a.ConversationID, &a.CustomerNo, &a.CustomerName,
			&a.ActionType, &a.TargetType, &a.TargetID, &a.Status, &a.Summary,
			&a.Payload, &a.CreatedAt, &a.ExpiresAt, &a.ConfirmedAt, &a.ExecutedAt, &a.Result)
		if err != nil {
			return a, err
		}
		a.IsOverdue = a.Status == "pending" && a.ExpiresAt < time.Now().UTC().Format(time.RFC3339)
		a.Summary = summarizePreview(a.Summary)
		return a, nil
	})
}

// summarizePreview turns the stored preview JSON into one readable line. The
// preview is written by the assistant and may hold either a summary string or a
// field map, so both shapes are handled.
func summarizePreview(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return ""
	}
	if s, ok := jsonString(raw); ok {
		return s
	}
	parts := []string{}
	for _, key := range []string{"order_no", "estimated_refund_display", "refund_amount_display", "summary", "message"} {
		if v, ok := jsonField(raw, key); ok && v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " · ")
}

// PendingActionByID loads one proposal for the detail drawer.
func (s *Store) PendingActionByID(ctx context.Context, id string) (PendingAction, error) {
	var a PendingAction
	err := s.db.QueryRowContext(ctx, `SELECT p.id, p.action_no, p.conversation_id, cu.customer_no,
		COALESCE(cu.display_name, ''), p.action_type, p.target_type, p.target_id, p.status,
		COALESCE(p.preview_json, ''), COALESCE(p.payload_json, '{}'), p.created_at, p.expires_at,
		COALESCE(p.confirmed_at, ''), COALESCE(p.executed_at, ''), COALESCE(p.result_json, '')
		FROM pending_actions p
		JOIN conversations c ON c.id = p.conversation_id
		JOIN customers cu ON cu.id = p.customer_id
		WHERE p.id = ? OR p.action_no = ?`, id, id).
		Scan(&a.ID, &a.ActionNo, &a.ConversationID, &a.CustomerNo, &a.CustomerName,
			&a.ActionType, &a.TargetType, &a.TargetID, &a.Status, &a.Summary,
			&a.Payload, &a.CreatedAt, &a.ExpiresAt, &a.ConfirmedAt, &a.ExecutedAt, &a.Result)
	if err == sql.ErrNoRows {
		return PendingAction{}, errNotFound("待确认动作不存在")
	}
	if err != nil {
		return PendingAction{}, fmt.Errorf("load pending action: %w", err)
	}
	a.IsOverdue = a.Status == "pending" && a.ExpiresAt < time.Now().UTC().Format(time.RFC3339)
	a.Summary = summarizePreview(a.Summary)
	return a, nil
}

// VoidPendingAction moves a pending proposal to expired. The console never
// confirms on the customer's behalf: confirmation is the customer's consent, and
// forging it would defeat the entire propose-then-confirm design.
func (s *Store) VoidPendingAction(ctx context.Context, id, adminID, adminName, reason string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status, customerID, conversationID string
	if err := tx.QueryRowContext(ctx, `SELECT status, customer_id, conversation_id FROM pending_actions WHERE id = ?`, id).
		Scan(&status, &customerID, &conversationID); err != nil {
		if err == sql.ErrNoRows {
			return "", errNotFound("待确认动作不存在")
		}
		return "", fmt.Errorf("load pending action: %w", err)
	}
	if status != "pending" {
		return status, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pending_actions SET status = 'expired' WHERE id = ? AND status = 'pending'`, id); err != nil {
		return "", fmt.Errorf("void pending action: %w", err)
	}
	if err := appendAuditTx(ctx, tx, auditEntry{
		CustomerID: customerID, ConversationID: conversationID,
		ActionType: "void_pending_action", TargetType: "pending_action", TargetID: id,
		ActorType: "admin", ActorID: adminID,
		Result: "succeeded", Reason: reason,
		Before: map[string]any{"status": status}, After: map[string]any{"status": "expired", "operator": adminName},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit void action: %w", err)
	}
	return "expired", nil
}

// ---------------------------------------------------------------------------
// Tickets
// ---------------------------------------------------------------------------

// TicketFilter narrows the ticket queue.
type TicketFilter struct {
	Query    string
	Status   string
	Type     string
	Priority string
	Page     Page
}

// ListTickets pages the queue ordered by priority then age.
func (s *Store) ListTickets(ctx context.Context, f TicketFilter) (List[TicketRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("t.status = ?", f.Status)
	}
	if f.Type != "" {
		w.add("t.ticket_type = ?", f.Type)
	}
	if f.Priority != "" {
		w.add("t.priority = ?", f.Priority)
	}
	w.like([]string{"t.ticket_no", "t.summary", "cu.customer_no", "cu.display_name", "COALESCE(o.order_no, '')"}, f.Query)

	from := ` FROM service_tickets t
		JOIN customers cu ON cu.id = t.customer_id
		LEFT JOIN orders o ON o.id = t.order_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT t.id, t.ticket_no, t.customer_id, cu.customer_no, COALESCE(cu.display_name, ''),
		COALESCE(t.conversation_id, ''), COALESCE(o.order_no, ''), t.ticket_type, t.status, t.priority,
		COALESCE(t.category, ''), t.summary, COALESCE(t.assigned_to, ''), t.created_at, t.updated_at` +
		from + w.clause() + `
		ORDER BY CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
		         t.created_at ASC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (TicketRow, error) {
		var t TicketRow
		err := r.Scan(&t.ID, &t.TicketNo, &t.CustomerID, &t.CustomerNo, &t.CustomerName,
			&t.ConversationID, &t.OrderNo, &t.TicketType, &t.Status, &t.Priority,
			&t.Category, &t.Summary, &t.AssignedTo, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return t, err
		}
		t.AgeHours = ageHours(t.CreatedAt)
		return t, nil
	})
}

// TicketDetail loads a ticket with its note trail and audit entries.
func (s *Store) TicketDetail(ctx context.Context, ticketNo string) (TicketDetail, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.ticket_no, t.customer_id, cu.customer_no,
		COALESCE(cu.display_name, ''), COALESCE(t.conversation_id, ''), COALESCE(o.order_no, ''),
		t.ticket_type, t.status, t.priority, COALESCE(t.category, ''), t.summary,
		COALESCE(t.assigned_to, ''), t.created_at, t.updated_at
		FROM service_tickets t
		JOIN customers cu ON cu.id = t.customer_id
		LEFT JOIN orders o ON o.id = t.order_id
		WHERE t.ticket_no = ?`, ticketNo)
	if err != nil {
		return TicketDetail{}, fmt.Errorf("load ticket: %w", err)
	}
	var t TicketRow
	if rows.Next() {
		if err := rows.Scan(&t.ID, &t.TicketNo, &t.CustomerID, &t.CustomerNo, &t.CustomerName,
			&t.ConversationID, &t.OrderNo, &t.TicketType, &t.Status, &t.Priority,
			&t.Category, &t.Summary, &t.AssignedTo, &t.CreatedAt, &t.UpdatedAt); err != nil {
			rows.Close()
			return TicketDetail{}, err
		}
	} else {
		rows.Close()
		return TicketDetail{}, errNotFound("工单不存在")
	}
	rows.Close()
	t.AgeHours = ageHours(t.CreatedAt)

	noteRows, err := s.db.QueryContext(ctx, `SELECT id, author_name, note,
		COALESCE(from_status, ''), COALESCE(to_status, ''), created_at
		FROM service_ticket_notes WHERE ticket_id = ? ORDER BY created_at`, t.ID)
	if err != nil {
		return TicketDetail{}, fmt.Errorf("load notes: %w", err)
	}
	notes := []TicketNote{}
	for noteRows.Next() {
		var n TicketNote
		if err := noteRows.Scan(&n.ID, &n.AuthorName, &n.Note, &n.FromStatus, &n.ToStatus, &n.CreatedAt); err != nil {
			noteRows.Close()
			return TicketDetail{}, err
		}
		notes = append(notes, n)
	}
	noteRows.Close()

	audit, err := s.AuditLogs(ctx, AuditFilter{TargetType: "service_ticket", TargetID: t.ID, Page: Page{Page: 1, PageSize: 50}})
	if err != nil {
		return TicketDetail{}, err
	}
	return TicketDetail{Ticket: t, Notes: notes, Audit: audit.Items}, nil
}

// UpdateTicket is the console's only lifecycle write on a business row. Status
// moves are validated against an explicit transition table and always leave a
// note plus an audit record.
func (s *Store) UpdateTicket(ctx context.Context, ticketNo string, in TicketUpdate, adminID, adminName string) (TicketRow, error) {
	if in.Status == "" && in.Priority == "" && in.AssignedTo == "" && in.Note == "" {
		return TicketRow{}, errValidation("没有需要更新的内容")
	}
	if in.Status != "" && !validTicketStatus(in.Status) {
		return TicketRow{}, errValidation("工单状态不合法")
	}
	if in.Priority != "" && !validPriority(in.Priority) {
		return TicketRow{}, errValidation("优先级不合法")
	}
	if len([]rune(in.Note)) > 2000 {
		return TicketRow{}, errValidation("处理记录不超过 2000 个字符")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TicketRow{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		id, status, priority, assigned, customerID string
		conversationID                             sql.NullString
	)
	err = tx.QueryRowContext(ctx, `SELECT id, status, priority, COALESCE(assigned_to, ''),
		customer_id, conversation_id FROM service_tickets WHERE ticket_no = ?`, ticketNo).
		Scan(&id, &status, &priority, &assigned, &customerID, &conversationID)
	if err == sql.ErrNoRows {
		return TicketRow{}, errNotFound("工单不存在")
	}
	if err != nil {
		return TicketRow{}, fmt.Errorf("load ticket: %w", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	newStatus := status
	if in.Status != "" {
		if in.Status != status && !ticketTransitionAllowed(status, in.Status) {
			return TicketRow{}, errConflict("不允许从 " + status + " 流转到 " + in.Status)
		}
		newStatus = in.Status
	}
	newPriority := priority
	if in.Priority != "" {
		newPriority = in.Priority
	}
	newAssigned := assigned
	if in.AssignedTo != "" {
		newAssigned = in.AssignedTo
	}
	if newStatus == "assigned" && newAssigned == "" {
		newAssigned = adminName
	}

	closedAt := "NULL"
	args := []any{newStatus, newPriority, newAssigned, now.Format(time.RFC3339)}
	q := `UPDATE service_tickets SET status = ?, priority = ?, assigned_to = ?, updated_at = ?`
	if newStatus == "closed" || newStatus == "resolved" {
		q += `, closed_at = ?`
		args = append(args, now.Format(time.RFC3339))
	} else {
		q += `, closed_at = ` + closedAt
	}
	q += ` WHERE id = ?`
	args = append(args, id)
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return TicketRow{}, fmt.Errorf("update ticket: %w", err)
	}

	if in.Note != "" || newStatus != status {
		note := in.Note
		if note == "" {
			note = "状态变更为 " + newStatus
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO service_ticket_notes
			(id, ticket_id, admin_user_id, author_name, note, from_status, to_status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			newID(), id, adminID, adminName, note, status, newStatus, now.Format(time.RFC3339)); err != nil {
			return TicketRow{}, fmt.Errorf("insert ticket note: %w", err)
		}
	}

	if err := appendAuditTx(ctx, tx, auditEntry{
		CustomerID: customerID, ConversationID: nullStr(conversationID),
		ActionType: "ticket_update", TargetType: "service_ticket", TargetID: id,
		ActorType: "admin", ActorID: adminID, Result: "succeeded", Reason: in.Note,
		Before: map[string]any{"status": status, "priority": priority, "assigned_to": assigned},
		After:  map[string]any{"status": newStatus, "priority": newPriority, "assigned_to": newAssigned, "operator": adminName},
	}); err != nil {
		return TicketRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return TicketRow{}, fmt.Errorf("commit ticket update: %w", err)
	}

	return s.ticketRow(ctx, ticketNo)
}

func (s *Store) ticketRow(ctx context.Context, ticketNo string) (TicketRow, error) {
	var t TicketRow
	err := s.db.QueryRowContext(ctx, `SELECT t.id, t.ticket_no, t.customer_id, cu.customer_no,
		COALESCE(cu.display_name, ''), COALESCE(t.conversation_id, ''), COALESCE(o.order_no, ''),
		t.ticket_type, t.status, t.priority, COALESCE(t.category, ''), t.summary,
		COALESCE(t.assigned_to, ''), t.created_at, t.updated_at
		FROM service_tickets t
		JOIN customers cu ON cu.id = t.customer_id
		LEFT JOIN orders o ON o.id = t.order_id
		WHERE t.ticket_no = ?`, ticketNo).
		Scan(&t.ID, &t.TicketNo, &t.CustomerID, &t.CustomerNo, &t.CustomerName,
			&t.ConversationID, &t.OrderNo, &t.TicketType, &t.Status, &t.Priority,
			&t.Category, &t.Summary, &t.AssignedTo, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return TicketRow{}, err
	}
	t.AgeHours = ageHours(t.CreatedAt)
	return t, nil
}

// AddTicketNote appends a handling note without changing status.
func (s *Store) AddTicketNote(ctx context.Context, ticketNo, note, adminID, adminName string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return errValidation("处理记录不能为空")
	}
	if len([]rune(note)) > 2000 {
		return errValidation("处理记录不超过 2000 个字符")
	}
	var id, status, customerID string
	var conversationID sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, status, customer_id, conversation_id
		FROM service_tickets WHERE ticket_no = ?`, ticketNo).
		Scan(&id, &status, &customerID, &conversationID)
	if err == sql.ErrNoRows {
		return errNotFound("工单不存在")
	}
	if err != nil {
		return fmt.Errorf("load ticket: %w", err)
	}
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO service_ticket_notes
		(id, ticket_id, admin_user_id, author_name, note, from_status, to_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, newID(), id, adminID, adminName, note, status, status, now); err != nil {
		return fmt.Errorf("insert note: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE service_tickets SET updated_at = ? WHERE id = ?`, now, id); err != nil {
		return fmt.Errorf("touch ticket: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Audit and tool calls
// ---------------------------------------------------------------------------

// AuditFilter narrows the audit log.
type AuditFilter struct {
	Query          string
	Result         string
	ActorType      string
	TargetType     string
	TargetID       string
	ConversationID string
	Page           Page
}

// AuditLogs pages the append-only trail.
func (s *Store) AuditLogs(ctx context.Context, f AuditFilter) (List[AuditRow], error) {
	w := &where{}
	if f.Result != "" {
		w.add("a.result = ?", f.Result)
	}
	if f.ActorType != "" {
		w.add("a.actor_type = ?", f.ActorType)
	}
	if f.TargetType != "" {
		w.add("a.target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		w.add("a.target_id = ?", f.TargetID)
	}
	if f.ConversationID != "" {
		w.add("a.conversation_id = ?", f.ConversationID)
	}
	w.like([]string{"a.action_type", "a.target_id", "COALESCE(a.reason, '')", "COALESCE(cu.customer_no, '')"}, f.Query)

	from := ` FROM action_audit_logs a LEFT JOIN customers cu ON cu.id = a.customer_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT a.id, COALESCE(a.request_id, ''), COALESCE(cu.customer_no, ''),
		COALESCE(a.conversation_id, ''), a.action_type, a.target_type, a.target_id,
		a.actor_type, COALESCE(a.actor_id, ''), a.result, COALESCE(a.reason, ''),
		COALESCE(a.before_json, ''), COALESCE(a.after_json, ''), a.created_at` +
		from + w.clause() + ` ORDER BY a.created_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (AuditRow, error) {
		var a AuditRow
		err := r.Scan(&a.ID, &a.RequestID, &a.CustomerNo, &a.ConversationID, &a.ActionType,
			&a.TargetType, &a.TargetID, &a.ActorType, &a.ActorID, &a.Result, &a.Reason,
			&a.Before, &a.After, &a.CreatedAt)
		return a, err
	})
}

// ToolCallFilter narrows tool-call listings.
type ToolCallFilter struct {
	ConversationID string
	ToolName       string
	Status         string
	Page           Page
}

// ToolCalls pages individual tool executions.
func (s *Store) ToolCalls(ctx context.Context, f ToolCallFilter) (List[ToolCall], error) {
	w := &where{}
	if f.ConversationID != "" {
		w.add("t.conversation_id = ?", f.ConversationID)
	}
	if f.ToolName != "" {
		w.add("t.tool_name = ?", f.ToolName)
	}
	if f.Status != "" {
		w.add("t.status = ?", f.Status)
	}
	from := ` FROM tool_executions t`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT t.id, t.conversation_id, t.tool_call_id, t.tool_name, t.status,
		COALESCE(t.input_json, ''), COALESCE(t.output_json, ''), COALESCE(t.error_code, ''),
		COALESCE(t.error_message_safe, ''), t.latency_ms, t.created_at, COALESCE(t.finished_at, '')` +
		from + w.clause() + ` ORDER BY t.created_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (ToolCall, error) {
		var t ToolCall
		var latency sql.NullInt64
		err := r.Scan(&t.ID, &t.ConversationID, &t.ToolCallID, &t.ToolName, &t.Status,
			&t.Input, &t.Output, &t.ErrorCode, &t.ErrorMessage, &latency, &t.CreatedAt, &t.FinishedAt)
		t.LatencyMS = nullInt(latency)
		return t, err
	})
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// Health counts rows in every business table so an operator can tell at a glance
// whether the process opened the dataset they expect.
func (s *Store) Health(ctx context.Context, schemaVersion int, dbPath, env, llmMode string) (Health, error) {
	tables := []string{
		"customers", "products", "product_skus", "orders", "order_items",
		"shipments", "shipment_events", "service_cases", "service_tickets",
		"payments", "refunds", "invoices", "promotions", "customer_benefits",
		"knowledge_articles", "conversations", "messages", "message_annotations",
		"tool_executions", "pending_actions", "action_audit_logs",
		"admin_users", "admin_sessions", "service_ticket_notes",
	}
	out := Health{SchemaVersion: schemaVersion, DBPath: dbPath, Env: env, LLMMode: llmMode, Tables: []HealthRow{}}
	for _, t := range tables {
		var n int
		// Table names come from the fixed list above, never from user input.
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+t).Scan(&n); err != nil {
			out.Tables = append(out.Tables, HealthRow{Table: t, Count: -1})
			continue
		}
		out.Tables = append(out.Tables, HealthRow{Table: t, Count: n})
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM idempotency_keys`).Scan(&out.Idempotency); err != nil {
		out.Idempotency = -1
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func ageHours(createdAt string) int {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return 0
	}
	return int(time.Since(t).Hours())
}

func itoa(v int) string { return fmt.Sprintf("%d", v) }

func toneFor(v, good, warn float64) string {
	switch {
	case v >= good:
		return "good"
	case v >= warn:
		return "warn"
	default:
		return "bad"
	}
}

func toneForNegative(v int) string {
	switch {
	case v == 0:
		return "good"
	case v < 5:
		return "warn"
	default:
		return "bad"
	}
}

func severity(v int) string {
	switch {
	case v == 0:
		return "good"
	case v < 5:
		return "warn"
	default:
		return "bad"
	}
}
