package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
)

// ProposalRequest is the input of every write-proposal tool.
type ProposalRequest struct {
	CustomerID     string
	ConversationID string
	RequestID      string
	ActionType     string
	OrderNo        string
	OrderItemID    string
	Quantity       int
	Changes        map[string]any
	Reason         string
	Description    string
}

// Proposal is the result of a write proposal: either a pending action awaiting
// confirmation, or a structured explanation of why it is not allowed.
type Proposal struct {
	Action      *domain.PendingAction `json:"action,omitempty"`
	Eligibility EligibilityResult     `json:"eligibility"`
	Summary     string                `json:"summary,omitempty"`
	ExpiresAt   string                `json:"expires_at,omitempty"`
}

// ActionResult reports the real outcome of a confirm/reject call.
type ActionResult struct {
	ActionID   string         `json:"action_id"`
	ActionNo   string         `json:"action_no"`
	ActionType string         `json:"action_type"`
	Status     string         `json:"status"`
	Result     map[string]any `json:"result,omitempty"`
	Idempotent bool           `json:"idempotent"`
}

type actionPayload struct {
	ActionType           string         `json:"action_type"`
	OrderNo              string         `json:"order_no"`
	OrderID              string         `json:"order_id"`
	OrderVersion         int            `json:"order_version"`
	Changes              map[string]any `json:"changes,omitempty"`
	CaseType             string         `json:"case_type,omitempty"`
	OrderItemID          string         `json:"order_item_id,omitempty"`
	Quantity             int            `json:"quantity,omitempty"`
	Reason               string         `json:"reason,omitempty"`
	Description          string         `json:"description,omitempty"`
	EstimatedAmountMinor *int64         `json:"estimated_amount_minor,omitempty"`
}

// Actions owns the confirm-before-write state machine.
type Actions struct {
	core
	elig *Eligibility
}

// Propose validates eligibility and creates a pending action. It never mutates
// business data: the order, refund and case tables are only touched on confirm.
func (s *Actions) Propose(ctx context.Context, req ProposalRequest) (*Proposal, error) {
	if strings.TrimSpace(req.ActionType) == "" {
		return nil, domain.ValidationError("action_type 不能为空")
	}
	eligibility, err := s.elig.Check(ctx, EligibilityRequest{
		CustomerID:  req.CustomerID,
		ActionType:  req.ActionType,
		OrderNo:     req.OrderNo,
		OrderItemID: req.OrderItemID,
		Quantity:    req.Quantity,
		Changes:     req.Changes,
		Reason:      req.Reason,
	})
	if err != nil {
		return nil, err
	}
	if !eligibility.Eligible {
		return &Proposal{Eligibility: *eligibility}, nil
	}
	conv, err := s.ownedConversation(ctx, req.CustomerID, req.ConversationID)
	if err != nil {
		return nil, err
	}
	if conv.Status == domain.ConversationClosed {
		return nil, domain.ConflictError("会话已关闭，无法发起新的操作")
	}

	order := eligibility.order
	now := s.nowUTC()

	// Service-case actions are stored as one action type with the concrete
	// case_type in the payload, matching the pending_actions CHECK constraint.
	storedActionType := req.ActionType
	if domain.IsServiceCaseType(req.ActionType) {
		storedActionType = domain.ActionSubmitServiceRequest
	}

	quantity := req.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	payload := actionPayload{
		ActionType:           req.ActionType,
		OrderNo:              order.OrderNo,
		OrderID:              order.ID,
		OrderVersion:         order.Version,
		Changes:              req.Changes,
		Quantity:             quantity,
		Reason:               req.Reason,
		Description:          req.Description,
		EstimatedAmountMinor: eligibility.EstimatedAmountMinor,
	}
	if domain.IsServiceCaseType(req.ActionType) {
		payload.CaseType = req.ActionType
	}
	payload.OrderItemID = eligibility.OrderItemID

	summary := buildSummary(req.ActionType, order.OrderNo, eligibility)
	preview := map[string]any{"summary": summary}
	for k, v := range eligibility.Preview {
		preview[k] = v
	}

	action := &domain.PendingAction{
		ID:             domain.NewID(),
		ActionNo:       domain.HumanNo("ACT", now),
		ConversationID: req.ConversationID,
		CustomerID:     req.CustomerID,
		ActionType:     storedActionType,
		TargetType:     "order",
		TargetID:       order.ID,
		PayloadJSON:    toJSON(payload),
		PreviewJSON:    toJSON(preview),
		Status:         domain.ActionPending,
		ExpiresAt:      now.Add(s.pendingActionTTL),
		CreatedAt:      now,
	}
	if err := s.store.CreatePendingAction(ctx, action); err != nil {
		return nil, domain.AsAppError(err)
	}
	s.setTaskState(ctx, req.ConversationID, action, "")

	return &Proposal{
		Action:      action,
		Eligibility: *eligibility,
		Summary:     summary,
		ExpiresAt:   action.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// Confirm executes a pending action after re-validating everything server-side.
// The pending -> confirmed transition is atomic, which makes double confirmation
// safe: only one caller executes, the others receive the stored result.
func (s *Actions) Confirm(ctx context.Context, customerID, conversationID, actionID, requestID string) (*ActionResult, error) {
	action, err := s.loadAction(ctx, customerID, conversationID, actionID)
	if err != nil {
		return nil, err
	}
	now := s.nowUTC()

	switch action.Status {
	case domain.ActionExecuted:
		return idempotentResult(action)
	case domain.ActionRejected:
		return nil, domain.ConflictError("该操作已被拒绝，无法执行")
	case domain.ActionExpired:
		return nil, domain.NewError(domain.CodeActionExpired, "该操作已过期，请重新发起")
	}
	if !action.ExpiresAt.After(now) {
		_ = s.store.FinishPendingAction(ctx, action.ID, domain.ActionExpired, "", nil)
		return nil, domain.NewError(domain.CodeActionExpired, "该操作已过期，请重新发起")
	}

	claimed, err := s.store.ClaimPendingAction(ctx, action.ID, now)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	if !claimed {
		// Lost the race: return whatever the winner produced.
		latest, err := s.loadAction(ctx, customerID, conversationID, actionID)
		if err != nil {
			return nil, err
		}
		switch latest.Status {
		case domain.ActionExecuted:
			return idempotentResult(latest)
		case domain.ActionRejected:
			return nil, domain.ConflictError("该操作已被拒绝，无法执行")
		case domain.ActionExpired:
			return nil, domain.NewError(domain.CodeActionExpired, "该操作已过期，请重新发起")
		case domain.ActionFailed:
			return nil, domain.NewError(domain.CodeConflict, "该操作执行失败，请重新发起")
		default:
			return nil, domain.ConflictError("该操作正在处理中，请稍后查询结果")
		}
	}

	var payload actionPayload
	if err := fromJSON(action.PayloadJSON, &payload); err != nil {
		s.finishFailed(ctx, action, requestID, "payload 解析失败")
		return nil, domain.NewError(domain.CodeInternal, "操作数据异常，请联系人工客服")
	}

	var result map[string]any
	txErr := s.store.WithTx(ctx, func(tx repository.Store) error {
		var execErr error
		switch action.ActionType {
		case domain.ActionCancelOrder:
			result, execErr = s.executeCancel(ctx, tx, customerID, action, payload, requestID)
		case domain.ActionModifyOrder:
			result, execErr = s.executeModify(ctx, tx, customerID, action, payload, requestID)
		case domain.ActionSubmitServiceRequest:
			result, execErr = s.executeServiceRequest(ctx, tx, customerID, action, payload, requestID)
		default:
			execErr = domain.ValidationError("不支持的操作类型")
		}
		if execErr != nil {
			return execErr
		}
		if err := tx.FinishPendingAction(ctx, action.ID, domain.ActionExecuted, toJSON(result), &now); err != nil {
			return domain.AsAppError(err)
		}
		return nil
	})
	if txErr != nil {
		s.finishFailed(ctx, action, requestID, domain.AsAppError(txErr).Message)
		if appErr := domain.AsAppError(txErr); appErr.Code != domain.CodeInternal {
			return nil, appErr
		}
		return nil, domain.NewError(domain.CodeInternal, "操作执行失败，订单未做变更，请稍后重试或转人工").Wrap(txErr)
	}

	s.setTaskState(ctx, action.ConversationID, nil, "")
	return &ActionResult{
		ActionID:   action.ID,
		ActionNo:   action.ActionNo,
		ActionType: action.ActionType,
		Status:     domain.ActionExecuted,
		Result:     result,
	}, nil
}

// Reject records an explicit customer rejection without touching business data.
func (s *Actions) Reject(ctx context.Context, customerID, conversationID, actionID string) (*ActionResult, error) {
	action, err := s.loadAction(ctx, customerID, conversationID, actionID)
	if err != nil {
		return nil, err
	}
	now := s.nowUTC()
	switch action.Status {
	case domain.ActionRejected:
		return &ActionResult{
			ActionID: action.ID, ActionNo: action.ActionNo, ActionType: action.ActionType,
			Status: domain.ActionRejected, Idempotent: true,
		}, nil
	case domain.ActionExecuted:
		return nil, domain.ConflictError("该操作已执行，无法拒绝")
	case domain.ActionExpired:
		return nil, domain.NewError(domain.CodeActionExpired, "该操作已过期")
	}

	ok, err := s.store.RejectPendingAction(ctx, action.ID, now)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	if !ok {
		return nil, domain.ConflictError("该操作状态已变化，请刷新后重试")
	}
	s.audit(ctx, &domain.ActionAuditLog{
		RequestID: "", CustomerID: customerID, ConversationID: conversationID,
		ActionType: action.ActionType, TargetType: "order", TargetID: action.TargetID,
		ActorType: domain.ActorCustomer, ActorID: customerID, Result: "rejected",
		Reason: "用户拒绝执行",
	})
	s.setTaskState(ctx, conversationID, nil, "")
	return &ActionResult{
		ActionID: action.ID, ActionNo: action.ActionNo, ActionType: action.ActionType,
		Status: domain.ActionRejected,
	}, nil
}

// ExpireStaleActions closes pending actions past their TTL.
func (s *Actions) ExpireStaleActions(ctx context.Context) (int64, error) {
	n, err := s.store.ExpirePendingActions(ctx, s.nowUTC())
	if err != nil {
		return 0, domain.AsAppError(err)
	}
	return n, nil
}

// ListPending returns the pending actions of a conversation.
func (s *Actions) ListPending(ctx context.Context, customerID, conversationID string) ([]domain.PendingAction, error) {
	if _, err := s.ownedConversation(ctx, customerID, conversationID); err != nil {
		return nil, err
	}
	actions, err := s.store.ListPendingActions(ctx, conversationID, domain.ActionPending, 20)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return actions, nil
}

// ---- execution ----

func (s *Actions) executeCancel(ctx context.Context, tx repository.Store, customerID string,
	action *domain.PendingAction, payload actionPayload, requestID string) (map[string]any, error) {

	order, err := s.reloadOrder(ctx, tx, customerID, payload.OrderID)
	if err != nil {
		return nil, err
	}
	eligibility, err := s.elig.Check(ctx, EligibilityRequest{
		CustomerID: customerID, ActionType: domain.ActionCancelOrder, OrderNo: order.OrderNo,
	})
	if err != nil {
		return nil, err
	}
	if !eligibility.Eligible {
		return nil, domain.ConflictError("当前状态不可取消：" + eligibility.Message)
	}

	before := map[string]any{"order_status": order.OrderStatus, "version": order.Version}
	now := s.nowUTC()
	if err := tx.CancelOrder(ctx, order.ID, order.Version, now); err != nil {
		if errors.Is(err, domain.ErrVersionConflict) {
			return nil, domain.ConflictError("订单状态已变化，取消未执行，请刷新后重试")
		}
		return nil, domain.AsAppError(err)
	}

	result := map[string]any{
		"order_no":     order.OrderNo,
		"order_status": domain.OrderCancelled,
		"cancelled_at": now.Format("2006-01-02T15:04:05Z07:00"),
	}

	// A paid unshipped order produces a refund request record (no real payout).
	if refundAmount := order.PaidAmountMinor - order.RefundedAmountMinor; refundAmount > 0 {
		payments, err := tx.ListPayments(ctx, order.ID)
		if err != nil {
			return nil, domain.AsAppError(err)
		}
		paymentID := ""
		for _, p := range payments {
			if p.PaymentStatus == "succeeded" || p.PaymentStatus == "partially_refunded" {
				paymentID = p.ID
				break
			}
		}
		refund := &domain.Refund{
			ID: domain.NewID(), RefundNo: domain.HumanNo("RF", now),
			OrderID: order.ID, PaymentID: paymentID,
			RefundStatus: domain.RefundRequested, RefundChannel: "原支付渠道",
			AmountMinor: refundAmount, Reason: "订单取消退款",
			RequestedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.CreateRefund(ctx, refund); err != nil {
			return nil, domain.AsAppError(err)
		}
		result["refund_no"] = refund.RefundNo
		result["refund_amount_minor"] = refundAmount
		result["refund_amount_display"] = domain.FormatAmount(refundAmount)
		result["refund_status"] = refund.RefundStatus
		result["refund_note"] = "模拟环境：退款申请已创建，不会真实打款"
	}

	after := map[string]any{"order_status": domain.OrderCancelled, "cancelled_at": result["cancelled_at"]}
	if err := tx.AppendAuditLog(ctx, &domain.ActionAuditLog{
		ID: domain.NewID(), RequestID: requestID, CustomerID: customerID, ConversationID: action.ConversationID,
		ActionType: domain.ActionCancelOrder, TargetType: "order", TargetID: order.ID,
		BeforeJSON: toJSON(before), AfterJSON: toJSON(after),
		ActorType: domain.ActorCustomer, ActorID: customerID, Result: "succeeded", Reason: payload.Reason,
		CreatedAt: now,
	}); err != nil {
		return nil, domain.AsAppError(err)
	}
	return result, nil
}

func (s *Actions) executeModify(ctx context.Context, tx repository.Store, customerID string,
	action *domain.PendingAction, payload actionPayload, requestID string) (map[string]any, error) {

	order, err := s.reloadOrder(ctx, tx, customerID, payload.OrderID)
	if err != nil {
		return nil, err
	}
	if payload.OrderVersion > 0 && order.Version != payload.OrderVersion {
		return nil, domain.ConflictError("订单信息已发生变化，请重新确认修改内容")
	}
	eligibility, err := s.elig.Check(ctx, EligibilityRequest{
		CustomerID: customerID, ActionType: domain.ActionModifyOrder,
		OrderNo: order.OrderNo, Changes: payload.Changes,
	})
	if err != nil {
		return nil, err
	}
	if !eligibility.Eligible {
		return nil, domain.ConflictError("当前状态不可修改：" + eligibility.Message)
	}

	fields := make(map[string]any, len(payload.Changes))
	before := map[string]any{}
	after := map[string]any{}
	for field, raw := range payload.Changes {
		value, ok := raw.(string)
		if !ok {
			return nil, domain.ValidationError("修改字段的值必须是字符串")
		}
		before[field] = currentOrderFieldValue(order, field)
		if field == "receiver_phone" {
			// Only the masked value is ever persisted; the repository maps this
			// field name onto the receiver_phone_masked column.
			masked := domain.MaskPhone(value)
			fields[field] = masked
			after[field] = masked
			continue
		}
		fields[field] = value
		after[field] = value
	}

	now := s.nowUTC()
	if err := tx.UpdateOrderFields(ctx, order.ID, fields, order.Version, now); err != nil {
		if errors.Is(err, domain.ErrVersionConflict) {
			return nil, domain.ConflictError("订单已被其他操作修改，请刷新后重新确认")
		}
		return nil, domain.AsAppError(err)
	}

	result := map[string]any{
		"order_no":    order.OrderNo,
		"changes":     after,
		"new_version": order.Version + 1,
	}
	if err := tx.AppendAuditLog(ctx, &domain.ActionAuditLog{
		ID: domain.NewID(), RequestID: requestID, CustomerID: customerID, ConversationID: action.ConversationID,
		ActionType: domain.ActionModifyOrder, TargetType: "order", TargetID: order.ID,
		BeforeJSON: toJSON(before), AfterJSON: toJSON(after),
		ActorType: domain.ActorCustomer, ActorID: customerID, Result: "succeeded", Reason: payload.Reason,
		CreatedAt: now,
	}); err != nil {
		return nil, domain.AsAppError(err)
	}
	return result, nil
}

func (s *Actions) executeServiceRequest(ctx context.Context, tx repository.Store, customerID string,
	action *domain.PendingAction, payload actionPayload, requestID string) (map[string]any, error) {

	order, err := s.reloadOrder(ctx, tx, customerID, payload.OrderID)
	if err != nil {
		return nil, err
	}
	caseType := payload.CaseType
	if caseType == "" {
		caseType = payload.ActionType
	}
	if !domain.IsServiceCaseType(caseType) {
		return nil, domain.ValidationError("不支持的售后类型")
	}

	eligibility, err := s.elig.Check(ctx, EligibilityRequest{
		CustomerID: customerID, ActionType: caseType, OrderNo: order.OrderNo,
		OrderItemID: payload.OrderItemID, Quantity: payload.Quantity,
	})
	if err != nil {
		return nil, err
	}
	if !eligibility.Eligible {
		return nil, domain.ConflictError("当前状态不可申请：" + eligibility.Message)
	}

	now := s.nowUTC()
	caseStatus := domain.CaseSubmitted
	if caseType == domain.CaseTypeRefund {
		caseStatus = domain.CasePendingReview
	}
	cs := &domain.ServiceCase{
		ID: domain.NewID(), CaseNo: domain.HumanNo("CASE", now),
		CustomerID: customerID, OrderID: order.ID,
		OrderItemID: eligibility.OrderItemID,
		CaseType:    caseType, CaseStatus: caseStatus,
		ReasonCode: payload.Reason, ReasonText: payload.Reason, Description: payload.Description,
		RequestedQuantity: int64(payload.Quantity),
		CreatedAt:         now, UpdatedAt: now,
	}
	if err := tx.CreateServiceCase(ctx, cs); err != nil {
		return nil, domain.AsAppError(err)
	}

	result := map[string]any{
		"order_no":    order.OrderNo,
		"case_no":     cs.CaseNo,
		"case_type":   cs.CaseType,
		"case_status": cs.CaseStatus,
		"quantity":    cs.RequestedQuantity,
	}

	if caseType == domain.CaseTypeRefund {
		amount := int64(0)
		if eligibility.EstimatedAmountMinor != nil {
			amount = *eligibility.EstimatedAmountMinor
		}
		payments, err := tx.ListPayments(ctx, order.ID)
		if err != nil {
			return nil, domain.AsAppError(err)
		}
		paymentID := ""
		for _, p := range payments {
			if p.PaymentStatus == "succeeded" || p.PaymentStatus == "partially_refunded" {
				paymentID = p.ID
				break
			}
		}
		refund := &domain.Refund{
			ID: domain.NewID(), RefundNo: domain.HumanNo("RF", now),
			OrderID: order.ID, PaymentID: paymentID, ServiceCaseID: cs.ID,
			RefundStatus: domain.RefundRequested, RefundChannel: "原支付渠道",
			AmountMinor: amount, Reason: payload.Reason,
			RequestedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.CreateRefund(ctx, refund); err != nil {
			return nil, domain.AsAppError(err)
		}
		result["refund_no"] = refund.RefundNo
		result["refund_amount_minor"] = amount
		result["refund_amount_display"] = domain.FormatAmount(amount)
		result["refund_status"] = refund.RefundStatus
	}

	if err := tx.AppendAuditLog(ctx, &domain.ActionAuditLog{
		ID: domain.NewID(), RequestID: requestID, CustomerID: customerID, ConversationID: action.ConversationID,
		ActionType: caseType, TargetType: "service_case", TargetID: cs.ID,
		BeforeJSON: toJSON(map[string]any{"order_status": order.OrderStatus}),
		AfterJSON:  toJSON(result),
		ActorType:  domain.ActorCustomer, ActorID: customerID, Result: "succeeded", Reason: payload.Reason,
		CreatedAt: now,
	}); err != nil {
		return nil, domain.AsAppError(err)
	}
	return result, nil
}

// ---- helpers ----

func (s *Actions) loadAction(ctx context.Context, customerID, conversationID, actionID string) (*domain.PendingAction, error) {
	action, err := s.store.GetPendingAction(ctx, actionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该待确认操作")
		}
		return nil, domain.AsAppError(err)
	}
	if action.CustomerID != customerID {
		return nil, domain.ForbiddenError("该操作不属于当前客户")
	}
	if conversationID != "" && action.ConversationID != conversationID {
		return nil, domain.ForbiddenError("该操作不属于当前会话")
	}
	return action, nil
}

func (s *Actions) reloadOrder(ctx context.Context, tx repository.Store, customerID, orderID string) (*domain.Order, error) {
	order, err := tx.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("订单不存在")
		}
		return nil, domain.AsAppError(err)
	}
	if order.CustomerID != customerID {
		return nil, domain.ForbiddenError("该订单不属于当前客户")
	}
	return order, nil
}

func (s *Actions) finishFailed(ctx context.Context, action *domain.PendingAction, requestID, message string) {
	now := s.nowUTC()
	_ = s.store.FinishPendingAction(ctx, action.ID, domain.ActionFailed, toJSON(map[string]any{"error": message}), &now)
	s.audit(ctx, &domain.ActionAuditLog{
		RequestID: requestID, CustomerID: action.CustomerID, ConversationID: action.ConversationID,
		ActionType: action.ActionType, TargetType: "order", TargetID: action.TargetID,
		ActorType: domain.ActorCustomer, ActorID: action.CustomerID, Result: "failed", Reason: message,
	})
	s.setTaskState(ctx, action.ConversationID, nil, "")
}

func idempotentResult(action *domain.PendingAction) (*ActionResult, error) {
	result := map[string]any{}
	if action.ResultJSON != "" {
		_ = json.Unmarshal([]byte(action.ResultJSON), &result)
	}
	return &ActionResult{
		ActionID: action.ID, ActionNo: action.ActionNo, ActionType: action.ActionType,
		Status: domain.ActionExecuted, Result: result, Idempotent: true,
	}, nil
}

// setTaskState records the open task in conversations.active_task_state_json so
// the next turn knows which action is waiting for the customer.
func (s *Actions) setTaskState(ctx context.Context, conversationID string, action *domain.PendingAction, summary string) {
	if conversationID == "" {
		return
	}
	state := ""
	if action != nil {
		state = toJSON(map[string]any{
			"pending_action_id": action.ID,
			"action_no":         action.ActionNo,
			"action_type":       action.ActionType,
			"target_id":         action.TargetID,
			"expires_at":        action.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
			"summary":           summary,
		})
	}
	_ = s.store.UpdateConversationState(ctx, conversationID, "", state, s.nowUTC())
}

// orderFieldLabels renders the whitelisted order fields in customer-facing copy.
var orderFieldLabels = map[string]string{
	"receiver_name":         "收件人",
	"receiver_phone":        "收件电话",
	"shipping_province":     "省份",
	"shipping_city":         "城市",
	"shipping_district":     "区县",
	"shipping_address":      "详细地址",
	"shipping_postal_code":  "邮编",
	"shipping_method":       "配送方式",
	"requested_delivery_at": "预约送达时间",
	"order_remark":          "订单备注",
}

func orderFieldLabel(field string) string {
	if label, ok := orderFieldLabels[field]; ok {
		return label
	}
	return field
}

func buildSummary(actionType, orderNo string, res *EligibilityResult) string {
	switch actionType {
	case domain.ActionCancelOrder:
		if res.EstimatedAmountMinor != nil && *res.EstimatedAmountMinor > 0 {
			return fmt.Sprintf("取消订单 %s，并申请退款 %s", orderNo, domain.FormatAmount(*res.EstimatedAmountMinor))
		}
		return fmt.Sprintf("取消订单 %s", orderNo)
	case domain.ActionModifyOrder:
		fields := make([]string, 0, 4)
		if diffs, ok := res.Preview["changes"].([]map[string]any); ok {
			for _, d := range diffs {
				if name, ok := d["field"].(string); ok {
					fields = append(fields, orderFieldLabel(name))
				}
			}
		}
		if len(fields) == 0 {
			return fmt.Sprintf("修改订单 %s 的收件信息", orderNo)
		}
		return fmt.Sprintf("修改订单 %s 的 %s", orderNo, strings.Join(fields, "、"))
	default:
		name := ""
		quantity := int64(1)
		if res.item != nil {
			name = res.item.ProductNameSnapshot
		}
		if v, ok := res.Preview["quantity"].(int); ok {
			quantity = int64(v)
		}
		label := caseTypeLabel(actionType)
		if res.EstimatedAmountMinor != nil && *res.EstimatedAmountMinor > 0 {
			return fmt.Sprintf("为订单 %s 的「%s」提交%s ×%d，预计退款 %s",
				orderNo, name, label, quantity, domain.FormatAmount(*res.EstimatedAmountMinor))
		}
		return fmt.Sprintf("为订单 %s 的「%s」提交%s ×%d", orderNo, name, label, quantity)
	}
}

func caseTypeLabel(caseType string) string {
	switch caseType {
	case domain.CaseTypeReturn:
		return "退货申请"
	case domain.CaseTypeExchange:
		return "换货申请"
	case domain.CaseTypeRepair:
		return "维修申请"
	case domain.CaseTypeRefund:
		return "退款申请"
	case domain.CaseTypeReship:
		return "补发申请"
	case domain.CaseTypePriceProtection:
		return "价保申请"
	default:
		return "售后申请"
	}
}
