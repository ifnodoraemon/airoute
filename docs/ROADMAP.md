# Airoute 下一步功能规划（2026 Q4 – 2027 H1）

> **版本基线**：`1.1.0-dev` / `a6ef42a` · **规划日期**：2026-10-27 · **最近更新**：2026-10-28
> **范围**：新功能与产品能力。本轮 115 项审计发现的 P0/P1/P2/P3 修复已完成，不在本文重复。
> **团队假设**：1–2 名资深全栈；「单二进制、零外部中间件」的工程约束不变。
> **证据口径**：所有现状判断与缺口均以仓库代码/文档为准，链接到行号；6 个领域的并行盘点明细见文末附录。

---

## 0. 一页结论

1. **我们已经把「能不能用」做完了**：数据面（协议转译、容灾、多模态、MCP）在同类自研网关里属于第一梯队；刚完成的审计把正确性与安全性也补齐了。
2. **下一步的主要矛盾是「透明替代直连」与「成本可解释」**：协议字段与 Responses 工具链决定业务方是否相信网关与直连一致；供应商比价决定成本能否被解释与议价。合规可证与可运维是随后的两条支撑线。
3. **v1.1 交付「供应商单价」**：供应商模型单价维护（定价页与我们的售价并排、二者分开维护、不计毛利）；随后进入 v1.2 账单与合规。
4. **开新功能之前先花 2 周做「补口」**：协议字段无损透传、Responses 工具链、请求级总预算、审计残留与部署清单小缺陷。这些是「透明替代直连」的信任底线，成本低、收益高。
5. **三条主线 + 一条并行线**：B 账单与合规（v1.2）→ C 企业集成与运维（v1.3）→ D 智能降本与协议前沿（v1.4）；E 信创交付与 B/C 并行，作为投标与验收材料线。

**如果只能先做三件事**：

| 顺序 | 事项 | 理由 |
|---|---|---|
| 1 | 协议字段透传 + Responses 工具链 | 现在 `response_format`、`seed`、`logprobs` 等字段在绑定阶段被静默丢弃，结构化输出与 agent 流量经网关会「无声降级」 |
| 2 | 供应商模型单价与售价并排 | 不引入数据面依赖与账本风险，直接支撑比价、议价与报价复核 |
| 3 | 部署清单与可观测补口（P0-05、P0-06 发版） | 客户 `kubectl apply`、镜像发版与内网可观测是交付硬阻塞，成本低、收益直接 |

---

## 1. 现状基线（我们站在哪里）

| 层面 | 已具备 | 证据 |
|---|---|---|
| **数据面** | OpenAI / Anthropic / Gemini 双向转译；chat / completions / responses / messages / models / embeddings / rerank / moderations / 图像 / 语音 / 视频；级联命名与通配符；下游自动探测；优先级 + 平滑加权轮询；三态熔断；首字前容灾；会话亲和 | [router.go](../internal/api/router.go#L266)、[dispatcher.go](../internal/router/dispatcher.go#L110)、[load_balancer.go](../internal/router/load_balancer.go#L19) |
| **控制面** | 渠道、模型路由、虚拟密钥、用户、分组倍率、定价、日志、请求追踪、供应商对账、MCP 广场、Skills Hub、公告、Playground | [controlplane/](../internal/controlplane) |
| **计费** | 四级价目解析（分组 → default → 通配符 → `*`）、缓存/分时/分组计价、钱包 + Stripe + 兑换码、幂等结算账本、漂移对账 worker | [engine.go](../internal/billing/engine.go#L265)、[settlement.go](../internal/storage/settlement.go#L68) |
| **安全** | API key SHA-256 摘要落库、HMAC 会话、CSRF、登录锁定与抗枚举、可信代理、控制面私网隔离（D15） | [auth.go](../internal/middleware/auth.go#L118)、[admin_access.go](../internal/middleware/admin_access.go#L55) |
| **存储 / HA** | SQLite（纯 Go/WAL）或 PostgreSQL；Redis 跨副本限流、日志流、配置版本同步、实时快照；异步审计 + 本地 spill 重放 | [storage/](../internal/storage)、[distributed/](../internal/distributed) |
| **部署** | 单二进制嵌入前端；amd64/arm64；Compose 双副本 + nginx、Swarm、Helm/K8s；`/health` 与 `/health/ready` 分离 | [Dockerfile](../Dockerfile)、[deploy/](../deploy)、[helm/](../helm) |
| **质量** | 729 个 Go 测试函数 / 222 个测试文件；597 个前端用例；CI 跑 `-race`、lint、构建 | [ci.yml](../.github/workflows/ci.yml) |

**一句话判断**：**数据面过剩，治理面不足。** 当前形态非常适合「一个团队、一个管理员、一套钱包」的试点；控制面缺的是合规可证（审计、密钥、SSO）与可运维能力，而不是更多的接入协议或组织层级。

---

## 2. 判断：下一步做什么

### 2.1 阻碍下一步推广的三个硬问题

| # | 问题 | 现状证据 | 影响 |
|---|---|---|---|
| **C** | **合规**：无管理员操作审计、仅 admin/user 两角色、无 MFA/会话撤销、无企业 SSO、上游渠道 key 明文落库并明文出参 | [repo_users.go](../internal/storage/repo_users.go#L21)、[oauth_handler.go](../internal/controlplane/oauth_handler.go#L25)、[repo_channels.go](../internal/storage/repo_channels.go#L21)、[admin_channels.go](../internal/controlplane/admin_channels.go#L43) | 等保「审计、三权分立、双因素、密钥保护」多项不达标；内网无法接统一身份 |
| **D** | **可运维**：15 条告警规则只是 YAML，进程内无评估与通知；无备份/恢复入口；Helm 固定 `Recreate`、无 PDB/反亲和/preStop | [airoute-alerts.yml](../deploy/prometheus/airoute-alerts.yml)、[migrations_runner.go](../internal/storage/migrations_runner.go#L55)、[deployment.yaml](../helm/airoute/templates/deployment.yaml#L11) | 私有化内网不会为单网关再搭 Prometheus+Alertmanager → 静默故障无人接收；升级即停机 |
| **E** | **透明替代**：chat 入站用固定结构体解码，未声明字段静默丢弃；Responses 的 tools 解析后被丢弃；无模型能力元数据 | [chat.go](../internal/model/chat.go#L55)、[handler.go](../internal/api/handler.go#L55)、[response.go](../internal/model/response.go#L60)、[models_handler.go](../internal/api/models_handler.go#L17) | `response_format`（JSON Schema）静默失效、agent 工具调用不可用——业务方会认为「网关和直连不一样」 |

### 2.2 结论

- **协议广度已经不是瓶颈**：10 类 `ProviderType`、5 类入站协议、多模态与 MCP 都已覆盖。
- **真正的瓶颈是「合规可证」与「透明替代」**：前者决定能否通过等保与内网准入，后者决定业务方是否相信网关与直连一致。
- **成本侧只需要解释力**：供应商单价与售价并排即可支撑比价与议价，不需要成本核算账本与报表体系。

---

## 3. 规划原则

1. **复用现有账本、快照与审计管道**，不另起一套计费/日志/告警体系。
2. **单二进制优先**：Prometheus / OTel / SIEM 作为「可选出口」，不做内网强制依赖。
3. **默认安全、可显式降级**：新增开关必须有安全默认，危险能力必须二次确认并留痕。
4. **每个版本必须可演示、可验收**：有页面、有指标、有回归测试；不做「只有 API 没有界面」的半成品。
5. **一次只推进一条主线**：跨主线共享的薄底座（审计、加密、密钥管理）先做，避免多条线同时改账本。

---

## 4. 路线图

> 工作量：**S** < 1 周，**M** 2–4 周，**L** > 1 个月（按 1–2 人）。每个版本按「1 个 L + 2 个 M + 若干 S」控制范围。

### 4.0 P0 补口（2 周，先于新功能）

> **执行进展（2026-10-27）**：P0-01、P0-02、P0-03、P0-04、P0-05、P0-07 均已完成并通过测试，变更与验证证据见 [P0 交付记录](audit/2026-10-27-p0/README.md)。P0-06（1.1.0 正式发版）不在本目标范围，留待代码评审后执行。

| ID | 事项 | 为什么现在做 | 工作量 | 验收口径 |
|---|---|---|---|---|
| **P0-01** | **协议字段无损透传**：入站保留原始 JSON，同协议转发只改写 `model`/`messages`/`stream`；未识别字段（`response_format`、`seed`、`logprobs`、`parallel_tool_calls`、`reasoning_effort`、`service_tier` 等）原样透传；跨协议用显式剥离表；流式 chunk 补透传字段 | 固定结构体解码已造成静默丢字段，结构化输出/可复现采样/评测在网关侧不可用 | M | JSON Schema 输出、seed、logprobs 经网关与直连行为一致；新增契约测试锁定字段清单 |
| **P0-02** | **Responses 工具链闭环（已实现）**：扁平 Responses function tools 转 chat tools、`tool_choice` 三种字符串与 function 对象、`parallel_tool_calls`；流式补 `function_call` 输出项、`function_call_arguments.delta/done` 与 `output_item.done`；非流式把 `tool_calls` 映射为 `function_call` 输出项；纯工具轮次不再补空 message 项。内置工具（web_search 等）明确跳过并注释 | `ResponseRequest.Tools` 解析后未进入下游请求，任何 agent/工具调用都会静默降级为纯文本 | M | 单测覆盖 tool/tool_choice 映射与 function_call 输出；流式单测断言 item/output_index/arguments 与 `response.completed` 一致性 |
| **P0-03** | **请求级总预算（非流式）**：`GATEWAY_REQUEST_BUDGET_SEC`（默认 300s，0 关闭）由渠道尝试与跨模型兜底共享；耗尽立即停止换渠道并返回 504 `request_budget_exceeded`；调用方自带更短 deadline 时优先，且不会被误报为预算耗尽；流式仍由渠道超时 + 首字超时约束 | 当前最坏等待 = 候选渠道数 × 渠道超时 × 兜底深度，客户端早已断开而网关仍在换渠道 | S | 客户端在预算内收到确定失败；预算耗尽不污染熔断；`request_budget_exceeded` 有独立错误码 |
| **P0-04** | **审计残留收口（已实现）**：`off_peak_discount<=0` 现在表示「无折扣」（存储与引擎双层，不再静默 50%）；外币被拒请求在写响应头前归零，与审计口径一致；迁移 v27 改为「延迟迁移」——失败不记版本、不阻断启动、下次启动自动重试；`-count=3` 的 Prometheus 计数串扰改为基线差值断言；MCP SSE 跨副本边界已写入 HA 文档。`prompt=10` 硬编码经核实已在审计修复中消失 | 已知残留会持续制造「小概率错账/误导」与排障噪音 | S | 残留清单逐项关闭；单测覆盖 0 折扣、外币归零、延迟迁移重试；`go test -count=3` 稳定 |
| **P0-05** | **部署与可观测小缺陷（已实现）**：修复 `deploy/kubernetes.yaml` 重复的 `admin-password` 键；`/metrics` 默认仅 loopback/私网，`GATEWAY_METRICS_ALLOW_PUBLIC=true` 可放开；新增只读 `make check-deploy-drift`（PG `max_connections`/nofile 漂移检测）；控制台「开发文档」改为真正的 `docs` 标签页。注：`X-Cache-Status` 由 nginx 为 Skills 边缘缓存设置，**保留** | 清单级缺陷会直接卡住客户 `kubectl apply`；公网可读的 `/metrics` 暴露内部计数器与故障拓扑；旧依赖容器与 compose 声明不一致 | S | k8s 清单解析通过；公网抓取 `/metrics` 返回 403、私网正常；漂移脚本在真实运行环境返回无漂移 |
| **P0-06** | **1.1.0 正式发版**：CHANGELOG、镜像 digest、发布说明、滚动验证记录 | 当前是 `1.1.0-dev`；审计成果需要一个可引用的正式版本 | S | tag + release notes + 双副本滚动验证 |
| **P0-07** | **国产厂商探测识别**：prober 识别 `bigmodel.cn`、`volces.com`、`moonshot.cn`、`dashscope`、`baidubce.com`、`tencentcloudapi.com` 并生成可读渠道名（已修）；百炼/千帆/混元前端预设按需补 | 这些厂商走 OpenAI 兼容协议，但探测回落 `openai` 后渠道名不可读，演示时难以确认接的是哪一家 | S | 探测结果名称正确；`inferProviderType` 仍返回 `openai`（不误选适配器）；单测覆盖 |

### 4.1 v1.1 供应商单价（已完成）

> **主题**：让成本「可解释」——把供应商模型单价与我们的售价放在一起，直接支撑比价、议价与报价复核。**演示目标**：定价页同一模型的两套价目并排可见，供应商价目可 CSV 批量维护。
>
> 供应商单价与我们的售价是两套独立数据：**分开维护、互不推导**（我们的售价 ≠ 供应商价）；未配置显示“未配置”而非 0。供应商价只用于比价，不参与扣款、不写审计成本字段、不计算毛利。

| ID | 事项 | 范围 | 工作量 | 状态 |
|---|---|---|---|---|
| **A-04** | **供应商单价维护** | 按「渠道名 + 模型」维护供应商单价（输入/输出每百万 token 价格 + 启用状态）；定价页「供应商单价」子视图按模型与我们的售价**并排展示**并标出价差（我们的售价 − 供应商价）；两套价目分开维护 | S–M | ✅ 已完成 |
| **A-05** | **供应商价目 CSV 导入导出** | 供应商单价 CSV（`channel,model,input_cost,output_cost,status`）导出；导入支持表头识别、5000 行上限、逐行错误、`dry_run=1` 预览、全量校验通过后写入 | S | ✅ 已完成 |

**验收**：

- 定价页「供应商单价」子视图按模型把我们的售价与供应商单价并排可见、可分别维护，并显示价差；供应商价未配置时明确显示“未配置”。
- 供应商价目可 CSV 导出、编辑后导入；导入有 `dry_run=1` 预览与逐行错误，校验失败整批不写入。
- `go test -race ./...`、web lint/test/build 全绿。

### 4.2 v1.2 账单与合规（8 周）

> **主题**：把「治理数据」变成「财务与审计可用的证据」。**演示目标**：一键生成上月账单（含明细与差异校验），并展示管理员操作审计链。

| ID | 事项 | 范围 | 工作量 |
|---|---|---|---|
| **B-01** | **月结账单与成本分摊** | 日/月聚合表（key / model / channel / 日）；月结快照冻结金额与口径（费率版本、折扣、币种）；账单 API + 控制台「账单」页 + CSV/JSON 导出；与 `billing_settlements` 账本做差异校验 | M |
| **B-02** | **控制面审计中心** | `audit_events`（操作者、动作、资源、before/after 摘要、结果、源 IP、UA、trace_id）；覆盖登录/改密/角色/密钥/渠道/定价/余额/安全策略；链式哈希防篡改；查询/导出；Syslog 与 HTTP Webhook 外送 | M |
| **B-03** | **API key 生命周期** | `expires_at` + 到期提醒/自动停用；双 key 平滑轮换（旧 key grace 期）；per-key IP/CIDR 白名单；协议 scope（chat/embeddings/rerank/images/audio/video/gemini）；自助申请 → 管理员审批 | M |
| **B-04** | **敏感配置信封加密** | `GATEWAY_MASTER_KEY` 或外部 KMS 抽象；信封加密（主密钥加密 DEK，DEK 加密字段）；首批覆盖渠道 `api_key`、SMTP 口令、Stripe/OIDC secret；渠道列表服务端掩码 + 轮换/重加密命令；密码套件可选 AES-256-GCM / SM4-GCM + SM3 | M |
| **B-05** | **备份恢复与迁移防降级** | `airoute db backup/restore/verify`（PG `pg_dump`、SQLite `VACUUM INTO`，带 sha256 清单与 dry-run）；启动比对 `maxSchemaVersion`，旧二进制打开新 schema 直接拒绝启动 | M |
| **B-06** | **计费长期归档 + 日终摘要** | 归档表保留 model/channel 维度；保留期独立可配；每日生成笔数/金额/首尾 `billing_id` 摘要并以哈希链串接；`/admin/billing/integrity` 校验接口 | M |
| **B-07** | **接入验收工具链** | OpenAPI 3.1 规范（路由 + model 结构）、生成的 SDK 冒烟脚本、控制台「自检报告」页（复用 `/admin/system/middlewares` 与 `/health/ready`）、一键 e2e 验收；交付即带验收证据。**状态（2026-10-28）**：`GET /admin/system/middlewares` 与 `/health/ready` 已实现且目前无其它消费者，控制台自检页与 OpenAPI/SDK 尚未开始；该端点在页面落地前不得删除 | M |

### 4.3 v1.3 企业集成与运维（8–10 周）

> **主题**：接上单位统一身份，让控制台可以安全地下放给二级单位。**演示目标**：LDAP/OIDC 登录后自动映射角色与计费分组。

| ID | 事项 | 范围 | 工作量 |
|---|---|---|---|
| **C-01** | **企业 SSO** | 可配置 OIDC（issuer/discovery、client_id/secret、scopes、claim 映射）+ LDAP/AD 绑定（ldaps/StartTLS）；JIT 建号；claims → role 与计费 group；控制台配置页与连通性测试；保留本地 root 兜底 | M |
| **C-02** | **RBAC 四角色** | 平台管理员 / 安全审计员 / 租户管理员 / 只读运营；`resource:action` 权限点；统一鉴权中间件替换散落的 `claims.Role` 判断；前端按权限隐藏菜单；每个路由补允许/拒绝契约测试 | M |
| **C-03** | **MFA 与会话撤销** | TOTP（admin 强制、用户可选）+ 恢复码；会话表（jti、IP/UA、最后活跃）+ 强制下线；统一 `GetTokenExpiryHours`，修掉注册/OAuth 硬编码 7 天；口令复杂度与历史策略 | S |
| **C-04** | **告警评估引擎 + 指标维度** | 进程内规则评估（`for` 持续期、去重、静默、恢复通知），等价迁移现有 15 条规则；指标增加 channel/provider/breaker 维度并设基数上限；Helm 可选 ServiceMonitor/PrometheusRule；2 个 Grafana 面板 | M |
| **C-05** | **K8s 多副本 HA 与无损发布** | 多副本切 `RollingUpdate`（`maxUnavailable=0`）+ `minReadySeconds` + `preStop`；补 PDB、拓扑打散；HPA 接 `active_connections`；修复清单重复键 | M |

### 4.4 v1.4 智能降本与协议前沿（10–12 周）

> **主题**：在稳定交付的基础上，把成本与延迟真正降下来。**演示目标**：同一模型接两家供应商，按「成本优先 + 延迟降权」自动选路，灰度 10% 切新渠道并可一键回滚。

| ID | 事项 | 范围 | 工作量 |
|---|---|---|---|
| **D-01** | **策略化路由** | priority 层内可选选择器：加权 / 延迟感知（滚动 TTFT+TBT 分位）/ 最少在途 / 成本优先 / 混合打分（带滞回与最小样本量）；按 key/计费分组/百分比灰度与金丝雀；决策原因写入 trace | L |
| **D-02** | **精确响应缓存** | key = 租户 + key + 模型 + 规范化请求体哈希 + 影响输出参数；命中非流式直返、流式重放 SSE；TTL/条目/按模型开关；命中写 `X-Cache-Status` 与「零上游成本」审计行；命中率与节省金额进成本报表 | M |
| **D-03** | **模型能力元数据 + 能力感知路由** | 模型级 `context_window`、输入/输出模态、tools/json_schema 支持；候选过滤（图像→视觉渠道、tools→工具渠道、超长→过滤/拒绝）；`/v1/models` 可选扩展字段；控制台能力矩阵 | M |
| **D-04** | **入站内容安全护栏** | 可插拔过滤器链（请求 pre / 响应与首个内容块 post）；内置关键词/正则词表（可热更新）；动作三档：拦截 / 脱敏继续 / 仅告警；可选外接审核服务；命中写审计 | M |
| **D-05** | **上游鉴权插件 AuthPlugin** | 鉴权从 `Authorization: Bearer` 抽成插件：bearer / api-key+api-version / SigV4 / GCP OAuth2 / TC3 / token exchange；渠道配置增 `auth` 段；新增厂商从「改代码发版」降为「填配置」 | L |
| **D-06** | **Playground 与真实流量闭环** | 支持 `tools`/`tool_calls` 可视化与参数预设；从日志/追踪快照一键带 payload 到 Playground 复现（当前只切页不带数据）；用于排障与演示 | M |

### 4.5 v2.0 信创交付线（与 B/C 并行，按投标节奏推进）

> **主题**：把「可交付性与合规可证性」补成投标材料。**演示目标**：离线内网一次性安装；国产数据库可跑；国密边界有明确落点。

| ID | 事项 | 范围 | 工作量 |
|---|---|---|---|
| **E-01** | **信创离线交付包** | 离线镜像包（digest 固定）、CycloneDX SBOM、第三方许可证清单、校验脚本；CI 增 `govulncheck`；Compose/Helm 支持注入内网 registry | S |
| **E-02** | **国产数据库兼容** | 优先金仓 / openGauss（复用现有 Rebind 方言抽象），达梦列第二阶段；抽象 upsert/自增/时间类型/大小写匹配；迁移脚本双写校验 | M / L |
| **E-03** | **国密边界** | 入口由 nginx GM 或国密网关终结 TLS；出站支持渠道级 CA 与 mTLS 客户端证书；字段加密可选 SM4-GCM + SM3；文档化边界，不宣称「全链路纯 Go 国密」 | M |
| **E-04** | **国产 OS/CPU 验证矩阵** | 麒麟 / 统信 + 鲲鹏 / 飞腾 ARM64 实机验证；输出验证记录与已知限制 | M |

---

## 5. 优先级矩阵

| 事项 | 价值 | 工作量 | 关键依赖 | 建议版本 |
|---|---|---|---|---|
| 协议字段透传 / Responses 工具链 | 高（信任底线） | M | 无 | **P0** |
| 供应商单价维护 | 高（成本可解释） | S–M | 无 | **v1.1（已完成）** |
| 供应商价目 CSV 导入导出 | 中（可运营） | S | 供应商单价 | **v1.1（已完成）** |
| 月结账单 | 高 | M | 计费账本 | v1.2 |
| 控制面审计中心 | 高（等保） | M | 无 | v1.2 |
| Key 生命周期 | 中高 | M | 快照扩展 | v1.2 |
| 敏感配置加密 | 高（密评） | M | 主密钥管理 | v1.2 |
| 接入验收工具链 | 中高（交付） | M | 无 | v1.2 |
| SSO + RBAC | 高（推广） | M + M | 审计 | v1.3 |
| 告警引擎 + 指标维度 | 高（可运维） | M | 无 | v1.3 |
| 策略路由 + 响应缓存 | 中高（降本） | L + M | 供应商价目 + 能力元数据 | v1.4 |
| 上游鉴权插件 | 高（生态） | L | 渠道配置迁移 | v1.4 |
| Playground 闭环 | 中 | M | 协议透传 + 工具链 | v1.4 |
| 信创离线包 / 国产库 / 国密 | 高（投标） | S / M / M | 真机与客户选型 | v2.0 并行 |

---

## 6. 明确不做（本期）

- **不做组织/预算/钱包流水/成本毛利**：组织与成本中心、预算与告警、钱包资金流水、成本与毛利（含用量日汇总与成本分析）不在产品范围内；成本侧只保留供应商单价维护与售价并排。
- **不做通用七层网关 / Ingress 替代**：只服务 LLM 流量，不抢 Nginx/Kong 的职责。
- **不做模型推理引擎或训练平台**：网关只做接入、调度与治理。
- **不做完整可观测平台**：不内建 Loki/ES/APM，只做 OTel/Prometheus 出口与轻量告警。
- **不做多区域双活 / 跨机房强一致**：当前 HA 定位是单集群多副本。
- **不做复杂审批工作流引擎**：只做 key 申请与调账的轻量审批 + 审计。
- **不做语义缓存**：先做精确缓存；语义缓存需先解决误命中与计费口径，列入候选池。
- **不做 SAML / SCIM**：先 OIDC + LDAP/AD，覆盖内网绝大多数 IdP。
- **不自研审核模型**：护栏只做可插拔过滤器，模型审核接外部服务。
- **不做发票/税务**：只做账单、成本分摊与导出，财务系统对接交给客户侧。

---

## 7. 验收与度量

### 7.1 版本级验收（每个版本都必须有）

- `go vet ./...`、`go test -race ./...` 全绿；web lint/test/build 全绿。
- 新增数据库迁移在 SQLite 与 PostgreSQL 双方言下均通过，且有升级/回滚演练记录。
- 新增对外字段进入契约测试（请求体/响应体字段名两侧锁定）。
- 关键路径有 e2e：协议透传、工具调用、供应商价目导入导出、账单生成、审计查询。

### 7.2 运营指标（衡量推广是否真的发生）

| 维度 | 指标 |
|---|---|
| 推广 | 接入团队数、活跃密钥数、SSO 登录占比 |
| 成本 | 供应商价目覆盖率（已配置供应商价的「渠道 + 模型」占比）、计费对账差异率 |
| 运维 | 告警 MTTA/MTTR、备份恢复演练通过率、迁移失败回滚次数 |
| 质量 | 协议一致性用例通过率、`-race` 稳定率、线上 5xx 率与 P99 |

---

## 8. 风险与开放问题

### 8.1 主要风险

| 风险 | 缓解 |
|---|---|
| **SSO 把管理员锁在门外** | 保留本地 root 兜底并写入文档；SSO 配置变更二次确认 |
| **信创适配依赖真机与客户选型** | 先做「可验证清单 + 最小适配」，不承诺未验证的数据库/OS |
| **1–2 人产能撑不起路线图** | 严格按「1 个 L + 2 个 M」控制每版本；候选池中的事项默认不承诺 |
| **账单口径未定**（预付 vs 后付、目录价 vs 折扣） | v1.2 开工前明确账单单据口径（目录价 × 分组倍率）与是否需要「先用后付、按月结算」 |

### 8.2 需要拍板的三个问题

1. **下一版本主线**：B 账单与合规（推荐）/ C 企业集成（SSO+RBAC）/ E 信创交付——哪条是当前考核或投标的硬要求？
2. **账单口径**：内部账单按「目录价 × 分组倍率」结算，是否需要「先用后付、按月结算」？
3. **推广范围**：先在本部门/专业条线复制，还是准备跨单位投标？前者优先 B，后者需要 E 线提前。

---

## 9. 本周可开工（不依赖任何决策）

1. **P0-01 / P0-02**：协议字段透传 + Responses 工具链。纯代码改动，无迁移，验收明确。
2. **P0-05**：部署清单与缓存头清理。小改动，直接消除客户 `kubectl apply` 的阻塞项。
3. **v1.1 收尾**：供应商单价与售价并排视图与 CSV 导入导出的验收；ROADMAP/README/API 文档与代码保持一致。

---

## 附录 A：分析方法与来源

本规划由 6 个领域并行盘点后交叉去重：协议与路由内核、计费与成本治理、身份/租户与安全合规、可观测性与高可用运维、控制台与开发者体验、上游接入/生态与信创。每条现状与缺口均回读代码/配置核实，证据链接到具体文件（尽量带行号）。

各领域一句话结论：

| 领域 | 核心结论 |
|---|---|
| 协议与路由内核 | 协议覆盖广，但字段透传、Responses 工具链、模型能力元数据缺失，「透明替代直连」还不完整 |
| 计费与成本治理 | 计费算术扎实，供应商价目维护已补齐；仍缺周期账单（v1.2） |
| 身份 / 租户 / 安全 | 基础安全已加固，但缺 SSO、RBAC、管理员审计、MFA 与密钥加密（v1.2–v1.3） |
| 可观测性 / HA | 有指标与规则文件，但无告警执行、无 OTel、无备份恢复，K8s 发布非无缝 |
| 控制台与开发者体验 | 「看」的能力完整，「管」与「交付」不足：变更审计、验收工具（v1.2） |
| 上游 / 生态 / 信创 | 上游适配面广，但鉴权硬编码、凭证明文、信创零验证、离线包与 SBOM 缺失 |

主要输入文档：

- [本轮审计总结](../docs/audit/2026-10-27-review/README.md)
- [P2/P3 修复与已知残留](../docs/audit/2026-10-27-review/remediation-p2-p3-2026-10-27.md)
- [工程规范与架构约束](../docs/ENGINEERING_GUIDELINES.md)
- [API 参考](../docs/API_REFERENCE.md)、[配置手册](../docs/CONFIGURATION.md)、[HA 架构](../docs/HA_ARCHITECTURE.md)

## 附录 B：候选池（未排期，按需再评估）

- OpenAI Realtime / WebSocket / Batch API 接入
- 语义缓存（向量检索 + 误命中防护）
- 提示词模板管理与评测（Evals / A-B）
- 供应商结算单导入与账单级对账
- 授信额度与月结账期（后付模式）
- 渠道多 key 池与 key 级健康度
- 集群 SLO 视图与容量水位
- Skills / MCP 市场治理（签名、版本、权限）
- 文档站（docs 静态站 + 在控制台内可达的导航）
- 多区域部署与跨集群配置分发
