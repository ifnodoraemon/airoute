# Bug 排查 B：核心运行时 / 鉴权 / 限流 / 并发真实缺陷

> **任务**：task-2。只读审计 + 探针测试，不改生产代码。
> **基线**：`381926d`（HEAD），`go build ./...` / `go vet ./...` / `gofmt -l` 干净，`go test ./...` 16 包 ok。
> **审计期间工作区有 Lead 并行删除死代码**（`internal/middleware/authz.go`、`quota_tracker.go`、`internal/billing/estimate.go` 等 6 文件、−119 行）；本报告行号以**当前工作区**为准（即删除后）。本文不重复上报被删除的 5 个死符号（README §8.3 遗留，归属 Lead）。
> **范围**：`internal/middleware`（auth / authz / ratelimit / tpm / admin_access / quota_tracker / trace / recovery 相关）、`internal/distributed`（Redis 限流 / in-flight 配额 / 集群广播 / live 聚合）、`internal/config`、`cmd/airoute`（启动 / 迁移 / 优雅退出 / 自检）、`internal/controlplane`（admin API / 会话 / 密钥 / 钱包 / 登录限流）。
> **不查**：风格、性能、`docs/audit/2026-10-28-inconsistencies/README.md` §3/§6/§8.3 已排除项与已修项。

---

## 0. 结论摘要

主链路（JWT/HMAC 验签、CSRF double-submit、quota 单位换算、release 的 clamp、Redis 限流回退、启动 fail-closed、优雅退出顺序、goroutine 生命周期）**复核未发现缺陷**，上一轮 A-01/A-02/C-12/D-02 的修复也站得住（见 §5 已排除）。

本轮新发现 **5 条真实缺陷**（2×P1 + 3×P2），全部有可执行探针；另有 **6 条待确认**（低置信 / 跨域 / 当前不可利用）。最值得先修的是 **B-01**：

| 编号 | 严重度 | 置信度 | 一句话 |
|---|---|---|---|
| **B-01** | **P1** | 高 | in-flight 钱包预授权的 Redis TTL=120s **短于请求最大生命周期**（非流式 600s、流式无上限）⇒ 预授权中途消失，并发请求可绕过「balance ≥ 已预授权 + 本次」；长请求结束时 release 按 user 维度聚合键释放，**删掉/扣减在途请求的 amount 与 count**，并发上限随之漂移 |
| **B-02** | **P1** | 高 | 登录成功后 `loginGuard.success` 连 **per-IP 喷洒桶**一起清空 ⇒ 持有一个有效账号即可无限重置 IP 维度；注册默认开放 ⇒ 攻击者自注册后把「20 次/15 分钟/IP」变成「每几秒 19 次，无上限」 |
| **B-03** | **P2** | 高 | `TraceMiddleware` 无校验地接受客户端 `X-Request-ID` / `X-Airoute-Trace-ID` / `X-Trace-ID` / `traceparent` 作为 `trace_id`（无长度、无字符集、traceparent 只查长度不查 hex）⇒ 审计关联可被投毒、任意字节入库（实测 4KB+CRLF 原样接受） |
| **B-04** | **P2** | 中高 | 会话 cookie 的 `Secure` 由**客户端可控的** `X-Forwarded-Proto` 决定，而出厂 nginx 恰好把这个头原样透传（`map ... default $http_x_forwarded_proto`）⇒ https 用户可被诱导签发**不带 Secure 的 7 天会话 cookie** |
| **B-05** | **P2** | 高（机制） | per-key TPM 实际是**固定窗口**（注释写「sliding」），窗口边界可 2× 突发；且 `CheckKeyRateLimits` 以 0 token 查询，请求自身 token 只在完成后才记账 ⇒ `used == limit` 仍放行，只能事后发现超额 |

---

## 1. 发现详情

### B-01（P1，高）in-flight 预授权 TTL 短于请求生命周期：预授权泄漏 + 跨请求释放

**位置**
- `internal/distributed/redis_quota.go:42-45` — `inflightQuotaTTLSeconds = 120`
- `internal/distributed/redis_quota.go:116-119` — reserve 时 `EXPIRE amountKey ttl` / `EXPIRE countKey ttl`（**整个请求期内不再续租**）
- `internal/distributed/redis_quota.go:145-150` — release Lua：`if inflight <= 1 { DEL countKey } else { DECR countKey }`
- `internal/distributed/redis_quota.go:156-185` — top-up Lua：`if EXISTS(key)==0 then return 0`（**key 过期后 top-up 静默不生效**）
- `internal/distributed/redis_quota.go:198` — `TopUpQuotaAmount` 又把 `ttl` 硬编码成字面量 `120`（未引用 `inflightQuotaTTLSeconds`，已属漂移隐患）
- `internal/middleware/quota_tracker.go:132-161 / 175-206` — reserve/release 均以 `userID` 为维度（聚合键）
- `internal/middleware/auth.go:544-573` — 认证时 reserve 0.05 CNY，请求结束 defer release；`auth.go:208-223` `settleReleaseAmount` 只决定释放**金额**，不改变「按 user 聚合」的键结构
- `internal/api/common.go:430-443` — 唯一一次 top-up 发生在 dispatch，之后不再续租
- `cmd/airoute/timeout_chain.go:24-33` + `configs/config.yaml:13,16` — `write_timeout_sec: 600`，且注释明确「流式(SSE)会清除写 deadline」；`internal/api/common.go:508-509` 亦写「> 180s」长流

**触发条件**：单用户，一个请求的在途时间 > 120s（长推理流式、长上下文、上游慢），且该用户在此期间再发起请求。

**根因**：预授权是一个「聚合键 + TTL」的软预留，而 TTL 按**请求粒度**设置的却比**请求最长生命周期**还短；release 无法区分「我的预留已过期」与「我的预留还在」，只能无条件扣减聚合并按 `inflight<=1` 删除计数键。

**影响**
1. **余额门失效**：TTL 到期后 `balance >= reserved + need` 的 `reserved` 归零，并发请求被放行到已被长请求占用的余额上（在途上限 64 × 0.05 CNY 的敞口，且 top-up 因 `EXISTS==0` 静默失效 ⇒ 长请求的真实成本**根本没有预留**），透支只由钱包负余额下限兜底。
2. **并发上限漂移**：长请求结束时会把**别的请求**的 count 扣掉，`inflight<=1` 时直接 `DEL` ⇒ 计数永久偏低，`maxInflightPerUser=64` 实际可被突破。
3. **双后端语义不一致**：local tracker（`quota_tracker.go:211-300`）没有 TTL，Redis 后端有 120s —— 与 C-12 修掉的「本地上限窗口 vs Redis 窗口」是同一类断裂（那次只修了 RPM，没修 quota）。

**证据（探针）**
- `internal/distributed/quota_ttl_bugprobe_test.go`
  - `TestBugB1_InflightReservationTTLShorterThanRequestLifetime`：断言 `TTL(120s) < write_timeout(600s)`，**当前预期通过**（修好后该测试会失败并提示删除）。
  - `TestBugB1_ReleaseAfterExpiryEatsAnotherReservation`：逐条模拟两段 Lua（无 Redis 环境），复现「A 过期 → B 预留 → A release 删掉 B 的 amount+count → 第三个请求拿到 1000/1000 全额」。
- `internal/distributed/quota_ttl_integration_bugprobe_test.go`（`//go:build integration`）：**真 Redis 复现**，用 DEL 模拟 TTL（与 TTL 效果一致，无需等 120s）：
  ```
  export PATH=$PATH:/usr/local/go/bin
  export AIROUTE_TEST_REDIS_URL=redis://127.0.0.1:6379/0
  go test -tags integration ./internal/distributed -run TestBugB1_RealRedis -v
  ```
  观测（本环境无 Redis，未执行；离线模型测试已给出同样结论）：`while B is in flight, A's release deleted B's amount and count keys; a third 1.00 CNY reservation was then admitted`。

**修复方向（供归属方决策，本任务不改代码）**
- TTL 与最大可能请求时长联动：`max(channel timeout) + margin`，或把 TTL 刷新放进流式 ticker / 心跳续租；
- 键结构改为 per-request（`amt:<uid>:<request-key>` + 聚合计数用 Lua 原子加减），release 只释放自己的键；
- 至少：top-up/流式过程中续租，且 release 不得因 `inflight<=1` 无条件 `DEL` 计数键（改为 min(units, booked) 的自有额度）。
- 顺带：`TopUpQuotaAmount` 的 `ttl := 120` 应引用 `inflightQuotaTTLSeconds`。

---

### B-02（P1，高）登录成功会清空 per-IP 喷洒桶：单账号即可无限重置 IP 维度

**位置**
- `internal/controlplane/login_guard.go:343-353` — `success()` 对 `loginBuckets(client)` 返回的**所有**桶调用 `reset`
- `internal/controlplane/login_guard.go:257-271` — `loginBuckets` 同时返回 `user` 桶与（可归因时）`ip` 桶
- `internal/controlplane/login_guard.go:26-27` — `maxLoginFailuresPerIP = 20`、`loginLockoutWindow = 15m`
- `internal/controlplane/account_handler.go:89,93` — 锁定账号成功验证密码、正常登录成功都调用 `success()`
- `internal/config/config_policies.go:34-42` — `IsRegistrationAllowed` 默认 `true`（未设 `ALLOW_REGISTRATION`）

**触发条件**：攻击者先自注册一个账号（注册默认开放）；用该账号一次成功登录穿插在喷洒之间。

**根因**：`success()` 的语义是「这次认证成功 ⇒ 清空这次认证涉及的失败计数」，但 per-IP 桶衡量的是**该地址对多个账号**的喷洒历史，与「本次登录属于哪个账号」无关。用 `subject = 请求用户名` 的 user 桶重置是正确；把同一请求的 IP 桶一起重置就是越权清零。

**影响**：per-IP 维度从「20 次/15 分钟」退化为「19 次 / 一轮 bcrypt 耗时（约 6 秒），可无限重复」；配合「用户名不存在也付 bcrypt 成本」（`account_handler.go:60-77`）意味着**跨账号在线密码喷洒速度提升约两个数量级**，而 per-IP 桶正是审计里专门为「不要一个 IP 锁死整栋楼」设计的喷洒防线。

**证据**：`internal/controlplane/login_guard_bugprobe_test.go::TestBugB2_SuccessfulLoginResetsPerIPSprayBucket`
```
go test ./internal/controlplane -run TestBugB2 -v
→ BUG-B2 confirmed: 58 sprayed password attempts were admitted from 203.0.113.9 inside a single 15m0s window (budget is 20) because each successful login of the attacker's own account reset the per-IP bucket
```
（同一测试内含对照组：没有 `success()` 时严格 20 次即锁。）

**修复方向**：`success()` 只重置 `user` 桶（可选：重置 `ip` 桶需同时匹配该 IP 与该账号的联合桶，且仅在「该账号此前无失败」时）。

---

### B-03（P2，高）trace_id 原样接受客户端输入：审计关联投毒 / 任意字节入库

**位置**
- `internal/middleware/trace.go:61-80` — `extractTraceID`：三个 header 直接 `TrimSpace` 返回，无长度/字符集校验；`traceparent` 只检查 `len(parts[1]) == 32`，**不校验是否 32 位 hex**
- `internal/middleware/trace.go:42-59` — 写入 gin context、`context.Context`、响应头 `X-Airoute-Trace-ID` / `X-Request-ID`
- `internal/middleware/trace.go:128-188` — 访问日志原样打印
- 消费方：`usage_logs.trace_id`（控制台以此关联请求与 trace）

**触发条件**：任意客户端在请求头里自带 `X-Request-ID`（或另外三个），值任意。

**根因**：把「上游链路透传」和「不可信输入」混为一谈；只在 W3C 分支做了一个长度检查，且检查的是字符数而不是格式。

**影响**
- 审计关联可被投毒：攻击者可让多次请求共用一个 trace_id，或与合法 trace 冲突，控制台按 trace 打开请求时看到被污染的关联；
- 4KB+ 的任意字节（含 CRLF，实测 4111 字节）进入 DB 列与响应头，属于日志/审计管道的注入面与存储放大；
- `traceparent` 的非 hex 32 字符串也被当成合法 trace id（"W3C 格式校验"名不副实）。

**证据**：`internal/middleware/trace_id_bugprobe_test.go`
```
go test ./internal/middleware -run TestBugB3 -v
→ BUG-B3 confirmed: a 4111-byte client header containing CR/LF is accepted as trace_id and echoed back
→ BUG-B3 confirmed: traceparent field "zzzz…z" (non-hex) accepted as trace id
```
（断言的是**当前 buggy 行为**，修好后会失败并提示删除。）

**修复方向**：只接受 `^[A-Za-z0-9._-]{1,64}$`，否则重新生成；`traceparent` 增加 hex 校验；不合法时丢弃而不是截断（避免伪造的「半截 id」）。

---

### B-04（P2，中高）会话 cookie 的 `Secure` 由客户端可控 header 决定

**位置**
- `internal/controlplane/session.go:23-31` — `cookieSecure`：TLS 或 `X-Forwarded-Proto: https` 即为 true（**不校验 peer 是否可信代理**）
- `internal/controlplane/session.go:35-56` — `SetSessionCookies` 用该值决定 `Secure`（会话 cookie 7 天、CSRF cookie 同步）
- `deploy/nginx-compose.conf:14-17` — `map $http_x_forwarded_proto $proxy_x_forwarded_proto { default $http_x_forwarded_proto; '' $scheme; }`：**默认透传客户端自己的值**
- `deploy/nginx-compose.conf:81,126,143,164,214` — `proxy_set_header X-Forwarded-Proto $proxy_x_forwarded_proto`

**触发条件**：TLS 在反代终结，客户端在登录请求上加 `X-Forwarded-Proto: http`（或任意非 https 值）。

**根因**：scheme 判定没有走「可信代理链」这一前提（同仓 `middleware.ResolveClientIdentity` 已经确立了「只信可信 peer 的转发头」的范式，session 层没复用）；而 nginx 侧的 map 又把客户端值当默认值透传，两层叠加使该头完全由客户端决定。

**影响**：登录响应可被诱导签发**不带 Secure** 的 7 天会话 cookie（HttpOnly 仍在）。浏览器之后在任何 `http://` 同域请求（降级链接、旧书签、混合内容）都会带上它，网络攻击者可截获管理员会话 —— 这正是 Secure 属性要防的场景。攻击者不能直接影响他人登录，故不是 P0/P1；但它让「出厂部署的会话 cookie 一定 Secure」这一安全承诺不成立。

**证据**：`internal/controlplane/session_cookie_bugprobe_test.go`
```
go test ./internal/controlplane -run TestBugB4 -v
→ BUG-B4 confirmed: with X-Forwarded-Proto: http the session and CSRF cookies are issued without Secure
```
（含对照：header 为 https 时确实带 Secure，说明判定完全跟随客户端。）

**修复方向**：`cookieSecure` 复用「peer ∈ GATEWAY_TRUSTED_PROXIES 才看转发头」的判定；nginx 侧改为 `default $scheme`（并显式覆盖，避免客户端注入）。

---

### B-05（P2，机制高 / 实际影响中）TPM 是固定窗口，且请求自身 token 不参与准入

**位置**
- `internal/middleware/tpm_tracker.go:8-9` — 注释称「sliding minute window」
- `internal/middleware/tpm_tracker.go:54-60` — `rollLocked`：只保留 `used int` + `windowFrom`，`now-windowFrom >= 1min` 时**整窗清零**（经典固定窗口，无法只淘汰滑出窗口的 token）
- `internal/middleware/tpm_tracker.go:19-27` — `allow(tokens)`；`authz.go:229-247` 调用时传 `0`
- `internal/api/common.go:255-266` — 请求**完成后**才 `Consume(totTokens)`
- `internal/middleware/ratelimit.go:19-30` — 对比：RPM 已改为真滑动窗口（`rpmWindow`），注释明确说明旧令牌桶「idle 可整桶突发」的问题

**触发条件**：客户端在窗口末尾用满 TPM，紧接着窗口翻页。

**根因**：RPM 被重写成滑动窗口时，TPM 仍是旧的固定窗口计数器；且 TPM 的准入只读历史用量、不预扣本次请求，无法估计的 token 只能事后入账。

**影响**
1. 任意 60s+ε 区间内最多可放行 2× TPM（实测探针：1 秒内 100→再 100）；与 RPM 的严格语义不一致，`X-RateLimit-Remaining-Tokens` 也因此会突然跳变；
2. `used == limit` 时仍放行（`used+0 <= limit`），超额只能在**下一次**请求才被发现，单次请求可把 TPM 冲高一个请求的量；对「防止 key 泄漏后被刷爆」的定位是打折的。

**证据**：`internal/middleware/tpm_window_bugprobe_test.go`
```
go test ./internal/middleware -run TestBugB5 -v
→ BUG-B5 confirmed: 100 tokens were consumed 1s ago and the gate immediately admits another 100 (fixed window, not the documented sliding minute)
→ BUG-B5 confirmed: the gate only refuses once usage is already over the limit (overshoot up to one request)
```
（探针通过 `windowFrom` 注入时间推进，这是该结构唯一的时间输入。）

**修复方向**：TPM 改成与 `rpmWindow` 同构的滑动窗口（或 token 桶 + 预扣估算值、完成后按差额校准）；注释与实现二者择一对齐。

---

## 2. 待确认（低置信 / 跨域 / 当前不可利用，未计入 P 级）

| 编号 | 归属 | 观察 | 为何不升级 |
|---|---|---|---|
| C-01 | **internal/api（D 域）** | `/metrics` 的 IP 判定仍是 `ResolveClientIP` + 私网放行（`internal/api/metrics_access.go:31-50`），**不要求 `identity.Forwarded`** —— 与 A-01 同根：admin 守卫已 fail-closed，metrics 仍 fail-open（无法归因的私网 peer 直接 200） | 出厂 compose 的 nginx 未路由 `/metrics`（grep `deploy/*.conf` 无命中），网关端口只 `expose`；一旦有人把 /metrics 挂到反代即成公网信息泄露。建议与 A-01 一起收口 |
| C-02 | **cmd/airoute** | 启动播种吞错：`repo.ListChannels()` / `ListAPIKeys()` 的 error 被丢弃（`cmd/airoute/server.go:150,169`），出错时按「空库」重新播种；而 `CreateChannel`/`CreateAPIKey` 无唯一约束（`internal/storage/repo_channels.go:113`、`repo_keys.go:311`）⇒ DB 瞬时故障时可插入重复渠道/密钥行（渠道重复会被路由当两个上游） | 需要 DB 在启动瞬间故障；影响是数据重复而非越权。建议 `if err != nil { skip seeding }` |
| C-03 | internal/distributed | `PublishReload` 忽略 `INCR` 错误、`ClusterEvent.NodeID` 从不赋值（`internal/distributed/redis.go:116-123`），订阅端不过滤自身消息（`redis.go:193-204`）⇒ 自己发的广播也会再触发一次本地 reload；`NodeID` 是死字段 | 幂等 reload，仅多一次 DB 读；属于「设计意图未接线」类（README §3.2 风格），Lead 若在清死代码可顺带处理 |
| C-04 | internal/controlplane | 登录白名单用 `strings.HasSuffix(c.Request.URL.Path, "/auth/login")`（`auth.go:54`），而不是精确匹配路由 | 当前只有 `/api/v1/auth/login` 命中；属**脆弱写法**：将来任何以 `/auth/login` 结尾的管理路由都会自动绕过鉴权 |
| C-05 | internal/controlplane | 登录端点无 CSRF 保护（登录 CSRF：可让受害者登录到攻击者账号） | cookie 为 `SameSite=Lax`，跨站表单 POST 不携带会话；影响低，记录备查 |
| C-06 | internal/controlplane | `loginBuckets` 的 user bucket 用 `normalizeLoginSubject`（小写）归一化，而用户名查找走 `GetUserByLogin`；若 DB 侧大小写敏感且允许 `Admin`/`admin` 并存，同一 bucket 会覆盖两个账号 | 未验证 `GetUserByLogin` 的大小写语义（跨 storage 域），低置信 |

---

## 3. 本轮明确「查了但没有」的结论（防止后人重复劳动）

- **admin_access（A-01/A-02/D-02 修复后）**：XFF 右遍历 + 仅信可信 peer、`X-Real-IP` 仅作为 XFF 缺失时的第二来源、`*.domain` 通配的长度校验、`explicitIPs` 分支的 fail-closed、非法 allowlist 的 503 —— **未发现新绕过**。特别验证：即使客户端注入 `X-Forwarded-For: 私网IP`，nginx 追加真实 IP 后从右往左仍取到真实客户端。
- **JWT / 会话签发**：`VerifyAdminToken` 先验签（`hmac.Equal`）再解码、`exp` 校验、`CredentialTag` 绑定 `password_hash`（改密即失效）、角色变更即失效、`deleted/locked` 复查（`token.go:106-136`、`auth.go:89-114`）—— 未发现缺陷。
- **CSRF**：double-submit + 仅对 cookie 认证的写请求生效 + `SameSite=Lax` + 登录重新签发 CSRF token（`session.go:94-107`、`auth.go:82-87`）—— 结构正确（Secure 判定另见 B-04）。
- **配额记账**：`reserveQuotaUnits` / `quotaReserveUnits` 两侧同为 1e-9 且同为 Ceil；release 的 Redis Lua 与 local 实现都 clamp，**不会把聚合打成负数**，也没有发现「重复释放导致负值」；`BillingReservedUnits` gauge 的 CAS 防空转正确（`quota_tracker.go:302-343`）。唯一问题是 B-01 的键结构/生命周期。
- **Redis 故障回退**：RPM 的 fail-open 只损失跨副本视图（本地同语义窗口兜底，C-12 已修）；quota 的 Redis 错误回退 local 且 gauge 按实际记账；登录限流 Redis 错误回退本地计数 —— 均为已声明的可用性取舍，不是越权。
- **分布式锁**：本仓库**没有**通用 Redis 锁（全仓无 `SetNX`/Redlock 调用），集群一致性靠「10s 周期 reload + pub/sub + version 对账」；因此「锁租约与临界区」不适用（结论而非遗漏）。
- **goroutine 泄漏**：`StartLiveStatsPublisher`、`SubscribeReload`（含重连退避）、`StartPeriodicSync`、calibration saver、ratelimit init ticker 都有退出通道；`-race` 跑 middleware/distributed/config 全绿。
- **启动顺序 / 优雅退出**：signal handler 在慢初始化之前安装；`SetDataPlaneReady(false)` 在首次快照前；`OpenDB` 内含迁移；defer 的 LIFO 顺序为「停同步 → 停各 retention → 结算 batcher → 异步 logger → 关 DB」，结算先于 logger/DB，正确。
- **配置默认值**：`GATEWAY_ADMIN_SECRET`/admin 密码为 fail-closed 启动检查（`security_check.go`）；`GetInitialUserBalance` 非法值归 0（fail-closed）；`GATEWAY_TRUSTED_PROXIES` 非法即拒绝启动；`StorageConfig.Validate` 非法即拒绝启动。
- **auth 的 402/429 分支**：402（key budget / wallet / 在途预授权）与 429（TPM/RPM）全部经 `auditAuthFailure`/`auditDenial` 落 `usage_logs`；401 刻意不落库（§6 已排除）。**未发现审计行缺失**。

---

## 4. 探针清单与运行方式

| 文件 | 用例 | 复现命令（`export PATH=$PATH:/usr/local/go/bin`） |
|---|---|---|
| `internal/distributed/quota_ttl_bugprobe_test.go` | B-01（常量不变量 + Lua 语义模型） | `go test ./internal/distributed -run TestBugB1 -v` |
| `internal/distributed/quota_ttl_integration_bugprobe_test.go` | B-01 真 Redis 复现（`//go:build integration`） | `AIROUTE_TEST_REDIS_URL=redis://127.0.0.1:6379/0 go test -tags integration ./internal/distributed -run TestBugB1_RealRedis -v` |
| `internal/controlplane/login_guard_bugprobe_test.go` | B-02（含对照组） | `go test ./internal/controlplane -run TestBugB2 -v` |
| `internal/middleware/trace_id_bugprobe_test.go` | B-03（含 traceparent 分支） | `go test ./internal/middleware -run TestBugB3 -v` |
| `internal/controlplane/session_cookie_bugprobe_test.go` | B-04 | `go test ./internal/controlplane -run TestBugB4 -v` |
| `internal/middleware/tpm_window_bugprobe_test.go` | B-05（窗口 + 准入不预扣） | `go test ./internal/middleware -run TestBugB5 -v` |

**约定**：每个探针都**断言当前缺陷行为**并在失败信息里写明「BUG-Bn appears fixed → 删除本探针与报告条目」，因此修复者改完代码后探针会**主动变红**提醒清理。全部探针在 `go test ./...` 下为绿（已验证：`go test ./...` 16 包 ok，`-race` 亦绿）。

---

## 5. 与上一轮审计的关系

- 未重复上报 README §2/§3 的 72 条（含 A-01/A-02/A-10/C-02/C-09/D-02/E-01 等已修项）。
- 未重复上报 §8.3 遗留的 5 个死符号（Lead 正在删）与「SQLite 迁移注册表缺 v43」（归属存储域）。
- 交叉印证：B-01 属 §3.3「同一件事两套实现（Redis 有 TTL / 本地无 TTL）」在 **quota** 上的残留（C-12 只修了 RPM）；B-05 属 §3.1「半接线：RPM 换成滑动窗口，TPM 没换」。

---

## 6. 修复记录（2026-10-28 修复阶段）

本轮修复在 bug 排查之后立即进行。**范围**：`internal/distributed`、`internal/middleware`、`internal/controlplane`、`cmd/airoute`（+ `internal/api` 的 C-01 与 MCP quota 接线、`internal/api` 测试）；未改 `internal/storage`/`internal/billing`/`internal/telemetry`/`web/`，未删任何导出符号，未 commit。

### 6.1 逐条结果

| 编号 | 状态 | 关键实现 | 回归测试 |
|---|---|---|---|
| **B-01** | 已修 | `distributed/inflightQuotaTTL`（var，测试可注入）替代 120s 常量，脚本全部改 `PEXPIRE`（毫秒）；新增 `quotaAmountKeepAliveLua`（`EXISTS` 守卫，**只刷新已存在键，绝不重建**）+ `Client.KeepInflightQuotaAlive(userID) (stop func())`（TTL/3 心跳、`stop` **同步等待** goroutine 退出、Redis 错误只 WARN 一次）；middleware 新增 `StartQuotaHeartbeat`，`auth.go` 与 `api/mcp_tools_routing.go` 均为「先 `stopHeartbeat()` 再 `ReleaseUserQuotaAmount`」的单一 defer；top-up 的字面量 `120` 收敛到同一常量 | `distributed/quota_heartbeat_test.go`（6）：TTL/interval 管道一致、心跳跨越 TTL 不消失（①）、无心跳按 TTL 过期（②）、stop→release 清键且 late tick 不复活（③）、无 Redis 客户端安全且 stop 幂等、stop 等待在跑的一次刷新；`distributed/quota_heartbeat_integration_test.go`（`-tags integration`，5 子用例，真 Redis） |
| **B-02** | 已修 | `loginBuckets` 拆为 `accountBuckets`（账户，5 次）+ `ipBuckets`（地址，20 次）；`success()` **只重置账户桶**，IP 桶只随 `loginLockoutWindow` 老化；`failure()`/`check()` 仍看全部维度 | `controlplane/login_guard_reset_test.go`（3）：19 次喷洒+自己登录后第 20 次仍被拦、成功登录仍清自己账户桶、账户锁独立生效 |
| **B-03** | 已修 | `validCustomTraceID`：`^[A-Za-z0-9._-]{1,64}$`；`traceIDFromTraceparent`：严格 4 段、2/32/16/2 长度、全小写 hex、trace-id/parent-id 非全零；不合法**一律重新生成**（不截断）；三个 header 依次回退 | `middleware/trace_id_validation_test.go`（4）：合法值原样保留（含 64 上限与 W3C traceparent）、9 类恶意值全被拒（4KB、CRLF、空格、超长、非 hex、字段过短、全零、大写 hex、多余段）、无效头回退到下一个有效头、生成器与校验器自洽 |
| **B-04** | 已修 | middleware 新增 `PeerIsTrustedProxy(r)` 与 `TrustedForwardedProto(c)`（仅当 TCP peer ∈ `GATEWAY_TRUSTED_PROXIES` 才采信转发头）；`cookieSecure` 只在「TLS 直连」或「受信代理声明 https」时为 true，不可信 peer 的 `X-Forwarded-Proto` 一律忽略 | `controlplane/session_cookie_test.go`（9 子用例）：不可信 peer 不能声称 https、也不能降级、无 TLS 不安全、直连 TLS 安全、受信代理 https/http/无头三分支、名单外 peer 被忽略、`ClearSessionCookies` 与登录侧一致 |
| **B-05** | 已修 | `tokenWindowSpan = 60s`；桶改为 `[]tokenUsage{at,tokens}` 逐条淘汰（`(now-window, now]`，与 `rpmWindow`/Redis ZSET 同语义）；准入改 `used < limit`（修 off-by-one）；热更新保留已消费 token；`CleanupIdle` 按 `lastSeen` | `middleware/tpm_window_test.go`（6）：边界不再 2× 突发、`used==limit` 拒绝、增量淘汰、内存有界、热更新立即生效、idle 清理；`tpm_tracker_test.go` 的固定窗口用例改写为滑动语义 |
| **C-01** | 已修 | `metricsDeniedStatus` 改用 `middleware.ResolveClientIdentity`：`Forwarded` 才按私网判定，未归因仅放行 loopback，其余 fail-closed（与 `AdminAccessGuard` 同口径） | `api/metrics_access_test.go` 重写为 12 例（loopback、私网不可归因、公网、public opt-in、token 四例、受信代理转私网/公网/无头、名单外 peer） |
| **C-02** | 已修（不乱改启动流程） | 抽出 `seedChannelsFromConfig` / `seedAPIKeysFromConfig`：**读失败不播种**（WARN 后继续启动，**非 fail-fast**）；仍是「仅空库播种」语义不变；配置内重名/重 key 去重 | `cmd/airoute/config_seeding_test.go`（3）：两次播种幂等且去重、读失败（关闭 DB）时返回错误且不写入、空配置/nil repo no-op |
| **C-03** | 已修 | `PublishReload` 的 `INCR` 失败不再吞（WARN + 说明后果：掉线副本要等下次成功发布才能自愈）、`json.Marshal` 错误返回、事件写入 `NodeID`（字段保留，未删）；订阅端跳过自身消息 | `distributed/redis_integration_test.go` 更新：peer 发布走共享 version + 事件投递断言，并新增「自身广播不被自己消费」断言（真 Redis 实跑 PASS） |
| **C-04** | 已修 | 登录白名单由 `HasSuffix` 改为常量 `loginPath = "/api/v1/auth/login"` 精确比较 | `controlplane/auth_login_path_test.go`（2）：`/api/v1/admin/users/7/auth/login` 等 3 个后缀形似路径仍 401、真实登录 200、普通管理路由仍 401、畸形 token 401 |

### 6.2 行为变化与升级注意（重要）

1. **B-01 额外 Redis 调用**：每个在途请求在 Redis 模式下每 `TTL/3`（默认 40s）执行一次 keep-alive 脚本（两条 `PEXPIRE`），一个请求通常 0–1 次；key 缺失时不会重建。预留 TTL 仍是 120s（**崩溃泄漏上限不变**），但**活请求不再丢预留**。注意：`PEXPIRE` 取代 `EXPIRE`，若有人用外部脚本读 TTL，读到的仍是秒级语义（`PTTL` 才是毫秒）。
2. **B-02 登录限流**：成功登录不再清 IP 桶 ⇒ 共享出口 IP（公司 NAT、移动网络）的合法用户，在 15 分钟内累计 20 次失败后该 IP 会被锁 15 分钟，且不会因为其中某人登录成功而重置。这是刻意的喷洒防护方向；若线上出现「整栋楼被锁」，应调大 `maxLoginFailuresPerIP` 或改用可信代理后的真实 IP 归因（`GATEWAY_TRUSTED_PROXIES`）。
3. **B-03 trace_id**：不符合 `[A-Za-z0-9._-]{1,64}` 的客户端 `X-Request-ID`/`X-Airoute-Trace-ID`/`X-Trace-ID`、以及非严格 W3C 的 `traceparent`（含**大写 hex**、全零 id、多余字段）不再被采纳，改为生成新 id。依赖「网关原样回显我的 request id」的上游需要把 id 规范成允许的字符集。
4. **B-04 cookie Secure**：判定改为「TLS 直连 或 受信代理声明 https」。**反代终结 TLS 的部署必须同时配置 `GATEWAY_TRUSTED_PROXIES` 并让代理设置 `X-Forwarded-Proto`**，否则会话 cookie 不再带 `Secure`（fail-closed 到"不声称安全"）。Lead 已把 `deploy/nginx-compose.conf`、`deploy/swarm/nginx-swarm.conf` 的 map 改为 `default $scheme`（边缘是 scheme 唯一权威），客户端注入的 `X-Forwarded-Proto` 不再透传。
5. **B-05 TPM 语义**：固定窗口 → 真滑动窗口。突发容量下降（不再有边界 2× 突发）；`used == limit` 现在直接 429（此前会多放行一个请求）；`X-RateLimit-Remaining-Tokens` 与拒绝判定一致。若线上有客户端按旧「每分钟重置」节奏压线跑满 TPM，会看到更多 429。
6. **C-01 /metrics 访问控制收紧**：未归因 peer（没有可信代理、也不是 loopback）现在 403。**Prometheus 从容器网络/私网直接抓取的部署需要显式配** `GATEWAY_TRUSTED_PROXIES`（并让代理转发真实客户端）、或 `GATEWAY_METRICS_TOKEN`、或 `GATEWAY_METRICS_ALLOW_PUBLIC=true`。
7. **C-02 启动行为**：**跳过播种，不 fail-fast**。DB 读失败时记 WARN 并继续启动（数据面仍由 10s 周期 reload 安装真实配置）；仅当数据库确实没有任何渠道/密钥行时才播种；配置内同名渠道/同 key 只建第一条。
8. **C-03 集群**：自身广播不再二次 reload（每次写操作少一次全量 DB 读）；`INCR` 失败会 WARN 但广播照发。
9. **C-04**：仅精确匹配 `/api/v1/auth/login` 免鉴权。

### 6.3 质量门（修复后实测）

```
gofmt -l internal cmd web                    → 空
go build ./...                               → OK
go vet ./...                                 → OK
go test -count=1 ./...                       → 16/16 包 ok
go test -count=1 -race ./internal/middleware/ ./internal/distributed/ ./internal/controlplane/ → ok
真 Redis（docker redis:7-alpine）：
  go test -tags integration ./internal/distributed -run 'TestInflightQuotaRealRedis' -v
    → heartbeat / crash-path / stop-release-clear / no-resurrect / aggregate-release 全 PASS
  go test -tags integration ./internal/distributed -run 'TestRedisClusterReloadAndSessionAffinity' -v
    → PASS（含「自身广播不被自己消费」新断言）
```

### 6.4 未做 / 留给归属方

- `deploy/nginx-compose.conf` / `deploy/swarm/nginx-swarm.conf` 的 `X-Forwarded-Proto` map：**Lead 已修**（`default $scheme`），本条只在报告留痕。
- `ClusterEvent.NodeID` 字段保留（不再死：发布端已赋值）；如需清理死代码，归属 Lead。
- C-05（登录 CSRF，SameSite=Lax 已缓解）、C-06（用户名大小写归一化存疑，跨 storage 域）仍在 §2 待确认，未修。
- `inflightQuotaTTL` 是可注入 var：**并发测试并行度高的包若也改它需要加锁**，当前仅 `internal/distributed` 内串行测试使用。

> **门禁复跑（最终确认）**：`gofmt -l internal cmd` 空、`go vet ./...` OK、`go test -count=1 ./...` 16/16 包 ok、`go test -count=1 -race ./internal/middleware/ ./internal/distributed/ ./internal/controlplane/` ok（controlplane 因 bcrypt 登录用例耗时 ~104s）。临时 Redis 容器（docker redis:7-alpine，仅供集成测试）已清理。
