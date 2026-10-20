package tools

import (
	"context"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/service"
)

// registerHandlers wires every tool name to an application service method.
// Handlers stay thin on purpose: business rules live in service, not here.
func (r *Registry) registerHandlers() {
	r.register(ToolSearchKnowledge, r.searchKnowledge)
	r.register(ToolSearchProducts, r.searchProducts)
	r.register(ToolGetProductDetail, r.getProductDetail)
	r.register(ToolListMyOrders, r.listMyOrders)
	r.register(ToolGetOrderDetail, r.getOrderDetail)
	r.register(ToolGetLogisticsStatus, r.getLogisticsStatus)
	r.register(ToolGetPaymentRefund, r.getPaymentRefundStatus)
	r.register(ToolGetServiceStatus, r.getServiceStatus)
	r.register(ToolGetInvoiceStatus, r.getInvoiceStatus)
	r.register(ToolGetBenefitsStatus, r.getBenefitsStatus)
	r.register(ToolGetPromotionRules, r.getPromotionRules)
	r.register(ToolCheckEligibility, r.checkEligibility)
	r.register(ToolRequestCancellation, r.requestCancellation)
	r.register(ToolModifyOrder, r.modifyOrder)
	r.register(ToolSubmitServiceRequest, r.submitServiceRequest)
	r.register(ToolCreateComplaintTicket, r.createComplaintTicket)
	r.register(ToolTransferToHuman, r.transferToHuman)
}

func (r *Registry) searchKnowledge(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	query := args.String("query")
	if strings.TrimSpace(query) == "" {
		return nil, domain.ValidationError("query 不能为空")
	}
	hits, err := r.svc.Knowledge.Search(ctx, query, args.String("category"), args.Int("limit", 3))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"query":    query,
		"articles": hits,
		"note":     "知识库只用于解释政策，订单/物流/退款状态必须实时查询业务接口",
	}, nil
}

func (r *Registry) searchProducts(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	products, err := r.svc.Catalog.Search(ctx, args.String("query"), args.String("category"), args.Int("limit", 5))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(products))
	for _, p := range products {
		out = append(out, map[string]any{
			"product_id":   p.ID,
			"product_code": p.ProductCode,
			"name":         p.Name,
			"brand":        p.Brand,
			"category":     p.Category,
			"description":  p.Description,
			"status":       p.Status,
			"attributes":   p.Attributes,
		})
	}
	return map[string]any{"products": out, "count": len(out)}, nil
}

func (r *Registry) getProductDetail(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	id := args.String("product_id")
	if id == "" {
		id = args.String("product_code")
	}
	if id == "" {
		return nil, domain.ValidationError("product_id 或 product_code 至少提供一个")
	}
	product, err := r.svc.Catalog.Detail(ctx, id)
	if err != nil {
		return nil, err
	}
	skus := make([]map[string]any, 0, len(product.SKUs))
	for _, k := range product.SKUs {
		skus = append(skus, map[string]any{
			"sku_id":         k.ID,
			"sku_code":       k.SKUCode,
			"sku_name":       k.SKUName,
			"specifications": k.Specifications,
			"list_price":     money(k.ListPriceMinor),
			"stock_quantity": k.StockQuantity,
			"status":         k.Status,
		})
	}
	return map[string]any{
		"product_id":   product.ID,
		"product_code": product.ProductCode,
		"name":         product.Name,
		"brand":        product.Brand,
		"category":     product.Category,
		"description":  product.Description,
		"attributes":   product.Attributes,
		"skus":         skus,
	}, nil
}

func (r *Registry) listMyOrders(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	orders, err := r.svc.Orders.List(ctx, ec.CustomerID, args.String("status"), args.Int("limit", 5), nil)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(orders))
	for i := range orders {
		out = append(out, orderBrief(&orders[i]))
	}
	return map[string]any{
		"orders": out,
		"count":  len(out),
		"note":   "只有订单号唯一时才直接操作；存在多个订单时先向用户确认是哪一个",
	}, nil
}

func (r *Registry) getOrderDetail(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	order, err := r.svc.Orders.Detail(ctx, ec.CustomerID, args.String("order_no"))
	if err != nil {
		return nil, err
	}
	return orderDetail(order), nil
}

func (r *Registry) getLogisticsStatus(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	if trackingNo := args.String("tracking_no"); trackingNo != "" {
		shipment, err := r.svc.Logistics.ByTrackingNo(ctx, ec.CustomerID, trackingNo)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"tracking_no": trackingNo,
			"shipments":   []map[string]any{shipmentView(shipment)},
			"note":        "预计送达时间如未返回则表示承运商未提供，不要自行推测",
		}, nil
	}
	orderNo := args.String("order_no")
	if orderNo == "" {
		return nil, domain.ValidationError("order_no 或 tracking_no 至少提供一个")
	}
	shipments, err := r.svc.Logistics.ForOrder(ctx, ec.CustomerID, orderNo)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(shipments))
	for i := range shipments {
		out = append(out, shipmentView(&shipments[i]))
	}
	return map[string]any{
		"order_no":  orderNo,
		"shipments": out,
		"count":     len(out),
		"note":      "物流节点仅限工具返回内容；没有轨迹就如实告知用户暂无轨迹",
	}, nil
}

func (r *Registry) getPaymentRefundStatus(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	status, err := r.svc.Refunds.Status(ctx, ec.CustomerID, args.String("order_no"), args.String("refund_no"))
	if err != nil {
		return nil, err
	}
	payments := make([]map[string]any, 0, len(status.Payments))
	for _, p := range status.Payments {
		payments = append(payments, map[string]any{
			"payment_no": p.PaymentNo,
			"method":     p.PaymentMethod,
			"status":     p.PaymentStatus,
			"amount":     money(p.AmountMinor),
			"refunded":   money(p.RefundedAmountMinor),
			"paid_at":    timeString(p.PaidAt),
		})
	}
	refunds := make([]map[string]any, 0, len(status.Refunds))
	for _, rf := range status.Refunds {
		refunds = append(refunds, map[string]any{
			"refund_no":    rf.RefundNo,
			"status":       rf.RefundStatus,
			"amount":       money(rf.AmountMinor),
			"reason":       rf.Reason,
			"requested_at": timeString(&rf.RequestedAt),
			"completed_at": timeString(rf.CompletedAt),
		})
	}
	return map[string]any{
		"order_no":     status.OrderNo,
		"order_status": status.OrderStatus,
		"paid":         money(status.PaidAmountMinor),
		"refunded":     money(status.RefundedAmountMinor),
		"refundable":   money(status.RefundableAmountMinor),
		"payments":     payments,
		"refunds":      refunds,
		"note":         "退款是否到账以 refunds.status 为准；requested/processing 均未到账",
	}, nil
}

func (r *Registry) getServiceStatus(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	if caseNo := args.String("case_no"); caseNo != "" {
		cs, err := r.svc.AfterSales.Get(ctx, ec.CustomerID, caseNo)
		if err != nil {
			return nil, err
		}
		return map[string]any{"cases": []map[string]any{caseView(cs)}}, nil
	}
	orderNo := args.String("order_no")
	if orderNo == "" {
		return nil, domain.ValidationError("order_no 或 case_no 至少提供一个")
	}
	cases, err := r.svc.AfterSales.ListByOrder(ctx, ec.CustomerID, orderNo)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cases))
	for i := range cases {
		out = append(out, caseView(&cases[i]))
	}
	return map[string]any{"order_no": orderNo, "cases": out, "count": len(out)}, nil
}

func (r *Registry) getInvoiceStatus(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	invoices, err := r.svc.Invoices.ListByOrder(ctx, ec.CustomerID, args.String("order_no"))
	if err != nil {
		return nil, err
	}
	filter := args.String("invoice_no")
	out := make([]map[string]any, 0, len(invoices))
	for _, inv := range invoices {
		if filter != "" && inv.InvoiceNo != filter {
			continue
		}
		out = append(out, map[string]any{
			"invoice_no":   inv.InvoiceNo,
			"invoice_type": inv.InvoiceType,
			"status":       inv.InvoiceStatus,
			"title":        inv.Title,
			"amount":       money(inv.AmountMinor),
			"requested_at": timeString(&inv.RequestedAt),
			"issued_at":    timeString(inv.IssuedAt),
		})
	}
	return map[string]any{"invoices": out, "count": len(out), "note": "演示环境不提供真实发票文件"}, nil
}

func (r *Registry) getBenefitsStatus(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	benefits, err := r.svc.Promotions.Benefits(ctx, ec.CustomerID, args.String("benefit_type"), args.Int("limit", 10))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(benefits))
	for _, b := range benefits {
		item := map[string]any{
			"benefit_type": b.BenefitType,
			"benefit_code": b.BenefitCode,
			"status":       b.Status,
			"expires_at":   timeString(b.ExpiresAt),
			"attributes":   b.Attributes,
		}
		if b.AmountMinor != nil {
			item["amount"] = money(*b.AmountMinor)
		}
		if b.Points != nil {
			item["points"] = *b.Points
		}
		out = append(out, item)
	}
	return map[string]any{"benefits": out, "count": len(out)}, nil
}

func (r *Registry) getPromotionRules(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	promos, err := r.svc.Promotions.List(ctx, args.String("query"), args.String("promotion_code"), args.Int("limit", 5))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(promos))
	for _, p := range promos {
		out = append(out, map[string]any{
			"promotion_code": p.PromotionCode,
			"title":          p.Title,
			"description":    p.Description,
			"type":           p.PromotionType,
			"status":         p.Status,
			"starts_at":      timeString(p.StartsAt),
			"ends_at":        timeString(p.EndsAt),
			"rules":          p.Rules,
		})
	}
	return map[string]any{"promotions": out, "count": len(out)}, nil
}

func (r *Registry) checkEligibility(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	result, err := r.svc.Eligibility.Check(ctx, eligibilityRequest(ec, args))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Registry) requestCancellation(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	proposal, err := r.svc.Actions.Propose(ctx, service.ProposalRequest{
		CustomerID:     ec.CustomerID,
		ConversationID: ec.ConversationID,
		RequestID:      ec.RequestID,
		ActionType:     domain.ActionCancelOrder,
		OrderNo:        args.String("order_no"),
		Reason:         args.String("reason"),
	})
	if err != nil {
		return nil, err
	}
	return proposalView(proposal, domain.ActionCancelOrder), nil
}

func (r *Registry) modifyOrder(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	proposal, err := r.svc.Actions.Propose(ctx, service.ProposalRequest{
		CustomerID:     ec.CustomerID,
		ConversationID: ec.ConversationID,
		RequestID:      ec.RequestID,
		ActionType:     domain.ActionModifyOrder,
		OrderNo:        args.String("order_no"),
		Changes:        args.Map("changes"),
		Reason:         args.String("reason"),
	})
	if err != nil {
		return nil, err
	}
	return proposalView(proposal, domain.ActionModifyOrder), nil
}

func (r *Registry) submitServiceRequest(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	caseType := args.String("case_type")
	proposal, err := r.svc.Actions.Propose(ctx, service.ProposalRequest{
		CustomerID:     ec.CustomerID,
		ConversationID: ec.ConversationID,
		RequestID:      ec.RequestID,
		ActionType:     caseType,
		OrderNo:        args.String("order_no"),
		OrderItemID:    args.String("order_item_id"),
		Quantity:       args.Int("quantity", 1),
		Reason:         args.String("reason"),
		Description:    args.String("description"),
	})
	if err != nil {
		return nil, err
	}
	return proposalView(proposal, caseType), nil
}

func (r *Registry) createComplaintTicket(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	ticket, err := r.svc.Tickets.Create(ctx, service.TicketRequest{
		CustomerID:     ec.CustomerID,
		ConversationID: ec.ConversationID,
		OrderNo:        args.String("order_no"),
		TicketType:     domain.TicketComplaint,
		Priority:       args.String("priority"),
		Summary:        args.String("summary"),
		Details:        args.String("details"),
	})
	if err != nil {
		return nil, err
	}
	return ticketView(ticket), nil
}

func (r *Registry) transferToHuman(ctx context.Context, ec ExecutionContext, args Args) (any, error) {
	summary := args.String("summary")
	if strings.TrimSpace(summary) == "" {
		summary = args.String("reason")
	}
	ticket, err := r.svc.Tickets.Create(ctx, service.TicketRequest{
		CustomerID:     ec.CustomerID,
		ConversationID: ec.ConversationID,
		TicketType:     domain.TicketHumanTransfer,
		Priority:       args.String("priority"),
		Summary:        summary,
		Details:        args.String("reason"),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ticket_no":  ticket.TicketNo,
		"status":     ticket.Status,
		"priority":   ticket.Priority,
		"message":    "已转接人工客服，会话已标记为已转接",
		"created_at": ticket.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// ---- view builders ----

func eligibilityRequest(ec ExecutionContext, args Args) service.EligibilityRequest {
	return service.EligibilityRequest{
		CustomerID:  ec.CustomerID,
		ActionType:  args.String("action_type"),
		OrderNo:     args.String("order_no"),
		OrderItemID: args.String("order_item_id"),
		Quantity:    args.Int("quantity", 1),
		Changes:     args.Map("changes"),
		Reason:      args.String("reason"),
	}
}

// proposalView exposes only what the UI/model needs: the action is pending and
// still requires an explicit confirmation.
func proposalView(p *service.Proposal, requestedAction string) map[string]any {
	out := map[string]any{
		"requires_confirmation": p.Action != nil,
		"eligibility":           p.Eligibility,
	}
	if p.Action != nil {
		out["action_id"] = p.Action.ID
		out["action_no"] = p.Action.ActionNo
		out["action_type"] = requestedAction
		out["summary"] = p.Summary
		out["preview"] = p.Eligibility.Preview
		out["expires_at"] = p.ExpiresAt
		out["status"] = p.Action.Status
		out["next_step"] = "已生成待确认操作，必须由用户在前端点击确认按钮后才会执行；在此之前不能声称已办结"
	}
	return out
}

func orderBrief(o *domain.Order) map[string]any {
	return map[string]any{
		"order_no":        o.OrderNo,
		"status":          o.OrderStatus,
		"created_at":      o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"paid":            money(o.PaidAmountMinor),
		"refunded":        money(o.RefundedAmountMinor),
		"is_presale":      o.IsPresale,
		"item_count":      len(o.Items),
		"shipping_method": o.ShippingMethod,
	}
}

func orderDetail(o *domain.Order) map[string]any {
	items := make([]map[string]any, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, map[string]any{
			"order_item_id":  it.ID,
			"product_name":   it.ProductNameSnapshot,
			"sku_name":       it.SKUNameSnapshot,
			"sku_attributes": it.SKUAttributes,
			"quantity":       it.Quantity,
			"refunded_qty":   it.RefundedQuantity,
			"unit_price":     money(it.UnitPriceMinor),
			"paid":           money(it.PaidAmountMinor),
			"item_status":    it.ItemStatus,
		})
	}
	return map[string]any{
		"order_no":     o.OrderNo,
		"status":       o.OrderStatus,
		"created_at":   o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"paid_at":      timeString(o.PaidAt),
		"shipped_at":   timeString(o.ShippedAt),
		"delivered_at": timeString(o.DeliveredAt),
		"cancelled_at": timeString(o.CancelledAt),
		"amounts": map[string]any{
			"item_amount":  money(o.ItemAmountMinor),
			"discount":     money(o.DiscountAmountMinor),
			"shipping_fee": money(o.ShippingFeeMinor),
			"paid":         money(o.PaidAmountMinor),
			"refunded":     money(o.RefundedAmountMinor),
			"currency":     o.Currency,
		},
		"receiver": map[string]any{
			"name":        o.ReceiverName,
			"phone":       o.ReceiverPhoneMasked,
			"region":      strings.Join(nonEmpty(o.ShippingProvince, o.ShippingCity, o.ShippingDistrict), " "),
			"address":     o.ShippingAddress,
			"postal_code": o.ShippingPostalCode,
		},
		"shipping_method":        o.ShippingMethod,
		"requested_delivery_at":  o.RequestedDeliveryAt,
		"remark":                 o.OrderRemark,
		"invoice_type_requested": o.InvoiceTypeRequested,
		"items":                  items,
		"item_count":             len(items),
		"version":                o.Version,
	}
}

func shipmentView(s *domain.Shipment) map[string]any {
	events := make([]map[string]any, 0, len(s.Events))
	for _, e := range s.Events {
		events = append(events, map[string]any{
			"time":        e.EventTime.Format("2006-01-02T15:04:05Z07:00"),
			"description": e.Description,
			"location":    e.Location,
		})
	}
	return map[string]any{
		"shipment_no":           s.ShipmentNo,
		"carrier":               s.CarrierName,
		"tracking_no":           s.TrackingNo,
		"status":                s.ShipmentStatus,
		"estimated_delivery_at": timeString(s.EstimatedDeliveryAt),
		"last_event_at":         timeString(s.LastEventAt),
		"events":                events,
	}
}

func caseView(c *domain.ServiceCase) map[string]any {
	return map[string]any{
		"case_no":            c.CaseNo,
		"case_type":          c.CaseType,
		"case_status":        c.CaseStatus,
		"requested_quantity": c.RequestedQuantity,
		"reason":             c.ReasonText,
		"description":        c.Description,
		"created_at":         c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":         c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func ticketView(t *domain.ServiceTicket) map[string]any {
	return map[string]any{
		"ticket_no":  t.TicketNo,
		"type":       t.TicketType,
		"status":     t.Status,
		"priority":   t.Priority,
		"summary":    t.Summary,
		"created_at": t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func money(minor int64) map[string]any {
	return map[string]any{"amount_minor": minor, "amount_display": domain.FormatAmount(minor)}
}

func timeString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02T15:04:05Z07:00")
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
