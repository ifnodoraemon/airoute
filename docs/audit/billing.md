# 计费/扣费正确性审计（Billing Correctness & Revenue Leakage Audit）

- **Scope**: `internal/billing/*`, `internal/storage/{money.go,repo_logs_writer.go,repo_users.go,async_logger.go}`, 全部收费调用点（`internal/api/*.go`, `internal/controlplane`）
- **Method**: 源码逐路径审计 + 线上 Postgres 只读 SQL 交叉验证（2 副本 airoute-1/2，Redis Streams 分布式模式）
- **Data snapshot**: 2026-10-23T02:15Z（10:15 Asia/Shanghai），`usage_logs` 28,829 行
- **Constraints honored**: 未修改任何 Go 源码、未重启容器、DB 仅 SELECT

## 0. 结论（TL;DR）

**扣费部分需要继续优化**，但优化的重点**不是"再调价格/再加优惠策略"**，而是三个结构性缺陷：

1. **扣费与审计日志管道强耦合**：钱包扣减只发生在 `usage_logs` 写入事务内，日志被丢弃 = 钱收不到（当前丢弃计数为 0，但代码路径存在且静默）。
2. **没有幂等键**：Redis Stream 在"事务已提交但 XACK 失败/进程重启"时会重放消息，重放会**同时重复插入流水并重复扣款**（无唯一约束可拦截）。这是唯一会向客户多收钱的缺陷，也是账目不可对账的根因。
3. **流式请求中途断连（499）零计费**：`/v1/chat/completions` 与 `/v1/messages` 在客户端断开时，已产生的 token 完全不收费（实测 6 条 499 记录带了 907,419 prompt token，收费 ¥0）。其中断连前的 prompt 成本已由上游收取，若断连发生在开始输出之后，则 completion 也一并免单——**可被恶意客户端稳定利用**。

价格配置层面当前**基本无需优化**（引擎算术实测 0 误差、分组乘数/通配符/时区/折扣钳制均正确），只有"未配置价格的模型落到硬编码默认价 + 默认开启 50% 夜间折扣"这一处需要收紧。

### 资金基线（用于换算所有影响）

| 指标 | 值 |
|---|---|
| 计费流水行数 / 已收费行数 | 28,829 / 26,840 |
| `SUM(cost)` | **¥393.63** |
| 单请求平均收费 | ¥0.01467 |
| 单请求最高收费 | ¥0.6448（63 万 prompt token） |
| 真实钱包实际扣减 | **仅 ¥1.02**（user `test`，余额 998.98） |
| admin 豁免未扣减 | ¥394.88（2 个 admin 名下 key，占 99.7%） |

> 关键含义：**当前线上 99.7% 的计费额来自 admin 豁免 key，钱包扣减路径几乎没有被真实付费流量压测过**。下面所有"会亏钱"的结论，按 `概率 × 金额` 排序时都要乘上"未来真实付费流量"这个放大系数。

---

## 1. 发现汇总表

| ID | 严重度 | 标题 | 资金影响 |
|---|---|---|---|
| F1 | **P0** | 计费无幂等键，Redis Stream 重放会重复扣款 | 每次重放事件对该批（≤100 条）**重复收费**，表上无唯一约束可拦截；用户被多扣，不可对账 |
| F2 | **P1** | 扣费与审计日志强耦合，日志丢弃 = 收入消失 | 队列积压/停机路径下该批（≤100 条）全部不收钱；按当前量 ≈ ¥1.47/批 |
| F3 | **P1** | 流式断连（499）零计费，可被客户端利用 | 已实测 6 条 / 907,419 token 收费 ¥0；理论上单次可免掉整段 completion |
| F4 | **P1** | 无透支钳制，余额可扣成负数且无告警 | 单请求透支上限 ≈ ¥0.65（本例），大 prompt 下无上限；并发窗口内可放大 N 倍 |
| F5 | **P1** | 未配价模型落硬编码默认价，且默认开启 50% 夜间/周末折扣 | 夜间/周末对未配价模型**少收 50%**；白天对 deepseek 类模型**多收 2×**（实测 110 行 ¥0.845 本应 ≈¥0.42） |
| F6 | **P1** | 非流式上游未返回 usage 时按 0 token 计费（无估算兜底） | 已实测 **148 条 200 成功请求收费 ¥0**（¥2.2 量级）；活跃渠道曾持续产生 |
| F7 | P2 | `/v1/moderations` 代理上游但完全没有计费调用 | 配了 moderation 渠道后 100% 成本白送（当前未配置 → 潜伏） |
| F8 | P2 | MCP 未带 key / 合成 `mcp-session` 的用量无法落到任何钱包 | 开放模式或 legacy key 下 LLM 调用白送（当前 0 行 → 潜伏） |
| F9 | P2 | 账本与审计流水不可对账（删除不冲账 + 双整数镜像 + 迁移遗留） | ¥2.6475 有账无流水、¥4.5002 有金额无整数镜像；无法证明收入/无法处理退款争议 |
| F10 | P2 | 按"请求模型"计费而非"实际服务模型"，跨模型降级时错价 | `model_fallbacks` 目前为空 → 潜伏；一旦启用且降级到更贵模型即静默亏损 |
| F11 | P3 | 原生 Anthropic 非流式把 cached token 按全价 prompt 计费 | 对客户**多收**约 90% × cached 部分 |
| F12 | P3 | Anthropic 非流式 cached 明细缺失 + cache_creation 平价 | 少量错价（多收/少收并存） |
| F13 | P3 | 流式 usage 帧"直接赋值"而非取 max，可能被后到的 0 帧清零 | 少收（低概率） |
| F14 | P3 | off-peak 以"记账时刻"而非"请求开始时刻"判定；非法时间串静默变 00:00-08:30 折扣 | 边界请求与配置错误时错价 |
| F15 | P3 | 批内取整口径不一致（聚合后取整 vs 逐行取整） | 每批 ≤1e-8 CNY 级漂移，当前实测 0，但破坏可对账性 |
| F16 | P3 | `DeductUserBalance` 死代码；50 个僵尸 Redis consumer；`used_cost_nano` 陈旧 | 误导维护者（以为有独立扣费 API）/ 运维噪音 |
| F17 | P3 | 12,610 行历史计费无 `pricing_snapshot`，无法复核 | 历史收入不可复算（¥244.02） |

---

## 2. 详细发现

### F1 · P0 · 计费无幂等键：Redis Stream 重放 = 重复扣款

**证据**

- 写入是普通 INSERT，无 `ON CONFLICT`：`internal/storage/repo_logs_writer.go:90-95`（单条）、`:240-244`（批量）。
- 表上没有任何唯一约束/幂等键：线上 `usage_logs` 仅有 `idx_usage_trace`（普通 btree）与 trgm 索引，**`trace_id` 非唯一**（已用 `pg_indexes` 核实）。
- 扣款与写入在**同一事务**内：`repo_logs_writer.go:78-134`（单条）、`:157-305`（批量）→ 事务重放 = 重复扣款。
- 分布式消费端：先写库、后 XACK；写失败保留在 PEL，ACK 失败也保留在 PEL：
  - `internal/storage/async_logger.go:272-291`（写库 3 次重试，失败 `return false` 留在 PEL）
  - `:294-299`（`AckLogs` 在 commit 之后；ACK 失败只是不更新 `ackedUpTo`，**消息仍在 PEL**）
  - `:302-344`（启动时 PEL 恢复：**重放**）
  - `:356-365`（AutoClaim：30s 后回收"空闲"消息，**自己提交过但没 ACK 的消息也会被自己重新领取**）
- 代码自己承认了这个漏洞：`async_logger.go:318-325`
  > `// XACK did not take effect, so the next read returns the same head.`\
  > `// Rewriting it would duplicate billing rows; the AutoClaim ticker retries the leftovers`

**为什么这是 P0（钱）**

一次"commit 成功 + XACK 失败"（Redis 抖动/重启/网络分区）或"commit 成功 + 进程被杀"（滚动发布）会让该批最多 100 条消息全部被重放；重放路径 `BatchRecordUsageLogsContext` 会再插 100 行流水并再扣一次钱。按当前均值 100 × ¥0.0147 ≈ **¥1.47/次**，但含大 prompt 请求时可到 **¥64/批**（实测单请求最高 ¥0.6448）。更严重的是：**没有任何字段能事后识别哪些行是重放**（`trace_id` 可重复、无 `billing_id`），所以无法退款、无法对账、无法证明系统没多收钱。这是唯一会**主动伤害客户**的缺陷。

顺带否决一个常见错误修法：**不能**直接给 `trace_id` 加唯一索引。`trace_id` 来自客户端请求头（`internal/middleware/trace.go:16-26, 55-70`：`X-Airoute-Trace-ID` / `X-Request-ID` / `X-Trace-ID` / `traceparent`），实测线上已存在 1 个被复用的 trace_id 被 92 条真实不同请求共用。对该列加唯一索引会**阻塞正常收费**，甚至让客户端"复用 trace 头即可免费"。

**最小修法（代码草图）**

1. 服务端生成、**非客户端可控**的幂等键：

```go
// internal/api/common.go, 在 cost 计算后、写日志前
params.BillingID = newBillingID() // e.g. "bill-" + 16 random bytes hex
```

2. 落库幂等（迁移，SQLite/Postgres 各一份）：

```sql
ALTER TABLE usage_logs ADD COLUMN billing_id TEXT;
CREATE UNIQUE INDEX uq_usage_billing_id ON usage_logs(billing_id) WHERE billing_id IS NOT NULL;
```

3. 只有"真正插入成功"的行才允许扣款（单条 / 批量都要改）：

```go
res, err := tx.Exec(r.db.Rebind(`INSERT INTO usage_logs (..., billing_id) VALUES (...)
    ON CONFLICT (billing_id) DO NOTHING`), args...)
if err != nil { return err }
if n, _ := res.RowsAffected(); n == 0 { continue } // 重放：跳过该行的所有钱包/used_cost 增量
```

批量路径必须用 `RETURNING billing_id`（或逐行 RowsAffected 累加）来构造 `keyCostMap`，只聚合"新插入"的行，否则重放时流水去重了但钱包仍被二次扣减。

4. 迁移回填：`UPDATE usage_logs SET billing_id = 'legacy-'||id WHERE billing_id IS NULL;`（保证唯一索引可建）。

同时把 `async_logger.go` 的 `AckLogs` 失败从"静默"改为记 ERROR 指标（`usage_log_ack_failures_total`），用于观测。

---

### F2 · P1 · 扣费与审计日志强耦合：日志被丢弃 = 收入静默消失

**证据**

- 唯一的扣款入口在日志写入事务里：`repo_logs_writer.go:100-132`、`:264-303`。
- 请求侧只调用 `GlobalAsyncLogger.Record(...)`，**没有失败反馈、没有兜底同步写**：`internal/api/common.go:99-126`。
- 丢弃路径（全部静默、只加 metric）：
  - `async_logger.go:123-138`：本地缓冲满 500ms → `dropping log record to prevent gateway freeze`
  - `async_logger.go:177-187`：连续 5 次 flush 失败 → `dropping unpersistable usage log batch`（整批 ≤100 条）
  - `async_logger.go:208-224`：停机时 3 次重试仍失败 → `usage logs remain unpersisted at shutdown`
  - `async_logger.go:116-122`：logger 已停止 → 丢弃
- Redis 不可用时会退化为本地内存缓冲（`async_logger.go:100-113`），进程崩溃即丢。
- 独立扣费 API `DeductUserBalance` **零生产调用方**（`repo_users.go:224`，全仓 grep 仅定义）→ 说明不存在第二条扣费路径。

**为什么是 P1（钱）**

DB 抖动（flush 超时 10s `async_logger.go:24`）叠加 Redis 同时不可用时，5 次失败后整批丢弃，这批请求**既不收费也不留痕**。按当前量 ≈ ¥1.47/批；真实付费流量下每批可达数十元。当前实测**未发生**（两个副本 `records_dropped_total=0`、`flush_failures_total=0`，已抓取 `/metrics` 核实），所以是"高损失 × 低概率"，排在 F1 之后。

**最小修法**

- `AsyncLogger.Record` 返回 `bool`（是否成功入队/入 Redis）；`recordUsage` 收到 `false` 时**同步**调用 `repo.RecordUsageLogContext(ctx3s, rec)` 兜底，仍失败则 `usage_log_records_dropped_total` + CRITICAL 日志 + 落盘 append-only 文件。
- 长期正解：**把钱包扣减从日志事务里拆出来**（先同步扣费，再异步审计）。这是架构级改动，建议列入下一迭代而不是本次热修。

---

### F3 · P1 · 流式客户端断连（499）零计费，可被利用

**证据**（已产生 token 但不收费的路径）

| 端点 | 位置 | 断连时的行为 |
|---|---|---|
| `POST /v1/chat/completions` (stream) | `chat_stream_handler.go:60-70` | `ctx.Err() != nil` → `statusCode = 499` → `recordUsage` 中 `success=false` → **cost=0** |
| `POST /v1/messages` (stream, 适配路径) | `inbound_anthropic_stream.go:126-138` | 同上，499 → 不计费 |
| `POST /v1/messages` (stream, 原生路径) | `inbound_anthropic_stream.go:449-455` | `ctx.Done()` → `statusCode = 499` → 不计费 |

计费闸门只有一行：`common.go:58-70` — `success := params.StatusCode >= 200 && params.StatusCode < 400`；`if success && !params.SkipBilling { 计算 cost }`。因此任何 499/502 一律 cost=0。

**对照**：另外 3 个流式家族反而会收费（不一致）：`completions_stream_handler.go:110-125`、`gemini_inbound_stream.go:86-94`（defer 里固定用 200）、`responses_stream.go:103-128`。也就是说 5 个流式实现里 2 个"断连免单"、3 个"断连照收"。

**线上实测**

```sql
select id, model, prompt_tokens, completion_tokens, cached_tokens, cost, status_code
from usage_logs where status_code=499;
-- 6 rows, 907,419 tokens, cost = 0
```

估算漏收：这 6 条以 cached prompt 为主（156k/119k/…），按 `deepseek-v4.1-flash` 现价约 **¥0.10**——金额小，但**结构上单次可免掉整段 completion**。

**为什么是 P1（钱）**

正常客户端超时/切网就会触发（概率不低）；更关键的是**这是可被主动利用的逃生门**：发流式请求 → 收够内容 → 主动断开 → 网关侧 499 → 不收费。对转售/逆向接入场景等同于免费。上游已经为这些 token 计费，成本全额由网关承担。

**最小修法**：把"审计状态"和"是否计费"解耦。

```go
// common.go: AuditRecordParams 增加
Billable bool // 即使 statusCode 非 2xx，也已产生上游成本

// common.go:60
if (success || params.Billable) && !params.SkipBilling && billing.GlobalEngine != nil { ... }

// chat_stream_handler.go: recordStreamEnd
billable := totalCompTokens > 0 || totalPromptTokens > 0
recordUsage(c, AuditRecordParams{..., StatusCode: statusCode, Billable: billable})
```

499 仍然以 499 记入审计（可观测性不变），但对已消耗/已交付的 token 计费。

---

### F4 · P1 · 无透支钳制与告警，余额可扣成负数

**证据**

- 扣款是裸减法，无 floor、无负余额检查、无事件：
  - 单条：`repo_logs_writer.go:106`、`:119`、`:128`
  - 批量：`:264`、`:295`
  - 死代码里同样没有：`repo_users.go:228`
- 准入前检查只有 `balance <= 0` 拦截：`internal/middleware/auth.go:232-242`
- 在途预留仅 **0.0005 元/请求**：`internal/middleware/quota_tracker.go:18-21`（`minReserveCost: 0.0005`），而实测单请求最高 ¥0.6448 → 预留量仅为实际成本的 **0.08%**。
- 余额可能"陈旧"：准入优先用内存快照 `auth.go:201-216`；仅当 DB 查询成功时才用新鲜值（`auth.go:151-198`，且 `keyRec, _ :=` **忽略了错误**）。DB 超时/失败 → 退回内存值（周期同步间隔 10s，`cmd/airoute/server.go:198`）。

**为什么是 P1（钱）**

`balance = 0.01` 的用户发一个 63 万 prompt token 的请求（¥0.64）→ 余额变 −0.63，事后才被 `balance <= 0` 拦住。10s 同步窗口 + 2s DB 超时窗口内，并发 N 个请求可把透支放大 N 倍。硬上限取决于单请求最大成本（当前实测 ¥0.6448，但 prompt 上限由上下文长度决定，可达数元）。收入直接损失，且**没有任何告警**告诉运营"这个用户透支了"。

**最小修法**

1. 预留额度按预估成本而非固定 0.0005：准入时用 prompt 字节数估算 prompt token × prompt 单价（`estimateTokens(len(body))`）作为 `minReserveCost` 传参。
2. 扣款时钳制 + 置位（Postgres）：

```sql
UPDATE users
   SET balance_units = GREATEST(COALESCE(balance_units,0) - ?, -? /*hardLimitUnits*/),
       balance = GREATEST(COALESCE(balance_units,0) - ?, -?) / 1e9,
       updated_at = CURRENT_TIMESTAMP
 WHERE ...
RETURNING (COALESCE(balance_units,0) < 0) AS overdrafted;
```

3. 透支时 `telemetry.GlobalMetrics.BillingOverdraft.Add(1)` + 把用户 `status='locked'`（或至少写一条审计事件），让运营可见。

---

### F5 · P1 · 未配价模型落硬编码默认价，且默认开启 50% 夜间/周末折扣

**证据**

- 线上 `model_prices` **只有 2 行**（`deepseek-v4.1-flash`、`aliyun/glm-5.3`，均 `off_peak_enabled=0, off_peak_mode='none'`）。
- 且 seed 只在表为空时执行：`internal/storage/repo_pricing_seed.go:4-9`（`if count > 0 { return nil }`）→ seed 里那一大批 `deepseek* / gpt-* / claude-* / flux-* / whisper-*` 通配价**全都没有**落到线上库。
- 未命中任何规则的模型落到**引擎内硬编码默认记录**：`internal/billing/engine.go:44-57`

```go
deflt: &storage.ModelPriceRecord{
    PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.2,
    OffPeakEnabled: true, OffPeakMode: "custom",
    OffPeakDiscount: 0.5, WeekendAllDay: true,
    OffPeakStart: "00:00", OffPeakEnd: "08:30",
}
```

- 命中路径：`engine.go:271`（`return e.deflt`）；折扣生效：`engine.go:388-399` + `strategy.go:59-138`（`WeekendAllDay` 在 `:64-67`）。

**为什么是 P1（钱）**

两个方向的错价同时存在，且是**系统性**的：

1. **夜间少收 50%**：任何没配价的模型，在 00:00–08:30 与整个周末自动 5 折。当前 `is_off_peak=1` 行数为 0（因为 2 个已配价模型都关了 off-peak，且未配价流量都落在工作日白天），**属潜伏但确定会触发**——今晚 00:00 起任何新模型流量就是 5 折。
2. **白天多收 / 少收**：默认价 ¥2/¥8 是拍脑袋值。实测 `volcengine/deepseek-v4-1-flash-260910`（110 行，115,772 prompt + 90,032 completion）按默认价收了 **¥0.8451**，按其同系列价 ¥1/¥4 应约 **¥0.42** → **多收 2×**（对客户不利，易引发投诉）。反之若某模型真实成本高于 ¥2/¥8，则**少收**。

**最小修法**

1. 默认记录改为**不折扣**：`OffPeakEnabled: false`（不确定的模型宁可按标价收，也不要自动打折）。
2. 启动时（或每日）自检：`SELECT DISTINCT model FROM usage_logs` / `model_routes` 与 `model_prices` 求差集，对未配价模型打 WARN 并写入 `system_events`，让运营补齐价格。
3. 让 seed 变成"补缺"而非"仅空表"：`INSERT ... ON CONFLICT DO NOTHING` 逐条补通配规则（迁移里做）。
4. 审计可辨识：`pricing_snapshot` 已含 `model`，建议再加 `pricing_source: "exact|wildcard|group|default"`，让运营能一眼筛出"被默认价计费"的请求。

---

### F6 · P1 · 非流式上游未返回 `usage` → 按 0 token 计费（无估算兜底）

**证据**：所有非流式处理器都是同一模式——`if resp.Usage != nil { ... }`，否则保持 0，并且**不做任何估算**：

- `internal/api/handler.go:113-129`（chat/completions）
- `internal/api/completions_handler.go:136-159`
- `internal/api/responses_handler.go:107-125`
- `internal/api/gemini_inbound.go:168-192`
- `internal/api/inbound_anthropic.go:100-108`（原生分支）
- `internal/api/embedding_handler.go:47-56`；`rerank_handler.go:52-60`

对比：流式家族都有估算兜底（`chat_stream_handler.go:72-80`、`gemini_inbound_stream.go:51-62`、`responses_stream.go:106-118`、`inbound_anthropic_stream.go:94-108`）。所以**同一渠道流式收费、非流式免费**。

**线上实测**

```sql
select model, channel, count(*) n, sum(total_tokens) tt, min(created_at), max(created_at)
from usage_logs where status_code=200 and cost=0 group by 1,2;
-- deepseek-v4.1-flash | gpustack-cluster | 92 | 0 | 10-19 → 10-19
-- deepseek-v4.1-flash | 罗总             | 56 | 0 | 10-21 → 10-22
```

**148 条 HTTP 200 成功请求收了 ¥0**（占 200 响应的 0.55%），`usage_source` 全是 `upstream`、token 全 0 —— 典型"上游没回 usage 块"。其中 56 条发生在**当前活跃渠道 `罗总`** 上，最后一条 2026-10-22 09:06Z，此后不再出现（渠道/上游行为变化）。按均值估算漏收 ≈ **¥2.2**；只要渠道再次变更就可能复发。

**最小修法**：非流式补齐与流式一致的兜底。

```go
// common.go 增加
func (p *AuditRecordParams) EnsureUsageFromEstimate(req *model.ChatCompletionRequest, resp *model.ChatCompletionResponse) {
    if p.PromptTokens == 0 && req != nil {
        p.PromptTokens = estimateCanonicalPromptTokens(req); p.UsageSource = "estimated"
    }
    if p.CompletionTokens == 0 && resp != nil && len(resp.Choices) > 0 {
        p.CompletionTokens = estimateTokens(len(resp.Choices[0].Message.GetContentString())); p.UsageSource = "estimated"
    }
}
```

在 5 个非流式 handler 的 `RecordUsage` 之前调用一次。`usage_source='estimated'` 已能被审计页面筛出，不会污染"上游真实计量"。

---

### F7 · P2 · `/v1/moderations` 代理上游但完全不计费

**证据**：`internal/api/moderation_handler.go:66-82` 在存在 `moderation` 协议渠道时把请求原样转发给上游（`DispatchHTTP`），随后 `c.Data(...)` 直接返回，**全文件没有 `RecordUsage` / `AsyncLogger.Record`**。路由已注册：`internal/api/router.go:260`（`/v1/moderations`）与 `:298`（root 别名）。

**为什么是 P2（钱）**：一旦运营给某渠道勾选 `moderation` 协议，每次审核调用都会产生上游费用而收费为 ¥0，**100% 白送**。当前线上两个渠道协议为 `openai_chat/openai_response/anthropic_messages`，无 `moderation`（已查 `channels.protocols`）→ 尚未发生。另外注意：无渠道时会返回**全部为 false 的伪造结果**（`:88-125`），那属于功能造假而非计费问题，但同样是风险点。

**最小修法**：在转发成功分支补一次计费，按固定价计（moderation 按输入 token 或按次）：

```go
if err == nil && resp.StatusCode == http.StatusOK {
    RecordUsage(c, AuditRecordParams{Model: modModel, Channel: resp.Channel,
        Duration: time.Since(start), StatusCode: resp.StatusCode, BillableUnits: 1})
    ...
}
```

并在 pricing 里要求该模型必须配置 `fixed_price > 0`（复用 `multimodal_handler.go:32-38` 的 `pricing_not_configured` 守卫）。

---

### F8 · P2 · MCP 合成身份导致用量无法落到任何钱包

**证据**：`internal/api/mcp_tools_routing.go:196-207`

```go
callerAPIKey := "mcp-session"
callerTenantID := "mcp"
if callerKeyRec != nil { ... 真实 key ... }
```

在开放模式（未提供 key 且 `CountAPIKeys()==0`）下，流水写入 `api_key='mcp-session', tenant_id='mcp'`。落到写入器：`repo_logs_writer.go:101` 的 `UPDATE api_keys ... WHERE key='mcp-session'` 命中 0 行；`:106` 的子查询 `user_id` 为 NULL → 0 行；`:113-123` 的 tenant 回退用 `username='mcp' OR email='mcp'` 匹配 0 行 → **无人被扣款，且无任何报错**。同一问题适用于任何"key 行不存在且 tenant 名不匹配用户"的历史 key。

**为什么是 P2（钱）**：MCP 的 `airoute_chat` 工具走完整 LLM 调度，成本真实存在；开放模式下全部白送。当前 `usage_logs` 中 `api_key='mcp-session'` 为 **0 行** → 尚未发生（因为线上已注册 API key，走严格鉴权）。另外 MCP 路径还缺 `pricing_snapshot` 与 `UsageSource`（`:216-241`），审计信息弱于 HTTP 路径。

**最小修法**：写入器在"应扣未扣"时**必须发声**——在 `repo_logs_writer.go:113-123` 的 tenant 回退后检查 `RowsAffected()`，为 0 时 `telemetry.GlobalMetrics.BillingUnattributed.Add(1)` 并 WARN（带上 `trace_id`、`cost`）。同时 MCP 路径改为：有 key 用真实 key；无 key 时拒绝（或显式标记 `unbilled_demo` 且不计入收入报表）。

---

### F9 · P2 · 账本与审计流水不可对账（删除不冲账 / 双整数镜像 / 迁移遗留）

**线上实测**（per-key 对账）

| key | 账本 `used_cost` | 账本 `used_cost_units` | 流水 `SUM(cost)` | 流水 `SUM(cost_units)` | 浮点缺口 | 整数缺口 |
|---|---|---|---|---|---|---|
| `sk-airoute-j9yh…` | 297.984531 | 297,984,530,600 | 295.337039 | 290,836,879,800 | **+2.647492** | **+7,147,650,800** |
| `sk-airoute-10d0…` | 96.898449 | 96,898,448,960 | 96.898449 | 96,898,448,960 | 0 | 0 |
| `sk-airoute-ce7c…` | 1.019242 | 1,019,241,800 | 1.019242 | 1,019,241,800 | 0 | 0 |

缺口可精确分解：`7,147,650,800 − 2,647,492,000 = 4,500,158,800` = **恰好等于 969 行 `cost_units=0` 的流水金额 ¥4.5002**。也就是说：

1. **整数缺口的一部分**：969 行历史流水（`cost>0` 且 `cost_units=0`，¥4.5002）只有浮点金额、没有整数镜像 → 任何以 `cost_units` 为准的报表会少算。
2. **浮点缺口 ¥2.6475**：账本多出、流水没有。`usage_logs` 的 id 序列 `148..max` **无任何空洞**（已用 `generate_series` 全量核对，0 missing），但 `id<148` 的 147 行缺失；147 × 均值 ¥0.0147 ≈ ¥2.16，量级吻合 → 早期测试期删除流水但**未回冲 `api_keys.used_cost`**。三个管理端点可删除流水（`DELETE /logs/:id`、`POST /logs/batch-delete`、`POST /logs/clear`，见 `router.go:236-238`）而**都不会调整账本**。
3. **双整数镜像**：`usage_logs` 同时存在 `cost_nano`（旧，525 行已回填，写入器不再写）与 `cost_units`（新）；`api_keys` 同理，`used_cost_nano=23.0794` 且 `used_cost_units=296.80`（**陈旧 13×**）。任何误读 `*_nano` 的代码会严重少算，是未来的埋雷（`migrations_postgres.go:450-453, 465-467`）。

好消息：`ReconcileMoney`（`internal/storage/money_reconcile.go:22-59`）**只读报表、不自动"修账"**，不存在"用流水重算余额导致二次扣/退"的风险。

**为什么是 P2（钱）**：不是直接漏收，而是**无法证明收得对**。对账缺口 + 无幂等键 + 可删流水三者叠加，使任何"客户说被多扣了"的争议都无法用数据裁决。

**最小修法**

1. 删除流水的三个端点改为**软删除**（`deleted_at`）或强制要求"仅删 `status_code>=400` 的行"，禁止删除 `cost>0` 行。
2. 迁移补 `cost_units`：`UPDATE usage_logs SET cost_units = ROUND(cost*1e9)::BIGINT WHERE cost>0 AND cost_units=0;`（969 行，金额可精确重建）。
3. 迁移后 `DROP COLUMN cost_nano / used_cost_nano`（或至少在代码里加 `// deprecated` 并从所有查询中移除），消灭双镜像。
4. 把 `ReconcileMoney` 接入每日巡检并对 `abs(difference)` 超阈值的 key 告警。

**2026-10-27 处置（迁移 v32 / v33）**

线上 ¥2.647492 的整数缺口已一次性冲销，不再作为永久基线：

- **v33 冲销**（`internal/storage/migrations_usage_log_cleanup.go`）在同一个 UPDATE 里同步减掉 `used_cost_units` 与 `used_cost`，`ledger_drift_units` 由 2,647,492,000 归零。冲销用指纹匹配（`id=1` 且 `used_cost_units − SUM(billing_settlements.cost_units) = 2,647,492,000`），其它部署匹配不到即 no-op；金额写入 `system_settings.billing_repair_f9_writeoff` 保留可解释性。用"减"而不是"按账本重算赋值"，是为了不吞掉并发结算进来的新扣费。
- `used_tokens` **有意未回写**：令牌确实消耗过，只是审计行被删，且没有可据以重建的账本（当前比 `SUM(usage_logs.total_tokens)` 高 8,730,494）。
- **v32 重建原因文案**：725 行 `%!w(<nil>)`（`fmt.Errorf("...%w", nil)` 的产物）与 51 行空原因的合成 499 得到可读说明；**状态码不变**（客户端当时确实收到那个码）。71 行空 502 与 4 行空 404 无可靠事实可重建，保持原样。
- 硬删端点如今只在流水未计费时才允许删除（`chargedUsageLogSQL`，`repo_logs.go:398-421`），B10 的"可删 `cost>0` 行"已不成立。
- **双 `*_nano` 列已删除**（迁移 v34 `drop_legacy_nano_money_mirrors`）：`users.balance_nano`、`api_keys.used_cost_nano`、`api_keys.budget_nano`、`usage_logs.cost_nano`。删前核对过：全仓无运行时读取（只有迁移 v12/v15 引用）、无索引/约束覆盖、所有 INSERT 均为显式列清单、镜像独有金额的行数为 0（迁移仍先回填一次再删，防止从旧 dump 恢复时丢数）。线上执行前另做了全库备份与事务内 dry-run，删后 `audit_units == settled_units` 且管理端读路径全部 200。F16 里"`used_cost_nano` 陈旧"一项随之关闭。

---

### F10 · P2 · 按"请求模型"计费而非"实际服务模型"，跨模型降级时错价

**证据**：所有计费调用点都用客户端请求的模型名，而不是实际服务模型：

- `common.go:61-69`（`params.Model`）
- `mcp_tools_routing.go:218`（`chatReq.Model`）

而调度器确实存在跨模型降级：`internal/router/dispatcher_fallback.go`（上限 4 跳、环检测齐全），`dispatcher.go:182, 280`、`dispatcher_stream.go:32, 176`。并且响应里的上游模型身份被覆盖为请求模型：`handler.go:110-112`（`resp.Model = req.Model`）→ 计费与审计都看不到真实服务模型。

**为什么是 P2（钱）**：当前 `model_fallbacks` 表**为空**（已查，0 行）→ 潜伏。一旦运营配置"A → B"降级：若 B 更贵（如 gpt-6 → gpt-6.1-sol），网关按 A 收费、按 B 付费，**静默亏损差价**；若 B 更便宜，则对客户多收。两条方向都错。

**最小修法**：降级发生时把实际模型放进上下文（如 `router.ContextKeyServedModel`），`recordUsage` 优先用它计价，并在 `pricing_snapshot` 里同时记录 `requested_model` 与 `served_model`（审计可见、可解释账单）。

---

### F11 · P3 · 原生 Anthropic 非流式把 cached token 按全价 prompt 计费（多收）

**证据**：`internal/api/inbound_anthropic.go:100-108` 的原生分支直接把 `resp.Usage.PromptTokens` 当 `PromptTokens` 传，**没有传 `CachedTokens`**：

```go
inputTokens, outputTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens
RecordUsage(c, AuditRecordParams{ ..., PromptTokens: inputTokens, ... }) // 无 CachedTokens
```

而引擎只有在 `CachedTokens` 传入时才会把 cached 部分按 `CacheReadPrice` 计价并从 prompt 里扣除（`engine.go:347-355`）。对照：同文件的适配分支 `:197-208` 正确传了 `cachedTokens`。

**影响**：cached token 按全价 prompt 计费。按默认价差异（2.0 vs 0.2，90%）→ 对客户**多收**。线上无原生 Anthropic 渠道（channels 只有 openai/gpustack 类型），故未发生。

**最小修法**：

```go
cached := resp.Usage.GetCachedTokens()
RecordUsage(c, AuditRecordParams{... PromptTokens: resp.Usage.PromptTokens, CachedTokens: cached ...})
```

---

### F12 · P3 · Anthropic 流式：cached 明细缺失 / cache_creation 平价

**证据**

- 原生流式把 `input_tokens + cache_read + cache_creation` 加总为 totalPrompt，同时只把 `cache_read` 记为 `CachedTokens`（`inbound_anthropic_stream.go:366-382`）→ `cache_creation` 按普通 prompt 价计费，而 Anthropic 对 cache write 收 **1.25×** → **少收 25%**（对 cache_creation 部分）。
- 适配流式的 `totalPromptTokens` 用 `max()` 累积（`inbound_anthropic_stream.go:341-343`），语义正确；但 `finalizeCompletion` 里用 `accumulatedCompChars/3`（`:75`）估算，与全局 `estimateTokens`（`/4`，`common.go:301-308`）**不一致** → 同一请求按不同端点估算口径不同（注释 `:301-304` 明确说要统一，这里漏了）。

**最小修法**：`cache_creation` 单独计一档价（沿用 `CacheReadPrice × 1.25` 或新增 `CacheWritePrice` 字段）；`/3` 改为 `estimateTokens(...)`。

---

### F13 · P3 · 流式 usage 帧"直接赋值"可能被 0 帧清零

**证据**：以下 3 处是"直接赋值"（后到的帧会覆盖先前的累计值）：`completions_stream_handler.go:91-92`、`gemini_inbound_stream.go:132-133`、`responses_stream.go:168-169`：

```go
if event.Chunk.Usage != nil {
    totalPromptTokens = event.Chunk.Usage.PromptTokens   // 直接覆盖
    totalCompTokens = event.Chunk.Usage.CompletionTokens
}
```

而 `chat_stream_handler.go:185-191` 与 `inbound_anthropic_stream.go:341-343` 用的是 `max()` 累积（并注释了原因："usage counters are cumulative; output-only trailers must not erase input/cache counts"）。若上游在中途/末尾发一个 `usage:{prompt_tokens:0,...}` 的帧，上述 3 处会把已累计的 token **清零** → 少收。

**最小修法**：统一改成 `max()` 累积（4 处，各 2 行）。

---

### F14 · P3 · off-peak 判定时点与时间解析

**证据**

- 计费用的时间点是**记账时刻**而非请求开始时刻：`common.go:68`（`time.Now()`）。跨越 08:30 边界的请求按结束时刻定档（半小时级偏差，影响很小但不确定）。
- 非法时间串被静默解析为 `0`：`strategy_validation.go:158-166`（`parseTimeToMinutes` 对 `"8:30"` 之外的脏数据返回 0）。在 `night` 模式下，`start==0 && end==0` 会被解释成 **00:00–08:30 折扣**（`strategy.go:40-43`）；在 `custom` 模式下同样有 `:116-119` 的兜底。即"配置写错 → 反而自动打折"。
- `offPeakStrategies` 缺 `"window"` 键（`strategy.go:165-169`），未知 mode 走 `WindowOffPeakStrategy`（`engine.go:307-311`）——行为可接受，但 `custom` 之外的拼写错误不会被拒绝。

**最小修法**：off-peak 用请求开始时间（`start` 传入 `recordUsage`）；`parseTimeToMinutes` 返回 `(int, error)`，保存价格时校验并拒绝非法值；保存时拒绝未知 `off_peak_mode`。

---

### F15 · P3 · 批内取整口径不一致

**证据**：批量写入时钱包/used_cost 用**聚合后**的 cost 取整一次（`repo_logs_writer.go:218`、`:257`、`:273`），而流水每行各自取整（`:211`、`:91`）。因此 `Δbalance_units` 可能比 `Σ cost_units` 差 `≤ n×1e-9` 元（每批 ≤100 行 → ≤1e-7 元）。单批可忽略，但会让 F9 的对账永远无法做到"精确相等"，也会随批量大小留下长期累积误差。

**最小修法**：批量路径改为按行取整后再求和（`Σ ToUnits(log.Cost)`），与流水保持同口径。

---

### F16 · P3 · 死代码 / 运维噪音

- **`DeductUserBalance` 无任何调用方**（`internal/storage/repo_users.go:223-230`，全仓 grep 仅定义）→ 容易让维护者误以为"存在独立扣费路径"。建议删除或在注释里明确标 `Deprecated: billing is applied inside RecordUsageLogContext`。
- **Redis consumer group 有 50 个僵尸 consumer**（`XINFO GROUPS airoute:stream:logs` → `consumers 50`，每次重启新增 `hostname-pid`，从不删除）。当前 `pending=0`、`lag=0` 所以不影响正确性，但会污染运维面板；建议启动时 `XGROUP DELCONSUMER` 清理超过 N 天无活动的 consumer。
- **`used_cost_nano=23.08` 与 `used_cost_units=296.80` 并存**（同一行两个数量级不同的"权威值"），见 F9。

---

### F17 · P3 · 历史流水缺少 `pricing_snapshot`，无法复核

12,610 行已收费流水（¥244.02，占计费额 62%）的 `pricing_snapshot=''`——这些行写于 snapshot 上线（2026-10-22 02:31Z）之前。**当前已 100% 覆盖**（10-22 03:00Z 之后 charged 行 `pricing_snapshot<>''` 全部非空，已用 SQL 按小时核实），所以不是持续缺陷；但历史部分无法用"当时的价目"复算，遇到退款争议只能依赖 `cost` 字段本身。

---

## 3. 已验证正确、**不需要**优化的部分（避免过度改造）

审计中特别验证了以下高风险点，结论是**当前实现正确**，无需返工：

| 检查项 | 结论 | 证据 |
|---|---|---|
| 引擎算术与落库金额 | **完全一致，0 误差** | 对 14,191 行有 snapshot 的已收费流水，用 `(uncached×pp + cached×crp + comp×cp)/1e6 + fp×units` 再乘 `group_multiplier` 与 `off_peak_discount` 重算，`SUM(abs(diff))=0.0000000000`、`max=0` |
| 8 位小数取整 | 正确 | `engine.go:413-417`；`roundPrecision` 在 cost/totalSaved/cacheSaved 三处都应用（`:401-403`） |
| cached token 不重复计费 | 正确 | `engine.go:347-355` 先 `cachedTokens=min(cached,prompt)`，再 `uncached = prompt - cached` 分开计价 |
| 分组乘数只应用一次 | 正确 | `engine.go:363-385`；组内 override（`p.GroupName == grp`）不再乘；有单测 `units_test.go:11-44` 覆盖 5 种组合 |
| 通配符最长前缀优先 | 正确 | `engine.go:109-114` 稳定排序（长度降序 + 字典序兜底），避免 map 随机遍历，`:233-260` 单次加锁不重入（注释明确写了死锁原因） |
| 时区 | 正确 | `strategy_validation.go:12-18` 缓存 `Asia/Shanghai`，`engine.go:291` 统一 `t.In(shanghaiLoc)` |
| 折扣钳制 | 正确 | `engine.go:281-289`：`discount<=0 → 0.5`、`>1.0 → 1.0`；slot 内 `d<=0||d>=1` 回落全局（`strategy.go:93-95`）；重叠校验在保存时执行（`pricing_service.go:193`，`ValidateSlotsOverlap` 支持跨零点） |
| 负数价格 | 被拒绝 | `pricing_service.go:159-186`（4 个价格字段全查）；`strategy_validation.go` 的注释也说明了"负数拒绝而非静默修复"的设计意图 |
| 双写（float + 整数 1e-9） | 所有写入点都成对写 | `repo_wallet.go:136,250`、`repo_users.go:186,204`、`repo_logs_writer.go:106/119/128/264/295`；线上实测 2 个用户的 `balance - balance_units/1e9` 漂移 = 0 |
| admin 豁免 | 正确、且不会被 tenant 回退误伤 | `repo_logs_writer.go:106` 的 `role != 'admin'`；`:114-118` 显式查询 `isOwnerAdmin` 并**跳过** tenant 回退；批量 `:277-286` 同逻辑 |
| API key 预算校验 | 正确（早期 bug 已修） | `auth.go:296-308`：`budget_units>0` 时以整数为准，否则回落浮点；注释记录了"两侧都必须为正才检查"的历史漏洞已修复 |
| Redis Stream 裁剪安全 | 正确 | `async_logger.go:378-393`：先 `MinPendingID`（group 级），无 pending 才 `TrimStreamBefore(ackedUpTo)`；不会裁掉未 ACK 的消息 |
| 价格变更跨副本传播 | ≤10s 内一致 | `controlplane/sync.go:171` 在 `ReloadFromDB` 中 `ReloadPrices()`；另有 Redis Pub/Sub 即时广播（`cmd/airoute/server.go:187`）+ 10s 周期兜底（`:198`） |
| `ReconcileMoney` 不会自动改账 | 安全 | `money_reconcile.go:22-59` 纯查询；线上无自动修复任务 |
| 失败请求不计费 | 正确（有意为之） | `common.go:58`；502/503/429/404/400/401 共 1,835 行 cost=0（无上游成本可收） |

---

## 4. 按"概率 × 金额"排序的量化结论

| 排名 | 发现 | 概率 | 单次金额 | 已观测损失 | 说明 |
|---|---|---|---|---|---|
| 1 | F1 重放重复扣款 | 低-中（重启/XACK 失败，group 已有 50 个 consumer 说明重启频繁） | ¥1.47–¥64/批 | 0（未检出重放行） | 唯一"多收客户"缺陷；不可检测 → 排第一 |
| 2 | F3 499 断连免单 | 中（正常超时）+ 可从恶意侧拉满 | 单次至整段 completion | **¥0.10 / 6 条 / 907k token** | 可被利用，说明必须修 |
| 3 | F5 默认价 + 默认 5 折 | 夜间/周末 100%；未配价模型 100% | 少收 50% 或 多收 2× | 110 行多收 ≈**¥0.42** | 今晚 00:00 就会开始少收 |
| 4 | F6 无 usage 不收费 | 中（渠道行为相关；已见 148 次） | 整条请求 | **¥2.2 / 148 条** | 已停止复发，但机制仍在 |
| 5 | F2 日志丢弃漏收 | 低（实测 0 次） | ¥1.47/批 | 0 | 设计风险 > 现实损失 |
| 6 | F4 透支 | 低-中（需余额低 + 并发/陈旧快照） | ≤单请求成本 × 并发 | 0（当前无负余额用户） | 无告警是主要问题 |
| 7 | F7/F8/F10 | 低（配置未启用） | 100% / 100% / 差价 | 0 | 潜伏，配置一变即生效 |
| 8 | F9 | 已发生 | 不可计量 | 账实差 ¥2.6475 + ¥4.5002 | 治理/对账问题 |
| 9 | F11–F17 | 低/微小 | < 1 分/请求 | — | 一致性打磨 |

**今天实际漏收合计 ≈ ¥2.7（占 ¥393.63 的 0.7%）**；**真正值得优先修的是会"多收/重复收"和"可被利用"的 F1、F3**——它们对收入的影响在真实付费流量下会被放大几个数量级，而当前 99.7% 流量是 admin 豁免，缺陷被完全掩盖了。

---

## 5. 建议的下一步（按优先级）

**立刻做（P0/P1，1–2 天）**

1. **F1 幂等键**：迁移加 `usage_logs.billing_id`（服务端生成）+ 部分唯一索引；单条与批量写入改为 `ON CONFLICT DO NOTHING`，并且**只有插入成功的行才累加钱包与 `used_cost`**。批量用 `RETURNING billing_id` 过滤聚合。这是唯一能同时解决"重复扣款"和"可对账"的改动。
2. **F3 断连计费**：`AuditRecordParams` 增加 `Billable bool`；`chat_stream_handler.go` / `inbound_anthropic_stream.go`（含原生）在 499 时对已消耗 token 计费（审计状态仍记 499）；顺手把 5 个流式实现的行为统一（`completions/gemini/responses` 已经会收，别改坏）。
3. **F5 默认价收紧**：把 `engine.go:44-57` 的 `OffPeakEnabled` 改为 `false`；加启动自检 "used models without price → WARN"；seed 改为逐条 `ON CONFLICT DO NOTHING` 补通配价。
4. **F6 非流式估算兜底**：5 个非流式 handler 在 `RecordUsage` 前调用统一的 `EnsureUsageFromEstimate`，标记 `usage_source='estimated'`。

**紧接着做（P2，1 周）**

5. **F2 兜底**：`Record()` 返回 bool，失败走同步写入 + 落盘；加 `usage_log_ack_failures_total`、`billing_unattributed_total` 指标。
6. **F9 账本治理**：删除流水端点改软删除；迁移回填 969 行 `cost_units`；删除 `*_nano` 旧镜像；`ReconcileMoney` 接日巡检告警。
7. **F7/F8 补口**：`/v1/moderations` 补计费 + 要求 `fixed_price>0`；写入器对"应扣未扣"打指标；MCP 无 key 时拒绝或明确标记不计费。
8. **F4 透支**：预留额度按预估 prompt 成本；扣款后 `balance_units<0` 触发锁户 + 告警。

**收尾（P3，随迭代）**

9. F11/F12 cached 计价修正；F13 统一 `max()` 累积；F14 用请求开始时间 + 时间串校验；F15 批内取整统一；F16 清理死代码/僵尸 consumer；F17 无需处理（已自然收敛）。

---

## 附录 A · 复现用只读 SQL（全部只读）

```sql
-- 1) 有账无流水/双镜像缺口
select k.key, k.used_cost, k.used_cost_units,
       (select coalesce(sum(cost),0) from usage_logs u where u.api_key=k.key) audit_float,
       (select coalesce(sum(cost_units),0) from usage_logs u where u.api_key=k.key) audit_units
from api_keys k;

-- 2) 成功但免费（无 usage）
select model, channel, count(*), sum(total_tokens)
from usage_logs where status_code=200 and cost=0 group by 1,2;

-- 3) 断连漏收
select id, model, prompt_tokens, completion_tokens, cost from usage_logs where status_code=499;

-- 4) 无主的已收费流水
select count(*), sum(cost) from usage_logs
where cost>0 and (api_key is null or api_key=''
   or not exists (select 1 from api_keys k where k.key=usage_logs.api_key));

-- 5) 整数镜像缺失（迁移遗留）
select count(*), sum(cost) from usage_logs where cost>0 and cost_units=0;

-- 6) 幂等键是否缺失
select indexname, indexdef from pg_indexes where tablename='usage_logs' and indexdef ilike '%unique%';

-- 7) 流水删除检测（id 空洞）
with g as (select generate_series((select min(id) from usage_logs),(select max(id) from usage_logs)) id)
select count(*) from g left join usage_logs u on u.id=g.id where u.id is null;

-- 8) 价格覆盖率
select distinct model from usage_logs
where model not in (select model from model_prices);
```

指标侧：`docker exec airoute-1 sh -c 'curl -s localhost:8080/metrics | grep usage_log_'`
→ `records_flushed_total=558`, `records_dropped_total=0`, `flush_failures_total=0`（airoute-2: 565 / 0 / 0）；
Redis：`XLEN airoute:stream:logs=5`、`XPENDING=0`、`XINFO GROUPS → consumers=50, pending=0, lag=0`。

## 附录 B · 未能在本次验证中确认的事项

1. **真实发生的重放次数**：没有 `billing_id`/幂等键，历史上是否已经发生过"commit+XACK 失败"的重放**无法从数据判定**（重放行与正常行在表里完全一样）。需要先落 F1 的幂等键才能开始观测。
2. **`id<148` 的 147 行被删原因**：无审计日志（`request_traces` 不含计费删除操作），只能由"金额量级吻合"推断为早期测试清理。
3. **上游真实成本**：无法拿到各渠道的账单，因此 F10（跨模型降级亏损）与 F5（默认价偏高/偏低）只能用"同系列价"近似，不能给出精确毛利。
4. **`model_fallbacks` 为空**，故跨模型降级路径只有代码级证据，无线上样本。
