package seed

import "fmt"

// Twenty fictional customers. Every identifier is obviously synthetic:
// phone numbers sit in the reserved-looking 138000000xx block, e-mail addresses
// use example.test and every address is inside 测试省/测试市 with 模拟路 and
// 测试小区 street names. No real person can be reconstructed from these rows.
//
// Customer numbers are CUST-1001..CUST-1020 (规范中的 U001..U020 一一对应).

type addressDef struct {
	Label    string
	Receiver string
	Province string
	City     string
	District string
	Street   string
	Postal   string
	Method   string
}

type customerDef struct {
	No      string
	Name    string
	Phone   string
	Profile string
	Addrs   []addressDef
}

func baseAddr(label, receiver, district, street string) addressDef {
	return addressDef{
		Label: label, Receiver: receiver, Province: "测试省", City: "测试市",
		District: district, Street: street, Postal: "000000", Method: "standard",
	}
}

func customerDefs() []customerDef {
	return []customerDef{
		{No: "CUST-1001", Name: "张三", Phone: "13800000001", Profile: "3C 高频买家",
			Addrs: []addressDef{
				baseAddr("家", "张三", "测试一区", "模拟路 101 号 测试花园 3 栋 1201"),
				baseAddr("公司", "张三", "测试二区", "模拟路 202 号 测试大厦 A 座 902"),
			}},
		{No: "CUST-1002", Name: "李四", Phone: "13800000002", Profile: "服装买家",
			Addrs: []addressDef{
				baseAddr("家", "李四", "测试三区", "模拟路 303 号 测试花园 7 栋 501"),
			}},
		{No: "CUST-1003", Name: "王沐", Phone: "13800000003", Profile: "生鲜高频买家",
			Addrs: []addressDef{
				baseAddr("家", "王沐", "测试一区", "模拟路 118 号 测试花园 2 栋 803"),
			}},
		{No: "CUST-1004", Name: "赵岚", Phone: "13800000004", Profile: "礼品定制买家",
			Addrs: []addressDef{
				baseAddr("家", "赵岚", "测试四区", "模拟路 406 号 测试花园 1 栋 1502"),
				baseAddr("收礼人", "赵岚（代收）", "测试五区", "模拟路 509 号 测试花园 9 栋 302"),
			}},
		{No: "CUST-1005", Name: "陈稳", Phone: "13800000005", Profile: "家电买家",
			Addrs: []addressDef{
				baseAddr("家", "陈稳", "测试二区", "模拟路 210 号 测试花园 5 栋 601"),
			}},
		{No: "CUST-1006", Name: "孙可", Phone: "13800000006", Profile: "优惠券高频用户",
			Addrs: []addressDef{
				baseAddr("家", "孙可", "测试三区", "模拟路 315 号 测试花园 6 栋 1104"),
			}},
		{No: "CUST-1007", Name: "周棠", Phone: "13800000007", Profile: "多次退款",
			Addrs: []addressDef{
				baseAddr("家", "周棠", "测试一区", "模拟路 133 号 测试花园 8 栋 202"),
			}},
		{No: "CUST-1008", Name: "吴笛", Phone: "13800000008", Profile: "多地址/改地址",
			Addrs: []addressDef{
				baseAddr("家", "吴笛", "测试四区", "模拟路 421 号 测试花园 4 栋 1001"),
				baseAddr("父母家", "吴建国", "测试五区", "模拟路 517 号 测试花园 2 栋 601"),
				baseAddr("公司", "吴笛", "测试二区", "模拟路 222 号 测试大厦 C 座 1803"),
			}},
		{No: "CUST-1009", Name: "郑川", Phone: "13800000009", Profile: "多单同时在途",
			Addrs: []addressDef{
				baseAddr("家", "郑川", "测试三区", "模拟路 330 号 测试花园 3 栋 704"),
			}},
		{No: "CUST-1010", Name: "冯楠", Phone: "13800000010", Profile: "投诉并转人工",
			Addrs: []addressDef{
				baseAddr("家", "冯楠", "测试一区", "模拟路 145 号 测试花园 5 栋 402"),
			}},
		{No: "CUST-1011", Name: "何斐", Phone: "13800000011", Profile: "跨境/保税",
			Addrs: []addressDef{
				baseAddr("家", "何斐", "测试四区", "模拟路 438 号 测试花园 6 栋 1702"),
			}},
		{No: "CUST-1012", Name: "许澜", Phone: "13800000012", Profile: "货到付款",
			Addrs: []addressDef{
				baseAddr("家", "许澜", "测试五区", "模拟路 526 号 测试花园 1 栋 902"),
			}},
		{No: "CUST-1013", Name: "邓一", Phone: "13800000013", Profile: "组合支付",
			Addrs: []addressDef{
				baseAddr("家", "邓一", "测试二区", "模拟路 234 号 测试花园 2 栋 503"),
			}},
		{No: "CUST-1014", Name: "苏禾", Phone: "13800000014", Profile: "预售定金尾款",
			Addrs: []addressDef{
				baseAddr("家", "苏禾", "测试三区", "模拟路 342 号 测试花园 4 栋 1203"),
			}},
		{No: "CUST-1015", Name: "林步", Phone: "13800000015", Profile: "套装与赠品",
			Addrs: []addressDef{
				baseAddr("家", "林步", "测试一区", "模拟路 157 号 测试花园 7 栋 1005"),
			}},
		{No: "CUST-1016", Name: "高远", Phone: "13800000016", Profile: "故障维修",
			Addrs: []addressDef{
				baseAddr("家", "高远", "测试四区", "模拟路 449 号 测试花园 3 栋 601"),
			}},
		{No: "CUST-1017", Name: "罗晴", Phone: "13800000017", Profile: "少件补发",
			Addrs: []addressDef{
				baseAddr("家", "罗晴", "测试五区", "模拟路 534 号 测试花园 8 栋 703"),
			}},
		{No: "CUST-1018", Name: "梁澈", Phone: "13800000018", Profile: "退货被拒收",
			Addrs: []addressDef{
				baseAddr("家", "梁澈", "测试二区", "模拟路 246 号 测试花园 5 栋 1504"),
			}},
		{No: "CUST-1019", Name: "宋知", Phone: "13800000019", Profile: "发票问题",
			Addrs: []addressDef{
				baseAddr("公司", "宋知", "测试三区", "模拟路 355 号 测试大厦 B 座 701"),
			}},
		{No: "CUST-1020", Name: "唐豆", Phone: "13800000020", Profile: "表达含糊/频繁切换",
			Addrs: []addressDef{
				baseAddr("家", "唐豆", "测试一区", "模拟路 168 号 测试花园 9 栋 1101"),
			}},
	}
}

// benefitDef is a seed-time coupon/points/insurance row.
type benefitDef struct {
	Type    string
	Code    string
	Status  string
	Amount  int64
	Points  int64
	Expires timeOffset
	Attrs   map[string]any
}

// timeOffset keeps benefit expiries relative to the dataset anchor.
type timeOffset struct {
	Days int
	Set  bool
}

func coupon(code string, amount int64, status string, days int, attrs map[string]any) benefitDef {
	return benefitDef{Type: "coupon", Code: code, Status: status, Amount: amount,
		Expires: timeOffset{Days: days, Set: true}, Attrs: attrs}
}

func couponAttrs(threshold int64, scope, note string, stackable bool) map[string]any {
	return map[string]any{
		"threshold_minor": threshold,
		"scope":           scope,
		"stackable":       stackable,
		"说明":              note,
	}
}

// benefitDefs returns the per-customer benefit set (20 sets, ~45 rows).
func benefitDefs(index int, customerNo string) []benefitDef {
	points := int64(320 + index*137)
	out := []benefitDef{
		{Type: "points", Code: fmt.Sprintf("POINTS-%s", customerNo), Status: "available", Points: points,
			Attrs: map[string]any{"说明": fmt.Sprintf("%d 积分，100 积分抵 1 元", points)}},
		{Type: "freight_insurance", Code: "FI-2026", Status: "available",
			Attrs: map[string]any{"保障": "退货运费补贴，单笔最高 15 元"}},
	}
	switch index {
	case 0: // 3C 买家：白条 + 大额券
		out = append(out,
			benefitDef{Type: "whitebar_status", Code: "WHITEBAR", Status: "available",
				Attrs: map[string]any{"额度": "3000 元（模拟）", "说明": "演示数据，不代表真实授信"}},
			coupon("CPN-3C-50", 5000, "available", 20, couponAttrs(299900, "3C数码", "满 2999 减 50，限 3C 品类", false)))
	case 5: // 券用户：一涨一过一临期
		out = append(out,
			coupon("CPN-ALL-30", 3000, "available", 12, couponAttrs(29900, "全场", "满 299 减 30，可与满减叠加", true)),
			coupon("CPN-EXPIRED-618", 5000, "expired", -35, couponAttrs(19900, "全场", "618 活动券，已过期", false)),
			coupon("CPN-FRESH-15", 1500, "available", 3, couponAttrs(9900, "生鲜冷链", "满 99 减 15，限生鲜品类", true)))
	case 6: // 退款用户：券已用于订单，退款后返券失败
		out = append(out,
			coupon("CPN-USED-7007", 2000, "used", 30, couponAttrs(9900, "全场", "已用于订单 ORD 系列，退款不返还", false)),
			coupon("CPN-RETURN-FAILED", 2000, "processing", 30,
				map[string]any{"说明": "退款成功但券返还失败，需人工补发", "related_refund": "按订单检索"}))
	case 8: // 多包裹：无券
		out = append(out, coupon("CPN-NEW-20", 2000, "available", 8, couponAttrs(9900, "全场", "新客券，仅首单可用", false)))
	case 11: // 货到付款：余额
		out = append(out, benefitDef{Type: "store_balance", Code: "BALANCE", Status: "available", Amount: 12800,
			Attrs: map[string]any{"说明": "模拟账户余额 128 元，可用于抵扣"}})
	case 12: // 组合支付：余额 + 券
		out = append(out,
			benefitDef{Type: "store_balance", Code: "BALANCE", Status: "available", Amount: 9900,
				Attrs: map[string]any{"说明": "模拟账户余额 99 元"}},
			coupon("CPN-STACK-NO", 1000, "disabled", 15,
				map[string]any{"说明": "与其他券不可叠加，已停用", "stackable": false}))
	}
	return out
}

func i64PtrValue(v int64) *int64 { return &v }
