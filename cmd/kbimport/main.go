// Command kbimport 把京东帮助中心的问答（docs/jd-faq/faq.jsonl）导入 knowledge_articles。
//
// 数据在库外用 docs/jd-faq/ 维护，本命令只负责落库，所以可以随时重跑：
// 以 article_code 为自然键，内容没变就跳过，变了就 version+1 覆盖，不会产生重复行。
// 默认写入 draft，避免未经核对的时效性内容直接影响线上回答（AI 只检索 published）。
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/domain"
)

// article 是 faq.jsonl 的一行：一条京东帮助中心问答。
type article struct {
	Code    string `json:"code"`
	Group   string `json:"group"`
	Sub     string `json:"sub"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// categoryByGroup 复用库里已有的英文分类键，admin 的筛选条按 category 聚合。
// 细分的二级分类（发货与签收、发票开具规则…）放 keywords 里，不单独建分类，
// 否则 29 个分类会把筛选条撑爆。
var categoryByGroup = map[string]string{
	"配送方式": "logistics",
	"支付问题": "payment",
	"发票问题": "invoice",
	"售后服务": "after_sales",
}

// keywordVocabulary 是检索用的词表：既要政策术语（拒收、红冲），也要用户口语
// （几天到、不能退吧）。SearchKnowledge 是 LIKE 匹配，keywords_json 命中才能补上
// 「标题里没写的同义词」这条召回路径。
var keywordVocabulary = []string{
	// 配送
	"运费", "免运费", "续重", "首重", "包邮", "拒收", "签收", "代收", "驿站", "快递柜",
	"自提", "时效", "次日达", "隔日达", "211", "京准达", "京尊达", "大件", "生鲜", "冷链",
	"丢件", "破损", "派送", "改地址", "改派", "拦截", "二次派送", "配送范围", "偏远地区",
	// 支付
	"支付", "付款", "到付", "在线支付", "快捷支付", "银联", "微信", "支付宝", "白条", "分期",
	"金采", "转账", "支票", "扫码", "企业账户", "余额", "京豆", "优惠券", "礼品卡", "扣款",
	// 发票
	"发票", "开票", "电子发票", "增值税", "专票", "普票", "抬头", "税号", "红冲", "补开",
	"发票类型", "报销",
	// 售后
	"退货", "换货", "退款", "维修", "返修", "上门取件", "7天无理由", "15天", "180天", "三包",
	"质保", "延保", "价保", "拆封", "定制", "闪电退款", "先行赔付", "只换不修", "以旧换新",
	"售后单", "审核", "寄回", "到账",
}

const codePrefix = "JD-"

func main() {
	file := flag.String("file", "docs/jd-faq/faq.jsonl", "待导入的问答文件（JSONL）")
	status := flag.String("status", "draft", "写入状态：draft | published | archived")
	dryRun := flag.Bool("dry-run", false, "只统计，不写库")
	prune := flag.Bool("prune", false, "删除库中已不在文件里的 JD-* 文章")
	flag.Parse()

	if !validStatus(*status) {
		fmt.Fprintf(os.Stderr, "状态非法: %q（只能是 draft / published / archived）\n", *status)
		os.Exit(1)
	}

	items, empty, err := loadArticles(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 %s 失败: %v\n", *file, err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "执行迁移失败: %v\n", err)
		os.Exit(1)
	}

	stats, err := importArticles(ctx, db, items, *status, *dryRun, *prune)
	if err != nil {
		fmt.Fprintf(os.Stderr, "写入失败（已回滚）: %v\n", err)
		os.Exit(1)
	}

	printReport(cfg.DBPath, *file, *status, *dryRun, stats, empty)
}

func validStatus(s string) bool {
	return s == "draft" || s == "published" || s == "archived"
}

// loadArticles 读 JSONL。正文为空的条目不入库：这类页面抓下来只有一个内嵌工具
// 或一个日期，写进去只会污染检索结果，交给人工补。
func loadArticles(path string) (items []article, empty []article, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var a article
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, nil, fmt.Errorf("第 %d 行不是合法 JSON: %w", line, err)
		}
		if a.Code == "" || a.Title == "" || a.Group == "" {
			return nil, nil, fmt.Errorf("第 %d 行缺少 code/title/group", line)
		}
		if strings.TrimSpace(a.Content) == "" {
			empty = append(empty, a)
			continue
		}
		items = append(items, a)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, nil, fmt.Errorf("文件里没有可导入的条目")
	}
	return items, empty, nil
}

type stats struct {
	Inserted int
	Updated  int
	Skipped  int
	Pruned   int
	ByCat    map[string]int
}

func importArticles(ctx context.Context, db *sql.DB, items []article, status string, dryRun, prune bool) (stats, error) {
	st := stats{ByCat: map[string]int{}}

	existing, err := loadExisting(ctx, db)
	if err != nil {
		return st, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return st, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, a := range items {
		category, ok := categoryByGroup[a.Group]
		if !ok {
			return st, fmt.Errorf("%s: 未知的一级分类 %q", a.Code, a.Group)
		}
		title := strings.TrimSpace(a.Title)
		content := strings.TrimSpace(a.Content) + "\n\n原文：" + a.URL
		keywords := strings.Join(keywordsFor(a), "\x00")

		st.ByCat[category]++

		prev, found := existing[a.Code]
		if found && prev.equal(category, title, content, keywords, a.URL, status) {
			st.Skipped++
			continue
		}
		if dryRun {
			if found {
				st.Updated++
			} else {
				st.Inserted++
			}
			continue
		}

		kwJSON, err := json.Marshal(keywordsFor(a))
		if err != nil {
			return st, fmt.Errorf("%s: 编码关键词: %w", a.Code, err)
		}
		if found {
			_, err = tx.ExecContext(ctx, `UPDATE knowledge_articles
				SET category = ?, title = ?, content = ?, keywords_json = ?, status = ?,
				    source = ?, version = version + 1, updated_at = ?
				WHERE article_code = ?`,
				category, title, content, string(kwJSON), status, a.URL, now, a.Code)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO knowledge_articles
				(id, article_code, category, title, content, keywords_json, status, source, version, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
				domain.NewID(), a.Code, category, title, content, string(kwJSON), status, a.URL, now, now)
		}
		if err != nil {
			return st, fmt.Errorf("%s: 写库: %w", a.Code, err)
		}
		if found {
			st.Updated++
		} else {
			st.Inserted++
		}
	}

	if prune && !dryRun {
		keep := make(map[string]bool, len(items))
		for _, a := range items {
			keep[a.Code] = true
		}
		var stale []string
		for code := range existing {
			if !keep[code] {
				stale = append(stale, code)
			}
		}
		sort.Strings(stale)
		for _, code := range stale {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM knowledge_articles WHERE article_code = ?`, code); err != nil {
				return st, fmt.Errorf("清理过期条目 %s: %w", code, err)
			}
			st.Pruned++
		}
	}

	if dryRun {
		return st, nil
	}
	if err := tx.Commit(); err != nil {
		return st, fmt.Errorf("commit: %w", err)
	}
	return st, nil
}

type snapshot struct {
	category, title, content, keywords, source, status string
}

func (s snapshot) equal(category, title, content, keywords, source, status string) bool {
	return s.category == category && s.title == title && s.content == content &&
		s.keywords == keywords && s.source == source && s.status == status
}

// loadExisting 只取本命令管理的 JD-* 行，seed 生成的 KB-* 演示数据完全不碰。
func loadExisting(ctx context.Context, db *sql.DB) (map[string]snapshot, error) {
	rows, err := db.QueryContext(ctx, `SELECT article_code, category, title, content,
		keywords_json, COALESCE(source, ''), status FROM knowledge_articles
		WHERE article_code LIKE ?`, codePrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("读取已有文章: %w", err)
	}
	defer rows.Close()

	out := map[string]snapshot{}
	for rows.Next() {
		var code string
		var s snapshot
		var kwJSON string
		if err := rows.Scan(&code, &s.category, &s.title, &s.content, &kwJSON, &s.source, &s.status); err != nil {
			return nil, err
		}
		// 关键词按 \x00 连接比较，省掉一次 JSON 反序列化。
		s.keywords = strings.Join(jsonStrings(kwJSON), "\x00")
		out[code] = s
	}
	return out, rows.Err()
}

func jsonStrings(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// keywordsFor 产出 3–10 个检索关键词：二级分类名打头，标题命中的词全部保留，
// 正文命中的词只取出现 ≥2 次且频次最高的，避免长条款里偶然提到的词（比如售后
// 细则里顺带一句「发票」）混进关键词。
func keywordsFor(a article) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		if s == "" || seen[s] || len(out) >= 10 {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(strings.TrimSpace(strings.Split(a.Sub, "/")[0])) // 退换货/返修申请 → 退换货

	for _, kw := range keywordVocabulary {
		if strings.Contains(a.Title, kw) {
			add(kw)
		}
	}

	type hit struct {
		kw string
		n  int
	}
	var hits []hit
	for _, kw := range keywordVocabulary {
		if n := strings.Count(a.Content, kw); n >= 2 {
			hits = append(hits, hit{kw, n})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].n > hits[j].n })
	for _, h := range hits {
		add(h.kw)
	}
	return out
}

func printReport(dbPath, file, status string, dryRun bool, st stats, empty []article) {
	mode := "写入"
	if dryRun {
		mode = "预演（未写库）"
	}
	fmt.Printf("京东帮助中心 FAQ 导入%s（db=%s）\n", mode, dbPath)
	fmt.Printf("  数据文件: %s\n", file)
	fmt.Printf("  目标状态: %s\n", status)

	cats := make([]string, 0, len(st.ByCat))
	for c := range st.ByCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	var parts []string
	for _, c := range cats {
		parts = append(parts, fmt.Sprintf("%s=%d", c, st.ByCat[c]))
	}
	fmt.Printf("  分类分布: %s\n", strings.Join(parts, " "))

	fmt.Printf("  新增 %d 条，更新 %d 条，无变化跳过 %d 条", st.Inserted, st.Updated, st.Skipped)
	if st.Pruned > 0 {
		fmt.Printf("，清理 %d 条", st.Pruned)
	}
	fmt.Println()

	if len(empty) > 0 {
		codes := make([]string, 0, len(empty))
		for _, a := range empty {
			codes = append(codes, a.Code)
		}
		fmt.Printf("  跳过空正文 %d 条: %s（需人工补写后再导入）\n", len(empty), strings.Join(codes, ", "))
	}
	if dryRun {
		fmt.Println("  说明: --dry-run 未落库，去掉该参数才会真正写入")
	} else if status == "draft" {
		fmt.Println("  下一步: 在 admin 知识库里抽查无误后，用 -status published 再跑一次即可上线")
	}
}
