package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
)

// TestOfflineSmoke 跑 testdata/intent/offline_smoke.jsonl：这批用例来自
// duty-im 候选池挖出的用户原话，用来守住 offline 规则模型的分支选择——
// 加词只许把没覆盖的说法接上，不许把原来走对的分支挤掉。
func TestOfflineSmoke(t *testing.T) {
	const path = "../../testdata/intent/offline_smoke.jsonl"
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("冒烟用例不存在，跳过: %v", err)
	}
	defer func() { _ = f.Close() }()

	model := NewOfflineModel()
	ctx := context.Background()
	total, failed := 0, 0

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c struct {
			ID         string `json:"id"`
			Utterance  string `json:"utterance"`
			ExpectTool string `json:"expect_tool"`
		}
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		if c.Utterance == "" || c.ExpectTool == "" {
			t.Fatalf("用例 %s 缺 utterance 或 expect_tool", c.ID)
		}
		total++

		resp, err := model.Respond(ctx, ModelRequest{Messages: []Message{{Role: RoleUser, Content: c.Utterance}}})
		if err != nil {
			t.Fatalf("用例 %s 调用失败: %v", c.ID, err)
		}
		if len(resp.ToolCalls) == 0 {
			failed++
			t.Errorf("用例 %s（%q）没有触发工具调用，期望 %s", c.ID, c.Utterance, c.ExpectTool)
			continue
		}
		if got := resp.ToolCalls[0].Name; got != c.ExpectTool {
			failed++
			t.Errorf("用例 %s（%q）走了 %s，期望 %s", c.ID, c.Utterance, got, c.ExpectTool)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	if total < 15 {
		t.Fatalf("冒烟用例只有 %d 条，实施方案要求 ≥15", total)
	}
	t.Logf("冒烟用例 %d 条，失败 %d 条", total, failed)
}
