# D 域（internal/api）修复状态（2026-10-28）

修复对象：本轮内部一致性审计中 D 域（API/控制面/前端契约）落在 `internal/api/` 的问题，外加 P1 `C-07`（校验拒绝完全静默）。
基线：`c8a2765`（审计基线 `1520d99`；期间并行会话完成结算/配额改造，本次修复已落在其新版本上）。
允许写入范围：`internal/api/**`、`docs/API_REFERENCE.md`、`docs/ROADMAP.md`、本文件。

## 验证结果

| 项 | 命令 | 结果 |
| :--- | :--- | :--- |
| 格式 | `gofmt -l internal/api/` | 空（干净） |
| 编译 | `go build ./...` | 通过 |
| 静态检查 | `go vet ./internal/api/` | 通过 |
| 单元测试 | `go test ./internal/api/ -count=1` | `ok ... 10.3s` |
| 竞态 | `go test ./internal/api/ -count=1 -race` | `ok ... 37.0s` |
| 全仓测试编译 | `go vet ./...` | 通过（含所有测试文件） |
| E2E 会话端点 | `go test ./tests/ -run TestAdminAuth -count=1` | `ok`（该用例已预期 `/user/me` 重复路由被删） |
| 死代码复查 | `deadcode ./cmd/airoute`（v0.52.0） | `ListUserRechargeOrders` / `DeleteUserKey` 不再出现（收尾轮已删除这两个 handler） |
| 收尾轮回归 | `gofmt -l internal/controlplane` + `go test ./internal/controlplane/ ./tests/` | 格式空、`ok 7.3s` / `ok 3.7s` |

新增测试文件 10 个（见表格「验证」列），全部随主套件通过。已知非本域失败：`internal/controlplane/channel_service_test.go:332`（F 域，未触碰）。

## 修复表

| 发现ID | 状态 | 文件:行 | 验证 | 备注 |
| :--- | :--- | :--- | :--- | :--- |
| **C-07（P1）** 校验拒绝完全静默 | 已修 | `internal/api/validation_denial.go:24`（新）；三处接入点 `handler.go:98`、`inbound_anthropic.go:72`、`gemini_inbound.go:167` | `validation_denial_test.go`：`TestRejectInvalidRequestPublishesReason` / `…IgnoresNil` / `TestChatValidationRejectionIsAuditable` | 统一走 `rejectInvalidRequest`：写 `middleware.ContextKeyErrorMessage`（含协议/字段/规则，且过 `sanitizeErrorMessage`）→ 打一条带 `trace_id` 的结构化 WARN → 再 `WriteGinResponse(c)`。401/402 既有审计语义未改动 |
| **B-06** Responses 入口不设 Protocol | 已修 | `internal/api/responses_protocol.go:7`（新）；`responses_handler.go:81` | `responses_protocol_test.go`：`TestResponsesChatRequestTagsProtocol`、`TestResponsesProtocolIsRecognisedByChannelMatching` | 抽 `responsesChatRequest()` 统一标注 `Protocol="responses"`；并断言该值被 `ChannelConfig.SupportsProtocol` 认作 response 协议 |
| **B-07** `count_tokens` 绕过熔断 | 部分修 + handoff | `internal/api/inbound_anthropic.go:289` | `go test ./internal/api/`（无专用测试，见备注） | 准入已接：直连循环跳过 `GetBreakerStatus(name)=="OPEN"` 的渠道。**router 未导出写侧能力**（`CanExecute/RecordSuccess/RecordFailure/AbortCanary` 是 `*CircuitBreaker` 的方法，而 `Dispatcher` 无访问器），故无法记录成功/失败/canary；api 侧**不记录任何成功**，不会把故障渠道记成健康。handoff 见下 |
| **B-08** 多模态响应不回写 model | 已修 | `internal/api/response_model_restore.go:33`（新）、`multimodal_response_body.go:24`（新）；接线 `multimodal_handler.go:111`、`video_handler.go:181` | `response_model_restore_test.go`（6 个用例：仅改顶层 / 已匹配不动 / 二进制 / 非法 JSON / 数组 / 无 model） | 只改顶层 `model`（嵌套值以 `json.RawMessage` 原样保留）；非 JSON 对象/非法 JSON/超 4MiB 原样返回，multipart 与二进制（音频、图片）不解码并已注释说明；重写后同步 `Content-Length`。流式路径无缓冲，未接（不适用） |
| **B-13** moderation 判空跑了选择器 | 已修 | `internal/api/moderation_handler.go:87` | `go test ./internal/api/`（既有套件覆盖 501 路径） | 改用只读变体 `ListChannelsForModel`（router 注释明示「introspection endpoints … must use this variant」），不再推进 SWRR 权重或占用 half-open canary 名额 |
| **C-02（api 侧）** cache creation token 透传 | 已修 | `internal/api/anthropic_usage.go:13`（新）；`inbound_anthropic.go:125`（native 一元）、`:219`（canonical 一元） | `anthropic_usage_test.go`：`TestNativeCacheCreationTokens`（5 例）、`TestCanonicalCacheCreationTokens` | native 路径从 `usage.cache_creation_input_tokens` 读取；canonical 路径走 provider 域新增的 `model.Usage.GetCacheCreationTokens()`；两者都传入 `AuditRecordParams.CacheCreationTokens` 并计入 PromptTokens。与流式路径（`inbound_anthropic_stream.go:437`）口径一致 |
| **C-04** 校准只覆盖 chat 估算 | 已修 | `internal/api/common.go:749/761`（embedding）、`:799/805`（rerank） | `token_estimate_calibration_test.go`：`TestModalityEstimatesUseCalibration`、`TestUpstreamUsageFeedsCalibrator`、`TestEmbeddingTokenIDInputIsNotCalibrated` | 估算分支改走 `calibratedPromptEstimate(model, local)`；拿到真实 usage 时调 `observePromptEstimate` 反哺校准器；token-id 输入是精确 token 数，**不**套 tokenizer 比例（避免二次放大）。顺带修掉 nil 请求的 nil-deref（重构中暴露，既有测试 `TestResolveEmbeddingAndRerankTokens` 覆盖） |
| **C-13** moderation 缺币种前置门 | 已修 | `internal/api/moderation_handler.go:94` | `go test ./internal/api/` | `DispatchHTTP` 前补 `validateModelCurrency(c, modModel)`，失败即 422（`unsupported_pricing_currency`），不再出现「上游已付费 + 结算清零」 |
| **C-14** 币种门与扣费取价模型不一致 | 已修 | `internal/api/model_currency.go:50`（新）；`common.go:1019`；安装点 `router.go:103` | `model_currency_test.go`：`TestValidateModelCurrencyCoversServedModel`、`TestValidateModelCurrencyIgnoresUnpricedServedModel` | `validateModelCurrency` 现在遍历 `currencyModelsForBilling`（请求模型 + 计费真正会用的 served 模型）；served 解析复用只读能力（`Dispatcher.ListChannelsForModel` + `GetUpstreamModel`，经 `installServedModelEnumerator` 注入，与既有 `SetServedModelResolver` 同范式），**未改 router**；判定条件对齐 `billingModelFor`（仅当 served 有显式价目才算），避免把无价目映射误判成 422 |
| **D-03** MCP `airoute_query_logs` 返回 digest | 已修 | `internal/api/mcp_key_projection.go:16`（新）；接线 `mcp_tools_ops.go:161` | `mcp_log_projection_test.go`：`TestPresentMCPLogKeys`、`TestPresentMCPLogKeysMasksWithoutRepository` | 本地投影：非 `ak_` 值用 `APIKeyPublicIDsByDigest` 解析成公开 ID，解析不到（含无 repo）用 `storage.MaskAPIKey` 掩码；注释写明与 controlplane 同口径但刻意不跨包依赖 |
| **D-05** MCP 管理员判定两套 | 已修 | `internal/api/mcp_tools.go:62`（合并后唯一实现）；调用点 `mcp_tools.go:243/302/343`、`mcp_tools_ops.go:20/73`、`mcp_handler.go:370` | `mcp_auth_test.go`（扩写：新增 stdio `api_key` 参数授予/拒绝两例） | `callerIsAdmin(ctx, args)` 吸收原 `mcpCallerIsAdmin`：优先用 transport 已鉴权的 caller，其次 context key，最后 `args["api_key"]`（stdio 通路），再经 `callerIsAdminForKey`；删除冗余函数。SQL 工具与 infra 工具现在同一判定 |
| **D-06** 两个内置 MCP 工具不在 tools/list | 已修 | `internal/api/mcp_schemas.go:111`、`:115` | `mcp_catalog_test.go`：`TestBuiltinMCPToolsMatchDispatcher`（AST 解析 dispatch 源与清单双向比对）、`TestBuiltinMCPToolCatalogIncludesUtilityTools` | 补 `airoute_get_current_time`、`airoute_calc_eval` 两条 schema；新增双向一致性测试：dispatch 里有 case 但清单没有 → 失败，清单有但无 handler → 失败 |
| **D-07** 四个无调用方端点 | 已修（删除） | `internal/api/router.go:177/187/218`（`/user/me`、`/user/wallet/orders` 列表、`/user/keys/:id` DELETE、`/admin/skills/:id/download`） | `go build ./...` + `go test ./internal/api/`；全仓确认仅 `internal/controlplane/admin_skills_etag_test.go` 自建引擎注册该路径，不受影响 | 删除前再次全仓核对（web/src、cmd、docs、tests）；`docs/API_REFERENCE.md` 新增「已移除端点 → 替代端点」表 |
| **D-08** `/admin/system/middlewares` 无消费者 | 已修（保留 + 注释） | `internal/api/router.go:231`（路由注释）；`docs/ROADMAP.md:119`（B-07 状态） | 文档改动，`go build` 不涉及 | 端点保留（B-07 计划复用），路由处注明「当前无前端消费者、删除前先看 handler 注释」；ROADMAP B-07 补状态一句。handler 注释需落在 `internal/controlplane/admin_overview.go`，不在本域写入范围 → handoff |
| **D-10** `/v1` 与 `/mcp` 协议版本不一致 | 已修 | `internal/api/ops_handler.go:144`、`mcp_info.go:37/40`、`mcp_handler.go:18`（类型注释） | `mcp_version_test.go`：`TestV1InfoAdvertisesCurrentMCPVersion`、`TestMCPInfoMatchesSupportedVersions` | 三处发现面都从 `mcpSupportedVersions` / `mcpDefaultProtocolVersion` 渲染（`/v1` 拼「最新版 + 向下兼容到最旧版」）；同时修掉 `HandleMCPInfo` 在 nil repo 下的 panic（`ListSkills` 未判空，测试暴露的真实健壮性缺口） |
| **E-01（api 侧）** 采样连判 3 次 | 已修 | `internal/api/trace_hooks.go:89`（`RecordSuccessTrace`）、`:112`（`RecordTrace`） | `go test ./internal/api/`（既有 trace 套件全通过） | 去掉两处 `tc.Enabled` 预判，采样只在 `TraceCapture.Record` 决策一次；`Record` 返回 bool 已存在，据此设置 `ContextKeyTraceRecorded` 去重标记（不再先设标记后失败）。`RecordSuccessTrace` 仍只在 `GlobalTraceCapture != nil` 时 marshalling |
| **D-13** API_REFERENCE 缺端点 | 已修 | `docs/API_REFERENCE.md:133`（新增 2.7/2.8/2.9）、`:402` 起（第 5/6/7 节） | 文档改动，按实现逐字段核对 | 新增 `/v1/responses`（请求字段、非流式对象、SSE 事件序列、`previous_response_id` 语义与 400）、`/v1/moderations`、`/v1/models`、`/health/ready`、`/api/v1/public/status`、MCP 全部入口与协议版本/鉴权/授权/发现、以及「已移除端点」对照表。未编造：字段与事件名取自 `internal/model/response.go`、`responses_stream_events.go` |

## Handoff（跨域，未在本域解决）

1. **B-07 熔断记账（router 域）**：需要在 `internal/router` 暴露写侧能力，例如
   `func (d *Dispatcher) BreakerCanExecute(name string) bool`、`RecordChannelSuccess(name string)`、`RecordChannelFailure(name string) bool`、`AbortChannelCanary(name string)`（内部已有 `CircuitBreaker.CanExecute/RecordSuccess/RecordFailure/AbortCanary`，只缺 `Dispatcher` 访问器）。
   api 侧届时把 `inbound_anthropic.go:289` 的只读 `GetBreakerStatus` 判断替换为 `CanExecute`，并按结果记录成功/失败/不可重试时 `AbortCanary`。
   当前状态：**准入生效、记账缺失、绝不误记成功**。
2. **D-08 handler 注释（controlplane 域）**：`internal/controlplane/admin_overview.go:210`（`GetSystemMiddlewares`）加一句「当前无前端消费者，ROADMAP B-07 预留」，与路由注释呼应。
3. ~~**D-07 遗留 handler（controlplane 域）**：删除路由后 `AdminHandler.DeleteUserKey`、`AdminHandler.ListUserRechargeOrders` 已无调用方~~ → **已清（2026-10-28 收尾轮，Lead 授权窄范围改动）**：删除 `internal/controlplane/payment_handler.go` 的 `ListUserRechargeOrders` 与 `internal/controlplane/user_keys_handler.go` 的 `DeleteUserKey`（连带该文件因此不再使用的 `strconv` import）；两处均无测试用例直接覆盖（无测试请求 `/api/v1/user/keys/:id` 或 `/api/v1/user/wallet/orders`），故未删测试。`repo.ListRechargeOrders` 仍被 admin 钱包视图使用（`wallet_handler.go:237`），未受影响。验证：`gofmt -l internal/controlplane` 空、`go build ./...`、`go vet ./...`、`go test ./internal/controlplane/ ./tests/` 全 ok、`deadcode ./cmd/airoute` 输出中不再出现这两个符号。
4. **D-09（`format_validation`）审计结论更正（重要）**：审计报告 04 的 D-09 判定为「无任何代码计算或显示」，本轮核对发现**该字段是活的**：`internal/controlplane/admin_keys.go:119` 用 `normalizeFormatValidation` 归一化写入 → `storage.APIKeyRecord.FormatValidation` → `repo_keys.go:540` 投影进 `model.APIKeyConfig` → `internal/config/validation_rules.go:74` 作为**每密钥校验等级覆盖**被消费。D-09 应从发现列表撤回（本次不在修复清单内，未改代码）。
5. **`docs/audit/2026-10-28-inconsistencies/04-api-console.md`** 中 D-07/D-10/D-13/B-06/B-08/B-13/C-02/C-04/C-13/C-14 的现状描述已被本轮修复覆盖；该报告属历史审计记录，未回改（如需同步状态请在汇总轮统一处理）。

## 附：新增/修改文件清单

新增（`internal/api/`）：`validation_denial.go`、`response_model_restore.go`、`multimodal_response_body.go`、`model_currency.go`、`responses_protocol.go`、`mcp_key_projection.go`、`anthropic_usage.go`
新增（测试）：`validation_denial_test.go`、`response_model_restore_test.go`、`responses_protocol_test.go`、`model_currency_test.go`、`token_estimate_calibration_test.go`、`mcp_catalog_test.go`、`mcp_log_projection_test.go`、`mcp_version_test.go`、`anthropic_usage_test.go`
修改：`handler.go`、`inbound_anthropic.go`、`gemini_inbound.go`、`responses_handler.go`、`moderation_handler.go`、`multimodal_handler.go`、`video_handler.go`、`common.go`、`router.go`、`mcp_tools.go`、`mcp_tools_ops.go`、`mcp_handler.go`、`mcp_schemas.go`、`mcp_info.go`、`ops_handler.go`、`trace_hooks.go`、`mcp_auth_test.go`、`docs/API_REFERENCE.md`、`docs/ROADMAP.md`
