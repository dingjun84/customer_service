package style

import (
	"encoding/json"
	"os"
	"testing"
)

const (
	badPath      = "../../testdata/style/bad_replies.jsonl"
	goldPath     = "../../testdata/style/gold_replies.jsonl"
	negativePath = "../../testdata/style/auto_reply_negatives.jsonl"
)

type badReply struct {
	ID         string   `json:"id"`
	Reply      string   `json:"reply"`
	Violations []string `json:"violations"`
}

type goldReply struct {
	ID     string `json:"id"`
	Reply  string `json:"reply"`
	Source string `json:"source_file"`
}

type negative struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text_redacted"`
}

func readJSONL[T any](t *testing.T, path string) []T {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("数据集不存在，跳过: %v", err)
	}
	defer func() { _ = f.Close() }()
	var out []T
	dec := json.NewDecoder(f)
	for dec.More() {
		var v T
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		out = append(out, v)
	}
	return out
}

// TestBadRepliesAreCaught 让 bad_replies.jsonl 变成可执行断言：每条反例标注的
// violations 必须真的被检出，否则说明要么反例写错，要么检查器漏了。
func TestBadRepliesAreCaught(t *testing.T) {
	rows := readJSONL[badReply](t, badPath)
	if len(rows) < 8 || len(rows) > 12 {
		t.Fatalf("反例应为 8~12 条，实际 %d", len(rows))
	}
	covered := map[string]bool{}
	for _, row := range rows {
		tags, _ := Check(row.Reply)
		detected := map[string]bool{}
		for _, tag := range tags {
			detected[tag] = true
			covered[tag] = true
		}
		if len(row.Violations) == 0 {
			t.Fatalf("%s 没有标注 violations", row.ID)
		}
		for _, want := range row.Violations {
			if !detected[want] {
				t.Errorf("%s 应命中 %s，实际命中 %v（原文：%s）", row.ID, want, tags, row.Reply)
			}
		}
	}
	if len(covered) < 6 {
		t.Fatalf("反例只覆盖 %d 种违规，实施方案要求 ≥6：%v", len(covered), covered)
	}
	t.Logf("反例 %d 条，覆盖违规类型 %d 种", len(rows), len(covered))
}

// TestGoldRepliesAreClean 正例不允许被误伤：检查器若把正例判成违规，
// 说明规则太宽，上了真实模型会大量假阳性。
func TestGoldRepliesAreClean(t *testing.T) {
	rows := readJSONL[goldReply](t, goldPath)
	if len(rows) < 8 || len(rows) > 12 {
		t.Fatalf("正例应为 8~12 条，实际 %d", len(rows))
	}
	for _, row := range rows {
		if tags, notes := Check(row.Reply); len(tags) > 0 {
			t.Errorf("%s 被误判为违规 %v（%v）：%s", row.ID, tags, notes, row.Reply)
		}
		if row.Source == "" {
			t.Errorf("%s 缺 source_file，无法回溯来源", row.ID)
		}
	}
}

// TestAutoReplyNegatives 守住 P2-B 的负例规模，并保证「欢迎光临」这类复读
// 真的能被抓住——这是 §6.2 要求的 5 条回归。
func TestAutoReplyNegatives(t *testing.T) {
	rows := readJSONL[negative](t, negativePath)
	if len(rows) < 100 {
		t.Fatalf("自动回复负例应 ≥100 条，实际 %d", len(rows))
	}
	kinds := map[string]bool{"system": true, "merchant_auto": true, "bot_wait": true}
	caught := 0
	for _, row := range rows {
		if !kinds[row.Kind] {
			t.Fatalf("%s 的 kind 非法: %q", row.ID, row.Kind)
		}
		if row.Text == "" {
			t.Fatalf("%s 文本为空", row.ID)
		}
		if caught >= 5 {
			continue
		}
		if tags, _ := Check(row.Text); len(tags) > 0 {
			caught++
		}
	}
	if caught < 5 {
		t.Fatalf("只有 %d 条负例能被规则抓住，实施方案要求 5 条回归", caught)
	}
	t.Logf("负例 %d 条，可被规则抓住的样本 %d 条", len(rows), caught)
}
