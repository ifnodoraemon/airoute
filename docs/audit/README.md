# Airoute 网关审计与修复总结（2026-10-23）

最新：[现有功能操作闭环审查与修复（2026-10-29）：16 组修复，双节点与真实浏览器验收](2026-10-29-operation-closure/README.md)。

新增：[功能流程与细节审查（18 项）](functional-details-2026-10-26.md) · [对应修复与验证（2026-10-27）](functional-details-remediation-2026-10-27.md) · [重建重启后全面审查（2026-10-27）](2026-10-27-review/README.md)（7 个领域、115 项发现，含线上验证记录） · [P0+P1 修复记录](2026-10-27-review/remediation-2026-10-27.md) · [剩余 P2/P3 修复记录](2026-10-27-review/remediation-p2-p3-2026-10-27.md) · [内部一致性审计（2026-10-28，72 项）](2026-10-28-inconsistencies/README.md) · [真实缺陷排查与代码瘦身（2026-10-28 bughunt）：23 条 P 级缺陷 + 4 条跨域加固 + 死代码归零](2026-10-28-bughunt/README.md) · [**第 3 轮真实缺陷排查与代码瘦身（2026-10-29）：4 P1 + 19 P2 + 5 P3，含上线 17h 实测复核、迁移 v44/v45 升级注意**](2026-10-29-bughunt/README.md)。

本目录下有三份完整审计报告：

| 文件 | 内容 |
|---|---|
| [`billing.md`](billing.md) | 计费/扣费正确性审计，17 项发现（F1–F17） |
| [`concurrency.md`](concurrency.md) | 并发与热路径审计，16 项发现 + 18 条补丁建议 |
| [`logs.md`](logs.md) | 日志/指标/DB/Redis 健康检查（含 10:03–10:11 线上 502 事件复盘） |

本文只记录**已经落地的修复**、**验证证据**和**尚未执行**的建议。

---

## 一、日志：为什么"总感觉哪里不对"

审计确认了 6 个具体缺陷，均已修复并线上验证。

### 1. 时间戳精度不一致（P0，已修）

`usage_logs.created_at` 有两条写入路径：

- 批量写入（一次 flush 多行）把时间格式化成 `2006-01-02 15:04:05` → **整秒截断**；
- 单行写入且 `CreatedAt` 为零时走数据库默认值 → **保留微秒**。

所以同一批 502 全部塌缩成 `02:09:44`，而同一秒的 429 却是 `02:09:44.905597`。后果正是无法回答的三件事：重试间隔、并发度、跨系统关联。

修复：`internal/storage/repo_logs.go` 新增微秒布局 `sqliteTimestampLayout = "2006-01-02 15:04:05.000000"`；`repo_logs_writer.go` 单行/批量两条路径**始终由服务端写入 created_at**（不再依赖列默认值），格式统一。旧数据仍可被时间过滤读取（保留 `legacySecondTimestampLayout`）。

验证：修复后新写入行 `date_trunc('second', created_at) = created_at` 全部为 `f`；回归测试 `internal/storage/usage_log_timestamp_test.go`。

### 2. 路由日志没有 trace_id / attempt（P0，已修）

`router` 分类的日志（attempt / fallback / breaker）此前 **0/891 携带 trace_id**（access 日志是 1950/1950）。一次 `attempt → 429 → 熔断跳过 → 502` 在日志里是几条互不相干的孤立记录。

修复：新增 `internal/router/log.go` 的 `routerLog(ctx)`，从请求 context 取 trace_id 并绑定到 logger；24 处路由日志全部改走它。失败日志额外带 `attempt`。`telemetry.TraceIDFromContext` + `telemetry.ContextKeyTraceID` 作为跨包共享的 context key。

验证：`internal/router/log_trace_test.go`；线上 access 行已带 `trace_id`。

### 3. access 日志无法定位"谁、为什么失败"（P1，已修）

原来只有 `status=502`，没有租户、没有 key、没有原因，只能回查数据库。

修复：`http_access` 增加 `tenant`、`key`（脱敏为 `sk-airoute***`）以及非 2xx 时的 `error`（取自统一的错误记录点，已做敏感信息清洗）。限流拒绝会写入 `cluster/local rate limit exceeded` 等具体原因。

验证：`{"status":404,...,"tenant":"test","key":"sk-airoute***","error":"no upstream provider available for requested model 'no-such-model-xyz'"}`。

### 4. 启动 banner 不是 JSON（P2，已修）

彩色多行 banner 直接写 stdout，在容器里破坏 `jq` / 日志采集器解析。

修复：`cmd/airoute/server.go` 的 `printStartupBanner` 仅在 stdout 是 TTY 时打印；容器内只保留结构化日志（`gateway server listening` 等）。

验证：重启后 `docker logs` 非 JSON 行数为 0。

### 5. 控制台系统日志缓冲区被 access 日志淹没 + 初始化顺序 bug（P1，已修）

- 环形缓冲区容量 2000，且每条请求都写一条 `http_access` → 300 req/s 突发时只够存 **6 秒**，事故现场在被打开控制台前就被挤掉；
- `GlobalRingBuffer` 原本在 `init()` 中创建，而 logger 的 `init()` 先执行 → **logger 绑定到 nil buffer**，第一次显式 `InitLogger` 之前的所有日志（配置加载、安全告警、启动失败）根本进不了控制台。

修复：`internal/telemetry/ring_buffer.go` 改为变量初始化器（保证在任何 `init()` 之前就绪）、容量 2000→5000、`http_access` 不再进入环形缓冲区（它已经持久化在 `usage_logs`，控制台另有审计日志页）。

验证：`GET /api/v1/admin/system/logs` 现在按启动顺序完整显示（首条是 `starting airoute`，证明 nil-buffer 已修），且 `http_access` 计数为 0。

### 6. 重复告警风暴 + p99 被硬钳制（P2，已修）

- 熔断 OPEN 时**每个被跳过的请求**都打一条 WARN（2 秒内 127 条）；Redis 抖动时按请求/按记录重复告警。新增 `telemetry.WarnLogger(key, interval, ...)`，同类告警 30s 一条。
- 路由里程碑日志（attempt / success）从 INFO 降为 DEBUG：它们是单请求内部细节，INFO 级别使其日志量翻 3 倍且无法关联。
- 延迟直方图最高桶为 60s，真实最大 113s 的请求落在桶外 → p99 恒等于 60000。修复：`internal/telemetry/metrics.go` 扩展桶（到 600s），并记录观测最大值作为溢出回退。

验证：`internal/telemetry/metrics_tail_test.go`；日志量在同样流量下约降为原来的 1/3。

### 7. 顺带修复：Postgres 专用 bug（P1）

`DB` 包装器只重绑了 `Exec/Query/QueryRow/QueryContext?`，漏掉了 `*Context` 变体：`QueryRowContext(..., ?)` 在 PostgreSQL 上直接语法错误。受影响的是**每个数据面请求的权威 key 查询**（错误被丢弃，所以静默退回 10s 内存快照）和 `GetUserByIDContext`。

修复：`internal/storage/db.go` 为 `DB` 和 `Tx` 补齐 `ExecContext/QueryContext/QueryRowContext` 的方言重绑。

验证：`internal/storage/postgres_billing_idempotency_test.go`（真实 PostgreSQL 集成测试）。

---

## 二、扣费：需要优化，且重点是结构而非价格

已落地：

| 缺陷 | 修复 |
|---|---|
| **F1 无幂等键**：Redis Stream 至少一次投递，commit 成功但 XACK 失败会重放 → 重复流水 + 重复扣款，且无法识别 | `usage_logs.billing_id`（服务端生成，迁移 v21 + 唯一索引）+ `ON CONFLICT (billing_id) DO NOTHING`；批量路径用 `RETURNING billing_id` **只对真正插入的行扣款**；新增指标 `airoute_billing_duplicates_skipped_total` |
| **F3 流式断连 499 零计费**（可被"收完内容再断开"利用） | `recordUsage` 将"499 且已产生 token"视为可计费；审计状态仍记 499 |
| **F5 未配价模型自动打 5 折** | 默认价记录的 `OffPeakEnabled` 改为 false（折扣必须由价格规则显式开启） |

验证：

- 幂等：`internal/storage/billing_idempotency_test.go`（SQLite 4 例）+ `postgres_billing_idempotency_test.go`（真实 PG 三次重投只扣一次）；
- 499：线上 `/v1/messages` 流式中断实测 `status=499, cost=0.000042`（42 prompt tokens），修复前为 0；单测 `internal/api/abort_billing_test.go`；
- 默认价：`internal/billing/default_price_test.go`；
- 正常计费无回归：实测一次请求 `cost=0.000064`，钱包余额变动 `-0.000064`，`billing_id` 非空。

未做（建议下一迭代）：F9 账本治理（删除流水不冲账、旧 nano 镜像清理）、跨模型降级时的"按实际服务模型计费"（F10，当前 model_fallbacks 为空所以潜伏）、以及把对账结果接到控制台页面上。

---

## 三、并发：真正的墙不是 DB 查询

审计结论修正了初始假设：每请求一次权威 DB 查询只占时延的 0.007%，真正的限制是：

| 瓶颈 | 措施 |
|---|---|
| **容器 `ulimit -n = 1024`**：每路流式占 2 个 fd → 单副本约 400 路、集群约 190–230 req/s | `docker-compose.yml` 为 gateway/lb/postgres/redis 增加 `ulimits.nofile = 65536`（已验证：`nofile=65536:65536`） |
| **PG 连接预算被算死**：2×50 = 100 = `max_connections`，第三副本无法接入，39 条 idle 长期占坑 | 连接池改为环境变量 `GATEWAY_DB_MAX_OPEN_CONNS`（默认 25）/ `GATEWAY_DB_MAX_IDLE_CONNS`（默认 8）；compose 将 `max_connections` 提到 200（下次重建 postgres 生效） |
| 每请求全量缓冲请求体仅为了保留 32KB 追踪片段（400 并发 ≈ 240–300MB） | `CaptureRequestSnapshot` 改为只读前 32KB，剩余部分通过 `io.MultiReader` 链回请求体 |
| Redis 调用无界：RPM 校验无超时 + go-redis 重试 3 次，最坏 +3–4s | `internal/middleware/ratelimit.go` 增加 300ms 超时，超时回落到本地令牌桶 |

---

## 四、RPM 限流是否生效（实测）

是，且两条链路都在工作：

- 将某个 key 的 `rpm` 临时改为 3，等待热加载后连续 6 次请求 `/v1/models`：**前 3 次 200，第 4–6 次 429**；
- 拒绝响应来自**分布式滑动窗口**（`"Cluster rate limit exceeded (RPM limit reached)"`），Redis 中可见 `airoute:ratelimit:rpm:*` 键；
- 恢复 `rpm=60` 后立即回到 200；
- 10:09 突发期间的 231 次本地限流 429 也说明本地令牌桶兜底有效。

注意：**key 1（admin, rpm=0）是不限流的**，当前大量内部流量走它，因此限流路径平时几乎不被压测；切换到租户 key 后每请求会多 4 次 Redis 往返。

---

## 四之二、第二轮修复（moderation / F6 / F4）

### `/v1/moderations`：从"伪造安全"改为显式拒绝

该接口是 OpenAI 内容审核协议（返回 11 个风险类别的 flagged/分数，不产生 token）。原实现在没有 `moderation` 协议渠道时**返回伪造的"全部安全"**（flagged=false、分数 0.00001、HTTP 200）——实测"我要杀光所有人"也返回 unflagged，属于假安全。

修复（`internal/api/moderation_handler.go`）：

- 无渠道时返回 **501 + `moderation_not_configured`**，明确说明需要配置 moderation 渠道；
- 伪造响应保留为本地联调开关 `GATEWAY_MODERATION_STUB=true`，并在使用时限频告警；
- 计费无需按 token；将来接付费厂商（阿里云内容安全等按调用计费）用按次 `fixed_price` 即可。

### F6：非流式请求缺 usage 时按估算计费

上游不返回 usage 时，5 个非流式处理器把 token 记 0 → 成功请求不收费（实测 148 条 HTTP 200 收费 0，约 ¥2.2）。修复：`internal/api/common.go` 新增 `resolveChatUsage` / `resolveEmbeddingTokens` / `resolveRerankTokens`，与流式处理器使用同一套字符→token 估算，并把 `usage_source=estimated` 写入审计行；覆盖 chat/completions/responses/gemini/anthropic（原生与适配两条路径）/embeddings/rerank。

### F4：钱包透支钳制 + 告警

原来 `balance = balance - cost` 没有下限：一个大请求或 10s 快照窗口内的并发可以把余额扣成任意负数，且无任何告警（在途预留仅 0.0005 元，实测单请求最高 0.65 元）。

修复（`internal/storage/wallet_deduct.go`）：所有扣款（单条/批量/租户回退）统一走 `deductWallet`，SQL 内用可移植 `CASE` 把余额钳制在 `-GATEWAY_MAX_OVERDRAFT_CNY`（默认 1 元，0 = 不允许透支），同时用整数列推导浮点镜像避免双写漂移；`RETURNING balance_units` 检测透支并上报 `airoute_billing_overdrafts_total` + 限频 WARN（区分"刚转负"与"已触底"）。

验证：SQLite 单测 4 例 + 真实 PostgreSQL 集成测试；线上 `/metrics` 已暴露新计数器，moderation 实测返回 501。

### F2（第一阶段）：让"钱"不再依赖日志能否落库 + 对账

原缺陷：扣款发生在审计日志事务里，而日志管道有 4 条丢弃路径（内存队列满 500ms、连续 5 次写库失败、停机重试耗尽、logger 已停止）。任何一条触发，对应的计费记录既不入库也不扣钱，且只在日志里留一句 ERROR——**这是"收入静默消失"，而不是重复扣款**，严重程度高于审计报告里的排序。

第一阶段落地：

1. **磁盘兜底（不再丢弃）**：`internal/storage/usage_spill.go`。四条丢弃路径全部改为写入本地 append-only 文件（`GATEWAY_USAGE_SPILL_DIR`，默认 `data/`，`fsync` 后返回），启动时按批重放并删除已持久化的文件；重放失败的文件保留到下次启动。记录自带 `billing_id`，两个副本共享卷导致的重复重放会被唯一索引拦成 no-op。新增指标 `airoute_usage_log_records_spilled_total`、`airoute_usage_log_spill_replay_failures_total`。
2. **对账 worker**：`internal/storage/billing_reconcile_job.go`（`GATEWAY_BILLING_RECONCILE_HOURS`，默认 6 小时，0 关闭）。周期比对两个此前无人核对的镜像：
   - `api_keys.used_cost_units` vs `SUM(usage_logs.cost_units)` → `airoute_billing_ledger_drift_units`
   - `users.balance_units` vs `ROUND(balance*1e9)` → `airoute_billing_wallet_drift_units`
   漂移非 0 时限频 WARN。worker 只报告、绝不自动改账。
3. **告警规则**：`deploy/prometheus/airoute-alerts.yml`，13 条规则覆盖：记录丢弃/落盘/重放损坏、重复投递、透支、两类账本漂移、5xx 率、p99、fallback 风暴、队列积压、写库失败、并发连接。

验证：spill 往返/幂等/损坏行/禁用 4 组单测；对账漂移 3 组单测（人为制造 ¥5 账本差与 ¥7 镜像差，指标精确匹配）。

**首次对账即抓到真实问题**：上线后 worker 立刻报 `ledger_drift = ¥7.1477`（两副本同时告警）。拆解后与审计报告的 F9 完全吻合：

| 来源 | 金额 | 处理 |
|---|---|---|
| 969 条旧流水 `cost>0` 但 `cost_units=0`（整数镜像上线前写入） | ¥4.5002 | 迁移 v22 回填 `cost_units = ROUND(cost*1e9)`，漂移降至 **¥2.6475** |
| 早期被删除的 147 行流水（id<148，无审计痕迹） | ¥2.6475 | 不可重建，作为已知基线保留在指标里 |

修复后线上核对：`used_cost_units - SUM(cost_units)` 只剩该 ¥2.6475，`balance_units` 与浮点镜像漂移为 0。

> 2026-10-27 更新：该 ¥2.6475 基线已由迁移 v33 一次性冲销（指纹匹配 `id=1` + 精确缺口，写落 `system_settings.billing_repair_f9_writeoff`），`ledger_drift_units` 归零；同批 v32 重建了 725 行 `%!w(<nil>)` 与 51 行空原因合成 499 的文案。细节见 `billing.md` F9「2026-10-27 处置」。

### F2（第二阶段，已完成）：同步扣费 + 异步审计

第一阶段只是让记录"丢不了"，扣费仍然发生在审计写入的同一个事务里——审计写不进去，钱就不动。第二阶段把两者彻底拆开：

1. **结算账本** `billing_settlements`（迁移 v23，`billing_id` 唯一 + 唯一索引）。扣费是否已发生，只看这张表里有没有对应 `billing_id`；插入成功 = 本次扣款生效，冲突 = 已经扣过。历史 32k 条流水在迁移时回填，其中 29,107 条无 `billing_id` 的旧行用 `legacy-<id>` 作为稳定键。
2. **请求路径同步结算**：`recordUsage` 在算出费用后立即调用 `storage.SettleChargeNow`（1.5s 上限），钱在请求结束时就落袋，不再等日志管道。失败（数据库不可用等）不是致命错误：记录照旧进入 Redis / 磁盘兜底，由异步写入器补扣，等于自动降级回第一阶段的行为。
3. **异步路径顺序反转**：`BatchRecordUsageLogsContext` 现在是"先结算、后审计"，两个独立事务。审计写失败只会让流水迟到，不会让钱迟到；重试时结算因 `billing_id` 冲突而跳过。
4. **对账口径拆分**：
   - `airoute_billing_ledger_drift_units` = key 账本 vs 结算账本（钱是否扣对）
   - `airoute_billing_audit_lag_units` = 结算账本 vs 审计流水（流水是否补齐）
   - 新增 `airoute_billing_settled_synchronously_total` / `settlements_deferred_total` 观察同步命中率与降级次数。
5. **自愈补账**：滚动发布时旧副本仍会写流水但不写结算（它不认识这张表），于是出现"有流水、无结算"。对账 worker 每轮按幂等方式补录 500 条 5 分钟前的缺口（只补账本、绝不再动钱）。

**线上验证（2026-10-26）**：

| 指标 | 结果 |
|---|---|
| 迁移 | v23 已应用，结算账本回填 32,188 → 自愈后 32,599 条 |
| 同步结算 | `settled_synchronously_total` 持续增长，`deferred_total = 0`（零降级） |
| 账实一致 | `settled - audit = 0`（精确相等），`audit_lag = 0` |
| 账本漂移 | 仍为已知基线 ¥2.6475（早期删行，不可重建） |
| 钱包镜像 | `wallet_drift = 0` |
| 单请求核对 | trace `tr-f2p2-check-001`：audit 65,000 units = settled 65,000 units |

**取舍**：每请求多一次本地数据库事务（约 1ms，相对 4.19s 的推理时延可忽略），换来"审计管道故障不影响收入"。审计行仍然必须送达——它是对账与排障的依据，所以保留 Redis PEL + 磁盘兜底 + 自愈补账三层保障。

---

## 四之三、客户端断连被误判为上游故障（2026-10-26 修复）

排障时发现审计里 259 条 `502 / context canceled`：

- 全部 `channel` 为空、`prompt/completion = 0`、`cost = 0`；
- 复现方式：流式请求在**首个 token 之前**断开（`curl -m 0.3`），必现 502 + `context canceled`；
- 代码原因：`dispatcher` 在 `ctx.Err() == context.Canceled` 时返回 `ctx.Err()`，而
  `resolveUpstreamFailureStatus` 没有识别取消，落到了兜底的 `502 upstream_failure`。

**这是客户端（或其 HTTP 超时/进程退出）主动断开，不是上游问题**，但被记成 502 造成三个后果：
把责任归给上游、污染 5xx 率/错误预算、让历史故障复盘把客户端断连算进上游故障。

修复：

1. `context.Canceled` → **499 `client_closed_request` / `client_canceled`**；
   `context.DeadlineExceeded` → 504 `upstream_timeout`；
2. 新增 `airoute_requests_client_canceled_total`，499 不再计入 `requests_failed_total`，
   控制台的服务端错误统计也随之只反映真实 5xx；
3. 迁移 v24 把历史 260 条误标行改回 499（`error_message = 'context canceled'` 且 502）；
4. 审计时间戳改为**请求完成时刻**（此前为零值，写入器用 flush 时刻兜底，导致同一批被取消的
   8 个请求时间戳完全相同、真实先后顺序丢失）。

### 断连也要计费，且要能归因到渠道

上面的修复让"客户端断开"不再冒充上游故障，但还有一个反向漏洞：这些请求**已经打到上游**，上游按 prompt 计了费，而我们记 0 token / 0 成本 —— 等于自己吸收成本。

修复：

1. **取消错误携带上下文**（`internal/router/errors.go`）：`newCanceledError` 现在返回带 `Trail`（正在等待的渠道）与 `PromptTokens`（请求估算 prompt）的 `DispatchError`，`errors.Is(err, context.Canceled)` 仍然成立，所以 499 判定不变；审计的 `channel` 列也终于有值了。
2. **断连按估算 prompt 计费**：派发阶段的取消，`recordFailedRequestWithError` 从 `DispatchError.PromptTokens` 取估算值并标 `usage_source=estimated`；流式处理器在 499 分支不再把 prompt 清零（5 个处理器统一）。
3. **渠道归因补全**：全部候选被熔断器跳过时，"attempted"为空导致 channel 为空 —— 现在用 `exhaustedTrail` 回落到 skipped 渠道，`channel` 列不再是空白（历史 736 条 502 属于此类）。

**线上验证**：`tr-abort-bill-001`（4000 字符 prompt，0.5s 断开）→ `499 / channel=罗总 / prompt_tokens=1000 / usage_source=estimated / cost=¥0.001`，余额精确扣减 ¥0.001。

**线上验证**：迁移后今天的 502 只剩 1 条——真正上游首 token 超时
（`channel 罗总 timed out waiting for first token (30s)`，41.4s）；9 条为客户端断连（499）。
新复现请求记录为 499，且 `client_canceled_total` 增长、`requests_failed_total` 不变。

---

## 四之四、计费口径：与上游账单对齐

目标明确为「上游收我们多少，我们就收客户多少，上游不收的我们也不收」。落地为 6 条规则：

| 场景 | 口径 | 实现 |
|---|---|---|
| 正常成功 | 按**上游实际服务的模型**的价格计费 | `billingModelFor` 通过 `dispatcher.ResolveServedModel(channel, requested)` 还原上游模型；仅当该模型有**显式价格**时替代别名价，避免把运维配置的别名价悄悄换成兜底价 |
| 上游失败（502/503/504） | 不计费 | `billable` 仅覆盖 2xx/3xx 与"499 且已消耗 token" |
| 客户端断连（499，已到达上游） | 按估算 prompt 计费 | `DispatchError.PromptTokens` / 流式 499 分支估算，标 `usage_source=estimated` |
| 认证/限流阶段被拒 | 不计费 | 请求未到达上游，无渠道、无 token |
| 缓存命中的 prompt | 按缓存价 | 价格表 `cache_read_price`，且 cached token 不重复计入未缓存部分 |
| 模型未配价 | 回退兜底价卡 + 启动告警 | 启动自检同时检查"请求模型"与"上游实际服务模型"（后者通过渠道映射还原） |

**为什么"按服务模型"很重要**：线上 `罗总` 渠道把 `deepseek-v4.1-flash` 映射为上游的
`volcengine/deepseek-v4-1-flash-261017`，而价格表里只有别名 `deepseek-v4.1-flash`（¥1/¥4）。
只按别名收费，等于用假设的价目去覆盖上游真实账单；一旦上游该模型的实际价格不同，差额全部由平台承担。

**验证**：单测覆盖"服务模型有价 → 按服务模型计费"和"服务模型无价 → 保留别名价"两种路径；
启动自检会在日志中列出"上游已服务但未配价"的模型（含渠道信息），便于补价格。

---

## 四之五、断连排空（B 方案）：与模型提供方账单逐单对齐

计费原则最终确认为：**模型提供方（上游）收我们多少，我们就向下游客户收多少；上游不收的我们也不收；下游是否断开与计费无关。**

要在"客户断开"时也做到逐单等于上游账单，就不能取消上游请求——否则拿不到该请求的 usage，只能估算。实现为"排空"：

1. **上游调用脱离下游上下文**：入口处用 `upstreamRequestContext`（`context.WithoutCancel` + 排空超时，默认 120s）创建派发上下文，客户断开不再取消模型提供方的调用。
2. **流式循环继续消费**：客户断开后不再 `return`，而是继续读上游流（只跳过对下游的写出），直到上游结束，收集它最后一帧的 `usage`。
3. **按上游真实 usage 计费**：审计状态仍记 499（客户确实断了），token 与成本使用上游回报值，`usage_source=upstream`（不再是 estimated）。
4. **量化代价**：新增 `airoute_aborted_drained_requests_total` 与 `airoute_aborted_drained_completion_tokens_total`——后者就是"已经付费但客户没收到"的 completion token，是排空策略的真实成本。
5. **可回退**：`GATEWAY_DRAIN_ABORTED_UPSTREAM=false` 恢复"立即取消 + 校准估算"的省钱模式；`GATEWAY_DRAIN_TIMEOUT_SEC` 控制单个排空的最长时间。

**线上验证**（客户 1.0~1.2 秒后断开，`max_tokens=300`）：

```
trace            status  channel  prompt  completion  cost       usage_source  drained_tokens
tr-drain-001       499     罗总      38        300     0.001238    upstream          300
tr-drain-002       499     罗总      39        300     0.001239    upstream          300
```

对比之前的"估算 prompt"版本：那时只收 ¥0.0004 左右（prompt 估算），上游实际按 300 completion 计费；现在逐单与上游账单一致，差额不再由平台承担。

---

## 四之六、熔断器在单渠道下为什么有害（已改为 advisory）

现象：线上只有 `罗总` 一条 active 渠道（`gpustack-cluster` 已 disabled），但日志里仍出现
`circuit breaker TRIPPED`，以及大量 `all 1 candidate channels failed ... skipped` 的 502
（10-22/23 共 736 条）。

原因：熔断器原本是为**多渠道 failover** 设计的——某个渠道连续失败就先摘掉它、把流量交给备用渠道。
但单渠道时没有"备用"可交，摘掉自己只意味着：

1. **限流被误判为故障**：`provider.IsChannelFailure` 把 429 当作渠道故障。上游限流是**健康的背压信号**，
   连续 3 次就把熔断打开 30 秒 → 之后所有请求在网关侧直接 502，**连上游都不调**；
   客户拿到的不是可退避重试的 429，而是 502，且这 30 秒是自伤式黑洞。
2. **抖动放大**：任何 3 次连续超时（包括我们自己 30s 首 token 超时）都会触发 30 秒不可用。

修复：

| 改动 | 说明 |
|---|---|
| 429 不再计入渠道故障 | 限流 → 直接透传 429（带 `Retry-After` 语义）给客户，可正常退避；仍会尝试下一条渠道 |
| 熔断器支持 advisory 模式 | `GATEWAY_BREAKER_ENABLED=false`：**记录状态但不拦截流量**，只有一条渠道时启用 |
| 参数可配 | `GATEWAY_BREAKER_FAILURES`（默认 3）、`GATEWAY_BREAKER_COOLDOWN_SEC`（默认 30） |

线上当前配置（`.env`）：

```
GATEWAY_BREAKER_ENABLED=false     # 单渠道：只记录，不拦截
GATEWAY_DRAIN_ABORTED_UPSTREAM=true
GATEWAY_DRAIN_TIMEOUT_SEC=120
```

启动日志会明确提示 `circuit breaker runs in advisory mode`。等新增第二条渠道、真正需要 failover 时，
把它改回 `true` 即可恢复拦截能力（429 不计故障的规则仍然适用）。

---

## 四之七、超时只有一个旋钮：渠道超时

原状态是三个各自独立的超时值，运维必须手工保持它们协调：

| 环节 | 原值 | 问题 |
|---|---|---|
| 渠道 `timeout_seconds` | **3000s** | 比边界代理还大，永远不会生效 |
| nginx `proxy_read_timeout` | 600s | 实际最先切断的一方 |
| 首 token 等待 | 30s（代码默认） | 与渠道超时无关，容易与渠道超时理解冲突 |

对使用者来说，"渠道超时"应该就是"这个渠道一次请求最多等多久"。因此合并为**单一旋钮**：

1. **渠道超时 = 唯一配置项**（当前 300s）。首 token 等待**直接等于**渠道超时
   （`GetFirstTokenTimeout` 不再返回更小的独立值）：设 300s 就等 300s，不会在 30s 被自己提前放弃。
2. **边界代理抬高到 1h**（nginx `proxy_read_timeout/proxy_send_timeout 3600s`），
   远高于渠道超时上限，保证"配了就算数"——不会再出现配置被 LB 提前截断的情况。
3. **上限是派生的，不是另一个配置**：`model.EdgeProxyTimeoutSeconds = 86400`（边界代理预算，一天）是唯一的基础设施常量，
   `MaxChannelTimeoutSeconds = 86400 - 60 = 86340` 由它派生（预留 60 秒余量，避免边界竞态）。
   这个预算故意设成"安全网"而非"业务上限"：配 3600s、7200s 都能直接保存生效，不会被一个拍脑袋的天花板拦住；
   超过 1 小时的预算会在启动日志给出 WARN 提示（允许，但提醒长静默请求会占住连接那么久）。
   保存渠道时超出上限会直接报错并说明派生关系；`cmd/airoute/nginx_timeout_test.go` 会校验
   `deploy/nginx-compose.conf` 里的 `proxy_read_timeout/proxy_send_timeout`（86400s）**必须等于**该常量，
   两者一旦漂移测试就红——因此不可能再出现"配了却被代理提前截断"。
4. **启动自检**打印实际链路并校验不变量（边界 > 上限）：

```
INFO timeout chain channel=罗总 first_token=5m0s channel_timeout=5m0s edge_timeout=1h0m0s
```

即：运维只设一个数，其余全部由它派生或由系统保证。

> 代价说明：渠道长期静默时不再有"早期失败"（此前 30s 就放弃并可切换渠道）。单渠道场景下早期失败没有收益，
> 反而会误伤慢但正常的上游；一旦恢复多渠道路由，需要重新引入"每渠道独立的首 token 窗口"才能快速 failover。

---

## 五、部署与遗留

本次修复通过 `scripts/docker-rebuild.sh` 滚动发布（两副本依次重建 + 健康检查），并已验证：

- 迁移 v21 已应用，`uq_usage_billing_id` 唯一索引存在；
- 连接池日志 `max_open=25, max_idle=8`；
- 新写入计费流水均带 `billing_id` 与微秒时间戳；
- 日志无 ERROR、无丢弃、无 flush 失败。

仍需在下次重建 postgres/redis 时生效：`max_connections=200`、postgres/redis 的 nofile 限制（网关与 LB 已生效）。
