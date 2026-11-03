# 数据完整性 Bug 排查 C：存储 / 计费 / 遥测（2026-10-28 bughunt）

> 范围：`internal/storage`（SQL/迁移/事务/结算分组提交/ledger/保留清理/FTS/分区）、`internal/billing`（价格解析、单位换算、缓存写入溢价、分时折扣、校准、快照）、`internal/telemetry`（并发安全、分位数、Prometheus 导出）。
> 口径：**真实缺陷**（不是「设计断裂」——那部分见 `docs/audit/2026-10-28-inconsistencies/README.md` §3/§6/§8.3，本文不重复）。
> 基线：`381926d` + 工作区未提交改动；`go build ./...`、`go vet ./...`、`gofmt`、`go test ./...`（16 包）在加入探针后仍全绿。
> 方法：只读审计 + 新增 `*_bugprobe_test.go` 探针（SQLite 内存库/PG 无本机实例的路径按方言代码推断并注明）。生产代码零改动。

---

## 1. Findings 总表

| ID | 严重度 | 置信度 | 域 | 位置 | 一句话 |
|---|---|---|---|---|---|
| BUG-DATA-01 | **P1**（金额，双扣） | 高（机制已实测）/ 中（线上触发面） | storage/settlement | `settlement.go:231-246` vs `:274-293` | 同一结算批次内出现两条相同 `billing_id` 时，ledger 只记 1 行，但钱包/计数器被扣 2 次 |
| BUG-DATA-02 | P2（金额，错算） | 中高 | billing/off-peak | `strategy.go:77-116`、`strategy_validation.go:69-77` | 跨零点时段 + `days` 过滤把「后半夜」算到错误星期：每天 6 小时该打折未打、另一段 6 小时多打折 |
| BUG-DATA-03 | P2（配置校验漏） | 中高 | billing/off-peak | `strategy_validation.go:182-190`、`:162-169` | 非法时分被静默解析成 00:00，校验通过：`"8:30pm"`→08:00、`"abc"`→00:00 |
| BUG-DATA-04 | P2（并发 UB） | 高（`-race` 实测） | billing/calibration | `calibration.go:112-118` vs `:75-80` | `Adjust` 在释放读锁后读 `ratioSample` 字段，与 `Observe` 的写形成数据竞争 |
| BUG-DATA-05 | P2（基数/内存） | 中高 | telemetry | `metrics.go:314-321`、`metrics_prometheus.go:154-167` | 按 model 的 estimate-bias 指标**无上限**，而兄弟指标有 500 上限；客户端可影响的 model 串会无界增长 map 与 Prometheus label |

已排除项见 §4，低置信/未闭环项见 §5。

---

## 2. 详情

### BUG-DATA-01（P1）结算批次内重复 `billing_id` → 钱包双扣、计数器虚增

**位置**
- 聚合扣费循环：`internal/storage/settlement.go:231-246`（`for _, log := range pending`，以 `settledIDs[log.BillingID]` 判定「新插入」后累加 `keyCost/keyTokens`）
- 插入去重：`internal/storage/settlement.go:274-293`（`insertSettlementRows` 用 `seen` map 跳过同批次重复行，`ON CONFLICT (billing_id) DO NOTHING RETURNING billing_id`）

**触发条件**
同一调用 slice 内出现 ≥2 条 `BillingID` 相同的记录，经任一「批量结算」入口：
- `Repository.settleCharges` / `BatchRecordUsageLogsContext`（`repo_logs_writer.go:238`，异步 logger 的 flush、Redis Stream 消费者 `async_logger.go:343`、spill 回放 `usage_spill.go:284` 都走它）；
- 或 `SettleUsageCharge` 收到重复记录。

即「at-least-once 管道把同一条记录投递了两次，且两次落在同一个批次」。此时：`insertSettlementRows` 的 `seen` 去重只插入 1 行 ledger；但随后对账循环遍历的是**未去重的 `pending`**，两条记录都能在 `settledIDs` 里查到，于是各自累加一次 cost/tokens。

**根因**
去重发生在「写 ledger」这一层，而不是「做金额聚合」这一层；`settledIDs` 是 `map[string]struct{}`，丢失了「这条 id 已经被处理过一次」的消费语义。聚合阶段应基于去重后的「首次出现的记录列表」，或用一个 `processed map[string]struct{}` 再跳过。

**影响**
- 真实资金：客户钱包被多扣一次（双扣），且 `api_keys.used_cost_units` / `used_tokens` 比 `SUM(billing_settlements.cost_units)` 多一份 → 之后 `BillingReconciler.ledgerDrift()`（`billing_reconcile_job.go:262-276`）报 drift，但钱已经扣了，且 ledger 本身看不出问题（ledger 是「正确」的一行）。
- 与既有承诺冲突：`settlement.go:70-75` 注释明确「the unique billing_id makes a batched settlement and a pipeline settlement of the same record apply exactly once」。批内重复是该承诺的漏洞。
- 现有测试不覆盖：`TestBatchRedeliveryIsBilledOnce` 只测「跨批次重投递」（第二次投递时 ledger 已有该 id，故不扣），未测「同批次内重复」。

**证据（探针，本机 SQLite 内存库实测通过）**
`internal/storage/settlement_dup_bugprobe_test.go`
```
go test ./internal/storage/ -run TestBugProbeSettlementDuplicateBillingIDChargesTwice -v
--- PASS: TestBugProbeSettlementDuplicateBillingIDChargesTwice
```
断言的是**当前错误行为**（修复后需翻转断言，注释已写明正确值）：
| 观测 | 当前（buggy） | 正确 |
|---|---|---|
| `billing_settlements` 行数 | 1 | 1 |
| `users.balance_units`（初始 10 CNY，两笔各 2 CNY） | **6_000_000_000**（扣 4） | 8_000_000_000（扣 2） |
| `api_keys.used_cost_units` | **4_000_000_000** | 2_000_000_000 |
| ledger `SUM(cost_units)` | 2_000_000_000 | 2_000_000_000 |
| `api_keys.used_tokens` | **2_000_000** | 1_000_000 |

**建议修复**（不改语义，仅去重粒度）
在 `settleChargesOnce` 里构造 `pending` 时同步维护 `seenIDs`，同 id 只保留第一条；或聚合循环改为遍历 `insertSettlementRows` 返回的（去重后）记录顺序。

**备注（记账归属）**：分组提交（`settlement_batcher.go`）本身在正常路径下是安全的——批内每条记录 `BillingID` 由 `settleUsage` 预先钉住（`api/common.go:392-393`），请求侧超时后由审计管道重投、靠 ledger 唯一键幂等；`Stop()` 竞态只会让调用方 300ms 超时并计入 deferred，不会丢单。**本条的根因在聚合层，不在 batcher 的分组/重放逻辑**。

---

### BUG-DATA-02（P2）跨零点优惠时段 + `days` 过滤：星期归属错误（每天 6h 错价）

**位置**
- 引擎匹配：`internal/billing/strategy.go:77-116`（先按 `ctx.ISODay` 过滤 `slot.Days`，再对 `start>end` 的跨零点时段做 `currentM >= sM || currentM < eM`）
- 写入校验：`internal/billing/strategy_validation.go:69-77`（把跨零点 slot 拆成 `[start,1440)` 与 `[0,end)`，**两段都归到同一个 days 上**做重叠检测）

**触发条件**
`OffPeakMode="custom"`，某 slot 跨零点（如 `22:00-06:00`）且配置了 `days`（如 `[1]` = 周一），且流量落在后半夜（周二 00:00–06:00）。

**根因**
两处对「跨零点时段的 day 语义」理解不一致：
- 校验层把它当作「周一 22:00 → 周二 06:00」的连续窗口（所以拆成周一的两段做重叠判断）；
- 执行层把 `days` 匹配到**请求发生的当天**，于是实际生效窗口变成「周一 00:00–06:00 + 周一 22:00–24:00」。
结果：配置为「周一夜间优惠」时，**周二 00:00–06:00 按原价收费（多收）**，而**周一 00:00–06:00 被优惠（少收，那其实是周日夜里的尾巴）**。对每个配置日每天各错 6 小时。

**影响**
计费金额错误（双向：多收与少收），且运维从配置界面/校验结果看不出问题（保存是通过的）。行级折扣叠加路径（group multiplier → off-peak 乘算）本身无重复折扣问题（见 §4）。

**证据**
`internal/billing/offpeak_bugprobe_test.go::TestBugProbeCrossMidnightSlotUsesWrongWeekday`
```
go test ./internal/billing/ -run TestBugProbeCrossMidnightSlotUsesWrongWeekday -v   # PASS（锁定 buggy 行为）
```
实测：`{start:"22:00",end:"06:00",days:[1]}` → 周二 01:00 `matched=false, discount=1.0`（应为 0.5）；周一 01:00 `matched=true, discount=0.5`（不应命中）；周三 01:00 不命中。

**建议修复**
统一语义并写进文档：跨零点时段的 post-midnight 段按「起始日」判定（`ISODay-1` 回绕），校验层的重叠检测与执行层共用同一个 `slot-day → 分钟区间` 展开函数，避免两套实现漂移。

---

### BUG-DATA-03（P2）非法时分被静默解析成 00:00，写入校验放行

**位置**
- `internal/billing/strategy_validation.go:182-190`：`strconv.Atoi` 的错误被丢弃（`h, _ :=` / `m, _ :=`），失败即 0。
- `internal/billing/strategy_validation.go:162-169`：只做 `0 <= minutes < 1440` 的区间检查，于是 0 合法。
- 消费方：`strategy.go:91-92`（槽位）、`strategy.go:39-40`（night/window 模式）同一解析器。

**触发条件**
运营/REST 写入任意含非数字前缀或后缀的时间串：`"8:30pm"`（24h 意图被读成 8:30 早上）、`"abc"`→00:00、`"07:xx"`→07:00。

**根因**
解析函数「宽容」，校验函数只校验解析结果的数值范围，没有校验输入串格式（无 `HH:MM` 正则/回填）。

**影响**
保存成功但实际生效窗口与配置不同（例如把「20:30-21:30」的 5 折错放到「08:00-09:30」），属于静默计费错误；`night`/`window` 模式的 `OffPeakStart/OffPeakEnd` 同样受影响。

**证据**
`internal/billing/offpeak_bugprobe_test.go::TestBugProbeMalformedSlotTimeAccepted`（PASS，锁定 buggy）
`ValidateOffPeakSlots("custom", '{"start":"8:30pm","end":"21:30",...}')` 返回 `nil`（被接受），`parseTimeToMinutes("8:30pm")==480`；`"abc"` 同样被接受且解析为 0。

**建议修复**
`parseTimeToMinutes` 改为 `(int, error)`，`validateOffPeakSlot` 拒绝解析失败；对存量数据可在读取侧继续按旧行为兜底（不回滚历史价格）。

---

### BUG-DATA-04（P2）`EstimateCalibrator.Adjust` 锁外读共享样本 → 数据竞争

**位置**
- 读（错误）：`internal/billing/calibration.go:112-118` — `RLock` 取指针 → `RUnlock` → 再读 `sample.count` / `sample.ratio`
- 写：`internal/billing/calibration.go:75-80` — 持写锁 `sample.ratio = ...`、`sample.count++`

**触发条件**
请求路径并发调用：某请求 `Observe`（记录 upstream/local 比值）与另一请求 `Adjust`（用学到的比值校正 token 估算）同时发生。生产上这是常态（`api/common.go:635` 与 `calibratedPromptEstimate` 都在请求路径）。

**根因**
锁只保护了 map 的查找，没有保护 `ratioSample` 的字段读取；`ratioSample` 是指针，`RUnlock` 后仍被解引用，与写者竞争。

**影响**
Go 内存模型下的未定义行为（amd64 上通常是读到旧值，32 位平台可能读到撕裂的 float64）；`go test -race` 会稳定报 race。实际业务影响小（估算 token 略偏），但这是请求热路径上的 concurrency bug，且会污染后续 `-race` CI。

**证据（`-race` 实测，已抓取原始输出）**
```
AIRoute_PROBE_RACE=1 go test -race -run TestBugProbeCalibratorAdjustReadsRatioUnlocked ./internal/billing/
WARNING: DATA RACE
Write at 0x... by goroutine 10:
  billing.(*EstimateCalibrator).Observe()  calibration.go:78
Previous read at 0x... by goroutine 11:
  billing.(*EstimateCalibrator).Adjust()   calibration.go:118
（`:80` sample.count++ 同样被报）
```
探针默认 `t.Skip`（避免普通 `go test ./...` 变红），需 `AIRoute_PROBE_RACE=1` + `-race` 启用。

**建议修复**
在 `RLock` 内直接把 `ratio`/`count` 拷成局部值（或提供 `snapshotSample(model)` 访问器），锁外只用拷贝。

---

### BUG-DATA-05（P2）`estimateBiasByModel` 无上限：model label 基数与内存可被客户端推高

**位置**
- 有上限的兄弟指标：`internal/telemetry/metrics.go:217-227`（`modelRequests` 达到 500 后新 model 归入 `_unrecognized`）
- 无上限：`internal/telemetry/metrics.go:314-321`（`SetEstimateBias` 对任意非空 model 直接 `m.estimateBiasByModel[model] = ...`）
- 导出：`internal/telemetry/metrics_prometheus.go:154-167`（每个条目一行 `airoute_billing_estimate_bias_ppm{model="…"}` / `..._samples{model="…"}`）
- 数据来源：`internal/billing/calibration.go:103` ← `observePromptEstimate(modelNameOf(req), …)`（`internal/api/common.go:635`），即**客户端请求里的 model 字符串**（未做 alias/served 归一化）

**触发条件**
客户端能请求任意 model 串且请求成功（透传/wildcard 路由、或多协议渠道接受任意模型名的部署）。每次成功的 upstream 用量上报都会 `SetEstimateBias` 一次。

**根因**
后加的「按 model 的 estimate-bias」指标复用了 model 名当 label，却漏掉了同包既有指标已经做过 500 上限保护；`SetEstimateBias` 每次调用还全量遍历 map 求加权和（map 越大越慢，请求路径上 O(n)）。

**影响**
- 进程内 map 无界增长（内存泄漏，FIFO 永不清除）；
- Prometheus 序列数无界（scrape 体积、series churn、TSDB 基数成本），与 `modelRequests` 的防护策略自相矛盾；
- `SetEstimateBias` 的加权求和变成 O(models)（请求路径）。

**证据**
`internal/telemetry/cardinality_bugprobe_test.go::TestBugProbeEstimateBiasLabelsAreUnbounded`
```
go test ./internal/telemetry/ -run TestBugProbeEstimateBiasLabelsAreUnbounded -v   # PASS（锁定 buggy）
```
喂 5000 个不同 model：`modelRequests` = **501**（500 + `_unrecognized` 溢出桶），`estimateBiasByModel` = **5000**，`ToPrometheusFormat()` 导出 ≥5000 条 estimate-bias 序列。

**建议修复**
`SetEstimateBias` 复用同一个 500 上限 + `_unrecognized` 溢出桶；顺手把加权和改成增量维护（或缓存），避免每次调用 O(n)。

**跨域归属**：存储/telemetry 侧加限即可；「用请求 model 还是归一化后的 served model 作 label」需 api/router 域确认（建议与 `dispatcher.ResolveServedModel` 对齐，和计费用同一口径）。

---

## 3. 探针说明

新增 4 个文件，均不修改生产代码；**默认 `go test ./...` 全绿**（探针断言的是当前 buggy 行为，注释写明修复后应改成的期望值）：

| 文件 | 用例 | 运行 |
|---|---|---|
| `internal/storage/settlement_dup_bugprobe_test.go` | `TestBugProbeSettlementDuplicateBillingIDChargesTwice` | `go test ./internal/storage/ -run TestBugProbe -v` |
| `internal/billing/offpeak_bugprobe_test.go` | `TestBugProbeCrossMidnightSlotUsesWrongWeekday`、`TestBugProbeMalformedSlotTimeAccepted` | `go test ./internal/billing/ -run TestBugProbe -v` |
| `internal/billing/calibration_race_bugprobe_test.go` | `TestBugProbeCalibratorAdjustReadsRatioUnlocked`（默认 skip） | `AIRoute_PROBE_RACE=1 go test -race -run TestBugProbeCalibrator ./internal/billing/ -v` |
| `internal/telemetry/cardinality_bugprobe_test.go` | `TestBugProbeEstimateBiasLabelsAreUnbounded` | `go test ./internal/telemetry/ -run TestBugProbe -v` |

验证基线（加入探针后）：`gofmt -l internal/{storage,billing,telemetry} → 空`；`go vet ./internal/{storage,billing,telemetry}/ → OK`；`go test ./... → 16 包 ok`。

---

## 4. 已排除（查过、判定不是本轮要报的缺陷）

| 项 | 结论 |
|---|---|
| 分组提交 `settlement_batcher` 的重放/超时 | 安全：每请求的 `BillingID` 在入队前已钉住（`settlement_batcher.go:183-186`、`api/common.go:392-393`），带审计管道 at-least-once 重投，ledger 唯一键幂等；`enqueue` 的 `ctx.Done()` 分支只计 deferred，不丢单；`Stop()` 竞态最坏 = 调用方按 300ms 超时兜底（`SettleChargeBatched` 调用点 `api/common.go:416` 有显式 deadline，不会挂死）。重复扣只出现在「同批次重复 id」这一条（BUG-DATA-01） |
| 结算批次扣费金额的舍入 | 引擎输出经 `roundPrecision`（1e-8）→ `ToUnits` 必为 10 的倍数，批内 `ToUnits(Σcost)` 与 `ΣToUnits(cost)` 在 float64 误差内一致；未构造出 `ledgerDrift` 的舍入假阳性 |
| `ledgerDrift` / `auditLag` / 保留清理的边界 | 核对：分区 `DROP` 与普通 `DELETE` 都在同一事务里 `addUsageAuditPrunedUnits`（`migrations_partition.go:731`、`repo_logs.go:577`）；`tenantLedgerWindow`(7d) < 审计保留(365d)；`ledgerLinkage` 的 cutoff 与 ledger 保留的可退休条件匹配。未发现 OFF-BY/时区错 |
| `dialectTime` / `dialectTimePrecise` | PG 侧绑 `time.Time`、SQLite 侧绑 UTC 文本，微秒布局与 `usage_logs.created_at` 一致；baseline 时间戳存/读都用 `sqliteTimestampLayout`（`billing_reconcile_job.go:434-465`）；E-04/E-05/E-06 的修复是有效的 |
| 迁移幂等性 | runner 故意「Up → 单独记录版本」（`migrations_runner.go:79-114`），崩溃重放依赖每个迁移幂等：核对 DDL 用 `IF NOT EXISTS`/`ensureSQLiteColumn`，数据迁移用 `WHERE` 收窄；未发现重放会失败的迁移。**SQLite 注册表缺 v43 是已知遗留**（README §8.3），非新增缺陷 |
| FTS（SQLite FTS5 external content） | v42 已补 `usage_logs_fts_au`（`migrations_storage_repairs.go:120-127`）覆盖全部被索引列的 UPDATE，并 `rebuild` 回填；`ftsPhrase` 用 `""` 转义双引号（`repo_logs.go:47-51`），MATCH 走参数绑定，无注入/语法注入 |
| 结算 SQL 的参数绑定 / Rebind | 抽查 `Rebind` 调用点与 `postgresChargeMergeSQL`：占位符全显式 `CAST`（避免 pq 无类型参数导致 integer 推断溢出），无 `?` 出现在字符串字面量/JSON 运算符里导致错位；分区/`to_char` 等 PG 分支均有方言守卫 |
| 钱包下限 / 超额豁免 / tenant 回退 | `walletUnitsExpr` 与 PG merged SQL 的 CASE 钳位等价；亚单位（<1e-9）charge 落到 tenant 回退是注释声明的刻意行为；`recordWalletDeductionEffect` 只在 COMMIT 后 flush，重试不重复计数 |
| `auditLag` 增量水位 | 迟到行（created_at 早于水位）由 24h 全量重校准兜底、注释已声明；`now-5min` overlap 不会重复计数 |
| `estimateQuantile` 与直方图 | 分桶插值语义与 Prometheus `histogram_quantile` 一致；`Histogram` 原子计数、`+Inf` 累积、`Observe` 过滤 NaN/Inf 均正确 |
| `PurgeUsageAuditIDsBefore` 30 天去重表 vs 365 天审计保留 | 理论上有 >30 天迟投递会写重复审计行，但 Redis PEL/spill 不可能滞留 30 天，**不可达**，不报 |

---

## 5. 待确认（低置信/需业务口径）

1. **BUG-DATA-02 的语义归属**：若产品定义就是「跨零点 slot 的 days 指两段的各自当天」，那执行层是对的、校验层的重叠检测才是错的；本文按「22:00→次日 06:00 属起始日」这一更常见的夜间优惠语义判定为 bug。请业务确认后再定修哪一侧（两侧必须一致）。
2. **`CacheWritePrice` 快照 vs 实际计费**：`engine.go:434` 的 cache-write 溢价按 `PromptPrice*1.25` 计算并写入快照，若未来引入独立的 cache-write 价列，快照字段语义会漂移（现在无价列，属预留，不报）。
3. **账本归档的月份键时区**：`to_char(created_at,'YYYY-MM')`（PG）依赖会话时区，月末边界行的 `billing_ledger_archive_<yyyymm>` 归档键可能落在相邻月（仅影响审计可读性，不影响金额）。归属 storage，需 PG 环境验证。
4. **`repairSettlementGap` 的 `RowsAffected` 错误被吞**（`billing_reconcile_job.go:234-237`：`if err != nil { return 0, nil }`）——只影响日志/计数可观测性，属 P3 风格级，未单列。

---

## 6. 修复记录（2026-10-28，状态 buggy → fixed）

5 条全部修复。**探针已翻转为回归测试**（原 `*_bugprobe_test.go` 删除，改为断言正确行为的常规用例，进入 `go test ./...`）。生产代码改动仅限 Lead 授权的 6 个文件，未 commit。

| ID | 状态 | 生产代码改动 | 回归测试 | 验证 |
|---|---|---|---|---|
| BUG-DATA-01（P1） | **fixed** | `internal/storage/settlement.go`：`settleChargesOnce` 构建 `pending` 时按 `BillingID` 去重（保留首次出现），重复记录进 `duplicates`，**仅在事务提交成功后**与 pending 一起标记 `Settled`（回滚则保持未结算，交由管道重投）；ledger 唯一键的跨批次幂等语义、currency reject、零成本短路均未改动 | `internal/storage/settlement_duplicate_charge_test.go`：`TestSettlementBatchDuplicateBillingIDChargesOnce`（余额/计数器/ledger 三处一致，两条记录均 Settled）、`TestSettlementBatchDuplicateAndRedeliveryChargesOnce`（批内重复 + 跨批次重投递 + 一笔独立扣费 ⇒ 恰好 2 笔） | `go test ./internal/storage/ -run TestSettlementBatchDuplicate -v` → PASS |
| BUG-DATA-02（P2） | **fixed** | `internal/billing/strategy.go`：`slotCoversInstant` 采用 **start-day 语义**——`days` 指窗口开始那天，跨零点后半夜按「前一天」匹配 `days`；抽出 `clockInWindow`/`offPeakDayMatches`/`previousISODay`，并在 `OffPeakSlot` 与 `slotCoversInstant` 注释里写明语义。`internal/billing/strategy_validation.go`：重叠检测改为 `slotSegments` 按有效星期（0=起始日、+1=次日）比较，与定价路径同规则 | `internal/billing/offpeak_slot_regression_test.go`：`TestCrossMidnightSlotUsesStartDay`（周一夜间/周二凌晨打折、周二 06:00 端点、周二夜与周一凌晨不打折）、`TestCrossMidnightSlotWithoutDaysMatchesEveryNight`（days 空=每天）、`TestNonCrossMidnightSlotDayFilterRegression`（非跨零点回归）、`TestValidateSlotsCrossMidnightOverlapUsesStartDay`（跨日重叠检出、连续夜不误报、错误信息含有效星期） | `go test ./internal/billing/ -run 'TestCrossMidnight\|TestValidateSlotsCrossMidnight' -v` → PASS |
| BUG-DATA-03（P2） | **fixed** | `internal/billing/strategy_validation.go`：新增严格 `parseClockMinutes`（1-2 位时 + 2 位分、00:00-23:59、拒绝 AM/PM、非数字、超界、符号/多余字段），非法值返回 error；`validateOffPeakSlot` 失败即拒绝保存；读取侧新增 `parseRowWindow`：非法行级窗口/槽位**不计折扣**（不再猜成 00:00），空配置仍走 night 模式历史默认 00:00-08:30（行为保留） | 同文件：`TestParseClockMinutesStrict`（6 个合法值 + 11 个非法值）、`TestValidateSlotsRejectsMalformedTimes`、`TestMalformedRowWindowChargesUndiscounted`（night/custom/默认 三模式）、`TestNightModeKeepsDefaultWindow`（默认窗口保留） | `go test ./internal/billing/ -run 'TestParseClockMinutes\|TestValidateSlotsRejectsMalformedTimes\|TestMalformedRowWindow\|TestNightModeKeepsDefault' -v` → PASS |
| BUG-DATA-04（P2） | **fixed** | `internal/billing/calibration.go`：`Adjust` 在 `RLock` 内把 `ratio`/`count` 拷贝成局部值后再计算，消除与 `Observe` 的字段竞争（原 `.count`/`.ratio` 读发生在解锁后） | `internal/billing/calibration_concurrency_test.go`：`TestCalibratorAdjustIsRaceFree`（5000 并发 Observe/Adjust，原 -race 探针去掉 env gate 转为常规用例）、`TestCalibratorConcurrentObserveAdjustAcrossModels`（8 模型并发 + Snapshot 校验） | `go test -race ./internal/billing/ -run TestCalibrator -v` → PASS（无 WARNING: DATA RACE） |
| BUG-DATA-05（P2） | **fixed** | `internal/telemetry/metrics.go`：新增 `modelLabelCap=500` / `modelLabelOverflow="_unrecognized"` 常量并让 `modelRequests` 复用；`estimateBiasByModel` 同上限，超限折叠到溢出桶并做**样本加权合并**（而非被最后一个 model 覆盖）；`SetEstimateBias` 改为**增量维护**加权和/权重（O(1)，去掉每次 O(n) 全表求和）。`internal/telemetry/metrics_prometheus.go`：导出处注明序列基数受 `modelLabelCap` 约束（导出侧本身按 map 迭代，已随上限有界） | `internal/telemetry/cardinality_bound_test.go`：`TestEstimateBiasModelLabelsAreBounded`（5000 model ⇒ 两个 label map ≤ 501、溢出桶存在、导出序列 ≤ 501）、`TestEstimateBiasOverflowMergesSampleWeighted`（折叠桶加权合并均值正确）、`TestEstimateBiasIncrementalMeanStaysExact`（重复更新/零样本权重/空 model 名） | `go test ./internal/telemetry/ -run TestEstimateBias -v` → PASS |

### 6.1 质量门结果（修复后，逐项）

| 检查 | 命令 | 结果 |
|---|---|---|
| gofmt | `gofmt -l internal/storage internal/billing internal/telemetry` | 空（干净） |
| build | `go build ./...` | OK |
| vet | `go vet ./internal/storage/ ./internal/billing/ ./internal/telemetry/` | OK |
| 全量测试 | `go test ./...` | **16 包全 ok**（含 tests/ 集成、web） |
| race | `go test -race ./internal/billing/ ./internal/storage/ ./internal/telemetry/` | 全 ok，无 DATA RACE 报告 |

### 6.2 行为变化 / 升级注意（需写入变更记录）

1. **批内重复投递不再双扣**（BUG-DATA-01）：同一批里重复的 `billing_id` 现在与跨批次重投递同语义——只扣一次；被跳过的副本在提交成功后标记 settled。金额与 `api_keys.used_cost_units` 不再多计，`ledgerDrift` 不再因此漂移。
2. **分时优惠语义明确为 start-day**（BUG-DATA-02）：`days` 表示窗口开始那天，跨零点后半夜归前一天。已按旧语义配置「跨零点 + days」的规则，生效窗口会移动（每个配置夜的后半夜开始打折、配置日的前半夜不再打折）——上线前建议运营复核此类规则；无 `days` 或非跨零点的规则不受影响。
3. **时间配置变严格**（BUG-DATA-03）：`ValidateOffPeakSlots` 现在拒绝 `8:30pm`、`abc`、`8:5`、`25:00` 等非 `HH:MM` 值（此前静默保存成别的窗口）。存量 DB 里的非法行不会被回滚，但读取侧一律按「不计折扣」处理（不再猜时间）。
4. **estimate-bias 指标序列有界**（BUG-DATA-05）：超过 500 个 model 后统一折叠到 `model="_unrecognized"`；该桶的 ppm 是折叠模型的样本加权均值、samples 为累加值。看板/告警若按具体 model 标签匹配，需知道超限模型不再单独出现。
5. 校准器的 `Adjust` 与 `Observe` 现在无竞争（BUG-DATA-04），行为数值不变。

### 6.3 测试文件变更清单

- 删除（原 buggy 行为探针）：`internal/storage/settlement_dup_bugprobe_test.go`、`internal/billing/offpeak_bugprobe_test.go`、`internal/billing/calibration_race_bugprobe_test.go`、`internal/telemetry/cardinality_bugprobe_test.go`。
- 新增（常规回归，随 `go test ./...` 执行）：`internal/storage/settlement_duplicate_charge_test.go`、`internal/billing/offpeak_slot_regression_test.go`、`internal/billing/calibration_concurrency_test.go`、`internal/telemetry/cardinality_bound_test.go`。
- 未修改既有测试期望（`engine_test.go`、`units_test.go`、`cache_and_bias_test.go`、`metrics_test.go`、`repo_pricing_offpeak_test.go` 全部原样通过）。
