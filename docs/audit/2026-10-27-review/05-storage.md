# Storage / Infra Review — 2026-10-27

- **Date**: 2026-10-27 (Asia/Shanghai)
- **HEAD**: `64e749a` (`fix: resolve functional audit findings across auth billing and workflows`)
- **Reviewer**: storage-auditor (task-5)
- **Scope**: `internal/storage/**`（DB 包装/方言重绑、migrations、repository、repo_*.go，排除 billing 专属文件：settlement / wallet_deduct / usage_spill / billing_reconcile_job / repo_logs / calibration；含 logs writer、video_tasks、oauth identities、pricing repo、shared identity、artifact/S3）、`internal/distributed/**`、`internal/telemetry/**`、`internal/config/**`、`internal/validator/**`
- **约束**: 只读审计。未修改任何 tracked 源码、未重启/停止容器、未执行 `docker compose build/up`、未写入数据库（仅 `SELECT`/catalog 查询）、探针全部写在 `/tmp/airoute-audit-storage` 私有 worktree 中。

## 测试与证据

| 检查 | 命令 / 方法 | 结果 |
|---|---|---|
| 基线测试 | `go test ./internal/storage/ ./internal/distributed/ ./internal/telemetry/ ./internal/config/ ./internal/validator/ -race -count=1` | **ok**（storage 35.6s / distributed 1.36s / telemetry 1.66s / config 2.58s / validator 1.81s） |
| 探针 A：日志环形缓冲 | worktree `go test ./internal/telemetry -run TestProbeRingBufferAppendAfterUnsubscribePanics -count=3` | **FAIL（确认）** 3/3 `panic: send on closed channel` |
| 探针 B：Redis URL 泄漏 | `go test ./internal/distributed -run TestProbeInitRedisLogsURLWithPassword` | **FAIL（确认）** 日志含 `SuperSecretPassw0rd` |
| 探针 C：Postgres DSN 泄漏 | `go test ./internal/storage -run TestProbeOpenDBLogsKeywordDSNPassword` | **FAIL（确认）** 日志含 `password=DsnSecretPassw0rd` |
| 探针 D：trace 脱敏 | `go test ./internal/storage -run TestProbeTruncateSnippetRedactsBearerToken` | **FAIL（确认）** 输出保留 `sk-airoute-probe-secret-123456` |
| 探针 E：配置 `$` 转义 | `go test ./internal/config -run TestProbeExpandEnvManglesDollarInSecret` | **FAIL（确认）** `pa$$word` → `paword` |
| 探针 F：off-peak 窗口 | `go test ./internal/storage -run TestProbeLegacyOffPeakWindowIsOverwrittenByInjectedSlot` | **FAIL（确认）** 存储被改写为 `00:00-08:30/0.5`；23:00 无折扣、03:00 折扣 0.50 |
| 线上只读核对 | `docker exec airoute-postgres psql -U airoute -d airoute -c "SELECT ..."` | `TimeZone=UTC`；`schema_migrations` 1–26 全部应用；`pg_indexes` 全量导出；`usage_logs` 51,426 行（7d 48,057 / 24h 18,606，87MB）；`request_traces` 461；`verification_codes` 0；`video_tasks` 0；`oauth_identities` 0 |

**已复核的前轮结论**（确认仍然成立 / 未回归）：

- README §一.7 `DB` 包装器补齐 `ExecContext/QueryContext/QueryRowContext`（`db.go:97-150`）：`DB` 与 `Tx` 各 3 个 Context 变体均调用 `Rebind`，无遗漏。
- concurrency.md F2 连接预算：`GATEWAY_DB_MAX_OPEN_CONNS`/`MAX_IDLE_CONNS`（默认 25/8）+ `statement_timeout=30000` 注入 DSN（`db.go:226-275`）已落地。
- README §一.5 环形缓冲初始化顺序：`GlobalRingBuffer` 为变量初始化器、容量 5000、`http_access` 跳过（`ring_buffer.go:44-56`, `:295-297`）；**但订阅/广播并发缺陷仍在，见 ST-01**。
- functional-details F18（OAuth 身份 `(provider, subject)`）：`oauth_identities` 主键与 `GetUserByOAuthIdentity` 正确；线上表已建。
- `migrations_sqlite.go` / `migrations_postgres.go` 迁移序列 1–26 完全对齐（version/name 一一对应），SQLite 专属实现（FTS5）在 `addUsageLogFTSIndex` 内按方言提前返回，PG 侧不执行。

## Findings

| ID | 严重度 | 位置 | 标题 | 已确认 | 影响 |
|---|---|---|---|---|---|
| ST-01 | P1 | `internal/telemetry/ring_buffer.go:101-110`, `:178-195` | 环形缓冲 `Append` 向已关闭订阅 channel 发送导致 panic | **是** | 后台 goroutine 打日志时进程直接退出；handler goroutine 则请求中断 |
| ST-02 | P1 | `internal/storage/repo_pricing.go:192-215` | 合成 off-peak slot 覆盖运维配置的单窗口，折扣时间窗与折扣率双双错误 | **是** | 配置窗口内按原价收费（多收），合成窗口内按 0.5 收费（少收） |
| ST-03 | P1 | `internal/storage/repo_traces.go:136-154` | `TruncateSnippet` 未脱敏 `Bearer <token>`，凭证落库 | **是** | `request_traces` 长期保存上游/客户密钥明文（最长 72h + 未索引大字段） |
| ST-04 | P1 | `internal/distributed/redis.go:53` | `REDIS_URL`（含密码）在解析失败时整体写入日志 | **是** | Redis 口令进入容器日志/日志平台 |
| ST-05 | P2 | `internal/storage/db.go:271` | libpq keyword DSN 整串写入日志，含 `password=` | **是** | Postgres 口令进入日志（URL 形式安全，keyword 形式泄漏） |
| ST-06 | P2 | `internal/config/config.go:234-243` | `os.ExpandEnv` 作用于整份 YAML，配置值里的 `$` 被当变量吞掉 | **是** | 含 `$` 的管理员密码/渠道 key/S3 secret 被静默改写，登录或上游鉴权失败 |
| ST-07 | P2 | `internal/storage/trace_capture.go:132,148,152` | 使用 `slog.Default()` 而非 `telemetry.Logger`，输出非 JSON 且不进环形缓冲 | 静态 | 容器日志出现非 JSON 行（前轮已修过同类 banner 问题），控制台看不到这些告警 |
| ST-08 | P2 | `internal/storage/async_logger.go:127` | Redis XADD 失败时每条记录打一条 WARN，未做限频 | 静态 | Redis 抖动时按记录数刷日志（前轮已对同类告警加 `WarnLogger`，此处遗漏） |
| ST-09 | P2 | `internal/storage/repo_verification.go:20-26`；`migrations_sqlite.go:252` / `migrations_postgres.go:201` | 验证码表无任何清理，索引 `(email, code)` 与真实查询形态不匹配 | 静态（线上 0 行，schema 已核） | 注册/改密长期运行后无界增长；`VerifyCode` 走 `email+purpose+used+expires_at+ORDER BY id` 用不上该索引 |
| ST-10 | P2 | `internal/storage/repo_users.go:87-99`；`migrations_postgres.go:71` + 线上索引 | `users.email` 无唯一约束，`GetUserByEmail` 无 `ORDER BY`，注册是 check-then-insert | 静态（线上无重复邮箱样本） | 并发注册可产生同邮箱账号；邮箱登录/OAuth 绑定可能取到任意一条 |
| ST-11 | P2 | `internal/config/config.go:144-158`；`internal/config/storage_config.go:28-48` | S3 凭证硬编码为默认值；YAML 只写部分 `storage` 字段时整块被丢弃 | 静态 | 源码内固定 secret；`driver: s3` 但缺字段时静默用默认密钥或空值 |
| ST-12 | P2 | `internal/config/config_policies.go:58-71` | `INITIAL_USER_BALANCE`/`DEFAULT_TRIAL_BALANCE` 解析失败或为负时静默回落 5.0 | 静态 | 配置写错即给每个新用户 5 元额度，无告警 |
| ST-13 | P2 | `internal/storage/repository.go:118`, `:194` | 控制台 `SavedCost` 用硬编码 1.8 CNY/1M cached token 计算 | 静态 | 缓存省钱数字与 `model_prices.cache_read_price` 无关，属编造指标 |
| ST-14 | P2 | `internal/storage/repo_timeseries.go:175-210` | 时间序列把时间窗内**全部** usage_logs 行拉进 Go 侧聚合，无 SQL 聚合/上限 | 静态 + 线上行数 | 7d 已 48,057 行；3s 缓存过期后每次刷新整窗扫描，随流量线性恶化 |
| ST-15 | P2 | `internal/storage/repo_traces.go:89-99` + 线上索引 | `ListRequestTraces` 按 `api_key` 过滤但 `request_traces` 无 `api_key` 索引 | 静态 + 线上索引 | 租户范围查询退化为顺序扫描（该表存 32KB 级 snippet，是体积最大的表） |
| ST-16 | P2 | `internal/storage/repo_video_tasks.go:12-16` | `video_tasks` 仅主键，无 owner/created_at 索引、无保留期、明文存上游 `base_url` | 静态 + 线上索引 | 表无界增长；按 owner 维度的运维查询无索引 |

严重度计数：**P0 = 0，P1 = 4，P2 = 12**（16 项，其中 6 项有可复现探针）。

---

## 明细

### ST-01【P1，已确认】环形缓冲广播与退订竞态 → `send on closed channel`

- **触发**：管理员打开控制台「系统日志」实时流（`GET /api/v1/admin/system/logs/stream`，`internal/controlplane/admin_system_logs.go:68` 调用 `rb.Subscribe()`，`:69` `defer cancel()`），随后关闭页面/HTTP 连接断开；与此同时任意 goroutine 打一条进入环形缓冲的日志。
- **代码路径**：`LogRingBuffer.Append` 在锁内复制订阅者切片（`ring_buffer.go:96-100`），**解锁后**再逐个 `select { case ch <- entry: }` 发送（`:101-110`）；`Subscribe` 的 `cancel` 在锁内 `close(existing)`（`:187-193`）。取消发生在「复制完成」与「发送」之间时，发送落在已关闭 channel 上。
- **证据**：探针 `TestProbeRingBufferAppendAfterUnsubscribePanics`（worktree）3/3 复现：`Append panicked after a subscriber cancelled: send on closed channel`。生产语义上，日志 goroutine 若是后台 worker（reconcile / live publisher / trace worker / spill replayer），Go 无 recover → 进程退出；若是 gin handler，则 `gin.Recovery` 只把该请求打成 500/中断流。
- **建议修复**：广播改为在持锁期间发送（订阅者 channel 带 200 缓冲且发送非阻塞，不会长时间持锁），或将「关闭」改为不 `close` 而是打标记、由读取方按 `cancel` 返回的 done channel 退出；也可用 `sync.Map` + 每订阅者独立 done channel 避免关闭竞态。

### ST-02【P1，已确认】合成 off-peak slot 覆盖运维配置的单窗口折扣

- **触发**：任何写入价格行且 `off_peak_enabled=true`、未显式提供 `off_peak_slots`、只给 `off_peak_start`/`off_peak_end`/`off_peak_discount` 的路径（`SaveModelPrice`、`ApplyModelRouteBundle` 及 `RouteService.buildRoutePricing` 新建行时均会走到；`internal/controlplane/route_service_test.go:240` 正是这种入参）。
- **代码路径**：`normalizeModelPrice`（`repo_pricing.go:202-204`）把空 slots 强制改写为 `[{"start":"00:00","end":"08:30","discount":0.5}]`；而计费 `CustomOffPeakStrategy`（`internal/billing/strategy.go:71-112`）只在 `len(slots)==0` 时才回落到 `off_peak_start/off_peak_end`（`:113`），因此合成 slot 一注入，运维配置的窗口**永远不再被评估**；同时 `:211-213` 把 `off_peak_discount<=0` 改成 0.5。
- **证据**（探针 F，`repo.SaveModelPrice` → `billing.NewEngine(repo).IsOffPeak`）：

  ```
  stored slots="[{\"start\":\"00:00\",\"end\":\"08:30\",\"discount\":0.5}]" start=22:00 end=06:00 discount=0.25
  23:00 => isOff=false discount=1.00   (配置窗口 22:00-06:00 内，应收 0.25 → 实收原价，多收)
  03:00 => discount=0.50               (配置 0.25 → 实收 0.50，少收一半)
  ```
- **影响**：双向计费错误，且错误方向取决于时段；对夜间折扣渠道（当前 `off_peak` 配置的典型用法）是持续的少收/多收。
- **建议修复**：仅当 `off_peak_start`/`off_peak_end` 也为空时才注入默认 slot；或在 `CustomOffPeakStrategy` 中让显式 legacy 窗口优先于空 slots。`off_peak_discount==0` 应视为「无折扣」而不是替换为 0.5。

### ST-03【P1，已确认】trace 脱敏漏掉 `Bearer <token>`，凭证落库

- **触发**：`GATEWAY_TRACE_CAPTURE=errors`（默认）下任何 ≥400 的请求；`Record` → `TruncateSnippet` → `RecordRequestTrace` 写 `request_traces.request_snippet`。
- **代码路径**：`SensitiveKeyPattern`（`repo_traces.go:154`）的值部分为 `([^",\s]+)`，遇空格即停；`"authorization":"Bearer sk-..."` 只会把 `Bearer` 替换成 `[REDACTED]`，真正的 token 留在后面。
- **证据**（探针 D）：输入 `{"headers":{"authorization":"Bearer sk-airoute-probe-secret-123456"}}`，输出 `{"headers":{"authorization":"=[REDACTED] sk-airoute-probe-secret-123456"}}`。
- **影响**：审计表按设计保留疑似密钥明文（默认 72h，`GATEWAY_TRACE_TTL_HOURS` 可放大）；这也是「日志/追踪 PII 泄漏」类问题的实际入口。
- **建议修复**：`Bearer\s+\S+` 单独规则；或对命中的字段值整段替换为 `[REDACTED]`（不保留首词），并补 `x-goog-api-key`、`cookie`、`set-cookie`。

### ST-04【P1，已确认】`REDIS_URL` 含密码被写入日志

- **触发**：`REDIS_URL` 无法被 `redis.ParseURL` 解析（scheme 写错、`rediss://` 下证书参数写坏、主机名带非法字符等）。
- **代码路径**：`internal/distributed/redis.go:53` 的 WARN 直接带 `"url", redisURL`。
- **证据**（探针 B）：

  ```
  {"level":"WARN","msg":"failed to parse REDIS_URL, falling back to local memory",
   "error":"redis: invalid URL scheme: http","url":"http://:SuperSecretPassw0rd@127.0.0.1:6379/0"}
  ```
- **影响**：Redis 口令进入容器 stdout、日志采集与环形缓冲（控制台可见）。修复：只记录 `scheme://host:port`（`opt.Addr`），绝不记录 userinfo。

### ST-05【P2，已确认】libpq keyword DSN 整串（含 password）入日志

- **触发**：`DATABASE_URL` 使用 keyword/value 形式（`host=... dbname=... password=...`，`OpenDB` 明确支持这种形式，`db.go:263-266`）。
- **代码路径**：`db.go:271` 通过 `strings.Split(dataSourceName, "@")` 的最后一段脱敏；keyword DSN 中没有 `@`，切分后「最后一段」就是整个 DSN。
- **证据**（探针 C）：`"source":"host=127.0.0.1 port=1 dbname=airoute user=airoute password=DsnSecretPassw0rd ..."`。
- **影响**：Postgres 口令进入日志。修复：按 `password=`/`user=` 键正则打码，或仅记录 `scheme+host+dbname`；URL 形式同样建议改为解析后重组。

### ST-06【P2，已确认】`expandConfigEnv` 吞掉配置值中的 `$`

- **触发**：`configs/config.yaml` 中任何值含 `$`（管理员密码、渠道 `api_key`、S3 secret、Redis 密码等）。
- **代码路径**：`expandConfigEnv` 先处理 `${NAME:-default}`，随后对**整份文件文本**调用 `os.ExpandEnv`（`config.go:242`），单一 `$` 也会被当变量引用。
- **证据**（探针 E）：`admin:\n  password: pa$$word` → `pa$$word`；`secret_key: pass$2026` → `pass2026`。
- **影响**：静默改写密钥 → 管理员无法登录 / 渠道鉴权失败，且没有任何日志提示（配置校验也没有字符集检查）。修复：只用正则做 `${VAR}` 替换，或用 `os.Expand` + 显式转义约定（`$$` → 字面 `$`），并在启动时校验关键字段未发生替换。

### ST-07【P2，静态】`trace_capture` 用 `slog.Default()` 绕过统一 logger

- **代码路径**：`trace_capture.go:132`（队列满丢快照）、`:148`（落库失败）、`:152`（清理统计）均为 `slog.Default()`。
- **影响**：这些行以 Go 默认 text handler 写 **stderr**，不是 `telemetry.InitLogger` 的 JSON handler，也不会进 `GlobalRingBuffer`（控制台看不到队列满/写库失败）。前轮 README §一.4 已把「容器内非 JSON 行」作为缺陷修过，这里是同类回归。
- **建议修复**：改用 `telemetry.ScopedLogger("system")`（丢快照/写失败是高价值告警，可叠加 `WarnLogger` 限频）。

### ST-08【P2，静态】Redis XADD 失败时按记录刷 WARN

- **代码路径**：`async_logger.go:122-128`，XADD 失败后每条 usage 记录打一条 WARN（`"failed to push log to Redis Stream..."`）。
- **影响**：Redis 抖动/不可达时，日志量与请求量同阶；前轮 README §一.6 对同类「按请求/按记录重复告警」已引入 `telemetry.WarnLogger`，此处遗漏（同文件其他告警已使用）。
- **建议修复**：`telemetry.WarnLogger(logger, "xadd-fallback", 30*time.Second, ...)`，并可用计数器观察回退次数。

### ST-09【P2，静态+线上 schema】验证码表无清理、索引与查询不匹配

- **代码路径**：`SaveVerificationCode` 每次发送都 `INSERT` 一行（`repo_verification.go:20-26`），全仓库没有任何 `DELETE FROM verification_codes`；`VerifyCode` 查询为 `WHERE email=? AND purpose=? AND used=0 AND expires_at > NOW() ORDER BY id DESC LIMIT 1`（`:38-44`）。
- **证据**：线上 `verification_codes` 仅 `verification_codes_pkey` 与 `idx_verify_email ON (email, code)`；索引第二列 `code` 对上述查询无用，`purpose/used/expires_at` 全部过滤在索引外，`ORDER BY id DESC` 需要额外排序。
- **建议修复**：加 `(email, purpose, used, id)` 复合索引（或 `(email, id DESC)`），并在保留任务里删过期码（与 `usage_retention` 同型 worker）；同时把 `expires_at` 清理窗口纳入对账/清理规范。

### ST-10【P2，静态】`users.email` 无唯一约束，注册是 check-then-insert

- **代码路径**：`migrations_postgres.go:71` 只定义 `email VARCHAR(255) DEFAULT ''`，索引 `idx_users_email` 非唯一（线上已核）；`GetUserByEmail` 用 `QueryRow` 且**无 `ORDER BY`**（`repo_users.go:87-99`）；注册在 `identity_handler.go:65`、OAuth 绑定在 `oauth_handler.go:205,217` 先查后插。
- **影响**：两个并发注册请求携带同一邮箱可同时通过「不存在」检查 → 同邮箱多账号；之后 `GetUserByLogin`（`repo_users.go:58-66`）按邮箱登录、OAuth 绑定取到的是 PostgreSQL 返回的任意一行，账号可能错配。当前线上只有 1 个绑定邮箱，故未观察到实例。
- **建议修复**：`CREATE UNIQUE INDEX ON users (LOWER(email)) WHERE email <> ''`（先做一次重复数据检查/合并），注册与 OAuth 绑定改 `INSERT ... ON CONFLICT DO NOTHING` 并依 `RowsAffected` 判定；`GetUserByEmail` 加 `ORDER BY id` 以消除非确定性。

### ST-11【P2，静态】S3 凭证硬编码 + 局部 storage 配置被整块丢弃

- **代码路径**：`config.go:144-158`（`DefaultConfig`）与 `storage_config.go:33-40` 都硬编码 `AccessKey: "airoute"` / `SecretKey: "airoute_cluster_secret_pass_2026"`；`GetStorageConfig` 仅在 `c.Storage.Driver != ""` 时整体采纳 `c.Storage`（`storage_config.go:42-44`），否则整块替换为默认值。
- **影响**：(a) 源码中固定 secret（仓库泄漏即凭证泄漏，且与 `.env` 实际值相同，危险在于默认值会被静默采用）；(b) YAML 只写 `storage.s3.*` 而未写 `driver` 时，这些字段被静默忽略并回落到默认 endpoint/secret；反过来写了 `driver: s3` 但漏 `access_key` 时得到空凭证而不是默认值——两种静默行为方向相反。
- **建议修复**：删除硬编码 secret，缺失时启动即失败并给出明确错误；配置合并按字段逐个覆盖而不是整块替换。

### ST-12【P2，静态】试用余额配置解析失败静默回落

- **代码路径**：`GetInitialUserBalance`（`config_policies.go:58-71`）：`INITIAL_USER_BALANCE`/`DEFAULT_TRIAL_BALANCE` 解析失败或为负数时直接落到 `5.0`。
- **影响**：`INITIAL_USER_BALANCE=0.0o`（笔误）或负值这类明显配置错误不会报错，而是给每个新注册用户 5 元额度——真实资金损失且无日志。同型问题：`GATEWAY_DB_MAX_OPEN_CONNS` 等非法值也静默回落（`db.go:407-419`），但那里回落到保守默认尚可接受，余额是「给钱」的方向，应严格。
- **建议修复**：解析失败/为负时启动报错（或至少 WARN 一次并明示采用的默认值）。

### ST-13【P2，静态】控制台「缓存省钱」用硬编码单价

- **代码路径**：`repository.go:117-119` 与 `:193-195`：`SavedCost = TotalCachedTokens * 1.8 / 1e6`。
- **影响**：该数字与 `model_prices.cache_read_price` 无关，只要缓存价不是 1.8 元/1M，控制台展示的「节省成本」就是编造的（对账单/汇报类界面属误导）。线上价格表 `cache_read_price` 由运维逐模型配置，几乎不可能恰好是 1.8。
- **建议修复**：用 `cached_tokens * (prompt_price - cache_read_price)` 按模型聚合（`usage_logs` 已有 `pricing_snapshot` 可取当时价格），或直接把该指标改成「cached token 数」并去掉金额语义。

### ST-14【P2，静态+线上数据】时间序列在 Go 侧全量聚合

- **代码路径**：`GetStatsTimeSeries` 的查询 `SELECT created_at, model, total_tokens, status_code, error_message, ttft_ms, cost FROM usage_logs WHERE created_at >= ?`（`repo_timeseries.go:175`）无 `LIMIT`、无 SQL `GROUP BY`，时间窗内每行都 `Scan` 后在 Go 里分桶、排序算 p50/p90/p99（`:196-290`）。
- **证据**：线上 `usage_logs` 7 天内 48,057 行、24 小时内 18,606 行（87MB 表）；缓存 TTL 3s（`:117`），控制台轮询即触发一次整窗扫描。
- **影响**：当前约 2–5ms，但成本随流量线性增长；7d 视图是"把一周审计流水读进内存"。属容量隐患而非当前故障。
- **建议修复**：分桶/百分位下推 SQL（`date_trunc`/`width_bucket` + `percentile_cont`），或至少对 `ttft_ms` 采样上限（如 `TABLESAMPLE`/`LIMIT`）并显式标注采样率。

### ST-15【P2，静态+线上索引】`request_traces` 缺 `api_key` 索引

- **代码路径**：`ListRequestTraces` 支持 `api_key = ?` 与 `api_key IN (...) `（`repo_traces.go:89-99`），这是控制台按租户/密钥检索的主要形态。
- **证据**：线上索引只有 `idx_trace_trace_id` / `idx_trace_chat_id` / `idx_trace_session` / `idx_trace_created` / pkey，无 `api_key`。
- **影响**：按密钥检索退化为顺序扫描；该表存 4096 字节级 snippet（`GATEWAY_TRACE_MAX_SNIPPET`），是体积增长最快的审计表（线上 461 行）。
- **建议修复**：`CREATE INDEX idx_trace_api_key ON request_traces(api_key)`（可加 `(api_key, id DESC)` 支持分页）。

### ST-16【P2，静态+线上索引】`video_tasks` 无二级索引、无保留期、明文存上游地址

- **代码路径**：`repo_video_tasks.go:12-16` 建表只有 `id TEXT PRIMARY KEY`；线上确认只有 `video_tasks_pkey`。
- **影响**：(a) 没有按 `owner` 的查询索引（`GetVideoTask` 走主键+owner 过滤，尚可，但运维按账号排查会全扫）；(b) 无 TTL/清理，任务记录永久保留（`channel_name`、`base_url` 暴露内网拓扑）；(c) `created_at TIMESTAMP`（PG 侧不带时区，与其余表 `TIMESTAMPTZ` 不一致，跨表对时/时区变更时语义不同）。
- **建议修复**：加 `(owner, created_at)` 索引并把 `created_at` 改为 `TIMESTAMPTZ`；纳入某个保留期 worker（如 30 天）或与 `request_traces` 的清理合流。

---

## 附：探针清单（worktree `/tmp/airoute-audit-storage`，未进入仓库）

| 探针文件 | 覆盖发现 | 失败即证据 |
|---|---|---|
| `internal/telemetry/audit_ringbuffer_probe_test.go` | ST-01 | `send on closed channel`（3/3） |
| `internal/distributed/audit_secret_log_probe_test.go` | ST-04 | 日志含 Redis 口令 |
| `internal/storage/audit_dsn_redaction_probe_test.go` | ST-03 / ST-05 | Bearer token 未脱敏；DSN 口令入日志 |
| `internal/config/audit_expandenv_probe_test.go` | ST-06 | `$` 被吞 |
| `internal/storage/audit_offpeak_probe_test.go` | ST-02 | 存储 slots 被改写；23:00 无折扣、03:00 折扣 0.50 |

收尾时已 `git worktree remove /tmp/airoute-audit-storage`（仓库工作树无残留文件；本报告是唯一新增文件）。
