// Package style 用固定规则检查一条客服回复是否违反口吻约定。
//
// 它不是"像不像人"的主观评判，只抓可机械判定的八类问题（见 Violations）。
// 用途有两个：把 testdata/style 下的反例集变成可执行断言；以后给真实模型
// 的输出打分时，同一套规则可以直接复用。
package style

import (
	"regexp"
	"strings"
)

// 违规标签，顺序即报告顺序，便于 diff。
const (
	ClaimsAI         = "claims_ai"
	Markdown         = "markdown"
	SectionHeaders   = "section_headers"
	AsksTransferPerm = "asks_transfer_permission"
	ExplainsSchema   = "explains_schema"
	ClosingFiller    = "closing_filler"
	OverPromise      = "over_promise"
	AutoWelcome      = "auto_welcome"
)

// Order 是报告输出顺序。
var Order = []string{
	ClaimsAI, Markdown, SectionHeaders, AsksTransferPerm,
	ExplainsSchema, ClosingFiller, OverPromise, AutoWelcome,
}

type rule struct {
	tag  string
	re   *regexp.Regexp
	note string
}

var rules = []rule{
	{ClaimsAI, regexp.MustCompile(`(?i)(\bAI\b|人工智能|机器人|智能助手|大模型|语言模型)`),
		"自称或暗示自己是 AI/机器人"},
	{Markdown, regexp.MustCompile(`(?m)(\*\*[^*]+\*\*|^#{1,6}\s|^\s*[-*]\s+|^\s*\d+[.、)]\s+)`),
		"使用了 Markdown 加粗/标题/列表"},
	{SectionHeaders, regexp.MustCompile(`(结论[:：]|建议做法[:：]|建议[:：]|原因[:：]|两点可参考|步骤如下[:：])`),
		"使用了「结论：」「建议：」这类小标题"},
	{AsksTransferPerm, regexp.MustCompile(`(需要我(帮您|帮你)?转人工吗|要不要转人工|需要(帮您|帮你)?转接人工吗|是否转人工)`),
		"把转人当成需要用户许可的选项来征询"},
	{ExplainsSchema, regexp.MustCompile(`(系统里没有|没有这个字段|数据库里|字段缺失|表里没有|后台没有这项数据)`),
		"向客户解释内部字段或数据结构"},
	{ClosingFiller, regexp.MustCompile(`(还需要帮忙吗|还需要什么帮助|还有其他问题|很高兴为您服务|祝您(生活愉快|购物愉快)|随时(联系|找我|问我))`),
		"结尾套话"},
	{OverPromise, regexp.MustCompile(`(已退款|已取消|已经(为您|给你)?退款|已经(帮您|帮你)?取消|保证(今天|明天)|一定到账|马上到账|(应该|估计|大概)(今天|明天|后天)?(就)?能(到|送到|到账))`),
		"未经确认就声称已退款/已取消或承诺到账"},
	{AutoWelcome, regexp.MustCompile(`(欢迎光临|亲[,，]?\s*欢迎光临|欢迎再次光临)`),
		"复读门店欢迎语"},
}

// Check 返回命中的违规标签（按 Order 去重排序）与每条的说明。
func Check(reply string) ([]string, []string) {
	text := reply
	hits := map[string]string{}
	for _, r := range rules {
		if r.re.MatchString(text) {
			hits[r.tag] = r.note
		}
	}
	// 微信式客服用「你」，出现大量「您」时不算违规，只作为观察项不输出，
	// 避免把礼貌用语误判成风格问题。
	var tags, notes []string
	for _, tag := range Order {
		if note, ok := hits[tag]; ok {
			tags = append(tags, tag)
			notes = append(notes, note)
		}
	}
	return tags, notes
}

// Clean 表示这条回复可以直接发出去。
func Clean(reply string) bool {
	if strings.TrimSpace(reply) == "" {
		return false
	}
	tags, _ := Check(reply)
	return len(tags) == 0
}
