# 统一思考配置

思考能力按 **渠道 + 实际模型** 保存，只需要关心两件事：**能否关闭思考、支持哪些档位**。开源部署和闭源 API 共用这套能力配置；参数转换由接入协议决定，不要求用户区分实现方式。

调用时只选一个“思考”值：默认、关闭（模型支持时）或具体档位。选择档位同时开启思考，无需先开开关再选强度。只有开关、没有可调档位的模型使用默认强度。

例如，同一个 `deepseek-v4.1-flash` 路由可以连接两个渠道：GPUStack 使用 `chat_template_kwargs.thinking` 开关，兼容代理可能忽略这个字段。选择关闭思考时，只调度确实能够关闭的渠道；故障转移不能丢弃该要求。


控制台的配置分成三层：

| 位置 | 管什么 | 生效范围 |
| --- | --- | --- |
| 服务商 → 思考能力 | 支持关闭、支持档位 | 该渠道的实际模型 |
| 模型路由 → 默认思考 | 一个默认选项，限制项收在高级设置 | 该公开路由 |
| 调试演练 / 副驾驶 → 思考 | 本次调用的临时选择 | 本次请求 |

例如，路由默认“开启·高”，用户本次选择“关闭”，网关先检查路由是否允许，再从支持关闭的渠道中选路。没有思考摘要不能据此判定没有思考。能力待确认时保持上游默认，不向用户保证支持开关或某个档位。

路由展开详情能直接查看每个实际上游的能力和采购报价。采购报价归属渠道模型，对外售价归属公开路由，详见[渠道采购报价](PROCUREMENT.md)。

## 配置位置

在「服务商 → 编辑 → 思考能力」选择实际模型，只设置“支持关闭思考”和“支持档位”。后端按渠道协议和实际模型处理参数，前端不提供适配方案列表；特殊部署差异由后台维护。渠道的 `model_capabilities` 字段也可以通过 API 或 YAML 设置，通常不需要填写 `profile`：

```json
{
  "model_capabilities": {
    "deepseek-v4.1-flash": {
      "thinking": {"can_disable": false, "efforts": ["low", "high", "max"]}
    }
  }
}
```

键名必须是模型映射后的名称；`*` 表示此渠道的默认约定。一个实际模型被多个公开路由引用时，共用同一份能力。`unknown` 可显式禁止名称推断；`unsupported` 只适用于确认没有思考模式的模型，不能用来表示未知。

`can_disable` 省略时沿用适配方案，`false` 可关闭对外提供的关闭选项。`efforts` 省略或为 `null` 时继承适配方案，`[]` 表示没有可调档位、使用固定默认强度；JSON 和 YAML 导出均保留此区别。两个字段只能缩小已确认的能力，不能凭空增加上游不接受的选项。

「模型路由 → 编辑 → 路由思考策略」保存 `thinking_policy`：

```json
{
  "default_mode": "on",
  "default_effort": "low",
  "allowed_modes": ["on"],
  "allowed_efforts": ["low", "high"]
}
```

省略或 `null` 的允许列表表示不限；显式空列表表示不允许任何值。默认模式必须在允许范围内；限制强度时需明确默认开关，避免上游未知默认值绕过限制。保存时，默认设置及允许列表中的每个档位都必须至少有一个已启用的绑定渠道支持。仅更新思考策略、仅修改渠道绑定时，也会结合已保存配置校验。路由可以缩小可用范围，不能增加上游能力。

`minimal / low / medium / high / xhigh / max` 是统一词表，每个实际上游只支持自己的子集。路由编辑器、演练和副驾驶都按后端解析的能力显示选项，不会为了凑齐六档而推测支持。可选强度是“已启用绑定渠道的支持档位合集”与“路由允许范围”的交集。编辑器标明每档有几个渠道支持；选定该档后，只有这些渠道可以参与调用和切换。没有任何渠道支持的档位会被拒绝，不会静默替换为其他强度。

只支持开启思考但未确认可调档位的模型使用上游默认强度，不显示虚构档位。能力未知的渠道不贡献开关或档位选项；映射改动后先按上游默认保存，再查看后端解析结果。已有设置失效时保留原值并提示修正，不能静默降档。强度限制的空列表会阻止显式开启思考；要使用模型默认强度，应取消强度限制。

## 调用与调度

调试演练和副驾驶通过统一的 Chat Completions 扩展表达意图：

```json
{
  "model": "your-public-route",
  "messages": [{"role": "user", "content": "计算17乘23，只回答结果。"}],
  "thinking_control": {"mode": "on", "effort": "low", "include_summary": true}
}
```

- `mode`：`auto` 遵循路由默认值，未设置默认值时遵循上游默认值；`on`、`off` 是必须满足的要求。
- `effort`：仅在 `on` 时有效，必须属于该上游和路由共同允许的集合。不把 `low`、`medium`、`high` 假定为所有模型通用枚举。
- `include_summary`：与内部是否思考分开。Gemini 使用它请求 `includeThoughts`，并不保证每次都有可展示内容。控制台只在能请求摘要的调用方式上提供该选项。

每次先应用模型映射，再按同一个解析器检查路由策略和上游能力，过滤之后才进入负载均衡、会话亲和与熔断。没有合适候选时返回 400，不悄悄忽略开关或转换成另一档强度。跨模型兜底保留原路由已生效的要求，同时接受兜底路由的限制。

SDK 未携带扩展且路由没有思考策略时保持原有透传行为。原生参数可以覆盖仅含默认值的路由设置；路由存在硬性限制时，不能用原生字段绕过限制，应使用统一扩展。扩展与 `reasoning_effort`、`thinking`、`enable_thinking` 等原生字段不能混用。Gemini GenerateContent、Anthropic Messages、OpenAI Responses 在匹配的原生渠道上也能接受扩展；不具备对应参数约定的协议组合明确拒绝。

## 参数约定与来源

`internal/model/thinking_profiles.json` 是随版本维护的能力表，包含协议格式、开关、强度和资料来源。优先级是：**实际上游模型的显式配置 → 渠道默认配置 → 按渠道协议与实际模型推断 → 未知**。路由策略和请求参数始终受这些能力约束。

| 调用方式 | 适配方法 |
| --- | --- |
| OpenAI | `reasoning_effort`；使用包含答案与思考的 `max_completion_tokens` 上限；部分模型的思考工具调用必须使用 Responses |
| Gemini 3+ | `thinkingLevel`，摘要另设 `includeThoughts` |
| Gemini 2.5 | `thinkingBudget`；模型之间是否可以关闭存在差异 |
| Gemma on Gemini API | `HIGH` 开启，`MINIMAL` 关闭 |
| DeepSeek 官方 API | `thinking.type` 与其支持的 `reasoning_effort` |
| vLLM / GPUStack DeepSeek V4.1 | `chat_template_kwargs.thinking`；强度映射到 25 / 50 / 75 / 100 |
| GLM 5.3 | 始终思考，只调节其支持的强度 |
| Qwen / DashScope | `enable_thinking`；预算映射仅在管理员确认上游支持后选用 |
| vLLM / SGLang 模板 | `chat_template_kwargs.enable_thinking`，依部署模板配置 |
| Ollama | 兼容端点使用 `reasoning_effort`；关闭为 `none`，开关型模型开启使用兼容值，档位型模型传已确认值。原生 `/api/chat` 的 `think` 不发送到 `/v1/chat/completions` |
| Claude | 老版本显式预算，新版本 adaptive / effort；不同模型能否关闭不同 |
| 固定思考代理 | `reasoning-only`，不声明未经确认的关闭或强度能力 |

不以一次 HTTP 200 判定参数有效：代理可能接收并忽略未知字段。`/models` 通常也不会完整描述思考参数。当前不会定时发起付费探测；显式配置与官方能力表配合实际调用证据，推断结果标记为 `catalog`，管理员配置标记为 `configured`。滚动别名可先使用只请求摘要的约定，确认实际版本后再配置强度。

同时声明 OpenAI 与 `anthropic_native` 的代理渠道按请求选择线协议：OpenAI 对话及 Responses 使用 OpenAI 端点，Anthropic 入站保留原生 Messages。`anthropic_native` 不应将所有对话强制改为 Messages，否则会丢失 GLM 等模型的 OpenAI 思考参数。仅提供 Messages 的上游不能承接不兼容的思考约定；无法转换的原生思考参数会明确拒绝。

Claude 的签名思考块、Gemini 的原生会话状态以及 DeepSeek 工具循环所需的 `reasoning_content` 会随会话保留，不能从展示文本重新拼造签名。

## 返回、界面与审计

Chat 响应及流式帧的 `airoute_thinking` 包含最终生效模式、强度、能力来源、约定、实际模型、上游报告的思考 token、摘要是否存在与结束原因。请求日志保存同样的元数据，不保存摘要正文。未报告思考用量时省略计数，明确返回 0 时保留 0。

前端分别呈现上游返回的思考内容 / 摘要和思考 token。没有摘要不代表模型没思考；没有计数字段也不解释为 0。达到输出上限会提示思考可能占用答案预算，Claude 手动预算必须小于输出上限。能力查询不会按某次请求的默认输出上限隐藏已支持档位；演练提示所需最小上限，副驾驶自动为手动思考预算保留答案空间。

2026-11-03 的现有真实渠道验证发现：Gemini 未开启摘要请求时，思考用量与摘要并不会一起显示；GPUStack DeepSeek 的官方 `thinking.type=disabled` 被忽略，模板开关才生效；兼容代理上的 DeepSeek 忽略了测试的关闭参数，因此不能加入要求关闭思考的候选。GLM 5.3、Gemma 和 Gemini 的配置分别按实际能力处理。未配置的 OpenAI、Claude、Ollama 渠道的实现按官方约定及转换回归测试验证，没有宣称在本环境完成真实调用。

2026-11-05 通过罗总真实上游复核：发现并修复多协议渠道强制使用 Messages、丢弃 GLM 强度的问题。修复后确认实际 OpenAI 请求分别携带 `low / high / max`，三档均成功返回；流式 `high` 也携带原值并返回结果。统一控制的 `medium` 在分发前拒绝，原生透传的 `medium` 由真实上游拒绝。仅支持默认思考的代理模型按模型默认调用，未开放未经确认的强度。GPUStack 停机模型未参与此次调用验证。

官方依据：[Google ThinkingConfig](https://ai.google.dev/api/generate-content#thinkingconfig)、[Gemma API](https://ai.google.dev/gemma/docs/core/gemma_on_gemini_api)、[DeepSeek](https://api-docs.deepseek.com/guides/thinking_mode/)、[vLLM DeepSeek V4.1](https://recipes.vllm.ai/deepseek-ai/DeepSeek-V4.1-Flash)、[GLM](https://www.alibabacloud.com/help/en/model-studio/glm)、[OpenAI](https://developers.openai.com/api/docs/guides/reasoning)、[Claude](https://platform.claude.com/docs/en/build-with-claude/extended-thinking)、[Qwen](https://help.aliyun.com/zh/model-studio/deep-thinking)、[Ollama](https://docs.ollama.com/capabilities/thinking)。
