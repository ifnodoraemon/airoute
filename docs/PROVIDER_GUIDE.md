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
