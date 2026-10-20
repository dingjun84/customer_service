// Package admin implements the operations console: staff accounts, server-side
// sessions, and cross-customer read models.
//
// It deliberately does not go through repository.Store. The console answers
// "what does the whole dataset look like right now", which is an aggregation and
// reporting concern rather than a business transaction, and pretending otherwise
// would mean widening the transactional port with dozens of report-only methods.
// Writes are still explicit, whitelisted and audited (see service.go).
package admin

import (
	"fmt"
	"strings"
	"time"
)

// Role values mirror the CHECK constraint in migration 0002.
const (
	RoleAdmin      = "admin"
	RoleSupervisor = "supervisor"
	RoleAgent      = "agent"
)

// Money renders minor units (CNY fen) the way the console displays them.
func Money(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	whole := minor / 100
	cents := minor % 100
	// Insert thousands separators without pulling in a formatting dependency.
	s := fmt.Sprintf("%d", whole)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := fmt.Sprintf("¥%s.%02d", b.String(), cents)
	if neg {
		return "-" + out
	}
	return out
}

// Page is a 1-based page request. The console uses page numbers rather than
// keyset cursors because operators jump between pages and filter constantly.
type Page struct {
	Page     int
	PageSize int
}

// Normalise clamps a page request into the supported window.
func (p Page) Normalise() Page {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p
}

// Offset is the SQL OFFSET for this page.
func (p Page) Offset() int { return (p.Page - 1) * p.PageSize }

// List is the standard paged envelope the console expects.
type List[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// NewList assembles a page, guaranteeing `items` is never null so the frontend
// can iterate without a null check.
func NewList[T any](items []T, total int, p Page) List[T] {
	if items == nil {
		items = []T{}
	}
	return List[T]{Items: items, Total: total, Page: p.Page, PageSize: p.PageSize}
}

// ---------------------------------------------------------------------------
// Accounts and sessions
// ---------------------------------------------------------------------------

// User is a console operator. The password hash never leaves this package.
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	LastLoginAt  string `json:"last_login_at,omitempty"`
	PasswordHash string `json:"-"`
}

// Session is a resolved login, handed to the request context.
type Session struct {
	User      User
	ExpiresAt time.Time
}

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------

// Metric is one headline number with a drill-down target the UI can link to.
type Metric struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Hint   string `json:"hint,omitempty"`
	Tone   string `json:"tone,omitempty"` // neutral | good | warn | bad
	Link   string `json:"link,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// QueueRow is one backlog bucket on the overview.
type QueueRow struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Count    int    `json:"count"`
	Overdue  int    `json:"overdue"`
	Link     string `json:"link,omitempty"`
	Severity string `json:"severity,omitempty"`
}

// DayPoint is one bucket of the activity sparkline.
type DayPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// Overview is the dashboard payload.
type Overview struct {
	Metrics      []Metric    `json:"metrics"`
	Queues       []QueueRow  `json:"queues"`
	Activity     []DayPoint  `json:"activity"`
	Intents      []IntentRow `json:"intents"`
	TopTools     []ToolStat  `json:"top_tools"`
	RecentFailed []ToolCall  `json:"recent_failures"`
	Health       Health      `json:"health"`
	GeneratedAt  string      `json:"generated_at"`
}

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

// ConversationRow is one line in the cross-customer conversation list.
type ConversationRow struct {
	ID             string `json:"id"`
	ConversationNo string `json:"conversation_no"`
	CustomerID     string `json:"customer_id"`
	CustomerNo     string `json:"customer_no"`
	CustomerName   string `json:"customer_name"`
	Status         string `json:"status"`
	Channel        string `json:"channel"`
	MessageCount   int    `json:"message_count"`
	ToolCount      int    `json:"tool_count"`
	PendingCount   int    `json:"pending_count"`
	Summary        string `json:"summary,omitempty"`
	CreatedAt      string `json:"created_at"`
	LastMessageAt  string `json:"last_message_at,omitempty"`
}

// MessageRow is a single turn, including tool turns for the trace view.
type MessageRow struct {
	ID              string   `json:"id"`
	Role            string   `json:"role"`
	Content         string   `json:"content"`
	ToolName        string   `json:"tool_name,omitempty"`
	ToolArguments   string   `json:"tool_arguments_json,omitempty"`
	ToolResult      string   `json:"tool_result_json,omitempty"`
	CreatedAt       string   `json:"created_at"`
	IntentPrimary   string   `json:"intent_primary,omitempty"`
	IntentSecondary string   `json:"intent_secondary,omitempty"`
	Sentiment       string   `json:"sentiment,omitempty"`
	Resolution      string   `json:"resolution_status,omitempty"`
	Confidence      *float64 `json:"confidence,omitempty"`
}

// ConversationDetail bundles everything the detail drawer renders.
type ConversationDetail struct {
	Conversation ConversationRow `json:"conversation"`
	Messages     []MessageRow    `json:"messages"`
	ToolCalls    []ToolCall      `json:"tool_calls"`
	Pending      []PendingAction `json:"pending_actions"`
	Audit        []AuditRow      `json:"audit_logs"`
	Orders       []OrderRow      `json:"orders"`
}

// ---------------------------------------------------------------------------
// Tickets
// ---------------------------------------------------------------------------

// TicketRow is one line in the ticket queue.
type TicketRow struct {
	ID             string `json:"id"`
	TicketNo       string `json:"ticket_no"`
	CustomerID     string `json:"customer_id"`
	CustomerNo     string `json:"customer_no"`
	CustomerName   string `json:"customer_name"`
	ConversationID string `json:"conversation_id,omitempty"`
	OrderNo        string `json:"order_no,omitempty"`
	TicketType     string `json:"ticket_type"`
	Status         string `json:"status"`
	Priority       string `json:"priority"`
	Category       string `json:"category,omitempty"`
	Summary        string `json:"summary"`
	AssignedTo     string `json:"assigned_to,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	AgeHours       int    `json:"age_hours"`
}

// TicketNote is one handling step.
type TicketNote struct {
	ID         string `json:"id"`
	AuthorName string `json:"author_name"`
	Note       string `json:"note"`
	FromStatus string `json:"from_status,omitempty"`
	ToStatus   string `json:"to_status,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// TicketDetail is the ticket drawer payload.
type TicketDetail struct {
	Ticket TicketRow    `json:"ticket"`
	Notes  []TicketNote `json:"notes"`
	Audit  []AuditRow   `json:"audit_logs"`
}

// TicketUpdate is the only mutate-in-place operation the console offers.
type TicketUpdate struct {
	Status     string `json:"status"`
	Priority   string `json:"priority,omitempty"`
	AssignedTo string `json:"assigned_to,omitempty"`
	Note       string `json:"note,omitempty"`
}

// ---------------------------------------------------------------------------
// Pending actions
// ---------------------------------------------------------------------------

// PendingAction is a proposal a customer still has to confirm. The console can
// only watch it or void it: confirming on the customer's behalf would break the
// "proposal + explicit confirmation" guarantee the whole system is built on.
type PendingAction struct {
	ID             string `json:"id"`
	ActionNo       string `json:"action_no"`
	ConversationID string `json:"conversation_id"`
	CustomerNo     string `json:"customer_no"`
	CustomerName   string `json:"customer_name"`
	ActionType     string `json:"action_type"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Status         string `json:"status"`
	Summary        string `json:"summary"`
	Payload        string `json:"payload_json"`
	Preview        string `json:"preview_json"`
	CreatedAt      string `json:"created_at"`
	ExpiresAt      string `json:"expires_at"`
	ConfirmedAt    string `json:"confirmed_at,omitempty"`
	ExecutedAt     string `json:"executed_at,omitempty"`
	Result         string `json:"result_json,omitempty"`
	IsOverdue      bool   `json:"is_overdue"`
}

// ---------------------------------------------------------------------------
// Audit / tools / analytics
// ---------------------------------------------------------------------------

// AuditRow is one append-only audit entry.
type AuditRow struct {
	ID             string `json:"id"`
	RequestID      string `json:"request_id,omitempty"`
	CustomerNo     string `json:"customer_no,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	ActionType     string `json:"action_type"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	ActorType      string `json:"actor_type"`
	ActorID        string `json:"actor_id,omitempty"`
	Result         string `json:"result"`
	Reason         string `json:"reason,omitempty"`
	Before         string `json:"before_json,omitempty"`
	After          string `json:"after_json,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// ToolCall is one tool execution row.
type ToolCall struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	ToolCallID     string `json:"tool_call_id"`
	ToolName       string `json:"tool_name"`
	Status         string `json:"status"`
	Input          string `json:"input_json,omitempty"`
	Output         string `json:"output_json,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message_safe,omitempty"`
	LatencyMS      *int64 `json:"latency_ms,omitempty"`
	CreatedAt      string `json:"created_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
}

// ToolStat aggregates one tool across all conversations.
type ToolStat struct {
	ToolName    string  `json:"tool_name"`
	Calls       int     `json:"calls"`
	Succeeded   int     `json:"succeeded"`
	Failed      int     `json:"failed"`
	Rejected    int     `json:"rejected"`
	SuccessRate float64 `json:"success_rate"`
	P95Latency  int64   `json:"p95_latency_ms"`
	AvgLatency  int64   `json:"avg_latency_ms"`
	TopError    string  `json:"top_error_code,omitempty"`
}

// IntentRow aggregates one annotated intent.
type IntentRow struct {
	IntentPrimary string  `json:"intent_primary"`
	Samples       int     `json:"samples"`
	Resolved      int     `json:"resolved"`
	Unresolved    int     `json:"unresolved"`
	Transferred   int     `json:"transferred"`
	ResolveRate   float64 `json:"resolve_rate"`
	AvgConfidence float64 `json:"avg_confidence"`
	Positive      int     `json:"positive"`
	Negative      int     `json:"negative"`
	Neutral       int     `json:"neutral"`
}

// ---------------------------------------------------------------------------
// Knowledge base
// ---------------------------------------------------------------------------

// KnowledgeRow is one article line.
type KnowledgeRow struct {
	ID          string `json:"id"`
	ArticleCode string `json:"article_code"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Source      string `json:"source,omitempty"`
	Version     int    `json:"version"`
	Keywords    string `json:"keywords_json"`
	Excerpt     string `json:"excerpt"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// KnowledgeDetail adds the full body.
type KnowledgeDetail struct {
	KnowledgeRow
	Content string `json:"content"`
}

// KnowledgeInput is the create/update payload. Status changes go through a
// dedicated method so publishing stays an explicit act.
type KnowledgeInput struct {
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Keywords []string `json:"keywords"`
	Source   string   `json:"source"`
	Status   string   `json:"status"`
}

// ---------------------------------------------------------------------------
// Business data
// ---------------------------------------------------------------------------

// CustomerRow is one customer line with its lifetime counters.
type CustomerRow struct {
	ID                string `json:"id"`
	CustomerNo        string `json:"customer_no"`
	DisplayName       string `json:"display_name"`
	PhoneMasked       string `json:"phone_masked"`
	Status            string `json:"status"`
	OrderCount        int    `json:"order_count"`
	PaidAmount        int64  `json:"paid_amount_minor"`
	PaidDisplay       string `json:"paid_display"`
	ConversationCount int    `json:"conversation_count"`
	TicketCount       int    `json:"ticket_count"`
	CreatedAt         string `json:"created_at"`
}

// CustomerDetail is the customer drawer payload.
type CustomerDetail struct {
	Customer      CustomerRow       `json:"customer"`
	Orders        []OrderRow        `json:"orders"`
	Conversations []ConversationRow `json:"conversations"`
	Tickets       []TicketRow       `json:"tickets"`
	Benefits      []BenefitRow      `json:"benefits"`
}

// BenefitRow is one coupon / points / balance entry.
type BenefitRow struct {
	ID          string `json:"id"`
	BenefitType string `json:"benefit_type"`
	BenefitCode string `json:"benefit_code,omitempty"`
	Status      string `json:"status"`
	Amount      string `json:"amount_display,omitempty"`
	Points      *int64 `json:"points,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// OrderRow is one order line.
type OrderRow struct {
	ID              string `json:"id"`
	OrderNo         string `json:"order_no"`
	CustomerNo      string `json:"customer_no"`
	CustomerName    string `json:"customer_name"`
	Status          string `json:"order_status"`
	Channel         string `json:"order_channel"`
	Paid            int64  `json:"paid_amount_minor"`
	PaidDisplay     string `json:"paid_display"`
	Refunded        int64  `json:"refunded_amount_minor"`
	RefundedDisplay string `json:"refunded_display"`
	ItemCount       int    `json:"item_count"`
	IsPresale       bool   `json:"is_presale"`
	CreatedAt       string `json:"created_at"`
	PaidAt          string `json:"paid_at,omitempty"`
}

// OrderItemRow is a line item with its product snapshot.
type OrderItemRow struct {
	ID          string `json:"id"`
	ProductName string `json:"product_name"`
	SKUName     string `json:"sku_name,omitempty"`
	ProductCode string `json:"product_code,omitempty"`
	Quantity    int    `json:"quantity"`
	RefundedQty int    `json:"refunded_quantity"`
	UnitPrice   int64  `json:"unit_price_minor"`
	UnitDisplay string `json:"unit_price_display"`
	PaidAmount  int64  `json:"paid_amount_minor"`
	Status      string `json:"item_status"`
}

// ShipmentRow is one parcel with its latest event.
type ShipmentRow struct {
	ID          string `json:"id"`
	ShipmentNo  string `json:"shipment_no"`
	CarrierName string `json:"carrier_name,omitempty"`
	TrackingNo  string `json:"tracking_no,omitempty"`
	Status      string `json:"shipment_status"`
	EstimatedAt string `json:"estimated_delivery_at,omitempty"`
	LastEventAt string `json:"last_event_at,omitempty"`
	LatestEvent string `json:"latest_event,omitempty"`
}

// OrderDetail is the order drawer payload.
type OrderDetail struct {
	Order     OrderRow         `json:"order"`
	Items     []OrderItemRow   `json:"items"`
	Shipments []ShipmentRow    `json:"shipments"`
	Payments  []PaymentRow     `json:"payments"`
	Refunds   []RefundRow      `json:"refunds"`
	Cases     []ServiceCaseRow `json:"service_cases"`
	Invoices  []InvoiceRow     `json:"invoices"`
	Receiver  ReceiverInfo     `json:"receiver"`
	Audit     []AuditRow       `json:"audit_logs"`
}

// ReceiverInfo is the shipping snapshot, already masked upstream.
type ReceiverInfo struct {
	Name    string `json:"name,omitempty"`
	Phone   string `json:"phone_masked,omitempty"`
	Address string `json:"address,omitempty"`
	Method  string `json:"shipping_method,omitempty"`
	Remark  string `json:"remark,omitempty"`
}

// PaymentRow is one payment attempt.
type PaymentRow struct {
	ID            string `json:"id"`
	PaymentNo     string `json:"payment_no"`
	Method        string `json:"payment_method"`
	Status        string `json:"payment_status"`
	Amount        int64  `json:"amount_minor"`
	AmountDisplay string `json:"amount_display"`
	Refunded      int64  `json:"refunded_amount_minor"`
	PaidAt        string `json:"paid_at,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// RefundRow is one refund.
type RefundRow struct {
	ID            string `json:"id"`
	RefundNo      string `json:"refund_no"`
	OrderNo       string `json:"order_no"`
	CustomerNo    string `json:"customer_no"`
	Status        string `json:"refund_status"`
	Channel       string `json:"refund_channel,omitempty"`
	Amount        int64  `json:"amount_minor"`
	AmountDisplay string `json:"amount_display"`
	Reason        string `json:"reason,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	RequestedAt   string `json:"requested_at"`
	CompletedAt   string `json:"completed_at,omitempty"`
}

// ServiceCaseRow is one after-sales case.
type ServiceCaseRow struct {
	ID          string `json:"id"`
	CaseNo      string `json:"case_no"`
	OrderNo     string `json:"order_no"`
	CustomerNo  string `json:"customer_no"`
	CaseType    string `json:"case_type"`
	Status      string `json:"case_status"`
	ReasonCode  string `json:"reason_code,omitempty"`
	Description string `json:"description,omitempty"`
	Quantity    int    `json:"requested_quantity"`
	Resolution  string `json:"resolution,omitempty"`
	CreatedAt   string `json:"created_at"`
	ClosedAt    string `json:"closed_at,omitempty"`
}

// InvoiceRow is one invoice.
type InvoiceRow struct {
	ID            string `json:"id"`
	InvoiceNo     string `json:"invoice_no,omitempty"`
	OrderNo       string `json:"order_no"`
	CustomerNo    string `json:"customer_no"`
	Type          string `json:"invoice_type"`
	Status        string `json:"invoice_status"`
	Title         string `json:"title"`
	Amount        int64  `json:"amount_minor"`
	AmountDisplay string `json:"amount_display"`
	RequestedAt   string `json:"requested_at"`
	IssuedAt      string `json:"issued_at,omitempty"`
}

// PromotionRow is one campaign.
type PromotionRow struct {
	ID            string `json:"id"`
	PromotionCode string `json:"promotion_code"`
	Title         string `json:"title"`
	Type          string `json:"promotion_type"`
	Status        string `json:"status"`
	StartsAt      string `json:"starts_at,omitempty"`
	EndsAt        string `json:"ends_at,omitempty"`
	Rules         string `json:"rules_json"`
	Description   string `json:"description,omitempty"`
}

// ProductRow is one product line.
type ProductRow struct {
	ID          string `json:"id"`
	ProductCode string `json:"product_code"`
	Name        string `json:"name"`
	Brand       string `json:"brand,omitempty"`
	Category    string `json:"category,omitempty"`
	Status      string `json:"status"`
	SKUCount    int    `json:"sku_count"`
	StockTotal  int64  `json:"stock_total"`
	PriceFrom   int64  `json:"price_from_minor"`
	PriceTo     int64  `json:"price_to_minor"`
	PriceRange  string `json:"price_range_display"`
	CreatedAt   string `json:"created_at"`
}

// SKURow is one variant.
type SKURow struct {
	ID           string `json:"id"`
	SKUCode      string `json:"sku_code"`
	SKUName      string `json:"sku_name"`
	Specs        string `json:"specifications_json"`
	Price        int64  `json:"list_price_minor"`
	PriceDisplay string `json:"list_price_display"`
	Stock        int64  `json:"stock_quantity"`
	Status       string `json:"status"`
}

// ProductDetail is the product drawer payload.
type ProductDetail struct {
	Product     ProductRow `json:"product"`
	Description string     `json:"description,omitempty"`
	Attributes  string     `json:"attributes_json"`
	SKUs        []SKURow   `json:"skus"`
}

// HealthRow is one table's row count, used as a dataset sanity check.
type HealthRow struct {
	Table string `json:"table"`
	Count int    `json:"count"`
}

// Health is the console's data-health card.
type Health struct {
	SchemaVersion  int         `json:"schema_version"`
	DBPath         string      `json:"db_path,omitempty"`
	Env            string      `json:"app_env"`
	Tables         []HealthRow `json:"tables"`
	Idempotency    int         `json:"idempotency_keys"`
	ActiveSessions int         `json:"active_sessions"`
	LLMMode        string      `json:"llm_mode"`
}
