# Billing Review — 2026-10-27

## 0. 元信息

| 项 | 值 |
|---|---|
| 日期 | 2026-10-27（Asia/Shanghai） |
| HEAD | `64e749a` *fix: resolve functional audit findings across auth billing and workflows*（即本轮 remediation 提交） |
| 范围 | `internal/billing/**`（engine / strategy / calibration / startup_check / units）、`internal/storage` 计费路径（settlement.go、wallet_deduct.go、usage_spill.go、billing_reconcile_job.go、repo_logs_writer.go、repo_logs.go、repo_pricing.go、async_logger.go、migrations_*billing*）、`internal/controlplane/pricing_service.go`、`internal/api` 费用/用量解析与记账调用点（common.go `resolve*`/`recordUsage`/快照、stream drain、multimodal/video/audio/MCP） |
| 前序文档 | `docs/audit/billing.md`（F1–F17）、`docs/audit/functional-details-remediation-2026-10-27.md`、`docs/audit/README.md` |
| 约束 | 未改任何受控源码/`.env`；未重启或构建容器；DB 仅 SELECT；探针只在私有 worktree `/tmp/airoute-audit-billing`（已删除） |

### 执行过的测试与结果

| 命令 | 结果 |
|---|---|
| `go test ./internal/billing/ ./internal/storage/ -run 'Billing\|Settle\|Wallet\|Reconcile\|Spill\|Price\|Usage' -race -count=1` | **PASS**（billing 1.222s / storage 9.883s，0 失败） |
| 私有 worktree 探针 `go test ./internal/storage/ -run TestProbe -count=1 -v` | **PASS**，6 个探针均输出预期证据（见各条 Evidence） |
| 线上只读 SQL（`docker exec airoute-postgres psql`，仅 SELECT） | 见"线上基线" |
| 线上 `/metrics`（两副本，只读 curl） | 见"线上基线" |

### 线上基线（2026-10-27T01:4xZ，两副本重启后 ≈15 分钟）

| 指标 | 值 |
|---|---|
| `usage_logs` / `billing_settlements` | 51,066 / 50,633 |
| `cost>0` 行 / `SUM(cost_units)` | 48,618 / ¥548.87 |
| 无 `billing_id` 行（历史） | 29,107 |
| 重复 `billing_id`（usage / ledger） | 0 / 0 |
| `cost>0` 且 `cost_units=0`（F9 遗留） | **0**（已回填） |
| `cost>0` 且 `pricing_snapshot` 为空 | 12,610（历史，F17） |
| 重启后新行 / 无 `billing_id` 新行 | 790 / **0** |
| 499 行（总数 / 已收费） | 324 / 54；**近 2 天带 token 且 cost=0 的 499 = 0**（F3 修复生效） |
| `is_off_peak=1` 行 / 默认费率（¥2/¥8）行 | 0 / 2（且均 `off_peak_discount=1`，F5 修复生效） |
| `model_prices` 行 | 2，均 `CNY`、`off_peak_enabled=0`、`mode=none` |
| `airoute_billing_ledger_drift_units` | 2,647,492,000（= ¥2.647492，历史遗留，F9 已知） |
| `airoute_billing_audit_lag_units` / `wallet_drift_units` | 0 / 0 |
| `airoute_billing_estimate_bias_ppm` | 49,275（本地估值为上游的 ~95.1%，即原始 /4 启发式低估 4.9%，校准后补偿） |

---

## 1. 结论（TL;DR）

上一轮 `billing.md` 的 P0/P1 主体已真正修复并有回归测试：`billing_id` 幂等键 + 唯一索引、499 断连计费、透支钳制、默认价不再打折、非流式 usage 估算兜底、结算与审计解耦、对账指标。线上数据也证实（0 重复 billing_id、0 条"带 token 的 499 收 ¥0"、默认费率行仅 2 条且无折扣）。

本轮找到 **15 项细节级缺陷**，全部在"修复引入的新结构"内部，重点不是再调价格：

1. **结算阶段的一次校验错误会毒化整批记录**：外币快照让 `settleCharges` 整批返回错误 → 该批（≤100 条）**全部不落审计、全部不收费**，Redis PEL 永久卡住、无死信/隔离（B1）。这是本轮唯一的"结构性丢钱"缺陷。
2. **没有 `billing_id` 的旧格式记录（滚动发布窗口内的 Redis 载荷 / 旧版本 spill 文件）在每次投递时重新生成随机幂等键** → 重投递真的重复扣款（B2，已复现 10.00→8.00→6.00）。
3. **MCP 工具调用绕过 CNY 校验与定价快照**：`CalculateCostDetailedWithGroup` 丢弃 snapshot，`settleCharges` 只从 snapshot 校验币种，且 MCP 路径没有 `validateModelCurrency`、没有 usage 兜底 → 遗留外币价按 CNY 面值扣款 + 上游不报 usage 时免费（B3/B7）。
4. **对账语义被"保留期清理/手工删流水"破坏**：`auditLag = SUM(settlements) - SUM(usage_logs)`，删/清流水后永久为正且无法归零；而 29,107 条历史流水的 ledger 键是 `legacy-<id>`、`usage_logs.billing_id` 仍是 NULL，**二者 join 命中 0 行**（¥398.48，占总收入 72.6% 的账无法按行核对）（B8/B9/B10）。
5. **估算口径仍分裂**：Anthropic 适配流式的 completion 仍用 `chars/3`，其余路径统一 `chars/4` → 缺 usage 时**多收约 33%**（B6）；客户端 abort 的 prompt 估算走 dispatcher 的未校准版本（B13）。

资金方向判断：B1/B7 是**漏收**；B2/B3/B6 是**多收**（客户投诉/退款风险）。漏收在当前流量下金额有限，但 B1 的批量放大效应与 B2 的重复扣款在真实付费流量下会被放大。

---

## 2. 发现汇总表

| ID | 严重度 | 位置 | 标题 | 确认 | 资金影响 |
|---|---|---|---|---|---|
| B1 | **P1** | `internal/storage/settlement.go:74`、`repo_logs_writer.go:91`、`async_logger.go:322` | 一条外币快照记录毒化整批：整批不落库、不收费、PEL 永久卡住 | CONFIRMED（探针） | 每批 ≤100 条全部漏收；PEL 无隔离 |
| B2 | **P1** | `internal/storage/settlement.go:84`、`async_logger.go:310`、`usage_spill.go:212` | 无 `billing_id` 的旧记录每次投递重新生成随机键 → 重投递重复扣款 | CONFIRMED（探针） | 10.00→8.00→6.00，双倍扣款 |
| B3 | **P1** | `internal/api/mcp_tools_routing.go:218`、`internal/storage/settlement.go:74` | MCP 计费无定价快照：CNY 校验被绕过（外币按面值扣 CNY） | CONFIRMED（归还 nil 快照探针） | 遗留 USD 价 → 多收 ~7× |
| B4 | P2 | `internal/api/moderation_handler.go:84` | `/v1/moderations` 转发上游但零计费（F7 未修） | STATIC | 配 moderation 渠道后 100% 白送 |
| B5 | P2 | `internal/storage/settlement.go:236` | 无主 key/无 tenant 时扣款静默丢弃，且 ledger 仍记为"已结算"，无指标无日志 | CONFIRMED（探针） | 该笔 100% 漏收且不可见 |
| B6 | P2 | `internal/api/inbound_anthropic_stream.go:75` | Anthropic 适配流 completion 估算仍 `chars/3`（其余 `/4`）→ 多收 ~33% | STATIC（算术确定） | 缺 usage 时对客户多收 1/3 |
| B7 | P2 | `internal/api/mcp_tools_routing.go:197` | MCP 无 usage 兜底估算（F6 修复未覆盖 MCP） | STATIC | 上游不报 usage → 整条免费 |
| B8 | P2 | `internal/storage/migrations_billing_settlements.go:56` | 历史 ledger 键 `legacy-<id>` 而 usage 行 `billing_id` 仍 NULL，**join 命中 0 行**（线上 29,107 行 / ¥398.48） | CONFIRMED（线上 SQL） | 72.6% 收入无法按行对账/无法修复 |
| B9 | P2 | `internal/storage/billing_reconcile_job.go:199`、`usage_retention.go:101` | `auditLag` 语义被保留期清理/删流水单向破坏，且不可归零/不可重置 | STATIC + 线上基线 | 真 lag 被永久噪声掩盖 |
| B10 | P2 | `internal/storage/repo_logs.go:350`、`:375` | 三个删流水端点仍硬删，可删除 `cost>0` 行，不冲销账本（F9 建议未实施） | STATIC | 收入证据可被抹除 |
| B11 | P2 | `internal/api/multimodal_handler.go:58`、`video_handler.go:83` | 视频先收费后持久化任务：持久化失败返回 502 但已扣费且无任务 ID 可查 | STATIC | 客户被扣费却拿不到任务，重试二次扣费 |
| B12 | P3 | `internal/storage/usage_spill.go:160`、`:131` | spill 重放会重放并**删除其他副本的活跃 spill 文件**；容量检查统计其他副本文件 | CONFIRMED（探针删除他人文件） | 读取→删除窗口内追加的记录丢失 |
| B13 | P3 | `internal/api/common.go:383`、`internal/model/tokens.go:35` | abort 计费用 dispatcher 的**未校准** prompt 估算（不经过校准器） | STATIC | 与其它路径口径不一致，偏差≈bias 4.9% |
| B14 | P3 | `internal/api/inbound_anthropic_stream.go:388`、`:450` | 原生 Anthropic 流：断连记 200（非 499）、断连时 output 计 0、`cache_creation` 按 prompt 价（F12 未修） | STATIC | 断连少收；cache write 少收 25% |
| B15 | P3 | `internal/billing/calibration.go:96` | `estimate_bias_ppm` 是"最后一条观测的模型"的比率，非全局/非带标签，跨模型互相覆盖 | STATIC | 运维指标误导（非资金） |

---

## 3. 逐项详情

### B1 · P1 · 一次校验错误毒化整批记录：整批不落库、不收费、PEL 卡死

**触发**：任意一条记录在 `settleCharges` 里触发 `ValidatePriceCurrency` 失败（`PricingSnapshot.Currency != "CNY"` 且 `Cost>0`）。最现实的来源是 B3 的"已提供模型价目来自遗留外币行"（`billingModelFor` 会切到 served model 的费率）。

**代码路径**

- `internal/storage/settlement.go:74-78`：币种校验在 `BeginTx` **之前**、对整个 `logs` 切片循环执行，任一失败 `return err` —— 整批直接放弃，健康记录也不处理。
- `internal/storage/repo_logs_writer.go:91-96`：`settleCharges` 失败即 `return err`，**审计行插入根本不会执行**（顺序是"钱先、账后"）。
- `internal/storage/async_logger.go:320-338`：Redis 消费者把 `BatchRecordUsageLogsContext` 的失败当作"整条消息批次失败"，`return false` → `ackIDs` 全部不 ACK，批次留在 PEL；30s 后 AutoClaim **永久重试同一条毒记录**，无死信、无隔离、无跳过。
- `internal/storage/async_logger.go:228-236`：本地队列路径连续 5 次失败后把整批写入 spill，下次启动 replay 再次失败 → 文件永久保留。
- 批次大小确认为 **100**（`cmd/airoute/server.go:161` → `InitAsyncLogger(repo, 10000, 100, 500ms)`，Redis 侧 `ReadGroupLogs(count=batchSize)`）。

**Evidence（探针，`/tmp/airoute-audit-billing`，已删除）**

```
poison batch: err=当前钱包仅支持 CNY 定价；请明确换算后以 CNY 保存 audit_rows=0 settlements=0 balance=10 (start 10.00)
after 5 retries: audit_rows=0 settlements=0 (healthy rows still unpersisted, healthy charges uncollected)
without the poison record the same healthy rows persist immediately: audit_rows=2 settlements=2
```

即：2 条健康的 ¥1.00 记录 + 1 条外币记录 → 整批 0 落库 0 扣款；移除毒记录后同一批立刻成功。

**影响**：单条毒记录可让同一批最多 100 条真实收入既不收费也不留痕，且 PEL 永久膨胀、每 30s 刷 ERROR 日志；运维只能手工 `XACK`/清 PEL 才能恢复。
**建议修复**：`settleCharges` 对"确定性校验错误"（外币/非法快照）逐条记录 `billing_settlements`（0 元）+ `BillingRejected` 指标 + WARN 后**跳过该条**，只对真正的瞬时错误返回 error；消费端把"确定性失败"与"可重试失败"区分开，前者 ACK 进死信、后者留 PEL。同时把币种校验提前到请求准入（对 served model 也校验，见 B3）。

---

### B2 · P1 · 无 `billing_id` 的旧记录每次投递都重新生成幂等键 → 重复扣款

**触发**：记录进入管道时没有 `BillingID`。`AsyncLogger.Record` 正常会补一个（`async_logger.go:112-114`），但**已经入队的旧格式载荷**（上一版本二进制写入 Redis 的 JSON、或旧版本写下的 spill `.jsonl`）没有该字段；消费者每次 `json.Unmarshal` 出全新结构体，`BillingID == ""`。

**代码路径**

- `internal/storage/settlement.go:84-86`：`if log.BillingID == "" { log.BillingID = NewBillingID() }` —— **在结算阶段**按每次投递生成随机键，而不是在入队/消费边界按消息身份生成确定性键。
- `internal/storage/async_logger.go:310-317`：每次 `processStreamMessages` 都重新反序列化 → 新的空 `BillingID`。
- `internal/storage/usage_spill.go:207-221`：spill replay 同样每次重新反序列化；且 `replayFile` 在某批 flush 失败时**保留整个文件**，下次启动会把已成功的行再放一遍 → 旧格式行走第二条 B2 路径。

**Evidence（探针）**

```
legacy no-key record: balance after 1st=8000000000 after 2nd=6000000000 (want 8000000000); audit rows=2 settlements=2
CONFIRMED: redelivery of a record without billing_id charged again: 8.00 -> 6
```

**线上现状**：采样时 `XLEN airoute:stream:logs` 仅剩 4 条消息且**全部**带 `billing_id`；重启后 790 条新行中无 `billing_id` 的 = **0**。因此这是**滚动发布的短暂窗口 + 旧版本 spill 文件**风险，非当前持续发生；但窗口内的一次 XACK 失败/AutoClaim 就是真实重复扣款，且事后无法区分。
**建议修复**：在消费/重放边界（Redis `msg.ID`、spill 文件行号）派生确定性键（如 `billing_id = "stream:" + msgID` / `"spill:" + file + ":" + lineNo`），并禁止 `settleCharges` 为"已入队记录"现场生成随机键（改为返回错误或按内容哈希）。

---

### B3 · P1 · MCP 计费没有定价快照：CNY 校验被整体绕过

**触发**：`airoute_chat` MCP 工具走到 LLM 调度时，费用用 `CalculateCostDetailedWithGroup` 计算（**不返回 snapshot**），随后直接 `Record(... Cost: cost ...)`。而 CNY 校验的唯一实现点是 `settleCharges` 的 `if log.PricingSnapshot != nil && log.Cost > 0`。

**代码路径**

- `internal/api/mcp_tools_routing.go:218`：`cost, _, _, isOffPeak, offPeakDiscount = ...CalculateCostDetailedWithGroup(...)` → snapshot 丢弃（`engine.go:378-381` 明确丢弃）。
- `internal/api/mcp_tools_routing.go:223-241`：构造 `UsageLogRecord` 时不设 `PricingSnapshot`。
- `internal/storage/settlement.go:74-78`：`PricingSnapshot == nil` → **不做币种校验**，直接按 CNY 扣 `Cost`。
- 对照：HTTP 路径用 `CalculateCostForUnits`（`common.go:120-128`）并把 snapshot 写进审计行（`common.go:186`），所以校验生效。
- 另外 MCP 路径**没有** `validateModelCurrency` 准入（对照 `handler.go:55` 等 13 处调用点），所以连"请求前拒绝"都没有。

**Evidence（探针）**

```
with snapshot: rejected as designed: 当前钱包仅支持 CNY 定价；请明确换算后以 CNY 保存
CONFIRMED: a cost-only record without a snapshot was charged unconditionally: balance 10.00 -> 5
```

线上 `model_prices` 当前 2 行均为 CNY，但迁移说明明确"已有 USD 等价格不会自动换算或改标币种"——一旦库里存在遗留外币行（本次修复的正是这种数据），MCP 路径会按 USD 数值当 CNY 扣款（≈7× 多收），而且审计行没有 snapshot，事后无法解释账单。
**建议修复**：MCP 路径改用 `CalculateCostForUnits` 并保存 snapshot；在 dispatch 前调用 `validateModelCurrency(c, chatReq.Model)`，并对 served model 的解析结果也做一次 `ValidatePriceCurrency`。

---

### B4 · P2 · `/v1/moderations` 仍零计费（上一轮 F7 未修复）

**触发**：运营给任一渠道勾选 `moderation` 协议后，`/v1/moderations` 会把请求转发上游并直接回包，全程没有 `RecordUsage`/`AsyncLogger.Record`。

**代码路径**：`internal/api/moderation_handler.go:84-96`（`DispatchHTTP` → `c.Data(...)` 返回）；路由 `internal/api/router.go:258` 与根别名 `:296`。全文件 grep `RecordUsage|AsyncLogger|FixedPrice` = 0 命中。
**Evidence**：静态；线上当前 `moderation_rows = 0`（未配置该协议渠道），故为潜伏。
**影响**：配置即 100% 白送上游成本；且没有 `pricing_not_configured` 守卫（对照 `multimodal_handler.go:38-44`）。
**建议修复**：在 2xx 分支补一次按次计费（`BillableUnits: 1`）并要求该模型 `fixed_price > 0`。

---

### B5 · P2 · 扣款静默丢弃，且 ledger 仍记为"已结算"（无指标、无日志）

**触发**：`api_keys` 中不存在该 key（历史孤儿 key、`mcp-session` 之类），且记录没有 `TenantID`（或 tenant 匹配不到非 admin 用户）。

**代码路径**：`internal/storage/settlement.go:236-257`：`deductWallet` 返回 `applied=false`（子查询 `user_id` 为 NULL）→ 非 admin → `if tenant := keyTenant[key]; tenant != ""` 才进兜底；tenant 为空则**直接跳过，无返回值、无计数、无日志**。而该记录的 `billing_settlements` 行**已经在事务里插入**（`:101`），即 ledger 宣称"已结算"，实际零扣款。
**Evidence（探针）**

```
key deleted: balance=10 (10.00 = not charged), ledger_rows=1; with tenant id the fallback charges it: balance=7
```

线上 `/metrics` 不存在任何 `unattributed`/`billing_rejected` 计数器（`grep -c unattributed` = 0），上一轮 F8 建议的"应扣未扣必须发声"未实施。
**影响**：该笔 100% 漏收且完全不可观测；同时 `ledgerDrift` 以 ledger 为"已收"基准，会把这类记录算作已收，掩盖缺口。
**建议修复**：`applied==false` 且无 tenant 兜底命中时 `BillingUnattributed.Add(1)` + WARN(trace_id, cost)，并不写 ledger 行（或写 0 值并标记 `unattributed`）。

---

### B6 · P2 · Anthropic 适配流式 completion 估算仍用 `chars/3`（多收 ~33%）

**触发**：上游 OpenAI 兼容流没有回 usage 帧（`upstreamUsageSeen == false`）且有输出字符时。

**代码路径**：`internal/api/inbound_anthropic_stream.go:70-81`：

```go
finalizeCompletion := func() {
    if upstreamUsageSeen { return }
    if accumulatedCompChars > 0 { totalCompTokens = accumulatedCompChars / 3 ... }
}
```

`finalizeCompletion()` 仅在 `:207` 被调用；随后 `recordStreamEnd`（`:110-113`）只在 `totalCompTokens == 0` 时才用 `estimateTokens`（`/4`），因此 **`/3` 的结果就是计费值**，`:110-113` 的 `/4` 分支实际上是死代码。`common.go:406-413` 明确写了"单一估算口径"的设计意图。

**Evidence**：静态算术——同一段 300 字符输出，`/3` = 100 tokens，`/4` = 75 tokens，差 33.3%；线上 `estimate_bias_ppm=49,275` 证明 `/4` 路径已被校准器对齐（+4.9% 修正），而 `/3` 路径不经过 `calibratedPromptEstimate`/`Adjust`，误差无法自我修正。
**影响**：仅在上游缺 usage 时发生，方向是**对客户多收三分之一**的 completion 费用。
**建议修复**：`finalizeCompletion` 改为 `totalCompTokens = calibratedCompletionEstimate(...)`（或至少 `estimateTokens(accumulatedCompChars)`），并删除 `:110-113` 的死分支。

---

### B7 · P2 · MCP 调用无 usage 兜底：上游不报 usage 即免费

**触发**：MCP `airoute_chat` 的 `resp.Usage == nil`（或全 0），而 HTTP 路径已有 `resolveChatUsage` 兜底。

**代码路径**：`internal/api/mcp_tools_routing.go:195-205`：`pTokens/cTokens/cachedTokens` 仅在 `resp.Usage != nil` 时赋值，否则保持 0 → `:218` 计算 cost=0 → `:241` 写一条 cost=0 的审计行。对照 `internal/api/common.go:466-491`（`resolveChatUsage`，含 prompt 校准估算 + completion 从 choices 估算 + `usage_source=estimated`）。
**Evidence**：静态；线上 `mcp_rows = 0`（MCP 工具尚未被真实调用），上一轮 F6 实测该上游行为曾产生 148 条 HTTP-200 收费 ¥0。
**影响**：同一类"上游省略 usage"的行为在 MCP 路径上会 100% 免单，且审计行显示 token=0，无法事后发现。
**建议修复**：MCP 路径直接复用 `resolveChatUsage(chatReq, resp)`，并写 `UsageSource`。

---

### B8 · P2 · 历史流水的 ledger 键 `legacy-<id>` 与 `usage_logs.billing_id`（NULL）不可 join

**触发**：迁移 v21 加 `billing_id` 列（历史行 NULL），迁移 v23 建 `billing_settlements` 并回填：现代行走 `billing_id`，历史行走 `'legacy-' || id`——但**没有回填 `usage_logs.billing_id`**。

**代码路径**：`internal/storage/migrations_billing_settlements.go:56-64`（legacy 回填），`internal/storage/migrations_usage_billing_id.go:12-14`（注释明确"legacy rows keep NULL，无需回填"）。
**Evidence（线上 SQL）**

```
legacy_settlement_rows | 29107
usage_null_billing_id  | 29107
legacy_rows_joinable   | 0        ← 按 billing_id join 命中 0 行
legacy_ledger_cost_cny | 398.4772 ← 占总收入 548.87 的 72.6%
```

连带后果：`repairSettlementGap`（`billing_reconcile_job.go:155-163`）的 `l.billing_id IS NOT NULL AND <> ''` 过滤把历史行全部排除，`NOT EXISTS(...)` 对它们永远不成立——该修复机制对这 72.6% 的收入完全无效；`auditLag` 之所以现在为 0，只是因为两边总额恰好相等（回填时逐行复制），并非 join 成功。
**建议修复**：迁移补一条 `UPDATE usage_logs SET billing_id = 'legacy-' || id WHERE billing_id IS NULL OR billing_id=''`（先于 ledger 回填/或在 repair 里按同一表达式 join），并让 repair 的 `NOT EXISTS` 用 `COALESCE(l.billing_id, 'legacy-'||l.id)`。

---

### B9 · P2 · `auditLag` 语义被保留期清理/删流水单向破坏且不可归零

**触发**：`UsageLogRetention`（默认 365 天，`GATEWAY_USAGE_LOG_RETENTION_DAYS=0` 才关闭）或管理员删流水后，`usage_logs` 行消失但 `billing_settlements` 保留。

**代码路径**：`internal/storage/billing_reconcile_job.go:199-210`：

```sql
SELECT COALESCE((SELECT SUM(cost_units) FROM billing_settlements),0)
     - COALESCE((SELECT SUM(cost_units) FROM usage_logs),0)
```

`internal/storage/usage_retention.go:101-118` 只删 `usage_logs`；`internal/storage/repo_logs.go:350/356/375` 的三个删除端点同样只删 `usage_logs`，都不动 ledger、不留基线。
**Evidence**：静态 + 线上基线（当前 `audit_lag_units=0` 仅因尚未触发 365 天清理/删除；`ledger_drift_units=¥2.6475` 则是历史硬删的证据）。
**影响**：一旦开启清理（或有人删过流水），`audit_lag` 会等于"被删除流水的**全部历史金额**"且永不回落，`billing-audit-lag` WARN 会永久每 6h 触发 → 真正的 lag 被噪声淹没；也无法区分"删了流水"与"漏写了流水"。
**建议修复**：维护 `billing_pruned_units` 基线（清理/删除时累加被删行的 `cost_units`），`auditLag = SUM(settlements) - (SUM(usage_logs) + pruned_units)`；或让清理同时删除对应 ledger 行（但需先决定 ledger 是否属于"资金凭证"）。

---

### B10 · P2 · 三个删流水端点仍硬删，可删除 `cost>0` 行且不冲销账本（F9 建议未实施）

**代码路径**：`internal/storage/repo_logs.go:349-353`（单条）、`:355-372`（批量）、`:374-383`（清空）；上游端点为管理接口（`router.go:236-238`，上一轮已记录）。均无 `deleted_at`、无 `cost>0` 守卫、无账本调整。
**Evidence**：静态 + 线上 id 空洞对应 ¥2.6475 账实差（上一轮 F9 已量化，本次 `ledger_drift_units` 仍为 2,647,492,000）。
**影响**：运营可一键抹除已收费流水的收入证据，而 key `used_cost_units` 与 ledger 不变 → 账实差永久化、退款争议无法裁决。
**建议修复**：改为软删除；或拒绝删除 `cost_units <> 0` 的行（与 F9 建议一致），并向 `billing_pruned_units` 累加。

---

### B11 · P2 · 视频先扣费后持久化任务：持久化失败 = 已扣费但拿不到任务

**触发**：`/v1/videos/generations` 上游 2xx，但 `SaveVideoTask` 失败（DB 抖动/唯一冲突）或 `writeVideoResponse` 失败。

**代码路径**：`internal/api/multimodal_handler.go:56-66` 先 `RecordUsage(...)`（并同步结算扣款），`:75-80` 才执行 `transform`（`video_handler.go:74-89` 里 `SaveVideoTask` + 重写响应 ID）；transform 失败 → 返回 502。
**Evidence**：静态（顺序确定）。
**影响**：客户被扣一次固定价、收到 502、且没有网关任务 ID 可轮询（上游任务仍在计费）；客户端重试 = 第二次扣费。资产型请求"失败仍收费"缺少补偿/幂等回执。
**建议修复**：把 `RecordUsage` 移到 transform 成功之后（或对 transform 失败在同一请求内做补偿冲正），至少保证"返回 2xx 的请求才收费"。

---

### B12 · P3 · spill 重放会消费并删除其他副本的活跃 spill 文件

**触发**：两副本共享 spilling 目录（`usageSpill.currentSizeLocked` 注释明确"共享卷"），`replay()` 遍历**所有** `usage_spill-*.jsonl` 并逐个 `os.Remove`，不区分是否本进程所有。

**代码路径**：`internal/storage/usage_spill.go:160-177`（遍历 + 删除）、`:69-77`（文件名含 hostname+pid+纳秒）、`:131-148`（容量统计把其他副本文件也算进自己的 256MB 配额）。
**Evidence（探针）**

```
replica A replayed=1; replica B's live spill file still exists=false
CONFIRMED: replica A consumed and deleted replica B's live spill file usage_spill-b-2.jsonl
B's next append re-created the file: written=1 err=<nil>
```

**影响**：A 读完 B 的文件到 `os.Remove` 之间 B 追加的记录会丢失（B 下次 append 因 `O_CREATE` 重建空文件）；此外一个副本的积压会因共享容量统计让**所有**副本一起禁用 spill（回到丢记录）。
**建议修复**：只重放本进程/已退出进程（pid 不存活）的文件；删除前用 `.replayed` 改名/租约；容量按每副本配额计算。

---

### B13 · P3 · abort 计费的 prompt 估算绕过了校准器

**触发**：非流式请求在 dispatch 阶段被客户端取消（499），`recordFailedRequestWithError` 用 `router.DispatchError.PromptTokens` 计费。

**代码路径**：`internal/api/common.go:383-389`（取 `de.PromptTokens`）← `internal/router/dispatcher.go:241`（`req.EstimatedPromptTokens()`）← `internal/model/tokens.go:35-49`（只做 `chars/4`，**不调用** `GlobalCalibrator.Adjust`）。
**Evidence**：静态；线上 `estimate_bias_ppm=49,275` 表明真实上游 tokenizer 比本地 `/4` 多 4.9%，即该路径系统性低估约 4.9%（方向：少收）。
**建议修复**：在 `recordFailedRequestWithError` 里对 `de.PromptTokens` 再走一次 `calibratedPromptEstimate(modelName, ...)`（或让 dispatcher 的估算函数使用校准器）。

---

### B14 · P3 · 原生 Anthropic 流的三个计费细节

1. **断连记 200 而非 499**：`internal/api/inbound_anthropic_stream.go:388-407` 的 defer 只用 `streamErr` 决定 502，不检查 `c.Request.Context().Err()`；客户端主动断开被记为成功（对照适配路径 `:131-143` 有 499 判定）。
2. **断连时 output 计 0**：同一 defer 直接用 `outputTokens`，而 `message_delta`（含 `output_tokens`）通常在断连后才到 → 断连请求只收 prompt 费（少收）。
3. **`cache_creation` 按 prompt 全价**：`:391` 把 `cache_creation_input_tokens` 并进 `totalPrompt`，而 `CachedTokens` 只传 `cachedReadTokens`（`:399`）；Anthropic 对 cache write 收 1.25× → 该部分少收 20–25%（上一轮 F12 未修，`provider/usage_mapping.go:14` 同样并入 prompt）。
**Evidence**：静态；线上无原生 Anthropic 渠道（`channels.protocols` 无 `anthropic_messages`），故尚未发生。
**建议修复**：defer 内补 `ctx.Err() != nil → 499` 并对已产生的 output 做估算；新增 `CacheWritePrice`（或 `CacheReadPrice × 1.25`）单独计价 `cache_creation`。

---

### B15 · P3 · `estimate_bias_ppm` 是"最后一条观测模型"的比率，跨模型互相覆盖

**代码路径**：`internal/billing/calibration.go:94-97`：每次 `Observe` 都 `GlobalMetrics.EstimateBiasPPM.Store((sample.ratio-1)*1e6)`，`sample` 是本次观测的模型；无 model 标签、非全局均值。
**Evidence**：静态；线上单值 49,275 无法区分模型。上一轮 metrics 里也未见该指标的文档说明。
**影响**：运维用它判断"估算偏差"会被最后一条流量带偏（多模型/多副本时尤其明显），属可观测性缺陷而非资金错误。
**建议修复**：改为带 `model` 标签的 Gauge/Counter，或发布全局样本加权均值。

---

## 4. 上一轮发现复核（`docs/audit/billing.md`）

| 上轮 | 结论 | 证据 |
|---|---|---|
| F1 无幂等键 → 重放重复扣款 | **已修（主体）** | `billing_id` 列 + 唯一索引（`migrations_usage_billing_id.go:28`）、`ON CONFLICT (billing_id) DO NOTHING`（`repo_logs_writer.go:172/195`）、`billing_settlements` 唯一键（`migrations_billing_settlements.go:39`）；线上重复 `billing_id` = 0/0；`TestBatchRedeliveryIsBilledOnce` 通过。**残留：B2（无键旧记录）、B1（毒批）** |
| F3 499 断连零计费 | **已修** | `common.go:104-118`（499 且 tokens>0 → 计费）+ 5 个流式 handler 的 drain/估算；线上近 2 天"带 token 且 cost=0 的 499" = **0**，324 条历史 499 中 54 条已收费。**残留：B14（原生 Anthropic 断连仍记 200/少收）** |
| F4 无透支钳制 | **已修** | `wallet_deduct.go:20-35,40,63-65`（SQL 层 floor + `BillingOverdrafts` + WARN）；`wallet_overdraft_test.go` 4 例通过；线上 `overdrafts_total=0`。**残留（非资金）：`quota_tracker.go:20` 预留仍固定 ¥0.0005，未按预估成本预留** |
| F5 默认价 + 默认 5 折 | **已修** | `engine.go:58-68` 默认记录 `OffPeakEnabled:false`；`startup_check.go` 启动自检未配价模型；线上近 7 天 36,168 行按 ¥1/¥4、仅 2 行为 ¥2/¥8 且 `off_peak_discount=1`、`is_off_peak=0` |
| F6 无 usage → 0 计费 | **已修（HTTP 全路径）** | `common.go:466-531`（`resolveChatUsage`/`resolveEmbeddingTokens`/`resolveRerankTokens`）+ 各 unary handler 调用；线上 `usage_source=estimated` 76 行。**残留：B7（MCP 路径未覆盖）** |
| F7 `/v1/moderations` 零计费 | **未修** | 见 B4 |
| F8 MCP/孤儿 key 应扣未扣无告警 | **部分修** | 扣款逻辑同前，但**未加** unattributed 指标/日志（`/metrics` 无此计数器）；tenant 兜底成功后仍不校验 `RowsAffected`。见 B5 |
| F9 账本/流水不可对账 | **部分修** | `cost_units` 已回填（线上 `cost>0 且 cost_units=0` = **0**）、新增 `BillingReconciler`；但**硬删仍存在（B10）**、**历史行 ledger 不可 join（B8）**、`auditLag` 语义会被清理破坏（B9）；`cost_nano/used_cost_nano` 列仍在但**全仓已无代码读取**（grep 0 命中，无害） |
| F10 按请求模型计费 | **已修** | `common.go:61-80`（`billingModelFor` + served model + 仅当 served 有显式价才切换）+ snapshot 记录实际 model；`startup_check.go:41-71` 上报未配价 served model |
| F15 批内取整口径不一致 | **已验证无漂移** | `engine.go:462-478`（统一 `roundPrecision` 1e-8 → 每行 cost 必为 10 个纳元整数倍）+ `settlement`/`usage_logs` 逐行 `ToUnits`；探针 `per-row units=2501814670 aggregate units=2501814670 equal=true` |

---

## 5. 已验证正确、**不要**误改的部分

| 检查项 | 结论 | 证据（本轮复核） |
|---|---|---|
| 幂等结算（同 `billing_id` 三重投递） | 正确 | `settlement_test.go:70`、`TestBatchRedeliveryIsBilledOnce` 通过；线上重复键 0 |
| 同步结算 → 审计写失败仍不重复扣款 | 正确 | `settlement_test.go:21`（`SettleUsageCharge` 后再 `RecordUsageLogContext` 余额不变） |
| 结算与审计解耦方向 | 正确（收益：日志管道故障不再漏收） | `RecordUsageLogContext` 先 `settleCharges` 再插行；`SettleChargeNow` 在 `common.go:204-208` 每请求调用，线上 `settled_synchronously_total` 411（airoute-1）/ 433（airoute-2），重启后累计 |
| CNY 准入守卫（HTTP 路径） | 正确且覆盖完整 | 13 个 handler 调用 `validateModelCurrency`（chat/completions/responses/messages(+count_tokens)/gemini/images/audio×3/video/embeddings/rerank）；`pricing_service.go:159-180` 与 `route_service.go:247` 双写入口都校验 |
| 透支钳制 SQL（双镜像） | 正确 | `wallet_deduct.go:40` 同一表达式派生 float，nano/float 不会漂移；线上 `wallet_drift_units=0` |
| 引擎算术 / cached 拆分 / 分组乘数只乘一次 | 正确（与上轮一致） | `engine.go:408-438`；`units_test.go` 通过 |
| 通配符最长前缀 + 无重入锁 | 正确 | `engine.go:105-126,302-329` |
| 聚合取整 = 逐行取整 | 正确 | 见 F15 行；探针 |
| `usage_logs`/`api_keys` 的 `cost_units` 回填 | 正确 | 线上 `cost>0 且 cost_units=0` = 0 |
| 对账 worker 不改账 | 正确 | `billing_reconcile_job.go:100-143` 纯查询 + repair 只补 ledger 行，不动余额；`TestReconcilerRepairsAuditRowsWithoutSettlement` 通过 |
| `usage_logs.billing_id` 唯一索引 | 正确（NULL 不冲突） | `migrations_usage_billing_id.go:26-28`：新行必带键，线上 0 重复 |

---

## 6. 建议优先级

1. **B1（P1）**：`settleCharges` 逐条隔离确定性校验失败 + 消费端区分"毒记录/可重试"——这是当前唯一会成批丢收入的缺陷。
2. **B2（P1）**：为"入队时无 `billing_id`"的记录派生**确定性**键（Redis msgID / spill 行号），杜绝滚动发布窗口重复扣款。
3. **B3+B7（P1/P2）**：MCP 路径改用 `CalculateCostForUnits`（带 snapshot）+ `validateModelCurrency` + `resolveChatUsage`，一次性对齐 HTTP 路径的币种/快照/估算三重保障。
4. **B4（P2）**：`/v1/moderations` 补计费（否则"配置即白送"）。
5. **B8/B9/B10（P2）**：回填 `usage_logs.billing_id='legacy-'||id`；删除/清理改软删 + `pruned_units` 基线，让对账指标重新有意义。
6. **B6/B11/B14（P2/P3）**：统一估算口径（`/3` → `/4`+校准）、视频先持久化后收费、原生 Anthropic 流补 499 与 cache-write 计价。
7. **B5/B12/B13/B15（P2/P3）**：补 unattributed 指标、spill 按副本隔离、abort 估算走校准、bias 指标带 model 标签。

---

## 附录 A · 复现命令（全部只读 / 私有 worktree）

```bash
# 私有 worktree（审计后已删除）
git -C /home/ifnodoraemon/myagent/airoute worktree add --detach /tmp/airoute-audit-billing HEAD
# 探针文件（仅存在于 worktree）：internal/storage/zz_probe_test.go
export PATH=/usr/local/go/bin:$PATH
cd /tmp/airoute-audit-billing
go test ./internal/storage/ -run 'TestProbe' -count=1 -v

# 目标测试（主仓，未改动）
cd /home/ifnodoraemon/myagent/airoute
go test ./internal/billing/ ./internal/storage/ -run 'Billing|Settle|Wallet|Reconcile|Spill|Price|Usage' -race -count=1

# 线上只读证据
docker exec airoute-postgres psql -U airoute -d airoute -At -c "
select (select count(*) from usage_logs) usage_rows,
       (select count(*) from billing_settlements) ledger_rows,
       (select count(*) from usage_logs where billing_id is null or billing_id='') null_ids,
       (select count(*) from billing_settlements s where not exists
          (select 1 from usage_logs l where l.billing_id = s.billing_id)) orphan_ledger;"
```

## 附录 B · 本轮未能确认的事项

1. **B1/B3 的线上可达性**：需要 `model_prices` 中存在外币行。线上当前 2 行均为 CNY，故 B1/B3 的"外币触发"只能通过静态代码路径 + 探针证明，**没有线上样本**。
2. **B2 的真实重复扣款次数**：需要"旧格式载荷被重投递"，线上 stream 现存 4 条消息均带 `billing_id`，无法从数据判断窗口内是否发生过。
3. **B9 的实际影响**：线上尚未产生 365 天前的数据（最早 2026-10-19），清理 worker 未真正删过行，因此 `auditLag` 目前仍为 0。
4. **原生 Anthropic 路径（B14）**：线上无 `anthropic_messages` 渠道，无实测样本。
