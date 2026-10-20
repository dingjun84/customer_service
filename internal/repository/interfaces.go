// Package repository declares the persistence contracts used by the service
// layer. Implementations live in repository/sqlite.
//
// Repositories must:
//   - use parameterised SQL only (never string-concatenated values);
//   - translate driver errors into domain.ErrNotFound / domain.ErrConflict /
//     domain.ErrVersionConflict;
//   - never decide business permissions (that is the service layer's job).
package repository

import (
	"context"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// Cursor is an opaque keyset-pagination position (created_at + id).
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// OrderQuery filters the current customer's order list.
type OrderQuery struct {
	CustomerID string
	Status     string
	Limit      int
	Cursor     *Cursor
}

// MessageQuery filters a conversation's message history.
type MessageQuery struct {
	ConversationID string
	Limit          int
	Before         *Cursor
}

// Store is the single persistence port consumed by services. WithTx hands out a
// Store bound to the same transaction so writes stay atomic.
type Store interface {
	Ping(ctx context.Context) error
	WithTx(ctx context.Context, fn func(Store) error) error

	// Customers.
	GetCustomer(ctx context.Context, id string) (*domain.Customer, error)
	GetCustomerByNo(ctx context.Context, customerNo string) (*domain.Customer, error)

	// Catalog.
	SearchProducts(ctx context.Context, query, category string, limit int) ([]domain.Product, error)
	GetProduct(ctx context.Context, id string) (*domain.Product, error)
	GetProductByCode(ctx context.Context, code string) (*domain.Product, error)
	ListSKUs(ctx context.Context, productID string) ([]domain.ProductSKU, error)

	// Orders.
	ListOrders(ctx context.Context, q OrderQuery) ([]domain.Order, error)
	GetOrderByNo(ctx context.Context, orderNo string) (*domain.Order, error)
	GetOrderByID(ctx context.Context, id string) (*domain.Order, error)
	ListOrderItems(ctx context.Context, orderID string) ([]domain.OrderItem, error)
	GetOrderItem(ctx context.Context, itemID string) (*domain.OrderItem, error)
	// UpdateOrderFields applies a whitelisted column/value map with optimistic
	// locking on orders.version.
	UpdateOrderFields(ctx context.Context, orderID string, fields map[string]any, expectedVersion int, now time.Time) error
	CancelOrder(ctx context.Context, orderID string, expectedVersion int, now time.Time) error
	// CountActiveServiceCases counts non-terminal cases of a type for an order.
	CountActiveServiceCases(ctx context.Context, orderID, caseType string) (int, error)
	// SumOpenRequestedQuantity sums requested quantity of open cases for an item.
	SumOpenRequestedQuantity(ctx context.Context, orderItemID string) (int, error)

	// Logistics.
	ListShipments(ctx context.Context, orderID string) ([]domain.Shipment, error)
	ListShipmentEvents(ctx context.Context, shipmentID string) ([]domain.ShipmentEvent, error)
	GetShipmentByTrackingNo(ctx context.Context, trackingNo string) (*domain.Shipment, error)

	// Payments and refunds.
	ListPayments(ctx context.Context, orderID string) ([]domain.Payment, error)
	ListRefunds(ctx context.Context, orderID string) ([]domain.Refund, error)
	GetRefundByNo(ctx context.Context, refundNo string) (*domain.Refund, error)
	CreateRefund(ctx context.Context, refund *domain.Refund) error

	// After-sales cases.
	ListServiceCasesByOrder(ctx context.Context, orderID string) ([]domain.ServiceCase, error)
	GetServiceCaseByNo(ctx context.Context, caseNo string) (*domain.ServiceCase, error)
	CreateServiceCase(ctx context.Context, c *domain.ServiceCase) error

	// Invoices.
	ListInvoicesByOrder(ctx context.Context, orderID string) ([]domain.Invoice, error)

	// Promotions and benefits.
	ListPromotions(ctx context.Context, query, promotionCode string, limit int) ([]domain.Promotion, error)
	ListBenefits(ctx context.Context, customerID, benefitType string, limit int) ([]domain.CustomerBenefit, error)

	// Knowledge base (keyword search only; no embeddings in the MVP).
	SearchKnowledge(ctx context.Context, query, category string, limit int) ([]domain.KnowledgeArticle, error)

	// Conversations.
	CreateConversation(ctx context.Context, c *domain.Conversation) error
	GetConversation(ctx context.Context, id string) (*domain.Conversation, error)
	ListConversations(ctx context.Context, customerID string, limit int, cursor *Cursor) ([]domain.Conversation, error)
	SetConversationStatus(ctx context.Context, id, status string, now time.Time) error
	UpdateConversationState(ctx context.Context, id, summary, taskStateJSON string, now time.Time) error
	TouchConversation(ctx context.Context, id string, at time.Time) error

	// Messages.
	AppendMessage(ctx context.Context, m *domain.Message) error
	GetMessage(ctx context.Context, id string) (*domain.Message, error)
	ListMessages(ctx context.Context, q MessageQuery) ([]domain.Message, error)
	ListRecentMessages(ctx context.Context, conversationID string, limit int) ([]domain.Message, error)

	// Tool execution trace.
	CreateToolExecution(ctx context.Context, e *domain.ToolExecution) error
	FinishToolExecution(ctx context.Context, id, status, outputJSON, errorCode, errorMessageSafe string, latencyMS int64, finishedAt time.Time) error
	ListToolExecutions(ctx context.Context, conversationID string, limit int) ([]domain.ToolExecution, error)

	// Pending actions.
	CreatePendingAction(ctx context.Context, a *domain.PendingAction) error
	GetPendingAction(ctx context.Context, id string) (*domain.PendingAction, error)
	ListPendingActions(ctx context.Context, conversationID, status string, limit int) ([]domain.PendingAction, error)
	// ClaimPendingAction atomically moves pending -> confirmed. It returns false
	// when another request already claimed (or closed) the action.
	ClaimPendingAction(ctx context.Context, id string, now time.Time) (bool, error)
	// RejectPendingAction atomically moves pending -> rejected.
	RejectPendingAction(ctx context.Context, id string, now time.Time) (bool, error)
	FinishPendingAction(ctx context.Context, id, status, resultJSON string, executedAt *time.Time) error
	ExpirePendingActions(ctx context.Context, now time.Time) (int64, error)

	// Audit trail (append-only for normal tools).
	AppendAuditLog(ctx context.Context, log *domain.ActionAuditLog) error
	ListAuditLogs(ctx context.Context, conversationID string, limit int) ([]domain.ActionAuditLog, error)

	// Tickets.
	CreateTicket(ctx context.Context, t *domain.ServiceTicket) error

	// Idempotency.
	GetIdempotency(ctx context.Context, key string) (*domain.IdempotencyRecord, error)
	PutIdempotency(ctx context.Context, rec *domain.IdempotencyRecord) error
}
