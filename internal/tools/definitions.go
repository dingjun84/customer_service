// Package tools defines the tool catalog exposed to the LLM and dispatches tool
// calls into application services.
//
// The model can only see JSON-Schema-described arguments. It can never issue SQL,
// choose another customer, or call an unregistered tool.
package tools

import (
	"encoding/json"

	"github.com/example/ecommerce-support/internal/domain"
)

// Definition is one tool description in a provider-neutral shape.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// OpenAITool renders the definition in the OpenAI-compatible "tools" format.
func (d Definition) OpenAITool() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        d.Name,
			"description": d.Description,
			"parameters":  d.Parameters,
		},
	}
}

// Tool names, kept as constants so tests and docs cannot drift.
const (
	ToolSearchKnowledge       = "search_knowledge"
	ToolSearchProducts        = "search_products"
	ToolGetProductDetail      = "get_product_detail"
	ToolListMyOrders          = "list_my_orders"
	ToolGetOrderDetail        = "get_order_detail"
	ToolGetLogisticsStatus    = "get_logistics_status"
	ToolGetPaymentRefund      = "get_payment_refund_status"
	ToolGetServiceStatus      = "get_service_status"
	ToolGetInvoiceStatus      = "get_invoice_status"
	ToolGetBenefitsStatus     = "get_customer_benefits_status"
	ToolGetPromotionRules     = "get_promotion_rules"
	ToolCheckEligibility      = "check_action_eligibility"
	ToolRequestCancellation   = "request_order_cancellation"
	ToolModifyOrder           = "modify_order"
	ToolSubmitServiceRequest  = "submit_service_request"
	ToolCreateComplaintTicket = "create_complaint_ticket"
	ToolTransferToHuman       = "transfer_to_human"
)

// Definitions returns the MVP tool catalog in a stable order.
func Definitions() []Definition {
	return []Definition{
		{
			Name: ToolSearchKnowledge,
			Description: "搜索已发布的客服政策文章（退换货规则、发票规则、运费说明等）。" +
				"只用于解释政策，不能替代订单/物流/退款的实时查询结果。",
			Parameters: obj(map[string]any{
				"query":    strProp("政策关键词，例如 退货期限、发票、运费险"),
				"category": strProp("可选的文章分类，例如 after_sales、invoice"),
				"limit":    intProp("返回条数，默认 3，最大 10", 1, 10),
			}, "query"),
		},
		{
			Name: ToolSearchProducts,
			Description: "按关键词搜索在售商品，返回商品编码、名称、品牌、分类和简要属性。" +
				"用户问商品规格、材质、价格区间时先搜索商品，再用 get_product_detail 获取 SKU。",
			Parameters: obj(map[string]any{
				"query":    strProp("商品关键词，例如 咖啡机、空气炸锅"),
				"category": strProp("可选的商品分类"),
				"limit":    intProp("返回条数，默认 5，最大 20", 1, 20),
			}),
		},
		{
			Name: ToolGetProductDetail,
			Description: "获取单个商品的详细参数与全部 SKU（规格、库存、标价）。" +
				"回答尺寸、材质、容量等问题必须使用该工具返回的字段，不要编造。",
			Parameters: obj(map[string]any{
				"product_id":   strProp("商品 UUID"),
				"product_code": strProp("商品业务编码，例如 P-1001（与 product_id 二选一）"),
			}),
		},
		{
			Name: ToolListMyOrders,
			Description: "列出当前客户的订单。客户身份由服务端注入，不能查询他人订单。" +
				"用户没有给出订单号且可能有多个订单时，用该工具确认目标订单，再追问用户确认。",
			Parameters: obj(map[string]any{
				"status": strProp("可选的订单状态过滤：pending_payment/paid/processing/shipped/completed/cancelled/closed"),
				"limit":  intProp("返回条数，默认 5，最大 20", 1, 20),
			}),
		},
		{
			Name: ToolGetOrderDetail,
			Description: "查询当前客户单个订单的详情：状态、金额、收件信息、商品明细。" +
				"订单号来自用户或 list_my_orders 的结果；不得猜测订单号。",
			Parameters: obj(map[string]any{
				"order_no": strProp("订单号，例如 ORD-10001"),
			}, "order_no"),
		},
		{
			Name:        ToolGetLogisticsStatus,
			Description: "查询订单的物流包裹与轨迹节点。必须按服务端返回的节点回答，不要虚构预计送达时间或物流节点。",
			Parameters: obj(map[string]any{
				"order_no":    strProp("订单号"),
				"tracking_no": strProp("运单号（与 order_no 二选一）"),
			}),
		},
		{
			Name: ToolGetPaymentRefund,
			Description: "查询订单的支付与退款状态、金额和时间。退款是否到账以该工具返回的 refund_status 为准，" +
				"不要因为用户说“已经退了”就认定退款成功。",
			Parameters: obj(map[string]any{
				"order_no":  strProp("订单号"),
				"refund_no": strProp("可选的退款单号"),
			}, "order_no"),
		},
		{
			Name:        ToolGetServiceStatus,
			Description: "查询退换货、维修、补发、价保等售后申请的处理状态。",
			Parameters: obj(map[string]any{
				"order_no": strProp("订单号"),
				"case_no":  strProp("可选的售后单号"),
			}),
		},
		{
			Name:        ToolGetInvoiceStatus,
			Description: "查询订单的发票类型、开票状态与开票时间。演示环境不提供真实发票文件。",
			Parameters: obj(map[string]any{
				"order_no":   strProp("订单号"),
				"invoice_no": strProp("可选的发票号"),
			}, "order_no"),
		},
		{
			Name:        ToolGetBenefitsStatus,
			Description: "查询当前客户的优惠券、积分、运费险、白条等权益的模拟状态。",
			Parameters: obj(map[string]any{
				"benefit_type": enumProp("权益类型", "coupon", "points", "freight_insurance", "whitebar_status", "store_balance", "other"),
				"limit":        intProp("返回条数，默认 10，最大 30", 1, 30),
			}),
		},
		{
			Name:        ToolGetPromotionRules,
			Description: "查询进行中的活动规则、起止时间与价保差额参考。回答“为什么没有优惠”“现在有什么活动”时使用。",
			Parameters: obj(map[string]any{
				"query":          strProp("活动关键词"),
				"promotion_code": strProp("活动编码"),
				"limit":          intProp("返回条数，默认 5，最大 10", 1, 10),
			}),
		},
		{
			Name: ToolCheckEligibility,
			Description: "检查某操作当前是否可执行（取消、改地址、退货、换货、维修、退款、补发、价保）。" +
				"返回 eligible、原因码和可操作范围。eligible=false 时必须如实解释原因或转人工，" +
				"不得跳过该检查直接调用写工具。",
			Parameters: obj(map[string]any{
				"action_type":   enumProp("操作类型", domain.SupportedActionTypes...),
				"order_no":      strProp("订单号"),
				"order_item_id": strProp("可选的订单商品 ID；多商品订单申请售后时必须提供"),
				"quantity":      intProp("申请数量，默认 1", 1, 999),
				"changes":       changesSchema(),
				"reason":        strProp("用户给出的原因"),
			}, "action_type", "order_no"),
		},
		{
			Name: ToolRequestCancellation,
			Description: "为未发货订单发起取消申请。资格通过后只会创建【待确认操作】，不会立刻取消订单；" +
				"必须让用户在前端确认按钮上确认，不要声称订单已取消。",
			Parameters: obj(map[string]any{
				"order_no": strProp("订单号"),
				"reason":   strProp("取消原因"),
			}, "order_no"),
		},
		{
			Name: ToolModifyOrder,
			Description: "修改未发货订单的收件信息（仅白名单字段）。会创建【待确认操作】，确认后才写入数据库。" +
				"不能修改金额、支付状态、订单状态、客户或商品。",
			Parameters: obj(map[string]any{
				"order_no": strProp("订单号"),
				"changes":  changesSchema(),
				"reason":   strProp("修改原因"),
			}, "order_no", "changes"),
		},
		{
			Name: ToolSubmitServiceRequest,
			Description: "提交退货/换货/维修/退款/补发/价保申请。资格通过后创建【待确认操作】，" +
				"确认后才生成售后单（退款同时生成退款记录）。不得声称已受理。",
			Parameters: obj(map[string]any{
				"case_type":     enumProp("售后类型", domain.CaseTypeReturn, domain.CaseTypeExchange, domain.CaseTypeRepair, domain.CaseTypeRefund, domain.CaseTypeReship, domain.CaseTypePriceProtection),
				"order_no":      strProp("订单号"),
				"order_item_id": strProp("订单商品 ID；多商品订单必须提供"),
				"quantity":      intProp("申请数量，默认 1", 1, 999),
				"reason":        strProp("申请原因代码或简述"),
				"description":   strProp("补充说明"),
			}, "case_type", "order_no"),
		},
		{
			Name:        ToolCreateComplaintTicket,
			Description: "用户明确投诉或反馈质量问题时创建工单。该工具会立即写入工单，无需再次确认。",
			Parameters: obj(map[string]any{
				"summary":  strProp("问题摘要，不超过 200 字"),
				"details":  strProp("详细描述"),
				"order_no": strProp("可选的相关订单号"),
				"priority": enumProp("优先级", domain.PriorityLow, domain.PriorityNormal, domain.PriorityHigh, domain.PriorityUrgent),
			}, "summary"),
		},
		{
			Name:        ToolTransferToHuman,
			Description: "转人工客服。当用户明确要求人工、情绪激烈、或工具无法解决时使用；会把会话标记为已转接。",
			Parameters: obj(map[string]any{
				"reason":   strProp("转人工原因"),
				"summary":  strProp("会话摘要，供人工客服快速了解上下文"),
				"priority": enumProp("优先级", domain.PriorityLow, domain.PriorityNormal, domain.PriorityHigh, domain.PriorityUrgent),
			}, "reason"),
		},
	}
}

// ---- schema builders ----

func obj(props map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		req := make([]any, 0, len(required))
		for _, r := range required {
			req = append(req, r)
		}
		schema["required"] = req
	}
	return schema
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "maxLength": 500}
}

func intProp(desc string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "minimum": min, "maximum": max}
}

func enumProp(desc string, values ...string) map[string]any {
	enum := make([]any, 0, len(values))
	for _, v := range values {
		enum = append(enum, v)
	}
	return map[string]any{"type": "string", "description": desc, "enum": enum}
}

// changesSchema is the whitelist of modifiable order fields. The service validates
// the same list; keeping it here means the model cannot even propose other fields.
func changesSchema() map[string]any {
	return obj(map[string]any{
		"receiver_name":         strProp("收件人姓名"),
		"receiver_phone":        strProp("收件人手机号（服务端只保存脱敏值）"),
		"shipping_province":     strProp("省"),
		"shipping_city":         strProp("市"),
		"shipping_district":     strProp("区/县"),
		"shipping_address":      strProp("详细地址"),
		"shipping_postal_code":  strProp("邮编"),
		"shipping_method":       enumProp("配送方式", "standard", "express", "scheduled"),
		"requested_delivery_at": strProp("预约送达时间，RFC3339，例如 2026-10-21T14:00:00+08:00"),
		"order_remark":          strProp("订单备注"),
	})
}

// Args is a decoded, schema-validated argument object.
type Args map[string]any

// String returns a trimmed string argument.
func (a Args) String(key string) string {
	if v, ok := a[key].(string); ok {
		return v
	}
	return ""
}

// Int returns an integer argument or def when absent.
func (a Args) Int(key string, def int) int {
	switch v := a[key].(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return def
}

// Map returns a nested object argument.
func (a Args) Map(key string) map[string]any {
	if v, ok := a[key].(map[string]any); ok {
		return v
	}
	return nil
}

// Has reports whether the key was present.
func (a Args) Has(key string) bool {
	_, ok := a[key]
	return ok
}
