package seed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func buildForTest(t *testing.T) *gen {
	t.Helper()
	g := newGen(DataAnchor)
	g.build()
	if len(g.errs) > 0 {
		t.Fatalf("生成阶段报错：\n%s", strings.Join(g.errs, "\n"))
	}
	return g
}

// TestDatasetMatchesSpecification pins the sizes the specification asks for, so
// a later tweak to a story cannot silently shrink coverage.
func TestDatasetMatchesSpecification(t *testing.T) {
	g := buildForTest(t)
	s := summaryOf(g.ds)

	ranges := []struct {
		name     string
		got      int
		min, max int
	}{
		{"商品", s.Products, 72, 72},
		{"SKU", s.SKUs, 110, 150},
		{"用户", s.Customers, 20, 20},
		{"订单", s.Orders, 100, 140},
		{"订单行", s.Items, 160, 220},
		{"退款", s.Refunds, 25, 40},
		{"包裹", s.Shipments, 80, 120},
		{"物流轨迹", s.Events, 500, 800},
		{"售后单", s.Cases, 35, 55},
		{"发票", s.Invoices, 20, 35},
		{"活动", s.Promotions, 8, 15},
		{"知识文章", s.Articles, 25, 40},
		{"会话", s.Conversations, 60, 90},
		{"消息", s.Messages, 450, 800},
		{"工具日志", s.ToolExecs, 150, 300},
		{"人工工单", s.Tickets, 10, 20},
	}
	for _, r := range ranges {
		if r.got < r.min || r.got > r.max {
			t.Errorf("%s 数量 %d 不在规范区间 [%d, %d]", r.name, r.got, r.min, r.max)
		}
	}

	// Every one of the 12 first-level intents needs at least 5 conversations, and
	// the high-risk intents need the extra safety coverage the spec demands.
	perCategory := map[string]int{}
	for _, c := range g.ds.scenarios {
		perCategory[c.Category]++
	}
	if len(perCategory) != 12 {
		t.Errorf("一级意图覆盖 %d 类，期望 12 类：%v", len(perCategory), perCategory)
	}
	for cat, n := range perCategory {
		if n < 5 {
			t.Errorf("一级意图「%s」只有 %d 个会话", cat, n)
		}
	}

	multi, highRisk := 0, 0
	for _, c := range g.ds.scenarios {
		if len(c.Turns) > 2 {
			multi++
		}
		if c.RiskLevel == "high" || c.RiskLevel == "critical" {
			highRisk++
		}
	}
	if multi < 20 {
		t.Errorf("多轮会话 %d 个，少于 20 个", multi)
	}
	if highRisk < 12 {
		t.Errorf("高风险/对抗场景 %d 个，少于 12 个", highRisk)
	}

	// Distribution sanity: the interesting states must actually exist.
	states := map[string]int{}
	for _, o := range g.ds.orders {
		states[o.Status]++
	}
	for _, st := range []string{"pending_payment", "paid", "processing", "shipped", "completed", "cancelled", "closed"} {
		if states[st] == 0 {
			t.Errorf("订单状态 %s 一个都没有", st)
		}
	}
	refundStates := map[string]int{}
	for _, r := range g.ds.refunds {
		refundStates[r.Status]++
	}
	for _, st := range []string{"requested", "pending_review", "approved", "processing", "succeeded", "failed", "rejected"} {
		if refundStates[st] == 0 {
			t.Errorf("退款状态 %s 一个都没有", st)
		}
	}
}

// TestGeneratedDatasetIsConsistent runs the same validator the import uses.
func TestGeneratedDatasetIsConsistent(t *testing.T) {
	g := buildForTest(t)
	report := validateDataset(g.ds, DataAnchor)
	if report.Violations > 0 {
		t.Fatalf("校验发现 %d 处不一致：\n%s", report.Violations, strings.Join(report.Messages(), "\n"))
	}
	if report.Checks < 1000 {
		t.Fatalf("只执行了 %d 项检查，校验器可能没跑全", report.Checks)
	}
}

// TestValidationRejectsContradictions feeds the validator deliberately broken
// rows: if any of these slips through, the import would ship a contradiction.
func TestValidationRejectsContradictions(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(g *gen)
		want   string
	}{
		{
			name: "退款金额超过实付",
			mutate: func(g *gen) {
				for i := range g.ds.refunds {
					if g.ds.refunds[i].Status == "succeeded" {
						g.ds.refunds[i].Amount = g.ds.refunds[i].Amount*10 + 100000
						return
					}
				}
			},
			want: "退款",
		},
		{
			name: "签收后还在派送",
			mutate: func(g *gen) {
				for i := range g.ds.shipments {
					sh := &g.ds.shipments[i]
					if sh.Status == "delivered" {
						sh.Status = "delivering"
						return
					}
				}
			},
			want: "包裹",
		},
		{
			name: "售后单引用其他订单的商品行",
			mutate: func(g *gen) {
				var foreign string
				for i := range g.ds.items {
					if g.ds.items[i].OrderID != g.ds.cases[0].OrderID {
						foreign = g.ds.items[i].ID
						break
					}
				}
				g.ds.cases[0].OrderItemID = foreign
			},
			want: "商品行",
		},
		{
			name: "发票金额超过实付",
			mutate: func(g *gen) {
				g.ds.invoices[0].Amount += 500000
			},
			want: "发票",
		},
		{
			name: "工具执行标记成功但结果是失败响应",
			mutate: func(g *gen) {
				for i := range g.ds.toolExecs {
					if g.ds.toolExecs[i].Status == "succeeded" {
						g.ds.toolExecs[i].Output = `{"ok":false,"error":{"code":"conflict","message":"x"}}`
						return
					}
				}
			},
			want: "工具执行",
		},
		{
			name: "消息时间在未来",
			mutate: func(g *gen) {
				last := &g.ds.messages[len(g.ds.messages)-1]
				last.CreatedAt = DataAnchor.Add(48 * 3600 * 1e9)
			},
			want: "未来",
		},
		{
			name: "整单退款后发票仍未红冲",
			mutate: func(g *gen) {
				for i := range g.ds.orders {
					o := &g.ds.orders[i]
					if o.Paid > 0 && o.Refunded == o.Paid {
						for j := range g.ds.invoices {
							if g.ds.invoices[j].OrderID == o.ID && g.ds.invoices[j].Status == "issued" {
								// keep the invoice issued: this is the contradiction
								return
							}
						}
					}
				}
				// fabricate one instead of skipping the case
				for i := range g.ds.orders {
					o := &g.ds.orders[i]
					if o.Paid > 0 && o.Refunded == o.Paid {
						g.ds.invoices = append(g.ds.invoices, invoiceRow{
							ID: "INV-BAD", CustomerID: o.CustomerID, OrderID: o.ID,
							Type: "electronic", Status: "issued", Title: "测试", Amount: 100,
							RequestedAt: o.CreatedAt, CreatedAt: o.CreatedAt, UpdatedAt: o.CreatedAt,
						})
						return
					}
				}
			},
			want: "发票",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := buildForTest(t)
			tc.mutate(g)
			report := validateDataset(g.ds, DataAnchor)
			if report.Violations == 0 {
				t.Fatalf("构造的矛盾没有被校验器发现：%s", tc.name)
			}
			joined := strings.Join(report.Messages(), "\n")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("校验报了，但信息里没有「%s」：\n%s", tc.want, joined)
			}
		})
	}
}

// TestDeterministicOutput guarantees two runs write byte-identical business
// keys, which is what makes the dataset reviewable and diffable.
func TestDeterministicOutput(t *testing.T) {
	digest := func() string {
		g := newGen(DataAnchor)
		g.build()
		if len(g.errs) > 0 {
			t.Fatalf("build errors: %v", g.errs)
		}
		var parts []string
		for _, o := range g.ds.orders {
			parts = append(parts, fmt.Sprintf("%s|%s|%d|%d|%s", o.No, o.ID, o.Paid, o.Refunded, o.Status))
		}
		for _, it := range g.ds.items {
			parts = append(parts, fmt.Sprintf("%s|%s|%d|%d", it.ID, it.OrderID, it.Quantity, it.Paid))
		}
		sort.Strings(parts)
		sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
		return hex.EncodeToString(sum[:])
	}
	if a, b := digest(), digest(); a != b {
		t.Fatalf("两次生成结果不一致：%s != %s", a, b)
	}
}

// TestPlaceholderGuard makes sure a template that asks for a field the order
// cannot provide is rejected instead of rendering an empty sentence.
func TestPlaceholderGuard(t *testing.T) {
	ctx := map[string]any{"order_no": "ORD-10001", "paid": "¥10.00"}
	tpl := convoTemplate{
		turns: []convoTurn{{user: "订单{{order_no}}", answer: "退款单号{{refund_no}}"}},
	}
	miss := missingFields(tpl, ctx)
	if len(miss) != 1 || miss[0] != "refund_no" {
		t.Fatalf("期望只发现 refund_no 缺失，实际 %v", miss)
	}
	got := fillPlaceholders("订单{{order_no}} 实付 {{paid}}，退 {{refund_no}}", ctx)
	if !strings.Contains(got, "ORD-10001") || !strings.Contains(got, "¥10.00") {
		t.Fatalf("占位符没有被替换：%s", got)
	}
}
