package seed

import (
	"fmt"
	"time"
)

// buildIMWorld appends CUST-2001..2051 and the ORD-20xxx world facts required by
// testdata/im_export/scenarios_im.jsonl. It runs after buildConversations so the
// original CUST-10xx / ORD-100xx stories and their seed dialogues stay untouched.
func (g *gen) buildIMWorld() {
	startDrafts := len(g.orderDrafts)
	g.appendIMCustomers()

	// CUST-2001 index is 21 (1-based) after the original 20 customers.
	c := func(custNo int) int { return custNo - 2000 + 20 } // CUST-2001 -> 21

	// ---- LG logistics ----
	// LG_200: three in-transit parcels for the same customer.
	g.imShipped(c(2001), "ORD-20001", 60, "SKU-P073-01", 12900, "SF", "", "in_transit")
	g.imShipped(c(2001), "ORD-20201", 48, "SKU-P013-01", 6900, "YTO", "", "in_transit")
	g.imShipped(c(2001), "ORD-20202", 56, "SKU-P074-01", 4300, "ZTO", "", "in_transit")

	g.imShipped(c(2002), "ORD-20002", 64, "SKU-P008-01", 8900, "SF", "", "in_transit") // unit override
	g.imPaid(c(2003), "ORD-20003", 72, "SKU-P014-01", 16800)                           // stalled, not shipped
	g.imDelivered(c(2004), "ORD-20004", 120, "SKU-P005-01", 25900, "SF", "代签收")         // fake delivery dispute
	g.imShipped(c(2005), "ORD-20005", 56, "SKU-P013-02", 6600, "JD", "", "delivering")
	g.imShipped(c(2006), "ORD-20006", 56, "SKU-P074-01", 4300, "ZTO", "", "delivering") // pickup code

	// LG_206 split packages: two forward shipments, second fixed MOCKSF7002.
	g.imSplitShip(c(2007), "ORD-20007", 48, "SKU-P009-01", 19900, "SKU-P073-01", 12900)

	g.imShipped(c(2008), "ORD-20008", 64, "SKU-P057-01", 9900, "SF", "MOCKSF7003", "in_transit")

	// ---- RT after-sales ----
	g.imDelivered(c(2009), "ORD-20009", 96, "SKU-P054-01", 15900, "SF", "本人")
	g.imDelivered(c(2010), "ORD-20010", 80, "SKU-P022-02", 22900, "SF", "本人") // customized engraved
	g.imDelivered(c(2011), "ORD-20011", 72, "SKU-P013-05", 7900, "YTO", "本人")
	g.imDelivered(c(2012), "ORD-20012", 60, "SKU-P013-05", 12900, "SF", "本人") // RT_211 size exchange
	g.imDelivered(c(2013), "ORD-20013", 1440, "SKU-P054-01", 18900, "SF", "本人") // ~60d ago, warranty
	g.imDelivered(c(2014), "ORD-20014", 720, "SKU-P014-02", 26800, "SF", "本人")  // ~30d, over window

	// ---- RF refunds ----
	g.imCancelled(c(2015), "ORD-20015", 48, "SKU-P057-01", 9900)
	g.imCancelled(c(2016), "ORD-20016", 60, "SKU-P057-01", 9900)
	g.imPartialRefund(c(2017), "ORD-20017", 96) // paid 5000, refunded 3000
	g.imCancelled(c(2018), "ORD-20018", 40, "SKU-P009-01", 19900)

	// ---- IV invoices ----
	g.imInvoiceRequested(c(2019), "ORD-20019", 200, "SKU-P060-01", 45800)
	g.imInvoiceIssued(c(2020), "ORD-20020", 180, "SKU-P014-02", 26800, "某某科技有限公", "wrong@example.test")
	g.imInvoiceIssued(c(2021), "ORD-20021", 160, "SKU-P073-01", 12900, "测试公司", "typo@example.test")
	g.imDelivered(c(2022), "ORD-20022", 100, "SKU-P013-01", 6900, "SF", "本人") // how-to, no invoice yet
	g.imInvoiceVAT(c(2023), "ORD-20023", 220, "SKU-P004-01", 89600)

	// ---- PY payment ----
	g.imDuplicatePay(c(2024), "ORD-20024", 30, "SKU-P014-01", 16800)

	// ---- PP product (pending or completed) ----
	g.imPending(c(2025), "ORD-20025", 10, "SKU-P073-01", 12900) // cup height
	g.imPending(c(2026), "ORD-20026", 8, "SKU-P022-01", 19900)  // gift box
	g.imPending(c(2027), "ORD-20027", 6, "SKU-P074-01", 4300)   // charger compatibility
	g.imDelivered(c(2028), "ORD-20028", 48, "SKU-P058-01", 4900, "YTO", "本人") // opened return
	g.imPending(c(2029), "ORD-20029", 5, "SKU-P015-04", 62900)                // OOS L
	g.imPending(c(2030), "ORD-20030", 4, "SKU-P075-01", 5900)                 // shelf life

	// ---- HT human transfer ----
	g.imShipped(c(2031), "ORD-20031", 56, "SKU-P073-01", 12900, "SF", "", "in_transit")
	g.imDelivered(c(2032), "ORD-20032", 90, "SKU-P008-01", 8900, "SF", "本人")
	g.imShipped(c(2033), "ORD-20033", 56, "SKU-P054-01", 15900, "JD", "", "in_transit")
	g.imDelivered(c(2034), "ORD-20034", 110, "SKU-P014-01", 29900, "SF", "本人")
	g.imDelivered(c(2035), "ORD-20035", 70, "SKU-P075-01", 5900, "YTO", "本人")
	g.imShipped(c(2036), "ORD-20036", 56, "SKU-P073-02", 12900, "SF", "", "delivering")

	// ---- CP complaint / chitchat ----
	g.imDelivered(c(2037), "ORD-20037", 150, "SKU-P056-01", 3900, "SF", "本人")
	g.imDelivered(c(2038), "ORD-20038", 140, "SKU-P013-01", 6900, "SF", "本人")
	g.imDelivered(c(2039), "ORD-20039", 130, "SKU-P073-01", 12900, "SF", "本人")
	g.imDelivered(c(2040), "ORD-20040", 120, "SKU-P056-01", 2900, "YTO", "本人")

	// ---- MD order modify ----
	g.imPaid(c(2041), "ORD-20041", 18, "SKU-P008-01", 8900)
	g.imPaid(c(2042), "ORD-20042", 16, "SKU-P073-01", 12900)
	g.imPaid(c(2043), "ORD-20043", 14, "SKU-P013-02", 6600)
	g.imShipped(c(2044), "ORD-20044", 64, "SKU-P054-01", 15900, "SF", "", "in_transit") // cancel ineligible
	g.imPaid(c(2045), "ORD-20045", 12, "SKU-P074-01", 4300)
	g.imPaid(c(2046), "ORD-20046", 10, "SKU-P073-01", 12900)

	// ---- TN tone ----
	g.imShipped(c(2047), "ORD-20047", 56, "SKU-P075-01", 5900, "SF", "", "in_transit")
	g.imDelivered(c(2048), "ORD-20048", 80, "SKU-P057-01", 9900, "SF", "本人")
	g.imDelivered(c(2049), "ORD-20049", 100, "SKU-P009-01", 19900, "SF", "本人")
	g.imPending(c(2050), "ORD-20500", 3, "SKU-P015-04", 62900)
	g.imShipped(c(2051), "ORD-20501", 56, "SKU-P073-01", 12900, "SF", "", "in_transit")

	for _, o := range g.orderDrafts[startDrafts:] {
		o.commit()
	}
}

func (g *gen) appendIMCustomers() {
	created := g.anchor.AddDate(0, -2, 0)
	names := []string{
		"艾测一", "毕测二", "崔测三", "邓测四", "鄂测五", "范测六", "高测七", "侯测八",
		"纪测九", "康测十", "雷测十一", "梅测十二", "倪测十三", "欧测十四", "潘测十五", "齐测十六",
		"任测十七", "沈测十八", "唐测十九", "万测二十", "夏测廿一", "尹测廿二", "邹测廿三", "安测廿四",
		"包测廿五", "曹测廿六", "戴测廿七", "费测廿八", "龚测廿九", "杭测三十", "姜测卅一", "孔测卅二",
		"蓝测卅三", "莫测卅四", "宁测卅五", "裴测卅六", "曲测卅七", "阮测卅八", "苏测卅九", "陶测四十",
		"魏测卌一", "邢测卌二", "俞测卌三", "卓测卌四", "楚测卌五", "东方测卌六", "关测卌七", "华测卌八",
		"靳测卌九", "陆测五十", "闵测五一",
	}
	for i, name := range names {
		no := fmt.Sprintf("CUST-%d", 2001+i)
		phone := fmt.Sprintf("13900002%03d", i+1)
		def := customerDef{
			No: no, Name: name, Phone: phone, Profile: "IM评测客户",
			Addrs: []addressDef{
				baseAddr("家", name, "测试六区", fmt.Sprintf("模拟路 %d 号 测试花园 IM-%d", 800+i, i+1)),
			},
		}
		g.customerDefs = append(g.customerDefs, def)
		row := customerRow{
			ID: stableID("customer", no), No: no, Name: name,
			PhoneMasked: maskPhone(phone), Status: "active",
			CreatedAt: created, UpdatedAt: created,
		}
		g.customers = append(g.customers, &row)
		g.ds.customers = append(g.ds.customers, row)
	}
}

// imNew creates a freeShip / noCompanion order with a fixed number and optional
// unit override so paid_amount_minor matches the IM fixture exactly.
func (g *gen) imNew(cust int, no string, hours float64, lines []lineOpt) *orderDraft {
	return g.newOrder(orderOpts{
		customer: cust, at: g.ago(hours), no: no, noCompanion: true, freeShip: true,
		lines: lines, channel: "mock_store",
	})
}

func (g *gen) imPending(cust int, no string, hours float64, sku string, unit int64) {
	g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
}

func (g *gen) imPaid(cust int, no string, hours float64, sku string, unit int64) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	o.pay(o.row.CreatedAt.Add(12*time.Minute), "wechat_pay")
}

func (g *gen) imShipped(cust int, no string, hours float64, sku string, unit int64, carrier, tracking, status string) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	shipAt := at.Add(8 * time.Hour)
	if shipAt.After(g.anchor) {
		shipAt = g.ago(30)
	}
	if status == "" {
		status = "in_transit"
	}
	o.ship(shipOpts{carrier: carrier, start: shipAt, status: status, TrackingNo: tracking})
	o.markShipped(shipAt)
}

func (g *gen) imDelivered(cust int, no string, hours float64, sku string, unit int64, carrier, signer string) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	shipAt := at.Add(10 * time.Hour)
	o.ship(shipOpts{carrier: carrier, start: shipAt, status: "delivered", signer: signer})
	o.markShipped(shipAt)
	// deliver uses the delivered event time; ship() already capped events ≤ anchor.
	deliv := shipAt.Add(32 * time.Hour)
	if deliv.After(g.anchor) {
		deliv = g.anchor
	}
	o.deliver(deliv)
}

func (g *gen) imCancelled(cust int, no string, hours float64, sku string, unit int64) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	o.cancel(at.Add(3*time.Hour), "用户取消，全额退款")
}

func (g *gen) imPartialRefund(cust int, no string, hours float64) {
	// Fixture: paid 5000, refunded 3000 — two lines 3000+2000, refund first line.
	o := g.imNew(cust, no, hours, []lineOpt{
		{sku: "SKU-P056-01", qty: 1, unit: 3000},
		{sku: "SKU-P058-01", qty: 1, unit: 2000},
	})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	shipAt := at.Add(10 * time.Hour)
	o.ship(shipOpts{carrier: "SF", start: shipAt, status: "delivered", signer: "本人"})
	o.markShipped(shipAt)
	deliv := shipAt.Add(32 * time.Hour)
	if deliv.After(g.anchor) {
		deliv = g.anchor
	}
	o.deliver(deliv)
	c := o.newCase(caseOpts{
		typ: "return", status: "submitted", item: 0, qty: 1,
		at: deliv.Add(20 * time.Hour), reasonCode: "no_reason",
		reasonText: "只退其中一件", desc: "部分退货，另一件仍在审核",
	})
	if c == nil {
		return
	}
	o.newRefund(refundOpts{
		amount: 3000, status: "succeeded", at: deliv.Add(28 * time.Hour),
		reason: "部分退货退款", caseDraft: c,
	})
	c.close("completed", deliv.Add(30*time.Hour), "已退款 30.00")
	// Second line still under review.
	o.newCase(caseOpts{
		typ: "return", status: "pending_review", item: 1, qty: 1,
		at: deliv.Add(22 * time.Hour), reasonCode: "no_reason",
		reasonText: "另一件也想退", desc: "第二件售后审核中",
	})
}

func (g *gen) imSplitShip(cust int, no string, hours float64, sku1 string, u1 int64, sku2 string, u2 int64) {
	o := g.imNew(cust, no, hours, []lineOpt{
		{sku: sku1, qty: 1, unit: u1},
		{sku: sku2, qty: 1, unit: u2},
	})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	shipAt := at.Add(8 * time.Hour)
	// First parcel already delivering; second still in transit with fixed tracking.
	o.ship(shipOpts{carrier: "SF", start: shipAt, status: "delivered", signer: "本人"})
	o.ship(shipOpts{carrier: "SF", start: shipAt.Add(6 * time.Hour), status: "in_transit", TrackingNo: "MOCKSF7002"})
	o.markShipped(shipAt)
	// Order stays shipped (not completed) — second parcel still on the way.
}

func (g *gen) imDuplicatePay(cust int, no string, hours float64, sku string, unit int64) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	o.attempt(at.Add(13*time.Minute), "wechat_pay", "pending", o.due, "MOCK-DUP-REVERSAL-PENDING-"+o.row.No)
}

func (g *gen) imInvoiceRequested(cust int, no string, hours float64, sku string, unit int64) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	shipAt := at.Add(10 * time.Hour)
	o.ship(shipOpts{carrier: "SF", start: shipAt, status: "delivered", signer: "本人"})
	o.markShipped(shipAt)
	deliv := shipAt.Add(32 * time.Hour)
	if deliv.After(g.anchor) {
		deliv = g.anchor
	}
	o.deliver(deliv)
	o.invoice(invoiceOpts{
		typ: "electronic", status: "requested", title: "测试科技有限公司",
		email: "finance@example.test", at: deliv.Add(6 * time.Hour),
	})
}

func (g *gen) imInvoiceIssued(cust int, no string, hours float64, sku string, unit int64, title, email string) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "wechat_pay")
	shipAt := at.Add(10 * time.Hour)
	o.ship(shipOpts{carrier: "SF", start: shipAt, status: "delivered", signer: "本人"})
	o.markShipped(shipAt)
	deliv := shipAt.Add(32 * time.Hour)
	if deliv.After(g.anchor) {
		deliv = g.anchor
	}
	o.deliver(deliv)
	issued := deliv.Add(20 * time.Hour)
	if issued.After(g.anchor) {
		issued = g.anchor
	}
	o.invoice(invoiceOpts{
		typ: "electronic", status: "issued", title: title, email: email,
		at: deliv.Add(4 * time.Hour), issuedAt: timePtr(issued),
	})
}

func (g *gen) imInvoiceVAT(cust int, no string, hours float64, sku string, unit int64) {
	o := g.imNew(cust, no, hours, []lineOpt{{sku: sku, qty: 1, unit: unit}})
	if o == nil {
		return
	}
	at := o.row.CreatedAt
	o.pay(at.Add(12*time.Minute), "alipay")
	shipAt := at.Add(10 * time.Hour)
	o.ship(shipOpts{carrier: "SF", start: shipAt, status: "delivered", signer: "前台"})
	o.markShipped(shipAt)
	deliv := shipAt.Add(32 * time.Hour)
	if deliv.After(g.anchor) {
		deliv = g.anchor
	}
	o.deliver(deliv)
	o.invoice(invoiceOpts{
		typ: "vat_special", status: "requested", title: "测试增值税专用抬头有限公司",
		taxNo: "TESTTAX000000IM01", at: deliv.Add(8 * time.Hour),
	})
}
