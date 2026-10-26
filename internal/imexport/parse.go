package imexport

import (
	"regexp"
	"strconv"
	"strings"
)

// msgHeadRe 匹配一条消息的头部：
//
//	[2026-09-01 12:03:44] 顾客 文字 #10231
//
// 正文可能跟在同一行，也可能从下一行开始（订单卡 JSON 就是多行）。
var msgHeadRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2})\]\s*[【\[]?([^\s#】\]]+)[】\]]?\s+([^\s#]+)\s*#(\d+)\s*(.*)$`)

// headerLineRe 匹配表头行的 key：value（中文或英文冒号）。
var headerLineRe = regexp.MustCompile(`^([^：:]{2,24})[：:]\s*(.*)$`)

// rawTurn 是一条尚未脱敏的消息，只在内存里存在。
type rawTurn struct {
	idx     int
	ts      string
	role    string
	msgType string
	text    string
}

// rawSession 是一个尚未脱敏的会话。
type rawSession struct {
	storeID  string
	vendorID string
	esID     string
	groupID  string
	taskIDs  []string
	turns    []rawTurn
	warnings []string
}

// ParseSession 解析单个导出 txt。它不抛错：格式不合的行会被跳过并记入
// warnings，因为 1000 个文件里出现个别脏行不该让整批失败。
func ParseSession(fileName string, data []byte) *rawSession {
	s := &rawSession{}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	var cur *rawTurn
	var body []string

	flush := func() {
		if cur == nil {
			return
		}
		joined := strings.TrimSpace(strings.Join(body, "\n"))
		if extra := strings.TrimSpace(cur.text); extra != "" {
			if joined == "" {
				joined = extra
			} else {
				joined = strings.TrimSpace(extra + "\n" + joined)
			}
		}
		cur.text = joined
		s.turns = append(s.turns, *cur)
		cur = nil
		body = nil
	}

	for _, line := range lines {
		if m := msgHeadRe.FindStringSubmatch(line); m != nil {
			flush()
			idx, _ := strconv.Atoi(m[4])
			cur = &rawTurn{
				idx:     idx,
				ts:      normalizeTS(m[1]),
				role:    normalizeRole(m[2]),
				msgType: normalizeType(m[3]),
				text:    m[5],
			}
			continue
		}
		if cur != nil {
			body = append(body, line)
			continue
		}
		// 消息开始之前的行按表头处理。
		if m := headerLineRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			s.applyHeader(strings.ToLower(strings.TrimSpace(m[1])), strings.TrimSpace(m[2]))
		}
	}
	flush()

	// 表头缺失时用文件名兜底：导出文件名形如 1197066_225178_…_PNM.txt，
	// 前两段正是门店 ID 与品牌 ID。
	if s.storeID == "" || s.vendorID == "" {
		parts := strings.Split(fileName, "_")
		if len(parts) >= 2 {
			if s.storeID == "" {
				s.storeID = parts[0]
			}
			if s.vendorID == "" {
				s.vendorID = parts[1]
			}
		}
	}
	if len(s.turns) == 0 {
		s.warnings = append(s.warnings, "no_message_block:"+fileName)
	}
	return s
}

func (s *rawSession) applyHeader(key, val string) {
	switch {
	case strings.Contains(key, "门店"):
		s.storeID = val
	case strings.Contains(key, "品牌") || strings.Contains(key, "商家"):
		s.vendorID = val
	case strings.Contains(key, "es_id") || strings.Contains(key, "esid") || strings.Contains(key, "渠道店"):
		s.esID = val
	case strings.Contains(key, "group") || strings.Contains(key, "会话"):
		s.groupID = val
	case strings.Contains(key, "任务"):
		// 「任务创建」不是任务号，跳过。
		if strings.Contains(key, "创建") || strings.Contains(key, "时间") {
			return
		}
		s.taskIDs = append(s.taskIDs, splitList(val)...)
	}
}

// splitList 按常见分隔符拆任务号，去掉空串。
func splitList(v string) []string {
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '/' || r == ' ' || r == '|'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// normalizeRole 把中文角色映射成下游统一的英文角色。
func normalizeRole(v string) string {
	v = strings.TrimSpace(v)
	switch {
	case strings.HasPrefix(v, "顾客") || strings.HasPrefix(v, "客户") || v == "用户":
		return "user"
	case strings.HasPrefix(v, "商家") || strings.HasPrefix(v, "客服") || strings.HasPrefix(v, "店员"):
		return "merchant"
	case strings.EqualFold(v, "ai") || strings.Contains(v, "智能") || strings.Contains(v, "机器人"):
		return "bot"
	case strings.HasPrefix(v, "系统"):
		return "system"
	}
	return strings.ToLower(v)
}

// normalizeType 只做轻量归一，保留原始类型词供 flags 判定。
func normalizeType(v string) string {
	v = strings.TrimSpace(v)
	switch v {
	case "5", "订单卡", "订单卡片":
		return "5"
	case "商品卡", "商品卡片":
		return "商品卡"
	case "图片", "照片", "image":
		return "图片"
	case "语音", "音频", "voice":
		return "语音"
	case "文字", "文本", "text":
		return "文字"
	}
	return v
}

func normalizeTS(v string) string {
	v = strings.ReplaceAll(v, "T", " ")
	return v
}
