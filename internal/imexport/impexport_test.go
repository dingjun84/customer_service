package imexport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 这份夹具按实施方案 §2.2 的真实格式写：表头中文冒号、消息头时间戳 + 角色 +
// 类型 + #ID、正文可多行。
const fixtureSession = `门店ID：1197066
品牌ID：225178
渠道店es_id：es_88123
会话group_id：grp_99001
任务：task_001,task_002
任务创建：2026-09-01 11:00:00
消息范围：10:00-12:00

[2026-09-01 12:03:44] 顾客 文字 #10231
我电话13812345678，地址广东省深圳市南山区科技园路1号，麻烦快点
[2026-09-01 12:03:50] 商家 文字 #10232
[自动回复]亲，欢迎光临本店，客服稍后回复
[2026-09-01 12:04:02] AI 文字 #10233
亲，稍微等一下，这边看看哦
[2026-09-01 12:04:30] 商家 文字 #10234
在的，我查下
[2026-09-01 12:05:10] 系统 文字 #10235
会话已转接
[2026-09-01 12:05:30] 顾客 图片 #10238
[2026-09-01 12:06:00] 顾客 5 #10236
{"order_view_id":"2026090112345678","title":"订单"}
[2026-09-01 12:07:00] 商家 文字 #10237
转交王芳同事帮你处理，方便的话加微信 shop_abc123
`

func TestParseSessionHeaderAndMessages(t *testing.T) {
	s := ParseSession("1197066_225178_x_PNM.txt", []byte(fixtureSession))
	if s.storeID != "1197066" || s.vendorID != "225178" {
		t.Fatalf("表头门店/品牌解析错误: %q %q", s.storeID, s.vendorID)
	}
	if s.esID != "es_88123" || s.groupID != "grp_99001" {
		t.Fatalf("es_id/group_id 解析错误: %q %q", s.esID, s.groupID)
	}
	if len(s.taskIDs) != 2 {
		t.Fatalf("任务号应解析出 2 个，实际 %v", s.taskIDs)
	}
	if len(s.turns) != 8 {
		t.Fatalf("消息数应为 8，实际 %d: %+v", len(s.turns), s.turns)
	}
	if got := s.turns[0].role; got != "user" {
		t.Fatalf("顾客应映射为 user，实际 %q", got)
	}
	if got := s.turns[2].role; got != "bot" {
		t.Fatalf("AI 应映射为 bot，实际 %q", got)
	}
	if got := s.turns[6].msgType; got != "5" {
		t.Fatalf("订单卡类型应为 5，实际 %q", got)
	}
}

func TestBuildCandidateFlagsAndRedaction(t *testing.T) {
	raw := ParseSession("a.txt", []byte(fixtureSession))
	st := &Stats{}
	cand := buildCandidate("a.txt", raw, st)

	if cand.DiscardAsGold {
		t.Fatalf("有人工商家与用户文字，不应丢弃: %s", cand.DiscardReason)
	}
	if cand.MerchantHumanCnt != 2 {
		t.Fatalf("人工商家条数应为 2（自动回复不计），实际 %d", cand.MerchantHumanCnt)
	}
	if cand.UserTextCount != 1 {
		t.Fatalf("用户文字条数应为 1，实际 %d", cand.UserTextCount)
	}
	if !cand.FlagsSession.HasMedia {
		t.Fatalf("含图片消息，has_media 应为 true")
	}

	byIdx := map[int]Turn{}
	for _, tn := range cand.Turns {
		byIdx[tn.Idx] = tn
	}
	flagsOf := func(idx int) []string { return byIdx[idx].Flags }
	if !containsFlag(flagsOf(10232), "is_auto_reply") {
		t.Fatalf("自动回复应标记 is_auto_reply: %v", flagsOf(10232))
	}
	if !containsFlag(flagsOf(10233), "is_wait_bot") {
		t.Fatalf("AI 套话应标记 is_wait_bot: %v", flagsOf(10233))
	}
	if !containsFlag(flagsOf(10236), "is_order_card") {
		t.Fatalf("订单卡应标记 is_order_card: %v", flagsOf(10236))
	}
	if !containsFlag(flagsOf(10238), "is_media") {
		t.Fatalf("图片应标记 is_media: %v", flagsOf(10238))
	}

	// 落盘文本不得含真实手机号、真实地址、真实微信/订单号。
	for _, tn := range cand.Turns {
		if ContainsLeak(tn.TextRedacted) {
			t.Fatalf("脱敏后仍泄漏: %q", tn.TextRedacted)
		}
		for _, bad := range []string{"13812345678", "shop_abc123", "科技园路", "王芳"} {
			if strings.Contains(tn.TextRedacted, bad) {
				t.Fatalf("脱敏后仍含 %q: %q", bad, tn.TextRedacted)
			}
		}
	}
	if !strings.Contains(cand.Turns[0].TextRedacted, PlaceholderPhone) {
		t.Fatalf("手机号应替换为占位符: %q", cand.Turns[0].TextRedacted)
	}
	if byIdx[10236].TextRedacted != PlaceholderOrderCard {
		t.Fatalf("订单卡正文应整体替换: %q", byIdx[10236].TextRedacted)
	}
	if st.RedactPhoneHits == 0 || st.RedactAddressHits == 0 || st.RedactContactHits == 0 {
		t.Fatalf("脱敏命中计数缺失: %+v", st)
	}
}

func TestDiscardWhenNoHumanMerchant(t *testing.T) {
	const onlyBot = `[2026-09-01 12:00:00] 顾客 文字 #1
在吗
[2026-09-01 12:00:05] 商家 文字 #2
[自动回复]您好，稍后回复
[2026-09-01 12:00:10] AI 文字 #3
亲，在的呢
`
	raw := ParseSession("b.txt", []byte(onlyBot))
	cand := buildCandidate("b.txt", raw, &Stats{})
	if !cand.DiscardAsGold || cand.DiscardReason != "no_human_merchant" {
		t.Fatalf("只有自动回复的会话应丢弃，实际 %+v", cand)
	}
}

func TestRedactTable(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"手机", "打13812345678找她", "打{{PHONE}}找她"},
		{"座机", "门店电话0755-12345678", "门店电话{{TEL}}"},
		{"长数字", "单号2026090112345678", "单号{{ORDER_ID}}"},
		{"微信", "加微信shop_abc123聊", "加微信{{CONTACT}}聊"},
		{"加V号", "开票可以加V15216899332哈～", "开票可以加V{{CONTACT}}哈～"},
		{"加下VV", "话，加下VV13915977921办理哈～", "话，加下VV{{CONTACT}}办理哈～"},
		{"加号符", "递事宜 ➕V13052850715", "递事宜 ➕V{{CONTACT}}"},
		{"地址", "广东省深圳市南山区科技园路1号", "{{ADDRESS}}"},
		{"链接", "看图 https://img.example.com/a.jpg 谢谢", "看图 {{MEDIA_URL}} 谢谢"},
		{"客服名", "转交王芳同事跟进", "转交{{AGENT_NAME}}同事跟进"},
		{"带区号手机", "电话8613812345678", "电话86{{PHONE}}"},
		{"信用代码", "统一社会信用代码：9142000070680457", "统一社会信用代码：{{ORDER_ID}}"},
		{"信用代码带字母", "税号：9142000070680457XA", "税号：{{ORDER_ID}}XA"},
		{"邮箱", "邮箱：147508389@qq.com", "邮箱：{{EMAIL}}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := Redact(tc.in)
			if got != tc.want {
				t.Fatalf("Redact(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRunWritesCandidatesAndStats(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "1197066_225178_a_PNM.txt"), []byte(fixtureSession), 0o644); err != nil {
		t.Fatal(err)
	}
	// 非 txt 文件与空文件都必须被跳过而不是报错。
	if err := os.WriteFile(filepath.Join(src, "_manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "empty.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := Run(Options{SourceDir: src, OutDir: out})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if st.InputFiles != 2 {
		t.Fatalf("只应统计 2 个 txt，实际 %d", st.InputFiles)
	}
	if st.SessionsTotal != 1 || st.SessionsKept != 1 {
		t.Fatalf("会话统计错误: total=%d kept=%d", st.SessionsTotal, st.SessionsKept)
	}
	if st.Ratios["user"] <= 0 || st.Ratios["merchant"] <= 0 {
		t.Fatalf("角色占比未计算: %v", st.Ratios)
	}
	if _, err := os.Stat(filepath.Join(out, "candidates.jsonl")); err != nil {
		t.Fatalf("candidates.jsonl 未生成: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "stats.json")); err != nil {
		t.Fatalf("stats.json 未生成: %v", err)
	}
}
