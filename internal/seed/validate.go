package seed

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Report is the outcome of one validation pass.
type Report struct {
	Checks     int
	Violations int
	issues     []string
}

func (r *Report) must(ok bool, format string, args ...any) {
	r.Checks++
	if ok {
		return
	}
	r.Violations++
	r.issues = append(r.issues, fmt.Sprintf(format, args...))
}

// Messages returns the human readable violations, sorted for stable output.
func (r *Report) Messages() []string {
	out := append([]string{}, r.issues...)
	sort.Strings(out)
	return out
}

// validateDataset checks every business invariant on the in-memory dataset.
// It runs before the import transaction, so a violation means nothing is written.
func validateDataset(ds *dataset, anchor time.Time) *Report {
	r := &Report{}
	v := &validator{ds: ds, anchor: anchor, r: r}
	v.index()
	v.relations()
	v.money()
	v.ordersState()
	v.shipmentsTrack()
	v.casesState()
	v.invoicesState()
	v.conversations()
	v.catalog()
	return r
}

type validator struct {
	ds     *dataset
	anchor time.Time
	r      *Report

	customers       map[string]*customerRow
	products        map[string]*productRow
	skus            map[string]*skuRow
	orders          map[string]*orderRow
	orderByNo       map[string]*orderRow
	items           map[string]*itemRow
	itemsByOrder    map[string][]*itemRow
	payments        map[string]*paymentRow
	paysByOrder     map[string][]*paymentRow
	refunds         map[string]*refundRow
	cases           map[string]*caseRow
	caseByOrder     map[string][]*caseRow
	shipments       map[string]*shipmentRow
	shipByOrder     map[string][]*shipmentRow
	events          map[string][]*eventRow
	invoices        map[string]*invoiceRow
	invoicesByOrder map[string][]*invoiceRow
	convs           map[string]*conversationRow
	msgs            map[string][]*messageRow
	msgsByID        map[string]*messageRow
	tools           map[string]*toolExecRow
}

func (v *validator) index() {
	v.customers = map[string]*customerRow{}
	v.products = map[string]*productRow{}
	v.skus = map[string]*skuRow{}
	v.orders = map[string]*orderRow{}
	v.orderByNo = map[string]*orderRow{}
	v.items = map[string]*itemRow{}
	v.itemsByOrder = map[string][]*itemRow{}
	v.payments = map[string]*paymentRow{}
	v.paysByOrder = map[string][]*paymentRow{}
	v.refunds = map[string]*refundRow{}
	v.cases = map[string]*caseRow{}
	v.caseByOrder = map[string][]*caseRow{}
	v.shipments = map[string]*shipmentRow{}
	v.shipByOrder = map[string][]*shipmentRow{}
	v.events = map[string][]*eventRow{}
	v.invoices = map[string]*invoiceRow{}
	v.invoicesByOrder = map[string][]*invoiceRow{}
	v.convs = map[string]*conversationRow{}
	v.msgs = map[string][]*messageRow{}
	v.msgsByID = map[string]*messageRow{}
	v.tools = map[string]*toolExecRow{}

	for i := range v.ds.customers {
		c := &v.ds.customers[i]
		v.customers[c.ID] = c
	}
	for i := range v.ds.products {
		p := &v.ds.products[i]
		v.products[p.ID] = p
	}
	for i := range v.ds.skus {
		s := &v.ds.skus[i]
		v.skus[s.ID] = s
	}
	for i := range v.ds.orders {
		o := &v.ds.orders[i]
		v.orders[o.ID] = o
		v.orderByNo[o.No] = o
	}
	for i := range v.ds.items {
		it := &v.ds.items[i]
		v.items[it.ID] = it
		v.itemsByOrder[it.OrderID] = append(v.itemsByOrder[it.OrderID], it)
	}
	for i := range v.ds.payments {
		p := &v.ds.payments[i]
		v.payments[p.ID] = p
		v.paysByOrder[p.OrderID] = append(v.paysByOrder[p.OrderID], p)
	}
	for i := range v.ds.refunds {
		rf := &v.ds.refunds[i]
		v.refunds[rf.ID] = rf
	}
	for i := range v.ds.cases {
		c := &v.ds.cases[i]
		v.cases[c.ID] = c
		v.caseByOrder[c.OrderID] = append(v.caseByOrder[c.OrderID], c)
	}
	for i := range v.ds.shipments {
		sh := &v.ds.shipments[i]
		v.shipments[sh.ID] = sh
		v.shipByOrder[sh.OrderID] = append(v.shipByOrder[sh.OrderID], sh)
	}
	for i := range v.ds.events {
		e := &v.ds.events[i]
		v.events[e.ShipmentID] = append(v.events[e.ShipmentID], e)
	}
	for i := range v.ds.invoices {
		inv := &v.ds.invoices[i]
		v.invoices[inv.ID] = inv
		v.invoicesByOrder[inv.OrderID] = append(v.invoicesByOrder[inv.OrderID], inv)
	}
	for i := range v.ds.conversations {
		c := &v.ds.conversations[i]
		v.convs[c.ID] = c
	}
	for i := range v.ds.messages {
		m := &v.ds.messages[i]
		v.msgs[m.ConvID] = append(v.msgs[m.ConvID], m)
		v.msgsByID[m.ID] = m
	}
	for i := range v.ds.toolExecs {
		t := &v.ds.toolExecs[i]
		v.tools[t.ToolCallID] = t
	}
}

func (v *validator) relations() {
	r := v.r
	for _, o := range v.ds.orders {
		_, ok := v.customers[o.CustomerID]
		r.must(ok, "订单 %s 的客户 %s 不存在", o.No, o.CustomerID)
		r.must(len(v.itemsByOrder[o.ID]) > 0, "订单 %s 没有商品行", o.No)
	}
	for _, it := range v.ds.items {
		o, ok := v.orders[it.OrderID]
		r.must(ok, "订单商品行 %s 指向不存在的订单", it.SKUCode)
		if ok {
			r.must(it.CreatedAt.Equal(o.CreatedAt), "商品行 %s 的创建时间与订单 %s 不一致", it.SKUCode, o.No)
		}
		if sku, ok := v.skus[it.SKUID]; ok {
			r.must(sku.ProductID == it.ProductID, "商品行 %s 的 SKU 与商品不匹配", it.SKUCode)
			r.must(sku.Code == it.SKUCode, "商品行 %s 的 SKU 编码快照与目录不一致", it.SKUCode)
		} else {
			r.must(false, "商品行 %s 引用了不存在的 SKU", it.SKUCode)
		}
	}
	for _, p := range v.ds.payments {
		_, ok := v.orders[p.OrderID]
		r.must(ok, "支付 %s 指向不存在的订单", p.No)
	}
	for _, rf := range v.ds.refunds {
		o, ok := v.orders[rf.OrderID]
		r.must(ok, "退款 %s 指向不存在的订单", rf.No)
		if rf.PaymentID != "" {
			pay, ok := v.payments[rf.PaymentID]
			r.must(ok, "退款 %s 指向不存在的支付记录", rf.No)
			if ok && o != nil {
				r.must(pay.OrderID == o.ID, "退款 %s 关联的支付记录属于其他订单", rf.No)
			}
		}
		if rf.CaseID != "" {
			cs, ok := v.cases[rf.CaseID]
			r.must(ok, "退款 %s 指向不存在的售后单", rf.No)
			if ok && o != nil {
				r.must(cs.OrderID == o.ID, "退款 %s 关联的售后单属于其他订单", rf.No)
				r.must(cs.CustomerID == o.CustomerID, "退款 %s 关联的售后单客户不一致", rf.No)
			}
		}
	}
	for _, c := range v.ds.cases {
		o, ok := v.orders[c.OrderID]
		r.must(ok, "售后单 %s 指向不存在的订单", c.No)
		if !ok {
			continue
		}
		r.must(c.CustomerID == o.CustomerID, "售后单 %s 的客户与订单不一致（串号）", c.No)
		if c.OrderItemID != "" {
			it, ok := v.items[c.OrderItemID]
			r.must(ok, "售后单 %s 指向不存在的订单商品行", c.No)
			if ok {
				r.must(it.OrderID == o.ID, "售后单 %s 引用了其他订单的商品行（%s）", c.No, it.OrderID)
			}
		}
	}
	for _, sh := range v.ds.shipments {
		o, ok := v.orders[sh.OrderID]
		r.must(ok, "包裹 %s 指向不存在的订单", sh.No)
		if ok {
			r.must(sh.CreatedAt.After(o.CreatedAt) || sh.CreatedAt.Equal(o.CreatedAt),
				"包裹 %s 创建时间早于订单 %s", sh.No, o.No)
		}
	}
	for _, e := range v.ds.events {
		_, ok := v.shipments[e.ShipmentID]
		r.must(ok, "轨迹事件 %s 指向不存在的包裹", e.Code)
	}
	for _, inv := range v.ds.invoices {
		o, ok := v.orders[inv.OrderID]
		r.must(ok, "发票 %s 指向不存在的订单", inv.No)
		if ok {
			r.must(inv.CustomerID == o.CustomerID, "发票 %s 的客户与订单不一致", inv.No)
		}
	}
	for _, m := range v.ds.messages {
		_, ok := v.convs[m.ConvID]
		r.must(ok, "消息 %d 指向不存在的会话", m.Sequence)
	}
	for _, t := range v.ds.toolExecs {
		_, ok := v.convs[t.ConvID]
		r.must(ok, "工具执行 %s 指向不存在的会话", t.ToolCallID)
		if t.MessageID != "" {
			m, ok := v.msgsByID[t.MessageID]
			r.must(ok, "工具执行 %s 指向不存在的消息", t.ToolCallID)
			if ok {
				r.must(m.ConvID == t.ConvID, "工具执行 %s 的消息属于其他会话", t.ToolCallID)
			}
		}
	}
	for _, t := range v.ds.tickets {
		if t.OrderID != "" {
			_, ok := v.orders[t.OrderID]
			r.must(ok, "工单 %s 指向不存在的订单", t.No)
		}
		if t.ConvID != "" {
			_, ok := v.convs[t.ConvID]
			r.must(ok, "工单 %s 指向不存在的会话", t.No)
		}
	}
	for _, p := range v.ds.pending {
		_, ok := v.orders[p.TargetID]
		r.must(ok, "待确认动作 %s 指向不存在的订单", p.ActionNo)
		_, cok := v.customers[p.CustomerID]
		r.must(cok, "待确认动作 %s 指向不存在的客户", p.ActionNo)
	}
}

func (v *validator) money() {
	r := v.r
	for _, o := range v.ds.orders {
		items := v.itemsByOrder[o.ID]
		var itemAmount, itemDiscount, itemPaid int64
		for _, it := range items {
			line := it.UnitPrice * it.Quantity
			r.must(line == it.UnitPrice*it.Quantity, "商品行 %s 金额异常", it.SKUCode)
			r.must(it.Paid == line-it.Discount, "商品行 %s 实付不等于成交额减分摊折扣", it.SKUCode)
			r.must(it.Discount <= line, "商品行 %s 分摊折扣超过成交额", it.SKUCode)
			r.must(it.RefundedQty <= it.Quantity, "商品行 %s 已退数量超过购买数量", it.SKUCode)
			itemAmount += line
			itemDiscount += it.Discount
			itemPaid += it.Paid
		}
		r.must(o.ItemAmount == itemAmount, "订单 %s 商品金额 %d 不等于行合计 %d", o.No, o.ItemAmount, itemAmount)
		r.must(o.Discount == itemDiscount, "订单 %s 折扣 %d 不等于行分摊合计 %d（分摊不闭合）", o.No, o.Discount, itemDiscount)
		r.must(o.Shipping == 0 || o.Shipping == standardShippingFee, "订单 %s 运费 %d 不符合规则", o.No, o.Shipping)
		if o.Shipping > 0 {
			r.must(o.ItemAmount-o.Discount < freeShippingThreshold, "订单 %s 已满包邮门槛却收取运费", o.No)
		}
		if !o.Presale && o.Paid > 0 {
			r.must(o.Paid == itemPaid+o.Shipping, "订单 %s 实付 %d 不等于行实付 %d + 运费 %d", o.No, o.Paid, itemPaid, o.Shipping)
		}

		var settled, settledCount int64
		var pendingAttempts int64
		for _, p := range v.paysByOrder[o.ID] {
			switch p.Status {
			case "succeeded", "partially_refunded", "refunded":
				settled += p.Amount
				settledCount++
			case "pending":
				pendingAttempts++
			}
			r.must(p.Amount > 0, "订单 %s 存在金额为 0 的支付记录 %s", o.No, p.No)
			if p.Status == "succeeded" && p.PaidAt == nil {
				r.must(false, "支付 %s 状态为成功但缺少支付时间", p.No)
			}
			if p.Status == "pending" && p.PaidAt != nil {
				r.must(false, "支付 %s 尚未成功却写入了支付时间", p.No)
			}
		}
		if o.Channel != "cod" {
			r.must(o.Paid == settled, "订单 %s 实付 %d 不等于成功支付合计 %d", o.No, o.Paid, settled)
		} else {
			r.must(settled == o.Paid, "货到付款订单 %s 的已收金额与支付记录不一致", o.No)
		}
		if o.Status == "pending_payment" {
			r.must(settled == 0 || o.Presale, "未支付订单 %s 存在成功支付记录", o.No)
		}

		var refunded int64
		var outstanding int64
		for _, rf := range v.ds.refunds {
			if rf.OrderID != o.ID {
				continue
			}
			r.must(rf.Amount > 0, "退款 %s 金额必须大于 0", rf.No)
			r.must(rf.RequestedAt.Before(anchorPlus(v.anchor)), "退款 %s 的申请时间在未来", rf.No)
			switch rf.Status {
			case "succeeded":
				refunded += rf.Amount
				r.must(rf.CompletedAt != nil, "退款 %s 已成功但缺少完成时间", rf.No)
			case "failed", "rejected", "cancelled":
				r.must(rf.CompletedAt == nil, "退款 %s 未成功却写入了完成时间", rf.No)
				r.must(rf.FailureReason != "", "退款 %s 失败/拒绝但缺少可解释原因", rf.No)
			default:
				outstanding += rf.Amount
			}
			if rf.CompletedAt != nil {
				r.must(!rf.CompletedAt.Before(rf.RequestedAt), "退款 %s 完成时间早于申请时间", rf.No)
			}
		}
		r.must(o.Refunded == refunded, "订单 %s 已退金额 %d 不等于成功退款合计 %d", o.No, o.Refunded, refunded)
		r.must(o.Refunded <= o.Paid, "订单 %s 已退金额 %d 超过实付 %d", o.No, o.Refunded, o.Paid)
		r.must(refunded+outstanding <= o.Paid, "订单 %s 退款合计（含处理中）超过实付", o.No)

		var payRefunded int64
		for _, p := range v.paysByOrder[o.ID] {
			r.must(p.Refunded <= p.Amount, "支付 %s 的已退金额超过支付金额", p.No)
			switch p.Status {
			case "refunded":
				r.must(p.Refunded == p.Amount, "支付 %s 标记为已全额退款但金额不符", p.No)
			case "partially_refunded":
				r.must(p.Refunded > 0 && p.Refunded < p.Amount, "支付 %s 的退款状态与金额不符", p.No)
			case "succeeded":
				r.must(p.Refunded == 0, "支付 %s 状态为成功但已有退款金额", p.No)
			}
			payRefunded += p.Refunded
		}
		r.must(payRefunded == o.Refunded, "订单 %s 支付记录退款合计 %d 与订单已退 %d 不一致", o.No, payRefunded, o.Refunded)
		_ = settledCount
		_ = pendingAttempts
	}
}

func anchorPlus(a time.Time) time.Time { return a.Add(time.Second) }

func (v *validator) ordersState() {
	r := v.r
	for _, o := range v.ds.orders {
		shipments := v.shipByOrder[o.ID]
		paidSettled := false
		for _, p := range v.paysByOrder[o.ID] {
			if p.Status == "succeeded" || p.Status == "partially_refunded" || p.Status == "refunded" {
				paidSettled = true
			}
		}

		r.must(!o.CreatedAt.After(v.anchor), "订单 %s 创建时间在未来", o.No)
		if o.PaidAt != nil {
			r.must(!o.PaidAt.Before(o.CreatedAt), "订单 %s 支付时间早于下单时间", o.No)
			r.must(!o.PaidAt.After(v.anchor), "订单 %s 支付时间在未来", o.No)
		}
		if o.ShippedAt != nil {
			r.must(o.PaidAt != nil || o.Channel == "cod", "订单 %s 未支付却已发货", o.No)
			r.must(o.Channel == "cod" || o.PaidAt == nil || !o.ShippedAt.Before(*o.PaidAt),
				"订单 %s 发货时间早于支付时间", o.No)
			r.must(!o.ShippedAt.After(v.anchor), "订单 %s 发货时间在未来", o.No)
		}
		if o.DeliveredAt != nil {
			r.must(o.ShippedAt != nil, "订单 %s 有签收时间却没有发货时间", o.No)
			r.must(o.ShippedAt == nil || !o.DeliveredAt.Before(*o.ShippedAt), "订单 %s 签收时间早于发货时间", o.No)
			r.must(!o.DeliveredAt.After(v.anchor), "订单 %s 签收时间在未来", o.No)
		}
		if o.CompletedAt != nil {
			r.must(o.DeliveredAt != nil, "订单 %s 已完单却没有签收记录", o.No)
			r.must(o.DeliveredAt == nil || !o.CompletedAt.Before(*o.DeliveredAt), "订单 %s 完单时间早于签收时间", o.No)
		}
		if o.CancelledAt != nil {
			r.must(!o.CancelledAt.Before(o.CreatedAt), "订单 %s 取消时间早于下单时间", o.No)
		}

		switch o.Status {
		case "pending_payment":
			r.must(!paidSettled || o.Presale, "待付款订单 %s 存在已结算支付", o.No)
			r.must(o.ShippedAt == nil, "待付款订单 %s 却已发货", o.No)
			r.must(len(v.invoicesByOrder[o.ID]) == 0, "待付款订单 %s 已开票", o.No)
			if o.Presale {
				r.must(o.Paid > 0 && o.Paid < o.ItemAmount-o.Discount+o.Shipping,
					"预售订单 %s 的已付金额应介于定金与应付总额之间", o.No)
				var hasDeposit bool
				for _, p := range v.paysByOrder[o.ID] {
					if p.Status == "succeeded" && p.Amount == presaleDepositMinor {
						hasDeposit = true
					}
				}
				r.must(hasDeposit, "预售订单 %s 缺少定金支付记录", o.No)
				r.must(len(shipments) == 0, "预售订单 %s 尾款未付却已生成包裹", o.No)
			} else {
				r.must(o.Paid == 0, "待付款订单 %s 的实付金额不为 0", o.No)
			}
		case "paid", "processing":
			r.must(o.PaidAt != nil, "已付款订单 %s 缺少支付时间", o.No)
			r.must(len(shipments) == 0 || allPending(shipments), "待发货订单 %s 已有揽收记录", o.No)
			r.must(o.ShippedAt == nil, "待发货订单 %s 已有发货时间", o.No)
		case "shipped":
			r.must(o.ShippedAt != nil, "已发货订单 %s 缺少发货时间", o.No)
			r.must(len(shipments) > 0, "已发货订单 %s 没有任何包裹", o.No)
			r.must(o.DeliveredAt == nil, "运输中订单 %s 已有签收时间", o.No)
			var hasMoving bool
			for _, sh := range shipments {
				switch sh.Status {
				case "picked_up", "in_transit", "delivering", "exception", "delivery_failed":
					hasMoving = true
				}
			}
			r.must(hasMoving, "运输中订单 %s 没有在途包裹", o.No)
		case "completed", "closed":
			r.must(o.DeliveredAt != nil, "已完成订单 %s 缺少签收时间", o.No)
			r.must(len(shipments) > 0, "已完成订单 %s 没有包裹", o.No)
			for _, sh := range shipments {
				if sh.Direction == "forward" {
					r.must(sh.Status == "delivered", "已完成订单 %s 的正向包裹 %s 状态为 %s", o.No, sh.No, sh.Status)
				}
				if sh.Direction == "reship" && sh.Status != "delivered" {
					// A replacement parcel may still be pending only while an
					// exchange/reship case is open.
					open := false
					for _, c := range v.caseByOrder[o.ID] {
						if (c.Type == "exchange" || c.Type == "reship") && !terminalCase(c.Status) {
							open = true
						}
					}
					r.must(open, "已完成订单 %s 的补发包裹 %s 未签收且无处理中的售后单", o.No, sh.No)
				}
			}
			if o.Status == "closed" {
				r.must(o.Paid > 0 && o.Refunded == o.Paid, "关闭订单 %s 未全额退款", o.No)
			}
		case "cancelled":
			r.must(o.CancelledAt != nil, "已取消订单 %s 缺少取消时间", o.No)
			for _, sh := range shipments {
				switch sh.Status {
				case "pending", "intercepted", "returned":
				default:
					r.must(false, "已取消订单 %s 的包裹 %s 仍在 %s（缺少拦截记录）", o.No, sh.No, sh.Status)
				}
			}
			if o.Paid > 0 {
				r.must(o.Refunded == o.Paid, "已取消订单 %s 已付款但退款未完成", o.No)
			}
		}
	}
}

func allPending(shipments []*shipmentRow) bool {
	for _, sh := range shipments {
		if sh.Status != "pending" {
			return false
		}
	}
	return true
}

var statusBearingCode = map[string]string{
	"created": "pending", "picked_up": "picked_up", "in_transit": "in_transit",
	"delivering": "delivering", "delivered": "delivered", "delivery_failed": "delivery_failed",
	"exception": "exception", "intercepted": "intercepted", "returned": "returned",
}

func (v *validator) shipmentsTrack() {
	r := v.r
	seenNo := map[string]bool{}
	seenTracking := map[string]bool{}
	for _, sh := range v.ds.shipments {
		r.must(!seenNo[sh.No], "包裹编号重复：%s", sh.No)
		seenNo[sh.No] = true
		r.must(!seenTracking[sh.TrackingNo], "运单号重复：%s", sh.TrackingNo)
		seenTracking[sh.TrackingNo] = true
		r.must(strings.HasPrefix(sh.TrackingNo, "MOCK"), "运单号 %s 未标记为模拟", sh.TrackingNo)
		r.must(strings.Contains(sh.CarrierName, "模拟"), "承运商 %s 未标注模拟", sh.CarrierName)

		events := v.events[sh.ID]
		r.must(len(events) > 0, "包裹 %s 没有轨迹", sh.No)
		if len(events) == 0 {
			continue
		}
		var prev time.Time
		for i, e := range events {
			r.must(!e.At.After(v.anchor), "包裹 %s 的轨迹时间 %s 在未来", sh.No, ts(e.At))
			if i > 0 {
				r.must(e.At.After(prev), "包裹 %s 的轨迹时间未严格递增（%s）", sh.No, ts(e.At))
			}
			prev = e.At
		}
		last := events[len(events)-1]
		r.must(sh.LastEventAt != nil && sh.LastEventAt.Equal(last.At),
			"包裹 %s 的最后更新时间与最后一条轨迹不一致", sh.No)
		expected, ok := statusBearingCode[last.Code]
		r.must(ok, "包裹 %s 的最后一条轨迹不是状态型事件（%s）", sh.No, last.Code)
		if ok {
			r.must(sh.Status == expected, "包裹 %s 状态为 %s，但最后一条轨迹表示 %s", sh.No, sh.Status, expected)
		}
		// A delivered parcel must not be followed by delivery events again.
		for _, e := range events {
			if e.Code == "delivered" {
				r.must(strings.Contains(e.Desc, "签收"), "包裹 %s 的签收节点缺少签收说明", sh.No)
			}
		}
		if sh.Status == "delivered" && sh.Direction == "return" {
			r.must(sh.EstimatedDeliveryAt == nil || !sh.EstimatedDeliveryAt.After(v.anchor),
				"退件包裹 %s 的预计送达时间在未来", sh.No)
		}
	}

	// Cold-chain products must show a cold-chain node.
	for _, it := range v.ds.items {
		p := v.products[it.ProductID]
		if p == nil {
			continue
		}
		if cold, _ := p.Attrs["cold_chain_required"].(bool); !cold {
			continue
		}
		for _, sh := range v.shipByOrder[it.OrderID] {
			if sh.Direction != "forward" {
				continue
			}
			var found bool
			for _, e := range v.events[sh.ID] {
				if e.Code == "cold_chain" {
					found = true
				}
			}
			if sh.Status != "pending" {
				r.must(found, "冷链商品 %s 的包裹 %s 缺少冷链轨迹节点", p.Code, sh.No)
			}
		}
	}
}

func (v *validator) casesState() {
	r := v.r
	priceProtectByOrder := map[string]int{}
	activeByItem := map[string]int{}
	for _, c := range v.ds.cases {
		o := v.orders[c.OrderID]
		if o == nil {
			continue
		}
		r.must(c.Quantity > 0, "售后单 %s 的申请数量必须大于 0", c.No)
		if c.OrderItemID != "" {
			it := v.items[c.OrderItemID]
			if it != nil {
				r.must(c.Quantity <= it.Quantity, "售后单 %s 的申请数量超过购买数量", c.No)
				r.must(c.Quantity+it.RefundedQty <= it.Quantity+it.RefundedQty, "售后单 %s 数量异常", c.No)
			}
		}
		if !terminalCase(c.Status) {
			activeByItem[c.OrderItemID]++
			r.must(activeByItem[c.OrderItemID] <= 1, "订单商品行 %s 同时存在多个处理中的售后单", c.OrderItemID)
		}
		switch c.Type {
		case "return", "exchange", "repair", "reship":
			r.must(o.Status == "shipped" || o.Status == "completed" || o.Status == "closed",
				"售后单 %s（%s）对应的订单状态为 %s", c.No, c.Type, o.Status)
		case "price_protection":
			priceProtectByOrder[o.ID]++
			r.must(priceProtectByOrder[o.ID] <= 1, "订单 %s 存在多个价保申请", o.No)
			r.must(o.PaidAt != nil, "价保单 %s 对应订单未支付", c.No)
			if c.Status != "rejected" && o.PaidAt != nil {
				r.must(c.CreatedAt.Sub(*o.PaidAt) <= priceProtectionHours*time.Hour,
					"价保单 %s 超出 15 天窗口却未拒绝", c.No)
			}
		}
		if c.Status == "rejected" {
			r.must(c.Resolution != "", "被拒售后单 %s 缺少可向用户解释的原因", c.No)
		}
		if c.Status == "completed" {
			r.must(c.ClosedAt != nil, "已完成售后单 %s 缺少关闭时间", c.No)
		}
		if c.Type == "reship" && (c.Status == "approved" || c.Status == "completed") {
			var found bool
			for _, sh := range v.shipByOrder[o.ID] {
				if sh.Direction == "reship" {
					found = true
				}
			}
			r.must(found, "补发单 %s 没有补发包裹", c.No)
		}
		if c.Type == "exchange" && c.Status == "awaiting_return_shipment" {
			var found bool
			for _, sh := range v.shipByOrder[o.ID] {
				if sh.Direction == "return" {
					found = true
				}
			}
			r.must(found, "换货单 %s 处于待退回状态但没有退件包裹", c.No)
		}
		// Policy exceptions: customized / perishable / intimate goods never get a
		// successful no-reason return.
		if c.Type == "return" && c.ReasonCode == "no_reason" &&
			(c.Status == "completed" || c.Status == "processing" || c.Status == "approved") {
			it := v.items[c.OrderItemID]
			if it != nil {
				if p := v.products[it.ProductID]; p != nil {
					code, _ := p.Attrs["return_policy_code"].(string)
					r.must(code != policyCustomized && code != policyFreshClaim && code != policyIntimate,
						"定制/生鲜/贴身商品 %s 出现了无理由退货成功的售后单 %s", p.Code, c.No)
				}
			}
			if o.DeliveredAt != nil && c.CreatedAt.Sub(*o.DeliveredAt) > afterSalesWindowHours*time.Hour {
				r.must(false, "售后单 %s 超出 7 天窗口却仍在处理", c.No)
			}
		}
	}
}

func (v *validator) invoicesState() {
	r := v.r
	for _, inv := range v.ds.invoices {
		o := v.orders[inv.OrderID]
		if o == nil {
			continue
		}
		r.must(o.PaidAt != nil, "发票 %s 对应的订单未支付", inv.No)
		r.must(inv.Amount > 0, "发票 %s 金额必须大于 0", inv.No)
		r.must(inv.Amount <= o.Paid, "发票 %s 金额超过订单实付", inv.No)
		if inv.Type == "vat_special" {
			r.must(inv.TaxNo != "", "专票 %s 缺少税号", inv.No)
		}
		if inv.Type == "electronic" {
			r.must(inv.Email != "", "电子发票 %s 缺少接收邮箱", inv.No)
		}
		r.must(strings.HasSuffix(inv.Email, "example.test") || inv.Email == "",
			"发票邮箱 %s 不是合成域名", inv.Email)
		switch inv.Status {
		case "issued":
			r.must(inv.No != "", "已开票记录缺少发票号")
			r.must(inv.IssuedAt != nil, "已开票记录 %s 缺少开票时间", inv.No)
		case "rejected":
			r.must(inv.No == "", "开票失败记录 %s 不应有发票号", inv.No)
			r.must(inv.IssuedAt == nil, "开票失败记录 %s 不应有开票时间", inv.No)
		case "red_flushed":
			r.must(o.Refunded > 0, "发票 %s 已红冲但订单没有任何退款", inv.No)
			r.must(inv.IssuedAt != nil, "红冲发票 %s 缺少原开票时间", inv.No)
		}
		if o.Refunded == o.Paid && o.Paid > 0 && inv.Status == "issued" {
			r.must(false, "订单 %s 已全额退款，但发票 %s 仍未红冲", o.No, inv.No)
		}
		if inv.IssuedAt != nil {
			r.must(!inv.IssuedAt.Before(inv.RequestedAt), "发票 %s 开票时间早于申请时间", inv.No)
			r.must(!inv.IssuedAt.After(v.anchor), "发票 %s 开票时间在未来", inv.No)
		}
	}
}

func (v *validator) conversations() {
	r := v.r
	toolMsg := map[string]*messageRow{}
	for i := range v.ds.messages {
		m := &v.ds.messages[i]
		if m.Role == "tool" {
			r.must(m.ToolName != "", "工具消息缺少 tool_name（会话 %s 第 %d 条）", m.ConvID, m.Sequence)
			r.must(m.ToolCallID != "", "工具消息缺少 tool_call_id（会话 %s 第 %d 条）", m.ConvID, m.Sequence)
			if m.ToolCallID != "" {
				_, dup := toolMsg[m.ToolCallID]
				r.must(!dup, "tool_call_id 重复：%s", m.ToolCallID)
				toolMsg[m.ToolCallID] = m
			}
			r.must(jsonValid(m.ToolArgs), "工具消息参数不是合法 JSON（%s）", m.ToolCallID)
			r.must(jsonValid(m.ToolResult), "工具消息结果不是合法 JSON（%s）", m.ToolCallID)
			r.must(strings.Contains(m.ToolResult, "\"ok\""), "工具结果缺少 ok 字段（%s）", m.ToolCallID)
		}
	}
	// Each tool execution must match exactly one tool message and vice versa.
	for _, t := range v.ds.toolExecs {
		m, ok := toolMsg[t.ToolCallID]
		r.must(ok, "工具执行 %s 没有对应的工具消息（日志与消息不一致）", t.ToolCallID)
		if ok {
			r.must(m.ConvID == t.ConvID, "工具执行 %s 与会话消息不一致", t.ToolCallID)
			r.must(m.ToolName == t.ToolName, "工具执行 %s 的工具名与消息不一致", t.ToolCallID)
			if t.Status == "succeeded" {
				r.must(strings.Contains(t.Output, "\"ok\":true"), "工具执行 %s 标记成功但结果不是成功响应（疑似幻觉）", t.ToolCallID)
				r.must(m.ToolResult == t.Output, "工具执行 %s 的输出与消息中的工具结果不一致", t.ToolCallID)
			} else {
				r.must(strings.Contains(t.Output, "\"ok\":false") || t.Output == "",
					"工具执行 %s 标记失败但结果不是失败响应", t.ToolCallID)
				r.must(t.ErrorCode != "", "工具执行 %s 失败但没有错误码", t.ToolCallID)
			}
			r.must(t.FinishedAt != nil && !t.FinishedAt.Before(t.CreatedAt),
				"工具执行 %s 的结束时间早于开始时间", t.ToolCallID)
		}
	}
	for _, m := range v.ds.messages {
		if m.Role != "tool" {
			continue
		}
		_, ok := v.tools[m.ToolCallID]
		r.must(ok, "工具消息 %s 没有对应的执行日志", m.ToolCallID)
	}

	for _, conv := range v.ds.conversations {
		list := v.msgs[conv.ID]
		r.must(len(list) > 0, "会话 %s 没有消息", conv.No)
		if len(list) == 0 {
			continue
		}
		r.must(list[0].Role == "user", "会话 %s 的第一条消息不是用户消息", conv.No)
		var prev time.Time
		seq := 0
		for _, m := range list {
			seq++
			r.must(m.Sequence == seq, "会话 %s 的消息序号不连续（%d != %d）", conv.No, m.Sequence, seq)
			if seq > 1 {
				r.must(m.CreatedAt.After(prev), "会话 %s 的消息时间未严格递增（第 %d 条）", conv.No, m.Sequence)
			}
			prev = m.CreatedAt
			r.must(!m.CreatedAt.After(v.anchor), "会话 %s 的消息时间在未来", conv.No)
		}
		r.must(conv.LastMessage.Equal(prev), "会话 %s 的最后消息时间与消息表不一致", conv.No)
		r.must(!conv.CreatedAt.After(list[0].CreatedAt), "会话 %s 的创建时间晚于首条消息", conv.No)
	}
	for _, p := range v.ds.pending {
		r.must(p.ExpiresAt.After(p.CreatedAt), "待确认动作 %s 的过期时间早于创建时间", p.ActionNo)
		if p.Status == "executed" {
			r.must(p.ConfirmedAt != nil, "待确认动作 %s 已执行但没有确认时间", p.ActionNo)
			r.must(p.ExecutedAt != nil, "待确认动作 %s 已执行但没有执行时间", p.ActionNo)
		}
		if p.Status == "pending" {
			r.must(p.ConfirmedAt == nil && p.ExecutedAt == nil, "待确认动作 %s 尚未确认却有执行记录", p.ActionNo)
		}
	}
	for _, a := range v.ds.audits {
		if a.Result == "succeeded" {
			r.must(a.After != "", "成功的审计记录 %s 缺少变更后快照", a.ActionType)
		}
	}
	for _, ann := range v.ds.annotations {
		m, ok := v.msgsByID[ann.MessageID]
		r.must(ok, "标注指向不存在的消息")
		if ok {
			r.must(m.Role == "user" || m.Role == "assistant", "标注打在了 %s 消息上", m.Role)
		}
		r.must(ann.Confidence >= 0 && ann.Confidence <= 1, "标注置信度越界")
	}
}

func (v *validator) catalog() {
	r := v.r
	seen := map[string]bool{}
	for _, p := range v.ds.products {
		r.must(!seen[p.Code], "商品编码重复：%s", p.Code)
		seen[p.Code] = true
		marker, _ := p.Attrs["is_synthetic"].(bool)
		r.must(marker, "商品 %s 未标记为合成数据", p.Code)
		r.must(p.Attrs["return_policy_code"] != nil, "商品 %s 缺少退换政策", p.Code)
	}
	skuCodes := map[string]bool{}
	for _, s := range v.ds.skus {
		r.must(!skuCodes[s.Code], "SKU 编码重复：%s", s.Code)
		skuCodes[s.Code] = true
		r.must(s.ListPrice > 0, "SKU %s 价格必须大于 0", s.Code)
		r.must(s.Stock >= 0, "SKU %s 库存为负", s.Code)
		if s.Status == "out_of_stock" {
			r.must(s.Stock == 0, "SKU %s 标记缺货但库存为 %d", s.Code, s.Stock)
		} else {
			r.must(s.Stock > 0, "SKU %s 状态为 %s 但库存为 0", s.Code, s.Status)
		}
	}
	for _, c := range v.ds.customers {
		r.must(strings.Contains(c.PhoneMasked, "****"), "客户 %s 的手机号未脱敏", c.No)
		r.must(!containsDigits(c.PhoneMasked, 11), "客户 %s 出现完整手机号", c.No)
	}
	for _, c := range v.ds.conversations {
		_ = c
	}
	for _, o := range v.ds.orders {
		r.must(strings.Contains(o.Address, "模拟路"), "订单 %s 的地址不是虚构地址", o.No)
		r.must(strings.Contains(o.ReceiverPhoneMasked, "****"), "订单 %s 的收件人手机号未脱敏", o.No)
	}
	for _, inv := range v.ds.invoices {
		r.must(inv.TaxNo == "" || strings.HasPrefix(inv.TaxNo, "TESTTAX"),
			"发票 %s 的税号不是测试值", inv.No)
	}
}

func containsDigits(s string, n int) bool {
	run := 0
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			run++
			if run >= n {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

func jsonValid(raw string) bool {
	if raw == "" {
		return false
	}
	var tmp any
	return json.Unmarshal([]byte(raw), &tmp) == nil
}

// validateDatabase re-checks the invariants that are cheaper or safer to verify
// with SQL after the import, plus the schema's own foreign key check.
func validateDatabase(ctx context.Context, db *sql.DB, anchor time.Time) (*Report, error) {
	r := &Report{}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return nil, fmt.Errorf("foreign_key_check: %w", err)
	}
	count := 0
	for rows.Next() {
		count++
	}
	rows.Close()
	r.must(count == 0, "PRAGMA foreign_key_check 报告 %d 条外键异常", count)

	type q struct {
		sql  string
		desc string
	}
	queries := []q{
		{`SELECT COUNT(1) FROM orders o WHERE o.paid_amount_minor <> COALESCE((
			SELECT SUM(p.amount_minor) FROM payments p
			WHERE p.order_id = o.id AND p.payment_status IN ('succeeded','partially_refunded','refunded')),0)
			AND o.order_channel <> 'cod'`, "订单实付与成功支付合计不一致"},
		{`SELECT COUNT(1) FROM orders o WHERE o.refunded_amount_minor <> COALESCE((
			SELECT SUM(r.amount_minor) FROM refunds r
			WHERE r.order_id = o.id AND r.refund_status = 'succeeded'),0)`, "订单已退金额与成功退款合计不一致"},
		{`SELECT COUNT(1) FROM order_items i JOIN orders o ON o.id = i.order_id
			WHERE i.refunded_quantity > i.quantity`, "商品行已退数量超过购买数量"},
		{`SELECT COUNT(1) FROM orders o WHERE o.refunded_amount_minor > o.paid_amount_minor`, "退款金额超过实付金额"},
		{`SELECT COUNT(1) FROM service_cases c JOIN orders o ON o.id = c.order_id
			WHERE c.customer_id <> o.customer_id`, "售后单客户与订单客户不一致"},
		{`SELECT COUNT(1) FROM service_cases c JOIN order_items i ON i.id = c.order_item_id
			WHERE i.order_id <> c.order_id`, "售后单引用了其他订单的商品行"},
		{`SELECT COUNT(1) FROM refunds r JOIN payments p ON p.id = r.payment_id
			WHERE p.order_id <> r.order_id`, "退款关联的支付记录属于其他订单"},
		{`SELECT COUNT(1) FROM shipments s WHERE s.last_event_at IS NOT NULL AND s.last_event_at <> (
			SELECT MAX(e.event_time) FROM shipment_events e WHERE e.shipment_id = s.id)`, "包裹最后更新时间与轨迹不一致"},
		{`SELECT COUNT(1) FROM shipments s WHERE EXISTS (
			SELECT 1 FROM shipment_events e WHERE e.shipment_id = s.id AND e.event_time > ?)`, "包裹轨迹时间在未来"},
		{`SELECT COUNT(1) FROM orders o WHERE o.created_at > ?`, "订单创建时间在未来"},
		{`SELECT COUNT(1) FROM invoices i WHERE i.amount_minor > (SELECT paid_amount_minor FROM orders o WHERE o.id = i.order_id)`,
			"发票金额超过订单实付"},
		{`SELECT COUNT(1) FROM orders o WHERE o.order_status = 'cancelled' AND EXISTS (
			SELECT 1 FROM shipments s WHERE s.order_id = o.id AND s.shipment_status IN ('in_transit','delivering','delivered'))`,
			"已取消订单仍存在在途/已签收包裹"},
		{`SELECT COUNT(1) FROM order_items i WHERE i.paid_amount_minor <> i.unit_price_minor * i.quantity - i.discount_amount_minor`,
			"商品行实付金额不闭合"},
	}
	for _, item := range queries {
		var n int
		var err error
		if strings.Contains(item.sql, "?") {
			err = db.QueryRowContext(ctx, item.sql, ts(anchor)).Scan(&n)
		} else {
			err = db.QueryRowContext(ctx, item.sql).Scan(&n)
		}
		if err != nil {
			return nil, fmt.Errorf("校验「%s」失败: %w", item.desc, err)
		}
		r.must(n == 0, "%s（%d 行）", item.desc, n)
	}
	return r, nil
}
