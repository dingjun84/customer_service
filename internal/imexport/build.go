package imexport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PlaceholderMedia 用于纯图片/语音消息：正文本身没有可用文本，留占位符
// 让下游知道这里「有内容但不可用」，而不是被当成空消息过滤掉。
const PlaceholderMedia = "{{MEDIA}}"

// Options 控制一次预处理运行。
type Options struct {
	SourceDir string // IM_ROOT
	OutDir    string // 一般是 testdata/im_export
	Limit     int    // 0 表示不限制文件数
	Now       func() time.Time
}

// Run 扫描源目录，产出 candidates.jsonl 与 stats.json。
func Run(opts Options) (*Stats, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	files, err := listSourceFiles(opts.SourceDir)
	if err != nil {
		return nil, err
	}
	if opts.Limit > 0 && len(files) > opts.Limit {
		files = files[:opts.Limit]
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出目录: %w", err)
	}

	outPath := filepath.Join(opts.OutDir, "candidates.jsonl")
	f, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("创建 candidates.jsonl: %w", err)
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)

	st := &Stats{
		SourceRoot:  opts.SourceDir,
		OutputFile:  outPath,
		GeneratedAt: opts.Now().Format(time.RFC3339),
		InputFiles:  len(files),
	}

	for _, path := range files {
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			st.FailedFiles++
			st.Warnings = append(st.Warnings, "read_failed:"+filepath.Base(path))
			continue
		}
		raw := ParseSession(filepath.Base(path), data)
		st.ParsedFiles++
		st.Warnings = append(st.Warnings, raw.warnings...)
		if len(raw.turns) == 0 {
			st.EmptyFiles++
			continue
		}
		cand := buildCandidate(filepath.Base(path), raw, st)
		if cand.DiscardAsGold {
			st.SessionsDiscarded++
			switch cand.DiscardReason {
			case "no_human_merchant":
				st.DiscardedNoHumanMerchant++
			case "no_user_text":
				st.DiscardedNoUserText++
			}
		}
		if err := writeJSONLine(w, cand); err != nil {
			return nil, fmt.Errorf("写 candidates: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		return nil, fmt.Errorf("flush candidates: %w", err)
	}
	st.finalize()
	if err := writeStats(filepath.Join(opts.OutDir, "stats.json"), st); err != nil {
		return nil, err
	}
	return st, nil
}

// buildCandidate 完成一次会话的脱敏、打标与计数，并把命中数累加进 stats。
func buildCandidate(name string, raw *rawSession, st *Stats) Candidate {
	turns := make([]Turn, 0, len(raw.turns))
	texts := make([]string, 0, len(raw.turns))
	userText, merchantHuman := 0, 0

	for _, rt := range raw.turns {
		var redacted string
		switch rt.msgType {
		case "5":
			redacted = PlaceholderOrderCard
			st.RedactOrderCardHit++
		case "商品卡":
			redacted = PlaceholderProductCard
			st.RedactProductCardHit++
		case "图片", "语音":
			// 媒体消息正文可能带 URL，仍然过一遍规则再决定占位符。
			cleaned, hits := Redact(rt.text)
			accumulate(st, hits)
			if strings.TrimSpace(cleaned) == "" {
				redacted = PlaceholderMedia
			} else {
				redacted = cleaned
			}
		default:
			cleaned, hits := Redact(rt.text)
			accumulate(st, hits)
			redacted = cleaned
		}

		// 安全网：任何漏网的真实手机号一律再抹一次，宁可多抹也不落盘。
		if ContainsLeak(redacted) {
			before := redacted
			redacted = phonePattern.ReplaceAllString(redacted, PlaceholderPhone)
			redacted = mediaURLPattern.ReplaceAllString(redacted, PlaceholderMediaURL)
			if redacted != before {
				st.Warnings = append(st.Warnings, "leak_swept:"+name)
			}
		}

		flags := messageFlags(rt.role, rt.msgType, rt.text, redacted)
		for _, fl := range flags {
			switch fl {
			case "is_auto_reply":
				st.FlagAutoReply++
			case "is_wait_bot":
				st.FlagWaitBot++
			case "is_media":
				st.FlagMedia++
			case "is_order_card":
				st.FlagOrderCard++
			case "is_product_card":
				st.FlagProductCard++
			}
		}

		if rt.role == "user" && rt.msgType == "文字" && strings.TrimSpace(redacted) != "" {
			userText++
		}
		if rt.role == "merchant" && !containsFlag(flags, "is_auto_reply") {
			merchantHuman++
		}

		switch rt.role {
		case "user":
			st.RoleUser++
		case "merchant":
			st.RoleMerchant++
		case "bot":
			st.RoleBot++
		case "system":
			st.RoleSystem++
		}
		st.MessagesTotal++

		turns = append(turns, Turn{
			Idx:          rt.idx,
			TS:           rt.ts,
			Role:         rt.role,
			MsgType:      rt.msgType,
			Flags:        flags,
			TextRedacted: redacted,
		})
		if rt.role != "system" {
			texts = append(texts, redacted)
		}
	}

	sf := sessionFlags(turns, texts)
	if sf.HasComplaintCue {
		st.FlagComplaintCue++
	}
	if sf.HasHumanRequest {
		st.FlagHumanRequest++
	}

	cand := Candidate{
		SourceFile:       name,
		StoreID:          raw.storeID,
		VendorID:         raw.vendorID,
		ESID:             raw.esID,
		GroupID:          raw.groupID,
		TaskIDs:          raw.taskIDs,
		MessageCount:     len(turns),
		UserTextCount:    userText,
		MerchantHumanCnt: merchantHuman,
		Turns:            turns,
		FlagsSession:     sf,
		CategoryGuess:    guessCategory(texts),
	}
	if cand.TaskIDs == nil {
		cand.TaskIDs = []string{}
	}
	if cand.GroupID == "" {
		cand.GroupID = "file:" + name
	}
	if merchantHuman == 0 {
		cand.DiscardAsGold = true
		cand.DiscardReason = "no_human_merchant"
	} else if userText < 1 {
		cand.DiscardAsGold = true
		cand.DiscardReason = "no_user_text"
	}
	return cand
}

func accumulate(st *Stats, hits map[string]int) {
	st.RedactPhoneHits += hits["phone"]
	st.RedactTelHits += hits["tel"]
	st.RedactOrderIDHits += hits["order_id"]
	st.RedactContactHits += hits["contact"]
	st.RedactAddressHits += hits["address"]
	st.RedactAgentNameHit += hits["agent_name"]
	st.RedactMediaURLHits += hits["media_url"]
	st.RedactEmailHits += hits["email"]
}

func containsFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}

// finalize 计算会话级汇总与角色占比。
func (st *Stats) finalize() {
	st.SessionsTotal = st.ParsedFiles - st.EmptyFiles
	st.SessionsKept = st.SessionsTotal - st.SessionsDiscarded
	st.Ratios = map[string]float64{}
	if st.MessagesTotal == 0 {
		return
	}
	pct := func(n int) float64 {
		v := float64(n) * 100 / float64(st.MessagesTotal)
		return float64(int(v*10+0.5)) / 10
	}
	st.Ratios["user"] = pct(st.RoleUser)
	st.Ratios["merchant"] = pct(st.RoleMerchant)
	st.Ratios["bot"] = pct(st.RoleBot)
	st.Ratios["system"] = pct(st.RoleSystem)
}

func listSourceFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取源目录 %s: %w", dir, err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".txt") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func writeJSONLine(w *bufio.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

func writeStats(path string, st *Stats) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 stats: %w", err)
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
