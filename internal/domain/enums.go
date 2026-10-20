package domain

// Order statuses.
const (
	OrderPendingPayment = "pending_payment"
	OrderPaid           = "paid"
	OrderProcessing     = "processing"
	OrderShipped        = "shipped"
	OrderCompleted      = "completed"
	OrderCancelled      = "cancelled"
	OrderClosed         = "closed"
)

// Shipment statuses.
const (
	ShipmentPending        = "pending"
	ShipmentPickedUp       = "picked_up"
	ShipmentInTransit      = "in_transit"
	ShipmentDelivering     = "delivering"
	ShipmentDelivered      = "delivered"
	ShipmentException      = "exception"
	ShipmentReturned       = "returned"
	ShipmentIntercepted    = "intercepted"
	ShipmentDeliveryFailed = "delivery_failed"
)

// Refund statuses.
const (
	RefundRequested     = "requested"
	RefundPendingReview = "pending_review"
	RefundApproved      = "approved"
	RefundProcessing    = "processing"
	RefundSucceeded     = "succeeded"
	RefundFailed        = "failed"
	RefundRejected      = "rejected"
	RefundCancelled     = "cancelled"
)

// Service case statuses.
const (
	CaseSubmitted          = "submitted"
	CasePendingReview      = "pending_review"
	CaseApproved           = "approved"
	CaseRejected           = "rejected"
	CaseAwaitingCustomer   = "awaiting_customer"
	CaseAwaitingReturnShip = "awaiting_return_shipment"
	CaseProcessing         = "processing"
	CaseCompleted          = "completed"
	CaseCancelled          = "cancelled"
)

// Service case types (also used as action_type / case_type values).
const (
	CaseTypeReturn          = "return"
	CaseTypeExchange        = "exchange"
	CaseTypeRepair          = "repair"
	CaseTypeRefund          = "refund"
	CaseTypeReship          = "reship"
	CaseTypePriceProtection = "price_protection"
)

// Pending action types, matching the pending_actions CHECK constraint.
const (
	ActionCancelOrder          = "cancel_order"
	ActionModifyOrder          = "modify_order"
	ActionSubmitServiceRequest = "submit_service_request"
)

// Pending action statuses.
const (
	ActionPending   = "pending"
	ActionConfirmed = "confirmed"
	ActionRejected  = "rejected"
	ActionExpired   = "expired"
	ActionExecuted  = "executed"
	ActionFailed    = "failed"
)

// Conversation statuses.
const (
	ConversationOpen        = "open"
	ConversationClosed      = "closed"
	ConversationTransferred = "transferred"
)

// Message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Tool execution statuses.
const (
	ToolStarted   = "started"
	ToolSucceeded = "succeeded"
	ToolFailed    = "failed"
	ToolRejected  = "rejected"
)

// Actor types for audit logs.
const (
	ActorAssistant  = "assistant"
	ActorCustomer   = "customer"
	ActorHumanAgent = "human_agent"
	ActorSystem     = "system"
	ActorAdmin      = "admin"
)

// Ticket types and priorities.
const (
	TicketHumanTransfer = "human_transfer"
	TicketComplaint     = "complaint"
	TicketQualityIssue  = "quality_issue"
	TicketOther         = "other"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// Invoice statuses.
const (
	InvoiceRequested  = "requested"
	InvoiceProcessing = "processing"
	InvoiceIssued     = "issued"
	InvoiceRejected   = "rejected"
	InvoiceVoided     = "voided"
	InvoiceRedFlushed = "red_flushed"
)

// ModifiableOrderFields is the whitelist of order columns the assistant may
// propose changing. Any other field must be rejected before reaching SQL.
var ModifiableOrderFields = []string{
	"receiver_name",
	"receiver_phone",
	"shipping_province",
	"shipping_city",
	"shipping_district",
	"shipping_address",
	"shipping_postal_code",
	"shipping_method",
	"requested_delivery_at",
	"order_remark",
}

// IsModifiableOrderField reports whether field is inside the write whitelist.
func IsModifiableOrderField(field string) bool {
	for _, f := range ModifiableOrderFields {
		if f == field {
			return true
		}
	}
	return false
}

// SupportedActionTypes are the action_type values accepted by the eligibility
// checker and by the write-proposal tools.
var SupportedActionTypes = []string{
	ActionCancelOrder,
	ActionModifyOrder,
	CaseTypeReturn,
	CaseTypeExchange,
	CaseTypeRepair,
	CaseTypeRefund,
	CaseTypeReship,
	CaseTypePriceProtection,
}

// IsSupportedActionType reports whether the action type is implemented.
func IsSupportedActionType(actionType string) bool {
	for _, a := range SupportedActionTypes {
		if a == actionType {
			return true
		}
	}
	return false
}

// IsServiceCaseType reports whether caseType maps to a service_cases.case_type.
func IsServiceCaseType(caseType string) bool {
	switch caseType {
	case CaseTypeReturn, CaseTypeExchange, CaseTypeRepair, CaseTypeRefund, CaseTypeReship, CaseTypePriceProtection:
		return true
	default:
		return false
	}
}

// IsUnshippedOrderStatus reports whether the order has not left the warehouse.
func IsUnshippedOrderStatus(status string) bool {
	switch status {
	case OrderPendingPayment, OrderPaid, OrderProcessing:
		return true
	default:
		return false
	}
}
