package seed

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// dataset is the whole generated dataset held in memory before a single
// transaction writes it. Every validator runs on this struct, so a violation
// aborts the import before any row reaches SQLite.
type dataset struct {
	customers     []customerRow
	products      []productRow
	skus          []skuRow
	promotions    []promotionRow
	benefits      []benefitRow
	articles      []articleRow
	orders        []orderRow
	items         []itemRow
	payments      []paymentRow
	refunds       []refundRow
	shipments     []shipmentRow
	events        []eventRow
	cases         []caseRow
	invoices      []invoiceRow
	conversations []conversationRow
	messages      []messageRow
	annotations   []annotationRow
	toolExecs     []toolExecRow
	pending       []pendingRow
	audits        []auditRow
	tickets       []ticketRow
	idempotency   []idempotencyRow
	scenarios     []scenarioRow
}

type customerRow struct {
	ID          string
	No          string
	Name        string
	PhoneMasked string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type productRow struct {
	ID        string
	Code      string
	Name      string
	Brand     string
	Category  string
	Desc      string
	Status    string
	Attrs     map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

type skuRow struct {
	ID        string
	ProductID string
	Code      string
	Name      string
	Specs     map[string]any
	ListPrice int64
	Stock     int64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type promotionRow struct {
	ID        string
	Code      string
	Title     string
	Desc      string
	Type      string
	Status    string
	StartsAt  *time.Time
	EndsAt    *time.Time
	Rules     map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

type benefitRow struct {
	ID         string
	CustomerID string
	Type       string
	Code       string
	Status     string
	Amount     *int64
	Points     *int64
	ExpiresAt  *time.Time
	Attrs      map[string]any
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type articleRow struct {
	ID        string
	Code      string
	Category  string
	Title     string
	Content   string
	Keywords  []string
	Status    string
	Source    string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type orderRow struct {
	ID                   string
	No                   string
	CustomerID           string
	Status               string
	Channel              string
	ItemAmount           int64
	Discount             int64
	Shipping             int64
	Paid                 int64
	Refunded             int64
	Presale              bool
	ReceiverName         string
	ReceiverPhoneMasked  string
	Province             string
	City                 string
	District             string
	Address              string
	PostalCode           string
	ShippingMethod       string
	RequestedDeliveryAt  *time.Time
	InvoiceTypeRequested string
	Remark               string
	CreatedAt            time.Time
	PaidAt               *time.Time
	ShippedAt            *time.Time
	DeliveredAt          *time.Time
	CompletedAt          *time.Time
	CancelledAt          *time.Time
	UpdatedAt            time.Time
	Version              int
}

type itemRow struct {
	ID            string
	OrderID       string
	ProductID     string
	SKUID         string
	ProductCode   string
	ProductName   string
	SKUCode       string
	SKUName       string
	SKUAttributes map[string]any
	Quantity      int64
	RefundedQty   int64
	UnitPrice     int64
	Discount      int64
	Paid          int64
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type paymentRow struct {
	ID          string
	No          string
	OrderID     string
	Method      string
	Status      string
	Amount      int64
	Refunded    int64
	ProviderRef string
	PaidAt      *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type refundRow struct {
	ID            string
	No            string
	OrderID       string
	PaymentID     string
	CaseID        string
	Status        string
	Channel       string
	Amount        int64
	Reason        string
	RequestedAt   time.Time
	CompletedAt   *time.Time
	FailureReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// shipmentRow carries a non-column Direction so the validator can tell forward,
// reshipment and return parcels apart (schema has no direction column).
type shipmentRow struct {
	ID                  string
	No                  string
	OrderID             string
	CarrierCode         string
	CarrierName         string
	TrackingNo          string
	Status              string
	EstimatedDeliveryAt *time.Time
	LastEventAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Direction           string
}

type eventRow struct {
	ID         string
	ShipmentID string
	At         time.Time
	Code       string
	Desc       string
	Location   string
	Source     string
	CreatedAt  time.Time
}

type caseRow struct {
	ID          string
	No          string
	CustomerID  string
	OrderID     string
	OrderItemID string
	Type        string
	Status      string
	ReasonCode  string
	ReasonText  string
	Desc        string
	Quantity    int64
	Resolution  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
}

type invoiceRow struct {
	ID          string
	No          string
	CustomerID  string
	OrderID     string
	Type        string
	Status      string
	Title       string
	TaxNo       string
	Amount      int64
	Email       string
	FileURL     string
	RequestedAt time.Time
	IssuedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type conversationRow struct {
	ID          string
	No          string
	CustomerID  string
	Status      string
	Channel     string
	ActiveTask  *string
	Summary     *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastMessage *time.Time
}

type messageRow struct {
	ID         string
	ConvID     string
	Role       string
	Content    string
	ToolName   string
	ToolCallID string
	ToolArgs   string
	ToolResult string
	Meta       map[string]any
	Sequence   int
	CreatedAt  time.Time
}

type annotationRow struct {
	ID         string
	MessageID  string
	Intent     string
	SubIntent  string
	Sentiment  string
	Resolution string
	Confidence float64
	Source     string
	CreatedAt  time.Time
}

type toolExecRow struct {
	ID           string
	ConvID       string
	MessageID    string
	ToolCallID   string
	ToolName     string
	Input        string
	Output       string
	Status       string
	ErrorCode    string
	ErrorMessage string
	LatencyMs    int64
	CreatedAt    time.Time
	FinishedAt   *time.Time
}

type pendingRow struct {
	ID          string
	ActionNo    string
	ConvID      string
	CustomerID  string
	ActionType  string
	TargetType  string
	TargetID    string
	Payload     string
	Preview     string
	Status      string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	ConfirmedAt *time.Time
	ExecutedAt  *time.Time
	ResultJSON  string
}

type auditRow struct {
	ID         string
	RequestID  string
	CustomerID string
	ConvID     string
	ActionType string
	TargetType string
	TargetID   string
	Before     string
	After      string
	ActorType  string
	ActorID    string
	Result     string
	Reason     string
	CreatedAt  time.Time
}

type ticketRow struct {
	ID         string
	No         string
	CustomerID string
	ConvID     string
	OrderID    string
	Type       string
	Status     string
	Priority   string
	Category   string
	Summary    string
	Details    string
	AssignedTo string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ClosedAt   *time.Time
}

type idempotencyRow struct {
	Key         string
	Operation   string
	RequestHash string
	Response    string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// scenarioRow is evaluation metadata. It is written to testdata/scenarios and
// never into the conversation tables, so it cannot leak into model context.
type scenarioRow struct {
	ScenarioID       string           `json:"scenario_id"`
	Category         string           `json:"category"`
	IntentTags       []string         `json:"intent_tags"`
	CustomerNo       string           `json:"customer_no"`
	ConversationNo   string           `json:"conversation_id"`
	Split            string           `json:"split"`
	Turns            []turn           `json:"turns"`
	Fixture          map[string]any   `json:"fixtures"`
	ExpectedTools    []map[string]any `json:"expected_tool_calls"`
	ForbiddenActions []string         `json:"forbidden_actions"`
	ExpectedOutcome  string           `json:"expected_outcome"`
	RiskLevel        string           `json:"risk_level"`
	PassCriteria     []string         `json:"pass_criteria"`
}

type turn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ---- deterministic identifiers ----

// stableID derives a UUID-shaped identifier from natural keys so regenerating
// the dataset produces byte-identical rows instead of fresh random UUIDs.
func stableID(parts ...string) string {
	sum := sha1.Sum([]byte("seed-v1|" + strings.Join(parts, "|")))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // version 5 style
	b[8] = (b[8] & 0x3f) | 0x80
	var dst [36]byte
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst[:])
}

// ---- value helpers ----

func mustJSON(v any) string {
	if v == nil {
		return "{}"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func jsonOrNil(v any) any {
	if v == nil {
		return nil
	}
	return mustJSON(v)
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

func timePtr(t time.Time) *time.Time { return &t }

func strOrNil(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func i64Ptr(v int64) *int64 { return &v }

// ---- bulk insert ----

type insertSpec struct {
	table string
	cols  []string
	rows  [][]any
}

func insertAll(ctx context.Context, tx *sql.Tx, specs []insertSpec) error {
	for _, spec := range specs {
		if len(spec.rows) == 0 {
			continue
		}
		ph := make([]string, len(spec.cols))
		for i := range ph {
			ph[i] = "?"
		}
		stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			spec.table, strings.Join(spec.cols, ", "), strings.Join(ph, ","))
		prepared, err := tx.PrepareContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("prepare %s: %w", spec.table, err)
		}
		for i, row := range spec.rows {
			if len(row) != len(spec.cols) {
				prepared.Close()
				return fmt.Errorf("%s row %d has %d values for %d columns", spec.table, i, len(row), len(spec.cols))
			}
			if _, err := prepared.ExecContext(ctx, row...); err != nil {
				prepared.Close()
				return fmt.Errorf("insert %s row %d: %w", spec.table, i, err)
			}
		}
		if err := prepared.Close(); err != nil {
			return fmt.Errorf("close %s: %w", spec.table, err)
		}
	}
	return nil
}

func (d *dataset) insertSpecs() []insertSpec {
	return []insertSpec{
		{"customers",
			[]string{"id", "customer_no", "display_name", "phone_masked", "status", "created_at", "updated_at"},
			mapRows(d.customers, func(r customerRow) []any {
				return []any{r.ID, r.No, r.Name, r.PhoneMasked, r.Status, ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"products",
			[]string{"id", "product_code", "name", "brand", "category", "description", "status", "attributes_json", "created_at", "updated_at"},
			mapRows(d.products, func(r productRow) []any {
				return []any{r.ID, r.Code, r.Name, r.Brand, r.Category, r.Desc, r.Status, mustJSON(r.Attrs), ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"product_skus",
			[]string{"id", "product_id", "sku_code", "sku_name", "specifications_json", "list_price_minor", "stock_quantity", "status", "created_at", "updated_at"},
			mapRows(d.skus, func(r skuRow) []any {
				return []any{r.ID, r.ProductID, r.Code, r.Name, mustJSON(r.Specs), r.ListPrice, r.Stock, r.Status, ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"promotions",
			[]string{"id", "promotion_code", "title", "description", "promotion_type", "status", "starts_at", "ends_at", "rules_json", "created_at", "updated_at"},
			mapRows(d.promotions, func(r promotionRow) []any {
				return []any{r.ID, r.Code, r.Title, r.Desc, r.Type, r.Status, tsPtr(r.StartsAt), tsPtr(r.EndsAt), mustJSON(r.Rules), ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"customer_benefits",
			[]string{"id", "customer_id", "benefit_type", "benefit_code", "status", "amount_minor", "points", "expires_at", "attributes_json", "created_at", "updated_at"},
			mapRows(d.benefits, func(r benefitRow) []any {
				return []any{r.ID, r.CustomerID, r.Type, strOrNil(r.Code), r.Status, ptrAny(r.Amount), ptrAny(r.Points), tsPtr(r.ExpiresAt), mustJSON(r.Attrs), ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"knowledge_articles",
			[]string{"id", "article_code", "category", "title", "content", "keywords_json", "status", "source", "version", "created_at", "updated_at"},
			mapRows(d.articles, func(r articleRow) []any {
				return []any{r.ID, r.Code, r.Category, r.Title, r.Content, mustJSON(r.Keywords), r.Status, r.Source, r.Version, ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"orders",
			[]string{"id", "order_no", "customer_id", "order_status", "order_channel", "currency", "item_amount_minor",
				"discount_amount_minor", "shipping_fee_minor", "paid_amount_minor", "refunded_amount_minor", "is_presale",
				"receiver_name", "receiver_phone_masked", "shipping_province", "shipping_city", "shipping_district",
				"shipping_address", "shipping_postal_code", "shipping_method", "requested_delivery_at",
				"invoice_type_requested", "order_remark", "created_at", "paid_at", "shipped_at", "delivered_at",
				"completed_at", "cancelled_at", "updated_at", "version"},
			mapRows(d.orders, func(r orderRow) []any {
				return []any{r.ID, r.No, r.CustomerID, r.Status, r.Channel, "CNY", r.ItemAmount, r.Discount, r.Shipping,
					r.Paid, r.Refunded, boolInt(r.Presale), r.ReceiverName, r.ReceiverPhoneMasked, r.Province, r.City,
					r.District, r.Address, r.PostalCode, r.ShippingMethod, tsPtr(r.RequestedDeliveryAt),
					strOrNil(r.InvoiceTypeRequested), strOrNil(r.Remark), ts(r.CreatedAt), tsPtr(r.PaidAt),
					tsPtr(r.ShippedAt), tsPtr(r.DeliveredAt), tsPtr(r.CompletedAt), tsPtr(r.CancelledAt),
					ts(r.UpdatedAt), r.Version}
			})},
		{"order_items",
			[]string{"id", "order_id", "product_id", "sku_id", "product_code_snapshot", "product_name_snapshot",
				"sku_code_snapshot", "sku_name_snapshot", "sku_attributes_json", "quantity", "refunded_quantity",
				"unit_price_minor", "discount_amount_minor", "paid_amount_minor", "item_status", "created_at", "updated_at"},
			mapRows(d.items, func(r itemRow) []any {
				return []any{r.ID, r.OrderID, r.ProductID, r.SKUID, r.ProductCode, r.ProductName, r.SKUCode, r.SKUName,
					mustJSON(r.SKUAttributes), r.Quantity, r.RefundedQty, r.UnitPrice, r.Discount, r.Paid, r.Status,
					ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"payments",
			[]string{"id", "payment_no", "order_id", "payment_method", "payment_status", "amount_minor",
				"refunded_amount_minor", "provider_transaction_ref", "paid_at", "created_at", "updated_at"},
			mapRows(d.payments, func(r paymentRow) []any {
				return []any{r.ID, r.No, r.OrderID, r.Method, r.Status, r.Amount, r.Refunded, strOrNil(r.ProviderRef),
					tsPtr(r.PaidAt), ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"refunds",
			[]string{"id", "refund_no", "order_id", "payment_id", "service_case_id", "refund_status", "refund_channel",
				"amount_minor", "reason", "requested_at", "completed_at", "failure_reason", "created_at", "updated_at"},
			mapRows(d.refunds, func(r refundRow) []any {
				return []any{r.ID, r.No, r.OrderID, strOrNil(r.PaymentID), strOrNil(r.CaseID), r.Status, r.Channel,
					r.Amount, strOrNil(r.Reason), ts(r.RequestedAt), tsPtr(r.CompletedAt), strOrNil(r.FailureReason),
					ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"shipments",
			[]string{"id", "shipment_no", "order_id", "carrier_code", "carrier_name", "tracking_no", "shipment_status",
				"estimated_delivery_at", "last_event_at", "created_at", "updated_at"},
			mapRows(d.shipments, func(r shipmentRow) []any {
				return []any{r.ID, r.No, r.OrderID, r.CarrierCode, r.CarrierName, r.TrackingNo, r.Status,
					tsPtr(r.EstimatedDeliveryAt), tsPtr(r.LastEventAt), ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"shipment_events",
			[]string{"id", "shipment_id", "event_time", "event_code", "description", "location", "source", "created_at"},
			mapRows(d.events, func(r eventRow) []any {
				return []any{r.ID, r.ShipmentID, ts(r.At), r.Code, r.Desc, strOrNil(r.Location), r.Source, ts(r.CreatedAt)}
			})},
		{"service_cases",
			[]string{"id", "case_no", "customer_id", "order_id", "order_item_id", "case_type", "case_status",
				"reason_code", "reason_text", "description", "requested_quantity", "resolution", "created_at", "updated_at", "closed_at"},
			mapRows(d.cases, func(r caseRow) []any {
				return []any{r.ID, r.No, r.CustomerID, r.OrderID, strOrNil(r.OrderItemID), r.Type, r.Status,
					strOrNil(r.ReasonCode), strOrNil(r.ReasonText), strOrNil(r.Desc), r.Quantity, strOrNil(r.Resolution),
					ts(r.CreatedAt), ts(r.UpdatedAt), tsPtr(r.ClosedAt)}
			})},
		{"invoices",
			[]string{"id", "invoice_no", "customer_id", "order_id", "invoice_type", "invoice_status", "title", "tax_no",
				"amount_minor", "email", "file_url", "requested_at", "issued_at", "created_at", "updated_at"},
			mapRows(d.invoices, func(r invoiceRow) []any {
				return []any{r.ID, strOrNil(r.No), r.CustomerID, r.OrderID, r.Type, r.Status, r.Title, strOrNil(r.TaxNo),
					r.Amount, strOrNil(r.Email), strOrNil(r.FileURL), ts(r.RequestedAt), tsPtr(r.IssuedAt),
					ts(r.CreatedAt), ts(r.UpdatedAt)}
			})},
		{"conversations",
			[]string{"id", "conversation_no", "customer_id", "status", "channel", "active_task_state_json", "summary",
				"created_at", "updated_at", "last_message_at"},
			mapRows(d.conversations, func(r conversationRow) []any {
				return []any{r.ID, r.No, r.CustomerID, r.Status, r.Channel, ptrString(r.ActiveTask), ptrString(r.Summary),
					ts(r.CreatedAt), ts(r.UpdatedAt), tsPtr(r.LastMessage)}
			})},
		{"messages",
			[]string{"id", "conversation_id", "role", "content", "tool_name", "tool_call_id", "tool_arguments_json",
				"tool_result_json", "metadata_json", "created_at"},
			mapRows(d.messages, func(r messageRow) []any {
				return []any{r.ID, r.ConvID, r.Role, r.Content, strOrNil(r.ToolName), strOrNil(r.ToolCallID),
					strOrNil(r.ToolArgs), strOrNil(r.ToolResult), mustJSON(r.Meta), ts(r.CreatedAt)}
			})},
		{"message_annotations",
			[]string{"id", "message_id", "intent_primary", "intent_secondary", "sentiment", "resolution_status",
				"confidence", "annotation_source", "created_at"},
			mapRows(d.annotations, func(r annotationRow) []any {
				return []any{r.ID, r.MessageID, strOrNil(r.Intent), strOrNil(r.SubIntent), strOrNil(r.Sentiment),
					strOrNil(r.Resolution), r.Confidence, r.Source, ts(r.CreatedAt)}
			})},
		{"tool_executions",
			[]string{"id", "conversation_id", "message_id", "tool_call_id", "tool_name", "input_json", "output_json",
				"status", "error_code", "error_message_safe", "latency_ms", "created_at", "finished_at"},
			mapRows(d.toolExecs, func(r toolExecRow) []any {
				return []any{r.ID, r.ConvID, strOrNil(r.MessageID), r.ToolCallID, r.ToolName, r.Input, strOrNil(r.Output),
					r.Status, strOrNil(r.ErrorCode), strOrNil(r.ErrorMessage), r.LatencyMs, ts(r.CreatedAt), tsPtr(r.FinishedAt)}
			})},
		{"pending_actions",
			[]string{"id", "action_no", "conversation_id", "customer_id", "action_type", "target_type", "target_id",
				"payload_json", "preview_json", "status", "expires_at", "created_at", "confirmed_at", "executed_at", "result_json"},
			mapRows(d.pending, func(r pendingRow) []any {
				return []any{r.ID, r.ActionNo, r.ConvID, r.CustomerID, r.ActionType, r.TargetType, r.TargetID,
					r.Payload, r.Preview, r.Status, ts(r.ExpiresAt), ts(r.CreatedAt), tsPtr(r.ConfirmedAt),
					tsPtr(r.ExecutedAt), strOrNil(r.ResultJSON)}
			})},
		{"action_audit_logs",
			[]string{"id", "request_id", "customer_id", "conversation_id", "action_type", "target_type", "target_id",
				"before_json", "after_json", "actor_type", "actor_id", "result", "reason", "created_at"},
			mapRows(d.audits, func(r auditRow) []any {
				return []any{r.ID, strOrNil(r.RequestID), strOrNil(r.CustomerID), strOrNil(r.ConvID), r.ActionType,
					r.TargetType, r.TargetID, strOrNil(r.Before), strOrNil(r.After), r.ActorType, strOrNil(r.ActorID),
					r.Result, strOrNil(r.Reason), ts(r.CreatedAt)}
			})},
		{"service_tickets",
			[]string{"id", "ticket_no", "customer_id", "conversation_id", "order_id", "ticket_type", "status",
				"priority", "category", "summary", "details", "assigned_to", "created_at", "updated_at", "closed_at"},
			mapRows(d.tickets, func(r ticketRow) []any {
				return []any{r.ID, r.No, r.CustomerID, strOrNil(r.ConvID), strOrNil(r.OrderID), r.Type, r.Status,
					r.Priority, strOrNil(r.Category), r.Summary, strOrNil(r.Details), strOrNil(r.AssignedTo),
					ts(r.CreatedAt), ts(r.UpdatedAt), tsPtr(r.ClosedAt)}
			})},
		{"idempotency_keys",
			[]string{"idempotency_key", "operation", "request_hash", "response_json", "created_at", "expires_at"},
			mapRows(d.idempotency, func(r idempotencyRow) []any {
				return []any{r.Key, r.Operation, r.RequestHash, strOrNil(r.Response), ts(r.CreatedAt), ts(r.ExpiresAt)}
			})},
	}
}

func mapRows[T any](in []T, f func(T) []any) [][]any {
	out := make([][]any, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func ptrAny[T any](v *T) any {
	if v == nil {
		return nil
	}
	return *v
}

func ptrString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
