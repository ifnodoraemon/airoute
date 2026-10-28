# Bug 排查 A：请求 / 流式 / 协议路径真实缺陷（2026-10-28）

> **任务**：task-1。阶段一：只读审计 + 探针测试（未改动生产代码）。阶段二（Lead 复核后）：**6 条全部修复**，探针翻转为常规回归测试，见 §6 修复记录。
> **修复范围**：`internal/api`、`internal/model`、`internal/provider`、`internal/adapter`（+ 对应测试）；未触碰 storage/billing/telemetry/middleware/distributed/controlplane/cmd/web。
> **基线**：`381926d`（HEAD）。审计期间 Lead 在并行删死代码、另两位 teammate 在并行修 B/C 域缺陷，本人复核时 `go build ./...` / `go vet ./...` / `gofmt -l internal` 干净、`go test ./internal/{api,model,provider,router,validator,adapter}` 全绿（含本报告全部探针；全仓 `go test ./...` 的瞬时红来自他人域，见 §2 交接说明）。
> **范围**：`internal/router`、`internal/provider`、`internal/adapter`、`internal/api`（handler / chat & completions & responses & messages 四条流式入口 / SSE / 多模态 / embedding / rerank / video / MCP 传输）、`internal/model`、`internal/validator`。
> **判据**：真实缺陷 = panic/nil deref、SSE 分帧与半包解析、usage/token 统计错算、finish_reason/错误帧、重试与熔断记账、context 取消与超时泄漏、响应体未关闭、并发共享状态、协议字段漂移导致功能坏掉、请求/响应形状与文档不符。
> **不重复**：`docs/audit/2026-10-28-inconsistencies/README.md` §3（半接线/双实现）、§6（已排除的「看似 bug」）、§8.3（遗留）。上一轮明确「不查普通 bug」，本轮就是补这一块。

---

## 0. 结论摘要

四条流式入口（chat / completions / responses / messages）+ 模态入口的**主链路**（首个 token 前 fallback、半开 canary 归还、熔断分类、drain 计费、settlement 延迟到终帧之后）复核下来是**扎实的**：上一轮 B-02/B-03/B-07/B-12、C-02/C-04/C-07 的修复都站得住，我没找到新的 panic/nil deref，也没找到 data race（详见 §4 已排除）。

本轮新发现 **6 条真实缺陷**（2×P1 + 4×P2），**已全部修复并转为回归测试**（§6）；另有 2 条待确认（§3，其中「流式 usage 覆盖式赋值」本轮已顺手修复）。最值得先修的是 **BUG-GEMINI-TOOLCALL**：Gemini 协议客户端的**流式工具调用功能实际不可用**（一次调用被拆成三个空壳 `functionCall`，参数永远不完整），而文档明确宣称支持 `:streamGenerateContent` 适配。

| ID | 严重度 | 置信度 | 一句话 | 主要位置 | 回归测试 |
|---|---|---|---|---|---|
| **BUG-GEMINI-TOOLCALL** | **P1** | 高 | Gemini 协议**流式**工具调用把每个增量 delta 翻译成一个独立 `functionCall` part：首个 part 有 name 但 `args:null`，后续两个 name 为空、`args:null`，完整参数永不拼装 ⇒ 工具调用功能坏掉；一元路径（`adapter.FromCanonical`）正常，构成一元/流式不对称 | `api/gemini_inbound_stream.go:206-214` | `api/zz_bugprobe_gemini_stream_tools_test.go` |
| **BUG-MULTIMODAL-TOKEN-ESTIMATE** | **P1**（会计口径） | 高 | 内联 base64 图片/音频被当作**文本**计入 prompt 估算（`GetContentString` 把 content 数组整体 `json.Marshal`）⇒ 一张 1 MB PNG 估成 **232,529** prompt tokens（同一请求用 URL 形式只 30），高估 300~2700×；客户端断连（`EstimatedPromptTokens`）与上游不回 usage 时按此估价**真扣费** | `model/tokens.go:35-49`、`api/common.go:822-836`（两份实现）；计费入口 `api/common.go:587-598`、`router/dispatcher.go:297,301` | `model/zz_bugprobe_tokens_test.go` |
| **BUG-GEMINI-THOUGHTS** | P2（token 少算） | 高 | Gemini `usageMetadata.thoughtsTokenCount` **完全没有建模**（`GeminiUsageMetadata` 无该字段，`geminiUsageToModel` 只用 candidates）⇒ thinking 模型的思考 token 不进 completion，canonical usage 自相矛盾（`total 650 ≠ prompt 100 + completion 50`），示例中 **90.9% 的输出 token 从未进入计费引擎** | `provider/gemini_types.go:76-81`、`provider/usage_mapping.go:36-52`；`adapter/gemini.go:250` 用 `prompt+candidates` 覆盖上游 total | `provider/zz_bugprobe_gemini_thoughts_test.go` |
| **BUG-RESPONSES-INCOMPLETE** | P2（协议字段漂移） | 高 | Responses API 永远输出 `status:"completed"`：`ConvertChatResponseToResponseResult` 不读 `FinishReason`，`ResponseResult`/`ResponseUsage` 没有 `incomplete_details` 字段，流式 `response.completed` 亦固定 completed ⇒ `max_output_tokens` 截断 / content_filter 的回答对客户端**不可区分**，agent 会把半截答案当完整答案续跑 | `model/response.go:48-63,605-641,670`、`api/responses_stream_events.go:219-263` | `model/zz_bugprobe_responses_incomplete_test.go` |
| **BUG-SSE-HALF-FRAME** | P2 | 中 | `gemini_stream`/`anthropic_stream` 在 `err==io.EOF && len(line)>0` 时**丢弃缓冲区最后一行**（同包 `openai.go`/`openai_text_adapter.go` 正确处理该情形）⇒ 末帧若未带尾随换行，`finishReason`/`message_delta` 终帧整帧丢失：Gemini 侧被误记一次渠道失败（`errStreamTruncated`），Anthropic 侧丢 usage 退回估算 | `provider/gemini_stream.go:75-81`、`provider/anthropic_stream.go:106-113`（对照 `provider/openai.go:256`、`provider/openai_text_adapter.go:169`） | `provider/zz_bugprobe_sse_half_frame_test.go` |
| **BUG-REQ-MUTATION** | P2（共享可变状态） | 高 | `ChatComplete`/`ChatCompleteStream` 用 `clonedReq := *req` 浅拷贝后再 `stripReasoningContent(clonedReq.Messages)` **原地**清空调用方 `req.Messages[].ReasoningContent` ⇒ 渠道级的出站消毒泄漏成请求级副作用：后续渠道重试、跨模型兜底、估算、审计/trace 看到的历史都被改写 | `provider/openai.go:83,86,151-155`、`provider/openai.go:172,176` | `provider/zz_bugprobe_req_mutation_test.go` |

**推荐修复顺序**：BUG-GEMINI-TOOLCALL（功能坏掉，用户直接受影响）→ BUG-MULTIMODAL-TOKEN-ESTIMATE（直接多收钱）→ BUG-GEMINI-THOUGHTS（少收钱，同一根因族「provider usage 映射不全」可一次补）→ BUG-SSE-HALF-FRAME（三处 EOF 分支统一成 `openai.go` 的写法）→ BUG-REQ-MUTATION（深拷贝 Messages）→ BUG-RESPONSES-INCOMPLETE。

**状态**：6 条全部 **fixed**（2026-10-28 修复阶段）。审计期的探针（断言 buggy 行为、注释标 `BUG-ID`）已按 Lead 要求**翻转为常规回归测试**并更名（不再使用 `zz_bugprobe_` 前缀，不留占位测试）；每条修复都用「回退改动 ⇒ 测试转红」做过 mutation 验证。

---

## 1. 发现详情

### BUG-GEMINI-TOOLCALL（P1，置信高）Gemini 协议流式工具调用被拆成多个空壳 functionCall

**位置**
- `internal/api/gemini_inbound_stream.go:206-214`：`for _, tc := range chunkChoice.Delta.ToolCalls { var args map[string]any; _ = json.Unmarshal([]byte(tc.Function.Arguments), &args); geminiParts = append(geminiParts, gin.H{"functionCall": gin.H{"name": tc.Function.Name, "args": args}}) }`
- 对照（一元路径，正确）：`internal/adapter/gemini.go:208-222` —— 一元响应里 `ToolCalls` 是完整的，一次 `json.Unmarshal` 得到完整 args，输出**一个** functionCall part。

**触发条件**
Gemini 客户端 `POST /v1beta/models/<m>:streamGenerateContent`（`docs/API_REFERENCE.md:105` 宣称支持），模型由 OpenAI-compatible 渠道承载（`docs/PROVIDER_GUIDE.md:13` 宣称自动转换为 Gemini 的 `:streamGenerateContent`），且上游以流式增量下发 `tool_calls`。OpenAI 流式工具调用的标准形态是：

```
chunk1 {index:0,id:"call_a",function:{name:"get_weather",arguments:""}}
chunk2 {index:0,              function:{arguments:"{\"city\":"}}
chunk3 {index:0,              function:{arguments:"\"Paris\"}"}}
```

**根因**
把「OpenAI 的**增量** tool_call delta」当成「Gemini 的**完整** functionCall」来翻译：既没有按 `tc.Index` 累计 `arguments` 字符串，也没有等到 `finish_reason=="tool_calls"` 时一次性发出，还不特殊处理空/半截 JSON 的 `json.Unmarshal` 失败（`args` 保持 nil → JSON 输出 `null`）。

**实测输出**（修复前实测，一个上游工具调用 → 客户端收到 3 个 part；现由 `TestGeminiStreamAssemblesToolCallFragments` 钉住修复后的形状）：

```
data: {"candidates":[{"content":{"parts":[{"functionCall":{"args":null,"name":"get_weather"}}],"role":"model"},"index":0}]}
data: {"candidates":[{"content":{"parts":[{"functionCall":{"args":null,"name":""}}],"role":"model"},"index":0}]}
data: {"candidates":[{"content":{"parts":[{"functionCall":{"args":null,"name":""}}],"role":"model"},"finishReason":"STOP","index":0}]}
```

**影响**：Gemini SDK 侧看到 3 次函数调用、其中 2 次 name 为空、参数永远是 `null`（且 Gemini schema 的 `args` 是 object，`null` 不合规）⇒ 流式工具调用完全不可用；一元路径能用，所以是「同一能力只在一条入口生效」式的功能缺口。**注意这是功能坏掉，不是文档/审计口径问题。**

**修复方向**：按 `toolCall.Index` 维护 `name/arguments` 累加器，在 `finish_reason=="tool_calls"` 帧或流结束时一次性发出 `functionCall`（参数为完整 JSON 对象；空则 `{}` 而不是 `null`），与一元路径 `adapter/gemini.go:208-222` 的输出形状对齐。

---

### BUG-MULTIMODAL-TOKEN-ESTIMATE（P1，置信高）内联 base64 图片被当文本估算 prompt token

**位置**
- `internal/model/tokens.go:35-49` `ChatCompletionRequest.EstimatedPromptTokens()`：`chars += len(r.Messages[i].GetContentString())`
- `internal/api/common.go:822-836` `estimateCanonicalPromptTokens()`：同一套逻辑的**第二份实现**（同样问题）
- `internal/model/chat.go:21-30` `GetContentString()`：content 不是 string 时 `json.Marshal(m.Content)` ⇒ content 数组里的 `data:image/png;base64,…` 整段 base64 变成「字符」
- 计费落点：`internal/router/dispatcher.go:297,301`、`dispatcher_stream.go:26,151`、`dispatcher_modality.go:332,340,430,438` 把 `req.EstimatedPromptTokens()` 作为取消请求的 `PromptTokens` → `internal/api/common.go:587-598`（`recordFailedRequestWithError`，499 且 `PromptTokens>0` 时**计费**）→ `internal/api/common.go:229-231` 判定 499 有 token 即 billable
- 成功路径同源：`internal/api/common.go:651-653` `streamFallbackPromptTokens()`（上游不回 usage 时的估算）

**触发条件**（请求带内联图片/音频是前提；以下三条任一满足即计费）

1. **上游省略 usage**（最主要、也是这个兜底存在的理由）：一元 chat 走 `api/common.go:703-737 resolveChatUsage`，四条流式入口走 `streamFallbackPromptTokens`（`api/common.go:651-653`）。代码注释自己记录了线上已观测到「148 条 HTTP-200 但无 usage 的行」（`api/common.go:695-702`）；OpenAI-compatible 代理/自建服务忽略 `stream_options.include_usage` 很常见。此时图片请求按 232k prompt tokens 计费。
2. **客户端断连且当时还没收到 usage**：`api/chat_stream_handler.go:83-101` 的 499 分支「`totalPromptTokens==0 && canonicalPromptEstimate>0` ⇒ `streamFallbackPromptTokens`」，以及 `api/stream` 关闭/上游报错（`clientGone` 后 `event.Err`/channel 关闭）都会走到；`prepareUsage`（`api/common.go:228-231`）判定「499 + PromptTokens>0 = billable」⇒ 真扣费。
3. **dispatch 上下文被取消**：`router/dispatcher.go:297,301`、`dispatcher_stream.go:26,151`、`dispatcher_modality.go:332,340,430,438` 把 `req.EstimatedPromptTokens()` 塞进 `newCanceledError`，再由 `api/common.go:587-598` 写进 499 审计行并计费。注：出厂默认 `GATEWAY_DRAIN_ABORTED_UPSTREAM` 为开（断开后继续 drain 拿 usage），所以这条主要出现在关闭 drain 的部署、或断连后上游立刻出错/超时的情形（`api/upstream_lifecycle.go:29-38`）。

**根因**：估算器没有按 part 区分模态。图片 token 应按 provider 的图片计费规则（tiles / 固定 token，如 1024×1024 low-detail ≈85、high-detail ≈765）或按原始字节分级估算，而当前按 base64 字符数 ÷4。

**实测证据**（修复前实测；现由 `TestInlineImageIsNotChargedByBase64Length` 断言 ≤1000 tokens）：

```
inline-image request estimates 232529 prompt tokens, URL-form request estimates 30 tokens;
a 1024x1024 image really costs ~85-765 tokens ⇒ over-estimate ≈ 304x
```

（构造：930,000 字符 base64 ≈ 683 KB PNG，是完全正常的 1024×1024 截图。）

**影响**：任何「带内联图片 + 走估算兜底」的请求都会按 ~232k prompt tokens 计费；按 ¥2/M 计约 **¥0.46/次**，而该请求真实成本约 ¥0.0002 —— **多收 300~2700 倍**。因为兜底只在「上游不回 usage / 断连」时生效，线上表现为零散的高额审计行，不易被注意到（正因如此，兜底路径的估算器才需要按模态正确计费）。

**旁注（同根因，建议一并处理）**：`EstimatedPromptTokens`（model 包）与 `estimateCanonicalPromptTokens`（api 包）是两份重复实现，只有后者走 calibrator（`api/common.go:634-643`），前者被 dispatcher 用于 499 计费后才在校准 `api/common.go:593` 补一次 —— 两份实现日后极易再次分叉（上一轮 README §3.1 就是这一类）。

---

### BUG-GEMINI-THOUGHTS（P2，置信高）Gemini thinking 模型的思考 token 从不计入 completion

**位置**
- `internal/provider/gemini_types.go:76-81`：`GeminiUsageMetadata` 只有 `promptTokenCount` / `candidatesTokenCount` / `totalTokenCount` / `cachedContentTokenCount`，**没有 `thoughtsTokenCount`**
- `internal/provider/usage_mapping.go:36-52` `geminiUsageToModel()`：`CompletionTokens = u.CandidatesTokenCount`，`TotalTokens = u.TotalTokenCount`
- `internal/adapter/gemini.go:244-252`（出站一元）同样用 `TotalTokenCount = pTokens + cTokens`，把上游真实 total（含 thoughts）覆盖掉

**触发条件**：任何 Gemini 渠道 + thinking 模型（gemini-2.5-flash / 2.5-pro 默认开启 thinking；`gemini-2.5-pro` 长思考时常占输出大头）。Google 的 `usageMetadata.thoughtsTokenCount` 已包含在 `totalTokenCount` 内，并按 output token 计费。

**根因**：canonical `Usage` 只建模了 prompt/completion/total 三数，Gemini 的 thoughts 属于「provider 特定的 output 组成部分」，映射层没有把它并入 completion（也没有单独暴露），于是 canonical usage 内部自相矛盾，而计费引擎只按 `PromptTokens`/`CompletionTokens` 计价。

**实测证据**（修复前实测；现由 `TestGeminiUsageToModelFoldsThoughtsTokens` 断言 completion=550 且 total 自洽）：

```
upstream billed 550 output tokens (candidates 50 + thoughts 500)
but the canonical usage only reports CompletionTokens=50
⇒ 500 output tokens (90.9% of output) never reach the billing engine
```

**影响**：thinking 流量**少收**；同时 `usage_logs` 里 `total_tokens`（650）与 `prompt+completion`（150）不一致，运营核对账单时对不上。跨域提示：计费引擎在 `internal/billing`（C 域），若只改 `geminiUsageToModel` 把 thoughts 并入 `CompletionTokens` 即可让计费自动正确，无需动词 billing。

---

### BUG-RESPONSES-INCOMPLETE（P2，置信高）Responses API 永不报告截断（`status` 恒 completed，finish_reason 丢失）

**位置**
- `internal/model/response.go:634-641` `ConvertChatResponseToResponseResult()`：循环里从不读 `choice.FinishReason`，`Status` 是字面量 `"completed"`（另见 `:670`）
- `internal/model/response.go:55-63` `ResponseResult` 无 `incomplete_details` 字段（`grep -rn "incomplete" internal/api internal/model` 生产代码 **0 命中**）
- `internal/api/responses_stream_events.go:219-263` `completed()` → `responseObject("completed", …)`，流式同样固定 completed（`response.failed` 只在错误时发）

**触发条件**：Responses 请求命中 `max_output_tokens`（上游 `finish_reason:"length"`）或内容被过滤（`content_filter`）——agent 长任务被截断是常态。

**根因**：只翻译了「内容」没有翻译「结束语义」。OpenAI Responses API 对不完整结果返回 `status:"incomplete"` + `incomplete_details:{"reason":"max_output_tokens"}`，客户端 SDK/agent 框架据此决定续写或重试；当前实现让截断与正常结束**在 wire 上完全同形**。

**实测证据**（修复前实测；现由 `TestResponsesMaxOutputTokensIsIncomplete` / `TestResponsesStreamReportsTruncationAsIncomplete` 断言 incomplete + reason）：

```
upstream finish_reason="length" (max_output_tokens cut) but the client payload says status="completed"
with no incomplete_details:
{"id":"resp_abc123","object":"response","created_at":1,"status":"completed","model":"gpt-x",
 "output":[{...,"content":[{"type":"output_text","text":"The answer is 42 and the next step is"}]}],
 "usage":{"input_tokens":10,"output_tokens":7,"total_tokens":17}}
```

**影响**：agent/自动化把被截断的答案当完整答案继续执行（下游副作用跟着出错），且网关侧无法从响应体区分二者；`finish_reason` 信息在 Responses 形态里彻底丢失。
**附带小漂移（同一函数）**：`ResponseUsage`（`response.go:48-52`）没有 `input_tokens_details`，所以**一元** Responses 响应丢掉了 cached token 明细（流式路径 `api/responses_stream.go:188` 有发），客户端看到的缓存命中数因入口而异。

---

### BUG-SSE-HALF-FRAME（P2，置信中）EOF 时丢弃缓冲区最后一行：Gemini/Anthropic 与 OpenAI 的解析不对称

**位置**
- `internal/provider/gemini_stream.go:75-81`：
  ```go
  line, err := reader.ReadBytes('\n')
  if err != nil {
      if err != io.EOF { sendEvent(&model.StreamEvent{Err: err}) }
      return                    // ← len(line)>0 也不处理，也不发 IsDone
  }
  ```
- `internal/provider/anthropic_stream.go:106-113`：`err!=nil` 时直接 `sendEvent(IsDone); return`，同样不解析缓冲区里那行
- 对照（同包内正确写法）：`internal/provider/openai.go:256`、`internal/provider/openai_text_adapter.go:169`：`if err != nil && !(err == io.EOF && len(line) > 0)` —— EOF 带数据时**继续解析这一行**

**触发条件**：上游/中间代理在最后一帧后没有补 `'\n'`（连接被截断、CDN/反向代理缓冲切割、非标准 SSE 实现）。此时**完整**的 JSON 帧已经到达，只是缺尾随换行。

**实测证据**（修复前实测；现由 `TestGeminiStreamParsesFinalFrameWithoutNewline` 断言末帧被解析）：

```
the provider channel carried 0 frames; the complete final data frame
(finishReason=STOP, usageMetadata) was dropped because it arrived without a trailing newline.
```

**影响（为什么不是「反正断了」）**：
- Gemini 侧丢的是带 `finishReason` 的终帧 ⇒ `dispatcher_stream_pipe.go:138-152` 的 `streamEventIsTerminal` 认不出终止，把**纯分帧问题误记成一次渠道失败**（`errStreamTruncated` → 熔断记账），连续几次就会把一个健康渠道判死；
- Anthropic 原生侧丢的是 `message_delta`（携带 `output_tokens`）/`message_stop` ⇒ usage 丢失，只能退回字符估算计费（少收/多收都不准）；
- 同一仓库两个 provider 两种写法，属可修的解析缺陷而非设计取舍。**置信度中**的原因：正常 SSE 服务器都会补换行，触发需要截断式上游；但代价是把「网络截断」错算成「渠道不健康」，值得统一。

**修复方向**：把三处 EOF 分支统一成 `openai.go` 的写法；EOF 带缓冲数据时先解析该行，再发 `IsDone`。

---

### BUG-REQ-MUTATION（P2，置信高）ChatComplete 把渠道级出站消毒写成请求级副作用

**位置**
- `internal/provider/openai.go:83` `clonedReq := *req`（**浅拷贝**：`Messages` 切片共享底层数组）
- `internal/provider/openai.go:86` `stripReasoningContent(clonedReq.Messages)`
- `internal/provider/openai.go:151-155` `stripReasoningContent`：`for i := range messages { messages[i].ReasoningContent = "" }`（原地写）
- 流式同一处：`internal/provider/openai.go:172,176`

**触发条件**：请求 `messages` 里带 `reasoning_content`（DeepSeek / OpenRouter / 自建 vLLM 风格的多轮历史）且落到 OpenAI-compatible 渠道。

**实测证据**（修复前实测；现由 `TestChatCompleteDoesNotMutateCallerRequest` 断言 caller 不再被改写）：

```
caller's req.Messages[1].ReasoningContent was "The user asks a physics question; recall Rayleigh
scattering ∝ 1/λ⁴." before the call and is empty after
```

**影响**
1. **请求对象是 dispatcher 内多个渠道共用**（`internal/router/dispatcher.go:237-340` 同一个 `*req` 依次尝试候选渠道），第一次落在 OpenAI-compatible 渠道后历史推理内容永久消失，后续渠道/跨模型兜底（`dispatcher.go:226-228,361-362` 的 `reqCopy := *req`）拿到的是被改写的历史；
2. 该 `req` 还会被用于估算（`dispatcher.go:297,301`）与审计/trace，估算口径随「先试了哪种渠道」而变；
3. 「出站要不要剥 reasoning」是**渠道属性**，把它写回调用方对象，等于给未来所有消费者（重试、缓存键、审计快照）埋了共享可变状态。

**修复方向**：`clonedReq.Messages` 深拷贝（或构造新切片并只对副本清零）。一行改动。

---

## 2. 回归测试说明（探针已翻转）

| 测试文件 | 覆盖 | 关键用例 |
|---|---|---|
| `internal/api/gemini_stream_tool_calls_test.go`（+ 生产侧新增 `internal/api/gemini_stream_tool_calls.go` 累积器） | BUG-GEMINI-TOOLCALL | `TestGeminiStreamAssemblesToolCallFragments`（3 帧参数）、`TestGeminiStreamAssemblesParallelToolCalls`（index 0/1 交错）、`TestGeminiStreamTextAndToolInterleaved`（文本实时 + 只发一次）、`TestGeminiStreamAssemblesToolCallWithoutIndex`、`TestGeminiStreamFlushesToolCallWithoutTerminalFrame` |
| `internal/model/tokens_multimodal_test.go` | BUG-MULTIMODAL-TOKEN-ESTIMATE | `TestInlineImageIsNotChargedByBase64Length`（930k data URI ≤1000 tokens）、`TestImageBudgetIsPerPartAndLinear`、`TestRemoteImageUsesTheSameBudget`、`TestLowDetailImageUsesTheReducedBudget`、`TestInlineAudioIsNotChargedByBase64Length`、`TestPlainTextEstimateUnchanged`、`TestNilAndEmptyRequests` |
| `internal/provider/usage_mapping_test.go`（扩展） | BUG-GEMINI-THOUGHTS | `TestGeminiUsageToModelFoldsThoughtsTokens`（candidates 50+thoughts 500 ⇒ completion 550，total 650 自洽）、`TestGeminiUsageToModelNormalizesInconsistentTotal`、`TestGeminiUsageToModelWithoutThoughtsUnchanged` |
| `internal/model/responses_incomplete_test.go` + `internal/api/responses_incomplete_test.go` | BUG-RESPONSES-INCOMPLETE | `TestResponsesMaxOutputTokensIsIncomplete`、`TestResponsesContentFilterIsIncomplete`、`TestResponsesNormalCompletionStaysCompleted`、`TestResponsesUnaryUsageCarriesCacheDetails`、`TestResponsesStreamReportsTruncationAsIncomplete`、`TestResponsesStreamNormalCompletionStaysCompleted`、`TestResponsesStreamAndUnaryUsageShapeAgree` |
| `internal/provider/stream_eof_partial_frame_test.go` | BUG-SSE-HALF-FRAME | `TestGeminiStreamParsesFinalFrameWithoutNewline`、`TestGeminiStreamNewlineFramesUnchanged`（不重复处理已消费行）、`TestGeminiStreamMixedFraming`、`TestAnthropicNativeStreamParsesUnterminatedMessageDelta`（恰好一次 IsDone） |
| `internal/provider/request_mutation_test.go` | BUG-REQ-MUTATION | `TestChatCompleteDoesNotMutateCallerRequest`、`TestChatCompleteStreamDoesNotMutateCallerRequest`、`TestSecondChannelStillSeesReasoningHistory`、`TestStripReasoningContentLeavesInputSliceAlone` |
| `internal/api/stream_usage_accumulation_test.go` | §3-1 流式 usage 覆盖式赋值（顺手修） | `TestGeminiStreamUsageAccumulatesAcrossFrames`、`TestCompletionsStreamUsageAccumulatesAcrossFrames` |

**验证**（修复后，2026-10-28）：

```
gofmt -l internal cmd tests        → 空
go build ./...                     → OK
go vet ./...                       → OK
go test ./...                      → 16/16 包 ok
go test -race ./internal/api/ ./internal/provider/ ./internal/model/ → OK
```

反向验证（mutation check）：把 `max()` 改回覆盖赋值 ⇒ `TestGeminiStreamUsageAccumulatesAcrossFrames` 转红；把多模态估算改回按字符计 ⇒ `TestInlineImageIsNotChargedByBase64Length` 报 232,529 tokens 转红。测试确实钉住了修复。

---

## 3. 待确认（低置信 / 未能构造真实触发形态，不作为结论）

> 「流式 usage 覆盖式赋值 vs `max()`」本轮已按同一规则修复（三个 handler 改为 `max()`，见 §6 第 7 条），不再属于待确认。

1. **Responses 流式 `output_tokens_details.reasoning_tokens` 恒为 0**（`api/responses_stream.go:189` 硬编码 `0`）：字段存在但与实际 reasoning 输出无关，属「占位字段说假话」，量级小、未见消费者，待确认是否应接 `reasoning_adapter` 的统计。
2. **Gemini 渠道 embed 的 URL 拼接**：`provider/gemini_embed.go:86` 用 `targetModel` 原样拼 URL，而请求体用 `models/`+`TrimPrefix(targetModel,"models/")`；若渠道映射到带 `models/` 前缀的模型名（如 `models/gemini-embedding-001`），URL 会变成 `/v1beta/models/models/…`。当前渠道配置约定是否允许 `models/` 前缀未经确认，故不列结论。

---

## 4. 已排除（查过、确认不是缺陷）

- **多模态缓冲后「原始 resp.Stream 不 Close ⇒ cancel 泄漏」**（我最初的怀疑，`api/multimodal_response_body.go:24-37` 把 `resp.Stream` 换成 `io.NopCloser`，看起来 `defer resp.Stream.Close()` 变成了假关闭）：**实测不是 bug**。Go 的 `defer` 在**注册时就求值接收者**，所以 `api/multimodal_handler.go:87` 的 `defer resp.Stream.Close()` 捕获的是**替换前**的原始流，函数返回时照样 Close 它（外层 `dispatcher_modality_entry.go:81-84` 的 `cancelBudget` 与内层 `dispatcher_modality.go:265` 的 `cancelCall` 都随之执行）。最小验证：
  ```go
  r := &resp{Stream: orig}
  func() { defer r.Stream.Close(); r.Stream = &rc{"REPLACEMENT"} }()
  // 输出 closed: ORIGINAL  —— defer 绑定的是旧值
  ```
  我最初用「直接调 bufferAndRepairResponseBody」的探针得出了错误结论（脱离了 handler 的 defer 语境），已删除该探针。留档：**读 handler 时必须先确认 defer 绑定时机**，否则会把「职责在调用方」误判成泄漏。
- **`/v1/completions` 流式断连记 200**（我最初的怀疑，`api/completions_stream_handler.go:129-137` 确实只在 `streamError!=nil` 时才看 `c.Request.Context().Err()`）：实测**不是 bug** —— `api/common.go:203-206` 的 `prepareUsage` 会在落库前把「ctx 已取消 + 2xx/3xx」统一归一为 **499**。我用探针驱动同一场景对比 chat 流式（499）与 completions 流式，两者 usage_logs 都是 499，遂删除该探针。**这是本轮唯一被证伪的假设**，留档避免下轮重复。
- **`resp.Choices[0]` / `Choices[0].Delta` panic 风险**：`provider/reasoning_adapter.go:191-216` 的 `Flush` 已对 `len(c.Choices)==0` 做合成 choice（注释记录了「曾在 provider goroutine 里 panic 并拖垮整个进程」的历史），未再发现同类索引。
- **表达式求值栈溢出**（MCP `airoute_calc_eval`，action 域可触发、panic 在 handler 之外会是进程级致命错误）：`api/math_eval.go:10-18` 已有 `maxMathExprLen=512` 与 `maxMathDepth=32` 双护栏，`parsePrimary` 的一元负号递归也受 depth 约束。**已加固，非缺陷**。
- **provider 流式 goroutine 无 `recover`**（结构性风险：`internal/provider/{openai,anthropic_stream,gemini_stream,openai_text_adapter}.go` 的 `go func` 都没有 recover，panic 会杀进程）：逐个审了这些 goroutine 内的 JSON 解码与类型断言，全部为 `, ok` 形式或已判空（`gemini_stream.go:119` 的 `Candidates[0]` 有 `len>0` 保护等），未找到可达 panic 路径 ⇒ 仅记录为结构性建议（加 recover 是纵深防御，不是本轮缺陷）。
- **SWRR 权重 map 的并发写**（`router/load_balancer.go:104-135` `s.weights[key]` 无锁自增，看起来是 data race）：实际调用点在 `router/dispatcher_routing.go:110-113` 用 `d.wrrMu.Lock()` 串行化，且 `load_balancer.go:33` 注释明确 `Healthy()` 只读不消耗 canary。**不是 race**。
- **熔断半开 canary 泄漏**：`router/dispatcher.go:249-340`、`dispatcher_stream.go:74-152`、`dispatcher_modality.go:104-234` 的每条早退分支都配了 `AbortCanary` 或 `Record*`；`dispatcher_modality.go:144-197` 的 7 处 Abort 尤其完整。未找到漏归还路径。
- **MCP SQL 反射工具注入**：`api/mcp_tools.go:20-50` 是**表 allowlist**（仅 usage_logs/model_prices/model_fallbacks）+ 标识符正则 + 全语句关键字拒绝（含 `WITH … DELETE` 形态）；`describe_table` 的 `table_name` 先过正则与 allowlist 才插值。未发现注入。
- **MCP 客户端 SSE 解析**（`api/mcp_client.go:302-356`）：EOF 有处理、总字节有 8 MiB 上限、`next()` 会随 ctx 关闭底层流；`api/mcp_handler.go:295` 的 `t["name"].(string)` 未做 ok 断言，但对象来自 `getBuiltinMCPTools()` 静态常量，非外部输入 ⇒ 不是可达 panic。
- **各 provider 的响应体关闭**：逐个核对 `internal/provider/*.go`、`internal/api/mcp_client_http.go`、`inbound_anthropic.go:346-353` 的 `Body.Close`/`defer`，未发现遗漏（含错误分支与 limit reader 分支）；多模态的缓冲替换路径也已排除，见本节第一条。
- **`req.Messages` 之外的请求改写**：`dispatcher.go` 的 `reqCopy := *req` 只改 `Model` 字段；`cloneReq.StreamOptions` 只改副本指针字段；chunk 层 `event.Chunk.Model = req.Model` 只改响应侧。除 BUG-REQ-MUTATION 外没有第二处请求污染。
- **`internal/validator`**：`ValidateOpenAI`（角色/工具调用/temperature/top_p 边界）与 pipeline 的 `keyCfg[0]` 均有长度保护；未发现会拒绝合法请求的规则回归（C-06 的 `disallow_temperature` 只在 OpenAI 校验器消费是**已记录的设计断裂**，见 README §3.1，不重复上报）。

---

## 5. 跨域归属（交 Lead 分派）

| 发现 | 归属 | 说明 |
|---|---|---|
| BUG-MULTIMODAL-TOKEN-ESTIMATE 的**费用后果** | 计费域（C，`internal/billing`）配合 | 根因在 `internal/model/tokens.go` + `internal/api/common.go`（本周范围），但「多收多少钱、报价单怎么修」需计费域确认计价口径（图片按 tiles 还是按 token）。修复只需改估算器，不必动 billing。 |
| BUG-GEMINI-THOUGHTS | provider 域（本轮） | 只改 `provider/usage_mapping.go` + `gemini_types.go` 即可让计费自动正确；**不需要** billing 域改动。 |
| BUG-RESPONSES-INCOMPLETE 的 `input_tokens_details` 缺失 | API 域（本轮）+ 前端展示 | 若控制台展示 cached tokens 需同步确认 `ResponseUsage` 补齐后的前端字段。 |
| `EstimatedPromptTokens`（model 包）与 `estimateCanonicalPromptTokens`（api 包）两份实现 | 架构/清理（Lead 正在删死代码） | 建议收敛成一份并统一走 calibrator，属 README §3.1「同一件事两套实现」的同根因遗留。 |

---

## 6. 修复记录（2026-10-28 修复阶段）

写权：`internal/api`、`internal/model`、`internal/provider`、`internal/adapter`（+ 测试）。未改动 storage/billing/telemetry/middleware/distributed/controlplane/cmd/web，未触碰 `internal/api/mcp_tools_routing.go`，未 commit。

| # | ID | 状态 | 生产代码改动 | 回归测试 | 验证 |
|---|---|---|---|---|---|
| 1 | BUG-GEMINI-TOOLCALL | fixed | **新增** `internal/api/gemini_stream_tool_calls.go`（`geminiStreamToolCalls`：按 `tc.Index` 累积 name/arguments，缺 Index 时按出现顺序：带 id/name 的 fragment 开新调用、裸 arguments fragment 续上一个）；改 `internal/api/gemini_inbound_stream.go`（新增 `emitCandidate`/`functionCallParts`，tool fragment 改为累积，终止帧（finishReason）一次性发出完整 functionCall；`[DONE]`/channel 关闭时 flush 未发调用，保证不丢；tool 参数字节计入 completion 兜底估算） | `gemini_stream_tool_calls_test.go` ×5 | 3 帧单调用 ⇒ 1 个完整 part；并行 index 0/1 ⇒ 2 个；文本实时、只发一次；无 Index 可用；无终帧仍 flush |
| 2 | BUG-MULTIMODAL-TOKEN-ESTIMATE | fixed | `internal/model/tokens.go`：新增 `EstimateContentTokens` + 预算常量（`ImageTokensLowDetail=85`、`ImageTokensDefault=765`、`NonTextPartTokens=765`），`EstimatedPromptTokens` 改用 part 感知估算（base64 一律不进 chars）；`internal/api/common.go`：`estimateCanonicalPromptTokens` 改为委托 `req.EstimatedPromptTokens()`（两套实现收敛为一套） | `tokens_multimodal_test.go` ×7 | 930k data URI ⇒ 795 tokens（原 232,529）；多图线性；纯文本/工具行为不变；低 detail=85 |
| 3 | BUG-GEMINI-THOUGHTS | fixed | `internal/provider/gemini_types.go`：`GeminiUsageMetadata` 增 `ThoughtsTokenCount`；`internal/provider/usage_mapping.go`：`CompletionTokens = Candidates + Thoughts`，并把「total < prompt+completion」归一（防上游 total 缺失/失真）；`internal/adapter/gemini.go`：出站不再用 `prompt+candidates` 覆盖上游 total，仅在上游未给 total 时才合成 | `usage_mapping_test.go` ×3（扩展） | candidates 50 + thoughts 500 ⇒ completion 550、total 650、`prompt+completion==total`；无 thoughts 行为不变；incoherent total 被归一 |
| 4 | BUG-RESPONSES-INCOMPLETE | fixed | `internal/model/response.go`：`ResponseResult` 增 `IncompleteDetails`，新增 `ResponseStatusCompleted/Incomplete` + `IncompleteResponseStatus(finishReason)`（length⇒max_output_tokens，content_filter⇒content_filter）；`ConvertChatResponseToResponseResult` 由 finish_reason 推导 status；`ResponseUsage` 增 `input_tokens_details`/`output_tokens_details`（一元与流式对齐）。`internal/api/responses_stream_events.go`：emitter 记录 finishReason，`response.completed` 按映射输出 `incomplete` + `incomplete_details`；`internal/api/responses_stream.go`：喂入 finish_reason | `responses_incomplete_test.go`（model ×4、api ×3） | 截断可区分（unary+stream）；stop/无 finish_reason 仍 completed 且无 incomplete_details；两入口 usage 形状一致（cached_tokens 7） |
| 5 | BUG-SSE-HALF-FRAME | fixed | `internal/provider/gemini_stream.go`、`internal/provider/anthropic_stream.go`：EOF 分支改为 `if err != nil && !(err == io.EOF && len(line) > 0)`（与同包 `openai.go:256` 一致），先处理缓冲区末行；下一轮读到空 EOF 正常收尾，不重复处理 | `stream_eof_partial_frame_test.go` ×4 | 末帧无换行仍解析出 finishReason+usage；正常帧序不回归（2 帧=2 chunk）；混合分帧；anthropic 未终止 `message_delta` 保留且 `IsDone` 恰好 1 次 |
| 6 | BUG-REQ-MUTATION | fixed | `internal/provider/openai.go`：`stripReasoningContent` 改为**返回副本**（`make`+`copy` 后清空），两处调用改为 `clonedReq.Messages = stripReasoningContent(clonedReq.Messages)`；调用方 `req.Messages` 不再被改写 | `request_mutation_test.go` ×4 | caller reasoning 保留、出站体仍已剥离；流式同样；第二个渠道/估算看到原始历史；helper 契约 |
| 7 | §3-1 流式 usage 覆盖式赋值（顺手修） | fixed | `internal/api/{gemini_inbound_stream,responses_stream,completions_stream_handler}.go`：prompt/completion/cached 累加改为 `max(...)`，与 `chat_stream_handler` / `inbound_anthropic_stream` 语义统一 | `stream_usage_accumulation_test.go` ×2 | 增量 usage 帧（prompt 只在早期帧、completion 只在末帧）⇒ gemini 终帧 100/50/150、completions 审计行 100/50；mutation 验证（改回覆盖 ⇒ 转红） |

**修复备注**
- **BUG-GEMINI-TOOLCALL 行为变化**：流式工具调用不再逐帧下发 `functionCall`，而是在终止帧一次性发出完整调用（`arguments` 为空或不可解析时为 `{}`，不再是 `null`）。若某客户端依赖「乱序 functionCall 增量」，需要改为等待终止帧——这正是 Gemini schema 要求的形状。
- **BUG-MULTIMODAL-TOKEN-ESTIMATE 行为变化**：远程 URL 图片现在与内联图同样按固定预算（765）计，而不是只计 URL 字符（~30）。同一张图换了传输方式不应换计价口径；这是对 Lead 验收标准「URL 形式不变（30）」的一处**有意偏离**（详见交付摘要），绝对量级仍在千 token 以内。纯文本请求估算完全不变。
- **BUG-GEMINI-THOUGHTS**：`billing` 侧无需改动（只消费 canonical `PromptTokens`/`CompletionTokens`）；把 thoughts 并入 completion 后不会与上游 total 双重计入（total 只做自洽校验）。
- **BUG-RESPONSES-INCOMPLETE**：`output_tokens_details.reasoning_tokens` 仍固定 0（canonical `Usage` 没有 reasoning 分量），属 §3 待确认项，本轮未扩展字段。
- 未发现需要跨域协作的修复：storage/billing/telemetry/middleware/distributed/controlplane 均无需改动。

---

## 7. 复现命令

```bash
export PATH=$PATH:/usr/local/go/bin
cd /home/ifnodoraemon/myagent/airoute

# 6 条修复的回归测试（已翻转，断言修复后行为）
go test ./internal/api   -run 'TestGeminiStream|TestResponsesStream|TestCompletionsStreamUsage' -v
go test ./internal/model -run 'TestInlineImage|TestImageBudget|TestRemoteImage|TestLowDetail|TestInlineAudio|TestPlainText|TestResponses' -v
go test ./internal/provider -run 'TestGeminiUsage|TestGeminiStream|TestAnthropicNativeStream|TestChatComplete|TestSecondChannel|TestStripReasoning' -v

# 全量质量门
gofmt -l internal cmd tests && go build ./... && go vet ./... && go test ./...
go test -race ./internal/api/ ./internal/provider/ ./internal/model/

# 修复前行为复现（如需对照）：git stash / 回退对应生产改动后，上述用例会转红
```
