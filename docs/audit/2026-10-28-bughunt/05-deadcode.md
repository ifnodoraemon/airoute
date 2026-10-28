# 05 · 代码瘦身：死代码 / 重复实现 / 无用资产盘点与安全删除

- **日期**：2026-10-28
- **基线**：`d15e6b4`（`refactor: drop five dead symbols the quota/estimate refactor left behind`，即 Lead 已完成的 5 个死符号删除，净 −119 行）
- **环境**：Go 1.26.1（`/usr/local/go/bin`）、Node v24.21.0、`helm` 3.x 可用
- **本轮纪律**：只做「删除 / 合并」的净减，不做纯重构；先报告后删除；涉及包外 API、测试覆盖面或语义判断的一律只报告。

---

## 0. 方法与覆盖

| 手段 | 命令 / 做法 | 覆盖 |
| :--- | :--- | :--- |
| 函数级死代码 | `/tmp/deadcode-bin52 ./cmd/airoute`（x/tools deadcode v0.52） | 主入口可达性 |
| 真死代码（含测试根） | `/tmp/deadcode-bin52 -test ./...` | 测试可达性 |
| 声明级全量盘点 | 自写 stdlib `go/ast` 工具，导出 5,325 条顶层声明（func/method/type/const/var/struct field）并做全仓引用计数 | 275 个非测试 `.go`（53,065 行）+ 325 个测试文件 |
| 复制粘贴检测 | 自写工具：`go/scanner` 归一化函数体（标识符→`I`、字面量→`L`）后 SHA1 分组 | 全部非测试函数，≥5 行才计入 |
| 前后端 API 面 | 路由表提取（110 条路径）↔ `web/src` / `cmd` / `docs` 消费方核对 | 端点 |
| 前端资产 | `acorn` + `acorn-jsx` 解析 129 个 `web/src` 模块，构建 import 图（含 `lazy(() => import())` 动态导入） | 组件 / 导出 / CSS 类 / 依赖 |
| 配置面 | `.env.example`（100 键）↔ Go `os.Getenv`/`env*` 助手读取点（110 个 env 名，97 个生产读取）↔ `docs/CONFIGURATION.md` | env 变量 |
| 数据库面 | 正则抽取 DDL 的 191 个表/列，统计 DDL 之外的引用 | 列（近似，见 §4.5 局限） |
| 依赖 / chart | `go mod tidy -diff`、`helm template` 前后对比、`helm lint` | go.mod / helm values |

**覆盖率校验**：`go list -deps ./cmd/airoute` 与 `go list ./...` 的差集只有 `tests`（纯集成测试包）与 `web/node_modules/flatted/golang`（vendored 第三方），因此「对主入口跑 deadcode」对全部生产包都是完整的。

---

## 1. ① 已删清单（第一轮自行删除，净 −17 行）

### D1. `internal/storage/settlement_batcher.go`：删除重复的 `envPositiveIntDflt`（净 −13 行）

- **符号**：`envPositiveIntDflt`（原 `internal/storage/settlement_batcher.go:120-131`，含注释 12 行）
- **证据**：
  - 与 `internal/storage/db.go:597` 的 `envPositiveInt` **函数体逐行相同**，仅形参名 `dflt`/`fallback` 不同（归一化函数体哈希一致，见 §3 第 10 组）。
  - 同包、未导出、无测试引用：`grep -rn 'envPositiveIntDflt' --include='*.go' internal` 仅命中本文件的 2 个调用点 + 声明。
  - 唯一差异是 2 处调用点参数（`GATEWAY_SETTLEMENT_QUEUE`、`GATEWAY_SETTLEMENT_BATCH_SIZE`），改调 `envPositiveInt` 语义完全一致（同样 `TrimSpace`→`Atoi`→非正数回落默认值）。
- **改动**：删除该函数与注释；2 个调用点改名为 `envPositiveInt`（`+2 / −15`）。
- **影响面**：包内私有；无 API 面变化；无行为变化。
- **风险**：低。
- **验证**：见 §6。

### D2. `helm/airoute/values.yaml`：删除无模板消费的 `serviceAccount` 值块（净 −4 行）

- **符号**：`serviceAccount.create` / `serviceAccount.name`（原 `values.yaml:17-19` + 空行）
- **证据**：
  - `grep -rn 'serviceAccount' helm/airoute/templates/` → **0 命中**；`_helpers.tpl` 也没有 `serviceAccountName` 引用；`docs/`、`deploy/` 同样无引用。
  - `helm template airoute helm/airoute` 前后对比：剔除该 chart 随机生成的 `admin-password` / `admin-secret`（`randAlphaNum`）后 **diff 为空**。
  - `helm lint helm/airoute` → `1 chart(s) linted, 0 chart(s) failed`。
- **影响面**：chart 渲染结果不变（此前用户即便设置这两个键也无效）。
- **风险**：极低。

### 为什么第一轮没有更多删除（第二轮的批准项见 §7'）

| 检查项 | 结果 |
| :--- | :--- |
| deadcode 主入口 | 仅 4 个**已声明测试钩子**（`forwardJSONRPCToolCall`、`MCPHandler.authenticateCaller`、`ResetReadinessChecks`、`ResetWarnGate`）——按任务约定不删 |
| deadcode `-test ./...` | 0 条（仅命中 `web/node_modules/flatted` 的 vendored Go 文件，不属于本仓库代码） |
| 未引用顶层声明（func/type/const/var） | **0** |
| 未引用测试辅助函数 | **0** |
| web/src 未引用模块 / 组件 / 导出 | **0**（详见 §4.1） |
| 未使用的 `.ui-*` CSS 类 | **0**（24 个全部有引用） |
| `go mod tidy -diff` | 空（无未用依赖） |

结论：仓库在本轮之前已被多轮审计清理得相当干净，**不存在「零引用 + 零行为变化」的整块死代码可再删**。剩余机会集中在「重复实现合并」（§3）与「接线未完成的功能」（§4.4），都属于需要判断的项，已全部转入 §2 待批。

---

## 2. ② 待 Lead 批准清单

### A. 重复实现合并（同包、语义等价，但属于「合并」而非「删除」，需批准）

| # | 合并对象 | 位置 | 差异 | 预计净减 | 风险 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| A1 | 3 个 bounded-batch purge | `repo_response_contexts.go:97-121`、`retention_maintenance.go:110-135`、`retention_maintenance.go:139-162` | 仅「表名 + 时间列 + ORDER BY 列 + 批次常量」不同；控制流逐行相同 | 实际 −25 行 | **已批准并执行（见 §7-R1）** |
| A2 | `responseContextOwner` ↔ `videoOwner` | `api/responses_handler.go:176-186`、`api/video_handler.go:23-33` | **函数体逐字节相同**（同包未导出，各仅 1-2 个调用点） | −10 行 | 低（但两处领域语义不同，建议统一命名后再删，故未自行处理） |
| A3 | 两个 retention worker 循环 | `storage/billing_ledger_retention.go:142-160`、`storage/usage_retention.go:88-106` | 仅 retention 值与日志文案不同 | ≈ −8 行 | 中 |
| A4 | env 读取小助手跨包重复 | `api/upstream_lifecycle.go:38`、`router/circuit_breaker_config.go:42`、`storage/repo_timeseries.go:59`、`storage/db.go:597`、`settlement_batcher.go:121` | 同一「正整数 env + 默认值」实现被复制 5 份到 4 个包 | ≈ −20 行 | 中（需新建公共 helper 包，会改 import 面） |
| A5 | `env=="true"` 判定 3 份 | `api/metrics_access.go:53`、`storage/migrations_partition.go:138`、`api/upstream_lifecycle.go:29`、`router/circuit_breaker_config.go:34` | 同一 `strings.TrimSpace(os.Getenv(name))=="true"` | ≈ −12 行 | 低-中 |
| A6 | 3 个 purge/retention 的 `Stop()` / worker 收尾 | `async_logger.go:194`、`settlement_batcher.go:310`、`billing_ledger_retention.go:134`、`billing_reconcile_job.go:89` | 同体（`close` + `wg.Wait`），接收者不同 | ≈ −10 行 | 中（需提取公共 `stopWorker`，收益有限） |

> 说明：A2-A6 均为「合并」而非删除，会引入新的参数化 helper，属于结构性改动，本轮只报告不实施。估算净减合计约 −60 行。
>
> **A2-A6 的执行状态**：Lead 第二轮仅批准 A1（storage 侧同包合并，本文件 §7-R1 已执行）；跨包 env 小助手统一（A4/A5）按 Lead 决定**不做**（需新增包、收益小）；A2 涉及跨领域命名、A3/A6 收益有限，留待后续。

### B. ~~`format_validation`：只写不读~~ —— **Lead 复核后撤回（误报）**

- **撤回结论**：该字段**是被读取并参与校验决策的**，不构成「只写不读」。
- **完整引用链**（Lead 提供，已逐点复核）：
  1. `internal/storage/repo_keys.go:540` — `APIKeyRecord.ToConfig()` 把 `rec.FormatValidation` 写入 `model.APIKeyConfig.FormatValidation`；
  2. `internal/config/validation_rules.go:75-79` — `ResolveValidationLevel(modelName, protocol, keyCfg)` 在 `keyCfg.FormatValidation != ""` 时用它**覆盖**生效校验级别；
  3. `internal/validator/pipeline.go:16/27/38` — `ValidateOpenAIRequest` / `ValidateAnthropicRequest` / `ValidateGeminiRequest` 三个入口都把 `keyCfg` 传进 `ResolveValidationLevel`。
- **我上一轮误判的原因**：第一轮 grep 的作用域是 `internal/api`、`internal/middleware`、`internal/validator`、`internal/router`，**漏掉了真正读取它的 `internal/config`**；且读取表达式是 `keyCfg.FormatValidation`（经由 `model.APIKeyConfig`），不在我先前列出的 `storage`/`admin_keys` 文件里。教训已记入 §7 的方法学备注。
- **处置**：**不删、不改**。上一轮「接线未完成」的说法作废。

### C. ~~`.env.example` 中的死 env 条目~~ —— **撤回（误报，审计方法有盲区）**

- **撤回结论**：`GITHUB_CLIENT_SECRET` / `GOOGLE_CLIENT_SECRET` **在生产的 OAuth 换码路径中被真实读取**，不是死配置。
- **证据**：`internal/controlplane/oauth_handler.go:227`
  `clientSecret := os.Getenv(fmt.Sprintf("%s_CLIENT_SECRET", strings.ToUpper(provider)))`（同文件 `:226/:228` 还有 `%s_CLIENT_ID`、`%s_REDIRECT_URI`）。因为变量名是 **`fmt.Sprintf` 动态拼出来的**，任何「按字面量 grep」的审计都会 0 命中——这正是我第一轮得出错误结论的原因；Lead 复核时同样受此盲区影响。
- **行为**：`clientID != "" && clientSecret != ""` 才走真实授权码交换；secret 为空时真实换码被跳过，非测试环境直接 `403 系统未配置 … 生产应用密钥`（`oauth_handler.go:276-286`）。
- **因此**：两行 env **保留不动**；并顺带补上了此前确实缺失的 `GITHUB_REDIRECT_URI` / `GOOGLE_REDIRECT_URI` 文档（见 §7-R2）。

### D. 反向文档缺口（代码读取但未文档化）—— **已补齐（见 §7-R3）**

- `GATEWAY_ENABLE_TABLE_PARTITIONING`（`storage/migrations_partition.go:90`）：原 `.env.example` 与 `docs/CONFIGURATION.md` **均未出现** → 已补；
- `GATEWAY_DB`（`cmd/airoute/server.go:53`）：`docs/CONFIGURATION.md` 原有该名称但未写清「旧别名 / 优先级 / 风险」→ 已细化；
- `GITHUB_REDIRECT_URI` / `GOOGLE_REDIRECT_URI`（`oauth_handler.go:30-31`）：**两处文档都没有** → 已补（本轮新发现）；
- `HOSTNAME`/`KUBERNETES_SERVICE_HOST`/`NODE_ID`（运行时注入，属合理，不补）。

### E. 无控制台消费者的端点（**保留，仅登记**）

| 端点 | 位置 | 证据 | 结论 |
| :--- | :--- | :--- | :--- |
| `GET /admin/system/middlewares` | `internal/api/router.go:235` | `web/src`、`cmd` 均 0 消费；但 `router.go:231` 与 `docs/ROADMAP.md:119`（B-07）明确写明为「控制台自检页」预留，**页面落地前不得删除** | 保留 |
| `POST /admin/mcp/servers/:id/probe` | `internal/api/router.go:297` | `web/src` 0 消费（`McpIntegrationView.jsx` 只调 `/toggle`）；仅 `controlplane/admin_mcp_local_detection_test.go` 覆盖 | **Lead 决定：登记不删**（具备真实网络探测能力，属功能面而非死代码） |

### F. 18 个「零 selector 读写」结构体字段：**全部保留**

全部是 OpenAI/Anthropic/Gemini 兼容协议的 **JSON wire 模型**，靠 `encoding/json` 反射读写（`grep` 自然为 0）：`ImageGenerationRequest.Quality/ResponseFormat/Style`、`ImageItem.B64JSON/RevisedPrompt`、`AudioSpeechRequest.Voice/ResponseFormat/Speed`、`AudioTranscriptionResponse.Language`、`VideoGenerationRequest.AspectRatio/Resolution`、`VideoTaskResponse.Progress`、`RerankRequest.TopN/ReturnDocuments`、`EmbeddingRequest.EncodingFormat`、`PromptTokensDetails.AudioTokens`、`AnthropicContentBlock.Signature`、`geminiEmbedContentRequest.TaskType`。删除任何一个都会改变数据面协议/透传能力 → **不删**。

### G. 超限文件（§5）：本轮不拆，仅出清单与拆分建议。

---

## 3. ③ 重复实现清单（31 组，归一化函数体哈希相同）

「同包」= 可直接合并；「跨包」= 需下沉公共 helper；「误报」= 仅 AST 形状相同、语义不同。

| # | 行数 | 侧 A | 侧 B | 差异 | 分类 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 1 | 25/26/24 | `storage/repo_response_contexts.go:97` `PruneResponseContexts` | `storage/retention_maintenance.go:110` `PurgeVerificationCodesBefore`、`:139` `PurgeVideoTasksBefore` | 表名/时间列/ORDER BY/批次常量 | **同包 · 已合并（§7-R1）** |
| 2 | 19/19 | `storage/billing_ledger_retention.go:142` `BillingLedgerRetention.worker` | `storage/usage_retention.go:88` `UsageLogRetention.worker` | retention 值、日志文案 | 同包 · A3 |
| 3 | 15/15 | `storage/billing_ledger_retention.go:79` `ResolveBillingLedgerRetention` | `storage/usage_retention.go:26` `ResolveUsageLogRetention` | env 名 + 默认值 + 返回结构 | 同包 · 可参数化（导出 API，保留签名） |
| 4 | 6/14 | `controlplane/channel_service.go:241` `normalizeChannelStatus` | `storage/repo_video_tasks.go:36` `videoTasksDDL` | 一个是字符串映射，一个是 DDL 执行 | **误报** |
| 5 | 12/12 | `storage/repo_channels.go:297` `BatchUpdateChannelStatus` | `storage/repo_keys.go:472` `BatchUpdateAPIKeyStatus` | B 多 `AND status <> 'deleted'` | 同包 · 需 flag 参数，收益 ≈0 |
| 6 | 12/12 | `storage/repo_keys.go:458` `BatchDeleteAPIKeys` | `storage/repo_pricing.go:41` `BatchDeleteModelPrices` | 表名/软删列不同 | 同包 · 收益 ≈0 |
| 7 | 11/11 | `api/responses_handler.go:176` `responseContextOwner` | `api/video_handler.go:23` `videoOwner` | **逐字节相同** | 同包 · A2 |
| 8 | 11/11 | `api/upstream_lifecycle.go:38` `upstreamDrainTimeout` | `router/circuit_breaker_config.go:42` `breakerCooldown` | env 名/默认值不同 | 跨包 · A4 |
| 9 | 11/11 | `router/circuit_breaker_config.go:54` `breakerFailureThreshold` | `storage/repo_timeseries.go:59` `ttftSampleLimit` | env 名/默认值不同 | 跨包 · A4 |
| 10 | 11/11 | `storage/db.go:597` `envPositiveInt` | `storage/settlement_batcher.go:121` `envPositiveIntDflt` | 仅形参名 | **同包 · 已合并（D1）** |
| 11 | 10/10 | `api/common.go:809` `localRerankPromptEstimate` | `model/tokens.go:72` `RerankRequest.EstimatedPromptTokens` | 同语义的 rerank prompt 估算，api 侧是本地复制 | 跨包 · 建议 api 直接复用 `model` |
| 12 | 10/10 | `api/mcp_client.go:247` `mcpReadBody` | `api/multimodal_handler.go:45` `readMultimodalBody` | 上限常量与错误文案不同（MCP 8 MiB / multimodal 64 MiB） | 同包 · 不合并（文案属可观测行为） |
| 13 | 10/10 | `middleware/auth.go:316` `DataPlaneUnreadyDenial` | `middleware/authz.go:137` `BalanceUnknownDenial` | 状态码相同但 code/message/Retry-After 全不同 | **误报** |
| 14 | 10/10 | `storage/async_logger.go:194` `AsyncLogger.Stop` | `storage/settlement_batcher.go:310` `SettlementBatcher.Stop` | 接收者不同 | 同包 · A6 |
| 15 | 9/9 | `storage/repo_mcp.go:123` `SetMCPServerEnabled` | `storage/repo_skills.go:74` `SetSkillEnabled` | 表名 + `SeedDefault*` 调用 | 同包 · 可参数化 |
| 16 | 9/9 | `storage/repo_model_routes.go:25` `createModelRoutesTable` | `storage/repo_oauth.go:12` `createOAuthIdentities` | DDL 语句不同 | 同包 · 可参数化（收益小） |
| 17 | 8/8 | `api/mcp_handler.go:50` `mcpProtocolSupported` | `controlplane/prober_heuristics.go:174` `contains` | 前缀匹配 vs 集合包含 | **误报** |
| 18 | 7/7 | `cmd/airoute/security_check.go:15` `allowInsecureDefaults` | `api/moderation_handler.go:22` `moderationStubEnabled` | env 名不同 | 跨包 · A5 |
| 19 | 7/7 | `api/metrics_access.go:53` `envFlagEnabled` | `storage/migrations_partition.go:138` `tablePartitioningEnabled` | env 名不同 | 跨包 · A5 |
| 20 | 7/7 | `api/upstream_lifecycle.go:29` `upstreamDrainingEnabled` | `router/circuit_breaker_config.go:34` `breakerEnabled` | env 名不同 | 跨包 · A5 |
| 21 | 7/7 | `controlplane/channel_view.go:73` `channelViews` | `controlplane/key_view.go:68` `apiKeyViews` | 元素类型不同 | 同包 · 可用泛型合并（收益 ≈0） |
| 22 | 7/7 | `provider/errors.go:63` `AsUpstreamError` | `router/errors.go:59` `AsDispatchError` | `errors.As` 目标类型不同 | 跨包 · 可泛型下沉 |
| 23 | 7/7 | `storage/billing_ledger_retention.go:134` `BillingLedgerRetention.Stop` | `storage/billing_reconcile_job.go:89` `BillingReconciler.Stop` | 接收者不同 | 同包 · A6 |
| 24 | 6/6 | `api/common.go:625` `modelNameOf` | `provider/errors.go:50` `UpstreamError.StatusCode` | 语义无关 | **误报** |
| 25 | 6/6 | `api/trace_hooks.go:57` `replayBody.Close` | `controlplane/pricing_service.go:260` `PricingService.reloadPrices` | 语义无关 | **误报** |
| 26 | 6×4 | `controlplane/admin_handler.go:133` `GetRepo` / `distributed/redis.go:242` `GetRDB` / `router/errors.go:51` `Unwrap` / `storage/repository.go:94` `GetDB` | — | 访问器惯例 | **误报** |
| 27 | 6×3 | `provider/anthropic.go:22` / `gemini.go:23` / `openai.go:52` 三个 `NewXProvider` | — | 构造器同形，Go 惯例 | **误报** |
| 28 | 6/6 | `router/errors.go:43` `DispatchError.Error` | `storage/repository.go:102` `Dialect` | 语义无关 | **误报** |
| 29 | 5/5 | `controlplane/sync.go:197` `Synchronizer.Ready` | `router/circuit_breaker.go:188` `CircuitBreaker.Disabled` | 语义无关 | **误报** |
| 30 | 5/5 | `controlplane/token.go:38` `getAdminSecret` | `distributed/redis.go:94` `GetClient` | 访问器惯例 | **误报** |
| 31 | 5/5 | `provider/client.go:90` `cancelOnCloseBody.Close` | `router/dispatcher_modality.go:65` `cancelOnCloseReadCloser.Close` | 两个 `io.Closer` 包装器 | 跨包 · 可下沉公共类型 |

**其它「单调用方薄包装」**（归一化未命中、但属冗余抽象，仅报告）：`api/common.go:620 estimateTokens → model.EstimateTokensFromChars`、`api/common.go:640 calibratedPromptEstimate → GlobalCalibrator.Adjust`、`api/common.go:651 streamFallbackPromptTokens → calibratedPromptEstimate`（3 层链式包装）、`provider/openai.go:158 ChatCompleteStream → chatCompleteStream`、`router/dispatcher.go:146 IsAnthropicWireChannel → isAnthropicWireChannel`、`router/dispatcher_stream.go:237 GetFirstTokenTimeout → resolveFirstTokenTimeout`、`storage/artifact_storage.go:48 FormatSkillKey → fmt.Sprintf`。这些多为「导出 API 需要稳定名」，删除会改包外 API，故不建议动。

---

## 4. ④ 无用资产清单

### 4.1 `web/src`（129 个模块、74 个命名导出、47 个默认导出）

| 检查项 | 结论 |
| :--- | :--- |
| 未被引用模块 | **0**。唯一未被 `src` 内部引用的是 `src/test/setup.js`，由 `vitest.config.js` 引用（非死） |
| 未被引用组件 / 导出 | **0**（解析了 `lazy(() => import(...))` 动态导入） |
| 仅测试使用的导出（6 个，属测试钩子，保留） | `api/client.js:35 resetApiClient`、`lib/escapeLayer.js:59 hasEscapeLayers`、`:64 resetEscapeLayers`、`lib/navGroups.js:90 navTabs`、`lib/providerPresets.js:15 ALL_PROTOCOLS`、`:43 PROVIDER_PRESETS` |
| 只在自身文件内使用、导出无外部消费者（6 个） | `MarkdownRenderer.jsx:101 RenderInline`、`config.js:56 csvEscape`、`hooks/useFormDirty.js:4 stableStringify`、`hooks/useKeyActions.js:21/27 clampSelfServiceRpm/Tpm`、`hooks/useModalA11y.js:19 useModalA11y`。去掉 `export` 只减 0 行（纯重构），本轮不做 |
| CSS 类 | `src/index.css` 定义 24 个 `.ui-*` 组件类，**全部有引用，0 个无用类**；其余均为 Tailwind `@apply` 组合与 CSS 变量（`--bg-*` 等被 `@layer base` 使用） |
| i18n 键 | 项目**没有 i18n 框架/资源文件**（中文文案硬编码在 JSX），无键可清 |
| `web/public` | `favicon.ico` / `favicon.svg` 均被 `index.html:6-7` 引用 |
| npm 依赖 | `lucide-react`/`marked`/`react`/`react-dom` 及全部 devDependencies 均有引用，无未用依赖 |

> 交叉核对：`docs/audit/2026-10-28-inconsistencies/04-api-console.md` 提到「前端共享的 `keyDisplayRef` 无人调用」——**该结论已过期**，`keyDisplayRef` 现被 7+ 个组件使用（`KeysView`、`AiCopilot`、`PlaygroundView`、`MyUsageView`、`OnboardingModal` 等），无需处理。

### 4.2 Go 侧结构体字段

> 本轮（第二轮）已按 Lead 要求用 AST 工具做**字段级全量盘点**：方法、四类清单（读写 / 只写不读 / 只读不写 / 从未读写）与逐条证据见 **§7-R4**。以下为第一轮的粗粒度结论，保留作对照。

- 顶层声明：5,325 条中 **0 条未被引用**。
- 「零 selector 读写」字段 18 个：全部为 JSON wire 模型（见 §2-F），保留。
- 「只写不读」selector 38 个：抽样全部为三类——(a) 序列化 DTO/遥测快照（`SuccessRate`、`IdleSeconds`、`PeakRPM`、`PeakTPM`、`AvgTTFT`、`P50/P90/P99`、`Dimensions`、`Meta`、`Costs`、`Failovers`、`CooldownRemainingMs`、`ChannelStatus`、`TimeRange`、`FirstRequestAt`、`LastRequestAt`、`Recent`）；(b) 上游请求模型（`IncludeUsage`、`RecoveryProbe`、`ProviderCustom`、`WeightPercent`、`TotalPrimaryWeight`、`HasCrossProtocol`、`ChainError`、`GeminiSystemInstruction`）；(c) 标准库字段（`TrimLeadingSpace`、`FieldsPerRecord`、`PoolSize`、`MinIdleConns`、`DialTimeout`、`ReadTimeout`、`WriteTimeout`、`Nullable`、`MultiReader`、`MaxBytesReader`、`MergeResponseConversation`、`WithTraceID`）。均通过 `encoding/json` 或第三方结构体消费，**不可删**。
- ~~唯一「接线未完成」的是 `format_validation`（§2-B）。~~ **已撤回（误报，见 §2-B 撤回说明）。**
- 「只读不写」类字段（`internal/distributed.ClusterEvent.NodeID` 从不赋值）已由同批 `02-core.md` C-03 上报，本文不重复。

### 4.3 配置项 / env 变量

| 检查 | 结论 |
| :--- | :--- |
| `internal/config` 40 个字段 | 全部被读取（引用数 ≤2 的 9 个已逐个复核：均为「yaml 解析 + 单一消费点」） |
| `.env.example` 100 键 | 96 个被 Go 生产代码读取；`POSTGRES_DB/PASSWORD/USER`、`TZ` 由 `docker-compose.yml` 消费（非死） |
| 死 env 文档条目 | `GITHUB_CLIENT_SECRET`、`GOOGLE_CLIENT_SECRET`（§2-C） |
| 代码读取但未文档化 | `GATEWAY_ENABLE_TABLE_PARTITIONING`、`GATEWAY_DB`（§2-D）；`HOSTNAME`/`KUBERNETES_SERVICE_HOST`/`NODE_ID` 为运行时注入 |
| 测试专用 env | `AIROUTE_TEST_*`、`AIROUTE_PARTITION_DEMO_DSN`、`S3_ENDPOINT`（合理，测试夹具） |

### 4.4 端点

110 条注册路径。除 §2-E 两条外均有消费方：数据面路径（`/chat/completions`、`/embeddings`、`/audio/*`、`/rerank`、`/responses`…）由第三方 SDK 消费；控制面 `:id` 类路径由前端模板字符串拼接（如 `users/${...}/status`、`skills/${...}/toggle`、`mcp/servers/${...}/toggle`），已按前缀逐个核对。

### 4.5 数据库列 / 索引

- DDL 共解析出 191 个表-列；**0 个列在 DDL 之外零引用**。
- **局限说明**：本方法基于 SQL 字符串正则在 storage 包内统计引用数，无法区分「只 INSERT」与「SELECT/WHERE」；精确「只写不读列」需要 SQL AST。本轮未发现零引用列，未做更强的读写分类。
- 索引层面本轮未发现「定义后无人使用」的新增项（此前审计已记录 `usage_logs` 相关索引的取舍）。

### 4.6 依赖与构建资产

- `go mod tidy -diff` → 空（go.mod/go.sum 无未用依赖）。
- `helm/airoute/values.yaml`：50 个叶子值中，`serviceAccount.*` 无模板消费（已删，D2）；其余（含 `.Values.config` 整块经 `toYaml` 渲染、`podSecurityContext`/`securityContext` 整块渲染）均有消费。
- `scripts/*.sh`：3 个脚本均被 `Makefile`/`.github/workflows/ci.yml`/`docker-compose.yml`/`RELEASE_GUIDE.md` 引用。
- `bin/airoute`（56 MB 二进制）与 `tests/data/` 未纳入 git（`.gitignore`），非仓库资产，未处理。

---

## 5. ⑤ 超限文件清单（>300 行；本轮不拆）

规范：`docs/ENGINEERING_GUIDELINES.md` §二「单文件 ≤300 行绝对红线」，例外为纯静态 Schema 与 SQL DDL 迁移文件。

**275 个非测试 `.go` 中 51 个超限**（占 18.5%）：

| 行数 | 文件 | 拆分建议 |
| ---: | :--- | :--- |
| 1026 | `internal/api/common.go` | 按职责拆 4-5 个文件：错误响应/审计辅助（`RespondOpenAIError`、`audit*`）、token 估算与校准（`estimateTokens`/`calibratedPromptEstimate`/`streamFallbackPromptTokens`）、上下文/键取值、流式工具 |
| 758 | `internal/storage/migrations_partition.go` | **DDL 迁移，规范豁免**；若要瘦身可把 43 号迁移的 SQL 移到 `migrations_partition_sql.go` |
| 675 | `internal/model/response.go` | 按协议对象拆：`response_request.go`（请求/输入类型）、`response_result.go`（响应/输出）、`response_convert.go`（与 chat 互转） |
| 643 | `internal/middleware/auth.go` | 拆 `auth_identity.go`（解析凭据/快照）、`auth_wallet.go`（余额与预扣）、`auth_denial.go`（拒绝响应构造） |
| 631 | `internal/storage/billing_reconcile_job.go` | 拆 `billing_reconcile_run.go`（runOnce 主体）、`billing_reconcile_drift.go`（`tenantLedgerDrift`/基线设置）、`billing_reconcile_config.go` |
| 627 | `internal/storage/db.go` | 拆 `db_open.go`（连接/DSN/重试）、`db_time.go`（dialectTime）、`db_helpers.go`（tableExists/withReturningID/env 助手） |
| 625 | `internal/storage/repo_logs.go` | 拆 `repo_logs_query.go`（列表/筛选）、`repo_logs_facets.go`（聚合/分面）、`repo_logs_stats.go` |
| 599 | `internal/storage/settlement.go` | 拆 `settlement_run.go`（批处理执行）、`settlement_retry.go`（重试/退避/可重试判定）、`settlement_wallet.go`（钱包扣减） |
| 591 | `internal/storage/repo_keys.go` | 拆 `repo_keys_crud.go`、`repo_keys_query.go`、`repo_keys_batch.go`（批量状态/删除） |
| 549 | `internal/api/inbound_anthropic_stream.go` | 拆 SSE 解析/事件翻译/结束帧处理三段 |
| 526 | `internal/storage/migrations_postgres.go` | DDL 迁移，豁免 |
| 511 | `internal/storage/billing_ledger_retention.go` | 拆 retention 配置解析 / worker / prune 执行 |
| 506 | `internal/storage/migrations_sqlite.go` | DDL 迁移，豁免 |
| 502 | `internal/billing/engine.go` | 拆 rate card 解析 / 查询（`GetPrice*`）/ 计算（`Calculate*`） |
| 486 | `cmd/airoute/server.go` | 拆 bootstrap / 超时链 / 关停流程（已有 `server_env.go`、`timeout_chain.go` 可继续切） |
| 484 | `internal/middleware/authz.go` | 拆拒绝响应 / 额度检查 / 客户端身份解析 |
| 473 | `internal/router/dispatcher_modality.go` | 拆 modality 分发 / 跨模型回退 / 错误映射 |
| 471 | `internal/api/mcp_tools.go` | 拆工具注册 / 只读查询校验 / 工具实现 |
| 465 | `internal/storage/async_logger.go` | 拆 writer / spill / stats |
| 441 | `internal/api/mcp_client.go` | 拆 HTTP 传输 / JSON-RPC 编解码 / 回退判定 |
| 432 | `internal/storage/repo_timeseries.go` | 拆查询 / 缓存 / 统计聚合 |
| 417 | `internal/storage/repo_reconciliation.go` | 拆报表查询 / 差异计算 |
| 415 | `internal/api/mcp_handler.go` | 拆会话/SSE/工具调用入口 |
| 412 | `internal/controlplane/oauth_handler.go` | 拆授权跳转 / 回调 / 用户名分配 |
| 412 | `internal/api/inbound_anthropic.go` | 拆请求转换 / 响应转换 |
| 405 | `internal/storage/repo_users.go` | 拆 CRUD / 钱包 / 唯一冲突处理 |
| 396 | `internal/router/dispatcher.go` | 拆候选发现 / 选择策略 / 执行 |
| 395 | `internal/storage/repo_logs_writer.go` | 拆单条写入 / 批量写入 / 编解码 |
| 395 | `internal/api/gemini_inbound.go` | 拆 unary / 请求转换 / 流适配 |
| 389 | `internal/controlplane/sync.go` | 拆快照类型 / 同步器 / 指纹 |
| 385 | `internal/api/mcp_client_http.go` | 与 `mcp_client.go` 同域，按传输/编解码重划 |
| 381 | `internal/provider/anthropic_stream.go` | 拆事件解析 / 帧重组 |
| 366 | `internal/telemetry/ring_buffer.go` | 拆 ring buffer / broadcast handler |
| 364 | `internal/router/circuit_breaker.go` | 已按状态模式拆状态，可把探针/指标再拆出 |
| 360 | `internal/telemetry/metrics.go` | 拆计数器 / 分位数 / 校准样本 |
| 359 | `internal/controlplane/login_guard.go` | 拆本地存储 / 策略 / 计数 |
| 354 | `internal/controlplane/admin_mcp.go` | 拆 CRUD / probe / 本地判定 |
| 349 | `internal/controlplane/admin_skills.go` | 拆 CRUD / artifact / manifest |
| 348 | `internal/controlplane/prober_adapters.go` | 拆各 provider 适配器 |
| 348 | `internal/api/router.go` | 拆 middleware 装配 / 公共组 / 管理组（当前一个函数装配全站） |
| 343 | `internal/middleware/quota_tracker.go` | 拆本地 tracker / Redis 后端适配 |
| 342 | `internal/storage/repo_channels.go` | 拆 CRUD / 批量 / 视图转换 |
| 333 | `internal/storage/usage_spill.go` | 拆写入 / 重放 / 回收 |
| 330 | `internal/controlplane/admin_overview.go` | 拆总览聚合 / 中间件拓扑 |
| 326 | `internal/api/mcp_tools_routing.go` | 拆路由工具 / 参数校验 |
| 322 | `internal/provider/openai.go` | 拆 URL 构建 / unary / stream |
| 319 | `internal/storage/settlement_batcher.go` | 拆 group-commit 循环 / 等待 / 停止 |
| 315 | `internal/controlplane/route_service.go` | 拆校验 / 写入 / 价格重载 |
| 312 | `internal/controlplane/admin_keys.go` | 拆 CRUD / 批量 / 视图 |
| 307 | `internal/distributed/redis_quota.go` | 拆 Lua 脚本常量 / 额度读写 / 心跳 |
| 306 | `internal/controlplane/pricing_service.go` | 拆校验 / 落库 / 重载 |

> 优先建议（收益/风险比最高）：`api/common.go`、`middleware/auth.go`、`storage/db.go`、`billing/engine.go`、`api/router.go`——它们都是「多职责集中」的典型，拆分不改变行为，且被测试覆盖。

---

## 6. ⑥ 净减行数统计与验证

### 净减（仅本任务改动，两轮合计）

```
$ git diff --numstat -- internal/storage/settlement_batcher.go helm/airoute/values.yaml \
      internal/storage/retention_maintenance.go internal/storage/repo_response_contexts.go
0       4       helm/airoute/values.yaml                        → 净  -4   （第一轮）
2       15      internal/storage/settlement_batcher.go           → 净 -13   （第一轮）
22      28      internal/storage/retention_maintenance.go        → 净  -6   （第二轮 §7-R1）
4       23      internal/storage/repo_response_contexts.go       → 净 -19   （第二轮 §7-R1）
----------------------------------------------------------------------------
生产代码 + chart 合计：+28 / −70  →  净 −42 行

13      2       docs/CONFIGURATION.md                            → 净 +11   （第二轮 §7-R2/R3，文档）
4       1       .env.example                                     → 净  +3   （第二轮 §7-R2/R3，文档）
86      0       internal/storage/retention_purge_test.go (新文件) → 净 +86   （第二轮 §7-R1，新增行为测试）
```

- **生产代码 + chart 净减：−42 行**（第一轮 −17，第二轮 −25）。
- **文档净增 +14 行**（撤回 2 条误报、补 3 处缺口：分区开关、`GATEWAY_DB` 语义、OAuth redirect URI）。
- **新增测试 +86 行**（覆盖合并后 helper 的分批/提前终止/失败返回三条路径）。
- 工作区全部改动合计（我的文件）：`+45 / −73 → 净 −28`。
- 不含 Lead 已提交的 `d15e6b4`（5 个符号，净 −119 行）。

> 说明：同一工作区还有其它 teammate 的并行改动（`internal/billing/*`、`internal/middleware/*`、`internal/distributed/redis_quota.go`、`internal/storage/settlement.go`、`web/src/*` 及多个 `*_bugprobe*` 文件）。这些不属于本任务，未计入上述数字，也未触碰。

### 验证命令与结果

| 命令 | 结果 |
| :--- | :--- |
| `gofmt -l internal cmd tests` | **无输出**（clean） |
| `go build ./...` | **PASS** |
| `go vet ./...` | **PASS**（第二轮重跑已恢复绿色：并行 teammate 的 `redis_quota.go` 重命名已收敛） |
| `go test ./...`（全部 16 个测试包） | **PASS / EXIT=0** — `cmd/airoute`、`internal/{adapter,api,billing,config,controlplane,distributed,middleware,model,provider,router,storage,telemetry,validator}`、`tests`、`web` 全 `ok`（另有 2 个包无测试文件） |
| `go test ./internal/storage/ -count=1` | **PASS**（含新增 `TestPurgeBeforeCutoffBatchingAndFailure` 3 个子测试） |
| `go test -run 'PurgeVerification|PurgeVideoTask|PruneResponseContexts|PurgeBeforeCutoff' ./internal/storage/` | **PASS** — 合并后三个 purge 的既有行为测试与新测试全绿 |
| `helm template airoute helm/airoute`（改前/改后） | 剔除随机的 `admin-password`/`admin-secret` 后 **diff 为空** |
| `helm lint helm/airoute` | `1 chart(s) linted, 0 chart(s) failed` |
| `go mod tidy -diff` | **空** |

前端本轮未删除任何文件，故未触发 `npm run lint` / `npm test`；`web/src` 的结论来自 AST import 图静态分析 + `grep` 复核。

---

## 7. 结论

1. **Go 死代码面已归零**：主入口 deadcode 仅剩 4 个既定测试钩子；5,325 条声明 0 条未引用；0 个未用测试辅助。仓库此前多轮审计已把这块清干净。
2. **两轮安全删除/合并 3 项、生产代码净 −42 行**：逐字节重复的 env 小助手、无模板消费的 chart 值、3 个 bounded-batch purge 合并为 1 个 helper；行为均可证明不变（含新增的分批/失败路径测试）。
3. **两条第一轮结论已被撤回**（`format_validation`、OAuth client secret），撤回证据与误判原因见 §2-B / §2-C；撤回比删错更重要。
4. **剩余净减空间**：同包重复实现 A2/A3/A6 约 −25 行（A4/A5 跨包统一按 Lead 决定不做），以及 §5 的 51 个超限文件（拆分属重构，另开专项）。
5. **结构体字段级盘点**见 §7-R4：`usageLogPartition.from`、`S3Storage.useSSL`、`EstimateCalibrator.observed` 为非序列化的「只写不读」字段候选（本轮只报告，未删）。

---

## 7'. 第二轮（已批准项）执行记录

> 批准范围（Lead）：写权 `internal/storage`、`docs/CONFIGURATION.md`、`.env.example`；**暂不碰 `internal/api`**。
> 环境：Go 1.26.1；工作区有 5 个 teammate 并行改动。

### R1. 同包重复实现合并：3 个 bounded-batch purge → 1 个 `purgeBeforeCutoff`

- **改动**：
  - `internal/storage/retention_maintenance.go`：新增私有 `(*Repository).purgeBeforeCutoff(deleteStmt string, cutoff time.Time, batchSize int)`，把「逐批 DELETE + `deleted < batch` 终止」的循环收敛为一处；
  - `PurgeVerificationCodesBefore` / `PurgeVideoTasksBefore` 改为一行委托，DELETE 语句原样保留为各自参数（表名/时间列差异留在调用点，注释保留）；
  - `internal/storage/repo_response_contexts.go`：`PruneResponseContexts` 同样改为委托（DELETE 语句与 `responseContextPruneBatchSize` 原样保留）。
- **语义等价性**：方言时间绑定仍走 `r.dialectTime(cutoff)`；每批上限仍是各表原常量（500）；终止条件仍是 `deleted < batchSize`；错误路径仍返回「已删计数 + error」；nil/未初始化仍返回 `(0, nil)`。SQL 文本逐字符未变（只是从内联改为参数传递），因此查询计划/索引使用不变。
- **净减**：`retention_maintenance.go` +22/−28、`repo_response_contexts.go` +4/−23 → **−25 行**。
- **测试**：现有 `TestVerificationCodeRetentionPrunesExpiredRows`、`TestVideoTaskRetentionPurgesOldRows`、`TestVideoTaskRetentionStillWorksAfterIndexChange`、`TestResponseContextExpiryAndPrune` 覆盖「cutoff 语义 / 单批删除 / 存活行保留」；**未覆盖**分批循环与错误返回，故新增 `internal/storage/retention_purge_test.go`（+86 行）：
  - `drains in bounded batches` — 插入 `retentionPruneBatchSize+7` 行，断言删满 507 且表清空（走满批 → 短批两次迭代）；
  - `stops after a short first batch` — 3 行一次删完，再跑一次返回 `(0, nil)`（提前终止）；
  - `reports a statement failure without a partial count` — 关闭 DB 后调用，断言 `err != nil && deleted == 0`（错误返回）。
- **验证**：`go test -count=1 ./internal/storage/` PASS；`go test ./...` 16 包全绿。
- **结论**：零行为变化，符合「保持方言处理/批次上限/终止条件/返回值语义完全不变」的要求。

### R2. 文档删减：OAuth client secret —— **批准项撤回，未删，改为修正错误承诺**

- **为什么不删**：Lead 与我都判断「无生产读取」，但复核代码后发现 `internal/controlplane/oauth_handler.go:226-228` 用 `fmt.Sprintf("%s_CLIENT_SECRET", …)` **动态构造 env 名**读取，字面量 `grep` 必然 0 命中。真实行为：id + secret 都非空才走真实授权码交换，secret 为空时非测试环境直接 `403`。删掉这两行会误导运维「不需要 secret」。
- **实际改动**（`.env.example`）：保留两行，修正注释为「client id + client secret 都需要（secret 为空时真实换码被跳过，登录返回 403）」。
- **同类修正**：`docs/CONFIGURATION.md` 的 OAuth 行补齐「id 与 secret 均从环境读取 + 空 secret 的 403 行为 + state cookie 要求」，并补上此前**两处文档都没有**的 `GITHUB_REDIRECT_URI` / `GOOGLE_REDIRECT_URI`。
- **遗留口径**：`docs/audit/2026-10-28-inconsistencies/04-api-console.md` 关于 `format_validation`「没有任何代码计算或显示」的旧结论同样作废（见 §2-B），建议由该报告的 owner 同步更正。

### R3. 补文档（反向缺口，非删减额度）

1. `GATEWAY_ENABLE_TABLE_PARTITIONING`
   - `docs/CONFIGURATION.md` §2.4 表格新增一行：默认 **unset（off）**、仅 PostgreSQL、`1/true/yes/on` 才注册迁移 43、其他值（含未设置）不注册任何 v43 迁移、启动行为不变。
   - 新增小节 **Usage-log table partitioning (opt-in, PostgreSQL only)**：表重写代价（ACCESS EXCLUSIVE + 全表 copy ≈2× 大小 + 索引重建）、`lock_timeout=5s` 导致迁移可延迟重试、`UNIQUE(billing_id)` 由 `usage_audit_ids` 注册表替代、**升级顺序**（先全量升级到含注册表写路径的版本，滚动发布期间保持关闭，否则旧二进制 `42P10`）、**回滚步骤**（保留 `usage_logs_legacy_<unix>`，具体 SQL 指向 `migrations_partition.go` 文件头）。
2. `GATEWAY_DB`：把原有合并行拆清语义——`GATEWAY_DB` 是**旧别名**，仅在 `GATEWAY_DB_DSN` 为空时读取，`-db` 参数优先；并写明风险（换路径会让现有部署静默启动在空库上）。
3. `GITHUB_REDIRECT_URI` / `GOOGLE_REDIRECT_URI`（本轮新发现）：补进 `docs/CONFIGURATION.md` OAuth 行 + `.env.example`（注释形式，默认由 `PUBLIC_URL` 推导）。

### R4. 结构体字段级盘点（只报告，未删）

**方法**：自写 stdlib `go/ast` 工具（`/tmp/fieldaudit`），对 275 个非测试 `.go` 的 **1,624 个结构体字段**统计四类事件：

| 事件 | 判定方式 |
| :--- | :--- |
| 写 | 赋值语句 LHS 中的 selector、`++`/`--` 目标、键控复合字面量 `T{Field: v}` 的 key |
| 读 | 其余所有 selector 使用 |
| 取地址 | `&x.Field`（`rows.Scan(&x.F)`、`json.Unmarshal(&x)` 这类**反射写入**的典型形态，单列避免误判） |

**置信度约定**：selector 不带类型信息，按字段名聚合会跨类型串台。因此「高置信」结论只取**字段名在全仓 1,624 个字段中唯一**的条目（74/71/19 条），并逐条 `grep` 复核；同名（`Tokens`、`ID`、`Ready` 等）条目不进入结论。

| 分类 | 全部条目 | 唯一名（高置信） | 结论与证据 |
| :--- | ---: | ---: | :--- |
| **读写** | 1,389 | — | 正常字段，无动作 |
| **只写不读** | 94 | 74 | 绝大多数带 `json:` tag 的 DTO/遥测快照，由 `encoding/json` 或前端消费（例：`channelView.APIKeyMasked`、`apiKeyView.KeyID`、`TimeSeriesData.Buckets`、`LatencyPercentiles.P50`、`BreakerSnapshot.RecoveryProbe`、`EnrichedPrice.CurrentDiscount`、`StreamOptions.IncludeUsage`、`GeminiSafetySetting.Threshold`）→ **不是死字段**。非序列化的 8 条见下 |
| **只读不写** | 120 | 71 | 三类，均非死字段：① 互斥锁（`Config.keyMu`、`Dispatcher.wrrMu/fallbacksMu`、`Repository.statsMu/facetMu`）；② 原子计数器/`sync.Once`（`telemetry.Metrics.*` 共 40+、`LiveMetrics.activeStreams`、`Histogram.sumUS`、`Synchronizer.version`、`DB.closeOnce`）；③ 只由 `json.Unmarshal` 填充的入参 DTO（`RerankRequest.Query/Documents`、`ResponseRequest.Instructions`、`AnthropicUsage.*`、`GeminiResponse.Candidates`） |
| **从未读也未写** | 21 | 19 | 全部是 OpenAI/Anthropic/Gemini wire 模型，靠序列化读写（`RerankRequest.TopN/ReturnDocuments`、`RerankItem.RelevanceScore/Document`、`ImageGenerationRequest.Quality/Style`、`ImageItem.B64JSON/RevisedPrompt`、`AudioSpeechRequest.Voice/Speed`、`VideoGenerationRequest.AspectRatio/Resolution`、`VideoTaskResponse.Progress/VideoURL`、`EmbeddingRequest.EncodingFormat`、`PromptTokensDetails.AudioTokens`、`AnthropicContentBlock.Signature`、`geminiEmbedContentRequest.TaskType`）→ **保留** |

**只写不读 · 非序列化候选（唯一名，逐条 grep 复核）**：

| 字段 | 位置 | 引用证据 | 判定 |
| :--- | :--- | :--- | :--- |
| `usageLogPartition.from` | `internal/storage/migrations_partition.go:586`（赋值 `:633`） | 全仓 `.from` 仅 1 处（赋值）；消费方只用 `.name` / `.to` / `.isDefault` | **死字段候选**（约 −2 行）。当前供运维/未来用途未被使用，删除前需确认 `pg_get_expr` 解析不需要回读起点 |
| `S3Storage.useSSL` | `internal/storage/s3_storage.go:25`（赋值 `:73`） | 全仓 `useSSL` 仅 2 处（声明 + 构造时赋值）；`endpoint` 已含 scheme，签名走 `s.endpoint` | **死字段候选**（约 −2 行） |
| `EstimateCalibrator.observed` | `internal/billing/calibration.go:24`（`c.observed++` 于 `:81`） | 生产与测试均无读取（测试里的 `.observed` 是 `provider` 包另一个结构体） | **死计数器候选**（约 −3 行）。若只是观测用，可改走 telemetry 指标 |
| `auditLagBaseline.lagUnits` | `internal/storage/billing_reconcile_job.go:402`（赋值 `:450`） | 无读取；但字段注释明确写「kept for operators…the incremental math below does not need it」，且同值已持久化到 `billing_audit_lag_baseline_units` 设置项 | **刻意只写，保留**（注释已声明意图） |
| `TraceCapture.recorded` / `.dropped` | `internal/storage/trace_capture.go:34-35`（`++` 于 `:161/:166`） | 生产 0 读取；**测试读取**（`trace_capture_policy_test.go:32/57/68`） | **仅测试可观测**：生产是「只写不读」的死计数器，直接删会动测试覆盖面 → 待批准 |
| `OffPeakEvaluationContext.LocalTime` | `internal/billing/strategy.go:30`（赋值 `engine.go:357`） | 全仓无读取；但 `strategy.go`/`engine.go` 正被另一 teammate 改写（off-peak Days 语义修复） | **暂缓**：等对方改动落地后复核，可能正在接线 |

**结论**：本次盘点**未发现任何可直接删除且零影响的字段**——真正「非序列化的只写不读」只有 3 个（`from` / `useSSL` / `observed`），且删除收益各 2-3 行，属「要不要清死状态」的取舍，符合 Lead「只报告不删」的决定。

### R5. 按 Lead 决定暂不做的项

| 项 | 决定 |
| :--- | :--- |
| 跨包 env 小助手统一（§2-A4/A5） | **不做**（需新增公共包，收益小） |
| `POST /admin/mcp/servers/:id/probe` 端点删除 | **登记不删**（具备真实网络探测能力，属功能面） |
| >300 行文件拆分（§5，51 个） | **只报告**，另开专项 |

### 方法学备注（本轮踩到的两个盲区，建议写入后续审计 checklist）

1. **动态构造的环境变量名**：`os.Getenv(fmt.Sprintf("%s_CLIENT_SECRET", …))` 这类写法会让「按字面量 grep」得出「该 env 无用」的错误结论（§2-C 即因此误报）。审计 env 时应同时搜 `Getenv(fmt.Sprintf`、`Getenv(name)` 包装与常量间接引用。
2. **读取跨 package 边界**：`format_validation` 的消费点是 `internal/config.ResolveValidationLevel`（通过 `model.APIKeyConfig` 结构体字段读取），而非 `api/middleware/validator` 里的字面量；仅 grep 调用方所在包会漏掉「数据经结构体字段流入另一包消费」的链路（§2-B 即因此误报）。字段级「只写不读」结论必须结合 `&x.Field`（Scan/Unmarshal 写入）与跨包结构体传递来判定，单靠 selector 计数会误判。
