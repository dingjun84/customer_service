package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// writeArtifacts writes the evaluation metadata that must NOT live in SQLite:
// a manifest describing the dataset and one JSONL line per scenario. Keeping the
// expectations out of the database means a prompt cannot accidentally read the
// answer it is being graded on.
func writeArtifacts(dir string, g *gen) error {
	if err := os.MkdirAll(filepath.Join(dir, "scenarios"), 0o755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}

	counts := summaryOf(g.ds)
	byCategory := map[string]int{}
	byRisk := map[string]int{}
	bySplit := map[string]int{}
	for _, sc := range g.ds.scenarios {
		byCategory[sc.Category]++
		byRisk[sc.RiskLevel]++
		bySplit[sc.Split]++
	}

	manifest := map[string]any{
		"dataset":     "customer-service-demo",
		"generator":   "internal/seed",
		"seed":        Seed,
		"anchor":      ts(g.anchor),
		"currency":    "CNY",
		"amount_unit": "minor",
		"synthetic":   true,
		"privacy": map[string]any{
			"emails":     "@example.test",
			"phones":     "1380000xxxx（测试号段）",
			"addresses":  "测试省/测试市/模拟路（虚构地址）",
			"names":      "虚构姓名",
			"payment":    "仅保留 mock 交易号，无真实凭证",
			"id_numbers": "不生成身份证号",
		},
		"counts": map[string]int{
			"customers": counts.Customers, "products": counts.Products, "skus": counts.SKUs,
			"orders": counts.Orders, "order_items": counts.Items, "payments": counts.Payments,
			"refunds": counts.Refunds, "shipments": counts.Shipments, "shipment_events": counts.Events,
			"service_cases": counts.Cases, "invoices": counts.Invoices,
			"promotions": counts.Promotions, "customer_benefits": counts.Benefits,
			"knowledge_articles": counts.Articles, "conversations": counts.Conversations,
			"messages": counts.Messages, "tool_executions": counts.ToolExecs,
			"service_tickets": counts.Tickets, "pending_actions": counts.PendingAction,
			"action_audit_logs": counts.Audits, "message_annotations": counts.Annotations,
		},
		"scenarios": map[string]any{
			"total":       len(g.ds.scenarios),
			"by_category": byCategory,
			"by_risk":     byRisk,
			"by_split":    bySplit,
		},
		"invariants": []string{
			"订单实付 = 成功支付合计；已退 = 成功退款合计；退款 ≤ 实付",
			"售后单客户与商品行必须属于同一订单",
			"包裹状态与最后一条轨迹一致，轨迹不晚于数据锚点",
			"已完成订单必有签收轨迹；已取消订单无在途或签收包裹",
			"发票金额 ≤ 实付；整单退款后发票必须红冲",
			"工具失败必有 error_code，成功必与消息中的结果逐字一致",
			"会话消息序号严格递增，工具调用与执行日志一一对应",
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	scenarios := append([]scenarioRow{}, g.ds.scenarios...)
	sort.SliceStable(scenarios, func(i, j int) bool {
		return scenarios[i].ScenarioID < scenarios[j].ScenarioID
	})
	f, err := os.Create(filepath.Join(dir, "scenarios", "test_scenarios.jsonl"))
	if err != nil {
		return fmt.Errorf("create scenario file: %w", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, sc := range scenarios {
		if err := enc.Encode(sc); err != nil {
			return fmt.Errorf("write scenario %s: %w", sc.ScenarioID, err)
		}
	}
	return nil
}
