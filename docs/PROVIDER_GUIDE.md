# Downstream Provider Integration Guide

Airoute supports native multi-protocol downstream engines with zero special-casing and automatic capability probing.

---

## 1. Google Gemini (Official Developer API)

- **Provider Type**: `gemini`
- **Base URL**: `https://generativelanguage.googleapis.com`
- **API Key**: Your Google AI Studio API key (`AIzaSy...`)
- **Key Behavior**:
  - Automatically translates OpenAI canonical format to Google's specialized `/v1beta/models/{model}:generateContent` and `:streamGenerateContent`.
  - Supports system instructions, temperature, topP, and tools.
- **Recommended Models**: `gemini-4-argon`, `gemini-3.8-flash`, `gemini-3.5-flash-lite`, `gemini-2.0-flash`

---

## 2. Anthropic Claude (Direct Native API)

- **Provider Type**: `anthropic`
- **Base URL**: `https://api.anthropic.com`
- **API Key**: `sk-ant-...`
- **Key Behavior**:
  - Transforms OpenAI Chat Completions requests into native Anthropic Claude Messages payloads.
  - Transforms Anthropic SSE streams back to standard OpenAI chunk deltas or vice versa.
- **Recommended Models**: `claude-opus-5.5`, `claude-sonnet-5.5`, `claude-haiku-5.5`, `claude-3-7-sonnet`

---

## 3. GPUStack (Private Inference Cluster)

- **Provider Type**: `gpustack`
- **Base URL**: `http://<gpustack-host>:<port>/v1-openai`
- **API Key**: `none` or GPUStack user token
- **Auto-Detection**:
  - Clicking **"🔍 智能探测"** in the Web UI immediately extracts all deployed models (e.g. `meta-llama/Llama-3.1-8B-Instruct`, `flux-schnell`, `deepseek-r1`) and binds them to the provider automatically!
- **Multimodal Support**: Native support for LLM Chat, Stable Diffusion / Flux image generation, and audio models deployed in GPUStack.

---

## 4. Sub2API (Aggregator Gateway)

- **Provider Type**: `sub2api`
- **Base URL**: `https://<your-sub2api-domain>/v1`
- **API Key**: Your Sub2API token
- **Protocols Supported**: Chat, Text, Claude, Images, TTS, Whisper STT, Video.
- **Cascading Feature**: Use cascading wildcards like `sub2api/*:*` to pass model requests directly.

---

## 5. vLLM / SGLang / Ollama / Local Engines

- **vLLM / SGLang**: Base URL usually `http://localhost:8000/v1`. Supports ultra-fast continuous batching and SSE streams.
- **Ollama**: Base URL usually `http://localhost:11434/v1`. Auto-Probe detects local model tags from `/api/tags`.
- **Pure Text Downstreams**: If the local engine only serves `/v1/completions`, Airoute transparently translates all incoming Chat requests into completion prompts, converting the response into standard Chat choices and streaming chunks.

---

## 6. Anthropic 入站与 `anthropic_native` 标记

入站协议与上游线协议是两个独立概念：

- `anthropic_messages`：该渠道**接受** `/v1/messages` 入站流量。网关会按渠道自身的
  Provider Type 把请求翻译成 OpenAI/Gemini 线协议，再把响应翻译回 Anthropic 格式。
  普通文本与工具调用都可以这样跨协议转译。
- `anthropic_native`：显式声明该渠道的**上游**本身就说 Anthropic Messages 线协议
  （典型场景：OpenAI 兼容聚合网关背后的 Claude 端点）。只有渠道类型为 `anthropic`
  或显式带有 `anthropic_native` 标记时，网关才会把 Anthropic JSON 原样发往上游
  `<base>/v1/messages`。

请求若携带无法跨协议保真的语义（`thinking`、`top_k`、`metadata`、`cache_control`、
签名的 thinking 块等），只会路由到原生 Anthropic 渠道；若唯一可用渠道没有该标记，
请求会因为没有可用渠道而失败。因此：

1. 官方 Anthropic：Provider Type 选 `anthropic`，无需额外标记。
2. OpenAI 兼容聚合网关但原生支持 Claude：在渠道 `protocols` 中加入
   `anthropic_native`（同时保留 `anthropic_messages` 作为入站能力）。
3. 仅支持 OpenAI 协议的上游：不要添加 `anthropic_native`，让网关转译。

可通过管理 API 增补标记（示例，渠道 4）：

```bash
curl -X PUT http://127.0.0.1:8080/api/v1/admin/channels/4 \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"...","type":"openai","base_url":"https://.../v1","api_key":"...",
       "models":["..."],"protocols":["openai_chat","openai_response","anthropic_messages","anthropic_native"],
       "priority":1,"weight":10,"timeout_seconds":300,"status":"active"}'
```

> 控制台的协议勾选目前不展示该标记，编辑渠道时请保留 `protocols` 中的
> `anthropic_native`（后续前端批次会补充专用开关）。
