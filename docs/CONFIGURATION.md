# Airoute Configuration & Tuning Manual

This document details the configuration options, cascading model rewrite syntax, circuit breaker parameters, and performance tuning for Airoute.

The file referenced by `-config` / `GATEWAY_CONFIG` is **required**: if it does not exist the process exits with an error instead of silently running on built-in defaults (which would mean the default SQLite path, no channels and the built-in `admin123` password).

---

## 1. Configuration File (`configs/config.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  # HTTP server timeouts. Streaming (SSE) responses clear the write deadline, so
  # these bound request-body reads and non-streaming responses only.
  read_timeout_sec: 600
  write_timeout_sec: 600
  log_level: "info"
  public_url: "${PUBLIC_URL}"

# Database is selected by command-line -db for SQLite or DATABASE_URL for PostgreSQL.
# HTTP transport and circuit-breaker limits are currently env vars / code defaults,
# not YAML keys.

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

The following environment variables are read by the server. YAML supports `${NAME}` and `${NAME:-default}` expansion. `.env.example` lists every variable below with its default, so it can be copied as a starting point.

### 2.1 Server & deployment

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_HOST` | `0.0.0.0` | HTTP listen address |
| `GATEWAY_PORT` | `8080` | HTTP listen port. In the Helm chart this is derived from `service.port` |
| `GATEWAY_CONFIG` | `configs/config.yaml` | Path to the YAML config file. **Required**: a missing file aborts startup |
| `GATEWAY_DB_DSN` / `GATEWAY_DB` | `data/gateway.db` | SQLite path used when `DATABASE_URL` is unset (equivalent to `-db`) |
| `DATABASE_URL` | unset | PostgreSQL URL; when set, SQLite is not used |
| `REDIS_URL` | unset | Redis connection for distributed rate limits, cache invalidation and the log queue |
| `PUBLIC_URL` / `GATEWAY_PUBLIC_URL` | unset | Externally reachable base URL (used for OAuth redirects and printed endpoints) |
| `GATEWAY_INGRESS_PORT` | falls back to `GATEWAY_PORT` | Host port the Compose load balancer publishes; also the default port the CLI probes |
| `GIN_MODE` | `release` (Compose/Swarm) | Kept for compatibility: gin is forced into release mode in code and the credential gate no longer depends on this value. Only affects the Stripe-webhook startup warning |

### 2.2 Admin, auth & quotas

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_ADMIN_USER` | `admin` | Initial admin username |
| `GATEWAY_ADMIN_USERNAME` | unset | Legacy alias, used only when `GATEWAY_ADMIN_USER` is empty |
| `GATEWAY_ADMIN_PASSWORD` | — | Initial admin password. Startup is **fail-closed**: `admin123`, a password shorter than 8 characters, or an empty value aborts the process |
| `GATEWAY_ADMIN_SECRET` | — | HMAC signing secret. Empty or well-known placeholder values abort startup; use one stable random value (≥ 32 chars) shared by every replica |
| `GATEWAY_ALLOW_INSECURE_DEFAULTS` | `false` | Escape hatch for throwaway local development: `true` downgrades the credential gate to a warning. Never enable in production |
| `GATEWAY_ADMIN_RESET` | `false` | One-shot: force the stored admin password back to `GATEWAY_ADMIN_PASSWORD` on the next start, then set it back to `false` |
| `ALLOW_REGISTRATION` | `false` (`.env.example`) | Public self-registration |
| `INITIAL_USER_BALANCE` | `5.0` | Trial balance granted on registration |
| `DEFAULT_TRIAL_BALANCE` | unset | Legacy alias of `INITIAL_USER_BALANCE`, used only when the latter is empty |
| `REQUIRE_EMAIL_VERIFICATION` | `true` (`.env.example`) | Require the e-mail verification code during registration (needs SMTP) |
| `TOKEN_EXPIRY_HOURS` | `168` | Session/token lifetime in hours |
| `GATEWAY_METRICS_TOKEN` | unset | When set, `/metrics` requires `Authorization: Bearer <token>` |

### 2.3 Runtime tuning: timeouts, breaker, drain

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_FIRST_TOKEN_TIMEOUT` | derived from the channel timeout | Only applies to channels without `timeout_seconds`; otherwise 30s (60s for reasoning model names such as o1/o3/r1/thinking) |
| `GATEWAY_BREAKER_ENABLED` | `true` | Tri-state circuit breaker. `false` = advisory mode (records state, never rejects traffic) — recommended for single-channel deployments |
| `GATEWAY_BREAKER_FAILURES` | `3` | Consecutive channel failures before the breaker opens. HTTP 429 never counts as a channel failure |
| `GATEWAY_BREAKER_COOLDOWN_SEC` | `30` | Open-state cooldown before half-open probing |
| `GATEWAY_DRAIN_ABORTED_UPSTREAM` | `true` | Keep consuming the upstream after the client disconnects and bill the upstream-reported usage; `false` = cancel immediately and bill an estimate |
| `GATEWAY_DRAIN_TIMEOUT_SEC` | `120` | Maximum time a single aborted request may be drained |
| `GATEWAY_PUBLIC_STATUS` | `true` | Set `false` to hide the model catalog on the public status page |
| `GATEWAY_BLOCK_PRIVATE_PROBE` | `false` | Reject probing RFC1918 private upstream addresses |
| `GATEWAY_BLOCK_LOOPBACK_PROBE` | `false` | Reject probing loopback upstream addresses |
| `GATEWAY_MAX_BODY_BYTES` | `67108864` (64 MiB) | Maximum accepted request body size |
| `GATEWAY_ALLOW_UNPRICED_MULTIMODAL` | `0` | Multimodal models without a positive `fixed_price` are rejected with `422 pricing_not_configured`; `1` allows zero-cost forwarding |

### 2.4 Billing, audit, storage & optional integrations

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_TRACE_CAPTURE` | `errors` | Request/response snapshot archiving: `off`, `errors` or `all` |
| `GATEWAY_TRACE_SAMPLE` | `100` | Percentage of **successful** requests captured when mode is `all` |
| `GATEWAY_TRACE_TTL_HOURS` | `72` | Retention window for archived snapshots |
| `GATEWAY_TRACE_MAX_SNIPPET` | `4096` | Maximum bytes retained per request/response snippet |
| `GATEWAY_USAGE_LOG_RETENTION_DAYS` | `365` | Retention window for audit (`usage_logs`) rows; `0` keeps every row |
| `GATEWAY_USAGE_LOG_PRUNE_INTERVAL_HOURS` | `6` | How often expired audit rows are pruned |
| `GATEWAY_MAX_OVERDRAFT_CNY` | `1.0` | Maximum wallet overdraft in CNY; `0` forbids negative balances |
| `GATEWAY_USAGE_SPILL_DIR` | `data` | Disk fallback for billing records when Redis/DB fail (`-` disables, not recommended) |
| `GATEWAY_USAGE_SPILL_MAX_MB` | `256` | Disk fallback size ceiling |
| `GATEWAY_BILLING_RECONCILE_HOURS` | `6` | Ledger reconciliation period; `0` disables |
| `GATEWAY_MODERATION_STUB` | `false` | Return a local fake "all safe" moderation response when no moderation channel exists (development only) |
| `GATEWAY_DB_MAX_OPEN_CONNS` | `25` | PostgreSQL pool size **per replica**; keep `replicas × value` below the server `max_connections` |
| `GATEWAY_DB_MAX_IDLE_CONNS` | `8` | PostgreSQL idle connections per replica |
| `STORAGE_DRIVER` | `local` | Artifact storage: `local`, `s3` or `rustfs` |
| `STORAGE_LOCAL_PATH` | `data/storage` | Directory used by the `local` driver |
| `STORAGE_S3_ENDPOINT` / `STORAGE_S3_BUCKET` / `STORAGE_S3_ACCESS_KEY` / `STORAGE_S3_SECRET_KEY` / `STORAGE_S3_REGION` / `STORAGE_S3_USE_SSL` / `STORAGE_S3_PATH_STYLE` / `STORAGE_S3_PUBLIC_URL_PREFIX` | see `.env.example` | S3-compatible artifact storage |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | unset | E-mail delivery for verification codes and password resets; disabled when `SMTP_HOST` is empty |
| `STRIPE_API_KEY` / `STRIPE_WEBHOOK_SECRET` / `STRIPE_SUCCESS_URL` / `STRIPE_API_BASE` | unset | Stripe payments; webhooks are rejected while `STRIPE_WEBHOOK_SECRET` is empty |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` / `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | unset | OAuth login providers |
| `ENABLE_SANDBOX_RECHARGE` / `ENABLE_SIMULATED_OAUTH` | `false` | Local development only: enable fake recharge / simulated OAuth |
| `AIROUTE_DEV_VERIFICATION_CODE` | unset | Set `true` to return e-mail verification codes in API responses (development only) |

### 2.5 CLI-only variables

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `AIROUTE_ENDPOINT` | derived from `PUBLIC_URL` / `GATEWAY_HOST:PORT` | Gateway the CLI (`status`, `models`, `keys`, `users`, `skills`, `mcp`, `chat`) talks to |
| `AIROUTE_TOKEN` | unset | Credential for CLI calls. `chat` requires a virtual API key and refuses to run without one |

Snapshot contents are redacted (API keys, tokens, passwords) before storage and are queryable at `GET /api/v1/admin/traces?trace_id=…` (admin only; regular users see only their own keys).

### Startup credential gate

The admin password and signing secret are validated before the database is opened. An empty signing secret, a password equal to `admin123` (or shorter than 8 characters), or one of the placeholder values that ship in sample manifests aborts startup with `refusing to start: insecure default credentials detected`. `GATEWAY_ALLOW_INSECURE_DEFAULTS=true` downgrades this to a warning for throwaway local environments only. The Helm chart generates a random password/secret, so it passes by construction.

### Timeout chain

There is one operator-facing timeout: each channel's `timeout_seconds`, which also becomes the first-token window for that channel. Everything above it is derived or asserted:

* channel timeout ≤ `model.MaxChannelTimeoutSeconds` (86340s, derived from the 86400s edge budget);
* every shipped edge proxy must allow the same 86400s budget — `cmd/airoute/nginx_timeout_test.go` asserts this for `deploy/nginx-compose.conf`, `deploy/swarm/nginx-swarm.conf`, `deploy/nginx.conf` and the Helm ingress annotations, so a drifting edge turns the test red;
* streaming (SSE) responses clear the Go `http.Server.WriteTimeout` deadline (`write_timeout_sec`, 600s in `configs/config.yaml`, 180s in the Helm chart) so long generations are not cut mid-stream; **non-streaming** responses remain bounded by that value.

Helm defaults to one SQLite pod with a ReadWriteOnce volume and creates a Kubernetes Secret for the admin password and signing secret. For multiple replicas, provide `database.existingSecret` with key `url` containing a PostgreSQL URL and set `persistence.enabled=false`; configure shared object storage if artifacts must be accessible from every pod. Helm refuses a multi-replica SQLite deployment. Retrieve the generated admin password with `kubectl get secret <fullname>-auth -o jsonpath='{.data.admin-password}' | base64 -d` (for the default `airoute` release, `<fullname>` is `airoute`). Keep the Secret across upgrades. In the chart, `service.port` is the single port value: it drives the Service, the container port, both probes and the `GATEWAY_PORT` the process listens on, so `--set service.port=…` always keeps them consistent.

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

Behavior details that matter in practice:

- Only real channel failures count: `401`/`403`/`408`, any `5xx`, network errors and timeouts. **HTTP `429` is not a channel failure** — it is upstream backpressure and is passed through (with `Retry-After` semantics) instead of opening the breaker.
- The threshold and cooldown are configurable: `GATEWAY_BREAKER_FAILURES` (default 3) and `GATEWAY_BREAKER_COOLDOWN_SEC` (default 30).
- `GATEWAY_BREAKER_ENABLED=false` turns the breaker into **advisory mode**: state is still tracked and logged, but traffic is never rejected. This is recommended while a deployment has only one channel, because tripping the only channel turns upstream errors into gateway-generated failures.
- State is per-process (not shared across replicas); each replica probes half-open independently.

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
