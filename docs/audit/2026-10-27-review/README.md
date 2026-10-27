# 重建重启后全面审查（2026-10-27）

- **代码版本**：`64e749a` — *fix: resolve functional audit findings across auth billing and workflows*（上一轮 18 项修复的提交）
- **运行环境**：Docker Compose 集群（`airoute-lb` nginx → `airoute-1`/`airoute-2`，PostgreSQL 16、Redis 7、RustFS）
- **动作**：`make docker-rebuild`（滚动重建两副本，镜像版本 `1.1.0-dev+g64e749a`）→ 全面审查（7 个领域并行 + Lead 线上验证）
- **结论**：服务重建重启成功且在真实流量下正常；本轮共记录 **115 项发现（去重前）**：**P0 2 条（同一根因）**、**P1 27 条**、**P2 81 条**、**P3 5 条**。其中跨领域重复项在下方“优先级汇总”中合并。

## 1. 验证基线（全部通过）

| 检查 | 结果 |
|---|---|
| 镜像构建 + 滚动重启 | `gateway-2`、`gateway-1` 依次重建，两副本 `healthy`，版本 `1.1.0-dev+g64e749a` |
| 入口健康检查 | `GET http://127.0.0.1:8080/health` → 200，`version=1.1.0-dev+g64e749a` |
| 两副本直连健康 | `airoute-1` / `airoute-2` 直连 `/health` → 200 |
| 启动日志 | 两副本 `ERROR` 行数 = **0**；仅剩已知的弱凭证/未配价/裁决模式 WARN |
| 真实流量 | 重启后 `/v1/messages?beta=true`（流式 Anthropic 入口）持续 200，租户 `密钥-1` 正常计费 |
| Go 测试 | `make test`（`go test -race ./...`）→ **exit 0，全部通过** |
| Go 静态检查 | `go vet ./...` → 无输出（通过） |
| Web lint | `npm run lint` → 0 告警 |
| Web 测试 | `npm test` → **38 文件 / 507 项通过** |
| Web 构建 | `npm run build` → 成功 |
| 迁移 | `schema_migrations` 最新 = **26**；`usage_logs` 50,943 行，账本/结算表结构完整 |

线上关键回归（本轮实测，详见 [live-verification](live-verification-2026-10-27.md)）：

- 无 key/错误 key → 401；新建 key 1 秒内两副本同时生效，删除后两副本同时 401（热加载正常）。
- 分布式 RPM：`rpm=3` 时连续 6 次 `/v1/models` → `200 200 200 429 429 429`（跨副本滑动窗口正常）。
- Cookie 写操作 CSRF：无 `X-CSRF-Token` → 403；带 token → 通过（404 为预期业务响应）。
- 非流式计费：`prompt=31/completion=1` → `cost=0.000035`，`billing_settlements.cost_units=35000`，key 账本 `used_cost_units=35000`（同步结算命中）。
- 流式 SSE：`200`、`text/event-stream`、`[DONE]`，结算记录一致（35,000 units）。
- 客户端断连 + 上游排空：`499`、`completion_tokens=50`（排空真实消耗）、`cost=0.000239`、`usage_source=upstream`、已结算 239,000 units，`airoute_aborted_drained_requests_total=1`。
- 对账 worker（启动 3 分钟后）：`ledger_drift=2,647,492,000 units`（= 已知历史基线 ¥2.6475）、`wallet_drift=0`、`audit_lag=0`、重复 `billing_id=0`、`settlements_deferred_total=0`。

## 2. 分领域报告与计数

| 报告 | 领域 | 发现数 | P0 | P1 | P2 | P3 |
|---|---|---|---|---|---|---|
| [01-auth-security.md](01-auth-security.md) | 认证 / 会话 / OAuth / 管理权限 / MCP 安全 | 17 | 1 | 4 | 12 | 0 |
| [02-billing.md](02-billing.md) | 计费引擎 / 结算 / 钱包 / 对账 | 15 | 0 | 3 | 8 | 4 |
| [03-routing.md](03-routing.md) | 路由 / 渠道选择 / 协议适配 / 超时 | 17 | 0 | 4 | 13 | 0 |
| [04-api.md](04-api.md) | HTTP 处理器 / SSE / 入站协议 / 多模态 / MCP 工具 | 20 | 0 | 7 | 13 | 0 |
| [05-storage.md](05-storage.md) | 存储 / 迁移 / Redis / 遥测 / 配置 | 16 | 0 | 4 | 12 | 0 |
| [06-web.md](06-web.md) | React 管理台 | 12 | 0 | 1 | 10 | 1 |
| [07-deploy.md](07-deploy.md) | CLI / 启动 / 部署 / 文档 / CI | 18 | 1 | 4 | 13 | 0 |
| **合计** | | **115** | **2** | **27** | **81** | **5** |

> 说明：P0=2 是同一根因（默认管理员凭证 + 占位 HMAC 密钥）分别被 auth 与 deploy 两位审查者独立发现；另有若干 P1 跨报告重复（如 MCP 计费绕过、120s 排空上限、视频先收费、moderation 零计费），下方按根因合并。

## 3. P0（唯一根因）：默认管理员凭证 + 公开占位密钥，且发布模式门禁不可达

| 证据 | 位置 |
|---|---|
| 线上 `POST /api/v1/auth/login` 用 `admin/admin123` 返回 200，`is_default_password=true` | 实测（见 [live-verification](live-verification-2026-10-27.md)） |
| `GATEWAY_ADMIN_SECRET` 等于仓库公开占位值之一（32 位弱密钥），管理员 JWT 可被离线伪造 | `.env:22-23`；[01-auth-security.md](01-auth-security.md) A01；[07-deploy.md](07-deploy.md) D01 |
| 启动门禁 `isRelease` 仅由 `GIN_MODE=release` 或 `log_level=warn/error` 触发；Compose/Swarm 均未设置，`config.yaml` 为 `info` → 只告警不退出 | [`cmd/airoute/server.go:73`](../../../cmd/airoute/server.go#L73)、[`cmd/airoute/security_check.go:11`](../../../cmd/airoute/security_check.go#L11) |
| 附带放大风险：开放注册 + ¥5 试用额度 + 无邮箱验证（`ALLOW_REGISTRATION=true`, `INITIAL_USER_BALANCE=5.0`, `REQUIRE_EMAIL_VERIFICATION=false`） | `.env:28-30`；[01-auth-security.md](01-auth-security.md) A05；[07-deploy.md](07-deploy.md) D14 |

**影响**：可登录管理台 / 伪造管理员会话；结合开放注册，外部可批量注册并消耗真实上游额度。**这是本轮唯一 P0，建议立即处理。**

## 4. P1 汇总（按根因合并，含跨报告重复）

### 4.1 认证与授权
| # | 问题 | 位置 / 证据 |
|---|---|---|
| 1 | MCP `airoute_chat` 绕过钱包余额、key 预算、账号锁定、模型白名单、RPM/TPM、usage 估算与定价快照；实测受限 key 成功调用未授权模型并产生费用 | [01 A02](01-auth-security.md)、[04 A04](04-api.md)、[02 B3](02-billing.md)；Lead 实测（见 live-verification） |
| 2 | 登录暴力破解可绕过：`c.ClientIP()` 信任伪造 `X-Forwarded-For`，且无 `SetTrustedProxies`、计数器按进程（2 副本）→ 轮换 XFF 可持续尝试 | [01 A04](01-auth-security.md) |
| 3 | 开放注册 + ¥5 试用额度，无限流/验证；测试中 5 次请求铸造 ¥25 与 5 个可用 key | [01 A05](01-auth-security.md)、[07 D14](07-deploy.md)；线上 `.env` 配置 |
| 4 | 启动时从 DB 热加载失败后数据面退化为“匿名放行”（fail-open） | [01 A03](01-auth-security.md) |

### 4.2 资金正确性
| # | 问题 | 方向 | 位置 |
|---|---|---|---|
| 5 | 一条外币定价快照记录毒化整批（≤100 条）：整批不落审计、不收费，PEL 永久重试且无死信 | 漏收 | [02 B1](02-billing.md) |
| 6 | 无 `billing_id` 的旧格式记录每次投递重新生成随机幂等键 → 重投递重复扣款（实测余额 10→8→6） | 多收 | [02 B2](02-billing.md) |
| 7 | MCP 计费丢失定价快照 → CNY 校验被绕过，遗留外币价按面值扣人民币（≈7×） | 多收 | [02 B3](02-billing.md)（与 #1 同根） |
| 8 | 视频生成在“变换/持久化任务”之前按 200 计费；失败返回 502，客户已扣费且无任务 ID | 多收 | [04 A02](04-api.md)、[02 B11](02-billing.md) |
| 9 | 流式请求的 prompt 回退估算只统计文本，漏算 tools/图像（实测 1 vs 2525 tokens） | 漏收 | [04 A03](04-api.md) |
| 10 | `/v1/moderations` 成功调用零审计零计费；上游 500 被伪装成 501“未配置” | 漏收/误导 | [04 A05](04-api.md)、[02 B4](02-billing.md) |
| 11 | 上游错误原文（可能含 provider key）写入 `usage_logs.error_message`，未脱敏 | 泄露 | [04 A06](04-api.md) |

### 4.3 超时与转发
| # | 问题 | 位置 |
|---|---|---|
| 12 | 共享 Transport `ResponseHeaderTimeout=60s` 使所有非流式渠道超时（API 允许到 86340s）在 60s 被截断，并计为渠道故障 | [03 R-01](03-routing.md) |
| 13 | 排空上下文把 **每个** chat/responses/messages 请求限制在绝对 120s，渠道超时（线上 300s）不可达、长流在第 120s 被切断 | [03 R-02](03-routing.md)、[04 A19](04-api.md) |
| 14 | 多模态路径把上游 429 计为渠道故障（3 次即熔断），与聊天路径“429 不算故障”的契约矛盾 | [03 R-03](03-routing.md) |
| 15 | `anthropic_messages` 能力被当作“上游使用 Anthropic 协议”：OpenAI/Gemini 渠道只要勾选该协议，就会收到发往 `<base>/v1/messages` 的 Anthropic JSON | [03 R-04](03-routing.md) |
| 16 | 多模态派发失败一律 502；客户端断连被记为上游故障且无排空 | [04 A01](04-api.md) |
| 17 | `/mcp/sse` 与 `/api/v1/admin/system/logs/stream` 未清除 `WriteTimeout`，控制面长连接在 600s（Compose）/180s（Helm）被切断 | [07 D05](07-deploy.md) |

### 4.4 存储 / 遥测 / 部署
| # | 问题 | 位置 |
|---|---|---|
| 18 | 环形缓冲区广播与退订竞态：`Subscribers` 取消关闭 channel 后，`Append` 仍向其发送 → `panic: send on closed channel`（管理台日志 SSE 断开 + 任意并发日志可触发） | [05 ST-01](05-storage.md) |
| 19 | `normalizeModelPrice` 合成 `00:00-08:30 @0.5` slot，覆盖运维配置的单窗口折扣（23:00 全价、03:00 0.5 而非 0.25，双向错计费） | [05 ST-02](05-storage.md) |
| 20 | trace 脱敏正则遇 `Bearer <token>` 的空格即停，凭证可落入 `request_traces`（保留 72h） | [05 ST-03](05-storage.md) |
| 21 | Redis URL 解析失败时把含密码的完整 URL 写日志（libpq keyword DSN 同样泄漏） | [05 ST-04](05-storage.md)、[05 ST-05](05-storage.md) |
| 22 | 边缘超时不变量只在 4 个部署目标中的 1 个被测试守护：Swarm nginx 600s、Helm ingress 300s、旧 nginx.conf 300s vs 86400s 预算 | [07 D02](07-deploy.md) |
| 23 | 随仓库发布的 `deploy/kubernetes.yaml` 自带 `GIN_MODE=release` 且未给 `GATEWAY_ADMIN_PASSWORD` → 启动即 `os.Exit(1)` CrashLoop | [07 D03](07-deploy.md) |
| 24 | 滚动发布只重建 gateway，不重建/重载 LB（线上 LB 仍 `nofile=1024`，创建于 2026-10-08；nginx 不重载会持有旧网关 IP） | [07 D04](07-deploy.md)；Lead 实测 |

### 4.5 前端
| # | 问题 | 位置 |
|---|---|---|
| 25 | AiCopilot 丢弃所有“消息不含 `stream error`”的流式业务错误（配额/鉴权/429/5xx）→ 空回答或伪造本地兜底文本，且 reader 不释放 | [06 W02](06-web.md) |

## 5. Lead 线上补充发现（仅本轮实测可见）

| ID | 严重度 | 问题 |
|---|---|---|
| L1 | P2 | `normalizeUsageSource` 把 `gateway_auth` 归一为 `upstream`：实测 401 鉴权失败审计行 `usage_source=upstream`；这类零成本行仍被分配 `billing_id`，且因 `cost_units=0 AND total_tokens=0` 永远不生成结算，形成只增不减的“无结算审计行”（当前 433 条，其中 429 条为历史零成本错误行）。 |
| L2 | P2 | 依赖容器配置漂移：`airoute-postgres`/`airoute-redis` 创建于 10-08/10-09，未随 compose 更新重建 —— 线上 `max_connections=100`（compose 声明 200）、PostgreSQL/Redis 均无 `nofile=65536`（PG soft 仍 1024），`docker-rebuild.sh` 只覆盖网关与 LB 问题同源。 |
| L3 | P2 | `/metrics` 未鉴权（`GATEWAY_METRICS_TOKEN` 未设置）→ 外部可读全部内部计数器；与开放注册/默认管理凭证叠加时进一步降低攻击门槛。 |

## 6. 交叉主题

1. **“单旋钮”只完成了一半**：渠道超时统一了配置，但 Transport 60s、排空 120s、边界代理（Swarm 600s/Helm 300s）、`WriteTimeout` 600s 仍在不同层截断；R-01/R-02/D02/D05/A19 是同一类问题的五个面。
2. **MCP 是鉴权/计费的第二套旁路**：`airoute_chat` 只校验 key.Status，绕过中间件里的余额/预算/锁定/白名单/RPM/TPM/估算；MCP 还缺定价快照，会话 `sessionId` 可被当作 bearer 凭据且未绑定 key。
3. **计费旁路仍存在于非聊天入口**：moderation、MCP、视频、多模态断连、旧 `/v1/completions`、Anthropic 适配流；聊天主路径已较完善，但“统一管道”尚未真正统一。
4. **降级路径的“可观测性谎言”**：`/health` 恒 200（DB/Redis 故障仍健康）、advisory 熔断不记录任何状态（控制台永远健康）、控制台缓存省钱用硬编码单价、Dashboard 为无样本渠道伪造 160ms/100% SLA。
5. **部署产物与代码脱节**：发布门禁不可达、Helm/Swarm/nginx 超时与不变量不一致、k8s manifest 开箱 CrashLoop、环境变量漂移（77 vs 39）、CI 不校验部署产物。
6. **前端仍保留凭证与状态残留**：登录/注册仍写 `localStorage.airoute_token`，auth 页面登录跳过租户清理，渠道上游 key 明文进 DOM。

## 7. 上一轮修复复核

- **确认已修**：F01（OAuth 稳定 `(provider,subject)` 身份）、F02（改密/重置撤销旧会话、角色/锁定复检）、F18（跨副本 OAuth state + 原子共享密钥）、F1 幂等键（线上重复 `billing_id=0`）、F3 499 断连计费（带 token 且 cost=0 的 499=0）、F4 透支钳制、F5 默认价不再自动打折、F6 非流式 usage 估算、F10 按实际服务模型计费、F15 取整口径、F05/F06/F09/F16（多模态 base path、视频任务钉选/模型校验、体检协议）。
- **部分修/未修**：F7（moderation 零计费未修）、F8（缺 unattributed 指标）、F9（硬删流水 + 历史 legacy 键不可 join）、F11（Gemini 内联错误仍被吞；AiCopilot 未修）、F12（Anthropic 断连仍记 200/少收）、F13/F14 相关细节见 02/04 报告。

## 8. 建议修复顺序

1. **立即（P0）**：轮换 `GATEWAY_ADMIN_PASSWORD` 与 `GATEWAY_ADMIN_SECRET`；在 Compose/Swarm/Helm 设置发布模式（或把弱凭证检查改为始终拒绝）；关闭或严格限流注册与试用额度；限制 `/metrics` 与 `/api/v1/admin/*` 的对外暴露。
2. **P1 资金/安全**：MCP 鉴权计费统一（#1/#5/#6/#7/#8/#9/#10）、错误脱敏（#11）、trace/Redis 日志脱敏（#20/#21）、开放注册加固（#3）、登录限速与可信代理（#2）、数据面 fail-open（#4）。
3. **P1 超时/协议**：Transport 60s 改为按请求/渠道、排空上限与渠道超时解耦（#12/#13）、多模态 429 与断连语义（#14/#16）、`anthropic_messages` 语义拆分（#15）、控制面 SSE 清 deadline（#17）。
4. **P1 稳定性**：环形缓冲区竞态（#18）、off-peak 配置（#19）、部署目标不变量/清单/LB 重建（#22/#23/#24）、AiCopilot 错误呈现（#25）。
5. **P2/P3**：按各报告表格逐项处理（81 项 P2、5 项 P3）。

## 9. 方法与边界

- 7 个领域并行审查，均使用 HEAD 的私有 git worktree 运行定向探针（`*-race` 用例 + 定向测试），主仓仅新增本目录；未修改任何源码/`.env`/服务，未重启服务。
- 线上验证由 Lead 完成后执行：真实 HTTP/SQL 只读检查、临时 API key（审计结束后软删除）、1～2 次真实推理调用（¥0.0002 级）、一次断连排空测试。
- 未覆盖：真实 PostgreSQL/Redis 集成测试（需独立实例）、真实 OAuth 三方往返、付费多模态上游、浏览器视觉走查、K8s/Swarm 实机部署；这些在报告中标注为 STATIC 或未验证。
- 残留测试痕迹：数据库新增 4 条 `status=deleted` 审计用 key（id 19-22），无用户余额异常；临时 `/tmp` 探针与 worktree 已清理。

## 10. 证据

- [live-verification-2026-10-27.md](live-verification-2026-10-27.md)：重建/重启、健康、基线测试与线上回归的完整命令与脱敏输出。
- 各领域报告内附探针命令、原始输出（脱敏）与 `file:line`。
