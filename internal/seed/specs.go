package seed

// orderSpecs is the curated story board: 118 orders, every one tied to a user
// profile and a testing purpose. Nothing here is random — the status mix,
// refund window edges and logistics exceptions are all deliberate.
func orderSpecs() []orderSpec {
	return []orderSpec{
		// ---------------- U001 张三：3C 高频买家（8 单） ----------------
		{kind: "unpaid", cust: 1, h: 2, lines: ls("SKU-P001-01", 1), invoice: "electronic"},
		{kind: "payFailed", cust: 1, h: 26, lines: ls("SKU-P004-01", 1), pay: "alipay", note: "pending"},
		{kind: "paid", cust: 1, h: 12, lines: ls("SKU-P006-02", 1)},
		{kind: "transit", cust: 1, h: 72, lines: ls("SKU-P005-01", 1), carrier: "SF"},
		{kind: "priceProtect", cust: 1, h: 192, lines: ls("SKU-P001-01", 1), item: 0, qty: 1},
		{kind: "invoiceIssued", cust: 1, h: 480, lines: ls("SKU-P060-01", 1), invoice: "vat_special"},
		{kind: "cancelPaid", cust: 1, h: 120, lines: ls("SKU-P003-01", 1)},
		{kind: "duplicateCharge", cust: 1, h: 40, lines: ls("SKU-P008-01", 1)},

		// ---------------- U002 李四：服装买家（6 单） ----------------
		{kind: "delivered", cust: 2, h: 120, lines: ls("SKU-P013-02", 1), invoice: "electronic"},
		{kind: "exchange", cust: 2, h: 96, lines: ls("SKU-P016-02", 1), item: 0, qty: 1, invoice: "electronic"},
		{kind: "returnPartial", cust: 2, h: 168, lines: ls2("SKU-P014-01", 1, "SKU-P013-01", 2), disc: 1000, item: 1, qty: 1, invoice: "electronic"},
		{kind: "fullReturn", cust: 2, h: 960, lines: ls("SKU-P015-01", 1)},
		{kind: "transit", cust: 2, h: 48, lines: ls("SKU-P018-02", 1), carrier: "SF"},
		{kind: "cancelled", cust: 2, h: 30, lines: ls("SKU-P013-03", 1)},

		// ---------------- U003 王沐：生鲜高频买家（8 单） ----------------
		{kind: "freshPartial", cust: 3, h: 96, lines: ls("SKU-P029-01", 1), cold: true, item: 0, qty: 1, invoice: "electronic"},
		{kind: "transit", cust: 3, h: 30, lines: ls("SKU-P035-01", 2), cold: true, carrier: "SF"},
		{kind: "stalled", cust: 3, h: 144, lines: ls("SKU-P030-01", 1), cold: true, carrier: "YTO"},
		{kind: "deliveryFailed", cust: 3, h: 60, lines: ls("SKU-P033-01", 1), cold: true, carrier: "JD"},
		{kind: "overWindow", cust: 3, h: 600, lines: ls("SKU-P036-01", 1), cold: true, item: 0, qty: 1, invoice: "electronic"},
		{kind: "paid", cust: 3, h: 6, lines: ls("SKU-P034-01", 1), cold: true},
		{kind: "cancelPaid", cust: 3, h: 72, lines: ls("SKU-P037-01", 2), cold: true},
		{kind: "transit", cust: 3, h: 24, lines: ls("SKU-P032-01", 1), cold: true, carrier: "ZTO"},

		// ---------------- U004 赵岚：礼品/定制（6 单） ----------------
		{kind: "customRejected", cust: 4, h: 96, lines: ls("SKU-P022-02", 1), item: 0, qty: 1, note: "刻字款"},
		{kind: "refundRequested", cust: 4, h: 120, lines: ls("SKU-P021-01", 1), item: 0, qty: 1, note: "茶具到货后发现杯口有缺口，已提交仅退款"},
		{kind: "transit", cust: 4, h: 72, lines: ls("SKU-P023-02", 1), scheduled: true, cold: true, carrier: "SF"},
		{kind: "fullReturn", cust: 4, h: 240, lines: ls("SKU-P024-02", 1), cold: true},
		{kind: "unpaid", cust: 4, h: 1, lines: ls("SKU-P025-01", 1), note: "定制相册，等待客户确认排版"},
		{kind: "invoiceIssued", cust: 4, h: 168, lines: ls2("SKU-P026-02", 1, "SKU-P050-01", 1), disc: 3000, invoice: "electronic"},

		// ---------------- U005 陈稳：家电买家（8 单） ----------------
		{kind: "transit", cust: 5, h: 72, lines: ls("SKU-P041-01", 1), scheduled: true, carrier: "JD"},
		{kind: "repair", cust: 5, h: 216, lines: ls("SKU-P042-01", 1), scheduled: true, item: 0, qty: 1, invoice: "vat_special"},
		{kind: "transit", cust: 5, h: 48, lines: ls("SKU-P048-01", 1), scheduled: true, carrier: "JD"},
		{kind: "priceProtectOld", cust: 5, h: 360, lines: ls("SKU-P039-01", 1), item: 0, qty: 1, invoice: "electronic"},
		{kind: "reship", cust: 5, h: 72, lines: ls("SKU-P040-01", 1), item: 0, qty: 1, note: "缺少遥控器，申请补发", invoice: "electronic"},
		{kind: "cancelPaid", cust: 5, h: 48, lines: ls("SKU-P043-01", 1)},
		{kind: "fullReturn", cust: 5, h: 720, lines: ls("SKU-P044-01", 1)},
		{kind: "paid", cust: 5, h: 5, lines: ls("SKU-P046-01", 1)},

		// ---------------- U006 孙可：优惠券用户（5 单） ----------------
		{kind: "delivered", cust: 6, h: 96, lines: ls2("SKU-P050-01", 2, "SKU-P051-01", 1), disc: 3000, invoice: "electronic",
			note: "使用满 299 减 30 优惠"},
		{kind: "refundFailed", cust: 6, h: 144, lines: ls("SKU-P054-01", 1), disc: 2000,
			note: "使用新客券 20 元，退款时券返还失败", item: 0, qty: 1},
		{kind: "transit", cust: 6, h: 48, lines: ls("SKU-P056-01", 2), carrier: "SF"},
		{kind: "cancelled", cust: 6, h: 48, lines: ls("SKU-P053-01", 2)},
		{kind: "transit", cust: 6, h: 40, lines: ls("SKU-P054-02", 1), carrier: "SF"},

		// ---------------- U007 周棠：多次退款（9 单） ----------------
		{kind: "refundProcessing", cust: 7, h: 120, lines: ls("SKU-P055-01", 1), item: 0, qty: 1, invoice: "electronic"},
		{kind: "refundRejected", cust: 7, h: 168, lines: ls("SKU-P054-02", 1), item: 0, qty: 1, invoice: "paper"},
		{kind: "refundFailed", cust: 7, h: 216, lines: ls("SKU-P045-01", 1), item: 0, qty: 1},
		{kind: "returnDone", cust: 7, h: 288, lines: ls("SKU-P047-01", 1), item: 0, qty: 1},
		{kind: "returnDone", cust: 7, h: 480, lines: ls("SKU-P049-01", 1), item: 0, qty: 1},
		{kind: "delivered", cust: 7, h: 72, lines: ls("SKU-P056-01", 1)},
		{kind: "cancelPaid", cust: 7, h: 96, lines: ls("SKU-P051-01", 1)},
		{kind: "unpaid", cust: 7, h: 3, lines: ls("SKU-P052-01", 1)},
		{kind: "priceProtect", cust: 7, h: 240, lines: ls("SKU-P060-01", 1), item: 0, qty: 1},

		// ---------------- U008 吴笛：多地址/改地址（5 单） ----------------
		{kind: "paid", cust: 8, addr: 0, h: 8, lines: ls("SKU-P063-01", 1)},
		{kind: "transit", cust: 8, addr: 1, h: 48, lines: ls("SKU-P061-01", 1), carrier: "YTO"},
		{kind: "paid", cust: 8, addr: 2, h: 3, lines: ls("SKU-P065-01", 1)},
		{kind: "refundApproved", cust: 8, addr: 1, h: 144, lines: ls("SKU-P062-01", 1), item: 0, qty: 1},
		{kind: "cancelled", cust: 8, addr: 2, h: 24, lines: ls("SKU-P066-01", 1)},

		// ---------------- U009 郑川：多单同时在途（8 单） ----------------
		{kind: "transit", cust: 9, h: 48, lines: ls("SKU-P010-01", 1), carrier: "SF"},
		{kind: "transit", cust: 9, h: 45, lines: ls("SKU-P010-01", 1), carrier: "YTO"},
		{kind: "stalled", cust: 9, h: 144, lines: ls("SKU-P011-01", 1), carrier: "JD"},
		{kind: "fullReturn", cust: 9, h: 192, lines: ls("SKU-P012-01", 1), invoice: "electronic"},
		{kind: "multiParcel", cust: 9, h: 72, lines: ls2("SKU-P017-01", 1, "SKU-P019-01", 1), disc: 1500},
		{kind: "paid", cust: 9, h: 4, lines: ls("SKU-P009-01", 2)},
		{kind: "unpaid", cust: 9, h: 0.5, lines: ls("SKU-P007-01", 1)},
		{kind: "overWindow", cust: 9, h: 960, lines: ls("SKU-P016-01", 1), item: 0, qty: 1, invoice: "electronic"},

		// ---------------- U010 冯楠：投诉并转人工（6 单） ----------------
		{kind: "lost", cust: 10, h: 96, lines: ls("SKU-P012-02", 1), signer: "驿站", item: 0, qty: 1, invoice: "electronic"},
		{kind: "safety", cust: 10, h: 120, lines: ls("SKU-P045-01", 1), item: 0, qty: 1},
		{kind: "transit", cust: 10, h: 24, lines: ls("SKU-P005-02", 1), carrier: "SF"},
		{kind: "cancelPaid", cust: 10, h: 72, lines: ls("SKU-P004-02", 1)},
		{kind: "delivered", cust: 10, h: 240, lines: ls("SKU-P006-01", 1), invoice: "electronic"},
		{kind: "paid", cust: 10, h: 20, lines: ls("SKU-P001-03", 1)},

		// ---------------- U011 何斐：跨境/保税（4 单） ----------------
		{kind: "transit", cust: 11, h: 96, lines: ls("SKU-P002-02", 1), channel: "cross_border", carrier: "JD"},
		{kind: "transit", cust: 11, h: 120, lines: ls("SKU-P002-01", 1), channel: "cross_border", carrier: "JD"},
		{kind: "presale", cust: 11, h: 48, lines: ls("SKU-P002-01", 1), channel: "presale"},
		{kind: "intercept", cust: 11, h: 120, lines: ls("SKU-P008-02", 1), carrier: "SF"},

		// ---------------- U012 许澜：货到付款（4 单） ----------------
		{kind: "codShipped", cust: 12, h: 48, lines: ls("SKU-P064-01", 1)},
		{kind: "codDelivered", cust: 12, h: 144, lines: ls("SKU-P049-01", 2), invoice: "electronic"},
		{kind: "cancelled", cust: 12, h: 24, lines: ls("SKU-P050-01", 1)},
		{kind: "transit", cust: 12, h: 48, lines: ls("SKU-P057-01", 1), carrier: "ZTO"},

		// ---------------- U013 邓一：组合支付（5 单） ----------------
		{kind: "mixed", cust: 13, h: 72, lines: ls("SKU-P001-02", 1), invoice: "vat_special"},
		{kind: "mixedAggregate", cust: 13, h: 72, lines: ls("SKU-P004-01", 1)},
		{kind: "returnPartial", cust: 13, h: 240, lines: ls2("SKU-P019-02", 1, "SKU-P020-01", 2), disc: 1500, item: 1, qty: 1},
		{kind: "presale", cust: 13, h: 24, lines: ls("SKU-P015-01", 1)},
		{kind: "unpaid", cust: 13, h: 2, lines: ls("SKU-P013-04", 1), note: "尺码 2XL 缺货，等待补货"},

		// ---------------- U014 苏禾：预售定金尾款（5 单） ----------------
		{kind: "presale", cust: 14, h: 72, lines: ls("SKU-P015-01", 1)},
		{kind: "presale", cust: 14, h: 120, lines: ls("SKU-P002-01", 1)},
		{kind: "presaleBalance", cust: 14, h: 96, lines: ls("SKU-P015-02", 1)},
		{kind: "fullReturn", cust: 14, h: 216, lines: ls("SKU-P011-02", 1)},
		{kind: "cancelled", cust: 14, h: 48, lines: ls("SKU-P014-02", 1)},

		// ---------------- U015 林步：套装与赠品（5 单） ----------------
		{kind: "delivered", cust: 15, h: 96, lines: ls2("SKU-P026-01", 1, "SKU-P050-01", 1), disc: 3000, invoice: "electronic",
			note: "满 199 赠礼盒手提袋，赠品不参与退款"},
		{kind: "reship", cust: 15, h: 72, lines: ls("SKU-P027-01", 1), item: 0, qty: 1, note: "积木缺少 3 个零件，申请补发", invoice: "electronic"},
		{kind: "transit", cust: 15, h: 40, lines: ls("SKU-P028-01", 1), carrier: "ZTO"},
		{kind: "paid", cust: 15, h: 24, lines: ls2("SKU-P021-01", 1, "SKU-P022-01", 1), disc: 3000},
		{kind: "cancelled", cust: 15, h: 12, lines: ls("SKU-P024-01", 1)},

		// ---------------- U016 高远：故障维修（5 单） ----------------
		{kind: "repair", cust: 16, h: 168, lines: ls("SKU-P039-01", 1), item: 0, qty: 1, note: "内胆涂层脱落，申请维修", invoice: "electronic"},
		{kind: "paid", cust: 16, h: 240, lines: ls("SKU-P043-02", 1)},
		{kind: "safety", cust: 16, h: 96, lines: ls("SKU-P055-01", 1), item: 0, qty: 1},
		{kind: "transit", cust: 16, h: 48, lines: ls("SKU-P042-01", 1), scheduled: true, carrier: "JD"},
		{kind: "paid", cust: 16, h: 6, lines: ls("SKU-P044-01", 1)},

		// ---------------- U017 罗晴：少件补发（5 单） ----------------
		{kind: "reship", cust: 17, h: 120, lines: ls("SKU-P017-02", 1), item: 0, qty: 1, note: "包内缺少防雨罩", invoice: "electronic"},
		{kind: "paid", cust: 17, h: 120, lines: ls("SKU-P061-02", 1)},
		{kind: "transit", cust: 17, h: 48, lines: ls("SKU-P019-02", 1), carrier: "YTO"},
		{kind: "cancelPaid", cust: 17, h: 72, lines: ls("SKU-P020-01", 1)},
		{kind: "fullReturn", cust: 17, h: 216, lines: ls("SKU-P070-01", 1), scheduled: true},

		// ---------------- U018 梁澈：退货被拒收（5 单） ----------------
		{kind: "merchantRefuse", cust: 18, h: 192, lines: ls("SKU-P018-01", 1), item: 0, qty: 1, invoice: "electronic"},
		{kind: "delivered", cust: 18, h: 240, lines: ls("SKU-P013-01", 2), invoice: "electronic"},
		{kind: "transit", cust: 18, h: 24, lines: ls("SKU-P014-03", 1), carrier: "YTO"},
		{kind: "paid", cust: 18, h: 2, lines: ls("SKU-P015-03", 1)},
		{kind: "fullReturn", cust: 18, h: 480, lines: ls("SKU-P070-02", 1), scheduled: true},

		// ---------------- U019 宋知：发票问题（5 单） ----------------
		{kind: "fullReturn", cust: 19, h: 144, lines: ls("SKU-P004-01", 1), invoice: "electronic"},
		{kind: "invoiceRed", cust: 19, h: 264, lines: ls("SKU-P052-01", 2), item: 0, qty: 1},
		{kind: "invoiceRejected", cust: 19, h: 96, lines: ls("SKU-P002-02", 1), invoice: "vat_special"},
		{kind: "paid", cust: 19, h: 5, lines: ls("SKU-P059-01", 1)},
		{kind: "transit", cust: 19, h: 72, lines: ls("SKU-P060-02", 1), carrier: "SF"},

		// ---------------- U020 唐豆：表达含糊（6 单） ----------------
		{kind: "transit", cust: 20, h: 48, lines: ls("SKU-P010-02", 1), carrier: "SF"},
		{kind: "delivered", cust: 20, h: 120, lines: ls("SKU-P007-02", 1), invoice: "electronic"},
		{kind: "transit", cust: 20, h: 60, lines: ls("SKU-P003-02", 1), carrier: "YTO"},
		{kind: "paid", cust: 20, h: 4, lines: ls("SKU-P063-01", 1)},
		{kind: "unpaid", cust: 20, h: 0.3, lines: ls("SKU-P009-02", 1)},
		{kind: "fullReturn", cust: 20, h: 168, lines: ls("SKU-P012-01", 1)},
	}
}
