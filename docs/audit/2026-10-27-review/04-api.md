# API / Streaming Review — 2026-10-27

| 项 | 内容 |
|---|---|
| 日期 | 2026-10-27 |
| HEAD | `64e749a` (fix: resolve functional audit findings across auth billing and workflows) |
| 范围 | `internal/api/**`：chat/completions/responses/anthropic/gemini 入口与 SSE 流、图像/音频/视频多模态、embeddings/rerank/moderations、MCP（protocol/handler/sse/tools）、models/ops/handler/router/session/trace_hooks/common、使用量捕获与 servable-model 计费调用点 |
| 方法 | 只读审查 + 定向测试 + 私有 worktree 探针（HEAD 快照，不触碰仓库源码与服务） |
| 结论 | **20 项细节缺陷：7 项 P1、13 项 P2**；其中 **18 项已复现（CONFIRMED）**，2 项为静态确认（STATIC） |

## 验证命令与结果

```
# 仓库内基线（不修改源码）
$ go test ./internal/api/ -run 'Chat|Stream|Video|MCP|Responses|Gemini|Anthropic|Moderation' -race -count=1
ok  github.com/ifnodoraemon/airoute/internal/api  3.265s
$ go test ./internal/api/ -count=1
ok  github.com/ifnodoraemon/airoute/internal/api  1.037s
$ go test ./internal/router/ ./internal/middleware/ -count=1
ok  github.com/ifnodoraemon/airoute/internal/router  0.104s
ok  github.com/ifnodoraemon/airoute/internal/middleware  0.005s
```

探针：`git worktree add --detach /tmp/airoute-audit-api HEAD`（复制 `web/dist` 以便编译），新增 6 个 `zz_audit_probe*_test.go`（仅存在于该 worktree），全部断言**期望的正确行为**：

```
$ go test ./internal/api/ -run 'TestProbe' -count=1 -v
--- FAIL: TestProbeMultimodalClientCancelStatus          (BUG 复现)
--- FAIL: TestProbeModerationMasksUpstreamError          (BUG 复现)
--- FAIL: TestProbeMCPSSEUnauthorizedContentType         (BUG 复现)
--- FAIL: TestProbeMCPNotificationGetsResponse           (BUG 复现)
--- FAIL: TestProbeMCPInitializeVersionNegotiation       (BUG 复现)
--- FAIL: TestProbeGeminiEmbedContentIsChatCompletion    (BUG 复现)
--- FAIL: TestProbeGeminiModelsBypassAllowlist           (BUG 复现)
--- FAIL: TestProbeResponsesStreamEventShape             (BUG 复现)
--- FAIL: TestProbeStreamPromptEstimateIgnoresTools      (BUG 复现)
--- FAIL: TestProbeVideoBilledThenFailed                 (BUG 复现)
--- FAIL: TestProbeTraceSessionIDAlwaysEmpty             (BUG 复现)
--- FAIL: TestProbeEstimatedCompletionTokenDivergence    (BUG 复现)
--- FAIL: TestProbeCountTokensUnknownModel               (BUG 复现)
--- FAIL: TestProbeOversizedBodyStatus                   (BUG 复现)
--- FAIL: TestProbeUsageLogPersistsUnsanitizedCredential (BUG 复现)
--- FAIL: TestProbeMCPChatBypassesKeyRestrictions        (BUG 复现)
--- FAIL: TestProbeAnthropicToolUseBlockShape            (BUG 复现)
--- FAIL: TestProbeMCPCrossSessionResponseInjection      (BUG 复现)
--- FAIL: TestProbeCompletionsTokenIDPrompt              (BUG 复现)
--- FAIL: TestProbeCompletionsStreamErrorNoDone          (BUG 复现)
```

运行完毕后 worktree 已删除；每条发现的关键证据在下方逐项引用（探针输出原文）。

## 已核对的既有发现（本轮 HEAD）

| 既有编号 | 结论 |
|---|---|
| F05 多模态 Base URL 重复拼接 | 已修复：`TestAuditReviewMultimodalPreservesAPIBasePath` 通过，本 worktree 复跑 OK |
| F06 视频任务查询绑定创建渠道 | 已修复：`TestVideoTaskOwnershipAndPinnedFailure`/`TestAuditReviewVideoPollReturnsToCreatingProvider` 通过；另核对 `dispatcher_modality.go:56-68`：pinned 渠道消失时返回 NoRoute，不会跨供应商回退，不会重演 F06 |
| F16 视频轮询模型参数 | 已修复：`HandleVideoTask` 对 `?model=` 冲突返回 400，模型权限复校 |
| F17 / F11 前端轮询与 SSE 提示 | 前端范围，非本报告 |
| F6 非流式 usage 估算 | 非流式已修（`resolveChatUsage` 等），但**流式提示词估算仍漏算 tools**（见 A03） |
| 客户端断连 499 归类（README 四之三） | chat/completions/responses/anthropic/gemini 流式已修；**多模态链路未修**（见 A01），**MCP 工具链路未覆盖**（见 A04） |
| 按服务模型计费 | `billingModelFor` 正常；但 **MCP `airoute_chat` 绕过 recordUsage**（见 A04） |
| 断连排空（README 四之五） | chat/anthropic/gemini 有排空；**多模态 POST 无排空**（A01），且**排空超时(120s) < 渠道超时(300s)**（A19） |

## 发现总表

| ID | 严重度 | 位置 | 标题 | 已复现 | 影响 |
|---|---|---|---|---|---|
| A01 | P1 | `internal/api/multimodal_handler.go:46-52` | 多模态派发失败一律 502；客户端断连记为上游故障且无排空 | CONFIRMED | 图像/音频/视频的客户端断连污染 5xx，且上游调用被真实取消 |
| A02 | P1 | `internal/api/multimodal_handler.go:57-79` | 视频生成：已按 200 成功计费后才做变换，变换失败返回 502 | CONFIRMED | 客户被扣费却收到 502；审计行写 200 与实际响应不一致 |
| A03 | P1 | `internal/api/chat_stream_handler.go:51-54` | 流式提示词估算漏算 tools/图像，与非流式口径相差千倍 | CONFIRMED | 断连/上游不报 usage 时系统性少计费（实测 1 vs 2525 token） |
| A04 | P1 | `internal/api/mcp_tools_routing.go:146-245` | MCP `airoute_chat` 绕过模型白名单、预算/配额、限流与 usage 估算 | CONFIRMED | 受限/欠费 key 可调用任意模型并产生 0 cost 流水 |
| A05 | P1 | `internal/api/moderation_handler.go:81-118` | moderation 上游失败被伪装成 501「未配置」；成功调用零审计零计费 | CONFIRMED | 客户端无法区分「无渠道」与「上游故障」；真实审核流量不可见 |
| A06 | P1 | `internal/api/common.go:365-391` | `usage_logs.error_message` 持久化未脱敏的上游原文（含 provider key） | CONFIRMED | 上游 401 回显的密钥落库；与响应/access-log 的脱敏策略不一致 |
| A07 | P1 | `internal/api/responses_stream.go:64-96,181-196` | Responses API SSE 事件缺 `item`/`item_id`/`output_index`/`content_index`/`sequence_number` | CONFIRMED | OpenAI 官方 SDK 无法装配事件流，流式 Responses 实际不可用 |
| A08 | P2 | `internal/api/gemini_inbound.go:36,75-163` | `:embedContent` 被 advertise 却按 chat completions 执行 | CONFIRMED | Gemini SDK 的嵌入调用返回 chat 候选，语义错误 |
| A09 | P2 | `internal/api/gemini_inbound.go:21-68` | `/v1beta/models*` 不校验 key 的模型白名单 | CONFIRMED | 与 `/v1/models` 行为不一致，泄露/枚举未授权模型 |
| A10 | P2 | `internal/api/mcp_handler.go:115,150-166` | JSON-RPC notification 返回响应；`initialize` 忽略客户端协议版本 | CONFIRMED | 协议不符（notification 应 202/无响应），旧版客户端握手被拒 |
| A11 | P2 | `internal/api/mcp_sse.go:84-102` | MCP SSE 的 401 JSON 响应带 `Content-Type: text/event-stream` | CONFIRMED | 严格客户端把错误体当事件流解析，握手失败信息不可读 |
| A12 | P2 | `internal/api/trace_hooks.go:75` | 追踪档案的 `session_id`/`chat_id` 恒为空 | CONFIRMED | trace 无法按会话关联，排障能力缺失 |
| A13 | P2 | `internal/api/inbound_anthropic_stream.go:75` | Anthropic 入口估算 completion 用 `/3`，其余入口用 `/4` | CONFIRMED | 同一输出经不同入口计费差 33%（实测 75 vs 100） |
| A14 | P2 | `internal/api/inbound_anthropic.go:234-308` | `count_tokens` 对不存在的模型返回 200 + 本地估算 | CONFIRMED | 应 404，客户端在模型未配置时继续请求 |
| A15 | P2 | `internal/api/handler.go:32-40`（及 5 处同构） | 超限请求体被映射为 400 `invalid_payload` 而非 413 | CONFIRMED | 客户端无法识别 413，错误文案误导（"Invalid JSON"） |
| A16 | P2 | `internal/api/inbound_anthropic_stream.go:325-336` | 适配流式 `tool_use` 的 `content_block_start` 缺必备 `input` 字段 | CONFIRMED | Anthropic SDK 解析工具块失败，Claude 客户端工具调用中断 |
| A17 | P2 | `internal/api/mcp_handler.go:72-113` | `POST /mcp/messages` 未校验会话归属，可向他人 SSE 会话注入响应 | CONFIRMED | 跨租户会话内容注入（agent 上下文污染） |
| A18 | P2 | `internal/api/session.go:47-73,131-146` | 会话亲和键取自未认证的 header/`user` 字段，Redis 键不含租户 | STATIC | 跨租户渠道钉选/投毒（30 分钟 TTL） |
| A19 | P2 | `internal/api/upstream_lifecycle.go:17,37-47` | 排空超时 120s 小于渠道超时 300s | STATIC | 慢渠道上断连请求无法拿到上游 usage，「逐单等于账单」退化为估算 |
| A20 | P2 | `internal/api/completions_handler.go:86-98`、`completions_stream_handler.go:110-153` | 旧版 `/v1/completions`：token-id prompt 被转成文本；流式错误后无 `[DONE]` | CONFIRMED | 语义错误 + 客户端等待终止符直至连接关闭 |

---

## 详细说明

### A01 · P1 · 多模态派发失败一律 502，客户端断连记为上游故障且无排空

**位置**：[multimodal_handler.go:46](/home/ifnodoraemon/myagent/airoute/internal/api/multimodal_handler.go#L46-L52)

**触发**：图像/语音/视频请求在上游返回前客户端断开（`curl -m 0.3`、移动网络掉线、前端 AbortController）。

**代码路径**：`dispatchAndTransform` 用 `c.Request.Context()`（未脱离下游）派发；`DispatchHTTP` 返回 `context canceled` 后，第 50-51 行**硬编码** `http.StatusBadGateway` 记录与响应，完全不走 `resolveUpstreamFailureStatus`（chat/embeddings/rerank/gemini/anthropic 都走它）。同一仓库已把该场景定义为 499 `client_closed_request`（README 四之三、迁移 v24）。

**证据**（探针 `TestProbeMultimodalClientCancelStatus`，image generations，派发中取消客户端）：

```
downstream status=502 body={"error":"context canceled"}
audit row status=502 channel="img" error="context canceled"
BUG: client cancel recorded as 502 (want 499)
```

**修复建议**：改为 `resolveUpstreamFailureStatus(err)` 得到状态与错误码；用 `upstreamRequestContext(c)` 派发（与 chat 一致，支持 `GATEWAY_DRAIN_ABORTED_UPSTREAM` 排空），否则上游可能已完成生成却被我们取消、既拿不到账单也无法计费。

### A02 · P1 · 视频生成先计费、后变换：变换失败时客户被扣费却收到 502

**位置**：[multimodal_handler.go:57-79](/home/ifnodoraemon/myagent/airoute/internal/api/multimodal_handler.go#L57-L79)、[video_handler.go:128-146](/home/ifnodoraemon/myagent/airoute/internal/api/video_handler.go#L128-L146)

**触发**：上游 2xx，但响应体不是 JSON、或超过 4 MiB、或 `SaveVideoTask` 失败（数据库抖动）。

**代码路径**：第 58-66 行先 `RecordUsage(StatusCode: resp.StatusCode=200)` → 立即同步结算（`SettleChargeNow`）并写审计行；随后第 75-79 行才执行 `transform`，失败时返回 502 `video_task_error`。审计行已定格为 200 成功，且没有第二条失败记录。

**证据**（探针 `TestProbeVideoBilledThenFailed`，上游返回 `this is not json`，固定价 ¥0.5）：

```
downstream status=502 body={"error":{"code":"video_task_error",...}}
audit row: status=200 cost=0.5 tokens=0/0 source="upstream"
BUG: the request was billed as a 200 success (cost=0.5) but the client received 502
```

**修复建议**：把 usage/结算挪到 transform 成功之后（或先只做审计、待响应写出成功再结算）；transform 失败时用 502 状态补记/更正审计，避免「扣费成功 + 客户端失败」的账实不符。

### A03 · P1 · 流式提示词估算漏算 tools/图像，与非流式口径差三个数量级

**位置**：[chat_stream_handler.go:51-54](/home/ifnodoraemon/myagent/airoute/internal/api/chat_stream_handler.go#L51-L54)（使用点 68-82）

**触发**：`stream=true` 且上游未回报 usage（自建/兼容上游常见），或客户端在 usage 帧到达前断开。

**代码路径**：`approxPromptChars` 只累加 `messages[].content`；`estimateCanonicalPromptTokens`（非流式 `resolveChatUsage` 使用）**还累加 `req.Tools` 的 JSON**。同一请求 stream 与非 stream 走的估算函数不同，工具定义（常见数 KB）在流式路径完全不计费；图像/多模态 content part 同理。

**证据**（探针 `TestProbeStreamPromptEstimateIgnoresTools`，10KB 工具 schema + 2 字符消息）：

```
stream audit prompt_tokens=1 | unary resolveChatUsage=2525 | estimateCanonicalPromptTokens=2525 | usage_source="estimated"
BUG: stream==true bills 1 prompt tokens for a request the unary path bills 2525 (tools ignored)
```

**修复建议**：流式路径直接复用 `estimateCanonicalPromptTokens(req)`（含 tools、content parts），并同样喂给 `observePromptEstimate` 做校准；四个流式处理器（chat/responses/anthropic/gemini）统一。

### A04 · P1 · MCP `airoute_chat` 绕过模型白名单、预算/配额/限流与 usage 估算

**位置**：[mcp_tools_routing.go:146-245](/home/ifnodoraemon/myagent/airoute/internal/api/mcp_tools_routing.go#L146-L245)

**触发**：任何持有有效 API key 的 MCP 客户端执行 `tools/call` → `airoute_chat`。

**代码路径**：第 155-171 行只校验 key 是否存在且 `status=="active"`；**没有** `middleware.ValidateModelAllowed`（模型白名单）、没有 budget/钱包余额校验（`middleware/auth.go:135-250` 在数据面做的检查）、没有 RPM/TPM 消耗；第 190 行直接 `dispatcher.Dispatch`。第 216-222 行只在 `resp.Usage != nil` 时取 token，**无估算兜底**，故上游省略 usage 时记 0 token / 0 cost。整个路径绕开 `recordUsage`：无 served-model 计费、无 `usage_source`、无 pricing snapshot。此外 `/mcp/*` 路由未挂 `RateLimitMiddleware`（[router.go:93-98](/home/ifnodoraemon/myagent/airoute/internal/api/router.go#L93-L98)）。

**证据**（探针 `TestProbeMCPChatBypassesKeyRestrictions`，key 仅允许 `cheap-model` 且 budget 已超额，上游故意不返回 usage）：

```
isErr=false result="expensive output"
audit row: status=200 model=expensive-model prompt=0 completion=0 cost=0 source="upstream"
BUG: airoute_chat ran 'expensive-model' with a key restricted to ['cheap-model'] and an exhausted budget
BUG: MCP chat row has zero tokens/cost when the upstream omits usage (no estimate fallback)
```

**修复建议**：把 key 记录绑定到 ctx 并复用数据面的校验（`ValidateModelAllowed` + budget/余额 + `GlobalTokenQuota`），改用 `resolveChatUsage` + `recordUsage`（自动带 served-model、估算与结算），MCP 路由组补挂 Auth/RateLimit 或显式限流。

### A05 · P1 · moderation 上游失败伪装成 501「未配置」，成功调用零审计零计费

**位置**：[moderation_handler.go:81-118](/home/ifnodoraemon/myagent/airoute/internal/api/moderation_handler.go#L81-L118)

**触发**：(a) 配置了 moderation 渠道但上游返回 5xx/429；(b) 上游返回 200。

**代码路径**：第 92 行仅当 `err == nil && resp.StatusCode == 200` 才转发；否则**静默丢弃**上游状态与错误体，继续落到第 103-118 行的 501 `moderation_not_configured`（或 stub）。成功路径第 94-95 行直接 `c.Data`，**没有 `RecordUsage`**：无审计行、无计费、无 usage_source，事后无法对账「审核调用量」。

**证据**（探针 `TestProbeModerationMasksUpstreamError`）：

```
upstream=500 -> downstream status=501 body={"error":{"code":"moderation_not_configured",...}}
BUG: upstream 500 surfaced to the client as 501 moderation_not_configured
upstream=200 -> downstream=200, audit rows=0
NOTE: 0 audit rows for 2 moderation calls
```

**修复建议**：有渠道时把上游状态/错误码透传（用 `resolveUpstreamFailureStatus`），仅「无候选渠道」才 501；成功路径补 `RecordUsage`（prompt 估算 + 按次 `BillableUnits`），否则该接口是审计与计费盲区。

### A06 · P1 · `usage_logs.error_message` 持久化未脱敏的上游原文（含 provider key）

**位置**：[common.go:365-391](/home/ifnodoraemon/myagent/airoute/internal/api/common.go#L365-L391)（写入点 [common.go:193](/home/ifnodoraemon/myagent/airoute/internal/api/common.go#L193)）；同类：[trace_hooks.go:81](/home/ifnodoraemon/myagent/airoute/internal/api/trace_hooks.go#L81)

**触发**：上游错误回显凭证（provider 401/400 常回显 `Incorrect API key provided: sk-...`），或上游把请求体原样回显。

**代码路径**：`recordFailedRequestWithError` 取 `msg = err.Error()` 原文写入 `UsageLogRecord.ErrorMessage`；只有响应体（`respondUpstreamDispatchError` 的 `safeErr`）与 access log（第 219 行 `sanitizeErrorMessage`）做了脱敏。trace 档案同样写原文。`common.go:551-562` 的注释本身承认上游会回显凭证。

**证据**（探针 `TestProbeUsageLogPersistsUnsanitizedCredential`）：

```
usage_logs.error_message = "upstream openai returned status 401: {\"error\":{\"message\":\"Incorrect API key provided: sk-live-abcdef1234567890\"}}"
access-log copy (sanitized) = "... sk-[REDACTED] ..."
BUG: raw provider key persisted into usage_logs.error_message
```

**修复建议**：写审计/trace 前统一过 `sanitizeErrorMessage`（数据库存储侧脱敏，而不是仅在出口脱敏），并对历史行做一次清洗迁移。

### A07 · P1 · Responses API SSE 事件缺必备字段，官方 SDK 无法装配

**位置**：[responses_stream.go:64-96](/home/ifnodoraemon/myagent/airoute/internal/api/responses_stream.go#L64-L96)、[responses_stream.go:181-196](/home/ifnodoraemon/myagent/airoute/internal/api/responses_stream.go#L181-L196)

**触发**：`POST /v1/responses` 且 `stream=true`，客户端为 openai-python / openai-node / Vercel AI SDK。

**代码路径**：事件体只有 `type` 与业务字段。`response.output_item.added` 用 `output_item` 键（规范为 `item`），且缺 `output_index`/`sequence_number`；`response.content_part.added` 缺 `item_id`/`output_index`/`content_index`；`response.output_text.delta` 只有 `delta`，缺 `item_id`/`output_index`/`content_index`/`sequence_number`（这些在官方 SDK 的事件模型里是必填）。

**证据**（探针 `TestProbeResponsesStreamEventShape`，逐事件打印）：

```
event=response.output_item.added   data={"output_item":{"content":[],"id":"msg_...","role":"assistant",...},"type":"response.output_item.added"}
event=response.content_part.added  data={"part":{"text":"","type":"output_text"},"type":"response.content_part.added"}
event=response.output_text.delta    data={"delta":"hel","type":"response.output_text.delta"}
BUG: response.output_item.added has no 'item' field (got keys [output_item type])
BUG: response.output_text.delta lacks required field "item_id" / "output_index" / "content_index" / "sequence_number"
```

**修复建议**：按官方事件 schema 补齐 `item`/`item_id`/`output_index`/`content_index`/`sequence_number`（单调递增），并让 `response.completed.output[].content` 与新事件字段一致；用官方 SDK 做一次流式回归。

### A08 · P2 · `:embedContent` 被宣称支持却按 chat completions 执行

**位置**：[gemini_inbound.go:36](/home/ifnodoraemon/myagent/airoute/internal/api/gemini_inbound.go#L36)、[gemini_inbound.go:75-140](/home/ifnodoraemon/myagent/airoute/internal/api/gemini_inbound.go#L75-L140)

**触发**：Gemini SDK `models.embed_content()` → `POST /v1beta/models/<m>:embedContent`。

**代码路径**：`/v1beta/models` 与模型详情把 `embedContent` 列入 `supportedGenerationMethods`；但 `HandleGeminiAction` 只特判 `countTokens`，其余 action 一律 `convertInboundGeminiToCanonical` 后走 chat 派发。

**证据**（探针 `TestProbeGeminiEmbedContentIsChatCompletion`）：

```
:embedContent -> upstream path="/chat/completions" downstream=200 body={"candidates":[...]}
models list advertises: "supportedGenerationMethods":["generateContent","countTokens","embedContent"]
BUG: embedContent was dispatched as a chat completion (/chat/completions)
```

**修复建议**：要么实现 `:embedContent`（转 `/v1/embeddings` 并回填 Gemini embedding 结构），要么从 `supportedGenerationMethods` 移除并对该 action 返回 400/501；不要静默当 chat 处理。

### A09 · P2 · `/v1beta/models*` 不校验 key 的模型白名单

**位置**：[gemini_inbound.go:21-43](/home/ifnodoraemon/myagent/airoute/internal/api/gemini_inbound.go#L21-L43)、[gemini_inbound.go:46-68](/home/ifnodoraemon/myagent/airoute/internal/api/gemini_inbound.go#L46-L68)

**触发**：受限 key（`allowed_models` 非空）请求 `/v1beta/models` 或 `/v1beta/models/<未授权模型>`。

**代码路径**：`HandleGeminiModels` 遍历 `GetAllSupportedModels()` 直接输出；`HandleGeminiModelDetail` 用 `ListChannelsForModel` 判存在。对比 `HandleModels`/`HandleModelDetail`（`models_handler.go:21,52`）都调用 `middleware.ValidateModelAllowed`。

**证据**（探针 `TestProbeGeminiModelsBypassAllowlist`，key 仅允许 `allowed-model`）：

```
/v1/models      : {"object":"list","data":[{"id":"allowed-model",...}]}
/v1beta/models  : models/allowed-model, models/secret-model      <-- 泄露
/v1beta/models/secret-model : status=200
BUG: /v1beta/models leaks models outside the key allowlist
```

**修复建议**：两个 Gemini 列表/详情端点同样调用 `middleware.ValidateModelAllowed`（详情对未授权模型返回 403，与 `/v1/models` 保持一致）。

### A10 · P2 · JSON-RPC notification 返回响应；`initialize` 忽略客户端协议版本

**位置**：[mcp_handler.go:115](/home/ifnodoraemon/myagent/airoute/internal/api/mcp_handler.go#L115)、[mcp_handler.go:150-166](/home/ifnodoraemon/myagent/airoute/internal/api/mcp_handler.go#L150-L166)

**触发**：(a) 客户端发送 `notifications/initialized` / `notifications/cancelled`（无 `id`）；(b) 客户端 `initialize` 携带 `protocolVersion: "2024-11-05"`。

**代码路径**：`ProcessRPC` 对 notification 返回 `newMCPResultResponse(req.ID, ...)`（第 165-166 行），`HandleMCPMessages` 一律 `HTTP 200 + JSON`；响应头第 115 行固定 `MCP-Protocol-Version: 2026-07-28`，`initialize` 结果也固定回 `2026-07-28`，不读客户端请求版本。

**证据**（探针 `TestProbeMCPNotificationGetsResponse` / `TestProbeMCPInitializeVersionNegotiation`）：

```
notifications/initialized -> status=200 body={"jsonrpc":"2.0","result":{}}
notifications/cancelled   -> status=200 body={"jsonrpc":"2.0","result":{}}
BUG: notification notifications/initialized received a JSON-RPC response
requested=2024-11-05, answered="2026-07-28", response MCP-Protocol-Version header="2026-07-28"
```

**修复建议**：notification（无 `id`）返回 `202 Accepted` 且空 body（Streamable HTTP 语义）；`initialize` 按客户端请求版本协商——支持则原样回，不支持则回自己支持的最高版本，并让 `MCP-Protocol-Version` 响应头等于协商结果。

### A11 · P2 · MCP SSE 的 401 JSON 响应带 `Content-Type: text/event-stream`

**位置**：[mcp_sse.go:84-102](/home/ifnodoraemon/myagent/airoute/internal/api/mcp_sse.go#L84-L102)

**触发**：仓库已存在 API key，客户端 `GET /mcp/sse` 未带 key（或 key 无效）。

**代码路径**：第 84-88 行先设置 SSE 响应头，第 90-102 行才鉴权并用 `c.JSON(401, ...)` 写 JSON；gin 的 `render.writeContentType` 仅在 `Content-Type` 为空时才覆盖，因此 401 体沿用了 `text/event-stream`。

**证据**（探针 `TestProbeMCPSSEUnauthorizedContentType`）：

```
status=401 content-type="text/event-stream" body={"error":"Authentication required for MCP SSE connection: ..."}
BUG: 401 JSON body is sent with Content-Type: text/event-stream
```

**修复建议**：先鉴权、后写 SSE 头；或鉴权失败时显式 `c.Header("Content-Type", "application/json")` 覆盖。

### A12 · P2 · 追踪档案的 `session_id`/`chat_id` 恒为空

**位置**：[trace_hooks.go:75](/home/ifnodoraemon/myagent/airoute/internal/api/trace_hooks.go#L75)（第 81 行同理）

**触发**：任意失败请求被归档（`GATEWAY_TRACE_CAPTURE=errors`，默认值即 errors）。

**代码路径**：`RecordTrace` 从 **gin context** 取 `c.GetString(router.ContextKeySessionID)` 与 `c.GetString("chat_id")`；但数据面各 handler 只把 sessionID 放进**派发 ctx**（`context.WithValue(reqCtx, ...)`，见 handler.go:89 等），从不对 gin context `c.Set`；`RecordTraceFromError`（唯一会 `c.Set` 的函数）在 `internal/api` 内无调用者。

**证据**（探针 `TestProbeTraceSessionIDAlwaysEmpty`，`respondUpstreamDispatchError(c, "sess-probe-123", ...)`）：

```
trace row: session_id="" chat_id="" status=404
BUG: trace session_id="" (handler passed "sess-probe-123")
```

**修复建议**：在 `RecordTrace` 前 `c.Set(router.ContextKeySessionID, sessionID)`，或让 `respondUpstreamDispatchError` 接受并转发 sessionID；chat_id 同理在拿到上游 ID 后写入。

### A13 · P2 · Anthropic 入口估算 completion 用 `/3`，其余入口用 `/4`

**位置**：[inbound_anthropic_stream.go:70-81](/home/ifnodoraemon/myagent/airoute/internal/api/inbound_anthropic_stream.go#L70-L81)（第 75 行 `accumulatedCompChars / 3`）

**触发**：`/v1/messages` 流式且上游未报 usage（或 usage 帧到达前结束）。

**代码路径**：`finalizeCompletion` 用 `/3`，而 `common.go` 明确要求「单一口径」`estimateTokens`（`model.EstimateTokensFromChars` ≈ `/4`）。`recordStreamEnd` 里 `totalCompTokens==0` 分支才用共享函数，但 `finalizeCompletion` 已先把值写成 `/3`，导致该分支不触发。

**证据**（探针 `TestProbeEstimatedCompletionTokenDivergence`，同一上游、同为 300 字符输出）：

```
session=s2 completion=100  (Anthropic 入口)
session=s1 completion=75   (OpenAI 入口)
shared estimator for 300 chars = 75
BUG: identical 300-char output is billed 75 completion tokens via OpenAI ingress and 100 via Anthropic ingress
```

**修复建议**：删除本地 `/3`，统一 `estimateTokens(accumulatedCompChars)`（与 chat/responses/completions 一致）。

### A14 · P2 · `count_tokens` 对不存在的模型返回 200 + 本地估算

**位置**：[inbound_anthropic.go:234-308](/home/ifnodoraemon/myagent/airoute/internal/api/inbound_anthropic.go#L234-L308)（第 300-308 行的 `lastErr == nil` 分支）

**触发**：`POST /v1/messages/count_tokens`，`model` 未配置任何渠道。

**代码路径**：`ResolveCandidateChannels` 返回空 → 循环不执行 → `lastErr == nil` → 走「无 native 渠道则本地估算」分支，返回 200 + `X-Airoute-Token-Estimate`。该分支的本意是「模型走 OpenAI/Gemini 兼容上游」，但无法区分「模型根本不存在」。同一文件另有隐患：该接口用 `c.Request.Context()` 与无超时的共享 client（第 261-275 行），不受渠道超时/熔断约束。

**证据**（探针 `TestProbeCountTokensUnknownModel`）：

```
model=ghost-model -> status=200 body={"input_tokens":2} est-header="true"
BUG: count_tokens for a nonexistent model returned 200 (want 404 model_not_found)
```

**修复建议**：候选渠道为空时返回 404 `model_not_found`；仅当存在非 native 候选时才走本地估算，并给该上游调用加独立 timeout。

### A15 · P2 · 超限请求体被映射为 400 `invalid_payload` 而非 413

**位置**：[handler.go:32-40](/home/ifnodoraemon/myagent/airoute/internal/api/handler.go#L32-L40)（同构：completions_handler.go:50、responses_handler.go:19、embedding_handler.go:16、rerank_handler.go:16、inbound_anthropic.go:46）

**触发**：请求体超过 `GATEWAY_MAX_BODY_BYTES`（默认 64 MiB，或网关/代理更低配置）。

**代码路径**：`middleware.BodyLimit` 用 `http.MaxBytesReader` 包裹；`ShouldBindJSON` 返回 `http: request body too large`，各 handler 一律映射为 400 + `invalid_payload` + 「Invalid JSON request body: ...」。

**证据**（探针 `TestProbeOversizedBodyStatus`，按 BodyLimit 方式包裹 64 字节上限）：

```
oversized body -> status=400 body={"error":{"code":"invalid_payload","message":"Invalid JSON request body: http: request body too large",...}}
BUG: request-body-too-large is mapped to 400 invalid_payload instead of 413
```

**修复建议**：`errors.As`/`errors.Is` 识别 `*http.MaxBytesError` → 413 `payload_too_large`；其余解析错误保留 400。

### A16 · P2 · 适配流式 `tool_use` 的 `content_block_start` 缺必备 `input`

**位置**：[inbound_anthropic_stream.go:325-336](/home/ifnodoraemon/myagent/airoute/internal/api/inbound_anthropic_stream.go#L325-L336)

**触发**：OpenAI 兼容上游返回 `tool_calls`，客户端使用 Anthropic SDK 流式接口（工具调用场景）。

**代码路径**：`content_block_start` 只写 `type/id/name`，未写 `input: {}`（Anthropic 流式规范里 tool_use 块启动即带空 input，随后由 `input_json_delta` 累积）。

**证据**（探针 `TestProbeAnthropicToolUseBlockShape`，上游返回单个工具调用）：

```
event: content_block_start
data: {"content_block":{"id":"call_a","name":"get_weather","type":"tool_use"},"index":1,"type":"content_block_start"}
tool_use content_block_start index=1 keys=[id name type]
BUG: tool_use content_block_start has no "input" field (Anthropic SDK requires it)
```

**修复建议**：tool_use 块补 `"input": {}`；同一处也应处理「仅 args、无 id/name」的续帧（当前会写到上一个块的 index）。

### A17 · P2 · `POST /mcp/messages` 未校验会话归属，可向他人 SSE 会话注入响应

**位置**：[mcp_handler.go:72-113](/home/ifnodoraemon/myagent/airoute/internal/api/mcp_handler.go#L72-L113)（第 105-113 行）

**触发**：攻击者持有任一有效 key，并获知/猜到（或经日志泄露）他人的 `sessionId`（64-bit 随机，作为 query 参数出现在每次 POST URL 中）。

**代码路径**：第 72-79 行若 session 存在则借用其 `sess.apiKey` 做鉴权；第 104-113 行把本次响应 `sess.Send()` 推入该 session 的 SSE 通道，**从不比较 `sess.apiKey` 与调用者 key**。同一函数也未校验调用者是否有权使用该会话。

**证据**（探针 `TestProbeMCPCrossSessionResponseInjection`，攻击者 key `sk-attacker`，受害者会话 `sk-victim`）：

```
attacker POST status=200 body={"jsonrpc":"2.0","id":7,"result":{}}
BUG: response injected into another client's SSE session: {"jsonrpc":"2.0","id":7,"result":{}}
```

**修复建议**：`sessionId` 与调用者 key 强绑定——POST 时要求携带的 key 与 `sess.apiKey` 一致（或使用一次性、绑定身份的会话令牌），不一致返回 403。

### A18 · P2 · 会话亲和键取自未认证 header，Redis 键不含租户（STATIC）

**位置**：[session.go:47-73](/home/ifnodoraemon/myagent/airoute/internal/api/session.go#L47-L73)、[session.go:131-146](/home/ifnodoraemon/myagent/airoute/internal/api/session.go#L131-L146)；消费方 [load_balancer.go:31-38](/home/ifnodoraemon/myagent/airoute/internal/router/load_balancer.go#L31-L38)

**触发**：客户端任选 `X-Session-ID`/`X-Conversation-ID`/`Session-Id`/`Conversation-Id`/`?session_id=`/cookie，或把 body 里的 `user` 字段留作身份标识（很多应用填邮箱/用户 ID）。

**代码路径**：`extractSessionID` 优先取这些**客户端可控**值；`resolveSessionID` 把它写进派发 ctx；`SessionAffinitySelector` 以 `(modelName, sessionID)` 从 Redis 读写 30 分钟 pin（键格式 `airoute:session:<model>:<sessionID>`，[redis.go:219](/home/ifnodoraemon/myagent/airoute/internal/distributed/redis.go#L219)/[redis.go:232](/home/ifnodoraemon/myagent/airoute/internal/distributed/redis.go#L232)），**不含租户/API key 维度**。因此同一 sessionID 的任意租户请求会被钉到同一渠道；攻击者可预先为可预测的 sessionID（如 `user=alice@example.com`）种下 pin，把他人流量导向指定上游（成本、限速、缓存侧写）。此外 [load_balancer.go:76-80](/home/ifnodoraemon/myagent/airoute/internal/router/load_balancer.go#L76-L80) 的 `SetSessionChannel` 用 `context.Background()` 在 goroutine 里异步写，无超时。

**修复建议**：亲和键加入租户/API key 前缀（`tenant:session`），或只对高熵的 `X-Session-ID` 生效；`user` 字段不应作为亲和键；写 Redis 的 goroutine 加超时。

### A19 · P2 · 排空超时 120s 小于渠道超时 300s（STATIC）

**位置**：[upstream_lifecycle.go:14-18](/home/ifnodoraemon/myagent/airoute/internal/api/upstream_lifecycle.go#L14-L18)、[upstream_lifecycle.go:36-47](/home/ifnodoraemon/myagent/airoute/internal/api/upstream_lifecycle.go#L36-L47)；配置 `.env:67-69`；渠道超时口径见 [docs/audit/README.md:341](/home/ifnodoraemon/myagent/airoute/docs/audit/README.md#L341)

**触发**：客户端断开后，上游总耗时落在 120s~300s（长文推理、慢渠道）。

**代码路径**：`upstreamRequestContext` 用 `context.WithTimeout(WithoutCancel(...), upstreamDrainTimeout())`，默认 120s 且 `.env` 显式 `GATEWAY_DRAIN_TIMEOUT_SEC=120`；而「唯一旋钮」渠道超时按 README 为 300s。排空在 120s 被切断 → 拿不到最终 usage → 退回估算，README 四之五「逐单等于上游账单」在慢请求上失效（上游仍在计费）。

**修复建议**：排空超时自动派生为 `max(channel_timeout, GATEWAY_DRAIN_TIMEOUT_SEC)`（或默认跟随渠道超时 + 小余量），启动自检校验 `drain >= channel_timeout`。

### A20 · P2 · 旧版 `/v1/completions`：token-id prompt 被转成文本；流式错误后无 `[DONE]`

**位置**：[completions_handler.go:86-98](/home/ifnodoraemon/myagent/airoute/internal/api/completions_handler.go#L86-L98)、[completions_stream_handler.go:110-153](/home/ifnodoraemon/myagent/airoute/internal/api/completions_stream_handler.go#L110-L153)

**触发**：(a) `{"prompt":[1,2,3]}`（OpenAI 合法的 token-id 数组提示）；(b) 流式上游中途报错。

**代码路径**：(a) `switch` 只处理 `string`/`[]any`/`[]string`，`[]any` 分支对每个元素 `fmt.Sprint` 并用 `\n` 拼接 → token 序列被当作十进制文本提示发给上游。(b) 错误分支 `break` 后仅当 `statusCode == 200`（或 `sawDone` 为真）才写 `[DONE]`；错误场景 `sawDone=false`，因此没有终止符，客户端只能等连接关闭。

**证据**（探针 `TestProbeCompletionsTokenIDPrompt` / `TestProbeCompletionsStreamErrorNoDone`）：

```
downstream=200 upstream body={"model":"davinci","messages":[{"role":"user","content":"1\n2\n3"}]}
BUG: token-id prompt [1,2,3] forwarded as the text "1\n2\n3"
data: {"error":{"message":"upstream stream error []: upstream overloaded","type":"stream_error"}}
BUG: error-terminated completions stream received no [DONE] sentinel
```

**修复建议**：识别 `[]int`/`[]float64` 为 token ids（透传或在协议不支持时明确 400），并把 `[DONE]` 写到所有终态分支（与 chat 的 error 分支保持一致）。

---

## 备注

- 未覆盖：真实 PostgreSQL/Redis 集成、真实上游（收费模型）联调、MCP stdio/远程服务器端到端；`mcp_tools_security.go`/`mcp_sql_guard.go` 归 auth reviewer，本轮未评审 SQL 守卫细节；定价/结算内部实现归 billing reviewer，本轮仅评审调用点。
- 本轮未修改任何跟踪源码或 `.env`，未重启/重建服务；所有探针位于一次性 worktree 并已删除。
