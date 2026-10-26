package seed

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// IMScenarioFile 是人工改写自 duty-im 候选池的场景文件，相对 artifacts 根目录。
// 它不参与 seed 的随机生成，但会在写评测元数据时合并进 test_scenarios.jsonl，
// 否则一次 make seed-reset 就会把这批场景静默冲掉。
const IMScenarioFile = "im_export/scenarios_im.jsonl"

// 12 个一级意图的中短名，与现有 89 条场景保持同一套命名，不混用附录里的长名。
var imCategories = map[string]bool{
	"物流配送": true, "订单修改": true, "退换货保修": true, "退款业务": true,
	"发票业务": true, "支付金融": true, "商品售前咨询": true, "商品质量故障": true,
	"优惠价格活动": true, "补发催件": true, "人工客服接入": true, "投诉与闲聊情绪": true,
}

// 工具白名单，与实施方案 §3.4 一致。
var imToolWhitelist = map[string]bool{
	"search_knowledge": true, "search_products": true, "get_product_detail": true,
	"list_my_orders": true, "get_order_detail": true, "get_logistics_status": true,
	"get_payment_refund_status": true, "get_service_status": true, "get_invoice_status": true,
	"get_customer_benefits_status": true, "get_promotion_rules": true,
	"check_action_eligibility": true, "request_order_cancellation": true, "modify_order": true,
	"submit_service_request": true, "create_complaint_ticket": true, "transfer_to_human": true,
}

var (
	imIDPattern     = regexp.MustCompile(`^SCN_IM_[A-Z]{2}_\d{3}$`)
	imCustomerNo    = regexp.MustCompile(`^CUST-2\d{3}$`)
	imConversationN = regexp.MustCompile(`^CONV-9\d{4}$`)
	imOrderNo       = regexp.MustCompile(`^ORD-2\d{4}$`)
	imPhoneLeak     = regexp.MustCompile(`(?:^|[^\d])1[3-9]\d{9}(?:[^\d]|$)`)
)

// loadIMScenarios 读取并校验 IM 场景。文件不存在时返回空切片：没有这批数据
// 的仓库照样能跑 seed，但一旦文件在，任何一条不合格都直接失败（fail closed），
// 避免坏场景混进评测集。
func loadIMScenarios(dir string) ([]scenarioRow, error) {
	path := filepath.Join(dir, filepath.FromSlash(IMScenarioFile))
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 IM 场景: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []scenarioRow
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var row scenarioRow
		if err := json.Unmarshal([]byte(raw), &row); err != nil {
			return nil, fmt.Errorf("%s 第 %d 行 JSON 解析失败: %w", IMScenarioFile, line, err)
		}
		if err := validateIMScenario(row); err != nil {
			return nil, fmt.Errorf("%s 第 %d 行（%s）: %w", IMScenarioFile, line, row.ScenarioID, err)
		}
		if seen[row.ScenarioID] {
			return nil, fmt.Errorf("%s 第 %d 行: scenario_id %s 重复", IMScenarioFile, line, row.ScenarioID)
		}
		seen[row.ScenarioID] = true
		out = append(out, row)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("读取 IM 场景: %w", err)
	}
	return out, nil
}

// validateIMScenario 是这批场景的唯一门禁：字段、枚举、工具白名单、轮次结构、
// PII 都在这里拦。它不判断话术好坏，那是人工评审的事。
func validateIMScenario(sc scenarioRow) error {
	if !imIDPattern.MatchString(sc.ScenarioID) {
		return fmt.Errorf("scenario_id 需形如 SCN_IM_LG_200，实际 %q", sc.ScenarioID)
	}
	if !imCategories[sc.Category] {
		return fmt.Errorf("category %q 不在 12 类里", sc.Category)
	}
	if len(sc.IntentTags) == 0 {
		return fmt.Errorf("intent_tags 为空")
	}
	if !imCustomerNo.MatchString(sc.CustomerNo) {
		return fmt.Errorf("customer_no 需形如 CUST-2xxx，实际 %q", sc.CustomerNo)
	}
	if !imConversationN.MatchString(sc.ConversationNo) {
		return fmt.Errorf("conversation_id 需形如 CONV-9xxxx，实际 %q", sc.ConversationNo)
	}
	if sc.Split != "dev" && sc.Split != "holdout" {
		return fmt.Errorf("split 只能是 dev/holdout，实际 %q", sc.Split)
	}
	switch sc.RiskLevel {
	case "low", "normal", "high", "critical":
	default:
		return fmt.Errorf("risk_level 非法: %q", sc.RiskLevel)
	}
	if n := len(sc.Turns); n < 2 || n > 6 || n%2 != 0 {
		return fmt.Errorf("turns 需 2~6 条且以 assistant 结尾，实际 %d 条", n)
	}
	for i, t := range sc.Turns {
		want := "user"
		if i%2 == 1 {
			want = "assistant_expected"
		}
		if t.Role != want {
			return fmt.Errorf("turns[%d] 角色应为 %s，实际 %q", i, want, t.Role)
		}
		if strings.TrimSpace(t.Content) == "" {
			return fmt.Errorf("turns[%d] 内容为空", i)
		}
		if imPhoneLeak.MatchString(t.Content) || strings.Contains(t.Content, "http://") ||
			strings.Contains(t.Content, "https://") {
			return fmt.Errorf("turns[%d] 含手机号或链接", i)
		}
	}
	for _, k := range []string{"order_no", "order_status", "paid_amount_minor", "refunded_amount_minor"} {
		if _, ok := sc.Fixture[k]; !ok {
			return fmt.Errorf("fixtures 缺 %s", k)
		}
	}
	if no, _ := sc.Fixture["order_no"].(string); !imOrderNo.MatchString(no) {
		return fmt.Errorf("fixtures.order_no 需形如 ORD-2xxxx，实际 %v", sc.Fixture["order_no"])
	}
	if len(sc.ExpectedTools) == 0 {
		return fmt.Errorf("expected_tool_calls 不能为空")
	}
	for _, call := range sc.ExpectedTools {
		name, _ := call["tool"].(string)
		if !imToolWhitelist[name] {
			return fmt.Errorf("工具 %q 不在白名单内", name)
		}
		if _, ok := call["required"]; !ok {
			return fmt.Errorf("工具 %q 缺 required 字段", name)
		}
	}
	if strings.TrimSpace(sc.ExpectedOutcome) == "" {
		return fmt.Errorf("expected_outcome 为空")
	}
	if len(sc.PassCriteria) == 0 {
		return fmt.Errorf("pass_criteria 不能为空")
	}
	return nil
}
