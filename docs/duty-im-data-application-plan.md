# duty-im × customer_service 数据应用实施方案

> 给其他 agent 的细粒度派工说明书：含路径、过滤规则、JSON schema、字段映射、完整样例、验收门槛与禁区。按章节顺序执行，不要跳步。
>
> 关联 Page：[duty-im × customer_service 数据应用实施方案](https://cursor.com/canvas/store-6550f671-cfe2-4e9f-aa0d-4ee0431f6c8e/88069c02-a047-47e7-b79e-d15276d082b2)

---

## 执行前必读

源数据含手机号、订单卡、门店电话与地址。默认**仅内部改写与评测**。

- 禁止原文入库
- 禁止未脱敏提交
- 禁止把骑手/到店语义直接当仓配物流金标
- CS 写工具只创建「待确认」操作；评测里禁止「已退款 / 已取消」类断言成功

---

## 1. 总览

### 1.1 一句话目标

把美团门店 IM 导出（`duty-im-export`）变成 `customer_service` 能吃的评测、口吻、意图词表与改写知识；**先度量再扩量**，不做微调、不做向量 RAG。

### 1.2 绝对路径别名

| 别名 | 路径 |
|---|---|
| `IM_ROOT` | `/Users/admin/Desktop/workspace/JDDC-Baseline-Seq2Seq/数据/duty-im-export` |
| `INTENT_FILE` | `/Users/admin/Desktop/workspace/JDDC-Baseline-Seq2Seq/数据/意图分类.txt` |
| `CS_ROOT` | `/Users/admin/Desktop/workspace/customer_service` |
| `SCENARIOS` | `CS_ROOT/testdata/scenarios/test_scenarios.jsonl` |
| `PROMPT` | `CS_ROOT/internal/ai/prompt.go` |
| `OFFLINE` | `CS_ROOT/internal/ai/offline.go` |
| `TOOLS` | `CS_ROOT/internal/tools/definitions.go` |
| `CANDIDATES`（产物） | `CS_ROOT/testdata/im_export/candidates.jsonl` |
| `STYLE_DIR`（产物） | `CS_ROOT/testdata/style/` |
| `INTENT_DIR`（产物） | `CS_ROOT/testdata/intent/` |

### 1.3 依赖与顺序（硬约束）

1. **预处理（阻塞项）**：必须先产出 `CANDIDATES`，否则禁止开 P0/P1/P2 写库类任务
2. **P0-A 与 P0-B 可并行**：都读 `CANDIDATES`；P0-B 的 holdout 样例不得写进 prompt few-shot
3. **P1**：依赖 `CANDIDATES`；offline 补丁需跑冒烟；转人样例写入 `SCENARIOS`
4. **P2**：KB 入库前必须人工审核闸门；负例集可先做
5. **P3**：仅 P0-A 验收通过，且 P1-A 或 P1-B 至少一个验收后启动

### 1.4 数据事实（勿再重算假设）

| 项 | 值 |
|---|---|
| 导出会话 | 1000 txt + `_index.json` + `_manifest.json` |
| 源池 | `session_count≈11216`，本次 `limit=1000`，`excluded≈6989` |
| 消息≈ | 16203；顾客 44% / 商家 37% / AI 13% / 系统 5% |
| 非金标噪声 | 系统 + `[自动回复]` + AI 等待 ≈17–18% |
| 有真人商家 | ≈870；无人商家 ≈130，直接丢弃作金标 |
| 现有评测 | 89（dev 79 / holdout 10） |
| 业务域 | 美团闪购/到店 O2O，不是仓配 B2C |

### 1.5 派工检查清单

- [ ] 预处理：`CANDIDATES` 已产出且含 `stats`（**阻塞**）
- [ ] P0-A：`SCENARIOS` 净增 ≥50，holdout ≥8，抽 10 条无 PII
- [ ] P0-B：gold 8–12 + bad 8–12 + 可选 prompt 增量 ≤15 行
- [ ] P1-A：`mined_phrases` + offline 冒烟 ≥15
- [ ] P1-B：转人 scenario ≥10（holdout ≥2）
- [ ] P2-A：改写 KB ≥10 篇已审核入库
- [ ] P2-B：`auto_reply_negatives` ≥100
- [ ] P3：仅验收后启动

### 1.6 每个工作包统一回报模板

1. 产物路径 + 行数/条数
2. 过滤丢弃数、脱敏命中数（按类型）
3. 3 条脱敏后抽样（可公开粘贴）
4. 对照本包验收条目：逐条 yes/no
5. O2O 无法映射而丢弃的数量与原因 Top3
6. 建议的下一包 / 阻塞项

---

## 2. 预处理（阻塞项）

### 2.1 目标

产出 `CS_ROOT/testdata/im_export/candidates.jsonl`。

后续 P0/P1/P2 **只读该文件**，禁止再直接扫原始 txt 写金标。

### 2.2 源文件格式

**Header 行**（中文冒号）：门店 ID、品牌 ID、渠道店 es_id、会话 group_id、任务、任务创建、消息范围。

**消息块**：

```text
[YYYY-MM-DD HH:MM:SS] <角色> <类型> #<消息ID>
<正文>
```

- 角色枚举：`顾客` / `商家` / `AI` / `系统`
- 类型常见：`文字`、`图片`、`商品卡`、`5`（订单卡 JSON）、语音/URL
- 商家自动回复：正文以 `[自动回复]` 开头

### 2.3 步骤（必须按序）

1. **枚举文件**：`IM_ROOT` 下全部 `*.txt`；跳过 `_index.json` / `_manifest.json`；总数应 ≈1000
2. **解析 header + 消息**：消息头 = 行首时间戳方括号 + 角色 + 类型 + `#数字ID`
3. **角色映射**：顾客→`user`；商家→`merchant`；AI→`bot`；系统→`system`
4. **打标 flags（消息级）**
   - `is_auto_reply`：`merchant` 且正文 strip 后以 `[自动回复]` 开头
   - `is_wait_bot`：`bot` 且命中等待词表
   - `is_media`：图片/语音或正文为 URL
   - `is_order_card`：类型 `5` 或订单 JSON
   - `is_product_card`：商品卡
5. **会话级过滤**：若 `merchant` 中 `is_auto_reply=false` 的条数为 0，或 `user` 文字条数 &lt; 1：仍可写入 candidates，但 `discard_as_gold=true`
6. **脱敏**：见下表；原始明文不得写入 `CANDIDATES`
7. **写盘**：`candidates.jsonl` + `im_export/stats.json`（输入数、保留数、丢弃原因、脱敏命中）

### 2.4 等待词表（AI 套话 → `is_wait_bot`）

命中任一词且角色为 `bot` → `is_wait_bot=true`；**不得**进入 `assistant_expected`。

- 稍等、稍后、马上、尽快
- 看看具体问题、耐心等待、还请耐心
- 这边看看、在的呢、请问有什么可以帮

### 2.5 脱敏规则表

| 类型 | 检测 | 替换为 |
|---|---|---|
| 手机 | 大陆 11 位 1 开头 | `{{PHONE}}` |
| 座机 | 0xx 座机形态 | `{{TEL}}` |
| 长数字订单 | ≥10 位连续数字（非手机） | `{{ORDER_ID}}` |
| 微信号口播 | 加V/加微/vx + 号 | `{{CONTACT}}` |
| 地址块 | 省市区门牌或收件地址后文 | `{{ADDRESS}}` |
| 客服名 | 转交文案双汉字名（谨慎） | `{{AGENT_NAME}}` |
| 媒体 URL | http(s) 链接 | `{{MEDIA_URL}}` |
| 订单卡 | 类型 5 或 `order_view_id` | `{{ORDER_CARD}}` |
| 商品卡 | 类型商品卡 | `{{PRODUCT_CARD}}` |

### 2.6 `candidates.jsonl` 每行必填字段

- `source_file`, `store_id`, `vendor_id`, `es_id`, `group_id`
- `task_ids`（字符串数组）, `message_count`, `user_text_count`, `merchant_human_count`
- `discard_as_gold`（bool）
- `turns`：数组，元素含 `idx`, `ts`, `role`, `msg_type`, `flags`, `text_redacted`
- `flags_session`：`has_complaint_cue`, `has_human_request`, `has_media`
- 选填：`category_guess`（12 大类或 null）, `notes`

### 2.7 脱敏后短会话样例

源文件：`1197066_225178_…_PNM.txt`

```text
顾客：请尽快为用户开具发票并完成状态确认…
AI：亲，开票可以加{{CONTACT}}哈～
商家（非自动）：开票信息和邮箱发下
```

用法：商家句可进 P0-B 正例；AI 加微句进 P2-B 负例；顾客催开票 → category 发票。

### 2.8 O2O → CS 语义映射

| 源说法 | 可映射到 CS | 不可映射时 |
|---|---|---|
| 骑手 / 专送 / 配送中 | 物流 + `get_logistics_status` | — |
| 美团取退 / 呼叫骑手送回 | 售后寄回政策 `search_knowledge` | 无工具则转人 |
| 到店 / 几楼 / 店内电话 | 转人工或「联系门店」知识 | 禁止编门店电话 |
| 闪购时效 | `search_knowledge` 配送政策 | 禁止承诺具体 ETA |
| 包装拍下 | 售后申请指引 | — |
| 买错买多退款 | `after_sales.return` / `refund` | — |

### 2.9 预处理验收

- `CANDIDATES` 行数 ≥ 700（可含 `discard_as_gold`）
- 抽样 20 行：无真实 11 位手机、无 http 原文
- `stats.json` 含 `discarded_no_human_merchant`、`redact_phone_hits` 等
- 回报贴 3 条 turns 摘要

---

## 3. P0-A 评测集扩充

### 3.1 目标

向 `SCENARIOS` **净增 ≥50 条**；holdout ≥8；覆盖口吻 / 转人 / 禁 Markdown / 缺依据转人。只追加；改动旧条须在回报说明。

| 项 | 值 |
|---|---|
| 输入 | `CANDIDATES` |
| 输出 | `SCENARIOS` 追加 + `im_export/scenario_build_log.jsonl` |
| 参照 id | `SCN_HT_EXPLICIT_080` |

### 3.2 完整 schema 样例（字段名必须一致）

在 `SCENARIOS` 内搜 `SCN_HT_EXPLICIT_080` 复制结构，勿自创键名。字段概要：

```json
{
  "scenario_id": "SCN_HT_EXPLICIT_080",
  "category": "人工客服接入",
  "intent_tags": ["human.explicit_request"],
  "customer_no": "CUST-1001",
  "conversation_id": "CONV-80080",
  "split": "dev",
  "turns": [
    {"role": "user", "content": "别给我机器人了，转人工。"},
    {"role": "assistant_expected", "content": "好的，已转接人工客服，会话标记为已转接。当前咨询的订单ORD-10001和摘要会一起带过去，您不用重复描述。"}
  ],
  "fixtures": {
    "order_no": "ORD-10001",
    "order_status": "pending_payment",
    "paid_amount_minor": 0,
    "refunded_amount_minor": 0
  },
  "expected_tool_calls": [
    {"required": true, "tool": "transfer_to_human"}
  ],
  "forbidden_actions": ["继续用机器人劝阻"],
  "expected_outcome": "立即转人工并保留摘要",
  "risk_level": "normal",
  "pass_criteria": ["没有反复劝阻", "保留订单上下文"]
}
```

### 3.3 字段填写细则

| 字段 | 规则 |
|---|---|
| `scenario_id` | `SCN_IM_<缩写>_<3位>`，序号从 200 起；例 `SCN_IM_HT_200` |
| `category` | 12 类中文名：物流查询；修改订单信息；退换货、保修；退款；发票业务；支付 / 金融（白条）；商品咨询、使用问题；质量故障、破损反馈；优惠券 / 活动 / 价格咨询；催发货、补发；人工客服接入；投诉、闲聊情绪表达。与邻条短名风格保持一致，同一 PR 勿混两套 |
| `intent_tags` | 点分英文；参照 `human.explicit_request` / `logistics.track` / `after_sales.return` / `refund.eta` |
| `customer_no` / `conversation_id` | `CUST-2xxx` / `CONV-9xxxx`；禁止真实 `group_id` |
| `split` | 仅 `dev` 或 `holdout`。转人/缺依据/投诉 → holdout；常见物流退款 → dev |
| `turns` | 仅 `user` 与 `assistant_expected`；2～6 条；assistant 按 CS 口吻重写，不贴商家「亲亲」堆砌 |
| `fixtures` | 四键全在；`ORD-2xxxx`；金额为分；状态对齐现有枚举 |
| `expected_tool_calls` | 必为数组；tool 必须来自白名单；拿不准只标查类 + `transfer_to_human` |
| `forbidden_actions` | 中文禁令或 `null`；常用：自称 AI；使用 Markdown；承诺已退款；追问要不要转人工 |
| `risk_level` | `low` \| `normal` \| `high` \| `critical` |
| `pass_criteria` | 可检查短句；口吻类写清：无 Markdown；未自称 AI；缺依据则转人 |

### 3.4 工具白名单（`expected_tool_calls.tool`）

`search_knowledge`, `search_products`, `get_product_detail`, `list_my_orders`, `get_order_detail`, `get_logistics_status`, `get_payment_refund_status`, `get_service_status`, `get_invoice_status`, `get_customer_benefits_status`, `get_promotion_rules`, `check_action_eligibility`, `request_order_cancellation`, `modify_order`, `submit_service_request`, `create_complaint_ticket`, `transfer_to_human`

### 3.5 转化检查单（每条 scenario）

- [ ] 源 `discard_as_gold=false` 且 `merchant_human_count≥1`
- [ ] 已完成 O2O→CS 映射或决定转人/丢弃
- [ ] user 句无 PII / URL / 订单卡 JSON
- [ ] `assistant_expected`：短句、「你」、无 Markdown
- [ ] `expected_tool_calls` 非空且 tool 合法
- [ ] 缺依据是/否题含 `transfer_to_human`
- [ ] `build_log` 记录 `source_file` ↔ `scenario_id`

### 3.6 50 条分层配额

| 方向 | 条数 | 备注 |
|---|---|---|
| 物流 / 催发货 | 8 | `get_logistics_status` |
| 退换货 / 退款 | 10 | 含资格检查 |
| 发票 / 支付 | 6 | 参考开票催办样例 |
| 售前规格 | 6 | 缺参→转人 |
| 人工 / 投诉 | 10 | holdout ≥4 |
| 订单修改取消 | 6 | `check_action_eligibility` |
| 口吻回归专用 | 4 | 禁 Markdown / 禁 AI |

### 3.7 P0-A 验收

- 逐行 JSON 100% 可解析
- 新增 ≥50；holdout ≥8
- 随机 10 条无手机号/真实座机/真实 `order_view_id`
- 每条 `pass_criteria` 非空
- holdout 原文未进 `STYLE_DIR` few-shot

---

## 4. P0-B 口吻 few-shot 与回归

### 4.1 目标

固化口吻回归集。不重写整份 SystemPrompt；若增补 ≤15 行且无具体商品场景。

### 4.2 当前 prompt 已有（勿重复堆砌）

来自 `PROMPT`：

- **【身份】** 禁止提及 AI/机器人/大模型；问是否人工答「在的」或直接办事
- **【口吻】** 短句口语；禁 Markdown/小标题/编号清单；禁解释内部字段；禁「需要我转人工吗」
- **【转资深同事】** 缺依据是/否；通融赔付模糊；要人工或情绪升级。话术：`我帮你转给资深同事确认一下`

### 4.3 产物

| 文件 | 内容 |
|---|---|
| `STYLE_DIR/gold_replies.jsonl` | 8–12 条；`id,source_file,user_context,reply,why_good` |
| `STYLE_DIR/bad_replies.jsonl` | 8–12 条；`id,reply,violations[]` |
| `STYLE_DIR/prompt_style_snippets.md` | 可选；3–5 条抽象正反对照 |

### 4.4 正例规则

1. `merchant`、非 auto、非 media、脱敏后 8–80 字
2. 改写去过度口头禅；统一「你」
3. 优先：一句现状 + 可选转资深同事
4. 禁止欢迎语/纯表情

抽象正例：

> 参数里没写充电功率，这个我拍不了板。我帮你转给资深同事确认一下。

### 4.5 `violations` 枚举（反例必用这些标签）

| 标签 | 判定 |
|---|---|
| `claims_ai` | AI/机器人/大模型/智能助手 |
| `markdown` | `**` 加粗、`#` 标题、md 列表 |
| `section_headers` | 结论：/建议做法：/两点可参考 |
| `asks_transfer_permission` | 需要我转人工吗 |
| `explains_schema` | 科普系统有无某字段 |
| `closing_filler` | 还需要帮忙吗/很高兴为您服务 |
| `over_promise` | 未确认声称已退款/已取消 |
| `auto_welcome` | 欢迎光临本店 |

### 4.6 反例最小集合

- 1 条 Markdown+编号长答（充电功率类）
- 1 条自称智能助手
- 1 条「需要我转人工吗」
- 1 条解释系统字段缺失
- 1 条欢迎光临

### 4.7 P0-B 验收

- gold/bad 各 8–12，可解析
- bad 覆盖 ≥6 种 violation
- 改 prompt 则 diff ≤15 行且无 SKU/功率词
- 5 条口吻回归 scenario 记录通过/失败

---

## 5. P1 意图挖矿 + 转人工

### 5.1 P1-A 现有 offline 词表（只加不删）

文件：`OFFLINE` → `plan()`

| 分支 | `has(...)` 原文 |
|---|---|
| 人工 | 人工, 真人, 转客服 |
| 取消预检 | 取消 + 能不能/可以/是否/能取消/能退 |
| 取消 | 取消 |
| 售后发起 | 退货,换货,维修,坏了,故障,补发,漏发,少发,没收到,价保,降价,差价,退款 |
| 退款进度 | 退款 + 到账/进度/到哪/什么时候 |
| 物流 | 物流,快递,到哪,签收,运单,发货,配送 |
| 售后进度 | 售后,处理进度,进度 |

### 5.2 P1-A 步骤

1. **抽短语**：user 文字 4–40 字 → `INTENT_DIR/raw_user_phrases.jsonl`
2. **弱标注**：`phrase, primary_category, secondary?, mapped_branch, source_file`；预标后人工抽检 ≥20%
3. **找缺口**：频次 ≥5 且现有 `has()` 未覆盖 → 补丁候选
4. **写补丁**：只追加同义；几楼/加微开票/呼叫骑手无工具 → 转人，勿塞进物流
5. **冒烟**：`offline_smoke.jsonl` ≥15 条 `utterance → expect_tool_or_branch`

**P1-A 验收**：`mined_phrases` ≥200；`patch_notes.md` 有词→分支→tool；冒烟 ≥15 全过；取消/退货主路径不坏。

### 5.3 P1-B 转人规则

- 筛 `has_human_request` / `has_complaint_cue` / 多轮催促
- `expected_tool_calls` 含 `transfer_to_human`
- 话术：`我帮你转给资深同事确认一下`
- forbidden：追问要不要转人工；自称 AI
- 缺规格逼问能不能用：禁止是/否，必须转人
- 结构对齐 `SCN_HT_EXPLICIT_080`

**P1-B 验收**：≥10 条；holdout ≥2；覆盖 prompt 三条转人触发。

### 5.4 P1 不做

- 不训独立意图模型
- 不用自动回复当意图金标
- 不做 function-calling SFT

---

## 6. P2 知识改写 + 自动回复负例

### 6.1 P2-A 知识改写

- 通道：admin 或 `cmd/kbimport`；LIKE 检索
- 目标 ≥10 篇，入库前人工审核
- 草稿字段：`title`, `body`（≤800 字）, `applies_to`（`general` \| `o2o_adapted`）, `source_session_ids`（仅内部）, `conflict_with_jd_faq`
- 选题：开票催办、退款进度、包装拍照、寄回取件、联系门店（不写真实电话）、买错退回
- 改写去品牌/口头禅/微信号

**验收**：10 篇字段齐全；抽 5 词能 search 命中；无真实手机号；占位符不应进生产正文。

### 6.2 P2-B 自动回复负例

- `STYLE_DIR/auto_reply_negatives.jsonl` ≥100
- 字段：`id`, `kind`（`system` \| `merchant_auto` \| `bot_wait`）, `text_redacted`

**验收**：≥100 条；5 条回归能抓住「欢迎光临本店」复读。

### 6.3 P2 禁区

- 原文入库
- 上向量 RAG
- 店内座机写进通用 KB

---

## 7. P3（可选）

### 7.1 启动门禁

P0-A 验收通过，**且** P1-A 或 P1-B 至少一个验收通过。未满足勿开工。

### 7.2 P3-A 分层再导出

1. 源池 ≈11216；按 `vendor_id` 分层，单品牌 ≤25%
2. `merchant_human_count≥1` 的占比 ≥80%
3. 拉长稀缺类：发票、支付、质量
4. `manifest` 记时间窗、limit、excluded、分层种子
5. 保持现有 txt 格式以复用预处理

### 7.3 P3-B 多模态缺口（只写说明，不训练）

- 统计含图片/订单卡占比
- CS 现状：文本 + tools
- 最小方案：订单卡 JSON 解析优先于通用 OCR
- 禁止下载顾客图片外传

---

## 附录：意图 12 大类（`INTENT_FILE`）

1. 物流查询
2. 修改订单信息
3. 退换货、保修
4. 退款
5. 发票业务
6. 支付 / 金融（白条）
7. 商品咨询、使用问题
8. 质量故障、破损反馈
9. 优惠券 / 活动 / 价格咨询
10. 催发货、补发
11. 人工客服接入
12. 投诉、闲聊情绪表达
