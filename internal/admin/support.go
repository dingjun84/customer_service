package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// Thin aliases keep the query layer readable and make it obvious that these are
// user-facing validation failures rather than driver errors.
func errValidation(msg string) error { return domain.ValidationError(msg) }
func errNotFound(msg string) error   { return domain.NotFoundError(msg) }
func errConflict(msg string) error   { return domain.ConflictError(msg) }

func newID() string { return domain.NewID() }

// jsonString decodes a JSON string literal; used for preview payloads that hold
// a bare summary string.
func jsonString(raw string) (string, bool) {
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", false
	}
	return s, true
}

// jsonField pulls one string-ish field out of a JSON object.
func jsonField(raw, key string) (string, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return fmt.Sprintf("%g", t), true
	case bool:
		return fmt.Sprintf("%t", t), true
	default:
		return "", false
	}
}

// auditEntry is the input for one append-only audit row.
type auditEntry struct {
	RequestID      string
	CustomerID     string
	ConversationID string
	ActionType     string
	TargetType     string
	TargetID       string
	ActorType      string
	ActorID        string
	Result         string
	Reason         string
	Before         any
	After          any
}

// appendAuditTx writes an audit row inside the caller's transaction, so a write
// and its audit trail can never diverge.
func appendAuditTx(ctx context.Context, tx *sql.Tx, e auditEntry) error {
	before, err := marshalNullable(e.Before)
	if err != nil {
		return err
	}
	after, err := marshalNullable(e.After)
	if err != nil {
		return err
	}
	actorType := e.ActorType
	if actorType == "" {
		actorType = "admin"
	}
	result := e.Result
	if result == "" {
		result = "succeeded"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO action_audit_logs
		(id, request_id, customer_id, conversation_id, action_type, target_type, target_id,
		 before_json, after_json, actor_type, actor_id, result, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), nullable(e.RequestID), nullable(e.CustomerID), nullable(e.ConversationID),
		e.ActionType, e.TargetType, e.TargetID, before, after, actorType, nullable(e.ActorID),
		result, nullable(e.Reason), nowRFC3339())
	if err != nil {
		return fmt.Errorf("append audit log: %w", err)
	}
	return nil
}

func marshalNullable(v any) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshal audit snapshot: %w", err)
	}
	return sql.NullString{String: string(raw), Valid: true}, nil
}

func nullable(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// ---------------------------------------------------------------------------
// Ticket state machine
// ---------------------------------------------------------------------------

var ticketStatuses = map[string]bool{
	"open": true, "assigned": true, "pending_customer": true, "resolved": true, "closed": true,
}

var ticketTransitions = map[string][]string{
	"open":             {"assigned", "pending_customer", "resolved", "closed"},
	"assigned":         {"open", "pending_customer", "resolved", "closed"},
	"pending_customer": {"assigned", "resolved", "closed"},
	"resolved":         {"closed", "open"},
	"closed":           {"open"},
}

func validTicketStatus(s string) bool { return ticketStatuses[s] }

func validPriority(s string) bool {
	switch s {
	case "low", "normal", "high", "urgent":
		return true
	}
	return false
}

// ticketTransitionAllowed keeps the queue honest: a closed ticket cannot jump
// straight back to resolved without being reopened first, which preserves the
// order in which an operator actually handled it.
func ticketTransitionAllowed(from, to string) bool {
	for _, allowed := range ticketTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

func nowRFC3339() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}
