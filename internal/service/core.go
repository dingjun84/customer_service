// Package service implements the application services: query composition,
// eligibility rules, write transactions and audit.
//
// Services never call the LLM and never read HTTP requests. They receive the
// already authenticated customer ID and enforce resource ownership themselves,
// so every entry point (HTTP API or tool registry) shares the same guarantees.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
)

// Options tunes business rules that must not be guessed by the model.
type Options struct {
	PendingActionTTL      time.Duration
	AfterSalesWindow      time.Duration
	PriceProtectionWindow time.Duration
	// Now is injectable so tests can control expiry and windows.
	Now func() time.Time
}

// core holds the shared dependencies of every application service.
type core struct {
	store                 repository.Store
	pendingActionTTL      time.Duration
	afterSalesWindow      time.Duration
	priceProtectionWindow time.Duration
	now                   func() time.Time
}

func (c core) nowUTC() time.Time {
	t := c.now().UTC()
	return t.Truncate(time.Second)
}

// Services is the aggregate handed to the HTTP layer and the tool registry.
type Services struct {
	store repository.Store

	Catalog     *Catalog
	Orders      *Orders
	Logistics   *Logistics
	Refunds     *Refunds
	AfterSales  *AfterSales
	Invoices    *Invoices
	Promotions  *Promotions
	Knowledge   *Knowledge
	Eligibility *Eligibility
	Actions     *Actions
	Tickets     *Tickets
}

// New wires every application service around one store.
func New(store repository.Store, opts Options) *Services {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.PendingActionTTL <= 0 {
		opts.PendingActionTTL = 10 * time.Minute
	}
	if opts.AfterSalesWindow <= 0 {
		opts.AfterSalesWindow = 7 * 24 * time.Hour
	}
	if opts.PriceProtectionWindow <= 0 {
		opts.PriceProtectionWindow = 15 * 24 * time.Hour
	}
	c := core{
		store:                 store,
		pendingActionTTL:      opts.PendingActionTTL,
		afterSalesWindow:      opts.AfterSalesWindow,
		priceProtectionWindow: opts.PriceProtectionWindow,
		now:                   opts.Now,
	}
	elig := &Eligibility{core: c}
	return &Services{
		store:       store,
		Catalog:     &Catalog{core: c},
		Orders:      &Orders{core: c},
		Logistics:   &Logistics{core: c},
		Refunds:     &Refunds{core: c},
		AfterSales:  &AfterSales{core: c},
		Invoices:    &Invoices{core: c},
		Promotions:  &Promotions{core: c},
		Knowledge:   &Knowledge{core: c},
		Eligibility: elig,
		Actions:     &Actions{core: c, elig: elig},
		Tickets:     &Tickets{core: c},
	}
}

// Store exposes the persistence port for conversation-level orchestration.
func (s *Services) Store() repository.Store { return s.store }

// ---- ownership helpers ----

// ownedOrder loads an order and enforces that it belongs to customerID.
// A foreign order is reported as forbidden, never as "not found with details".
func (c core) ownedOrder(ctx context.Context, customerID, orderNo string) (*domain.Order, error) {
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	if orderNo == "" {
		return nil, domain.ValidationError("order_no 不能为空")
	}
	order, err := c.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该订单，请确认订单号是否正确")
		}
		return nil, domain.AsAppError(err)
	}
	if order.CustomerID != customerID {
		return nil, domain.ForbiddenError("该订单不属于当前客户")
	}
	return order, nil
}

// ownedOrderByID is ownedOrder for internal IDs (pending action targets).
func (c core) ownedOrderByID(ctx context.Context, customerID, orderID string) (*domain.Order, error) {
	order, err := c.store.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该订单")
		}
		return nil, domain.AsAppError(err)
	}
	if order.CustomerID != customerID {
		return nil, domain.ForbiddenError("该订单不属于当前客户")
	}
	return order, nil
}

// ownedConversation loads a conversation and enforces ownership; actions are only
// accepted while the conversation is open.
func (c core) ownedConversation(ctx context.Context, customerID, conversationID string) (*domain.Conversation, error) {
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	if conversationID == "" {
		return nil, domain.ValidationError("conversation_id 不能为空")
	}
	conv, err := c.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该会话")
		}
		return nil, domain.AsAppError(err)
	}
	if conv.CustomerID != customerID {
		return nil, domain.ForbiddenError("该会话不属于当前客户")
	}
	return conv, nil
}

func (c core) ownedCase(ctx context.Context, customerID, caseNo string) (*domain.ServiceCase, error) {
	cs, err := c.store.GetServiceCaseByNo(ctx, caseNo)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该售后单")
		}
		return nil, domain.AsAppError(err)
	}
	if cs.CustomerID != customerID {
		return nil, domain.ForbiddenError("该售后单不属于当前客户")
	}
	return cs, nil
}

// ---- audit helpers ----

func (c core) audit(ctx context.Context, log *domain.ActionAuditLog) {
	if log.ID == "" {
		log.ID = domain.NewID()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = c.nowUTC()
	}
	if log.TargetType == "" {
		log.TargetType = "order"
	}
	// Audit failures must never mask the business result: log and continue.
	_ = c.store.AppendAuditLog(ctx, log)
}

func toJSON(v any) string {
	if v == nil {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func fromJSON(raw string, dest any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), dest)
}
