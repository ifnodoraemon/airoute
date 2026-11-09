# Airoute API Reference Specification

Airoute provides high-performance, unified API endpoints conforming to OpenAI standard specifications, Anthropic Claude native specifications, and multimodal generation interfaces.

---

## 1. Authentication & Rate Limiting

Client requests to Data Plane endpoints under `/v1` require API key authentication via standard HTTP headers:
- `Authorization: Bearer sk-airoute-xxxx`
- Or `x-api-key: sk-airoute-xxxx`

If no API keys are configured, the gateway operates in open bypass mode (no authentication required).

---

## 2. Text & Chat Endpoints

### 2.1 Chat Completions
- **Endpoint**: `POST /v1/chat/completions`
- **Supported Providers**: All (OpenAI, DeepSeek, vLLM, SGLang, GPUStack, Sub2API, Anthropic Claude, Google Gemini).

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-airoute-xxxx" \
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

### 2.2 Anthropic Claude Messages API
- **Endpoint**: `POST /v1/messages`
- **Supported Clients**: Native Anthropic Python/TypeScript SDK, Claude Code, Cursor, Cline.
- **Full-Duplex Translation**: Clients using Anthropic format can query upstream OpenAI or GPUStack clusters; request and SSE stream responses are bidirectionally converted with zero latency.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "x-api-key: sk-airoute-xxxx" \
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

### 2.3 Anthropic Claude Token Counting
- **Endpoint**: `POST /v1/messages/count_tokens`
- **Supported Clients**: Anthropic SDKs (e.g. `client.messages.count_tokens(...)`), Agent token budget estimators.
- **Accuracy**: Delegates counting to a native Anthropic upstream, including tools and image/document blocks. Returns `501` when no native Anthropic channel is configured; it does not claim a character-based estimate is an exact token count.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/messages/count_tokens \
  -H "x-api-key: sk-airoute-xxxx" \
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

### 2.4 Google Gemini Native API
- **Endpoints**:
  - `POST /v1beta/models/:model:generateContent`
  - `POST /v1beta/models/:model:streamGenerateContent`
  - `POST /v1beta/models/:model:countTokens`
  - `POST /v1beta/models/:model:embedContent` / `:batchEmbedContents`
  - `POST /v1beta/models/:model:batchGenerateContent` / `:asyncBatchEmbedContent`
  - Files / cachedContents / batches resource routes under `/v1beta`
  - Upload: `/upload/v1beta/files` (including resumable sessions); download: `/v1beta/files/:id:download?alt=media`
  - Interactions: `POST /v1beta/interactions`, `GET/DELETE /v1beta/interactions/:id`, `POST /v1beta/interactions/:id/cancel`
  - Live WebSocket: `/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent` (also `v1alpha`)
  - `GET /v1beta/models`
  - `GET /v1beta/models/:model`
- **Supported Clients**: Current Google GenAI SDKs (`@google/genai`, `google-genai`, `google.golang.org/genai`), Gemini CLI. Configure the SDK Base URL to the gateway and use an Airoute key; Vertex upstream project/authentication is configured on the channel.

Resource management and resumption require authenticated callers. Lists contain only resources created or observed through this gateway by the same owner. IDs, cached content, uploaded files, interaction continuations and Live resumption handles are pinned to the original upstream credential; rotating that credential requires creating new resources. File uploads without an explicit model select an allowed Google channel; use `?model=<alias>` when multiple Google accounts are configured. Set `PUBLIC_URL` to the external HTTPS origin for resumable upload URLs.

Batch and background Interactions are metered by a PostgreSQL-backed worker even if the client stops polling. Result JSONL files are read for upstream usage; missing usage is explicitly marked as estimated. Stored job state and deterministic billing IDs prevent duplicate settlement across replicas. Batch discounts and cache storage fees are not automatically imported from Google: the gateway uses its configured prices.

The model catalog uses per-upstream metadata when available and otherwise conservative model/protocol declarations. Native forwarding retains vendor fields; incompatible cross-protocol features return an error. The console **Gemini Native** tab accepts inline image/audio/video/document inputs, displays native media, and preserves complete signed parts when appending history.

#### Request Example (countTokens):
```bash
curl -X POST http://localhost:8080/v1beta/models/gemini-4-argon:countTokens \
  -H "Authorization: Bearer sk-airoute-xxxx" \
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
- **Stream handling**: OpenAI-compatible reasoning deltas are normalized in memory. Native Anthropic streams preserve signed thinking blocks; cross-protocol reasoning behavior depends on the target API and is not byte-for-byte passthrough.

### 2.7 OpenAI Responses API
- **Endpoint**: `POST /v1/responses`
- **Supported Clients**: OpenAI Responses SDK and agents that keep a conversation server-side via `previous_response_id`.
- **Conversion**: the request is translated into the canonical chat request (input items → messages, `instructions` → system message, `max_output_tokens`, tools/tool_choice) and candidates are resolved with the `responses` routing protocol, so channels that speak the response protocol are preferred.
- **Conversation replay**: a request carrying `previous_response_id` replays the stored conversation of that response. Context is kept for a bounded retention window and is scoped to the calling API key/user; an id that is unknown, expired or owned by another caller returns `400 previous_response_not_found` instead of silently answering without history.
- **Non-streaming response**: an object with `id` (`resp_…`), `object: "response"`, `created_at`, `status: "completed"`, `model`, `output` (message items with `output_text` parts, or `function_call` items) and `usage`.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Authorization: Bearer sk-airoute-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "input": "Explain prompt caching in one paragraph.",
    "instructions": "You are a concise assistant."
  }'
```

#### Streaming (`"stream": true`)
The response is `text/event-stream` and emits the Responses event sequence: `response.created`, `response.in_progress`, `response.output_item.added`, `response.content_part.added`, `response.output_text.delta`, `response.content_part.done`, `response.output_item.done`, and finally `response.completed`. Each event carries the response id so a client can resume the conversation with `previous_response_id`.

> The `model` field in the answer always echoes the model the client requested. A provider that reports its own upstream name (or omits the field) is normalized by the gateway.

### 2.8 OpenAI Moderations
- **Endpoint**: `POST /v1/moderations`
- **Body**: `{"input": "text" | ["text", ...], "model": "text-moderation-latest"}`. The model defaults to `text-moderation-latest`.
- **Behaviour**: proxied to a channel that serves the `moderation` protocol. The channel's own verdict is returned unchanged and the call is audited as one fixed-price unit. When no moderation channel is configured the endpoint answers `501 moderation_not_configured` — it never fabricates an "everything is safe" verdict unless the explicit development switch `GATEWAY_MODERATION_STUB` is enabled.
- **Pricing**: the model must have a positive `fixed_price` and a CNY rate row, like every other billable ingress.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/moderations \
  -H "Authorization: Bearer sk-airoute-xxxx" \
  -H "Content-Type: application/json" \
  -d '{"input": "Some user-generated text"}'
```

### 2.9 Models Registry
- **Endpoints**: `GET /v1/models` (list) and `GET /v1/models/{model}` (detail)
- **Response**: OpenAI-shaped model objects (`id`, `object: "model"`, `created`, `owned_by`) for every model the gateway can route, so SDKs that discover models work unchanged.

---

## 3. Multimodal & Information Retrieval Endpoints

### 3.1 AI Image Generation
- **Endpoint**: `POST /v1/images/generations`
- **Models**: `dall-e-3`, `flux-schnell`, `stable-diffusion-3`, etc.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/images/generations \
  -H "Authorization: Bearer sk-airoute-xxxx" \
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
  -H "Authorization: Bearer sk-airoute-xxxx" \
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
  -H "Authorization: Bearer sk-airoute-xxxx" \
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
  -H "Authorization: Bearer sk-airoute-xxxx" \
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
  -H "Authorization: Bearer sk-airoute-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cogvideox-5b",
    "prompt": "A drone shot flying over a futuristic city at sunset",
    "aspect_ratio": "16:9"
  }'

# Returns: {"task_id": "video_task_123", "status": "PENDING"}

# 2. Poll task status
curl -X GET http://localhost:8080/v1/videos/tasks/video_task_123 \
  -H "Authorization: Bearer sk-airoute-xxxx"
```

### 3.6 Vector Embeddings
- **Endpoint**: `POST /v1/embeddings`
- **Supported Providers**: OpenAI, GPUStack, vLLM, Ollama, SGLang, Sub2API, Google Gemini.

#### Request Example:
```bash
curl -X POST http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer sk-airoute-xxxx" \
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
  -H "Authorization: Bearer sk-airoute-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bge-reranker-large",
    "query": "What is high availability in enterprise LLM gateways?",
    "documents": [
      "Airoute streams responses and can retry before the first content chunk.",
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
        "text": "Airoute streams responses and can retry before the first content chunk."
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
| `POST` | `/api/v1/admin/channels/thinking/probe` | 探测实际模型的关闭能力及思考档位，返回逐项确认状态；不自动保存渠道 |
| `POST` | `/api/v1/admin/channels/:id/test` | Ping downstream provider for latency & response verification |
| `DELETE`| `/api/v1/admin/channels/:id` | Delete provider and remove from routing |
| `GET` | `/api/v1/admin/keys` | List all client API keys with rate limits & spend caps |
| `POST` | `/api/v1/admin/keys` | Create client API key; `group_name` must select an existing pricing group |
| `PUT` | `/api/v1/admin/keys/:id` | Update client API key; an explicit `group_name` must select an existing pricing group |
| `DELETE`| `/api/v1/admin/keys/:id` | Revoke client API key |
| `GET` | `/api/v1/admin/logs?limit=50` | Query real-time audit logs with TTFT, tokens, latency, status code |
| `GET` | `/api/v1/admin/logs?limit=50&count=0&facets=0` | Rows only: skips the filtered COUNT and facet DISTINCT (dashboard poll / CSV export) |
| `GET` | `/api/v1/admin/stats/overview` | Query aggregated gateway statistics (QPS, TTFT, Tokens) |
| `GET` | `/api/v1/admin/models` | List all active models across providers |
| `GET` | `/api/v1/admin/supplier-costs` | List supplier unit prices keyed by channel name + model (reference only; not used for charging) |
| `POST` | `/api/v1/admin/supplier-costs` | Create or overwrite the supplier price for a channel + model: `channel`, `model`, `input_cost`, `output_cost` (CNY per 1M tokens), `status` (`active`/`disabled`) |
| `DELETE`| `/api/v1/admin/supplier-costs/:id` | Delete one supplier price row |
| `GET` | `/api/v1/admin/supplier-costs/export` | Export supplier prices as CSV |
| `POST` | `/api/v1/admin/supplier-costs/import` | Import supplier prices from CSV; `?dry_run=1` validates every row and reports errors without writing |

创建或调整 Key 的 `group_name` 仅可选择定价管理中已配置倍率或模型费率的分组，留空使用默认组（普通用户创建时沿用账号分组）。未知分组返回 `400`，不会随 Key 写入而创建分组；普通用户仍只能选择默认组或其账号已获授权的分组。

付费请求按完整预估金额占用钱包余额和 Key 预算，补差不足时在请求上游之前返回 `402`；预留存储不可用时返回可重试的 `503 billing_unavailable`。缺省输出上限为 4096 token，可用对应协议的输出上限字段显式调整（MCP 使用 `max_tokens`）。请求侧结算等待超时不会归还待扣额度；最终扣款和该请求预留的释放在同一事务提交。异步任务的预留随任务持久保存，已付费结果读取与任务取消不要求再次预留。

实际用量异常超出预估时完整记入消费和钱包欠款，后续付费请求会因余额或预算不足而拒绝。文件输入的原生批任务返回 `422`，需改为内联输入；付费原生 Agent 调用需配置纯固定费用。预授权和集群升级要求见 [配置说明](CONFIGURATION.md#rate-limiting-and-sessions)。

### Supplier unit prices vs. our sell price

Supplier unit prices are maintained **separately** from our sell price and never derived from each other:

- **Sell price** — the price we charge, resolved by the existing pricing pipeline (group → default → wildcard → `*`, with cache/off-peak/group multipliers).
- **Supplier price** — what a channel/model costs us, keyed by `channel` (name) + `model`: input/output price per 1M tokens plus an `active`/`disabled` status. On the pricing page the supplier list is paired per model with our sell price (default group tier), with the spread (ours − supplier) shown next to it.
- A missing supplier price is reported as “not configured”, never as `0`. Supplier prices do **not** affect billing, are not written to usage records, and are **not** used to compute margin or cost analytics.

> Organization/cost-center, budget/alert, wallet-ledger and cost/margin endpoints are out of product scope and are not provided.

### API key representations

A client key has two representations and they are not interchangeable:

- `key` — the bearer credential (`sk-airoute-…`, 43 characters). Returned **only** by the creation response, which is marked `key_returned_once: true`. The gateway stores a SHA-256 digest and cannot show the credential again.
- `key_id` — the public identifier (`ak_` followed by 20 hex characters). Present on every list and creation response; it names the key in logs, exports and support tickets and **cannot authenticate a call**.

List responses contain neither the credential nor its digest.

Note the two different uses of the name: the `key_id` **field** in a key object is the public `ak_…` identifier, while the `key_id` **query parameter** accepted by `/api/v1/admin/logs` and `/api/v1/admin/stats/overview` is the numeric row `id` used for drill-down.

---

### Announcements and private notifications

The console's **Announcements & messages** page (`#/notifications`) separates a platform announcement timeline from a personal inbox. Administrators also have a drafts tab. All signed-in accounts can read published posts; each private message is visible only to its recipient, including when the caller is an administrator.

| Method | Endpoint | Access / behavior |
| :--- | :--- | :--- |
| `GET` | `/api/v1/announcements/banner` | Public: latest published post with banner enabled, or `data: null` |
| `GET` | `/api/v1/admin/announcements` | Administrator: paginated posts, optionally `status=draft\|published` |
| `GET` | `/api/v1/admin/announcements/{id}` | Administrator: one editable announcement record |
| `POST` | `/api/v1/admin/announcements` | Administrator: create an independent draft or published post |
| `PUT` | `/api/v1/admin/announcements/{id}` | Administrator: edit, publish, or withdraw a post to draft |
| `DELETE` | `/api/v1/admin/announcements/{id}` | Administrator: delete the post, delivery and its read markers |
| `GET` | `/api/v1/user/notifications` | Session: announcement timeline and own private messages |
| `GET` | `/api/v1/user/notifications/unread-count` | Session: total, announcement and personal unread counts |
| `POST` | `/api/v1/user/notifications/{id}/read` | Session: idempotently mark a visible item read; foreign private messages return 404 |
| `POST` | `/api/v1/user/notifications/read-all` | Session: mark one category read through a received snapshot |
| `POST` | `/api/v1/admin/notifications` | Administrator: deliver a private message to one account |

Announcement writes use `title`, `content`, `level` (`info`, `warning`, `critical`), `status` (`draft`, `published`) and `show_banner`. Creates also require a unique `request_id`; a retry with identical content returns the same record, while conflicting content returns 409. Banner display is optional and defaults to false. Drafts are administrator-only. Editing published content replaces its delivery and marks the corrected post unread. Changing only banner display preserves read state. Withdrawal removes the active delivery and banner; deletion removes the record and read state. Other announcement records remain intact.

Migration 53 creates the announcement, delivery and read tables and deletes the obsolete `system_announcement` setting. The singleton JSON configuration and its singular API routes have been removed; there is no import, fallback or alias.

Notification lists accept `kind=announcement|message|all`, `unread=true|false`, `page` (default 1) and `page_size` (default 20, maximum 100). The `data` object contains `items`, `total`, `page`, `page_size`, `latest_sequence`, `unread_count`, `announcement_unread_count` and `message_unread_count`. Each item contains `id`, `kind`, `title`, `content`, `level`, `created_at` and nullable `read_at`; announcement deliveries also expose `announcement_id`. Read state is stored per durable account ID across devices.

For **Mark this tab as read**, send `{"kind":"message","through_sequence":18}` using `latest_sequence` from the displayed response. Later deliveries, including drafts published after that snapshot, stay unread. Announcement and personal read state remain separate.

Private message writes accept `request_id`, `recipient_username`, `title`, `content` and `level`. Request IDs contain 1–128 letters, digits, underscores or hyphens, starting with a letter or digit. Repeating the same ID and payload returns the same delivery; changing its payload returns 409. Titles are limited to 120 characters and content to 10000; blank published text and invalid priority are rejected. Text paragraphs are preserved without executing HTML.

Run `bash scripts/test-notifications-postgres.sh` to verify the schema, account isolation, post lifecycle, read snapshots and concurrent publishers against disposable PostgreSQL.

## 5. Observability Endpoints

- **Health Check**: `GET /health` (`{"status": "healthy"}`)
- **Readiness**: `GET /health/ready` — `200` only when the data-plane snapshot is loaded and every registered dependency probe (storage, Redis, ...) succeeds; otherwise `503` with a per-dependency breakdown. Rolling restarts gate on this endpoint, not on `/health`.
- **Public Status**: `GET /api/v1/public/status` — the unauthenticated status-page payload (service state, model/modality availability and latency). It carries no credentials and no tenant data.
- **Prometheus Metrics**: `GET /metrics` (Standard Prometheus exposition format). Reachable from loopback/private clients by default; set `GATEWAY_METRICS_TOKEN` to require `Authorization: Bearer <token>` or `GATEWAY_METRICS_ALLOW_PUBLIC=true` to open it.
- **Web App**: `GET /app/` or `GET /` (Embedded React dashboard)

---

## 6. Model Context Protocol (MCP)

The gateway is a full MCP server, so an agent (Cursor, Claude Desktop, Cline, the bundled `airoute mcp stdio` adapter) can use the gateway's skills and remote MCP servers as tools.

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/mcp` | Discovery document: server identity, protocol versions, SSE/messages endpoints and copyable client configs (no credentials) |
| `GET` | `/mcp/sse` | Opens the SSE channel; authentication is required before the stream is switched to `text/event-stream` |
| `POST` | `/mcp/messages` | JSON-RPC 2.0 over HTTP (streamable). A request without an `id` is a notification and is answered `202` with an empty body |
| `GET`/`POST` | `/v1/mcp`, `/v1/mcp/sse`, `/v1/mcp/messages` | The same endpoints under the `/v1` prefix, for clients configured with a `/v1` base URL |

- **Protocol versions**: `2026-07-28` (default), `2025-11-25` and `2024-11-05` are supported; the server negotiates the client's `protocolVersion` and answers with the newest revision it implements when the requested one is unknown. The negotiated value is returned in the `MCP-Protocol-Version` header (and exposed via CORS) on every response.
- **Authentication**: the caller presents an API key as `Authorization: Bearer …`, `X-API-Key`, `?apiKey=`, or (legacy) `?token=` / `?key=`. The same data-plane admission rules apply as `/v1/*`: key status, owner account state, key budget and wallet balance. Anonymous access is allowed only before the first key is ever created.
- **Authorization**: `airoute_cluster_status`, `airoute_model_topology` and the SQL reflection tools (`read_query`, `list_tables`, `describe_table`) require a key owned by an administrator. Out-of-band callers (stdio adapter, tool arguments) may present `api_key` as a tool argument.
- **Tool discovery**: `tools/list` is the authoritative catalog. Tools belonging to a disabled skill are hidden; remote MCP plaza servers contribute their own tools once enabled.

---

## 7. Deprecated / Removed Endpoints

These were registered historically but had no consumer and are intentionally gone. Point clients at the replacement:

| Removed | Replacement |
| :--- | :--- |
| `GET /api/v1/user/me` | `GET /api/v1/auth/me` |
| `POST /api/v1/user/keys` | `POST /api/v1/admin/keys` (owner and self-service quota checks apply to regular callers) |
| `DELETE /api/v1/user/keys/{id}` | `DELETE /api/v1/admin/keys/{id}` (same owner check for regular callers) |
| `GET /api/v1/user/wallet/orders` (list) | `GET /api/v1/user/wallet/orders/{order_no}` (single order) |
| `GET /api/v1/admin/skills/{id}/download` | `GET /api/v1/skills/{id}/download` (public bundle download) |

### 渠道模型思考能力探测

`POST /api/v1/admin/channels/thinking/probe` 接收 `base_url`、`type`、实际 `model`，以及可选的 `api_key`、`channel_id`、`protocols`、`thinking`（当前能力声明）。编辑已保存渠道时可省略密钥，服务端仅在目标地址与已保存地址一致时复用凭证。

返回 `{code:0,data:{model,capability,checks,thinking,message}}`：`checks` 按 `off` 和各档位返回 `status`（`supported / unsupported / unknown`）及 `reason`；`thinking` 只包含本次能确认的结果，全部待确认时省略。固定、无思考或未知预设同样按渠道协议进行实际探测，不因名称或旧配置跳过请求。命名档位协议试探全部六档及已声明的自定义档位；确认新能力需要变更参数约定时，`thinking.profile` 返回可保存的约定，未确认结果不会自动改写已有配置。探测使用有限数量的短请求，不返回上游凭证、模型正文或错误正文，不写数据库。控制台合并确认结果并保留待确认的已有配置，用户保存后生效。

渠道连通探测的 `data.thinking_capabilities` 同时返回每个实际模型的自动能力。优先采用上游 `capabilities.reasoning_effort` 明确声明的枚举，缺失时采用名称能力表。支持 `ultra` 及有效的自定义命名档位，不代表所有模型均支持内置全部档位。
