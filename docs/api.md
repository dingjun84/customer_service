# HTTP API 契约（MVP v1）

所有路径以 `/api/v1` 为前缀（健康检查除外）。UTF-8 JSON。

## 1. 认证与请求头

| Header | 说明 |
|---|---|
| `X-Demo-Customer-ID` | 开发态身份，如 `CUST-1001`。仅 `APP_ENV=dev/test` 生效；缺失或客户不存在返回 401 |
| `X-Request-ID` | 可选；不传则服务端生成 `req_<uuid>`，并在响应头回写 |
| `Content-Type` | 写接口必须 `application/json` |

`X-Demo-Customer-ID` 只是开发态身份注入，**不是生产鉴权**。非 dev/test 环境所有 `/api/v1/*` 请求返回 401。

## 2. 响应约定

成功：

```json
{"data": {"...": "..."}, "meta": {"limit": 20, "next_cursor": "MjAyNi0x..."}}
```

失败：

```json
{"error": {"code": "forbidden", "message": "该订单不属于当前客户", "request_id": "req_abc", "details": {}}}
```

| code | HTTP | 场景 |
|---|---:|---|
| `bad_request` | 400 | 非 JSON、多余内容 |
| `validation_error` | 400 | 字段非法、limit 越界、消息过长 |
| `unauthorized` | 401 | 缺少/无法识别身份 |
| `forbidden` | 403 | 资源不属于当前客户 |
| `not_found` | 404 | 资源或路由不存在 |
| `conflict` | 409 | 状态冲突、版本冲突、重复提交 |
| `confirmation_required` | 409 | 需要用户显式确认 |
| `rate_limited` | 429 | 操作过于频繁（如后台登录连续失败） |
| `action_expired` | 410 | 待确认动作过期 |
| `llm_unavailable` | 503 | 模型未启用/不可用 |
| `tool_execution_failed` | 502 | 工具执行失败 |
| `internal_error` | 500 | 未预期错误（不返回堆栈） |

分页：列表 `limit` 默认 20（消息 50），上限 100（商品 50）；超大值直接 `validation_error`。`next_cursor` 为不透明字符串，原样回传。

## 3. 健康检查

### GET /healthz
```json
{"data": {"status": "ok", "app_env": "dev", "uptime_seconds": 12}}
```

### GET /readyz
数据库可用且迁移完成时 200；否则 503 `not_ready`。
```json
{"data": {"status": "ready", "schema_version": 1, "llm_mode": "offline",
          "llm_enabled": false, "llm_configured": false, "dev_api": true,
          "db_path": "./data/ecommerce_support.db",
          "dataset": {"customers": 20, "products": 72, "orders": 118,
                      "conversations": 89, "messages": 461}}}
```

`llm_mode` 取 `openai` / `offline` / `disabled`；`offline` 表示本地规则演示模型（非大模型），控制台用它显示顶部徽标。

`db_path` 与 `dataset` **仅在 dev/test 返回**，用于回答「当前进程打开的是不是 seed 写的那份库」——控制台把它显示在左上角标题下方。生产环境不暴露路径与行数。

## 4. 会话与消息

### POST /api/v1/conversations
```json
{"channel": "web_demo"}
```
→ `201`
```json
{"data": {"conversation_id": "uuid", "conversation_no": "CONV-20261020-8F3A2C", "status": "open",
          "channel": "web_demo", "created_at": "...", "updated_at": "..."}}
```

### GET /api/v1/conversations?limit=20&cursor=...
返回当前客户会话列表（按创建时间倒序），`meta.next_cursor` 用于翻页。

### GET /api/v1/conversations/{conversationID}
```json
{"data": {
  "conversation_id": "uuid", "status": "open", "channel": "web_demo",
  "active_task": {"pending_action_id": "uuid", "action_type": "cancel_order", "target_id": "order_uuid"},
  "pending_actions": [{
    "action_id": "uuid", "action_no": "ACT-20261020-77AB12", "action_type": "cancel_order",
    "status": "pending", "summary": "取消订单 ORD-10002，并申请退款 ¥499.00",
    "preview": {"order_no": "ORD-10002", "estimated_refund_display": "¥499.00"},
    "expires_at": "2026-10-20T12:10:00Z"
  }]
}}
```

### GET /api/v1/conversations/{conversationID}/messages?limit=50&before=...
返回按时间正序的消息页（只含 user / 有内容的 assistant，工具内部消息仅在 dev trace 中可见）。`before` 传上一页 `meta.next_cursor`。

### POST /api/v1/conversations/{conversationID}/messages
```json
{"message": "我的订单 ORD-10003 到哪了？", "client_message_id": "client-msg-001"}
```
→ `200`
```json
{"data": {
  "conversation_id": "uuid",
  "user_message": {"id": "msg_u", "role": "user", "content": "...", "created_at": "..."},
  "assistant_message": {"id": "msg_a", "role": "assistant", "content": "包裹目前在派送中…"},
  "tool_executions": [{"tool_call_id": "call_...", "tool_name": "get_logistics_status",
                       "status": "succeeded", "latency_ms": 12}],
  "pending_actions": []
}}
```

- 同一会话内相同 `client_message_id` 重发 → 返回首次结果并带 `"idempotent": true`，不重复写消息、不重复调工具。
- LLM 不可用时返回 503，但用户消息已持久化；用相同 `client_message_id` 重试不会重复插入用户消息。
- 达到工具轮次/次数上限时返回 `"degraded": true` 与安全提示文案。

### POST /api/v1/conversations/{conversationID}/actions/{actionID}/confirm
无 body。返回真实执行结果：
```json
{"data": {"action_id": "uuid", "action_no": "ACT-...", "action_type": "cancel_order",
          "status": "executed", "idempotent": false,
          "result": {"order_no": "ORD-10002", "order_status": "cancelled",
                     "refund_no": "RF-20261020-AB12CD", "refund_amount_minor": 49900,
                     "refund_amount_display": "¥499.00", "refund_status": "requested"}}}
```
- 动作不属于当前客户/会话 → 403；已过期 → 410 `action_expired`；已拒绝 → 409；重复确认 → 200 且 `idempotent=true`。
- 执行前重新校验资格与订单版本，状态已变化 → 409 `conflict`，业务数据不变。

### POST /api/v1/conversations/{conversationID}/actions/{actionID}/reject
→ `{"data": {"status": "rejected", ...}}`，订单不变，写审计（result=rejected）。

### POST /api/v1/conversations/{conversationID}/close
→ `{"data": {"status": "closed", ...}}`。关闭后发消息返回 409。

## 5. 只读业务 API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/products?q=咖啡机&category=厨房电器&limit=20` | 搜索在售商品 |
| GET | `/api/v1/products/{productID}` | 商品详情 + 全部 SKU（支持 UUID 或 `product_code`） |
| GET | `/api/v1/orders?status=shipped&limit=20&cursor=...` | 当前客户订单列表 |
| GET | `/api/v1/orders/{orderNo}` | 订单详情 + 商品快照 |
| GET | `/api/v1/orders/{orderNo}/shipments` | 包裹与轨迹（倒序节点） |
| GET | `/api/v1/orders/{orderNo}/service-cases` | 该订单售后单 |
| GET | `/api/v1/service-cases/{caseNo}` | 售后详情 |
| GET | `/api/v1/invoices?order_no=ORD-10009` | 发票记录 |

订单详情示例（金额同时给出 `*_minor` 与格式化值）：

```json
{"data": {
  "order_no": "ORD-10003", "order_status": "shipped",
  "created_at": "2026-10-16T...", "paid_at": "...", "shipped_at": "...",
  "amounts": {"item_amount_minor": 134900, "paid_amount_minor": 134900,
              "refunded_amount_minor": 0, "currency": "CNY",
              "paid_display": "¥1349.00", "refunded_display": "¥0.00"},
  "receiver": {"name": "张三", "phone_masked": "138****1234", "province": "广东省",
               "city": "深圳市", "district": "南山区", "address": "科技园南路 88 号"},
  "shipping_method": "standard",
  "items": [{"id": "item_uuid", "product_name": "意式半自动咖啡机 20Bar",
             "sku_name": "曜石黑", "quantity": 1, "refunded_quantity": 0,
             "unit_price_minor": 134900, "unit_price_display": "¥1349.00"}]
}}
```

## 6. 开发诊断 API（仅 dev）

需 `APP_ENV=dev|test` **且** `DEV_API_ENABLED=true`，否则路由不注册（404）。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/dev/customers` | 演示身份目录：全部客户的编号、姓名、手机掩码、订单数、会话数 |
| GET | `/api/v1/dev/conversations/{conversationID}/trace` | 会话 + 工具调用（入参/结果/耗时/错误码）+ 审计记录 |
| GET | `/api/v1/dev/tool-executions?conversation_id=...&limit=50` | 工具调用日志 |

`/api/v1/dev/customers` 供控制台动态生成身份下拉框，避免把客户写死在前端。`meta.order_total` 是所有客户的订单合计，可用来核对「118 单是不是都在」：

```json
{"data": [{"customer_no": "CUST-1001", "display_name": "张三", "phone_masked": "138****0001",
           "order_count": 8, "conversation_count": 10}],
 "meta": {"count": 20, "order_total": 118, "conversation_total": 89}}
```

工具入参落库前已脱敏：手机号掩码、地址/备注截断。

## 7. 兼容性与限制

- 不提供通用 CRUD（不能直接改订单）；写操作只有"提案 + 确认"和工单两类。
- 不支持 SSE/WebSocket，发送消息为同步阻塞，最长受 `LLM_TIMEOUT_SECONDS` 与工具超时约束。
- 时间字段均为 UTC RFC3339；金额为人民币分。

---

## 8. 后台管理 API（/api/admin/v1）

运营后台使用**独立身份体系**：账号密码换取服务端会话 Cookie，与 `X-Demo-Customer-ID` 无关，也不接受该请求头。

### 8.1 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/auth/mode` | 未鉴权；返回 `{"app_env","dev"}`，供登录页判断是否展示内置账号提示 |
| POST | `/auth/login` | `{"username","password"}` → 200，`Set-Cookie: wb_admin_session=…` |
| POST | `/auth/logout` | 撤销当前会话并清 Cookie |
| GET | `/auth/me` | 返回当前用户与运行信息（`app_env` / `llm_mode` / `schema_version` / 会话到期时间） |

- Cookie：`HttpOnly`、`SameSite=Lax`、`Path=/`；非 dev/test 环境带 `Secure`。
- 会话表只存 token 的 SHA-256 摘要，明文 token 不落库；`ADMIN_SESSION_TTL_HOURS` 控制有效期。
- 密码存储 `pbkdf2_sha256$<iter>$<salt>$<hash>`（HMAC-SHA256，12 万次迭代，16 字节随机盐），校验使用常数时间比较。
- 同一 IP + 用户名连续失败 5 次后锁 5 分钟，返回 `429 rate_limited`。
- 写请求若带 `Origin` 头且与 `Host` 不一致 → `403 forbidden`（配合 SameSite 覆盖 CSRF）。

### 8.2 读接口

统一 `{"data":…,"meta":{"total","page","page_size","total_pages"}}`；`page` 从 1 起，`page_size` 最大 100，越界返回 `validation_error`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/overview` | 概览：6 个指标、5 个待办队列、14 天趋势、意图 Top、工具 Top、最近失败、health |
| GET | `/health` | 迁移版本、环境、模型模式、活跃会话、幂等键数、各表行数 |
| GET | `/conversations?q=&status=&channel=&page=` | 跨客户会话列表（含消息数/工具数/待确认数） |
| GET | `/conversations/{id}` | 会话 + 全部消息（含 tool 轮次）+ 工具轨迹 + 待确认 + 审计 + 关联订单 |
| GET | `/tickets?q=&status=&type=&priority=` | 工单队列，按优先级与等待时长排序 |
| GET | `/tickets/{ticketNo}` | 工单 + 处理记录 + 审计 |
| GET | `/pending-actions?q=&status=&action_type=` | 待确认动作列表 |
| GET | `/pending-actions/{id}` | 单条（支持 UUID 或动作号） |
| GET | `/audit-logs?q=&result=&actor_type=&target_type=&target_id=` | 审计流水 |
| GET | `/tool-executions?conversation_id=&tool_name=&status=` | 工具调用明细 |
| GET | `/analytics/tools` | 每个工具的调用量、成功率、平均/P95 耗时、主要错误码 |
| GET | `/analytics/intents` | 意图聚合：样本、解决/未解决/转人工、解决率、置信度、情绪 |
| GET | `/knowledge?q=&category=&status=` | 知识库列表（不含正文） |
| GET | `/knowledge/categories` | 分类及各分类文章数 |
| GET | `/knowledge/{id}` | 文章详情（含正文），支持 UUID 或 `article_code` |
| GET | `/customers?q=&status=` | 客户列表（订单数、累计消费、会话数、工单数） |
| GET | `/customers/{customerNo}` | 客户 + 订单 + 会话 + 工单 + 权益 |
| GET | `/orders?q=&status=` | 订单列表 |
| GET | `/orders/{orderNo}` | 订单 + 明细 + 包裹 + 支付 + 退款 + 售后 + 发票 + 审计 |
| GET | `/products?q=&category=&status=` | 商品列表（SKU 数、库存合计、价格区间） |
| GET | `/products/{id}` | 商品 + 描述 + SKU 明细，支持 UUID 或 `product_code` |
| GET | `/service-cases?q=&status=&type=` | 售后单 |
| GET | `/refunds?q=&status=` | 退款 |
| GET | `/invoices?q=&status=` | 发票 |
| GET | `/promotions?q=&status=` | 促销活动（含规则 JSON） |

### 8.3 写接口

后台只有三类写操作，全部在同一事务内写业务表 + `action_audit_logs`（`actor_type=admin`）。

**工单流转**

```
POST /tickets/{ticketNo}
{"status":"assigned","priority":"high","assigned_to":"张三","note":"已接管"}
```

- 状态机：`open → assigned|pending_customer|resolved|closed`；`assigned → open|pending_customer|resolved|closed`；
  `pending_customer → assigned|resolved|closed`；`resolved → closed|open`；`closed → open`。
- 不在迁移表内的流转返回 `409 conflict`；状态值本身非法返回 `400 validation_error`。
- 状态变化或填写 `note` 时写入 `service_ticket_notes`；`assigned` 未指定处理人时自动记为当前操作人。

**追加处理记录**

```
POST /tickets/{ticketNo}/notes
{"note":"已联系承运商，预计明日更新轨迹"}
```

**作废待确认动作**

```
POST /pending-actions/{actionID}/void
{"reason":"重复提交"}
```

- 只能把 `pending` 改成 `expired`；重复作废是幂等的空操作。
- **不提供"代替客户确认"接口**：确认代表客户本人同意，代确认会破坏"提案 + 确认"的核心约束。

**知识库**

```
POST /knowledge      {"category","title","content","keywords":[],"source","status"}
PUT  /knowledge/{id}  同上（version 自增）
POST /knowledge/{id}/status   {"status":"draft|published|archived"}
```

- `title` ≤ 200 字，`content` ≤ 20000 字，`keywords` ≤ 20 个，`category` ≤ 40 字。
- 新建时 `status` 默认 `draft`；文章编码形如 `KB-20261020-8F3A2C`。
