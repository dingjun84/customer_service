package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// Eligibility reason codes are stable, machine-readable and safe to show to the model.
const (
	ReasonOK               = "ok"
	ReasonUnsupported      = "unsupported_action"
	ReasonOrderNotShipped  = "order_not_shipped"
	ReasonAlreadyShipped   = "already_shipped"
	ReasonOrderClosed      = "order_closed"
	ReasonOrderNotPaid     = "order_not_paid"
	ReasonOutOfWindow      = "out_of_window"
	ReasonQuantityExceeded = "quantity_exceeded"
	ReasonItemNotInOrder   = "item_not_in_order"
	ReasonItemRequired     = "item_required"
	ReasonConflictingCase  = "conflicting_case"
	ReasonNoChanges        = "no_changes"
	ReasonInvalidField     = "invalid_field"
	ReasonNothingToRefund  = "nothing_to_refund"
)

// EligibilityRequest is the server-side input of the eligibility checker.
type EligibilityRequest struct {
	CustomerID  string
	ActionType  string
	OrderNo     string
	OrderItemID string
	Quantity    int
	Changes     map[string]any
	Reason      string
}

// EligibilityResult is returned to the model. `eligible` is a server decision and
// must never be derived from prompt wording.
type EligibilityResult struct {
	Eligible             bool           `json:"eligible"`
	ActionType           string         `json:"action_type"`
	OrderNo              string         `json:"order_no,omitempty"`
	ReasonCode           string         `json:"reason_code"`
	Message              string         `json:"message"`
	Limits               map[string]any `json:"limits,omitempty"`
	Preview              map[string]any `json:"preview,omitempty"`
	EstimatedAmountMinor *int64         `json:"estimated_amount_minor,omitempty"`
	AllowedFields        []string       `json:"allowed_fields,omitempty"`
	OrderItemID          string         `json:"order_item_id,omitempty"`

	// Internal handles for proposal creation (never serialized).
	order *domain.Order
	item  *domain.OrderItem
}

// EligibleWithPreview is the normalized view used by tool handlers.
func (r *EligibilityResult) EligibleWithPreview() bool { return r != nil && r.Eligible }

// Eligibility implements the business rules of section 7 of the spec.
type Eligibility struct{ core }

// Check evaluates whether an action is currently allowed for an order.
// It returns a business result for ineligible cases and an error only for
// missing/forbidden resources or invalid input structure.
func (s *Eligibility) Check(ctx context.Context, req EligibilityRequest) (*EligibilityResult, error) {
	actionType := strings.TrimSpace(req.ActionType)
	if !domain.IsSupportedActionType(actionType) {
		return &EligibilityResult{
			Eligible:   false,
			ActionType: actionType,
			ReasonCode: ReasonUnsupported,
			Message:    "该操作类型不受支持",
		}, nil
	}

	order, err := s.ownedOrder(ctx, req.CustomerID, req.OrderNo)
	if err != nil {
		return nil, err
	}

	res := &EligibilityResult{
		Eligible:      false,
		ActionType:    actionType,
		OrderNo:       order.OrderNo,
		ReasonCode:    ReasonOK,
		AllowedFields: domain.ModifiableOrderFields,
		order:         order,
	}

	switch actionType {
	case domain.ActionCancelOrder:
		return s.checkCancel(ctx, order, res)
	case domain.ActionModifyOrder:
		return s.checkModify(order, req, res)
	case domain.CaseTypePriceProtection:
		return s.checkPriceProtection(ctx, order, res)
	default:
		return s.checkAfterSales(ctx, order, req, res)
	}
}

func fail(res *EligibilityResult, code, message string) *EligibilityResult {
	res.Eligible = false
	res.ReasonCode = code
	res.Message = message
	return res
}

func allow(res *EligibilityResult, message string) *EligibilityResult {
	res.Eligible = true
	res.ReasonCode = ReasonOK
	res.Message = message
	return res
}

func (s *Eligibility) checkCancel(ctx context.Context, order *domain.Order, res *EligibilityResult) (*EligibilityResult, error) {
	switch order.OrderStatus {
	case domain.OrderCancelled, domain.OrderClosed:
		return fail(res, ReasonOrderClosed, "订单已取消或关闭，无法再次取消"), nil
	}
	if !domain.IsUnshippedOrderStatus(order.OrderStatus) {
		return fail(res, ReasonAlreadyShipped, "订单已发货或已完成，无法直接取消，可改为申请退货退款"), nil
	}
	shipments, err := s.store.ListShipments(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	for _, sh := range shipments {
		if sh.ShipmentStatus != domain.ShipmentPending {
			return fail(res, ReasonAlreadyShipped, "订单已被物流揽收，无法直接取消"), nil
		}
	}

	preview := map[string]any{
		"order_no":        order.OrderNo,
		"current_status":  order.OrderStatus,
		"target_status":   domain.OrderCancelled,
		"refund_expected": order.PaidAmountMinor > order.RefundedAmountMinor,
	}
	if refund := order.PaidAmountMinor - order.RefundedAmountMinor; refund > 0 {
		res.EstimatedAmountMinor = &refund
		preview["estimated_refund_minor"] = refund
		preview["estimated_refund_display"] = domain.FormatAmount(refund)
	}
	res.Preview = preview
	return allow(res, "订单尚未发货，可以取消"), nil
}

func (s *Eligibility) checkModify(order *domain.Order, req EligibilityRequest, res *EligibilityResult) (*EligibilityResult, error) {
	switch order.OrderStatus {
	case domain.OrderCancelled, domain.OrderClosed:
		return fail(res, ReasonOrderClosed, "订单已取消或关闭，无法修改"), nil
	}
	if !domain.IsUnshippedOrderStatus(order.OrderStatus) {
		return fail(res, ReasonAlreadyShipped, "订单已发货，收件信息无法修改，请联系承运商或转人工"), nil
	}
	if len(req.Changes) == 0 {
		return fail(res, ReasonNoChanges, "未提供需要修改的字段"), nil
	}

	changes := make(map[string]string, len(req.Changes))
	for field, raw := range req.Changes {
		if !domain.IsModifiableOrderField(field) {
			return fail(res, ReasonInvalidField,
				fmt.Sprintf("字段 %s 不允许修改，允许修改的字段为: %s", field, strings.Join(domain.ModifiableOrderFields, ", "))), nil
		}
		value, ok := raw.(string)
		if !ok {
			return fail(res, ReasonInvalidField, fmt.Sprintf("字段 %s 的值必须是字符串", field)), nil
		}
		value = strings.TrimSpace(value)
		if err := validateOrderFieldValue(field, value); err != nil {
			return fail(res, ReasonInvalidField, err.Error()), nil
		}
		changes[field] = value
	}

	preview := map[string]any{
		"order_no":       order.OrderNo,
		"current_status": order.OrderStatus,
		"changes":        []map[string]any{},
	}
	diffs := make([]map[string]any, 0, len(changes))
	for field, value := range changes {
		before := currentOrderFieldValue(order, field)
		after := value
		if field == "receiver_phone" {
			after = domain.MaskPhone(value)
		}
		diffs = append(diffs, map[string]any{"field": field, "before": before, "after": after})
	}
	preview["changes"] = diffs
	res.Preview = preview
	res.Limits = map[string]any{"immutable_fields": []string{"order_status", "paid_amount_minor", "refunded_amount_minor", "payment_status", "customer_id", "order_items"}}
	return allow(res, "订单未发货，收件信息可以修改"), nil
}

func (s *Eligibility) checkAfterSales(ctx context.Context, order *domain.Order, req EligibilityRequest, res *EligibilityResult) (*EligibilityResult, error) {
	actionType := res.ActionType
	if order.OrderStatus == domain.OrderCancelled || order.OrderStatus == domain.OrderClosed {
		return fail(res, ReasonOrderClosed, "订单已取消或关闭，无法申请售后"), nil
	}
	if order.OrderStatus != domain.OrderShipped && order.OrderStatus != domain.OrderCompleted {
		return fail(res, ReasonOrderNotShipped, "订单尚未发货，暂不支持该售后申请；如不需要可申请取消订单"), nil
	}

	// The 7-day style window is computed server-side, never estimated by the model.
	base := order.DeliveredAt
	if base == nil {
		base = order.ShippedAt
	}
	if base != nil && s.nowUTC().Sub(*base) > s.afterSalesWindow {
		return fail(res, ReasonOutOfWindow,
			fmt.Sprintf("已超过售后申请期限（签收后 %d 天内）", int(s.afterSalesWindow.Hours()/24))), nil
	}

	items, err := s.store.ListOrderItems(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	item, resErr := resolveOrderItem(items, req.OrderItemID)
	if resErr != nil {
		res.ReasonCode = ReasonItemRequired
		res.Message = resErr.Error()
		return res, nil
	}
	res.item = item
	res.OrderItemID = item.ID

	quantity := req.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	if quantity > int(item.Quantity) {
		return fail(res, ReasonQuantityExceeded, fmt.Sprintf("申请数量超过购买数量，最多 %d 件", item.Quantity)), nil
	}

	openQty, err := s.store.SumOpenRequestedQuantity(ctx, item.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	available := int(item.Quantity) - int(item.RefundedQuantity) - openQty
	if available <= 0 {
		return fail(res, ReasonConflictingCase, "该商品已全部退款或已有处理中的售后申请"), nil
	}
	if quantity > available {
		return fail(res, ReasonQuantityExceeded, fmt.Sprintf("该商品可申请数量为 %d 件", available)), nil
	}

	conflict, err := s.hasActiveCase(ctx, order.ID, item.ID)
	if err != nil {
		return nil, err
	}
	if conflict {
		return fail(res, ReasonConflictingCase, "该商品已有处理中的售后申请，不能重复提交"), nil
	}

	res.Limits = map[string]any{"available_quantity": available}

	preview := map[string]any{
		"order_no":      order.OrderNo,
		"case_type":     actionType,
		"product_name":  item.ProductNameSnapshot,
		"sku_name":      item.SKUNameSnapshot,
		"quantity":      quantity,
		"order_item_id": item.ID,
	}
	if actionType == domain.CaseTypeRefund {
		if order.PaidAmountMinor-order.RefundedAmountMinor <= 0 {
			return fail(res, ReasonNothingToRefund, "该订单已无可退金额"), nil
		}
		estimate := proratedRefund(item, quantity)
		refundable := order.PaidAmountMinor - order.RefundedAmountMinor
		if estimate > refundable {
			estimate = refundable
		}
		if estimate <= 0 {
			return fail(res, ReasonNothingToRefund, "该商品无可退金额"), nil
		}
		res.EstimatedAmountMinor = &estimate
		preview["estimated_refund_minor"] = estimate
		preview["estimated_refund_display"] = domain.FormatAmount(estimate)
		preview["refund_note"] = "模拟环境：审核通过后按原支付渠道退回，不会真实打款"
	}

	res.Preview = preview
	return allow(res, "符合"+actionType+"申请条件"), nil
}

func (s *Eligibility) checkPriceProtection(ctx context.Context, order *domain.Order, res *EligibilityResult) (*EligibilityResult, error) {
	if order.PaidAt == nil && order.OrderStatus == domain.OrderPendingPayment {
		return fail(res, ReasonOrderNotPaid, "订单未支付，不适用价保"), nil
	}
	start := order.PaidAt
	if start == nil {
		start = &order.CreatedAt
	}
	if s.nowUTC().Sub(*start) > s.priceProtectionWindow {
		return fail(res, ReasonOutOfWindow,
			fmt.Sprintf("已超过价保申请期限（支付后 %d 天内）", int(s.priceProtectionWindow.Hours()/24))), nil
	}

	diff, err := s.estimatePriceProtection(ctx, order)
	if err != nil {
		return nil, err
	}
	res.EstimatedAmountMinor = &diff
	res.Preview = map[string]any{
		"order_no":                     order.OrderNo,
		"estimated_difference_minor":   diff,
		"estimated_difference_display": domain.FormatAmount(diff),
		"note":                         "模拟环境：差额为按活动规则估算，实际以审核结果为准",
	}
	if diff <= 0 {
		return fail(res, "no_price_difference", "当前未发现该订单商品的降价活动，暂无可申请的差价"), nil
	}
	return allow(res, "符合价保申请条件"), nil
}

// estimatePriceProtection sums promotion-provided differences for the order items.
// Rules live in promotions.rules_json so the model never invents an amount.
func (s *Eligibility) estimatePriceProtection(ctx context.Context, order *domain.Order) (int64, error) {
	promos, err := s.store.ListPromotions(ctx, "", "", 50)
	if err != nil {
		return 0, domain.AsAppError(err)
	}
	if len(promos) == 0 {
		return 0, nil
	}
	items, err := s.store.ListOrderItems(ctx, order.ID)
	if err != nil {
		return 0, domain.AsAppError(err)
	}
	var total int64
	for _, item := range items {
		for _, p := range promos {
			if p.Rules == nil {
				continue
			}
			diffRaw, ok := p.Rules["price_protection_diff_minor"]
			if !ok {
				continue
			}
			diff, ok := toInt64(diffRaw)
			if !ok || diff <= 0 {
				continue
			}
			if !ruleMatchesSKU(p.Rules, item.SKUCodeSnapshot) {
				continue
			}
			total += diff * item.Quantity
			break
		}
	}
	return total, nil
}

func ruleMatchesSKU(rules map[string]any, skuCode string) bool {
	raw, ok := rules["sku_codes"]
	if !ok {
		return true
	}
	list, ok := raw.([]any)
	if !ok {
		return true
	}
	if len(list) == 0 {
		return true
	}
	for _, v := range list {
		if s, ok := v.(string); ok && s == skuCode {
			return true
		}
	}
	return false
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

// hasActiveCase reports whether the item already has a non-terminal case.
func (s *Eligibility) hasActiveCase(ctx context.Context, orderID, itemID string) (bool, error) {
	cases, err := s.store.ListServiceCasesByOrder(ctx, orderID)
	if err != nil {
		return false, domain.AsAppError(err)
	}
	for _, c := range cases {
		if isTerminalCase(c.CaseStatus) {
			continue
		}
		if c.OrderItemID == "" || c.OrderItemID == itemID {
			return true, nil
		}
	}
	return false, nil
}

func isTerminalCase(status string) bool {
	switch status {
	case domain.CaseCompleted, domain.CaseCancelled, domain.CaseRejected:
		return true
	default:
		return false
	}
}

// resolveOrderItem picks the target item: an explicit ID must belong to the order,
// otherwise a single-item order is unambiguous and multi-item orders must specify.
func resolveOrderItem(items []domain.OrderItem, requestedID string) (*domain.OrderItem, error) {
	if requestedID != "" {
		for i := range items {
			if items[i].ID == requestedID {
				return &items[i], nil
			}
		}
		return nil, fmt.Errorf("order_item_id 不属于该订单")
	}
	if len(items) == 1 {
		return &items[0], nil
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("订单没有可售后的商品")
	}
	return nil, fmt.Errorf("订单包含多个商品，请指定 order_item_id")
}

// proratedRefund spreads the item's paid amount over its quantity so partial
// returns are refunded by the actual paid price, not the list price.
func proratedRefund(item *domain.OrderItem, quantity int) int64 {
	if item.Quantity <= 0 {
		return 0
	}
	return item.PaidAmountMinor * int64(quantity) / item.Quantity
}

func validateOrderFieldValue(field, value string) error {
	switch field {
	case "receiver_name":
		if value == "" || len([]rune(value)) > 50 {
			return fmt.Errorf("收件人姓名需为 1-50 个字符")
		}
	case "receiver_phone":
		digits := 0
		for _, r := range value {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits < 5 || digits > 20 {
			return fmt.Errorf("手机号格式不正确")
		}
	case "shipping_province", "shipping_city", "shipping_district":
		if value == "" || len([]rune(value)) > 40 {
			return fmt.Errorf("省市区字段不能为空且不超过 40 个字符")
		}
	case "shipping_address":
		if value == "" || len([]rune(value)) > 200 {
			return fmt.Errorf("详细地址需为 1-200 个字符")
		}
	case "shipping_postal_code":
		if len(value) < 4 || len(value) > 10 {
			return fmt.Errorf("邮编格式不正确")
		}
	case "shipping_method":
		switch value {
		case "standard", "express", "scheduled":
		default:
			return fmt.Errorf("配送方式仅支持 standard / express / scheduled")
		}
	case "requested_delivery_at":
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return fmt.Errorf("预约送达时间必须是 RFC3339 格式")
		}
		if t.Before(time.Now().Add(-time.Minute)) {
			return fmt.Errorf("预约送达时间不能早于当前时间")
		}
	case "order_remark":
		if len([]rune(value)) > 200 {
			return fmt.Errorf("订单备注不超过 200 个字符")
		}
	}
	return nil
}

func currentOrderFieldValue(order *domain.Order, field string) string {
	switch field {
	case "receiver_name":
		return order.ReceiverName
	case "receiver_phone":
		return order.ReceiverPhoneMasked
	case "shipping_province":
		return order.ShippingProvince
	case "shipping_city":
		return order.ShippingCity
	case "shipping_district":
		return order.ShippingDistrict
	case "shipping_address":
		return order.ShippingAddress
	case "shipping_postal_code":
		return order.ShippingPostalCode
	case "shipping_method":
		return order.ShippingMethod
	case "requested_delivery_at":
		return order.RequestedDeliveryAt
	case "order_remark":
		return order.OrderRemark
	default:
		return ""
	}
}
