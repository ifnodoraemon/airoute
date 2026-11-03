# Bug 排查 F：router 并发与协议适配（2026-10-29 bughunt）

> **口径**：本轮目标是**上一轮没覆盖的真实缺陷**，域 = `internal/router`（SWRR / 会话亲和 / 三态熔断 / 重试·超时·预算）+ `internal/provider` + `internal/adapter`（OpenAI ⇄ Anthropic ⇄ Gemini 双向转换），并接手上一轮遗留两条（Responses `reasoning_tokens`、`gemini_embed` 的 `models/` URL）。
> **方法**：先读上一轮 [README](../2026-10-28-bughunt/README.md)、[01-request §3/§4](../2026-10-28-bughunt/01-request.md)、[02-core §6](../2026-10-28-bughunt/02-core.md) 排除重复项；每条结论都配**包内 `*_bugprobe_test.go` 探针**（`go test -run Bugprobe` 可跑，并发/分布类给出量化数字），探针**断言当前错误行为**以保证 `go test ./...` 保持绿；修复阶段再翻转为正确断言并去掉 `Bugprobe` 前缀。
> **基线**：`2f43d28`（全绿）。本轮**未改生产代码、未 commit**。
> **验证**：`go test -count=1 ./internal/{model,provider,adapter,router}/` 全 ok；`go test -race -count=1 ./internal/{router,adapter,provider}/` 全 ok（无数据竞争报告）；最终 `gofmt -l` 空、`go test -count=1 ./...` 16/16 包 ok。
> **行号口径**：以 HEAD `2f43d28` 为准。`internal/{model,provider,adapter,router}` 本轮期间**未被并发改动**，行号稳定；`internal/api/common.go` 与 `internal/billing/calibration.go` 同期被其他域并发修改（死代码清理 / 估算收敛，与本文结论无关），这两处行号可能偏移 ±10 行，按函数名检索即可。

---

## 0. 一句话结论

新增 **1 条 P1 + 6 条 P2**，全部带可复现探针。P1 是 Lead 自查的 LEAD-01（typed `[]model.ContentPart` 不被 `ParseMessageContent` 识别）：**我独立复现成立**（100× 高估 + 跨协议图片变文本 + 校准器被压到下界），但复核出 Lead 描述里两处需要修正的细节（见 §3）。6 条 P2 里 2 条是上一轮遗留被证实为真缺陷（不再列"待确认"），3 条是协议转换的字段级丢失，1 条是会话亲和"一致哈希"名不符实。

主链路无 P0：没有崩溃、没有鉴权绕过、没有并发数据竞争（`-race` 实测干净）。

---

## 1. 发现总表

| ID | P | 域 | 一句话 | 位置 | 置信度 |
|---|---|---|---|---|---|
| **F-01** | **P1** | model/adapter/provider/api | typed `[]ContentPart` 走 JSON→文本回退：估算 ×100、跨协议图片变文本、校准器被压到 floor | `model/multimodal.go:38` + `adapter/anthropic.go:132` / `adapter/gemini.go:142,151` | 高（实测） |
| F-02 | P2 | model/responses | Responses `output_tokens_details.reasoning_tokens` 恒 0，而上游值**就在** `Usage.Extra` 里被丢掉 | `model/response.go:715`、`api/responses_stream.go:193` | 高 |
| F-03 | P2 | provider/gemini_embed | `models/` 前缀模型：URL 拼成 `/v1beta/models/models/…`，而同请求 body 已归一化 → 自相矛盾 | `provider/gemini_embed.go:95,147` | 中高 |
| F-04 | P2 | provider/convert | `tool_choice` 跨协议**整个丢失**（Anthropic 有字段不填、Gemini 无 toolConfig） | `provider/anthropic_convert.go:169`、`gemini_convert.go:165` | 高 |
| F-05 | P2 | provider/anthropic_stream | 流式 usage 丢 `cache_creation_input_tokens`：缓存写入按普通输入价计费（1.25× → 1×） | `provider/anthropic_stream.go:364` vs `usage_mapping.go:23` | 高（实测） |
| F-06 | P2 | router/affinity | 会话亲和实为 `%N` 取模而非一致哈希：加 1 个渠道重映射 **74.8%** 会话（应 ~25%） | `router/load_balancer.go:45-51` | 中高（实测） |
| F-07 | P2 | adapter/provider | URL 型图片跨协议双向损坏：入站 Anthropic `source.type=url` 整块丢弃；出站 OpenAI https 图片被塞进 base64 字段 → 上游 400 | `adapter/anthropic.go:66-81`、`anthropic_convert.go:111-122`、`gemini_convert.go:103-110` | 中高（实测） |

上一轮遗留两条：F-02、F-03 **均复现为真缺陷**（不再是"待确认"）。

---

## 2. 发现详情

### F-01（P1，金额 + 功能）`ParseMessageContent` 不认 typed `[]ContentPart`
**位置**
- 根因：`internal/model/multimodal.go:38` `ParseMessageContent` 只处理 `string`（:44）与 `[]any`（:54），其余走 `json.Marshal` 回退成**单个 text part**（:99-106）。
- 生产 typed 内容的地方：`internal/adapter/anthropic.go:58,66-81,132`（`Content: contentParts`，`[]model.ContentPart`）、`internal/adapter/gemini.go:66,77,142,151`。
- 消费方：`model/tokens.go:60-83`（`EstimateContentTokens`）、`:97-111`（`EstimatedPromptTokens`）；`provider/anthropic_convert.go:84-97`、`provider/gemini_convert.go:26-33,83-132`、`provider/openai_text_adapter_convert.go:47`。

**触发条件**：`/v1/messages` 或 Gemini 原生协议带图请求（入站适配器产出 typed 内容），且 ①上游未回 usage/客户端中途断开（估算计费），或 ②路由到 Anthropic/Gemini/`openai_text` 渠道（出站转换）。OpenAI 渠道不受影响——typed `[]ContentPart` 的 json tag 正确，直接序列化即为合法 OpenAI `image_url`。

**根因**：`ParseMessageContent` 的类型分发漏了 `[]ContentPart`；由于所有下游（估算、三个转换器）都只经它读内容，一处漏识别同时污染计费口径与出站协议。

**影响**
1. **多扣（~100×）**：`internal/api/common.go:838 estimateCanonicalPromptTokens`（流式兜底 `:671 streamFallbackPromptTokens`） → 499/无 usage 路径 `usage_source=estimated` 真扣费；探针实测同内容 771 vs 76,829 tokens。预授权（`common.go:438`）与取消返回的 `PromptTokens`（`dispatcher*.go` 8 处）同样被放大。
2. **校准器被压到下界**：`api/chat_stream_handler.go:56,96` 在上游**回了** usage 时仍用该本地估算 `observePromptEstimate` → `billing/calibration.go:59-79` 学 `ratio=actual/local≈0.02`，被 `calibrationFloor=0.4`（:46）截断为 0.4；此后该模型所有"无 usage/取消"请求的 `calibratedPromptEstimate` 被压到 0.4×（**欠计费 2.5×**），需约 10 次正常样本（EMA α=0.1）才回归。**带图流量持续时长期停在 0.4。**
3. **功能**：跨协议带图请求不可用——图片作为 `data:...;base64,...` 纯文本发上游（Anthropic 收到 text block、Gemini 收到普通 text part），模型看不到图，客户端拿到"看似正常但失明"的回答。

**探针**：`internal/model/typed_content_bugprobe_test.go`（771 vs 76,829，含根因断言：typed → 单 text part 且含 `base64,`）、`internal/provider/typed_content_convert_bugprobe_test.go:TestBugprobe{Gemini,Anthropic}ConvertSendsTypedImageAsText`（并对照证明 `[]any` 同内容会被正确转成 image/inlineData）。

**建议修复**（承接 LEAD-01）：`ParseMessageContent` 增加 `[]ContentPart`（建议连带 `ContentPart` 单值）分支直接复用、不走 JSON；改这一处即可同时修复估算与三个转换器。回归：①typed 与 `[]any` 估算同量级；②两个转换器对 typed 图片产出 image/inlineData；③带图请求不污染校准（或 `Observe` 只用表示一致的估算）。

---

### F-02（P2，协议）Responses `reasoning_tokens` 恒 0 —— 上一轮遗留，确认是缺陷
**位置**：`internal/model/response.go:715` `usage.OutputTokensDetails = &ResponseOutputTokensDetails{ReasoningTokens: 0}`；流式对应 `internal/api/responses_stream.go:193`。
**触发条件**：任何推理型上游（在 `completion_tokens_details.reasoning_tokens` 上报的 OpenAI 兼容上游，如 R1/o 系列）走 `/v1/responses`。
**根因**：canonical `Usage` 确实**没有**独立 reasoning 字段，但 `Usage.Extra` 会原样保留未建模字段（`model/json_extras.go:20`，`json_extras_test.go:103-119` 已断言 `completion_tokens_details` 被保留）。转换函数没有读它，直接硬编码 0 —— 上一轮判定"属字段扩展"低估了：**数据在进程内、可用，是被主动丢弃**。
**影响**：`/v1/responses` 客户端（agent 框架常据此判断 reasoning 预算/成本）看到 0，与计费口径不一致；ibid. `responses_stream.go` 每个终帧同样虚报 0。金额不受影响（计费读 `CompletionTokens`，reasoning 已包含在内）。
**探针**：`internal/model/response_reasoning_tokens_bugprobe_test.go`（上游 99 → Responses 0，并断言 `Usage.Extra` 里确有 99）。
**建议修复**：转换时从 `chatResp.Usage.Extra["completion_tokens_details"]` 解析 `reasoning_tokens`（流式在 handler 里累积同理）；缺失时才为 0。

---

### F-03（P2，功能）`gemini_embed` 对 `models/` 前缀模型的 URL 拼装 —— 上一轮遗留，确认是缺陷
**位置**：`internal/provider/gemini_embed.go:95`（单条）与 `:147`（批量）用 `"/v1beta/models/"+targetModel+":embedContent"`；同函数 `:105`、`:160` 的 **body** 却是 `"models/"+strings.TrimPrefix(targetModel,"models/")`。
**触发条件**：渠道 `ModelMapping` 的上游名带 `models/` 前缀（Google 自己的 ListModels / 文档就是这个形态，例如 `models/gemini-embedding-001`）。
**根因**：URL 未做与 body 相同的归一化 → `POST /v1beta/models/models/gemini-embedding-001:embedContent`，上游 404；请求体又是合法的单前缀，日志里"URL 与 body 互相打脸"，运维极难定位。
**影响**：该映射下 embedding 全部失败（其他模态同映射同样失败，见下），属配置健壮性缺陷而非数据错账。
**探针**：`internal/provider/gemini_embed_url_bugprobe_test.go`（httptest 捕获真实路径 `…/models/models/…`，并断言 body 为单前缀；顺带记录 `buildGeminiURL` 对 chat 路径同样双前缀 `gemini.go:41-50`）。
**建议修复**：在 URL 层统一归一化（与 body 同一 helper），或更彻底地在 `ChannelConfig.GetUpstreamModel` 出口剥掉 `models/` 前缀（对 chat/embed/stream 一次修完）；回归覆盖单条+批量+chat。

---

### F-04（P2，功能）`tool_choice` 跨协议丢失
**位置**：`internal/provider/anthropic_convert.go:169-179` 构造 `AnthropicRequest` 时未赋 `ToolChoice`（`anthropic_types.go:22` 字段存在；native 直通路径 `:12-21` 反而有 `ToolChoice: original.ToolChoice`）；`internal/provider/gemini_convert.go:165-180` 完全没有 `toolConfig`（`GeminiRequest` 也无此字段）。
**触发条件**：OpenAI 客户端设 `tool_choice:"required"` 或 `{"type":"function","function":{"name":"x"}}`，请求被路由到 Anthropic 或 Gemini 渠道。
**根因**：转换器只搬了 `tools`，漏了"必须调用哪个工具"的约束字段。
**影响**：上游收到工具声明但不受约束，模型可以自由散文作答 → agent 循环卡在"模型不调工具"；与 OpenAI 直连行为不一致，且**静默**（无报错）。Gemini 侧连能力都没有（需 `toolConfig.functionCallingConfig.mode=ANY`）。
**探针**：`internal/provider/typed_content_convert_bugprobe_test.go:TestBugprobeAnthropicConvertDropsToolChoice`（`ToolChoice="required"` → Anthropic 侧 nil、Gemini 侧无 `toolConfig`，同时断言 `tools` 已转发）。
**建议修复**：Anthropic 直接映射（`required`→`{"type":"any"}`、`{"type":"function",...}`→`{"type":"tool","name":...}`，复用 `adapter.convertAnthropicToolChoice` 的逆映射）；Gemini 补 `toolConfig`。

---

### F-05（P2，金额）Anthropic **流式** usage 丢 `cache_creation_input_tokens`
**位置**：`internal/provider/anthropic_stream.go:364-369` 手工重建 `PromptTokensDetails`，只带 `CachedTokens`；同样三个计数在**非流式**走 `provider/usage_mapping.go:13-30`，`CachedTokens` 与 `CacheCreationTokens` 都带。
**触发条件**：OpenAI 形态客户端（`/v1/chat/completions`、`/v1/responses`、`/v1/completions`）以 `stream:true` 走 Anthropic 渠道，且上游报了缓存写入（prompt caching）。
**根因**：流式路径没有复用 `anthropicUsageToModel`，手写映射漏字段。
**影响**：`billing/engine.go:433-439` 把 prompt 拆成"未缓存 / 缓存读 / 缓存写"三桶，缓存写按 `1.25×` 输入价计费；`CacheCreationTokens=0` 时这桶退化为普通输入价 → **系统性少收**（该桶 25% 差价 × 缓存写入量），且同一请求流式与非流式账单不一致。探针实测：prompt=330、cached=20、cache_creation=**0**（非流式同数据为 300）。
**探针**：`internal/provider/anthropic_stream_cache_bugprobe_test.go`（假 SSE：message_start 带 cache_creation=300 → 终帧 usage 丢 300；并对照断言 `anthropicUsageToModel` 保留 300）。
**建议修复 + 交接**：provider 侧补 `CacheCreationTokens`；**同时** `internal/api/chat_stream_handler.go:47,113`（及 `completions_stream_handler.go`、`responses_stream.go`）根本没有累积/传 `CacheCreationTokens` 的变量，只改 provider 仍会被 api 层丢弃——需 api 域配合（`inbound_anthropic_stream.go:434-437` 是正确范例）。

---

### F-06（P2，成本/性能）会话亲和是 `%N` 取模，不是一致哈希
**位置**：`internal/router/load_balancer.go:45-51`（注释写 "Consistent Hash via FNV-1a"，实现是 `startIdx = fnv(session+":"+model) % len(group)`）。
**触发条件**：同模型有 ≥2 个渠道且启用会话亲和（有 session id），任一渠道增删/上下线。
**根因**：取模 N 的映射在 N 变化时几乎全量重排；一致哈希（环/rendezvous）只需迁移 ~1/N。
**影响**：探针实测**加第 4 个渠道后 74.8% 会话换渠道**（一致哈希应 ~25%）→ 前缀缓存（KV Cache）命中率骤降，首 token 延迟与上游成本上升；多渠道轮转/灰度发布会持续冲刷缓存。属"注释/设计承诺 vs 实现"的真实断裂（`docs/ROUTING_TOPOLOGY.md:6` 把亲和当卖点），不是数据正确性问题。
**探针**：`internal/router/affinity_bugprobe_test.go`（2000 会话 × 3→4 渠道，量出 74.8%，并反查 `fnv%N` 机制）。
**建议修复**：换 ring（虚拟节点）或 rendezvous hash，保留 Redis pin 兜底；回归钉住"增删 1 渠道重映射比例 ≤ ~1/N + 容差"。

---

### F-07（P2，功能）URL 型图片跨协议双向损坏
**位置**
- 入站：`internal/adapter/anthropic.go:66-81`（`case "image"`）只处理 `source.type=="base64"`，`type:"url"` 时 `media_type/data` 皆空 → 整块**静默丢弃**（探针实测转换后内容只剩 `[{"type":"text",...}]`）。
- 出站：`model/multimodal.go:110-119 ParseDataURI` 对非 `data:` 值原样返回（注释"raw base64 or remote URL"），`provider/anthropic_convert.go:111-122` 写进 `source.type=base64`、`provider/gemini_convert.go:103-110` 写进 `inlineData.data`——两个字段都只接受 base64。
**触发条件**：客户端用 https 图片 URL（Anthropic 原生 `source.type=url`，或 OpenAI `image_url.url=https://…`）走跨协议渠道。
**影响**：入站方向图片**无声消失**（模型"失明"作答）；出站方向上游 400（`data` 非 base64）或 Gemini 拒收 → 普通 OpenAI 图片请求无法由 Anthropic/Gemini 渠道服务。
**探针**：`internal/adapter/url_image_bugprobe_test.go`（URL 图片被丢、base64 图片正常，对照成立）、`internal/provider/url_image_bugprobe_test.go`（Anthropic `source.type="base64"` 的 `data` 是 URL；Gemini `inlineData.data` 是 URL）。
**建议修复**：入站把 `url` source 转成 `image_url`（保留 URL，不下载）；出站对非 `data:` 的 URL 走各协议的 URL 形态（Anthropic `source.type=url`）或按需抓取内联（需体积/超时护栏）；`ParseDataURI` 的"remote URL"分支需要调用方分形态，不能直接当 base64。

---

## 3. LEAD-01 独立复核（不采信、自行复现）

**结论：成立，且我独立复现出比 Lead 更强的量化与一处需修正的细节。**

1. **类型分发确认**：`ParseMessageContent` 仅 `string`/`[]any`；`adapter/anthropic.go:58,66-81,132` 与 `adapter/gemini.go:66,77,142,151` 确实产出 typed `[]model.ContentPart`。
2. **金额（实测）**：300 KB base64 图 + 一句文本 → `typed=76,829` vs `[]any=771` tokens，**99.6×**（Lead 报 98×，量级一致；差异来自 payload 填充长度）。
3. **功能（实测）**：`convertOpenAIToGemini` 对 typed 图片产出 0 个 `inlineData`、1 个含 `base64,` 的 text part；`convertOpenAIToAnthropic` 同样无 image block。相同内容的 `[]any` 表示**能**正确转成 image/inlineData → 缺陷锚定在 typed 表示，不在转换器本身。
4. **修正 ①（数值）**：Lead 写"校准 ratio 压到 ~0.01"，但 `billing/calibration.go:46 calibrationFloor=0.4` 会**截断**，实际最低只能到 0.4 → 后续估算最多**欠计费 2.5×**，不是 100×。投毒真实存在（`ratio=actual/local`），但影响上界应按 2.5× 记，恢复需 ~10 次正常样本（EMA α=0.1）。
5. **修正 ②（归因）**：Lead 把"OpenAI 渠道不受影响"写对了，但需补一句：**纯文本**请求不受影响（无 typed part 参与），只有带图/多模态请求触发；因此线上存量校准暂时没被带偏（与 Lead 查到的线上 ratio ≈1–2.4 自洽）。

**复核探针**（已入仓，跑得通）：`internal/model/typed_content_bugprobe_test.go`、`internal/provider/typed_content_convert_bugprobe_test.go`。已在 §2 F-01 给出修复建议，与 Lead 一致（改 `ParseMessageContent` 一处覆盖全部下游）。

---

## 4. 本轮"查过但不是缺陷"（给下一轮省时间）

以下都是我的**初始怀疑**，用代码追踪 + 探针推翻了，别重复上报：

1. **熔断半开 canary 名额归还**：逐分支核对 5 条 dispatch 路径全部归还/记账齐备——`Dispatch`（`dispatcher.go:249` 取名额；`:264,293,300,309,324,332` Abort，`:340` RecordFailure，`:286` RecordSuccess）、`DispatchStream`（`dispatcher_stream.go:74`；`failStreamAttempt:25,30,32,35,37` 覆盖全部失败分类）、`dispatchHTTPOnce`（`dispatcher_modality.go:104`；`:144,171,178,184,189,193,217,234` Abort，`:197,213` 记账）、`dispatchEmbeddingOnce`/`dispatchRerankOnce` 同样完整。**上轮结论复核成立，无新泄漏。**
2. **SWRR 权重推进并发**：`wrrWeights` 的写只发生在 `dispatcher_routing.go:110-113`（持 `wrrMu`），剪枝在 `dispatcher.go:87-99`（同锁），affinity 分支不写权重。`go test -race ./internal/router/` 干净。
3. **熔断三态迁移边界**：`closed→open`（连续失败达阈值）、`open→half_open`（cooldown 到，`circuit_breaker.go:108-114`）、`half_open→closed`（successThreshold=1，`:151-159`，计数在 `toClosed` 清零）、`half_open→open`（`:161-163`）；OPEN 下迟到的 success/failure 故意忽略（`:116-130` 有原因注释）。**未见错乱。观察项**：`Healthy()`（`:254-276`）未镜像 `halfOpenState.CanExecute` 的"2×cooldown 陈旧名额重置"（`:139-146`），只影响亲和选路时的可选性，不影响名额计数。
4. **重试语义**：跨渠道重试只发生在推理类请求（chat/embedding/rerank/图片音频），无副作用；body 每次 attempt 重建（`bytes.NewReader`），multipart 重写会**重新生成 boundary 并同步 Content-Type**（`multipart_model_rewrite.go:110`），无"body 已消费"问题；确定性 4xx 由 `IsFatalPayloadError` 中止 fan-out，404 才继续（`errors.go:119-143`）；429 不记熔断（`errors.go:76-96`）。
5. **请求预算**：`withRequestBudget`（`request_budget.go:60-70`）在已有 caller deadline 时不覆盖，递归 fallback 沿袭同一 deadline + marker；`DispatchHTTP` 把 budget cancel 挂到响应体 `Close`（`dispatcher_modality.go:56-87`），成功路径不会提前取消 body。未见重复计时或提前放行。
6. **流式首字 select 的 `attemptCtx.Done()` 分支**（`dispatcher_stream.go:152-158`）：`ctx.Err()!=nil` 时 `firstErr` 留空、循环多转一圈，但 `ctx.Done()` 同时候选、下一轮必然命中，不是死循环/忙等，不记缺陷。
7. **`ValidateModelChannel` 15s 超时**：`controlplane/admin_topology_probe.go:29` 已 `WithTimeout(15s)`，`routing_probe.go:273` 的文案不是虚报。

---

## 5. 探针清单与运行方式

```
# 全部探针（当前断言"错误行为"，所以是绿的；修复阶段翻转为正确断言并去掉 Bugprobe）
go test -run Bugprobe -v ./internal/model/ ./internal/provider/ ./internal/adapter/ ./internal/router/

internal/model/typed_content_bugprobe_test.go                    F-01 估算 99.6× + 根因
internal/model/response_reasoning_tokens_bugprobe_test.go        F-02 reasoning_tokens 0
internal/provider/typed_content_convert_bugprobe_test.go         F-01 图片变文本；F-04 tool_choice 丢失
internal/provider/gemini_embed_url_bugprobe_test.go              F-03 models/ 双前缀
internal/provider/anthropic_stream_cache_bugprobe_test.go        F-05 流式丢 cache_creation
internal/provider/url_image_bugprobe_test.go                     F-07 出站 URL 图片当 base64
internal/adapter/url_image_bugprobe_test.go                      F-07 入站 URL 图片被丢
internal/router/affinity_bugprobe_test.go                        F-06 mod-N 重映射 74.8%
```

质量门（本轮实测）：`go test -count=1 ./internal/{model,provider,adapter,router}/` 全 ok；`go test -race -count=1 ./internal/{router,adapter,provider}/` 全 ok；`go vet` 干净。**未改生产代码、未 commit。**

---

## 6. 修复记录（2026-10-29 修复阶段）

> 修复按 Lead 指定的顺序执行，**全部改动仅限本域**（`internal/{model,provider,adapter,router}` + `internal/api` 的流式 usage 相关文件），未 commit。探针已翻转为常规回归测试（去掉 `Bugprobe` 前缀、断言正确行为）。

### 6.1 执行顺序

1. **F-01**（P1，一处修全部）：`ParseMessageContent` 加 typed 分支；同时决定是否清存量校准（本轮线上 ratio 正常，倾向不清，只加回归防止首次带图流量投毒）。
2. **F-05 + F-02**（金额/协议，改动小）：provider 流式 usage 复用 `anthropicUsageToModel`；`api/*_stream_handler.go` 累积 `CacheCreationTokens`（需 api 域配合）；Responses 从 `Usage.Extra` 取 `reasoning_tokens`。
3. **F-04 + F-07**（协议能力补齐）：`tool_choice`↔`tool_choice`/`toolConfig`；图片 URL 形态按协议分流。
4. **F-03**（配置健壮性）：URL 层归一化 `models/` 前缀。
5. **F-06**（性能/成本）：亲和换 ring/rendezvous；这项改动面最大，建议单独提交并带"重映射率 ≤1/N+容差"回归。

**跨域归属**：F-05 的 api 侧累积属请求/流式域（01），F-02 的 `responses_stream.go` 也属该域；provider/model 侧都在本域，已在报告标注，请 Lead 分派时一并给到修复人以免只改一半。

### 6.2 逐条结果

| ID | 状态 | 关键实现 | 回归测试（翻转后） |
|---|---|---|---|
| **F-01** | 已修 | `model/multimodal.go`：`ParseMessageContent` 新增 `[]ContentPart` / `ContentPart` / `*ContentPart` / `[]*ContentPart` 分支（`normalizeContentParts` 浅拷贝并补全隐式 part 类型），不再走 JSON→文本回退；新增 `model.IsRemoteContentURL` 供图片分流 | `model/typed_content_test.go`：`TestParseMessageContentReusesTypedParts`、`TestTypedContentEstimateMatchesMapContent`（typed 与 `[]any` 估算**相等**，300KB 图 771 tokens）；`provider/typed_content_convert_test.go`：`TestConvertGeminiSendsTypedImageAsInlineData`、`TestConvertAnthropicSendsTypedImageAsImageBlock` |
| **F-07** | 已修 | 入站 `adapter/anthropic.go`：`source.type="url"` → `image_url`（不再整块丢弃）；`adapter/gemini.go`：`fileData.fileUri` → `image_url`。出站 `provider/anthropic_convert.go`：远程图 → `source.type="url"`；`gemini_convert.go`：远程图 → `fileData.fileUri`（新增 `GeminiFileData`），base64/data URI 仍走 `source.type=base64` / `inlineData` | `adapter/url_image_test.go`：`TestAnthropicAdapterKeepsURLSourceImage`、`TestGeminiAdapterKeepsFileDataURI`；`provider/url_image_test.go`：`TestAnthropicConvertForwardsRemoteImageAsURLSource`、`TestGeminiConvertForwardsRemoteImageAsFileData`（各含 data URI 不回归的子断言） |
| **F-04** | 已修 | `provider/anthropic_convert.go`：`anthropicToolChoice`（auto→`{type:auto}`、required/any→`{type:any}`、function→`{type:tool,name}`、none/未知→省略 + WARN）；`provider/gemini_convert.go`：`geminiToolConfig`（AUTO/ANY/NONE、function→ANY+`allowedFunctionNames`、未知→省略 + WARN）；`gemini_types.go` 新增 `toolConfig` 类型 | `provider/typed_content_convert_test.go`：`TestAnthropicToolChoiceMapping`、`TestGeminiToolConfigMapping`（覆盖 auto/any/tool/none/未知整张映射表）、`TestConvertersForwardToolChoice` |
| **F-05** | 已修 | `provider/anthropic_stream.go` + `usage_mapping.go`：抽出 `cacheTokenDetails(read, creation)`，unary 与 stream 共用（流式不再只带 `CachedTokens`）；`api/{chat,completions}_stream_handler.go`、`responses_stream.go`、`gemini_inbound_stream.go` 累积 `GetCacheCreationTokens()` 并传入 `AuditRecordParams.CacheCreationTokens` | `provider/anthropic_stream_cache_test.go`：`TestAnthropicStreamReportsCacheCreationTokens`（300 → 300）、`TestAnthropicStreamOmitsEmptyCacheBreakdown`；`api/stream_usage_regression_test.go`：`TestAnthropicStreamBillsCacheWriteAtPremium`（prompt=1000/creation=900 → cost **0.00253**；按普通输入价是 0.00208） |
| **F-02** | 已修 | `model/chat.go`：`Usage.GetReasoningTokens()` 解析 `Extra["completion_tokens_details"].reasoning_tokens`（nil/畸形安全返回 0）；`model/response.go` unary 与 `api/responses_stream.go` stream 均改用该值（流式按 `max` 累积） | `model/response_reasoning_tokens_test.go`：`TestResponsesReportsUpstreamReasoningTokens`（99→99 且 output_tokens 不重复计）、`TestResponsesReasoningTokensAbsentIsZero`、`TestUsageGetReasoningTokensHandlesMalformedExtra`；`api/stream_usage_regression_test.go`：`TestResponsesStreamReportsUpstreamReasoningTokens`（`response.completed.usage.output_tokens_details.reasoning_tokens=77`） |
| **F-03** | 已修 | `provider/api_url.go` 新增 `geminiModelPath`（剥 `models/` 前缀）；`gemini.go:buildGeminiURL`（generateContent + streamGenerateContent）与 `gemini_embed.go`（embedContent / batchEmbedContents 的 URL 与 body）统一走它 | `provider/gemini_embed_url_test.go`：`TestGeminiEmbedAcceptsModelsPrefixMapping`（`models/…` 与裸名两种映射 → URL `/v1beta/models/<id>:embedContent`、body `models/<id>`）、`TestGeminiBatchEmbedAcceptsModelsPrefixMapping`、`TestGeminiChatURLNormalisesModelsPrefix` |
| **F-06** | 已修 | `router/load_balancer.go`：`%N` 取模 → rendezvous（最高随机权重）哈希（`rendezvousPick` + FNV-1a 64 + splitmix64 终混），健康渠道优先、全不健康时仍返回最高分（保持原兜底语义）；Redis pin 逻辑不变；`dispatcher_routing.go` 注释同步为 rendezvous | `router/affinity_test.go`：`TestSessionAffinityRemapsFewSessionsOnChannelAdd`（3→4 **26.2%**、4→5 **20.6%**、4→3 26.2%，门槛 <40%；修复前 74.8%）、`TestSessionAffinityIsStickyAndSpread`、`TestSessionAffinitySkipsUnhealthyChannels` |

### 6.3 修复后的实测数字

- **F-01 金额**：同一 300 KB base64 图 + 一句文本，`[]any` 与 typed `[]ContentPart` 都估算 **771 tokens**（修复前 typed 76,829，约 100×）；不再污染 `observePromptEstimate`。
- **F-05 金额**：cache-write 900 tokens 的流式请求账单 **0.00253** vs 修复前按普通输入价的 0.00208（1.25× 缓存写溢价恢复）。
- **F-06 分布**：加/删 1 个渠道的会话重映射率 **26.2% / 20.6% / 26.2%**（理论 25% / 20% / 25%），修复前 3→4 为 **74.8%**（`%N` 取模）。
- **F-04 语义**：`tool_choice` 不再静默丢失；未知值省略并 WARN（映射表被测试钉住）。

### 6.4 质量门（修复后实测）

```
gofmt -l internal cmd web              → 空
go build ./...                         → OK
go vet ./...                           → OK
go test -count=1 ./...                 → 15/16 包 ok
go test -race -count=1 ./internal/{model,adapter,provider,router,api}
                                       → 5/5 包 ok（api 49.4s，无数据竞争）
```

唯一红的是 `internal/controlplane`（`TestChannelServiceBatchStatusRefusesUnknownStatus`）：该文件在本轮修复期间被 **controlplane 域**并发改写（`channel_service_test.go` 在 09:05:21 刚新增该用例、对应生产修复尚未落地），与本域改动无交集——本域五包在 `-race` 下全绿。

### 6.5 跨域交接（未在本轮改动）

1. **F-05 的 api 侧已接线**（4 个流式 handler 传 `CacheCreationTokens`），但 `internal/api/inbound_anthropic_stream.go` 之外的**非流式**估算不受影响：非流式 Anthropic 走 `anthropicUsageToModel`，本来就正确。
2. **F-02** 的 `responses_handler.go`（unary 落库/回包）在别域并发修改中；本域只改了 `model/response.go` 的转换与 `responses_stream.go` 的流式 usage，二者已覆盖 `/v1/responses` 的 unary + stream 两种形态。
3. **存量校准**：本轮线上 ratio 正常（≈1–2.4），未做清理；F-01 修复后首次带图流量不再投毒，无需迁移。
4. **`api/mcp_batch3_fix_test.go`**：该用例原先硬编码旧的 `%N` 哈希来挑选"能区分两租户"的 session，随 F-06 改为通过真实 dispatcher 路由计算期望渠道（语义仍是"同 session id 的不同租户必须隔离"），已 PASS。

**修复后仍建议下一轮关注**：`adapter/anthropic.go` 对 `document`/`search_result` 等非文本 block 仍是丢弃（未在两项发现范围内）；Gemini `fileData` 的 MIME 目前按 `image/jpeg` 兜底，非图片远程资源需要按 Content-Type 细化。
