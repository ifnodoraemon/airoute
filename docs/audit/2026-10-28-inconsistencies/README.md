# 内部一致性审计汇总（2026-10-28）

> **审计口径**：用户原话——「本来计划起 xx 作用的，但是其他地方没用这种」。
> 即：**设计意图与实现/使用之间的断裂**，不查普通 bug、不查风格、不查性能。
>
> **基线**：commit `1520d99`（`fix(admin): name keys by public id in the call log and traces`）。
> 审计期间并行会话把 HEAD 推进到 `190b382`，并对 `settlement/quota` 有未提交改造；各分报告的行号均以 `1520d99` 为准。
>
> **方法**：5 个域并行审计（配置/路由/计费/API/存储）+ 全仓 `deadcode`（含/不含测试入口两轮）+ 逐条调用链核实 + Lead 交叉复核。
> **产出**：5 份分域报告（本目录），共 **72 条发现**；其中 **62 条高置信度**。Lead 独立复核了 14 条关键结论，修正 1 条（D-09）。

---

## 1. 一句话结论

代码的**主链路是好的**：扣费只有一个漏斗、裁剪（v1.1 四项能力）在代码/路由/前端/文档里都清得很干净、`gofmt` 干净、没有孤儿前端文件、没有整列只写不读的表。

问题集中在**「规则接了一条路，兄弟路径没接」**：同一条校验/预算/熔断/审计/采样/时间戳规则，往往只在 Chat 一元路径或某一个入口生效，其余入口静默绕过；以及**同一件事有两套实现**（掩码、trace_id、MCP 管理员判定、RPM 窗口、patch 语义、key 表示），未接线的那一套已经漂移。

最值得先修的是**控制面边界**这一组：D15 守卫在出厂 Compose 拓扑下因取 IP 方式退化而**恒放行**，nginx 侧 ACL 又只出现在 3 份配置的 1 份、且只保护默认 vhost——两道防线同时不成立。

---

## 2. 建议修复顺序

### P0：安全边界（口径与文档承诺不符，出厂即失效）

| 编号 | 问题 | 关键位置 |
|---|---|---|
| A-01 | D15 控制面守卫用 `ResolveClientIP`（丢掉 `Forwarded`），Compose 出厂 `GATEWAY_TRUSTED_PROXIES` 为空、网关只 `expose` 由 LB 转发 ⇒ peer 恒为 LB 私网 IP ⇒ **守卫从不拒绝**；同仓 `account_handler.go:55` 用的是正确写法 `ResolveClientIdentity`+`IPIsForwarded` | `internal/middleware/admin_access.go:158`、`authz.go:402,432`、`docker-compose.yml:33,138,228` |
| A-02 | 边缘 ACL 只在 `nginx-compose.conf` 默认 vhost；`console.*`/`admin.*` vhost 无 `allow/deny`（`Host: console.x` 即绕过），Swarm 与单机 nginx 全文 0 条 `allow` | `deploy/nginx-compose.conf:97,168`、`deploy/swarm/nginx-swarm.conf`、`deploy/nginx.conf` |
| D-02 | 守卫谓词只认 `/api/v1/admin` 与 `/app`；同一份 console shell 公开挂在 `/` 与 `/assets`；会话型 `/api/v1/user/*`、`/api/v1/auth/*` 不在谓词内 | `internal/middleware/admin_access.go:78`、`web/web.go:78,90`、`internal/api/router.go:145,168` |
| D-01 | 渠道列表把**上游供应商 api_key 明文**回传浏览器；前端只用来算 `Boolean(ch.api_key)`，而它假定的 `api_key_configured` 后端不存在。客户端密钥已是 digest+`ak_`，渠道密钥仍明文 | `internal/storage/repo_channels.go:49`、`controlplane/admin_channels.go:49`、`web/src/hooks/useChannelActions.js:273` |

### P1：金额/审计口径（会少收、会查不到）

| 编号 | 问题 | 关键位置 |
|---|---|---|
| C-02 | 缓存写入溢价 1.25× **只**接在 native Anthropic 流式；unary 与所有 canonical 路径因 `anthropicUsageToModel` 把 cache creation 并进 prompt → 按 1.0× 计费，缓存密集流量系统性少收 ~20% | `internal/billing/engine.go:387`、`api/inbound_anthropic_stream.go:461`、`provider/usage_mapping.go:14` |
| C-09 | 启动定价检查第二段被第一段短路：`if len(unpriced)==0 { return }` 在 `UnpricedServedModelsInUse` 之前 ⇒「别名有价、上游 served 模型没价」永不告警（正是按 served 模型计费想抓的失真） | `cmd/airoute/server.go:289`、`billing/startup_check.go:41` |
| A-10 | `auditAuthFailure` 只覆盖 key 状态/额度快路径；钱包 402、在途额度 402、限流 429 都不落审计，与注释「402/403/429 stay visible」矛盾 | `internal/middleware/auth.go:34,253,304,311`、`ratelimit.go:184` |
| C-07 | `ValidationError.Error()` 从未被调用：校验拒绝只渲染响应后 `return`，不写 `ContextKeyErrorMessage`、无审计行 ⇒ 服务端完全静默 | `internal/validator/errors.go:19`、`api/handler.go:96` |
| E-01 | 成功 trace 采样连判 **3 次**（`RecordSuccessTrace`→`RecordTrace`→`tc.Record` 各掷一次随机数）⇒ `sample=10` 实得 ~0.1%；且 all 模式只有 2 个 chat 入口归档成功请求 | `internal/api/trace_hooks.go:96,113`、`storage/trace_capture.go:118` |
| E-02 | `request_traces` 的 TTL 清理活在采集 worker 里，`GATEWAY_TRACE_CAPTURE=off` 时 worker 不启动 ⇒ 历史快照永不清理，TTL 失效 | `internal/storage/trace_capture.go:47,153` |

### P2：口径漂移与半接线（先确认真实需求再动）

其余 60 条见第 4 节索引。高性价比的一批：B-02（流式首 token 即记熔断成功，中途失败永不入账）、B-04/C-08（请求指标 router+api 双计，Prometheus ≈2×）、C-06（`disallow_temperature` 只有 OpenAI 校验器消费）、E-04/E-05/E-06（写入侧时间戳不分方言，PG 非 UTC 时区会偏/验证码立即过期）、D-03/D-04/E-08/E-09（`1520d99` 的 digest→`ak_` 改造只覆盖部分读路径）。

---

## 3. 按「断裂类型」归类（这是本次审计的主视角）

### 3.1 半接线：规则只接了一条路径

| 规则（设计意图） | 接通的路径 | 漏掉的路径 |
|---|---|---|
| 请求总预算 `GATEWAY_REQUEST_BUDGET_SEC` | Chat 一元（`dispatcher.go:217` 唯一调用） | Embedding / Rerank / 多模态（最坏 = 候选数 × 渠道超时）— B-01 |
| 渠道健康记账（熔断） | 一元每次失败 `RecordFailure`；流式首 token 前失败也记 | 流式**首 token 之后**的断流/error 帧只搬运不记账 ⇒ 坏渠道永远 CLOSED — B-02 |
| 404 处置（节点未托管该模型→换渠道） | Chat 明确继续下一渠道 | Embedding/Rerank 用 `IsClientError`（含 404）直接终止；多模态 4xx 原样透传 — B-03 |
| 跨模型兜底 `fallback_model` | Chat 一元/流式 | Embedding/Rerank/多模态不查兜底表，但拓扑页对任意模型都画兜底链 — B-11 |
| reasoning 标签剥离 | OpenAI chat 一元/流式、`openai_text` 一元 | `openai_text` **流式**裸文本直出 — B-12 |
| 缓存写入溢价 1.25× | native Anthropic 流式 | native unary、canonical 全路径 — C-02 |
| Token 校准 | chat 形状估算 | embedding/rerank 估算（裸 `estimateTokens`）— C-04 |
| `disallow_temperature` | `validator/openai.go:112` | `ValidateAnthropic`/`ValidateGemini` 的 `rule` 形参**函数体内零引用** — C-06 |
| 币种前置门 `validateModelCurrency` | 13 个入口 | moderation 缺门；且门校验**别名价**、扣费用 **served 模型价** — C-13/C-14 |
| 成功 trace 归档 | chat 单次/流式 | 其余协议不归档；采样连判 3 次 — E-01 |
| 审计 key 作用域（含历史密钥） | logs / stats 用 `ListHistoricalAPIKeysByUser` | traces 用 `ListAPIKeysByUser`（已撤销密钥的 trace 对本人不可见）— E-08 |
| digest→`ak_` 映射（`1520d99`） | 控制台 call log、traces | MCP `airoute_query_logs` 返回原始 digest；`/stats/*` 的 `key=` 参数（非管理员要 digest、管理员传 `ak_` 静默归零）— D-03/D-04/E-09 |
| MCP 工具发现 | handler + dispatch 都在 | `airoute_get_current_time`/`airoute_calc_eval` 不在 `tools/list` 的 24 项清单里（能调用、不能被发现）— D-06 |
| MCP 管理员判定 | infra 工具用 `mcpCallerIsAdmin`（有 args 回退） | SQL 反射工具用 `callerIsAdmin`（仅 context）⇒ stdio 管理员永远被拒 — D-05 |
| 渠道 patch（省略 vs 显式 0） | 密钥更新用指针语义 | 渠道更新用「零值即忽略」⇒ priority/weight 无法设为 0，前端还用 `\|\| 1` 掩盖 — D-14 |

### 3.2 写入不读 / 读取不写

- E-03：`billing_settlements.tenant_id`、`total_tokens` 三处写入、全仓无 SELECT，也无账本列表接口。
- E-15：`StatsOverview.PeakRPM/PeakTPM` 从不赋值，前端还把它当兜底 ⇒ 永远显示 0。
- C-05：`EstimateCalibrator.Ratio()`（含样本数）无消费者 ⇒「校准到底生效没有」不可观测（`Adjust` 在样本<5 时静默不生效）。
- D-12：前端共享辅助 `keyDisplayRef` 零生产调用，7 个视图各写一遍 `key_id || #id`。
- E-12：`ReconcileMoney` 注释称「周期运行」，生产从未接线（真跑的是另一套 SQL 的 `BillingReconciler`）。
- E-13：`GetAPIKeyBySecret` 注释称「请求路径鉴权必须使用」，实际生产自己 `HashAPIKey` 后调 `GetAPIKeyByKeyContext`。

### 3.3 同一件事两套实现（未接线的一套已漂移）

| 概念 | 两套实现 | 漂移后果 |
|---|---|---|
| trace_id 生成 | middleware（`tr-<微秒%1e8>-<12B>`）；storage 两处兜底逻辑相同且 **`TraceID = SessionID`**、格式是第三种 `tr-<微秒>` | 未注入 trace 的写入路径会把会话亲和 ID 写进链路 ID 列 — A-11 |
| API key 掩码 | `storage.MaskAPIKey`（前 8 + 后 4）；`middleware/maskAPIKey`（前 10 无后缀） | 同一实体两种显示 — A-11 附带 |
| 请求指标 | router `RecordRequest` + api `RecordRequestWithModel`（同一组计数器） | 一元双计、流式单计 ⇒ QPS/失败率/P99 失真 — B-04 = C-08（两域独立发现同一根因） |
| Anthropic 出站转换 | `adapter.AnthropicAdapter.FromCanonical`（**未接线**）vs `api/inbound_anthropic.go` 手写 | 未接线那套仍会合成无签名 `thinking` 块、不扣 cache、id 无 `msg_` 前缀，测试还把错误契约钉死 — B-05 |
| 拓扑主备分层 | 调度器：最小 priority 为第一层；拓扑页：硬编码 `Priority == 1` | 全 priority=2 时控制台显示 0 主 N 备、权重占比全 0 — B-09 |
| 分时折扣非法值 | 行级钳到 1.0（「never invent a discount」）；槽位级回落到行级折扣 | 写 `1.5`（想加价）或 `0`（不想打折）实际得到 5 折 — C-10 |
| 模型通配符 | 定价：只认尾部 `*`；校验规则：支持 `x*`/`*x`/`*`/精确 | `*o1`、`gpt-4o*mini` 能存库但永不匹配，静默回落 — C-11 |
| RPM 限流 | Redis：严格 60s 滑动窗口；本地：满桶起步的令牌桶 | 语义不等价；Redis 抖动回退满桶 = 瞬时配额翻倍 — C-12 |
| MCP 协议版本 | `/v1` 发现面写 2024-11-05；`/mcp` 写 2026-07-28 + 三版协商 | 客户端可能降级 — D-10 |
| key 响应形状 | `apiKeyView`（显式字段）；`PUT /admin/keys/:id` 直接返回裸 storage 记录 | 该端点字段集由 storage tag 决定，其他端点不会 — D-11 |
| 两侧写库时间戳 | `responseContextTimestamp`（PG 传 `time.Time`）只在 response_contexts 用；其余 `created_at` 全绑 SQLite 文本 | PG 会话时区非 UTC 时整体偏移；`verification_codes.expires_at` 三种约定 ⇒ 验证码立即过期 — E-04/E-05/E-06 |
| 超时链 | 自检只比 channel↔edge(86400s)；真正会先截断的 `http.Server.WriteTimeout`(600s/Helm 180s) 完全不读 | 启动日志宣称「已证明不变量」 — A-06 |

### 3.4 文档 / 清单与代码不符

- A-05：`GATEWAY_FIRST_TOKEN_TIMEOUT` 代码是 `min(渠道 timeout, override)`（对所有渠道生效、只能缩短），`.env.example` 与 `CONFIGURATION.md` 都写「只影响未配置 timeout 的渠道」。
- A-04：边缘 `client_max_body_size 50m` < 网关 64 MiB，413 文案却写 "gateway limit"。
- A-13：文档承诺「密码 ≥16 / 弱签名密钥拒绝启动」，代码只查 8 字符 + 5 个占位串精确匹配。
- A-14：`configs/config.yaml` 头部称「所有参数均已与 .env 打通」，但 `read/write_timeout_sec`、`log_level`、`model_validation` 无任何 env 通路。
- A-07/A-08：`GATEWAY_ADMIN_RESET` 只有 Compose 传；存储驱动三种清单三种接线（Compose 强制 S3 凭据、Swarm 完全没配、Helm 只能靠未文档化的 `--set config.storage.*`）。
- A-09：访问日志「跳过探针」清单是 `/health`，而四份部署清单实际探的是 `/health/ready` ⇒ 噪音没被过滤。
- D-13：`/v1/responses`（README 主推）不在 API 参考里；`/v1/moderations`、`/v1/models`、`/mcp` 也没有。
- A-15：代码还读 `.env.example` 清单外的 `S3_ENDPOINT`、`STRIPE_BASE_URL`（未文档化别名）。

### 3.5 死代码 / 仅测试使用的生产 API 面

`deadcode` 从 `main` 不可达、但被 `-test` 判定可达的 15 个符号 **全部只在测试里出现**。逐条核实后分三类：

| 类型 | 条目 | 处置建议 |
|---|---|---|
| 注释明示测试钩子（**不是问题**） | `ops.ResetReadinessChecks`、`telemetry.ResetWarnGate`、`storage.SetGlobalRepository`(半单例)、`middleware.WithTraceID`、`mcp.authenticateCaller`(薄封装) | 保留或加「仅测试」注释 |
| 被取代的公开 API 面 | `billing.CalculateCost*` 5 个变体（生产只走 `CalculateCostForUnitsWithCacheCreation`）、`storage.ReconcileMoney`、`GetAPIKeyBySecret`、一批单条仓储方法（`GetChannel`/`GetModelPrice`/`SetModelFallback`/`CreateRedemptionCode`/`ListUsageLogs`/`RecordUsageLog`/`BatchRecordUsageLogs`） | 删除，或注释标「仅测试/兼容」 |
| 漂移陷阱 | `adapter.AnthropicAdapter.FromCanonical`（误接即回归）、`telemetry/validator` 的死访问器 | 删除或合并 |

另：`tokenBucket.Capacity/Rate`（A-12）、`billing.Ratio`（C-05）、`ValidationError.Error`（C-07 的后果是审计静默）均属此类。

---

## 4. 全量发现索引

> 影响/置信度沿用分报告判定；「已核」= Lead 独立复现过。

### 4.1 配置 / 启动 / 部署（[01-config-bootstrap.md](01-config-bootstrap.md)，15 条）

| ID | 一句话 | 影响 | 置信 | 已核 |
|---|---|---|---|---|
| A-01 | D15 守卫在反代后恒判私网，出厂拓扑下从不拒绝 | 高 | 高 | ✅ |
| A-02 | nginx ACL 只在 1/3 配置、且只在默认 vhost | 高 | 高 | ✅ |
| A-03 | `isControlPlanePath` 漏 `/`，console shell 在 `/` 无守卫（并入 D-02） | 中 | 中 | ✅ |
| A-04 | 边缘 50m vs 网关 64 MiB 矛盾，413 文案归错组件 | 中 | 高 | |
| A-05 | 首 token 超时语义两份文档反向描述 | 中 | 高 | |
| A-06 | 超时链自检漏掉 `WriteTimeout` 这一层 | 中 | 高 | ✅ |
| A-07 | `GATEWAY_ADMIN_RESET` 只在 Compose 接线 | 中 | 高 | |
| A-08 | 存储接线三种清单三种程度 | 中 | 高 | |
| A-09 | 日志跳过清单漏 `/health/ready` | 低 | 高 | |
| A-10 | 402/429 拒绝不落审计（与注释矛盾） | 中 | 高 | ✅ |
| A-11 | trace_id 三处生成、两处用 session_id 冒充 | 中 | 高 | ✅ |
| A-12 | `tokenBucket.Capacity/Rate` 死访问器 | 低 | 高 | ✅ |
| A-13 | 凭证强度门禁文档 16 字符/弱值拒绝，代码 8 字符/5 个占位串 | 中低 | 高 | |
| A-14 | `config.yaml`「所有参数与 .env 打通」不成立 | 低 | 高 | |
| A-15 | 读清单外未文档化别名 `S3_ENDPOINT`/`STRIPE_BASE_URL` | 低 | 高 | ✅ |

### 4.2 路由 / 渠道 / 协议（[02-router-protocol.md](02-router-protocol.md)，13 条）

| ID | 一句话 | 影响 | 置信 | 已核 |
|---|---|---|---|---|
| B-01 | 请求预算只接 Chat 一元 | 中 | 高 | |
| B-02 | 流式首 token 后失败不入熔断账 | 中 | 高 | ✅ |
| B-03 | 404 处置三套规则（Chat 换渠道 / Embedding 终止 / 多模态透传） | 中 | 高 | ✅ |
| B-04 | 请求指标 router+api 双计 | 中 | 高 | ✅ |
| B-05 | `adapter.AnthropicAdapter.FromCanonical` 未接线且已漂移 | 低 | 高 | |
| B-06 | Responses 入口不设 Protocol ⇒ `openai_response` 渠道永不命中 | 中 | 高 | ✅ |
| B-07 | `count_tokens` 是唯一绕过熔断器的数据面路径 | 低 | 中 | |
| B-08 | 多模态响应不回写 model，别名还原只覆盖结构化路径 | 低 | 中 | |
| B-09 | 拓扑页把 `Priority==1` 硬编码为主层 | 低 | 高 | |
| B-10 | `errors.go` 注释称 429 计入渠道失败（代码排除，行为对/注释旧） | 低 | 高 | |
| B-11 | 跨模型兜底只对 Chat/Stream 生效 | 低中 | 中 | |
| B-12 | `openai_text` 流式未接 reasoning 标签剥离 | 低 | 高 | |
| B-13 | moderation 判空跑一次选择器 ⇒ SWRR 权重单请求推进两次 | 低 | 高 | |

### 4.3 计费 / 校验 / 遥测 / 分布式（[03-billing-telemetry.md](03-billing-telemetry.md)，14 条）

| ID | 一句话 | 影响 | 置信 | 已核 |
|---|---|---|---|---|
| C-01 | 5 个 `CalculateCost*` 死链，唯一扣费入口是 `WithCacheCreation` | 中 | 高 | ✅ |
| C-02 | 缓存写入 1.25× 只在 native Anthropic 流式 | 中 | 高 | ✅ |
| C-03 | 校准整表覆盖写库，两副本互相覆盖 | 中 | 高 | |
| C-04 | 校准只覆盖 chat 形状估算 | 低 | 中 | |
| C-05 | `Ratio()` 无消费者，校准是否生效不可观测 | 低 | 高 | ✅ |
| C-06 | `disallow_temperature` 只有 OpenAI 校验器消费 | 中 | 高 | ✅ |
| C-07 | `ValidationError.Error()` 从未调用 ⇒ 校验拒绝静默 | 低 | 高 | |
| C-08 | 请求指标双计（= B-04） | 中 | 高 | ✅ |
| C-09 | 启动定价检查第二段被短路 | 中 | 高 | ✅ |
| C-10 | 分时槽位非法折扣回落成 5 折 | 中 | 高 | |
| C-11 | 定价通配符只认尾部 `*`，保存不校验 pattern | 中 | 中 | |
| C-12 | Redis RPM 与本地令牌桶语义不等价 | 低 | 中 | |
| C-13 | moderation 是唯一缺币种前置门的计费入口 | 低 | 高 | |
| C-14 | 币种门校验别名价、扣费用 served 模型价 | 低 | 高 | |

### 4.4 API / 控制面 / 前端（[04-api-console.md](04-api-console.md)，14 条）

| ID | 一句话 | 影响 | 置信 | 已核 |
|---|---|---|---|---|
| D-01 | 渠道列表明文回传上游 api_key | 中 | 高 | ✅ |
| D-02 | 控制面谓词漏 `/`、`/assets`、`/api/v1/user`、`/api/v1/auth` | 中 | 高 | ✅ |
| D-03 | MCP `airoute_query_logs` 返回原始 digest | 中 | 高 | |
| D-04 | `/stats/*` 的 `key=` 参数表示随角色而异、非法值静默归零 | 中 | 高 | |
| D-05 | MCP 管理员判定两套，stdio 下 SQL 工具恒被拒 | 中 | 高 | |
| D-06 | 两个内置 MCP 工具不在 `tools/list` | 低 | 高 | ✅ |
| D-07 | 4 个端点全仓无调用方 | 低 | 高 | |
| D-08 | `/admin/system/middlewares` 是未建成页面的预留端点 | 低 | 高 | |
| D-09 | `format_validation` 无自动计算、无展示（**见第 5 节修正**） | 低 | 高 | ✅（已修正） |
| D-10 | `/v1` 与 `/mcp` 声明的 MCP 协议版本不一致 | 低 | 高 | |
| D-11 | `PUT /admin/keys/:id` 返回裸 storage 记录 | 低 | 高 | |
| D-12 | `keyDisplayRef` 零调用，7 处视图重复实现 | 低 | 高 | |
| D-13 | API 参考缺 `/v1/responses` 等已实现入口 | 低 | 高 | |
| D-14 | 渠道更新「零值即忽略」vs 密钥更新指针语义 | 低 | 高 | |

### 4.5 存储（[05-storage.md](05-storage.md)，16 条）

| ID | 一句话 | 影响 | 置信 | 已核 |
|---|---|---|---|---|
| E-01 | 成功 trace 采样连判 3 次；all 模式只覆盖 2 个 chat 入口 | 中 | 高 | ✅ |
| E-02 | trace TTL 清理随 `TRACE_CAPTURE=off` 一起消失 | 中 | 高 | |
| E-03 | `billing_settlements.tenant_id`/`total_tokens` 只写不读 | 低 | 高 | |
| E-04 | 写入侧时间戳不分方言（PG `TIMESTAMPTZ` 依赖会话时区） | 中 | 高 | |
| E-05 | `verification_codes.expires_at` 三种时区约定 | 中 | 高 | |
| E-06 | `video_tasks` 单一 DDL 通吃双方言（PG 得到无时区列） | 低 | 高 | |
| E-07 | `idx_video_tasks_owner` 既服务不了清理也服务不了查询 | 低 | 高 | |
| E-08 | 已撤销密钥的 trace 对本人不可见（作用域用未删除键） | 中 | 高 | |
| E-09 | `ak_` 解析只接线在日志端点，stats 仍把 `ak_` 哈希成空 digest | 低 | 中 | |
| E-10 | FTS5 无 UPDATE 触发器，v32 改写 `error_message` 后索引陈旧 | 低 | 中 | |
| E-11 | `SetGlobalRepository` 死 API（`NewRepository` 直接写包变量） | 低 | 高 | ✅ |
| E-12 | `ReconcileMoney` 仅测试调用，注释称周期运行 | 低 | 高 | |
| E-13 | `GetAPIKeyBySecret` 注释称鉴权必须用，生产无人调用 | 低 | 中 | |
| E-14 | 一批被批量版取代的单条仓储方法只剩测试 | 低 | 高 | |
| E-15 | `StatsOverview.PeakRPM/PeakTPM` 只读不写恒 0 | 低 | 高 | |
| E-16 | `trace/chat/session` 单列索引服务不了 `= ? OR LIKE '%?%'` | 低 | 中 | |

---

## 5. Lead 核验记录与修正

**独立复现（读代码确认断点为真）**：A-01、A-02、A-03、A-06、A-10、A-11、A-12、A-15、B-02、B-03、B-04、B-06、C-01、C-02、C-05、C-06、C-09、D-01、D-02、D-06、E-01、E-11。

**修正 1 条（D-09，应为「未文档化能力」而非「死字段」）**：
`api_keys.format_validation` 并非全链路空转——`storage.APIKeyRecord.ToConfig()`（`repo_keys.go:535`）会把它带进 `model.APIKeyConfig.FormatValidation`，`config.ResolveValidationLevel()`（`validation_rules.go:74`）据此覆盖该 key 的校验级别，且 `getAPIKeyConfig(c)` 在三个校验入口都传了它。也就是说：**通过管理 API 显式设置它，运行时会生效**。真实缺口是「没有自动计算（格式探测）、没有前端展示、没有文档」，而不是「没人读」。建议按「补文档/补 UI，或删字段」二选一。

**细化 1 条（A-10 的措辞）**：分报告写「只有两条快路径调用 `auditAuthFailure`」，实际共 7 处调用（含 401 与存储侧 key 状态/额度）。结论不变：**钱包余额 402、在途额度 402、限流 429 三类拒绝确实不落审计**，与函数注释「402/403/429 stay visible」矛盾。

**另记 2 条（并行会话相关，未计入统计）**：
1. `admin_logs.go` 新增的 `key=ak_…` 解析分支目前**无客户端**——控制台日志筛选只发数字 `key_id`（`useLogsData.js:93`、`App.jsx:1299`）。属正在合入的修复的一部分，但要么前端改用 `key=`，要么删掉该分支。
2. 审计期间并行会话正把结算改为分组提交（新增 `settlement_batcher.go`、`billing/estimate.go`、`telemetry/histogram.go`），并给 `quota_tracker` 改整数单位预授权；本报告结论以 `1520d99` 为准，结算/配额相关的 P1 项请在合并后再核一遍。

---

## 6. 已排除（看着像、实为故意）

各分报告共列出 40+ 条排除项，最容易被误报的几类：

- **D15 守卫的 `/api/v1/admin`、`/app` 本身**：`AdminAccessGuard` 确实在 `router.go:92` 全局注册且有 wiring 测试；问题在 IP 判定（A-01）与谓词覆盖（D-02），不是「没挂」。
- **nginx `proxy_read_timeout 86400s` 与 `model.EdgeProxyTimeoutSeconds`**：由 `nginx_timeout_test.go` 强制一致，不是漂移。
- **401 不落 `usage_logs`**、**流式不套总预算**、**`count_tokens` 无渠道超时**、**`request_traces` 不做独立保留 worker**、**`billing_settlements` 无保留期**、**FTS5 仅 SQLite / pg_trgm 仅 PG**、**`channel` 的相关度字段**：均有注释/文档说明，属刻意设计。
- **`mcp.authenticateCaller`、`forwardJSONRPCToolCall`、`ResetReadinessChecks`**：`deadcode` 命中但均为已声明保留的测试/兼容壳，非安全洞、非漂移。
- **v1.1 裁剪残留**：路由表、非迁移后端代码、`web/src` 导航/路由/i18n、`API_REFERENCE.md` 全部干净（`org_id`/`cost_basis`/`usage_daily`/`wallet_transactions` 在非测试代码零命中；v40 删除清单无残留引用）。
- **数据面「无人读」的请求字段**（`top_n`、`voice`、`aspect_ratio` 等）：随 body 原样透传给上游，不是「接受但忽略」。
- **`usage_logs` 23 个写入列**：逐列核对，没有整列只写不读；三张审计表存的都是 digest，没有误存 `ak_` 导致 JOIN 落空。

另：本轮指标/请求量口径的双计（B-04/C-08）与「27 个 Prometheus 指标自增点与导出点双向闭合」并不冲突——前者的两份样本各自都导出了，只是同一请求被计入两次。

---

## 7. 建议的下一步

1. **先修 P0 四条**（控制面边界 + 渠道明文密钥）：前两条是部署清单与一个函数（`admin_access.go` 改用 `ResolveClientIdentity`），D-01 是渠道读路径加 `api_key_configured` + 掩码。
2. **再处理 P1**：C-02（缓存写入溢价）、C-09（启动检查短路）、A-10（审计覆盖）都是小改动、高收益。
3. **P2 按「同类根因」批量做**：时间戳方言助手统一（E-04/E-05/E-06 一次改完）、digest→`ak_` 投影下沉到 storage 层（D-03/D-04/E-08/E-09 一次改完）、指标写入收敛到一层（B-04/C-08）、死 API 与死端点一次性清理（3.5 节 + D-07/D-08）。
4. **补测试钉住语义**：尤其是 429 不计熔断、采样只判一次、`format_validation`/`key=` 的取舍——本项目已有「注释与代码漂移后被后人改回去」的先例（B-10）。

---

## 8. 修复结果（2026-10-28 同日完成）

**72 条全部处置**：70 fixed、1 撤回（D-09 是误报，见 §5）、1 从「部分修」补到全修（B-07）。另按同一根因补修了 3 处审计未列出的问题（见 8.2 第 9 条与分域文件）。

**总体验证**（修复后）：

```
gofmt -l internal cmd tests web     → 空
go build ./...                      → OK
go vet ./...                        → OK
go test ./...                       → 16/16 包 ok（含 tests/ 集成）
go test -race ./internal/{api,router,storage}  → ok
cd web && npm run lint && npm test  → 0 warning；51 文件 / 666 用例全过
go run deadcode@v0.52.0 ./...       → 见 8.3（仅剩已声明测试钩子与并行会话遗留）
```

分域修复记录：A [fixes-config-security.md](fixes-config-security.md) · B [fixes-router-provider.md](fixes-router-provider.md) · C [fixes-billing.md](fixes-billing.md) · D [fixes-api.md](fixes-api.md) · E [fixes-storage.md](fixes-storage.md) · F [fixes-controlplane-web.md](fixes-controlplane-web.md)

### 8.1 逐域结果

| 域 | 条目 | 结果 |
|---|---|---|
| A 配置/安全/部署 | A-01…A-14（A-15 由 E/F 分做） | 全 fixed。A-01 守卫改 `ResolveClientIdentity` + Compose 固定 `edge` subnet（172.30.0.0/16）；A-02 三份 nginx 全补 ACL + 清单断言测试；A-03 调查后判定 `/` 是公开落地页、**不**纳入守卫，改为把「控制面 = /api/v1/admin + /app」写进代码与文档；C-12 本地限流改为与 Redis 同语义的 60s 滑动窗口 |
| B 路由/协议/适配 | B-01…B-12 + C-02 provider 侧 | 全 fixed。B-04 删掉 router 侧 4 处指标自增；B-05 删除已漂移的 `AnthropicAdapter.FromCanonical` 并改为显式护栏；B-07 补齐 4 个熔断访问器并把 count_tokens 7 个出口记账接全 |
| C 计费/校验/遥测 | C-01/C-03/C-05/C-06/C-10/C-11/C-12 | 全 fixed。C-01 删 5 个死变体；C-03 校准改 read-merge-write；C-11 glob 下沉到叶子包 `internal/model`（避开 config→billing 循环依赖），并用「旧实现作 reference」逐例证明既有行为零变化 |
| D API 层 | C-07 + B-06/B-07/B-08/B-13 + C-02/C-04/C-13/C-14 + D-03/D-05/D-06/D-07/D-08/D-10/D-13 + E-01 | 除 D-09（撤回）外全 fixed。删 4 个无调用方路由；MCP 日志投影 digest→`ak_`；`/v1` 与 `/mcp` 版本发现面统一；API 参考补齐 responses/moderations/models/MCP |
| E 存储层 | E-01…E-16 + A-11/A-15 | 全 fixed。E-01/E-02 修采样与 TTL；E-04/E-05/E-06 统一 `dialectTime`；新迁移 **v42**（删 3 个无用索引、重建 video 索引、FTS UPDATE 触发器 + rebuild）；E-12 删 `ReconcileMoney`；E-14 删 7 个单条方法、迁 27 个测试文件 |
| F 控制面/前端 | D-01/D-04/D-09/D-11/D-12/D-14/E-08/E-09 + C-10/D-08 | 全 fixed。D-01 渠道响应改 `channelView`（零明文 + `api_key_configured`/`api_key_masked`）；D-14 存储层 coercion 由 E 移除后端到端可用；顺带修正两个「假测试」（原本靠 JSON 解析失败而非重叠检测拿 400） |

### 8.2 升级注意：本批修复改变了以下线上行为

1. **控制面默认 fail-closed**（A-01）：反代部署必须正确配置 `GATEWAY_TRUSTED_PROXIES`，否则 `/api/v1/admin`、`/app` 对所有人 403。⚠️ **本条的取值已被后续提交 `2f43d28` 修正**：必须写**代理自身地址**（Compose 出厂是 `172.30.0.10/32`），**不能写整个 edge 网段**——宿主机经发布端口访问时源地址是网桥网关 `172.30.0.1`，信任整个网段会把它当成又一层代理跳，导致客户端无法归因、宿主机自己打开控制台 403。以 `.env.example` / `docs/CONFIGURATION.md` 的当前值为准（见 [第 3 轮 DEPLOY-01](../2026-10-29-bughunt/README.md)）。
2. **边缘 ACL 全 vhost 生效**（A-02）：从公网经 nginx 访问 `/app`、`/api/v1/admin` 现在会被 403（这是原本的设计意图）。
3. **渠道响应不再回传上游 `api_key`**（D-01）：外部脚本若读 `channel.api_key` 判断"是否已配置"，改读 `api_key_configured`。
4. **`/stats?key=` 语义收紧**（D-04）：`ak_…`→digest 自动解析；无法解析的值现在返回 **400**（原先静默返回 0 统计）。
5. **4 个端点删除**（D-07）：`/api/v1/user/me`（用 `/api/v1/auth/me`）、`GET /api/v1/user/wallet/orders`、`DELETE /api/v1/user/keys/:id`（用 admin 路由）、`GET /api/v1/admin/skills/:id/download`（用公开下载）。
6. **MCP 日志工具的 `api_key` 字段改为 `ak_…`**（D-03）：agent 侧若按 digest 匹配需调整。
7. **MCP 工具清单新增 2 项**（D-06），`/v1` 声明版本改为 2026-07-28（D-10）。
8. **渠道 priority/weight/timeout 显式 0 现在会真正落库**（D-14）：创建路径仍是「0 = 默认值」，更新路径改为「显式值原样保存」。
9. **令牌桶→滑动窗口**（C-12）：RPM 语义变为「任意 60s 窗口内最多 N 次」，突发不再可借（原先令牌桶允许等速率回填）；Redis 故障时只失去跨副本视图，不再放松额度。
10. **成功 trace 采样率变准**（E-01）：`GATEWAY_TRACE_SAMPLE=10` 原实际约 0.1%，修复后真为 10%——**归档量会上升约 100 倍**，请按磁盘/保留期复核；且 `capture=off` 时 TTL 清理现在照常运行（E-02）。
11. **请求指标不再双计**（B-04/C-08）：`airoute_requests_total`/失败率/延迟分位数约为原先的一半/更准——**Prometheus 告警阈值与看板基线需要复核**。
12. **Responses 流量会分给 `openai_response` 渠道**（B-06）：多协议渠道的流量分布会变化。
13. **`openai_text` 流式现在剥离 reasoning 标签**（B-12）；流内错误帧文案改为 `upstream <ch> returned status N: msg`（B-02）。
14. **管理签名密钥 <32 字符将拒绝启动**（A-13，fail-closed）；密码阈值仍为 8。
15. **Compose 存储默认从 s3 改回 local**（A-08），且不再强制 S3 凭据；依赖旧默认值的部署需显式 `STORAGE_DRIVER=s3`。
16. **新迁移 v42** 在启动时执行（删索引/重建索引/FTS 触发器 + rebuild），大表上首次启动会有一次索引维护开销。
17. **删除了公开 API 面**：billing 5 个 `CalculateCost*`、7 个 storage 单条方法、`ReconcileMoney`、`middleware.WithTraceID`、`telemetry.Metrics.RecordRequest`、`provider.IsClientError`——仅内部包，外部无影响。

### 8.3 遗留（非本轮范围）

**本轮修复自身引入的 4 个新死符号已在本轮末尾清掉**（删路由后遗留的 `ListUserRechargeOrders`/`DeleteUserKey`、被替换的 `IsClientError`、被委托取代的 `ValidateSlotsOverlap`），对应测试也同步迁移。

- ~~**并行会话遗留的 5 个死符号**~~（已在后续提交 `d15e6b4`「drop five dead symbols the quota/estimate refactor left behind」清除，`deadcode` 现只剩 4 个已声明测试钩子）：`billing.EstimateCostUnits`、`middleware.ReserveUserQuota`/`ReleaseUserQuota`/`MaxInflightPerUser`、`storage.usageAuditIDsTableExists`。
- **SQLite 迁移注册表缺 v43**（对方的分区迁移只注册 PG 分支）：建议补 no-op，保持两方言版本号一致。
- **B-11 增强项**：模态兜底递归未并入上一模型的 channel trail（失败行 channel 只留最后一跳）。
- **`dispatcher_modality.go` 仍 473 行**（> 300 行指引）：拆分属行为无关大重构，留待后续。
- **D-09 报告修订**：04-api-console.md 仍按"死字段"描述，实际是「可设置且运行时生效、缺 UI/文档」，以 §5 与 D 域修复记录为准。
- **`deadcode` 仍报 4 个已声明测试钩子**：`MCPHandler.authenticateCaller`、`forwardJSONRPCToolCall`、`ResetReadinessChecks`、`telemetry.ResetWarnGate`（均有注释说明，属刻意保留）。

> deadcode 使用注意：把库包直接喂给它（`./internal/xxx/...`）会打印 `no main packages` 且**不做任何分析**（假阴性）；必须对 main 入口跑：`go run golang.org/x/tools/cmd/deadcode@v0.52.0 ./cmd/airoute`（或 `./...`）。

### 8.4 提交状态与安全网

- 另一个会话的结算/配额/分区改造已提交 **5 个 commit**（`223c5de`…`c8a2765`）。
- **本批修复仍在工作区未提交**：相对 `c8a2765` 共 **169 个文件、+3544/−1563**，另有 53 个新增（未跟踪）文件。便于你 review 后自行提交或按域拆分。
- 快照/备份：`git stash create` 得到 `7ff15cf`（我们开工前的双方 WIP 状态）；`/tmp/wip_combined.patch`（6068 行合并 diff）、`/tmp/wip_untracked.tar.gz`（未跟踪文件）。
