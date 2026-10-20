# Agent 工具目录（MVP v1）

工具是模型**唯一**能触达业务数据的方式：没有"执行 SQL"或"任意 HTTP"工具，所有工具只接受 JSON Schema 描述过的参数（未知字段直接拒绝），客户身份由 `ExecutionContext` 注入，模型无法指定。

返回结构统一：

```json
{"ok": true, "data": {}, "error": null, "request_id": "req_xxx"}
```
失败：`{"ok": false, "error": {"code": "forbidden", "message": "该订单不属于当前客户"}}`。  
`code ∈ {validation_error, forbidden, not_found, conflict, action_expired, internal_error, unknown_tool, tool_execution_failed}`。

---

## 1. 只读工具

| Tool | 必填参数 | 可选参数 | 说明 |
|---|---|---|---|
| `search_knowledge` | `query` | `category`, `limit` (≤10) | 搜索已发布政策文章，返回摘要片段、来源、更新时间。仅解释政策，不可替代实时数据 |
| `search_products` | — | `query`, `category`, `limit` (≤20) | 搜索在售商品，返回编码/名称/品牌/分类/简要属性 |
| `get_product_detail` | `product_id` 或 `product_code` | — | 商品参数 + 全部 SKU（规格、库存、标价） |
| `list_my_orders` | — | `status`, `limit` (≤20) | 当前客户订单列表；用于多订单歧义时引导用户确认 |
| `get_order_detail` | `order_no` | — | 订单状态、金额、收件信息、商品快照 |
| `get_logistics_status` | `order_no` 或 `tracking_no` | — | 包裹 + 轨迹节点；不返回不存在的预计送达时间 |
| `get_payment_refund_status` | `order_no` | `refund_no` | 支付/退款金额与状态；`requested/processing` 均未到账 |
| `get_service_status` | `order_no` 或 `case_no` | — | 退货/换货/维修/补发/价保的处理状态 |
| `get_invoice_status` | `order_no` | `invoice_no` | 发票类型、状态、开票时间（演示环境无真实文件） |
| `get_customer_benefits_status` | — | `benefit_type`, `limit` (≤30) | 优惠券/积分/运费险/白条（模拟）状态 |
| `get_promotion_rules` | — | `query`, `promotion_code`, `limit` (≤10) | 活动规则、起止时间、价保参考差额 |
| `check_action_eligibility` | `action_type`, `order_no` | `order_item_id`, `quantity`, `changes`, `reason` | 资格检查，返回 `eligible`/`reason_code`/`limits`/`preview` |

`action_type ∈ {cancel_order, modify_order, return, exchange, repair, refund, reship, price_protection}`。

`check_action_eligibility` 返回示例：

```json
{"eligible": false, "action_type": "cancel_order", "order_no": "ORD-10003",
 "reason_code": "already_shipped", "message": "订单已发货或已完成，无法直接取消，可改为申请退货退款"}
```

原因码：`ok` `unsupported_action` `order_not_shipped` `already_shipped` `order_closed` `order_not_paid` `out_of_window` `quantity_exceeded` `item_not_in_order` `item_required` `conflicting_case` `no_changes` `invalid_field` `nothing_to_refund`。

## 2. 写入提案工具（需用户确认）

这三类工具**只创建 `pending_actions`**，不修改业务数据。返回中的 `action_id` 供前端确认按钮使用。

| Tool | 必填参数 | 可选参数 | 说明 |
|---|---|---|---|
| `request_order_cancellation` | `order_no` | `reason` | 资格通过后生成取消提案；已付款会附带退款金额预估 |
| `modify_order` | `order_no`, `changes` | `reason` | 修改收件信息，`changes` 仅接受白名单字段 |
| `submit_service_request` | `case_type`, `order_no` | `order_item_id`, `quantity`, `reason`, `description` | 退货/换货/维修/退款/补发/价保 |

`changes` 白名单（`additionalProperties=false`）：

`receiver_name` `receiver_phone` `shipping_province` `shipping_city` `shipping_district` `shipping_address` `shipping_postal_code` `shipping_method`(`standard|express|scheduled`) `requested_delivery_at`(RFC3339) `order_remark`

`receiver_phone` 由服务端转换为 `receiver_phone_masked` 落库，不保存完整号码。

返回示例：

```json
{"requires_confirmation": true,
 "action_id": "uuid", "action_no": "ACT-20261020-AB12CD",
 "action_type": "modify_order", "status": "pending",
 "summary": "修改订单 ORD-10002 的 shipping_address",
 "preview": {"changes": [{"field": "shipping_address", "before": "旧地址", "after": "新地址"}]},
 "expires_at": "2026-10-20T12:10:00Z",
 "next_step": "已生成待确认操作，必须由用户点击确认按钮后才会执行"}
```

资格不通过时同一结构返回 `requires_confirmation:false` 与 `eligibility`，**不会**创建动作。

## 3. 立即写入工具

| Tool | 必填参数 | 可选参数 | 说明 |
|---|---|---|---|
| `create_complaint_ticket` | `summary` | `details`, `order_no`, `priority` | 用户明确投诉/质量问题时建单，无需二次确认 |
| `transfer_to_human` | `reason` | `summary`, `priority` | 建转人工工单，并把会话状态置为 `transferred` |

`priority ∈ {low, normal, high, urgent}`。

## 4. 执行约束

| 约束 | 值 |
|---|---|
| 单条用户消息最大模型/工具轮次 | `LLM_MAX_TOOL_ROUNDS`（默认 5） |
| 单条用户消息最大工具调用数 | `LLM_MAX_TOOL_CALLS_PER_TURN`（默认 8） |
| 单次工具执行超时 | `TOOL_TIMEOUT_SECONDS`（默认 10s） |
| 单次消息长度 | 2000 字 |
| 待确认动作有效期 | `PENDING_ACTION_TTL_MINUTES`（默认 10 分钟） |

超出上限时：未执行的调用记为 `rejected`（原因"单轮工具调用次数已达上限"），返回安全提示并建议转人工。

## 5. 审计与脱敏

每次调用写 `tool_executions`：`tool_name`、`tool_call_id`、`input_json`（脱敏 + 截断 4KB）、`output_json`（截断 8KB）、`status ∈ {started, succeeded, failed, rejected}`、`error_code`、`latency_ms`、起止时间。

写操作额外写 `action_audit_logs`：`before_json`/`after_json`、`actor_type ∈ {assistant, customer, system, ...}`、`result ∈ {succeeded, failed, rejected}`、`request_id`。审计记录没有对外删除接口。

## 6. 调用顺序（提示词约束，服务端兜底）

1. 用户问事实 → 先查工具，再回答；不得从历史消息推断当前状态。
2. 用户要办理业务 → 先 `check_action_eligibility`，再调用对应提案工具。
3. `eligible=false` → 如实解释原因或 `transfer_to_human`，禁止换工具重试。
4. 提案工具返回后 → 告知用户"需要点击确认"，禁止声称已办结。
5. 工具报错 → 不编造结果，给出重试/转人工建议。
