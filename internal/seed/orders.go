package seed

import (
	"fmt"
	"hash/fnv"
	"sort"
	"time"
)

// Business constants. They mirror the knowledge base articles and the
// eligibility engine defaults (7-day after-sales window, 15-day price
// protection, free shipping over 99 yuan, 8 yuan otherwise).
const (
	freeShippingThreshold int64 = 9900
	standardShippingFee   int64 = 800
	afterSalesWindowHours       = 7 * 24
	exchangeWindowHours         = 15 * 24
	priceProtectionHours        = 15 * 24
	presaleDepositMinor   int64 = 10000
)

var (
	carrierSF = struct{ code, name string }{"SF_MOCK", "顺丰速运（模拟）"}
	carrierYT = struct{ code, name string }{"YTO_MOCK", "圆通速递（模拟）"}
	carrierJD = struct{ code, name string }{"JD_MOCK", "京东物流（模拟）"}
	carrierZT = struct{ code, name string }{"ZTO_MOCK", "中通快递（模拟）"}
)

type lineOpt struct {
	sku  string
	qty  int64
	unit int64 // optional list-price override (historic price snapshot)
}

type orderOpts struct {
	customer    int // 1-based index of customerDefs
	addr        int
	at          time.Time
	channel     string
	presale     bool
	lines       []lineOpt
	discount    int64
	freeShip    bool
	method      string
	invoice     string
	remark      string
	requested   *time.Time
	no          string // non-empty: fixed order number (skip nextOrderNo)
	noCompanion bool   // true: never append category companion SKU
}

type itemDraft struct {
	row      itemRow
	product  *productRow
	sku      *skuRow
	qty      int64
	refunded int64
	unit     int64
	discount int64
	paid     int64
	openQty  int64 // requested but not yet resolved
}

type shipmentDraft struct {
	row    shipmentRow
	events []eventRow
}

type caseDraft struct {
	row      caseRow
	refundNo string
	wasOpen  bool
}

type orderDraft struct {
	g         *gen
	row       orderRow
	items     []*itemDraft
	payments  []*paymentRow
	refunds   []*refundRow
	shipments []*shipmentDraft
	cases     []*caseDraft
	invoices  []*invoiceRow
	due       int64
	paid      int64
}

// ---- construction ----

func (g *gen) newOrder(opts orderOpts) *orderDraft {
	if opts.customer < 1 || opts.customer > len(g.customers) {
		g.fail("order: customer index %d out of range", opts.customer)
		return nil
	}
	cust := g.customers[opts.customer-1]
	def := g.customerDefs[opts.customer-1]
	if len(def.Addrs) == 0 {
		g.fail("order: customer %s has no address", cust.No)
		return nil
	}
	addrIdx := opts.addr
	if addrIdx >= len(def.Addrs) {
		addrIdx = 0
	}
	addr := def.Addrs[addrIdx]

	no := opts.no
	if no == "" {
		no = g.nextOrderNo()
	}
	d := &orderDraft{g: g}
	d.row = orderRow{
		ID: stableID("order", no), No: no, CustomerID: cust.ID,
		Status: "pending_payment", Channel: opts.channel,
		ReceiverName: addr.Receiver, ReceiverPhoneMasked: maskPhone(g.customerDefs[opts.customer-1].Phone),
		Province: addr.Province, City: addr.City, District: addr.District, Address: addr.Street,
		PostalCode: addr.Postal, ShippingMethod: firstNonEmpty(opts.method, addr.Method),
		InvoiceTypeRequested: opts.invoice, Remark: opts.remark,
		RequestedDeliveryAt: opts.requested,
		CreatedAt:           opts.at, UpdatedAt: opts.at, Version: 1,
		Presale: opts.presale,
	}
	if d.row.Channel == "" {
		d.row.Channel = "mock_store"
	}

	var itemAmount int64
	for _, line := range opts.lines {
		sku := g.skus[line.sku]
		if sku == nil {
			g.fail("order %s: unknown sku %s", no, line.sku)
			continue
		}
		product := g.productByID[sku.ProductID]
		unit := sku.ListPrice
		if line.unit > 0 {
			unit = line.unit
		}
		if line.qty <= 0 {
			g.fail("order %s: sku %s quantity %d", no, line.sku, line.qty)
			continue
		}
		it := &itemDraft{product: product, sku: sku, qty: line.qty, unit: unit}
		d.items = append(d.items, it)
		itemAmount += unit * line.qty
	}
	if len(d.items) == 0 {
		g.fail("order %s: no items", no)
		return nil
	}

	// About 40% of single-line baskets also contain one 凑单 add-on from the same
	// category, which is what pushes the dataset's order-line count into the
	// 160-220 range without inventing a second order story. The choice is a
	// stable hash of the order number, so two runs produce the same basket.
	// IM world fixtures need exact paid amounts, so they set noCompanion.
	if !opts.noCompanion && len(opts.lines) == 1 && basketBucket(no) < 40 {
		base := d.items[0]
		if comp := companionSKU(base.product.Category); comp != "" && comp != base.sku.Code {
			if sku := g.skus[comp]; sku != nil {
				d.items = append(d.items, &itemDraft{
					product: g.productByID[sku.ProductID], sku: sku, qty: 1, unit: sku.ListPrice,
				})
				itemAmount += sku.ListPrice
			}
		}
	}

	d.row.ItemAmount = itemAmount
	if opts.discount > itemAmount {
		g.fail("order %s: discount %d exceeds item amount %d", no, opts.discount, itemAmount)
		opts.discount = itemAmount
	}
	d.row.Discount = opts.discount
	d.allocateDiscount()
	d.row.Shipping = 0
	if !opts.freeShip && itemAmount-opts.discount < freeShippingThreshold {
		d.row.Shipping = standardShippingFee
	}
	d.due = itemAmount - opts.discount + d.row.Shipping
	// Item identities are fixed at construction time so cases and refunds can
	// reference them before the rows are committed.
	for i, it := range d.items {
		it.row.ID = stableID("item", no, fmt.Sprintf("%d", i))
	}
	g.orderDrafts = append(g.orderDrafts, d)
	return d
}

// companionSKU returns a cheap add-on that belongs to the same category, so a
// two-line basket never mixes a phone with, say, frozen beef.
func companionSKU(category string) string {
	switch category {
	case "3C数码":
		return "SKU-P009-01"
	case "服装鞋包":
		return "SKU-P020-01"
	case "食品个护":
		return "SKU-P051-01"
	case "家居小家电":
		return "SKU-P047-01"
	case "生鲜冷链":
		return "SKU-P032-01"
	case "礼品与定制":
		return "SKU-P028-01"
	}
	return ""
}

func basketBucket(orderNo string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte("basket|" + orderNo))
	return int(h.Sum32() % 100)
}

// allocateDiscount spreads the order-level discount over lines with the largest
// remainder method so the sum always equals the order discount exactly.
func (o *orderDraft) allocateDiscount() {
	total := o.row.ItemAmount
	disc := o.row.Discount
	if disc == 0 || total == 0 {
		for _, it := range o.items {
			it.discount = 0
			it.paid = it.unit * it.qty
		}
		return
	}
	type share struct {
		it       *itemDraft
		raw      int64
		rem      int64
		assigned int64
	}
	shares := make([]*share, 0, len(o.items))
	var assigned int64
	for _, it := range o.items {
		line := it.unit * it.qty
		raw := disc * line / total
		rem := disc*line - raw*total
		shares = append(shares, &share{it: it, raw: raw, rem: rem})
		assigned += raw
	}
	left := disc - assigned
	sort.SliceStable(shares, func(i, j int) bool {
		if shares[i].rem != shares[j].rem {
			return shares[i].rem > shares[j].rem
		}
		return shares[i].it.unit*shares[i].it.qty > shares[j].it.unit*shares[j].it.qty
	})
	for i := 0; i < int(left) && i < len(shares); i++ {
		shares[i].raw++
	}
	for _, sh := range shares {
		sh.it.discount = sh.raw
		sh.it.paid = sh.it.unit*sh.it.qty - sh.raw
	}
}

func (o *orderDraft) fail(format string, args ...any) {
	o.g.fail("order %s: %s", o.row.No, fmt.Sprintf(format, args...))
}

// ---- payments ----

type payPart struct {
	method string
	amount int64
}

func (o *orderDraft) pay(at time.Time, method string) {
	o.payParts(at, payPart{method: method, amount: o.due - o.paid})
}

// payParts records channel-level payments. Their sum must close the remaining
// payable amount exactly; anything else is a build error, not a rounding
// accident. A presale order therefore pays the deposit first and the balance
// later, never both in one step.
func (o *orderDraft) payParts(at time.Time, parts ...payPart) {
	remaining := o.due - o.paid
	if remaining <= 0 {
		o.fail("order already fully paid")
		return
	}
	var sum int64
	for _, p := range parts {
		if p.amount <= 0 {
			o.fail("payment part %s amount %d", p.method, p.amount)
			return
		}
		sum += p.amount
	}
	if sum != remaining {
		o.fail("payment parts sum %d != remaining %d", sum, remaining)
		return
	}
	for _, p := range parts {
		o.addPayment(at, p.method, "succeeded", p.amount, "")
	}
	o.paid = sum + o.paid
	o.row.Paid = o.paid
	if o.row.PaidAt == nil {
		o.row.PaidAt = timePtr(at)
	}
	if o.paid == o.due && o.row.Status == "pending_payment" {
		o.row.Status = "paid"
	}
	if o.paid > o.due {
		o.fail("paid %d exceeds due %d", o.paid, o.due)
	}
}

// payDeposit records the presale deposit only. The order stays in
// pending_payment until the balance closes the remaining amount.
func (o *orderDraft) payDeposit(at time.Time, method string, amount int64) {
	if amount <= 0 || o.paid+amount > o.due {
		o.fail("deposit %d invalid (due %d, paid %d)", amount, o.due, o.paid)
		return
	}
	o.addPayment(at, method, "succeeded", amount, "")
	o.paid += amount
	o.row.Paid = o.paid
	if o.row.PaidAt == nil {
		o.row.PaidAt = timePtr(at)
	}
}

// attempt records a non-settled payment attempt (failed / pending / reversal).
func (o *orderDraft) attempt(at time.Time, method, status string, amount int64, ref string) *paymentRow {
	return o.addPayment(at, method, status, amount, ref)
}

func (o *orderDraft) addPayment(at time.Time, method, status string, amount int64, ref string) *paymentRow {
	no := o.g.nextPaymentNo()
	row := &paymentRow{
		ID: stableID("payment", no), No: no, OrderID: o.row.ID, Method: method, Status: status,
		Amount: amount, ProviderRef: ref, CreatedAt: at, UpdatedAt: at,
	}
	if status == "succeeded" || status == "partially_refunded" || status == "refunded" {
		row.PaidAt = timePtr(at)
	}
	o.payments = append(o.payments, row)
	return row
}

// ---- logistics ----

type shipOpts struct {
	carrier    string
	start      time.Time
	status     string
	direction  string // forward | reship | return
	signer     string
	note       string
	intercept  bool // add an intercept-requested event before the terminal node
	coldChain  bool
	TrackingNo string // optional fixed tracking number (e.g. MOCKSF7002)
}

type shipEvent struct {
	code string
	desc string
	loc  string
	at   time.Time
}

// ship builds one parcel. The parcel status is derived from the last
// status-bearing event, so "delivered but still delivering" cannot happen.
func (o *orderDraft) ship(opts shipOpts) *shipmentDraft {
	carrierCode, carrierName := carrierSF.code, carrierSF.name
	switch opts.carrier {
	case "YTO":
		carrierCode, carrierName = carrierYT.code, carrierYT.name
	case "JD":
		carrierCode, carrierName = carrierJD.code, carrierJD.name
	case "ZTO":
		carrierCode, carrierName = carrierZT.code, carrierZT.name
	}
	events, finalStatus, lastAt := buildTrack(opts)
	// A parcel can never produce a future trace: if the scripted timeline would
	// run past the data anchor, move the whole parcel earlier by the overflow.
	if overflow := lastAt.Sub(o.g.anchor); overflow > 0 {
		for i := range events {
			events[i].at = events[i].at.Add(-overflow)
		}
		opts.start = opts.start.Add(-overflow)
		lastAt = lastAt.Add(-overflow)
	}

	prefix := "SHP-"
	switch opts.direction {
	case "reship":
		prefix = "SHP-B"
	case "return":
		prefix = "SHP-R"
	}
	no := o.g.nextShipmentNo(prefix)
	row := shipmentRow{
		ID: stableID("shipment", no), No: no, OrderID: o.row.ID,
		CarrierCode: carrierCode, CarrierName: carrierName,
		TrackingNo: firstNonEmpty(opts.TrackingNo, o.g.nextTrackingNo(carrierCode)), Status: finalStatus,
		CreatedAt: opts.start.Add(-2 * time.Hour), UpdatedAt: lastAt,
		LastEventAt: timePtr(lastAt), Direction: opts.direction,
	}
	if opts.direction == "" {
		row.Direction = "forward"
	}
	if finalStatus != "delivered" && finalStatus != "returned" {
		row.EstimatedDeliveryAt = timePtr(opts.start.Add(48 * time.Hour))
	}
	draft := &shipmentDraft{row: row}
	now := o.g.anchor
	for _, e := range events {
		draft.events = append(draft.events, eventRow{
			ID: stableID("event", no, e.code, ts(e.at)), ShipmentID: row.ID, At: e.at,
			Code: e.code, Desc: e.desc, Location: e.loc, Source: "mock_logistics", CreatedAt: now,
		})
	}
	o.shipments = append(o.shipments, draft)
	return draft
}

// buildTrack returns a monotonic event list. Every event after the first is at
// least three hours later, so timestamps can never go backwards.
func buildTrack(opts shipOpts) ([]shipEvent, string, time.Time) {
	status := opts.status
	origin := "测试市一区"
	hub := "测试市分拨中心"
	dest := "测试市派送站"
	if opts.direction == "return" {
		origin, hub, dest = "测试市三区", "测试市退货仓", "测试市退货仓"
	}
	var events []shipEvent
	add := func(offset time.Duration, code, desc, where string) {
		events = append(events, shipEvent{code: code, desc: desc, loc: where, at: opts.start.Add(offset)})
	}

	if status == "pending" {
		add(0, "created", "包裹已创建，等待承运商揽收", origin)
		return events, "pending", events[len(events)-1].at
	}
	add(0, "picked_up", "承运商已揽收快件", origin)
	if opts.coldChain {
		add(3*time.Hour, "cold_chain", "冷链运输中，温度 2-6℃ 正常", hub)
	}

	switch status {
	case "picked_up":
		return events, "picked_up", events[len(events)-1].at
	case "in_transit":
		add(5*time.Hour, "in_transit", "快件已离开寄件网点，发往分拨中心", origin)
		add(11*time.Hour, "in_transit", "快件已到达【"+hub+"】", hub)
		return events, "in_transit", events[len(events)-1].at
	case "exception", "delivery_failed", "delivering", "delivered", "intercepted", "returned":
		add(5*time.Hour, "in_transit", "快件已离开寄件网点，发往分拨中心", origin)
		add(11*time.Hour, "in_transit", "快件已到达【"+hub+"】", hub)
		add(16*time.Hour, "in_transit", "快件已从【"+hub+"】发出", hub)
		add(23*time.Hour, "in_transit", "快件已到达【"+dest+"】", dest)
	default:
		return events, "in_transit", events[len(events)-1].at
	}

	if opts.intercept {
		add(25*time.Hour, "intercept_requested", "已提交物流拦截申请，等待承运商处理", dest)
	}
	if status == "exception" {
		add(26*time.Hour, "exception", "快件异常："+firstNonEmpty(opts.note, "外包装破损，待核实"), dest)
		return events, "exception", events[len(events)-1].at
	}
	if status == "intercepted" {
		add(28*time.Hour, "intercepted", "拦截成功，快件将退回仓库", dest)
		return events, "intercepted", events[len(events)-1].at
	}

	add(28*time.Hour, "delivering", "快件正在派送，请保持电话畅通", dest)
	switch status {
	case "delivering":
		return events, "delivering", events[len(events)-1].at
	case "delivery_failed":
		add(33*time.Hour, "delivery_failed", "派送失败："+firstNonEmpty(opts.note, "联系不上收件人，将安排二次派送"), dest)
		return events, "delivery_failed", events[len(events)-1].at
	case "returned":
		add(33*time.Hour, "returned", "收件人拒收，快件退回仓库", dest)
		return events, "returned", events[len(events)-1].at
	case "delivered":
		signer := firstNonEmpty(opts.signer, "本人")
		add(32*time.Hour, "delivered", "快件已签收，签收人："+signer, dest)
		return events, "delivered", events[len(events)-1].at
	}
	return events, "delivering", events[len(events)-1].at
}

// deliver records delivery of the forward parcel and completes the order.
func (o *orderDraft) deliver(at time.Time) {
	if o.row.ShippedAt == nil {
		o.fail("deliver before ship")
		return
	}
	if at.After(o.g.anchor) {
		at = o.g.anchor
	}
	completed := at.Add(2 * time.Hour)
	if completed.After(o.g.anchor) {
		completed = o.g.anchor
	}
	o.row.DeliveredAt = timePtr(at)
	o.row.CompletedAt = timePtr(completed)
	o.row.UpdatedAt = completed
	o.row.Status = "completed"
}

func (o *orderDraft) markShipped(at time.Time) {
	// shipped_at follows the first real pickup event, so a shifted parcel can
	// never claim to have shipped after it was picked up.
	start := at
	for _, sh := range o.shipments {
		if sh.row.Direction != "forward" || len(sh.events) == 0 {
			continue
		}
		if sh.events[0].At.Before(start) {
			start = sh.events[0].At
		}
	}
	at = start
	o.row.ShippedAt = timePtr(at)
	o.row.UpdatedAt = at
	if o.row.Status == "paid" || o.row.Status == "processing" {
		o.row.Status = "shipped"
	}
}

// cancel enforces the two preconditions the spec cares about: nothing has been
// picked up, and any paid money is refunded through a refund record.
func (o *orderDraft) cancel(at time.Time, reason string) {
	for _, sh := range o.shipments {
		if sh.row.Status != "pending" {
			o.fail("cancel after pickup (shipment %s is %s)", sh.row.No, sh.row.Status)
			return
		}
	}
	o.row.Status = "cancelled"
	o.row.CancelledAt = timePtr(at)
	o.row.UpdatedAt = at
	if o.paid > 0 && o.row.Refunded < o.paid {
		o.newRefund(refundOpts{
			amount: o.paid - o.row.Refunded, status: "succeeded", at: at,
			reason: firstNonEmpty(reason, "订单取消，全额退款"), noCase: true,
		})
	}
}

// ---- after-sales ----

type caseOpts struct {
	typ        string
	status     string
	item       int
	qty        int64
	reasonCode string
	reasonText string
	desc       string
	at         time.Time
	resolution string
	noWindow   bool
}

func (o *orderDraft) newCase(opts caseOpts) *caseDraft {
	if opts.item < 0 || opts.item >= len(o.items) {
		o.fail("case: item index %d out of range", opts.item)
		return nil
	}
	item := o.items[opts.item]
	if opts.qty <= 0 {
		opts.qty = 1
	}
	open := item.qty - item.refunded - item.openQty
	if opts.qty > open {
		o.fail("case %s: qty %d exceeds available %d", opts.typ, opts.qty, open)
		return nil
	}
	for _, c := range o.cases {
		if c.row.OrderItemID != orderedItemID(o, opts.item) {
			continue
		}
		if !terminalCase(c.row.Status) {
			o.fail("case %s: item already has active case %s", opts.typ, c.row.No)
			return nil
		}
	}
	if opts.status != "rejected" {
		switch opts.typ {
		case "return", "exchange", "repair", "refund", "reship":
			if o.row.Status != "shipped" && o.row.Status != "completed" {
				o.fail("case %s before shipment (order %s)", opts.typ, o.row.Status)
				return nil
			}
		case "price_protection":
			if o.row.PaidAt == nil {
				o.fail("price protection without payment")
				return nil
			}
		}
	}
	// Policy windows and product exceptions.
	if !opts.noWindow {
		switch opts.typ {
		case "return", "refund":
			if code, ok := item.product.Attrs["return_policy_code"].(string); ok {
				if (code == policyCustomized || code == policyIntimate) && opts.reasonCode == "no_reason" {
					o.fail("case: %s product cannot use no-reason return", item.product.Code)
					return nil
				}
				if code == policyFreshClaim && opts.reasonCode == "no_reason" {
					o.fail("case: perishable product cannot use no-reason return")
					return nil
				}
			}
			if opts.reasonCode == "no_reason" && o.row.DeliveredAt != nil {
				if opts.at.Sub(*o.row.DeliveredAt) > afterSalesWindowHours*time.Hour {
					o.fail("case: no-reason return beyond 7 days")
					return nil
				}
			}
		case "exchange", "repair":
			if o.row.DeliveredAt != nil && opts.at.Sub(*o.row.DeliveredAt) > exchangeWindowHours*time.Hour {
				o.fail("case: %s beyond 15 days", opts.typ)
				return nil
			}
		case "price_protection":
			if opts.at.Sub(*o.row.PaidAt) > priceProtectionHours*time.Hour {
				o.fail("case: price protection beyond 15 days")
				return nil
			}
		}
	}

	no := o.g.nextCaseNo()
	row := caseRow{
		ID: stableID("case", no), No: no, CustomerID: o.row.CustomerID, OrderID: o.row.ID,
		OrderItemID: orderedItemID(o, opts.item), Type: opts.typ, Status: opts.status,
		ReasonCode: opts.reasonCode, ReasonText: opts.reasonText, Desc: opts.desc,
		Quantity: opts.qty, Resolution: opts.resolution,
		CreatedAt: opts.at, UpdatedAt: opts.at,
	}
	if terminalCase(opts.status) {
		row.ClosedAt = timePtr(opts.at)
	}
	draft := &caseDraft{row: row}
	o.cases = append(o.cases, draft)
	if !terminalCase(opts.status) {
		draft.wasOpen = true
		item.openQty += opts.qty
	}
	return draft
}

// close moves a case to a terminal state and releases the reserved quantity.
func (c *caseDraft) close(status string, at time.Time, resolution string) {
	c.row.Status = status
	c.row.UpdatedAt = at
	if resolution != "" {
		c.row.Resolution = resolution
	}
	if terminalCase(status) {
		c.row.ClosedAt = timePtr(at)
	}
}

type refundOpts struct {
	amount      int64
	status      string
	at          time.Time
	reason      string
	failure     string
	caseDraft   *caseDraft
	noCase      bool
	completedAt *time.Time
	channel     string
	// compensation refunds (price protection, damaged-produce allowance) pay
	// money without taking the goods back, so the refunded quantity stays 0.
	compensation bool
}

// newRefund is the single place that moves money. It refuses to create a refund
// larger than the remaining refundable amount and only counts succeeded refunds.
func (o *orderDraft) newRefund(opts refundOpts) *refundRow {
	if opts.amount <= 0 {
		o.fail("refund amount %d", opts.amount)
		return nil
	}
	if o.paid == 0 {
		o.fail("refund on unpaid order")
		return nil
	}
	if o.row.Refunded+opts.amount > o.paid {
		o.fail("refund %d exceeds refundable %d", opts.amount, o.paid-o.row.Refunded)
		return nil
	}
	no := o.g.nextRefundNo()
	row := &refundRow{
		ID: stableID("refund", no), No: no, OrderID: o.row.ID, Status: opts.status,
		Channel: firstNonEmpty(opts.channel, "原支付渠道"), Amount: opts.amount,
		Reason: opts.reason, RequestedAt: opts.at, CreatedAt: opts.at, UpdatedAt: opts.at,
	}
	if opts.caseDraft != nil {
		row.CaseID = opts.caseDraft.row.ID
		opts.caseDraft.refundNo = no
	}
	// Attach the refund to the payment that actually settled the money.
	if len(o.payments) > 0 {
		row.PaymentID = o.settledPayment().ID
	}
	if opts.status == "succeeded" {
		done := opts.at.Add(24 * time.Hour)
		if opts.completedAt != nil {
			done = *opts.completedAt
		}
		if done.After(o.g.anchor) {
			done = o.g.anchor
		}
		if done.Before(opts.at) {
			o.fail("refund completed before requested")
			return nil
		}
		row.CompletedAt = timePtr(done)
		o.row.Refunded += opts.amount
		o.applyPaymentRefund(opts.amount)
		if opts.caseDraft != nil && !opts.compensation {
			item := o.itemByID(opts.caseDraft.row.OrderItemID)
			if item != nil {
				qty := opts.caseDraft.row.Quantity
				if item.refunded+qty > item.qty {
					o.fail("refunded quantity %d exceeds purchased %d", item.refunded+qty, item.qty)
				} else {
					item.refunded += qty
				}
				if opts.caseDraft.wasOpen {
					item.openQty -= qty
					if item.openQty < 0 {
						item.openQty = 0
					}
				}
			}
		}
	} else if opts.status == "failed" || opts.status == "rejected" {
		row.FailureReason = firstNonEmpty(opts.failure, "渠道退回失败，请更换收款方式后重试")
	}
	o.refunds = append(o.refunds, row)
	return row
}

func (o *orderDraft) settledPayment() *paymentRow {
	for _, p := range o.payments {
		if p.Status == "succeeded" || p.Status == "partially_refunded" || p.Status == "refunded" {
			return p
		}
	}
	return o.payments[0]
}

// applyPaymentRefund spreads the refund over settled payments proportionally so
// combined payments keep a coherent per-channel refunded amount.
func (o *orderDraft) applyPaymentRefund(amount int64) {
	var settled []*paymentRow
	var total int64
	for _, p := range o.payments {
		if p.Status == "succeeded" || p.Status == "partially_refunded" || p.Status == "refunded" {
			settled = append(settled, p)
			total += p.Amount
		}
	}
	if len(settled) == 0 {
		return
	}
	var assigned int64
	for i, p := range settled {
		share := amount * p.Amount / total
		if i == len(settled)-1 {
			share = amount - assigned
		}
		assigned += share
		p.Refunded += share
		if p.Refunded >= p.Amount {
			p.Status = "refunded"
		} else if p.Refunded > 0 {
			p.Status = "partially_refunded"
		}
	}
}

func (o *orderDraft) itemByID(id string) *itemDraft {
	for _, it := range o.items {
		if it.row.ID == id {
			return it
		}
	}
	return nil
}

func orderedItemID(o *orderDraft, idx int) string { return o.items[idx].row.ID }

func terminalCase(status string) bool {
	switch status {
	case "completed", "rejected", "cancelled":
		return true
	}
	return false
}

// ---- invoices ----

type invoiceOpts struct {
	typ      string
	status   string
	title    string
	taxNo    string
	email    string
	at       time.Time
	amount   int64
	fileURL  string
	issuedAt *time.Time
	suffix   string
}

func (o *orderDraft) invoice(opts invoiceOpts) *invoiceRow {
	if o.row.PaidAt == nil {
		o.fail("invoice on unpaid order")
		return nil
	}
	amount := opts.amount
	if amount == 0 {
		amount = o.paid - o.row.Refunded
	}
	if amount > o.paid-o.row.Refunded {
		o.fail("invoice amount %d exceeds paid-refunded %d", amount, o.paid-o.row.Refunded)
		return nil
	}
	if opts.typ == "vat_special" && opts.taxNo == "" {
		o.fail("vat special invoice without tax number")
		return nil
	}
	if opts.typ == "electronic" && opts.email == "" {
		o.fail("electronic invoice without email")
		return nil
	}
	no := ""
	status := opts.status
	if status == "" {
		status = "issued"
	}
	if status != "rejected" {
		no = o.g.nextInvoiceNo() + opts.suffix
	}
	row := &invoiceRow{
		ID: stableID("invoice", o.row.No, no, opts.typ), No: no, CustomerID: o.row.CustomerID, OrderID: o.row.ID,
		Type: opts.typ, Status: status, Title: opts.title, TaxNo: opts.taxNo, Amount: amount,
		Email: opts.email, RequestedAt: opts.at, CreatedAt: opts.at, UpdatedAt: opts.at,
	}
	if status == "issued" || status == "red_flushed" || status == "voided" {
		issued := opts.at.Add(26 * time.Hour)
		if opts.issuedAt != nil {
			issued = *opts.issuedAt
		}
		if issued.After(o.g.anchor) {
			issued = o.g.anchor
		}
		if issued.Before(opts.at) {
			o.fail("invoice issued before requested")
			return nil
		}
		row.IssuedAt = timePtr(issued)
		if opts.typ == "electronic" {
			row.FileURL = firstNonEmpty(opts.fileURL, fmt.Sprintf("https://example.invalid/invoices/%s.pdf", no))
		}
	}
	o.invoices = append(o.invoices, row)
	return row
}

// ---- commit ----

func (o *orderDraft) commit() {
	r := o.row
	r.Paid = o.paid
	r.Refunded = o.row.Refunded
	r.ItemAmount = o.row.ItemAmount
	r.Discount = o.row.Discount
	r.Shipping = o.row.Shipping
	if r.UpdatedAt.Before(r.CreatedAt) {
		r.UpdatedAt = r.CreatedAt
	}
	// A completed order whose money is fully returned is closed, not merely
	// completed: it is the only status reading that matches "已退款完成".
	if r.Status == "completed" && r.Paid > 0 && r.Refunded == r.Paid {
		r.Status = "closed"
	}
	// Item rows are snapshots: the historic unit price never follows the catalog.
	for i, it := range o.items {
		it.row = itemRow{
			ID: stableID("item", r.No, fmt.Sprintf("%d", i)), OrderID: r.ID,
			ProductID: it.product.ID, SKUID: it.sku.ID,
			ProductCode: it.product.Code, ProductName: it.product.Name,
			SKUCode: it.sku.Code, SKUName: it.sku.Name, SKUAttributes: it.sku.Specs,
			Quantity: it.qty, RefundedQty: it.refunded, UnitPrice: it.unit,
			Discount: it.discount, Paid: it.paid, Status: "active",
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
		o.g.ds.items = append(o.g.ds.items, it.row)
		if it.refunded >= it.qty {
			o.g.ds.items[len(o.g.ds.items)-1].Status = "completed"
		}
	}
	// Orders are appended after items so a failing item loop is visible.
	o.g.ds.orders = append(o.g.ds.orders, r)
	for _, p := range o.payments {
		o.g.ds.payments = append(o.g.ds.payments, *p)
	}
	for _, rf := range o.refunds {
		o.g.ds.refunds = append(o.g.ds.refunds, *rf)
	}
	for _, sh := range o.shipments {
		o.g.ds.shipments = append(o.g.ds.shipments, sh.row)
		o.g.ds.events = append(o.g.ds.events, sh.events...)
	}
	for _, c := range o.cases {
		o.g.ds.cases = append(o.g.ds.cases, c.row)
	}
	for _, inv := range o.invoices {
		o.g.ds.invoices = append(o.g.ds.invoices, *inv)
	}
}

// ---- small helpers ----

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func maskPhone(raw string) string {
	if len(raw) == 11 {
		return raw[0:3] + "****" + raw[7:11]
	}
	return "****"
}
