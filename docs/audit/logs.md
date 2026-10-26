# Airoute 网关 —— 日志与实时指标健康审计

> 任务：监控日志，看是否有什么问题（task-3）
> 审计范围：`docker logs airoute-1/airoute-2`、`GET /metrics`（含两个副本直连）、PostgreSQL 只读查询、Redis INFO/Stream 查询
> 快照时间：**2026-10-23 10:09 – 10:16 (Asia/Shanghai)**（指标/日志/DB 于 10:16:11 原子采集），容器于 09:33:34 / 09:33:51 启动，日志窗口 `--since 6h` 已全覆盖（实际运行仅 ~43 分钟）
> 只读约束：未修改源码、未重启容器、未对数据库做任何写操作（全部 SELECT / INFORMATION_SCHEMA）

---

## 0. 结论速览（TL;DR）

| # | 结论 | 级别 |
|---|------|------|
| 1 | **10:03–10:11 发生一次约 9 分钟的压测型流量突发**，峰值 **319 req/s**（10:09:44），期间上游渠道 `罗总` 返回 429，熔断器抖动后 128 个请求以 **502 快速失败**（772ms 内）；**峰值分钟失败率 46%（342/739）**，整个两分钟突发窗口 51%（470/921）。10:11 起已完全自愈。 | P1（已恢复） |
| 2 | **熔断器 cooldown 失效**：`cooldown_sec=30`，但实测 TRIPPED→recovered 间隔仅 **6–20ms**，1 秒内 15 次 TRIPPED / 15 次 recovered，熔断器实际不提供保护，只是高频抖动。 | **P1** |
| 3 | **单渠道无冗余**：`model_routes` 中 `deepseek-v4.1-flash` 只有 1 个 provider（`total_channels=1`）。上游一限流＝全站 5xx，没有可切换的 fallback。 | **P0** |
| 4 | **管理凭据为文档默认值**：`GATEWAY_ADMIN_SECRET` 实测等于源码 `weakAdminSecrets` 中的占位符 `airoute-cluster-admin-shared-secret-key-32b`（md5 校验一致），且 admin 密码为 `admin123` → 管理令牌可被伪造。 | **P0** |
| 5 | **计费/日志管道核对无丢失**：1406 次 LLM 请求 = 1174 次计入费 + 231 次本地限流（不产生费用）+ 1 次 `/v1/models`；flushed 1173 = model 成功 934 + 失败 239，`dropped=0`、`flush_failures=0`、`queue_depth=0`、DB `trace_id` 无重复 → **无漏计费、无重复计费**。 | 健康 |
| 6 | 控制面健康：admin/console API **p50=3ms / p90=9ms / p99=19.1ms / max=39ms**，**0 个 ≥5s 请求**，无死锁、无锁等待。 | 健康 |
| 7 | 日志质量缺陷：**20.6% 的日志是每 10s 的 INFO 重复刷屏**（≈168k 行/天）、启动 banner 为非 JSON ANSI 彩色文本、router/breaker 日志**全部缺 trace_id**（0/891）无法与请求关联。 | P2 |
| 8 | `p99` 指标存在**系统性低估**：`estimateQuantile` 在 +Inf 桶回退为最后一档 `60000`，而真实观测 max = **113128ms**；`ttft` 仅覆盖 **24.3%** 请求（312/1285，仅流式子集）。 | P2 |

一句话总结：**服务本身没坏，管道没丢钱，但「单渠道 + 熔断器不生效 + 默认凭据」三件事叠加，把一次普通的上游限流放大成了 17% 的 5xx；同时日志的可观测性不足以在第一时间定位这条链路。**

---

## 1. 日志清单与噪声结构

### 1.1 体量

同一时刻（**10:16:11**）原子采样，运行 ~42.6 分钟：

| 指标 | airoute-1 | airoute-2 | 合计 |
|---|---|---|---|
| JSON slog 行 | 2506 | 2469 | **4975** |
| 非 JSON 行（启动 banner） | 5 | 5 | **10** |
| INFO | 2364 | 2351 | **4715（94.8%）** |
| WARN | 142 | 118 | **260（5.2%）** |
| ERROR / panic / deadlock | 0 | 0 | **0** |
| 日志速率 | 59.2 行/min | 58.0 行/min | ≈117 行/min → **≈168k 行/天** |

> 说明：`docker logs` 原始输出含 5 行非 JSON（banner），必须 `grep '^{'` 再交给 `jq`，否则解析直接失败。

### 1.2 按 category / msg 的分布（两副本合计，4975 行）

| category | 行数 | 占比 |
|---|---|---|
| system | 1966 | 39.5% |
| router | 1947 | 39.1% |
| sync | 518 | 10.4% |
| billing | 514 | 10.3% |
| breaker | 30 | 0.6% |

| msg | 行数 | 说明 |
|---|---|---|
| `http_access` | 1950 | 访问日志（结构完整，含 trace_id） |
| `attempting chat completion` | 770 | 路由尝试 |
| `channel execution succeeded` | 661 | 渠道成功 |
| `reloaded model pricing engine` | 514 | **噪声，10s 一次/副本** |
| `hot reloaded data plane memory state from database` | 512 | **噪声，10s 一次/副本** |
| `attempting streaming connection` | 277 | 流式请求（突发后流量以流式为主） |
| `circuit breaker is OPEN, bypassing dead upstream` | 127 | 告警（突发期） |
| `channel execution failed, triggering fallback` | 109 | 告警（突发期） |
| `circuit breaker recovered to closed state` | 15 | 熔断恢复 |
| `circuit breaker TRIPPED to OPEN state, bypassing provider` | 15 | 熔断触发 |
| `subscribed to Redis cluster invalidation channel` | 2 | 启动订阅 |

### 1.3 噪声量化

两条 INFO 心跳日志（`billing` / `sync`）实测间隔：**每条每 10s 一次，两副本各自独立** →
- 每副本 ≈ **12.2 行/min**，等于 `17,500 行/天/副本`、**35,000 行/天**（两副本）；
- 占全部日志的 **1026 / 4975 = 20.6%**（1.2 节两 msg 合计 1026 行）；
- 按当前速率外推：**≈168k 行/天** 总日志量，其中 1/5 无信息量。

`sync` 的 “hot reloaded data plane memory state from database” 还隐含**每 10s 每副本一次全量 DB 重载**（≈17,280 次/天/副本），是隐藏的数据库负载，不仅是日志噪声。

---

## 2. WARN / ERROR 事件全清单

**本次窗口共 260 条 WARN，0 条 ERROR。** 分布上可分为「启动期安全告警」与「突发期路由告警」两类。

### 2.1 启动期安全告警（每副本 3 条，共 6 条）

| 时间（+08） | level | category | msg | 含义 |
|---|---|---|---|---|
| 09:33:34.215 / 09:33:51.303 | WARN | system | `SECURITY WARNING: GATEWAY_ADMIN_SECRET is empty or a well-known placeholder (admin tokens would be forgeable)` | 管理密钥是源码内置占位符 → **JWT/管理令牌可被伪造** |
| 09:33:34.215 / 09:33:51.303 | WARN | system | `SECURITY WARNING: admin password is the default admin123 or shorter than 8 characters` | 控制台超管口令为 `admin123` |
| 09:33:34.535 / 09:33:51.608 | WARN | sync | `security notice: default admin credentials in use` | 实际已用默认口令登录/初始化（username=admin） |

**密钥比对结论（未打印明文）**：容器内 `GATEWAY_ADMIN_SECRET` 长度 43，md5 = `bd00d0b6…4ef7`，与源码 `cmd/airoute/security_check.go:11` 的 `weakAdminSecrets` 中 `airoute-cluster-admin-shared-secret-key-32b` **完全一致**。
→ 这是**随文档/示例清单一起发布的占位密钥**，任何拿到仓库的人都能伪造管理员令牌。**必须轮换**。

### 2.2 突发期路由/熔断告警（254 条，全部集中在 10:09:43–10:09:45）

| 时间 | msg | 次数 | 关键字段 | 含义 |
|---|---|---|---|---|
| 10:09:43.99 – 10:09:45.43 | `channel execution failed, triggering fallback` | 109 | `channel=罗总`、`error=upstream 罗总 returned status 429: {"error":"rate limit exceeded","type":"rate_limit_error"}`、`fallback_index=1` | 上游限流，触发 fallback；但只有 1 个候选渠道，fallback 无处可去 |
| 10:09:44.068–.943 | `circuit breaker TRIPPED to OPEN state, bypassing provider` | 15 | `provider=罗总`、`consecutive_failures=3`、`cooldown_sec=30` | 连续 3 次失败打开熔断 |
| 10:09:44.069–10:09:45.157 | `circuit breaker recovered to closed state` | 15 | `provider=罗总` | **在 cooldown 期内即「恢复」→ cooldown 未生效** |
| 10:09:44.080–.818 | `circuit breaker is OPEN, bypassing dead upstream` | 127 | `channel=罗总`（字段名不一致，无 `provider`/`cooldown_sec`） | 熔断打开，直接快速失败（这 127 条对应 128 个 502） |
| 10:09:44.86 | `circuit breaker is OPEN, bypassing dead upstream stream` | 1 | — | 流式路径同源问题 |
| 10:09:45.xx | `stream connection failed, trying next channel` | 2 | — | 流式切渠道失败 |

### 2.3 关键词扫描（排除误报）

| 关键词 | 命中 | 说明 |
|---|---|---|
| `panic` / `deadlock` / `rollback` / `insufficient_quota` / `quota exceeded` / `balance` | **0** | 无崩溃、无死锁、无额度/余额类错误 |
| `context deadline` / `reset by peer` / `connection refused` / `EOF` | **0** | 无网络层失败 |
| `timeout` | 2 | 仅启动日志里的 DSN `statement_timeout=30000`，非错误 |
| `ERROR` 级别 | **0** | 全链路失败也只有 WARN —— 见 §8 日志质量 |
| 敏感数据（`sk-` / `Bearer` / `api_key=` / `Authorization`） | **0** | 日志未泄漏密钥；`admin123` 仅出现在告警文案本身；DB 连接串密码已在启动日志中正确脱敏为 `postgres:5432/airoute?options=…` |

---

## 3. HTTP 流量与延迟分析

数据源：`http_access` 结构化行（10:16 快照 1950 条，两副本 1000 + 950），字段 `trace_id / method / path / status / latency_ms / ip / bytes`。

### 3.1 每分钟吞吐（两副本实测合计 = 真实到达网关的速率）

| 分钟 | 请求 | 200 | 429 | 502 | | 分钟 | 请求 | 200 | 429 | 502 |
|---|---|---|---|---|---|---|---|---|---|---|
| 09:33 | 4 | 4 | 0 | 0 | | 10:00 | 5 | 5 | 0 | 0 |
| 09:34 | 9 | 9 | 0 | 0 | | 10:01 | 5 | 5 | 0 | 0 |
| 09:35 | 10 | 10 | 0 | 0 | | 10:02 | 5 | 5 | 0 | 0 |
| 09:36 | 7 | 7 | 0 | 0 | | 10:03 | 13 | 13 | 0 | 0 |
| 09:37 | 13 | 13 | 0 | 0 | | 10:04 | 38 | 38 | 0 | 0 |
| 09:38 | 19 | 19 | 0 | 0 | | 10:05 | 106 | 106 | 0 | 0 |
| 09:39 | 21 | 21 | 0 | 0 | | 10:06 | 65 | 65 | 0 | 0 |
| 09:40 | 31 | 31 | 0 | 0 | | 10:07 | 66 | 66 | 0 | 0 |
| 09:41 | 9 | 9 | 0 | 0 | | 10:08 | 266 | 266 | 0 | 0 |
| 09:42 | 37 | 37 | 0 | 0 | | **10:09** | **739** | **397** | **214** | **128** |
| 09:43 | 20 | 20 | 0 | 0 | | 10:10 | 182 | 54 | 128 | 0 |
| 09:44 | 16 | 16 | 0 | 0 | | 10:11 | 28 | 28 | 0 | 0 |
| 09:45 | 8 | 8 | 0 | 0 | | 10:12 | 18 | 18 | 0 | 0 |
| 09:46 | 29 | 29 | 0 | 0 | | 10:13 | 20 | 20 | 0 | 0 |
| 09:47 | 23 | 23 | 0 | 0 | | 10:14 | 29 | 29 | 0 | 0 |
| 09:48–09:59 | 5–10/min | 100% | 0 | 0 | | 10:15 | 15 | 15 | 0 | 0 |
| | | | | | | 10:16 | 6 | 6 | 0 | 0 |

**基线**：空闲时 5–10 req/min（控制台轮询 + 偶发对话）。
**突发**：10:05 起爬升（106）→ 10:08（266）→ **10:09（739，峰值 319 req/s）** → 10:10（182，含 128 次重试）→ 10:11 起回落、**10:11 之后 100% 200，无任何告警**。

### 3.2 路径分组延迟分位数（单位 ms，实测日志）

| 路径分组 | n | p50 | p90 | p99 | max | ≥5s | ≥30s | 错误率 |
|---|---|---|---|---|---|---|---|---|
| **LLM `/v1/chat/completions`** | 1128 | 918.0 | 4300.4 | **60669.8** | **113128** | 68 | 26 | **41.4%** |
| **LLM `/v1/messages?beta=true`**（流式） | 277 | 6694.0 | 14611.8 | 28119.7 | 62332 | 172 | 3 | 1.1% |
| **admin/console API**（`/api/v1/admin/*`） | 490 | **3.0** | **9.0** | **19.1** | **39** | **0** | **0** | **0.0%** |
| user API（`/api/v1/user/*`, `/api/v1/announcement`） | 50 | 1.0 | 4.0 | 8.5 | 9 | 0 | 0 | 0.0% |
| static/UI（`/`, `/assets/*`） | 4 | 0.0 | 2.1 | 2.9 | 3 | 0 | 0 | 0.0% |
| LLM `/v1/models` | 1 | 4.0 | — | — | 4 | 0 | 0 | 0.0% |

**结论（回答任务问题 2）**：
- **控制面完全健康**：admin/console API 最慢 39ms，**不存在任何 ≥5s 的管理/控制台请求**，无控制面阻塞。
- **数据面长尾极重**：非流式 `/v1/chat/completions` p99 = **60.7s**、max = **113.1s**；68 个请求 ≥5s、26 个 ≥30s。上游（`罗总`）本身慢，网关没有有效超时收敛（`write_timeout_sec: 600`，见 §9）。
- 流式 `/v1/messages` p50 6.6s 属「整条 SSE 流」耗时，指标口径与 `latency_ms` 混用，不可与非流式直接比较。

### 3.3 按状态码看延迟（LLM 路径）

| status | n | p50 | p90 | max | 解读 |
|---|---|---|---|---|---|
| 200 | 936 | 3663 | 10958 | 113128 | 正常但长尾极重 |
| 429 | 342 | **1** | 51 | 63 | 本地限流器 1ms 内拒绝（231 个）／上游 429 约 42ms 返回（111 个），**两者混在同一状态码** |
| 502 | 128 | **2** | **2** | **3** | **2ms 内失败** → 熔断 OPEN，根本没发起上游调用 |

### 3.4 状态码 × 路由

| 路由 | 200 | 429 | 502 |
|---|---|---|---|
| `/v1/chat/completions` | 661 | 340 | 127 |
| `/v1/messages?beta=true` | 274 | 2 | 1 |
| `/api/v1/admin/models` | 115 | — | — |
| `/api/v1/admin/pricing` | 115 | — | — |
| `/api/v1/admin/stats/overview` | 111 | — | — |
| `/api/v1/admin/keys` | 111 | — | — |
| `/api/v1/announcement` | 42 | — | — |
| `/api/v1/admin/logs?…` | 38 | — | — |
| `/api/v1/user/wallet` | 8 | — | — |
| 其他（静态/healthz/models） | 7 | — | — |
| **合计** | **1480** | **342** | **128** |

> 注：128 个 502 中 127 个落在 `/v1/chat/completions`、1 个落在 `/v1/messages`；429 共 342 个（340 + 2）。

### 3.5 最慢 10 个请求

| 时间（+08） | status | latency_ms | path | 响应体 |
|---|---|---|---|---|
| 09:46:28.411 | 200 | **113128** | /v1/chat/completions | 91840 B |
| 09:46:40.045 | 200 | 90410 | /v1/chat/completions | 61485 B |
| 10:06:20.910 | 200 | 90370 | /v1/chat/completions | 64367 B |
| 09:40:24.489 | 200 | 89506 | /v1/chat/completions | 64619 B |
| 10:06:56.726 | 200 | 81485 | /v1/chat/completions | 63507 B |
| 09:40:15.273 | 200 | 80864 | /v1/chat/completions | 59783 B |
| 10:06:04.954 | 200 | 75105 | /v1/chat/completions | 64087 B |
| 10:07:19.468 | 200 | 74504 | /v1/chat/completions | 56977 B |
| 09:35:04.537 | 200 | 72802 | /v1/chat/completions | 61353 B |
| 09:45:47.440 | 200 | 72156 | /v1/chat/completions | 50354 B |

全部为成功请求（200），全部是大响应体（50–90KB）的补全 —— 慢在**上游生成**而非网关。

---

## 4. 502/503 突发专项分析：突发流量 → 上游限流 → 熔断抖动（10:03–10:11）

### 4.1 秒级时间线（两副本合并）

| 时刻 | 请求 | 200 | 429 | 502 |
|---|---|---|---|---|
| 10:09:40 | 39 | 39 | 0 | 0 |
| 10:09:41 | 4 | 4 | 0 | 0 |
| 10:09:42 | 8 | 8 | 0 | 0 |
| 10:09:43 | 30 | 29 | **1** | 0 |
| **10:09:44** | **319** | **27** | **164** | **128** |
| 10:09:45 | 52 | 6 | **46** | 0 |
| 10:09:46 | 14 | 14 | 0 | 0 |
| 10:09:47 | 19 | 19 | 0 | 0 |
| 10:09:48+ | 正常 | 100% | 0 | 0 |

- **429 时间跨度**：`10:09:43.990` → `10:09:45.430`（1.4s）
- **502 时间跨度**：`10:09:44.082` → `10:09:44.854`（**772ms**，128 个）
- **10:10:04 二次重试波**：128 个请求再次到达，全部被**本地限流器**以 429 拒绝（无上游调用、无计费行）
- **10:11 之后至快照时刻（>10 分钟）**：100% 200，无熔断事件 → **已完全恢复，熔断处于 CLOSED**

### 4.2 失败链（实测日志与 DB 双证据）

```
10:09:11 负载开始爬升（15 req/s）—— 请求量持续超上游配额
   │
10:09:43 上游渠道 罗总 返回 429 rate_limit_error    ← usage_logs 中 111 行 status=429 可证
   │        （错误原文：upstream 罗总 returned status 429: {"error":"rate limit exceeded",...}）
10:09:44 连续 3 次失败 → 熔断器 TRIPPED OPEN（15 次）
   │        但 6–20ms 后立刻 recovered（cooldown_sec=30 被无视）→ 抖动 15 轮
10:09:44 熔断 OPEN 期间到达的 128 个请求 → 直接 502（p50 = 2ms，未发起上游调用）
   │        DB 证据：128 行 status_code=502, cost=0, error_message =
   │        "every candidate channel for model deepseek-v4.1-flash is temporarily disabled
   │         by the circuit breaker, so no upstream was called (skipped: 罗总)"
10:09:45 上游 429 继续返回 46 次
10:09:56 / 10:10:04 重试涌入 → 被本地 RPM 限流器拒绝 3 + 128 次 429
10:11+    流量回落，全部 200
```

### 4.3 「是上游故障还是持续性问题」—— 判定

| 判据 | 结论 |
|---|---|
| 时间聚集性 | **极强**：128 个 502 全部落在 772ms 内；429 全部落在 1.4s 内 |
| 是否持续 | **否**：10:11 起至快照（>10 分钟）零 5xx、零熔断 |
| 涉及渠道/模型 | 单一渠道 `罗总`；模型 `deepseek-v4.1-flash`（`total_channels=1`） |
| 根因 | **流量突发打满唯一上游渠道的速率配额（429）**，非上游宕机、非网关资源耗尽 |
| 放大因素 | ① 单渠道无 fallback；② 熔断 cooldown 不生效导致抖动而非稳定快速失败；③ 本地限流器与上游 429 返回同一状态码，无法区分 |

### 4.4 历史对照（usage_logs 只读聚合，全量 28,749 行）

| 日期 | 502 | 503 | 当日总行数 | 当日错误率 |
|---|---|---|---|---|
| 2026-10-12 | 1 | 0 | 203 | 0.5% |
| 2026-10-16 | 3 | 0 | 55 | 5.5% |
| 2026-10-19 | 9 | 0 | 2627 | 0.3% |
| 2026-10-20 | 1 | 0 | 2272 | 0.04% |
| 2026-10-21 | 57 | 0 | 6370 | 0.9% |
| **2026-10-22** | **941** | **480** | 16002 | **8.9%** |
| **2026-10-23（今日）** | **157** | **0** | 1215 | 12.9% |

- **历史 5xx 同样高度聚集**：10-22 有 1,421 次 5xx（16:00–19:48 时段，449 次 503 带 `channel=罗总`），是比今天更大的一次同类事故；今天是同一模式的复现。
- **503 观察不到**：本次窗口 0 个 503。全量 503（480）全部来自 10-22 的「渠道执行失败」路径；今天的失败走的是「熔断 OPEN 快速失败」路径并返回 **502**。
  → **同一语义（无可用上游）返回两种状态码（502/503），语义不一致**，客户端无法统一重试策略。建议统一为 **503 + `Retry-After`**。
- Lead 快照的 502=1041 → 现在 1169，**增量 +128 恰等于本次突发**，无其他隐藏失败。

---

## 5. 计费 / 日志管道核对（是否有漏账）

方法：以 `trace_id` 为键，把 `http_access` 日志与 `usage_logs` 表做**逐请求精确 join**（两者都带 trace_id）。

### 5.1 对账矩阵（10:16 快照：1406 个 LLM 请求 vs 1196 行 DB 记录）

| HTTP 状态 | 在 DB 中有行 | 在 DB 中无行 | 说明 |
|---|---|---|---|
| **200** | 935 | **1** | 唯一无行的是 `GET /v1/models`（元数据接口，本就不计费）→ **正常** |
| **429** | 111 | **231** | 有行=上游 429（真实发生上游调用）；无行=**本地限流器拒绝**（未调用上游、无额度消耗）→ **正常** |
| **502** | **128** | 0 | 全部落库，`cost=0`、`error_message` 已填 → **正常** |

**重复计费检查**：1196 行 DB 记录对应 1196 个唯一 `trace_id`，**重复数 = 0** → 无双重计费。

**完整性核对（等式闭合）**：
- 请求侧：`1406 = 935（计费 200）+ 1（/v1/models 不计费）+ 111（上游 429 计费）+ 231（本地限流 429 不计费）+ 128（502 计费）` ✅
- 落库侧：`1196 = 935 + 111 + 128 + 22`，其中 22 行 = **18 行在日志快照之后约 20s 才 flush**（均为 200，无异常）+ **4 行属于重启前旧进程**（`created_at` 09:33:02–09:33:20，早于本容器启动 09:33:34）→ **账实完全相符，无漏账**

### 5.2 管道计数器（两副本 `/metrics` 直连，权威值）

| 计数器 | airoute-1 | airoute-2 | 合计 | 判定 |
|---|---|---|---|---|
| `requests_total` | 1058 | 1012 | 2070 | — |
| `usage_log_records_flushed_total` | **591** | **582** | **1173** | 与 DB 行数（1196，含 22 行时差）吻合 |
| `usage_log_records_dropped_total` | **0** | **0** | **0** | ✅ 无丢弃 |
| `usage_log_flush_failures_total` | **0** | **0** | **0** | ✅ 无批量写失败 |
| `usage_log_queue_depth` | **0** | **0** | **0** | ✅ 无积压 |
| `model_requests_total{success}` | 458 | 476 | 934 | 934 + 239 = **1173 = flushed** ✅ |
| `model_requests_total{failed}` | **133** | **106** | **239** | 239 = 111(429) + 128(502)，事故后**不再增长** ✅ |
| `fallback_total` | 43 | 68 | 111 | 与 fallback 日志条数一致 |

**结论：日志→Redis→DB 全链路 0 丢失、0 失败、0 积压、0 重复。管线本身健康，没有因本次突发产生漏账。**

补充说明：同一时刻 `requests_total`(2070) 比 `http_access` 行数(1950) 多 **120 行（≈5.8%）**，差额主要是 **`/health` 探针**（Docker healthcheck 每 10s 一次，`curl -f /health`）—— 该路径**完全不进访问日志**（见 §8）。

### 5.3 计费数据质量

| 检查项 | 结果 |
|---|---|
| 失败行 `cost` | 429/502 全部 `cost=0.000`（不扣费）✅ |
| 失败行 `error_message` | 128 + 111 = 239 行，**缺失 0 行** ✅ |
| 失败行 `channel` | **502 的 128 行 `channel` 为空**，全量 1169 个 502 中 1126 行 channel 为空 → 失败无法归因到渠道 ❌（见 §8） |
| 全量 502 的 `model` | 均有 `deepseek-v4.1-flash`，但 channel 空 → 只有 model 维度可查 |
| `channel` 取值规范 | 出现 `罗总→罗总`（31 行，带 fallback 链符号）与 `罗总` 两种格式，**字段格式不统一** ❌ |
| 金额 | 本次重启后累计 `cost` = 9.8378；全量 390.09+（与 Lead 快照口径一致） |
| 重复计费 | 0（trace_id 唯一）✅ |

### 5.4 Redis 日志流（XADD 管道）

| 指标 | 值 | 判定 |
|---|---|---|
| `XLEN airoute:stream:logs` | **9** | 消费极快，无积压 |
| 消费组 `airoute_log_group` lag | **0** | ✅ |
| `XPENDING` | **0** | ✅ 无未确认消息 |
| `entries-read` | **27154** | 累计已消费 |
| consumers | **50** | ⚠️ 仅 2 个活跃，**48 个僵尸**（见 §7） |

---

## 6. PostgreSQL 健康

版本 PostgreSQL 16.15（alpine）。全部为只读查询。

### 6.1 连接与负载

| 指标 | 值 | 判定 |
|---|---|---|
| `max_connections` | 100（保留 3 给 superuser） | — |
| 当前连接（airoute） | **40** = 39 idle + 1 active | 40% 占用 |
| `idle in transaction` | **0** | ✅ 无悬挂事务 |
| 最长 state 年龄 | 2m17s（idle） | ✅ |
| 两副本连接池配置 | `SetMaxOpenConns(50)` × 2 = **100 = max_connections** | ⚠️ **零余量**，任一副本扩容/重连风暴即打满 |
| `application_name` | 39 个连接**全为空** | ⚠️ 无法区分来自哪个副本/服务 |
| 慢查询（`state<>'idle'`） | 采样时仅 1 条（审计自身的 SELECT） | ✅ 无长事务 |
| `pg_stat_statements` | **未安装** | ⚠️ 无法做语句级耗时分析 |

### 6.2 库级指标（`pg_stat_database`）

| 指标 | 值 | 判定 |
|---|---|---|
| deadlocks | **0** | ✅ |
| conflicts | **0** | ✅ |
| temp_files / temp_bytes | 6 / 3.6 MB | ✅ 无大排序溢出 |
| xact_commit / xact_rollback | 766,064 / **28,760** | 回滚率 **3.76%**（偏高但多为应用层错误路径回滚） |
| blks_read / blks_hit | 2,428 / 18,112,491 | **缓存命中率 99.99%** ✅ |
| 数据库总大小 | 54 MB | ✅ |

### 6.3 表 / 索引

| 表 | 总大小 | heap | 索引 | 活行 | 死行 |
|---|---|---|---|---|---|
| **usage_logs** | **42 MB** | 13 MB | **29 MB** | 28,750 | 0 |
| request_traces | 3.3 MB | 1.8 MB | 344 kB | 1,659 | 0 |
| 其他（skills/keys/users/…） | < 200 kB | — | — | — | — |

**索引膨胀问题（明确可优化）**：`usage_logs` 共 **20 个索引，29MB（占表 69%）**，其中 6 个 trigram GIN 索引 **`idx_scan = 0`（从未被使用）**：

| 索引 | 大小 | idx_scan |
|---|---|---|
| `idx_usage_trgm_chat_id` | 5,376 kB | **0** |
| `idx_usage_trgm_trace_id` | 4,952 kB | **0** |
| `idx_usage_trgm_session_id` | 2,696 kB | **0** |
| `idx_usage_trgm_model` | 1,320 kB | **0** |
| `idx_usage_trgm_error_message` | 1,176 kB | **0** |
| `idx_usage_trgm_channel` | 264 kB | **0** |
| 合计 | **≈15.8 MB** | 0 |

→ 约 16MB 纯写入放大的无用索引，直接拖慢每行 INSERT（突发期 319 req/s 下更明显）。**建议：删除或在确认模糊搜索需求前改为 `pg_trgm` 之外的部分索引 / 仅保留 `trace_id`。**

### 6.4 自动清理与保留策略（是否无限增长）

**保留策略存在且已接线**：`cmd/airoute/server.go:173` → `storage.InitUsageLogRetention(repo)`；实现于 `internal/storage/usage_retention.go`。

| 参数 | 默认值 | 运行环境 |
|---|---|---|
| 保留天数 | **365 天** | 未设置 `GATEWAY_USAGE_LOG_RETENTION_DAYS` → 取 365 |
| 清理周期 | 6 小时 | 同上（未覆盖） |
| 单批删除上限 | 5,000 行 | 固定 |
| 启动延迟 | 5 分钟 | 固定 |
| 清理写法 | `DELETE … WHERE id IN (SELECT id … LIMIT 5000)` 分批 | 有界，友好 |

**风险点**：
1. 单次最多删 5,000 行 × 4 次/天 = **上限 20,000 行/天**，而近 5 日平均写入 **5,707 行/天**、峰值单日 **16,002 行**（10-22）—— 峰值日已逼近清理上限，**回补/历史积压时永远追不上**。
2. 365 天 × 7,000 行/天 ≈ 2.5M 行 ≈ **3.7 GB**（按 1.46 KB/行，含索引）—— 空间可接受，但索引占比 69% 会进一步放大。
3. `usage_logs` 上次 autovacuum = **10-22 09:03 UTC（约 25h 前）**，`n_dead_tup = 0`、`autovacuum_count = 9` → 当前无需清理（无更新/删除），**但没有针对大批量 DELETE 的 vacuum 策略**，将来清理后需关注膨胀。

**增长速率**：`usage_logs` 近 5 日 **平均 5,707 行/天**，10-22 峰值 16,002 行。当前 42MB，索引 29MB。

---

## 7. Redis 健康

版本 7.4.11，uptime 1 天。

| 指标 | 值 | 判定 |
|---|---|---|
| `used_memory` | **2.98 MB**（峰值 20.79 MB） | ✅ 低 |
| `maxmemory` | **0（不限制）** | ⚠️ 无上限保护 |
| `maxmemory-policy` | **noeviction** | ⚠️ 内存耗尽时**写入直接报错**（XADD 失败 → 日志丢失） |
| `connected_clients` | 80 | ✅（PoolSize=128 每副本，未打满） |
| `blocked_clients` | 2 | ✅ 正常（XREAD BLOCK 消费者） |
| `rejected_connections` | 0 | ✅ |
| keyspace | db0 **7 个 key**，4 个带 TTL | ✅ |
| `evicted_keys` | **0** | ✅ 无驱逐 |
| `expired_keys` | 175 | ✅ 正常 |
| `keyspace_hits / misses` | 853,230 / **109** | **命中率 99.99%** ✅ |
| `instantaneous_ops_per_sec` | 9 | ✅ |
| AOF | 开启，`appendfsync everysec`，`aof_current_size` **37.2 MB**（base 6.4 MB），`aof_last_write_status: ok` | ⚠️ 无 rewrite 进行中 |
| `rdb_changes_since_last_save` | 386,512 | ⚠️ 距上次 RDB 保存已积累较多 |
| 流长度 / pending / lag | 9 / 0 / 0 | ✅ 见 §5.4 |

**问题 1 —— 消费者泄漏（P3）**：`XINFO GROUPS` 显示 **50 个 consumer**，但只有 2 个对应当前副本；其余 48 个 `idle` 高达 **91,663,795 ms ≈ 25.5 小时**（即上一批容器生命周期）。consumer 名字形如 `070fd41b1bf8-1`（容器 hostname 派生），**每次重启都新增而从不清理**。当前 `pending=0` 无实际影响，但会持续膨胀，且掩盖真实消费端数量。

**问题 2 —— 跨栈共享 Redis（P2）**：同一 Redis 实例中存在 `nano:stream:logs`（**1,749 条**），属于 **nanorag 技术栈**。即两个不相关的应用共用同一 Redis 且 `maxmemory=0 + noeviction` —— 任何一方写入暴涨都可能让另一方 XADD 失败（进而丢日志/丢限流状态）。key 前缀虽已隔离，但**资源与故障域未隔离**。

key 清单：`airoute:live:2b14daf9d35f`、`airoute:live:a6ea57ca0601`、`airoute:cluster:config_version`、`airoute:stream:logs`、`nano:stream:logs`，以及突发期的限流 zset `airoute:ratelimit:rpm:sk-airoute-…`（453 members，带 TTL）。

---

## 8. 日志质量问题与精确修复

| # | 问题 | 证据 | 精确修复 |
|---|---|---|---|
| L1 | **启动横幅为非 JSON 的 ANSI 彩色多行文本**，破坏日志管道（`jq` 直接报错） | 每副本 5 行：空行 + `\x1b[1m\x1b[32m🚀 Airoute 网关已成功就绪\x1b[0m` + 2 行带 `\x1b[36m` 的地址 + 空行 | ① 改为结构化：`slog.Info("gateway ready", "addr", ..., "console", ...)`；② 装饰性横幅仅在 `os.Stdout` 是 TTY 时输出（`term.IsTerminal`），或提供 `--no-banner` / `AIRoute_NO_BANNER=1`；③ **绝不要写 ANSI 转义到 stdout 日志流** |
| L2 | **每 10s 的 INFO 刷屏占 20.6%**，且掩盖真实事件 | `reloaded model pricing engine` 514 行 + `hot reloaded data plane memory state from database` 512 行（每副本各 12 行/min） | ① 降到 `DEBUG`；② 或在 `internal/controlplane/sync.go` 中比较内容指纹（`sha256` / `config_version`），**仅在实际变化时打 INFO**（Redis 已有 `airoute:cluster:config_version`）；③ 两条合并为一条 `config reloaded` 并附 `version/changed=bool`；④ 顺带把 10s 全量 DB 重载改为**按版本号轮询**，减少无谓 DB 压力（≈17k 次/天/副本） |
| L3 | **router / breaker 日志 100% 缺 `trace_id`**，突发时无法把 502 与失败尝试关联 | `channel execution failed` 109 条、`breaker TRIPPED` 15 条、`attempting chat completion` 767 条、`channel execution succeeded` 658 条 —— 带 trace_id 的数量均为 **0**；而 `http_access` 1950/1950 全有 | 在这些 `telemetry.ScopedLogger(...)` 调用中补 `"trace_id", middleware.TraceIDFromContext(ctx)`（日志器已在请求上下文中）；最小改动是给 `slog` 传 `ctx` 并使用 `slog.With("trace_id", …)` |
| L4 | **`http_access` 无错误字段**：502/429 行与 200 行结构相同，事后无法从访问日志判断失败原因 | 字段固定为 `time/level/msg/category/trace_id/method/path/status/latency_ms/ip/bytes` | 补 `error_kind`（`upstream_rate_limit` \| `local_rate_limit` \| `all_channels_breaker_open` \| `upstream_5xx`）、`upstream_channel`、`fallback_count`、`is_stream`、`ttft_ms` |
| L5 | **本地限流与上游限流都用 429，无法区分** | 342 个 429 中 111 个有计费行（上游）、231 个无计费行（本地）；访问日志两者完全同构 | 增加 `reject_source="local_ratelimit\|upstream"` 与 `retry_after_ms`；本地限流建议返回 429 + `X-RateLimit-*` 头 |
| L6 | **失败记录的 `channel` 为空**，失败无法归因渠道 | DB：全量 502 共 1169 行，其中 **1126 行 channel 为空**（熔断快速失败路径未记候选渠道）；而 429 行有 `channel=罗总` | 在 `internal/api/common.go` 落库前，把 `attempted_channels` / `last_channel` 写入 `channel`（或新增 `channel_chain` 列）；熔断路径也带上 `skipped: [罗总]`（该信息目前只在 `error_message` 文本里） |
| L7 | **熔断日志字段不一致**，无法告警 | `TRIPPED`：`provider + consecutive_failures + cooldown_sec`；`is OPEN`：只有 `channel`（无 provider/cooldown）；`recovered`：无任何数值 | 统一为 `provider、state、consecutive_failures、open_duration_ms、cooldown_sec` 五个字段；`recovered` 必须带 `open_duration_ms` |
| L8 | **`/health` 探针完全不入访问日志**，探针失败在日志中不可见 | `grep '"/health"'` = 0；38 分钟内 Docker 每 10s 探活 ≈228 次/副本；`requests_total`(2070) 与 access 行(1950) 差 ~120 | 要么把 `/health` 记为 DEBUG 级访问日志，要么**明确配置排除列表**并在文档里写明；监控上给 `/health` 单独指标（目前它不计入 `requests_total` 口径的透明度不足） |
| L9 | **全链路失败仍只有 WARN，无 ERROR** | `channel execution failed` 是 WARN；只有 `ERROR` 级别从未出现 | 分级：单渠道失败=WARN；**全部候选渠道失败/熔断全开=ERROR**（便于按级别做告警与过滤） |
| L10 | **水平指标口径缺陷**：p99 被硬性钳到 60000；TTFT 覆盖率仅 17.5% | `estimateQuantile` 在最后一个有限桶后直接 `return buckets[len-1]`(=60000)（`internal/telemetry/metrics.go:144`），而实测 max=113128；`ttft_count` = 148+164 = **312** vs model 请求 1285（**24.3%**，且指标无 `is_stream` 标签区分）；TTFT 桶最大 5000 | ① 增加 `+Inf` 的真实值或更高桶并令 `p99` 在越界时取 `max`；② TTFT 只对 `is_stream && 有首 token` 的请求统计，并在指标名上体现（`ttft_streaming_*`），同时输出 `ttft_samples_total` 让使用者知道覆盖率 |
| L11 | **未启用 `pg_stat_statements`** | 扩展不存在 | 启用扩展 + `shared_preload_libraries`（只读审计无法做；建议运维加） |
| L12 | 敏感数据 | 日志中 `sk-`/`Bearer`/`api_key=`/`Authorization` 命中 **0**；启动日志 DSN 已脱敏（仅 `postgres:5432/airoute?options=…`）✅ | 保持；另注意 `docker inspect` 可见 `DATABASE_URL` 明文口令（环境变量注入，非日志问题），建议改用 secrets 挂载 |

---

## 9. 优先级异常清单

### P0 —— 立即处理

1. **管理密钥为公开占位符 + 默认口令**
   - 证据：`GATEWAY_ADMIN_SECRET` md5 与源码 `weakAdminSecrets` 中 `airoute-cluster-admin-shared-secret-key-32b` 一致；WARN 明示 “admin tokens would be forgeable”；口令 `admin123`。
   - 影响：任何人可伪造管理员 JWT、直接登录控制台 → 改价、改渠道、读全量用量数据。
   - 修复：轮换 `GATEWAY_ADMIN_SECRET` / `GATEWAY_ADMIN_PASSWORD` 为强随机值；生产模式下**启动即 fail-fast**（当前仅 WARN 后继续运行，属可被忽略的风险）。

2. **`deepseek-v4.1-flash` 只有 1 个上游渠道，无 fallback**
   - 证据：`model_routes.providers` 仅 1 项（`channel_id=4`）；日志 `total_channels=1`、`fallback_index=1` 后无更多候选；DB 中 111 次 429 全部 `channel=罗总`。
   - 影响：上游一限流 = 全站对话接口 5xx（本次 128 × 502）。
   - 修复：为该模型配置 ≥2 个渠道（不同 provider/额度），并让 fallback 在 429 时真正生效（当前 fallback 链长度只有 1）。

### P1 —— 高

3. **熔断器 cooldown 未生效，导致抖动而非保护**
   - 证据：`TRIPPED(cooldown_sec=30)` 与 `recovered` 成对出现，间隔 **6–20ms**；1 秒内 15 组；恢复后立即再次冲击限流上游。
   - 影响：熔断既没起到快速失败保护，也没给上游恢复时间；每次「恢复」都再打一次 429。
   - 修复：`recovered` 只在 cooldown 到期后由 half-open 探针触发；half-open 只放行 1 个探测请求；补 `open_duration_ms` 日志（见 L7）。

4. **失败语义不一致：同一类失败返回 502 与 503 混杂**
   - 证据：今日熔断全开 → **502**（128）；10-22 渠道执行失败 → **503**（480）；DB 中 `channel=罗总→罗总` 还混入 fallback 链符号。
   - 影响：客户端无法按状态码统一重试/降级。
   - 修复：统一为 **503 + `Retry-After`**；502 只用于「上游返回了非法响应」；channel 字段保持单一渠道名，fallback 链另存字段（L6）。

5. **上游慢请求无有效超时收敛**
   - 证据：200 响应 max = **113.1s**，68 个 ≥5s / 26 个 ≥30s；`configs/config.yaml:11 write_timeout_sec: 600`（代码默认 120）。
   - 影响：慢上游长期占用连接/goroutine；600s 写超时 + `ReadHeaderTimeout 15s` 之外缺少单请求上限。
   - 修复：按模型设置上游 timeout（渠道表已有 `timeout_seconds` 字段，建议 60s 上限）；网关 `write_timeout_sec` 收窄到 120 或对流式单独放宽。

### P2 —— 中

6. **连接池零余量**：2 × `SetMaxOpenConns(50)` = 100 = `max_connections`（仅留 3 superuser）→ 无扩容空间，重连风暴即打满。建议降到每副本 30（或按 PgBouncer 收口），并设置 `application_name`（当前 39 个连接全为空，无法归因副本）。
7. **索引膨胀**：`usage_logs` 20 个索引 29MB（占表 69%），6 个 trigram GIN 索引 ~15.8MB **从未被扫描**，纯写入放大。删除或改造。
8. **INFO 刷屏 + 10s 全量 DB 重载**：20.6% 日志无信息量；`hot reloaded data plane memory state` 隐含每副本 10s 一次全量 DB 读取（≈17k 次/天/副本）。改版本号轮询 + 变更才记日志。
9. **保留策略吞吐临界**：365 天保留 / 6h 清理 / 单批 5000 → 上限 2 万行/天，vs 近 5 日均 5,707、峰值 16,002 行/天。建议提高到单批 2 万或清理周期 1h，以便历史积压可追平。
10. **Redis 无内存上限且跨栈共享**：`maxmemory=0` + `noeviction`，同实例承载 nanorag 的 `nano:stream:logs`（1,749 条）→ 任一栈暴涨会让另一方 XADD 失败（丢日志/丢限流计数）。建议独立实例或设 `maxmemory` + 告警。
11. **无 `pg_stat_statements`**，且无慢查询采样 —— 本次无法量化 DB 侧耗时分布。

### P3 —— 低

12. **日志消费组 50 个 consumer，48 个僵尸**（idle 最久 25.5h）。重启时 `XGROUP DELCONSUMER` 或使用 `consumer` 名字带时间戳 + 定期清理。
13. **`/health` 不入访问日志**，`requests_total` 与访问日志口径差 ~10%。
14. **启动 banner 5 行非 JSON / ANSI**（解析器友好性）。
15. **DB 失败行 channel 为空（1126 行）**、channel 格式混入 `→` 符号（见 L6，数据质量问题而非计费问题）。
16. **AOF 37.2MB / base 6.4MB、`rdb_changes_since_last_save` 386,512**，未见 rewrite 进行中，建议关注 AOF 增长（当前不影响可用性）。

---

## 10. 建议新增的告警阈值

现状：**没有看到任何告警规则落地**（`/metrics` 有指标，但无 Prometheus rule 文件；日志侧无告警）。以下按“先能发现问题”排序：

| # | 告警 | 表达式 / 依据 | 阈值 | 理由 |
|---|---|---|---|---|
| **A1（最有价值）** | **上游限流首次出现** | `increase(usage_logs{status_code=429}[5m]) > 0` 或日志 `channel execution failed` 含 `rate limit` | **> 0 即告警** | 429 历史上首次出现就是本次事故起点；早 30s 发现即可止血 |
| A2 | 5xx 速率 | `sum(rate(airoute_requests_total{status=~"5.."}[1m]))`（需按 status 打标签，当前无标签→先补指标）| > 1% 持续 2 分钟，>5% 立即 page | 突发期峰值分钟 46% |
| A3 | 熔断抖动 | `increase(log{msg="circuit breaker TRIPPED"}[5m]) > 3` 且 `increase(recovered[5m]) > 3` | 5 分钟内 >3 次 | 本次 1 秒 15 次，属明显异常 |
| A4 | 计费管道丢失 | `airoute_usage_log_records_dropped_total > 0` 或 `flush_failures_total > 0` 或 `queue_depth > 100` | **任何非零立即 page** | 直接等于漏账 |
| A5 | 延迟长尾 | `airoute_latency_p99_ms > 30000` 持续 5 分钟（并先修 L10 的 60s 钳位） | >30s | 当前 p99 已达 60.7s |
| A6 | 上游整体不可用 | `total_channels - available_channels == total_channels`（或日志 `all candidates … disabled by circuit breaker`）| >0 立即 page | 这次 128 个 502 的直接信号 |
| A7 | DB 连接水位 | `pg_stat_activity` count / `max_connections` | >80% 持续 5 分钟 | 当前 40%，池总和=100 无余量 |
| A8 | Redis 流积压 | `XPENDING > 0` 或 `lag > 1000` 持续 1 分钟 | >0 / >1000 | 当前 0，作为丢数前兆 |
| A9 | 死锁/回滚 | `pg_stat_database.deadlocks > 0`（instant）/ `xact_rollback` 比例 >5%/5min | 见左 | 当前 0 / 3.76% |
| A10 | 日志噪声比 | churn 行数 / 总行数 | >30% | 当前 20.6%，防止掩盖真实告警 |
| A11 | 无流量（僵尸/入口故障） | `rate(airoute_requests_total[5m]) == 0` 且 `/health` 正常 | 5 分钟 | 10:12 后流量骤降，长时间为零需确认是压测结束还是入口异常 |

**仪表盘建议**：把 `http_access` 的 `path` 维度做成 p50/p90/p99 + 状态码热力图；把 `channel execution failed` 的 `error` 字段（429/5xx/超时）做分类计数 —— 目前这些信息只在自由文本里。

---

## 附：复现命令（只读）

```bash
# 日志（必须过滤非 JSON banner）
docker logs --since 6h airoute-1 2>&1 | grep '^{' > /tmp/a1.json
docker logs --since 6h airoute-2 2>&1 | grep '^{' > /tmp/a2.json
jq -r 'select(.level!="INFO") | "\(.time)\t\(.level)\t\(.category)\t\(.msg)"' /tmp/a1.json

# 指标（直连每个副本，避免 LB 只命中其中一个）
for c in airoute-1 airoute-2; do
  docker exec $c sh -c 'wget -qO- http://127.0.0.1:8080/metrics' | grep '^airoute_'
done

# 逐请求对账（trace_id join）
docker exec airoute-postgres psql -U airoute -d airoute -X -A -F, -t \
  -c "copy (select trace_id,status_code,cost,duration_ms,model,channel from usage_logs
            where created_at >= '2026-10-23 01:33:00+00') to stdout with csv" > /tmp/db.csv

# DB 健康
docker exec airoute-postgres psql -U airoute -d airoute -X -c \
  "select state,count(*) from pg_stat_activity where datname='airoute' group by 1;"
docker exec airoute-postgres psql -U airoute -d airoute -X -c \
  "select deadlocks,conflicts,temp_files,xact_commit,xact_rollback from pg_stat_database where datname='airoute';"

# Redis
docker exec airoute-redis redis-cli XLEN airoute:stream:logs
docker exec airoute-redis redis-cli XPENDING airoute:stream:logs airoute_log_group
docker exec airoute-redis redis-cli XINFO GROUPS airoute:stream:logs
```
