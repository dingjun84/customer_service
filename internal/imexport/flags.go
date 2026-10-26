package imexport

import (
	"strings"
)

// waitWords 来自实施方案 §2.4：AI 等待套话。命中即不是金标答复。
var waitWords = []string{
	"稍等", "稍后", "马上", "尽快",
	"看看具体问题", "耐心等待", "还请耐心",
	"这边看看", "在的呢", "请问有什么可以帮",
}

// complaintCues / humanRequestCues 用于会话级信号位，下游 P1-B 靠它们筛转人样本。
var complaintCues = []string{
	"投诉", "差评", "太差", "什么态度", "骗人", "垃圾", "生气", "气死", "无语", "曝光", "举报",
}

var humanRequestCues = []string{
	"人工", "真人", "转客服", "真人客服", "找客服",
}

// categoryKeywords 与 SCENARIOS 里现有 12 类短名一一对应。命中最多的类胜出，
// 平票按本表顺序取前者，保证同一输入永远得到同一结果。
var categoryKeywords = []struct {
	category string
	words    []string
}{
	{"人工客服接入", []string{"人工", "真人", "转客服"}},
	{"投诉与闲聊情绪", []string{"投诉", "差评", "态度", "气死", "无语", "曝光", "举报"}},
	{"发票业务", []string{"发票", "开票", "抬头", "税号", "红冲"}},
	{"退款业务", []string{"退款", "退钱", "到账", "原路退"}},
	{"退换货保修", []string{"退货", "换货", "维修", "保修", "七天", "无理由"}},
	{"补发催件", []string{"补发", "漏发", "少发", "催", "还没到", "缺件"}},
	{"物流配送", []string{"物流", "快递", "配送", "到哪", "骑手", "送达", "签收", "运单"}},
	{"支付金融", []string{"支付", "付款", "白条", "花呗", "扣款", "付不了", "余额"}},
	{"商品质量故障", []string{"坏了", "破损", "质量问题", "漏液", "变质", "发霉", "不工作"}},
	{"优惠价格活动", []string{"优惠", "券", "满减", "便宜", "价保", "差价", "活动价"}},
	{"订单修改", []string{"改地址", "改电话", "修改", "取消订单", "换个地址"}},
	{"商品售前咨询", []string{"规格", "尺寸", "颜色", "有货", "怎么用", "能用吗", "保质期"}},
}

// messageFlags 给单条消息打标。
func messageFlags(role, msgType, rawText string, redacted string) []string {
	flags := make([]string, 0, 4)
	trimmed := strings.TrimSpace(rawText)

	if role == "merchant" && strings.HasPrefix(trimmed, "[自动回复]") {
		flags = append(flags, "is_auto_reply")
	}
	if role == "bot" && containsAny(trimmed, waitWords) {
		flags = append(flags, "is_wait_bot")
	}
	if msgType == "图片" || msgType == "语音" || strings.Contains(redacted, PlaceholderMediaURL) {
		flags = append(flags, "is_media")
	}
	if msgType == "5" || strings.Contains(rawText, "order_view_id") {
		flags = append(flags, "is_order_card")
	}
	if msgType == "商品卡" {
		flags = append(flags, "is_product_card")
	}
	return flags
}

// sessionFlags 汇总会话级信号位。
func sessionFlags(turns []Turn, texts []string) SessionFlags {
	f := SessionFlags{}
	for _, t := range turns {
		for _, fl := range t.Flags {
			if fl == "is_media" {
				f.HasMedia = true
			}
		}
	}
	for _, s := range texts {
		if containsAny(s, complaintCues) {
			f.HasComplaintCue = true
		}
		if containsAny(s, humanRequestCues) {
			f.HasHumanRequest = true
		}
	}
	return f
}

// guessCategory 用关键词计数给会话一个弱标注；没有命中就返回 nil，
// 下游据此知道「这条没猜出来」，而不是被塞一个默认类。
func guessCategory(texts []string) *string {
	scores := make([]int, len(categoryKeywords))
	for _, s := range texts {
		for i, c := range categoryKeywords {
			for _, w := range c.words {
				if strings.Contains(s, w) {
					scores[i]++
				}
			}
		}
	}
	best, bestScore := -1, 0
	for i, sc := range scores {
		if sc > bestScore {
			best, bestScore = i, sc
		}
	}
	if best < 0 {
		return nil
	}
	cat := categoryKeywords[best].category
	return &cat
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
