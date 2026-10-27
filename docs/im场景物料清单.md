# IM 场景物料清单（方案 A）

> 数据锚点：`DataAnchor = 2026-10-20T04:00:00Z`  
> 场景文件：`testdata/im_export/scenarios_im.jsonl`（51）  
> 世界事实：`testdata/im_export/scenarios_im_world.jsonl`（51，自检全覆盖）  
> 生成入口：`internal/seed/imworld.go` → `build()` 在 `buildConversations()` 之后调用 `buildIMWorld()`

## 1. 场景 → 世界对象映射摘要

| 场景 | 客户 | 主订单 | 状态 | 实付(分) | 关键 SKU / 备注 |
|---|---|---|---|---|---|
| LG_200 | CUST-2001 | ORD-20001 | shipped | 12900 | P073 保温杯；另有 ORD-20201/20202 在途 |
| LG_201 | CUST-2002 | ORD-20002 | shipped | 8900 | 查轨迹 |
| LG_202 | CUST-2003 | ORD-20003 | paid | 16800 | 已付未发催发 |
| LG_203 | CUST-2004 | ORD-20004 | completed | 25900 | 代签收争议 |
| LG_204 | CUST-2005 | ORD-20005 | shipped | 6600 | 改派 |
| LG_205 | CUST-2006 | ORD-20006 | shipped | 4300 | P074 快充头 / 取件码 |
| LG_206 | CUST-2007 | ORD-20007 | shipped | 32800 | 两包裹；第二运单 `MOCKSF7002` |
| LG_207 | CUST-2008 | ORD-20008 | shipped | 9900 | 顺丰 `MOCKSF7003` |
| RT_208 | CUST-2009 | ORD-20009 | completed | 15900 | 退货凭证 |
| RT_209 | CUST-2010 | ORD-20010 | completed | 22900 | P022-02 刻字定制 |
| RT_210 | CUST-2011 | ORD-20011 | completed | 7900 | 退货运费 |
| RT_211 | CUST-2012 | ORD-20012 | completed | 12900 | 尺码换货（原冲突单号已改） |
| RT_212 | CUST-2013 | ORD-20013 | completed | 18900 | 保修（约 60 天前签收） |
| RT_213 | CUST-2014 | ORD-20014 | completed | 26800 | 超无理由窗口（约 30 天） |
| RF_214 | CUST-2015 | ORD-20015 | cancelled | 9900 / 退 9900 | 退款 ETA |
| RF_215 | CUST-2016 | ORD-20016 | cancelled | 9900 / 退 9900 | 称未到账 |
| RF_216 | CUST-2017 | ORD-20017 | completed | 5000 / 退 3000 | 两行部分退 |
| RF_217 | CUST-2018 | ORD-20018 | cancelled | 19900 / 退 19900 | 重复退款 |
| IV_218 | CUST-2019 | ORD-20019 | completed | 45800 | 发票 requested |
| IV_219 | CUST-2020 | ORD-20020 | completed | 26800 | 抬头错误已开具 |
| IV_220 | CUST-2021 | ORD-20021 | completed | 12900 | 邮箱错误 |
| IV_221 | CUST-2022 | ORD-20022 | completed | 6900 | 怎么开票（尚无票） |
| IV_222 | CUST-2023 | ORD-20023 | completed | 89600 | 专票 requested |
| PY_223 | CUST-2024 | ORD-20024 | paid | 16800 | 成功 1 + pending 重复扣款 |
| PP_224 | CUST-2025 | ORD-20025 | pending_payment | 0 | P073 高度参数 |
| PP_225 | CUST-2026 | ORD-20026 | pending_payment | 0 | 礼盒 |
| PP_226 | CUST-2027 | ORD-20027 | pending_payment | 0 | P074 兼容性转人 |
| PP_227 | CUST-2028 | ORD-20028 | completed | 4900 | 开封退货政策 |
| PP_228 | CUST-2029 | ORD-20029 | pending_payment | 0 | SKU-P015-04 缺货 |
| PP_229 | CUST-2030 | ORD-20030 | pending_payment | 0 | P075 保质期 |
| HT_230..235 | CUST-2031..2036 | ORD-20031..20036 | 见 world JSONL | — | 转人工/投诉/主管 |
| CP_236..239 | CUST-2037..2040 | ORD-20037..20040 | completed | — | 投诉与闲聊 |
| MD_240..245 | CUST-2041..2046 | ORD-20041..20046 | paid/shipped | — | 改址/取消/白名单 |
| TN_246..250 | CUST-2047..2051 | ORD-20047.. / 20500/20501 | — | — | 语气约束 |

完整字段以 `scenarios_im_world.jsonl` 为准。

## 2. 商品 / 知识补齐

- 新增商品：`P073` 保温杯、`P074` 快充头、`P075` 洗衣凝珠（均 synthetic）。
- 新增 SKU：`SKU-P013-05` 白色 S（有货）；`SKU-P015-04` 米白 L（`stock=0`）。
- 目录规模：商品 75 / SKU 129。
- 知识文章：在原 32 篇基础上新增 8 篇至上限 40：
  - `KB-RET-07/08`、`KB-INV-04/05/06/07`、`KB-PRD-04`、`KB-LOG-07`
  - **不重复**造发货时效文：`KB-LOG-01「配送时效说明」`已覆盖发货/送达时效。
  - **不**发布 `testdata/kb_drafts`。

## 3. 待审决定

1. **运单 SF→MOCKSF**：场景 LG_206/207 的 `SF7002/SF7003` 改为 `MOCKSF7002/MOCKSF7003`，与 seed `nextTrackingNo` 的 `MOCK` 前缀及 validate 规则一致；`assistant_expected` 已同步。
2. **RT_211 订单号**：原 `ORD-20011` 与 RT_210 冲突，改为 `ORD-20012`（fixtures + turns 同步）。
3. **时钟锚点**：评测与 seed 一律冻结在 `2026-10-20T04:00:00Z`；轨迹/支付/签收时间均为相对锚点的偏移，且不超过锚点。
4. **金额对齐策略**：IM 订单一律 `freeShip=true` + `noCompanion=true`；必要时用 `line.unit` 覆盖标价，使 `paid_amount_minor` 与 fixtures 逐分一致（不改 fixtures）。
5. **LG_200 额外两单**：同客户增加 `ORD-20201`/`ORD-20202` 在途，满足「3 笔在途」话术；场景正文未强制点名这两单。
6. **ORD-20500/20501**：保留，不改号。
7. **CUST-10xx / ORD-100xx**：既有 seed 故事与会话未改动；IM 客户为 `CUST-2001..2051`，电话 `13900002xxx`。
8. **缺货**：`SKU-P015-04` stock=0，供 PP_228/TN_249 库存话术。
9. **DB 重建**：若 `bin/server` 占用主库，seed 写入 `data/ecommerce_support.new.db`（`DB_PATH=...`），**需停服后切换**到主库文件名；重建前备份到 `data/backup/<timestamp>/`，并用 ATTACH 恢复 JD 来源知识与非 seed 会话及相关行。
10. **专票税号**：演示税号使用 `TESTTAX…` 前缀以通过 validate。
11. **部分退款 RF_216**：两行 30+20 元，已成功退 30，第二行售后 `pending_review`。
12. **知识上限**：文章数顶满 40；未再塞草稿箱内容。

## 4. 验收要点

- `make scenarios` → 140 条（89 generated + 51 IM）
- `go test ./...` 通过
- `buildIMWorld` 已挂入 `build()`
- RT_211=`ORD-20012`；运单 MOCK 前缀
- 不提交 `.env`、`data/*.db`、backup 大库
