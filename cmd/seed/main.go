// Command seed writes the fictional demo dataset. It is idempotent: running it
// twice never duplicates natural keys. --reset truncates business tables and is
// only allowed when APP_ENV is dev or test.
//
// The command prints the shape of what it wrote and the outcome of every
// consistency check; a non-zero exit means nothing was committed.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/seed"
)

func main() {
	reset := flag.Bool("reset", false, "清空演示数据后重新写入（仅 dev/test 可用）")
	artifacts := flag.String("artifacts", "testdata", "评测元数据输出目录（manifest.json + scenarios/），留空则不输出")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}
	if *reset && !cfg.IsDev() {
		fmt.Fprintf(os.Stderr, "拒绝执行: APP_ENV=%s 下不允许 --reset\n", cfg.AppEnv)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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

	seeder := seed.New(db, logger)
	summary, err := seeder.Run(ctx, *reset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "写入演示数据失败: %v\n", err)
		os.Exit(1)
	}

	if *artifacts != "" {
		if err := seeder.ExportArtifacts(*artifacts); err != nil {
			fmt.Fprintf(os.Stderr, "输出评测元数据失败: %v\n", err)
			os.Exit(1)
		}
	}

	printSummary(cfg.DBPath, summary)
	if summary.Skipped {
		fmt.Println("  说明: 库里已有本数据集，本次未重新生成（需要重建请加 -reset）")
		return
	}
	printDistribution(ctx, db)
	fmt.Printf("  一致性检查: %d 项全部通过（内存 %d 项 + 落库 %d 项）\n",
		summary.Checks, summary.DatasetChecks, summary.DatabaseChecks)
	if *artifacts != "" {
		fmt.Printf("  评测元数据: %s/manifest.json, %s/scenarios/test_scenarios.jsonl\n", *artifacts, *artifacts)
	}
	fmt.Println("  下一步: go run ./cmd/server，然后带 X-Demo-Customer-ID: CUST-1001 调用 API")
}

func printSummary(dbPath string, s seed.Summary) {
	fmt.Printf("演示数据就绪（db=%s）\n", dbPath)
	rows := [][2]any{
		{"客户", s.Customers}, {"商品", s.Products}, {"SKU", s.SKUs},
		{"订单", s.Orders}, {"订单行", s.Items}, {"支付", s.Payments},
		{"退款", s.Refunds}, {"包裹", s.Shipments}, {"轨迹", s.Events},
		{"售后单", s.Cases}, {"发票", s.Invoices}, {"活动", s.Promotions},
		{"权益", s.Benefits}, {"知识文章", s.Articles},
		{"会话", s.Conversations}, {"消息", s.Messages}, {"工具日志", s.ToolExecs},
		{"工单", s.Tickets}, {"待确认动作", s.PendingAction}, {"审计", s.Audits},
		{"意图标注", s.Annotations},
	}
	for i := 0; i < len(rows); i += 4 {
		var b strings.Builder
		b.WriteString("  ")
		for j := i; j < i+4 && j < len(rows); j++ {
			fmt.Fprintf(&b, "%-8s%-6d", rows[j][0], rows[j][1])
		}
		fmt.Println(strings.TrimRight(b.String(), " "))
	}
}

// printDistribution queries the database instead of the in-memory dataset: the
// operator sees what is actually stored, not what the generator intended.
func printDistribution(ctx context.Context, db *sql.DB) {
	groups := []struct {
		title string
		sql   string
	}{
		{"订单状态", `SELECT order_status, COUNT(1) FROM orders GROUP BY order_status`},
		{"支付状态", `SELECT payment_status, COUNT(1) FROM payments GROUP BY payment_status`},
		{"退款状态", `SELECT refund_status, COUNT(1) FROM refunds GROUP BY refund_status`},
		{"包裹状态", `SELECT shipment_status, COUNT(1) FROM shipments GROUP BY shipment_status`},
		{"售后状态", `SELECT case_status, COUNT(1) FROM service_cases GROUP BY case_status`},
		{"发票状态", `SELECT invoice_status, COUNT(1) FROM invoices GROUP BY invoice_status`},
		{"会话状态", `SELECT status, COUNT(1) FROM conversations GROUP BY status`},
	}
	for _, g := range groups {
		counts, err := queryCounts(ctx, db, g.sql)
		if err != nil {
			fmt.Fprintf(os.Stderr, "统计「%s」失败: %v\n", g.title, err)
			continue
		}
		fmt.Printf("  %s: %s\n", g.title, counts)
	}
}

func queryCounts(ctx context.Context, db *sql.DB, query string) (string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	pairs := map[string]int{}
	var keys []string
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return "", err
		}
		pairs[k] = n
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, pairs[k]))
	}
	return strings.Join(parts, " "), nil
}
