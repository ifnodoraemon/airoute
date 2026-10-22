# Airoute Configuration & Tuning Manual

This document details the configuration options, cascading model rewrite syntax, circuit breaker parameters, and performance tuning for Airoute.

---

## 1. Configuration File (`configs/config.yaml`)

```yaml
server:
  port: 8080
  read_timeout_sec: 120
  write_timeout_sec: 120
  redis_url: "${REDIS_URL}"

# Database is selected by command-line -db for SQLite or DATABASE_URL for PostgreSQL.
# HTTP transport and circuit-breaker limits are currently code defaults, not YAML keys.

# Initial Channels (Can also be managed dynamically via Web UI / SQLite)
channels:
  - name: "gpustack-primary"
    type: "gpustack"
    base_url: "http://192.168.1.100:80/v1-openai"
    api_key: "none"
    models:
      - "meta-llama/Llama-3.1-8B-Instruct"
      - "deepseek-r1-distill-qwen-14b"
      - "flux-schnell"
    model_mapping:
      "local/*": "*"
      "my-corp/llama3": "meta-llama/Llama-3.1-8B-Instruct"
    protocols:
      - "openai_chat"
      - "openai_text"
      - "images"
    priority: 1
    weight: 10
    timeout_seconds: 60

  - name: "google-gemini-official"
    type: "gemini"
    base_url: "https://generativelanguage.googleapis.com"
    api_key: "AIzaSy..."
    models:
      - "gemini-2.0-flash"
      - "gemini-1.5-pro"
    protocols:
      - "openai_chat"
      - "anthropic_messages"
    priority: 1
    weight: 10

  - name: "anthropic-claude-direct"
    type: "anthropic"
    base_url: "https://api.anthropic.com"
    api_key: "sk-ant-..."
    models:
      - "claude-3-5-sonnet-20241022"
    protocols:
      - "openai_chat"
      - "anthropic_messages"
    priority: 1
    weight: 10

# 客户端 API 访问密钥 (Client API Keys, 可通过 Web 控制台动态签发)
api_keys:
  - key: "sk-airoute-8f92a1c4b7e3"
    tenant_id: "engineering-dept"
    allowed_models: []          # Empty means all models permitted
    rpm: 120                    # Rate limit: 120 requests/minute
    tpm: 500000                 # Token limit: 500,000 tokens/minute
```

---

## 2. Environment Variables

The following environment variables are read by the server. YAML supports `${NAME}` and `${NAME:-default}` expansion.

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_PORT` | `8080` | HTTP listen port |
| `GATEWAY_CONFIG` | `configs/config.yaml` | Path to YAML configuration file |
| `DATABASE_URL` | unset | PostgreSQL connection URL; when unset, SQLite uses the `-db` path |
| `GATEWAY_ADMIN_PASSWORD` | `admin123` outside Helm | Initial admin password; set a strong value before exposing the console |
| `GATEWAY_ADMIN_SECRET` | random per process outside Helm | HMAC secret; use one stable shared value for multiple replicas |
| `REDIS_URL` | unset | Optional Redis Streams logging queue |
| `GATEWAY_FIRST_TOKEN_TIMEOUT` | 30 seconds (60 for reasoning model names) | Upper bound in seconds for the first content token, capped by channel timeout |
| `GIN_MODE` | `release` | Web engine mode (`release` or `debug`) |
| `GATEWAY_MAX_BODY_BYTES` | `67108864` (64 MiB) | Maximum accepted request body size |
| `GATEWAY_TRACE_CAPTURE` | `errors` | Request/response snapshot archiving: `off`, `errors` or `all` |
| `GATEWAY_TRACE_SAMPLE` | `100` | Percentage of **successful** requests captured when mode is `all` |
| `GATEWAY_TRACE_TTL_HOURS` | `72` | Retention window for archived snapshots |
| `GATEWAY_TRACE_MAX_SNIPPET` | `4096` | Maximum bytes retained per request/response snippet |
| `GATEWAY_USAGE_LOG_RETENTION_DAYS` | `365` | Retention window for audit (`usage_logs`) rows; `0` keeps every row |
| `GATEWAY_USAGE_LOG_PRUNE_INTERVAL_HOURS` | `6` | How often expired audit rows are pruned |
| `GATEWAY_METRICS_TOKEN` | unset | When set, `/metrics` requires `Authorization: Bearer <token>` |
| `GATEWAY_PUBLIC_STATUS` | `true` | Set `false` to hide the model catalog on the public status page |
| `GATEWAY_BLOCK_PRIVATE_PROBE` | `false` | Reject probing RFC1918 private upstream addresses |
| `GATEWAY_BLOCK_LOOPBACK_PROBE` | `false` | Reject probing loopback upstream addresses |
| `AIROUTE_DEV_VERIFICATION_CODE` | unset | Set `true` to return email verification codes in API responses (development only) |

Snapshots are redacted (API keys, tokens, passwords) before storage and are queryable at `GET /api/v1/admin/traces?trace_id=…` (admin only; regular users see only their own keys).

Helm defaults to one SQLite pod with a ReadWriteOnce volume and creates a Kubernetes Secret for the admin password and signing secret. For multiple replicas, provide `database.existingSecret` with key `url` containing a PostgreSQL URL and set `persistence.enabled=false`; configure shared object storage if artifacts must be accessible from every pod. Helm refuses a multi-replica SQLite deployment. Retrieve the generated admin password with `kubectl get secret <fullname>-auth -o jsonpath='{.data.admin-password}' | base64 -d` (for the default `airoute` release, `<fullname>` is `airoute`). Keep the Secret across upgrades.

---

## 3. Cascading Model Mapping Syntax

Airoute supports unconstrained cascading model aliasing and prefix stripping:

### Rule 1: Exact Model Alias
```yaml
model_mapping:
  "yy/xxx/xx": "xxx/xx"
```
Client queries `yy/xxx/xx`, upstream receives `xxx/xx`.

### Rule 2: Prefix Wildcard Stripping
```yaml
model_mapping:
  "org/dept/*": "*"
```
Client queries `org/dept/v1/deepseek-ai/DeepSeek-V3`, upstream receives `v1/deepseek-ai/DeepSeek-V3`.

### Rule 3: Provider-Prefix Targeted Routing
When multiple channels configure the same model (e.g. `gpt-4o` on both `sub2api` and `openai-official`), the client can prefix the model with the channel's name:
- Query `sub2api/gpt-4o`: automatically routes to channel `sub2api` and strips the prefix to `gpt-4o`.
- Query `openai-official/gpt-4o`: automatically routes to channel `openai-official`.

---

## 4. Stability & Circuit Breaker Architecture

Airoute implements a Tri-State Circuit Breaker state machine:

```
    [ CLOSED ] --- (Consecutive Failures >= 3) ---> [ OPEN ]
        ^                                               |
        |                                       (Cooldown 30s)
        |                                               v
    [ CLOSED ] <--- (2 Canary Probes Pass) <--- [ HALF-OPEN ]
        |                                               |
        +------------ (Any Failure) --------------------+
```

### Safe Fallback Window
When streaming requests fail:
- If an upstream returns HTTP `429`, `500`, or network error **before the first token is emitted to the client**, the gateway seamlessly retries with the next healthy backup provider.
- Failover is possible until a content token or tool call is emitted. Errors after that point are sent on the active stream.

### Multimodal billing
Images use the configured `fixed_price` per requested image (`n`, default 1). Audio and video creation use one configured fixed-price unit per successful request; video task polling is not billed. These are gateway prices, not live upstream invoices. A multimodal model with no positive fixed price is rejected with `pricing_not_configured`; set `GATEWAY_ALLOW_UNPRICED_MULTIMODAL=1` only when zero-cost forwarding is intentional.

---

## 5. Model & Key Protocol Validation Governance

Airoute includes an intelligent protocol validation engine for OpenAI, Anthropic Claude, and Google Gemini schemas.

### Default Behavior
- **Default validation is `off`**: Raw requests are passed directly to upstream providers with zero parsing overhead and maximum compatibility.
- **Granular Activation**: Validation can be enabled selectively per model or per API Key (`strict` or `lenient`).

### Configuration Syntax (`configs/config.yaml`)

```yaml
model_validation:
  default_level: "off"  # "off" | "lenient" | "strict"
  rules:
    # Strict validation on reasoning models (rejects temperature/top_p)
    - model: "o1*"
      protocol: "openai"
      level: "strict"
      disallow_temperature: true
    # Strict validation for Anthropic Claude (requires max_tokens > 0)
    - model: "claude-*"
      protocol: "anthropic"
      level: "strict"

api_keys:
  # Enable strict protocol validation for a specific client key
  - key: "sk-strict-auditor"
    tenant_id: "qa-testing"
    format_validation: "strict"
```


### Rate limiting and sessions

- **RPM** is enforced with a distributed Redis sliding window (or a local token bucket without Redis).
- **TPM** is enforced with a per-key, one-minute sliding window and reports `X-RateLimit-Limit-Tokens` / `X-RateLimit-Remaining-Tokens`; over-budget requests get `429` with `Retry-After: 60`.
- **Sessions** are cookie-based: login sets an HttpOnly `airoute_session` cookie plus a readable `airoute_csrf` cookie. State-changing requests authenticated by cookie must echo the CSRF value in `X-CSRF-Token`. API clients keep using `Authorization: Bearer`.
- **Login throttling** locks an account/IP pair for 15 minutes after 5 consecutive failures (HTTP 429).
