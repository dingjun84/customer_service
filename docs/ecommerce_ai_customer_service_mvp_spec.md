# 电商 AI 客服业务系统 MVP 技术设计与 Agent 开发任务书

**版本：** 1.0  
**日期：** 2026-10-20  
**目标技术栈：** Go + SQLite + HTTP API + LLM Tool Calling  
**用途：** 交由一个或多个代码 Agent 按任务实现；本文是第一版的产品与工程约束。

---

## 0. 给执行 Agent 的说明

请先完整阅读本文件，再开始修改代码。目标不是实现覆盖所有电商平台的生产客服系统，而是交付一个可以在本地启动、写入模拟数据、通过 API 对话、让大模型按需调用业务工具并安全读写 SQLite 的可运行 MVP。

### 必须遵守

1. 先完成数据模型、服务接口和 API 契约，再并行实现；不要自行发明与本文冲突的字段或路径。
2. 不需要单独调用意图分类模型。大模型根据消息与可用工具自行决定直接回答、调用工具、追问或转人工。
3. 模型不能直接访问数据库，不能生成或执行任意 SQL。所有读写都经过已注册工具、业务服务和 Repository。
4. 真实订单、物流、支付、退款状态必须以数据库查询结果为准。知识库只用于政策解释，不能替代实时业务数据。
5. 所有变更操作都必须由服务端校验权限和业务规则；对于取消订单、修改订单、发起售后等操作，先创建待确认操作，再由用户通过专用确认 API 确认执行。
6. 不接入真实支付、真实物流、真实发票或真实客服平台；第一版全部使用模拟业务数据。
7. 不提交真实 API Key、个人信息或真实客户数据。测试数据必须为虚构数据。
8. 修改完成后执行 `gofmt`、`go test ./...`、`go vet ./...`，并更新 README 与运行说明。

### Definition of Done

- 可通过单条命令启动 HTTP 服务。
- 启动时自动运行 SQLite migration，数据库文件目录不存在时能够创建。
- `GET /healthz` 和 `GET /readyz` 返回健康状态。
- 可以通过 seed CLI 写入一套可重复执行的演示数据。
- 可以创建会话、发送消息、保存用户/助手/工具消息、调用工具并生成助手答复。
- 查询工具可读取模拟业务数据；变更工具会进行资格校验、用户确认、事务写入和审计记录。
- 用户不能查询或修改其他客户的订单、物流、退款和售后数据。
- 关键流程有自动化测试，测试使用临时 SQLite 数据库而不是共享开发库。
- README 描述环境变量、运行、seed、测试、API 示例和已知限制。

---

## 1. 产品范围与设计原则

### 1.1 MVP 目标

构建一个为 AI 客服提供真实、结构化、可控业务工具的本地业务系统。用户通过客服 API 发起自然语言对话，模型可以直接回答简单问题，也可以调用工具查询商品、订单、物流、退款、售后、发票、优惠和知识库；需要办理业务时，先检查资格并生成待确认操作，再经用户确认执行。

### 1.2 第一版覆盖范围

- 商品和 SKU 查询。
- 订单列表、订单详情和订单商品查询。
- 物流包裹及轨迹查询。
- 支付、退款和售后申请状态查询。
- 退货、换货、维修、退款申请、补发、价保申请的模拟提交流程。
- 订单允许字段的修改与未发货订单取消流程。
- 发票、优惠活动、用户权益和客服政策查询。
- 转人工、投诉/质量问题工单。
- 对话记录、上下文状态、工具调用日志、待确认操作和审计日志。
- Seed 命令、测试夹具和 API 文档。

### 1.3 明确不做

- 不做独立意图分类器或分类微调。
- 不做向量数据库、Embedding/RAG 平台；知识检索先用 SQLite 关键词匹配。
- 不做微服务、消息队列、Redis、分布式工作流。
- 不接真实支付、物流、白条、运费险或发票服务。
- 不做生产级 OAuth/账号系统；MVP 使用开发态身份注入，并明确隔离生产配置。
- 不做 SSE/ WebSocket 流式输出；第一版同步返回最终回复，后续再扩展。
- 不实现任意订单字段修改；第一版只开放白名单字段。

### 1.4 核心原则

- **Agent 决策，业务服务兜底。** 模型选择工具，后端校验可否执行。
- **读与写分离。** 只读工具尽量简单；写工具需要资格检查、确认、事务和审计。
- **业务事实与聊天记录分离。** 对话里“退款成功”不是事实，`refunds` 中的当前状态才是事实。
- **单体模块化。** 一个 Go 服务、一个 SQLite 数据库、清楚的包边界，不要过早拆服务。
- **可替换模型。** 通过 `ChatModel` 接口隔离 LLM 提供商；只要求配置一个支持 tool calling 的 OpenAI-compatible Chat Completions 接口。
- **数据结构先稳定。** 不同意图复用同一业务实体和工具，不为 289 个意图各建表。

---

## 2. 技术选型

| 层             | MVP 选型                                                | 说明                                      |
| ------------- | ----------------------------------------------------- | --------------------------------------- |
| 语言            | Go（使用开发环境可用的稳定版本）                                     | 静态类型、部署简单、并发与标准库完善                      |
| HTTP          | `net/http`                                            | 路由、请求处理、中间件以标准库实现，减少依赖                  |
| 数据库           | SQLite 文件数据库                                          | 单机、低并发写入、便于本地演示和 Agent 测试               |
| SQL 接口        | `database/sql`                                        | Repository 使用显式 SQL，不引入 ORM             |
| SQLite Driver | `modernc.org/sqlite`                                  | CGo-free，减少不同开发环境中的原生依赖问题；锁定经过测试的依赖版本   |
| Migration     | `migrations/*.sql` + `go:embed` + `schema_migrations` | 简单可审计；每次迁移在事务中执行，migration 按序号不可修改已发布版本 |
| LLM           | Provider Adapter + OpenAI-compatible tool calling     | Base URL、模型名、Key 由环境变量注入，不绑定特定厂商        |
| 日志            | `log/slog`                                            | 结构化日志，默认不输出手机号、地址、API Key 或完整敏感工具入参     |
| 配置            | 环境变量 + 默认值                                            | 不引入配置中心                                 |
| 测试            | `testing` + 临时 SQLite 文件                              | Repository、服务、工具、HTTP 和端到端测试分层          |

**SQLite 使用约束：** 每个连接都要启用 `foreign_keys` 和 `busy_timeout`；在启动迁移前、事务之外启用 WAL。配套文件 `ecommerce_support_mvp_sqlite.sql` 是可独立执行的完整 schema（含 PRAGMA）；配套文件 `ecommerce_support_mvp_migration_0001.sql` 是 DDL-only migration 内容，不含连接级 PRAGMA。实际工程的 `migrations/0001_init.sql` 应基于 DDL-only 文件，连接参数由 database 初始化代码设置。时间统一存 UTC RFC3339 文本，金额存最小货币单位（人民币分），布尔值存 `INTEGER 0/1`，JSON 结构存为 TEXT 并在应用层校验。MVP 默认单服务实例运行；若后续出现多实例或高频并发写入，应迁移到 PostgreSQL 等服务型数据库，而不是通过 SQLite 共享文件扩展。

建议依赖保持最少：SQLite driver 不可省；若代码复杂度仍低，暂不引入第三方路由框架、ORM、DI 容器或 Agent 框架。

---

## 3. 系统架构与请求流程

```text
Frontend / Test Client
        |
        v
HTTP API + Request ID + Identity Middleware
        |
        +-----------------------+---------------------+
        |                       |                     |
        v                       v                     v
Conversation Service      Customer Read APIs     Dev/Debug APIs
        |
        v
Context Assembler --> ChatModel (LLM Provider Adapter)
        ^                         |
        |                         | assistant text / tool calls
        |                         v
Conversation Store <--- Tool Registry / Tool Executor
                                  |
                                  v
                          Application Services
                         /         |          \
                        v          v           v
                   Policy Rules  Repositories  Audit/Idempotency
                        \          |           /
                         \         v          /
                             SQLite DB
```

### 3.1 单次对话流程

1. API 中间件生成 `request_id`，识别当前开发客户身份；正式环境以后由认证系统提供 identity。
2. Conversation Service 检查会话是否属于当前客户、状态是否允许继续，然后持久化用户消息。
3. Context Assembler 组合系统规则、最近完整对话、当前任务状态；只有用户引用早期任务或状态不完整时才检索更早对话。
4. ChatModel 收到消息和当前允许使用的工具定义，返回直接回复或工具调用。
5. Tool Registry 校验工具是否存在、参数是否符合 Schema，再调用对应 Application Service。
6. 工具执行前后记录 `tool_executions`；服务端执行业务权限与规则校验。
7. 查询工具返回数据给模型继续生成；写工具通常创建 `pending_actions`，而不是立即执行。
8. 若模型输出最终答复，保存 assistant message 并返回 API。
9. 用户点击确认后，确认 API 通过 Application Service 执行操作，事务内修改业务表并写 `action_audit_logs`，然后更新待确认操作状态。

### 3.2 工具调用循环约束

- 每条用户消息最多执行 5 轮模型/工具循环；可配置。
- 每条用户消息最多执行 8 次工具调用；可配置。
- 超限时返回安全、可理解的错误并提供转人工，不无限循环。
- 只读且彼此独立的调用后续可以并行；MVP 先按序执行，尤其禁止并发执行多个写操作。
- 每个工具都返回结构化结果；不把 SQL 错误、堆栈、内部文件路径返回给模型或用户。
- 工具超时、LLM 超时、非法 JSON 和未知工具名都必须有测试。

---

## 4. 推荐目录结构

```text
.
├── cmd/
│   ├── server/main.go
│   └── seed/main.go
├── internal/
│   ├── config/                 # 环境变量、默认值、校验
│   ├── httpapi/
│   │   ├── router.go
│   │   ├── middleware.go       # request id、开发身份、日志、panic recovery
│   │   ├── response.go         # 统一成功/错误结构
│   │   ├── handlers/           # 按 conversation/product/order 等拆分
│   │   └── dto/                # 请求/响应 DTO
│   ├── domain/                 # 核心实体、状态常量、业务错误
│   ├── repository/
│   │   ├── sqlite/             # database/sql 显式 SQL
│   │   └── interfaces.go       # 供 service 依赖的接口
│   ├── service/
│   │   ├── catalog.go
│   │   ├── orders.go
│   │   ├── logistics.go
│   │   ├── refunds.go
│   │   ├── aftersales.go
│   │   ├── invoices.go
│   │   ├── promotion.go
│   │   ├── eligibility.go
│   │   ├── actions.go
│   │   └── tickets.go
│   ├── ai/
│   │   ├── model.go            # ChatModel interface + DTO
│   │   ├── openai_compatible.go
│   │   ├── orchestrator.go     # tool calling loop
│   │   ├── context.go          # 上下文组装/窗口限制
│   │   └── prompt.go
│   ├── tools/
│   │   ├── registry.go
│   │   ├── definitions.go      # JSON Schema + 描述
│   │   └── handlers.go         # 工具 -> Application Service
│   ├── conversation/
│   │   └── service.go
│   ├── database/
│   │   ├── sqlite.go
│   │   └── migrate.go
│   └── seed/                   # 幂等测试数据构建器
├── migrations/
│   └── 0001_init.sql
├── docs/
│   ├── api.md
│   └── agent-tool-catalog.md
├── data/                       # 本地 DB；加入 .gitignore
├── go.mod
├── README.md
└── .env.example
```

该结构为推荐结构，不要求每个文件机械对应。如果文件数量很少，可以合并同一层中的小文件，但不能把 HTTP Handler、SQL、LLM 请求和业务规则全部写进 `main.go`。

---

## 5. 模块职责与接口边界

| 模块                   | 职责                            | 不应做的事情                    |
| -------------------- | ----------------------------- | ------------------------- |
| Config               | 读取和校验环境变量                     | 不含业务规则                    |
| Database/Migration   | 打开 SQLite、设置 PRAGMA、应用迁移、关闭连接 | 不直接响应 HTTP                |
| Repository           | 用参数化 SQL 读写单个/相关实体，支持事务       | 不决定用户是否有权限、不调用 LLM        |
| Application Service  | 组合 Repository、执行业务规则、事务和幂等逻辑  | 不依赖模型自然语言判断业务资格           |
| Tool Registry        | 定义工具名、描述、参数 Schema、分发器、结果格式   | 不暴露任意 SQL 或任意 HTTP 请求能力   |
| AI Orchestrator      | 对话、模型调用、工具循环、最终回复             | 不直接访问数据库、不绕过工具调用业务服务      |
| Context Assembler    | 选择最近完整对话、当前任务状态、按需历史          | 不把所有历史和所有工具结果无限追加         |
| Conversation Service | 保存消息、会话状态、摘要及当前任务状态           | 不把业务状态作为聊天文本的唯一存储         |
| HTTP API             | 参数解析、身份上下文、调用 Service、返回 DTO  | 不在 Handler 中写复杂业务 SQL     |
| Seed CLI             | 写入固定且虚构的开发数据，支持重复执行           | 不在生产环境自动清库                |
| Audit/Tool Trace     | 记录工具参数摘要、结果、耗时与写操作前后变化        | 不存支付凭据、API Key、非必要的完整敏感数据 |

### 5.1 必需服务接口（建议）

```go
type ChatModel interface {
    Respond(ctx context.Context, req ModelRequest) (ModelResponse, error)
}

type ToolExecutor interface {
    Execute(ctx context.Context, ec ExecutionContext, name string, args json.RawMessage) (ToolResult, error)
}

type ConversationService interface {
    Create(ctx context.Context, customerID string, channel string) (Conversation, error)
    SendMessage(ctx context.Context, customerID, conversationID, text string) (ChatTurnResult, error)
    Get(ctx context.Context, customerID, conversationID string) (Conversation, error)
}
```

可按实现需要调整 DTO 或拆分 Repository，但调用边界和安全约束应保持不变。模型 DTO 应支持 assistant text、tool calls、tool result 和 finish reason。

---

## 6. SQLite 数据模型

完整可执行建表 SQL 在配套文件 `ecommerce_support_mvp_sqlite.sql` 中。本节解释各表用途和关系。所有 ID 由应用生成 UUID 字符串；禁止把自增 ID 暴露成跨系统业务编号。

### 6.1 业务主数据表

| 表                    | 用途与关键字段                                                                                                   |
| -------------------- | --------------------------------------------------------------------------------------------------------- |
| `customers`          | 客户：`id`、`customer_no`、`display_name`、`phone_masked`、`status`                                              |
| `products`           | 商品：`product_code`、`name`、`brand`、`category`、`description`、`attributes_json`、`status`                      |
| `product_skus`       | SKU：`product_id`、`sku_code`、`sku_name`、`specifications_json`、`list_price_minor`、`stock_quantity`、`status` |
| `orders`             | 订单主信息：`order_no`、`customer_id`、`order_status`、金额字段、收货地址快照、`is_presale`、时间戳、`version`                      |
| `order_items`        | 订单商品快照：商品/SKU 快照、数量、已退款数量、成交价格、折扣分摊                                                                       |
| `shipments`          | 一个订单可有多个物流包裹，保存承运商、运单号、状态、预计送达时间                                                                          |
| `shipment_events`    | 包裹轨迹事件：事件时间、节点描述、地点、来源                                                                                    |
| `payments`           | 支付流水：支付渠道、支付状态、金额、已退款金额、付款时间；不保存银行卡凭证                                                                     |
| `refunds`            | 退款记录：退款单号、关联订单/支付/售后单、状态、金额、申请及完成时间                                                                       |
| `service_cases`      | 退货、换货、维修、退款、补发、价保等业务申请及状态                                                                                 |
| `invoices`           | 发票申请/开票状态、抬头、税号、金额、电子文件引用；演示环境中的 file URL 为模拟值                                                            |
| `promotions`         | 活动规则与时间窗口，规则结构放 `rules_json`                                                                              |
| `customer_benefits`  | 优惠券、积分、运费险状态等用户权益；白条仅使用模拟状态，不保存账号或财务凭证                                                                    |
| `knowledge_articles` | 客服规则文章、分类、关键字、正文、来源、版本及发布状态                                                                               |

### 6.2 AI 运行与审计表

| 表                     | 用途与关键字段                                           |
| --------------------- | ------------------------------------------------- |
| `conversations`       | 会话归属、渠道、状态、摘要、`active_task_state_json`            |
| `messages`            | user/assistant/tool/system 消息；工具调用参数和结果保留结构化 JSON |
| `message_annotations` | 可选离线分析：意图一级/二级标签、情绪、解决状态、置信度；不作为在线请求必经环节          |
| `tool_executions`     | 每次调用的工具名、参数 JSON、返回 JSON、状态、错误码和耗时                |
| `pending_actions`     | 等待用户确认的取消、修改或售后动作；包括动作 payload、预览、过期时间和执行状态       |
| `action_audit_logs`   | 记录重要变更的 actor、目标、变更前后 JSON、结果、原因和 request ID      |
| `service_tickets`     | 人工转接、投诉、质量问题工单                                    |
| `idempotency_keys`    | 防止相同客户端重试造成重复写操作；保存请求指纹和最终响应                      |
| `schema_migrations`   | Migration 版本历史，由 migration runner 维护              |

### 6.3 关键关系

- `customers 1:N orders`、`orders 1:N order_items`。
- `orders 1:N shipments`、`shipments 1:N shipment_events`。
- `orders 1:N payments`、`orders 1:N refunds`。
- `orders 1:N service_cases`；可选精确到 `order_items`。
- `service_cases 1:N refunds`（例如退款申请产生退款记录）。
- `customers 1:N conversations`、`conversations 1:N messages`。
- `conversations 1:N tool_executions / pending_actions / service_tickets`。
- 写操作完成后新增 `action_audit_logs`；审计记录不可被普通客服工具删除。


### 6.4 数据库开发规范

- 禁止字符串拼接 SQL 参数；一律使用 `?` 参数占位符。
- SQLite 外键必须在每个连接启用；测试中验证外键错误会失败。
- 金额用 integer minor units，例如 CNY ¥12.34 保存为 `1234`；API 返回时可额外提供 `amount_minor` 与格式化金额。
- 时间保存 UTC RFC3339；不存依赖本地时区的裸日期时间。
- JSON 字段必须由 Go 编码/解码并校验；不允许直接将用户任意 JSON 原样拼到 SQL。
- 重要状态更新使用事务；订单变更带 `version` 条件更新，避免并发覆盖。
- 为 customer、order、status、created_at、tracking_no、conversation 和 tool call 添加索引（已在 SQL 中提供初始索引）。
- 开发数据库放在 `data/` 并加入 `.gitignore`。`seed --reset` 只能在 `APP_ENV=dev/test` 下执行。

---

## 7. 状态与业务规则

MVP 的资格规则应实现成 Go 业务服务，不是提示词中的自然语言规则。提示词可以解释结果，但不能最终决定资格。

### 7.1 订单规则（演示规则）

- 取消：只允许满足取消条件的未发货订单；已被物流揽收或已签收订单不得直接取消。已付款订单取消后创建退款申请状态，不模拟真实打款。
- 修改订单：第一版仅允许在未发货阶段修改收件人、掩码手机号、收货省市区/地址、邮编、备注、预约送货时间等白名单字段；禁止模型改 `customer_id`、价格、支付状态、退款金额、订单状态和订单商品。
- 退换货：必须验证订单归属、商品属于该订单、申请数量合法、未超已退/已申请数量，并检查是否已存在冲突的活动售后单。具体 7 天规则使用配置/订单时间统一实现，不由 LLM 估算。
- 退款：金额不能超过可退款余额；部分退货要按订单商品成交额和优惠分摊规则计算，不可简单按标价乘数量。
- 补发：应关联订单/商品和原因，记录服务单；不得直接静默增加订单商品数量。
- 价保：由价保资格服务根据订单时间、商品、促销规则返回 eligible 和差额估算；MVP 仅模拟提交。
- 投诉/转人工：允许创建工单，不需要用户再确认“是否创建投诉”才能记录用户明确提出的投诉；转人工后会话标记 `transferred` 或保持可继续追踪，具体选一种并在实现中保持一致。

### 7.2 待确认操作状态机

```text
pending --用户确认--> confirmed --执行成功--> executed
   |                      |--执行失败--> failed
   |--用户拒绝--> rejected
   |--超时------> expired
```

- 待确认 payload 创建后不可由前端任意修改。
- 确认接口必须校验 action 归属当前 customer、状态为 pending、未过期；服务端在执行前重新读取订单和资格状态。
- 确认操作应具备幂等性：重复确认不得二次取消、二次创建售后单或二次退款。
- 订单修改/取消/创建售后单成功、失败或被拒绝，都要留审计记录。
- 模型不得仅凭一条含糊的“好吧”就绕过待确认 action API；前端应展示明确的动作预览和确认按钮。

### 7.3 统一错误码

| Code | HTTP | 用途 |
|---|---:|---|
| `bad_request` | 400 | JSON 格式错误或参数结构无效 |
| `validation_error` | 400 | 字段值或枚举不合法 |
| `unauthorized` | 401 | 未识别客户身份（生产鉴权以后扩展） |
| `forbidden` | 403 | 资源不属于当前客户或动作无权限 |
| `not_found` | 404 | 资源不存在 |
| `conflict` | 409 | 状态冲突、重复提交或版本冲突 |
| `confirmation_required` | 409 | 操作尚需用户显式确认 |
| `action_expired` | 410 | 待确认操作过期 |
| `llm_unavailable` | 503 | 模型配置缺失或服务不可用 |
| `tool_execution_failed` | 502 | 工具调用执行失败（对用户隐藏内部错误） |
| `internal_error` | 500 | 未预期错误，响应不泄露堆栈 |

错误响应格式：

```json
{
  "error": {
    "code": "not_found",
    "message": "未找到对应资源",
    "request_id": "req_xxx",
    "details": {}
  }
}
```

成功响应默认格式：`{"data": {...}, "meta": {...}}`。对于列表接口，`meta` 中包含 `limit` 与 `next_cursor`（如实现 cursor 分页）。不要在同一个接口混用多套响应风格。

---

## 8. HTTP API 设计

所有公开 API 统一前缀 `/api/v1`。JSON 使用 UTF-8；每个请求生成/传播 `X-Request-ID`。第一版不提供直连数据库的通用 CRUD API，也不把工具执行器直接暴露为任意远程调用接口。

### 8.1 健康检查

| Method & Path | 用途 |
|---|---|
| `GET /healthz` | 进程存活，不依赖外部模型 |
| `GET /readyz` | 数据库连接与 migration 可用；如配置为必须启用 LLM，也可报告 LLM 配置状态，但不要在健康检查中发起昂贵模型调用 |

### 8.2 会话与消息

| Method & Path | 用途 |
|---|---|
| `POST /api/v1/conversations` | 创建当前客户的新会话 |
| `GET /api/v1/conversations?limit=20&cursor=...` | 查询当前客户的会话列表 |
| `GET /api/v1/conversations/{conversationID}` | 查询会话状态、摘要、活动任务状态（不要向前端泄露内部敏感字段） |
| `GET /api/v1/conversations/{conversationID}/messages?limit=50&before=...` | 查询消息历史 |
| `POST /api/v1/conversations/{conversationID}/messages` | 发消息并同步执行 Agent/工具循环，返回最终答复 |
| `POST /api/v1/conversations/{conversationID}/actions/{actionID}/confirm` | 用户显式确认待执行操作 |
| `POST /api/v1/conversations/{conversationID}/actions/{actionID}/reject` | 用户拒绝待执行操作 |
| `POST /api/v1/conversations/{conversationID}/close` | 关闭会话 |

创建会话请求：

```json
{"channel":"web_demo"}
```

发送消息请求：

```json
{"message":"我的订单 ORD-10001 到哪了？","client_message_id":"client-msg-001"}
```

`client_message_id` 可选，用于去重前端重试；同一个会话中相同 client message ID 不应重复创建用户消息或重复执行工具。

发送消息响应示例：

```json
{
  "data": {
    "conversation_id": "conv_uuid",
    "user_message": {"id":"msg_user_uuid","content":"我的订单 ORD-10001 到哪了？"},
    "assistant_message": {"id":"msg_assistant_uuid","content":"包裹目前在运输途中……"},
    "tool_executions": [
      {"tool_name":"get_logistics_status","status":"succeeded","latency_ms":12}
    ],
    "pending_actions": []
  }
}
```

如需确认操作，`assistant_message` 应说明将做什么，`pending_actions` 返回 `action_id`、`action_type`、人类可读的 `summary`、`preview` 和 `expires_at`。前端据此展示确认按钮。最终成功消息应根据确认 API 的真实结果呈现。

### 8.3 商品、订单与售后只读 API

| Method & Path | 用途 | 权限约束 |
|---|---|---|
| `GET /api/v1/products?q=咖啡机&limit=20` | 搜索商品 | 可公开的商品字段 |
| `GET /api/v1/products/{productID}` | 商品详情及 SKU | 不返回内部成本字段 |
| `GET /api/v1/orders?status=shipped&limit=20` | 当前客户订单列表 | 只能查当前 customer |
| `GET /api/v1/orders/{orderNo}` | 当前客户订单详情 | 校验订单属于当前 customer |
| `GET /api/v1/orders/{orderNo}/shipments` | 物流包裹与轨迹摘要 | 校验订单归属 |
| `GET /api/v1/orders/{orderNo}/service-cases` | 订单售后申请 | 校验订单归属 |
| `GET /api/v1/service-cases/{caseNo}` | 售后详情 | 校验售后单归属 |
| `GET /api/v1/invoices?order_no=ORD-10001` | 查询发票状态 | 校验订单归属 |

注意：上表是给客服 UI、测试工具或调试客户端使用的业务 API。AI Tool Registry 调用同一 Application Service，不应通过 HTTP 回调自身 API，也不应复制第二套业务逻辑。

### 8.4 开发诊断 API（仅开发环境）

| Method & Path | 用途 |
|---|---|
| `GET /api/v1/dev/conversations/{conversationID}/trace` | 查看本会话模型调用、工具调用摘要与耗时 |
| `GET /api/v1/dev/tool-executions?conversation_id=...&limit=50` | 查看工具调用调试日志 |

这些 API 必须在 `APP_ENV=dev` 且显式开启 `DEV_API_ENABLED=true` 时才注册，否则应返回 404；不能把调试端点默认暴露到生产部署中。

### 8.5 开发态身份

MVP 可使用 `X-Demo-Customer-ID: CUST-1001` 注入客户身份，但仅当 `APP_ENV=dev` 时允许。该 header 由 middleware 验证 customer 存在后写入 request context。任何请求 body 或工具参数中的 `customer_id` 都不得覆盖已验证的身份。生产环境必须由登录态/认证 middleware 注入客户身份，不能把开发 header 当作生产认证。

### 8.6 分页和参数边界

- 列表默认 `limit=20`，最大 `limit=100`；非法或过大的值返回 `validation_error` 或收敛到上限，选定一种统一策略。
- 禁止将任意列名、排序表达式或 SQL 片段从 query 参数拼接到 SQL。
- `orderNo`、`caseNo` 等是业务编号；`conversationID`、`productID` 等是 UUID ID，命名要保持一致。
- GET 请求不产生业务副作用；写操作使用 POST。

---

## 9. AI Tools 目录与参数约定

工具只在 LLM Orchestrator 中注册，模型只能看见 JSON Schema 描述过的参数。下面列出的工具是 MVP 建议集；可以按工程需要调整名称，但需同步更新工具测试与文档。

### 9.1 第一版工具清单

| Tool | 类型 | 主要输入 | 作用/限制 |
|---|---|---|---|
| `search_knowledge` | 只读 | `query`、可选 `category`、`limit` | 搜索已发布政策文章；返回摘要、正文片段、来源和更新时间 |
| `search_products` | 只读 | `query`、可选 `category`、`limit` | 搜索可售商品，不返回未授权内部字段 |
| `get_product_detail` | 只读 | `product_id` 或 `product_code` | 获取参数、材质、SKU、库存和商品政策关联信息 |
| `list_my_orders` | 只读 | 可选 `status`、`limit` | customer ID 从执行上下文获取，模型不能指定其他客户 |
| `get_order_detail` | 只读 | `order_no` | 返回当前客户订单摘要、订单商品、金额与状态 |
| `get_logistics_status` | 只读 | `order_no` 或 `tracking_no` | 验证归属后返回包裹与轨迹；不虚构预计送达时间 |
| `get_payment_refund_status` | 只读 | `order_no`，可选 `refund_no` | 返回付款和退款状态、金额、时间；不返回支付凭证 |
| `get_service_status` | 只读 | `order_no` 或 `case_no` | 返回退换货、维修、补发、价保申请状态 |
| `get_invoice_status` | 只读 | `order_no`，可选 `invoice_no` | 返回发票类型、状态和开票时间；不伪造发票文件 |
| `get_customer_benefits_status` | 只读 | 可选 `benefit_type` | 查询该客户的优惠券、积分、运费险模拟状态等 |
| `get_promotion_rules` | 只读 | 可选 `query`、`promotion_code` | 返回适用活动、起止时间、规则摘要 |
| `check_action_eligibility` | 只读 | `action_type`、`order_no`；按业务可选 `order_item_id`、`quantity`、`changes`、`reason` | 返回 `eligible`、原因码、可操作范围和所需确认信息；`modify_order` 必须校验 `changes` 白名单 |
| `request_order_cancellation` | 写入提案 | `order_no`、`reason` | 资格通过后创建 `pending_actions`，不直接取消订单 |
| `modify_order` | 写入提案 | `order_no`、`changes` | 仅接受允许修改的白名单字段，创建待确认操作，不直接更新订单 |
| `submit_service_request` | 写入提案 | `case_type`、`order_no`、可选 `order_item_id`、`quantity`、`reason`、`description` | 资格通过后创建待确认操作；确认后才创建实际 service case/refund request |
| `create_complaint_ticket` | 写入 | `summary`、`details`、可选 `order_no`、`priority` | 用户明确投诉时创建工单；身份和订单归属由后端校验 |
| `transfer_to_human` | 写入 | `reason`、`summary`、可选 `priority` | 创建人工接入工单，附会话摘要、相关订单和已执行步骤 |

### 9.2 参数约束

- `action_type` 仅允许 `cancel_order`、`modify_order`、`return`、`exchange`、`repair`、`refund`、`reship`、`price_protection` 等已实现枚举。
- `case_type` 同样必须是服务端已知枚举；不识别的类型直接返回 `validation_error`。
- `modify_order.changes` 仅允许：`receiver_name`、`receiver_phone`、`shipping_province`、`shipping_city`、`shipping_district`、`shipping_address`、`shipping_postal_code`、`shipping_method`、`requested_delivery_at`、`order_remark`。演示库中的电话只保存脱敏值，服务层将 `receiver_phone` 输入转换后写入 `receiver_phone_masked`，不集成真实快递通讯录。第一版如不想支持某字段，直接移除，不做空实现。
- 禁止模型提供 `customer_id`、`order_status`、`paid_amount_minor`、`refunded_amount_minor`、`payment_status` 或数据库 ID 以外的权限字段来改变身份/金额/状态。
- 查询多个实体时设置结果上限；只返回回答该问题必要的字段，不把客户完整资料和整个订单历史一次性发给模型。
- 工具返回 `eligible=false` 时，模型应该解释原因或转人工，不能绕过资格检查再调用写工具。

### 9.3 工具统一返回结构

```json
{
  "ok": true,
  "data": {},
  "error": null,
  "request_id": "req_xxx"
}
```

失败时 `ok=false`，`error` 包含稳定的 `code` 与安全的 `message`。不要向模型返回 SQL 堆栈、密钥、文件路径或未脱敏个人数据。

### 9.4 工具调用审计

每次调用都要记录：工具名、调用 ID、入参（先做脱敏）、结果状态、结构化输出（限制长度）、耗时、错误码、开始/结束时间。写操作除了工具调用日志，还必须写业务审计日志。失败也要保存安全错误码用于回溯。

---

## 10. 上下文管理与模型调用策略

### 10.1 MVP 上下文策略

1. 系统提示词与当前工具定义。
2. 最近 6 轮完整对话作为初始基线（实际按 token 预算调整）；完整轮次包含关联的 assistant tool call 和 tool result，不能只保留一半调用链。
3. `conversations.active_task_state_json` 中尚未完成的任务。
4. 当前用户消息。
5. 只有当前内容出现“上次、刚才、继续、那笔、还是没解决”等历史指代、或当前任务状态不能唯一确定目标时，才检索较早的对话。
6. 当前物流、退款、订单状态一律调用实时工具查询，不从历史消息推断当前状态。

### 10.2 模型配置

建议环境变量：

```dotenv
APP_ENV=dev
HTTP_ADDR=:8080
DB_PATH=./data/ecommerce_support.db
DEV_API_ENABLED=true
LLM_ENABLED=false
LLM_BASE_URL=
LLM_API_KEY=
LLM_MODEL=
LLM_TIMEOUT_SECONDS=30
LLM_MAX_TOOL_ROUNDS=5
LLM_MAX_TOOL_CALLS_PER_TURN=8
CONVERSATION_RECENT_TURNS=6
CONVERSATION_MAX_CONTEXT_CHARS=40000
PENDING_ACTION_TTL_MINUTES=10
LOG_LEVEL=info
```

若 `LLM_ENABLED=false`，服务可以正常启动、健康检查、跑 Repository/API 测试；实际聊天端点返回明确的 `llm_unavailable`，或测试中注入 `FakeChatModel`。不要假装模型已成功响应。

### 10.3 LLM Adapter 要求

- 使用接口封装模型请求，业务层不得引用某个供应商 SDK 的具体类型。
- 支持 assistant text、tool calls、tool result 消息以及调用结束原因。
- 解析模型返回的 JSON 参数后再次做服务端 Schema 校验；模型返回非法 JSON 不得执行工具。
- 设置 HTTP timeout；隐藏/脱敏 Authorization；对可重试的网络错误设置有限重试，不对写工具重试。
- LLM 不可用时返回可识别错误，不丢失已经持久化的用户消息。
- 系统提示词不得把客户提供的文本、知识库文章或工具输出解释成高优先级系统指令；这些内容都是不可信业务数据。

---

## 11. 开发任务拆分与交付顺序

本节可以直接拆成 Agent 任务。建议先由一个协调 Agent 固定接口和公共类型，再将独立模块分派给不同 Agent。并行 Agent 不要同时修改同一批文件；`go.mod`、migration 总文件和公共 DTO 由集成 Agent 统一合并。

### Phase 0：项目骨架与公共契约（集成 Agent）

**任务 P0-1：初始化工程**
- 建立 `go.mod`、`cmd/server`、`internal/`、`migrations/`、`README.md`、`.env.example`。
- 加入配置加载、结构化日志、request ID、统一响应和错误类型。
- 完成 `GET /healthz`。

**验收：** `go test ./...` 可运行；服务可启动；配置缺失时给出清晰错误；不输出密钥。

**任务 P0-2：冻结公共接口**
- 创建 `domain` 类型、DTO、Application Service 接口、ToolResult、ModelRequest/Response。
- 明确 ID、状态枚举、金额与时间类型。

**验收：** 其他 Agent 能仅依赖接口编译其模块；接口变更有协调 Agent 同步。

### Phase 1：数据库与 Repository（数据库 Agent）

**任务 P1-1：SQLite 初始化**
- 使用 `database/sql` 与 `modernc.org/sqlite`。
- 设置外键、busy timeout、WAL；限制并发写操作风险。
- 支持从环境变量读 DB_PATH，自动创建目录。

**任务 P1-2：Migration runner**
- 以 `ecommerce_support_mvp_migration_0001.sql` 为 DDL 参考，放入 `migrations/0001_init.sql` 并通过 `go:embed` 打包；不要把 `PRAGMA journal_mode` 放进迁移事务。
- 数据库启动时先在事务外设置连接级 PRAGMA，再建立/读取 `schema_migrations`，只应用未执行的版本。
- 迁移需要按版本执行并可失败回滚。


**任务 P1-3：Repository**
- 覆盖客户、商品、订单、物流、支付/退款、售后、发票、活动/权益、知识库、会话/消息、工具日志、待确认动作、审计与工单。
- 使用参数化 SQL；返回可识别的 `ErrNotFound`、`ErrConflict`。
- 支持事务边界，尤其是确认写操作。

**验收：** migration 在空数据库成功；第二次启动不会重复创建；外键/唯一约束工作；关键查询有 Repository tests；所有测试使用临时数据库。

### Phase 2：业务服务与状态规则（业务 Agent）

**任务 P2-1：查询服务**
- 商品搜索/详情、客户订单列表/详情、物流和轨迹、支付/退款、售后、发票、权益、活动、知识库。
- 每个客户范围的查询都强制接收由 middleware 传入的 customer ID，并验证资源归属。

**任务 P2-2：资格判断服务**
- 实现取消、修改地址、退货/换货/维修/退款/补发/价保的规则函数。
- 返回结构化 `eligible`、reason code、用户可读说明、必要限制和 preview。
- 规则失败是正常业务结果，不应被当成 500。

**任务 P2-3：Pending Action 执行**
- 创建待确认动作、生成预览、设置 TTL。
- 实现 confirm/reject/expire 流程；确认时重新读取数据、重新执行资格校验。
- 执行成功、失败或拒绝均保存审计日志；按幂等键防止重复操作。
- 修改订单只接受字段白名单；版本号冲突返回 409。

**验收：** 已发货订单不能修改地址或直接取消；不符合条件的售后无法创建；重复确认不会产生重复业务单；所有写事务均有审计记录。

### Phase 3：HTTP API（API Agent）

**任务 P3-1：会话 API**
- 实现创建/列表/详情/消息分页/发送消息/关闭会话。
- 所有会话 API 验证当前客户对会话的拥有权。

**任务 P3-2：业务只读 API**
- 实现商品、订单、物流、售后和发票只读接口。
- 查询 API 只返回业务需要字段，金额格式统一，分页上限统一。

**任务 P3-3：动作确认 API**
- 实现 confirm/reject 路径；确认操作检查当前客户、状态、过期时间。
- 响应真实执行结果，不以“收到确认请求”冒充“业务操作成功”。

**任务 P3-4：开发诊断 API**
- 仅在开发模式注册工具追踪 API。

**验收：** httptest 覆盖成功、参数错误、未登录、越权、资源不存在、重复提交；统一错误 Envelope。

### Phase 4：Tool Registry 与业务工具（工具 Agent）

**任务 P4-1：工具定义**
- 为第 9 节工具建立明确 description、参数 JSON Schema、required、enum、限制和输出结构。
- 工具定义集中注册，禁止 handler 自行把任意参数映射成 SQL。

**任务 P4-2：工具处理器**
- 每个工具调用相应 Application Service；在 Executor 层校验身份、参数、调用上限和超时。
- 查询工具和写入提案工具分开；写工具先创建 `pending_actions`。

**任务 P4-3：工具执行日志**
- 记录 started/succeeded/failed/rejected、耗时、参数/结果 JSON 的安全摘要和错误码。
- 做手机号、地址等字段的日志脱敏。

**验收：** 未注册工具拒绝执行；非法参数不进入业务服务；工具不能查询其他用户；所有工具有单元测试。

### Phase 5：LLM Adapter、Agent 循环与上下文（AI Agent）

**任务 P5-1：模型接口与 Provider Adapter**
- 实现 `ChatModel` interface 和 OpenAI-compatible tool calling adapter。
- 模型名称、base URL、API key、timeout 从环境变量读取。
- 支持 FakeChatModel 供确定性测试使用。

**任务 P5-2：上下文组装**
- 最近完整对话、active task state、当前消息；需要时按客户和会话检索更早历史。
- 不无限加载历史，不用旧对话取代实时业务查询。

**任务 P5-3：Agent Tool Loop**
- 处理 direct answer、single/multiple tool calls、tool result、final answer、tool error 和 max-rounds。
- 把 user、assistant、tool 消息按顺序持久化。
- 工具循环达到上限时返回安全答复或转人工。

**验收：** FakeChatModel 可模拟一次查询、多步查询、工具失败、非法参数、无限调用被限制；LLM 关闭时聊天端点明确报错；助手不能在工具失败时声称操作成功。

### Phase 6：Seed、业务场景与端到端测试（QA/数据 Agent）

**任务 P6-1：Seed CLI**
- 建立 `go run ./cmd/seed`，生成固定的、虚构的客户和业务数据。
- 同一 seed 可重复执行且不生成重复自然键记录；提供 dev/test 专用 reset 参数。

**任务 P6-2：标准场景数据**
至少包含：
1. 未付款订单。
2. 已付款未发货订单。
3. 已发货、多个物流节点的订单。
4. 已签收但客户声称未收到的订单。
5. 已有退款申请、尚未到账的订单。
6. 仅部分商品退货/退款的多商品订单。
7. 退换货资格不满足的订单。
8. 缺少配件/漏发可补发订单。
9. 已开票和待开票订单。
10. 多个订单属于同一用户，用于验证“那个订单”的歧义处理。

**任务 P6-3：端到端测试**
- 通过 HTTP 测试会话 -> 模型 tool call -> 查 DB -> tool result -> assistant response。
- 写操作测试待确认、拒绝、过期、确认成功、重复确认、并发状态变化。
- 越权测试客户 A 不可读写客户 B 数据。

**验收：** 测试可在干净机器执行，不依赖真实模型 Key；真实模型的 smoke test 可单独作为可选命令，不作为 CI 必需条件。

### Phase 7：整合与交付（集成 Agent）

- 解决包依赖环；Repository 不依赖 HTTP，Domain 不依赖 AI SDK。
- 运行 `gofmt -w .`、`go test ./...`、`go vet ./...`。
- 更新 README、API 文档、`.env.example` 和数据库迁移说明。
- 检查 `.gitignore` 包含 `.env`、`data/`、日志文件和临时测试数据库。
- 输出已完成/未完成任务、启动方式、测试结果、遗留风险。

---

## 12. 面向多个 Agent 的分工建议

| Agent | 主要文件所有权 | 交付物 | 不要改动 |
|---|---|---|---|
| Agent A：基础设施/集成 | `cmd/`、`internal/config`、`internal/database`、公共 DTO、`README.md`、`go.mod` | 项目可启动、migration runner、统一接口 | 不在 HTTP Handler 内写业务 SQL |
| Agent B：Repository/Schema | `migrations/`、`internal/repository/`、`internal/repository/sqlite/` | SQLite schema、Repository、仓储测试 | 不自行更改公开 API 路径与 Tool schema |
| Agent C：业务服务/安全 | `internal/service/`、`internal/domain/`、动作确认与审计逻辑 | 资格校验、写操作事务、状态机 | 不调用 LLM、不读取 HTTP 请求体 |
| Agent D：AI/Tools | `internal/ai/`、`internal/tools/` | 模型 Adapter、工具注册、上下文、工具循环 | 不写业务表 SQL；只调用 Application Service |
| Agent E：HTTP/QA | `internal/httpapi/`、`docs/api.md`、测试夹具/端到端测试 | HTTP API、身份中间件、API tests | 不独立新增业务规则或修改 schema |

**合并规则：** Agent A 作为协调者维护公共 interface、状态枚举、DTO 和 `go.mod`。Agent B 先提交表结构和 Repository 方法签名；Agent C/D/E 依赖冻结后的签名开发。若确需调整字段或接口，先更新本文和 SQL，再通知相关 Agent；不要为追求各模块独立通过编译而复制一套 DTO 或创建重复表。

建议每个子任务在 PR/提交说明中包括：目标、文件列表、公共接口变更、测试命令和测试结果。

---

## 13. 测试矩阵与关键验收案例

| 场景 | 期望结果 |
|---|---|
| 用户问“你好” | 可直接回复，不必调用工具 |
| 用户问商品尺寸 | 调用商品查询，回答来自 DB 的规格字段 |
| 用户查物流 | 查所属订单的 shipment 与 events，不能猜物流节点 |
| 用户只说“帮我查一下”且没有当前目标 | 追问，或利用明确的未完成任务状态；不可猜订单 |
| 用户要求取消已发货订单 | 资格工具返回不符合，不能创建可执行的取消动作 |
| 用户要求修改未发货订单地址 | 创建动作预览；用户确认后才更新地址并增加审计日志 |
| 用户拒绝待确认动作 | action 状态 rejected，业务订单不变 |
| 待确认操作过期后确认 | 返回 `action_expired`，订单不变 |
| 同一动作重复 confirm | 不重复修改/创建售后单，返回幂等结果或 conflict |
| 客户 A 请求客户 B 的订单号 | 不返回订单、地址、支付、物流或退款信息 |
| 模型调用未注册工具 | 不执行；记录 rejected 状态 |
| 模型传入错误的 JSON 参数 | 不执行；返回安全的参数错误 |
| LLM 网络超时 | 保存 user message，返回 `llm_unavailable`；可继续重试而不重复提交用户消息 |
| 工具读取失败 | 助手不能编造结果或声称业务成功 |
| 两次并发修改同一订单 | 一个成功，另一个冲突/需要重新确认；不得静默覆盖 |
| Seed 执行两次 | 不产生重复自然键/订单/轨迹 |
| 外键指向不存在实体 | 数据库拒绝写入 |
| HTTP 入参超长 | 受限并返回 validation error，不能造成无界内存增长 |

### 13.1 基本工程检查

```bash
gofmt -w .
go test ./...
go vet ./...
```

建议实现 `make test`、`make run`、`make seed` 作为便捷命令。CI 不应依赖外部 LLM 服务；对 LLM 使用 FakeChatModel 单元测试，另设可选 `make smoke-llm`。

---

## 14. 环境变量、启动与交付

### 14.1 `.env.example`

```dotenv
APP_ENV=dev
HTTP_ADDR=:8080
DB_PATH=./data/ecommerce_support.db
DEV_API_ENABLED=true
LLM_ENABLED=false
LLM_BASE_URL=
LLM_API_KEY=
LLM_MODEL=
LLM_TIMEOUT_SECONDS=30
LLM_MAX_TOOL_ROUNDS=5
LLM_MAX_TOOL_CALLS_PER_TURN=8
CONVERSATION_RECENT_TURNS=6
CONVERSATION_MAX_CONTEXT_CHARS=40000
PENDING_ACTION_TTL_MINUTES=10
LOG_LEVEL=info
```

程序可以不依赖 dotenv 包；README 说明复制 `.env.example` 后，可通过 shell export 环境变量。不要让应用把 `.env` 上传或打印到日志。

### 14.2 预期使用方式

```bash
# 1. 启动服务；自动建目录、跑 migration
 go run ./cmd/server

# 2. 另一个终端写入虚构演示数据
 go run ./cmd/seed

# 3. 检查服务
 curl http://localhost:8080/healthz
 curl http://localhost:8080/readyz

# 4. 创建会话（示例 customer 必须已经通过 seed 创建）
 curl -X POST http://localhost:8080/api/v1/conversations \
   -H 'Content-Type: application/json' \
   -H 'X-Demo-Customer-ID: CUST-1001' \
   -d '{"channel":"web_demo"}'
```

具体示例中的会话 ID、订单号以 seed 输出为准。聊天接口需要配置支持 tool calling 的 LLM，或在测试中注入 FakeChatModel。

### 14.3 最终交付清单

- [ ] Go 工程源代码。
- [ ] SQLite migration 和配套 schema 文件。
- [ ] seed CLI 与标准测试数据。
- [ ] AI Tool Registry 和 LLM Adapter。
- [ ] HTTP API 与 API 契约文档。
- [ ] 业务规则、待确认操作、幂等与审计实现。
- [ ] 单元、API、集成和越权测试。
- [ ] README、`.env.example`、运行命令和已知限制。
- [ ] `go test ./...`、`go vet ./...` 通过结果。

---

## 15. 后续扩展方向（不属于 MVP 阻塞项）

- SSE 流式消息和前端确认卡片。
- 更正式的认证、客服角色和权限策略。
- 运营维护知识库、规则和商品信息的后台 UI。
- FTS5/向量检索和相关性评估。
- LLM 成本、首 token/总响应时间、工具成功率和转人工率监控。
- 离线意图标注与 289 类意图统计；只服务分析与评估，不强制进入在线请求链路。
- 将模拟服务适配器替换成真实订单/仓储/物流/支付系统接口。
- 数据迁移到 PostgreSQL；拆分外部集成或异步任务。

以上扩展不应反向增加 MVP 的实现复杂度。首要目标是让 Agent 通过受控工具读取可信业务数据、正确解释结果，并在清楚的权限边界内执行可审计的模拟业务操作。

---

## 参考链接

- Go `net/http`：https://pkg.go.dev/net/http
- Go `database/sql`：https://pkg.go.dev/database/sql
- SQLite Go driver `modernc.org/sqlite`：https://pkg.go.dev/modernc.org/sqlite
- SQLite WAL：https://sqlite.org/wal.html
- SQLite Foreign Key Support：https://sqlite.org/foreignkeys.html
