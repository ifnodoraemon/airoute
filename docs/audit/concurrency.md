# Airoute 网关 —— 并发能力与热路径性能审计

> 任务：**如何提升并发能力**（task-2）
> 审计范围：数据面 `/v1/*`、`/v1beta/*` 全链路（中间件 → 路由 → 上游流式 → 计费/日志），连接池、HTTP 服务器、单 goroutine 工作线程
> 数据快照：**2026-10-23 10:09 – 10:22 (Asia/Shanghai)**，2 副本 `airoute-1/airoute-2` + `airoute-lb`(nginx) + `airoute-postgres`(16) + `airoute-redis`(7)
> 只读约束：**未修改任何 Go 源码、未重启任何容器、DB 仅 SELECT / EXPLAIN**（`pg_stat_statements` 可用但未安装，未做 CREATE EXTENSION）
> 实测手段：`/proc/1/stat` CPU 时间差分、`docker exec airoute-redis MONITOR`、`INFO commandstats` 增量、同 bridge 兄弟容器实测 RTT、`/proc/1/fd`、访问日志逐行解析
> **代码快照说明**：审计期间有其他进程/代理在并发修改部分源码（`internal/storage/async_logger.go`、`repo_logs_writer.go`、`internal/telemetry/metrics.go`、`internal/router/circuit_breaker.go` 的 mtime 为 10:19–10:22，正在加入 `billing_id` 幂等键）。本报告所有 `file:line` 已于 **10:27 重新核对到当前工作树**；这些并发改动只新增字段/幂等键，未推翻本报告的并发结论。

---

## 0. 结论速览（TL;DR）

**并发能力的第一瓶颈不是"每请求多查一次 DB"，而是三个硬上限：容器 FD=1024、Postgres 连接数（2×50=100 恰好打满 max_connections=100）、以及每请求约 60ms 的 CPU。**

| # | 结论 | 关键数字 | 级别 |
|---|------|----------|------|
| 1 | **容器 `ulimit -n = 1024`**（gateway/nginx/postgres/redis 全部），每路流式请求占 **2 个 FD**（下游 + 上游） | 单副本并发 ≈ **390–480 路**，集群 ≈ 780–960 路；按实测时延 4.19s 换算 → **单副本仅 ~93–115 req/s，集群 ~190–230 req/s** | **P0** |
| 2 | **Postgres 连接预算被算死**：`SetMaxOpenConns(50)` × 2 副本 = **100 = max_connections**，可用连接仅 **97**（`superuser_reserved_connections=3`）；且 `MaxIdleConns(25)` 让约 **40 条连接长期 idle 占坑** | 实测 45/100 已占用（39 idle），几乎零流量下就占 45%；**再加第 3 个副本必然连接耗尽** | **P0** |
| 3 | **每请求 CPU ≈ 59.7ms**（实测：29 请求 / 1.73s CPU）；同一时段平均 prompt **18.6 万 token**、completion **985 token**、响应 **19–109KB** → 约 **200–1000 个 SSE 帧/请求** | 20 核 → CPU 上限 **≈335 req/s/副本**；逐帧路径（每帧 1 次全局锁 + 2 次 `json` + 2 次 `fmt` + 1 次 `Flush`）是最大可优化项 | **P1** |
| 4 | **请求体被无条件全量读入并保留整个流生命周期**：`CaptureRequestSnapshot` 在 `mode=errors`（默认）下也会 `GetRawData()` 全量缓冲，再用 `bytes.NewReader` 挂回 Body → 约 **600–750KB/请求** 存活到流结束 | 400 并发 ≈ **240–300MB** 额外常驻 + 全量拷贝开销；`trace` 模式为 `errors` 时成功请求 100% 白付 | **P1** |
| 5 | **同步 Redis 调用在请求 goroutine 内**：完成路径 `XADD`（250ms 超时）；租户 key 另加 **3 次 EVALSHA**（RPM + quota 申请/释放） | 典型 +0.3ms（XADD）/ +1.0ms（租户 4 次往返，实测 RTT 0.28ms）；Redis 抖动时最坏 **+0.75s（XADD fallback）/ +3~4s（AllowRPM 无超时+重试3次）** | **P1** |
| 6 | **当前流量 100% 走 admin key**（`sk-airoute-j9yhwt5tgd4d`，rpm=0、role=admin）→ 限流/quota/TPM 路径**完全未被压测**；换成租户 key 后每请求多 4 次 Redis 往返 | MONITOR 60s 实测：`evalsha` 增量 **0**、`xadd` 增量 13/18 请求 | **P1（风险）** |
| 7 | 每请求固定 **2 行 INFO 日志**（`http_access` + `attempting streaming connection`），全部经同一 slog handler 互斥锁 + stdout 写 + 环形缓冲 `map` 分配 | 1000 req/s 时 2000 行/s 串行化；另加 **每 10s 每副本 2 行** reload 日志（实测 2 行/10s/replica） | **P2** |
| 8 | **澄清**：每请求同步 DB 权威查询确实存在但很便宜（实测 0.286ms mean），10s 全量 reload 也很便宜（~8 条查询/10s）；两者是"该删的债"而非"当前的瓶颈" | auth 查询占 4.19s 时延的 **0.007%** | **P2/P3** |

> **一句话**：想提升并发，先改 `ulimits`（P0，收益 2 倍以上）和 PG 连接预算（P0，决定能否加副本）；再降每请求 CPU（P1，决定天花板）；Redis 往返与日志是"故障放大项"，正常时几乎不花钱、异常时会把时延翻倍。

---

## 1. 实测基线（数字出处）

| 指标 | 实测值 | 采集方式 |
|---|---|---|
| 单副本请求总量 | 1020 (r1) / 971 (r2)，运行 36min | 直连容器 `curl 127.0.0.1:8080/metrics` |
| 数据面平均时延 | **4191ms (r1) / 4183ms (r2)** | `airoute_latency_avg_ms` |
| TTFT 均值 | 4045ms / 4041ms | `airoute_ttft_avg_ms` |
| p50 / p90 / p99 | **2085 / 6373 / 60000 ms** | `airoute_latency_p50_ms` 等 |
| 活跃连接 | **1–2** | `airoute_active_connections` |
| 峰值吞吐 | **319 req/s**（10:09:44，174+145，两副本合计） | 访问日志按秒聚合；**但均为快速失败（429/502，≈1ms）**，不代表流式并发能力 |
| CPU / 请求 | **59.7 ms**（60s 窗口：29 请求 / 1.730s CPU，`/proc/1/stat` utime+stime 差分） | 见附录命令 |
| 平均 prompt / completion | **186,459 / 985 token**（2min 内 52 行：9,695,892 / 51,214） | `usage_logs` 只读聚合 |
| 响应体大小 | 19,131 – 108,990 bytes | 访问日志 `bytes` 字段 |
| Redis 往返（同 bridge） | **avg 0.28ms / max 2ms**（97 样本） | 兄弟容器 `redis-cli --latency` |
| Redis 服务端单命令耗时 | `evalsha` **60.77µs**、`xadd` **22.42µs**、`get` 6.3µs、`xreadgroup` 15.67µs | `INFO commandstats` |
| PG auth 查询（含往返） | **n=10: min 0.170 / p50 0.239 / mean 0.286 / max 0.785 ms**（RTT 地板 0.169ms） | 兄弟容器 `psql \timing` × 10 次 |
| PG 连接 | **45/100**（39 idle + 1 active + 5 后台），`max_connections=100`，`superuser_reserved_connections=3` | `pg_stat_activity` |
| Redis 连接 | `connected_clients` 80–82（每副本约 40），`maxclients=10000`，AOF 开启 | `INFO clients` |
| FD 上限 / 已用 | **1024**（podman/docker 默认）；gateway 53–56（其中 socket 47–50） | `ulimit -n`、`/proc/1/fd` |
| nginx | `worker_processes auto`(=20) × `worker_connections 1024`，`keepalive 256`，`least_conn` | `/etc/nginx/nginx.conf` + `deploy/nginx-compose.conf:21-26` |
| CPU / GOMAXPROCS | 20 核；容器无 CPU limit；**未设置 GOMAXPROCS/GOGC/GOMEMLIMIT** | `nproc`、`docker exec env` |
| Redis Stream 消费组 | XLEN=7、lag=0、pending=0、**consumers=50 个（重启遗留）** | `XINFO GROUPS/CONSUMERS` |

---

## 2. 数据面热路径逐跳拆解

### 2.1 中间件装配（`internal/api/router.go`）

```
全局（所有路由）: gin.Recovery() :22
  → middleware.BodyLimit(64MiB)            :25
  → middleware.TraceMiddleware()           :28   ← 生成 trace_id、写 2 个响应头
  → middleware.AccessLogMiddleware()       :29   ← 每请求 1 行 INFO（slog 全局锁 + stdout 写）
  → CORS/安全头闭包                        :32-61
数据面 /v1        : v1.Use(AuthMiddleware) .Use(RateLimitMiddleware)   :250-252
数据面 /v1beta    : 同上                                              :278-280
根路径别名        : 同上（同 3 组各挂一次）                            :289-291
```

结论：`/v1/chat/completions`、`/v1/messages` 只经过 **BodyLimit → Trace → AccessLog → Auth → RateLimit → Handler**；**没有**全局并发上限中间件、没有 pprof、没有超时中间件。

### 2.2 每请求阻塞/网络调用明细（流式，租户 key 最坏情况）

| # | 调用点 | 类型 | 触发条件 | 典型耗时（实测/推算） | 最坏耗时（超时来源） |
|---|---|---|---|---|---|
| 1 | `config.GetAPIKey` + `ActiveAPIKeyCount` (`middleware/auth.go:64,103`) | 进程内 `RWMutex` | 每请求 3 次 | <1µs | 与 reload 写锁竞争时 ≈ 一次 map 重建时间（µs 级） |
| 2 | `repo.GetAPIKeyByKeyContext` (`middleware/auth.go:151-155`) | **PG 查询** | 每请求 ×1 | **0.286ms**（p50 0.239） | **2000ms**（`authStorageTimeout=2s`, `auth.go:22`）→ **超时后 fail-open 继续放行** |
| 3 | `repo.GetUserByIDContext` (`auth.go:207-216`) | PG 查询 | 仅快照缺 user 字段 | — | 2000ms（当前不触发：快照已带 user 信息） |
| 4 | `globalQuotaTracker.TryAcquire` (`auth.go:246` → `quota_tracker.go:41-49`) | **Redis EVALSHA** | `userStatus!="" && role!=admin && balance>0 && userID>0`（当前 admin 流量**不触发**） | **0.34ms**（60.8µs + 0.28ms RTT） | **300ms**（`quota_tracker.go:42`） |
| 5 | `GlobalTokenQuota.Allow` + `Remaining` (`ratelimit.go:168,180`) | 进程内 ×2 锁 | `k.TPM>0`（当前 TPM=0，不触发） | ~0.2µs | — |
| 6 | `client.AllowRPM` (`ratelimit.go:191` → `redis_quota.go:72`) | **Redis EVALSHA（无独立超时！）** | `k.RPM>0`（当前 rpm=0，不触发） | ~0.36ms | **无 deadline**，仅受 `ReadTimeout=1s` + go-redis 默认 `MaxRetries=3` 约束 → **≈3–4s** |
| 7 | `SessionAffinitySelector.Select` → `GetSessionChannel` (`load_balancer.go:33-48`) | **Redis GET** | 有 sessionID（**总是有**，见 `api/session.go:131-138`）**且** 首个 priority 组内 >1 channel | — | 1s×3 重试（当前只有 1 个 active channel → **不触发**） |
| 8 | `SetSessionChannel` (`load_balancer.go:75-79`) | Redis SET（goroutine 内，不阻塞） | 同上 | — | 不影响请求 |
| 9 | 上游 `prov.ChatCompleteStream` (`provider/openai.go:197`) | **HTTP 上游** | 每请求 ×1 | **TTFT 4045ms / 总 4191ms** | 首 token 超时 30s（reasoning 模型 60s，`dispatcher_stream.go:252-266`）+ `ResponseHeaderTimeout 60s` |
| 10 | `globalQuotaTracker.Release` (`auth.go:257` defer → `quota_tracker.go:68-75`) | **Redis EVALSHA** | 与 #4 配对，**请求结束（流结束后）才释放** | 0.34ms | **300ms** |
| 11 | `billing.CalculateCostForUnits` (`api/common.go:61` → `engine.go:329,372`) | 进程内 **2× RWMutex RLock** + map 查找 | 每成功请求 | ~0.2µs | 价格 reload 写锁排队时 μs 级 |
| 12 | `GlobalTokenQuota.Consume` (`common.go:80`) | 进程内 ×2 锁 | `TPM>0` | ~0.1µs | — |
| 13 | `telemetry.GlobalMetrics.RecordRequestWithModel` (`common.go:83` → `metrics.go:67-110`) | 进程内**全局互斥锁** | 每请求 ×1 | ~0.2µs（无竞争） | 高并发下随基数增长 |
| 14 | `GlobalLiveMetrics.AddTokens/AddChars` (`common.go:96`, 流式每帧 `chat_stream_handler.go:181`) | 进程内**全局互斥锁** | 非流式 ×1 / **流式每 SSE 帧 ×1** | 每帧 ~20-40ns | 所有流在同一把锁上排队 |
| 15 | `AsyncLogger.Record` → `XAddLog` (`common.go:104` → `async_logger.go:106-122`) | **Redis XADD（同步！）** | Redis active 时**每请求 ×1**（含 4xx 审计） | **0.30ms**（22.4µs + 0.28ms） | **250ms**（`async_logger.go:111`）；失败后回退本地 channel 再等 **500ms**（`:129-145`）→ **最坏 750ms** |
| 16 | `AccessLogMiddleware` 记日志 (`middleware/trace.go:144-152`) | 进程内**全局锁 + 写 stdout(syscall)** + 环形缓冲 `map` 分配 (`ring_buffer.go:211-258`) | 每请求 ×1 | ~10–30µs | 高 QPS 下串行排队 |
| 17 | `dispatcher_stream.go:62-66` + `dispatcher.go:215-220` 的 INFO 日志 | 同上 | **每请求 ×1** | ~10–30µs | 同上 |
| 18 | `CaptureRequestSnapshot` (`handler.go:30` / `inbound_anthropic.go:44` → `trace_hooks.go:24-41`) | 全量读 body + 保留引用 | **每请求 ×1**（即使 trace 关闭/成功） | 实测 body **600–750KB**：拷贝 ≈0.3–1ms + 整段内存存活至流结束 | 上限 `GATEWAY_MAX_BODY_BYTES` 64MiB（nginx 50m） |

**合计**：
- **admin key（当前线上）**：DB 0.29ms + XADD 0.30ms ≈ **0.6ms**，占 4191ms 的 **0.014%**。
- **租户 key（未来真实收费流量）**：DB 0.29ms + 4×Redis ≈ 1.4ms ≈ **1.7ms**；若 `RPM>0 && 多 channel` 再加 0.28ms。
- **异常放大**：Redis 半死时 `AllowRPM`(3–4s) + quota(0.6s) + XADD(0.25→0.75s) ≈ **最坏 4.8s**，**相当于把 4.19s 的平均时延翻倍**。

### 2.3 一个必须知道的事实：当前流量没有压到"租户路径"

MONITOR 实测 60s：`evalsha` **增量 0**、`xadd` 增量 13（18 个完成请求）、`ratelimit:*` / `inflight:*` key **一个都不存在**。原因（`api_keys` 只读核对）：

| key id | key | rpm | tpm | user_id | role | balance |
|---|---|---|---|---|---|---|
| 1 | `sk-airoute-j9yhwt5tgd4d` | 0 | 0 | 1 | **admin** | 10000149 |
| 15 | `sk-airoute-10d0…72d8` | 600 | 0 | 1 | admin | 10000149 |
| 16 | `sk-airoute-d2fc…7d55` | 60 | 100000 | 3 | user | 998.98 |
| 18 | `sk-airoute-ce7c…874f` | 600 | 0 | 3 | user | 998.98 |

→ 线上流量走的是 **id=1：rpm=0（跳过 Redis 限流）、role=admin（跳过 quota 预留）、tpm=0（跳过 TPM）**。
**含义：限流 / TPM / 分布式 quota 三套路径在真实并发下从未被验证过**，一旦切到租户 key，每请求多 4 次 Redis 往返、且 `AllowRPM` 无超时，是上线后最可能"突然变慢"的地方。

---

## 3. 并发上限推导（Scaling Ceiling）

### 3.1 各资源上限

| 资源 | 配置 | 上限算式 | 结果（单副本 / 集群） | 是否当前瓶颈 |
|---|---|---|---|---|
| **文件描述符** | `ulimit -n = 1024`，`/proc/1/fd` 基线 53–56 | `(1024 − 56 基线 − 178 池最坏[PG50+Redis128]) / 2 FD每流` | **≈395 路 / ≈790 路** | ✅ **第一瓶颈** |
| Postgres 连接 | `SetMaxOpenConns(50)` ×2 = 100；`max_connections=100`，可用 97 | 3 副本 = 150 > 97 | 2 副本 **刚好打满**；**3 副本必然失败** | ✅ **扩容硬墙** |
| Postgres 吞吐 | auth 查询 0.286ms/次，占连接时间 | 50 / 0.286ms | **≈17.5 万次/s** | ❌ 远不是瓶颈 |
| Redis 吞吐 | 单线程；租户 4 ops/请求 | ~10 万 ops/s ÷ 4 | **≈2.5 万 req/s** | ❌ 不是瓶颈 |
| Redis 连接池 | `PoolSize=128`, `MinIdleConns=16` | 128 × (1/0.28ms) | **≈45 万 ops/s** | ❌ 不是瓶颈 |
| CPU | 20 核，`GOMAXPROCS=20`（无 cgroup limit） | `20 / 0.0597s` | **≈335 req/s / 副本**（≈670 集群） | ⚠️ 第二瓶颈（FD 之后） |
| async_logger 单 worker | `batchSize=100`, `flushInterval=500ms`, buffer=10000 (`server.go:161`) | 每批 ≈ (4×multi-row INSERT + N×UPDATE + 1×UPDATE users + COMMIT/fsync) ≈ 3–10ms | **≈1–3 万条/s**（远高于需求） | ⚠️ **故障型**：DB 停顿 10s 则 buffer 满 → 每请求阻塞 500ms |
| Redis Stream 单 consumer | 每副本 1 goroutine，`Count=100`, `Block=500ms` (`async_logger.go:375`) | 100 条 / (RTT+插入) | >1 万条/s | ❌ 不是瓶颈 |
| goroutine | 每流 3 个（handler + `pipeStreamEvents` + provider reader），2×64 事件 channel | Go 无硬上限，~2KB 栈 | 1 万路 ≈ 3 万 goroutine ≈ 60MB | ❌ 不是瓶颈 |
| 内存（请求体） | 每请求保留 600–750KB body（实测 prompt 18.6 万 token） | 17.5GB 可用 / 0.7MB | **≈2.5 万路**（但有 GC 压力） | ⚠️ 400 并发 ≈ +280MB |
| nginx | 20 worker × `worker_connections 1024`，但 **进程 FD 也是 1024** → 每 worker 实际 ≈512 个代理连接 | 20 × 512 | **≈1 万路 / 集群** | 次于 gateway |
| HTTP Server | `ReadTimeout 600s`、`WriteTimeout 600s`、`IdleTimeout 120s`、`ReadHeaderTimeout 15s`、`MaxHeaderBytes 1MiB` (`server.go:217-225`, `configs/config.yaml:10-11`) | 无 `MaxConns`，600s 超时把慢连接放大 143 倍于平均时延 | 慢速攻击可吃满 1024 FD | ⚠️ 安全/容量风险 |

### 3.2 Little's Law 换算（并发 → 吞吐）

`并发 = 吞吐 × 时延`。以 FD 上限 395–480 路/副本为硬顶：

| 场景 | 平均时延 | 单副本吞吐上限 | 集群（2 副本） |
|---|---|---|---|
| 快速失败（429/502） | ~1ms | 395,000 req/s（受 CPU/nginx 限制） | — |
| **实测流式（当前负载）** | **4.19s** | **94–115 req/s** | **189–229 req/s** |
| Lead 报告的平均（6.7s） | 6.7s | 59–72 req/s | 118–143 req/s |
| 长文/推理（p90 6.37s） | 6.37s | 62–75 req/s | 124–151 req/s |

> 对照：10:09:44 实测的 **319 req/s** 峰值是 429/502 快速失败打出来的（时延 ~1ms，并发≈0.3），**不代表**流式业务能力。若这 319 req/s 是 4.19s 的流式请求，需要 **2,137 路并发**，远超当前 780–960 路的集群上限 → 会被 FD 直接顶住。
> **也就是说：当前架构的真实流式吞吐上限 ≈ 190–230 req/s（集群），想再往上必须先解 FD。**

### 3.3 特别提醒：横扩能力被 PG 连接数学锁死

```
Postgres max_connections = 100, superuser_reserved = 3  →  可用 97
每副本 SetMaxOpenConns(50) → 2 副本 = 100 > 97   （当前已零余量）
第 3 副本 = 150 → 必然出现 "sorry, too many clients already"
实测：45/100 已被占用，其中 39 条是 idle —— MaxIdleConns(25)/副本 长期占坑
```
**结论：现在的配置只允许 2 个副本，且 console API 与数据面共用同一进程/同一连接池，任何后台任务（reload、admin 查询、迁移、psql 运维）都在抢这 97 个槽位。** 这是"加副本提升并发"路上的第一道墙。

---

## 4. 发现明细

### F1【P0】容器 FD 上限 1024 —— 直接决定并发路数
- 证据：`docker exec airoute-1 sh -c 'ulimit -n'` → **1024**（lb/pg/redis 同）；`/proc/1/fd` = 53–56（socket 47–50）。
- 机理：每路流式请求必须同时持有 **下游 client FD + 上游 provider FD**（`server.go:217-225` + `provider/openai.go:197`），即 2 FD/流；`docker-compose.yml` 未配置 `ulimits`，继承默认 1024。
- 数字：`(1024 − 56 − 178) / 2 ≈ 395` 路/副本；对应 4.19s 时延 → **94 req/s/副本**。
- 影响：超过 ~400 路后表现为 `accept4: too many open files` / `dial tcp: too many open files`，**错误率陡增且无告警指标**。
- 修复：`docker-compose.yml` 两个 gateway 服务加 `ulimits: {nofile: 65536}`；nginx 加 `worker_rlimit_nofile 65536`。**行为零变化，收益 ≈ FD 顶从 395 → 数万，吞吐顶从 ~100 → ~335 req/s/副本（转为 CPU 受限）**。

### F2【P0】Postgres 连接预算：2×50 = 100 = max_connections
- 代码：`internal/storage/db.go:239-245`（`SetMaxOpenConns(50)`、`SetMaxIdleConns(25)`、`ConnMaxLifetime 10m`、`ConnMaxIdleTime 5m`）。
- 实测：`max_connections=100`、`superuser_reserved_connections=3`、当前 **45 连接（39 idle）**。
- 风险 A（扩容）：3 副本必然连接耗尽（见 3.3）。
- 风险 B（无界等待）：`database/sql` 在池满时**无超时地阻塞**。数据面 auth 有 2s ctx 兜底（`auth.go:153`），但 **control-plane / reload / admin 查询大量使用无 ctx 的 `QueryRow/Exec`**（如 `repo_channels.go:320`、`repo_keys.go:285`），一旦池被占满，`ReloadFromDB` 会卡在 `sync.go:152` 的 `s.mu` 持锁状态下 → **控制面全面挂起**（后台 reload、admin 写操作全排在同一把锁后）。
- 修复（最小）：`MaxOpenConns 25~30`、`MaxIdleConns 8~10`（当前 25 idle/副本纯浪费 ~40 槽位），并给 reload 的所有查询加 ctx 超时；或上 PgBouncer（transaction pooling）把 97 变成 ~1000。

### F3【P1】每请求 CPU ≈ 59.7ms，逐 SSE 帧路径是主成本
- 实测：60s 窗口 29 请求、1.730s CPU（`/proc/1/stat` utime+stime）→ **59.7ms/请求**；同期平均 18.6万 prompt token、985 completion token、响应 19–109KB。
- 逐帧成本（每帧）：`telemetry.GlobalLiveMetrics.AddChars`（**全局 mutex**，`live.go:129-136`）→ `json.Marshal`（`chat_stream_handler.go:193`）→ `fmt.Fprintf("data: %s\n\n")`（`:195`，2 次分配）→ `flusher.Flush()`（`:196`，1 次 syscall）；上游侧另有 `bufio` + `json.Unmarshal` + `ReasoningStreamAdapter`（`provider/openai.go:227-250`）。
- 另有固定成本：约 600–750KB 请求体被 `ShouldBindJSON` 反序列化（`handler.go:32`）→ 转成上游请求再 `json.Marshal`（`provider/openai.go:180`）→ 再加 `CaptureRequestSnapshot` 的全量拷贝（F4）。
- 数字：20 核 / 0.0597s = **335 req/s/副本**；若把每帧成本减半（复用 buffer、去掉逐帧全局锁），CPU 顶可到 **500–670 req/s/副本**。
- **注意**：当前**没有 pprof**（`cmd/` 未注册 `net/http/pprof`），CPU 无法归因到函数级。建议在独立 debug 端口挂 pprof 后再做一次 30s profile 确认（这是最省力的"先测量再优化"步骤）。

### F4【P1】请求体被无条件全量缓冲 + 跨流保留
- 代码：`internal/api/trace_hooks.go:24-41`
  ```go
  raw, err := c.GetRawData()                     // ① 全量读入（实测 600–750KB，上限 64MiB）
  snapshot := raw; if len > 32KB { snapshot = snapshot[:32KB] }
  c.Set(ContextKeyRequestSnapshot, string(snapshot))
  c.Request.Body = io.NopCloser(bytes.NewReader(raw))   // ② 整个 raw 存活到请求结束
  ```
- 触发：`/v1/chat/completions`（`handler.go:30`）与 `/v1/messages`（`inbound_anthropic.go:44`）**无条件调用**；而 `TraceCapture.Enabled` 默认 `mode="errors"`（`trace_capture.go:61-110`）→ 成功请求白付。实测 10min 状态码分布 `{200:540, 429:158, 502:90}`（r1）/`{200:518, 429:184, 502:38}`（r2）→ **约 68% 的成功请求 100% 白付**；`4xx/5xx` 才真正需要快照。
- 影响：每路流多 600–750KB 常驻 + 一次全量拷贝；400 并发 ≈ **240–300MB** 额外堆，GC 频率与 STW 概率上升。这是"提升并发"里唯一直接吃内存的项。
- 最小修复：用 `io.TeeReader` 在 `ShouldBindJSON` 读 body 的同时旁路截取 32KB（不再预读全量、不再把 raw 挂回 Body）；或仅当 `GATEWAY_TRACE_CAPTURE=all` 时才预读。

### F5【P1】同步 Redis 调用压在请求 goroutine 的"收尾"路径
- XADD：`async_logger.go:106-122`（`ctx 250ms`，L111），在 `recordUsage` 内（`common.go:104-126`）。
- 关键时序（流式）：`chat_stream_handler.go:104` / `:113` / `:117` / `:146` 都是**先 `recordOnce.Do(recordStreamEnd)`（内含 XADD），再写 trailer / `[DONE]` / `Flush`** → 客户端看到的**最后一个字节被日志写入延迟**：典型 +0.30ms，Redis 抖动时 **+250ms**；若 XADD 失败还要回退本地 channel（`:129-145` 再等 **500ms**）→ **最坏 +750ms**，且每次失败都打一条 WARN（日志放大）。
- 租户 quota：`auth.go:246` 与 `defer :257` 各 1 次 EVALSHA（300ms 超时）→ 流式请求的预留额度**要等流结束才释放**（正确，但意味着 per-user in-flight 随并发线性增长，balance=998.98 / 0.0005 ≈ 可容纳 200 万路，不成问题）。
- 修复：把 XADD 也走现有本地 channel（先入 10000 缓冲，worker 侧 `XADD` 后再算持久化），流式收尾只做内存入队（~100ns）；quota 改为本地计数 + 1s 周期对账。**动作最小、收益最大的"去往返"组合。**

### F6【P1】`AllowRPM` 无独立超时 + go-redis 默认重试 3 次
- 代码：`ratelimit.go:189-209` 直接传 `c.Request.Context()`（无 deadline）→ `redis_quota.go:72`；`internal/distributed/redis.go:58-62` 只设了 `DialTimeout 2s / ReadTimeout 1s / WriteTimeout 1s`，**未设 `MaxRetries`**（go-redis v9 默认 3）。
- 数字：Redis 半死时每次 `AllowRPM` 最坏 ≈ `3 × (1s + backoff)` ≈ **3–4s**，而它发生在 handler 之前 → 期间占着 1 路 FD + 1 个 goroutine。
- 修复：数据面 Redis 调用统一 `ctx, cancel := context.WithTimeout(ctx, 100ms)` + `MaxRetries: 0`（或 1）。失败已有 fail-open 逻辑（`ratelimit.go:208`），行为不变但最坏时延从 3–4s 降到 100ms。

### F7【P1】quota 预留每次 2 次 Redis 往返
- 代码：`middleware/quota_tracker.go:35-59`（acquire）与 `:63-75`（release），各带 300ms ctx；Lua 脚本 `redis_quota.go:35-61`。
- 数字：租户 key 每请求 **+0.68ms**（2×0.34ms）。不是延迟问题，是**放大问题**：Redis 抖动时 +0.6s，且 acquire 与 release 分处请求首尾，两次都可能卡。
- 修复（推荐阶梯）：① 先用本地 `atomic` 计数做快速路径，Redis 只在每 N 次或每 1s 对账一次（跨副本超发窗口 ≤1s，余额 998 元时风险可忽略）；② 或者把 acquire/release 合并进一次 pipeline。

### F8【P2】10s 全量 reload + 固定 2 行 INFO（每副本）
- 代码：`controlplane/sync.go:248-280`（`StartPeriodicSync(10s)`，`server.go:198`）→ `ReloadFromDB` `:148-180`。
- 每次 tick 的 DB 负载：`ToModelChannels`（2 查询：`repo_channels.go:320-328`）、`ToModelAPIKeys`（`ListAPIKeys` + `ListUsers`，`repo_keys.go:308-321`）、`HasRegisteredAPIKeys`、`GetModelFallbacks`、`ListModelPrices`（`engine.go:76`）、`GetSetting`（`engine.go:118`）→ **约 8 条查询 / 10s / 副本**（实测很便宜）。
- 真正的成本：① 每次 tick 都重建全部 map/slice，然后 `dispatcher.UpdateChannels` 取 `d.mu` **写锁**（`dispatcher.go:77-80`）→ 周期性阻塞所有路由读；② **2 行 INFO/10s/副本**（`sync.go:164-168`、`engine.go:133`），实测 60s 内 12 行/副本，占该窗口日志的 **41%**（12/29）；③ 2000 条环形缓冲被这些噪声 + 访问日志冲刷（高 QPS 下控制台日志视图 1s 就翻页）。
- 修复：用已有的 `airoute:cluster:config_version`（`redis.go:16`，`PublishReload` 里 `INCR`，`:111`）做变更检测——版本没变就直接 return（连查询都不发）；两条 INFO 降 Debug 或仅在 `version` 变化时打。

### F9【P2】billing 双 RLock
- 代码：`engine.go:329` `GetPriceWithGroup`（RLock `:211-212`）→ `engine.go:372` `GetGroupMultiplier`（再 RLock `:142-143`）；每成功请求执行一次，`api/common.go:61`。
- 数字：每请求 2 对 RLock/RUnlock ≈ 0.2µs（**不是瓶颈**），但 `sync.RWMutex` 在写者排队时会阻塞新读者（Go 的实现语义）→ 价格 reload（`engine.go:127-131` 写锁）期间所有计费计算短暂停顿。
- 修复：`ReloadPrices` 里构建一个不可变 `pricingSnapshot{prices, wildcards, multipliers, deflt}`，用 `atomic.Pointer` 发布；`GetPriceWithGroup` 一次 `Load()` + 无锁查找。顺手把两次查找合成一次（省一次 RLock）。

### F10【P2】`AsyncLogger` 单 worker 与背压设计
- 代码：`async_logger.go:160-234`（单 goroutine，`batchSize=100`、`flushInterval=500ms`、channel 容量 10000，`server.go:161`）。
- 常态：实测 `queue_depth 0`、`dropped 0`、`flushed 539+` → 健康。
- 故障模式：DB 慢到接近 `logWriteTimeout=10s` 时，1000 req/s 会在 10s 内填满 10000 缓冲，之后每个请求在 `:129-145` 最多阻塞 **500ms** → 吞吐断崖式下跌（并且丢弃日志）。**这是"并发能力"的隐藏单点**：一个 goroutine + 一条 DB 事务链承载全部计费证据。
- 修复：`batchSize 100 → 500`（`BatchRecordUsageLogsContext` 本来就按 25 行分块多值插入，`repo_logs_writer.go:190-283`，单批 500 只是 20 个 chunk，仍一个事务）；buffer 10000 → 50000；`Record` 满时**非阻塞丢弃并计数**而不是阻塞 500ms（当前策略是反压，宁可掉日志也不掉吞吐更合理——需产品确认）。

### F11【P2】日志串行化（2 行/请求）
- 路径：`middleware/trace.go:144-152`（http_access，INFO）+ `dispatcher_stream.go:62-66` / `dispatcher.go:215-220`（attempting streaming，INFO）。
- 成本：每条日志 = `slog` JSON handler 全局 `mu` + JSON 序列化 + **stdout 同步写（容器日志驱动）** + `LogRingBuffer.Append`（`ring_buffer.go:64-98`：全局 `mu.Lock` + 拷贝订阅者切片）+ `BroadcastHandler.Handle` **每行新建 map**（`ring_buffer.go:219-227`）。
- 数字：实测 3.2 行/请求（含 10s reload 噪声）；1000 req/s → 2000 行/s 串行；环形缓冲 2000 条在高 QPS 下 1 秒被覆盖。
- 修复：① `attempting streaming connection` / `attempting chat completion` 降 Debug（它们只在排障时需要）；② 访问日志降 Debug 或异步（`LogRingBuffer` 复用 map、容量外置）；③ 保留 WARN/ERROR 同步。

### F12【P2】HTTP Server 超时与头大小
- 代码：`cmd/airoute/server.go:217-225`；`configs/config.yaml:10-11` → `read_timeout_sec: 600`、`write_timeout_sec: 600`（默认值 `config/config.go:126-127` 是 120）。
- 问题：`ReadTimeout 600s` 意味着一个慢速（或恶意）客户端可以**占住 1 个 FD + 1 个 goroutine 10 分钟**；`MaxHeaderBytes 1MiB` × 并发 = 可被放大的内存；无 `MaxConns`。结合 F1 的 1024 FD，**慢速攻击 400 路就能把网关打死**。
- SSE 已显式清除写 deadline（`api/common.go:212-213`），所以 `WriteTimeout` 对长流无影响 → 可以安全下调到 120s。
- 修复：`ReadHeaderTimeout 15s`（已有）、`ReadTimeout 60s`、`WriteTimeout 120s`、`MaxHeaderBytes 64<<10`；SSE 行为不变。

### F13【P3】Redis Stream 消费组 consumer 泄漏（50 个）
- 代码：`async_logger.go:241-247`，`consumerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())`（L246）；停机时**从不** `XGROUP DELCONSUMER`。
- 实测：`XINFO CONSUMERS` = **50 个**，其中 48 个 `pending=0` 且 idle 数千万 ms（= 上次重启前留下）；`XINFO GROUPS` consumers 也是 50。
- 影响：每 30s 的 `XAUTOCLAIM`（`:365`）要扫整个 group 的 PEL；当前 pending=0 所以代价很小，但 consumer 表会无界增长（每次滚动发布 +2，永不回收）。
- 修复：用稳定 node id（`NODE_ID` env 已有，`telemetry/live.go:65-78`）做 consumerID，退出时 `XGROUP DELCONSUMER`。

### F14【P3】空转轮询与后台 Redis 常驻流量
- 实测（60s 增量）：`xreadgroup` **4/s**（2 副本 × 每 500ms 阻塞读，`async_logger.go:375`）、`set airoute:live:<node>` **2/s**（`distributed/live.go:52`）、`get airoute:cluster:config_version` 每 15s/副本（`redis.go:201`）。
- 影响：~6 ops/s 常驻，可忽略；但 `XREADGROUP BLOCK 500` 常驻占 2 个 Redis 连接，且 500ms 的 `Block` 让单条日志最多等 500ms 才被消费（`flushTimer` 复用同一值）。若要更低延迟可拆成两个参数（阻塞 1s，批量 500）。

### F15【P3】`usage_logs` 索引过多，写入放大在单 worker 上
- 实测：`usage_logs` **16 个索引**，其中 3 个是 GIN trigram（`idx_usage_trgm_trace_id/chat_id/session_id`），列 `id`；当前 28,749 行。
- 每次 flush 一个事务：4 个 25 行 multi-row INSERT + 每 key 一条 `UPDATE api_keys` + 每 key 一条 `UPDATE users ... WHERE id=(SELECT user_id FROM api_keys WHERE key=?)` + COMMIT/fsync（`repo_logs_writer.go:271-360`）。同 key 的所有 flush 会**串行化在 `api_keys`/`users` 行锁上**（当前单 worker 所以无感，一旦扩 worker 立刻冲突）。
- 修复：删掉未使用的 GIN 索引（trgm 只服务后台模糊搜索）；或按 `created_at` 分区；`UPDATE users` 的子查询改成 JOIN/预取 user_id。

### F16【P3】无全局并发闸门
- 代码：全仓库无 `semaphore`/`MaxConcurrent`/`InFlightLimit`（grep 仅命中连接池配置）。
- 问题：突发流量（实测 319 req/s 突发）只能靠 FD/CPU 被动限流，表现是 `too many open files` 而不是可观测的 429/503。
- 修复：加一个 `MaxInFlight = 800/副本` 的信号量中间件，超出直接 503 + `Retry-After`，把"随机 EMFILE"变成"受控背压"。

---

## 5. 锁竞争与 map 增长专项

| 锁 | 位置 | 每请求次数 | 竞争面 | 是否有界/清理 | 结论 |
|---|---|---|---|---|---|
| `Config.keyMu` (RW) | `config/config.go:57-67, 73-91` | 3 | 全局读 vs 10s 写（`UpdateAPIKeys`） | ✅ | 可忽略 |
| `UserQuotaTracker.mu` | `quota_tracker.go:51` | 0（Redis 模式下） | 仅 Redis 失败回退时 | ⚠️ **map 无清理** | 若 Redis 长期挂掉 → 全按 userID 累积（有界于用户数） |
| `RateLimiter.mu` + `tokenBucket.mu` | `ratelimit.go:64-116` | 0（Redis 模式）/ 2（本地模式） | 全局读 + 每 bucket | ✅ 10min 清理（`:118-129`） | 健康 |
| `TokenQuotaTracker.mu` + bucket | `tpm_tracker.go:71-108` | 2（Allow）+2（Remaining）+2（Consume） | **全局单锁**取 bucket | ✅ 10min 清理 | 每请求 4–6 次全局锁，量小但可合并成 1 次 |
| `BillingEngine.mu` (RW) | `engine.go:142, 211` | 2×RLock | 写=价格 reload | ✅ | 提议 atomic.Pointer（F9） |
| `telemetry.Metrics.mu` | `metrics.go:78` / `:158` | 2（RecordRequest + RecordTTFT） | **全局单锁** | ⚠️ `modelRequests` 上限 500（`:97`）✅ | 小；可用原子/环形替代 |
| `LiveMetrics.mu` | `live.go:118,133,100` | **每 SSE 帧 1 次**（流式） | **全局单锁，所有流共享** | ✅ 180 桶定长 | 高频热点，建议原子化（F3） |
| `LogRingBuffer.mu` | `ring_buffer.go:72` | 2（每行日志） | 全局 | ✅ 定长 2000 | 正常；QPS 高时是串行点 |
| `CircuitBreaker.mu` | `circuit_breaker.go:221,251,266` | 2（CanExecute + RecordSuccess） | 全局单锁（**所有 channel 共用**） | ✅ 有界于 channel 数 | 全局写锁在每请求首尾各一次，可改 shard |
| `AsyncLogger.ch` | `async_logger.go:33` | 1 | 单 worker | ✅ 10000 缓冲 | 故障型瓶颈（F10） |
| `Dispatcher.mu` / `wrrMu` / `fallbacksMu` | `dispatcher.go:18-25` | 2-3 次 RLock/Lock | 写=reload 10s | ✅ wrr 权重按活跃 channel 清理（`:87-99`） | 正常 |
| `TraceCapture.queue` | `trace_capture.go:41,123` | 仅错误/全量模式 | 1024 缓冲，满即丢 + `slog.Warn` | ✅ | 可接受 |

**map 增长结论**：`RateLimiter.buckets`、`TokenQuotaTracker.buckets`、`CircuitBreaker.breakers`、`Dispatcher.wrrWeights`、`Metrics.modelRequests`、`UserQuotaTracker.inFlightUsers` 全部有界或已清理；**唯一无界增长的是 Redis Stream consumer group 的 consumer 表（50 且每次重启 +1，F13）**。

---

## 6. 流式请求专项

| 问题 | 结论 | 依据 |
|---|---|---|
| 流式请求是否长期占用 DB 连接？ | **否**。auth 的 DB 查询在流开始前就 `cancel()` 归还连接；计费落库在 async_logger worker 里，不在请求 goroutine。 | `auth.go:153-155`（`cancel()` 后立即释放）、`common.go:104` → `async_logger.go:106` |
| 每流几个 goroutine？ | **3 个**：HTTP handler goroutine + `pipeStreamEvents`（`dispatcher_stream.go:159-160`）+ provider 读流 goroutine（`provider/openai.go:227`）；另有 2 个 64 容量 chan。 | 同上 |
| 每流几 FD？ | **2 个**（下游 + 上游），网关侧；nginx 侧同样 2 个。FD 顶见 F1。 | 实测 + `provider/openai.go:197` |
| 逐 chunk 分配/锁？ | **有**：`GlobalLiveMetrics.AddChars` 全局锁（每帧）、`json.Marshal`（每帧 1 次分配）、`fmt.Fprintf`（每帧 2 次）、`Flush`（每帧 1 syscall）、上游 `bufio`+`json.Unmarshal`+reasoning tag 扫描。 | `chat_stream_handler.go:169-197`、`live.go:129-146`、`openai.go:227-260` |
| `RecordUsage` 是否在流 flush 之后？ | **是**（在最后一帧之后、`[DONE]` 之前）：`recordOnce.Do(recordStreamEnd)` 先于 trailer/`[DONE]`/`Flush` → **同步 XADD 计入客户端可见尾延迟**。 | `chat_stream_handler.go:104,113,117,146-150` |
| 断连处理？ | `c.Request.Context().Done()` 退出循环（`:109`），defer 里 `recordOnce` 仍会记一次 499 审计（计费问题见 billing.md）。goroutine 有 `pipeStreamEvents` 的 ctx 退出保护，**无泄漏**。 | `:107-119`、`dispatcher_stream.go:190-220` |
| 每 chunk 是否有 DB 写？ | **无**，DB 写只在流结束后进 async_logger。 | `common.go:104` |

---

## 7. 优先级补丁清单

> 排序标准：**先解硬上限（P0，倍数收益），再降每请求成本（P1），再做卫生（P2/P3）**。
> "收益"基于第 1、3 节实测数字推算；"行为变化"= 是否改变对外可观测语义。

| ID | 严重度 | 动作 | 预期收益 | 风险 | 最小改动 | 行为变化 |
|---|---|---|---|---|---|---|
| **P0-1** | P0 | **提高容器 FD 上限** | 流式并发 395 → 数万路；吞吐顶 94 → **≈335 req/s/副本**（转为 CPU 受限），**集群 ~190 → ~670 req/s** | 低（仅资源上限，需确认宿主机 `fs.file-max`） | `docker-compose.yml` gateway-1/2 加 `ulimits: {nofile: 65536}`；`deploy/nginx-compose.conf` upstream 段外加 `worker_rlimit_nofile 65536;` | **无** |
| **P0-2** | P0 | **修 PG 连接预算，解锁横扩** | 让第 3 个副本可行；把 39 条 idle 占坑降下来；避免池满时 control-plane 无界阻塞 | 低-中（池变小后并发 DB 峰值需观察） | `db.go:239-240` → `SetMaxOpenConns(25)`、`SetMaxIdleConns(8)`；`max_connections` 提到 200（或上 PgBouncer）；reload 查询补 ctx 超时 | 无（需同时确认 25 够用：实测 auth 查询 0.29ms × 25 ≈ 8.6 万/s） |
| **P1-3** | P1 | **降每请求 CPU（逐帧路径）** | CPU 顶 335 → 500~670 req/s/副本（每帧成本减半） | 中（协议保真，需回归 SSE 输出） | `chat_stream_handler.go:193-197` 改 `append` 到复用 buffer + `w.Write`（去 `fmt.Fprintf`）；`live.go:129` 改原子计数/流内累加、`EndStream` 时一次落账；上游 `openai.go:231` 复用 `bufio.Reader` | 无（字节级等价需测试） |
| **P1-4** | P1 | **先去测量再优化 CPU** | 省下盲目优化 | 低 | 在独立内部端口挂 `net/http/pprof`（新增文件 10 行，不动现有路由），取 30s CPU profile 确认 F3 归因 | 无 |
| **P1-5** | P1 | **XADD 移出请求 goroutine** | 尾延迟 −0.3ms（最坏 −750ms）；Redis 抖动不再影响客户端 `[DONE]` | 中（持久化语义：worker XADD 成功后才算落账，停机需 drain） | `async_logger.go:99-112`：Redis 分支改为先入 `al.ch`（非阻塞 + 满则计数），由 worker/独立 goroutine 批量 `XADD`；`chat_stream_handler.go` 顺序不变 | 无观测变化（日志延迟 ≤500ms） |
| **P1-6** | P1 | **给数据面 Redis 调用加超时/关重试** | 最坏 +3~4s → +100ms；Redis 抖动时不再翻倍时延 | 低（已有 fail-open） | `ratelimit.go:191` 改用 `context.WithTimeout(100ms)`；`redis.go:58-62` 加 `MaxRetries: 0`（或 1） | 无（Redis 挂时仍 fail-open） |
| **P1-7** | P1 | **quota 改本地计数 + 周期对账** | 租户 key 每请求 −0.68ms（2 次往返），Redis 故障面减半 | 中（跨副本超发窗口 ≤1s） | `quota_tracker.go:35-75`：本地 atomic in-flight 快速路径，后台每 1s 与 Redis 对账 | **有**（超发窗口从 0 变 ≤1s） |
| **P1-8** | P1 | **请求快照改为旁路 32KB（不预读全量）** | 每流省 600–750KB + 一次全量拷贝；400 并发 ≈ −280MB 堆 | 低 | `trace_hooks.go:24-41` 改 `io.TeeReader` 限 32KB；`handler.go:30` / `inbound_anthropic.go:44` 调用点不变 | 无（错误快照仍保留 32KB） |
| **P2-9** | P2 | **auth 权威查询 TTL 缓存（1–2s）+ reload 广播失效** | 每请求 −0.29ms、−1 次 PG 往返；把 PG 从数据面故障域里摘除；消除"2s 超时后 fail-open"窗口 | 中（吊销延迟 ≤TTL） | `auth.go:151-198`：`sync.Map`/`ristretto` 缓存，key 为 `effectiveKey.Key`，TTL 1–2s；`ReloadAndBroadcast` 时清空（`sync.go:233-244`） | **有**（吊销/余额变更延迟 ≤2s；当前注释明确要求"下一请求可见"，需产品确认） |
| **P2-10** | P2 | **reload 加变更检测 + 日志降级** | 去掉 8 查询/10s/副本 与 41% 的日志噪声；消除周期性 `d.mu` 写锁抖动 | 低 | `sync.go:148` 开头读 `ConfigVersionKey`（`redis.go:16`），未变则 return；`sync.go:164` 与 `engine.go:133` 降 Debug 或仅变更时打 | 无（reload 语义不变，只是跳过无变化的重建） |
| **P2-11** | P2 | **billing 改 atomic.Pointer 不可变快照** | 每请求 −2 次 RLock；价格 reload 不再阻塞计费读者 | 低 | `engine.go`：`ReloadPrices` 构建 `pricingSnapshot`，`snapshot atomic.Pointer[...]`；`GetPriceWithGroup`/`GetGroupMultiplier` 各一次 `Load()` | 无 |
| **P2-12** | P2 | **日志降噪/异步** | 请求内日志 2 行 → 0-1 行；省 ~20–60µs/请求的全局锁+syscall；环形缓冲不再 1s 翻页 | 低 | `dispatcher_stream.go:62`、`dispatcher.go:215` 降 Debug；`trace.go:144` 访问日志改 Debug 或走异步队列；`ring_buffer.go:219` 复用 map | 无（排障时用 Debug 级别开回） |
| **P2-13** | P2 | **收紧 HTTP 超时/头大小** | 防慢速攻击吃满 1024 FD；FD 预算可预测 | 中（需确认无 >60s 的非流式请求） | `server.go:220-224`：ReadTimeout 60s、WriteTimeout 120s、MaxHeaderBytes 64KiB；`configs/config.yaml:10-11` 同步 | **有**（超时更严格；SSE 已清 deadline 不受影响） |
| **P2-14** | P2 | **async_logger 批量与背压策略** | 消除"DB 慢→buffer 满→每请求阻塞 500ms"的吞吐断崖 | 中（取决于是否允许丢日志） | `server.go:161`：batch 100→500、buffer 10000→50000；`async_logger.go:129-145` 满时改非阻塞丢弃+计数 | **有**（极端情况下丢审计日志而非阻塞业务） |
| **P3-15** | P3 | **consumer 用稳定 ID + 退出 DELCONSUMER** | 消费组 consumer 表 50 → 2，停止无界增长 | 低 | `async_logger.go:241-247` 用 `NODE_ID` fallback hostname；`Stop()` 里 `XGROUP DELCONSUMER` | 无 |
| **P3-16** | P3 | **加全局 in-flight 闸门** | 突发时返回可控 503 而非 EMFILE；指标可观测 | 低 | 新增 middleware：`chan struct{}` 信号量 800/副本，超限 `503 + Retry-After`；挂在 `router.go:250` 前 | **有**（新增 503 状态码） |
| **P3-17** | P3 | **清理 `usage_logs` 冗余索引** | 降低 flush 写放大，给将来多 worker 腾空间 | 中（需确认无查询依赖） | 删除 3 个 GIN trigram 索引或改 `created_at` 分区（迁移脚本，非本次改动） | 无（后台模糊搜索变慢） |
| **P3-18** | P3 | **合并 TPM tracker 的多次加锁** | −4 次全局锁/请求 | 低 | `tpm_tracker.go:87-108`：增加 `AllowAndRemaining` 单次取桶 | 无 |

### 若只能做三件事

1. **P0-1 提高 FD**（半天工作量，集群流式吞吐 ×2~3）。
2. **P0-2 PG 连接预算**（否则永远只能跑 2 个副本，横扩无路）。
3. **P1-5 + P1-6 + P1-7 去掉请求 goroutine 里的 Redis 往返**（正常时省 ~1.4ms/请求，异常时把"时延翻倍"变回"几乎无感"）。

---

## 8. 附录：复现命令（全部只读）

```bash
# CPU（ticks: utime+stime）
docker exec airoute-1 sh -c 'awk "{print \$14+\$15}" /proc/1/stat'

# FD 上限与占用
docker exec airoute-1 sh -c 'ulimit -n; ls /proc/1/fd | wc -l; ls -l /proc/1/fd | grep -c socket'

# Redis 服务端单命令耗时 / 速率
docker exec airoute-redis redis-cli INFO commandstats | grep -E 'eval|xadd|xreadgroup'
docker exec airoute-redis redis-cli --latency                 # 需在业务网络内执行
docker exec airoute-redis redis-cli XINFO GROUPS airoute:stream:logs
docker exec airoute-redis redis-cli XINFO CONSUMERS airoute:stream:logs airoute_log_group

# Redis 实际调用序列（短时只读观测）
timeout 25 docker exec airoute-redis redis-cli MONITOR

# Postgres 连接与查询代价
docker exec -e PGPASSWORD="$POSTGRES_PASSWORD" airoute-postgres psql -U airoute -d airoute \
  -tAc "SHOW max_connections; SHOW superuser_reserved_connections; SELECT state,count(*) FROM pg_stat_activity GROUP BY state;"
# auth 查询 p50/mean（10 次）
docker run --rm --network airoute_default -e PGPASSWORD="$POSTGRES_PASSWORD" postgres:16-alpine \
  psql -h postgres -U airoute -d airoute -c '\timing on' \
  -c "SELECT k.id FROM api_keys k LEFT JOIN users u ON (k.user_id>0 AND u.id=k.user_id) OR (COALESCE(k.user_id,0)=0 AND k.tenant_id!='' AND u.username=k.tenant_id) WHERE k.key='<key>';"

# 逐行访问日志聚合（状态码/路径/每秒峰值/日志行数）
docker logs --since 6h airoute-1 2>&1 | grep '"http_access"'

# 直连单副本指标（绕过 LB 轮询）
for r in airoute-1 airoute-2; do docker exec $r curl -s http://127.0.0.1:8080/metrics; done
```

---

## 9. 与其他审计的关系

- **billing.md**：本报告只关心"并发与热路径代价"；计费正确性（幂等、499 零计费、透支）以 billing.md 为准。二者在 F5（同步 XADD）与 F10（async_logger 背压）上有交集：**去同步化不能牺牲"日志写入成功才算收费"这一前提**。
- **logs.md**：日志噪声与 10:09–10:11 的 429/502 突发以 logs.md 为准；本报告只补充"该突发是上游快速失败，不能用来评估流式并发能力"这一结论。
