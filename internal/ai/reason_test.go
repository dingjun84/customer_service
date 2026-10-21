package ai

import "testing"

func TestSplitReasonForms(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		reason string
		rest   string
	}{
		{
			name:   "tagged line then answer",
			in:     "<备注>先列出名下订单确认是哪一笔</备注>\n\n你的名下有两笔订单。",
			reason: "先列出名下订单确认是哪一笔",
			rest:   "你的名下有两笔订单。",
		},
		{
			name:   "colon form with blank line",
			in:     "备注：物流是实时数据，必须查轨迹\n\n你的包裹正在派送中。",
			reason: "物流是实时数据，必须查轨迹",
			rest:   "你的包裹正在派送中。",
		},
		{
			name:   "markdown bold wrapper",
			in:     "**备注：**已拿到签收时间，直接回答\n\n包裹已签收。",
			reason: "已拿到签收时间，直接回答",
			rest:   "包裹已签收。",
		},
		{
			name:   "cjk bracket form",
			in:     "【备注】用户没给订单号，先查列表\n\n请选择订单。",
			reason: "用户没给订单号，先查列表",
			rest:   "请选择订单。",
		},
		{
			name:   "note only, tool round",
			in:     "备注：单轮工具次数已达上限，先给兜底话术",
			reason: "单轮工具次数已达上限，先给兜底话术",
			rest:   "",
		},
		{
			name:   "no note keeps every byte",
			in:     "你的包裹已签收，签收时间 10-02 14:00。",
			reason: "",
			rest:   "你的包裹已签收，签收时间 10-02 14:00。",
		},
		{
			name:   "unclosed tag keeps the body",
			in:     "<备注>先查订单\n包裹在派送中。",
			reason: "先查订单",
			rest:   "包裹在派送中。",
		},
		{
			name:   "only the first line becomes the note",
			in:     "备注：第一行说完了\n第二行仍属于正文\n\n正文。",
			reason: "第一行说完了",
			rest:   "第二行仍属于正文\n\n正文。",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, rest := SplitReason(tc.in)
			if reason != tc.reason {
				t.Fatalf("reason = %q, want %q", reason, tc.reason)
			}
			if rest != tc.rest {
				t.Fatalf("rest = %q, want %q", rest, tc.rest)
			}
		})
	}
}

func TestNormalizeResponseKeepsAdapterNote(t *testing.T) {
	// The offline model sets Reason itself and its Content never carries markup.
	resp := NormalizeResponse(ModelResponse{Content: "你的包裹正在派送中。", Reason: "已查到轨迹节点"})
	if resp.Reason != "已查到轨迹节点" {
		t.Fatalf("reason = %q", resp.Reason)
	}
	if resp.Content != "你的包裹正在派送中。" {
		t.Fatalf("content = %q", resp.Content)
	}

	// A provider that ignores the instruction must not lose its answer.
	plain := NormalizeResponse(ModelResponse{Content: " 你好，有什么可以帮你？ "})
	if plain.Reason != "" || plain.Content != "你好，有什么可以帮你？" {
		t.Fatalf("plain = %+v", plain)
	}
}
