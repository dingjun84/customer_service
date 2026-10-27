package seed

// Catalog definitions for the synthetic store. Every brand, model number and
// price here is fictional: source_type is always "synthetic" and price_type is
// "synthetic_test_price" (see attrs), so nothing can be mistaken for a real
// quote or a real product page.
//
// Category counts: 3C 18 / 服装鞋包 12 / 礼品定制 10 / 生鲜冷链 12 /
// 家居小家电 10 / 食品个护 11 = 75 products, 129 SKUs（含 IM 场景补齐的缺货/尺码 SKU）。

const (
	policyStandard      = "standard_7d"   // 7 天无理由
	policyUnopenedOnly  = "unopened_only" // 拆封/开封后不支持无理由
	policyCustomized    = "customized_no_return"
	policyFreshClaim    = "fresh_24h_claim"    // 生鲜 24 小时坏果/变质申诉
	policyIntimate      = "intimate_no_return" // 贴身衣物
	policyLargeItem     = "large_item_special" // 大件：预约上门、需验货
	policyDigitalActive = "digital_activation" // 激活后不支持无理由
)

type skuDef struct {
	suffix string
	name   string
	specs  map[string]any
	price  int64
	stock  int64
}

type productDef struct {
	code     string
	name     string
	brand    string
	category string
	desc     string
	attrs    map[string]any
	skus     []skuDef
}

// specs turns alternating key/value pairs into a JSON object.
func specs(kv ...any) map[string]any {
	out := make(map[string]any, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		if key == "" {
			continue
		}
		out[key] = kv[i+1]
	}
	return out
}

// pattrs builds product attributes: sourcing marks, warranty, return policy and
// any extra public specs. Policy flags are derived, never invented per order.
func pattrs(warrantyMonths int, policy string, kv ...any) map[string]any {
	out := map[string]any{
		"source_type":                "synthetic",
		"is_synthetic":               true,
		"price_type":                 "synthetic_test_price",
		"price_checked_at":           "2026-10-20T04:00:00Z",
		"source_url":                 "",
		"warranty_months":            warrantyMonths,
		"return_policy_code":         policy,
		"is_customized":              policy == policyCustomized,
		"is_perishable":              policy == policyFreshClaim,
		"cold_chain_required":        false,
		"invoice_supported":          true,
		"price_protection_supported": warrantyMonths >= 12 && policy != policyFreshClaim && policy != policyCustomized,
	}
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		if key == "" {
			continue
		}
		out[key] = kv[i+1]
	}
	return out
}

func catalogDefs() []productDef {
	return []productDef{
		// ---------------- 3C 数码（18） ----------------
		{code: "P001", name: "星野 X7 5G 智能手机", brand: "星野", category: "3C数码",
			desc:  "6.7 英寸 OLED 直屏，5000mAh 电池，支持 66W 快充。演示机型，非真实在售型号。",
			attrs: pattrs(24, policyDigitalActive, "屏幕", "6.7 英寸 OLED", "电池", "5000mAh", "充电", "66W", "网络", "5G 双卡"),
			skus: []skuDef{
				{"-01", "128GB 曜石黑", specs("存储", "128GB", "颜色", "曜石黑"), 399900, 26},
				{"-02", "256GB 曜石黑", specs("存储", "256GB", "颜色", "曜石黑"), 449900, 18},
				{"-03", "256GB 星霜白", specs("存储", "256GB", "颜色", "星霜白"), 449900, 4},
			}},
		{code: "P002", name: "远山 Mate 60 商务手机", brand: "远山", category: "3C数码",
			desc:  "6.8 英寸曲面屏，卫星消息演示能力，主打商务长续航。演示机型。",
			attrs: pattrs(24, policyDigitalActive, "屏幕", "6.8 英寸曲面", "电池", "5100mAh", "网络", "5G"),
			skus: []skuDef{
				{"-01", "12GB+256GB 雅川青", specs("内存", "12GB", "存储", "256GB", "颜色", "雅川青"), 549900, 12},
				{"-02", "16GB+512GB 砚黑", specs("内存", "16GB", "存储", "512GB", "颜色", "砚黑"), 649900, 6},
			}},
		{code: "P003", name: "红枫 Note 14 智能手机", brand: "红枫", category: "3C数码",
			desc:  "入门 5G 机型，5000mAh 大电池，适合备用机。演示机型。",
			attrs: pattrs(12, policyDigitalActive, "屏幕", "6.5 英寸 LCD", "电池", "5000mAh", "网络", "5G"),
			skus: []skuDef{
				{"-01", "8GB+128GB 星夜黑", specs("内存", "8GB", "存储", "128GB", "颜色", "星夜黑"), 159900, 40},
				{"-02", "8GB+256GB 晴山蓝", specs("内存", "8GB", "存储", "256GB", "颜色", "晴山蓝"), 179900, 22},
			}},
		{code: "P004", name: "青柏 ThinkBook 14 轻薄本", brand: "青柏", category: "3C数码",
			desc:  "14 英寸 2.2K 屏，16GB 内存，适合办公与网课。演示机型。",
			attrs: pattrs(24, policyStandard, "屏幕", "14 英寸 2.2K", "内存", "16GB", "硬盘", "512GB SSD", "重量", "1.4kg"),
			skus: []skuDef{
				{"-01", "i5/16GB/512GB", specs("处理器", "i5", "内存", "16GB", "硬盘", "512GB SSD"), 499900, 9},
				{"-02", "i7/16GB/1TB", specs("处理器", "i7", "内存", "16GB", "硬盘", "1TB SSD"), 629900, 5},
			}},
		{code: "P005", name: "星野 Pad Air 11 平板电脑", brand: "星野", category: "3C数码",
			desc:  "11 英寸 2K 屏，支持手写笔，分 Wi-Fi 与蜂窝版本。演示机型。",
			attrs: pattrs(12, policyStandard, "屏幕", "11 英寸 2K", "重量", "480g", "配件", "手写笔需另购"),
			skus: []skuDef{
				{"-01", "128GB Wi-Fi 版", specs("容量", "128GB", "网络", "Wi-Fi"), 259900, 15},
				{"-02", "256GB 蜂窝版", specs("容量", "256GB", "网络", "蜂窝"), 349900, 7},
			}},
		{code: "P006", name: "声悦 Air 真无线降噪耳机", brand: "声悦", category: "3C数码",
			desc:  "半入耳设计，主动降噪，充电盒总续航 30 小时。演示机型。",
			attrs: pattrs(12, policyStandard, "降噪", "主动降噪", "续航", "单次 6 小时", "接口", "USB-C", "兼容", "iOS/Android"),
			skus: []skuDef{
				{"-01", "标准版 白", specs("版本", "标准版", "颜色", "白色"), 99900, 30},
				{"-02", "降噪版 白", specs("版本", "降噪版", "颜色", "白色"), 129900, 20},
			}},
		{code: "P007", name: "远山 FreeBuds 蓝牙耳机", brand: "远山", category: "3C数码",
			desc:  "入耳式蓝牙耳机，IP54 防水，通话降噪。演示机型。",
			attrs: pattrs(12, policyStandard, "防水", "IP54", "续航", "单次 5 小时", "蓝牙", "5.3"),
			skus: []skuDef{
				{"-01", "白色", specs("颜色", "白色"), 49900, 60},
				{"-02", "黑色", specs("颜色", "黑色"), 49900, 45},
			}},
		{code: "P008", name: "极充 20000mAh 移动电源", brand: "极充", category: "3C数码",
			desc:  "20000mAh 容量，22.5W 快充，可上飞机（额定能量 74Wh）。演示机型。",
			attrs: pattrs(12, policyStandard, "容量", "20000mAh", "额定能量", "74Wh", "输出", "22.5W", "航空", "可随身携带"),
			skus: []skuDef{
				{"-01", "白色", specs("颜色", "白色"), 19900, 80},
				{"-02", "黑色", specs("颜色", "黑色"), 19900, 55},
			}},
		{code: "P009", name: "极充 65W 氮化镓充电器", brand: "极充", category: "3C数码",
			desc:  "65W 氮化镓，支持 PD/QC，单口与双口两个版本。演示机型。",
			attrs: pattrs(12, policyStandard, "功率", "65W", "协议", "PD3.0/QC4+", "插头", "国标"),
			skus: []skuDef{
				{"-01", "单口 65W", specs("接口", "单 USB-C", "功率", "65W"), 12900, 100},
				{"-02", "双口 65W", specs("接口", "USB-C + USB-A", "功率", "65W"), 15900, 70},
			}},
		{code: "P010", name: "云键 静音无线键鼠套装", brand: "云键", category: "3C数码",
			desc:  "2.4G 无线连接，静音按键，含无线鼠标。演示机型。",
			attrs: pattrs(12, policyStandard, "连接", "2.4G 无线", "布局", "104 键全尺寸", "供电", "7 号电池"),
			skus: []skuDef{
				{"-01", "灰白", specs("颜色", "灰白"), 16900, 45},
				{"-02", "深空灰", specs("颜色", "深空灰"), 16900, 38},
			}},
		{code: "P011", name: "迅存 移动固态硬盘", brand: "迅存", category: "3C数码",
			desc:  "USB 3.2 Gen2 接口，读取 1050MB/s，金属外壳。演示机型。",
			attrs: pattrs(36, policyStandard, "接口", "USB 3.2 Gen2", "读取", "1050MB/s", "兼容", "Windows/macOS"),
			skus: []skuDef{
				{"-01", "1TB", specs("容量", "1TB"), 59900, 25},
				{"-02", "2TB", specs("容量", "2TB"), 99900, 10},
			}},
		{code: "P012", name: "星野手环 8", brand: "星野", category: "3C数码",
			desc:  "1.6 英寸 AMOLED，心率与血氧监测，5ATM 防水。演示机型。",
			attrs: pattrs(12, policyStandard, "屏幕", "1.6 英寸 AMOLED", "防水", "5ATM", "续航", "14 天", "兼容", "Android/iOS"),
			skus: []skuDef{
				{"-01", "标准版 黑", specs("版本", "标准版", "颜色", "黑色"), 24900, 50},
				{"-02", "NFC 版 黑", specs("版本", "NFC 版", "颜色", "黑色"), 32900, 22},
			}},
		{code: "P059", name: "云印 黑白激光打印机 P1", brand: "云印", category: "3C数码",
			desc:  "A4 黑白激光，支持无线打印，鼓粉分离。演示机型。",
			attrs: pattrs(12, policyStandard, "类型", "黑白激光", "接口", "USB/Wi-Fi", "耗材", "P1 硒鼓", "幅面", "A4"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准版"), 89900, 14},
			}},
		{code: "P060", name: "星野 27 英寸显示器", brand: "星野", category: "3C数码",
			desc:  "IPS 面板，2K/4K 两种规格，支持 65W 反向供电。演示机型。",
			attrs: pattrs(36, policyStandard, "面板", "IPS", "接口", "HDMI/DP/USB-C", "支架", "升降旋转"),
			skus: []skuDef{
				{"-01", "27 英寸 2K", specs("尺寸", "27 英寸", "分辨率", "2560×1440"), 129900, 16},
				{"-02", "27 英寸 4K", specs("尺寸", "27 英寸", "分辨率", "3840×2160"), 189900, 8},
			}},
		{code: "P063", name: "极充 车载充电器 45W", brand: "极充", category: "3C数码",
			desc:  "双口车充，45W 总输出，支持 PD+QC。演示机型。",
			attrs: pattrs(12, policyStandard, "功率", "45W", "接口", "USB-C + USB-A", "输入", "12-24V"),
			skus: []skuDef{
				{"-01", "双口 45W", specs("接口", "双口"), 6900, 120},
			}},
		{code: "P064", name: "洁净 车载吸尘器", brand: "洁净", category: "3C数码",
			desc:  "无线手持，12000Pa 吸力，附三种吸头。演示机型。",
			attrs: pattrs(12, policyStandard, "吸力", "12000Pa", "供电", "锂电池", "配件", "三种吸头"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准版"), 24900, 30},
				{"-02", "增强版", specs("版本", "增强版"), 32900, 12},
			}},
		{code: "P071", name: "星河 AX3000 路由器", brand: "星河", category: "3C数码",
			desc:  "Wi-Fi 6，双频 3000M，支持 Mesh 组网。演示机型。",
			attrs: pattrs(24, policyStandard, "无线", "Wi-Fi 6 AX3000", "接口", "千兆 WAN/LAN", "组网", "Mesh"),
			skus: []skuDef{
				{"-01", "单只装", specs("数量", "1 只"), 22900, 40},
				{"-02", "两只装", specs("数量", "2 只"), 39900, 15},
			}},
		{code: "P072", name: "守望 智能摄像头 2K", brand: "守望", category: "3C数码",
			desc:  "2K 分辨率，红外夜视，本地 TF 卡与云存储二选一。演示机型。",
			attrs: pattrs(12, policyStandard, "分辨率", "2K", "夜视", "红外 10m", "存储", "TF 卡/云存储", "联网", "2.4G Wi-Fi"),
			skus: []skuDef{
				{"-01", "2K 固定版", specs("云台", "不支持", "分辨率", "2K"), 19900, 35},
				{"-02", "2K 云台版", specs("云台", "支持", "分辨率", "2K"), 25900, 18},
			}},

		// ---------------- 服装鞋包（12） ----------------
		{code: "P013", name: "素棉 纯棉短袖 T 恤", brand: "素棉", category: "服装鞋包",
			desc:  "100% 精梳棉，克重 190g，圆领基础款。",
			attrs: pattrs(0, policyStandard, "面料", "100% 棉", "克重", "190g", "版型", "常规"),
			skus: []skuDef{
				{"-01", "白色 M", specs("颜色", "白色", "尺码", "M"), 6900, 120},
				{"-02", "白色 L", specs("颜色", "白色", "尺码", "L"), 6900, 90},
				{"-03", "黑色 L", specs("颜色", "黑色", "尺码", "L"), 6900, 60},
				{"-04", "黑色 XL", specs("颜色", "黑色", "尺码", "XL"), 6900, 0},
				{"-05", "白色 S", specs("颜色", "白色", "尺码", "S"), 6900, 80},
			}},
		{code: "P014", name: "风野 轻量运动外套", brand: "风野", category: "服装鞋包",
			desc:  "防风梭织面料，适合春秋通勤与轻运动。",
			attrs: pattrs(0, policyStandard, "面料", "锦纶", "季节", "春秋", "帽型", "连帽"),
			skus: []skuDef{
				{"-01", "藏青 M", specs("颜色", "藏青", "尺码", "M"), 29900, 40},
				{"-02", "藏青 L", specs("颜色", "藏青", "尺码", "L"), 29900, 35},
				{"-03", "卡其 L", specs("颜色", "卡其", "尺码", "L"), 29900, 18},
			}},
		{code: "P015", name: "雪岭 90 白鸭绒羽绒服", brand: "雪岭", category: "服装鞋包",
			desc:  "充绒量 180g，蓬松度 600+，可机洗。",
			attrs: pattrs(0, policyStandard, "充绒量", "180g", "绒子含量", "90%", "蓬松度", "600+", "填充物", "白鸭绒"),
			skus: []skuDef{
				{"-01", "黑色 L", specs("颜色", "黑色", "尺码", "L"), 59900, 25},
				{"-02", "黑色 XL", specs("颜色", "黑色", "尺码", "XL"), 59900, 15},
				{"-03", "米白 M", specs("颜色", "米白", "尺码", "M"), 62900, 8},
				{"-04", "米白 L", specs("颜色", "米白", "尺码", "L"), 62900, 0},
			}},
		{code: "P016", name: "轻驰 城市缓震跑鞋", brand: "轻驰", category: "服装鞋包",
			desc:  "缓震中底，透气网面，日常慢跑适用。",
			attrs: pattrs(0, policyStandard, "鞋楦", "标准", "中底", "EVA 缓震", "适用", "日常慢跑"),
			skus: []skuDef{
				{"-01", "41 码 灰蓝", specs("鞋码", "41", "颜色", "灰蓝"), 39900, 30},
				{"-02", "42 码 灰蓝", specs("鞋码", "42", "颜色", "灰蓝"), 39900, 28},
				{"-03", "42 码 黑", specs("鞋码", "42", "颜色", "黑色"), 39900, 20},
				{"-04", "43 码 黑", specs("鞋码", "43", "颜色", "黑色"), 39900, 9},
			}},
		{code: "P017", name: "行者 20L 通勤双肩包", brand: "行者", category: "服装鞋包",
			desc:  "20L 容量，独立 15.6 英寸电脑仓，防泼水面料。",
			attrs: pattrs(12, policyStandard, "容量", "20L", "材质", "涤纶", "电脑仓", "15.6 英寸"),
			skus: []skuDef{
				{"-01", "黑色", specs("颜色", "黑色"), 19900, 55},
				{"-02", "灰绿", specs("颜色", "灰绿"), 19900, 33},
			}},
		{code: "P018", name: "蓝布 直筒牛仔裤", brand: "蓝布", category: "服装鞋包",
			desc:  "直筒版型，含棉 98%，微弹。",
			attrs: pattrs(0, policyStandard, "面料", "98% 棉 2% 氨纶", "版型", "直筒", "裤长", "常规"),
			skus: []skuDef{
				{"-01", "30 码 深蓝", specs("腰围", "30", "颜色", "深蓝"), 25900, 30},
				{"-02", "32 码 深蓝", specs("腰围", "32", "颜色", "深蓝"), 25900, 26},
				{"-03", "34 码 浅蓝", specs("腰围", "34", "颜色", "浅蓝"), 25900, 12},
			}},
		{code: "P019", name: "眠白 全棉四件套", brand: "眠白", category: "服装鞋包",
			desc:  "60 支长绒棉，含被套、床单、枕套两只。",
			attrs: pattrs(0, policyStandard, "面料", "60 支长绒棉", "件数", "4 件", "工艺", "活性印染"),
			skus: []skuDef{
				{"-01", "1.5m 床 灰", specs("适用床宽", "1.5m", "颜色", "浅灰"), 39900, 20},
				{"-02", "1.8m 床 灰", specs("适用床宽", "1.8m", "颜色", "浅灰"), 42900, 16},
				{"-03", "1.8m 床 米", specs("适用床宽", "1.8m", "颜色", "米白"), 42900, 10},
			}},
		{code: "P020", name: "暖织 羊毛围巾", brand: "暖织", category: "服装鞋包",
			desc:  "100% 羊毛，尺寸 180×30cm，可作礼品包装。",
			attrs: pattrs(0, policyStandard, "材质", "100% 羊毛", "尺寸", "180×30cm", "礼盒", "可选"),
			skus: []skuDef{
				{"-01", "驼色", specs("颜色", "驼色"), 15900, 34},
				{"-02", "深灰", specs("颜色", "深灰"), 15900, 28},
			}},
		{code: "P061", name: "象饮 保温水壶 800ml", brand: "象饮", category: "服装鞋包",
			desc:  "316 不锈钢内胆，保温 12 小时，户外便携。",
			attrs: pattrs(0, policyStandard, "容量", "800ml", "材质", "316 不锈钢", "保温", "12 小时"),
			skus: []skuDef{
				{"-01", "墨绿", specs("颜色", "墨绿"), 13900, 40},
				{"-02", "沙色", specs("颜色", "沙色"), 13900, 26},
			}},
		{code: "P062", name: "舒展 瑜伽垫 8mm", brand: "舒展", category: "服装鞋包",
			desc:  "TPE 材质，8mm 加厚，附绑带。",
			attrs: pattrs(0, policyStandard, "厚度", "8mm", "尺寸", "183×61cm", "材质", "TPE"),
			skus: []skuDef{
				{"-01", "紫色", specs("颜色", "紫色"), 8900, 50},
				{"-02", "灰色", specs("颜色", "灰色"), 8900, 44},
			}},
		{code: "P069", name: "恒温 保暖内衣套装", brand: "恒温", category: "服装鞋包",
			desc:  "德绒面料，贴身保暖，含上衣与长裤。",
			attrs: pattrs(0, policyIntimate, "面料", "德绒", "套装", "上衣+长裤", "贴身", "是"),
			skus: []skuDef{
				{"-01", "男款 L 深灰", specs("颜色", "深灰", "尺码", "L", "款型", "男款"), 19900, 36},
				{"-02", "男款 XL 深灰", specs("颜色", "深灰", "尺码", "XL", "款型", "男款"), 19900, 22},
				{"-03", "女款 M 藕粉", specs("颜色", "藕粉", "尺码", "M", "款型", "女款"), 19900, 30},
			}},
		{code: "P070", name: "行者 24 寸拉杆行李箱", brand: "行者", category: "服装鞋包",
			desc:  "PC 箱体，万向静音轮，TSA 密码锁。",
			attrs: pattrs(24, policyLargeItem, "尺寸", "24 寸", "箱体", "PC", "轮子", "万向静音轮", "锁具", "TSA 密码锁"),
			skus: []skuDef{
				{"-01", "深灰", specs("颜色", "深灰"), 45900, 18},
				{"-02", "米白", specs("颜色", "米白"), 45900, 11},
			}},

		// ---------------- 礼品与定制（10） ----------------
		{code: "P021", name: "陶语 青瓷茶具礼盒", brand: "陶语", category: "礼品与定制",
			desc:  "8 件套青瓷茶具，含礼盒与手提袋。",
			attrs: pattrs(0, policyStandard, "件数", "8 件", "材质", "青瓷", "礼盒", "含"),
			skus: []skuDef{
				{"-01", "8 件套", specs("件数", "8 件"), 39900, 15},
			}},
		{code: "P022", name: "象饮 保温杯礼盒（可刻字）", brand: "象饮", category: "礼品与定制",
			desc:  "500ml 保温杯礼盒，可选刻字，刻字后不支持无理由退货。",
			attrs: pattrs(0, policyCustomized, "容量", "500ml", "材质", "316 不锈钢", "定制", "刻字"),
			skus: []skuDef{
				{"-01", "礼盒装 不刻字", specs("刻字", "否"), 19900, 33},
				{"-02", "礼盒装 刻字", specs("刻字", "是"), 22900, 20},
			}},
		{code: "P023", name: "花时 鲜花礼盒", brand: "花时", category: "礼品与定制",
			desc:  "当日鲜切花，可预约配送日期，附贺卡。",
			attrs: pattrs(0, policyFreshClaim, "花材", "玫瑰+尤加利", "配送", "预约", "贺卡", "含", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "11 枝", specs("枝数", "11 枝"), 19900, 12},
				{"-02", "19 枝", specs("枝数", "19 枝"), 29900, 8},
			}},
		{code: "P024", name: "可颂 巧克力礼盒", brand: "可颂", category: "礼品与定制",
			desc:  "比利时风味巧克力，需 18℃ 以下保存。",
			attrs: pattrs(0, policyUnopenedOnly, "净含量", "240g", "保质期", "9 个月", "储存", "18℃ 以下", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "24 粒", specs("规格", "24 粒"), 15900, 25},
				{"-02", "32 粒", specs("规格", "32 粒"), 21900, 14},
			}},
		{code: "P025", name: "印刻 定制相册", brand: "印刻", category: "礼品与定制",
			desc:  "按客户照片定制排版，生产周期 3-5 天，不支持无理由退货。",
			attrs: pattrs(0, policyCustomized, "页数", "24 页", "尺寸", "A4/A5", "生产周期", "3-5 天"),
			skus: []skuDef{
				{"-01", "A4 硬壳", specs("尺寸", "A4", "封皮", "硬壳"), 16900, 10},
				{"-02", "A5 软壳", specs("尺寸", "A5", "封皮", "软壳"), 11900, 10},
			}},
		{code: "P026", name: "年礼 节日礼篮", brand: "年礼", category: "礼品与定制",
			desc:  "含坚果、茶叶、糕点与赠品手提袋。",
			attrs: pattrs(0, policyUnopenedOnly, "组合", "坚果+茶叶+糕点", "赠品", "手提袋"),
			skus: []skuDef{
				{"-01", "标准版", specs("规格", "标准"), 29900, 18},
				{"-02", "尊享版", specs("规格", "尊享"), 45900, 9},
			}},
		{code: "P027", name: "拼趣 城市建筑积木", brand: "拼趣", category: "礼品与定制",
			desc:  "1200 片装，适合 14 岁以上，缺件可补发。",
			attrs: pattrs(0, policyStandard, "片数", "1200 片", "年龄", "14+", "缺件", "可补发"),
			skus: []skuDef{
				{"-01", "1200 片", specs("片数", "1200 片"), 25900, 22},
			}},
		{code: "P028", name: "纸间 纪念徽章文具套装", brand: "纸间", category: "礼品与定制",
			desc:  "徽章 2 枚 + 笔记本 + 中性笔，文创礼盒。",
			attrs: pattrs(0, policyStandard, "套装", "徽章+本+笔", "材质", "马口铁/纸质"),
			skus: []skuDef{
				{"-01", "标准套装", specs("套装", "标准"), 6900, 60},
			}},
		{code: "P065", name: "墨川 钢笔礼盒", brand: "墨川", category: "礼品与定制",
			desc:  "含钢笔、墨水一支、礼盒，可选 EF/F 笔尖。",
			attrs: pattrs(12, policyStandard, "笔尖", "EF/F", "配件", "墨水一支", "礼盒", "含"),
			skus: []skuDef{
				{"-01", "EF 尖", specs("笔尖", "EF"), 19900, 16},
				{"-02", "F 尖", specs("笔尖", "F"), 19900, 14},
			}},
		{code: "P066", name: "印刻 定制马克杯", brand: "印刻", category: "礼品与定制",
			desc:  "350ml 陶瓷杯，可定制图案或文字，定制品不退换。",
			attrs: pattrs(0, policyCustomized, "容量", "350ml", "材质", "陶瓷", "定制", "图案/文字"),
			skus: []skuDef{
				{"-01", "350ml 定制", specs("容量", "350ml"), 8900, 40},
			}},

		// ---------------- 生鲜冷链（12） ----------------
		{code: "P029", name: "山野 丹东草莓", brand: "山野", category: "生鲜冷链",
			desc:  "净重 500g/盒，冷链发货，坏果 24 小时内可申诉。",
			attrs: pattrs(0, policyFreshClaim, "净重", "500g", "产地", "辽宁丹东", "储存", "0-4℃ 冷藏", "保质期", "3 天", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "2 盒", specs("规格", "2 盒"), 5900, 60},
				{"-02", "4 盒", specs("规格", "4 盒"), 10900, 30},
			}},
		{code: "P030", name: "山野 赣南脐橙", brand: "山野", category: "生鲜冷链",
			desc:  "果径 75-80mm，净重 5 斤，产地直发。",
			attrs: pattrs(0, policyFreshClaim, "净重", "5 斤", "果径", "75-80mm", "产地", "江西赣州", "储存", "阴凉通风"),
			skus: []skuDef{
				{"-01", "5 斤", specs("净重", "5 斤"), 3990, 80},
				{"-02", "10 斤", specs("净重", "10 斤"), 6990, 40},
			}},
		{code: "P031", name: "果礼 苹果礼盒", brand: "果礼", category: "生鲜冷链",
			desc:  "12 枚装礼盒，单果 80mm 以上，适合送礼。",
			attrs: pattrs(0, policyFreshClaim, "枚数", "12 枚", "单果", "80mm+", "产地", "陕西洛川", "礼盒", "含"),
			skus: []skuDef{
				{"-01", "12 枚", specs("枚数", "12 枚"), 8900, 35},
			}},
		{code: "P032", name: "晨鸣 鲜鸡蛋", brand: "晨鸣", category: "生鲜冷链",
			desc:  "30 枚装，7 日内新鲜蛋，破损包赔。",
			attrs: pattrs(0, policyFreshClaim, "枚数", "30 枚", "净重", "1.5kg", "储存", "0-7℃", "保质期", "30 天"),
			skus: []skuDef{
				{"-01", "30 枚", specs("枚数", "30 枚"), 3990, 90},
			}},
		{code: "P033", name: "牧原 冷冻牛肋条", brand: "牧原", category: "生鲜冷链",
			desc:  "净重 1kg，冷冻分装，冷链运输。",
			attrs: pattrs(0, policyFreshClaim, "净重", "1kg", "储存", "-18℃ 冷冻", "保质期", "12 个月", "分装", "2 袋", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "1kg", specs("净重", "1kg"), 9900, 40},
			}},
		{code: "P034", name: "热带 冷冻榴莲肉", brand: "热带", category: "生鲜冷链",
			desc:  "泰国金枕榴莲肉，-18℃ 冷冻，解冻后即食。",
			attrs: pattrs(0, policyFreshClaim, "净重", "300g", "产地", "泰国", "储存", "-18℃ 冷冻", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "300g", specs("净重", "300g"), 6900, 30},
				{"-02", "600g", specs("净重", "600g"), 12900, 18},
			}},
		{code: "P035", name: "牧场 低温鲜牛奶", brand: "牧场", category: "生鲜冷链",
			desc:  "950ml 巴氏杀菌鲜奶，2-6℃ 冷藏，保质期 7 天。",
			attrs: pattrs(0, policyFreshClaim, "容量", "950ml", "杀菌", "巴氏", "储存", "2-6℃", "保质期", "7 天", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "950ml", specs("容量", "950ml"), 1990, 100},
			}},
		{code: "P036", name: "海港 冷冻虾仁", brand: "海港", category: "生鲜冷链",
			desc:  "净重 500g，去壳去虾线，冷冻保存。",
			attrs: pattrs(0, policyFreshClaim, "净重", "500g", "储存", "-18℃ 冷冻", "加工", "去壳去虾线", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "500g", specs("净重", "500g"), 4900, 55},
			}},
		{code: "P037", name: "田垄 叶菜组合", brand: "田垄", category: "生鲜冷链",
			desc:  "净重 1.5kg，含 4 种时令叶菜，当日采摘。",
			attrs: pattrs(0, policyFreshClaim, "净重", "1.5kg", "组合", "4 种叶菜", "配送", "当日达", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "1.5kg", specs("净重", "1.5kg"), 2990, 70},
			}},
		{code: "P038", name: "快厨 预制菜三拼", brand: "快厨", category: "生鲜冷链",
			desc:  "3 份装冷藏预制菜，加热即食，保质期 5 天。",
			attrs: pattrs(0, policyFreshClaim, "份量", "3 份", "储存", "0-4℃", "保质期", "5 天", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "三拼", specs("份量", "3 份"), 5900, 45},
			}},
		{code: "P067", name: "甜屿 生日蛋糕兑换券", brand: "甜屿", category: "生鲜冷链",
			desc:  "电子兑换券，需提前 24 小时预约，有效期 90 天。",
			attrs: pattrs(0, policyFreshClaim, "有效期", "90 天", "预约", "提前 24 小时", "形式", "电子券"),
			skus: []skuDef{
				{"-01", "6 寸券", specs("尺寸", "6 寸"), 15900, 50},
				{"-02", "8 寸券", specs("尺寸", "8 寸"), 21900, 30},
			}},
		{code: "P068", name: "月圆 广式月饼礼盒", brand: "月圆", category: "生鲜冷链",
			desc:  "8 枚装广式月饼，保质期 45 天，节庆短保商品。",
			attrs: pattrs(0, policyUnopenedOnly, "净含量", "640g", "枚数", "8 枚", "保质期", "45 天"),
			skus: []skuDef{
				{"-01", "8 枚", specs("枚数", "8 枚"), 19900, 24},
			}},

		// ---------------- 家居 / 小家电（10） ----------------
		{code: "P039", name: "米厨 电饭煲 4L", brand: "米厨", category: "家居小家电",
			desc:  "4L 容量，厚釜内胆，12 小时预约。",
			attrs: pattrs(12, policyStandard, "容量", "4L", "功率", "860W", "内胆", "厚釜", "预约", "12 小时"),
			skus: []skuDef{
				{"-01", "4L", specs("容量", "4L"), 39900, 26},
			}},
		{code: "P040", name: "清风 落地电风扇", brand: "清风", category: "家居小家电",
			desc:  "16 英寸，三档风速，可升降。",
			attrs: pattrs(12, policyStandard, "尺寸", "16 英寸", "功率", "55W", "档位", "3 档", "升降", "支持"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准"), 24900, 32},
			}},
		{code: "P041", name: "冰川 双门冰箱 260L", brand: "冰川", category: "家居小家电",
			desc:  "260L 双门，一级能效，送货上楼需预约。",
			attrs: pattrs(36, policyLargeItem, "容量", "260L", "能效", "一级", "尺寸", "550×560×1730mm", "电压", "220V", "上楼", "需预约"),
			skus: []skuDef{
				{"-01", "260L", specs("容量", "260L"), 189900, 8},
			}},
		{code: "P042", name: "洁净 滚筒洗衣机 10kg", brand: "洁净", category: "家居小家电",
			desc:  "10kg 滚筒，变频电机，含基础安装。",
			attrs: pattrs(36, policyLargeItem, "容量", "10kg", "电机", "变频", "安装", "含基础安装", "电压", "220V", "上楼", "需预约"),
			skus: []skuDef{
				{"-01", "10kg", specs("容量", "10kg"), 229900, 6},
			}},
		{code: "P043", name: "净界 空气净化器", brand: "净界", category: "家居小家电",
			desc:  "CADR 400，适用 28-48㎡，滤芯型号 F3。",
			attrs: pattrs(24, policyStandard, "CADR", "400m³/h", "适用面积", "28-48㎡", "滤芯型号", "F3", "噪音", "≤34dB"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准"), 99900, 12},
				{"-02", "标准版+滤芯", specs("版本", "含备用滤芯"), 109900, 7},
			}},
		{code: "P044", name: "洁净 无线吸尘器 V8", brand: "洁净", category: "家居小家电",
			desc:  "25kPa 吸力，续航 45 分钟，配 4 个吸头。",
			attrs: pattrs(24, policyStandard, "吸力", "25kPa", "续航", "45 分钟", "重量", "1.6kg", "配件", "4 个吸头"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准"), 89900, 18},
			}},
		{code: "P045", name: "鲜果 便携榨汁杯", brand: "鲜果", category: "家居小家电",
			desc:  "350ml 随行杯，USB 充电，一键启动。",
			attrs: pattrs(12, policyStandard, "容量", "350ml", "充电", "USB-C", "功率", "50W"),
			skus: []skuDef{
				{"-01", "350ml", specs("容量", "350ml"), 9900, 48},
			}},
		{code: "P046", name: "朗月 护眼台灯", brand: "朗月", category: "家居小家电",
			desc:  "国 AA 级照度，色温可调，无频闪。",
			attrs: pattrs(24, policyStandard, "照度", "国 AA 级", "色温", "3000-5000K", "插头", "国标两脚", "调光", "无级"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准"), 19900, 40},
				{"-02", "Pro 版", specs("版本", "Pro"), 29900, 15},
			}},
		{code: "P047", name: "收纳家 折叠收纳箱 60L", brand: "收纳家", category: "家居小家电",
			desc:  "60L 折叠收纳箱，PP 材质，承重 30kg。",
			attrs: pattrs(0, policyStandard, "容量", "60L", "材质", "PP", "承重", "30kg"),
			skus: []skuDef{
				{"-01", "60L", specs("容量", "60L"), 7900, 66},
			}},
		{code: "P048", name: "眠白 乳胶床垫 1.8m", brand: "眠白", category: "家居小家电",
			desc:  "1.8×2.0m，乳胶厚度 5cm，大件配送需预约。",
			attrs: pattrs(24, policyLargeItem, "尺寸", "1.8×2.0m", "乳胶厚度", "5cm", "硬度", "适中", "上楼", "需预约"),
			skus: []skuDef{
				{"-01", "1.8m", specs("尺寸", "1.8m"), 129900, 7},
			}},

		// ---------------- 食品饮料 / 个护日用（10） ----------------
		{code: "P049", name: "牧场 纯牛奶整箱", brand: "牧场", category: "食品个护",
			desc:  "250ml×24 盒整箱，常温保存，保质期 6 个月。",
			attrs: pattrs(0, policyUnopenedOnly, "规格", "250ml×24", "保质期", "6 个月", "储存", "常温"),
			skus: []skuDef{
				{"-01", "整箱 24 盒", specs("规格", "24 盒"), 6990, 120},
			}},
		{code: "P050", name: "果仁 每日坚果礼盒", brand: "果仁", category: "食品个护",
			desc:  "30 包装，混合坚果，含赠品手提袋。",
			attrs: pattrs(0, policyUnopenedOnly, "净含量", "750g", "包数", "30 包", "保质期", "9 个月", "赠品", "手提袋"),
			skus: []skuDef{
				{"-01", "30 包", specs("包数", "30 包"), 12900, 60},
			}},
		{code: "P051", name: "拾豆 挂耳咖啡", brand: "拾豆", category: "食品个护",
			desc:  "10g×20 包，中度烘焙，充氮保鲜。",
			attrs: pattrs(0, policyUnopenedOnly, "净含量", "200g", "烘焙度", "中度", "保质期", "12 个月"),
			skus: []skuDef{
				{"-01", "20 包", specs("包数", "20 包"), 8900, 90},
			}},
		{code: "P052", name: "云岫 明前龙井礼盒", brand: "云岫", category: "食品个护",
			desc:  "100g 明前龙井，礼盒装，适合送礼。",
			attrs: pattrs(0, policyUnopenedOnly, "净含量", "100g", "等级", "特级", "产地", "浙江杭州", "礼盒", "含"),
			skus: []skuDef{
				{"-01", "100g", specs("净含量", "100g"), 29900, 20},
			}},
		{code: "P053", name: "鲜果 NFC 橙汁", brand: "鲜果", category: "食品个护",
			desc:  "1L×2 瓶，非浓缩还原，冷藏保存。",
			attrs: pattrs(0, policyUnopenedOnly, "容量", "1L×2", "配料", "100% 橙汁", "储存", "0-6℃", "保质期", "21 天", "cold_chain_required", true),
			skus: []skuDef{
				{"-01", "1L×2", specs("容量", "1L×2"), 3990, 50},
			}},
		{code: "P054", name: "皓齿 声波电动牙刷", brand: "皓齿", category: "食品个护",
			desc:  "声波马达，IPX7 防水，刷头型号 T2。",
			attrs: pattrs(24, policyUnopenedOnly, "刷头型号", "T2", "防水", "IPX7", "续航", "30 天", "模式", "3 种"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准"), 19900, 34},
				{"-02", "含 4 刷头", specs("版本", "含刷头"), 24900, 16},
			}},
		{code: "P055", name: "风速 高速吹风机", brand: "风速", category: "食品个护",
			desc:  "11 万转高速马达，恒温护发，国标插头。",
			attrs: pattrs(24, policyStandard, "功率", "1600W", "电压", "220V", "风温", "三档", "重量", "480g"),
			skus: []skuDef{
				{"-01", "标准版", specs("版本", "标准"), 39900, 28},
			}},
		{code: "P056", name: "洁净 洗衣液 3kg", brand: "洁净", category: "食品个护",
			desc:  "3kg 补充装，适用于机洗与手洗。",
			attrs: pattrs(0, policyUnopenedOnly, "容量", "3kg", "适用范围", "棉麻/化纤", "香型", "淡香"),
			skus: []skuDef{
				{"-01", "3kg", specs("容量", "3kg"), 4990, 110},
			}},
		{code: "P057", name: "毛球 全价猫粮", brand: "毛球", category: "食品个护",
			desc:  "2kg 成猫全价粮，鸡肉配方，保质期 18 个月。",
			attrs: pattrs(0, policyUnopenedOnly, "净含量", "2kg", "适用阶段", "成猫", "保质期", "18 个月", "批次", "见包装"),
			skus: []skuDef{
				{"-01", "2kg", specs("净含量", "2kg"), 11900, 45},
			}},
		{code: "P058", name: "柔丫 婴儿湿巾", brand: "柔丫", category: "食品个护",
			desc:  "80 抽×6 包，无香型，含木糖醇。",
			attrs: pattrs(0, policyUnopenedOnly, "片数", "80 抽×6", "成分", "无香无酒精", "保质期", "36 个月"),
			skus: []skuDef{
				{"-01", "6 包", specs("抽数", "80 抽×6"), 4900, 70},
			}},

		// ---------------- IM 场景补齐（3） ----------------
		{code: "P073", name: "暖行 保温杯 500ml", brand: "暖行", category: "家居小家电",
			desc:  "500ml 日用保温杯，高 21 厘米，杯口直径 7 厘米。演示机型，非真实在售型号。",
			attrs: pattrs(0, policyStandard, "容量", "500ml", "高度", "21cm", "杯口直径", "7cm", "材质", "316 不锈钢", "保温", "6 小时"),
			skus: []skuDef{
				{"-01", "雾灰 500ml", specs("颜色", "雾灰", "容量", "500ml"), 12900, 55},
				{"-02", "米白 500ml", specs("颜色", "米白", "容量", "500ml"), 12900, 40},
			}},
		{code: "P074", name: "极充 20W 快充头", brand: "极充", category: "3C数码",
			desc:  "20W USB-C 快充头，兼容主流手机。演示机型。",
			attrs: pattrs(12, policyStandard, "功率", "20W", "协议", "PD", "插头", "国标", "接口", "USB-C"),
			skus: []skuDef{
				{"-01", "白色 20W", specs("颜色", "白色", "功率", "20W"), 4300, 200},
			}},
		{code: "P075", name: "净泡 洗衣凝珠", brand: "净泡", category: "食品个护",
			desc:  "30 颗装洗衣凝珠，淡香型，开封后不支持无理由退货。",
			attrs: pattrs(0, policyUnopenedOnly, "颗数", "30 颗", "香型", "淡香", "保质期", "24 个月"),
			skus: []skuDef{
				{"-01", "30 颗", specs("颗数", "30 颗"), 5900, 90},
			}},
	}
}
