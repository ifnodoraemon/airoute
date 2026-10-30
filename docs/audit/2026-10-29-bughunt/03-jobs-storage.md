# 异步任务 / 存储维护路径 Bug 排查 G（2026-10-29 bughunt 第 3 轮）

> **范围**：`internal/storage` 的**异步与维护路径**——`async_logger`（Redis Stream 消费/重试/回放/批量 flush）、`usage_spill`、`trace_capture`（采样与 TTL 清理）、`billing_reconcile_job`、`billing_ledger_retention`、`retention_maintenance` + 各 purge、`artifact/s3 storage`、`partition` 维护与迁移一致性。
> **口径**：只找**真实运行期缺陷**（丢单、卡队列、错账、静默丢失、无界增长）；上一轮 `docs/audit/2026-10-28-bughunt/03-data.md` §6 已修复的条目**不重报**（尤其 `settlement.go` 批内去重）。风格/死代码不在本文范围。
> **基线**：`2f43d28`（全绿）。**生产代码零改动、未 commit**；新增 3 个 `*_bugprobe_test.go`（断言**当前错误行为**，因此 `go test ./...` 仍全绿，修复后需翻转）。
> **方法**：只读审计 + 可复现探针；线上只读证据（`docker exec airoute-redis redis-cli ...`、PG `SELECT`），未写任何 Redis/PG。

---

## 0. 一句话结论

异步链路的**数据通路正确性**（写库后才 XACK、PEL 重试、批内幂等、trim 不越界）上一轮已相当扎实——本轮复核未发现新的错账/丢单机制。真正的问题集中在**故障与生命周期边界**：**Redis 消费组一旦丢失就永不重建（P1，写入仍在成功，消费永久停摆且零告警）**、**毒丸消息被静默 ACK 丢弃且无任何日志/指标（P2）**、**本地 spill 只在进程启动时回放一次，运行期落盘记录永不回放（P2）**、**TraceCapture 停止后仍接受记录并报告"已归档"（P2）**。另有 3 条 P3（消费者条目无界累积、账本保留并发冲突吞掉整趟、保留清理失败后静默 1 小时）。

**未发现新的 P0 / 金额类缺陷**：线上 `per-key` 账本漂移实测 0 行（§4）。

---

## 1. Findings 总表

| ID | 级别 | 置信度 | 域 | 位置 | 一句话 |
|---|---|---|---|---|---|
| **JOBS-01** | **P1** | 高（机制实测） | async_logger / Redis Stream | `async_logger.go:291,423-427`、`distributed/redis_streams.go:25-34,37-58` | 消费组只在 worker 启动时创建**一次**且错误被丢弃；组一旦不存在（启动瞬时失败/Redis 丢失该组/运维删除），`XREADGROUP` 永久 `NOGROUP`，错误分支裸 `sleep(200ms)` 无日志无指标无重建 ⇒ 审计入库**静默永久停摆**，而 `Record()` 仍 `XADD` 成功返回、stream 不再 trim、无任何告警，只能靠重启恢复 |
| **JOBS-02** | P2 | 高（实测） | async_logger / 毒丸 | `async_logger.go:307-325` | 无法 `json.Unmarshal` 或缺少 `data` 字段的 stream 条目被**无条件 ACK 后丢弃**，不记日志、不加 `UsageLogRecordsDropped`、不告警 ⇒ 一条可计费审计行无声消失，且与"charged 审计行不可删"的既有承诺冲突 |
| **JOBS-03** | P2 | 高（实测） | usage_spill / 回放 | `async_logger.go:74-75,176-191`、`usage_spill.go:150` | spill 回放是**一次性 goroutine**，只在进程启动跑一次。运行期（启动之后）因 DB 故障/缓冲满落盘的记录**再也无人回放**，直到下次重启；spill 涨满 `GATEWAY_USAGE_SPILL_MAX_MB`（默认 256MB）后新记录被直接丢弃并报 BILLING LOSS |
| **JOBS-04** | P2 | 高（实测） | trace_capture / 生命周期 | `trace_capture.go:144-172,174-189,237-247` | `Stop()` 后 `Record()` 仍返回 **true**（"已入队/已归档"的对外契约），但 worker 已退出、队列无人 drain ⇒ 快照永久丢失且调用方无法区分；`recorded` 计数虚增 |
| **JOBS-05** | P3 | 高（代码+线上） | async_logger / 消费组卫生 | `async_logger.go:288-296`（全仓无 `XGROUP DELCONSUMER`） | consumer 名是 `hostname-pid`，每次重启/滚动发布新增一个条目且**永不回收**；线上组已有 **136 个** consumer（最老 idle ≈3 天）。不丢单，但组元数据无界增长、孤儿 PEL 排查失真 |
| **JOBS-06** | P3 | 中 | billing_ledger_retention | `billing_ledger_retention.go:258-268,171-181` | 双副本并发归档时，后提交者 `deleted != len(ids)` → 回滚并 `return 0,0,nil`（正确）但 `pruneOnce` 据此提前 `break` 整趟，且无日志：那趟白跑、后续批次留到下一趟 |
| **JOBS-07** | P3 | 中 | retention_maintenance | `retention_maintenance.go:65-72,77-91` | `shouldRunRetention` 先 CAS 占位再执行 purge；purge 失败不回退占位 ⇒ 一次瞬时 DB 错误让该表清理静默停 1 小时（`retentionPruneInterval`） |
| **JOBS-08** | P3（潜在） | 中 | migrations_runner | `migrations_runner.go:29-42,63-93` | `errMigrationDeferred` 时 `continue` 到**后续版本**：当前唯一可能 deferred 的 v43 在列表末位故无实害；一旦将来在中间插入可 deferred 迁移，后续迁移会在不完整 schema 上执行。另：advisory lock 仅 PG，SQLite 多进程部署无跨进程迁移锁 |

级判据：P1 = 会造成长期数据缺失且无告警/无法自愈；P2 = 特定触发下静默丢数据或状态失真；P3 = 运维卫生/可观测性/白跑，正确性无损。

---

## 2. 详情

### JOBS-01（P1）Redis 消费组丢失后永不重建 ⇒ 审计入库静默永久停摆

**位置**
- 组创建只调一次且错误丢弃：`internal/storage/async_logger.go:291` — `_ = client.EnsureConsumerGroup(ctx, RedisLogStreamKey, RedisLogGroup)`
- 读失败分支无可观测性、无重建：`async_logger.go:422-427`
  ```go
  msgs, err := client.ReadGroupLogs(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, int64(al.batchSize), al.flushTimer)
  if err != nil {
      time.Sleep(200 * time.Millisecond)
      continue
  }
  ```
- 组创建实现：`internal/distributed/redis_streams.go:25-34`（`XGroupCreateMkStream(..., "0")`，仅 `BUSYGROUP` 视为成功）
- 投递路径仍成功：`async_logger.go:117-130`（`XAddLog` 成功即 `return`，**不落本地 spill、不告警**）

**触发条件**（满足任一，且该进程不重启就一直坏）
1. 启动瞬间 `XGROUP CREATE` 失败（Redis 尚未 ready/瞬时 IO 错误/超时）——`_ =` 把错误丢掉，之后不再重试；
2. Redis 从**不含该组**的状态恢复（无持久化/dev 环境重建、failover 到落后副本、`redis-cli FLUSHDB`、运维误删 stream 或组）；
3. Redis 数据被 `maxmemory` 驱逐策略清掉 stream 键（键在组在，键没了组也没了）。

**根因**：把"消费组存在性"当成一次性初始化假设。`ReadGroupLogs` 的 `NOGROUP` 错误走的是**通用**错误分支：裸 sleep + continue，既没有 `EnsureConsumerGroup` 重试，也没有一次性 ERROR 日志或失败计数（`UsageLogFlushFailures` 也不加）。于是"读不到"和"没有消息"在可观测性上完全相同。

**影响**
- **审计链路停摆**：此后每笔请求仍走 `XAddLog` 成功 → `Record()` 直接返回，**本地 buffer 与 spill 都不会接盘**；stream 持续增长（trim 只在成功读取之后才执行，见 `:434-448`），Redis 内存无界上涨。
- **对账指标失真**：`usage_logs` 停止新增，`auditLag` 会把它报成"结算多于审计"的漂移，但没有人知道真正的根因是消费者死了。
- **可能漏收**：若某笔请求的结算被 batcher 以 `settlement_wait_timeouts` defer（`settlement_batcher.go` 语义：由审计管道后续结算），它的入账依赖这条管道 ⇒ 管道停摆期间**这笔钱不入账**，直到重启回放。
- 无 ERROR 日志、无指标、无自愈：只能靠人发现"日志不增长了"并重启。

**证据（探针，真 Redis 实测通过）** `internal/storage/async_stream_bugprobe_test.go`
```
docker run -d --rm -p 16379:6379 redis:7-alpine
AIROUTE_TEST_REDIS_URL=redis://127.0.0.1:16379/0 \
  go test ./internal/storage/ -run TestBugProbeRedisConsumerNeverRecreatesMissingGroup -v
--- PASS ---
  missing group: persisted rows=0, stream length=1, groups=0 (record stuck in the stream)
```
即：组被删除后写入一条记录，给了 worker 2.5s（≈12 个 200ms 重试周期）仍然 **0 行入库、消息滞留 stream、组没有被重建**。探针不设 env 时 `t.Skip`，故 `go test ./...` 保持全绿。

**建议修复**：`ReadGroupLogs` 出错时区分 `NOGROUP`（一次性 WARN + 重建组 + 计数 `UsageLogGroupRecreateFailures`），其余错误也做**限频**日志；重建用 `XGroupCreateMkStream(..., "0")` 语义安全（从头消费，billing_id 幂等）。另外建议 `Record()` 在 `XAddLog` 前探测一次组健康（或由 `XAdd` 之后的周期性 `XINFO` 校正），至少让降级路径能落到本地 spill。

**归属**：`internal/storage/async_logger.go` + `internal/distributed/redis_streams.go`。

---

### JOBS-02（P2）毒丸消息被静默 ACK 丢弃，零日志零指标

**位置**：`async_logger.go:303-325`
```go
for _, msg := range msgs {
    ackIDs = append(ackIDs, msg.ID)          // ← 无论能否解析都进 ACK 列表
    raw, ok := msg.Values["data"].(string)
    if !ok || raw == "" { continue }         // ← 静默跳过
    var rec UsageLogRecord
    if err := json.Unmarshal([]byte(raw), &rec); err == nil { ... }  // ← 失败静默跳过
}
...
if len(ackIDs) > 0 { client.AckLogs(ctx, ..., ackIDs...) }         // ← 连毒丸一起 ACK
```

**触发条件**：stream 中出现任何无法解析的 `data`——截断的 XADD、跨版本不兼容的载荷（例如未来字段/类型变更后的滚动升级窗口）、第三方/脚本误写同一 stream、`data` 缺失或非 string。

**根因**：把"解析失败"归类为"已消费"。为避免卡队列而 ACK 是对的方向，但 ACK 与"记录损失"的告警/计数脱钩了。

**影响**
- 一条可计费的审计行**无声消失**：金额侧（请求路径已结算）不受影响，但审计/对账证据没了；这与仓库自己的承诺冲突——charged 审计行不可删（`repo_logs.go:425-452`、`ErrChargedUsageLogImmutable`）。
- 若载荷太老没带 `billing_id`，`:316-322` 的兜底（用 stream id 派生稳定键）也永远不执行。
- `BillingIDFromStreamEntry` 的幂等兜底、`UsageLogRecordsDropped` 指标、BILLING LOSS 级别的 ERROR 日志**全都不会触发**——事故后无任何线索。

**证据** `TestBugProbeRedisPoisonEntryIsAckedSilently`
```
poison ids=...-0,...-1 healthy=...-2: pending=0 dropped_metric_delta=0 flushed_metric_delta=1
--- PASS ---
```
同批次里健康记录入库 1 条，两条毒丸 **pending=0（已 ACK）且 dropped 指标 delta=0（完全没记录）**。

**建议修复**：解析失败时 `UsageLogRecordsDropped.Add(1)` + 限频 ERROR（带 msg.ID、长度、前 200 字节）+ 可选把原始载荷落 quarantine 文件，然后再 ACK；这样既不会卡队列，也不会无声丢证据。

**归属**：`internal/storage/async_logger.go`。

---

### JOBS-03（P2）本地 spill 只在启动时回放一次，运行期落盘记录永不回放

**位置**
- 一次性启动 goroutine：`async_logger.go:74-75`（`al.wg.Add(1); go al.spillReplayer()`）
- 只跑一次的实现：`async_logger.go:176-191` → `usage_spill.replay(al.repo)`（`usage_spill.go:150-216`）——无 ticker、无重试、无后续调用（全仓 grep `\.replay(` 仅此一处）
- 落盘触发点：`async_logger.go:229-237`（连续 5 次 flush 失败）、`:140-149`（缓冲满 500ms）、`:258-270`（停机 drain 失败）
- 注释承诺与实际不符：`usage_spill.go:40-42` / `async_logger.go:40-42` 写的是"replayed on the next start"（一致），但 `async_logger.go:230-233` 的注释是 **"spool the batch to disk so it is replayed later"**——并不存在"later"的回放。

**触发条件**：进程启动**之后**才发生 DB 故障（滚动重启后 DB 抖动、连接池耗尽、只读切换、SQLite 锁争用超时），flush 连续失败 ≥5 次 ⇒ 批次落盘；或者队列满 500ms ⇒ 单条落盘。DB 恢复后：**没有任何组件会去读这些文件**。

**根因**：把"spill 回放"设计成启动期独占步骤，运行期没有周期性 replay/巡检。

**影响**
- 故障恢复后审计行**长期缺失**（直到下次重启），管理员在日志页看不到这些请求，`auditLag` 持续报警但根因不可见；
- **可能漏收**：对 batcher defer 的 charge，结算只在回放时发生 ⇒ 这笔钱在进程生命周期内一直不入账；
- **容量悬崖**：spill 达到 `GATEWAY_USAGE_SPILL_MAX_MB`（默认 256MB）后 `append` 直接返回错误，后续记录被丢弃并报 BILLING LOSS（`usage_spill.go:94-96`、`async_logger.go:158-167`）——本来只是"延迟"，变成"永久丢失"。

**证据** `TestBugProbeUsageSpillIsOnlyReplayedAtStartup`（SQLite 内存库，确定性）
```
go test ./internal/storage/ -run TestBugProbeUsageSpillIsOnlyReplayedAtStartup -v
  "replayed spilled usage records" count=1                       ← 启动期文件确实被回放
  late spill record: usage_logs rows=0, file present=true, size=358
--- PASS ---
```
第二个（启动后才生成的）spill 文件在 1.2s / 30 个 flush tick 后**仍然是 0 行入库**。

**建议修复**：在 `worker()` 的 ticker 分支里按低频（如 1–5 分钟，或每次"最近有落盘"后的退避）调用 `al.spill.replay(al.repo)`；`replay` 本身已做 claim/rename 与幂等（billing_id、spill 行号派生键），重复调用安全。注意跨副本共享卷用 `claimable` 的 grace 窗口保护，已具备。

**归属**：`internal/storage/async_logger.go`（`usageSpill.replay` 无需改动）。

---

### JOBS-04（P2）TraceCapture 停止后仍接受记录并返回 true

**位置**
- `trace_capture.go:144-172`（`Record`：只检查 `mode == "off"` 与采样，**没有任何 stopped 判定**，`select` 的 `default` 分支之外一律入队并返回 true）
- `trace_capture.go:174-189`（`worker`：`<-tc.stop` 后 drain 并 `return`，之后无人读队列）
- `trace_capture.go:237-247`（`Stop`：`close(stop)` + `wg.Wait()`，不关闭队列、不置停止标志）

**触发条件**：`Stop()` 之后（或与 `Stop()` 并发）任何 `Record()` 调用。服务端顺序是 `srv.Shutdown(ctx)`（等待在途 handler）→ 之后 defer 里 `traceCapture.Stop()`，所以正常 HTTP 路径不命中；但**非 HTTP goroutine**（延迟清理、后台上报、SSE 收尾后的异步钩子）、嵌入方/测试 harness、或 Shutdown 超时（`shutdownTimeout`）后的残留 handler 都可能晚于 Stop 调用 `RecordTrace`。

**根因**：停止只保证"goroutine 退出"，没有让对外接口感知停止状态；队列未关闭，`Record` 的布尔返回值语义（true = 会被归档）与事实脱节。

**影响**：运行期快照静默丢失，且调用方无法区分"已归档"与"丢失"（`ContextKeyTraceRecorded` 还会被置位，导致同一请求的后续重试也不再记录）；`recorded` 计数虚增。属可观测性数据，不涉及金额。

**证据** `TestBugProbeTraceCaptureAcceptsRecordsAfterStop`
```
go test ./internal/storage/ -run TestBugProbeTraceCaptureAcceptsRecordsAfterStop -v
  after Stop: Record() = true, persisted rows = 0
--- PASS ---
```

**建议修复**：`Stop()` 里置 `stopped` 标志（`atomic.Bool`），`Record` 在进入时检查并返回 false；或在 `Stop` 后仍允许"最后一批"入队但由 `Stop` 在 `wg.Wait()` 之前多跑一轮 drain（更复杂）。前者最简单且契约清晰。

**归属**：`internal/storage/trace_capture.go`。

---

### JOBS-05（P3）消费组 consumer 条目无界累积（线上实测 136 个）

**位置**：`async_logger.go:284-296` — `consumerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())`；全仓**没有任何** `XGROUP DELCONSUMER` / consumer 回收逻辑（`grep -rn "DELCONSUMER"` 为空）。

**线上只读证据**（2026-10-29 08:5x）
```
docker exec airoute-redis redis-cli XINFO GROUPS airoute:stream:logs
  consumers: 136   pending: 0   lag: 0   last-delivered-id: 1793234848507-0
docker exec airoute-redis redis-cli --raw XINFO CONSUMERS airoute:stream:logs airoute_log_group | head
  04f614a90021-1  pending 0 idle 258605893   ← 约 3 天前那个容器的残留
  0562f85dcf08-1  pending 0 idle 259838457
```
每次容器重启/滚动发布新增一个 `container-id-pid` 条目，永不删除。

**影响**：不丢单（`XAUTOCLAIM` 会跨 consumer 认领）；但组元数据无界增长，`XINFO`/`XPENDING`（运维与告警都会读）与控制台读放大；孤儿 PEL 排查时死 consumer 噪音掩盖真问题（死 consumer 若留下 pending 行，`MinPendingID` 会把 trim 永久挡住——这一点与 JOBS-01 叠加时更隐蔽）。

**证据（探针，真 Redis）** `TestBugProbeRedisStaleConsumersAreNeverPruned`
```
group consumers: before=1 after one logger lifecycle=2, ghost still present=true
--- PASS ---
```
（探针用 `XREADGROUP` 伪造"上一个进程遗留的 consumer=airoute-ghost-999"，跑完整的 logger start→Stop 生命周期后该条目依然存在。）

**建议修复**：`Stop()`（或 worker 启动时的巡检）对空闲超过 N 天且 `pending=0` 的 consumer 执行 `XGROUP DELCONSUMER`；至少对**自己**这个 consumerID 在退出时清理（`DELCONSUMER` 会自动把它的 pending 转交/需先 `XAUTOCLAIM`，故安全做法是：仅当 `XPENDING ... consumer=<self>` 为 0 时删除自己）。

**归属**：`internal/storage/async_logger.go` + `internal/distributed/redis_streams.go`。

---

### JOBS-06（P3）账本保留：并发冲突分支让整趟退出且无日志

**位置**：`billing_ledger_retention.go:258-268`
```go
deleted, err := res.RowsAffected()
if deleted != int64(len(ids)) {
    // Another replica's retention pass retired (...) some of these rows ...
    return 0, 0, nil
}
```
调用方 `pruneOnce`（`:171-181`）把"0 行"解读为"这轮没东西可删"并 `break` 掉整个批次循环。

**触发条件**：两个副本的 `pruneOnce` 同时命中同一批候选行（`GATEWAY_BILLING_LEDGER_RETENTION_DAYS` 开启时，双副本在相近时刻启动/同一 ticker 周期）。

**根因**：`return 0,0,nil` 把"冲突回滚"与"无事可做"压成同一个返回值。

**影响**：正确性无损（回滚是必须的，否则归档金额会与账本不符）；但那一趟白跑、后续批次推迟一个 interval，且日志里完全没有痕迹（不会出现 WARN），排障时看不到"双副本在互相打架"。

**建议**：返回一个可识别的 sentinel（如 `errPruneContended` 或 `(0,0,errPruneContended)`），`pruneOnce` 记一条限频 WARN 后**重试该批**而不是 break。

---

### JOBS-07（P3）`maybePrune` 占位先于执行，失败后静默停 1 小时

**位置**：`retention_maintenance.go:65-72`（`shouldRunRetention` CAS 占位）、`:77-91`（`maybePrune` 先占位再 `purge`，失败只 WARN）。

**触发条件**：`GATEWAY_*_RETENTION_DAYS > 0` 时，某个写请求触发 chance-prune，恰逢 DB 瞬时错误（锁超时/连接断开）。

**影响**：占位已写入 atomic，下一次尝试要等满 `retentionPruneInterval = 1h`（`retention_maintenance.go:29`）；`verification_codes`/`video_tasks` 不是账单证据，但会导致表在故障窗口内额外膨胀，且 WARN 只发一次（10 分钟限频）。

**建议**：失败时把 slot 回退为 0（`slot.Store(0)`）或使用"上次成功时间"而不是"上次尝试时间"。

---

### JOBS-08（P3/潜在）迁移 runner：deferred 不阻断后续版本；SQLite 无跨进程锁

**位置**：`migrations_runner.go:29-42`（advisory lock 仅 `Dialect()=="postgres"`）、`:63-93`（`errors.Is(err, errMigrationDeferred)` → `continue` 到**下一个版本**）。

**现状与影响**
- 当前唯一会 deferred 的迁移是 v43（`migrations_postgres.go` 条件注册，位于列表末位），且它只解析 `errMigrationDeferred`/锁冲突 ⇒ 目前**无实害**。但"deferred 后继续跑后续版本"是不安全的前向假设：一旦在中间插入可 deferred 的迁移（索引构建、唯一约束去重等），后续迁移会在**缺少该版本 schema**的库上执行，且版本表记录会呈现"43 已记、42 缺失"的乱序态。
- `schema_migrations` 的创建与版本读写均通过 `db.DB`（连接池），不是持锁连接；PG 侧因为 advisory lock 是会话级、由另一个连接持有，仍能互斥（可行）。SQLite 侧无锁：若以多进程方式共享同一个 SQLite 文件（非 Compose 默认，但本地开发/嵌入方可能），两个进程可同时跑迁移 ⇒ 同一版本 `Up` 并发执行 + 版本行重复插入（`version INT PRIMARY KEY` 会挡第二次，但错误会让启动失败）。
- 另：`applyMigration` 的 `Up` 与版本记录不在同一事务（注释已承认），依赖每个迁移"幂等"，v43 的 copy-switch 事实上**不幂等但可恢复**（`relkind='p'` 检测），已复核无数据风险。

**建议**：deferred 时应 `break` 而非 `continue`（把后续版本留到下次启动，保证版本单调）；SQLite 增加 `PRAGMA busy_timeout` + 单进程假设的显式文档/检测。

---

## 3. 已排除项（避免下轮重复）

| 项 | 结论 |
|---|---|
| **XACK 时机** | 正确：先 `BatchRecordUsageLogsContext` 成功，再 `AckLogs`（`async_logger.go:327-353`）；失败 `return false` 留在 PEL，靠 AutoClaim(30s idle) 重投 |
| **stream trim 安全性** | 正确：仅当 `MinPendingID` 为空（全组无 pending）才 `XTRIM MINID ackedUpTo`（`:434-448`）；`ackedUpTo` 只会在 AutoClaim 认领更旧 id 时**回退**，回退只会少删、不会误删未确认条目 |
| **PEL 重投幂等** | 正确：`billing_id` 在 `Record()` 入口钉住（`:105-110`），老载荷用 `BillingIDFromStreamEntry`/`BillingIDFromSpillLine` 派生稳定键；审计侧 `UNIQUE(billing_id)` / 分区模式 `usage_audit_ids` 去重 |
| **批内重复扣费** | **上一轮已修**（`settlement.go:162-213` 的 `seenBillingIDs`），不重报 |
| **结算 vs 回放竞争** | 复核无新缺陷：`insertSettlementRows` 的 `ON CONFLICT DO NOTHING RETURNING` 保证跨事务恰好一次，重复投递只标记 `Settled` 不移动金额 |
| **spill claim/rename 设计** | 正确：`os.Rename` 原子 claim + grace 窗口 + 失败回滚原名；崩溃在 claimed 名下的"派生键漂移"仅影响 **pre-billing_id 老载荷**（当前所有落盘记录都带 BillingID），列为已知残余，不单列 |
| **账本保留的金额口径** | 正确：`DROP`/`DELETE` 与 `billing_ledger_archives` 累加同事务、`settledTotalUnits` 把归档加回、`ledgerDrift` 按 key 扣减，`pruneBatch` 的 `deleted != len(ids)` 回滚是必要保护 |
| **分区路径（PG-only）** | 本地无 PG 分区实例（线上 `usage_logs` relkind=`r`，未启用），仅代码复核：整月 DROP 与 pruned 累加同事务、DEFAULT 分区不 DROP、跨 cutoff 的月份留给批量 DELETE、`usage_audit_ids` 30 天独立时钟——口径自洽；DEFAULT 可能因"时钟回拨/历史月"积累数据而阻塞未来月份 `CREATE PARTITION`，属文档化权衡，未列为缺陷 |
| **`auditLag` 双副本并发** | 推演无缺陷：每趟把 `(evidence, watermark)` 同一事务落库，两副本各自基于旧 baseline 计算的配对**自洽**；落后副本的 watermark 更早 → 下一趟补计，不会重复计数 |
| **`trace_capture` 采样口径** | 正确：`Record` 是唯一采样点（`trace_capture.go:151-154`），`Enabled` 不抽随机数（上一轮 E-01 修复保持有效）；errors 恒采、success 按百分比，符合 `.env.example`/`CONFIGURATION.md` |
| **`PurgeRequestTraces` / `purgeBeforeCutoff` 时区** | 正确：写侧 `CURRENT_TIMESTAMP`（SQLite=UTC）与 `dialectTime`（UTC 文本 / PG timestamptz）一致；线上 trace 最老行 ≈71.5h < TTL 72h，符合预期 |
| **artifact/S3 半写与孤儿** | 本地存储 `tmp + rename` 原子提交且全程持锁；S3 单次 PUT 原子；`SaveSkill/ToggleSkill/DeleteSkill` 会在 DB 成功后尽力删除旧 key（失败仅留下不可达孤儿对象，属 S3 存储成本而非数据错误）。`DownloadSkillZip` 忽略 `Put` 错误后会由随后 `Get` 失败转 500，不会返回坏内容——列为**低危已知**，不单列 |

---

## 4. 线上只读证据（2026-10-29 08:50–08:57 +08）

```bash
# Redis（只读）
docker exec airoute-redis redis-cli XINFO GROUPS airoute:stream:logs
#   consumers=136  pending=0  lag=0  entries-read=65206  last-delivered-id=1793234848507-0
docker exec airoute-redis redis-cli XLEN airoute:stream:logs        # 7（trim 正常）
docker exec airoute-redis redis-cli XPENDING airoute:stream:logs airoute_log_group  # 0（无积压）
docker exec airoute-redis redis-cli --raw XINFO CONSUMERS airoute:stream:logs airoute_log_group | head
#   04f614a90021-1 idle≈258605893ms / 0562f85dcf08-1 idle≈259838457ms   ← 残留 consumer（JOBS-05）
docker exec airoute-redis redis-cli --scan --pattern 'airoute:*'    # cluster:config_version / live:<id> / stream:logs
```

```sql
-- PG（只读）
select relkind from pg_class where relname='usage_logs';            -- r（未分区，分区路径线上未启用）
select count(*) from billing_ledger_archives;                        -- 0（保留策略未开启，符合默认）
select min(created_at), max(created_at), count(*) from request_traces; -- 最老 ≈71.5h < TTL 72h ✔
select (select sum(cost_units) from usage_logs) audit_sum,
       (select sum(cost_units) from billing_settlements) ledger_sum; -- 835452462560 / 835452462560 ✔ 相等
-- per-key 漂移（账实一致性的关键口径）
select count(*) drift_keys, coalesce(sum(abs(d)),0) from (
  select k.key, k.used_cost_units - coalesce(s.t,0) - coalesce(a.cost_units,0) d
  from api_keys k
  left join (select api_key,sum(cost_units) t from billing_settlements group by 1) s on s.api_key=k.key
  left join billing_ledger_archives a on a.api_key=k.key) x where d<>0;   -- 0 行 / 0 units ✔
select count(*) from billing_settlements s where not exists
  (select 1 from api_keys k where k.key=s.api_key);                   -- 28（已删除 key 的结算行，解释全局 sum 与 key 汇总的 0.396 CNY 差）
select key,value from system_settings where key like 'billing_%';     -- audit lag 基线 0、calibration 正常
```

结论：线上**空闲态账实一致**（与 `00-live-verification.md` 一致），没有任何一条 finding 已在线上造成错账；JOBS-01/02/03 是"尚未触发的静默丢失通道"，JOBS-05 是已经在发生的元数据累积（136 个 consumer）。

---

## 5. 复现探针（3 个文件，均不改生产代码）

| 文件 | 探针 | 依赖 | 断言（当前= buggy 行为） |
|---|---|---|---|
| `internal/storage/usage_spill_replay_bugprobe_test.go` | `TestBugProbeUsageSpillIsOnlyReplayedAtStartup` | SQLite 内存库 + `t.TempDir()` | 启动期 spill 文件回放成功（1 行）；**启动后**生成的文件 1.2s 后仍 0 行且文件仍在 |
| `internal/storage/trace_capture_stop_bugprobe_test.go` | `TestBugProbeTraceCaptureAcceptsRecordsAfterStop` | SQLite 内存库 | `Stop()` 后 `Record()` 返回 **true**，`request_traces` **0 行** |
| `internal/storage/async_stream_bugprobe_test.go` | `TestBugProbeRedisConsumerNeverRecreatesMissingGroup`<br>`TestBugProbeRedisPoisonEntryIsAckedSilently`<br>`TestBugProbeRedisStaleConsumersAreNeverPruned` | **可丢弃的真 Redis**（`AIROUTE_TEST_REDIS_URL`，未设时 `t.Skip`） | 组丢失后 0 行入库/stream 滞留；毒丸 pending=0 且 dropped 指标 delta=0；伪造的旧 consumer 在完整生命周期后仍存在 |

```bash
export PATH=$PATH:/usr/local/go/bin

# 无外部依赖的两条（确定性）
go test ./internal/storage/ -run 'TestBugProbeUsageSpillIsOnlyReplayedAtStartup|TestBugProbeTraceCaptureAcceptsRecordsAfterStop' -v

# Redis 三条（隔离容器，端口自选，不与生产 airoute-redis 混用）
docker run -d --rm --name airoute-bugprobe-redis -p 16379:6379 redis:7-alpine
AIROUTE_TEST_REDIS_URL=redis://127.0.0.1:16379/0 \
  go test ./internal/storage/ -run 'TestBugProbeRedis' -v -timeout 90s
docker stop airoute-bugprobe-redis
```

**质量门（探针加入后，本机实测）**

```
gofmt -l internal/storage/*bugprobe_test.go   → 空
go vet ./internal/storage/                    → OK
go test -count=1 ./...                        → 16/16 包 ok（storage 8.47s，全量含 cmd/api/controlplane/tests/web）
```

> 探针刻意断言**当前错误行为**并通过，因此不会让 `go test ./...` 变红；修复后需按注释翻转断言并去掉 `bugprobe` 后缀（沿用上一轮 §7.5 的约定）。

---

## 6. 修复优先级建议

1. **JOBS-01（P1）**：`NOGROUP` → 重建组 + 限频告警 + 计数。改动小、收益最大（消除"永久静默停摆"）。
2. **JOBS-02（P2）**：毒丸计数 + 限频 ERROR（+ quarantine）。10 行以内。
3. **JOBS-03（P2）**：worker ticker 里周期 replay（复用现成 `spill.replay`）。需要注意多副本 claim 已具备。
4. **JOBS-04（P2）**：`TraceCapture` 增加 `stopped` 标志并让 `Record` 返回 false。
5. P3 三条可合并成一个"维护路径可观测性"小提交（contended sentinel、失败回退 slot、deferred 时 break）。

**跨轮注意**：JOBS-01/03 与上一轮 `settlement_batcher` 的"defer 到审计管道结算"语义耦合——管道停摆/延迟即意味着那部分金额延迟入账，修复时建议同时确认 `settlement_wait_timeouts` 的告警阈值能覆盖"管道积压"这个维度。

---

## 6. 修复记录（2026-10-29，状态 buggy → fixed）

Lead 复核后按 JOBS-01→04→06/07 修复；JOBS-05/08 仅登记不修。生产代码改动限定在异步/维护路径文件，**未 commit**。

| ID | 状态 | 生产代码改动 | 回归测试（原 `*bugprobe*` 已删除/翻转） | 验证 |
|---|---|---|---|---|
| **JOBS-01**（P1） | **fixed** | `internal/distributed/redis_streams.go`：新增 `ErrConsumerGroupMissing` / `IsConsumerGroupMissing`（按 `NOGROUP`/`no such key` 分类）、`ConsumerGroupStats`（pending/consumers/lag）、`DeadLetterLog`；`EnsureConsumerGroup` 把组缺失折成可识别错误。`internal/storage/async_logger.go`：新增 `ensureLogConsumerGroup`（5 次退避重试，不再 `_ =` 丢弃）、`logConsumerGroupHealth`（启动/恢复自检日志）、read 路径 `NOGROUP` → **自动重建组 + 计数 + ERROR + 立即继续**（不再裸 sleep）、其它读错误 → 计数 + 限流 WARN；AutoClaim 周期分支也做缺失组重建（无流量时也能恢复）。新增指标 `UsageLogStreamReadFailures` / `UsageLogGroupRecreated` | `internal/storage/stream_consumer_integration_test.go`：`TestRedisConsumerRecoversFromMissingGroup`（删组后 6s 内入库=1、`UsageLogGroupRecreated` 递增、PEL 已 ACK）；`internal/distributed/redis_streams_classify_test.go`：`TestIsConsumerGroupMissing`（5 类错误分类）、`TestConsumerGroupStatsInactiveClient` | `AIROUTE_TEST_REDIS_URL=… go test ./internal/storage/ -run TestRedis -v` → PASS |
| **JOBS-02**（P2） | **fixed** | `async_logger.go`：`processStreamMessages` 对「缺 data / data 为空 / 非法 JSON」三类毒丸统一走 `deadLetterStreamEntry`：递增 `UsageLogStreamPoisoned`、ERROR（含 entry id + reason + payload 字节数）、写入**有界** DLQ `airoute:stream:dead`（`usageLogDeadLetterMaxLen=1000` 精确 MAXLEN），payload 按 **rune 边界**截断到 4KB；随后照旧 ACK（不卡 PEL） | 同文件：`TestRedisPoisonEntryIsDeadLettered`（DLQ 2 条、`original_id`/`reason` 正确、poisoned 计数=2、pending=0、健康记录仍入库）；`TestRedisDeadLetterStreamIsBounded`（12 条写入 limit=5 后长度 ≤5） | 同上 → PASS |
| **JOBS-03**（P2） | **fixed** | `async_logger.go`：一次性 `spillReplayer` → `spillReplayLoop`（启动立即一趟 + 每 `GATEWAY_USAGE_SPILL_REPLAY_SEC`（默认 60s）一趟 + 停机最后一趟），`spillRecords` 成功后 `kickSpillReplay()` **触发式立即回放**；`spillReplayMu` 单飞防并发；失败保留文件（沿用 `usage_spill.replay` 既有语义）。新增指标 `UsageLogSpillReplays` | `internal/storage/usage_spill_replay_test.go`：`TestUsageSpillIsReplayedAfterStartup`（启动后落盘的记录 5s 内入库、文件被删除）、`TestUsageSpillReplayPicksUpAnotherProcessFile`（另一进程/副本的 quiet 文件被周期趟回放） | `go test ./internal/storage/ -run TestUsageSpill` → PASS |
| **JOBS-04**（P2） | **fixed** | `trace_capture.go`：新增 `stopped atomic.Bool`；`Stop()` 置位（`stopOnce` 内，仍幂等）；`Record()` / `Enabled()` 在停止后返回 **false**；已入队记录仍由 `drain()` 落库 | `internal/storage/trace_capture_lifecycle_test.go`：`TestTraceCaptureRejectsRecordsAfterStop`（Record=false、Enabled=false、0 行）、`TestTraceCaptureStopIsIdempotent`（Stop 两次）、`TestTraceCaptureDrainsQueuedRecordOnStop`（Stop 前入队的仍落库=1，不误伤） | `go test ./internal/storage/ -run TestTraceCapture` → PASS |
| **JOBS-06**（P3） | **fixed** | `billing_ledger_retention.go`：`deleted != len(ids)` 不再 `return 0,0,nil`（那让整趟静默 break），改为 `errBillingLedgerPruneContended` 哨兵 + `ledgerPruneDeletionMismatch` 分类；新增 `pruneBatchWithRetry`：冲突时递增 `BillingLedgerPruneContended`、限流 WARN、**重试该批一次**；二次冲突才把该趟留给下个 interval | `internal/storage/retention_schedule_test.go`：`TestLedgerPruneDeletionMismatchClassification`（相等/少删/多删三个分支） | `go test ./internal/storage/ -run TestLedgerPrune` → PASS |
| **JOBS-07**（P3） | **fixed** | `retention_maintenance.go`：purge 失败时 `rescheduleRetentionRetry` 把 slot 回卷成「`retentionPruneInterval-retentionPruneRetryInterval` 之前已尝试」，即 **1 分钟后**可重试（既不静默停 1h，也不形成重试风暴）；WARN 附带 `retry_in` | 同文件：`TestMaybePruneFailureAllowsFasterRetry`（失败后 ~1min 到期、立即重试仍被限流）、`TestMaybePruneSuccessKeepsFullInterval`（成功仍保持 1h 节流、只跑 1 次）、`TestRescheduleRetentionRetryEdgeCases`（nil slot / retry≥interval） | 同上 → PASS |
| JOBS-05（P3） | 登记不修 | 无（consumer 条目回收需运维策略，且 `DELCONSUMER` 与 PEL 交接需单独设计） | — | 报告 §2 保留 |
| JOBS-08（P3） | 登记不修 | 无（deferred 语义变更影响迁移矩阵，属独立变更） | — | 报告 §2 保留 |

### 6.1 质量门结果

| 检查 | 命令 | 结果 |
|---|---|---|
| gofmt | `gofmt -l`（本次改动全部文件） | 空 |
| build | `go build ./...` | OK |
| vet | `go vet ./internal/storage/ ./internal/distributed/ ./internal/telemetry/` | OK |
| 常规全量 | `go test -count=1 ./internal/storage/ ./internal/distributed/ ./internal/telemetry/` | 3/3 ok（storage 8.6s） |
| race | `AIROUTE_TEST_REDIS_URL=… go test -race ./internal/storage/ -run 'TestRedis\|TestUsageSpill\|TestTraceCapture\|TestLedgerPrune\|TestMaybePrune\|TestReschedule'` | ok，无 DATA RACE |
| 真 Redis 集成 | `docker run -d --rm -p 16379:6379 redis:7-alpine` + `AIROUTE_TEST_REDIS_URL=redis://127.0.0.1:16379/0 go test ./internal/storage/ -run TestRedis -v` | 3/3 PASS（容器跑完已 `docker rm -f` 清理） |
| 全仓 | `go test -count=1 ./...` | 我的 3 个包 ok；`internal/controlplane` 失败**与本轮改动无关**——是 bug-cp 正在进行的 CP 修复（其 `*bugprobe*` 探针报告 "appears fixed … delete this probe"，且 batch-status 校验正在改为 400），呈并发编辑中间态 |

### 6.2 行为变化（需写入变更记录）

1. **审计管道自愈**（JOBS-01）：Redis 消费组丢失（failover/无持久化重启/误删）不再需要重启网关——消费者在 read 路径与 30s 周期检查中发现 `NOGROUP` 即重建组并从 `0` 重放积压（`billing_id` 幂等保证不重复入账）。同时 `/metrics` 新增 `airoute_usage_log_stream_read_failures_total`、`airoute_usage_log_group_recreated_total`，启动日志新增 `audit-log consumer group health`（pending/consumers/lag）。
2. **毒丸可见**（JOBS-02）：无法解码的 stream 条目会写入 `airoute:stream:dead`（上限 1000 条）并递增 `airoute_usage_log_stream_poisoned_total`。运维多了「查 DLQ」这一恢复入口；DLQ 是诊断流，**不是**账单证据，不参与对账。
3. **spill 持续回放**（JOBS-03）：`GATEWAY_USAGE_SPILL_REPLAY_SEC`（默认 60s）控制周期；落盘后还会立即触发一次。审计行不再需要「等重启」才补；`airoute_usage_log_spill_replays_total` 可观察补录量。
4. **trace 停止语义**（JOBS-04）：`Stop()` 后 `Record`/`Enabled` 返回 false（调用方不再误判「已归档」）；已入队快照仍会落库，不影响正常关停数据完整性。
5. **维护路径可观测**（JOBS-06/07）：账本保留的副本冲突会 `WARN` + `airoute_billing_ledger_prune_contended_total` + 重试一次；机会式清理失败后 1 分钟即可重试（此前静默 1 小时）。

### 6.3 测试文件变更清单

- **删除（buggy 行为探针）**：`internal/storage/usage_spill_replay_bugprobe_test.go`、`internal/storage/trace_capture_stop_bugprobe_test.go`、`internal/storage/async_stream_bugprobe_test.go`。
- **新增（常规回归，默认 `go test ./...` 执行）**：`internal/storage/usage_spill_replay_test.go`、`internal/storage/trace_capture_lifecycle_test.go`、`internal/storage/retention_schedule_test.go`、`internal/distributed/redis_streams_classify_test.go`。
- **新增（真 Redis，`AIROUTE_TEST_REDIS_URL` 未设时 skip）**：`internal/storage/stream_consumer_integration_test.go`。
