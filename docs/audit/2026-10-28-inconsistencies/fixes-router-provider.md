# 修复记录：B 域（路由 / 渠道 / 协议适配）+ C-02 provider 侧

> 域：`internal/router/`、`internal/provider/`、`internal/adapter/`、`internal/model/`
> 对应任务：`task-7`；审计来源：[02-router-protocol.md](02-router-protocol.md)（B 域 13 条 + README §4.2）
> 基线：审计基线 `1520d99`；本域改动落在工作区（未提交，遵守「禁止 git 写操作」）。
> 另一会话已把其 WIP 提交到 `c8a2765`，本域改动复核后仍在工作区中生效；行号以本文写作时的工作区为准。
>
> **分工边界**：B-06 / B-08 / B-13 的落地在 `internal/api`（D 负责），本域不改 api 侧；
> B-07 由本域补熔断记账接口并在 `count_tokens` 接线（lead 授权的跨域例外：只改
> `internal/api/inbound_anthropic.go` 的 count_tokens 循环段）。本域完成 B-01~B-05、B-07、B-09~B-12
> 与 C-02 的 provider/model 侧。

## 1. 状态表

| 发现ID | 状态 | 改动文件:行 | 验证 | 备注 |
|---|---|---|---|---|
| B-01 | ✅ 已修 | `internal/router/dispatcher_modality_entry.go:23-45`（Embedding/Rerank 入口包 `withRequestBudget`）<br>`:47-87`（DispatchHTTP 入口包预算 + 预算 cancel 挂到 body 释放点）<br>`internal/router/dispatcher_modality.go:94-100,168-176,289-296,334-342,390-397,432-440`（三条路径的循环起点/失败点判预算）<br>`internal/router/request_budget_test.go:147,195` | `go test ./internal/router/ -run 'TestModalityDispatchStopsAtRequestBudget\|TestDispatchHTTPBodySurvivesBudgetRelease'` 通过（~3s，1s 预算收敛） | 见 §2.1 |
| B-02 | ✅ 已修 | `internal/router/dispatcher_stream.go:171-182`（提交后失败回调）<br>`internal/router/dispatcher_stream_pipe.go:16-56`（`recordStreamFailure`）、`:58-118`（`pipeStreamEvents` 增加 once 失败回调 + 终止标记判定）<br>`internal/provider/errors.go:158-210`（`streamErrorStatus`/`jsonStatusCode`）<br>`internal/provider/openai.go:294-310`、`anthropic_stream.go:137-156`、`gemini_stream.go:102-113`（内嵌 error 帧带状态）<br>`internal/router/dispatcher_stream_fallback_test.go:144,192,211` | `go test ./internal/router/ -run 'TestStreamMid\|TestStreamClientCancel'` + `-race` 通过 | 见 §2.2 |
| B-03 | ✅ 已修 | `internal/provider/errors.go:136-155`（`IsModelNotFound`、`IsFatalPayloadError`）、`:119-133`（`IsClientError` 仅注释收窄语义，行为不变）<br>`internal/router/dispatcher.go:305-330`（chat：先 404→换渠道，再 400/413/422→终止）<br>`internal/router/dispatcher_stream.go:28-39`（流式同规则）<br>`internal/router/dispatcher_modality.go:183-193,345-355,443-453`（Embedding/Rerank/HTTP 统一）<br>`internal/router/dispatcher_modality_test.go:37,78` | `go test ./internal/router/ -run 'TestModalityDispatchContinuesAfterNodeSpecific404\|TestModalityDispatchStopsOnFatalPayloadError'` 通过 | 见 §2.3 |
| B-04 / C-08 | ✅ 已修（router 侧） | `internal/router/dispatcher_fallback.go:140-155`（`recordChannelSuccess` 删除 `RecordRequest`）<br>`internal/router/dispatcher.go:366`、`internal/router/dispatcher_modality.go:269-274,367-372,465-470`（3 处失败计数删除）<br>`internal/telemetry/metrics.go`（**B-04 直接后果**：删除已无生产调用方的 `Metrics.RecordRequest`）、`internal/telemetry/metrics_tail_test.go:23,26,43`（改 `RecordRequestWithModel("", …)`） | 全仓 `grep -rn "RecordRequest(" --include=*.go` 只剩定义被删后的 0 命中；`go test ./internal/telemetry/ ./internal/router/` 通过 | 指标唯一写入者回到 api `RecordUsage`；见 §2.4 |
| B-05 | ✅ 已修（保守方案） | `internal/adapter/anthropic.go:228-249`（`FromCanonical` 改为显式拒绝 + `ErrAnthropicOutboundUnsupported`）、删除 73 行漂移实现<br>`internal/adapter/adapter.go:40-52`（anthropic 移出注册表，仅注册 gemini）<br>`internal/adapter/adapter_test.go:16-52`（删「thinking 块」错误契约断言，改为钉住拒绝语义）、`:234`（重命名） | `go test ./internal/adapter/` 通过 | 见 §2.5 |
| B-07 | ✅ 已修（本域 + 授权跨域一处） | `internal/router/breaker_access.go`（新文件，`ChannelCanExecute`/`NoteChannelSuccess`/`NoteChannelFailure`/`ReleaseHalfOpenProbe` 四个导出访问器，委托 `CircuitBreaker` 语义）<br>`internal/router/breaker_access_test.go`（3 条语义用例：阈值/成功清零/半开 canary 归还/advisory）<br>`internal/api/inbound_anthropic.go:276-307`（准入改用 `ChannelCanExecute`，替换原 `GetBreakerStatus=="OPEN"` 判断，避免双重跳过）、`:321-370`（各失败/成功出口统一 `recordOutcome`：nil→成功；取消→归还 canary；`IsChannelFailure`→记失败；其余→归还 canary） | `go test ./internal/router/ ./internal/api/` 通过；`go test ./internal/router/ -run 'TestBreakerAccessors\|TestReleaseHalfOpenProbe'` 通过 | 见 §2.11 |
| B-09 | ✅ 已修 | `internal/router/topology.go:78-120`（`minPriority` 替代 `Priority==1`）、`:141-152`（权重占比/主备计数用 minPriority）<br>`internal/router/topology_test.go:13,48,70,86` | `go test ./internal/router/ -run 'TestModelRoutes\|TestRoutingSnapshotKeepsConfiguredPriorities'` 通过 | 全 priority=2 时主层权重占比正确 |
| B-10 | ✅ 已修 | `internal/provider/errors.go:68-84`（`IsChannelFailure` 规格注释改为「5xx/401/403/408/网络计入；429 只换渠道不计入」）<br>`internal/provider/channel_failure_test.go:7-15`（注释指向该规格） | 既有 `TestRateLimitIsNotAChannelFailure` 通过 | 行为未改，只改注释契约（历史事故是「按旧注释改代码」） |
| B-11 | ✅ 已修（选方案一：配置即生效） | `internal/router/dispatcher_modality_entry.go:89-131`（`dispatchModalityWithFallback` 递归 + `nextFallbackContext` cycle/depth 保护）<br>`internal/router/errors.go:24-40,92-105`（`DispatchError.Canceled`，用于排除客户端取消）<br>`internal/router/dispatcher_modality_test.go:101,168` | 4 个子用例（no-route / exhausted / cycle guard / chain a→b→c）+ HTTP 用例通过 | 见 §2.11 |
| B-12 | ✅ 已修 | `internal/provider/reasoning_adapter.go:236-295`（`reasoningStreamPipeline` = ProcessChunk/Flush 共用）<br>`internal/provider/openai_text_adapter.go:198-246`（text 流接入 + 终止前 Flush）<br>`internal/provider/openai.go:222-240,311-318`（改用同一 pipeline）<br>`internal/provider/openai_text_reasoning_stream_test.go:18` | `go test ./internal/provider/ -run TestTextStreamSplitInlineReasoning` 通过（跨 chunk 拆标签） | 一元/流式规则同源 |
| C-02（provider 侧） | ✅ 已修（本域一半） | `internal/model/chat.go:90-131`（`PromptTokensDetails.CacheCreationTokens` + `GetCacheCreationTokens()`）<br>`internal/model/usage.go:47-66`（顶层 `cache_creation_input_tokens` / `prompt_tokens_details.cache_creation_tokens` 落到明细，prompt 口径不变）<br>`internal/provider/usage_mapping.go:5-30`（anthropic 填 `CacheCreationInputTokens`；gemini 恒 0 并注明上游无此字段）<br>`internal/provider/usage_mapping_test.go:5,35,50`、`internal/model/usage_test.go:8` | `go test ./internal/provider/ ./internal/model/` 通过 | **api 侧透传待 D 接入**，见 §4 |

**未处理（按分工不属本域）**：B-06、B-07、B-08、B-13（api/controlplane 侧，D 负责）；C-02 的计费落点与 api 透传（D）；C-01/C-03~C-14 属 C 域。

## 2. 逐条说明

### 2.1 B-01 请求预算只接 Chat 一元
三条非流式模态路径改为「入口包预算 + 尝试循环起点判预算 + 失败点把预算耗尽归因为网关」：

- `DispatchEmbedding` / `DispatchRerank`：入口 `withRequestBudget(ctx)`（调用方已有更短 deadline 时自动尊重），
  循环起点 `ctx.Err()` 命中预算则返回 `ErrRequestBudgetExceeded`（复用 `newRequestBudgetError`，带渠道 trail），
  attempt 返回 `DeadlineExceeded` 时也判一次，避免把预算耗尽记成「渠道失败」再白等下一个渠道超时。
- `DispatchHTTP`：预算 cancel **不能**在函数返回时执行——响应体会在返回后才被 handler 拷给客户端，而
  `httpReq` 的 context 派生自预算 context。因此预算 cancel 与既有渠道超时 cancel 一起挂到
  `cancelOnCloseReadCloser.Close`（新增外层 wrap），并额外用「成功则把取消权交给 body」的 `released` 标志
  防止 defer 提前 cancel。测试 `TestDispatchHTTPBodySurvivesBudgetRelease` 钉住该释放时机。
- 副作用（已知、合理）：多模态的**响应体传输**时间现在也计入总预算。
- 流式仍不接总预算：`request_budget.go:13-16` 已写明「流式靠首 token 窗口」，属审计排除项，本次未改。

### 2.2 B-02 流式首 token 后失败不入熔断账
- 提交点仍 `RecordSuccess`（首 token 已到客户端，不能把已提交的流改判失败），但 `pipeStreamEvents` 增加
  `onFailure` 回调：收到 `event.Err != nil`，或输入 channel 关闭却**从未出现终止标记**
  （`IsDone` 或 `finish_reason`）时，回调一次（`reported` 保证「一帧 error + 紧接 EOF」只记一次）。
- `recordStreamFailure` 的排除项：请求 context 已结束（客户端断开）、`context.Canceled`、fatal payload
  （400/413/422）。带状态的错误沿用共享分类器：429（背压）与 404（本节点不托管）仍不计入。
  **未分类的流中错误**（传输中断、provider 报错帧、无终止标记的截断）一律计入——否则「先吐 1 token 再
  必断」的渠道依旧永远 CLOSED。
- 为了让内嵌 error 帧也能被分类，三个 provider 的流内 error 帧从 `fmt.Errorf` 改为 `NewUpstreamError`，
  状态由 `streamErrorStatus` 按 unary 等价语义映射（rate_limit/quota→429、overload/unavailable/5xx 类→503、
  invalid_request→400、authentication→401、未知→503）。这样 429 帧不会被误记熔断（`TestStreamMidRateLimitDoesNotChargeBreaker`）。
- 未做审计可选项「把 RecordSuccess 延后到 IsDone」：那会让首 token 已交付但客户端中途取消的渠道不计成功，
  反而丢失健康信号，本次不改。

### 2.3 B-03 404 处置三套规则
共享判定抽到 provider 层（router/provider 均可引用，且已有 `errors_test.go` 覆盖）：
`IsModelNotFound`（404）、`IsFatalPayloadError`（400/413/422，**不含 404**）。调用点统一为
「先判 404 → `AbortCanary` + 记入 attempted + 继续下一渠道；再判 fatal payload → 终止」：

- Chat（`dispatcher.go`）：原来靠「404 不是 channel failure → 落 continue」隐式实现，现在显式化并加日志；
  400/413/422 提前终止行为不变。
- 流式（`failStreamAttempt`）：404 明确走「换渠道不终止」，fatal payload 仍终止。
- Embedding / Rerank：原来用 `IsClientError`（含 404）直接终止，改为 404 换渠道、fatal 终止。
- DispatchHTTP：4xx 分支先判 404（缓存 body 进 `lastErr`，继续下一渠道），其余 4xx 保持原样透传；
  传输错误分支同样先 404。
- `IsClientError` 保留原语义（api 回归测试 `internal/api/regression_test.go:132` 仍在用），只在注释里说明
  「不要单独用它判终止」。

### 2.4 B-04 = C-08 指标双计
`telemetry.GlobalMetrics.RecordRequest` 在 router 的 4 个调用点全部删除（`recordChannelSuccess` 成功 1 处、
Chat/HTTP/Embedding/Rerank 失败各 1 处），`RecordFallback`、熔断、日志全部保留。指标唯一写入者回到
api `RecordUsage`（流式原本就只记一次，现在一元/流式口径一致）。
router 内没有测试断言这些计数器，无需改测试；全仓 `tests/` 集成测试只断言 `/metrics` 存在指标名。

**连带清理（B-04 的直接后果，lead 授权）**：router 侧删完后，`telemetry.Metrics.RecordRequest`
（`RecordRequestWithModel("", …)` 的别名）已无任何生产调用方，属死符号；已删除该方法，并把
`metrics_tail_test.go` 的 3 处调用改为 `RecordRequestWithModel("", …)`（断言值不变，模型维度仍为“无标签”）。
`internal/telemetry/metrics_test.go` 由 C 域修过，本次只读未改（它本就只用 `RecordRequestWithModel`）。

### 2.5 B-05 `adapter.AnthropicAdapter.FromCanonical` 未接线且已漂移
采用任务给出的保守方案：
- 删除 73 行漂移实现（会为 `ReasoningContent` 合成无签名 `{"type":"thinking"}` 块、不减 cache read、id 无 `msg_` 前缀）。
- `ProtocolAdapter` 接口仍要求 `FromCanonical`（api 的 `adapter.Get("anthropic")` 与
  `NewAnthropicAdapter()` 兜底构造都要求该类型满足接口），因此保留方法但改为返回
  `ErrAnthropicOutboundUnsupported`：把「误接即回归」的陷阱变成显式护栏。
- `init()` 只注册 gemini；`adapter.go` 注释写明 anthropic 出站转换由 `internal/api/inbound_anthropic.go` 负责
  （原因：禁止合成无签名 thinking 块；cache 扣减与 `msg_` 前缀也在那边）。
- api 侧零改动：`ConvertAnthropicToCanonical` 的 `if !ok { ad = adapter.NewAnthropicAdapter() }` 兜底继续工作。
- 测试：删除钉死「thinking 块」错误契约的断言（原 `adapter_test.go:328-333` 段），
  `TestAnthropicAdapter_InboundOnly` 改为断言「入站转换可用 + 出站显式拒绝 + anthropic 不在注册表」。

### 2.6 B-09 拓扑页 `Priority==1` 硬编码
`GetModelRoutes` 先求「已匹配渠道的最小 priority」作为主层，再据此判
`hasFallbackTier`（有更高 priority 层）、`totalPrimaryWeight`（主层 **active** 渠道权重和）、
`PrimaryProvidersCount`/`FallbackProvidersCount`、`weight_percent`。与
`dispatcher_routing.go:83-118`（`sort.Ints` 后 group 0 为主层、享会话亲和）语义一致。
顺带修正：主层权重合计为 0 时，主层渠道仍计入 `PrimaryProvidersCount`（原来会全落 backup，造成「0 主 N 备」）。

### 2.7 B-10 `errors.go` 注释称 429 计入渠道失败
注释改为实际语义，并在 `channel_failure_test.go` 的 429 用例上注明它钉住的规格出处；
另新增 `TestPayloadClassificationKeeps404Retryable`（404 可换渠道、400/413/422 才是 fatal、401/403/408/429/5xx 都不是 fatal）。

### 2.8 B-11 跨模型兜底只对 Chat/Stream 生效
选**方案一（配置存在就生效）**，理由：
1. `/api/v1/admin/models/routes` 对任意模型都接受 `fallback_model`，拓扑页对任意模型都画兜底链；
   配置面已承诺，执行面缺失属「半接线」，补齐比在 UI 上加「仅对话生效」更符合控制面语义。
2. embedding/rerank/多模态同样是「模型→渠道」路由，兜底模型只是换一个模型名，语义安全。
3. 复用 chat 的 `nextFallbackContext`（cycle + depth=4），并让预算 context 贯穿递归，兜底不会突破
   `GATEWAY_REQUEST_BUDGET_SEC`。

实现：三条路径的 `*Once` 保持纯尝试循环，公开入口统一走泛型
`dispatchModalityWithFallback`；只在「无候选 / 全部失败（`DispatchError`）」时兜底，
`DispatchError.Canceled`（客户端取消）与 fatal payload/预算错误不兜底。HTTP 路径
`PinnedChannel` 属运维强制指定，按原语义**不**兜底。

### 2.9 B-12 `openai_text` 流式未接 reasoning 剥离
把 chat 流式那段 `ProcessChunk/Flush` 提炼为 `reasoningStreamPipeline`（`reasoning_adapter.go`），
`openai.go` chat 流与 `openai_text_adapter.go` text 流共用：逐 chunk `Emit`，
在 `[DONE]`/EOF 前 `Flush` 残留的半截标签。测试用跨两个 data 帧拆开的 `<think>` 前缀验证 buffer 生效，
断言 content 中不再出现标签、reasoning_content 拿到思考片段、最终 answer 保留。
顺带把 text 适配的请求侧纯函数（`buildCompletionURL`/`requiresChatCapabilities`/`convertChatToPrompt`/
`ErrTextAdapterUnsupported`）移到 `internal/provider/openai_text_adapter_convert.go`，使主文件回到 235 行（≤300）。

### 2.10 C-02 缓存写入 token 的 provider/model 侧表达
- `model.PromptTokensDetails` 新增 `CacheCreationTokens int json:"cache_creation_tokens,omitempty"`
  （放在 `cached_tokens` 旁边，语义是「PromptTokens 中有多少是缓存写入」），并加 `Usage.GetCacheCreationTokens()` 访问器。
- **prompt 口径不变**：`anthropicUsageToModel` 仍是 `input + cache_read + cache_creation`；新字段是子集，
  与 `billing.CalculateCostForUnitsWithCacheCreation` 的「三个互斥桶都来自 promptTokens」约定一致，不会重复计费。
- anthropic 映射填 `CacheCreationInputTokens`；纯写入（无 cache read）时也创建 details（原实现只在有 read 时创建）。
- gemini 恒 0（`GeminiUsageMetadata` 无对应字段，不臆造）；openai 兼容路径在 `Usage.UnmarshalJSON` 里
  从顶层 `cache_creation_input_tokens` 或 `prompt_tokens_details.cache_creation_tokens` 填充。

### 2.11 B-07 `count_tokens` 是唯一绕过熔断器的数据面路径
`/v1/messages/count_tokens` 不进 Dispatcher（它是 Anthropic-wire-only 的辅助端点：不计费、无跨模型兜底、
无首 token 窗口），因此 D 域的准入只做了「OPEN 就跳过」，但**无法记账**。

- **router 侧新增最小桥接**（`internal/router/breaker_access.go`）：
  `ChannelCanExecute`（= `CanExecute`，半开会消费 canary 槽）、`NoteChannelSuccess`（= `RecordSuccess`）、
  `NoteChannelFailure`（= `RecordFailure`，advisory 模式下记账但放行由 `CircuitBreaker` 自身保证）、
  `ReleaseHalfOpenProbe`（= `AbortCanary`，把未产生结论的 canary 槽还回去）。全部委托现有
  `CircuitBreaker`，不复制阈值/冷却/canary 逻辑；nil 接收者安全（辅助调用可能早于路由装配）。
- **api 侧循环**（`internal/api/inbound_anthropic.go`，授权范围内）：
  1. 删掉原来的 `GetBreakerStatus(name)=="OPEN"` 判断，改为 `!ChannelCanExecute(name) → continue`
     （一个判断同时覆盖 OPEN 与半开 canary 限流，不做双重跳过）。顺序保持「先判 wire 协议、再判准入」，
     避免对非 anthropic 渠道白占 canary。
  2. 新增局部 `recordOutcome(channel, err)`：`err==nil` → 成功；请求 ctx 已结束或 `context.Canceled`
     → 只归还 canary；`provider.IsChannelFailure(err)`（5xx/401/403/408/网络）→ 记失败；
     其余（本地 marshal/建请求失败、非健康结论）→ 只归还 canary。
  3. 每个出口都记一次：marshal 失败、建请求失败、`Do` 失败、读 body 失败、非 200、200 但 JSON 解析失败、
     成功。非 200 用 `provider.NewUpstreamError(channel, status, body)` 做分类（不改变对外错误文案），
     因此 429 仍按背压处理、5xx 才入熔断——与 B-03/B-10 的口径一致。
- 与 D 的边界：D 原有的「跳过 OPEN 渠道」准入语义被保留且更严格（现在也尊重半开 canary），
  D 若后续重排该循环，只需保留 `ChannelCanExecute` 准入 + 出口 `recordOutcome` 两处。

## 3. 验证结果

环境：`export PATH=$PATH:/usr/local/go/bin`（go1.26.1 linux/amd64），工作区 HEAD=`c8a2765`（另一会话已提交）。

```
$ gofmt -l internal/router/ internal/provider/ internal/adapter/ internal/model/ internal/api/inbound_anthropic.go internal/telemetry/
（空）

$ go build ./...
（通过，exit 0）

$ go vet ./...
（通过，exit 0）

$ go test ./internal/router/ ./internal/api/ ./internal/telemetry/ ./internal/provider/ ./internal/model/ ./internal/adapter/
ok  	github.com/ifnodoraemon/airoute/internal/router
ok  	github.com/ifnodoraemon/airoute/internal/api
ok  	github.com/ifnodoraemon/airoute/internal/telemetry
ok  	github.com/ifnodoraemon/airoute/internal/provider
ok  	github.com/ifnodoraemon/airoute/internal/model
ok  	github.com/ifnodoraemon/airoute/internal/adapter

$ go test -race ./internal/router/...
ok  	github.com/ifnodoraemon/airoute/internal/router	10.8s

$ go test ./...        # 全仓回归
第一次（13:07）：exit=0，16 个包 ok（含 tests/ 集成）
第二次（13:15，B-07/telemetry 改动后）：exit=1，仅 internal/controlplane 两条失败——
  TestSavePricingRateHTTPContract / TestPricingServiceSaveRejectsOverlappingOffPeakSlots
  （C-10 分时槽位校验，属 C 域；这两个文件在 13:12 之后被并发修复者改动，非本域改动引起）
第三次（13:18，C 域并发改动落定后）：exit=0，16 个包 ok（controlplane 复跑通过）
```

本域最后一次全绿证据：`go test ./internal/router/ ./internal/api/ ./internal/telemetry/
./internal/provider/ ./internal/model/ ./internal/adapter/` 全 ok；`go build ./...`/`go vet ./...` exit 0；
`go test ./...` exit=0（16 包）。

新增/改写的测试（本域）：

| 用例 | 覆盖 |
|---|---|
| `internal/router/request_budget_test.go:147 TestModalityDispatchStopsAtRequestBudget` | embedding/rerank/HTTP 三条路径被 1s 预算收敛 |
| `internal/router/request_budget_test.go:195 TestDispatchHTTPBodySurvivesBudgetRelease` | 预算 cancel 不早于响应体读完 |
| `internal/router/dispatcher_stream_fallback_test.go:144 TestStreamMidFailureRecordsBreakerFailure` | 首 token 后断流 → 熔断失败恰好 1 次 |
| `.../dispatcher_stream_fallback_test.go:192 TestStreamMidRateLimitDoesNotChargeBreaker` | 流中 429 不计熔断 |
| `.../dispatcher_stream_fallback_test.go:211 TestStreamClientCancelDoesNotChargeBreaker` | 客户端取消不计熔断 |
| `internal/router/dispatcher_modality_test.go:37 TestModalityDispatchContinuesAfterNodeSpecific404` | embedding/rerank 首渠道 404 → 第二渠道成功（trail `node-a→node-b`） |
| `.../dispatcher_modality_test.go:78 TestModalityDispatchStopsOnFatalPayloadError` | 400 终止且不重放到其它渠道 |
| `.../dispatcher_modality_test.go:101 TestModalityDispatchUsesCrossModelFallback` | no-route / exhausted / cycle / chain 四情形 |
| `.../dispatcher_modality_test.go:168 TestDispatchHTTPUsesCrossModelFallback` | 多模态 HTTP 兜底 |
| `internal/router/topology_test.go:13,48,70,86` | 全 priority=2 主层权重、单层、inactive 不计权重、快照优先级 |
| `internal/provider/openai_text_reasoning_stream_test.go:18` | text 流式 ` thinking` 拆分（跨 chunk） |
| `internal/provider/usage_mapping_test.go:5,35,50` + `internal/model/usage_test.go:8` | C-02 字段填充与 prompt 口径不变 |
| `internal/provider/errors_test.go:23` | 404/fatal payload 分类 |
| `internal/adapter/adapter_test.go:16` | anthropic 入站可用 + 出站拒绝 + 不进注册表 |
| `internal/router/breaker_access_test.go:11,55,80` | B-07 四个访问器：阈值/成功清零/nil 安全、半开 canary 归还、advisory 模式只记账不拒绝 |

## 4. handoff（需要其它域配合 / 后续注意）

1. **C-02 api 透传（D，P1）**：canonical 路径目前没有把新字段送进计费。
   需在这些位置加 `CacheCreationTokens: resp.Usage.GetCacheCreationTokens()`：
   - `internal/api/handler.go:159-167`（chat unary，`resolveChatUsage` 旁边；注意 stream 需累加 `max`）
   - `internal/api/completions_handler.go:201`、`internal/api/responses_handler.go:156`、`internal/api/gemini_inbound.go:240`
   - 流式：`chat_stream_handler.go:113`、`completions_stream_handler.go:143`、`responses_stream.go:105`、
     `gemini_inbound_stream.go:89`（各自已有 `totalCachedTokens` 的累加循环，可同处累加 cache creation）
   native Anthropic 两处（`inbound_anthropic.go:130`、`inbound_anthropic_stream.go:437`）D 已接，无需改。
   本域已保证字段存在且被三个 provider 映射正确填充，且 **prompt 总数不变**（cache creation 是其子集）。

2. **B-05 注册表变化（无需 api 改动，但请注意）**：`adapter.Get("anthropic")` 现在返回 `(nil,false)`，
   `ConvertAnthropicToCanonical` 走 `NewAnthropicAdapter()` 兜底构造（原有代码路径）。
   若后续删除该兜底，anthropic 入站转换会失效——建议保留兜底或在 `Get` 处显式处理。

3. **B-02 审计文案变化**：流内 error 帧现在是 `*provider.UpstreamError`，审计/客户端看到的错误文本从
   `upstream stream error [overloaded]: ...` 变为 `upstream <channel> returned status 503: ...`。
   全仓无测试断言旧文本；若控制台有按文本匹配的展示逻辑，请改用 `provider.AsUpstreamError`。

4. **B-01 语义扩展**：`DispatchHTTP` 的总预算现在覆盖响应体传输（budget cancel 在 body Close 时执行）。
   调用方必须关闭 `resp.Stream`（api 侧现有 handler 均 `defer Close`）；若不关，预算 context 会在
   `GATEWAY_REQUEST_BUDGET_SEC` 后自然过期（与既有渠道超时的行为一致）。

5. **B-11 审计链**：模态兜底递归没有像 chat 那样用 `withChannelTrail` 把上一模型的渠道并入 trail，
   最终失败行的 `usage_logs.channel` 只保留最后一个模型的渠道。若需要完整链路，可在
   `dispatchModalityWithFallback` 里补 `withChannelTrail`（本域已预留 `DispatchError.Trail`）。

6. **B-04 观察面**：router 不再写请求指标，`airoute_requests_total` 现在只有 api 层一份样本
   （一元/流式口径一致）。若有 Grafana 面板/告警按「2× 一元请求量」调过阈值，需要复核。
   另外 `telemetry.Metrics.RecordRequest` 已随本次清理删除（B-04 的直接后果），如还有测试/工具引用它，
   改用 `RecordRequestWithModel("", …)`。

7. **B-07 接口面（D 若再动 count_tokens）**：`internal/api/inbound_anthropic.go` 的 count_tokens 循环现在
   依赖两处本域约定：(a) 准入用 `h.dispatcher.ChannelCanExecute(channel.Name)`（同时覆盖 OPEN 与半开
   canary，**不要**再叠 `GetBreakerStatus=="OPEN"` 判断）；(b) 每个出口必须调一次
   `recordOutcome(channel.Name, err)`（成功传 nil）。四个访问器语义：
   `NoteChannelFailure` 在 advisory 模式下记账但不拦截，`ReleaseHalfOpenProbe` 只归还 canary 槽。
   端到端（api 侧）的熔断记账尚无专门单测（跨域不便新增 api 测试文件），由 router 侧
   `breaker_access_test.go` 钉住访问器语义、api 既有 count_tokens 用例保证行为不回归；
   若要端到端用例，建议 D 在 `internal/api/` 补（用 httptest 上游 + `d.GetBreakerStatus` 断言）。

## 5. 规约偏差说明

- `internal/router/dispatcher_modality.go` 现 473 行（改动前 494 行）：**改动前已超 300 行指引**。本次把
  `RewriteMultipartModel` 拆到 `multipart_model_rewrite.go`、把三个公开入口拆到
  `dispatcher_modality_entry.go`，净减少 21 行；`dispatchHTTPOnce`（201 行）为既有函数，未做行为无关的大重构，
  以降低审计修复轮的风险。剩余拆分建议后续单独 PR（把 Embedding/Rerank 循环与 HTTP 循环分文件）。
- 其余改动文件均在 300 行内（`provider/errors.go` 210、`reasoning_adapter.go` 295、
  `openai_text_adapter.go` 235、`dispatcher_stream.go` 260、`topology.go` 223、`breaker_access.go` 68 等）。
- 跨域例外只碰了两处、均为 lead 明确授权：`internal/api/inbound_anthropic.go` 的 count_tokens 循环段
  （B-07 接线，含新增 `errors` import）与 `internal/telemetry/metrics.go`/`metrics_tail_test.go`
  （删除 B-04 造成的死符号 `Metrics.RecordRequest`）。api 其它逻辑、其余 C 域文件未改。
  第三轮清理又碰了两处、同样由 lead 授权：`internal/provider/errors.go`（删 `IsClientError`）与
  `internal/api/regression_test.go`（把 `clientEr` 断言迁移到 `IsFatalPayloadError` + `IsModelNotFound`）。
- 文档中「全仓 go test ./...」的结论按时间点记录：13:07 全绿；13:15 复跑后 `internal/controlplane`
  曾出现 2 条 C-10 相关失败（并发修复者正在改 `admin_pricing*.go`/`pricing_service.go`），与本域无关；
  13:18 C 域改动落定后全仓复跑回到 exit=0 / 16 包 ok。

## 6. 第三轮清理（lead 指派）

### 6.1 删除 B-03 替换后遗留的死符号 `provider.IsClientError`
B-03 把所有调用点统一到 `IsModelNotFound` / `IsFatalPayloadError` 后，`IsClientError` 生产零调用
（仅 `internal/api/regression_test.go` 还在断言它），属 deadcode 复查命中的新死符号。

- **删除** `internal/provider/errors.go` 的 `IsClientError`（原 `:119-134`），并把历史原因写进
  `IsFatalPayloadError` 的注释：旧谓词把 400/413/422 与 404 混为一谈，而两者需要相反动作
  （终止 fan-out vs 换下一个节点），这正是 B-03 的根因，避免后来者再合并回去。
- **迁移唯一测试引用**：`internal/api/regression_test.go:103-146` 的
  `TestIsChannelFailureClassification` 把 `clientEr bool` 拆成 `fatalPayload bool` + `modelNotFound bool`
  两列，分别断言 `IsFatalPayloadError` 与 `IsModelNotFound`（404 只落 modelNotFound、400/413/422 只落
  fatalPayload、401/403/408/429/5xx 两者皆 false）。断言比原来更精确，未弱化任何已修行为。
- 全仓复查：`grep -rn "IsClientError" --include=*.go internal tests` 仅剩 `IsFatalPayloadError` 注释里的
  「former IsClientError」历史说明（刻意保留），无任何代码引用。

### 6.2 `internal/middleware/authz.go` 的配额遗留死符号（**未改动，仅备注**）
`ReserveUserQuota`（`authz.go:306`）、`ReleaseUserQuota`（`authz.go:311`）、`MaxInflightPerUser`
（`quota_tracker.go:63`）是并行会话把配额改为整数单位预授权（`ReserveUserQuotaAmount` /
`ReleaseUserQuotaAmount` / `DefaultMaxInflightPerUser`）后的遗留：
- `ReserveUserQuota`/`ReleaseUserQuota` 仅被 `internal/middleware/quota_amount_test.go:175,179,185,186` 引用；
- `MaxInflightPerUser` 全仓零读取（只有定义与注释）。
属 **middleware 域**且不在本轮范围，按要求**未改动**；建议 middleware 负责人后续删除或标注「仅测试/兼容」。

### 6.3 第三轮验证

```
$ gofmt -l internal/provider internal/api/regression_test.go internal/router
（空）

$ go build ./...      → exit 0
$ go vet ./...        → exit 0

$ go test ./internal/provider/ ./internal/router/ ./tests/
ok  	github.com/ifnodoraemon/airoute/internal/provider	1.827s
ok  	github.com/ifnodoraemon/airoute/internal/router	9.839s
ok  	github.com/ifnodoraemon/airoute/tests	4.457s

$ go test ./internal/api/ -run TestIsChannelFailureClassification -v
--- PASS: TestIsChannelFailureClassification (0.00s)
$ go test ./internal/api/     → ok（11.597s）

$ go test ./...       → exit=0，16 个包 ok（含 tests/ 集成）
```
