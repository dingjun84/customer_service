package seed

import (
	"fmt"
	"time"
)

// Each order is a scripted business story, not a random combination of status
// columns. The dispatcher below performs the same actions a human operator
// would (pay, ship, deliver, cancel, open a case, refund, invoice), so status,
// money and logistics stay consistent by construction.

type orderSpec struct {
	kind      string
	cust      int
	addr      int
	h         float64 // hours before the data anchor that the order was created
	lines     []lineOpt
	disc      int64
	pay       string
	carrier   string
	signer    string
	item      int
	qty       int64
	note      string
	channel   string
	invoice   string
	cold      bool
	scheduled bool
	freeShip  bool
}

func ls(sku string, qty int64) []lineOpt { return []lineOpt{{sku: sku, qty: qty}} }

func ls2(a string, qa int64, b string, qb int64) []lineOpt {
	return []lineOpt{{sku: a, qty: qa}, {sku: b, qty: qb}}
}

func (g *gen) ago(hours float64) time.Time {
	return g.anchor.Add(-time.Duration(hours * float64(time.Hour)))
}

func (g *gen) buildOrders() {
	for _, sp := range orderSpecs() {
		g.story(sp)
	}
	for _, o := range g.orderDrafts {
		o.commit()
	}
}

func (g *gen) story(sp orderSpec) {
	at := g.ago(sp.h)
	pay := firstNonEmpty(sp.pay, "wechat_pay")
	o := g.newOrder(orderOpts{
		customer: sp.cust, addr: sp.addr, at: at, channel: sp.channel, lines: sp.lines,
		discount: sp.disc, freeShip: sp.freeShip, invoice: sp.invoice, remark: sp.note,
	})
	if o == nil {
		return
	}
	if sp.scheduled {
		o.row.ShippingMethod = "scheduled"
		o.row.RequestedDeliveryAt = timePtr(at.Add(72 * time.Hour))
	}

	switch sp.kind {
	case "unpaid":
		return

	case "payFailed":
		o.attempt(at.Add(4*time.Minute), pay, "failed", o.due, "MOCK-FAIL-"+o.row.No)
		if sp.note == "pending" {
			o.attempt(at.Add(14*time.Minute), "alipay", "pending", o.due, "MOCK-PENDING-"+o.row.No)
		}
		return

	case "paid":
		o.pay(at.Add(12*time.Minute), pay)
		return

	case "processing":
		o.pay(at.Add(12*time.Minute), pay)
		o.row.Status = "processing"
		return

	case "presale":
		o.row.Presale = true
		o.payDeposit(at.Add(8*time.Minute), pay, presaleDepositMinor)
		o.attempt(at.Add(15*time.Minute), "alipay", "pending", o.due-o.paid, "MOCK-BALANCE-"+o.row.No)
		return

	case "presaleBalance":
		o.row.Presale = true
		o.payDeposit(at.Add(8*time.Minute), pay, presaleDepositMinor)
		o.payParts(at.Add(70*time.Hour), payPart{method: "alipay", amount: o.due - o.paid})
		o.row.Status = "processing"
		return

	case "duplicateCharge":
		o.pay(at.Add(12*time.Minute), pay)
		// Second charge arrived from the provider and is awaiting reversal; it is
		// not part of the settled amount.
		o.attempt(at.Add(13*time.Minute), pay, "pending", o.due, "MOCK-DUP-REVERSAL-PENDING-"+o.row.No)
		return

	case "mixed":
		half := o.due / 2
		o.payParts(at.Add(12*time.Minute),
			payPart{method: "wechat_pay", amount: half},
			payPart{method: "account_balance", amount: o.due - half})
		return

	case "mixedAggregate":
		o.pay(at.Add(12*time.Minute), "mixed")
		return

	case "transit", "delivering", "deliveryFailed", "exception", "stalled":
		o.pay(at.Add(12*time.Minute), pay)
		shipAt := at.Add(20 * time.Hour)
		status := map[string]string{
			"transit": "in_transit", "delivering": "delivering",
			"deliveryFailed": "delivery_failed", "exception": "exception", "stalled": "in_transit",
		}[sp.kind]
		o.ship(shipOpts{
			carrier: sp.carrier, start: shipAt, status: status, note: sp.note, coldChain: sp.cold,
		})
		o.markShipped(shipAt)
		return

	case "multiParcel":
		o.pay(at.Add(12*time.Minute), pay)
		start := at.Add(20 * time.Hour)
		o.ship(shipOpts{carrier: "SF", start: start, status: "delivered", signer: "驿站"})
		o.ship(shipOpts{carrier: "YTO", start: start.Add(30 * time.Hour), status: "in_transit"})
		o.markShipped(start)
		return

	case "cancelled":
		o.cancel(at.Add(40*time.Minute), "用户取消订单")
		return

	case "cancelPaid":
		o.pay(at.Add(12*time.Minute), pay)
		o.cancel(at.Add(3*time.Hour), "发货前取消，全额退款")
		return

	case "intercept":
		o.pay(at.Add(12*time.Minute), pay)
		shipAt := at.Add(20 * time.Hour)
		o.ship(shipOpts{carrier: sp.carrier, start: shipAt, status: "intercepted", intercept: true})
		o.markShipped(shipAt)
		g.cancelAfterIntercept(o, shipAt.Add(30*time.Hour), "已发货拦截成功，订单取消")
		return

	case "codShipped":
		// Cash on delivery: nothing is collected yet, so paid_amount stays 0.
		o.row.Channel = "cod"
		o.attempt(at.Add(15*time.Minute), "cash_on_delivery", "pending", o.due, "MOCK-COD-"+o.row.No)
		shipAt := at.Add(20 * time.Hour)
		o.ship(shipOpts{carrier: "ZTO", start: shipAt, status: "in_transit"})
		o.markShipped(shipAt)
		o.row.Status = "shipped"
		return

	case "codDelivered":
		o.row.Channel = "cod"
		shipAt := at.Add(20 * time.Hour)
		sh := o.ship(shipOpts{carrier: "ZTO", start: shipAt, status: "delivered", signer: "本人"})
		o.markShipped(shipAt)
		collect := sh.row.LastEventAt.Add(20 * time.Minute)
		o.addPayment(collect, "cash_on_delivery", "succeeded", o.due, "MOCK-COD-COLLECT-"+o.row.No)
		o.paid = o.due
		o.row.Paid = o.paid
		o.row.PaidAt = timePtr(collect)
		o.deliver(*sh.row.LastEventAt)
		o.row.CompletedAt = timePtr(collect.Add(30 * time.Minute))
		return
	}

	// Everything below starts from a delivered order and then adds after-sales.
	g.deliverForward(o, pay, at, sp)
	if sp.invoice != "" && sp.kind != "invoiceRed" && sp.kind != "invoiceRejected" && sp.kind != "invoiceIssued" {
		o.invoice(invoiceOpts{
			typ: sp.invoice, status: "issued", title: invoiceTitle(sp), taxNo: invoiceTax(sp),
			email: "user@example.test", at: o.row.DeliveredAt.Add(20 * time.Hour),
		})
	}
	item := sp.item
	qty := sp.qty
	if qty == 0 {
		qty = 1
	}
	delivered := *o.row.DeliveredAt
	prorated := func() int64 {
		it := o.items[item]
		if it.qty <= 0 {
			return 0
		}
		return it.paid * qty / it.qty
	}

	switch sp.kind {
	case "delivered":
		return

	case "fullReturn":
		// Whole order returned: every line is refunded at its paid amount and the
		// shipping fee comes back too, so order.refunded equals order.paid and the
		// order closes.
		returnAt := delivered.Add(46 * time.Hour)
		o.ship(shipOpts{carrier: "SF", start: returnAt, status: "delivered", direction: "return"})
		for i := range o.items {
			it := o.items[i]
			c := o.newCase(caseOpts{
				typ: "return", status: "submitted", item: i, qty: it.qty,
				at: delivered.Add(20 * time.Hour), reasonCode: "no_reason",
				reasonText: "整单退货", desc: "商品完好，整单申请退货退款",
			})
			if c == nil {
				return
			}
			o.newRefund(refundOpts{
				amount: it.paid, status: "succeeded", at: returnAt.Add(28 * time.Hour),
				reason: "整单退货退款", caseDraft: c,
			})
			c.close("completed", returnAt.Add(32*time.Hour), fmt.Sprintf("已退款 %s", formatYuan(it.paid)))
		}
		if o.row.Shipping > 0 {
			o.newRefund(refundOpts{
				amount: o.row.Shipping, status: "succeeded", at: returnAt.Add(34 * time.Hour),
				reason: "整单退货，退还运费", compensation: true,
			})
		}
		for _, inv := range o.invoices {
			if inv.Status == "issued" {
				inv.Status = "red_flushed"
				inv.UpdatedAt = returnAt.Add(36 * time.Hour)
			}
		}
		return

	case "returnDone":
		c := o.newCase(caseOpts{
			typ: "return", status: "submitted", item: item, qty: qty, at: delivered.Add(20 * time.Hour),
			reasonCode: firstNonEmpty(sp.note, "no_reason"), reasonText: "尺码/颜色不合适",
			desc: "商品未使用，吊牌完整，申请退货",
		})
		if c == nil {
			return
		}
		returnAt := delivered.Add(50 * time.Hour)
		o.ship(shipOpts{carrier: "SF", start: returnAt, status: "delivered", direction: "return"})
		c.close("processing", returnAt.Add(28*time.Hour), "仓库已签收，质检中")
		o.newRefund(refundOpts{
			amount: prorated(), status: "succeeded", at: returnAt.Add(30 * time.Hour),
			reason: "退货退款", caseDraft: c,
		})
		c.close("completed", returnAt.Add(34*time.Hour), fmt.Sprintf("已退款 %s", formatYuan(prorated())))

	case "returnPartial":
		c := o.newCase(caseOpts{
			typ: "return", status: "submitted", item: item, qty: qty, at: delivered.Add(18 * time.Hour),
			reasonCode: "no_reason", reasonText: "只退其中一件",
			desc: "多件订单中有一件不合适，其余保留",
		})
		if c == nil {
			return
		}
		returnAt := delivered.Add(44 * time.Hour)
		o.ship(shipOpts{carrier: "YTO", start: returnAt, status: "delivered", direction: "return"})
		o.newRefund(refundOpts{
			amount: prorated(), status: "succeeded", at: returnAt.Add(26 * time.Hour),
			reason: "部分退货退款", caseDraft: c,
		})
		c.close("completed", returnAt.Add(30*time.Hour), fmt.Sprintf("部分退款 %s", formatYuan(prorated())))

	case "refundProcessing":
		c := o.newCase(caseOpts{
			typ: "refund", status: "processing", item: item, qty: qty, at: delivered.Add(20 * time.Hour),
			reasonCode: "quality_issue", reasonText: "商品故障", desc: firstNonEmpty(sp.note, "使用后出现故障，申请退款"),
		})
		if c == nil {
			return
		}
		o.newRefund(refundOpts{
			amount: prorated(), status: firstNonEmpty(sp.note, "processing"), at: delivered.Add(26 * time.Hour),
			reason: "商品故障退款，审核中", caseDraft: c,
		})
		c.close("processing", delivered.Add(30*time.Hour), "审核通过，退款处理中")

	case "refundRejected":
		c := o.newCase(caseOpts{
			typ: "refund", status: "pending_review", item: item, qty: qty, at: delivered.Add(20 * time.Hour),
			reasonCode: "no_reason", reasonText: "不想要了", desc: firstNonEmpty(sp.note, "商品已拆封使用，申请仅退款"),
		})
		if c == nil {
			return
		}
		o.newRefund(refundOpts{
			amount: prorated(), status: "rejected", at: delivered.Add(24 * time.Hour),
			reason: "仅退款申请", failure: "商品已拆封使用，不符合仅退款条件", caseDraft: c,
		})
		c.close("rejected", delivered.Add(30*time.Hour), "商品已拆封使用，不支持仅退款")

	case "refundFailed":
		c := o.newCase(caseOpts{
			typ: "return", status: "submitted", item: item, qty: qty, at: delivered.Add(20 * time.Hour),
			reasonCode: "no_reason", reasonText: "不喜欢", desc: "商品完好，申请退货退款",
		})
		if c == nil {
			return
		}
		returnAt := delivered.Add(46 * time.Hour)
		o.ship(shipOpts{carrier: "SF", start: returnAt, status: "delivered", direction: "return"})
		o.newRefund(refundOpts{
			amount: prorated(), status: "failed", at: returnAt.Add(28 * time.Hour),
			reason: "退货退款", failure: "原支付银行卡已销户，退回失败", caseDraft: c,
		})
		o.newRefund(refundOpts{
			amount: prorated(), status: "processing", at: returnAt.Add(32 * time.Hour),
			reason: "换卡重试退款，待渠道处理", caseDraft: c,
		})
		c.close("processing", returnAt.Add(33*time.Hour), "仓库已收货，退款重试中")

	case "reship":
		c := o.newCase(caseOpts{
			typ: "reship", status: "approved", item: item, qty: qty, at: delivered.Add(16 * time.Hour),
			reasonCode: "missing_part", reasonText: "少件/缺配件",
			desc: firstNonEmpty(sp.note, "开箱发现缺少配件，申请补发"),
		})
		if c == nil {
			return
		}
		reshipAt := delivered.Add(30 * time.Hour)
		o.ship(shipOpts{carrier: "JD", start: reshipAt, status: "delivered", direction: "reship", signer: "本人"})
		c.close("completed", reshipAt.Add(34*time.Hour), "补发包裹已签收")

	case "exchange":
		c := o.newCase(caseOpts{
			typ: "exchange", status: "approved", item: item, qty: qty, at: delivered.Add(18 * time.Hour),
			reasonCode: "size_issue", reasonText: "尺码不合适", desc: "换同款大一码",
		})
		if c == nil {
			return
		}
		returnAt := delivered.Add(40 * time.Hour)
		o.ship(shipOpts{carrier: "YTO", start: returnAt, status: "picked_up", direction: "return"})
		o.ship(shipOpts{carrier: "YTO", start: returnAt.Add(48 * time.Hour), status: "pending", direction: "reship"})
		c.close("awaiting_return_shipment", returnAt.Add(4*time.Hour), "退件已寄出，等待仓库签收")

	case "repair":
		c := o.newCase(caseOpts{
			typ: "repair", status: "processing", item: item, qty: qty, at: delivered.Add(30 * time.Hour),
			reasonCode: "quality_issue", reasonText: "商品故障",
			desc: firstNonEmpty(sp.note, "通电后无法启动，申请维修"),
		})
		if c == nil {
			return
		}
		c.close("processing", delivered.Add(36*time.Hour), "已寄至维修点，检测中")

	case "safety":
		c := o.newCase(caseOpts{
			typ: "repair", status: "processing", item: item, qty: qty, at: delivered.Add(10 * time.Hour),
			reasonCode: "safety_issue", reasonText: "安全隐患（漏电/冒烟）",
			desc: "使用时出现异味并冒烟，已断电停用，要求换新或退款",
		})
		if c == nil {
			return
		}
		c.close("processing", delivered.Add(12*time.Hour), "安全故障升级，转人工专员处理")

	case "customRejected":
		c := o.newCase(caseOpts{
			typ: "return", status: "pending_review", item: item, qty: qty, at: delivered.Add(22 * time.Hour),
			reasonCode: "customized_no_return", reasonText: "不想要了",
			desc: "刻字商品，收货后申请退货",
		})
		if c == nil {
			return
		}
		c.close("rejected", delivered.Add(30*time.Hour), "定制商品不支持无理由退货，可协助转卖或付费维修")

	case "overWindow":
		c := o.newCase(caseOpts{
			typ: "return", status: "pending_review", item: item, qty: qty, at: delivered.Add(24 * time.Hour),
			reasonCode: "out_of_window", reasonText: "不想要了",
			desc: "签收超过 7 天后申请无理由退货", noWindow: true,
		})
		if c == nil {
			return
		}
		c.close("rejected", delivered.Add(30*time.Hour), "已超过 7 天无理由退货期限，不予受理")

	case "refundRequested":
		c := o.newCase(caseOpts{
			typ: "refund", status: "submitted", item: item, qty: qty, at: delivered.Add(12 * time.Hour),
			reasonCode: "quality_issue", reasonText: "商品有问题", desc: firstNonEmpty(sp.note, "刚提交仅退款申请，等待审核"),
		})
		if c == nil {
			return
		}
		o.newRefund(refundOpts{
			amount: prorated(), status: "requested", at: delivered.Add(14 * time.Hour),
			reason: "仅退款申请已提交", caseDraft: c,
		})

	case "refundApproved":
		c := o.newCase(caseOpts{
			typ: "return", status: "awaiting_return_shipment", item: item, qty: qty,
			at: delivered.Add(20 * time.Hour), reasonCode: "no_reason", reasonText: "不喜欢",
			desc: "退货申请已通过，等待用户寄回",
		})
		if c == nil {
			return
		}
		o.newRefund(refundOpts{
			amount: prorated(), status: "approved", at: delivered.Add(26 * time.Hour),
			reason: "审核通过，待收到退货后打款", caseDraft: c,
		})

	case "merchantRefuse":
		c := o.newCase(caseOpts{
			typ: "return", status: "awaiting_return_shipment", item: item, qty: qty,
			at: delivered.Add(20 * time.Hour), reasonCode: "no_reason", reasonText: "不喜欢",
			desc: "申请退货并已寄回",
		})
		if c == nil {
			return
		}
		returnAt := delivered.Add(44 * time.Hour)
		o.ship(shipOpts{carrier: "SF", start: returnAt, status: "delivered", direction: "return"})
		c.close("rejected", returnAt.Add(30*time.Hour),
			"商家收到退货后发现吊牌已剪、有明显穿着痕迹，不符合二次销售条件，拒收原路退回")

	case "freshPartial":
		c := o.newCase(caseOpts{
			typ: "refund", status: "processing", item: item, qty: qty, at: delivered.Add(6 * time.Hour),
			reasonCode: "fresh_damaged", reasonText: "坏果/变质",
			desc: "签收后发现部分果肉发霉，按要求提供商品与面单同框照片",
		})
		if c == nil {
			return
		}
		amount := o.items[item].paid * 40 / 100
		o.newRefund(refundOpts{
			amount: amount, status: "succeeded", at: delivered.Add(10 * time.Hour),
			reason: "生鲜坏果按比例补偿", caseDraft: c, compensation: true,
		})
		c.close("completed", delivered.Add(14*time.Hour), fmt.Sprintf("按 40%% 比例补偿 %s", formatYuan(amount)))

	case "priceProtect":
		c := o.newCase(caseOpts{
			typ: "price_protection", status: "processing", item: item, qty: qty,
			at: delivered.Add(16 * time.Hour), reasonCode: "price_drop", reasonText: "降价补差",
			desc: "同款商品在价保窗口内降价，申请差价",
		})
		if c == nil {
			return
		}
		amount := int64(5000) * qty
		o.newRefund(refundOpts{
			amount: amount, status: "succeeded", at: delivered.Add(20 * time.Hour),
			reason: "价保差价补偿", caseDraft: c, compensation: true,
		})
		c.close("completed", delivered.Add(26*time.Hour), fmt.Sprintf("价保成立，补差 %s", formatYuan(amount)))

	case "priceProtectOld":
		c := o.newCase(caseOpts{
			typ: "price_protection", status: "pending_review", item: item, qty: qty,
			at: delivered.Add(20 * time.Hour), reasonCode: "out_of_window", reasonText: "价保申请",
			desc: "用户引用旧价保规则（7 天）要求补差", noWindow: true,
		})
		if c == nil {
			return
		}
		c.close("rejected", delivered.Add(26*time.Hour), "已超过 15 天价保窗口，且当前无生效降价活动")

	case "lost":
		c := o.newCase(caseOpts{
			typ: "refund", status: "pending_review", item: item, qty: qty,
			at: delivered.Add(4 * time.Hour), reasonCode: "not_received", reasonText: "显示签收未收到",
			desc: "物流显示驿站签收，但用户与驿站均未找到包裹",
		})
		if c == nil {
			return
		}
		o.newRefund(refundOpts{
			amount: o.paid, status: "pending_review", at: delivered.Add(6 * time.Hour),
			reason: "未收到货核查期间冻结退款", caseDraft: c,
		})
		c.close("processing", delivered.Add(8*time.Hour), "已向承运商发起丢件核查，24 小时内回复")

	case "invoiceIssued":
		o.invoice(invoiceOpts{
			typ: firstNonEmpty(sp.invoice, "electronic"), status: "issued", title: invoiceTitle(sp),
			taxNo: invoiceTax(sp), email: "user@example.test", at: delivered.Add(20 * time.Hour),
		})

	case "invoiceRed":
		inv := o.invoice(invoiceOpts{
			typ: "electronic", status: "issued", title: "测试市示例科技有限公司",
			taxNo: "TESTTAX0000000001", email: "user@example.test", at: delivered.Add(18 * time.Hour),
		})
		c := o.newCase(caseOpts{
			typ: "return", status: "submitted", item: item, qty: qty, at: delivered.Add(30 * time.Hour),
			reasonCode: "no_reason", reasonText: "退货并重开发票", desc: "部分退货，需要红冲后重开差额发票",
		})
		if c == nil || inv == nil {
			return
		}
		returnAt := delivered.Add(54 * time.Hour)
		o.ship(shipOpts{carrier: "SF", start: returnAt, status: "delivered", direction: "return"})
		o.newRefund(refundOpts{
			amount: prorated(), status: "succeeded", at: returnAt.Add(28 * time.Hour),
			reason: "部分退货退款", caseDraft: c,
		})
		c.close("completed", returnAt.Add(32*time.Hour), "部分退货完成，已红冲原发票")
		inv.Status = "red_flushed"
		inv.UpdatedAt = returnAt.Add(34 * time.Hour)
		o.invoice(invoiceOpts{
			typ: "electronic", status: "issued", title: "测试市示例科技有限公司",
			taxNo: "TESTTAX0000000001", email: "user@example.test",
			at: returnAt.Add(36 * time.Hour), suffix: "-R1",
		})

	case "invoiceRejected":
		o.invoice(invoiceOpts{
			typ: "vat_special", status: "rejected", title: "测试市示例商贸有限公司",
			taxNo: "TESTTAX-BAD", email: "finance@example.test", at: delivered.Add(20 * time.Hour),
		})
	}
}

// deliverForward pays, ships one parcel and delivers it, which also completes
// the order. delivered_at comes from the parcel's delivered event, never from a
// separately supplied value.
func (g *gen) deliverForward(o *orderDraft, pay string, at time.Time, sp orderSpec) {
	o.pay(at.Add(12*time.Minute), pay)
	shipAt := at.Add(20 * time.Hour)
	sh := o.ship(shipOpts{
		carrier: sp.carrier, start: shipAt, status: "delivered",
		signer: firstNonEmpty(sp.signer, "本人"), coldChain: sp.cold,
	})
	if sh == nil {
		return
	}
	o.markShipped(shipAt)
	o.deliver(*sh.row.LastEventAt)
}

// cancelAfterIntercept closes an order whose parcel was stopped in transit.
// The parcel must already be intercepted, otherwise cancel() would have to lie
// about "nothing was picked up".
func (g *gen) cancelAfterIntercept(o *orderDraft, at time.Time, reason string) {
	for _, sh := range o.shipments {
		if sh.row.Status != "intercepted" && sh.row.Status != "returned" && sh.row.Status != "pending" {
			o.fail("cancel after intercept: parcel %s is %s", sh.row.No, sh.row.Status)
			return
		}
	}
	o.row.Status = "cancelled"
	o.row.CancelledAt = timePtr(at)
	o.row.UpdatedAt = at
	if o.paid > o.row.Refunded {
		o.newRefund(refundOpts{
			amount: o.paid - o.row.Refunded, status: "succeeded", at: at,
			reason: firstNonEmpty(reason, "拦截成功，全额退款"), noCase: true,
		})
	}
}

func formatYuan(minor int64) string {
	return fmt.Sprintf("¥%d.%02d", minor/100, minor%100)
}

func invoiceTitle(sp orderSpec) string {
	if sp.invoice == "vat_special" {
		return "测试市示例科技有限公司"
	}
	return "测试市示例科技有限公司"
}

func invoiceTax(sp orderSpec) string {
	if sp.invoice == "vat_special" || sp.invoice == "" {
		return "TESTTAX0000000001"
	}
	return ""
}
