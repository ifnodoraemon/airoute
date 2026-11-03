# 路由/渠道/协议适配一致性审计（2026-10-28）

## 概览

本次只读审计覆盖 `internal/router/`、`internal/provider/`、`internal/adapter/`、`internal/model/`（基线 commit `1520d99`），逐字段核对了 `ChannelConfig` 的 14 个字段、5 条 dispatch 路径（Chat 一元 / Stream / Embedding / Rerank / HTTP 多模态）与 3 个 provider 的一元↔流式实现对称性。

结论：本域的「死」几乎全部是**能力接了一半**——开关/字段被定义、被读取、被测过，但只在部分路径进入决策。最集中的三个根因是：(1) `GATEWAY_REQUEST_BUDGET_SEC` 与失败分类只在 Chat 路径成立，Embedding/Rerank/多模态是另一套较弱语义；(2) 熔断与指标记账在「一元 vs 流式」之间不对称（流式首 token 即记成功、中途失败永不入账；指标一元双计、流式单计）；(3) 同一份协议/摘要能力存在两套实现，未接线的那一套已经漂移（`adapter.AnthropicAdapter.FromCanonical`）。

`ChannelConfig` 字段本身没有「定义了但从不被读」的情况；`tags` / `capabilities` / `modalities` / `max_tokens` / `proxy` / 自定义 header / `rate_limit` 这些字段在 v1.1 根本不存在（管理 API `ChannelRequest` 与存储列 1:1，有契约测试守护），所以线索中相应项按「非缺口」列入排除区。

## 发现

### B-01 全局请求预算只接在 Chat 一元路径：Embedding / Rerank / 多模态完全没有总预算
- **类别**：半接线
- **位置**：`internal/router/request_budget.go:13`、`internal/router/dispatcher.go:217`、`internal/router/dispatcher_modality.go:75,247,324`
- **设计意图**：`request_budget.go:13-17` 把这个变量定义为「bounds how long a **non-streaming dispatch** may spend across channel attempts and cross-model fallback」，理由写得很清楚：「generous enough for the default timeout chain (channel timeout × candidate channels) while still bounding a request that would otherwise wait for every candidate to time out」。这个动机对 embedding / rerank / 多模态同样成立。
- **现状**：`withRequestBudget` 全仓只有 `Dispatch` 一处调用。`DispatchStream` 不接有注释说明是故意的（流式靠首 token 窗口，见排除区），但 `DispatchEmbedding` / `DispatchRerank` / `DispatchHTTP` 三条非流式路径没有任何替代机制，最坏耗时仍是 `候选渠道数 × 渠道超时`（默认 60s×N，可配置到 86340s×N）。HTTP 层为「预算耗尽」专门造的 504 语义因此也只可能由 Chat 一元触发。
- **证据**：
  ```go
  // internal/router/dispatcher.go:216-218  —— 唯一接预算处
  // budget because it derives its context from this one.
  ctx, cancelBudget := withRequestBudget(ctx)
  defer cancelBudget()
  ```
  ```go
  // internal/router/dispatcher_modality.go:246-248  —— 兄弟路径无预算
  func (d *Dispatcher) DispatchEmbedding(ctx context.Context, req *model.EmbeddingRequest) (*model.EmbeddingResponse, error) {
  	channels := d.ResolveCandidateChannels(ctx, req.Model, "embeddings")
  ```
  ```go
  // internal/api/common.go:775-776  —— 只有 Chat 一元能走到的 504 语义
  if errors.Is(err, router.ErrRequestBudgetExceeded) {
  	return http.StatusGatewayTimeout, "gateway_error", "request_budget_exceeded", channel
  ```
- **影响**：中 — 多渠道路由下 embedding/rerank/图片/音频请求可能串行等满每个渠道的超时，客户端先于网关放弃，且无法用 `GATEWAY_REQUEST_BUDGET_SEC` 收敛。
- **置信度**：高
- **建议**：在 `DispatchEmbedding` / `DispatchRerank` / `DispatchHTTP` 入口同样包一层 `withRequestBudget`，并在各自循环起点检查 `requestBudgetExceeded`（HTTP 流式 body 需沿用 `cancelOnCloseReadCloser` 的释放时机）。

### B-02 流式路径首 token 即记熔断成功，中途失败永不入账（一元路径则逐次记账）
- **类别**：不对称强制
- **位置**：`internal/router/dispatcher_stream.go:170`、`internal/router/dispatcher_stream.go:208-238`、`internal/router/dispatcher_fallback.go:129-138`
- **设计意图**：熔断器按「上游是否健康」记账，一元路径对每次失败都 `RecordFailure`（`recordChannelFailure`），失败分类覆盖 401/403/408/5xx/网络错误。同一份健康信号理应在流式路径同样落账。
- **现状**：流式在**拿到第一个有效 token 的瞬间**就 `RecordSuccess`，随后 `pipeStreamEvents` 只做搬运：上游中途断流、SSE 内嵌 error 帧、`ctx` 超时等都只变成 `StreamEvent.Err` 交给 API 层落审计行，**不再回写熔断器**。也就是说「先吐 1 个 token 再必然断流」的渠道永远 CLOSED，而同样行为的渠道在一元路径上三次就会 TRIPPED。首 token 前的失败走 `failStreamAttempt` 记账是对的，断点正好在「提交之后」。
- **证据**：
  ```go
  // internal/router/dispatcher_stream.go:170-173  —— 提交即记成功
  d.circuitBreaker.RecordSuccess(ch.Name)
  trail := formatChannelTrail(ctx, attempted, ch.Name)
  outChan := make(chan *model.StreamEvent, 64)
  go pipeStreamEvents(ctx, outChan, preamble, streamChan, trail, attemptCancel)
  ```
  ```go
  // internal/router/dispatcher_stream.go:223-230  —— 中途失败只搬运，不记账
  case event, ok := <-in:
  	if !ok {
  		return
  	}
  	if event == nil {
  		continue
  	}
  	event.Channel = trail
  ```
  ```go
  // internal/router/dispatcher_fallback.go:129-131  —— 一元路径的对照实现
  func (d *Dispatcher) recordChannelFailure(ctx context.Context, chName string, err error, attempt int) string {
  	d.circuitBreaker.RecordFailure(chName)
  	telemetry.GlobalMetrics.RecordFallback()
  ```
- **影响**：中 — 「能连通、能出首 token、但长输出必断」的坏渠道对熔断器不可见（这类故障恰恰是长连接上游最常见的劣化形态），保护只在短请求上生效。
- **置信度**：高
- **建议**：`pipeStreamEvents` 在收到 `event.Err != nil` 或非正常 EOF 时回调一个 `d.recordStreamFailure(chName, err)`（沿用 `provider.IsChannelFailure` 分类），并考虑把已提交流的成功记账延后到 `IsDone`。

### B-03 404 的失败分类三套规则：Chat 继续下一渠道，Embedding/Rerank/多模态直接终止
- **类别**：重复漂移
- **位置**：`internal/router/dispatcher.go:205-210,314-324`、`internal/provider/errors.go:112-121`、`internal/router/dispatcher_modality.go:295-298,369-372,205-225`
- **设计意图**：Chat 路径把这条规则写成了显式注释与判据：只有 400/422/413 是「换渠道也会失败」的确定性客户端错误；**404 表示这个节点没托管该模型，应当继续下一个候选渠道**（`dispatcher.go:316`）。
- **现状**：Embedding/Rerank 用的是另一套 `IsClientError`，它对「4xx 且非 401/403/408/429」一律返回 true —— **包含 404**，于是第一个回 404 的渠道会直接终止整个 fan-out 并把 404 抛给客户端，本可服务该模型的其他渠道一次都不试。多模态 `DispatchHTTP` 更粗：任何 4xx 都走「确定性客户端错误」分支原样透传，不做换渠道重试。三条路径对同一个上游条件给出三种处置。
- **证据**：
  ```go
  // internal/router/dispatcher.go:316  —— chat 的规则
  // Note: 404 from a single channel means this node does not host the model; we fall through to next channels.
  ```
  ```go
  // internal/provider/errors.go:119-120  —— embedding/rerank 的规则
  return ue.Status >= 400 && ue.Status < 500 && ue.Status != 401 && ue.Status != 403 && ue.Status != 408 && ue.Status != 429
  ```
  ```go
  // internal/router/dispatcher_modality.go:295-298  —— 404 直接终止
  if provider.IsClientError(err) {
  	d.circuitBreaker.AbortCanary(ch.Name)
  	return nil, err
  }
  ```
- **影响**：中 — 同一 embedding/rerank 模型按节点分散部署（部分节点 404）时，请求命中错误节点即失败，容灾能力与 Chat 不等价。
- **置信度**：高
- **建议**：抽一个共享的 `isFatalClientPayloadError`（400/413/422，404 明确排除）供四条路径复用，`DispatchHTTP` 的 4xx 分支同样先判 404 再决定是否换渠道。

### B-04 请求指标被两层各记一次：一元请求双计、流式单计
- **类别**：重复漂移
- **位置**：`internal/router/dispatcher_fallback.go:141-150`、`internal/router/dispatcher_stream.go:170-174`、`internal/api/common.go:242-255`、`internal/telemetry/metrics.go:155-206`、`internal/telemetry/metrics_prometheus.go:54-56`
- **设计意图**：`RecordRequest` 与 `RecordRequestWithModel` 其实是同一个函数（前者固定 `model=""`），写入同一组 `TotalRequests / SuccessfulRequests / FailedRequests` 与同一份延迟直方图。指标只应有一个写入者。
- **现状**：一元 Chat / Embedding / Rerank / 多模态成功时，router 的 `recordChannelSuccess` 记一次（模型空、仅上游耗时），API 层 `RecordUsage` 再记一次（真实模型、端到端耗时）；失败时 router 的 `RecordRequest(false,...)` 与 API 的 `recordFailedRequest*` 同样双记。**唯独流式路径**（`dispatcher_stream.go` 内联记 breaker + trail，刻意不调 `recordChannelSuccess`）只在 API 层记一次。结果：`airoute_requests_total` ≈ 2×一元 + 1×流式，`/models` 维度的 per-model 计数却只来自 API 层，两个视图对不上；P50/P90/P99 每两条样本里掺一条「仅上游耗时」。
- **证据**：
  ```go
  // internal/router/dispatcher_fallback.go:142-144  —— 第 1 次
  d.circuitBreaker.RecordSuccess(chName)
  trail := formatChannelTrail(ctx, attempted, chName)
  telemetry.GlobalMetrics.RecordRequest(true, dur, promptTokens, compTokens)
  ```
  ```go
  // internal/telemetry/metrics.go:204-206  —— 同一个计数器
  func (m *Metrics) RecordRequest(success bool, dur time.Duration, promptTokens, completionTokens int) {
  	m.RecordRequestWithModel("", success, dur, promptTokens, completionTokens)
  }
  ```
  ```go
  // internal/api/common.go:249  —— API 层第 2 次（RecordUsage 内）
  telemetry.GlobalMetrics.RecordRequestWithModel(params.Model, success, params.Duration, ...)
  ```
- **影响**：中 — Prometheus 的请求量/成功率/延迟分位数对一元流量系统性翻倍，且流式与一元口径不同；容量评估与告警阈值都会偏。
- **置信度**：高
- **建议**：指标写入收敛到 API 层（router 只保留熔断与 fallback 计数），或在 router 侧改记独立的「上游尝试」计数器。

### B-05 `adapter.AnthropicAdapter.FromCanonical` 从未接线，且规则已与在用实现漂移
- **类别**：死接线（含重复漂移）
- **位置**：`internal/adapter/anthropic.go:227-300`、`internal/api/inbound_anthropic.go:125-215`
- **设计意图**：`adapter` 包的 `ProtocolAdapter` 双向契约是「入站 `ToCanonical` + 出站 `FromCanonical`」，`init()` 里注册了 anthropic 与 gemini 两个实现，显然打算让两个方向的协议转换都走这层（`adapter.go:40-43`）。
- **现状**：入站方向确实走了（`ConvertAnthropicToCanonical`），gemini 的出站方向也走了（`convertCanonicalToGeminiResponse` 调 `FromCanonical`），**只有 anthropic 的 `FromCanonical` 没有任何调用者**；`handleAnthropicUnary` 自己手写了 90 行等价转换。两套实现已经分叉：未接线的那套会为 `ReasoningContent` 合成一个 `{"type":"thinking"}` 块（`adapter/anthropic.go:237-242`），而在用实现明确注释「绝不能合成无签名 thinking 块，否则下一轮请求会被 Anthropic 以 `thinking.signature: Field required` 拒绝」（`inbound_anthropic.go:130-137`）；`usage` 也未扣减 cache read、`id` 未补 `msg_` 前缀。谁把这个方法接上，就会立刻把已经修好的 bug 复现一遍。
- **证据**：
  ```go
  // internal/adapter/anthropic.go:237-241  —— 未接线实现仍在合成 thinking 块
  if choice.Message.ReasoningContent != "" {
  	contentBlocks = append(contentBlocks, map[string]any{
  		"type":     "thinking",
  		"thinking": choice.Message.ReasoningContent,
  	})
  }
  ```
  ```go
  // internal/api/inbound_anthropic.go:130-133  —— 在用实现禁止这么做
  // Do NOT synthesise a {"type":"thinking"} block from ReasoningContent.
  // The Anthropic Messages API requires every thinking block to carry a
  // provider signature, ... an unsigned block therefore makes the follow-up request fail
  ```
  ```go
  // internal/adapter/adapter_test.go:328-333  —— 测试还把「错误契约」钉死了
  if blocks[0]["type"] != "thinking" || blocks[0]["thinking"] != "Thinking step 1... Done." {
  	t.Fatalf("unexpected thinking block: %+v", blocks[0])
  }
  if blocks[1]["type"] != "text" || blocks[1]["text"] != "Final answer." {
  ```
- **影响**：低（当前不可达）— 但属于「误接即回归」的陷阱型死代码，且架构文档要求的适配器对称性实际只有一半成立。
- **置信度**：高
- **建议**：让 `handleAnthropicUnary` 改走 `adapter.Get("anthropic").FromCanonical` 并把两处规则合并（含 thinking 禁令、cache 扣减、`msg_` 前缀），或删除该实现并收窄接口。

### B-06 `/v1/responses` 入口不设 Protocol，`openai_response` 渠道协议在决策点永不生效
- **类别**：半接线
- **位置**：`internal/model/channel.go:52-58`、`internal/api/responses_handler.go:104-131`、对照 `internal/api/completions_handler.go:127`
- **设计意图**：`SupportsProtocol` 专门为 Responses 写了别名表（`response/responses/openai_response/openai_responses` 互相等价，且「Upstream chat providers can seamlessly adapt to openai_response」），`docs/PROVIDER_GUIDE.md:87` 把 `openai_response` 列为渠道可声明协议，`topology.go:97-99` 还据此展示「OpenAI Chat ⇄ Responses 双向转译支持」。这套规则的唯一用途就是被 Responses 请求查到。
- **现状**：Chat 入口不设 `Protocol`（天然 "chat"）、Completions 设 `"openai_text"`、Messages 设 `"anthropic_messages"`，**唯独 Responses 入口把 `ChatCompletionRequest.Protocol` 留空**，于是 `Dispatch` 里 `proto` 落到 `"chat"`，别名表 52/56 两条分支永远查不到。后果：模型同时有 `protocols:["openai_chat"]` 与 `protocols:["openai_response"]` 两个渠道时，Responses 请求只会命中前者；后者被整体排除（`ResolveCandidateChannels` 的 proto="" 兜底只在「一个都没匹配」时才触发），其权重/优先级对 Responses 流量完全无效。`tests/openai_responses_test.go:140` 之所以能过，正是靠「单渠道 → 落空 → 兜底」这条偶然路径。
- **证据**：
  ```go
  // internal/model/channel.go:52,56  —— 为 responses 写的别名规则
  if (p == "response" || p == "responses" || p == "openai_response" || p == "openai_responses") && (proto == "response" || ...) {
  // Upstream chat providers can seamlessly adapt to openai_response
  if (p == "chat" || p == "openai_chat") && (proto == "response" || proto == "responses" || ...) {
  ```
  ```go
  // internal/api/completions_handler.go:127  —— 兄弟入口显式声明
  Protocol:    "openai_text",
  ```
  ```go
  // internal/api/responses_handler.go:104-113  —— Responses 入口没有等价赋值
  sessionID := sanitizeSessionID(req.PreviousResponseID)
  ...
  reqCtx, cancelUpstream := upstreamRequestContext(c)
  ```
- **影响**：中 — 声明了 `openai_response` 的渠道对 Responses 流量形同不存在（除非它是该模型唯一渠道），控制台却按「双向转译支持」展示；协议声明与实际选路不一致。
- **置信度**：高
- **建议**：在 Responses 入口设 `chatReq.Protocol = "responses"`（并为 Gemini 入口明确 `"gemini"`/`"chat"` 的取舍），让 `SupportsProtocol` 的别名表真正参与选路。

### B-07 `/v1/messages/count_tokens` 绕过熔断器：既不查 `CanExecute`，也不记失败
- **类别**：半接线
- **位置**：`internal/api/inbound_anthropic.go:253-312`、对照 `internal/router/dispatcher.go:258` / `dispatcher_modality.go:96,257,334`
- **设计意图**：路由决策处注释反复强调熔断是「fail fast if upstream is down」的准入闸门（`dispatcher.go:257-264`、`CircuitBreaker.CanExecute` 的 WARNING 注释），所有上游调用都应经过它；`ResolveCandidateChannels` 还被特意标注「选择器会改共享状态」。
- **现状**：count_tokens 是唯一一条**完全绕过 Dispatcher** 的数据面上游调用：它先调 `ResolveCandidateChannels`（会推进 SWRR 权重、消费亲和选择）拿到候选，然后自己 `http.NewRequestWithContext(c.Request.Context(), ...)` + `provider.SharedDefaultHTTPClient.Do` 直发。于是 (a) 熔断 OPEN 的渠道仍会被打；(b) 失败既不 `RecordFailure` 也不 `AbortCanary`（半开探针若在此路径被消费就永久漏计）；(c) 成功也不计入健康统计。渠道超时缺失是被文档承认的（见排除区），熔断这一半没有。
- **证据**：
  ```go
  // internal/api/inbound_anthropic.go:253-262  —— 取了候选，但不走调度器准入
  channels := h.dispatcher.ResolveCandidateChannels(c.Request.Context(), req.Model, "anthropic_messages")
  var lastErr error
  for _, channel := range channels {
  	if !router.IsAnthropicWireChannel(channel) {
  		continue
  	}
  ```
  ```go
  // internal/api/inbound_anthropic.go:298  —— 直连共享客户端
  resp, err := provider.SharedDefaultHTTPClient.Do(httpReq)
  ```
- **影响**：低 — count_tokens 不计费、体量小；但它是数据面唯一的熔断盲区，且失败无法把坏渠道推进 OPEN。
- **置信度**：中
- **建议**：在循环内补 `CanExecute` / `RecordFailure` / `RecordSuccess` / `AbortCanary`（或为「直连型辅助上游调用」抽一个统一的 `upstreamDoBounded(ch, ...)` 包装，把预算+熔断+审计一次接全）。

### B-08 多模态：入站改写 model，出站 JSON 响应不回写，别名还原只覆盖结构化路径
- **类别**：不对称强制
- **位置**：`internal/router/dispatcher_modality.go:104-125,227-236`、对照 `internal/provider/openai_embed.go:67-68`、`internal/api/handler.go:145-147`、`internal/api/chat_stream_handler.go:201-203`
- **设计意图**：模型别名必须对下游透明：请求侧把别名改写成上游真名（JSON 走 `RewriteJSONModel`、multipart 走 `RewriteMultipartModel`），响应侧再改回请求名。Chat 一元、Chat 流式、Embedding、Rerank 都成对实现了这一点（并各有明确注释「Restore user's requested model in the response」）。
- **现状**：`DispatchHTTP` 只做了请求侧改写，响应体按字节原样透传（`multipart`/图片/音频/视频 JSON 都在 handler 里 `io.Copy`），**没有任何 model 还原环节**。视频路径只替换 `id`/`task_id`（`video_handler.go:158-171`），不碰 `model`。上游若在响应里回显自己的模型名（视频任务 JSON 很常见），下游看到的就是映射后的上游名而不是请求名；Chat/Embed/Rerank 则不会。
- **证据**：
  ```go
  // internal/router/dispatcher_modality.go:110-110  —— 只有请求侧
  targetBody = RewriteJSONModel(targetBody, targetModel)
  ```
  ```go
  // internal/router/dispatcher_modality.go:227-236  —— 成功即原样透传响应体
  trail := d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), 0, 0)
  return &UpstreamResponse{
  	StatusCode: httpResp.StatusCode,
  	ProviderName: ch.Name, ChannelID: ch.ID, BaseURL: ch.BaseURL,
  	Headers: httpResp.Header,
  	Stream:  &cancelOnCloseReadCloser{ReadCloser: httpResp.Body, cancel: cancelCall},
  ```
  ```go
  // internal/provider/openai_embed.go:67-68  —— 兄弟路径的还原承诺
  // Restore user's requested model in the response
  embResp.Model = req.Model
  ```
- **影响**：低 — 影响面限于响应体回显 model 的多模态上游；但对「别名透明」这一已声明的对外契约是一处缺口，也让下游按 model 归类计费/统计时拿到两套名字。
- **置信度**：中
- **建议**：给 `UpstreamRequest` 增加 `RestoreModel bool`，在 `DispatchHTTP` 成功分支对 JSON 响应做一次对称的 model 回写（或在 handler 的 transform 里统一处理）。

### B-09 拓扑页把「优先级 1」硬编码为主渠道层，与实际调度器「最小 priority 为第一层」不一致
- **类别**：重复漂移
- **位置**：`internal/router/topology.go:101-105,126-134`、`internal/router/dispatcher_routing.go:83-116`
- **设计意图**：`docs/ROUTING_TOPOLOGY.md` 的规则是「低数字优先级先尝试，同层按配置权重选路」——主/备层级由**相对最小值**决定，拓扑页应当展示调度器的实际分层。
- **现状**：调度器按 priority 值分组建 map、`sort.Ints` 后 `groupIdx == 0` 即第一层（享受会话亲和）；拓扑/快照则把 `Priority > 1` 当备用层、`Priority == 1` 且 active 才计入 `TotalPrimaryWeight`，权重占比只在 `info.Priority == 1` 时计算。当运维把所有渠道的优先级都设为 2（或 3）时，调度器照常把它们当主层用，控制台却显示 `primary_providers_count = 0`、全部落到 `fallback_providers_count`、`total_primary_weight = 0`、每个渠道 `weight_percent = 0`。
- **证据**：
  ```go
  // internal/router/topology.go:101-105  —— 硬编码 1
  if ch.Priority > 1 {
  	hasFallbackTier = true
  } else if ch.Status == "active" {
  	totalP1Weight += ch.Weight
  }
  ```
  ```go
  // internal/router/topology.go:127-133  —— 权重占比同样硬编码 1
  if info.Priority == 1 && totalP1Weight > 0 {
  	pct := (float64(info.Weight) / float64(totalP1Weight)) * 100
  ```
  ```go
  // internal/router/dispatcher_routing.go:93-104  —— 调度器用的是相对最小
  sort.Ints(priorities)
  for groupIdx, p := range priorities {
  	group := priorityGroups[p]
  	if groupIdx == 0 && sessionID != "" { ... }
  ```
- **影响**：低 — 仅观感/排障误导（主备标错、权重占比全 0），不影响真实选路；但「容灾拓扑」页正是运维判断主备关系的唯一入口。
- **置信度**：高
- **建议**：拓扑层复用与调度器同一套分层定义（先求最小 priority 作为主层），把 `Priority == 1` 的比较改成与 `minPriority` 比较。

### B-10 `errors.go` 的规格注释称 429 计入渠道失败，代码实际排除 429
- **类别**：重复漂移（注释契约不符）
- **位置**：`internal/provider/errors.go:68-90`
- **设计意图**：`IsChannelFailure` 是熔断/换渠道的唯一分类器，注释是它的规格说明；`docs/ENGINEERING_GUIDELINES.md` §5.7 明确「注释是契约的一部分，发现不符的注释应按缺陷处理」。
- **现状**：注释写「5xx **and 429**: yes (upstream overloaded or broken)」，但 switch 里只有 401/403/408 与 `>= 500`，429 落入 `default: return false`。真实行为（429 不计熔断、只换渠道）与另一处注释（`dispatcher_modality.go:178-182` 解释「429 是健康方的背压，之前把它算作渠道失败导致三次限流就打不开渠道」）一致，说明**行为是对的、`errors.go` 的规格说明是旧的**。这类漂移会让后来者按注释「修复」代码，把已经修好的问题改回去。
- **证据**：
  ```go
  // internal/provider/errors.go:71-74  —— 注释
  //   - 5xx and 429: yes (upstream overloaded or broken)
  //   - 401/403: yes, the channel credential is broken
  ```
  ```go
  // internal/provider/errors.go:81-89  —— 代码
  case ue.Status == 401 || ue.Status == 403:
  	return true
  case ue.Status == 408:
  	return true
  case ue.Status >= 500:
  	return true
  default:
  	return false
  ```
- **影响**：低 — 无运行时行为问题，属维护性陷阱（本项目已有同类注释事故先例）。
- **置信度**：高
- **建议**：把注释改为「5xx / 401 / 403 / 408 / 网络错误计入；429 只换渠道不计入」，并补一条 429 单测钉死该语义。

### B-11 跨模型兜底（model fallback）只对 Chat/Stream 生效，其它模态配置了也不会被读
- **类别**：半接线
- **位置**：`internal/router/dispatcher.go:226,348`、`internal/router/dispatcher_stream.go:50,183`、`internal/router/dispatcher_modality.go:247,324,75`、`internal/router/routing_snapshot.go:66-83`
- **设计意图**：`SetModelFallbacks` 是全局 map，`/api/v1/admin/models/routes` 对任意模型都接受 `fallback_model`；`docs/ROUTING_TOPOLOGY.md` 承诺「按实际优先级展示全部渠道及跨模型兜底链」，`GetRoutingSnapshot` 对**每一个**模型都渲染 fallback 链，不区分模态。
- **现状**：`GetModelFallback` 只有 4 处调用，全在 Chat 一元/流式路径。Embedding / Rerank / 多模态在候选为空或全部失败时直接 `newNoRouteError` / `newExhaustedError`，从不查兜底表。于是给 embedding 模型配了 `fallback_model` 的操作者，在控制台看到一条生效的兜底链，实际请求永远不会走。
- **证据**：
  ```go
  // internal/router/dispatcher.go:226  —— 只有 chat 一元查兜底
  if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
  ```
  ```go
  // internal/router/dispatcher_modality.go:249-251  —— embedding 无兜底
  if len(channels) == 0 {
  	return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for embedding model '%s'", req.Model))
  ```
  ```go
  // internal/router/routing_snapshot.go:67,74-76  —— 任意模型都画出兜底链
  step := RoutingStep{Model: current, ..., FallbackModel: fallbacks[current], ...}
  if step.FallbackModel == "" { break }
  ```
- **影响**：低/中 — 配置项存在但部分模态静默无效，容器排障时「兜底不生效」难以定位。
- **置信度**：中
- **建议**：要么在 Embedding/Rerank/HTTP 的 no-route 与 exhausted 分支复用同一段兜底逻辑，要么在路由配置 API 与拓扑页按模态标注「仅对话生效」。

### B-12 `openai_text` 流式适配没有接 reasoning 标签剥离，一元却接了（同一能力两条路径规则不同）
- **类别**：半接线
- **位置**：`internal/provider/openai.go:141,223`、`internal/provider/openai_text_adapter.go:155,159-289`
- **设计意图**：`reasoning_adapter.go` 的职责注释是「把上游内联的 ` thinking…<｜end▁of▁thinking｜>` 文本从 content 拆到 reasoning_content」，`ENGINEERING_GUIDELINES.md` §4「架构对称性」明确要求一元↔流式同构。OpenAI chat 一元（`SeparateReasoningInResponse`）与 chat 流式（`NewReasoningStreamAdapter`）都接了，text 适配一元也接了。
- **现状**：`chatCompleteTextStream`（`openai_text` 渠道的流式）既没有 `NewReasoningStreamAdapter`，也没有任何标签处理，直接把 `choices[].text` 塞进 `Delta.Content`。同一个 text-completion 上游：非流式请求把思考链拆到 `reasoning_content`，流式请求把 ` thinking` 原样吐给客户端 —— 客户端按 `reasoning_content` 渲染思考过程时，流式与非流式表现不一致。
- **证据**：
  ```go
  // internal/provider/openai_text_adapter.go:155  —— 一元接了
  SeparateReasoningInResponse(&chatResp)
  ```
  ```go
  // internal/provider/openai_text_adapter.go:262-270  —— 流式没接，裸文本直出
  choices = append(choices, model.ChunkChoice{
  	Index: ch.Index,
  	Delta: model.ChunkDelta{
  		Role:    "assistant",
  		Content: ch.Text,
  	},
  ```
  ```go
  // internal/provider/openai.go:223  —— chat 流式的对照（接了）
  reasoningAdapter := NewReasoningStreamAdapter()
  ```
- **影响**：低 — 仅影响声明 `openai_text` 的上游 + 内联标签式推理模型，但正是「一元↔流式必须对称」这条明文约束的反例。
- **置信度**：高
- **建议**：`chatCompleteTextStream` 复用 `NewReasoningStreamAdapter`（与 `chatCompleteStream` 同一段 ProcessChunk/Flush 逻辑），把该处理提炼成两条流式路径共用的 helper。

### B-13 moderation 用「判空」跑了一次选择器，DispatchHTTP 又跑一次：SWRR 权重每条请求被推进两次
- **类别**：死接线（算出来但不用于决策）
- **位置**：`internal/api/moderation_handler.go:83`、`internal/api/moderation_handler.go:95`、`internal/router/dispatcher_routing.go:104-115`
- **设计意图**：`GetChannelsForModel...` 家族里，`ListChannelsForModel` 的注释明确区分了两种用途：「Routing selectors mutate shared state (SWRR weights, half-open canary slots), so introspection endpoints ... must use this variant」，即凡是**不使用选路结果**的调用都必须走只读变体。
- **现状**：moderation handler 先用 `GetChannelsForModelAndProtocol`（会跑 `SessionAffinitySelector`/`SWRRSelector`，推进平滑加权轮询状态）只为拿到 `len(channels) > 0` 这个布尔，紧接着 `DispatchHTTP` 内部又调 `ResolveCandidateChannels` 再选一次。第一次的选路结果被丢弃，但 SWRR 的 `s.weights[...] += w; winner -= total` 副作用已经发生。多渠道路由的 moderation 模型下，同一请求对同一模型的轮询计数被推两格，实际流量分配会相对其它入口偏移。
- **证据**：
  ```go
  // internal/api/moderation_handler.go:83-84  —— 只要布尔
  channels := h.dispatcher.GetChannelsForModelAndProtocol(modModel, "moderation")
  if len(channels) > 0 {
  ```
  ```go
  // internal/api/moderation_handler.go:95  —— 再选一次
  resp, err := h.dispatcher.DispatchHTTP(reqCtx, &router.UpstreamRequest{...})
  ```
  ```go
  // internal/router/dispatcher_routing.go:110-114  —— 选择器确实改状态
  d.wrrMu.Lock()
  selector := NewSWRRSelector(d.wrrWeights)
  selected := selector.Select(ctx, group, modelName, sessionID)
  d.wrrMu.Unlock()
  ```
- **影响**：低 — 只影响 moderation 入口的多渠道权重分布（该入口流量小、模型多为单渠道）。
- **置信度**：高
- **建议**：把判空改成 `ListChannelsForModel(modModel, "moderation")`（只读变体），或直接省掉前置检查、由 `DispatchHTTP` 的 no-route 错误决定响应。

## 已排除的疑似项

- **`GATEWAY_REQUEST_BUDGET_SEC` 未接入流式**：`request_budget.go:13-16` 明确写了「Streaming keeps the first-token timeout instead: a total deadline would cut long generations that are working as intended」——故意，已排除（流式另有真缺口，见 B-02）。
- **count_tokens 没有渠道超时预算**：`provider/client.go:13-19` 明确把 count_tokens 归为「auxiliary endpoints」并用 `DefaultDirectCallTimeout`（10min）兜底，属已记录的设计；本报告只报其熔断缺口的另一半（B-07）。
- **`topology.go` / `routing_snapshot.go` / `GetModelRoutes` 的多数字段不参与选路**：设计上就是「advisory / 只给 admin 展示」，`ROUTING_TOPOLOGY.md` 与各文件注释均说明其不得运行选择器；`ListChannelsForModel` / `Snapshot` / `Healthy` 的只读约定经核对全部被遵守（未发现误用 `CanExecute` 的读路径）。仅 B-09 的层级口径是真漂移。
- **`ChannelConfig` 没有 `tags` / `capabilities` / `modalities` / `max_tokens` / `proxy` / 自定义 header / `rate_limit` 字段**：v1.1 中这些字段根本不存在；管理 API 的 `ChannelRequest`（`controlplane/admin_channels.go:63-75`）与存储列 1:1 且有契约测试守护，属「非缺口」而非死接线。14 个实际字段（`Name/Type/BaseURL/APIKey/Models/ModelMapping/Protocols/Priority/Weight/TimeoutSeconds/Status/ID/RouteRules`）逐个核对：**全部在决策点被读**，无「定义未读」项。
- **`breaker_snapshot.go` 的字段（`RecoveryProbe`/`CooldownRemainingMs`/`CanExecute`）看起来没人用于决策**：唯一消费者是 admin 拓扑快照，且 `Snapshot` 是纯只读（不消费探针），与 `CanExecute` 的 mutating 语义刻意分开，正确。
- **advisory 模式（`GATEWAY_BREAKER_ENABLED=false`）一致性**：`CanExecute`/`Healthy`/`RecordSuccess`/`RecordFailure`/`Snapshot` 五个入口都处理了 `disabled`，且 `RecordFailure` 会记账但仍放行，与 `breaker_advisory_test.go` 的规格一致。
- **anthropic 流式 usage 与一元 usage 的「两套实现」**：`anthropic_stream.go:340-349` 内联复刻了 `usage_mapping.go:14-19` 的公式（input+cache_read+cache_creation、cached 只有 >0 才填）。今天两边规则完全相同，属重复但未漂移，不记缺陷（若日后改口径需同时改两处，建议顺手复用 `anthropicUsageToModel`）。
- **三个 provider 的 usage 映射与模型名还原对称性**：openai 走 `model.Usage.UnmarshalJSON`、anthropic 走 `anthropicUsageToModel`、gemini 走 `geminiUsageToModel`，三者一元↔流式都被调用；gemini/openai 的 `resp.Model = req.Model`、chunk 的 `Model: req.Model` 都做了还原。未发现缺项（多模态除外，见 B-08）。
- **`api_url.go` 各 provider 是否一致**：`APIURL` / `BuildAnthropicURL` / `joinAPIURLVersioned` 共享同一套「版本段只写一次、裸 origin 补版本、整段 endpoint 不重复拼接」规则，嵌入/rerank/多模态都走它；未发现拼接漂移。（越界观察：`controlplane/admin_routes_probe.go:43` 的 `strings.TrimRight(BaseURL,"/") + "/models"` 自行拼 URL、绕过了这套规则，属 controlplane 域，仅记录不算本报告发现。）
- **灰度/canary 能力**：`p2_dispatch_canary_test.go` 验证的是**半开熔断探针**（`CanaryInFlight`、`AbortCanary`），不存在流量灰度开关；`AbortCanary` 在四条主路径的「非渠道失败/取消/不支持的 provider」分支都已补齐，未发现泄漏点。
- **`InferModality` 按模型名猜模态用于选路**：它只用于 `ValidateModelChannel` 的「仅支持对话模型」闸门与拓扑展示，不参与 dispatch 选路，符合注释。
- **Responses 流式 token 估算硬编码（prompt=10）**：`.opencode/memory/project.md` 已登记为已知待办，且在 `internal/api` 域，不重复报。
- **`ValidateModelChannel` / `ProbeModelRoute` 两套「验证模型」实现语义不同**（前者真调 ChatComplete、后者 GET /models）：前者在 router 域且行为与 `ROUTING_TOPOLOGY.md` 描述一致；后者的 URL 拼接问题归 B 报告范围外（controlplane）。

## 覆盖面

- **基线**：`1520d99`（`git log --oneline -1` 已确认；工作区另有并行产物，未涉及本域）。
- **逐行读过**：`internal/router/` 的 `dispatcher.go`、`dispatcher_routing.go`、`dispatcher_stream.go`、`dispatcher_fallback.go`、`dispatcher_modality.go`、`load_balancer.go`、`circuit_breaker.go`、`circuit_breaker_config.go`、`breaker_snapshot.go`、`request_budget.go`、`routing_probe.go`、`routing_snapshot.go`、`topology.go`、`errors.go`；`internal/provider/` 的 `provider.go`、`client.go`、`api_url.go`、`errors.go`、`usage_mapping.go`、`reasoning_adapter.go`、`openai.go`、`openai_text_adapter.go`、`openai_embed.go`、`anthropic.go`、`anthropic_stream.go`、`gemini.go`、`gemini_stream.go`、`gemini_embed.go`；`internal/adapter/` 的 `adapter.go`、`anthropic.go`（`gemini.go` 结构与调用点核对）；`internal/model/` 的 `channel.go`、`routing.go`、`chat.go`、`usage.go`、`tokens.go`、`multimodal.go`。
- **调用链核对（只读，跨域取证）**：`internal/api/` 的 `handler.go`、`chat_stream_handler.go`、`responses_handler.go`、`responses_stream.go`、`completions_handler.go`、`completions_stream_handler.go`、`inbound_anthropic.go`、`inbound_anthropic_stream.go`、`gemini_inbound.go`、`gemini_inbound_stream.go`、`embedding_handler.go`、`rerank_handler.go`、`multimodal_handler.go`、`video_handler.go`、`audio_handler.go`、`moderation_handler.go`、`common.go`、`upstream_lifecycle.go`；`internal/controlplane/` 的 `admin_channels.go`、`channel_service.go`、`admin_channels_probe.go`、`admin_routes_probe.go`、`admin_topology.go`、`admin_topology_probe.go`、`route_service.go`；`internal/storage/repo_channels.go`、`repo_model_routes.go`、`controlplane/sync.go`；`internal/telemetry/metrics.go`、`metrics_prometheus.go`；`cmd/airoute/server.go`、`timeout_chain.go`。
- **旁证**：`git log/show`（`1520d99`、`644ba81`、`3039e73`、`1dca316`）、`docs/ROUTING_TOPOLOGY.md`、`docs/PROVIDER_GUIDE.md`、`docs/ENGINEERING_GUIDELINES.md`、`docs/design/v1.1-scope-cut-2026-10-28.md`、`.opencode/memory/project.md`；相关单测（`request_budget_test.go`、`first_token_timeout_test.go`、`stream_preamble_test.go`、`breaker_*_test.go`、`p2_dispatch_canary_test.go`、`served_model_test.go`、`p2_rewrite_json_test.go`、`multipart_model_rewrite_test.go`、`tests/openai_responses_test.go`）。
- **未覆盖**：`internal/router/*_test.go` 未逐一通读（按需取用）；`internal/provider/anthropic_convert.go`、`gemini_convert.go` 仅核对了模型名/流式调用点与对称性，未逐字段比对协议字段覆盖；`internal/controlplane/prober*.go`（渠道探测启发式）未展开（属 controlplane 域）。
