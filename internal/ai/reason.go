package ai

import (
	"strings"
)

// The "reason" note is a demo-grade explainability hook, not a business rule.
//
// The system prompt asks the model to open every reply (both the rounds that call
// tools and the final answer) with one short line explaining why. This file owns
// the single parsing path so every adapter behaves the same: a real provider's
// text is parsed here, the offline rule model sets ModelResponse.Reason directly,
// and the scripted test model simply leaves it empty.

// Reason kinds reported to the API/UI.
const (
	// ReasonKindToolCall marks the note of a round that only requests tools.
	ReasonKindToolCall = "tool_call"
	// ReasonKindAnswer marks the note of the reply the user actually reads.
	ReasonKindAnswer = "answer"
)

// reasonTagOpen / reasonTagClose is the preferred delimited form.
const (
	reasonTagOpen  = "<备注>"
	reasonTagClose = "</备注>"
)

// reasonMaxRunes caps the note: it is a label, not an explanation.
const reasonMaxRunes = 160

// SplitReason pulls the leading note out of model text and returns it together
// with the remaining answer. A model that ignores the instruction yields ("", original),
// so callers never lose content because of a formatting miss.
func SplitReason(content string) (reason string, rest string) {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if trimmed == "" {
		return "", content
	}

	if strings.HasPrefix(trimmed, reasonTagOpen) {
		body := trimmed[len(reasonTagOpen):]
		if end := strings.Index(body, reasonTagClose); end >= 0 {
			return cleanReason(body[:end]), strings.TrimLeft(body[end+len(reasonTagClose):], " \t\r\n")
		}
		// Unclosed tag: trust only the first line so the answer body survives.
		line, tail := splitFirstLine(body)
		return cleanReason(line), tail
	}

	line, tail := splitFirstLine(trimmed)
	if note, ok := reasonFromLine(line); ok {
		return note, tail
	}
	return "", content
}

// splitFirstLine returns the first line and everything after it, minus the
// blank lines that separate the note from the body.
func splitFirstLine(s string) (string, string) {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx], strings.TrimLeft(s[idx+1:], " \t\r\n")
	}
	return s, ""
}

// reasonFromLine recognises the prefix forms models actually emit: the tag, the
// CJK bracket, a bare "备注：", and markdown-bold wrappers around any of them.
func reasonFromLine(line string) (string, bool) {
	s := strings.TrimSpace(line)
	s = strings.TrimSpace(strings.Trim(s, "*_"))
	for _, prefix := range []string{
		reasonTagOpen, "【备注】", "[备注]", "【理由】",
		"备注：", "备注:", "理由：", "理由:",
		"reason:", "Reason:", "REASON:",
	} {
		if !strings.HasPrefix(s, prefix) {
			continue
		}
		out := cleanReason(strings.TrimPrefix(s, prefix))
		if out == "" {
			return "", false
		}
		return out, true
	}
	return "", false
}

// cleanReason normalises one note into a single short line.
func cleanReason(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, reasonTagClose)
	s = strings.TrimSpace(strings.Trim(s, "*_"))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, "“”\"'")
	return truncate(s, reasonMaxRunes)
}

// NormalizeResponse separates the note from the answer. Adapters that already
// know the note (the offline model) set Reason and keep Content clean, so the
// parse only runs when the note came back as text.
func NormalizeResponse(resp ModelResponse) ModelResponse {
	if strings.TrimSpace(resp.Reason) != "" {
		resp.Content = strings.TrimSpace(resp.Content)
		return resp
	}
	reason, content := SplitReason(resp.Content)
	resp.Reason = reason
	resp.Content = strings.TrimSpace(content)
	return resp
}

// WithReason attaches a note to a response. Models that produce their own
// justification text call this instead of embedding markup.
func WithReason(resp ModelResponse, reason string) ModelResponse {
	resp.Reason = cleanReason(reason)
	return resp
}
