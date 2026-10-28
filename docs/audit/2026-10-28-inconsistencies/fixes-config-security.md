# 修复记录：A 域（配置 / 安全边界 / 部署 / 启动）

> 域：`internal/middleware`、`cmd/airoute`、`deploy/`、`helm/`、`docker-compose.yml`、`docker-stack.yml`、
> `configs/config.yaml`、`.env.example`、`docs/CONFIGURATION.md`、`web/web.go`、`tests/audit_remediation_test.go`、`tests/trace_and_ha_test.go`
> 对应任务：`task-6`；审计来源：[01-config-bootstrap.md](01-config-bootstrap.md)（A-01…A-15）+ README 的 C-09、D-02
> 基线：审计基线 `1520d99`；本轮 HEAD 为 `c8a2765`（并行会话的 5 个提交）。**未执行任何 git 写操作。**

## 1. 状态表

| 发现ID | 状态 | 改动文件:行 | 验证 | 备注 |
|---|---|---|---|---|
| A-01 | ✅ 已修 | `internal/middleware/admin_access.go:162-221`（`decide()` 改用 `ResolveClientIdentity` + fail-closed）<br>`internal/middleware/admin_access_test.go`（新语义：默认仅 loopback；新增 public surface 用例、forwarded 公网/私网、未转发私网、loopback、显式清单命中）<br>`docker-compose.yml:30-36,148-155,166-172,262-276`（`edge` 网络固定 subnet + `GATEWAY_TRUSTED_PROXIES` 默认）<br>`.env.example:31-41`<br>`docs/CONFIGURATION.md:122-125,217-233` | `go test ./internal/middleware/ -run TestAdminAccessGuard` 全过（8 组新语义用例）；`docker compose config` exit 0 且解析出 `subnet: 172.30.0.0/16`、gateway 在 `default+edge`、lb 仅 `edge` | **语义**：`Forwarded==true` → 按 `identity.IP` 判 loopback/私网放行、公网 403；`Forwarded==false` → 仅 loopback 放行，其它 403 且错误文案直接点名 `GATEWAY_TRUSTED_PROXIES`。旧测试里「私网 peer 放行」的断言已按新语义改写为 403（`TestAdminAccessGuardDefaultsToLoopbackOnly`） |
| A-02 | ✅ 已修 | `deploy/nginx-compose.conf:107-133`（console/admin vhost 补 ACL）<br>`deploy/swarm/nginx-swarm.conf:92-116`（console/admin）+`:130-152`（default vhost，此前全文 0 条 allow）<br>`deploy/nginx.conf:26-47`（单 vhost ACL）<br>`cmd/airoute/nginx_admin_acl_test.go`（清单断言，新文件） | `go test ./cmd/airoute/ -run TestShippedNginx` 通过；另用 python 逐块复算：三份配置 ACL 块数 2/2/1，均含 `deny all` 与 4 条 allow；两份 console vhost 均含该 location | 三份文件按各自既有结构补，未合并成一份。`api.*` vhost 的 `location ~*` 403 保持不动。测试按「引号内花括号不计入」解析块，避免 413 JSON 体破坏块平衡 |
| A-03 / D-02 | ✅ 已修（**结论：不把 `/` 纳入守卫**） | `internal/middleware/admin_access.go:50-64,94-100`（注释：明确「控制面 = `/api/v1/admin` + `/app`」及四类**故意不纳入**的路径）<br>`web/web.go:77-84`（`/` = 公开落地页/登录页，禁止加进谓词）<br>`docs/CONFIGURATION.md:217-233` | 依据：读 `web/src/lib/consoleRoute.js`（裸 `/` → `viewMode:'landing'`）、`web/src/App.jsx:665-730`（landing 渲染 `LandingPage`、auth 渲染 `AuthPage`）；`web/web.go` 里 `/assets` 仅挂 `dist/assets`（内容哈希静态包，无数据） | 判断依据：`/` 渲染的是**公开落地页 + 登录页**，纳入守卫会让公网无法登录；真正受保护的是 `/api/v1/admin` 数据面与 `/app` 入口。`/api/v1/auth/*`（CLI 远程登录）、`/api/v1/user/*`（租户自助钱包/用量）保持公开，已写进代码注释与文档。谓词本身未改动 |
| A-04 | ✅ 已修 | `deploy/nginx-compose.conf:6-10`（64m）+ 413 文案<br>`deploy/swarm/nginx-swarm.conf:23-26` + 文案<br>`deploy/nginx.conf:10-13` + 文案<br>`helm/airoute/values.yaml:56-58`（ingress `proxy-body-size: 64m`） | `TestShippedNginxBodyLimitMatchesGateway` 通过；`grep -rn "50m\|50MB"` 在 deploy/helm/.env.example/docs/CONFIGURATION.md 均为 0 | 413 文案统一为 `payload exceeds 64MB edge proxy limit.`（归因到边缘代理）；`docs/CONFIGURATION.md:142` 与 `.env.example` 原本就写 64 MiB，无需改数值 |
| A-05 | ✅ 已修 | `.env.example:159-162`<br>`docs/CONFIGURATION.md:131` | 文档对照代码注释 `internal/router/dispatcher_stream.go` 的 `min(渠道 timeout, override)` | 改为「全局首字上限：对所有渠道生效，只能缩短、不能放宽渠道 timeout（取 min）」，并给出「设 10 会把 3600s 推理渠道压到 10s」的后果说明 |
| A-06 | ✅ 已修 | `cmd/airoute/timeout_chain.go:18-41`（`serverWriteTimeout`、`writeTimeoutCutsChannel`）、`:42-104`（`reportTimeoutChain` 增 `writeTimeout` 参数 + 日志 `server_write_timeout` + 告警）<br>`cmd/airoute/server.go:255-258`、`:397-403`（`http.Server.WriteTimeout` 与自检共用同一 helper，杜绝两个数漂移）<br>`cmd/airoute/timeout_chain_test.go`（重写：3 个用例 12 个子用例） | `go test ./cmd/airoute/ -run 'TestServerWriteTimeout\|TestWriteTimeoutCutsChannel\|TestEdgeProxyTimeout'` 通过 | 告警条件：`channelTimeout > writeTimeout > 0`，提示「非流式请求会先被网关切断」并给出调参路径 |
| A-07 | ✅ 已修 | `docker-stack.yml:90-95`<br>`helm/airoute/values.yaml:105-109`（新键 `auth.adminReset: false`）<br>`helm/airoute/templates/deployment.yaml:65-70`<br>`deploy/kubernetes.yaml:53-57` | `helm template test helm/airoute` exit 0，渲染出 `GATEWAY_ADMIN_RESET value: "false"`；`grep -rn GATEWAY_ADMIN_RESET` 四份清单齐 | 默认 `false`；Swarm 无 `env_file`，注释点明「不列=静默失效」 |
| A-08 | ✅ 已修 | `docker-compose.yml:40-56`（driver 默认 `local`、去掉 `:?` 强制）+`:207-244`（storage 服务改为 `profiles: ["object-storage"]`、去掉 gateway 对它的 `depends_on healthy`）<br>`docker-stack.yml:96-104,118-122,197-205`（`STORAGE_*` + `airoute-data:/app/data` 持久卷）<br>`helm/airoute/values.yaml:111-140`（`config.server.redis_url`、`config.storage.*` 注释示例）<br>`.env.example:82-88`（`STORAGE_DRIVER=local`） | `docker compose config` exit 0：`storage` 被 profile 排除（`services` 只剩 gateway-1/2、lb、postgres、redis），gateway 的 `depends_on` 只剩 postgres/redis ⇒ 「cp .env.example .env && docker compose up」不再因缺 S3 凭据失败 | 无 S3 凭据时由 `config.Validate` 在 `driver=s3` 时报错拒绝启动（`cmd/airoute/server.go:341-349`）。storage 服务保留为可选 profile：`docker compose --profile object-storage up` |
| A-09 | ✅ 已修 | `internal/middleware/trace.go:136-145` | 代码审查 + `go build ./internal/middleware/`；跳过清单现为 `/health`、`/health/*`、`/healthz`、`/healthz/*`、`/readyz`、`/api/v1/public/status`、`/metrics` | 四份部署清单实际探的是 `/health/ready`（每 5–10s），此前恰好没被过滤 |
| A-10 | ✅ 已修 | `internal/middleware/auth.go:243-290`（抽出 `recordAdmissionRejection` 单一出口 + `auditDenial`）、`:465-466`（存储覆写后 key 额度 402）、`:534-540`（`AccountLockedDenial` 403 / `AccountDenial` 钱包 402）、`:558-560`（在途额度 402）<br>`internal/middleware/ratelimit.go:174-178`（429 落审计）<br>`internal/middleware/auth_audit_denials_test.go`（新文件，2 条端到端用例） | `go test ./internal/middleware/ -run 'TestAuthMiddlewareAuditsWalletRejection\|TestRateLimitMiddlewareAuditsThrottle\|TestAuditAuthFailureSkipsInbound401'` 通过：钱包 402 与 429 各恰好 1 行 `usage_logs`（`usage_source=gateway_auth`，`api_key` 为 digest），401 仍为 0 行 | **注意**：`auth.go` 的这部分改动已被并行会话的 `6fd2fa2 feat(admission)` 提交一并纳入 HEAD（`git show 6fd2fa2 -- internal/middleware/auth.go` 可查），因此不在 `git diff` 里；内容仍是本域实现。401 不落审计的刻意例外保持不动；`auditDenial` 直接写 context 里的 digest（再次 hash 会写出 join 不到任何 key 的行） |
| A-11（middleware 侧） | ✅ 已修 | `internal/middleware/trace.go:99-104`（删除 `WithTraceID` 死包装）<br>`tests/trace_and_ha_test.go:11-16,152-155`（改用 `telemetry.WithTraceID`） | `go test ./tests/ -run TestDB_DialectRebind` 通过（overlay 方式，见 §3）；`grep -rn "middleware.WithTraceID"` 0 命中 | 存储侧自造 trace_id 归 E 域，未动 |
| A-12 | ✅ 已修 | `internal/middleware/ratelimit.go:54-58`（删除 `Capacity()`/`Rate()`，保留行为注释）<br>`tests/audit_remediation_test.go:322-336`（改为行为断言：升到 6000 RPM 后 20ms 内可再取 token；1 RPM 下不可能） | `go test ./tests/ -run TestAudit_RateLimiterDynamicRPMUpdate` 通过 | 不再暴露内部状态，改为验证「热更新确实生效」 |
| A-13 | ✅ 已修（保守口径：密码保持 8、签名密钥新增 ≥32） | `cmd/airoute/security_check.go:23-31,44-50`（`minAdminSecretLen=32`，保留 5 个占位串精确匹配）<br>`cmd/airoute/security_check_test.go:44-52`（新增 `abc`、31 字符两条用例）<br>`.env.example:48-50`<br>`docs/CONFIGURATION.md:111-112,184-188`<br>`deploy/kubernetes.yaml:127-129`（16→8 的口径修正） | `go test ./cmd/airoute/ -run TestProductionSecurityIssues` 通过（6 组弱值 + 1 组强值） | 选**保守**方案：不改密码阈值（8→16 会让 8–15 字符密码的既有部署直接起不来）。**影响面**：`GATEWAY_ADMIN_SECRET` 短于 32 字符的既有部署将 fail-closed 拒绝启动，需换 ≥32 字符随机值（轮换会使已有 admin session 失效）；已在 CONFIGURATION.md 明写该破坏性变更 |
| A-14 | ✅ 已修（两条路都做） | `configs/config.yaml:1-20`（头部措辞改为「标 ${VAR} 的参数…」，三项改为 `${GATEWAY_READ_TIMEOUT_SEC:-600}` / `${GATEWAY_WRITE_TIMEOUT_SEC:-600}` / `${GATEWAY_LOG_LEVEL:-info}`）<br>`cmd/airoute/server_env.go`（新文件，`applyServerEnvOverrides`）<br>`cmd/airoute/server.go:69-72`（调用点）<br>`cmd/airoute/server_env_test.go`（新文件，4 组子用例）<br>`.env.example:14-22`、`docs/CONFIGURATION.md:102-105` | `go test ./cmd/airoute/ -run TestApplyServerEnvOverrides` 通过；另用 overlay 临时用例验证 `configs/config.yaml` 本身：默认 (600,600,info)、设环境变量后 (900,1200,debug) | 双保险：YAML 展开值可被 env 覆盖，`server.go` 又显式覆写（因为 Compose 只读挂载配置文件，改文件不现实）。非法/非正值忽略，YAML 值胜出 |
| A-15 | ⏭ 不属本域 | — | — | 由 E（`S3_ENDPOINT`）、F（`STRIPE_BASE_URL`）删除，本域无责 |
| C-09 | ✅ 已修 | `cmd/airoute/pricing_startup_check.go`（新文件：`warnUnpricedModels` / `warnUnpricedServedModels` 各自独立判断与告警）<br>`cmd/airoute/server.go:301-315`（goroutine 改为顺序调用两个独立段落） | `go build ./cmd/airoute/`、`go vet` 通过（新增文件无单测，因两条检查都直接查库+打日志；逻辑为纯拆分，原两条日志 payload 逐字保留） | 原 `if len(unpriced)==0 { return }` 短路已消除：`servedUnpriced` 有值就独立告警。未改动该 goroutine 之外的任何逻辑 |
| C-01（测试侧，Lead 追加） | ✅ 已修 | `tests/audit_remediation_test.go:308-320`（2 处 `CalculateCostDetailed` → `CalculateCostForUnitsWithCacheCreation(model,"default",prompt,completion,cached,0,1,t)`，7 参，第 6 个返回值 `snapshot` 用 `_` 接） | `go test ./tests/ -run TestAudit_WeekdayCustomOffPeakDiscount` 通过（断言值 15.0/30.0 不变） | C 域已删 5 个变体；本文件归本域独占修改 |
| C-12（Lead 追加，收口） | ✅ 已修（**统一为 60s 滑动窗口**） | `internal/middleware/ratelimit.go`（整体重写限流器：`rpmWindow` + `allow`/`allowAt`/`Allow`，`rpmWindowSpan=60s`）；`internal/middleware/authz.go:278-282`（降级路径注释）；`internal/middleware/ratelimit_window_test.go`（新文件，4 条用例）；`docs/CONFIGURATION.md:346` | `go test ./internal/middleware/` ok；`TestRegister*`（8 条）全过；`TestRateLimitMiddleware_Enforcement`、`TestAPIKey_ActiveVsDisabled`、`TestUserWallet_RedemptionAndRecharge`、`TestAudit_RateLimiterDynamicRPMUpdate` 全过且**未改任何断言** | 本地降级实现由「满桶令牌桶」改为与 `internal/distributed/redis_quota.go` 完全一致的滑动窗口：窗口 `(now-60s, now]`、先剔除 `<= now-60s` 再判 `count < rpm` 才记录。`rpm=5` ⇒ 连续 5 次放行、第 6 次 429、60s 后恢复；rpm 热更新重置窗口。C-12 的根因（两套窗口语义不等价、Redis 抖动回退多放行一窗）消除 |
| C-11（Lead 追加，收口） | ✅ 已修（glob 下沉到叶子包 `internal/model`） | 新增 `internal/model/model_pattern.go`（`MatchModelPattern` + `globMatch`，含 `*` 可跨 `/` 的语义与注释）<br>`internal/billing/model_pattern.go:1-16`（改为 3 行委托，保留 `patternSpecificity`）<br>`internal/config/validation_rules.go:44-56`（删除内联 prefix/suffix/exact 分支，改调 `model.MatchModelPattern`；空 pattern 归一为 `*`）<br>新增 `internal/model/model_pattern_test.go`（32 组用例）、`internal/config/validation_rules_glob_test.go`（等价性 + 端到端 + infix 用例） | `go test ./internal/model/ ./internal/billing/ ./internal/config/ ./internal/validator/` 全 ok；等价性用例把旧 matcher 作为 reference 实现在测试里，对 6 种 pattern × 9 个模型逐一比对，结果全同；`go test ./internal/controlplane/ ./tests/` 亦 ok | 之所以不能直接在 config 里 import billing：`go list -deps ./internal/billing` 经 `internal/storage` 依赖 `internal/config`，config→billing 是**循环依赖**（已实测）。`internal/model` 是叶子包且被双方 import，唯一无环落点。旧实现把 `gpt-4o*mini` 当字面名（永不命中），统一后按 pricing 已文档化的语义命中——这是有意的扩展，已单列测试钉住 |

## 2. 关键实现说明

### A-01 为什么必须用 `ResolveClientIdentity` 而不是「peer 是私网就放行」

出厂 Compose 里 gateway 只 `expose: 8080`，所有请求来自 nginx 容器。旧代码 `ResolveClientIP` 丢掉
`ClientIdentity.Forwarded`，于是「peer 是 172.x」被当成「客户端在私网」⇒ 公网请求全部放行。
新 `decide()` 三分支：

1. `Forwarded==true`（来自可信代理、可归因）→ 按 `identity.IP` 判 loopback/私网/link-local 放行，公网 403；
2. `Forwarded==false` 且 peer 是 loopback → 放行（本机开发/回环绑定）；
3. `Forwarded==false` 且 peer 非 loopback → **403 fail-closed**，文案直接提示配置 `GATEWAY_TRUSTED_PROXIES`。

显式清单（`GATEWAY_ADMIN_ALLOWED_IPS`）分支匹配 `identity.IP`；`Forwarded==false` 时只有「peer 本身命中清单」才放行（于是无代理的 LAN/集群直连仍有可用逃生口）。

为了让出厂拓扑真的能归因，`docker-compose.yml` 新增固定 subnet 的 `edge` 网络（lb 与两个 gateway 共享，lb 只挂 `edge`；gateway 再挂 `default` 以访问 postgres/redis/可选 storage），并把
`GATEWAY_TRUSTED_PROXIES` 默认值设为该网段。**subnet 选 `172.30.0.0/16`**：本机已有
`172.28.0.0/16`（data-analysis_app-network）、`172.22.0.0/16`（本项目旧的 default 网络）被占用，用它们会导致 `docker compose up` 报地址池重叠。

### A-08 为什么把 storage 服务改成 profile

若只把 gateway 的 `STORAGE_S3_*` 改成 `${VAR:-}`，`storage`（rustfs）服务自身的 `:?` 与
gateway 的 `depends_on: storage: condition: service_healthy` 仍会让「没填 S3 凭据」的
`docker compose up` 整体失败 —— 与 `configs/config.yaml` 的「local 开箱即用」冲突。因此：
storage 服务加 `profiles: ["object-storage"]`（默认不启动）+ 去掉 gateway 对它的依赖；
local 模式产物落在已有的 `gateway-data` 卷（`/app/data`）里，开箱即用。

### 无代理部署的可用性（A-01 的必然副作用，已文档化）

新语义下「peer 非 loopback 且无转发头」一律拒。裸二进制/LAN 直连、`deploy/kubernetes.yaml` 无
ingress 的场景必须用 `GATEWAY_ADMIN_ALLOWED_IPS` 列出客户端 CIDR（显式清单同样接受「peer 本身命中」）。
已写入 `docs/CONFIGURATION.md` 的 Control-plane exposure 段、`docker-stack.yml` /
`deploy/kubernetes.yaml` / `helm values.yaml` 的注释。

## 3. 验证结果（实际输出摘要）

```
$ gofmt -l <本域全部改动文件>
（空）

$ go build ./...
（无输出，exit 0）

$ go vet ./...
（无输出，exit 0）

$ go test ./internal/middleware/ ./cmd/airoute/ ./internal/config/ -count=1
ok  github.com/ifnodoraemon/airoute/internal/middleware  1.289s
ok  github.com/ifnodoraemon/airoute/cmd/airoute          3.338s
ok  github.com/ifnodoraemon/airoute/internal/config      0.035s

$ go test ./... -count=1        # 全仓（C-11/C-12 收口后最终一次）
ok  cmd/airoute, internal/{adapter,api,billing,config,controlplane,distributed,
    middleware,model,provider,router,storage,telemetry,validator}, tests, web   (16/16)
```

`tests` 的 `TestAdminAuth_LoginAndProtectedRoutes` 已在 D 域收尾后恢复绿色（`go test ./tests/` 现为 ok）；
上述段落记录的是本轮中途观察到的他域在途状态。

C-12 收口后的复验（Lead 点名的 9 个用例，**一个断言都没改**）：

```
$ go test ./internal/controlplane/ -run 'TestRegister' -count=1
ok  github.com/ifnodoraemon/airoute/internal/controlplane  1.433s
   （TestRegisterIsRateLimitedPerClient: throttled after 5 successful registrations ✓）

$ go test ./tests/ -run 'TestRateLimitMiddleware_Enforcement|TestAPIKey_ActiveVsDisabled|TestUserWallet_RedemptionAndRecharge|TestAudit_RateLimiterDynamicRPMUpdate' -count=1
ok  github.com/ifnodoraemon/airoute/tests  0.158s   （4/4 PASS，含 request 2 expected 200 OK ✓）

$ go test ./internal/middleware/ -run 'TestRPMWindow|TestRateLimitMiddleware' -count=1
PASS  TestRPMWindowMatchesRedisSlidingWindow / TestRPMWindowHotUpdateResetsWindow /
      TestRateLimitMiddlewareAllowsFullBurstThenThrottles / TestRPMWindowKeepsPerMinuteAllowance /
      TestRateLimitMiddlewareAuditsThrottle

$ go test ./internal/middleware/ ./internal/controlplane/ ./tests/ ./cmd/airoute/ -count=1
ok    internal/middleware
FAIL  internal/controlplane   ← TestSavePricingRateHTTPContract / TestPricingServiceSaveRejectsOverlappingOffPeakSlots
ok    tests
ok    cmd/airoute
```

`internal/controlplane` 的 2 条 off-peak 槽位用例（`TestSavePricingRateHTTPContract`、
`TestPricingServiceSaveRejectsOverlappingOffPeakSlots`）曾红，是 F 域 `PricingService.Save` 尚未接
`billing.ValidateOffPeakSlots`（[fixes-billing.md](fixes-billing.md) handoff 第 3 条）；F 域收尾后已绿，
最终全仓 16/16 通过。

补充验证：

```
$ docker compose config                      # exit 0
  edge: subnet 172.30.0.0/16；gateway-1/2 networks=[default, edge]；lb networks=[edge]
  storage 服务被 profile 排除；gateway depends_on=[postgres, redis]
$ helm template test helm/airoute             # exit 0，含 GATEWAY_ADMIN_RESET value: "false"
$ python3 -c 'yaml.safe_load_all(...)'        # docker-compose/docker-stack/helm values/k8s/config.yaml 全部 OK
$ go test ./tests/ -run 'TestAudit_WeekdayCustomOffPeakDiscount|TestAudit_RateLimiterDynamicRPMUpdate|TestDB_DialectRebind' -overlay=<剔除他域坏文件的 overlay>
ok  github.com/ifnodoraemon/airoute/tests    0.061s
```

## 4. handoff

1. **→ Lead / D 域**：`tests/admin_auth_test.go:82-89` 的 `/api/v1/user/me` 曾因 D 域删除重复端点而 404；
   D 域收尾后 `go test ./tests/` 已恢复 ok，无需本域再动（如需复查：路径应为 `/api/v1/auth/me`）。
2. **→ Lead（本地未跟踪 `.env`）**：仓库根的 `.env`（gitignored）目前是
   `GATEWAY_TRUSTED_PROXIES=172.22.0.0/16` 与 `STORAGE_DRIVER=s3`。新的 Compose 固定网段是
   `172.30.0.0/16`，因此本机跑 `docker compose up` 前需把该行改为空或 `172.30.0.0/16`
   （否则控制面会 fail-closed 403）。`.env` 不在本域可写清单内，未改。
3. **→ 部署运维（破坏性变更，已写入 CONFIGURATION.md）**：
   - `GATEWAY_ADMIN_SECRET` 现在必须 ≥32 字符，旧短密钥会拒绝启动；
   - 反代部署必须配置 `GATEWAY_TRUSTED_PROXIES`（否则控制面 fail-closed 403）；
   - 无代理部署必须配置 `GATEWAY_ADMIN_ALLOWED_IPS`。
4. **→ E 域（A-11 存储侧）**：`internal/storage/async_logger.go` 与 `repo_logs_writer.go` 仍各自用
   `SessionID` 冒充 `trace_id` 并生成第三种格式；本域只删了 middleware 侧死包装。
5. **→ C/E 域（C-12 的文档联动，未改他人文件）**：`internal/distributed/redis_quota.go:10-31` 的
   package 注释仍在描述「本地是满桶令牌桶、两者不等价、回退会放宽」，这段话在本次 C-12 收口后**已过期**
   （本地现在是同语义 60s 滑动窗口）。该文件不属本域，请其所有者把这段注释改为「两套实现语义一致，
   回退仅失去跨副本视图」。
6. **→ 已完成（C-11）**：glob 已下沉到 `internal/model/model_pattern.go`，`billing.MatchModelPattern`
   退化为 3 行委托，`config.GetValidationRule` 改为调 `model.MatchModelPattern`（空 pattern → `*`）。
   `internal/billing` 侧只改了 `model_pattern.go`（C 域知情并授权）；`internal/model` 只新增 1 个文件
   （B 域知情并授权）。后续若要再加 pattern 语义（如 `?` 单字符），只改 `internal/model` 一处即可。
7. **→ A-06 的 x 层联动**：`cmd/airoute/timeout_chain.go` 的 `edgeProxyTimeout` 仍取
   `model.EdgeProxyTimeoutSeconds`；本轮新增的是 `serverWriteTimeout`（读 cfg）。若以后要统一
   Compose/Helm 的 `write_timeout_sec` 默认值（600 vs 180），`configs/config.yaml` 与
   `helm/airoute/values.yaml:117-119` 需一起改，`server_env.go` 的覆盖顺序已就绪。
8. **→ 后续（C-12 的 Redis 侧一致性）**：本地窗口现在与 Redis 严格一致（窗口宽度、边界、判序都相同），
   但两份实现仍各自维护；`internal/distributed` 的 `rpmWindowMs` 与 middleware 的 `rpmWindowSpan`
   若将来改窗口宽度需同步（建议在任一侧加注释互指，本域已在 `rpmWindowSpan` 注释里指明）。
