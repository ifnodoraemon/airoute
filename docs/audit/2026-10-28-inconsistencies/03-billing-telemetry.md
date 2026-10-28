# 计费/校验/遥测/分布式一致性审计（2026-10-28）

## 概览

**审计基线：commit `1520d99`**（审计期间 HEAD 前移到 `190b382`，该提交只改 `docs/CONFIGURATION.md`；工作区另有并行会话**未提交**的结算/配额改造，涉及文件与影响见「覆盖面」末尾。本文所有 `path:line` 均指 `1520d99` 的内容，代码块同时保留符号名以便定位。）

本次只读审计覆盖 `internal/billing`、`internal/validator`、`internal/telemetry`、`internal/distributed`，并追到 `internal/api` 的计费/校验/遥测调用点确认「谁在读」。共 14 条发现，主线可以概括成一句话：**金额计算本身只有一个漏斗（`RecordUsage` → `CalculateCostForUnitsWithCacheCreation` → `settleCharges`），但漏斗内部的多条规则只接上了一条路径**——缓存写入溢价只有 native Anthropic 流式接、校准只有 chat 形状的 prompt 接、校验规则字段只有 OpenAI 协议接、启动定价检查的第二个分支被第一个分支短路。遥测侧存在**同一请求被 router 与 api 各记一次**的双计数；分布式侧降级分支都已实现，但 RPM 的两套实现窗口语义并不等价。金额类结论均给出「handler → engine → 结算」证据。

## 发现

### C-01 五个 `CalculateCost*` 变体是死链，唯一扣费入口只有一个 `WithCacheCreation` 版本

- **类别**：死接线
- **位置**：`internal/billing/engine.go:375`、`:391`、`:510`、`:515`、`:521`
- **设计意图**：这五个导出方法各带完整注释（"computes total request cost … for a specified pricing group"、"using current server time and default group"），显然是给调用方按「是否分组 / 是否分时 / 是否已知时刻」挑一个用的公开计价 API。
- **现状**：生产代码里**只有** `CalculateCostForUnitsWithCacheCreation` 被调用（1 处，`internal/api/common.go:205`）。被 deadcode 判死的 5 个方法只被彼此和测试调用：`CalculateCostDetailed`/`CalculateCostWithGroup`/`CalculateCost` 是一条纯包装链，`CalculateCostForUnits` 强制 `cacheCreationTokens=0`，`CalculateCostDetailedWithGroup` 丢掉 snapshot 与 units。测试引用见 `internal/billing/engine_test.go:182`、`units_test.go:40`、`default_price_test.go:31`、`cache_and_bias_test.go:30`。
- **证据**：
  ```go
  // internal/billing/engine.go:510
  func (e *BillingEngine) CalculateCostDetailed(modelName string, promptTokens, completionTokens, cachedTokens int, t time.Time) (cost float64, ...) {
      return e.CalculateCostDetailedWithGroup(modelName, "default", promptTokens, completionTokens, cachedTokens, t)
  }
  // internal/billing/engine.go:515-522
  func (e *BillingEngine) CalculateCostWithGroup(...) { ... e.CalculateCostDetailedWithGroup(...) }
  func (e *BillingEngine) CalculateCost(...) { return e.CalculateCostWithGroup(modelName, "default", ...) }
  ```
  ```go
  // internal/api/common.go:204-214 —— 全仓唯一的计费调用点
  if billable && !params.SkipBilling && billing.GlobalEngine != nil {
      cost, savedCost, _, isOffPeak, offPeakDiscount, pricingSnapshot = billing.GlobalEngine.CalculateCostForUnitsWithCacheCreation(
          billingModelFor(params, keyGroup), keyGroup, params.PromptTokens, params.CompletionTokens,
          params.CachedTokens, params.CacheCreationTokens, params.BillableUnits, time.Now())
  ```
- **影响**：中 — 没有运行时错误，但「哪一个是权威计费实现」无法从 API 面读出；后来者若接上 `CalculateCostDetailed`/`CalculateCost`（它们不带 cache-creation 与 units），会静默少收固定价与缓存写入费，正是 C-02 已经发生过一次的路径。
- **置信度**：高
- **建议**：删掉这 5 个变体，或收成一个内部 `calculate()` + 一个导出入口，让「扣费只有一个函数」在类型层面成立。

### C-02 缓存写入溢价 1.25x 只在 native Anthropic 流式路径接通

- **类别**：半接线
- **位置**：`internal/billing/engine.go:387`、`internal/api/inbound_anthropic_stream.go:462`、`internal/provider/usage_mapping.go:14`
- **设计意图**：`cacheWritePriceMultiplier` 的注释写得很明确——"Cache writes used to be billed at the plain prompt rate, which under-charged every cache miss by 20% … The multiplier is applied to the resolved prompt price"。即：缓存写入必须按 prompt 价 ×1.25 计费。
- **现状**：全仓只有 `handleNativeAnthropicStream` 把 `cache_creation_input_tokens` 单独取出来并传进 `AuditRecordParams.CacheCreationTokens`（`inbound_anthropic_stream.go:461-462`、`:421`）。其余所有路径都拿不到这个数：(a) native Anthropic **非流式**（`inbound_anthropic.go:115/119`）只传 `CachedTokens`，虽然它已经 `json.Unmarshal` 了原始 body 却没读 `usage.cache_creation_input_tokens`；(b) 所有走 canonical 的路径（OpenAI 兼容 chat / responses / gemini）经过 `anthropicUsageToModel` 时把 cache creation 直接加进了 prompt 总数、只单独暴露 cache read，写入量在结构上就丢了。
- **证据**：
  ```go
  // internal/provider/usage_mapping.go:14-22
  prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
  usage := &model.Usage{PromptTokens: prompt, ...}
  if u.CacheReadInputTokens > 0 { usage.PromptTokensDetails = &model.PromptTokensDetails{CachedTokens: ...} }
  // ↑ CacheCreationInputTokens 只被并入 prompt，从未单独暴露
  ```
  ```go
  // internal/api/inbound_anthropic_stream.go:461-462（唯一取出点）
  cachedReadTokens = intFromJSONNumber(usage["cache_read_input_tokens"])
  cacheCreationTokens = intFromJSONNumber(usage["cache_creation_input_tokens"])
  ```
  ```go
  // internal/api/inbound_anthropic.go:115-119（同协议的 unary 分支没有）
  inputTokens, outputTokens, cachedTokens, usageSource := resolveChatUsage(canonicalReq, resp)
  RecordUsage(c, AuditRecordParams{... CachedTokens: cachedTokens ...}) // 无 CacheCreationTokens
  ```
- **影响**：中 — 缓存写入按 1.0x 而非 1.25x 计费，缓存密集的 Anthropic 流量系统性少收约 20% 的写入成本；同一模型同一请求流式/非流式价格不同。
- **置信度**：高
- **建议**：在 `model.Usage`/canonical 映射里保留 cache-creation 字段并在 unary native 与 canonical 路径一并透传，否则把该溢价降级为明确的「仅流式」并在注释与计费口径文档中写明。

### C-03 校准结果整表覆盖写库：两副本互相覆盖，落库后的合并只在启动时发生一次

- **类别**：写入不读 / 重复漂移
- **位置**：`internal/billing/calibration_persist.go:20`、`:38`
- **设计意图**：`LoadCalibration` 的注释写明理由——"Ratios must outlive a process … and **two replicas must charge the same way**. Learning them separately per replica (and resetting on every deploy) is the opposite of strict alignment with the provider invoice."
- **现状**：读侧确实做了合并（`Restore` 只在「本地样本更多」时保留本地），但写侧 `SaveCalibration` 是**整表覆盖**：`Snapshot()` 取本副本全量 map 直接塞进同一个 setting key。而 server 里每个副本都跑 5 分钟定时保存 + 每 200 次观察的 persist hook，于是 A、B 两副本每 5 分钟轮流把对方学到的模型**整段删掉**再写回自己的视图；运行时没有任何再合并路径。结果是两副本各按自己的 `samples` 计费（同一 prompt 在 A/B 上单价不同），而重启后两个副本都只继承「最后写库的那一个」的样本。
- **证据**：
  ```go
  // internal/billing/calibration_persist.go:42-50
  snapshot := calibrator.Snapshot()
  if len(snapshot) == 0 { return nil }
  encoded, err := json.Marshal(snapshot)          // 只有本副本视图，无 merge
  return repo.SetSetting(calibrationSettingKey, string(encoded))
  ```
  ```go
  // cmd/airoute/server.go:236-247（每个副本都跑）
  case <-ticker.C:
      if err := billing.SaveCalibration(repo, billing.GlobalCalibrator); err != nil { ... }
  ```
  ```go
  // internal/billing/calibration.go:182-185（合并语义只在 Load 时）
  existing := c.samples[model]
  if existing != nil && existing.count >= state.Samples { continue }
  ```
- **影响**：中 — 「两副本按同一系数计费」这一设计目标没有达成；重启后校准样本可能整体回退到某个副本的局部观测，估算扣费的偏差不会被纠正。
- **置信度**：高
- **建议**：写库改为「读出 → 按 count 取大者合并 → 写回」的未丢失合并（例如单行一条 model 的 upsert），或把校准放到一个带版本的共享存储里。

### C-04 校准器只从 chat 形状的 prompt 采样，embedding/rerank 的估算扣费完全不经过校准

- **类别**：半接线
- **位置**：`internal/api/common.go:555`、`:653`、`:676`
- **设计意图**：`EstimateCalibrator` 的 doc 覆盖所有估算场景——"Estimates (**client aborts, upstreams that omit usage**) then use the learned ratio, so the amount we charge converges on what the provider actually bills"。
- **现状**：`Observe` 的唯一调用点是 `resolveChatUsage`（只服务 chat/completions/responses/anthropic 这类 chat 形状），`Adjust` 也只在这几条估算岔路上被调用（`streamFallbackPromptTokens`、`resolveChatUsage`、499 的 abort 估算）。`resolveEmbeddingTokens`/`resolveRerankTokens` 在上游没回 usage 时用**裸** `estimateTokens` 估算并发起扣费，既不观察也不校准；而 embedding/rerank 模型基本不会从 chat 路径积累样本，所以即便接上 `Adjust` 也拿不到比值。同一个「上游没报 usage 就估算扣费」的规则，在 chat 上带校准、在 embedding/rerank 上不带。
- **证据**：
  ```go
  // internal/api/common.go:555、560-561（唯一 observe/adjust 入口）
  func observePromptEstimate(modelName string, localEstimate, upstreamActual int) {
      billing.GlobalCalibrator.Observe(modelName, localEstimate, upstreamActual) }
  func calibratedPromptEstimate(modelName string, localEstimate int) int {
      return billing.GlobalCalibrator.Adjust(modelName, localEstimate) }
  ```
  ```go
  // internal/api/common.go:660-665、683-687（未经校准的估算扣费）
  return estimateTokens(chars), usageSourceEstimated
  ...
  return estimateTokens(chars), usageSourceEstimated   // rerank: query + 所有候选文档
  ```
- **影响**：低 — embedding/rerank 单价低、usage 大多由上游返回，但「估算口径统一」这一声明在这些入口上是假的。
- **置信度**：中
- **建议**：要么让这两条估算也走 `calibratedPromptEstimate`（并在 chat 也就同一模型采样），要么修改校准器注释，明确校准只覆盖 chat prompt。

### C-05 `EstimateCalibrator.Ratio()` 没有任何消费者：校准的可信度（样本数）对外不可见

- **类别**：死接线
- **位置**：`internal/billing/calibration.go:125`
- **设计意图**：注释写 "Ratio reports the learned ratio **and sample count** for a model. Ratio is 1.0 before any observation." —— 这是给运维看「这个模型学到多少、能不能信」的诊断入口。
- **现状**：全仓（含 web、controlplane、admin 接口）没有任何 `Ratio(` 调用，只有 `calibration_test.go`/`calibration_persist_test.go` 用它做断言。Prometheus 侧只导出了每模型偏差 `airoute_billing_estimate_bias_ppm{model=...}`（`metrics.go:290` → `metrics_prometheus.go:154`），**没有样本数**；而 `Adjust` 恰恰在 `count < calibrationMinSamples(5)` 时静默返回原值，运维无法从任何界面区分「比值已生效」和「样本不够、校准没生效」。
- **证据**：
  ```go
  // internal/billing/calibration.go:125-135
  func (c *EstimateCalibrator) Ratio(modelName string) (float64, uint64) { ... }
  // grep 结果：生产代码 0 调用（仅 calibration_test.go:24/43/52/63、calibration_persist_test.go:30/47）
  ```
  ```go
  // internal/billing/calibration.go:113-115（样本不足即静默不生效）
  if sample == nil || sample.count < calibrationMinSamples {
      return localEstimate
  }
  ```
- **影响**：低 — 没有错误计费，但「校准到底生效了没有」不可观测，排障只能猜。
- **置信度**：高
- **建议**：把样本数并入 `SetEstimateBias` 的 per-model 导出（或加 `airoute_billing_estimate_samples{model=...}`），删掉裸 `Ratio`。

### C-06 校验规则字段 `disallow_temperature` 只有 OpenAI 校验器消费，anthropic/gemini 校验器收到 `rule` 却完全不读

- **类别**：半接线
- **位置**：`internal/validator/anthropic.go:14`、`internal/validator/gemini.go:13`、`internal/validator/openai.go:112`
- **设计意图**：`ModelValidationRule` 是跨协议共享结构（`internal/config/validation_rules.go:9-15`），带 `Protocol string // "openai", "anthropic", "gemini"` 与 `DisallowTemperature bool // e.g. for o1/o3 reasoning models`；`pipeline.go` 三个入口都把解析出的 `rule` 传进对应的校验器。
- **现状**：`ValidateAnthropic` 与 `ValidateGemini` 的 `rule` 形参**在函数体内零引用**（grep 命中只有签名那一行），`disallow_temperature` 只在 `openai.go:112` 被读。因此一条 `{model: "claude-*", protocol: "anthropic", disallow_temperature: true}` 或 gemini 的对应规则会被正常加载、正常匹配、正常「生效」，却永远不拦温度参数——配置面与执行面不一致。同协议下 `Level` 生效、`DisallowTemperature` 静默失效，读代码无法区分这是遗漏还是「本来只给 OpenAI」。
- **证据**：
  ```go
  // internal/validator/anthropic.go:14（rule 未被引用）
  func ValidateAnthropic(req *model.AnthropicInboundRequest, rule *config.ModelValidationRule, level string) *ValidationResult {
  // internal/validator/gemini.go:13（同上）
  func ValidateGemini(req *provider.GeminiRequest, rule *config.ModelValidationRule, level string) *ValidationResult {
  ```
  ```go
  // internal/validator/openai.go:112-113（唯一消费点）
  disallowTemp := (rule != nil && rule.DisallowTemperature)
  if disallowTemp { if req.Temperature != nil && *req.Temperature != 1.0 { ... } }
  ```
- **影响**：中 — 运维以为给推理模型下的温度约束在全协议生效，实际只有 OpenAI 入口生效；Anthropic/Gemini 入口仍可传自定义 temperature。
- **置信度**：高
- **建议**：在两个校验器里补上同一段 `rule.DisallowTemperature` 检查（anthropic 的 `req.Temperature`、gemini 的 `generationConfig.temperature`），或把字段改名/加注释限定为 openai-only 并在配置校验里拒绝其他协议的该字段。

### C-07 `ValidationError.Error()` 从未被调用：校验失败既无审计原因也无访问日志原因

- **类别**：死接线
- **位置**：`internal/validator/errors.go:19`
- **设计意图**：`ValidationError` 实现了 `error` 接口（`Error()` 会拼出 `[protocol] invalid field: message`），预期是作为错误值被返回、打日志、进审计行。
- **现状**：三个入口都是 `if !valRes.Valid && valRes.Error != nil { valRes.Error.WriteGinResponse(c); return }`（`handler.go:96-99`、`inbound_anthropic.go:71-73`、`gemini_inbound.go:166-168`），随后直接 `return`——**没有** `c.Set(middleware.ContextKeyErrorMessage, ...)`、没有 `RecordUsage`、没有 WARN 日志。所以校验拒绝：客户端只拿到协议格式的错误 JSON，服务端访问日志里这一行 400 的 error_message 为空，`usage_logs` 也没有行，事后无法知道是哪个字段/哪条规则拒的。`Error()` 之所以「不可达」，是因为这个类型从未作为 error 值流动，只当响应渲染器用。
- **证据**：
  ```go
  // internal/validator/errors.go:19-27
  func (e *ValidationError) Error() string { ... }   // grep：生产代码 0 调用
  ```
  ```go
  // internal/api/handler.go:96-103（拒绝后直接 return，未写审计原因）
  if !valRes.Valid && valRes.Error != nil { valRes.Error.WriteGinResponse(c); return }
  if len(valRes.Warnings) > 0 { c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; ")) }
  ```
- **影响**：低 — 不影响金额（被拒请求不扣费），但校验拦截在审计面上完全静默，`disallow_temperature` 之类的配置是否在拦人无法验证。
- **置信度**：高
- **建议**：拒绝分支里 `c.Set(middleware.ContextKeyErrorMessage, valRes.Error.Error())`（或打一条限流 WARN），让访问日志能说出被拒原因。

### C-08 请求指标在 router 与 api 两层各记一次：`requests_total` / `failed_total` / 延迟直方图约为真实值的 2 倍

- **类别**：重复漂移
- **位置**：`internal/router/dispatcher_fallback.go:144`、`internal/api/common.go:249`
- **设计意图**：`RecordRequestWithModel` 的注释是 "records a completed request with its model, duration and token count"；`RecordRequest` 同样是 "records a completed request"。两个函数语义相同，只是在「带不带 model 维度」上分层，看起来设计意图是「router 记底层、api 记带模型的那一份」。
- **现状**：一次成功变成两次自增：router 在 `recordChannelSuccess` 里 `RecordRequest(true, 派发耗时, ...)`（同时计入 `SuccessfulRequests` 与延迟桶），api 的 `recordUsage` 又 `RecordRequestWithModel(params.Model, success, 端到端耗时, ...)`。失败同样双计：`dispatch`/`DispatchHTTP` 全渠道失败时 `RecordRequest(false,0,0,0)`（`dispatcher.go:364`、`dispatcher_modality.go:239/316/390`），api 的 `recordFailedRequestWithError → RecordUsage` 再记一次 failed。于是 `airoute_requests_total`、`airoute_requests_failed_total`、`airoute_request_duration_ms_count` 都约 2×，且与 `usage_logs` 行数、控制台「总请求数」不一致；两种耗时混在同一组延迟桶里（派发耗时 vs 端到端耗时），P50/P99 被拉低。
- **证据**：
  ```go
  // internal/router/dispatcher_fallback.go:141-145（成功时 router 记一次）
  func (d *Dispatcher) recordChannelSuccess(...) string {
      d.circuitBreaker.RecordSuccess(chName)
      telemetry.GlobalMetrics.RecordRequest(true, dur, promptTokens, compTokens)
  ```
  ```go
  // internal/api/common.go:248-256（同一请求 api 再记一次）
  } else {
      telemetry.GlobalMetrics.RecordRequestWithModel(params.Model, success, params.Duration, params.PromptTokens, params.CompletionTokens)
  }
  ```
  ```go
  // internal/router/dispatcher.go:364（失败时 router 记一次，api 还会再记）
  telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
  ```
- **影响**：中 — Prometheus 的 QPS/错误率/成功率/P99 全部失真（分母 2×、失败率被同一比例放大），与数据库口径对不上；`deploy/prometheus` 的告警规则因此要么误报要么钝化。
- **置信度**：高
- **建议**：二选一：router 只 `RecordFallback`/熔断，不再自增请求计数；或 api 层不再重复计数并明确「Prometheus 记派发、DB 记请求」的口径。

### C-09 启动定价检查的第二个分支被第一个分支的空结果短路，「上游实际服务模型未定价」几乎永不告警

- **类别**：死接线
- **位置**：`cmd/airoute/server.go:289`、`internal/billing/startup_check.go:41`
- **设计意图**：`UnpricedServedModelsInUse` 的注释说明它存在的理由——"A channel can rewrite the client-facing alias (… turns deepseek-v4.1-flash into volcengine/deepseek-v4-1-flash-261017), and if that upstream model is unpriced the charge silently falls back to the alias price or the default rate card - either way it is a guess."
- **现状**：两个检查彼此独立（别名是否有价 vs 渠道映射后的上游模型是否有价），但 server 把第二个检查**嵌在第一个检查「结果非空」的分支里**：`if len(unpriced) == 0 { return }` 在 `UnpricedServedModelsInUse` 调用之前。最常见的配置状态恰恰是「所有别名都配了价」→ 第一个列表为空 → 直接 return → 第二个检查永不执行。而「别名有价、上游模型没价」正是这条检查要抓的场景（计费按别名价走，与厂商账单对不上），现在它永远不会触发。两个函数的检查对象与计费表同源（都调 `ResolvePriceSource`），问题只在调用编排。
- **证据**：
  ```go
  // cmd/airoute/server.go:284-303（第二个检查被第一个短路）
  unpriced, err := billing.UnpricedModelsInUse(repo, billing.GlobalEngine, 7*24*time.Hour)
  if err != nil { ...; return }
  if len(unpriced) == 0 {
      return                       // ← 第一个为空就结束，后面的检查不再执行
  }
  ...
  servedUnpriced, servedErr := billing.UnpricedServedModelsInUse(repo, billing.GlobalEngine, 7*24*time.Hour, dispatcher.ResolveServedModel)
  ```
- **影响**：中 — 渠道模型映射导致的「按别名计价」长期无人发现（正是 `billingModelFor` 与 `ResolvePriceSource` 那套逻辑想解决的计费失真）。
- **置信度**：高
- **建议**：把两个检查拆成同级的两段（各自判断、各自告警），别用第一个的结果门控第二个。

### C-10 分时优惠：非法折扣在行级「钳到原价」，在槽位级却回落到行级折扣；校验器只查重叠、不查引擎真正读的字段

- **类别**：重复漂移
- **位置**：`internal/billing/strategy.go:92`、`internal/storage/repo_pricing.go:197`、`internal/billing/strategy_validation.go:21`
- **设计意图**：行级 `normalizeModelPrice` 立了规矩——"0 (or an unset field) means **no discount**. It used to become 0.5, so a rule the operator created without a coefficient silently billed half price … never invent a discount."；`CustomOffPeakStrategy` 的注释也声明槽位冲突用 "Best Discount Rule" 解决。
- **现状**：同一条「非法折扣不得凭空造出折扣」的规则，槽位级做法相反：slot 里 `discount` 若 `<=0 || >=1.0`，不报错、不钳到 1.0，而是**替换成行级 `ctx.Discount`**（通常是 0.5）——运维写 `1.5`（想要加价）或 `0`（不想要折扣）时，实际得到 5 折。与此同时 `ValidateSlotsOverlap` 只校验时间格式与「同日时段不重叠」，既不校验引擎真正读的 `Days`（写 0/8 时该槽位永不匹配、静默失效）与 `Discount`，而它禁止的「重叠」恰恰是引擎用最低折扣已经确定性地处理掉的场景；另外引擎只在 `mode=="custom"` 时读 slots（`engine.go:365-371`），但两个写入方在 `mode` 为 `night`/空时仍会对 slots 做重叠校验，校验对象与执行对象不一致。
- **证据**：
  ```go
  // internal/billing/strategy.go:90-95（槽位非法折扣 → 回落到行级折扣，凭空造出 5 折）
  sM := parseTimeToMinutes(slot.Start); eM := parseTimeToMinutes(slot.End)
  d := slot.Discount
  if d <= 0 || d >= 1.0 { d = ctx.Discount }
  ```
  ```go
  // internal/storage/repo_pricing.go:197-203（行级非法折扣 → 原价，明确「不造折扣」）
  if rec.OffPeakDiscount <= 0 || rec.OffPeakDiscount > 1.0 {
      rec.OffPeakDiscount = 1.0
  }
  ```
  ```go
  // internal/billing/strategy_validation.go:56-70 附近：只校验时间/重叠，Days、Discount 未校验
  ```
- **影响**：中 — 槽位配置写错时会**少收 50%**（而非按原价或多收），且没有任何报错；同时重叠校验在 `mode!=custom` 时校验的是一份引擎根本不会读的配置。
- **置信度**：高
- **建议**：槽位非法折扣与行级同口径钳到 1.0 并记录告警；`ValidateSlotsOverlap` 补 `Days∈1..7`、`Discount∈(0,1)` 校验，并仅在 `mode=="custom"` 时执行。

### C-11 定价的模型通配符只认「尾部 `*`」，与校验规则的通配符语义不一致，且保存时不校验 pattern

- **类别**：重复漂移
- **位置**：`internal/billing/engine.go:113`、`internal/config/validation_rules.go:46`
- **设计意图**：两处都在实现「按模型名通配符匹配一条规则」这同一个概念：定价的 `wildcardRule`（注释："wildcard lists every `*`-suffixed pattern … so the most specific rule always wins"）与 `ModelValidationRule.Model`（注释："glob/prefix pattern, e.g. `claude-3-5-sonnet*`, `o1*`"）。
- **现状**：定价侧只把 `HasSuffix("*")` 的行放进 wildcard 索引，其余一律当**精确模型名**存进 map（`m[grp+":"+model]`），匹配时也只做 `strings.HasPrefix`；校验规则侧则支持前缀 `x*`、后缀 `*x`、`*` 与精确四种。而定价保存路径（`SaveRate → ValidatePriceRecord`）只校验币种与「价格非负」，对 model 字段只要求非空。于是 `*o1`、`gpt-4o*mini` 这类在配置语法上完全合理、在镜像的校验规则语法里也成立的 pattern 会被接受并落库，然后**永远不匹配任何请求**，静默回落到默认价/兜底价——运维看到「规则存在」，实际从未生效。
- **证据**：
  ```go
  // internal/billing/engine.go:112-118（只认尾部 *）
  modelLower := strings.ToLower(strings.TrimSpace(rec.Model))
  if strings.HasSuffix(modelLower, "*") {
      wildcards = append(wildcards, wildcardRule{group: grp, prefix: strings.TrimSuffix(modelLower, "*"), price: rec})
  }
  ```
  ```go
  // internal/config/validation_rules.go:46-56（同一概念支持四种语义）
  if pat == "*" || pat == "" { matched = true
  } else if strings.HasSuffix(pat, "*") { matched = strings.HasPrefix(modelLower, strings.TrimSuffix(pat, "*"))
  } else if strings.HasPrefix(pat, "*") { matched = strings.HasSuffix(modelLower, strings.TrimPrefix(pat, "*"))
  } else { matched = (pat == modelLower) }
  ```
  ```go
  // internal/controlplane/pricing_service.go:188-192（保存时只看非空）
  rec.Model = strings.TrimSpace(rec.Model)
  if rec.Model == "" { return &ValidationError{Message: "模型标识为必填项"} }
  ```
- **影响**：中 — 价格规则静默失效（少收或多收），且 `UnpricedModelsInUse` 只在启动时按 7 天窗口提示一次，不会指出「你有一条永远匹配不上的规则」。
- **置信度**：中
- **建议**：定价保存时校验 pattern 只允许「精确 / 尾部 `*` / 单个 `*`」，并把不合法 pattern 明确报错；或统一两处通配符实现（抽一个 `MatchModelPattern`）。

### C-12 Redis RPM 滑动窗口与本地令牌桶语义不等价，Redis 故障时本地回退会额外放行一整个窗口

- **类别**：重复漂移
- **位置**：`internal/distributed/redis_quota.go:13`、`internal/middleware/ratelimit.go:83`
- **设计意图**：限流是同一个产品语义（每 key 每分钟 N 次），`docs/CONFIGURATION.md:318` 写明 "RPM is enforced with a distributed Redis sliding window (or a local token bucket without Redis)"，`authz.go:247` 把本地桶称作 "the same fail-open path"。
- **现状**：两套实现不等价。Redis 版是严格 60s 滑动窗口：`ZREMRANGEBYSCORE` 清 60s 外 + `ZCARD < rpm`，任何 60s 窗口内最多 rpm 次。本地版是**令牌桶**：`capacity=rpm`、`rate=rpm/60`，桶初始**装满**，空闲后回满——所以一个空闲过的 key 可以瞬间打满 rpm 次、随后再以 rpm/60 持续放行，任意 60s 窗口内可显著超过 rpm。更关键的是故障回退：Redis 查询报错/超时时直接落到本地桶，而该桶此时往往是满的，等于对已经在 Redis 窗口里花掉配额的 key **额外再发一整份 rpm**；已有条目要等 Redis 恢复后才重新参与判定。同类实现、更严格的语义在分布式侧、更宽松的在单机侧，且 `Retry-After` 等都写 1s，但两者窗口模型不同。
- **证据**：
  ```lua
  -- internal/distributed/redis_quota.go:20-28（严格 60s 滑动窗口）
  redis.call('ZREMRANGEBYSCORE', key, 0, clearBefore)
  local current = redis.call('ZCARD', key)
  if current < limit then ... ZADD ... return 1 else return 0 end
  ```
  ```go
  // internal/middleware/ratelimit.go:84-85、113-118（满桶起步的令牌桶）
  expectedCapacity := float64(rpm)
  expectedRate := float64(rpm) / 60.0
  b = &tokenBucket{tokens: expectedCapacity, capacity: expectedCapacity, rate: expectedRate, lastUpdate: time.Now()}
  ```
  ```go
  // internal/middleware/authz.go:247-252（Redis 出错即回退本地满桶）
  // On Redis error/timeout, gracefully fall through to the local bucket.
  bucket := GlobalRateLimiter.GetBucket(key.Key, key.RPM)
  ```
- **影响**：低 — 限流是软约束，但「Redis 抖动 = 瞬时配额翻倍」与「分布式严格、单机宽松」会让压测/容量评估结论失真。
- **置信度**：中
- **建议**：本地回退改成与 Redis 同语义的滑动窗口（或令牌桶 capacity=1 起步不预填），并在文档里写明故障回退会额外放行。

### C-13 `moderation` 是唯一缺少「定价币种前置校验」的计费入口

- **类别**：不对称强制
- **位置**：`internal/api/moderation_handler.go:121`（对照 `internal/api/handler.go:81` 等 10 处）
- **设计意图**：`validateModelCurrency` 的注释——"The wallet settles only CNY. **Reject a legacy foreign rate before work starts.**"；同一个 `moderation_handler.go` 自己在别处也会用 `c.Set(ContextKeyErrorMessage, ...)` 表达拒绝，说明该文件并非豁免约束的特例。
- **现状**：`responses`、`chat`、`completions`、`embedding`、`rerank`、`audio`(×2)、`video`、`image`、`gemini`、`anthropic`(×2)、MCP（`validateMCPModelCurrency`）都在调用上游前做币种门；`moderation` 的代理分支直接 `DispatchHTTP` 后 `RecordUsage(... BillableUnits: 1)`。若该模型的价目行是历史非 CNY 行，其他入口会 422 `unsupported_pricing_currency`，moderation 却会正常调用上游、返回 200，然后在 `recordUsage` 里被 `normalizeUnbillableCurrency` 把成本清零（`common.go:218-221`）——上游真花了钱、网关收 0，只有一个 Prometheus 计数（`BillingChargesRejected`）记录。
- **证据**：
  ```go
  // internal/api/moderation_handler.go:110-129（无 validateModelCurrency，直接计费）
  if resp.StatusCode >= 200 && resp.StatusCode < 300 {
      ...
      RecordUsage(c, AuditRecordParams{ SessionID: sessionID, Channel: resp.Channel, Model: modModel,
          Duration: dur, StatusCode: resp.StatusCode, BillableUnits: 1 })
  ```
  ```go
  // internal/api/common.go:877-886（其他入口用的前置门）
  func validateModelCurrency(c *gin.Context, modelName string) bool {
      if err := storage.ValidatePriceCurrency(billing.GlobalEngine.GetPriceWithGroup(modelName, getKeyGroup(c))); err != nil {
          RespondOpenAIError(c, http.StatusUnprocessableEntity, ...); return false
      }
  ```
- **影响**：低 — 兜底逻辑保证不会错扣，但「前置拒绝」与「事后清零」两种口径并存：同样的历史脏价目，moderation 让上游白干、审计行 0 元，其他入口直接 422。
- **置信度**：高
- **建议**：在 moderation 的 `GetChannelsForModelAndProtocol` 前补一次 `validateModelCurrency(c, modModel)`，与其他入口同口径。

### C-14 币种前置门校验的是「客户端请求模型」的价目，真正扣费用的却是「渠道实际上游模型」的价目

- **类别**：不对称强制
- **位置**：`internal/api/common.go:882`（校验别名） vs `internal/api/common.go:85`（扣费用 served 模型）
- **设计意图**：`billingModelFor` 明确「按渠道映射后的实际服务模型计费」（"The upstream invoices us for the model it served, so billing must follow the channel's model mapping"）；`validateModelCurrency` 的职责是「让无法结算的价目在开工前被拒」。
- **现状**：两者取价的模型不同。`validateModelCurrency(c, req.Model)` 用**请求模型**（别名）取价目，而 `recordUsage` 用 `billingModelFor(params, keyGroup)`——当渠道把别名映射到另一个模型、且该模型有显式价目时，计费按 **served 模型**的价目行算。若 served 模型的价目行是历史非 CNY 行，前置门看的是别名行（CNY，通过），真正计算时 `normalizeUnbillableCurrency` 才把成本清零：请求 200、`X-Airoute-Cost: 0`、上游已花费。设计上「前置拒绝」在「按 served 模型计费」这条新链路上只覆盖了一半。
- **证据**：
  ```go
  // internal/api/common.go:85-96（扣费取 served 模型）
  func billingModelFor(params AuditRecordParams, keyGroup string) string {
      served := servedModelResolver(params.Channel, params.Model)
      if billing.GlobalEngine != nil && billing.GlobalEngine.ResolvePriceSource(served, keyGroup) != billing.PriceSourceDefault {
          return served
      }
      return params.Model
  }
  ```
  ```go
  // internal/api/common.go:878-887（前置门取请求模型）
  if err := storage.ValidatePriceCurrency(billing.GlobalEngine.GetPriceWithGroup(modelName, getKeyGroup(c))); err != nil {
  ```
- **影响**：低 — 有 `normalizeUnbillableCurrency` 兜底不会错扣金额，但「开工前拒绝」的承诺在 served-model 计费下不成立；同时 `serve` 模型价目为 CNY 而别名行是外币时，前置门会误拒一个本来能正常结算的请求。
- **置信度**：高
- **建议**：让 `validateModelCurrency` 与 `billingModelFor` 共用同一个「扣费模型解析」结果（把 served 解析下沉到公共函数），或在 `recordUsage` 里把 `normalizeUnbillableCurrency` 命中视为需要记录/告警的异常而非静默。

## 已排除的疑似项

1. **`internal/telemetry/context.go:72 ResetWarnGate`（deadcode）**：注释明确写 "Tests use it to keep cases independent; **production code never needs it**"，属文档说明的测试辅助（`internal/distributed/redis_redact_test.go:16`、`internal/storage/trace_logging_regression_test.go:17` 等使用）。其背后 `warnGate` 是 `sync.Map`，键的基数由渠道/固定字符串决定（`breaker-open:<channel>` 等），不构成无界增长。
2. **`internal/telemetry/metrics.go` 指标「定义但永远为 0」**：逐字段核对了 28 个原子计数器/直方图的**自增点**与 **Prometheus 导出点**，全部双向闭合（如 `BillingWalletDriftUnits` 由 `billing_reconcile_job.go:131` 刷新、`BillingDuplicatesSkipped` 由 `repo_logs_writer.go:212/249` 自增，全部在 `metrics_prometheus.go` 导出）；未发现只在测试里自增、或热路径自增但未导出的指标。
3. **`BillingReconciler`（漂移/审计滞后/未链接账面）**：`InitBillingReconcile` 在 `cmd/airoute/server.go:261` 已启动、5 个 gauge 均被 `deploy/prometheus/airoute-alerts.yml` 引用；不是「算了没人读」。
4. **校准「只写不读」**：`LoadCalibration`（`server.go:227`）+ `Adjust`（`common.go:561`）都在真实路径上，校准值确实参与估算扣费；真正的问题是跨副本覆盖（C-03）与覆盖面（C-04），不是「没人读」。
5. **`distributed` 的降级分支是否「看起来支持多副本、实际永远走本地」**：逐一核实后**降级分支都真实实现**——`StartLiveStatsPublisher` 无 Redis 直接 no-op、`AggregateLiveSnapshot` 读失败返回本地快照（`admin_live_stats.go:35` 仍被调用）、`AllowRPM`/`TryAcquireInFlightQuota` 均有本地回退；`LiveSnapshot.Nodes` 由 `mergeLiveSnapshots` 按 peer 数重算（`distributed/live.go:144/155`），不是永远 1。
6. **TPM 没有 Redis 分布式实现**：`docs/CONFIGURATION.md:318-319` 明确只把 RPM 写成分布式滑动窗口、TPM 写成 "per-key, one-minute sliding window"，属已声明的设计差异；副作用（多副本下 TPM 实际为 N×）记录在此，不单列发现。
7. **`airoute_active_connections` 只有 5 个 chat 形状入口自增**（chat/completions/responses/gemini/anthropic，`handler.go:105`、`completions_handler.go:133`、`responses_handler.go:101`、`gemini_inbound.go:207`、`inbound_anthropic.go:79`），embedding/rerank/moderation/多模态不自增：`telemetry/live.go:177-179` 的注释已显式列出这 5 个协议，属代码内说明的既有范围，未单列（若要求「所有在途请求」，则这是待补项而非前后矛盾）。
8. **路由编辑器新建价目行默认开启分时优惠（`route_service.go:185-191`：`offEnabled=true`、折扣 0.5、周末全天）**：与 DB schema 默认值一致（`migrations_sqlite.go:149-155`：`off_peak_enabled DEFAULT 1`、`off_peak_discount DEFAULT 0.5`、`weekend_all_day DEFAULT 1`），且已有行会沿用原值；engine 兜底价目 `OffPeakEnabled:false` 只针对「没配价的模型」，注释亦如此说明。判定为有意默认，不报。
9. **`ToUnits(Σ成本)` 与 `Σ ToUnits(成本)` 的双重换算漂移（`settlement.go:187` vs `:239`）**：理论上逐行取整与聚合后取整会差几单位，但 `billing.roundPrecision` 先把成本钳到 1e-8 元，转 1e-9 units 后恒为 10 的整数倍，求和后再取整不会产生偏差；实测口径不会触发 `airoute_billing_ledger_drift_units` 告警。属可忽略的理论差异。
10. **`settleCharges`（同步结算/批量写入）两套入口**：在 `1520d99` 上，`SettleChargeNow → SettleUsageCharge → settleCharges` 与审计批量写入 `flush → settleCharges` 走的是**同一个** `settleCharges` + `insertSettlementRows`，`cost_units` 都用 `ToUnits(log.Cost)`，未发现两套独立结算实现漂移（`repo_logs_writer.go:124-130` 已把「先钱后行」写成单一路径）；`double-rounding` 亦不产生实际偏差（见上一条）。并行会话正在把结算改为分组提交（新增 `SettleChargeBatched`、指标 `airoute_settlement_*`），该改动仍在同一条 `settleCharges` 实现上做批量，属性能/时延重构而非两套账本；若后续出现「请求路径直接写余额 + 批量路径也写余额」两套实现，则应重开此条。
11. **`categorizeLog` 的消息文本启发式**（`ring_buffer.go:345`）：注释明说只作无 category 调用点的兜底、子系统应使用 `ScopedLogger`，属有意保留。

## 覆盖面

- **审计基线**：commit `1520d99`（`git log --oneline -1` 在审计开始时确认；期间 HEAD 前移到 `190b382 docs(billing): spell out the quota-budget ceiling and replica propagation delay`，该提交只改 `docs/CONFIGURATION.md`，不影响代码结论）。本文所有行号以 `1520d99` 为准。
- **并行改动（工作区未提交，非本次审计内容，但会影响引用的行号/实现）**：审计进行中，另一会话正在实现「结算分组提交 + 配额预授权改整数单位」，`git diff 1520d99` 涉及 `cmd/airoute/server.go`、`internal/api/{common,chat_stream_handler,completions_stream_handler,gemini_inbound_stream,inbound_anthropic_stream,responses_stream}.go`、`internal/distributed/redis_quota.go`、`internal/middleware/{authz,quota_tracker}.go`、`internal/storage/{db,repo_pricing,repo_users,settlement,wallet_deduct}.go`、`internal/telemetry/{metrics,metrics_prometheus}.go`。对本文结论的影响逐条核对如下：
  - **不受影响（代码未改）**：C-01、C-05、C-06、C-07、C-09、C-10（`normalizeModelPrice` 未改，`ListModelPrices` 仅改为走 `Reader()`）、C-11、C-13、C-14；C-02/C-04/C-08 所在的 `common.go` 改动只重排了「结算 vs 入队」顺序、新增 `RecordUsageDeferred`/`settleUsage`，**计费公式、`CacheCreationTokens` 透传点、`Observe/Adjust` 调用点、请求计数调用点均未变**。
  - **部分被并行工作覆盖**：C-12 的 `AllowRPM`/`slidingWindowLua`（严格 60s 滑动窗口）在工作区中**仍未改**，本地令牌桶（`middleware/ratelimit.go`）也未改，因此「两套窗口语义不等价」依旧成立；但 `redis_quota.go` 的在途配额已从「固定 0.0005 浮点」改为「整数单位金额预授权 + 并发上限」，与本报告未单列的「固定预留成本」问题重叠。
  - **已排除项 2/10 的口径变化**：`metrics.go`/`metrics_prometheus.go` 新增了 7 个结算批量/预授权指标（额度/预授权相关指标尚未全部接入导出，属并行会话在途），本文「28 个指标双向闭合」的结论仅对 `1520d99` 成立；`settlement.go` 正在改为分组提交，基线里「同步结算与异步批量写入共用同一个 `settleCharges`」的结论描述了 `1520d99` 的结构。
  - **新增未跟踪文件（不在 `1520d99` 基线内，未纳入审计结论）**：`internal/billing/estimate.go`（准入侧成本上界估算，刻意不含缓存/分时折扣，属**估算**而非扣费路径，不影响 C-01「扣费只有一个入口」）、`internal/telemetry/histogram.go`（结算直方图）、`internal/storage/settlement_batcher.go`（分组提交）。
  - **只读边界**：除本文件外未写入任何文件；未修改生产代码/测试代码。
- **逐行读完**：`internal/billing/{engine,strategy,strategy_validation,calibration,calibration_persist,startup_check}.go`；`internal/validator/{errors,pipeline,types,openai,anthropic,gemini}.go`；`internal/telemetry/{metrics,metrics_prometheus,logger,ring_buffer,live,context}.go`；`internal/distributed/{redis,redis_quota,redis_streams,live}.go`。
- **跨域只读（确认谁读/谁不读）**：`internal/api/common.go`（全文）、`handler.go`、`inbound_anthropic.go`、`inbound_anthropic_stream.go`、`gemini_inbound.go`、`responses_handler.go`、`embedding_handler.go`、`rerank_handler.go`、`multimodal_handler.go`、`audio_handler.go`、`moderation_handler.go`、`mcp_tools_routing.go`；`internal/config/validation_rules.go`；`cmd/airoute/server.go`；`internal/controlplane/{pricing_service,admin_pricing,admin_live_stats}.go`；`internal/storage/{repo_pricing,repo_model_routes,settlement,billing_reconcile_job,repo_logs_writer,money}.go`；`internal/middleware/{authz,ratelimit,quota_tracker,tpm_tracker,auth}.go`（仅用于判定分布式两套实现是否一致）；`docs/audit/billing.md`、`docs/CONFIGURATION.md:318-319`、`docs/design/v1.1-scope-cut-2026-10-28.md`。
- **deadcode 命中符号逐条核实**：`calibration.go:125 Ratio`（→C-05，仅测试）；`engine.go:375/391/510/515/521`（→C-01，仅测试与死链）；`telemetry/context.go:72 ResetWarnGate`（→已排除 1，文档说明测试用）；`validator/errors.go:19 ValidationError.Error`（→C-07，仅测试；生产用 `WriteGinResponse`）。staticcheck 未提供有效结果，未依赖。
- **检索手段**：`grep -rn <symbol> --include=*.go` 区分生产/测试引用（每个符号都核对了 `*_test.go` 命中清单）；`git log --oneline -1` 确认基线。
- **未覆盖**：`web/` 前端；`internal/storage` 结算/账本内部实现的深度审计（并行会话正在做的「同步落库 vs 批量写入」主题，本次只做交叉验证并据实记录于已排除 10）；`internal/middleware` 的准入业务逻辑本身（仅比对分布式两套实现）。
