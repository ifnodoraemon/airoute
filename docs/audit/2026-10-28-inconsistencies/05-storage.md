# 存储层一致性审计（2026-10-28）

## 概览

审计基线 **`1520d99`**（工作区并行改动较多，审计时以该 commit 的已提交内容为准）。范围 `internal/storage/` 全部 108 个文件（约 1.1 万行非测试代码），跨域只读 `internal/api`、`internal/router`、`internal/middleware`、`internal/controlplane` 中的 Repository 调用点以确认「谁写谁读」。共 16 条发现：写入不读 4 条、读取不写 1 条、双方言漂移 3 条、保留/索引不对称 3 条、死仓储 5 条，其中 12 条高置信度。

本轮新增的核心不一致：**时间戳写入侧完全没有做方言分支**（只有读取/清理侧做了），`request_traces` 的保留清理与采集开关耦合，`billing_settlements` 有 2 列只写不读，以及一批"已由批量版取代、只剩测试调用"的存储方法。

`usage_logs` 的 23 个写入列已逐列核对完毕（写入点 `repo_logs_writer.go:159` → 消费点 `repo_logs.go:329`、`repo_log_facets.go:91`、`repo_timeseries.go`、admin 导出/详情），**没有**发现「写入了但完全没人读」的列；数据库对象与 v40 删除清单也核对无残留引用。

## 发现

### E-01 成功请求的 trace 采样被连续判定三次，实际采样率是配置值的三次方；且 all 模式只有两个 chat 入口归档成功请求

- **类别**：写入不读（采样配置读入后与写入条件不一致）
- **位置**：`internal/storage/trace_capture.go:114`、`internal/api/trace_hooks.go:94`、`:111`
- **设计意图**：`GATEWAY_TRACE_SAMPLE` 是「成功请求归档百分比」（`trace_capture.go:69-80`，默认 100）。commit `70d7981` 明确要让 all 模式归档「被采样的成功请求」，而不是只归档失败。
- **现状**：一次成功请求从 `RecordSuccessTrace` 到落库要经过 **3 次** `Enabled(200)`，每次都重新掷一次随机数：`RecordSuccessTrace` 判一次（`trace_hooks.go:96`）→ 调 `RecordTrace` 再判一次（`trace_hooks.go:113`）→ `tc.Record` 内部又判一次（`trace_capture.go:118`）。`sample=10` 时实际归档率 ≈ 0.1% 而非 10%。另外 `RecordSuccessTrace` 全仓只有 2 个调用点（chat 单次 `handler.go:166`、chat 流式 `chat_stream_handler.go:117`），embedding/rerank/responses/anthropic/gemini/图片/审核等成功请求在 all 模式下永远不会被归档。
- **证据**：

```go
// trace_capture.go:107-110  每次调用独立掷随机数
if tc.sample >= 100 { return true }
return rand.Float64()*100 < tc.sample
```
```go
// trace_hooks.go:96  第 1 次
if tc == nil || c == nil || !tc.Enabled(http.StatusOK) { return }
// trace_hooks.go:113 第 2 次
if tc == nil || c == nil || !tc.Enabled(statusCode) { return }
// trace_capture.go:118 第 3 次
if !tc.Enabled(rec.StatusCode) { return }
```

- **影响**：中 — 运维按 `GATEWAY_TRACE_SAMPLE=5` 配了 5%，实际只拿到 ~0.01%，采样策略形同关闭；非 chat 协议的 `all` 模式没有成功快照。
- **置信度**：高
- **建议**：只在 `TraceCapture.Record` 一处判定采样（调用方传"是否已允许"或删除前置判定），并让各协议成功路径统一调 `RecordSuccessTrace`。

### E-02 `request_traces` 的 TTL 清理只在采集开关打开时才存在，关掉采集后历史快照永不清理

- **类别**：保留不对称
- **位置**：`internal/storage/trace_capture.go:47`、`:138-158`
- **设计意图**：`GATEWAY_TRACE_TTL_HOURS`（默认 72h）声明 trace 快照的保留期，清理 worker 每小时 `PurgeRequestTraces` 一次（`trace_capture.go:153-156`）。
- **现状**：唯一的清理逻辑活在 `worker()` 里，而 `InitTraceCapture` 在 `mode=="off"`（或 repo 为 nil）时**直接 return，不启 worker**（`:47-49`）。`GATEWAY_TRACE_CAPTURE=off` 是"不再采集"，但已归档的行（最长 72h，可被配到很大）从此没有任何删除路径；`PurgeRequestTraces` 全仓也只有这一个调用点。对照 `usage_logs` 的 retention 是独立 worker（`usage_retention.go:54-67`），与"是否继续写日志"解耦。
- **证据**：

```go
// trace_capture.go:47-49  采集关掉 → worker（含清理 ticker）不启动
if tc.mode == "off" || repo == nil { return tc }
// trace_capture.go:153-156  清理只在 worker 的 select 里
case <-purge.C:
    if n, err := tc.repo.PurgeRequestTraces(time.Now().Add(-tc.ttl)); err == nil && n > 0 {
```

- **影响**：中 — 关闭采集后 `request_traces` 变成只增不删（含 32KB 级 snippet），TTL 配置失效。
- **置信度**：高
- **建议**：把 trace 清理移到独立 worker（或 mode=off 时仍启动只做清理的 worker），使其与采集开关解耦。

### E-03 `billing_settlements.tenant_id` 与 `total_tokens` 只写不读

- **类别**：写入不读
- **位置**：`internal/storage/settlement.go:164`、`:186-189`；建表 `internal/storage/migrations_billing_settlements.go:16-25`
- **设计意图**：结算账本记录"这笔钱已从谁的钱包扣掉"，`tenant_id` 供租户归集、`total_tokens` 供用量对账（v23 迁移还从 `usage_logs` 回填了两列）。
- **现状**：写入侧共 3 处（同步结算 `settlement.go:186-189`、启动补齐 `billing_reconcile_job.go:178-179`、v23 回填）都填了这两列；读取侧全仓只有 `api_key`/`cost_units`/`billing_id`/`created_at` 被 SELECT（`billing_reconcile_job.go:209`、`:230`、`:258-262`），**没有任何语句读取 `tenant_id` 或 `total_tokens`**，也没有账本列表接口（v1.1 砍掉了 wallet ledger 版面）。
- **证据**：

```go
// settlement.go:164  账本列清单
settlementColumns = `billing_id, api_key, tenant_id, cost_units, total_tokens, created_at`
// billing_reconcile_job.go:209  唯一按 key 聚合的读点
LEFT JOIN (SELECT api_key, SUM(cost_units) AS settled FROM billing_settlements GROUP BY api_key) s ON s.api_key = k.key
// billing_reconcile_job.go:230  唯一按总额的读点
SELECT COALESCE((SELECT SUM(cost_units) FROM billing_settlements), 0) - ...
```

- **影响**：低 — 存储/回填白做，且 `tenant_id` 实际上没有索引但每次结算都写；对账口径只能按 key，无法按租户。
- **置信度**：高
- **建议**：要么给这两个列一个读点（租户级账本/对账视图），要么在下一版迁移里删列，别保留"看起来有用"的死列。

### E-04 写入侧时间戳一律绑 SQLite 文本格式，PostgreSQL `TIMESTAMPTZ` 列依赖会话时区；同仓已有方言正确实现却只在 response_contexts 使用

- **类别**：双方言漂移
- **位置**：`internal/storage/repo_logs_writer.go:200`、`internal/storage/settlement.go:188`、`internal/storage/repo_response_contexts.go:56-65`
- **设计意图**：PG 侧 `usage_logs.created_at` / `billing_settlements.created_at` 都是 `TIMESTAMPTZ`（`migrations_postgres.go:65`、`migrations_billing_settlements.go:23`），代码里也已有明确方言助手 `responseContextTimestamp`：PG 绑 `time.Time`，SQLite 绑文本（注释直言"绑错会让比较依赖会话时区"）。
- **现状**：两个写入点都把 UTC 时间格式化成 `sqliteTimestampLayout` **字符串**，两个方言共用：PG 收到无时区文本后按 `TimeZone` 解释，会话时区非 UTC 时落库瞬间整体偏移（控制台显示时间错位；与外部系统按 UTC 对时也对不上）。读取/清理侧则有的走文本、有的走 time.Time，口径继续分叉（见 E-05/E-06）。
- **证据**：

```go
// repo_logs_writer.go:200  两个方言都绑文本
createdAt.UTC().Format(sqliteTimestampLayout), log.BillingID,
// settlement.go:188  同上
createdAt.UTC().Format(sqliteTimestampLayout),
```
```go
// repo_response_contexts.go:56-65  同仓的正确写法（只此一处用）
func (r *Repository) responseContextTimestamp(at time.Time) any {
    if ... r.db.Dialect() == "postgres" { return at.UTC() }
    return at.UTC().Format(legacySecondTimestampLayout)
}
```

- **影响**：中 — 当前线上 PG `TimeZone=UTC`（前轮审计实测），故暂未爆发；换到本地时区的 PG（自建/国产化部署常见）后所有 usage 时间显示与跨系统对账整体偏移。
- **置信度**：高
- **建议**：把 `responseContextTimestamp` 提升为通用 `dialectTime(time.Time)` 并替换所有 `created_at` 写入点。

### E-05 `verification_codes.expires_at` 同一列存在三种时区约定：写入绑 UTC 文本、查询用 `NOW()`、清理绑 `time.Time`

- **类别**：双方言漂移
- **位置**：`internal/storage/repo_verification.go:17`、`:39-42`；`internal/storage/retention_maintenance.go:117-120`
- **设计意图**：`VerifyCode` 注释记录了历史事故（`datetime()` 是 SQLite-only，PG 全 500），因此显式方言分支：SQLite `datetime(expires_at) > datetime('now')` vs PG `expires_at > NOW()`。查找清理侧则刻意给 PG 绑 `time.Time`，"让比较不依赖会话时区"。
- **现状**：PG 列是 `TIMESTAMPTZ`（`migrations_postgres.go:139`），但**写入**仍绑 `"2006-01-02 15:04:05"` 文本 → PG 按会话时区解释，得到比预期早/晚一个时区偏移的瞬间；而**验证**用绝对时间 `NOW()`、**清理**用绝对 time.Time。写入与读取对同一列采用不同约定，会话时区非 UTC 时验证码会在预期前若干小时就"过期"（注册/改密直接失败）。
- **证据**：

```go
// repo_verification.go:17  写入：两个方言都绑文本
expiresAt.UTC().Format("2006-01-02 15:04:05")
// repo_verification.go:40-42  读取：PG 用绝对时间比较
codeQuery = `... AND expires_at > NOW() ORDER BY id DESC LIMIT 1`
// retention_maintenance.go:118-119  清理：PG 特意绑 time.Time
if r.db.Dialect() == "postgres" { cutoffValue = cutoff.UTC() }
```

- **影响**：中 — 服务器时区非 UTC 时验证码立刻过期（功能级故障）；当前线上 UTC 故潜伏，属项目记忆里登记过的同类事故未收尾。
- **置信度**：高
- **建议**：写入改用与清理相同的方言助手（PG 传 `time.Time`），并补一条"写入→过期→清理"的双方言用例。

### E-06 `video_tasks` 用一份 DDL 建表（PG 得到无时区 `TIMESTAMP`），且清理绑 UTC 文本，与全库 `TIMESTAMPTZ` 约定不一致

- **类别**：双方言漂移
- **位置**：`internal/storage/repo_video_tasks.go:19-26`、`internal/storage/retention_maintenance.go:147`
- **设计意图**：保留期清理按 `created_at` 删除 30 天前的任务（`retention_maintenance.go:143-153`），默认值来自数据库 `CURRENT_TIMESTAMP`。
- **现状**：`createVideoTasks` 是**唯一一个不分支方言**的建表函数（对比 `createRequestTraces` 各写一份、`createResponseContexts` 分支、`billing_settlements` 分支）。PG 侧 `created_at TIMESTAMP`（无时区）→ `CURRENT_TIMESTAMP` 存的是会话本地墙钟；SQLite 的 `CURRENT_TIMESTAMP` 是 UTC。清理却统一把 cutoff 格式化成 **UTC** 文本去比较 → PG 上两者基准不同，保留窗口整体偏移一个时区。另 `video_tasks` 的时间列也无 `idx_...created`（见 E-07）。
- **证据**：

```go
// repo_video_tasks.go:20-24  单一 DDL，两个方言共用
CREATE TABLE IF NOT EXISTS video_tasks (
  id TEXT PRIMARY KEY, upstream_id TEXT NOT NULL, owner TEXT NOT NULL,
  model TEXT NOT NULL, channel_id BIGINT NOT NULL, channel_name TEXT NOT NULL,
  base_url TEXT NOT NULL, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
```
```go
// retention_maintenance.go:147-152  统一 UTC 文本
cutoffValue := cutoff.UTC().Format(legacySecondTimestampLayout)
`DELETE FROM video_tasks WHERE id IN (SELECT id FROM video_tasks WHERE created_at < ? ORDER BY created_at LIMIT ?)`
```

- **影响**：低 — 30 天窗口错一个时区，行数不大；但"一份 DDL 通吃双方言"是同仓其它建表函数已经修掉的模式，属回归风险点。
- **置信度**：高
- **建议**：`createVideoTasks` 按方言分支（PG 用 `TIMESTAMPTZ`），清理用统一时区助手。

### E-07 运维索引 `idx_video_tasks_owner(owner, created_at)` 既服务不了清理，也服务不了查询

- **类别**：保留不对称（索引与真实查询形态不匹配）
- **位置**：`internal/storage/migrations_operational_indexes.go:26`；查询 `internal/storage/retention_maintenance.go:150-152`、`internal/storage/repo_video_tasks.go:125-129`
- **设计意图**：该索引的注释写着"Operator lookups by account **and the retention purge** both need (owner, created_at)"。
- **现状**：保留清理只按 `created_at` 过滤并 `ORDER BY created_at`——`(owner, created_at)` 的前导列不是 `owner`，无法服务；唯一读接口 `GetVideoTask` 走主键 `id + owner`，也用不上它。两半都没被任何 SQL 用到，纯写放大。
- **证据**：

```go
// migrations_operational_indexes.go:26
`CREATE INDEX IF NOT EXISTS idx_video_tasks_owner ON video_tasks(owner, created_at)`
// retention_maintenance.go:150-152  只过滤 created_at（无 owner 条件）
`DELETE FROM video_tasks WHERE id IN (SELECT id FROM video_tasks WHERE created_at < ? ORDER BY created_at LIMIT ?)`
```

- **影响**：低 — 一次多余索引维护；清理仍是全表扫描（表小，暂不痛）。
- **置信度**：高
- **建议**：改建成 `(created_at)`（服务清理）并单独保留 `(owner)`（服务按账号查）。

### E-08 已撤销密钥的 `request_traces` 对本人不可见：trace 作用域用"未删除键"，logs/stats 用"历史键"

- **类别**：写入不读（写入的行被读取路径过滤掉）
- **位置**：`internal/controlplane/admin_traces.go:32`、`internal/storage/repo_keys.go:248-249`、`:277-291`
- **设计意图**：审计数据的租户作用域要覆盖"历史上用过的密钥"，否则撤销密钥等于抹掉客户的计费/排障证据——`ListHistoricalAPIKeysByUser` 的注释正是这么写的，logs（`admin_logs.go:66`）与 stats（`admin_overview.go:77`、`:160`）都用了它。
- **现状**：只有 traces 端点取的是 `ListAPIKeysByUser`，其 SQL 带 `WHERE user_id = ? AND status <> 'deleted'`。因此普通用户对自己已撤销密钥的 trace 快照（写入时是 digest）永远查不到——非 admin 的 trace 列表被收窄成"当前有效密钥"，与审计页/统计页口径不一致。
- **证据**：

```go
// admin_traces.go:32  用"未删除"
keys, err := h.repo.ListAPIKeysByUser(user.ID)
// repo_keys.go:249  隐含 status <> 'deleted'
`... FROM api_keys WHERE user_id = ? AND status <> 'deleted' ORDER BY id ASC`
// admin_logs.go:66  同场景却用历史键
userKeys, _ := h.repo.ListHistoricalAPIKeysByUser(user.ID)
```

- **影响**：中 — 客户排障时看不到被撤销密钥的请求快照；对账/审计口径与 logs 不一致。
- **置信度**：高
- **建议**：traces 作用域统一改用 `ListHistoricalAPIKeysByUser`。

### E-09 public id 解析只接线在日志端点，统计端点仍把 `ak_…` 当凭据做 SHA-256，静默返回全零

- **类别**：死仓储（`APIKeyDigestByPublicID` 半接线）
- **位置**：`internal/controlplane/admin_overview.go:50`、`:115`、`:139`、`:187`；对照 `internal/controlplane/admin_logs.go:76-84`
- **设计意图**：commit `1520d99` 把控制台改成用 `ak_…` 显示密钥，并明确"console 传回它显示的标识，审计列存 digest，先解析再过滤"——`admin_logs.go` 因此新增 `IsAPIKeyPublicID` → `APIKeyDigestByPublicID` 解析。
- **现状**：同一个 commit 只改了日志端点。`GetStatsOverview`/`GetStatsTimeSeries` 的 `key` 查询参数依旧原样透传（`:115`、`:190`），最后落到 `GetStatsOverviewForKeys` → `NormalizeStoredAPIKeyReference`，后者对任何"不是 64 位 hex"的值直接 `HashAPIKey`（`repo_logs_writer.go:52-60`）→ 得到一个匹配不到任何 key 的 digest → 指标全零，不报错。控制台当前用 `key_id`（数字 id）所以未暴露，但 API/旧前端传 `ak_…` 会静默变零。
- **证据**：

```go
// admin_overview.go:50 / :115  原样透传
keyParam := strings.TrimSpace(c.Query("key"))
stats, err := h.repo.GetStatsOverviewForKeys([]string{keyParam})
// repo_logs_writer.go:56-59  非 digest 一律哈希（含 ak_…）
if IsAPIKeyHash(value) { return value }
return HashAPIKey(value)
// admin_logs.go:78-80  同场景的正确解析（只存在于日志端点）
if storage.IsAPIKeyPublicID(targetKey) {
    if digest, err := h.repo.APIKeyDigestByPublicID(targetKey); err == nil && digest != "" {
```

- **影响**：低 — 面板对 `ak_…` 静默归零且无告警；`APIKeyDigestByPublicID` 只被一个端点使用。
- **置信度**：中
- **建议**：在 `LogFilter`/stats 入口统一做一次"public id → digest"归一化，或让 `NormalizeStoredAPIKeyReference` 对 `ak_` 前缀直接报错而不是哈希。

### E-10 FTS5 索引只维护 INSERT/DELETE，v32 迁移 UPDATE `error_message` 后索引陈旧

- **类别**：写入不读（索引与内容漂移）
- **位置**：`internal/storage/migrations_usage_fts.go:34-46`；`internal/storage/migrations_usage_log_cleanup.go:72-88`
- **设计意图**：注释称"usage_logs rows are append-only, so insert and purge are the only events the index has to follow"，因此只建 `AFTER INSERT` 与 `AFTER DELETE` 触发器。
- **现状**：`usage_logs` 并非只追加：v24 改 `status_code`（不影响 FTS 列），v32 `rewriteLegacyUsageLogReasons` 会 `UPDATE usage_logs SET error_message = ...`，而 `error_message` 正是 FTS5 的五列之一。升级库上 v20 建好索引（含 `rebuild`），随后 v32 改写文本 → 索引里仍是旧文本：搜新原因文本搜不到（`id IN (SELECT rowid FROM usage_logs_fts WHERE ... MATCH ?)` 无命中），搜旧脏文本反而命中已修复的行。
- **证据**：

```go
// migrations_usage_fts.go:37-44  只有 ai / ad 两个触发器
`CREATE TRIGGER IF NOT EXISTS usage_logs_fts_ai AFTER INSERT ON usage_logs ...`
`CREATE TRIGGER IF NOT EXISTS usage_logs_fts_ad AFTER DELETE ON usage_logs ...`
// migrations_usage_log_cleanup.go:77  v32 改写被索引列
`UPDATE usage_logs SET error_message = REPLACE(error_message, '%!w(<nil>)', ?) WHERE error_message LIKE '%!w(<nil>)%'`
```

- **影响**：低 — 只影响升级库里被 v32 修过的那批历史行的关键词检索。
- **置信度**：中
- **建议**：加 `AFTER UPDATE OF trace_id, chat_id, session_id, model, channel, error_message` 触发器（或 v32 末尾执行一次 `'rebuild'`）。

### E-11 `SetGlobalRepository` 是不可达死 API：全局单例的写入绕过了 setter

- **类别**：死仓储
- **位置**：`internal/storage/repository.go:70-73`、`:81-85`
- **设计意图**：`globalRepo` 是给非仓储层（`middleware` 的权威密钥回退查询）用的进程级单例；`SetGlobalRepository` 提供注入点。
- **现状**：deadcode 从 `main` 不可达（Lead 提供的基线）。原因清楚：唯一的赋值在 `NewRepository` 里直接写包变量 `globalRepo = r`（`:83`），生产路径从不调用 setter；`SetGlobalRepository` 只在测试里被引用（30 处，用于保存/恢复 `GetGlobalRepository`）。读取侧是真实生产路径（`middleware/auth_storage_fallback.go:55`、`middleware/auth.go:239`），所以是"一半单例机制"——读有生产用户、写只有测试用户。
- **证据**：

```go
// repository.go:71  只有测试调用
func SetGlobalRepository(repo *Repository) { globalRepo = repo }
// repository.go:83  生产赋值绕过 setter
func NewRepository(db *DB) *Repository { r := &Repository{db: db}; globalRepo = r; return r }
// middleware/auth_storage_fallback.go:55  生产读点
repo := storage.GetGlobalRepository()
```

- **影响**：低 — 死 API + 单例赋值路径不统一（想换实现时 setter 不生效的陷阱）。
- **置信度**：高
- **建议**：删掉 setter（测试改为用 `NewRepository` 后恢复旧值），或在 `NewRepository` 里统一走 setter。

### E-12 `ReconcileMoney` 只有测试调用：注释声称"周期性运行"，生产从未接线

- **类别**：死仓储
- **位置**：`internal/storage/money_reconcile.go:15-18`、`internal/storage/billing_reconcile_job.go:101-157`
- **设计意图**：`ReconcileMoney` 的注释："meant to be run periodically after the dual-write rollout to prove that the legacy REAL columns and the integer mirrors agree"。
- **现状**：全仓无生产调用点（仅 `money_test.go:53`），无 CLI/端点接线（`cmd/` 下无 reconcile 入口）。真正跑起来的是 `BillingReconciler.runOnce`，它用自己的一套 SQL（`ledgerDrift` 走 `billing_settlements`、`walletMirrorDrift` 走 `users`），**并不调用** `ReconcileMoney`。所以"按 key 对比 usage_logs 与 api_keys"的这条对账口径只存在于测试里。
- **证据**：

```go
// money_reconcile.go:16-18
// ReconcileMoney compares per-key integer cost ledgers and reports drift. It is
// meant to be run periodically after the dual-write rollout ...
func (r *Repository) ReconcileMoney(limit int) ([]*MoneyDrift, error) {
```

- **影响**：低 — 少一条对账口径（`usage_logs` vs `api_keys` 的按 key 对比），不影响资金；但"已实现、被测过、没人跑"容易误以为在跑。
- **置信度**：高
- **建议**：把 `ReconcileMoney` 接进 `runOnce` 发布成 metric，或删除并同步删除文档声称。

### E-13 `GetAPIKeyBySecret` 文档称"请求路径鉴权必须使用"，实际生产无人调用

- **类别**：死仓储
- **位置**：`internal/storage/repo_keys.go:378-399`
- **设计意图**：注释写得很重："This is the lookup the request-path authentication must use; GetAPIKeyByKey expects an already hashed identifier and is for internal/administrative use."
- **现状**：生产鉴权不用它：`middleware/auth_storage_fallback.go:65` 与 `middleware/auth.go:242` 都是自己 `HashAPIKey` 后调 `GetAPIKeyByKeyContext`。`GetAPIKeyBySecret` 只有测试调用（10 处）。文档与现状相反，后来者按注释改鉴权会走一条没人验证过的路径。
- **证据**：

```go
// repo_keys.go:391
rec, err := r.GetAPIKeyByKeyContext(ctx, HashAPIKey(secret))
// auth_storage_fallback.go:65  生产路径：调用方自己哈希
rec, err := repo.GetAPIKeyByKeyContext(lookupCtx, presentedHash)
```

- **影响**：低 — 死代码 + 权威文档误导。
- **置信度**：中
- **建议**：让中间件改调 `GetAPIKeyBySecret`（收口常量时间比较与删除态判定），或删除该函数并修正注释。

### E-14 一批"已被批量/带过滤版本取代"的单条方法只剩测试调用

- **类别**：死仓储
- **位置**：`internal/storage/repo_channels.go:132`、`internal/storage/repo_pricing.go:130`、`:94`、`internal/storage/repo_wallet.go:25`、`internal/storage/repo_logs.go:203`、`internal/storage/repo_logs_writer.go:91`、`:109`
- **设计意图**：这些方法各自有注释说明用途（渠道详情、单模型价格、单条 fallback、单张兑换码、最近日志、单条/批量写日志）。
- **现状**：生产都已切到批量或带条件版本，单条版只剩测试：`GetChannel`→`ListChannels`（`channel_service.go:75`，全仓无任何生产代码调用 `GetChannel`，且 `channel_service.go:22` 注释说明 `GetChannel+UpdateChannel` 的读改写方案已被取代）；`GetModelPrice`→`GetModelPriceExact`/分组版（`route_service.go:135` 注释"It used to load the row with GetModelPrice(), which is hard-wired to the ..."）；`SetModelFallback`→`SetModelFallbacks`（`sync.go:35`）；`CreateRedemptionCode`→`CreateRedemptionCodes`；`ListUsageLogs`→`ListUsageLogsWithFilterAndCount`（`admin_logs.go:108`）；`RecordUsageLog`/`BatchRecordUsageLogs`→带 ctx 的版本（`async_logger.go:226`、`:335`）。
- **证据**：

```go
// repo_channels.go:132  生产无人调用（channel_service 用 ListChannels）
func (r *Repository) GetChannel(id int64) (*ChannelRecord, error) {
// repo_pricing.go:94  生产用批量 SetModelFallbacks（sync.go:35）
func (r *Repository) SetModelFallback(model, fallbackModel string, enabled bool) error {
```
```go
// repo_logs_writer.go:91 / :109  生产走 Context 版本
func (r *Repository) RecordUsageLog(log *UsageLogRecord) error { ... }
func (r *Repository) BatchRecordUsageLogs(logs []*UsageLogRecord) error { ... }
// async_logger.go:226  真正的生产调用
err := al.repo.BatchRecordUsageLogsContext(flushCtx, batch)
```

- **影响**：低 — 维护面扩大；单条版若被误用会绕过 ctx 截止时间（正是 `BatchRecordUsageLogsContext` 注释警告的问题）。
- **置信度**：高
- **建议**：删除仅测试使用的单条包装，测试改调生产版本；保留的包装方法在注释里标清"仅测试/兼容"。

### E-15 `StatsOverview.PeakRPM/PeakTPM` 只读不写，`/stats/overview` 永远返回 0

- **类别**：读取不写
- **位置**：`internal/storage/repository.go:28-29`、`:104-144`；前端兜底 `web/src/components/DashboardView.jsx:202-203`
- **设计意图**：overview 里带"Peak RPM in recent timeframe"，供仪表盘在时序数据未就绪时回退展示。
- **现状**：`GetStatsOverview` 的 Scan 目标里没有这两个字段，全函数也没有任何赋值 → 字段恒为零值，JSON 恒为 `0`。真正被赋值的是 `TimeSeriesData.PeakRPM/PeakTPM`（`repo_timeseries.go:359-360`），前端优先读时序、失败时才读 `stats.peak_rpm`，于是"兜底"永远是 0。
- **证据**：

```go
// repository.go:28-29  声明并序列化
PeakRPM int64 `json:"peak_rpm"`    // Peak RPM in recent timeframe
PeakTPM int64 `json:"peak_tpm"`    // Peak TPM in recent timeframe
// repository.go:121-122  Scan 列表里没有这两个字段
row.Scan(&stats.TotalRequests, ..., &stats.SuccessRequests, &stats.TTFTSamples)
// DashboardView.jsx:202  前端把它当兜底
const peakRPM = timeseriesData?.peak_rpm ?? stats.peak_rpm ?? 0;
```

- **影响**：低 — 时序接口不可用时仪表盘峰值显示 0（把"无数据"伪装成"峰值为 0"）。
- **置信度**：高
- **建议**：要么在 `GetStatsOverview` 里补一次 60s 窗口峰值查询，要么删除这两个字段并让前端不再兜底。

### E-16 `usage_logs` 的 trace/chat/session 单列索引无法服务其唯一查询形态（`= ? OR LIKE '%?%'`）

- **类别**：保留不对称（索引与真实查询形态不匹配）
- **位置**：`internal/storage/migrations_sqlite.go:237-239`（PG 同 `migrations_postgres.go:186-188`）；查询 `internal/storage/repo_logs.go:223-234`
- **设计意图**：v1/v7/v10 为控制台过滤器建了 `idx_usage_trace/chat/session`；v18 又为 tenant/key/model 补了 `(列, created_at)` 复合索引，理由写在 `migrations_usage_indexes.go:3-7`："scoped filter combined with a created_at range is exactly the shape of every page load"。
- **现状**：trace/chat/session 三个过滤器的 WHERE 恒为 `(col = ? OR col LIKE '%?%' ESCAPE '\')`——`LIKE` 分支带前导 `%`，任何 B-tree 索引都用不上；OR 又要求两侧同时可评估，因此单列索引在这条查询里不会被选择。而 v18 只给另外三列补了复合索引，这三列既没有复合索引、单列索引也白建（纯写放大）。同样是"精确+LIKE 双匹配"的设计，只有一半列配了能用的索引。
- **证据**：

```go
// repo_logs.go:224-225  唯一过滤形态：等于 OR 前导模糊
where += ` AND (trace_id = ? OR trace_id LIKE ? ESCAPE '\')`
args = append(args, f.TraceID, "%"+escapeLikePattern(f.TraceID)+"%")
// migrations_usage_indexes.go:12-14  v18 只补了这三列
{"idx_usage_tenant_created", "tenant_id, created_at"},
{"idx_usage_key_created", "api_key, created_at"},
{"idx_usage_model_created", "model, created_at"},
```

- **影响**：低 — 三列过滤仍全表扫描（`usage_logs` 是最大的审计表），同时每条插入多维护 3 个索引。
- **置信度**：中
- **建议**：把"等于+前导 LIKE"拆成两段查询（或对超长/含通配输入只走精确匹配），再按真实形态建索引；确认无用的单列索引在迁移中删除。

## 已排除的疑似项

- **迁移版本 30/35/36/38 的缺口**：SQLite 与 PG 的注册表完全一致（都缺同样 4 个版本），v40 `dropLegacyGovernanceObjects` 的注释说明 v1.1 治理切片（org/budget/wallet/usage_daily）已被整体切除，缺口是那次切除的遗留而非漂移；runner 按注册表逐条校验，不影响升级。
- **v40 删除对象后的残留引用**：全仓 grep `org_id`/`cost_basis_units`/`usage_logs.channel_id`/`usage_daily`/`wallet_transactions`/`budgets` 在非测试代码中零命中（`repo_reconciliation.go:37` 的 `ChannelID` 是 `channels.id`，`repo_video_tasks.go` 的 `channel_id` 是独立表列）；`dropLegacyGovernanceObjects` 先删索引再删列，SQLite `DROP COLUMN` 与 PG `DROP COLUMN IF EXISTS` 均已分支。无残留读写。
- **`billing_settlements` 无保留期**：账本是资金凭证，且 `ledgerDrift`/`auditLag` 依赖全量 `SUM(cost_units)`，删除会直接破坏对账；`auditLag` 用 `usage_log_pruned_cost_units` 基线补平被裁剪的审计行——属刻意设计，不算不对称。
- **`verification_codes`/`video_tasks` 的机会式清理**：`retention_maintenance.go:13-19` 明确说明"这类表写得少，写路径顺手删即可"，是有意选择，不按"worker 未启动"报。
- **FTS5 仅 SQLite / pg_trgm 仅 PG**：`migrations_usage_fts.go:13-15` 与 `migrations_usage_search.go:17-24` 都有方言提前返回与降级注释，属设计内差异。
- **`usage_logs` 23 个写入列逐一核对**：`cost_units`（裁剪基线/对账）、`billing_id`（幂等/对账）、`usage_source`（前端"估算"标记）、`ttft_ms`/`duration_ms`/`is_off_peak`/`off_peak_discount`（控制台展示）、`pricing_snapshot`（`LogDetailModal.jsx` 复核单价）均有真实读点，没有整列只写不读。
- **`request_traces.api_key` 是否误存 public id**（Lead 指定核查）：否。`middleware/auth.go:329` 写入 ctx 的是 digest（`effectiveKey.Key`），`trace_hooks.go` 直接取用后经 `RecordRequestTrace` 落库；三张审计表（usage_logs/billing_settlements/request_traces）存的都是 digest，v29 迁移也把三表一起重写。唯一半接线点在统计端点的 `key` 参数（见 E-09）。
- **`recharge_orders.stripe_session_id`**：虽无按该列的查询，但 `GetRechargeOrder`/`ListRechargeOrders` 会读出并返回给客户端（`payment_handler.go:165` 写入），属"读出来展示"，不按写入不读报。
- **`MaskAPIKey`**：生产在用（`admin_logs.go:216`），非死代码。
- **`zz_tmp_settlement_bench_test.go`**：按 Lead 更新，该文件已在本轮之前被并行会话删除，不再记录。
- **`inClause[T]` 辅助函数（`94cfe73`）**：抽样核对 `repo_logs.go:442`、`repo_keys.go:454/468`，prefix 参数与占位符顺序正确，未发现遗留的手写 IN 列表错位。

## 覆盖面

- **审计基线**：`1520d99`（`git log --oneline -1` 确认；工作区另有并行会话的 `.docx` 与 `docs/audit/high-concurrency-billing-2026-10-28.md` 未纳入）。报告内所有 `path:line` 均已用 `git show 1520d99:<file>` 回查确认——审计期间并行会话已在工作区改动 `internal/storage/settlement.go`、`repo_keys.go`、`repo_pricing.go`、`db.go`、`wallet_deduct.go` 并新增 `settlement_batcher.go`，故不以工作区现状为准。
- **逐行阅读**：`db.go`、`repository.go`、`sqlutil.go`、`money.go`、`settlement.go`、`wallet_deduct.go`、`money_reconcile.go`、`billing_reconcile_job.go`、`repo_logs.go`、`repo_logs_writer.go`、`repo_log_facets.go`、`repo_timeseries.go`、`repo_traces.go`、`trace_capture.go`、`async_logger.go`、`usage_spill.go`、`usage_retention.go`、`retention_maintenance.go`、`repo_keys.go`、`repo_wallet.go`、`repo_verification.go`、`repo_video_tasks.go`、`repo_response_contexts.go`、`migrations_runner.go`、`migrations_sqlite.go`、`migrations_postgres.go`、`migrations_cleanup.go`、`migrations_*` 全部分支文件、`artifact_storage.go`/`local_storage.go`（接口与驱动选择）。
- **方法调用点抽样**：对 `internal/storage` 343 个函数/方法做了"包外引用 / 包内引用 / 测试引用"三分统计，逐个人工确认了 10 个"仅测试调用"候选（E-11~E-14 报告其中 4 组，其余包装方法确认为 gRPC/CLI 之外的真实内部调用）。
- **方言扫描**：grep `Rebind`、`Dialect()`、`sqlite`、`postgres`、`strftime`、`datetime(`、`NOW()`、`ILIKE`、`INSERT OR IGNORE`、`RETURNING`，逐处确认两侧实现（`db.go` 的 Rebind/rewriteLikeToILike/InsertGetID、各迁移分支、`timeFilterClause`、`walletMirrorDrift`、`backfillUsageLogCostUnits`、FTS/pg_trgm 均已核对，未报）。
- **保留期清单**：`usage_logs`(365d，独立 worker)、`request_traces`(72h，随采集)、`verification_codes`(7d，写路径)、`video_tasks`(30d，写路径)、`response_contexts`(7d，写路径)、`billing_settlements`/`redemption_codes`/`recharge_orders`(永久，凭证)；`oauth_identities`/`system_settings`/`channels`/`api_keys`/`users`/`model_prices` 有业务上界。
- **风险点未深挖**（超时/超范围，建议后续单独立项）：`repo_timeseries` 的 `GROUP BY` 在 PG 上对 `EXTRACT(EPOCH FROM created_at)` 无法利用 `idx_usage_created`（表达式分组）；`artifact_storage` 的对象文件没有保留期（只随技能更新/删除单点清理）；`usage_spill` 的 claimed 残留文件依赖下次启动回收。
- **未运行测试**：本轮为只读审计，未执行 `go test`/`go build`，未改动任何生产或测试代码；唯一写入为本报告。
