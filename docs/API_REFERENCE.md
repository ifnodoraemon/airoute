# Airoute API Reference Specification

Airoute provides high-performance, unified API endpoints conforming to OpenAI standard specifications, Anthropic Claude native specifications, and multimodal generation interfaces.

---

## 1. Authentication & Rate Limiting

Client requests to Data Plane endpoints under `/v1` require API key authentication via standard HTTP headers:
- `Authorization: Bearer sk-nano-xxxx`
- Or `x-api-key: sk-nano-xxxx`

If no API keys are configured, the gateway operates in open bypass mode (no authentication required).

---

## 2. Text & Chat Endpoints

### 2.1 Chat Completions
- **Endpoint**: `POST /v1/chat/completions`
- **Supported Providers**: All (OpenAI, DeepSeek, vLLM, SGLang, GPUStack, Sub2API, Anthropic Claude, Google Gemini).
- **Protocol Adaptation**: If downstream only supports text completion (`/v1/completions`), Airoute automatically formats messages into a prompt and translates the response back.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [
      {"role": "system", "content": "You are a helpful coding assistant."},
      {"role": "user", "content": "Write a quicksort in Go."}
    ],
    "stream": true,
    "temperature": 0.7
  }'
```

### 2.2 Legacy Text Completions
- **Endpoint**: `POST /v1/completions`
- **Supported Providers**: All. If downstream only supports chat completions, the request is automatically wrapped into a user chat message.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/completions \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-davinci-003",
    "prompt": "Translate the following English text to French: Hello world",
    "max_tokens": 128
  }'
```

### 2.3 Anthropic Claude Messages API
- **Endpoint**: `POST /v1/messages`
- **Supported Clients**: Native Anthropic Python/TypeScript SDK, Claude Code, Cursor, Cline.
- **Full-Duplex Translation**: Clients using Anthropic format can query upstream OpenAI or GPUStack clusters; request and SSE stream responses are bidirectionally converted with zero latency.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "x-api-key: sk-nano-xxxx" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-5.5",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Explain quantum computing briefly."}
    ]
  }'
```

### 2.4 Anthropic Claude Token Counting
- **Endpoint**: `POST /v1/messages/count_tokens`
- **Supported Clients**: Anthropic SDKs (e.g. `client.messages.count_tokens(...)`), Agent token budget estimators.
- **Protocol Adaptation**: Supports counting tokens for upstream Claude engines and estimates tokens for translated OpenAI/GPUStack upstreams.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/messages/count_tokens \
  -H "x-api-key: sk-nano-xxxx" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-5.5",
    "messages": [
      {"role": "user", "content": "Estimate token usage for this payload."}
    ]
  }'
```

#### Response Example:
```json
{
  "input_tokens": 16
}
```

### 2.5 Google Gemini Native API
- **Endpoints**:
  - `POST /v1beta/models/:model:generateContent`
  - `POST /v1beta/models/:model:streamGenerateContent`
  - `POST /v1beta/models/:model:countTokens`
  - `GET /v1beta/models`
  - `GET /v1beta/models/:model`
- **Supported Clients**: Google GenAI SDK (`@google/genai`, `google-generativeai`), Gemini CLI, Vertex AI compatible tooling.
- **Protocol Adaptation**: Full-duplex protocol conversion translating Gemini SDK calls into upstream format (Gemini upstream zero-copy, or adapted to OpenAI/DeepSeek/Claude models).

#### Request Example (countTokens):
```bash
curl -X POST http://localhost:8080/v1beta/models/gemini-4-argon:countTokens \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{"role": "user", "parts": [{"text": "Hello world!"}]}]
  }'
```

#### Response Example:
```json
{
  "totalTokens": 3
}
```

### 2.6 DeepSeek-R1 / Reasoning Models Transparency
- **Transparent Field**: `reasoning_content` in Chat Completions chunk and message objects.
- **Stream Transparency**: Streaming chunks carrying reasoning tokens (such as DeepSeek-R1 thinking traces or OpenAI o1/o3 reasoning chunks) are forwarded transparently with zero-copy overhead without stripping thinking blocks.

---

## 3. Multimodal & Information Retrieval Endpoints

### 3.1 AI Image Generation
- **Endpoint**: `POST /v1/images/generations`
- **Models**: `dall-e-3`, `flux-schnell`, `stable-diffusion-3`, etc.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/images/generations \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "flux-1.1-pro",
    "prompt": "A modern datacenter server room with green neon lighting, 8k digital art",
    "size": "1024x1024",
    "quality": "standard",
    "n": 1
  }'
```

### 3.2 Text-to-Speech (TTS)
- **Endpoint**: `POST /v1/audio/speech`
- **Response**: Binary audio stream (`audio/mpeg`), streamed directly to client without buffering in memory.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/audio/speech \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tts-1",
    "input": "欢迎体验 Airoute 极致性能企业级多模态网关系统。",
    "voice": "alloy",
    "response_format": "mp3"
  }' --output output.mp3
```

### 3.3 Audio Transcription (Whisper STT)
- **Endpoint**: `POST /v1/audio/transcriptions`
- **Content-Type**: `multipart/form-data`

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/audio/transcriptions \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -F "file=@/path/to/audio.mp3" \
  -F "model=whisper-large-v3-turbo"
```

### 3.4 Audio Translation
- **Endpoint**: `POST /v1/audio/translations`
- **Content-Type**: `multipart/form-data`
- **Description**: Translates foreign audio recordings directly into English text.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/audio/translations \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -F "file=@/path/to/french_speech.mp3" \
  -F "model=whisper-large-v3-turbo"
```

### 3.5 Video Generation & Task Polling
- **Task Submission**: `POST /v1/videos/generations`
- **Task Polling**: `GET /v1/videos/tasks/:id`

#### Request Example:
```bash
# 1. Submit task
curl -X POST http://localhost:8080/v1/videos/generations \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cogvideox-5b",
    "prompt": "A drone shot flying over a futuristic city at sunset",
    "aspect_ratio": "16:9"
  }'

# Returns: {"task_id": "video_task_123", "status": "PENDING"}

# 2. Poll task status
curl -X GET http://localhost:8080/v1/videos/tasks/video_task_123 \
  -H "Authorization: Bearer sk-nano-xxxx"
```

### 3.6 Vector Embeddings
- **Endpoint**: `POST /v1/embeddings`
- **Supported Providers**: OpenAI, GPUStack, vLLM, Ollama, SGLang, Sub2API, Google Gemini.
- **Protocol Adaptation**: Full native translation for Google Gemini (`:embedContent` and `:batchEmbedContents`), model rewriting, token tracking, and automatic safe fallback across candidate embedding nodes. Supports single string or batch string inputs.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-3-small",
    "input": ["Deep learning infrastructure", "Next-generation agentic AI gateway"]
  }'
```

#### Response Example:
```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.0023, -0.015, 0.045, 0.088]
    },
    {
      "object": "embedding",
      "index": 1,
      "embedding": [-0.034, 0.082, 0.011, -0.005]
    }
  ],
  "model": "text-embedding-3-small",
  "usage": {
    "prompt_tokens": 12,
    "total_tokens": 12
  }
}
```

### 3.7 Rerank (Cross-Encoder Document Re-ranking)
- **Endpoint**: `POST /v1/rerank`
- **Supported Engines**: GPUStack, Hugging Face Text Embeddings Inference (TEI), Infinity, Xinference, Cohere, Jina.
- **HA Safe Fallback**: Protected by gateway circuit breaker. If the primary reranker encounters OOM, 503, or network timeout, the gateway automatically retries the backup reranker before sending headers.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/rerank \
  -H "Authorization: Bearer sk-nano-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bge-reranker-large",
    "query": "What is high availability in enterprise LLM gateways?",
    "documents": [
      "Airoute provides zero-copy streaming and pre-first-token fallback.",
      "The weather today in Tokyo is sunny with blooming cherry blossoms.",
      "Distributed databases using Raft consensus ensure strong consistency."
    ],
    "top_n": 2,
    "return_documents": true
  }'
```

#### Response Example:
```json
{
  "id": "rerank-c7e148a0",
  "results": [
    {
      "index": 0,
      "relevance_score": 0.9856,
      "document": {
        "text": "Airoute provides zero-copy streaming and pre-first-token fallback."
      }
    },
    {
      "index": 2,
      "relevance_score": 0.3210,
      "document": {
        "text": "Distributed databases using Raft consensus ensure strong consistency."
      }
    }
  ],
  "usage": {
    "prompt_tokens": 86,
    "total_tokens": 86
  }
}
```

---

## 4. Control Plane Admin APIs

All Admin APIs are under `/api/v1/admin`:

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/admin/channels` | List all configured providers with live circuit breaker statuses |
| `POST` | `/api/v1/admin/channels` | Create a new provider and immediately hot-reload in memory |
| `PUT` | `/api/v1/admin/channels/:id` | Update an existing provider configuration and hot-reload in memory |
| `POST` | `/api/v1/admin/channels/probe` | **Deep Auto-Probe**: Automatically normalize URL, probe live models, fingerprint upstream engine (GPUStack, vLLM, SGLang, Ollama, DeepSeek, Gemini, Anthropic, Sub2API), infer protocols (`chat`, `completion`, `messages`, `embeddings`, `rerank`, etc.) |
| `POST` | `/api/v1/admin/channels/:id/test` | Ping downstream provider for latency & response verification |
| `DELETE`| `/api/v1/admin/channels/:id` | Delete provider and remove from routing |
| `GET` | `/api/v1/admin/keys` | List all client API keys with rate limits & budgets |
| `POST` | `/api/v1/admin/keys` | Create client API key |
| `DELETE`| `/api/v1/admin/keys/:id` | Revoke client API key |
| `GET` | `/api/v1/admin/logs?limit=50` | Query real-time audit logs with TTFT, tokens, latency, status code |
| `GET` | `/api/v1/admin/stats/overview` | Query aggregated gateway statistics (QPS, TTFT, Tokens) |
| `GET` | `/api/v1/admin/models` | List all active models across providers |

---

## 5. Observability Endpoints

- **Health Check**: `GET /health` (`{"status": "healthy"}`)
- **Prometheus Metrics**: `GET /metrics` (Standard Prometheus exposition format)
- **Web App**: `GET /app/` or `GET /` (Embedded React dashboard)
