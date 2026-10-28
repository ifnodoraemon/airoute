# Airoute 扣费（计费 / 结算）在高并发下的现状与优化方案

> 审计日期：2026-10-28
> 范围：数据面 `/v1/*`、`/v1beta/*`、MCP 的**扣费链路**——准入额度、计价、结算落库、审计落库、对账。
> 证据来源：① 当前工作树源码；② 线上只读快照（`airoute-1/2` 指标 + Postgres 16 `pg_stat_*` / `pg_indexes` / `EXPLAIN`，未写入任何数据）；③ 本地并发基准（SQLite WAL，临时探针已删除）。
> 与前序审计的关系：`docs/audit/billing.md` 覆盖**正确性**（幂等、外币、对账口径），`docs/audit/concurrency.md` 覆盖**基础设施**（FD、CPU、Redis 往返），仅 F15 顺带提到"同 key 串行化"。本文补上缺失的那一层：**扣费写入路径自身的吞吐与额度窗口**。

---

## 0. 结论速览

| # | 结论 | 关键证据 | 级别 |
|---|------|----------|------|
| 1 | **并发安全 ≠ 并发容量。** 幂等键、整数账本、钱先账后、批量兜底都做对了；但结算是"**每请求一个独立写事务**"，热点 key/user 上只能串行。 | `internal/storage/settlement.go:68-159`；实测单条结算 0.29 ms，吞吐 ~2.8–3.5k/s 且**不随并发增长**（G=128 仍是 2.4–3.5k/s，p99 165–234 ms） | **P0** |
| 2 | **最大的资金风险不在结算事务，而在"准入→结算"之间的额度窗口**：准入只判断 `balance > 0`，在途预留仅 **0.0005 元/请求**，比真实单请求成本小 **2–3 个数量级**。 | `middleware/authz.go:109`、`middleware/quota_tracker.go:20`；线上 key 18：`rpm=600`、余额 906 元，一次突发可放行数百请求 | **P0** |
| 3 | 结算跑在**请求 goroutine 内**：unary 在 `c.JSON` 之前、流式在 `[DONE]` 之前，1.5 s 超时会直接变成用户可见尾延迟。 | `internal/api/common.go:312-320`、`internal/api/handler.go:154`、`chat_stream_handler.go:144-145` | **P1** |
| 4 | 结算与鉴权共用同一个连接池，且 PG **无 `lock_timeout`**（实测 `lock_timeout=0`，应用只设 `statement_timeout=30s`）：热点行等待会占住池槽 → 鉴权 2 s 超时 **fail-open 放行** → 结算排队 → deferred。 | `internal/storage/db.go:221,283-296`；`internal/middleware/auth.go:210-216` | **P0** |
| 5 | **批量化是唯一被实测验证的大杠杆**：同一串行写者下，100 条合并 1 个事务 → **19.4k 条/s（≈6.1× 对 K=1 基线）**；500 条 → 31.2k/s（≈9.9×）。现有 `settleCharges` 本来就支持切片，缺的只是"请求侧攒批"。 | 本地基准 Scenario C；`settlement.go:68` | **P0** |
| 6 | 线上写放大可量化：`api_keys` 更新 **60,535 次 / 62,718 条结算 ≈ 1:1**；审计表 `usage_logs` 19 个索引、索引空间 160 MB，其中 **6 个 trigram 索引共 ~97 MB，几乎 0 次扫描**；结算账本无 `api_key` 索引、无保留策略。 | 线上 `pg_stat_user_tables` / `pg_stat_user_indexes` | **P1** |
| 7 | 批量路径（异步兜底 / 未来的 group commit）里 `map` 遍历**加锁顺序随机**，多 key 批次存在 PG 死锁回滚风险。 | `settlement.go:236-288` | **P1** |
| 8 | 透支 floor（-1 元）把"账面债务"钳死了，但**已交付未收的钱不可见**：ledger 记全额、钱包少扣，而 `ledgerDrift` 比的是 `api_keys` 与 ledger（两者都是全额）→ 漂移恒为 0。 | `wallet_deduct.go:20,40`；`billing_reconcile_job.go:199-215` | **P2** |

**一句话**：正确性防线已经不错，但吞吐模型是"每笔一次数据库事务"，资金模型是"余额>0 就放行"。高并发下先崩的不是账，是**尾延迟**和**额度敞口**；优化顺序应是 **① 预授权（防敞口）→ ② 结算攒批（提吞吐/降尾延迟）→ ③ 隔离连接池 + 锁超时（防雪崩）→ ④ 事务与索引瘦身（降放大）**。

---

## 1. 现在高并发时是怎么处理的（逐跳拆解）

### 1.1 准入：余额只判正负，预留额度是常数

`AuthMiddleware` 每个数据面请求都会做（`internal/middleware/auth.go`）：

1. **内存快照**匹配 key（`lookupSnapshotKey`，10 s 一次的 reload 快照）；
2. 未命中时回落存储（`lookupStorageCredential`）；
3. **每请求一次权威 DB 查询** `GetAPIKeyByKeyContext`（`api_keys LEFT JOIN users`，带 2 s 超时）——拿到最新的 `users.balance`、`api_keys.used_cost_units`，因此余额与吊销是"下一请求可见"的；
4. 预算 / 状态 / 账户检查：`AccountDenial` **只判断 `userBalance <= 0`**（`authz.go:109`），不判断"够不够本次请求"；
5. 在途预留 `ReserveUserQuota`：Redis Lua 原子地 `INCR` 一个**计数器**，条件为 `balance - (inFlight+1) * minReserveCost >= 0`，而 `minReserveCost = 0.0005` 元（`quota_tracker.go:20`）；Redis 不可用则退回本地 `map` 计数；请求结束 `DECR`。

```text
并发保护实际强度 = (余额 > 0) + (每请求预留 0.0005 元) + (per-key RPM/TPM)
```

对照线上真实成本：10-22 日均 0.010 元/请求、10-28 日 0.029 元/请求，而长上下文单请求可达 **0.37 元**（18.6 万 prompt × 2 元/1M）。**预留值比真实成本小 20–700 倍。**

### 1.2 计价：纯内存，几乎不是瓶颈

`internal/billing/engine.go`：

- 价目表在 `RWMutex` 保护下，每 10 s 全量 reload（`ReloadPrices`，`:84-158`）；通配符按最长前缀预排序，避免 map 随机序。
- 计价分四段：未命中 prompt / cache read / cache write（1.25× 溢价）/ completion，再乘组倍率、错峰折扣，`roundPrecision` 到 1e-8，产出 `UsagePricingSnapshot`（`:402-501`）。
- 每次计价 2 次 `RLock` + map 查找，**亚微秒级**，`concurrency.md` 已确认不是瓶颈（F9 建议改 `atomic.Pointer` 只是锦上添花）。

### 1.3 结算：请求 goroutine 内的 6 次往返事务

`RecordUsage`（`internal/api/common.go:163-363`）在**返回响应之前**调用：

```go
// common.go:312-320
if record.Cost > 0 || record.TotalTokens > 0 {
    settleCtx, cancelSettle := context.WithTimeout(context.Background(), settlementTimeout) // 1.5s
    _ = storage.SettleChargeNow(settleCtx, record)
    cancelSettle()
}
```

`SettleChargeNow → settleCharges`（`internal/storage/settlement.go:36-159`）的 SQL 形状：

| # | 语句 | 作用 | 锁 / 代价 |
|---|------|------|-----------|
| 1 | `BEGIN` | 开事务 | 占 1 个池连接 |
| 2 | `INSERT INTO billing_settlements (...) ON CONFLICT (billing_id) DO NOTHING RETURNING billing_id` | **幂等闸门** + 结算账本 | 唯一索引插入；PG 上 `RETURNING` 只回新插入行 → 天然防重复扣款 |
| 3 | `UPDATE api_keys SET used_cost=..., used_cost_units=..., used_tokens=... WHERE key=?` | 计数器 | **行锁，持有到 COMMIT**；热点 key 在这里串行 |
| 4 | `SELECT id FROM users WHERE ... FOR UPDATE`（仅 PG，`lockWalletUserRows`） | 锁钱包行 | **额外 1 次往返 + 行锁**（见 P1-5，实际冗余） |
| 5 | `UPDATE users SET balance_units = CASE ... , balance = (CASE ...)/1e9 WHERE ... RETURNING balance_units` | 扣款 | **行锁**；单语句算新值 + 透支 floor，避免读改写丢失更新 |
| 6 | `COMMIT` | 提交 | `synchronous_commit=on`（实测）→ **1 次 WAL fsync** |

失败时**不重试、不阻塞**：记 `billing_settlements_deferred_total`，记录继续走审计管道，由异步批量路径补结算（`:341-351`）。

### 1.4 审计落库：异步批量（已经是"批处理"的正面例子）

`AsyncLogger`（`internal/storage/async_logger.go`）：

- 有 Redis → 同步 `XADD`（250 ms 超时，失败回落本地 channel）；无 Redis → 本地 channel（10000 缓冲，满则 500 ms 反压后 spill 到磁盘）。
- worker 攒 **100 条 / 500 ms** 一次 flush（`:209-280`）。
- `BatchRecordUsageLogsContext` **先钱后账**（`repo_logs_writer.go:117-131`）：先 `settleCharges(batch)`（已同步结算的记录 `Settled=true` 直接跳过），再 25 行/语句插 `usage_logs`。
- 失败 5 次 → spill 文件；Redis 侧有 PEL + AutoClaim；滚动发布期间 `Settled` 标记不落 Redis 但幂等键会保证只扣一次。

### 1.5 对账：6 小时一次，只报不改

`BillingReconciler`（`billing_reconcile_job.go`）每 6 h（`GATEWAY_BILLING_RECONCILE_HOURS`）：

- `repairSettlementGap`：给"有审计行、无账本行"的记录补账本（滚动发布遗留）；
- `ledgerDrift`：`api_keys.used_cost_units` vs `SUM(billing_settlements.cost_units)`；
- `walletMirrorDrift`：`users.balance_units` vs 浮点镜像；
- `auditLag`：`SUM(settlements) - (SUM(usage_logs) + pruned_units)`；
- `ledgerLinkage`：已结算但找不到审计行的金额。

### 1.6 一张图（unary 请求）

```text
client ──► Auth ──────────────────────────────────────────────┐
           │  · 快照匹配 (内存)                                  │
           │  · 权威读: SELECT api_keys JOIN users  ← 1 RTT     │
           │  · balance > 0 ?  (不判断够不够本单)                │
           │  · Redis Lua 预留 0.0005 元  ← 1 RTT               │
           ▼                                                     │
        Dispatcher ──► upstream（LLM 数秒）                      │
           │                                                     │
           ▼                                                     │
        RecordUsage                                              │
           │  · 计价 (内存, <1µs)                                 │
           │  · 同步结算: BEGIN/INSERT/UPDATE k/UPDATE u/COMMIT   │ ← 6 RTT + fsync
           │    (1.5s 超时 → deferred)                            │
           │  · AsyncLogger.Record (本地 channel 或 XADD 0.3ms)   │
           │  · 写 X-Airoute-Cost 头                              │
           ▼                                                     │
        c.JSON(resp)  ──► client（客户端看到的延迟包含上面全部）  │
                                                                 │
        worker: 100 条/500ms → 先 settle(幂等跳过) 再插审计行     │
        reconcile: 6h 一次，只读对账                             ┘
```

---

## 2. 实测：并发下到底会发生什么

### 2.1 线上只读快照（2026-10-28 11:40 左右）

| 项 | 实测值 | 说明 |
|---|---|---|
| `usage_logs` / `billing_settlements` | **63,154 / 62,717** 行 | 账本与审计近乎 1:1 |
| 数据规模 | usage_logs **250 MB**（heap 90 MB + 索引 160 MB）；settlements 29 MB | 审计表 ~4 KB/行（含 19 个索引） |
| `api_keys` / `users` 更新次数 | **60,535 / 24,919**（HOT 更新 99.9%） | 每笔结算 ≈ 1 次 key 行更新；钱包行仅 4 个用户，且多为 admin 豁免 |
| 事务量 | `xact_commit=1,542,408`、`rollback=29,411`、`deadlocks=0` | 结算事务目前只占很小一部分 |
| PG 参数 | `synchronous_commit=on`、`fsync=on`、`wal_sync_method=fdatasync`、`max_connections=200`、**`lock_timeout=0`** | 每次结算 COMMIT 一次 fsync；等锁没有上限 |
| 应用连接池 | `GATEWAY_DB_MAX_OPEN_CONNS=25` / idle 8，2 副本 | 结算事务与鉴权读共用 |
| 索引 | `billing_settlements` 只有 `(billing_id)`、`(created_at)`；**无 `api_key`** | 对账/台账按 key 聚合要全表扫 |
| 审计表索引热点 | 6 个 trigram 索引共 **~97 MB**（5 个 `idx_scan` 0–4 次，`error_message` 26 次） | 写入放大 6 个索引，查询几乎不用 |
| 真实流量构成 | 37 个 key，绝大多数是 **admin（钱包豁免）**；有扣款的 user 只有 1 个，余额 906 元 | **真正的"扣钱包"并发还没被生产验证过** |
| 结算指标 | `settled_synchronously_total=21/20`、`deferred=0`、`overdrafts=0`、`ledger_drift=0`、`queue_depth=0` | 当前负载下链路健康 |

> 结论：**当前生产负载（日均千级请求、几乎全 admin）完全没有压到这条路径。** 下面的并发结论来自本地基准 + 代码路径，不是线上故障。

### 2.2 本地并发基准（SQLite WAL，`MaxOpenConns=1`，与单机部署同构）

用仓库真实 `Repository.SettleUsageCharge` / `settleCharges` 压测（文件型 SQLite + WAL + `busy_timeout=5000`，确认 `journal_mode=wal`），每记录 `billing_id` 唯一、cost=0.002 元、CNY 快照。共 5 次独立运行，吞吐波动 ±20%；下表为一次代表性运行。

**Scenario A：热点账户**（所有 goroutine 同一 user + 同一 key）

| G | records/s | p50 | p95 | p99 | max |
|---|---|---|---|---|---|
| 1 | 2,894 | 0.14 ms | 0.46 ms | 8.5 ms | 19.6 ms |
| 8 | 2,713 | 1.07 ms | 16.9 ms | 33.4 ms | 41.2 ms |
| 32 | 2,750 | 4.49 ms | 42.1 ms | 54.5 ms | 102 ms |
| 128 | 2,682 | 35.8 ms | 132.4 ms | **217 ms** | 428 ms |

**Scenario B：分片账户**（每 goroutine 独立 user + key，128 用户/128 key）

| G | records/s | p50 | p99 |
|---|---|---|---|
| 1 | 2,523 | 0.15 ms | 9.1 ms |
| 8 | 2,610 | 1.15 ms | 32.2 ms |
| 32 | 2,521 | 4.90 ms | 59.7 ms |
| 128 | 2,688 | 34.8 ms | 213 ms |

**Scenario C：合并事务**（K 条结算 / 1 个事务）

| K | records/s | 提升（对 K=1） |
|---|---|---|
| 1 | 3,160 | 1× |
| 10 | 10,235 | **3.2×** |
| 100 | 19,396 | **6.1×** |
| 500 | 31,212 | **9.9×** |
| 100（G=8） | 17,102 | 5.4× |

**探针：单条结算的成本分解**（G=1，per-op mean）

| 探针 | 吞吐 | 单次 mean | 说明 |
|---|---|---|---|
| 空事务（BEGIN+COMMIT） | 389,232/s | 0.002 ms | 事务机制本身可忽略 |
| `SELECT 1` | 284,882/s | 0.003 ms | 连接/语句开销可忽略 |
| 只插账本行 | 6,429/s | 0.155 ms | 唯一索引 + WAL 写 |
| 只更新 key + 钱包 | 6,980/s | 0.143 ms | 两行 UPDATE |
| **完整结算** | **3,267/s** | **0.305 ms** | ≈ 上面两项之和（实测 3.27k vs 理论 3.35k） |

**WAL checkpoint 探针（E）**：同一热账户负载下把 `wal_autocheckpoint=0` 后，G=1 从 2,894 → **4,621 rec/s（+60%）**，p99 从 **8.46 ms → 0.96 ms**，max 19.6 → 5.0 ms；G=32 也从 3,241 → 4,887 rec/s。**单写者的尾延迟很大一部分来自每 ~300 条结算一次的自动 checkpoint 停顿。**

**连接池探针（D）**：同一文件 DB 上 `pool=8` vs `pool=1`，4 组配对运行平均 3,206 vs 3,151 rec/s（**无一致收益**），且 max 延迟更差（204–437 ms vs 58–79 ms）—— 单写者锁把信号量换成了 SQLite 的 busy wait。

**完整性校验**：50,000 条记录 → `billing_settlements` 50,000 行、`SUM(cost_units)` 与钱包扣减完全一致（无重复、无丢失）。

### 2.3 怎么读这些数字

1. **吞吐是平的，延迟是线性涨的** → 典型的单写者排队（Little's Law：并发 ↑ 而服务率不变 → 排队时间 ∝ 并发；p50 从 0.14 ms 涨到 35.8 ms ≈ 256×）。
2. **分片账户没有改善**（G=128：2,682 vs 2,688）→ 在"每请求一事务 + 单写者"模型下，瓶颈是**事务数量本身**，不是行锁。PG 上才会叠加第二层行锁串行。
3. **~98% 的成本是写语句 + 提交**，不是事务机制：账本插入 0.155 ms + 两行更新 0.143 ms ≈ 完整结算 0.305 ms。
4. **攒批是唯一有量级收益的手段**：K=100 → 6.1×，K=500 → 9.9×；K=100 且在 8 并发下仍有 5.4×。
5. **PostgreSQL 的账要这样算（推算，未在线压测）**：6 次往返 × 实测 RTT 0.28 ms ≈ 1.7 ms，加 `synchronous_commit=on` 的 fsync ≈ 0.2–1 ms → 单事务 **≈2 ms**；连接池 25 → 全库理论上限 ~12k 事务/s；但**同一 key 被行锁钳成约 1/(持锁时间) ≈ 500–1000 次/s**。跨 key 的并发可以让 PG 合并 WAL flush，所以"全局"比"单 key"宽松一个数量级。
6. 线上 `api_keys` 更新 : 结算 = **1:1** 证实"每笔一次 key 行更新"；`usage_logs` 的 ~97 MB trigram 索引证实审计侧写放大。
7. **本基準的边界**：只跑 `settleCharges` 本身，不含鉴权读、审计插入、Redis；在单机 SQLite 模式下这些**共用同一条连接**，因此真实容量低于上表。PG 的行锁行为（分片应变快、热点账户串行在 `users` 行）未实测，需要用方案 B/C 的预发压测确认。

---

## 3. 问题清单

### P0-1【资金风险】准入只判 `balance > 0`，"在途预留"形同虚设

- **现象**：`AccountDenial` 只拦 `balance <= 0`（`authz.go:109`）；`ReserveUserQuota` 每请求只预留 **0.0005 元**（`quota_tracker.go:20`），而长上下文单请求成本可达 0.37 元。
- **后果**：一次突发里所有请求都在"余额还是正的"那一刻通过准入；结算随后才把余额打下去。可交付的服务金额 ≈ `并发数 × 单请求成本`，与余额无关。
  - 例（按长上下文请求 0.37 元/单）：余额 10 元、`rpm=600` 的 key，600 并发 × 0.37 元 = **222 元**服务被放行 → 可收 10 元（+ floor 1 元），**未收 ~211 元**；若按线上日均 0.03 元/单，则是 18 元服务 / 10 元余额，未收 ~9 元。
  - `GATEWAY_MAX_OVERDRAFT_CNY=1.0` 只把**账面余额**钳在 -1 元，**不追回已交付的服务**，所以它限的是"账"，不是"敞口"。
- **为什么现有保护不够**：RPM/TPM 限的是速率不是金额；余额是"结算后"的快照；预留量级差 2–3 个数量级。
- **位置**：`internal/middleware/auth.go:279-323`、`authz.go:98-118`、`quota_tracker.go:19-22,35-59`、`distributed/redis_quota.go:35-61,87-103`。

### P0-2【吞吐/延迟】每请求一个写事务 + 热点行锁

- 每条收费 = 1 个事务（6 往返 + 1 fsync），行锁从第 3 条语句持到 COMMIT。
- 同一 API key 是**最强热点**（一个应用通常一个 key）：`api_keys` 行锁把所有请求串成一条线；PG 上再加 `users` 钱包行锁。
- 实测：SQLite 下 128 并发 p99 **234 ms** 且吞吐不涨；PG 推算单 key **500–1000/s**、p99 随并发线性上升；突发时会大量触发 1.5 s 超时 → deferred → 同步路径白做一遍。
- **位置**：`settlement.go:68-159`、`common.go:312-320`、线上 `api_keys` 更新 1:1 证据。

### P0-3【雪崩】结算与鉴权共用连接池，且没有 `lock_timeout`

- 结算事务在等行锁时**一直占着池里的连接**（客户端 1.5 s 才 cancel）；25 个连接可以被同一个热点 key 全部占满。
- 池满后，鉴权查询 `GetAPIKeyByKeyContext` 在 2 s 超时后 **fail-open**（`auth.go:210-216`）：继续用快照放行——这既放大风险，又让"数据库稍微慢一点"变成"计费与鉴权同时失真"。更糟的边界是快照里没有用户字段时 `AccountDenial` 对 `userStatus == ""` 直接返回 nil（`authz.go:99-101`），**余额检查被整体跳过**，在途预留也不会执行（`auth.go:311` 要求 `userStatus != ""`）。
- 服务端 `lock_timeout=0`（实测）、`statement_timeout=30s`：等锁最长 30 s，远大于客户端的 1.5 s，取消后发现连接可能已不可用 → 池抖动。
- **位置**：`db.go:221,283-296`；`auth.go:210-216`。

### P1-4【尾延迟】结算在响应路径上

- unary：`RecordUsage` 在 `c.JSON` 之前（`handler.go:154,166`）；流式：`recordStreamEnd`（内含 `XADD` 与结算）在 `[DONE]`/trailer 之前（`chat_stream_handler.go:144-145,194-195`）。
- 正常时 +2–3 ms；高并发时 +几十~1500 ms；Redis 抖动时另有 `XADD` 的 250/500 ms（`concurrency.md` F5 已记录）。
- 用户感知的是"最后一帧延迟"，对 SSE 体验最敏感。

### P1-5【事务冗余】`SELECT ... FOR UPDATE` 是多余的一次往返

`deductWallet` 先 `lockWalletUserRows`（PG 上 `SELECT id FROM users ... FOR UPDATE`，`wallet_deduct.go:122-142`），再执行一条自带 `RETURNING` 的原子 `UPDATE`。**`UPDATE` 本身就会原子地取行锁并基于当前值计算**，而代码并不读取"旧余额"——这次 `FOR UPDATE` 只多花 1 个 RTT、多拿一次锁、并引入锁顺序问题。

### P1-6【死锁】批量路径的加锁顺序不确定

`applyChargeAggregates` 先遍历 `keyCost`（map）更新 `api_keys`，再遍历同一个 map 扣钱包（`settlement.go:236-288`）。**Go 的 map 遍历顺序随机**：两个批次若含 key A/B 且顺序相反，在 PG 上互相等对方已持有的行锁 → 死锁（PG 会检测并回滚一个，代价是整批重试）。
当前"每请求一条记录"时不触发；**但异步兜底批次（最多 100 条，多 key）以及未来的 group commit 会直接踩上**。

### P1-7【写放大】审计表 19 个索引、~97 MB 是几乎不用的 trigram

- `usage_logs` 250 MB 中索引 160 MB；6 个 trigram 索引共 ~97 MB，`idx_scan` 分别 0/4/0/2/0/26 次。
- 每条审计行插入 = 19 个索引条目 → 与结算争 WAL/IO；这也是"为什么结算涨不上去"的隐性因素（同一个 DB、同一批磁盘）。
- 线上 `xact_commit` 154 万 / 10 天，说明 DB 大多数事务其实是别的路径；但审计写是**每请求都有**的持续负载。

### P1-8【规模】对账缺索引 + 账本无界

- `billing_settlements` 只有 `billing_id`、`created_at` 索引；`ledgerDrift` 的 `GROUP BY api_key` 与 `auditLag` 的两个全表 `SUM` 每 6 h 跑一次，随表线性变慢（现在 62.7k 行没事，1000 万行就是分钟级）。
- 账本没有保留/归档策略（审计行有 365 天保留），只增不减；账本是资金凭证，不能简单删，需要**分区或归档**。

### P2-9【可观测性】没有结算延迟指标

现有指标只有计数器（`settled_synchronously_total`、`settlements_deferred_total`、`overdrafts`），**没有结算耗时直方图、批次大小、排队深度、锁超时计数**。并发问题发生时只能看到"deferred 变多"，看不到"慢在哪"。

### P2-10【资金口径】floor 钳制掉的应收不可见

`walletUnitsExpr` 在 `balance - cost < floor` 时直接写成 `floor`（`wallet_deduct.go:40`）：账面债务被限制在 -1 元，但**少收的那部分金额既不入账、不入指标、也不进对账**（ledger 记全额、api_keys 记全额 → `ledgerDrift=0`）。只有 `billing_overdrafts_total` 记了"发生过几次"，没记"少收多少"。风控和财务都无法回答"这个月因为透支放行了多少钱"。

### P2-11【崩溃窗口】先结算、后入队

`common.go` 先 `SettleChargeNow`，成功后才 `GlobalAsyncLogger.Record`。若进程在两者之间崩溃：**账本有、审计行没有** → `ledgerLinkage` 会告警，但没有自动修复方向（`repairSettlementGap` 只补"审计→账本"）。钱的损失为 0，但财务凭证缺失。
把顺序反过来（先入队、再结算）更稳：审计行幂等（`billing_id`），重放时会补结算，两个方向都能自愈。

### P2-12【单机部署】SQLite `MaxOpenConns(1)` 让读也串行

`db.go:340` 把 SQLite 连接池设为 1：结算事务、鉴权读、审计 flush **全部排队在一条连接上**。WAL 本可"1 写多读"，现在的配置拿不到读并发。压测也显示：加连接不提升"写吞吐"，但能明显降低排队（p50 4.1→2.4 ms）。另外 **WAL 自动 checkpoint 的停顿直接吃掉了 40% 的单写者预算**（见方案 F）。

---

## 4. 优化方案

> 排序标准：先补资金敞口，再提吞吐，再防雪崩，最后做规模与卫生。每项都给出改动点、预期收益与风险。

### 方案 A【P0】预授权：把"余额 > 0"改成"余额 ≥ 在途预估之和"

**改动点**

1. `quota_tracker.go`：把计数型预留改为**金额型预留**（单位 1e-9，与账本一致）：

```lua
-- reserve.lua（替换 redis_quota.go 的 inFlightQuotaAcquireLua）
local key   = KEYS[1]
local bal   = tonumber(ARGV[1])   -- 准入时读到的 balance_units
local need  = tonumber(ARGV[2])   -- 本次预估最大费用 units
local ttl   = tonumber(ARGV[3])
local inflight = tonumber(redis.call('GET', key) or '0')
if bal - inflight - need < 0 then return 0 end
redis.call('INCRBY', key, need)
redis.call('EXPIRE', key, ttl)
return 1
```

释放时用实际结算金额 `INCRBY -actual`（不足则 `DEL`），TTL 120 s 自愈。

2. 估算来源：
   - auth 阶段请求体还没解析，先用**可配置的粗预留**（`GATEWAY_INFLIGHT_RESERVE_CNY`，建议 0.05 = 5 分）；
   - 解析出 model 与 `max_tokens` 后，在 dispatch 前用计价引擎算 `estCost`，把差额补进预留（top-up）；结算后用实际值释放。
   - 无 `max_tokens` 的请求按"服务端单请求最大成本上限"预留（新增 `GATEWAY_MAX_REQUEST_COST_CNY`，超过直接 402）。
3. 失败语义：预留不足 → `402 insufficient_quota`（复用现有文案），比"先服务后白送"正确。

**预期收益**：把突发敞口从 `并发 × 单请求成本` 降到 `余额 + 1 元`；按上面的例子，单账户单次突发少损失 ~200 元。**这是本次审计里最该先做的资金修复。**

**风险**：估算偏高会误拒（用"预热期 + TTL + 实际值释放"缓解）；Redis 挂掉时退回本地金额计数（已有骨架）。

### 方案 B【P0】结算攒批（group commit）：每 N 笔 1 个事务

**思路**：`settleCharges` 本来就接收 `[]*UsageLogRecord` 并已按 key/user 聚合（`settlement.go:68-159`），只需要在请求侧加一个"微批协调器"：

```go
// 示意：internal/storage/settlement_batcher.go
type settleJob struct {
    rec  *UsageLogRecord
    done chan error
}

type SettlementBatcher struct {
    repo      *Repository
    ch        chan settleJob     // 容量 8192
    maxBatch  int                // 100
    linger    time.Duration      // 2ms
}

func (b *SettlementBatcher) run() {
    ticker := time.NewTicker(b.linger)
    jobs := make([]settleJob, 0, b.maxBatch)
    flush := func() {
        if len(jobs) == 0 { return }
        logs := make([]*UsageLogRecord, 0, len(jobs))
        for _, j := range jobs { logs = append(logs, j.rec) }
        ctx, cancel := context.WithTimeout(context.Background(), settlementTimeout)
        err := b.repo.SettleUsageCharges(ctx, logs)   // ← 现有 settleCharges
        cancel()
        for _, j := range jobs { j.done <- err }
        jobs = jobs[:0]
    }
    for {
        select {
        case j := <-b.ch:
            jobs = append(jobs, j)
            if len(jobs) >= b.maxBatch { flush() }
        case <-ticker.C:
            flush()
        }
    }
}
```

请求侧（`common.go`）：

```go
// 先入队审计（durable，见方案 H），再等这批的结算结果；超时/失败则交给异步管道
if record.Cost > 0 || record.TotalTokens > 0 {
    if storage.GlobalAsyncLogger != nil { storage.GlobalAsyncLogger.Record(record) } // 先置 durable
    settleCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
    _ = storage.SettleChargeBatch(settleCtx, record) // 入批等结果，超时不阻塞
    cancel()
}
```

**预期收益**（实测）：K=10 → 3.2×；K=100 → **6.1×**；K=500 → 9.9×。PG 上还会额外减少 COMMIT/fsync 次数，收益更大。尾延迟从"1.5 s 超时"降到"linger（2 ms）+ 一次批事务"。
**风险**：
- 崩溃窗口：已入队但未 flush 的收费记录——**先用审计管道兜底**（本方案已调整为先 `Record`），崩溃后 Redis/PEL/spill 重放会补结算；
- 批次内某条记录失败会拖累整批（现有 `settleCharges` 对"确定性错误"逐条隔离，见 `rejectUnbillableCharge`），瞬时错误整批重试，可接受；
- 必须配合方案 D-3 的**稳定加锁顺序**，否则批量多 key 会死锁。

### 方案 C【P0】结算独立连接池 + `lock_timeout` + 快速失败

1. `OpenDB` 增加"结算专用池"（同一 DSN，`SetMaxOpenConns(8)`，或按 2 副本 × 8 ≤ 200 预算），结算/对账只走它；
2. DSN 加 `-c lock_timeout=500`（PG）：等锁 >500 ms 直接失败 → deferred，**不占着连接死等**；
3. 结算事务用 `SET LOCAL statement_timeout='2s'`，外层再包 1.5 s ctx；
4. 对 `40P01`（死锁）/`55P03`（锁不可用）做**一次带抖动重试**，其余错误直接 deferred。

**预期收益**：热点 key 不再饿死鉴权读；避免"DB 慢 → 连接池满 → 鉴权 fail-open → 风险放大"的级联；最坏时延可预测。
**风险**：池变小后需要观察并发峰值；`lock_timeout` 对长事务敏感（结算都是短事务，安全）。

### 方案 D【P1】事务瘦身（把 6 次往返压到 3–4 次）

1. **删掉冗余的 `lockWalletUserRows`**（`wallet_deduct.go:74,122-142`）：`UPDATE ... RETURNING` 已原子，省 1 RTT + 1 次锁。
2. **PG 用 data-modifying CTE 合并 key + 钱包更新**（SQLite 分支保留两条语句）：

```sql
WITH k AS (
  UPDATE api_keys SET used_cost = used_cost + $1,
         used_cost_units = COALESCE(used_cost_units,0) + $2,
         used_tokens = used_tokens + $3, updated_at = CURRENT_TIMESTAMP
  WHERE key = $4 RETURNING user_id
)
UPDATE users u
   SET balance_units = CASE WHEN COALESCE(u.balance_units,0) - $5 < $6 THEN $6 ELSE COALESCE(u.balance_units,0) - $5 END,
       balance = (...) / 1000000000.0,
       updated_at = CURRENT_TIMESTAMP
  FROM k
 WHERE u.id = k.user_id AND u.role <> 'admin'
RETURNING u.balance_units;
```

3. **批量路径按 key/user 排序后再加锁**（确定性顺序，消除死锁）。
4. **把 `used_cost` 计数器拆出结算事务**（可选）：它只服务于预算检查与展示，可每 100 ms 用内存计数批量 `UPDATE`；预算检查容忍 100 ms 滞后。这样结算事务只保留"账本行 + 钱包行"两件事。
5. 顺手把 `api_keys` 的 `used_cost` 更新合并到批（group commit 下天然每 key 每批 1 次）。

**预期收益**：单事务 RTT 6 → 3–4（PG 上约 -30~40% 事务时长）；单 key 上限同步提高。

### 方案 E【P1】索引与对账规模

1. `CREATE INDEX CONCURRENTLY idx_billing_settlements_api_key ON billing_settlements(api_key, cost_units)` → `ledgerDrift` 变 index-only；`auditLag` 仍全表 SUM，建议维护 `system_settings` 增量基线（结算 +、清理 -）。
2. **删除低频 trigram 索引**：`idx_usage_trgm_chat_id`(36 MB, 0 次)、`idx_usage_trgm_trace_id`(26 MB, 4 次)、`idx_usage_trgm_session_id`(14 MB, 0 次)、`idx_usage_trgm_model`(10 MB, 2 次)、`idx_usage_trgm_channel`(5 MB, 0 次)、`idx_usage_trgm_error_message`(6 MB, 26 次)——合计 ~97 MB。审计写入少 6 个索引条目（约 30% 索引写放大）。若控制台模糊搜索需要，保留 1 个并改成 `pg_trgm` 的 `gin_trgm_ops` 部分索引。
3. `usage_logs` 按月分区或归档到对象存储（现有 `rustfs` 可用），保留期已有 worker，补齐"归档再删"。
4. `billing_settlements` 按月分区；对账查询带 `created_at` 范围走分区裁剪。

**预期收益**：审计写路径与结算共享的磁盘/WAL 压力下降；对账从"全表扫描"变成"索引扫描 + 增量"。

### 方案 F【P1】SQLite 单机模式：读写分离连接池

`OpenDB` 对 SQLite 返回两个句柄：写池 1 连接（`_txlock=immediate`），读池 N 连接（WAL 下并发读安全，`busy_timeout` 已有 5 s）。鉴权读、列表查询走读池；结算/审计写走写池。
**预期收益**：单机模式读延迟不再被结算/审计 flush 阻塞。压测里 `pool=8` 对写吞吐没有帮助（2.2 节），但它降低了排队 p50——把"读"从这个队列里拿出来正是同一机制的正确用法。

**同时调 WAL checkpoint**：实测把 `wal_autocheckpoint` 关掉（改由维护窗口 `PRAGMA wal_checkpoint(TRUNCATE)` 或调大页数阈值）后，同一热账户负载 G=1 **2,894 → 4,621 rec/s（+60%）**、p99 **8.46 → 0.96 ms**、max 19.6 → 5.0 ms。单机部署把"每 ~300 条结算一次 checkpoint 停顿"移出请求路径，是仅次于攒批的第二杠杆。
**风险**：WAL 文件会增长（需监控大小并定期 checkpoint）；写连接必须是单写者，避免 `SQLITE_BUSY` 升级死锁。

### 方案 G【P2】可观测性（先能看见，再谈优化）

新增指标：

| 指标 | 类型 | 用途 |
|---|---|---|
| `airoute_settlement_duration_seconds{outcome}` | histogram | 结算 p50/p99，按 settled/deferred 分 |
| `airoute_settlement_batch_size` | histogram | group commit 实际批大小 |
| `airoute_settlement_queue_depth` | gauge | 协调器排队深度 |
| `airoute_settlement_lock_timeouts_total` | counter | 锁等待超时 |
| `airoute_billing_reserved_units` | gauge | 在途预授权金额 |
| `airoute_billing_waived_units_total` | counter | 透支 floor 少收金额（见 P2-10） |

告警：`deferred / total > 0.5%`、`ledger_drift_units != 0`、`overdrafts > 0`、`settlement p99 > 100 ms`。

### 方案 H【P2】资金口径与崩溃顺序

1. **先入队后结算**（见 P2-11）：`Record` 在前（幂等 + 至少一次），结算在后；崩溃只会"晚收"，不会"漏凭证"。
2. **记录 waived 金额**：`deductWallet` 检测到被 floor 钳制时返回 `waived`，累加指标并写一条 WARN（已有限流）；财务口径从"只知次数"变为"知道金额"。
3. 也可考虑 `users.receivable_units`（应收）代替纯钳制，把透支变成"有账可追"，但这属于产品/合规决策，不建议顺手改。

### 若只做三件事

1. **方案 A 预授权按金额**——先把资金敞口关掉（P0，风险最高）。
2. **方案 B 结算攒批**——实测 6×（K=100）/ 10×（K=500）吞吐、尾延迟降一个数量级（P0，收益最大）。
3. **方案 C 隔离池 + `lock_timeout`**——把"热点 key 拖垮全网关"的级联路径切断（P0，最容易被忽略）。

---

## 5. 验证与验收

**基准回归（建议入库）**：把本次探针整理为 `internal/storage/settlement_bench_test.go`（`BenchmarkSettleCharges/records=1|10|100`），作为长期基线；当前基线是 SQLite 单进程 **~3.5k rec/s（K=1）/ ~20.5k（K=100）**。

**压测场景**（k6 / vegeta，对本地或预发环境）：

| 场景 | 目的 | 观测 |
|---|---|---|
| 单 key 热点，50/200/500 并发 | 验证行锁串行与尾延迟 | p50/p99、`deferred` 比例、PG `wait_event='transactionid'` |
| 多 key 分散 | 验证全局吞吐上限 | 吞吐/并发曲线是否平台化 |
| 单 user 多 key | 验证钱包行锁 | users 行锁等待 |
| 低余额 + 高 RPM 突发 | 验证预授权是否挡住超额消费 | 402 比例、`waived_units`、实际服务金额 vs 余额 |
| Redis 抖动（延迟注入 500 ms） | 验证 fail-open 与 deferred 降级 | 尾延迟、deferred、连接池占用 |

**验收标准**：

- 单 key 结算吞吐 ≥ **5,000/s**（PG，group commit 后）；
- 结算 p99 < **20 ms**，`deferred / total` < **0.1%**；
- 突发场景下"已交付服务金额 ≤ 余额 + 1 元"；
- 锁超时/死锁 **0**；对账 `ledger_drift=0`、`wallet_drift=0`、`audit_lag=0`。

---

## 6. 不建议做的事

1. **不要用 Redis 当权威余额**：现有"DB 账本 + 整数纳元 + 幂等键"是正确性基石，Redis 只适合做预授权/缓存。
2. **不要去掉 `billing_id` 幂等键**，也不要用客户端可控的 trace/request-id 派生。
3. **不要单纯加大连接池**：实测加连接不提升写吞吐（SQLite 更明显），PG 上还会撞 `max_connections`；先攒批再谈连接。
4. **不要在鉴权路径加重试/长等待**：现有 2 s 超时 fail-open 已经是风险面，正确方向是让结算**不要占池**（方案 C）。
5. **不要把透支 floor 当风控**：它只限制账面债务，敞口必须靠准入预授权（方案 A）。

---

## 附录 A · 复现命令（全部只读）

```bash
# 线上表规模 / 写放大
docker exec airoute-postgres psql -U airoute -d airoute -At -F'|' -c "
select relname,n_tup_ins,n_tup_upd,n_tup_hot_upd,n_dead_tup,idx_scan,seq_scan
from pg_stat_user_tables where relname in ('api_keys','users','usage_logs','billing_settlements') order by relname;"

# 索引大小与使用率（找无用的 trigram）
docker exec airoute-postgres psql -U airoute -d airoute -At -F'|' -c "
select indexrelname, pg_size_pretty(pg_relation_size(indexrelid)), idx_scan
from pg_stat_user_indexes where relname='usage_logs' order by pg_relation_size(indexrelid) desc;"

# 关键参数
docker exec airoute-postgres psql -U airoute -d airoute -At -c "
select name,setting from pg_settings
where name in ('synchronous_commit','fsync','wal_sync_method','lock_timeout','statement_timeout','max_connections');"

# 结算与审计指标（token 在容器环境变量里，不回显）
docker exec airoute-1 sh -c 'curl -s -H "Authorization: Bearer $GATEWAY_METRICS_TOKEN" \
  http://127.0.0.1:8080/metrics | grep -E "billing|usage_log"'
```

## 附录 B · 关键代码位置

| 关注点 | 位置 |
|---|---|
| 同步结算调用 + 1.5 s 超时 | `internal/api/common.go:99-102,312-320` |
| 结算事务 / 幂等账本 / 聚合扣款 | `internal/storage/settlement.go:36-51,68-159,172-224,228-290` |
| 钱包扣款与透支 floor / 锁行 | `internal/storage/wallet_deduct.go:20,40,53-115,122-142` |
| 审计批量写（先钱后账） | `internal/storage/repo_logs_writer.go:117-131` |
| 异步管道 / 攒批 / spill | `internal/storage/async_logger.go:95-154,209-280` |
| 计价引擎（内存价目表） | `internal/billing/engine.go:84-158,260-328,402-501` |
| 准入：余额判断 + 在途预留 | `internal/middleware/auth.go:279-323`、`authz.go:98-118`、`quota_tracker.go:19-22,35-59` |
| 分布式预授权 Lua | `internal/distributed/redis_quota.go:35-61,87-103` |
| 连接池 / PG 超时参数 | `internal/storage/db.go:221,283-296,340` |
| 对账 | `internal/storage/billing_reconcile_job.go:176-196,199-215,226-238` |
| 账本表结构与索引 | `internal/storage/migrations_billing_settlements.go:16-43` |
| 服务端超时与 499 | `cmd/airoute/server.go:217-225` |

## 附录 C · 本次基準的边界

- 只在 **SQLite WAL / 单进程 / `MaxOpenConns=1`** 下测得（5 次独立运行，吞吐 ±20%）；只覆盖 `settleCharges` 本身，不含鉴权读/审计写/Redis。PG 的 `fsync`、行锁行为是**推算**，需要用方案 B/C 的预发压测确认。
- 生产流量几乎全是 admin（钱包豁免），所以"钱包行锁"这条路径在生产上**尚未被验证**。
- 完整性校验覆盖了 50,000 条结算（行数、cost_units、钱包扣减一致），未在 PG 上复跑。

---

## 9. 实施状态（2026-10-28 落地）

> 本节记录报告中的建议落地成了什么、在哪里、实测收益多少，以及**与建议不同的有意偏离**。所有改动通过 `go build ./...`、`go test ./...`（全仓）与 `go test -race ./internal/storage/ ./internal/middleware/ ./internal/api/`。

### 9.1 改动总览

| 项 | 状态 | 主要文件 |
|---|---|---|
| P0-1 按金额预授权 + 在途并发上限 + dispatch 补差 | ✅ | `middleware/quota_tracker.go`、`middleware/auth.go`、`distributed/redis_quota.go`、`billing/estimate.go`、`api/common.go` + 10 个 dispatch 点 |
| P0-2 结算攒批（group commit） | ✅ | `storage/settlement_batcher.go`、`api/common.go`、`cmd/airoute/server.go` |
| P0-3 lock_timeout + 重试（独立结算池**未做**，见偏离 2） | ✅（部分） | `storage/db.go`、`storage/settlement.go` |
| P1-4 流式尾延迟重排 | ✅ | `api/*_stream*.go`（5 个流式 handler） |
| P1-5 事务瘦身 / 稳定锁序 | ✅ | `storage/settlement.go`、`wallet_deduct.go` |
| P1-6 SQLite 读写池 + WAL checkpoint | ✅ | `storage/db.go`、`repo_users.go`、`repo_keys.go`、`repo_pricing.go` |
| P1-7 账本索引 / 删无用 trigram / 对账增量 / 账本生命周期 | ✅ | `storage/migrations_billing_scale.go`(v41)、`billing_reconcile_job.go`、`billing_ledger_retention.go` |
| P2-9 结算可观测性 | ✅ | `telemetry/histogram.go`、`metrics.go`、`metrics_prometheus.go` |
| P2-10 透支 waived 口径 | ✅ | `storage/wallet_deduct.go`（commit 后落账） |
| P2-11 结算/入队顺序 | ⚠️ 有意保持"先结算后入队" | `api/common.go`（原因见偏离 3） |
| P2-12 SQLite 单连接 | ✅ | 同 P1-6 |

### 9.2 实测效果（仓库内基准，SQLite WAL，`go test -bench`）

| 场景 | 结果 | 对比 |
|---|---|---|
| 单笔结算（K=1） | **134 µs/事务 ≈ 7.5k 笔/s** | 改造前外测基线 2.8–3.5k/s |
| 合并 10 笔/事务 | 35.6 µs/笔 ≈ **28k 笔/s** | K=1 的 ~3.8× |
| 合并 100 笔/事务 | 15.1 µs/笔 ≈ **66k 笔/s** | K=1 的 ~8.9× |
| 20 并发走 batcher（默认 2ms linger） | 105 µs/笔 ≈ 9.5k 笔/s | 低并发时受 linger 约束，符合预期 |

基准已入库：`internal/storage/settlement_bench_test.go`（`BenchmarkSettleChargesBatched`、`BenchmarkSettlementBatcherParallel`），作为回归基线。
单笔从 2.8–3.5k/s 提升到 7.5k/s 来自**事务瘦身**（删掉冗余 `SELECT FOR UPDATE`、SQLite 少一条写语句）与 **WAL checkpoint 移出写路径**；攒批再叠 8.9×。

### 9.3 关键语义

1. **预授权**：准入先按 `GATEWAY_INFLIGHT_RESERVE_CNY`（默认 0.05）+ 单用户在途上限（默认 64）预留；解析出 model/prompt/max_tokens 后，在 dispatch 前用 `billing.EstimateCostCNY` 把预留**补到保守估算值**（clamp 到 `balance - 已预留`）；结算后按 `max(预留, 实际)` 释放。余额不足返回 402，不再"余额>0 就放行"。
2. **攒批**：每笔收费入队，最多等 `GATEWAY_SETTLEMENT_WAIT_MS`（默认 300ms）拿到"已提交"确认；超时/队列满 → 交给至少一次投递的审计管道补结算。`billing_id` 唯一索引保证"批结算 + 管道补结算"只扣一次。批处理器结算**副本**，调用方成功后自行置 `Settled`，避免与请求 goroutine 共享可变状态。
3. **重试与指标**：PG 40P01/55P03/57014 最多 2 次抖动重试；`BillingOverdrafts` / `BillingWaivedUnits` 改为**事务提交成功后**才落账，重试回滚不会重复计数。
4. **流式**：`recordStreamEnd` 只做计价与准备；结算被推迟到 `[DONE]` 已 flush 之后执行，settlement 等待不再推迟客户端看到的终止帧。
5. **对账**：结算侧保留精确 SUM（保证"账本行被删当次可见"），漏洞最大的审计侧（250MB `usage_logs`）改为滚动基线 + 5 分钟重叠窗口的增量；账本归档进 `billing_ledger_archives` 后 `auditLag`/`ledgerDrift` 仍恒等。
6. **账本生命周期**：`GATEWAY_BILLING_LEDGER_RETENTION_DAYS` 默认 0（永久保留）；启用时只删"审计行已不存在且过期"的行，金额转入归档表，并在 `system_settings` 留月度归档键与 WARN 日志。

### 9.4 用户决策与最终实现

| 决策点 | 选择 | 落地 |
|---|---|---|
| 余额/状态未知时的准入姿态 | **fail-closed 503** | `middleware/authz.go:BalanceUnknownDenial` + `auth.go` 在"有 owner 但状态未知"时拒绝（自由读豁免）；不再出现"余额检查与预授权同时被跳过" |
| 审计入队与结算顺序 | **先入队后结算** | `api/common.go:settleUsage` 先把 `CloneForPipeline()` 交给至少一次管道、再结算；billing_id 在克隆前钉死，两条路径同键；崩溃窗口关闭 |
| 结算专用连接池 | **暂不做** | 维持共享主池：`lock_timeout=500ms` + 重试已把"等锁占池"从 30s 压到 0.5s，再开池会让每副本连接预算翻倍 |
| 大表月分区 | **现在做** | 迁移 v43 `usage_logs_monthly_partitions`（PG-only、`GATEWAY_ENABLE_TABLE_PARTITIONING=true` 才注册）：单事务 copy-switch、整月 DROP、DEFAULT 兜底、`usage_audit_ids` 去重登记表；`billing_settlements` **不分区**（29MB，且唯一键是资金幂等闸门） |
| 提交方式 | **拆成 4 个提交** | ① 结算核心 ② 准入与接线 ③ schema·对账·保留·告警·文档 ④ 月分区（后补的决定，独立成提交） |

### 9.5 真机验证（PostgreSQL 16，临时容器，未触碰线上库）

| 验证项 | 结果 |
|---|---|
| `AIROUTE_TEST_DATABASE_URL=... go test -tags=integration ./internal/storage/ -run Postgres` | **11/11 PASS**（含 CTE 合并结算路径、透支钳制、审计重投幂等） |
| 迁移 v41 记录 | `schema_migrations.version=41` 存在 |
| 迁移 v43（分区，临时容器 + flag） | `TestUsageLogsPartition*` **4/4 PASS**：copy-switch 后行数/金额一致、整月 DROP、锁竞争 deferred 不阻塞启动、空表可迁移；1M 行实测启动 30.6s、磁盘 2.07×、单月查询只扫 1 个分区 |
| `idx_billing_settlements_api_key` | 存在且 `indisvalid=t`；`SET enable_seqscan=off` 后 `ledgerDrift` 的 `GROUP BY api_key` 走 **Index Only Scan** |
| `billing_ledger_archives` | 表已创建 |
| 无用 trigram 清理 | `usage_logs` 索引 20 → 15，仅保留 `idx_usage_trgm_error_message` |
| `lock_timeout` | `TestApplyPostgresStatementTimeoutsIncludesLockTimeout` 通过 |

### 9.6 告警规则

`deploy/prometheus/airoute-alerts.yml` 新增 `airoute-billing-settlement` 组 10 条：结算推迟率 >1%、请求侧等待超时、行锁超时、结算 p99 >100ms、批大小长期退化为单笔、在途预留 > 钱包总额 80%、waived 金额增长、账本/余额漂移、审计滞后。为此新增 `airoute_billing_wallet_total_units` 指标（对账 worker 顺带统计，作为预留量的分母）。

### 9.7 提交记录

| # | commit | 内容 |
|---|---|---|
| 1 | `feat(billing): group-commit settlement, slimmer transactions, read pool` | 攒批、事务瘦身、稳定锁序、lock_timeout、SQLite 读池 + checkpoint、指标 |
| 2 | `feat(admission): amount-based pre-authorisation and fail-closed balance` | 金额预授权、dispatch 补差、fail-closed、先入队后结算、流式尾延迟 |
| 3 | `feat(billing-ops): ledger index, incremental reconcile, retention, alerts` | v41、增量对账、账本保留、告警、启动装配、配置与文档 |
| 4 | `feat(billing): optional monthly partitioning for the audit log` | v43 月分区、去重登记表、分区保留、PG 42702 修复 |

> 提交快照已在独立 worktree 复验：`go build` + 除"前端产物占位"导致的 UI 断言外全量测试通过。

### 9.8 残余风险

- **分区迁移默认关闭且必须按顺序上线**：先把所有副本升级到含 `usage_audit_ids` 写路径的版本，再开 `GATEWAY_ENABLE_TABLE_PARTITIONING=true`（旧二进制在新分区表上 `ON CONFLICT (billing_id)` 会报 42P10，审计写失败但不影响已结算的钱）。维护窗口内会持 ACCESS EXCLUSIVE 锁做 copy-switch（1M 行约 20s 量级），需要 2× 表空间；回滚 SQL 写在 `migrations_partition.go` 文件头（rename 回 legacy 表 + 删 v43 版本记录），全程不 DROP 线上数据。SQLite 无分区能力，保持原表 + 归档策略。
- **去重登记表保留 30 天**：超过 30 天的极端重投可能产生重复审计行（资金侧仍由 `billing_settlements` 唯一键保证）。
- **waived 口径仍是上限近似**（SQLite/PG 的 `UPDATE ... RETURNING` 拿不到旧余额）；"从 floor 上方恰好钳到 floor"的单会整笔计入，高估 ≤ 一个透支额度/单，仅影响指标。
- **预授权估算依赖 call-site**：HTTP 的 10 个 dispatch 点与 MCP 已覆盖；其它模态（embedding/rerank/multimodal）仍按粗预留 + 并发上限。
- **`billing_settlements` 未分区**：增长慢（29MB/62k 行），但长期仍需要归档或分区；若将来分区，幂等闸门要先迁到独立登记表。
- 无独立结算连接池：若出现池饥饿，再按连接预算拆分。

