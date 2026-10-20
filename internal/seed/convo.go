package seed

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Conversation seeds. Each scenario is a scripted multi-turn dialogue bound to a
// real order of the same customer, and every tool message is produced from the
// same dataset the tool would read at runtime. Assistant text is filled from the
// actual payload, so the dialogue can never claim a state the database
// contradicts.

type toolCall struct {
	name string
	args map[string]any
	fail string // error code: forbidden / not_found / conflict / action_expired / tool_execution_failed
	note string
}

type convoTurn struct {
	user   string
	tools  []toolCall
	answer string
}

type convoTemplate struct {
	key       string
	category  string
	intents   []string
	risk      string
	split     string
	pick      string // order status filter
	instances []int  // customer indexes (1-based)
	turns     []convoTurn
	expected  []string
	forbidden []string
	outcome   string
	pass      []string
	ticket    *ticketSpec
	action    *actionSpec
	transfer  bool
	sentiment string
}

type actionSpec struct {
	typ       string // cancel_order | modify_order | submit_service_request
	status    string // pending | executed | rejected | expired
	payload   map[string]any
	preview   map[string]any
	confirmed bool
}

type ticketSpec struct {
	typ      string
	status   string
	priority string
	category string
	summary  string
	details  string
}

func (g *gen) buildConversations() {
	perCategory := map[string]int{}
	highRisk := 0
	multiTurn := 0
	for _, tpl := range convoTemplates() {
		bound := 0
		for _, custIdx := range tpl.instances {
			o := g.pickOrder(custIdx, tpl.pick)
			if o == nil {
				continue
			}
			if g.emitConversation(tpl, custIdx, o) {
				bound++
				if tpl.risk == "high" || tpl.risk == "critical" {
					highRisk++
				}
				if len(tpl.turns) > 1 {
					multiTurn++
				}
			}
		}
		if bound == 0 {
			g.errs = append(g.errs, fmt.Sprintf("会话模板 %s 没有任何可用实例（pick=%s），场景与数据脱节", tpl.key, tpl.pick))
		}
		perCategory[tpl.category] += bound
	}
	for cat, n := range perCategory {
		if n < 5 {
			g.errs = append(g.errs, fmt.Sprintf("一级意图「%s」只有 %d 个会话，少于规范要求的 5 个", cat, n))
		}
	}
	switch {
	case len(g.ds.conversations) < 60 || len(g.ds.conversations) > 90:
		g.errs = append(g.errs, fmt.Sprintf("会话数 %d 不在 60–90 区间", len(g.ds.conversations)))
	case len(g.ds.messages) < 450 || len(g.ds.messages) > 800:
		g.errs = append(g.errs, fmt.Sprintf("消息数 %d 不在 450–800 区间", len(g.ds.messages)))
	case len(g.ds.toolExecs) < 150 || len(g.ds.toolExecs) > 300:
		g.errs = append(g.errs, fmt.Sprintf("工具日志 %d 条不在 150–300 区间", len(g.ds.toolExecs)))
	case len(g.ds.tickets) < 10 || len(g.ds.tickets) > 20:
		g.errs = append(g.errs, fmt.Sprintf("人工工单 %d 个不在 10–20 区间", len(g.ds.tickets)))
	case multiTurn < 20:
		g.errs = append(g.errs, fmt.Sprintf("多轮会话只有 %d 个，少于 20 个", multiTurn))
	case highRisk < 12:
		g.errs = append(g.errs, fmt.Sprintf("高风险/安全类场景只有 %d 个，少于 12 个", highRisk))
	}
}

// pickOrder returns the newest order of a customer matching every token of the
// filter, so a scenario is never bound to an order that contradicts it.
func (g *gen) pickOrder(custIdx int, pick string) *orderRow {
	if custIdx < 1 || custIdx > len(g.customers) {
		return nil
	}
	cust := g.customers[custIdx-1]
	tokens := strings.Split(pick, "+")
	var candidates []*orderRow
	for i := range g.ds.orders {
		o := &g.ds.orders[i]
		if o.CustomerID != cust.ID {
			continue
		}
		if g.matchOrder(o, tokens) {
			candidates = append(candidates, o)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

func (g *gen) matchOrder(o *orderRow, tokens []string) bool {
	for _, t := range tokens {
		switch {
		case t == "", t == "any":
		case t == "presale":
			if !o.Presale {
				return false
			}
		case t == "cod":
			if o.Channel != "cod" {
				return false
			}
		case t == "has_tracking":
			if len(g.forwardShipments(o.ID)) == 0 {
				return false
			}
		case t == "multi_parcel":
			if len(g.forwardShipments(o.ID)) < 2 {
				return false
			}
		case t == "multi_item":
			if len(g.itemsOf(o.ID)) < 2 {
				return false
			}
		case t == "stalled":
			if !g.stalledParcel(o) {
				return false
			}
		case t == "shipment_exception":
			if !g.hasShipmentStatus(o, "exception", "delivery_failed") {
				return false
			}
		case t == "has_refund":
			if len(g.refundsOf(o.ID)) == 0 {
				return false
			}
		case t == "no_refund":
			if len(g.refundsOf(o.ID)) > 0 {
				return false
			}
		case t == "refund_succeeded":
			if !g.hasRefundStatus(o.ID, "succeeded", "partially_refunded") {
				return false
			}
		case t == "refund_processing":
			if !g.hasRefundStatus(o.ID, "processing", "approved") {
				return false
			}
		case t == "refund_failed":
			if !g.hasRefundStatus(o.ID, "failed") {
				return false
			}
		case t == "refund_rejected":
			if !g.hasRefundStatus(o.ID, "rejected") {
				return false
			}
		case t == "refund_pending":
			if !g.hasRefundStatus(o.ID, "requested", "pending_review") {
				return false
			}
		case t == "has_case":
			if len(g.casesOf(o.ID)) == 0 {
				return false
			}
		case t == "case_open":
			if !g.hasCaseStatus(o.ID, "submitted", "pending_review", "approved", "awaiting_customer", "awaiting_return_shipment", "processing") {
				return false
			}
		case strings.HasPrefix(t, "case_type="):
			if !g.hasCaseType(o.ID, strings.TrimPrefix(t, "case_type=")) {
				return false
			}
		case t == "case_rejected":
			if !g.hasCaseStatus(o.ID, "rejected") {
				return false
			}
		case t == "has_invoice":
			if len(g.invoicesOf(o.ID)) == 0 {
				return false
			}
		case t == "no_invoice":
			if len(g.invoicesOf(o.ID)) > 0 {
				return false
			}
		case t == "invoice_issued":
			if !g.hasInvoiceStatus(o.ID, "issued") {
				return false
			}
		case t == "invoice_red":
			if !g.hasInvoiceStatus(o.ID, "red_flushed") {
				return false
			}
		case t == "invoice_rejected":
			if !g.hasInvoiceStatus(o.ID, "rejected") {
				return false
			}
		case t == "has_return_parcel":
			if len(g.returnShipments(o.ID)) == 0 {
				return false
			}
		case t == "has_reship_parcel":
			if len(g.reshipShipments(o.ID)) == 0 {
				return false
			}
		case t == "multi_payment":
			if g.paymentAttempts(o.ID) < 2 {
				return false
			}
		case t == "payment_failed":
			if !g.hasPaymentStatus(o.ID, "failed") {
				return false
			}
		case t == "in_window":
			if o.DeliveredAt == nil || g.anchor.Sub(*o.DeliveredAt) > afterSalesWindowHours*time.Hour {
				return false
			}
		case t == "out_of_window":
			if o.DeliveredAt == nil || g.anchor.Sub(*o.DeliveredAt) <= afterSalesWindowHours*time.Hour {
				return false
			}
		case t == "out_of_protection":
			if o.PaidAt == nil || g.anchor.Sub(*o.PaidAt) <= priceProtectionHours*time.Hour {
				return false
			}
		default:
			if o.Status != t {
				return false
			}
		}
	}
	return true
}

func (g *gen) emitConversation(tpl convoTemplate, custIdx int, o *orderRow) bool {
	chatCtx := g.contextFor(o)
	if miss := missingFields(tpl, chatCtx); len(miss) > 0 {
		g.errs = append(g.errs, fmt.Sprintf("会话模板 %s 引用了订单 %s 上不存在的字段：%s",
			tpl.key, o.No, strings.Join(miss, "、")))
		return false
	}

	cust := g.customers[custIdx-1]
	msgCount := conversationMessageCount(tpl)
	startAt := g.conversationStart(o, msgCount)
	if !startAt.Before(g.anchor) {
		g.errs = append(g.errs, fmt.Sprintf("会话模板 %s 无法在订单 %s 上安排出早于数据锚点的对话时间", tpl.key, o.No))
		return false
	}
	interval := 90 * time.Second
	if span := g.anchor.Sub(startAt); span < time.Duration(msgCount+2)*interval {
		interval = span / time.Duration(msgCount+2)
	}

	convNo := g.nextConversationNo()
	convID := stableID("conversation", convNo)

	// A dialogue is closed once the assistant has answered; it stays open only
	// while a high-impact action is still waiting for the customer to confirm,
	// and it is transferred when a human owns it.
	status := "closed"
	if tpl.transfer || (tpl.ticket != nil && tpl.ticket.typ == "human_transfer") {
		status = "transferred"
	} else if tpl.action != nil && tpl.action.status == "pending" {
		status = "open"
	}
	var activeTask *string
	if tpl.action != nil && tpl.action.status == "pending" {
		s := fmt.Sprintf(`{"action_type":%q,"order_no":%q,"step":"awaiting_confirmation"}`, tpl.action.typ, o.No)
		activeTask = &s
	}

	conv := conversationRow{
		ID: convID, No: convNo, CustomerID: cust.ID, Status: status, Channel: "web_demo",
		ActiveTask: activeTask, CreatedAt: startAt, UpdatedAt: startAt,
	}
	summary := fmt.Sprintf("%s：%s（订单 %s）", tpl.category, tpl.outcome, o.No)
	conv.Summary = &summary

	seq := 0
	at := startAt
	next := func() time.Time {
		seq++
		at = startAt.Add(time.Duration(seq) * interval)
		return at
	}

	addMsg := func(role, content string, tool *toolCall, payload map[string]any, execStatus string) *messageRow {
		ts := next()
		m := messageRow{
			ID: stableID("msg", convNo, fmt.Sprintf("%d", seq)), ConvID: convID, Role: role,
			Content: content, Sequence: seq, CreatedAt: ts,
			Meta: map[string]any{"seq": seq},
		}
		if tool != nil {
			m.ToolName = tool.name
			m.ToolCallID = fmt.Sprintf("call_%s_%02d", strings.TrimPrefix(convNo, "CONV-"), seq)
			m.ToolArgs = mustJSON(tool.args)
			m.ToolResult = mustJSON(payload)
			m.Meta["tool_status"] = execStatus
			latency := int64(220 + seq*17)
			g.ds.toolExecs = append(g.ds.toolExecs, toolExecRow{
				ID: stableID("toolexec", convNo, m.ToolCallID), ConvID: convID, MessageID: m.ID,
				ToolCallID: m.ToolCallID, ToolName: tool.name, Input: m.ToolArgs, Output: m.ToolResult,
				Status: execStatus, ErrorCode: errCodeOf(payload), ErrorMessage: errMsgOf(payload),
				LatencyMs: latency, CreatedAt: ts, FinishedAt: timePtr(ts.Add(time.Duration(latency) * time.Millisecond)),
			})
		}
		g.ds.messages = append(g.ds.messages, m)
		// Intent annotations belong on natural language turns only; a tool message
		// has no intent of its own.
		if role == "user" {
			g.ds.annotations = append(g.ds.annotations, annotationRow{
				ID: stableID("annotation", convNo, fmt.Sprintf("%d", seq)), MessageID: m.ID,
				Intent: tpl.category, SubIntent: firstIntent(tpl.intents), Sentiment: tpl.sentiment,
				Resolution: resolutionFor(status), Confidence: 0.86, Source: "offline_eval", CreatedAt: ts,
			})
		}
		return &g.ds.messages[len(g.ds.messages)-1]
	}

	for _, trn := range tpl.turns {
		addMsg("user", fillPlaceholders(trn.user, chatCtx), nil, nil, "")
		for _, call := range trn.tools {
			args := map[string]any{}
			for k, v := range call.args {
				args[k] = fillValue(v, chatCtx)
			}
			call.args = args
			payload, execStatus := g.toolResponse(call, o, chatCtx)
			addMsg("tool", "", &call, payload, execStatus)
		}
		addMsg("assistant", fillPlaceholders(trn.answer, chatCtx), nil, nil, "")
	}
	conv.LastMessage = timePtr(at)
	conv.UpdatedAt = at
	g.ds.conversations = append(g.ds.conversations, conv)

	if tpl.ticket != nil {
		g.addTicket(tpl, cust, o, convID, at, chatCtx)
	}
	if tpl.action != nil {
		g.addAction(tpl, cust, o, convID, at, chatCtx)
	}
	g.ds.scenarios = append(g.ds.scenarios, scenarioRow{
		ScenarioID:     "SCN_" + strings.ToUpper(tpl.key) + "_" + fmt.Sprintf("%03d", len(g.ds.scenarios)+1),
		Category:       tpl.category,
		IntentTags:     tpl.intents,
		CustomerNo:     cust.No,
		ConversationNo: convNo,
		Split:          tpl.split,
		Turns:          scenarioTurns(tpl, chatCtx),
		Fixture: map[string]any{
			"order_no": o.No, "order_status": o.Status, "paid_amount_minor": o.Paid,
			"refunded_amount_minor": o.Refunded,
		},
		ExpectedTools:    expectedCalls(tpl.expected),
		ForbiddenActions: tpl.forbidden,
		ExpectedOutcome:  tpl.outcome,
		RiskLevel:        tpl.risk,
		PassCriteria:     tpl.pass,
	})
	return true
}

func conversationMessageCount(tpl convoTemplate) int {
	n := 0
	for _, t := range tpl.turns {
		n += 2 + len(t.tools)
	}
	return n
}

func firstIntent(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func resolutionFor(status string) string {
	switch status {
	case "transferred":
		return "transferred"
	case "closed":
		return "resolved"
	default:
		return "unknown"
	}
}

func scenarioTurns(tpl convoTemplate, ctx map[string]any) []turn {
	out := make([]turn, 0, len(tpl.turns)*2)
	for _, t := range tpl.turns {
		out = append(out, turn{Role: "user", Content: fillPlaceholders(t.user, ctx)})
		out = append(out, turn{Role: "assistant_expected", Content: fillPlaceholders(t.answer, ctx)})
	}
	return out
}

func expectedCalls(names []string) []map[string]any {
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]any{"tool": n, "required": true})
	}
	return out
}

// conversationStart puts the dialogue after the event it talks about (payment,
// shipping, delivery) and always leaves room for `msgCount` messages before the
// data anchor, so no message can ever be in the future.
func (g *gen) conversationStart(o *orderRow, msgCount int) time.Time {
	base := o.CreatedAt.Add(45 * time.Minute)
	if o.PaidAt != nil && o.PaidAt.Add(30*time.Minute).After(base) {
		base = o.PaidAt.Add(30 * time.Minute)
	}
	if o.ShippedAt != nil && o.ShippedAt.Add(90*time.Minute).After(base) {
		base = o.ShippedAt.Add(90 * time.Minute)
	}
	if o.DeliveredAt != nil && o.DeliveredAt.Add(60*time.Minute).After(base) {
		base = o.DeliveredAt.Add(60 * time.Minute)
	}
	latest := g.anchor.Add(-time.Duration(msgCount+2) * 90 * time.Second)
	if base.After(latest) {
		base = latest
	}
	if floor := o.CreatedAt.Add(10 * time.Minute); base.Before(floor) {
		base = floor
	}
	return base
}

// ---- tool payloads ----

func (g *gen) contextFor(o *orderRow) map[string]any {
	items := g.itemsOf(o.ID)
	ctx := map[string]any{
		"order_no":          o.No,
		"status":            statusCN(o.Status),
		"channel":           channelCN(o.Channel),
		"paid":              formatYuan(o.Paid),
		"refunded":          formatYuan(o.Refunded),
		"refundable":        formatYuan(o.Paid - o.Refunded),
		"receiver":          o.ReceiverName,
		"phone":             o.ReceiverPhoneMasked,
		"city":              o.City + o.District,
		"address":           o.Address,
		"created_at":        o.CreatedAt.Format("01-02 15:04"),
		"invoice_requested": invoiceTypeCN(o.InvoiceTypeRequested),
		"item_count":        len(items),
	}
	if o.PaidAt != nil {
		ctx["paid_at"] = o.PaidAt.Format("01-02 15:04")
	}
	if o.ShippedAt != nil {
		ctx["shipped_at"] = o.ShippedAt.Format("01-02 15:04")
	}
	if o.DeliveredAt != nil {
		ctx["delivered_at"] = o.DeliveredAt.Format("01-02 15:04")
	}

	forward := g.forwardShipments(o.ID)
	if len(forward) > 0 {
		sh := forward[0]
		ctx["tracking"] = sh.TrackingNo
		ctx["carrier"] = sh.CarrierName
		ctx["shipment_status"] = shipmentCN(sh.Status)
		ctx["parcel_count"] = len(forward)
		if sh.LastEventAt != nil {
			ctx["last_event"] = sh.LastEventAt.Format("01-02 15:04")
			ctx["last_event_desc"] = g.lastEventDesc(sh.ID)
		}
	}
	if back := g.returnShipments(o.ID); len(back) > 0 {
		ctx["return_tracking"] = back[0].TrackingNo
	}

	if refunds := g.refundsOf(o.ID); len(refunds) > 0 {
		r := refunds[len(refunds)-1]
		ctx["refund_no"] = r.No
		ctx["refund_status"] = refundCN(r.Status)
		ctx["refund_amount"] = formatYuan(r.Amount)
		ctx["refund_channel"] = r.Channel
	}
	if cases := g.casesOf(o.ID); len(cases) > 0 {
		c := cases[len(cases)-1]
		ctx["case_no"] = c.No
		ctx["case_status"] = caseCN(c.Status)
		ctx["case_type"] = caseTypeCN(c.Type)
	}
	if invoices := g.invoicesOf(o.ID); len(invoices) > 0 {
		inv := invoices[len(invoices)-1]
		ctx["invoice_no"] = firstNonEmpty(inv.No, "（待生成）")
		ctx["invoice_status"] = invoiceStatusCN(inv.Status)
		ctx["invoice_amount"] = formatYuan(inv.Amount)
		ctx["invoice_type"] = invoiceTypeCN(inv.Type)
	}
	if len(items) > 0 {
		it := items[0]
		ctx["product"] = it.ProductName
		ctx["sku"] = it.SKUName
		ctx["product_code"] = it.ProductCode
		ctx["qty"] = it.Quantity
		ctx["unit_price"] = formatYuan(it.UnitPrice)
		ctx["item_id"] = it.ID
	}
	ctx["other_order_no"] = g.foreignOrderNo(o)
	ctx["second_order_no"] = g.secondOrderNo(o)
	return ctx
}

func (g *gen) itemsOf(orderID string) []*itemRow {
	var out []*itemRow
	for i := range g.ds.items {
		if g.ds.items[i].OrderID == orderID {
			out = append(out, &g.ds.items[i])
		}
	}
	return out
}

func (g *gen) refundsOf(orderID string) []*refundRow {
	var out []*refundRow
	for i := range g.ds.refunds {
		if g.ds.refunds[i].OrderID == orderID {
			out = append(out, &g.ds.refunds[i])
		}
	}
	return out
}

func (g *gen) casesOf(orderID string) []*caseRow {
	var out []*caseRow
	for i := range g.ds.cases {
		if g.ds.cases[i].OrderID == orderID {
			out = append(out, &g.ds.cases[i])
		}
	}
	return out
}

func (g *gen) invoicesOf(orderID string) []*invoiceRow {
	var out []*invoiceRow
	for i := range g.ds.invoices {
		if g.ds.invoices[i].OrderID == orderID {
			out = append(out, &g.ds.invoices[i])
		}
	}
	return out
}

func (g *gen) shipmentsOf(orderID, direction string) []*shipmentRow {
	var out []*shipmentRow
	for i := range g.ds.shipments {
		sh := &g.ds.shipments[i]
		if sh.OrderID == orderID && sh.Direction == direction {
			out = append(out, sh)
		}
	}
	return out
}

func (g *gen) forwardShipments(orderID string) []*shipmentRow {
	return g.shipmentsOf(orderID, "forward")
}

func (g *gen) returnShipments(orderID string) []*shipmentRow {
	return g.shipmentsOf(orderID, "return")
}

func (g *gen) reshipShipments(orderID string) []*shipmentRow {
	return g.shipmentsOf(orderID, "reship")
}

func (g *gen) lastEventDesc(shipmentID string) string {
	var best *eventRow
	for i := range g.ds.events {
		e := &g.ds.events[i]
		if e.ShipmentID != shipmentID {
			continue
		}
		if best == nil || e.At.After(best.At) {
			best = e
		}
	}
	if best == nil {
		return ""
	}
	return best.Desc
}

func (g *gen) hasRefundStatus(orderID string, statuses ...string) bool {
	for _, r := range g.refundsOf(orderID) {
		for _, s := range statuses {
			if r.Status == s {
				return true
			}
		}
	}
	return false
}

func (g *gen) hasCaseStatus(orderID string, statuses ...string) bool {
	for _, c := range g.casesOf(orderID) {
		for _, s := range statuses {
			if c.Status == s {
				return true
			}
		}
	}
	return false
}

func (g *gen) hasCaseType(orderID, typ string) bool {
	for _, c := range g.casesOf(orderID) {
		if c.Type == typ {
			return true
		}
	}
	return false
}

func (g *gen) hasInvoiceStatus(orderID string, statuses ...string) bool {
	for _, inv := range g.invoicesOf(orderID) {
		for _, s := range statuses {
			if inv.Status == s {
				return true
			}
		}
	}
	return false
}

func (g *gen) hasShipmentStatus(o *orderRow, statuses ...string) bool {
	for _, sh := range g.forwardShipments(o.ID) {
		for _, s := range statuses {
			if sh.Status == s {
				return true
			}
		}
	}
	return false
}

func (g *gen) stalledParcel(o *orderRow) bool {
	for _, sh := range g.forwardShipments(o.ID) {
		if sh.LastEventAt == nil {
			continue
		}
		if sh.Status == "in_transit" || sh.Status == "picked_up" {
			if g.anchor.Sub(*sh.LastEventAt) >= 48*time.Hour {
				return true
			}
		}
	}
	return false
}

func (g *gen) hasPaymentStatus(orderID string, statuses ...string) bool {
	for i := range g.ds.payments {
		p := &g.ds.payments[i]
		if p.OrderID != orderID {
			continue
		}
		for _, s := range statuses {
			if p.Status == s {
				return true
			}
		}
	}
	return false
}

// paymentAttempts counts every payment record of the order, including failed and
// pending attempts: a duplicate charge is exactly the case where a succeeded
// payment is followed by a second attempt.
func (g *gen) paymentAttempts(orderID string) int {
	n := 0
	for i := range g.ds.payments {
		if g.ds.payments[i].OrderID == orderID {
			n++
		}
	}
	return n
}

// foreignOrderNo returns an order that belongs to a different customer, used by
// the cross-tenant / privilege-boundary scenarios.
func (g *gen) foreignOrderNo(o *orderRow) string {
	for i := range g.ds.orders {
		if g.ds.orders[i].CustomerID != o.CustomerID {
			return g.ds.orders[i].No
		}
	}
	return ""
}

// secondOrderNo returns another order of the same customer, used by the
// disambiguation scenarios ("我有两个在途订单").
func (g *gen) secondOrderNo(o *orderRow) string {
	var fallback string
	for i := range g.ds.orders {
		cand := &g.ds.orders[i]
		if cand.CustomerID != o.CustomerID || cand.ID == o.ID {
			continue
		}
		if cand.Status == "shipped" {
			return cand.No
		}
		if fallback == "" {
			fallback = cand.No
		}
	}
	return fallback
}

func (g *gen) toolResponse(call toolCall, o *orderRow, ctx map[string]any) (map[string]any, string) {
	if call.fail != "" {
		return map[string]any{
			"ok": false, "data": nil,
			"error":      map[string]any{"code": call.fail, "message": firstNonEmpty(call.note, defaultErrMessage(call.fail))},
			"request_id": "req_" + strings.ToLower(strings.TrimPrefix(o.No, "ORD-")),
		}, execStatusFor(call.fail)
	}
	data := map[string]any{}
	switch call.name {
	case "get_order_detail":
		items := []map[string]any{}
		for _, it := range g.itemsOf(o.ID) {
			items = append(items, map[string]any{
				"order_item_id": it.ID, "product_name": it.ProductName, "sku_name": it.SKUName,
				"quantity": it.Quantity, "paid_amount_display": formatYuan(it.Paid),
			})
		}
		data = map[string]any{
			"order_no": o.No, "order_status": o.Status, "order_status_display": statusCN(o.Status),
			"paid_amount_display": formatYuan(o.Paid), "refunded_amount_display": formatYuan(o.Refunded),
			"is_presale": o.Presale, "items": items,
			"receiver_phone_masked": o.ReceiverPhoneMasked,
		}
	case "list_my_orders":
		orders := []map[string]any{}
		for _, ord := range g.ds.orders {
			if ord.CustomerID != o.CustomerID {
				continue
			}
			orders = append(orders, map[string]any{
				"order_no": ord.No, "order_status": ord.Status,
				"order_status_display": statusCN(ord.Status), "total_display": formatYuan(ord.Paid),
			})
			if len(orders) >= 5 {
				break
			}
		}
		data = map[string]any{"orders": orders}
	case "get_logistics_status":
		parcels := []map[string]any{}
		for _, sh := range g.ds.shipments {
			if sh.OrderID != o.ID {
				continue
			}
			var events []map[string]any
			for _, e := range g.ds.events {
				if e.ShipmentID == sh.ID {
					events = append(events, map[string]any{"event_time": ts(e.At), "description": e.Desc, "location": e.Location})
				}
			}
			sort.SliceStable(events, func(i, j int) bool {
				return events[i]["event_time"].(string) > events[j]["event_time"].(string)
			})
			parcels = append(parcels, map[string]any{
				"shipment_no": sh.No, "tracking_no": sh.TrackingNo, "carrier": sh.CarrierName,
				"parcel_type":     parcelTypeCN(sh.Direction),
				"shipment_status": sh.Status, "shipment_status_display": shipmentCN(sh.Status),
				"last_event_at": tsPtrString(sh.LastEventAt), "events": events,
			})
		}
		data = map[string]any{"order_no": o.No, "shipments": parcels, "count": len(parcels)}
	case "get_payment_refund_status":
		refunds := []map[string]any{}
		for _, r := range g.ds.refunds {
			if r.OrderID != o.ID {
				continue
			}
			refunds = append(refunds, map[string]any{
				"refund_no": r.No, "refund_status": r.Status, "refund_status_display": refundCN(r.Status),
				"amount_display": formatYuan(r.Amount), "requested_at": ts(r.RequestedAt),
			})
		}
		data = map[string]any{
			"order_no": o.No, "order_status": o.Status, "paid_amount_display": formatYuan(o.Paid),
			"refunded_amount_display":   formatYuan(o.Refunded),
			"refundable_amount_display": formatYuan(o.Paid - o.Refunded), "refunds": refunds,
		}
	case "get_service_status":
		cases := []map[string]any{}
		for _, c := range g.ds.cases {
			if c.OrderID != o.ID {
				continue
			}
			cases = append(cases, map[string]any{
				"case_no": c.No, "case_type": c.Type, "case_status": c.Status,
				"case_status_display": caseCN(c.Status), "resolution": c.Resolution,
			})
		}
		data = map[string]any{"order_no": o.No, "cases": cases}
	case "get_invoice_status":
		invoices := []map[string]any{}
		for _, inv := range g.ds.invoices {
			if inv.OrderID != o.ID {
				continue
			}
			invoices = append(invoices, map[string]any{
				"invoice_no": inv.No, "invoice_type": inv.Type, "invoice_status": inv.Status,
				"amount_display": formatYuan(inv.Amount), "issued_at": tsPtrString(inv.IssuedAt),
			})
		}
		data = map[string]any{"order_no": o.No, "invoices": invoices}
	case "search_knowledge":
		data = map[string]any{"articles": []map[string]any{
			{"article_code": "KB-RET-01", "title": "7 天无理由退货规则", "summary": "签收后 7 天内、不影响二次销售可退；定制/生鲜/拆封食品不支持。"},
			{"article_code": "KB-REF-01", "title": "退款到账时间", "summary": "processing 不代表到账，微信/支付宝 1-3 个工作日。"},
		}}
	case "search_products":
		data = map[string]any{"products": []map[string]any{
			{"product_code": "P001", "name": "星野 X7 5G 智能手机", "brand": "星野", "category": "3C数码"},
		}}
	case "get_product_detail":
		items := g.itemsOf(o.ID)
		if len(items) > 0 {
			it := items[0]
			for i := range g.ds.products {
				if g.ds.products[i].ID == it.ProductID {
					data = map[string]any{
						"product_code": g.ds.products[i].Code, "name": g.ds.products[i].Name,
						"brand": g.ds.products[i].Brand, "attributes": g.ds.products[i].Attrs,
					}
				}
			}
		}
		if len(data) == 0 {
			data = map[string]any{"product_code": "P001", "name": "星野 X7 5G 智能手机"}
		}
	case "check_action_eligibility":
		action, _ := call.args["action_type"].(string)
		eligible, code, msg := eligibilityFor(o, action, g)
		data = map[string]any{
			"eligible": eligible, "action_type": action, "order_no": o.No,
			"reason_code": code, "message": msg,
		}
	case "request_order_cancellation":
		eligible, code, msg := eligibilityFor(o, "cancel_order", g)
		data = map[string]any{
			"requires_confirmation": eligible,
			"eligibility": map[string]any{
				"eligible": eligible, "action_type": "cancel_order",
				"order_no": o.No, "reason_code": code, "message": msg,
			},
			"action_no": g.nextActionNo(g.anchor), "action_type": "cancel_order", "status": "pending",
			"summary": "取消订单 " + o.No, "expires_at": ts(g.anchor.Add(10 * time.Minute)),
		}
	case "modify_order":
		eligible, code, msg := eligibilityFor(o, "modify_order", g)
		data = map[string]any{
			"requires_confirmation": eligible,
			"eligibility": map[string]any{
				"eligible": eligible, "action_type": "modify_order",
				"order_no": o.No, "reason_code": code, "message": msg,
			},
			"action_no": g.nextActionNo(g.anchor), "action_type": "modify_order", "status": "pending",
			"summary": "修改订单 " + o.No + " 的收件信息", "expires_at": ts(g.anchor.Add(10 * time.Minute)),
			"allowed_fields": []string{"receiver_name", "receiver_phone", "shipping_address"},
		}
	case "submit_service_request":
		caseType, _ := call.args["case_type"].(string)
		eligible, code, msg := eligibilityFor(o, firstNonEmpty(caseType, "return"), g)
		data = map[string]any{
			"requires_confirmation": eligible,
			"eligibility": map[string]any{
				"eligible": eligible, "action_type": firstNonEmpty(caseType, "return"),
				"order_no": o.No, "reason_code": code, "message": msg,
			},
			"action_no": g.nextActionNo(g.anchor), "action_type": firstNonEmpty(caseType, "return"), "status": "pending",
			"summary":    "提交" + caseTypeCN(firstNonEmpty(caseType, "return")) + "申请（订单 " + o.No + "）",
			"expires_at": ts(g.anchor.Add(10 * time.Minute)),
		}
	case "create_complaint_ticket", "transfer_to_human":
		data = map[string]any{"ticket_no": g.nextTicketNo(), "status": "open"}
	case "get_promotion_rules":
		data = map[string]any{"promotions": []map[string]any{
			{"promotion_code": "PROMO-PRICEBACK-15D", "title": "价保 15 天", "price_protection_diff_minor": 5000},
		}}
	case "get_customer_benefits_status":
		data = map[string]any{"benefits": []map[string]any{
			{"benefit_type": "coupon", "benefit_code": "CPN-ALL-30", "status": "available", "amount_display": "¥30.00"},
		}}
	}
	return map[string]any{
		"ok": true, "data": data, "error": nil,
		"request_id": "req_" + strings.ToLower(strings.TrimPrefix(o.No, "ORD-")),
	}, "succeeded"
}

func tsPtrString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return ts(*t)
}

func execStatusFor(code string) string {
	switch code {
	case "tool_execution_failed":
		return "failed"
	default:
		return "rejected"
	}
}

func errCodeOf(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if ok, _ := payload["ok"].(bool); ok {
		return ""
	}
	if e, ok := payload["error"].(map[string]any); ok {
		if code, ok := e["code"].(string); ok {
			return code
		}
	}
	return ""
}

func errMsgOf(payload map[string]any) string {
	if e, ok := payload["error"].(map[string]any); ok {
		if msg, ok := e["message"].(string); ok {
			return msg
		}
	}
	return ""
}

func defaultErrMessage(code string) string {
	switch code {
	case "forbidden":
		return "该订单不属于当前客户"
	case "not_found":
		return "未找到对应记录，请确认订单号"
	case "conflict":
		return "当前状态不允许该操作"
	case "action_expired":
		return "待确认操作已过期，请重新发起"
	default:
		return "工具执行失败，请稍后重试或转人工"
	}
}

func eligibilityFor(o *orderRow, action string, g *gen) (bool, string, string) {
	switch action {
	case "cancel_order":
		switch o.Status {
		case "cancelled", "closed":
			return false, "order_closed", "订单已取消或关闭，无法再次取消"
		case "pending_payment", "paid", "processing":
			return true, "ok", "订单尚未发货，可以取消"
		default:
			return false, "already_shipped", "订单已发货或已完成，无法直接取消，可改为申请退货退款"
		}
	case "modify_order":
		switch o.Status {
		case "cancelled", "closed":
			return false, "order_closed", "订单已取消或关闭，无法修改"
		case "pending_payment", "paid", "processing":
			return true, "ok", "订单未发货，收件信息可以修改"
		default:
			return false, "already_shipped", "订单已发货，收件信息无法修改，请联系承运商"
		}
	case "return", "exchange", "repair", "refund", "reship":
		if o.Status != "shipped" && o.Status != "completed" && o.Status != "closed" {
			return false, "order_not_shipped", "订单尚未发货，暂不支持该售后申请"
		}
		if action == "refund" && o.Paid-o.Refunded <= 0 {
			return false, "nothing_to_refund", "该订单已无可退金额"
		}
		if o.DeliveredAt != nil && g.anchor.Sub(*o.DeliveredAt) > afterSalesWindowHours*time.Hour {
			return false, "out_of_window", "已超过售后申请期限（签收后 7 天内）"
		}
		return true, "ok", "符合申请条件"
	case "price_protection":
		if o.PaidAt == nil {
			return false, "order_not_paid", "订单未支付，不适用价保"
		}
		if g.anchor.Sub(*o.PaidAt) > priceProtectionHours*time.Hour {
			return false, "out_of_window", "已超过价保申请期限（支付后 15 天内）"
		}
		return true, "ok", "符合价保申请条件"
	}
	return false, "unsupported_action", "该操作类型不受支持"
}

func statusCN(s string) string {
	switch s {
	case "pending_payment":
		return "待付款"
	case "paid":
		return "已付款待发货"
	case "processing":
		return "备货中"
	case "shipped":
		return "运输中"
	case "completed":
		return "已签收"
	case "cancelled":
		return "已取消"
	case "closed":
		return "已关闭"
	}
	return s
}

func shipmentCN(s string) string {
	switch s {
	case "pending":
		return "待揽收"
	case "picked_up":
		return "已揽收"
	case "in_transit":
		return "运输中"
	case "delivering":
		return "派送中"
	case "delivered":
		return "已签收"
	case "exception":
		return "物流异常"
	case "returned":
		return "已退回"
	case "intercepted":
		return "已拦截"
	case "delivery_failed":
		return "派送失败"
	}
	return s
}

func refundCN(s string) string {
	switch s {
	case "requested":
		return "已提交申请"
	case "pending_review":
		return "审核中"
	case "approved":
		return "审核通过待打款"
	case "processing":
		return "退款处理中"
	case "succeeded":
		return "退款成功"
	case "failed":
		return "退款失败"
	case "rejected":
		return "退款被拒"
	case "cancelled":
		return "退款已取消"
	}
	return s
}

func caseCN(s string) string {
	switch s {
	case "submitted":
		return "已提交"
	case "pending_review":
		return "待审核"
	case "approved":
		return "已通过"
	case "rejected":
		return "已拒绝"
	case "awaiting_customer":
		return "待用户操作"
	case "awaiting_return_shipment":
		return "待寄回"
	case "processing":
		return "处理中"
	case "completed":
		return "已完成"
	case "cancelled":
		return "已取消"
	}
	return s
}

func channelCN(s string) string {
	switch s {
	case "cod":
		return "货到付款"
	case "presale":
		return "预售订单"
	case "cross_border":
		return "跨境保税"
	case "mock_store":
		return "自营商城"
	}
	return s
}

func invoiceTypeCN(s string) string {
	switch s {
	case "electronic":
		return "电子普通发票"
	case "paper":
		return "纸质发票"
	case "vat_special":
		return "增值税专用发票"
	case "none", "":
		return "未申请开票"
	}
	return s
}

func invoiceStatusCN(s string) string {
	switch s {
	case "requested":
		return "已申请待开"
	case "processing":
		return "开票中"
	case "issued":
		return "已开具"
	case "rejected":
		return "开票申请被拒"
	case "voided":
		return "已作废"
	case "red_flushed":
		return "已红冲"
	}
	return s
}

func caseTypeCN(s string) string {
	switch s {
	case "return":
		return "退货"
	case "exchange":
		return "换货"
	case "repair":
		return "维修"
	case "refund":
		return "仅退款"
	case "reship":
		return "补发"
	case "price_protection":
		return "价保"
	}
	return s
}

func parcelTypeCN(dir string) string {
	switch dir {
	case "reship":
		return "补发包裹"
	case "return":
		return "退货包裹"
	}
	return "正向包裹"
}

// ---- tickets / pending actions ----

func (g *gen) addTicket(tpl convoTemplate, cust *customerRow, o *orderRow, convID string, at time.Time, ctx map[string]any) {
	t := tpl.ticket
	no := g.nextTicketNo()
	row := ticketRow{
		ID: stableID("ticket", no), No: no, CustomerID: cust.ID, ConvID: convID, OrderID: o.ID,
		Type: t.typ, Status: t.status, Priority: t.priority, Category: t.category,
		Summary: fillPlaceholders(t.summary, ctx), Details: fillPlaceholders(t.details, ctx),
		CreatedAt: at.Add(2 * time.Minute), UpdatedAt: at.Add(90 * time.Minute),
	}
	if t.status == "resolved" || t.status == "closed" {
		row.ClosedAt = timePtr(at.Add(20 * time.Hour))
	}
	if t.status != "open" {
		row.AssignedTo = "agent_demo_01"
	}
	g.ds.tickets = append(g.ds.tickets, row)
}

func (g *gen) addAction(tpl convoTemplate, cust *customerRow, o *orderRow, convID string, at time.Time, ctx map[string]any) {
	a := tpl.action
	no := g.nextActionNo(at)
	created := at.Add(time.Minute)
	payload := map[string]any{}
	for k, v := range a.payload {
		payload[k] = fillValue(v, ctx)
	}
	preview := map[string]any{}
	for k, v := range a.preview {
		preview[k] = fillValue(v, ctx)
	}
	row := pendingRow{
		ID: stableID("action", no), ActionNo: no, ConvID: convID, CustomerID: cust.ID,
		ActionType: a.typ, TargetType: "order", TargetID: o.ID,
		Payload: mustJSON(payload), Preview: mustJSON(preview), Status: a.status,
		CreatedAt: created, ExpiresAt: created.Add(10 * time.Minute),
	}
	switch a.status {
	case "executed":
		confirmed := created.Add(2 * time.Minute)
		executed := confirmed.Add(5 * time.Second)
		row.ConfirmedAt = timePtr(confirmed)
		row.ExecutedAt = timePtr(executed)
		row.ResultJSON = mustJSON(map[string]any{"ok": true, "order_no": o.No})
		g.ds.audits = append(g.ds.audits, auditRow{
			ID: stableID("audit", no), RequestID: "req_" + strings.ToLower(no), CustomerID: cust.ID,
			ConvID: convID, ActionType: a.typ, TargetType: "order", TargetID: o.ID,
			Before:    mustJSON(map[string]any{"order_status": o.Status}),
			After:     mustJSON(map[string]any{"order_status": o.Status, "applied": true}),
			ActorType: "customer", ActorID: cust.No, Result: "succeeded",
			Reason: "用户在前端点击确认", CreatedAt: executed,
		})
	case "pending":
		// still waiting for the customer to confirm
	case "rejected":
		row.ConfirmedAt = timePtr(created.Add(time.Minute))
		g.ds.audits = append(g.ds.audits, auditRow{
			ID: stableID("audit", no), RequestID: "req_" + strings.ToLower(no), CustomerID: cust.ID,
			ConvID: convID, ActionType: a.typ, TargetType: "order", TargetID: o.ID,
			Before: mustJSON(map[string]any{"order_status": o.Status}), ActorType: "customer",
			ActorID: cust.No, Result: "rejected", Reason: "用户放弃本次操作", CreatedAt: created.Add(time.Minute),
		})
	case "expired":
		row.ExpiresAt = created.Add(10 * time.Minute)
	}
	g.ds.pending = append(g.ds.pending, row)
}
