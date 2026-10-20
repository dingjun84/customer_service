package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func planFor(t *testing.T, text string) ModelResponse {
	t.Helper()
	model := NewOfflineModel()
	resp, err := model.Respond(context.Background(), ModelRequest{
		Messages: []Message{{Role: RoleUser, Content: text}},
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	return resp
}

func firstCall(t *testing.T, resp ModelResponse) (string, map[string]any) {
	t.Helper()
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected exactly one tool call, got %d (content=%q)", len(resp.ToolCalls), resp.Content)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(resp.ToolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("decode arguments: %v", err)
	}
	return resp.ToolCalls[0].Name, args
}

func TestOfflineModelPlansCancellation(t *testing.T) {
	name, args := firstCall(t, planFor(t, "我要取消 ORD-10002"))
	if name != "request_order_cancellation" {
		t.Fatalf("tool = %s, want request_order_cancellation", name)
	}
	if args["order_no"] != "ORD-10002" {
		t.Fatalf("order_no = %v", args["order_no"])
	}
}

func TestOfflineModelNormalizesOrderNumberCase(t *testing.T) {
	_, args := firstCall(t, planFor(t, "查一下 ord-10003 的物流"))
	if args["order_no"] != "ORD-10003" {
		t.Fatalf("order_no = %v, want ORD-10003", args["order_no"])
	}
}

func TestOfflineModelAsksForOrderInsteadOfGuessing(t *testing.T) {
	name, _ := firstCall(t, planFor(t, "我要退货"))
	if name != "list_my_orders" {
		t.Fatalf("tool = %s, want list_my_orders (must not guess an order)", name)
	}
}

func TestOfflineModelExtractsModifyChanges(t *testing.T) {
	name, args := firstCall(t, planFor(t, "把 ORD-10001 的地址改成 深圳市南山区科技园南路 99 号"))
	if name != "modify_order" {
		t.Fatalf("tool = %s, want modify_order", name)
	}
	changes, ok := args["changes"].(map[string]any)
	if !ok {
		t.Fatalf("changes missing: %v", args)
	}
	if !strings.Contains(changes["shipping_address"].(string), "科技园南路 99 号") {
		t.Fatalf("shipping_address = %v", changes["shipping_address"])
	}
}

func TestOfflineModelReadsOrderBeforeAfterSales(t *testing.T) {
	// A service request must read the order first: multi-item orders need an item.
	name, _ := firstCall(t, planFor(t, "ORD-10006 我要退货"))
	if name != "get_order_detail" {
		t.Fatalf("tool = %s, want get_order_detail", name)
	}
}

func TestOfflineModelAsksWhichItemOnMultiItemOrder(t *testing.T) {
	model := NewOfflineModel()
	toolResult := `{"ok":true,"data":{"order_no":"ORD-10006","items":[
		{"order_item_id":"itm_1","product_name":"咖啡机","sku_name":"白","paid":{"amount_display":"¥1299.00"}},
		{"order_item_id":"itm_2","product_name":"咖啡豆","sku_name":"袋装","paid":{"amount_display":"¥89.00"}}]}}`

	resp, err := model.Respond(context.Background(), ModelRequest{Messages: []Message{
		{Role: RoleUser, Content: "我要给 ORD-10006 申请退货"},
		{Role: RoleAssistant, Content: ""},
		{Role: RoleTool, ToolName: "get_order_detail", Content: toolResult},
	}})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("must ask the user instead of guessing an item, got %d calls", len(resp.ToolCalls))
	}
	if !strings.Contains(resp.Content, "需要你指定") {
		t.Fatalf("content = %q", resp.Content)
	}
}

func TestOfflineModelContinuesAfterItemSelection(t *testing.T) {
	model := NewOfflineModel()
	toolResult := `{"ok":true,"data":{"order_no":"ORD-10006","items":[
		{"order_item_id":"itm_1","product_name":"咖啡机","sku_name":"白","paid":{"amount_display":"¥1299.00"}},
		{"order_item_id":"itm_2","product_name":"咖啡豆","sku_name":"袋装","paid":{"amount_display":"¥89.00"}}]}}`

	resp, err := model.Respond(context.Background(), ModelRequest{Messages: []Message{
		{Role: RoleUser, Content: "我要给 ORD-10006 申请退货"},
		{Role: RoleAssistant, Content: "订单 ORD-10006 里有 2 个商品，需要你指定退货哪一个：\n\n1. 咖啡机\n2. 咖啡豆\n\n可以说「第一个」或直接报商品名。"},
		{Role: RoleUser, Content: "第二个"},
		{Role: RoleAssistant, Content: ""},
		{Role: RoleTool, ToolName: "get_order_detail", Content: toolResult},
	}})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	name, args := firstCall(t, resp)
	if name != "submit_service_request" {
		t.Fatalf("tool = %s, want submit_service_request", name)
	}
	if args["order_item_id"] != "itm_2" {
		t.Fatalf("order_item_id = %v, want itm_2", args["order_item_id"])
	}
	if args["case_type"] != "return" {
		t.Fatalf("case_type = %v, want return", args["case_type"])
	}
}

func TestOfflineModelRendersFactsFromToolResult(t *testing.T) {
	model := NewOfflineModel()
	content := `{"ok":true,"data":{"order_no":"ORD-10002","status":"paid",
		"amounts":{"paid":{"amount_display":"¥499.00"},"refunded":{"amount_display":"¥0.00"}},
		"receiver":{"name":"张三","phone":"138****1234","region":"广东省 深圳市","address":"科技园南路 88 号"},
		"items":[{"product_name":"咖啡机","sku_name":"曜石黑","quantity":1,"paid":{"amount_display":"¥499.00"}}]}}`

	resp, err := model.Respond(context.Background(), ModelRequest{Messages: []Message{
		{Role: RoleUser, Content: "查一下 ORD-10002"},
		{Role: RoleTool, ToolName: "get_order_detail", Content: content},
	}})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	for _, want := range []string{"ORD-10002", "已付款待发货", "¥499.00", "138****1234"} {
		if !strings.Contains(resp.Content, want) {
			t.Fatalf("content missing %q: %q", want, resp.Content)
		}
	}
	if strings.Contains(resp.Content, "已取消") {
		t.Fatalf("must not claim a state that the tool did not return: %q", resp.Content)
	}
}

func TestOfflineModelExplainsToolError(t *testing.T) {
	model := NewOfflineModel()
	content := `{"ok":false,"error":{"code":"forbidden","message":"该订单不属于当前客户"}}`

	resp, _ := model.Respond(context.Background(), ModelRequest{Messages: []Message{
		{Role: RoleUser, Content: "查一下 ORD-20001"},
		{Role: RoleTool, ToolName: "get_order_detail", Content: content},
	}})
	if !strings.Contains(resp.Content, "不属于当前演示身份") {
		t.Fatalf("content = %q", resp.Content)
	}
}
