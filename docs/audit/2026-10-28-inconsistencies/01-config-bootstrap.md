# 配置/启动/部署一致性审计（2026-10-28）

## 概览

审计基线 commit `1520d99`（域：`cmd/airoute/`、`internal/config/`、`internal/middleware/`、部署清单与配置文档）。共 15 项发现，其中高置信度 14 项、中 1 项。

最值得先修的三项：**A-01** D15 控制面守卫在三种发布拓扑的出厂配置下判定失真（反向代理之后的 peer 恒为私网 ⇒ 守卫从不拒绝），**A-02** nginx 侧的同一策略只加在 3 份配置的其中 1 份、且只加在默认 vhost（`console.*`/`admin.*` vhost 完全无 ACL）；这两条合起来意味着「管理面默认只允许私网/回环」这个承诺在出厂部署里不成立。第三是 **A-04/A-05**：边缘 50m 与网关 64MiB 的 body 上限互相矛盾且错误文案归错了组件，首 token 超时的语义被两份文档反向描述。

其余多为「半接线」：超时链自检只校验了 channel↔edge 一层（漏掉真正会先截断的 Go `WriteTimeout`）、`GATEWAY_ADMIN_RESET` 与 `STORAGE_*` 只在 Compose 通路上接了线（Swarm/Helm 静默缺项）、审计 401/402 的落库规则在 middleware 内部不对称、trace_id 生成有三处实现且两处拿 session_id 冒充。

## 发现

### A-01 D15 控制面守卫在反向代理之后恒判「私网」，出厂配置下等于没有
- **类别**：不对称强制 / 半接线
- **位置**：`internal/middleware/admin_access.go:158`、`internal/middleware/authz.go:296`、`internal/middleware/authz.go:432`、`docker-compose.yml:33`、`docker-compose.yml:229`、`.env.example:32`、`docs/CONFIGURATION.md:214`
- **设计意图**：`admin_access.go:15-25` 明说守卫要「在网关内部、在每个代理之后」强制控制面只允许 loopback/私网；`docs/CONFIGURATION.md:214` 承诺「public clients receive `403` even through the catch-all virtual host」。为此 `ClientIdentity` 专门保留了 `Forwarded` 字段（`authz.go:293-300`：「callers that apply a per-IP penalty should only do so when this is true」）。
- **现状**：守卫用的是**丢失 `Forwarded` 信息**的 `ResolveClientIP`（`authz.go:432-435` 只取 `.IP`）。Compose 出厂把 `GATEWAY_TRUSTED_PROXIES` 传成空（`docker-compose.yml:33` 默认 `${GATEWAY_TRUSTED_PROXIES:-}`，`.env.example:32-33` 注释为空），于是 `ResolveClientIdentity` 走「peer 不可信」分支，返回**LB 容器的私网地址**（`authz.go:402-405`）。`decide()` 看到 `ip.IsPrivate()` 就直接放行（`admin_access.go:169-171`）。出厂拓扑里网关只 `expose: 8080`（`docker-compose.yml:138`，不发布宿主端口），所有请求都必然经过 LB ⇒ **所有客户端在守卫眼里都是私网**，公网请求同样放行。`ResolveClientIdentity` 的正确用法在同一仓库里就有（`internal/controlplane/account_handler.go:55-59` 用 `IPIsForwarded`），只有这个安全守卫退化了。
- **证据**：
```go
// internal/middleware/admin_access.go:158-171
	raw := ResolveClientIP(c)          // <- drops ClientIdentity.Forwarded
	ip := net.ParseIP(raw)
	if ip == nil { ... }
	...
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return "", 0                    // LB 的 172.x 地址 -> 放行
	}
```
```go
// internal/middleware/authz.go:402-405
	if err != nil || !ipInNetworks(peerIPValue, trusted) {
		// Untrusted (or unconfigured) peer: ... the peer is the only address we can attribute.
		return ClientIdentity{IP: peer}   // Forwarded=false，但调用方丢掉了这个信息
	}
```
- **影响**：高 — 「管理面默认仅私网可达」在出厂 Compose/Swarm/Helm 下不成立，只剩 JWT 一道门；登录/注册限流也因此退化（所有客户端共用 LB 的 IP 桶）。附带：文档让你把 `GATEWAY_TRUSTED_PROXIES` 设成 Compose 网段，但 `docker-compose.yml` 没有 `networks:` 段、不固定 subnet，注释里的 `172.22.0.0/16` 并非实际分配值。
- **置信度**：高
- **建议**：守卫改用 `ResolveClientIdentity`，`Forwarded==false` 即视为「无法确定客户端」并 fail-closed；同时让 Compose 固定子网并默认把该网段写进 `GATEWAY_TRUSTED_PROXIES`。

### A-02 nginx 的控制面 ACL 只加在 3 份配置中的 1 份，且只覆盖默认 vhost
- **类别**：半接线 / 不对称强制
- **位置**：`deploy/nginx-compose.conf:168`、`deploy/nginx-compose.conf:97`、`deploy/nginx-compose.conf:55`、`deploy/swarm/nginx-swarm.conf:56`、`deploy/nginx.conf:16`、`docs/CONFIGURATION.md:214`
- **设计意图**：`deploy/nginx-compose.conf:37-43` 与 `:164-167` 明确说「边缘也要限制控制面，作为 defence in depth（网关在代理之后执行同一策略）」。
- **现状**：`allow/deny` 只出现在 `nginx-compose.conf` 的**默认 vhost**（168-175）；同文件的 `console.airoute.internal` / `admin.airoute.internal` vhost（97-141）的 `location /` 完全没有 ACL —— 而它正是为控制面准备的那一个，只要请求带 `Host: console.<任意后缀>`（正则 `~^(console|admin)\..+$` 会命中）就走这条路，无需 DNS。`api.*` vhost 反而是 403（58-61）。另外两份出厂 nginx：`deploy/swarm/nginx-swarm.conf` 全文 0 条 `allow`（只有 api.* 的 403）、`deploy/nginx.conf` 单 vhost 无任何 ACL。配合 A-01（网关侧守卫也判定失真），公网客户端带 `Host: console.x` 即可到达 `/api/v1/admin`。文档只承诺「catch-all vhost 会 403」，掩盖了这个更宽的洞。
- **证据**：
```nginx
# deploy/nginx-compose.conf:97-100（console/admin vhost，无 allow/deny）
server {
    listen 80;
    server_name console.airoute.internal admin.airoute.internal ~^(console|admin)\..+$;
    location = / { return 302 /app/; }
```
```nginx
# deploy/nginx-compose.conf:168-175（只有默认 vhost 有 ACL）
    location ~ ^/(api/v1/admin|app)(/|$) {
        allow 127.0.0.1; allow 10.0.0.0/8; allow 172.16.0.0/12; ...
        deny all;
```
- **影响**：高 — 边缘层唯一生效的控制面网络边界只保护 Host 不匹配的那条路；Swarm/单机配置完全没有边缘边界。
- **置信度**：高
- **建议**：把同一份 `allow/deny`（或 `include` 片段）加到所有 vhost 的 `^/(api/v1/admin|app)` location，三份 nginx 配置统一；并用 `nginx_timeout_test.go` 同款手段做清单断言。

### A-03 控制面守卫的路径清单漏掉 `/`，而同一份控制台外壳在 `/` 上无守卫
- **类别**：半接线
- **位置**：`internal/middleware/admin_access.go:78`、`web/web.go:78`、`web/web.go:84`
- **设计意图**：守卫覆盖「the embedded console under /app」（`admin_access.go:50-54`），目的是让控制台不对外。
- **现状**：`isControlPlanePath` 只认 `/api/v1/admin` 与 `/app`；`web.RegisterStaticRoutes` 同时把同一份 `index.html` 挂在 `/`（`web.go:78`）和 `/app/`（`web.go:85`）。所以守卫在 `/app` 上限制的那份 SPA，用 `/` 一样能拿到。`/api/v1/admin` 仍被守卫（虽然 A-01 让它失效），但「控制台外壳仅私网可见」这个目标从未成立。
- **证据**：
```go
// internal/middleware/admin_access.go:78-85
func isControlPlanePath(path string) bool {
	switch {
	case path == "/api/v1/admin", strings.HasPrefix(path, "/api/v1/admin/"):
		return true
	case path == "/app", strings.HasPrefix(path, "/app/"):
		return true
	}
	return false
}
```
```go
// web/web.go:78, 84-85
	r.GET("/", serveIndex)                                    // 同一份控制台外壳，无守卫
	r.GET("/app", func(c *gin.Context) { c.Redirect(301, "/app/") })
	r.GET("/app/", serveIndex)
```
- **影响**：中 — 守卫的路径集合与实际暴露面不一致；只改 `/app` 的 ACL 不会改变控制台的可达性（注：A-01 修复后数据面仍受保护，故影响限于「外壳可达」而非「数据可取」）。
- **置信度**：中
- **建议**：要么把 `/` 纳入 `isControlPlanePath`（并接受 `http://127.0.0.1:8080` 的可用性由 IP 判定保证），要么把控制台入口统一收敛到 `/app`。

### A-04 边缘 50m 与网关 64MiB 的 body 上限互相矛盾，413 文案还归错了组件
- **类别**：不一致强制 / 文档脱节
- **位置**：`deploy/nginx-compose.conf:7`、`deploy/nginx-compose.conf:63`、`deploy/nginx.conf:11`、`deploy/swarm/nginx-swarm.conf:24`、`helm/airoute/values.yaml:56`、`internal/middleware/body_limit.go:14`、`.env.example:103`、`docs/CONFIGURATION.md:142`
- **设计意图**：`GATEWAY_MAX_BODY_BYTES` 是网关自己的上限（默认 64 MiB，`.env.example:103` 明确给 `67108864`），文档把 64 MiB 描述为「Maximum accepted request body size」。
- **现状**：三份 nginx 与 Helm ingress 全部把边缘卡在 `50m`（= 52,428,800 B），**小于**网关的 64 MiB（67,108,864 B）。50–64 MiB 的请求永远到不了网关，却被 nginx 用一句 `payload exceeds 50MB gateway limit` 拒绝（`nginx-compose.conf:63-68`、`:157-162`），把边缘限制说成「gateway limit」。运维按文档调大 `GATEWAY_MAX_BODY_BYTES` 不会生效，报警文案还会指向错误的组件。
- **证据**：
```nginx
# deploy/nginx-compose.conf:7, 63-67
client_max_body_size 50m;
    error_page 413 = @error413;
    location @error413 {
        return 413 '{"error":{"message":"Request entity too large: payload exceeds 50MB gateway limit.",...}}';
```
```go
// internal/middleware/body_limit.go:11-24
// defaultMaxBodyBytes caps request bodies at 64 MiB, comfortably above the
const defaultMaxBodyBytes int64 = 64 << 20
```
- **影响**：中 — 上限不可按文档调优；413 的错误归因误导排障（且 50m 与 64MiB 的差值正好落在多模态大 payload 区间）。
- **置信度**：高
- **建议**：边缘与网关统一到同一个数值（推荐都用 `GATEWAY_MAX_BODY_BYTES` 的默认 64 MiB，或把默认降到 50 MiB 并同步 `.env.example`/文档），并把 413 文案改成「edge proxy limit」。

### A-05 `GATEWAY_FIRST_TOKEN_TIMEOUT` 的语义被两份文档反向描述
- **类别**：文档脱节
- **位置**：`.env.example:139`、`docs/CONFIGURATION.md:131`、`internal/router/dispatcher_stream.go:277`- **设计意图**：代码注释明确：「GATEWAY_FIRST_TOKEN_TIMEOUT is documented as an upper bound, so it can shorten that window but never extend it past the channel budget」（`dispatcher_stream.go:277-281`）。
- **现状**：`.env.example:139-141` 写「留空 = 由渠道 timeout 派生；显式设置**只影响未配置 timeout 的渠道**」，`docs/CONFIGURATION.md:131` 写「Only applies to channels without `timeout_seconds`」。代码是 `min(渠道 timeout（或 30/60s 默认）, override)`（`dispatcher_stream.go:282-296`）：override 对**所有**渠道生效，只会缩短。按文档设置 `GATEWAY_FIRST_TOKEN_TIMEOUT=10` 以为「只影响没配 timeout 的渠道」的运维，会静默把 `timeout_seconds: 3600` 的推理渠道首字窗口压到 10s。
- **证据**：
```go
// internal/router/dispatcher_stream.go:282-295
func resolveFirstTokenTimeout(ch *model.ChannelConfig, modelName string, override time.Duration) time.Duration {
	timeout := 30 * time.Second
	if ch != nil && ch.TimeoutSeconds > 0 {
		timeout = time.Duration(ch.TimeoutSeconds) * time.Second
	} else { /* 60s for reasoning */ }
	if override > 0 && override < timeout {
		return override          // 对所有渠道生效，只缩短
	}
	return timeout
}
```
- **影响**：中 — 按文档调参会造成不明原因的推理请求超时；同一参数两份文档还与代码三方不一致。
- **置信度**：高
- **建议**：以代码为准改写 `.env.example:139-141` 与 `docs/CONFIGURATION.md:131`：「全局首字上限，只能缩短、不能放宽渠道 timeout」。

### A-06 超时链自检只证明 channel↔edge 一层，漏掉真正会先截断的 Go `WriteTimeout`
- **类别**：半接线
- **位置**：`cmd/airoute/timeout_chain.go:18`、`cmd/airoute/timeout_chain.go:66`、`cmd/airoute/server.go:383`、`configs/config.yaml:10`、`helm/airoute/values.yaml:110`
- **设计意图**：`timeout_chain.go:26-30` 说这个检查的目的就是「prove that invariant at startup instead of assuming it」，且 `server.go:222` 在启动时调用它；文档把「一个 operator-facing 超时」写进 `docs/CONFIGURATION.md:190-195`。
- **现状**：`reportTimeoutChain` 只比较 `channel timeout` 与常量 `edgeProxyTimeout`（86400s），并把 `edge_timeout=86400` 打进启动日志（`timeout_chain.go:50-56`）。它**完全不读** `cfg.Server.WriteTimeoutSec`（`server.go:385`，出厂 600s、Helm 180s）——而这是非流式请求真正会先撞上的那一道（流式由 `InitSSEStream` 清 deadline 后才轮到 edge）。于是启动日志给出「channel 3600s < edge 86400s ✔」的结论，实际 600s 就被网关自己切断；`channelTimeout > time.Hour` 的告警（58-65）也不会提到这一层。
- **证据**：
```go
// cmd/airoute/server.go:380-388
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSec) * time.Second, // 600s，未被任何检查引用
```
```go
// cmd/airoute/timeout_chain.go:66-71（只比这一层）
		if channelTimeout > edge {
			telemetry.ScopedLogger("router").Warn("channel timeout exceeds the edge proxy timeout; the proxy will cut requests first", ...)
		}
```
- **影响**：中 — 自检给出的「已证明」结论不成立，非流式长请求（或响应写出耗时长的请求）在 600s 被静默切断，且没有任何警告。
- **置信度**：高
- **建议**：`reportTimeoutChain` 增加 `channel/write_timeout` 这一对（或把 `write_timeout_sec` 纳入同一份派生关系），并在日志里同时打印 `server_write_timeout`。

### A-07 `GATEWAY_ADMIN_RESET` 只在 Compose 通路上接线，Swarm/Helm/K8s 静默无效
- **类别**：半接线 / 部署清单漂移
- **位置**：`docker-compose.yml:8`、`docker-stack.yml:81`、`helm/airoute/templates/deployment.yaml:51`、`deploy/kubernetes.yaml:26`、`docs/CONFIGURATION.md:114`
- **设计意图**：`GATEWAY_ADMIN_RESET` 是文档化的「一次性救回管理员密码」开关（`docs/CONFIGURATION.md:114`，`.env.example:52-54`）；Compose 里的 pass-through 注释把风险写得很清楚：「Without this pass-through the variable never reaches the container and the reset silently does nothing」。
- **现状**：只有 `docker-compose.yml:12` 传了它。`docker-stack.yml` 是 Swarm stack（不支持 `env_file`），其 `airoute` 服务 `environment` 列表（81-100）没有这一项；Helm `deployment.yaml` env（51-87）与静态 `deploy/kubernetes.yaml`（26-55）也没有。也就是说：管理员密码被改乱后，按同一份文档，只有单机 Compose 能救回来。
- **证据**：
```yaml
# docker-compose.yml:8-12
  # One-shot escape hatch: ... Without this
  # pass-through the variable never reaches the container and the reset silently
  # does nothing.
  GATEWAY_ADMIN_RESET: ${GATEWAY_ADMIN_RESET:-false}
```
```yaml
# docker-stack.yml:81-100（environment 全表，没有 GATEWAY_ADMIN_RESET）
    environment:
      - TZ=Asia/Shanghai
      - GATEWAY_HOST=0.0.0.0
      - DATABASE_URL=...
      - REDIS_URL=redis://airoute-redis:6379/0
      - GATEWAY_ADMIN_SECRET=${GATEWAY_ADMIN_SECRET:? ...}
```
- **影响**：中 — 运维应急开关在两种集群部署上静默失效；单机（Compose/裸 binary）有效，形成「同一能力按部署方式不同」的不一致。
- **置信度**：高
- **建议**：在 `docker-stack.yml`、Helm `values`+`deployment`、`deploy/kubernetes.yaml` 里补上同名 pass-through（默认 false），或用同一个 `env` 片段生成。

### A-08 产物存储的接线程度在三种清单里完全不同：Compose 强制 s3、Swarm 完全没有、Helm 只能走未文档化的 config 键
- **类别**：部署清单漂移 / 不对称强制
- **位置**：`docker-compose.yml:40`、`docker-compose.yml:43`、`configs/config.yaml:62`、`internal/config/storage_config.go:163`、`cmd/airoute/server.go:341`、`docker-stack.yml:79`、`.env.example:83`、`helm/airoute/values.yaml:105`
- **设计意图**：`configs/config.yaml:62-63` 把 `local` 定位成「本地磁盘目录，**默认开箱即用**」，`defaultStorageConfig` 也以 local 为安全基线（`internal/config/storage_config.go:30-46`）；`STORAGE_*` 只应在选 s3/rustfs 时才要求凭据（`server.go:342-347` 的拒绝启动逻辑也是按 driver 分支的）。
- **现状**：`docker-compose.yml:40` 把 driver 默认写成 `s3`，并在 `:43-44` 用 `${STORAGE_S3_ACCESS_KEY:?...}` / `${STORAGE_S3_SECRET_KEY:?...}` **无条件**要求凭据（storage 服务 `:207-208` 同样 `:?`）。于是「cp .env.example .env && docker compose up」在没填 S3 凭据时直接失败，而同一份仓库的 YAML 声称 local 开箱即用；.env.example:83 也把 `STORAGE_DRIVER=s3` 当默认值。另一个极端是 Swarm：`docker-stack.yml` 的 `airoute` 服务（79-137）没有任何 `STORAGE_*`、也没有服务定义或 `/app/data` 卷 → 镜像里烘进的 `configs/config.yaml`（Dockerfile:75）让 driver 落到 `local`，Skills 包写进容器可写层，任务重建即丢，且没有任何开关能改。Helm 居中：`values.yaml` 没有 storage/redis 一等键，但 `configmap.yaml:9` 会把 `.Values.config` 整块渲染成 config.yaml，所以 `--set config.storage.*` / `--set config.server.redis_url=…` 实际可用——只是完全没文档。
- **证据**：
```yaml
# docker-compose.yml:40-44（无条件强制凭据，与 config.yaml 的 local 默认冲突）
  STORAGE_DRIVER: ${STORAGE_DRIVER:-s3}
  STORAGE_S3_ENDPOINT: ${STORAGE_S3_ENDPOINT:-http://storage:9000}
  STORAGE_S3_ACCESS_KEY: ${STORAGE_S3_ACCESS_KEY:?Set STORAGE_S3_ACCESS_KEY in .env}
  STORAGE_S3_SECRET_KEY: ${STORAGE_S3_SECRET_KEY:?Set STORAGE_S3_SECRET_KEY in .env}
```
```yaml
# configs/config.yaml:62-65（声明 local 开箱即用）
# 支持模式: "local" (本地磁盘目录，默认开箱即用) | "s3" / "rustfs"
storage:
  driver: "${STORAGE_DRIVER:-local}"
```
- **影响**：中 — 同一能力三种接线：Compose 强制 S3、Swarm 无法配置（产物易失）、Helm 只能靠未文档化的值；「开箱即用」的承诺在 Compose 下不成立。
- **置信度**：高
- **建议**：Compose 改为「driver 默认 local；只有 driver=s3/rustfs 时才校验凭据」（compose 用 profiles 或去掉 `:?` 交给 `Validate()` 报错）；Swarm 补 `STORAGE_*` 与 `airoute-storage`/卷；Helm 把 `config.server.redis_url`、`config.storage` 写进 `values.yaml` 注释示例。

### A-09 访问日志的「跳过探针」清单漏了 `/health/ready`，而所有清单实际探的就是它
- **类别**：半接线
- **位置**：`internal/middleware/trace.go:143`、`docker-compose.yml:144`、`docker-stack.yml:106`、`helm/airoute/templates/deployment.yaml:98`、`deploy/kubernetes.yaml:76`
- **设计意图**：`trace.go:142` 注释「Skip high-frequency probes and metrics scrapers to keep logs clean」。
- **现状**：跳过清单是 `/health`、`/api/v1/public/status`、`/metrics`。但按 `cmd/airoute/deploy_readiness_test.go` 强制对齐的四份清单，Compose 每 10s、Swarm 每 10s、K8s readiness 每 5s 探的都是 `/health/ready`；`/healthz`、`/readyz`、`/v1/health` 这些别名也不在清单里。于是唯一的「高频探针过滤」过滤掉了没人用的 `/health`，真正的高频探针照旧每 5–10s 每个副本写一行 `http_access`。
- **证据**：
```go
// internal/middleware/trace.go:142-145
		// Skip high-frequency probes and metrics scrapers to keep logs clean
		if path == "/health" || path == "/api/v1/public/status" || path == "/metrics" {
			return
		}
```
```yaml
# docker-compose.yml:144
      test: ["CMD-SHELL", "curl -f http://127.0.0.1:$${GATEWAY_PORT:-8080}/health/ready || exit 1"]
```
- **影响**：低 — 日志噪音/存储成本；排障时真实请求被探针行淹没（每副本每天约 8.6k 行）。
- **置信度**：高
- **建议**：把 `/health/ready`、`/healthz*`、`/readyz`、`/v1/health` 一并放进跳过清单（或改成前缀匹配 `path == "/health" || strings.HasPrefix(path, "/health/")`）。

### A-10 「402/403/429 拒绝都落审计」的承诺只在 middleware 的两条快路径上实现
- **类别**：不对称强制（审计侧）
- **位置**：`internal/middleware/auth.go:34`、`internal/middleware/auth.go:214`、`internal/middleware/auth.go:258`、`internal/middleware/auth.go:304`、`internal/middleware/auth.go:313`、`internal/middleware/ratelimit.go:180`
- **设计意图**：`auditAuthFailure` 的注释把目的写死了：「logs a client-side admission rejection (insufficient balance, disabled or throttled key) into the audit table so **402/403/429 denials stay visible to operators**」（`auth.go:34-36`）。
- **现状**：只有「快路径 key status」与「快路径 key budget」两处调用 `auditAuthFailure`（214-229）。同一函数里后面三处同类拒绝都不调用：存储覆写后的 budget 拒绝（250-261）、钱包/账号拒绝（297-307，402 insufficient_quota）、在途额度拒绝（311-323，402）。`RateLimitMiddleware` 的 429（`ratelimit.go:170-185`）也从不落审计。结果：注释点名的「402/403/429」中，只有一类 402 会被记录，钱包不足与限流完全不留痕——而 `usage_logs` 恰恰是控制台请求日志的唯一来源。
- **证据**：
```go
// internal/middleware/auth.go:214-229（会落审计）
		if denial := KeyStatusDenial(effectiveKey.Status); denial != nil {
			auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
			AbortWithDenial(c, denial); return
		}
		if !freeRead {
			if denial := KeyBudgetDenial(...); denial != nil {
				auditAuthFailure(c, rawKey, denial.StatusCode, ...); AbortWithDenial(c, denial); return
```
```go
// internal/middleware/auth.go:304-307（同一含义，不落审计）
		} else if denial := AccountDenial(userStatus, userRole, userBalance); denial != nil {
			AbortWithDenial(c, denial)   // 无 auditAuthFailure
			return
		}
```
- **影响**：中 — 控制台看不到「余额不足/限流」导致的拒绝，用户投诉时无据可查；同一份日志里同类拒绝时有时无。
- **置信度**：高
- **建议**：把 `auditAuthFailure` 收进一个 helper，在 `AuthMiddleware` 的所有 402/403 出口与 `RateLimitMiddleware` 的 429 出口统一调用（401 保持现有刻意的例外）。

### A-11 trace_id 有三处生成实现，其中两处拿 session_id 冒充 trace_id，`middleware.WithTraceID` 是死包装
- **类别**：重复漂移 / 死接线
- **位置**：`internal/middleware/trace.go:34`、`internal/middleware/trace.go:106`、`internal/storage/async_logger.go:101`、`internal/storage/repo_logs_writer.go:183`
- **设计意图**：项目 ID 体系明确区分 `trace_id=请求链路` 与 `session_id=会话亲和`（`.opencode/memory/project.md:12`），trace_id 由 middleware 生成（`trace.go:40-59`）。
- **现状**：(1) `middleware.WithTraceID`（`trace.go:104-108`）只是 `telemetry.WithTraceID` 的一层包装，全仓库（含测试）零调用——deadcode 已确认；真正在用的是 `trace.go:51` 直接调 telemetry。(2) 存储侧另有两处**自造** trace_id：`async_logger.go:101-108` 与 `repo_logs_writer.go:183-189`，逻辑一模一样，且第一条分支是 `rec.TraceID = rec.SessionID`——把会话亲和 ID 写进链路 ID 列。此时同一 session 的所有请求在 `usage_logs.trace_id` 上会塌成一个值，而格式（`tr-<微秒>`）又与 middleware 的 `tr-<微秒%1e8>-<12B hex>` 不同，按 trace_id 关联日志与审计行的能力直接失效。
- **证据**：
```go
// internal/storage/async_logger.go:101-108（与 repo_logs_writer.go:183-189 重复）
	if rec.TraceID == "" {
		if rec.SessionID != "" {
			rec.TraceID = rec.SessionID          // session_id 冒充 trace_id
		} else {
			rec.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixMicro())  // 第三种格式
		}
	}
```
```go
// internal/middleware/trace.go:104-108（零调用）
// WithTraceID attaches traceID to a standard context. Callers outside this package
// (tests, background jobs) use it instead of hand-building a context key.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return telemetry.WithTraceID(ctx, traceID)
}
```
- **影响**：中 — 审计行与访问日志按 trace_id 无法关联；死包装误导调用者以为存在两条入口。
- **置信度**：高
- **建议**：删除 `middleware.WithTraceID`；两处存储兜底抽成一个函数并只生成 `tr-<微秒>-<hex>`（不再用 session_id），或在注释里写明「为兼容旧行为」。

### A-12 速率限制器的 `Capacity()` / `Rate()` 是死方法（仅测试引用）
- **类别**：死接线
- **位置**：`internal/middleware/ratelimit.go:54`、`internal/middleware/ratelimit.go:62`
- **设计意图**（推断）：`tokenBucket` 的容量/速率由 `getBucket` 在热更新时改写（`ratelimit.go:87-98`），两个访问器应为「控制台/诊断读取当前桶参数」预留，但该读取点从未落地。
- **现状**：全仓库只有 `tests/audit_remediation_test.go:333-334` 调用它们；生产代码没有任何读取点（deadcode 亦已标记）。
- **证据**：
```go
// internal/middleware/ratelimit.go:54-66
// Capacity returns current bucket capacity.
func (tb *tokenBucket) Capacity() float64 { tb.mu.Lock(); defer tb.mu.Unlock(); return tb.capacity }
// Rate returns current refill rate.
func (tb *tokenBucket) Rate() float64 { tb.mu.Lock(); defer tb.mu.Unlock(); return tb.rate }
```
- **影响**：低 — 死代码；`tokenBucket.Allow()`（49-52）虽然也只是 `allow()` 的包装，但它是活的（`authz.go:252` 调用），只有 `Capacity`/`Rate` 两个访问器无人读取。
- **置信度**：高
- **建议**：删除这两个未使用的访问器（或把测试改成读结构体字段后删）。

### A-13 凭证强度门禁：文档承诺「16 字符密码 / 弱签名密钥拒绝」，代码只查 8 字符与 5 个固定占位串
- **类别**：文档脱节
- **位置**：`.env.example:48`、`docs/CONFIGURATION.md:111`、`cmd/airoute/security_check.go:25`、`cmd/airoute/security_check.go:38`
- **设计意图**：启动门禁是 fail-closed 的安全承诺（`docs/CONFIGURATION.md:184-186`：「An empty signing secret, a password equal to admin123 (or shorter than 8 characters), or one of the placeholder values that ship in sample manifests aborts startup」）。
- **现状**：`.env.example:48-49` 写「必须为强随机值（密码 >= 16 字符，签名密钥 >= 32 字符）」「**弱值**或仓库公开占位值会让进程直接拒绝启动」。代码里密码阈值是 `len(pwd) < 8`（`security_check.go:44`，与 CONFIGURATION.md 的 8 一致，与 .env.example 的 16 不一致）；签名密钥只做**精确匹配** 5 个占位串（25-31），`GATEWAY_ADMIN_SECRET=abc` 这类明显弱值会被放行，与「弱值直接拒绝启动」相矛盾。
- **证据**：
```go
// cmd/airoute/security_check.go:25-31（只有精确占位串）
var weakAdminSecrets = map[string]struct{}{
	"": {}, "airoute-secret-key-change-me": {},
	"airoute-cluster-admin-shared-secret-key-32b": {}, ...
}
// :44 密码阈值是 8
		if pwd := cfg.GetAdminPassword(); pwd == "admin123" || len(pwd) < 8 {
```
- **影响**：中低 — 运维按 .env 承诺以为短密钥会被门禁拦住，实际会带病启动（HMAC 签名密钥可被暴力/猜测）。
- **置信度**：高
- **建议**：代码补长度校验（密码 ≥ 16、secret ≥ 32）或把 `.env.example` 改成「建议值」；二者必须一致。

### A-14 `configs/config.yaml` 顶部「所有参数均已与 .env 打通」不成立
- **类别**：文档脱节 / 写入不读
- **位置**：`configs/config.yaml:3`、`configs/config.yaml:10`、`configs/config.yaml:12`、`configs/config.yaml:45`、`helm/airoute/values.yaml:110`
- **设计意图**：文件头声明「所有参数均已与项目根目录下的 .env 统一环境变量打通，开箱即用」，`.env.example` 自我描述为「Single Source of Truth」。
- **现状**：`server.read_timeout_sec`/`write_timeout_sec`（10-11）与 `server.log_level`（12）是**硬编码字面量**，仓库里没有任何 `GATEWAY_*TIMEOUT*`/`LOG_LEVEL` 环境变量读取点（已全量 grep 确认），`server.go:69-76` 也只覆写 Port/Host；`model_validation.rules`（45-59）同理。也就是说这几项只能改文件，改 `.env` 无效——而 Compose 把 `./configs/config.yaml` 只读挂进容器（`docker-compose.yml:137`），运维必须改仓库文件。另一处后果：出厂 YAML 的 600s 与 Helm chart 渲染的 180s（`helm/airoute/values.yaml:110-111`）是两套默认值，`docs/CONFIGURATION.md:194` 只能靠文字说明二者不同。
- **证据**：
```yaml
# configs/config.yaml:2-12
# 所有参数均已与项目根目录下的 .env 统一环境变量打通，开箱即用
server:
  host: "${GATEWAY_HOST:-0.0.0.0}"
  port: ${GATEWAY_PORT:-8080}
  public_url: "${PUBLIC_URL}"
  read_timeout_sec: 600        # 无 env 通路
  write_timeout_sec: 600       # 无 env 通路
  log_level: "info"            # 无 env 通路
```
- **影响**：低 — 声明与事实不符；排查「为什么改了 .env 没生效」时会误导（尤其与 A-06 的超时链问题叠加）。
- **置信度**：高
- **建议**：把 header 改成「列表中标 ${VAR} 的项与 .env 打通」；或给这三项加 `GATEWAY_READ_TIMEOUT_SEC`/`GATEWAY_WRITE_TIMEOUT_SEC`/`GATEWAY_LOG_LEVEL` 并在 `server.go` 覆写，同时对 Helm 与出厂 YAML 取同一个默认。

### A-15 代码读取了两个清单里没有的变量别名（`S3_ENDPOINT`、`STRIPE_BASE_URL`）
- **类别**：文档脱节（清单遗漏方向）
- **位置**：`internal/storage/s3_storage.go:37`、`internal/controlplane/payment_handler.go:87`、`.env.example:86`、`.env.example:178`
- **设计意图**：`.env.example` 自我描述为「Single Source of Truth」且 `docs/CONFIGURATION.md:89` 声称「`.env.example` lists **every** variable below」；两个正确名字（`STORAGE_S3_ENDPOINT`、`STRIPE_API_BASE`）都已文档化，别名看起来是历史兼容。
- **现状**：`S3_ENDPOINT` 与 `STRIPE_BASE_URL` 是代码里的第二优先读取点，但在 `.env.example` 与 `docs/CONFIGURATION.md` 中都不存在，也没有任何注释标记它们是 legacy 别名（对比同仓 `GATEWAY_ADMIN_USERNAME`/`DEFAULT_TRIAL_BALANCE` 都明确注明了「旧别名」）。在出厂路径上这两个回退基本不可达：`GetStorageConfig()` 已把 `STORAGE_S3_ENDPOINT` 解析进 `cfg.Endpoint`（`internal/config/storage_config.go:174`），`STRIPE_API_BASE` 也总在前；因此它们是「清单外的手工后门」而非文档化能力。
- **证据**：
```go
// internal/storage/s3_storage.go:34-39
	endpointStr := cfg.Endpoint
	if endpointStr == "" {
		endpointStr = os.Getenv("STORAGE_S3_ENDPOINT")
		if endpointStr == "" {
			endpointStr = os.Getenv("S3_ENDPOINT")     // 清单里没有
		}
	}
```
```go
// internal/controlplane/payment_handler.go:85-88
		stripeBase := os.Getenv("STRIPE_API_BASE")
		if stripeBase == "" {
			stripeBase = os.Getenv("STRIPE_BASE_URL")  // 清单里没有
		}
```
- **影响**：低 — 运维按清单配置不会踩坑；但「清单即全集」的承诺不成立，且两个变量可以静默改变存储端点/Stripe 端点（例如容器环境里遗留的 `S3_ENDPOINT`）。
- **置信度**：高
- **建议**：要么在 `.env.example` 里补上并注明「旧别名，仅在前者为空时生效」，要么删掉这两处回退。

## 已排除的疑似项

1. **`admin_access.go` 的三个 `GATEWAY_ADMIN_*` 常量「只在测试里被读」** — 不成立。`loadAdminAccessPolicy`（88-101）在每次请求里读这三个变量（带 atomic 缓存），`AdminAccessGuard` 在 `internal/api/router.go:92` 全局注册（`r.Use`，位于静态资源挂载之前），`internal/api/admin_guard_wiring_test.go` 亦专门断言了注册顺序。它们是活的，问题在判定逻辑（见 A-01）而非接线。
2. **nginx `proxy_read_timeout 86400s` 与 `model.EdgeProxyTimeoutSeconds` 漂移** — 不成立，是刻意的一致：常量 `86400`（`internal/model/channel.go:173`）就是为此定义，`cmd/airoute/nginx_timeout_test.go` 对三份 nginx 与 Helm ingress 注解逐一断言相等（实测三份配置全部 86400）。
3. **401 不写入 `usage_logs`** — 刻意。`internal/middleware/auth.go:38-47` 明确解释：401 表示从未认证、无 model/channel、不计费，落库只会制造噪音并污染错误率。
4. **`GATEWAY_DRAIN_TIMEOUT_SEC` 默认值在 `cmd/airoute`（120s）与 `internal/api`（`defaultUpstreamDrainTimeout=2m`）各有一份** — 刻意，`server.go:422-425` 的注释说明这是为了让 shutdown 预算可从 drain 推导，两份值当前一致。
5. **`/metrics` 默认仅回环/私网** — 文档（`docs/CONFIGURATION.md:120-121`）与代码（`internal/api/metrics_access.go:19-50`）一致，`GATEWAY_METRICS_TOKEN`/`GATEWAY_METRICS_ALLOW_PUBLIC` 两个变量均已接线并有优先级注释。
6. **`model_validation` 默认 off 但出厂 YAML 带 3 条 strict 规则** — 刻意且文档一致（`configs/config.yaml:41-59` 说明「仅对个别配置了规则的模型开启」）；`config.ResolveValidationLevel` 经 `internal/validator/pipeline.go:16-38` 真实生效，`disallow_temperature` 也在 `validator/openai.go:112` 被读取。

## 覆盖面

- **审计基线**：commit `1520d99`（`git log --oneline -1` 复核；工作区另有并行产物 `.docx` 与 `docs/audit/high-concurrency-billing-2026-10-28.md`，未纳入）。
- **通读**：`cmd/airoute/`（main.go、server.go、timeout_chain.go、security_check.go、cli.go、cli_status.go 及 `nginx_timeout_test.go`/`startup_timeouts_test.go`/`deploy_readiness_test.go`）、`internal/config/`（config.go、config_policies.go、storage_config.go、validation_rules.go）、`internal/middleware/`（admin_access.go、auth.go、authz.go、body_limit.go、ratelimit.go、quota_tracker.go、trace.go、tpm_tracker.go 及其测试清单）、`internal/api/router.go` 全文 + `metrics_access.go` + `mcp_authz.go`、`internal/controlplane/admin_handler.go` 局部（public URL）、`account_handler.go` 局部（login 身份）、`admin_bootstrap.go` 引用处、`internal/buildinfo/`、`deploy/`（三份 nginx、kubernetes.yaml、prometheus/airoute-alerts.yml）、`helm/airoute/`（values/configmap/deployment/ingress）、`docker-compose.yml`、`docker-stack.yml`、`Dockerfile`、`Makefile`、`scripts/`（3 个脚本）、`configs/config.yaml`、`.env.example`、`docs/CONFIGURATION.md`、`web/web.go`（静态路由）。
- **核查方式**：全量枚举 `os.Getenv` 与常量式 env 读取（含 `envFlagEnabled`/`envPositiveInt`/`envRetentionDays`/`TrustedProxiesFromEnv`/`config_policies.go` getter 这类非字面量读取），逐个建立「清单→读取点→决策点」；对 `.env.example` 里每个变量统计非测试引用数；对 `deploy/prometheus/airoute-alerts.yml` 的 16 个指标名逐个回查导出点（全部命中）；对 deadcode 命中项做全仓库（含测试）引用核实。
- **未覆盖**：`internal/api` 的业务处理细节（计费/结算/协议转换，另有审计员）、`internal/storage`/`internal/billing` 的内部实现（仅核查其 env 读取与决策点）、前端 `web/src`、`internal/provider`/`internal/router` 的调度细节（仅核查超时相关常量与函数）、`tests/` 下的集成/E2E 用例；未运行测试（只读审计，避免与并行改动相互干扰）。
