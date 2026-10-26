package imexport

import (
	"regexp"
	"strings"
)

// 脱敏占位符。下游（评测、口吻、意图）看到的是这些占位符，而不是真值。
const (
	PlaceholderPhone       = "{{PHONE}}"
	PlaceholderTel         = "{{TEL}}"
	PlaceholderOrderID     = "{{ORDER_ID}}"
	PlaceholderContact     = "{{CONTACT}}"
	PlaceholderAddress     = "{{ADDRESS}}"
	PlaceholderAgentName   = "{{AGENT_NAME}}"
	PlaceholderMediaURL    = "{{MEDIA_URL}}"
	PlaceholderOrderCard   = "{{ORDER_CARD}}"
	PlaceholderProductCard = "{{PRODUCT_CARD}}"
	PlaceholderEmail       = "{{EMAIL}}"
)

// redactRule 是一条有序规则：顺序决定谁先吃掉文本，手机号必须先于长数字，
// 否则 11 位手机号会被当成订单号。
type redactRule struct {
	key         string
	re          *regexp.Regexp
	placeholder string
	// group 指定只替换第几个捕获组（1 起），0 表示替换整体命中。
	// 「加微信 abc123」这类只替换号本身、保留「加微信」前缀的规则就靠它。
	group int
}

// phonePattern / mediaURLPattern 同时被安全网复用：脱敏跑完后仍能命中，
// 说明规则有漏，需要再抹一次并记 warn。phonePattern 也要求前后不是数字，
// 否则 18 位统一社会信用代码里的 11 位片段会被误判成手机号。
var (
	phonePattern    = regexp.MustCompile(`(?:^|[^\d])1[3-9]\d{9}(?:[^\d]|$)`)
	mediaURLPattern = regexp.MustCompile(`https?://[^\s"'<>，。；、）)]+`)
)

var redactRules = []redactRule{
	{
		key:         "media_url",
		re:          mediaURLPattern,
		placeholder: PlaceholderMediaURL,
	},
	{
		// 导流号排在手机号之前：「加V15216899332」是微信号，先按 CONTACT 处理，
		// 否则会被当成手机号，丢掉「这是导流话术」的语义。
		key:         "contact",
		re:          regexp.MustCompile(`(?i)((?:微信|weixin|vx|v信|wx|威信|薇信)\s*(?:号|同号)?\s*[:：]?\s*)([A-Za-z][-_A-Za-z0-9]{4,19}|\d{5,20})`),
		placeholder: PlaceholderContact,
		group:       2,
	},
	{
		// 「加下VV13915977921」「➕V13052850715」：必须有「加/添加/搜」前缀，
		// 避免把正文里单个字母 v 误当导流。
		key:         "contact",
		re:          regexp.MustCompile(`((?:加|添加|搜|➕|＋|\+)\s*(?:个|下|一下)?\s*[vV]{1,2}\s*(?:号|同号)?\s*[:：]?\s*)([A-Za-z][-_A-Za-z0-9]{4,19}|\d{5,20})`),
		placeholder: PlaceholderContact,
		group:       2,
	},
	{
		// 前后都不能是数字，且允许 +86 前缀；执行顺序上已在导流号之后。
		key:         "phone",
		re:          regexp.MustCompile(`(?:^|[^\d])(?:\+?86)?(1[3-9]\d{9})(?:[^\d]|$)`),
		placeholder: PlaceholderPhone,
		group:       1,
	},
	{
		key:         "tel",
		re:          regexp.MustCompile(`\b(?:0\d{2,3}[-\s]?\d{7,8}|400[-\s]?\d{3}[-\s]?\d{4})\b`),
		placeholder: PlaceholderTel,
	},
	{
		// 以省级名开头，避免把「我的地址是广东省…」里的「我的地址是」一起吃进去。
		key:         "address",
		re:          regexp.MustCompile(`(?:北京|天津|上海|重庆|河北|山西|辽宁|吉林|黑龙江|江苏|浙江|安徽|福建|江西|山东|河南|湖北|湖南|广东|海南|四川|贵州|云南|陕西|甘肃|青海|台湾|内蒙古|广西|西藏|宁夏|新疆|香港|澳门)(?:省|市|自治区|特别行政区)?[\p{Han}]{0,10}(?:市|区|县)[\p{Han}\d]{0,25}`),
		placeholder: PlaceholderAddress,
	},
	{
		key:         "address",
		re:          regexp.MustCompile(`[\p{Han}\d]{2,10}(?:路|街|大道|巷|胡同)[\p{Han}\d]{0,8}(?:号|弄|栋|楼|单元|室)[\p{Han}\d\-]{0,20}`),
		placeholder: PlaceholderAddress,
	},
	{
		key:         "address",
		re:          regexp.MustCompile(`(?:收件|收货|送|寄)\s*地址\s*[:：]?\s*[^\n]{2,40}`),
		placeholder: PlaceholderAddress,
	},
	{
		// 「转交王芳同事」里的客服名。规则刻意收窄，避免把「转交人工」当人名；
		// 只替换名字，保留「转交…同事」这个动作词。
		key:         "agent_name",
		re:          regexp.MustCompile(`(?:转交|转给)([\p{Han}]{2})(?:同事|经理|主管)`),
		placeholder: PlaceholderAgentName,
		group:       1,
	},
	{
		// 抬头/税号常与邮箱一起给出，邮箱不在 §2.5 的表里，但同样不该落进
		// 候选池，这里一并替换并单独计数。必须排在长数字之前，否则长邮箱
		// 前缀里的数字会先把整段吃掉。
		key:         "email",
		re:          regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`),
		placeholder: PlaceholderEmail,
	},
	{
		// 长数字不要求尾部词边界：统一社会信用代码常见「9142000070680457XA」
		// 这种数字后直接跟字母的写法，加 \b 会整段漏掉。
		key:         "order_id",
		re:          regexp.MustCompile(`\d{10,}`),
		placeholder: PlaceholderOrderID,
	},
}

// Redact 返回脱敏后的文本与各规则命中次数。命中次数按规则 key 累计，
// 同一 key 的多条规则（如地址）合并计数。
func Redact(text string) (string, map[string]int) {
	hits := map[string]int{}
	out := text
	for _, rule := range redactRules {
		if rule.group > 0 {
			out = rule.re.ReplaceAllStringFunc(out, func(m string) string {
				sub := rule.re.FindStringSubmatch(m)
				if len(sub) <= rule.group {
					return m
				}
				hits[rule.key]++
				return strings.Replace(m, sub[rule.group], rule.placeholder, 1)
			})
			continue
		}
		out = rule.re.ReplaceAllStringFunc(out, func(string) string {
			hits[rule.key]++
			return rule.placeholder
		})
	}
	return strings.TrimSpace(out), hits
}

// ContainsLeak 用于验收自查：脱敏后的文本里不应再出现 11 位手机号或 http 原文。
func ContainsLeak(text string) bool {
	if phonePattern.MatchString(text) {
		return true
	}
	return strings.Contains(text, "http://") || strings.Contains(text, "https://")
}
