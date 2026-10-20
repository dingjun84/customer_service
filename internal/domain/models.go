package domain

import "time"

// Customer is an end user of the store.
type Customer struct {
	ID          string    `json:"id"`
	CustomerNo  string    `json:"customer_no"`
	DisplayName string    `json:"display_name,omitempty"`
	PhoneMasked string    `json:"phone_masked,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Product is a sellable catalog entry. Internal cost fields are intentionally
// absent from the schema so they can never leak through a DTO.
type Product struct {
	ID             string         `json:"id"`
	ProductCode    string         `json:"product_code"`
	Name           string         `json:"name"`
	Brand          string         `json:"brand,omitempty"`
	Category       string         `json:"category,omitempty"`
	Description    string         `json:"description,omitempty"`
	Status         string         `json:"status"`
	AttributesJSON string         `json:"-"`
	Attributes     map[string]any `json:"attributes,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	SKUs           []ProductSKU   `json:"skus,omitempty"`
}

// ProductSKU is a concrete variant of a product.
type ProductSKU struct {
	ID                 string         `json:"id"`
	ProductID          string         `json:"product_id"`
	SKUCode            string         `json:"sku_code"`
	SKUName            string         `json:"sku_name"`
	SpecificationsJSON string         `json:"-"`
	Specifications     map[string]any `json:"specifications,omitempty"`
	ListPriceMinor     int64          `json:"list_price_minor"`
	StockQuantity      int64          `json:"stock_quantity"`
	Status             string         `json:"status"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// Order is the order header plus its shipping snapshot.
type Order struct {
	ID                   string     `json:"id"`
	OrderNo              string     `json:"order_no"`
	CustomerID           string     `json:"customer_id"`
	OrderStatus          string     `json:"order_status"`
	OrderChannel         string     `json:"order_channel"`
	Currency             string     `json:"currency"`
	ItemAmountMinor      int64      `json:"item_amount_minor"`
	DiscountAmountMinor  int64      `json:"discount_amount_minor"`
	ShippingFeeMinor     int64      `json:"shipping_fee_minor"`
	PaidAmountMinor      int64      `json:"paid_amount_minor"`
	RefundedAmountMinor  int64      `json:"refunded_amount_minor"`
	IsPresale            bool       `json:"is_presale"`
	ReceiverName         string     `json:"receiver_name,omitempty"`
	ReceiverPhoneMasked  string     `json:"receiver_phone_masked,omitempty"`
	ShippingProvince     string     `json:"shipping_province,omitempty"`
	ShippingCity         string     `json:"shipping_city,omitempty"`
	ShippingDistrict     string     `json:"shipping_district,omitempty"`
	ShippingAddress      string     `json:"shipping_address,omitempty"`
	ShippingPostalCode   string     `json:"shipping_postal_code,omitempty"`
	ShippingMethod       string     `json:"shipping_method"`
	RequestedDeliveryAt  string     `json:"requested_delivery_at,omitempty"`
	InvoiceTypeRequested string     `json:"invoice_type_requested,omitempty"`
	OrderRemark          string     `json:"order_remark,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	PaidAt               *time.Time `json:"paid_at,omitempty"`
	ShippedAt            *time.Time `json:"shipped_at,omitempty"`
	DeliveredAt          *time.Time `json:"delivered_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	CancelledAt          *time.Time `json:"cancelled_at,omitempty"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Version              int        `json:"version"`

	Items []OrderItem `json:"items,omitempty"`
}

// OrderItem is an immutable product snapshot inside an order.
type OrderItem struct {
	ID                  string         `json:"id"`
	OrderID             string         `json:"order_id"`
	ProductID           string         `json:"product_id,omitempty"`
	SKUID               string         `json:"sku_id,omitempty"`
	ProductCodeSnapshot string         `json:"product_code_snapshot,omitempty"`
	ProductNameSnapshot string         `json:"product_name_snapshot"`
	SKUCodeSnapshot     string         `json:"sku_code_snapshot,omitempty"`
	SKUNameSnapshot     string         `json:"sku_name_snapshot,omitempty"`
	SKUAttributesJSON   string         `json:"-"`
	SKUAttributes       map[string]any `json:"sku_attributes,omitempty"`
	Quantity            int64          `json:"quantity"`
	RefundedQuantity    int64          `json:"refunded_quantity"`
	UnitPriceMinor      int64          `json:"unit_price_minor"`
	DiscountAmountMinor int64          `json:"discount_amount_minor"`
	PaidAmountMinor     int64          `json:"paid_amount_minor"`
	ItemStatus          string         `json:"item_status"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// Shipment is one parcel of an order.
type Shipment struct {
	ID                  string          `json:"id"`
	ShipmentNo          string          `json:"shipment_no"`
	OrderID             string          `json:"order_id"`
	CarrierCode         string          `json:"carrier_code,omitempty"`
	CarrierName         string          `json:"carrier_name,omitempty"`
	TrackingNo          string          `json:"tracking_no,omitempty"`
	ShipmentStatus      string          `json:"shipment_status"`
	EstimatedDeliveryAt *time.Time      `json:"estimated_delivery_at,omitempty"`
	LastEventAt         *time.Time      `json:"last_event_at,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	Events              []ShipmentEvent `json:"events,omitempty"`
}

// ShipmentEvent is a tracking node of a parcel.
type ShipmentEvent struct {
	ID          string    `json:"id"`
	ShipmentID  string    `json:"shipment_id"`
	EventTime   time.Time `json:"event_time"`
	EventCode   string    `json:"event_code,omitempty"`
	Description string    `json:"description"`
	Location    string    `json:"location,omitempty"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
}

// Payment is a payment attempt/transaction. No credentials are stored.
type Payment struct {
	ID                     string     `json:"id"`
	PaymentNo              string     `json:"payment_no"`
	OrderID                string     `json:"order_id"`
	PaymentMethod          string     `json:"payment_method"`
	PaymentStatus          string     `json:"payment_status"`
	AmountMinor            int64      `json:"amount_minor"`
	RefundedAmountMinor    int64      `json:"refunded_amount_minor"`
	ProviderTransactionRef string     `json:"-"`
	PaidAt                 *time.Time `json:"paid_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// Refund is a refund record. "Refund succeeded" in chat is not a fact; this row is.
type Refund struct {
	ID            string     `json:"id"`
	RefundNo      string     `json:"refund_no"`
	OrderID       string     `json:"order_id"`
	PaymentID     string     `json:"payment_id,omitempty"`
	ServiceCaseID string     `json:"service_case_id,omitempty"`
	RefundStatus  string     `json:"refund_status"`
	RefundChannel string     `json:"refund_channel,omitempty"`
	AmountMinor   int64      `json:"amount_minor"`
	Reason        string     `json:"reason,omitempty"`
	RequestedAt   time.Time  `json:"requested_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	FailureReason string     `json:"failure_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ServiceCase is a return/exchange/repair/refund/reship/price-protection request.
type ServiceCase struct {
	ID                string     `json:"id"`
	CaseNo            string     `json:"case_no"`
	CustomerID        string     `json:"customer_id"`
	OrderID           string     `json:"order_id"`
	OrderItemID       string     `json:"order_item_id,omitempty"`
	CaseType          string     `json:"case_type"`
	CaseStatus        string     `json:"case_status"`
	ReasonCode        string     `json:"reason_code,omitempty"`
	ReasonText        string     `json:"reason_text,omitempty"`
	Description       string     `json:"description,omitempty"`
	RequestedQuantity int64      `json:"requested_quantity"`
	Resolution        string     `json:"resolution,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
}

// Invoice is an invoice request/issue record.
type Invoice struct {
	ID            string     `json:"id"`
	InvoiceNo     string     `json:"invoice_no,omitempty"`
	CustomerID    string     `json:"customer_id"`
	OrderID       string     `json:"order_id"`
	InvoiceType   string     `json:"invoice_type"`
	InvoiceStatus string     `json:"invoice_status"`
	Title         string     `json:"title"`
	TaxNo         string     `json:"tax_no,omitempty"`
	AmountMinor   int64      `json:"amount_minor"`
	Email         string     `json:"-"`
	FileURL       string     `json:"file_url,omitempty"`
	RequestedAt   time.Time  `json:"requested_at"`
	IssuedAt      *time.Time `json:"issued_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Promotion describes an active campaign.
type Promotion struct {
	ID            string         `json:"id"`
	PromotionCode string         `json:"promotion_code"`
	Title         string         `json:"title"`
	Description   string         `json:"description,omitempty"`
	PromotionType string         `json:"promotion_type"`
	Status        string         `json:"status"`
	StartsAt      *time.Time     `json:"starts_at,omitempty"`
	EndsAt        *time.Time     `json:"ends_at,omitempty"`
	RulesJSON     string         `json:"-"`
	Rules         map[string]any `json:"rules,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// CustomerBenefit is a coupon/points/freight-insurance/whitebar mock status.
type CustomerBenefit struct {
	ID             string         `json:"id"`
	CustomerID     string         `json:"customer_id"`
	BenefitType    string         `json:"benefit_type"`
	BenefitCode    string         `json:"benefit_code,omitempty"`
	Status         string         `json:"status"`
	AmountMinor    *int64         `json:"amount_minor,omitempty"`
	Points         *int64         `json:"points,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	AttributesJSON string         `json:"-"`
	Attributes     map[string]any `json:"attributes,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// KnowledgeArticle is a policy article used for policy explanations only.
type KnowledgeArticle struct {
	ID           string    `json:"id"`
	ArticleCode  string    `json:"article_code"`
	Category     string    `json:"category"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	KeywordsJSON string    `json:"-"`
	Keywords     []string  `json:"keywords,omitempty"`
	Status       string    `json:"status"`
	Source       string    `json:"source,omitempty"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Conversation is a chat session owned by one customer.
type Conversation struct {
	ID                  string     `json:"id"`
	ConversationNo      string     `json:"conversation_no"`
	CustomerID          string     `json:"customer_id"`
	Status              string     `json:"status"`
	Channel             string     `json:"channel"`
	ActiveTaskStateJSON string     `json:"-"`
	Summary             string     `json:"summary,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	LastMessageAt       *time.Time `json:"last_message_at,omitempty"`
}

// Message is a single chat/tool message.
type Message struct {
	ID                string    `json:"id"`
	ConversationID    string    `json:"conversation_id"`
	Role              string    `json:"role"`
	Content           string    `json:"content"`
	ToolName          string    `json:"tool_name,omitempty"`
	ToolCallID        string    `json:"tool_call_id,omitempty"`
	ToolArgumentsJSON string    `json:"-"`
	ToolResultJSON    string    `json:"-"`
	MetadataJSON      string    `json:"-"`
	CreatedAt         time.Time `json:"created_at"`
}

// ToolExecution is an audit row for one tool call.
type ToolExecution struct {
	ID               string     `json:"id"`
	ConversationID   string     `json:"conversation_id"`
	MessageID        string     `json:"message_id,omitempty"`
	ToolCallID       string     `json:"tool_call_id"`
	ToolName         string     `json:"tool_name"`
	InputJSON        string     `json:"input_json"`
	OutputJSON       string     `json:"output_json,omitempty"`
	Status           string     `json:"status"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessageSafe string     `json:"error_message_safe,omitempty"`
	LatencyMS        *int64     `json:"latency_ms,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

// PendingAction is a write proposal awaiting explicit customer confirmation.
type PendingAction struct {
	ID             string     `json:"id"`
	ActionNo       string     `json:"action_no"`
	ConversationID string     `json:"conversation_id"`
	CustomerID     string     `json:"customer_id"`
	ActionType     string     `json:"action_type"`
	TargetType     string     `json:"target_type"`
	TargetID       string     `json:"target_id"`
	PayloadJSON    string     `json:"-"`
	PreviewJSON    string     `json:"-"`
	Status         string     `json:"status"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
	ExecutedAt     *time.Time `json:"executed_at,omitempty"`
	ResultJSON     string     `json:"-"`
}

// ActionAuditLog records important business mutations.
type ActionAuditLog struct {
	ID             string    `json:"id"`
	RequestID      string    `json:"request_id,omitempty"`
	CustomerID     string    `json:"customer_id,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
	ActionType     string    `json:"action_type"`
	TargetType     string    `json:"target_type"`
	TargetID       string    `json:"target_id"`
	BeforeJSON     string    `json:"before_json,omitempty"`
	AfterJSON      string    `json:"after_json,omitempty"`
	ActorType      string    `json:"actor_type"`
	ActorID        string    `json:"actor_id,omitempty"`
	Result         string    `json:"result"`
	Reason         string    `json:"reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ServiceTicket is a human-transfer/complaint/quality ticket.
type ServiceTicket struct {
	ID             string     `json:"id"`
	TicketNo       string     `json:"ticket_no"`
	CustomerID     string     `json:"customer_id"`
	ConversationID string     `json:"conversation_id,omitempty"`
	OrderID        string     `json:"order_id,omitempty"`
	TicketType     string     `json:"ticket_type"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	Category       string     `json:"category,omitempty"`
	Summary        string     `json:"summary"`
	Details        string     `json:"details,omitempty"`
	AssignedTo     string     `json:"assigned_to,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ClosedAt       *time.Time `json:"closed_at,omitempty"`
}

// IdempotencyRecord stores a request fingerprint and the response to replay.
type IdempotencyRecord struct {
	Key          string    `json:"key"`
	Operation    string    `json:"operation"`
	RequestHash  string    `json:"request_hash"`
	ResponseJSON string    `json:"response_json,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}
