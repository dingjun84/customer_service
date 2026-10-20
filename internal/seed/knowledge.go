package seed

import "time"

// Promotions are executable rules, not prose: every threshold, discount, scope
// and stacking decision is in rules_json so the assistant can compute instead of
// guessing. Dates are derived from the dataset anchor.

type promotionDef struct {
	Code      string
	Title     string
	Desc      string
	Type      string
	Status    string
	StartDays float64
	EndDays   float64
	Rules     map[string]any
}

func promotionDefs() []promotionDef {
	return []promotionDef{
		{Code: "PROMO-FULL299-30", Title: "满 299 减 30", Desc: "全场满 299 元减 30 元，与品类券可叠加，单笔最高减 30 元。",
			Type: "threshold_discount", Status: "active", StartDays: -20, EndDays: 25,
			Rules: map[string]any{
				"threshold_minor": 29900, "discount_minor": 3000, "scope": "全场",
				"stackable_with_coupon": true, "max_discount_minor": 3000, "exclude_categories": []any{"生鲜冷链"},
			}},
		{Code: "PROMO-2ITEMS-95", Title: "两件 95 折", Desc: "同款商品购买 2 件及以上享 95 折，不与满减叠加。",
			Type: "quantity_discount", Status: "active", StartDays: -18, EndDays: 20,
			Rules: map[string]any{
				"min_quantity": 2, "discount_percent": 95, "scope": "全场",
				"stackable_with_threshold": false, "max_discount_minor": 20000,
			}},
		{Code: "PROMO-GIFT-BAG", Title: "店铺满赠手提袋", Desc: "满 199 元赠礼盒手提袋一个，赠品不单独计价、不参与退款。",
			Type: "gift_threshold", Status: "active", StartDays: -30, EndDays: 30,
			Rules: map[string]any{
				"threshold_minor": 19900, "gift": "礼盒手提袋", "gift_value_minor": 0, "refundable": false,
			}},
		{Code: "PROMO-NEW-CUSTOMER", Title: "新客券满 99 减 20", Desc: "新注册用户首单可用，有效期 8 天，不可与其他券叠加。",
			Type: "coupon", Status: "active", StartDays: -60, EndDays: 30,
			Rules: map[string]any{
				"threshold_minor": 9900, "discount_minor": 2000, "new_customer_only": true,
				"stackable": false, "max_uses_per_customer": 1,
			}},
		{Code: "PROMO-EXPIRED-618", Title: "618 全场满 199 减 50", Desc: "618 大促券，已于活动结束后失效。",
			Type: "coupon", Status: "expired", StartDays: -140, EndDays: -120,
			Rules: map[string]any{
				"threshold_minor": 19900, "discount_minor": 5000, "stackable": false, "expired_reason": "活动已结束",
			}},
		{Code: "PROMO-CAT-FRESH", Title: "生鲜品类满 99 减 15", Desc: "仅限生鲜冷链品类，与满减可叠加，不适用于礼盒装。",
			Type: "category_coupon", Status: "active", StartDays: -10, EndDays: 15,
			Rules: map[string]any{
				"threshold_minor": 9900, "discount_minor": 1500, "category": "生鲜冷链",
				"stackable_with_threshold": true, "exclude_skus": []any{"SKU-P031-01"},
			}},
		{Code: "PROMO-NO-STACK", Title: "专享券（不可叠加）", Desc: "满 199 减 20 专享券，与其他任何优惠互斥。",
			Type: "coupon", Status: "active", StartDays: -9, EndDays: 21,
			Rules: map[string]any{
				"threshold_minor": 19900, "discount_minor": 2000, "stackable": false,
				"conflicts_with": []any{"PROMO-FULL299-30", "PROMO-CAT-FRESH"},
			}},
		{Code: "PROMO-REFUND-RETURN", Title: "退款返券规则", Desc: "退货成功后退回所用优惠券；券已过期则不返还，返还失败进入人工处理。",
			Type: "refund_policy", Status: "active", StartDays: -90, EndDays: 60,
			Rules: map[string]any{
				"return_coupon_on_refund": true, "skip_if_expired": true,
				"fallback": "人工补发", "max_return_days": 30,
			}},
		{Code: "PROMO-POINTS-DEDUCT", Title: "积分抵扣", Desc: "100 积分抵 1 元，单笔最多抵订单金额 10%。",
			Type: "points_deduction", Status: "active", StartDays: -120, EndDays: 90,
			Rules: map[string]any{
				"points_per_yuan": 100, "max_deduct_percent": 10, "min_order_minor": 1000,
			}},
		{Code: "PROMO-PRICEBACK-15D", Title: "价保 15 天", Desc: "支付后 15 天内同款降价可申请差价，按活动差额计算。",
			Type: "price_protection", Status: "active", StartDays: -30, EndDays: 30,
			Rules: map[string]any{
				"window_days": 15, "price_protection_diff_minor": 5000,
				"sku_codes":            []any{"SKU-P001-01", "SKU-P005-01", "SKU-P006-02", "SKU-P060-01", "SKU-P039-01"},
				"max_claims_per_order": 1,
			}},
		{Code: "PROMO-PRICEBACK-EXPIRED", Title: "价保 7 天（旧规则）", Desc: "旧版价保规则，窗口 7 天，已于上月停止执行。",
			Type: "price_protection", Status: "expired", StartDays: -80, EndDays: -40,
			Rules: map[string]any{
				"window_days": 7, "price_protection_diff_minor": 3000,
				"sku_codes": []any{"SKU-P001-01"}, "max_claims_per_order": 1,
			}},
		{Code: "PROMO-PRESALE-DEPOSIT", Title: "预售定金 100 抵 300", Desc: "预售商品支付 100 元定金抵 300 元尾款，尾款 7 日内支付，逾期取消。",
			Type: "presale_deposit", Status: "active", StartDays: -12, EndDays: 12,
			Rules: map[string]any{
				"deposit_minor": 10000, "offset_minor": 30000,
				"balance_due_days": 7, "sku_codes": []any{"SKU-P002-01", "SKU-P015-01"},
			}},
		{Code: "PROMO-OLD-CAMPAIGN", Title: "开学季满 500 减 60", Desc: "开学季活动，已于 9 月结束，用户常引用旧活动要求补差。",
			Type: "threshold_discount", Status: "expired", StartDays: -75, EndDays: -45,
			Rules: map[string]any{
				"threshold_minor": 50000, "discount_minor": 6000, "scope": "3C数码",
				"stackable_with_coupon": false, "expired_reason": "活动已结束",
			}},
	}
}

// articleDef is one knowledge article. Old versions stay archived so the
// assistant must prefer the currently effective rule.
type articleDef struct {
	Code     string
	Category string
	Title    string
	Content  string
	Keywords []string
	Status   string
	Version  int
}

func articleDefs() []articleDef {
	return []articleDef{
		// ---- 物流配送（6）----
		{Code: "KB-LOG-01", Category: "logistics", Title: "配送时效说明", Version: 3, Status: "published",
			Content: "标准配送：付款后 48 小时内发货，运输 2-4 天送达；加急配送：次日达（限测试市城区）。" +
				"预售商品按商品页标注的发货时间发货。所有时效均为预计，节假日顺延，实际以承运商节点为准。",
			Keywords: []string{"几天到", "时效", "什么时候发货", "预计送达"}},
		{Code: "KB-LOG-02", Category: "logistics", Title: "节假日与雨雪天气配送安排", Version: 1, Status: "published",
			Content: "法定节假日期间承运商揽收与派送会顺延 1-3 天；雨雪、台风等极端天气可能导致区域暂停收派。" +
				"遇到天气原因造成的延迟不计入超时赔付，可在轨迹中看到「天气原因延迟」节点。",
			Keywords: []string{"节假日", "春节", "台风", "延迟", "天气"}},
		{Code: "KB-LOG-03", Category: "logistics", Title: "偏远地区配送范围", Version: 2, Status: "published",
			Content: "偏远地区（含部分乡镇、海岛）仅支持标准配送，时效为 5-8 天，不承诺次日达。" +
				"生鲜冷链商品不支持偏远地区配送，下单时会被系统拦截。",
			Keywords: []string{"偏远", "乡镇", "不发货", "配送范围"}},
		{Code: "KB-LOG-04", Category: "logistics", Title: "驿站代收与签收方式", Version: 2, Status: "published",
			Content: "默认允许放驿站/快递柜，签收人可能显示为「驿站」「门卫」「前台」。" +
				"如需本人签收，请在订单备注中注明，或在派送前联系承运商修改。" +
				"显示已签收但未收到时，先核对驿站/快递柜，24 小时内可提交「未收到货」核查。",
			Keywords: []string{"驿站", "快递柜", "代收", "签收人", "没收到"}},
		{Code: "KB-LOG-05", Category: "logistics", Title: "签收争议处理流程", Version: 1, Status: "published",
			Content: "物流显示签收但用户未收到：1）核对驿站、快递柜、门卫；2）提交「未收到货」核查，承运商 24 小时内回访；" +
				"3）核实丢件后按订单实付全额退款或补发，无需用户承担费用。核查期间订单状态不变更。",
			Keywords: []string{"显示签收", "未收到", "丢件", "核查"}},
		{Code: "KB-LOG-06", Category: "logistics", Title: "改派、拦截与二次派送", Version: 2, Status: "published",
			Content: "派送前可改派地址或改约时间；已揽收后改地址需承运商同意，可能产生改派费。" +
				"拦截仅在「已发货未签收」时受理，拦截成功后退款原路退回；拦截失败会继续派送，需拒收后走退货。" +
				"派送失败（联系不上/地址不详）会安排二次派送，二次失败后退回仓库。",
			Keywords: []string{"改地址", "拦截", "二次派送", "拒收", "改派"}},

		// ---- 订单修改/取消/预售（4）----
		{Code: "KB-ORD-01", Category: "order", Title: "订单取消规则", Version: 3, Status: "published",
			Content: "未发货订单（待付款、已付款待发货）可直接取消，已付款金额 1-3 个工作日原路退回。" +
				"已发货订单无法直接取消，需走退货退款或物流拦截。已取消订单不可恢复，需要重新下单。",
			Keywords: []string{"取消订单", "不想要了", "未发货", "已发货"}},
		{Code: "KB-ORD-02", Category: "order", Title: "修改收件信息规则", Version: 2, Status: "published",
			Content: "未发货前可修改收件人、手机号、省市区、详细地址、配送方式、预约时间和备注。" +
				"已发货后收件信息不可修改，需联系承运商改派。每次修改都会生成待确认操作，需用户点击确认后生效。",
			Keywords: []string{"改地址", "改电话", "修改收件人", "改备注"}},
		{Code: "KB-ORD-03", Category: "order", Title: "预售定金与尾款", Version: 2, Status: "published",
			Content: "预售商品先付定金（100 元），定金抵扣 300 元尾款，尾款需在 7 天内支付。" +
				"逾期未付尾款订单自动取消、定金不退；尾款支付后按预售发货时间发出。" +
				"预售订单在尾款支付前不可发货，也不能申请售后。",
			Keywords: []string{"预售", "定金", "尾款", "抵扣"}},
		{Code: "KB-ORD-04", Category: "order", Title: "部分商品取消", Version: 1, Status: "published",
			Content: "多商品订单未发货时支持整单取消；暂不支持单独取消其中一件。" +
				"已发货订单可对单件商品申请退货退款，优惠按商品行实付金额分摊退还。",
			Keywords: []string{"部分取消", "只退一件", "拆单"}},

		// ---- 退换货（6 + 1 旧版）----
		{Code: "KB-RET-01", Category: "after_sales", Title: "7 天无理由退货规则", Version: 4, Status: "published",
			Content: "自签收之日起 7 天内，商品未使用、吊牌完整、不影响二次销售的，可申请无理由退货。" +
				"定制商品、鲜活易腐商品、拆封的食品与个护用品、贴身衣物不支持无理由退货。" +
				"退货运费由买家承担，有运费险的订单可获最高 15 元补贴。超出 7 天不再受理无理由退货。",
			Keywords: []string{"退货", "7天", "无理由", "期限", "吊牌"}},
		{Code: "KB-RET-01-V1", Category: "after_sales", Title: "7 天无理由退货规则（2025 旧版）", Version: 1, Status: "archived",
			Content: "旧版规则：自签收之日起 15 天内可申请无理由退货，无需说明理由，退货运费由买家承担。" +
				"本版本已于 2025-12-31 失效，请勿再引用。",
			Keywords: []string{"15天", "无理由", "旧版"}},
		{Code: "KB-RET-02", Category: "after_sales", Title: "拆封与试用规则", Version: 2, Status: "published",
			Content: "服装可试穿，但吊牌剪掉、洗涤、有异味或污渍视为影响二次销售，不支持无理由退货。" +
				"3C 商品拆封后仍支持 7 天无理由，但激活（登录账号/插卡联网）后仅支持质量问题退换。" +
				"食品、个护、母婴类一经开封不支持无理由退货。",
			Keywords: []string{"拆封", "试用", "吊牌", "激活", "影响二次销售"}},
		{Code: "KB-RET-03", Category: "after_sales", Title: "定制商品退换规则", Version: 2, Status: "published",
			Content: "刻字、定制图案、定制尺寸的商品（如刻字保温杯、定制相册、定制马克杯）下单后即进入生产，" +
				"不支持 7 天无理由退货。若存在质量问题或与定制内容不符，可免费重做或全额退款。",
			Keywords: []string{"定制", "刻字", "不退", "重做"}},
		{Code: "KB-RET-04", Category: "after_sales", Title: "生鲜商品售后规则", Version: 3, Status: "published",
			Content: "生鲜商品签收后 24 小时内可对坏果、变质、缺重发起售后，需提供商品与面单同框照片。" +
				"坏果按比例退款，缺重按缺少重量折算退款，整单变质可全额退款。超过 24 小时不再受理，除非有冷链断链证据。",
			Keywords: []string{"生鲜", "坏果", "烂了", "缺重", "24小时"}},
		{Code: "KB-RET-05", Category: "after_sales", Title: "贴身衣物退换限制", Version: 1, Status: "published",
			Content: "内衣、保暖内衣、袜子等贴身衣物，出于卫生考虑，非质量问题不支持退货。" +
				"质量问题（破洞、掉色、开线）可在签收后 15 天内换货。",
			Keywords: []string{"内衣", "贴身", "卫生", "不退"}},
		{Code: "KB-RET-06", Category: "after_sales", Title: "换货与维修规则", Version: 3, Status: "published",
			Content: "签收后 15 天内出现非人为质量问题可申请换货或维修；超过 15 天且在保修期内仅支持维修。" +
				"换货需配件齐全，检测确认非人为损坏后由仓库寄出新品，换货包裹与退件包裹分别独立。" +
				"人为损坏、进液、私自拆机不在保修范围内。",
			Keywords: []string{"换货", "维修", "质量问题", "保修", "人为损坏"}},

		// ---- 退款（4）----
		{Code: "KB-REF-01", Category: "refund", Title: "退款到账时间", Version: 3, Status: "published",
			Content: "退款审核通过后按原支付渠道退回：微信支付/支付宝 1-3 个工作日，银行卡 3-7 个工作日，余额实时到账。" +
				"退款单状态为 requested、pending_review、approved、processing 时均表示尚未到账，只有 succeeded 才是已到账。",
			Keywords: []string{"退款", "到账", "多久", "还没收到"}},
		{Code: "KB-REF-02", Category: "refund", Title: "原路退回说明", Version: 1, Status: "published",
			Content: "退款只能退回原支付账户，不支持改退到其他账户或他人账户。" +
				"原支付方式已注销（如银行卡销户）时，退款会失败并转人工处理，需要用户提供新的收款方式并完成身份核验。",
			Keywords: []string{"原路退回", "换账户", "退款失败", "销户"}},
		{Code: "KB-REF-03", Category: "refund", Title: "部分退款与优惠分摊", Version: 2, Status: "published",
			Content: "部分退货按商品行实付金额退款：行实付 = 行成交价×数量 - 该行分摊的优惠。" +
				"订单级满减按各行金额占比分摊，除不尽的分位计入金额最大的行。" +
				"运费在整单退货时全额退还，部分退货不退还运费。赠品不参与退款计算。",
			Keywords: []string{"部分退款", "退多少", "满减", "分摊", "运费"}},
		{Code: "KB-REF-04", Category: "refund", Title: "组合支付退款", Version: 2, Status: "published",
			Content: "组合支付（如微信 + 余额、支付宝 + 白条）按各渠道实际支付金额分别退回，" +
				"余额部分实时到账，第三方渠道 1-3 个工作日。优惠券按规则返还，已过期券不返还。" +
				"退款记录会分别关联各支付渠道，可在退款详情查看拆分金额。",
			Keywords: []string{"组合支付", "拆分退款", "余额", "白条"}},

		// ---- 发票（3）----
		{Code: "KB-INV-01", Category: "invoice", Title: "发票类型与申请条件", Version: 3, Status: "published",
			Content: "订单支付后可申请电子普通发票（默认）、纸质普通发票、增值税专用发票。" +
				"专票需提供公司名称、税号、开户行与账号；电子票需提供接收邮箱。发票金额为订单实付金额。",
			Keywords: []string{"发票", "开票", "专票", "电子票", "税号"}},
		{Code: "KB-INV-02", Category: "invoice", Title: "发票抬头修改与红冲", Version: 2, Status: "published",
			Content: "电子发票开具后 30 天内可申请修改抬头或税号，需先红冲原发票再重新开具。" +
				"发生退款时，已开具的发票必须先红冲；部分退款按退款金额红冲后重开差额发票。" +
				"纸质发票红冲需寄回原件。",
			Keywords: []string{"改抬头", "税号错误", "红冲", "重开"}},
		{Code: "KB-INV-03", Category: "invoice", Title: "补开发票与开票失败", Version: 1, Status: "published",
			Content: "订单完成后 30 天内可补开发票，超期需人工申请。开票失败常见原因：税号格式错误、邮箱无效、抬头含特殊字符。" +
				"失败后可修正信息重新提交，同一订单最多重试 3 次，超过需转人工。",
			Keywords: []string{"补开发票", "开票失败", "税号格式", "邮箱"}},

		// ---- 优惠（4 + 1）----
		{Code: "KB-PRO-01", Category: "promotion", Title: "优惠券使用规则", Version: 3, Status: "published",
			Content: "优惠券有门槛、品类、有效期限制，下单时系统自动选择最优券，用户也可手动切换。" +
				"一单只能用一张平台券；标注「不可叠加」的券与其他任何优惠互斥。过期券不可使用、不可延期。",
			Keywords: []string{"优惠券", "用不了", "门槛", "过期", "叠加"}},
		{Code: "KB-PRO-02", Category: "promotion", Title: "满减与叠加规则", Version: 2, Status: "published",
			Content: "满 299 减 30 为全场活动，生鲜冷链不参与；两件 95 折与满减互斥，系统按优惠金额更高的方案计算。" +
				"店铺满赠与其他活动可同时享受，赠品不折现、不参与退款。",
			Keywords: []string{"满减", "叠加", "两件折扣", "满赠"}},
		{Code: "KB-PRO-03", Category: "promotion", Title: "积分抵扣规则", Version: 1, Status: "published",
			Content: "100 积分抵 1 元，单笔最多抵扣订单金额的 10%，不足 1 元部分不抵扣。" +
				"退款时积分按抵扣金额等比例退回，已过期积分不补。",
			Keywords: []string{"积分", "抵扣", "京豆"}},
		{Code: "KB-PRO-04", Category: "promotion", Title: "价保规则（15 天）", Version: 4, Status: "published",
			Content: "自支付之日起 15 天内，同一 SKU 在同一活动下出现降价，可申请差价补偿，每单限一次。" +
				"差价按活动规则计算，不含优惠券、积分抵扣与运费；秒杀、清仓、临期特价不参与价保。" +
				"超出 15 天、订单未支付、或商品本身不支持价保的，不予受理。",
			Keywords: []string{"价保", "降价", "补差价", "15天"}},
		{Code: "KB-PRO-05", Category: "promotion", Title: "活动结束后的补差诉求", Version: 1, Status: "published",
			Content: "活动结束后不再补差。用户引用已结束活动（如开学季、618）要求按旧价补差的，一律不成立；" +
				"若订单在价保窗口内且当前有生效活动，可走价保流程。",
			Keywords: []string{"活动结束", "补差", "旧活动", "不补"}},

		// ---- 商品使用/保修（3）----
		{Code: "KB-PRD-01", Category: "product", Title: "保修政策说明", Version: 2, Status: "published",
			Content: "3C 数码整机保修 12-24 个月（以商品页为准），大家电 36 个月，配件 12 个月。" +
				"保修期内非人为故障免费维修；人为损坏、进液、私自拆机不保修，可付费维修。" +
				"维修需提供订单号，无需发票。",
			Keywords: []string{"保修", "保修期", "维修", "免费"}},
		{Code: "KB-PRD-02", Category: "product", Title: "安全故障应急处理", Version: 1, Status: "published",
			Content: "出现漏电、冒烟、焦味、异常发烫时：立即断电并停止使用，不要自行拆机、不要继续充电，" +
				"将商品移至远离易燃物处。随后联系客服走安全故障专线，凭订单号优先换新或全额退款，" +
				"涉及人身或财产损失的转人工专员处理。",
			Keywords: []string{"漏电", "冒烟", "发烫", "安全问题", "起火"}},
		{Code: "KB-PRD-03", Category: "product", Title: "配件与耗材兼容", Version: 2, Status: "published",
			Content: "滤芯、刷头、硒鼓等耗材按型号匹配：净化器 F3、电动牙刷 T2、打印机 P1 硒鼓。" +
				"使用非原装耗材造成的故障不在保修范围内。配件缺失可在签收后 48 小时内申请补发，无需退回整机。",
			Keywords: []string{"配件", "耗材", "滤芯", "刷头", "兼容"}},
	}
}

// knowledgeTime derives article timestamps: archived versions are older.
func knowledgeTime(anchor time.Time, def articleDef) (time.Time, time.Time) {
	created := anchor.AddDate(0, 0, -240)
	updated := anchor.AddDate(0, 0, -10)
	if def.Status == "archived" {
		created = anchor.AddDate(0, 0, -400)
		updated = anchor.AddDate(0, 0, -300)
	}
	return created, updated
}
