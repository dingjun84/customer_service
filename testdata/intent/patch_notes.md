# offline 关键词补丁说明（P1-A）

来源：`testdata/intent/raw_user_phrases.jsonl`（3874 条用户原话，取自
`testdata/im_export/candidates.jsonl` 中 `discard_as_gold=false` 的会话）。

找缺口的方法：先按现有 `has()` 词表给每条原话定分支，`mapped_branch=none`
的进入候选；单条短语频次普遍很低（唯一短语 3874 条，频次 ≥5 的只有 8 条），
所以又做了 2~4 字 n-gram 聚合，只保留总频次 ≥15 且未被现有词表覆盖的片段。

## 补了什么（只追加，未删任何词）

| 新增说法 | 落在分支 | 触发工具 | 依据 |
|---|---|---|---|
| 到哪儿 | logistics | `get_logistics_status` | 「快递到哪儿了」是「到哪」的口语变体 |
| 几天到 | logistics | `get_logistics_status` | 同上，问时效而非要承诺 |
| 什么时候到 | logistics | `get_logistics_status` | 同上 |
| 查快递 | logistics | `get_logistics_status` | 动作在前、名词在后 |
| 快递不动 | logistics | `get_logistics_status` | 描述停滞 |
| 没物流 | logistics | `get_logistics_status` | 口语省略 |
| 发了吗 | logistics | `get_logistics_status` | 问是否已出库 |
| 充不上 / 充不进 | after_sales | `get_order_detail` | 故障描述，先读单再判断 |
| 用不了 / 不能用 | after_sales | `get_order_detail` | 与「坏了」等价 |
| 开不了机 / 没反应 | after_sales | `get_order_detail` | 电子产品故障口语 |
| 发错 / 错发 / 少件 | after_sales | `get_order_detail` | 少发/发错的同义 |
| 能用 / 通用 / 适配 | product | `search_products` | 售前兼容性提问 |
| 保质期 | product | `search_products` | 商品属性类 |
| 怎么退 | knowledge | `search_knowledge` | 问流程，不是发起售后 |
| 怎么开票 / 开票流程 | knowledge | `search_knowledge` | 问流程 |

## 刻意没加的

| 说法 | 频次 | 为什么不加 |
|---|---|---|
| 骑手、专送、到店、几楼 | 103+ | 实施方案禁区：骑手/到店语义不能直接当仓配物流金标。这类应转人，塞进 `logistics` 会把 O2O 说法学成 CS 事实 |
| 加微 / 加V 号 | — | 导流话术，属于 P2-B 负例，不是意图词 |
| 怎么处理 | 8 | 歧义太大，任何分支都会猜错；应追问具体问题，保持 `none` |
| 呼叫骑手退回 | — | 无对应工具，按方案转人 |

## 冒烟

`testdata/intent/offline_smoke.jsonl` 共 21 条，覆盖新增词 9 条 + 原有主路径 12 条
（取消预检、取消、改址、退款进度、退货、转人、投诉、发票、优惠、权益、默认列表）。
由 `internal/ai/offline_smoke_test.go` 断言，当前 21/21 通过；加词后取消与退货
两条主路径仍然走原来的分支。
