# 存储层一致性修复（2026-10-28）

## 概览

- **基线**：`c8a2765`（另一会话的 5 个结算/分区 commit 已合入）+ 工作区未提交改动（含并行会话的 `internal/api`、`internal/billing` 等）。
- **范围**：`internal/storage/**`（唯一生产代码写入域）+ 状态文件（本文件）。
- **结果**：清单 18 项中 **16 项已修**（P1 2/2），**1 项按裁决降级**（E-14 仅删 2 个中的 1 个，其余 6 个加 `Deprecated:` 并 handoff），**1 项改为删除**（E-12）。新增迁移 **v42**（`migrations_storage_repairs.go`，双方言幂等），新增测试 **6 个文件**。
- **验证**：`gofmt -l`（改动文件）为空；`go build ./...` 干净；`go vet ./...` 干净；`go test ./internal/storage/ -count=1` **ok**；`-race` **ok**（见下）。
- **已知非本域**：`internal/api`、`internal/controlplane`、`tests/*` 仍有 6 个对被 `Deprecated` 存储方法的调用，等 D/F 域排期后统一迁移（handoff 列表在末尾）。

## 修复明细

| ID | 级别 | 结论 | 关键改动 | 测试 |
|---|---|---|---|---|
| E-01 | P1 | 已修 | `TraceCapture.Record(rec) bool` 成为**唯一**采样判定点；`Enabled` 只做策略判定（不再掷随机数），返回是否真正入队；`mode=="off"` 返回 false | `trace_capture_policy_test.go`: `TestTraceRecordIsTheSingleSamplingPoint`（1000 次 `sample=10` 落在 50..200）、`TestTraceRecordReportsQueueOutcome` |
| E-02 | P1 | 已修 | 清理从 `worker()` 拆出为独立 `purgeWorker()`；`start()` 在 `repo!=nil` 时**始终**启动它（`mode=="off"` 也不影响），并在启动时先清一次；`Stop()` 幂等关停两个 goroutine | `TestTraceRetentionRunsWithCaptureOff`（off 模式下过期行被删、新行保留、不再入队） |
| E-03 | P2 | 已修（方案 a） | `billing_reconcile_job.go` 新增 `tenantLedgerDrift()`：结算账本与审计流按 `tenant_id` 在 7 天窗口（<365 天保留期、避开 5 分钟结算宽限）内比对 `cost_units`，漂移走 WARN；同时按租户输出 `settled_units/total_tokens` 汇总 INFO，为两列建立真实读点（不新增 API） | `tenant_ledger_drift_test.go`: 漂移检测、token 被读取、在途行不误报 |
| E-04 | P2 | 已修 | 新增通用 `dialectTime/dialectTimePrecise`（`sqlutil.go`：PG 绑 `time.Time`、SQLite 绑固定宽度 UTC 文本），替换**全部**时间戳写入点（usage_logs 单条/批量、billing_settlements、verification_codes、response_contexts）与**同列比较点**（usage/ledger/traces 清理 cutoff、时序窗口、models_used、reconcile cutoff、审计滞后基线窗口、ledger retention cutoff），避免"写入正确但比较仍按会话时区解释"的新裂缝 | `dialect_time_test.go`: 绑定形态、微秒布局落库、E-11 全局单例；`dialect_time_pg_test.go`（无 DSN 自动 skip） |
| E-05 | P2 | 已修 | 与 E-04 同一助手：`verification_codes.expires_at` 写入改 `r.dialectTime`，与 `VerifyCode` 的 `NOW()`/`datetime('now')`、清理 cutoff 三者口径一致 | `TestVerificationCodeExpiryUsesDialectBinding`（未过期可验、已过期不可验、落库文本形态）；PG 侧见 skip 用例 |
| E-06 | P2 | 已修 | `createVideoTasks` 按方言分支：PG 用 `TIMESTAMPTZ`、SQLite 用 `DATETIME`（抽出 `videoTasksDDL(dialect)` 便于单测）；迁移 v42 对已存在的 PG 库把 `timestamp` 列转为 `TIMESTAMPTZ`（`USING ... AT TIME ZONE current_setting('TimeZone')`，catalogue 守卫、可重放）；清理 cutoff 走 `dialectTime` | `TestVideoTasksDDLIsDialectSpecific`、`TestVideoTaskRetentionStillWorksAfterIndexChange` |
| E-07 | P2 | 已修 | v42：删除复合 `idx_video_tasks_owner(owner, created_at)`，改建 `idx_video_tasks_created(created_at)` + `idx_video_tasks_owner(owner)`（清理按 `created_at` 过滤排序，PK 读路径无需复合索引） | `TestStorageRepairMigrationIndexes`（含索引定义断言） |
| E-10 | P2 | 已修 | v42（SQLite）：新增 `usage_logs_fts_au AFTER UPDATE OF trace_id, chat_id, session_id, model, channel, error_message`（external-content 的 delete+insert 标准写法），末尾 `INSERT INTO usage_logs_fts(usage_logs_fts) VALUES('rebuild')` 修复 v24/v32 已改写的历史行 | `TestStorageRepairMigrationFTSUpdateTrigger`（旧文本不再命中、新文本可搜到） |
| E-11 | P2 | 已修（按裁决 b） | 保留 `SetGlobalRepository`，`NewRepository` 改为调用它——全局单例只有一条写路径，deadcode 不再报"从 main 不可达" | `TestNewRepositoryRegistersGlobalRepository` |
| E-12 | P2 | 已删（说明理由） | 删除 `money_reconcile.go`（`ReconcileMoney`/`MoneyDrift`）及其单测。**理由**：它按 key 比较 `SUM(usage_logs.cost_units)` 与 `api_keys.used_cost_units`，而 usage_logs 会按保留期裁剪且只有全局裁剪基线，第一次裁剪之后每个长期 key 都会被判成漂移（必然误报）；真正在生产运行的 `ledgerDrift` 以结算账本为基准且已由 `billing_ledger_retention` 做归档抵扣，是剪枝安全的等价口径。已在 `ledgerDrift` 注释里写明这一取代关系 | 删除的测试原为其唯一调用点；`ledgerDrift` 既有测试覆盖 |
| E-13 | P2 | 已修（改注释） | `GetAPIKeyBySecret` 注释改为事实描述：中间件当前自行 `HashAPIKey` 后调 `GetAPIKeyByKeyContext`（需要看到 deleted 行才能显式拒绝），该函数只是"持有明文凭据调用方"的便捷入口 | 行为未变，无新增测试 |
| E-14 | P2 | 已完成（第二轮收口） | **7 个方法全部删除**：`SetModelFallback`、`CreateRedemptionCode`、`ListUsageLogs`、`RecordUsageLog`、`BatchRecordUsageLogs`、`GetChannel`、`GetModelPrice`；所有引用点（27 个测试文件）改到生产版本；新增 Repository 级批量 `SetModelFallbacks(map[string]string) error`（事务内 upsert）作为 `SetModelFallback` 的替代 | `go vet ./...` 干净（编译全部测试）即"删干净"判据；`./internal/storage ./internal/api ./tests` 全红转绿，`./internal/controlplane` 仅剩 2 个与本次无关的定价失败（见下） |
| E-15 | P2 | 已修 | `GetStatsOverview` / `GetStatsOverviewForKeys` 补算 `PeakRPM/PeakTPM`；把时序的峰值 SQL 抽成共享 `peakRates(windowStart, keyCond, keyArgs)`（一次查询同时得到 RPM/TPM，避免 N+1），两个端点口径一致；窗口常量 `overviewPeakWindow=1h`（5s 缓存之后成本可接受） | 由 `repo_timeseries` 既有测试与 overview 既有测试覆盖；新增逻辑走同一 helper |
| E-16 | P2 | 已修 | v42 删除 `idx_usage_trace`/`idx_usage_chat`/`idx_usage_session`（注意：**保留**了仍在用的 `idx_usage_channel` 等）；`repo_logs.go` 在过滤器处加注释说明为何不建索引（`= ? OR LIKE '%?%'` 的 B-tree 不可用，只会增加热插入写放大） | `TestStorageRepairMigrationIndexes` |
| A-11 | — | 已修 | 新增 `trace_id.go`：`ensureTraceID` + `newFallbackTraceID`，格式 `tr-<微秒>-<12 hex>`；`async_logger.Record` 与 `repo_logs_writer.appendArgs` 两处删除 `TraceID = SessionID` 兜底；已有 trace_id 不覆盖（重投递幂等） | `TestEnsureTraceIDNeverBorrowsSessionID`、`TestUsageWriterDoesNotPersistSessionIDAsTraceID` |
| A-15 | — | 已修 | `s3_storage.go` 删除未文档化的 `S3_ENDPOINT` 兜底，只认 `STORAGE_S3_ENDPOINT`；核查 `internal/config/storage_config.go` 本就没有该兜底（无需改 config 域） | `TestNewS3StorageIgnoresGenericS3Endpoint`、`TestNewS3StorageAcceptsDocumentedEndpoint` |

## 迁移 v42

- **文件**：`internal/storage/migrations_storage_repairs.go`，函数 `repairStorageIndexesAndTraceSearch`。
- **版本**：`42`，已同时注册进 SQLite 与 PostgreSQL 两份迁移表（v41 属并行会话 `migrations_billing_scale`、v43 属 `migrations_partition`，未触碰）。
- **内容**：删 3 个无用 `usage_logs` 单列索引（双方言）→ 重建 video_tasks 两个单列索引（双方言）→ PG 把 `video_tasks.created_at` 由 `timestamp` 转 `TIMESTAMPTZ`（catalogue 守卫）→ SQLite 建 FTS `AFTER UPDATE` 触发器并 `rebuild`。
- **幂等性**：全部 `DROP INDEX IF EXISTS` / `CREATE INDEX IF NOT EXISTS` / `CREATE TRIGGER IF NOT EXISTS` + information_schema 守卫；`TestStorageRepairMigrationIsIdempotent` 直接连跑两次验证。
- **双方言**：SQLite 分支由全部 storage 测试**实际执行**；PG 分支通过 `AIROUTE_TEST_DATABASE_URL` 的 skip 风格用例与 `OpenDB(dsn)`（会真实跑完 v1..v42）覆盖，未配 DSN 时自动 skip（本机未配，故 PG 侧本轮为"代码审查 + 可 skip 用例"级别）。

## 验证结果

| 检查 | 命令 | 结果 |
|---|---|---|
| 格式 | `gofmt -l internal/storage/*.go` | 空（改动/新增文件全部合规） |
| 编译 | `go build ./...` | **通过**（无输出） |
| 静态检查 | `go vet ./...` | **通过**（无输出；含 `tests/` 与所有 `_test.go`） |
| 单元+集成（SQLite） | `go test ./internal/storage/ -count=1` | **ok**（约 7.5s，全绿） |
| 竞态 | `go test ./internal/storage/ -count=1 -race` | **ok**（`72.4s`，无 data race 报告） |

新增测试文件：`trace_capture_policy_test.go`、`dialect_time_test.go`、`dialect_time_pg_test.go`、`migrations_storage_repairs_test.go`、`tenant_ledger_drift_test.go`、`s3_storage_endpoint_test.go`；另更新既有测试 `repo_channels_patch_test.go`、`repo_model_routes_test.go`（改调 `ListChannels`）、`money_test.go`（删除 `ReconcileMoney` 用例）。

## Handoff

1. **E-14 已完成**：7 个 `Deprecated` 方法已删除，全部引用点迁移到生产版本（详见文末「E-14 收口」）。当前仓储层不再有单条/包装方法死代码。
2. **D 域（api）**：`TraceCapture.Record` 现在**返回 bool**（是否真正入队），可直接用于"是否归档"的判断；E-01 的覆盖缺口仍待 api 补：`RecordSuccessTrace` 目前只在 chat 单次/流式两个入口调用，embedding/rerank/responses/anthropic/gemini/图片/审核的成功请求在 `capture=all` 下不会被归档。
3. **E-08 / E-09**（controlplane 域，未在本轮清单）：trace 作用域仍用 `ListAPIKeysByUser`（已撤销密钥的 trace 对本人不可见，应与 logs/stats 统一用 `ListHistoricalAPIKeysByUser`）；stats 的 `key=` 参数仍不做 `ak_…` → digest 解析。
4. **并行会话（partition owner）注意**：`migrations_postgres.go` 注册了 v43，而 `migrations_sqlite.go` 列表止于 v42——历史两份注册表一直保持同版本号；建议在 SQLite 侧补一个 no-op v43（`Up: func(*DB) error { return nil }`）以保持注册表一致。本轮按裁决未触碰该文件。
5. **config 域**：`internal/config/storage_config.go` 未发现 `S3_ENDPOINT` 兜底，无需改动；`S3_ENDPOINT` 仅剩文档/环境示例层面无引用。

## 备注与偏差

- A-11 的 `internal/storage/trace_id.go` 在审计期间被并行会话的 `d821c56` 一起提交（内容即本轮的 `ensureTraceID`，已核对一致），因此它不在当前 `git status` 的未跟踪列表里；不要误判为遗漏。
- `migrations_postgres.go` 里 v43（partition）位于列表末尾的注释块中，SQLite 侧没有 v43（见 handoff 第 4 条）。

- E-14 未按原计划"删干净"：`GetChannel`/`GetModelPrice` 有 **6 个跨包测试引用**（`internal/controlplane`、`tests/`），超出本轮写权限；强行删除会让 `go vet ./...` 失败。按分段执行策略改为 `Deprecated:` + handoff。
- E-12 选择"删除"而非"并入 runOnce"：并入会在保留期裁剪后持续误报（见状态表理由），与"真跑并且有信号"的目标相反。
- E-03 选择方案 a 且把对比窗口做成"结算宽限之后、保留期之内"的 7 天窗口——直接全量按租户比对会被保留期裁剪与在途结算污染。
- 未改 `migrations_partition.go` / `migrations_billing_scale.go` / `db.go`；未做任何 git 写操作。

## 追加：D-14 存储层阻断点（F 域 handoff）

- **问题**：`repo_model_routes.go` 的 `updateChannelTx`（以及重复实现同一逻辑的 `Repository.UpdateChannel`）对更新路径也做零值归一化：`Priority==0→1`、`Weight==0→10`、`TimeoutSeconds==0→60`。控制面已改为"显式 0 生效"，落库时又被改回默认值，`PUT /admin/channels/:id` 无法把 priority/weight 设为 0。
- **改动**：
  - `updateChannelTx`：删除三处 coercion，只保留 `Status==""→active`；注释写清"创建 vs 更新对零值语义不同且是有意的"（创建=省略字段用默认值；更新=显式值原样落库，priority 0/weight 0/timeout 0 都是有意义的值）。
  - `Repository.UpdateChannel`：不再复制一份 SQL 和归一化逻辑，改为开事务委托 `updateChannelTx`——两个更新入口共用同一实现，不会再漂移。
  - **创建路径未动**：`CreateChannel`（`repo_channels.go:103-111`）仍把 0 → 1/10/60。
- **新增测试**：`repo_channels_zero_routing_test.go`（`TestUpdateChannelPersistsExplicitZeroRoutingFields`、`TestPatchChannelPersistsExplicitZeroRoutingFields`、`TestCreateChannelStillDefaultsZeroRoutingFields`）。
- **验证**：`gofmt -l internal/storage` 空；`go build ./...`、`go vet ./...` 干净；`go test ./internal/storage/ ./internal/controlplane/ -count=1` 均 **ok**（5.0s / 6.6s，F 域 `TestUpdateChannelAppliesExplicitZeroRoutingFields` 绿）。

## E-14 收口（第二轮）

- **删除的 7 个方法**（生产代码已无调用者，删除后 `go vet ./...` 通过 = 测试全部编译通过）：
  - `SetModelFallback(model, target, enabled)` → 新增 **`SetModelFallbacks(map[string]string) error`**（单事务批量 upsert，enabled=1）。仓储层此前确实没有批量版本（只有 `*router.Dispatcher` / `*controlplane.Synchronizer` 上的同名方法），因此补了一个语义清晰的批量 API，而不是让测试去走 `ApplyModelRouteBundle`（后者会顺带写 model_routes 行，改变测试语义）。
  - `CreateRedemptionCode` → `CreateRedemptionCodes([]*RedemptionCodeRecord{...})`
  - `ListUsageLogs(limit, offset)` → `ListUsageLogsWithFilterAndCount(LogFilter{Limit, Offset})`（3 返回值，调用点补 `_`）
  - `RecordUsageLog` → `RecordUsageLogContext(context.Background(), ...)`
  - `BatchRecordUsageLogs` → `BatchRecordUsageLogsContext(context.Background(), ...)`
  - `GetChannel` → `ListChannels()` + 查找（storage 用 `findChannel`，`tests/` 用新增 `channelByID`）
  - `GetModelPrice` → `GetModelPriceWithGroup(m, "default")`（与原单条实现完全同义：default 组 + 组回退）
- **改动的测试文件（27 个）**：
  - `internal/storage/`（9）：`repo_model_routes_test.go`、`billing_history_test.go`、`account_wallet_regression_test.go`、`repo_log_filters_test.go`、`api_key_digest_test.go`、`repo_timeseries_test.go`、`repo_routing_activity_test.go`、`stats_regression_test.go`、`postgres_integration_test.go`
  - `internal/api/`（5）：`billing_edge_fix_test.go`、`channel_audit_test.go`、`mcp_authz_regression_test.go`、`responses_context_test.go`、`usage_capture_test.go`
  - `internal/controlplane/`（4）：`functional_regression_test.go`、`admin_overview_keyid_test.go`、`admin_reconciliation_test.go`、`admin_logs_page_limit_test.go`
  - `tests/`（9）：`admin_model_delete_test.go`、`p0_p1_p2_fixes_test.go`、`trace_and_ha_test.go`、`pricing_and_model_routes_test.go`、`api_test.go`、`audit_remediation_test.go`、`chat_id_and_channel_test.go`、`mcp_and_session_logs_test.go`、`security_and_logic_audit_test.go`
  - 说明：`internal/controlplane/admin_channels_view_test.go` 的 `store.GetChannel` 未改——`store` 是 `fakeChannelStore`（测试替身自己的方法），与 `Repository.GetChannel` 无关。
- **验证**：`gofmt -l internal/ tests/` 空；`go build ./...`、`go vet ./...` 干净；`go test ./internal/storage/ ./internal/api/ ./internal/controlplane/ ./tests/ -count=1` → storage/api/tests **ok**，controlplane 有 **2 个与本次无关的失败**：
  - `TestSavePricingRateHTTPContract/rejects_overlapping_off-peak_slots_with_400`（期望 400，实得 200）
  - `TestPricingServiceSaveRejectsOverlappingOffPeakSlots`（期望 `*ValidationError`，实得 nil）
  - **判定为非本次改动**：失败路径在 `internal/controlplane/pricing_service.go` → `internal/billing/ValidateOffPeakSlots`，两文件均为**另一会话未提交的改动**（mtime：`strategy_validation.go` 13:03:58、`pricing_service.go` 13:12:53、`admin_pricing.go`/`admin_pricing_test.go` 更晚；而我在 13:01 跑该包时是 **ok** 的）；根因是其新的"仅 `OffPeakMode=="custom"` 才校验 slots"门控与测试期望（空 mode 也期望报错）不一致。这两个测试走 `fakePricingStore`，完全不经过被删的仓储方法。
