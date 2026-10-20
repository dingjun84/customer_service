package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/example/ecommerce-support/internal/tools"
)

// OfflineModel is a deterministic, rule-based ChatModel used for local demos and
// UI reviews when no provider credential is available.
//
// It is NOT a language model. It matches keywords, calls exactly the same tools a
// real model would call, and renders the tool output as Chinese text. Every fact
// it states comes from a tool result, so ownership checks, eligibility rules and
// the confirm-before-write flow are exercised the same way as the real path.
//
// It never invents order numbers, amounts or logistics nodes: when a request
// needs an order number that the user did not provide, it asks for one after
// listing the customer's own orders.
type OfflineModel struct{}

// NewOfflineModel builds the rule-based demo model.
func NewOfflineModel() *OfflineModel { return &OfflineModel{} }

var (
	orderNoPattern = regexp.MustCompile(`(?i)ord-\d{3,}`)
	phonePattern   = regexp.MustCompile(`1[3-9]\d{9}`)
)

// Respond implements ChatModel.
func (m *OfflineModel) Respond(_ context.Context, req ModelRequest) (ModelResponse, error) {
	if len(req.Messages) == 0 {
		return TextResponse("你好，我是客服助手。你可以直接说订单号，或者告诉我想查什么。"), nil
	}
	last := req.Messages[len(req.Messages)-1]
	switch last.Role {
	case RoleTool:
		// A follow-up step may still be needed (e.g. an after-sales request on a
		// multi-item order requires the item ID from the order detail).
		if follow, ok := m.followUp(req, last); ok {
			return follow, nil
		}
		return TextResponse(renderToolResult(last.ToolName, last.Content)), nil
	case RoleUser:
		return m.plan(last.Content, req.Messages), nil
	}
	// Anything else (system/assistant tail): answer the most recent user turn.
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == RoleUser {
			return m.plan(req.Messages[i].Content, req.Messages), nil
		}
	}
	return TextResponse("请告诉我你想处理的订单或问题。"), nil
}

// followUp continues a multi-step flow after a read tool ran.
func (m *OfflineModel) followUp(req ModelRequest, last Message) (ModelResponse, bool) {
	if last.ToolName != tools.ToolGetOrderDetail {
		return ModelResponse{}, false
	}
	userText := lastUserMessage(req.Messages)
	caseType, ok := afterSalesCaseType(userText)
	if !ok {
		// A short reply such as "第二个" carries no intent of its own: recover it
		// from the assistant's item-selection question.
		caseType, ok = pendingCaseType(req.Messages)
	}
	if !ok {
		return ModelResponse{}, false
	}
	var res offlineResult
	if err := json.Unmarshal([]byte(last.Content), &res); err != nil || !res.OK {
		return ModelResponse{}, false
	}
	var data map[string]any
	if err := json.Unmarshal(res.Data, &data); err != nil {
		return ModelResponse{}, false
	}

	orderNo := str(data, "order_no")
	if orderNo == "" {
		return ModelResponse{}, false
	}
	items := list(data, "items")
	itemID := pickItem(items, userText)
	if itemID == "" && len(items) > 1 {
		var b strings.Builder
		fmt.Fprintf(&b, "订单 %s 里有 %d 个商品，需要你指定%s哪一个：\n\n", orderNo, len(items), zhCaseType(caseType))
		for i, raw := range items {
			item := asMap(raw)
			fmt.Fprintf(&b, "%d. %s %s（%s）\n", i+1, str(item, "product_name"),
				str(item, "sku_name"), moneyText(item["paid"]))
		}
		b.WriteString("\n可以说「第一个」或直接报商品名。")
		return TextResponse(b.String()), true
	}

	args := map[string]any{
		"case_type":   caseType,
		"order_no":    orderNo,
		"quantity":    1,
		"reason":      truncate(userText, 60),
		"description": userText,
	}
	if itemID != "" {
		args["order_item_id"] = itemID
	}
	return call(tools.ToolSubmitServiceRequest, args), true
}

// lastUserMessage returns the most recent user utterance in the conversation.
func lastUserMessage(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

// recentOrderNo finds the order number mentioned earlier in this conversation.
func recentOrderNo(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != RoleUser {
			continue
		}
		if found := orderNoPattern.FindString(msgs[i].Content); found != "" {
			return strings.ToUpper(found)
		}
	}
	return ""
}

// pickItem maps "第一个"/"第二个"/商品名 onto an order item ID.
func pickItem(items []any, text string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return str(asMap(items[0]), "order_item_id")
	}
	ordinals := []string{"第一个", "第二个", "第三个", "第四个"}
	for index, word := range ordinals {
		if index < len(items) && strings.Contains(text, word) {
			return str(asMap(items[index]), "order_item_id")
		}
	}
	for _, raw := range items {
		item := asMap(raw)
		name := str(item, "product_name")
		if len([]rune(name)) >= 2 && strings.Contains(text, name) {
			return str(item, "order_item_id")
		}
	}
	return ""
}

// afterSalesCaseType classifies an after-sales intent. The second return value
// is false when the text is not an after-sales request at all.
func afterSalesCaseType(text string) (string, bool) {
	lower := strings.ToLower(text)
	has := func(words ...string) bool {
		for _, word := range words {
			if strings.Contains(lower, word) {
				return true
			}
		}
		return false
	}
	switch {
	case has("换货", "换一个", "换新"):
		return "exchange", true
	case has("维修", "坏了", "故障", "不工作", "无法开机"):
		return "repair", true
	case has("补发", "漏发", "少发", "没收到", "还没收到"):
		return "reship", true
	case has("价保", "降价", "差价"):
		return "price_protection", true
	case has("退货", "退了"):
		return "return", true
	case has("退款") && !has("到账", "进度", "到哪", "什么时候", "多久"):
		return "refund", true
	}
	return "", false
}

// itemSelectionFollowUp reports whether the user is answering the assistant's
// "which item?" question, so the flow can continue on the same order.
func itemSelectionFollowUp(msgs []Message) bool {
	if _, ok := pendingCaseType(msgs); !ok {
		return false
	}
	return recentOrderNo(msgs) != ""
}

// pendingCaseType recovers the intent of a short follow-up ("第二个") from the
// assistant's previous item-selection question.
func pendingCaseType(msgs []Message) (string, bool) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != RoleAssistant {
			continue
		}
		content := msgs[i].Content
		// Tool-call rows have no user-facing text: skip them and keep looking for
		// the assistant question.
		if strings.TrimSpace(content) == "" {
			continue
		}
		if !strings.Contains(content, "需要你指定") {
			return "", false
		}
		for _, candidate := range []string{"return", "exchange", "repair", "refund", "reship", "price_protection"} {
			if strings.Contains(content, "指定"+zhCaseType(candidate)) {
				return candidate, true
			}
		}
		return "", false
	}
	return "", false
}

// plan maps one user utterance onto a single tool call.
func (m *OfflineModel) plan(text string, history []Message) ModelResponse {
	raw := strings.TrimSpace(text)
	lower := strings.ToLower(raw)
	orderNo := ""
	if loc := orderNoPattern.FindString(raw); loc != "" {
		orderNo = strings.ToUpper(loc)
	}

	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(lower, w) {
				return true
			}
		}
		return false
	}

	// A short follow-up such as "第一个" or "这个呢" refers to the order named
	// earlier in the same conversation. Longer utterances must name it again.
	if orderNo == "" && len([]rune(raw)) <= 24 && has("第", "这个", "该订单", "它", "继续") {
		orderNo = recentOrderNo(history)
	}

	// requiresOrder returns a "please pick an order" reply when the user has not
	// named one yet; the model must never guess an order number.
	requiresOrder := func() (ModelResponse, bool) {
		if orderNo != "" {
			return ModelResponse{}, false
		}
		if has("规则", "政策", "几天", "多久", "怎么算", "标准") {
			return call(tools.ToolSearchKnowledge, map[string]any{"query": raw, "limit": 3}), true
		}
		return call(tools.ToolListMyOrders, map[string]any{"limit": 10}), true
	}

	switch {
	case has("人工", "真人", "转客服"):
		return call(tools.ToolTransferToHuman, map[string]any{
			"reason":   raw,
			"summary":  truncate(raw, 120),
			"priority": "high",
		})

	case has("投诉", "举报", "太差", "态度"):
		return call(tools.ToolCreateComplaintTicket, map[string]any{
			"summary":  truncate(raw, 80),
			"details":  raw,
			"order_no": orderNo,
			"priority": "high",
		})

	case has("取消") && has("能不能", "可以", "是否", "能取消", "能退"):
		resp, waiting := requiresOrder()
		if waiting {
			return resp
		}
		return call(tools.ToolCheckEligibility, map[string]any{
			"action_type": "cancel_order",
			"order_no":    orderNo,
		})

	case has("取消"):
		resp, waiting := requiresOrder()
		if waiting {
			return resp
		}
		return call(tools.ToolRequestCancellation, map[string]any{
			"order_no": orderNo,
			"reason":   "用户申请取消订单",
		})

	case (has("改", "换") && has("地址", "收件", "收货", "电话", "手机")) || has("修改地址"):
		resp, waiting := requiresOrder()
		if waiting {
			return resp
		}
		changes := parseChanges(raw)
		if len(changes) == 0 {
			return TextResponse(fmt.Sprintf(
				"可以改 %s 的收件信息，但需要你给出新内容，例如：把 %s 的地址改成 深圳市南山区科技园南路 99 号，或把收件电话改成 13900001111。",
				orderNo, orderNo))
		}
		return call(tools.ToolModifyOrder, map[string]any{
			"order_no": orderNo,
			"changes":  changes,
			"reason":   "用户申请修改收件信息",
		})

	case has("退货", "换货", "维修", "坏了", "故障", "补发", "漏发", "少发", "没收到", "价保", "降价", "差价", "退款"):
		if has("退款") && has("到账", "进度", "到哪", "什么时候") {
			resp, waiting := requiresOrder()
			if waiting {
				return resp
			}
			return call(tools.ToolGetPaymentRefund, map[string]any{"order_no": orderNo})
		}
		resp, waiting := requiresOrder()
		if waiting {
			return resp
		}
		// Read the order first: a multi-item order must name the item, and the
		// model may not invent an order_item_id.
		return call(tools.ToolGetOrderDetail, map[string]any{"order_no": orderNo})

	case has("物流", "快递", "到哪", "签收", "运单", "发货", "配送"):
		resp, waiting := requiresOrder()
		if waiting {
			return resp
		}
		return call(tools.ToolGetLogisticsStatus, map[string]any{"order_no": orderNo})

	case has("售后", "处理进度", "进度"):
		resp, waiting := requiresOrder()
		if waiting {
			return resp
		}
		return call(tools.ToolGetServiceStatus, map[string]any{"order_no": orderNo})

	case has("发票"):
		resp, waiting := requiresOrder()
		if waiting {
			return resp
		}
		return call(tools.ToolGetInvoiceStatus, map[string]any{"order_no": orderNo})

	case has("优惠券", "积分", "权益", "白条", "余额"):
		return call(tools.ToolGetBenefitsStatus, map[string]any{"limit": 10})

	case has("活动", "满减", "优惠"):
		return call(tools.ToolGetPromotionRules, map[string]any{"query": raw, "limit": 5})

	case has("规则", "政策", "几天", "多久", "运费", "流程"):
		return call(tools.ToolSearchKnowledge, map[string]any{"query": raw, "limit": 3})

	case has("买", "商品", "有卖", "多少钱", "推荐", "规格", "材质", "尺寸", "功率"):
		return call(tools.ToolSearchProducts, map[string]any{"query": raw, "limit": 5})

	// "第二个" (or a product name) after the item-selection question continues the
	// same after-sales flow on the order named earlier in the conversation.
	case itemSelectionFollowUp(history):
		return call(tools.ToolGetOrderDetail, map[string]any{"order_no": recentOrderNo(history)})

	case orderNo != "":
		return call(tools.ToolGetOrderDetail, map[string]any{"order_no": orderNo})

	case has("订单"):
		return call(tools.ToolListMyOrders, map[string]any{"limit": 10})
	}

	return TextResponse("我可以帮你查订单、物流、退款进度、售后处理、发票，也能代你发起取消/改地址/退货等申请。" +
		"你可以直接说订单号，例如：查一下 ORD-10003 的物流。\n\n" +
		"（当前为本地规则演示模型，复杂问题请配置真实模型后体验。）")
}

func call(name string, args map[string]any) ModelResponse {
	raw, err := json.Marshal(args)
	if err != nil {
		return TextResponse("参数解析失败，请换个说法。")
	}
	return ToolCallResponse("offline_call", name, string(raw))
}

// parseChanges extracts the fields the demo model is able to understand. Only
// whitelisted order fields are ever produced.
func parseChanges(text string) map[string]any {
	changes := map[string]any{}
	if phone := phonePattern.FindString(text); phone != "" {
		changes["receiver_phone"] = phone
	}
	for _, marker := range []string{"改成", "改为", "换成", "换到", "更新为"} {
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(text[idx+len(marker):])
		rest = strings.Trim(rest, "。，,.!！ ")
		if rest == "" || phonePattern.MatchString(rest) {
			break
		}
		changes["shipping_address"] = truncate(rest, 120)
		break
	}
	return changes
}

// ---- result rendering ----

type offlineResult struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func renderToolResult(toolName, content string) string {
	var res offlineResult
	if err := json.Unmarshal([]byte(content), &res); err != nil {
		return "工具返回内容无法解析，请换个说法或转人工客服。"
	}
	if !res.OK {
		return renderToolError(res)
	}
	var data map[string]any
	dec := json.NewDecoder(bytes.NewReader(res.Data))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return "已拿到数据，但内容无法展示，请转人工客服。"
	}

	switch toolName {
	case tools.ToolListMyOrders:
		return renderOrders(data)
	case tools.ToolGetOrderDetail:
		return renderOrderDetail(data)
	case tools.ToolGetLogisticsStatus:
		return renderShipments(data)
	case tools.ToolGetPaymentRefund:
		return renderRefunds(data)
	case tools.ToolGetServiceStatus:
		return renderCases(data)
	case tools.ToolGetInvoiceStatus:
		return renderInvoices(data)
	case tools.ToolCheckEligibility:
		return renderEligibility(data)
	case tools.ToolRequestCancellation, tools.ToolModifyOrder, tools.ToolSubmitServiceRequest:
		return renderProposal(data)
	case tools.ToolCreateComplaintTicket:
		return fmt.Sprintf("已为你创建工单 %s（优先级 %s），人工客服会尽快跟进。",
			str(data, "ticket_no"), zhPriority(str(data, "priority")))
	case tools.ToolTransferToHuman:
		return fmt.Sprintf("已转接人工客服，工单号 %s，当前优先级 %s。会话已标记为已转接，稍后由人工客服接手。",
			str(data, "ticket_no"), zhPriority(str(data, "priority")))
	case tools.ToolSearchProducts:
		return renderProducts(data)
	case tools.ToolGetProductDetail:
		return renderProductDetail(data)
	case tools.ToolSearchKnowledge:
		return renderArticles(data)
	case tools.ToolGetBenefitsStatus:
		return renderBenefits(data)
	case tools.ToolGetPromotionRules:
		return renderPromotions(data)
	}
	return "已获取数据：" + truncate(compactJSON(data), 400)
}

func renderToolError(res offlineResult) string {
	code, msg := "", ""
	if res.Error != nil {
		code, msg = res.Error.Code, res.Error.Message
	}
	if msg == "" {
		msg = "工具执行失败"
	}
	switch code {
	case "forbidden":
		return "这个订单不属于当前演示身份，所以我不能查看或操作它。你可以在右上角切换到对应客户身份后再试。"
	case "not_found":
		return msg + "。请确认订单号是否正确，或者我可以先列出你名下的订单。"
	case "validation_error":
		return "请求参数不完整：" + msg
	case "conflict":
		return msg + "\n\n订单状态已经变化，我没有对订单做任何修改。"
	case "action_expired":
		return "这个待确认操作已过期，需要重新发起一次申请。"
	}
	return "操作没有完成：" + msg
}

func renderOrders(data map[string]any) string {
	orders := list(data, "orders")
	if len(orders) == 0 {
		return "你名下暂时没有订单。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "你名下共有 %d 个订单：\n", len(orders))
	for i, raw := range orders {
		o := asMap(raw)
		fmt.Fprintf(&b, "%d. %s · %s · %s（%s 下单）\n", i+1,
			str(o, "order_no"), zhOrderStatus(str(o, "status")),
			moneyText(o["paid"]), shortTime(str(o, "created_at")))
	}
	b.WriteString("\n告诉我要处理哪一个订单即可，例如：查一下 " + str(asMap(orders[0]), "order_no") + " 的物流。")
	return b.String()
}

func renderOrderDetail(data map[string]any) string {
	amounts := asMap(data["amounts"])
	var b strings.Builder
	fmt.Fprintf(&b, "订单 %s（%s）\n", str(data, "order_no"), zhOrderStatus(str(data, "status")))
	fmt.Fprintf(&b, "实付 %s", moneyText(amounts["paid"]))
	if refunded := moneyText(amounts["refunded"]); refunded != "" && refunded != "¥0.00" {
		fmt.Fprintf(&b, "，已退款 %s", refunded)
	}
	b.WriteString("\n")
	for _, raw := range list(data, "items") {
		it := asMap(raw)
		name := str(it, "product_name")
		if sku := str(it, "sku_name"); sku != "" {
			name += " · " + sku
		}
		fmt.Fprintf(&b, "商品：%s ×%s（%s）\n", name, num(it["quantity"]), moneyText(it["paid"]))
	}
	if r := asMap(data["receiver"]); len(r) > 0 {
		fmt.Fprintf(&b, "收件：%s %s，%s %s\n", str(r, "name"), str(r, "phone"), str(r, "region"), str(r, "address"))
	}
	if paidAt := str(data, "paid_at"); paidAt != "" {
		fmt.Fprintf(&b, "付款时间：%s\n", shortTime(paidAt))
	}
	if shippedAt := str(data, "shipped_at"); shippedAt != "" {
		fmt.Fprintf(&b, "发货时间：%s\n", shortTime(shippedAt))
	}
	b.WriteString("\n需要我查物流、退款进度或申请售后吗？")
	return b.String()
}

func renderShipments(data map[string]any) string {
	shipments := list(data, "shipments")
	if len(shipments) == 0 {
		return fmt.Sprintf("订单 %s 目前还没有物流记录，说明包裹还没有交给承运商。", str(data, "order_no"))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "订单 %s 共有 %d 个包裹：\n", str(data, "order_no"), len(shipments))
	for _, raw := range shipments {
		s := asMap(raw)
		fmt.Fprintf(&b, "\n%s（%s）· %s\n", str(s, "tracking_no"), str(s, "carrier"), zhShipmentStatus(str(s, "status")))
		events := list(s, "events")
		if len(events) == 0 {
			b.WriteString("暂无轨迹节点。\n")
		} else {
			latest := asMap(events[0])
			fmt.Fprintf(&b, "最新轨迹：%s %s", shortTime(str(latest, "time")), str(latest, "description"))
			if loc := str(latest, "location"); loc != "" {
				fmt.Fprintf(&b, "（%s）", loc)
			}
			b.WriteString("\n")
		}
		if eta := str(s, "estimated_delivery_at"); eta != "" {
			fmt.Fprintf(&b, "预计送达：%s\n", shortTime(eta))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderRefunds(data map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "订单 %s（%s）\n", str(data, "order_no"), zhOrderStatus(str(data, "order_status")))
	fmt.Fprintf(&b, "已支付 %s，已退款 %s，可退 %s\n",
		moneyText(data["paid"]), moneyText(data["refunded"]), moneyText(data["refundable"]))
	refunds := list(data, "refunds")
	if len(refunds) == 0 {
		b.WriteString("目前没有退款记录。")
		return b.String()
	}
	for _, raw := range refunds {
		r := asMap(raw)
		fmt.Fprintf(&b, "\n退款单 %s · %s · %s\n申请时间：%s",
			str(r, "refund_no"), zhRefundStatus(str(r, "status")),
			moneyText(r["amount"]), shortTime(str(r, "requested_at")))
		if done := str(r, "completed_at"); done != "" {
			fmt.Fprintf(&b, "，完成时间：%s", shortTime(done))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n如果你看到的是“已申请/处理中”，说明退款还没有到账，到账时间以支付渠道为准。")
	return b.String()
}

func renderCases(data map[string]any) string {
	cases := list(data, "cases")
	if len(cases) == 0 {
		return fmt.Sprintf("订单 %s 目前没有售后单。需要我帮你发起退货、换货、维修或补发申请吗？", str(data, "order_no"))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "订单 %s 的售后申请：\n", str(data, "order_no"))
	for i, raw := range cases {
		c := asMap(raw)
		fmt.Fprintf(&b, "%d. %s · %s · %s（申请于 %s）\n", i+1,
			str(c, "case_no"), zhCaseType(str(c, "case_type")),
			zhCaseStatus(str(c, "case_status")), shortTime(str(c, "created_at")))
		if desc := str(c, "description"); desc != "" {
			fmt.Fprintf(&b, "   说明：%s\n", truncate(desc, 60))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderInvoices(data map[string]any) string {
	invoices := list(data, "invoices")
	if len(invoices) == 0 {
		return "这个订单还没有开票记录。电子发票一般在确认收货后可申请，需要我帮你发起申请吗？"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "共有 %d 条开票记录：\n", len(invoices))
	for i, raw := range invoices {
		inv := asMap(raw)
		fmt.Fprintf(&b, "%d. %s · %s · %s · %s · %s\n", i+1,
			str(inv, "invoice_no"), zhInvoiceType(str(inv, "invoice_type")),
			zhInvoiceStatus(str(inv, "status")), moneyText(inv["amount"]), str(inv, "title"))
		if issued := str(inv, "issued_at"); issued != "" {
			fmt.Fprintf(&b, "   开票时间：%s\n", shortTime(issued))
		}
	}
	b.WriteString("\n演示环境不提供真实发票文件。")
	return b.String()
}

func renderEligibility(data map[string]any) string {
	action := zhAction(str(data, "action_type"))
	orderNo := str(data, "order_no")
	if eligible, _ := data["eligible"].(bool); eligible {
		msg := fmt.Sprintf("可以%s。", action)
		if disp := str(asMap(data["preview"]), "estimated_refund_display"); disp != "" {
			msg += "预计退款 " + disp + "。"
		}
		return fmt.Sprintf("订单 %s %s\n需要我现在提交申请吗？提交后会生成一张待确认卡片，你点确认才真正执行。",
			orderNo, msg)
	}
	reason := str(data, "message")
	if reason == "" {
		reason = "当前状态不允许该操作"
	}
	return fmt.Sprintf("订单 %s 现在还不能%s：%s\n我没有对订单做任何修改。如果情况特殊，我可以帮你转人工客服。",
		orderNo, action, reason)
}

func renderProposal(data map[string]any) string {
	requires, _ := data["requires_confirmation"].(bool)
	if !requires {
		reason := ""
		if e := asMap(data["eligibility"]); len(e) > 0 {
			reason = str(e, "message")
		}
		if reason == "" {
			reason = "当前状态不满足办理条件"
		}
		return "这次申请没有通过资格校验：" + reason + "\n我没有对订单做任何修改。如果需要特殊处理，可以让我转人工客服。"
	}
	summary := str(data, "summary")
	return fmt.Sprintf("已生成待确认操作 %s：%s\n\n请在下方「待确认操作」卡片上点「确认执行」，我才会真正提交；在此之前订单不会有任何变化。",
		str(data, "action_no"), summary)
}

func renderProducts(data map[string]any) string {
	products := list(data, "products")
	if len(products) == 0 {
		return "没有搜到匹配的商品，可以换个关键词，例如「咖啡机」「空气炸锅」。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "找到 %d 个商品：\n", len(products))
	for i, raw := range products {
		p := asMap(raw)
		fmt.Fprintf(&b, "%d. %s（%s，%s）\n", i+1, str(p, "name"), str(p, "product_code"), str(p, "category"))
		if d := str(p, "description"); d != "" {
			fmt.Fprintf(&b, "   %s\n", truncate(d, 60))
		}
	}
	b.WriteString("\n想看哪个商品的规格和库存？告诉我名字即可。")
	return b.String()
}

func renderProductDetail(data map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s（%s）\n%s / %s\n", str(data, "name"), str(data, "product_code"),
		str(data, "brand"), str(data, "category"))
	if d := str(data, "description"); d != "" {
		fmt.Fprintf(&b, "%s\n", d)
	}
	for _, raw := range list(data, "skus") {
		s := asMap(raw)
		fmt.Fprintf(&b, "· %s %s，库存 %s 件（%s）\n", str(s, "sku_name"),
			moneyText(s["list_price"]), num(s["stock_quantity"]), str(s, "status"))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderArticles(data map[string]any) string {
	articles := list(data, "articles")
	if len(articles) == 0 {
		return "知识库里没有匹配的政策文章，可以换个关键词，或者我帮你转人工客服确认。"
	}
	var b strings.Builder
	for i, raw := range articles {
		a := asMap(raw)
		fmt.Fprintf(&b, "%d. %s\n", i+1, str(a, "title"))
		if c := str(a, "content"); c != "" {
			fmt.Fprintf(&b, "   %s\n", truncate(strings.Join(strings.Fields(c), " "), 120))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderBenefits(data map[string]any) string {
	benefits := list(data, "benefits")
	if len(benefits) == 0 {
		return "你名下暂时没有可用的优惠券或权益记录。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "你名下有 %d 条权益记录：\n", len(benefits))
	for _, raw := range benefits {
		item := asMap(raw)
		line := fmt.Sprintf("· %s %s（%s）", zhBenefit(str(item, "benefit_type")),
			str(item, "benefit_code"), str(item, "status"))
		if amt, ok := item["amount"]; ok {
			line += " 面额 " + moneyText(amt)
		}
		if pts, ok := item["points"]; ok {
			line += " 积分 " + num(pts)
		}
		if exp := str(item, "expires_at"); exp != "" {
			line += "，到期 " + shortTime(exp)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderPromotions(data map[string]any) string {
	promos := list(data, "promotions")
	if len(promos) == 0 {
		return "目前没有查到进行中的活动。"
	}
	var b strings.Builder
	for _, raw := range promos {
		p := asMap(raw)
		fmt.Fprintf(&b, "· %s %s\n", str(p, "promotion_code"), str(p, "title"))
		if d := str(p, "description"); d != "" {
			fmt.Fprintf(&b, "  %s\n", truncate(d, 80))
		}
		if end := str(p, "ends_at"); end != "" {
			fmt.Fprintf(&b, "  截止：%s\n", shortTime(end))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---- small helpers ----

func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch typed := v.(type) {
		case string:
			return typed
		case json.Number:
			return typed.String()
		}
	}
	return ""
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func list(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

func num(v any) string {
	switch typed := v.(type) {
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%.2f", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case string:
		return typed
	}
	return ""
}

func moneyText(v any) string {
	m := asMap(v)
	if m == nil {
		return ""
	}
	return str(m, "amount_display")
}

// shortTime renders "2026-10-18T09:12:00Z" as "2026-10-18 17:12" in local time is
// out of scope here; UTC keeps the demo deterministic and matches the API docs.
func shortTime(raw string) string {
	if len(raw) >= 16 {
		return strings.Replace(raw[:16], "T", " ", 1)
	}
	return raw
}

func compactJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func zhOrderStatus(s string) string {
	if zh, ok := map[string]string{
		"pending_payment": "待付款", "paid": "已付款待发货", "processing": "处理中",
		"shipped": "已发货", "completed": "已完成", "cancelled": "已取消", "closed": "已关闭",
	}[s]; ok {
		return zh
	}
	return s
}

func zhShipmentStatus(s string) string {
	if zh, ok := map[string]string{
		"pending": "待揽收", "picked_up": "已揽收", "in_transit": "运输中",
		"delivering": "派送中", "delivered": "已签收", "exception": "物流异常",
		"returned": "已退回", "intercepted": "已拦截", "delivery_failed": "派送失败",
	}[s]; ok {
		return zh
	}
	return s
}

func zhRefundStatus(s string) string {
	if zh, ok := map[string]string{
		"requested": "已申请待审核", "pending_review": "审核中", "approved": "审核通过",
		"processing": "退款处理中", "succeeded": "退款成功", "failed": "退款失败",
		"rejected": "已拒绝", "cancelled": "已取消",
	}[s]; ok {
		return zh
	}
	return s
}

func zhCaseType(s string) string {
	if zh, ok := map[string]string{
		"return": "退货", "exchange": "换货", "repair": "维修",
		"refund": "退款", "reship": "补发", "price_protection": "价保",
	}[s]; ok {
		return zh
	}
	return s
}

func zhCaseStatus(s string) string {
	if zh, ok := map[string]string{
		"submitted": "已提交", "pending_review": "审核中", "approved": "审核通过",
		"rejected": "已拒绝", "awaiting_customer": "等待用户操作",
		"awaiting_return_shipment": "等待寄回", "processing": "处理中",
		"completed": "已完成", "cancelled": "已取消",
	}[s]; ok {
		return zh
	}
	return s
}

func zhInvoiceType(s string) string {
	if zh, ok := map[string]string{
		"electronic": "电子普通发票", "vat_special": "增值税专用发票", "paper": "纸质发票",
	}[s]; ok {
		return zh
	}
	return s
}

func zhInvoiceStatus(s string) string {
	if zh, ok := map[string]string{
		"requested": "已申请待开票", "pending": "待开票", "issuing": "开票中",
		"issued": "已开票", "failed": "开票失败", "rejected": "开票失败", "cancelled": "已取消",
	}[s]; ok {
		return zh
	}
	return s
}

func zhAction(s string) string {
	if zh, ok := map[string]string{
		"cancel_order": "取消订单", "modify_order": "修改订单", "return": "退货",
		"exchange": "换货", "repair": "维修", "refund": "退款",
		"reship": "补发", "price_protection": "申请价保",
	}[s]; ok {
		return zh
	}
	return s
}

func zhPriority(s string) string {
	if zh, ok := map[string]string{
		"low": "低", "normal": "普通", "high": "高", "urgent": "紧急",
	}[s]; ok {
		return zh
	}
	return s
}

func zhBenefit(s string) string {
	if zh, ok := map[string]string{
		"coupon": "优惠券", "points": "积分", "freight_insurance": "运费险",
		"whitebar_status": "白条", "store_balance": "店铺余额", "other": "其他",
	}[s]; ok {
		return zh
	}
	return s
}
