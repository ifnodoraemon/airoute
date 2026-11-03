# Lead 自查发现（2026-10-29）

> 来源：上线后复核 + 对上一轮新增代码的针对性自查。这里的条目**已由 Lead 本人用可执行探针确认**，交给对应域做正式复现与修复。

---

## LEAD-01（P1，金额 + 功能）`ParseMessageContent` 不认 typed `[]model.ContentPart`，协议适配路径的图片同时被「当文本计费」和「当文本转发」

### 位置
- 根因：`internal/model/multimodal.go:38 ParseMessageContent` 只处理 `string` 与 `[]any`，其余（含 `[]ContentPart`）走 `json.Marshal` 回退成**文本**。
- 产生 typed 内容的地方（协议入口转换）：
  - `internal/adapter/anthropic.go:132`（`Content: contentParts`，Anthropic 入口的图片 → `image_url` part）
  - `internal/adapter/gemini.go:66,77`（Gemini 入口同理）
- 受影响的下游：
  - 计费/取消路径：`internal/router/dispatcher.go:297,301`、`dispatcher_stream.go:26,151`、`dispatcher_modality.go:332,340,430,438` 都调 `req.EstimatedPromptTokens()`；
  - 出站协议转换：`internal/provider/anthropic_convert.go:84`、`gemini_convert.go:26,83`、`openai_text_adapter_convert.go:47` 都调 `ParseMessageContent`。

### 触发条件
客户端用 **Anthropic Messages（`/v1/messages`）或 Gemini 原生协议**发带图请求，然后：
1. 上游**未回 usage**（或客户端中途断开 → 499 仍计费）：`EstimatedPromptTokens()` 用回退文本估算，base64 被字符数化；
2. 或该请求被路由到**原生 Anthropic / Gemini / openai_text** 渠道：出站转换看到 `hasImage=false`，把整段内容（含 `data:image/png;base64,...`）作为一个 **text block** 发给上游 → 图片丢失/上游报错。

### 证据（Lead 探针 + router 域独立复核）

> router 域复核（未采信、自行复现）结论：**成立**，实测 300 KB 图 + 一句文本 = **76,829 vs 771 tokens（99.6×）**；出站 `convertOpenAIToGemini` 对 typed 图片产出 **0 个 inlineData + 1 个含 base64 的 text part**，同内容 `[]any` 表示正确。探针落在 `internal/model/typed_content_bugprobe_test.go`、`internal/provider/typed_content_convert_bugprobe_test.go`。

Lead 原始探针：
`internal/model` 内临时测试，构造同内容的两份请求（一份 typed、一份 `[]any`）：

```
typed []ContentPart estimate = 75028
[]any estimate               = 769
BUG: typed []ContentPart over-estimates (75028 tokens) — base64 counted as text
```

（300 KB base64 图片 + 一句文本；`[]any` 走新逻辑 = 769 tokens，typed 走 JSON 回退 = 75,028 tokens，约 **98×** 高估。）

### 影响
- **金额①：按 ~98× 高估计费。** `internal/api/common.go:703 resolveChatUsage` 在上游没回 `prompt_tokens` 时用 `estimateCanonicalPromptTokens(req)`（= `req.EstimatedPromptTokens()`）兜底并标 `usage_source=estimated`，随后进入结算；499 路径同理。
- **金额②：校准器被带偏（有边界）。** 同一函数在上游**回了** usage 的分支里，会用这个被放大的 `localEstimate`（75k）与真实值（约 800）去 `observePromptEstimate` 学习比例。一次带图请求就能把该模型的校准 ratio 压到下限 —— 但有 `calibrationFloor=0.4` 钳位（`internal/billing/calibration.go:45`），所以**最坏是持续少计约 2.5×**，不会归零（Lead 初判「压到 0.01」有误，bug-router 复核时修正）。只影响多模态请求；纯文本请求不经过这条路径。
- **功能：跨协议带图请求不可用。** Anthropic/Gemini 入口的图片在路由到 **Anthropic / Gemini / openai_text** 渠道时，出站转换（`provider/anthropic_convert.go:84`、`gemini_convert.go:26,83`、`openai_text_adapter_convert.go:47`）拿到的是 JSON 文本回退，`hasImage=false` → 整段（含 `data:...;base64`）作为 **text block** 发给上游：图片丢失/上游报错/按文本计费。OpenAI 渠道不受影响（canonical 直接 JSON 序列化，`ContentPart` 的 json tag 正确）。
- 结论：上一轮 `BUG-MULTIMODAL-TOKEN-ESTIMATE` 只修了 `[]any` 一种表示，**修不完整**；typed `[]ContentPart` 这条路径同时漏了计费口径与出站转换。

### 线上现状（未被污染的证据）
`system_settings` 里 `billing_token_calibration` 当前值：

```json
{"aliyun/glm-5.3":{"ratio":2.360699342498506,"samples":5},
 "deepseek-v4.1-flash":{"ratio":1.047968923126834,"samples":7704}}
```

两个比例都在合理区间（≈1–2.4），说明**还没有** Anthropic/Gemini 入口的带图请求把校准带偏——缺陷是潜伏的，不是已发生的账务事故。修复后无需清理存量校准，但应补回归防止首次带图流量触发投毒。

### 建议修复
在 `ParseMessageContent` 增加 `[]ContentPart`（以及单个 `ContentPart`）分支直接返回/复用，避免 JSON 回退；并补三条回归：
1. `EstimatedPromptTokens()` 对 typed 与 `[]any` 两种表示给出相同量级（≤ 图片预算 + 文本）；
2. `convertOpenAIToAnthropic`/`convertOpenAIToGemini` 对 typed 图片内容产出 `image`/`inlineData` part 而不是 text block；
3. 带图请求不会把该模型的估算校准比例带偏（`Observe` 只在表示一致时学习）。

修复后需评估是否清理已被投毒的校准状态（持久化位置见 `internal/billing/calibration_persist.go` 的表名），并抽查线上是否存在被压低的 ratio。

> 归属：`internal/model` + `internal/adapter` + `internal/provider`（router 域本轮负责人）；修复后请跑 `go test ./internal/{model,adapter,provider,api,router}/`。
