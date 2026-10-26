# duty-im × customer_service 实施回报（P0 + P1 + P2 草稿）

按 `docs/duty-im-data-application-plan.md` 执行。数据源：`data/duty-im-export`
（1000 txt + `_index.json` + `_manifest.json`，已被 `.gitignore` 的 `data/` 忽略，
不进版本库）。文中所有文本均为脱敏后或人工改写，无原文入库。

## 0. 执行顺序

共用预处理 → P0-A / P0-B → P1-A / P1-B → P2-B → P2-A（草稿待审核）。P3 未启动
（门禁要求 P0-A 通过且 P1 至少一个包通过，且需先确认价值）。

---

## 1. 共用预处理（阻塞项，已解除）

产物：

| 文件 | 行数 | 说明 |
|---|---|---|
| `testdata/im_export/candidates.jsonl` | 1000 | 每行一个会话，含 `turns[].text_redacted` |
| `testdata/im_export/stats.json` | — | 丢弃数、按类型的脱敏命中、角色占比 |

解析结果与实施方案 §1.4 的数据事实逐项吻合：

| 项 | 方案预期 | 实际 |
|---|---|---|
| 导出会话 | 1000 | 1000（解析 1000，失败 0，空 0） |
| 消息数 | ≈16203 | 16203 |
| 角色占比 | 44 / 37 / 13 / 5 | 43.9 / 37.3 / 13.3 / 5.5 |
| 无人商家会话 | ≈130 | 130 |
| 非金标噪声 | 17–18% | 自动回复 839 + AI 等待 1216，占消息 12.7%（叠加系统 5.5% 后 18.2%） |

丢弃：无人商家 130 + 无用户文字 16 = 146，保留 854。

脱敏命中：手机 202 / 座机 94 / 长数字 209 / 联系号 18 / 地址 56 / 客服名 0 /
URL 1014 / 邮箱 25。全量扫描 `candidates.jsonl`：手机号泄漏 0 条、http 原文 0 条、
真实邮箱 0 条。

三条脱敏后抽样（可直接公开）：

1. `{{MEDIA_URL}}`（图片消息正文只剩占位符）
2. `亲 麻烦您拍下商品现在的状态和包装哦`
3. `需要申请买错买多退货退款，退货方式{{CONTACT}}`

脱敏规则踩到的两个坑（已修）：

- 「加V15216899332」「加下VV13915977921」「➕V13052850715」这类**导流号**原先被
  当成普通手机号，语义丢失。现在导流规则排在手机号之前，替换为 `{{CONTACT}}`。
- 18 位统一社会信用代码（`9142000070680457XA`）尾部直接跟字母，`\b` 边界匹配
  不到，整串漏出。长数字规则去掉尾部边界后覆盖；同时手机号规则改为「前后都不是
  数字」，避免信用代码里的 11 位片段被误判成手机。

验收（§2.9）：行数 ≥700 ✔（1000）｜抽样无手机号/无 http ✔（全量 0）｜
`stats.json` 含 `discarded_no_human_merchant` 等 ✔｜3 条 turns 摘要 ✔

---

## 2. P0-A 评测集扩充

产物：

| 文件 | 行数 | 说明 |
|---|---|---|
| `testdata/im_export/scenarios_im.jsonl` | 51 | 手写场景，`SCN_IM_*_2xx` |
| `testdata/im_export/scenario_build_log.jsonl` | 51 | `scenario_id ↔ source_file` |
| `testdata/scenarios/test_scenarios.jsonl` | 140 | 合并后（原 89 + 51） |
| `testdata/manifest.json` | — | 增加 `generated 89 / from_im 51 / total 140` |

分层（§3.6 配额）：物流 9 / 退换货保修 7 / 退款 4 / 发票 5 / 支付 1 / 售前 7 /
人工与投诉 12 / 订单修改 6 / 口吻回归 5。原文需求是 8/10/6/6/10/6/4=50，实际在
退换货与退款之间做了合并（7+4），因为源数据里「退款」多由退货触发，硬拆会造出
同一故事的变体。

分区：dev 33 / holdout 18（35%）。转人工类 19 条，其中 holdout 12 条。
风险分布：critical 6 / high 19 / normal 20 / low 6。

**为什么这批场景不会被重建冲掉**：`test_scenarios.jsonl` 是 `internal/seed`
的产物。直接把 50 行 append 进去，下一次 `make seed-reset` 就会静默丢失。因此把
`scenarios_im.jsonl` 当作源文件，`writeArtifacts` 在写盘前合并，并按
`scenario_id` 去重排序；任何一条不合格（id 形态、枚举、工具白名单、PII、轮次
结构）直接失败，不会半套写入。另加 `make scenarios`（`cmd/scenarios`），
不打开数据库就能刷新这 140 条。

验收（§3.7）：逐行 JSON 可解析 ✔｜净增 ≥50 ✔（51）｜holdout ≥8 ✔（18）｜
随机 10 条无手机号/座机/order_view_id ✔｜`pass_criteria` 非空 ✔｜holdout 原文
未进 `STYLE_DIR` ✔（gold 只取 dev 与普通会话）

---

## 3. P0-B 口吻 few-shot 与回归

产物：

| 文件 | 行数 | 说明 |
|---|---|---|
| `testdata/style/gold_replies.jsonl` | 10 | `id, source_file, user_context, reply, why_good` |
| `testdata/style/bad_replies.jsonl` | 10 | 8 种 violation 覆盖 |
| `testdata/style/prompt_style_snippets.md` | 5 组 | 抽象正反对照 |
| `testdata/style/tone_regression.jsonl` | 5 | 口吻回归记录 |

正例全部来自真实商家人工回复（非自动回复、8–80 字、脱敏后），按「一句现状 +
可选转资深同事」重写，去掉了「亲亲」堆砌。反例覆盖 `markdown`、`section_headers`、
`claims_ai`、`asks_transfer_permission`、`explains_schema`、`closing_filler`、
`over_promise`、`auto_welcome` 八种。

**没有改 prompt**：`internal/ai/prompt.go` 已包含【身份】【口吻】【何时转资深同事】
【正反各一句】，§4.2 要求的三块都在，再叠加就是重复堆砌。该文件当前还有你自己
未提交的改动，我没有触碰。

新增 `internal/style`：把八类违规做成可执行规则。`bad_replies.jsonl` 的每条标注
都会被断言真的检出，`gold_replies.jsonl` 必须零误伤。口吻回归的 5 条记录
`method=static_rule_check`、`model_check=pending`——真实模型下的通过/失败要等接了
密钥才能判，这里没有假装跑过。

验收（§4.7）：gold/bad 各 8–12 ✔（10/10）｜bad 覆盖 ≥6 种 ✔（8）｜
prompt diff ≤15 行 ✔（0 行）｜5 条口吻回归已记录 ✔

---

## 4. P1 意图挖矿 + 转人工

产物：

| 文件 | 行数 | 说明 |
|---|---|---|
| `testdata/intent/raw_user_phrases.jsonl` | 3874 | `phrase, freq, primary_category, mapped_branch, covered_by_current_has, source_file` |
| `testdata/intent/offline_smoke.jsonl` | 21 | `utterance → expect_tool` |
| `testdata/intent/patch_notes.md` | — | 词 → 分支 → 工具，含「刻意没加」 |
| `internal/ai/offline.go` | — | 只追加同义，未删任何词 |
| `internal/ai/offline_smoke_test.go` | — | 21/21 通过 |

缺口怎么找的：唯一短语 3874 条，频次 ≥5 的只有 8 条（长尾极重），单条短语筛不出
东西，所以做了 2~4 字 n-gram 聚合，保留总频次 ≥15 且现有 `has()` 未覆盖的片段。
补了 15 个说法：`到哪儿/几天到/什么时候到/查快递/快递不动/没物流/发了吗` →
logistics；`充不上/充不进/用不了/不能用/开不了机/没反应/发错/错发/少件` →
after_sales；`能用/通用/适配/保质期` → product；`怎么退/怎么开票/开票流程` →
knowledge。

刻意没加：**骑手、专送、到店、几楼**（触碰禁区，应转人而不是塞进 logistics）、
加微/加V 导流话术（属 P2-B 负例）、`怎么处理`（歧义过大，保持 none 让模型追问）。

P1-B 转人工：IM 场景里 `transfer_to_human` 共 19 条，覆盖 prompt 三条触发——
缺依据的是/否（`SCN_IM_PP_226`、`SCN_IM_TN_248`）、通融赔付（`SCN_IM_HT_233`）、
要人工或情绪升级（`HT_230/231/232/235`、`CP_238`）。

验收（§5.2）：`mined_phrases` ≥200 ✔（3874）｜`patch_notes.md` 有词→分支→tool ✔｜
冒烟 ≥15 全过 ✔（21/21）｜取消/退货主路径不坏 ✔（冒烟内含 S03/S04/S07）
验收（§5.3）：转人 scenario ≥10 ✔（19）｜holdout ≥2 ✔（12）｜覆盖三条触发 ✔

---

## 5. P2 知识改写 + 自动回复负例

产物：

| 文件 | 行数 | 说明 |
|---|---|---|
| `testdata/style/auto_reply_negatives.jsonl` | 101 | `id, kind(system\|merchant_auto\|bot_wait), text_redacted` |
| `testdata/kb_drafts/im_kb_drafts.jsonl` | 10 | 草稿，`status=draft`，未入库 |

负例：merchant_auto 46 / bot_wait 40 / system 15，全部脱敏后文本。其中
「欢迎光临本店」类复读样本能被 `internal/style` 直接抓住，5 条回归见
`internal/style/style_test.go`。

知识库草稿 10 篇（开票催办、退款状态机、退货拍照、寄回取件、联系门店、买错退回、
抬头错改、部分退款口径、签收前破损、超期退货），字段含 `title/body/applies_to/
source_session_ids/conflict_with_jd_faq`，正文最长 152 字（限 800）。正文不含手机号、
不含链接、不含占位符。5 个探针词（开票 / 退款 / 拍照 / 寄回 / 超期）在草稿集上
LIKE 均能命中。

**没有入库**：§6.1 要求入库前人工审核。你审核通过后我再跑导入；`contact 门店`
那篇标了 `applies_to=o2o_adapted`，只讲渠道不给号码。

验收（§6.1）：10 篇字段齐全 ✔｜5 词可命中 ✔｜无真实手机号 ✔｜占位符未进正文 ✔
验收（§6.2）：≥100 ✔（101）｜5 条回归能抓住欢迎语复读 ✔

---

## 6. O2O 无法映射而丢弃的（Top 3）

1. **骑手 / 专送 / 到店取退**：O2O 即时配送语义，CS 只有仓配物流与寄回售后，
   按方案转人或丢弃，没有当成物流金标。
2. **门店环境投诉**（隔壁噪音、营业时间、堂食体验）：与电商客服业务无关，
   只保留 1 条作为「越界转人」样本，其余不进场景。
3. **到店自提 / 预约到店时间**：CS 无对应字段与工具，改为「转人」表达。

## 7. 下一步建议

- P2-A 审核：10 篇草稿确认后用 `cmd/kbimport` 或 admin 入库，发布前不要 `published`。
- 口吻回归补真实模型跑分：`model_check=pending` 的 5 条要接 `LLM_API_KEY` 才能定论。
- P3 暂缓：先看 P0-A/P1 的线下评审结果，再决定要不要按 `vendor_id` 分层全量导出。
