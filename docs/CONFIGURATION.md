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

Shipped defaults in this file are safe by design: `auth.allow_registration: false`, `auth.initial_user_balance: 0` (no free credit), and `storage.driver: local` with **no** S3 credentials. `s3`/`rustfs` deployments must set `STORAGE_S3_ACCESS_KEY` and `STORAGE_S3_SECRET_KEY` explicitly — startup fails with a clear error instead of silently using a default credential or falling back to a local directory.

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
| `GATEWAY_READ_TIMEOUT_SEC` | `600` | `http.Server.ReadTimeout`. Overrides `server.read_timeout_sec` in the config file, which the Compose stack mounts read-only |
| `GATEWAY_WRITE_TIMEOUT_SEC` | `600` | `http.Server.WriteTimeout`. Bounds non-streaming responses (streaming responses clear the deadline). Overrides `server.write_timeout_sec` |
| `GATEWAY_LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error`. Overrides `server.log_level` |
| `GIN_MODE` | `release` (Compose/Swarm) | Kept for compatibility: gin is forced into release mode in code and the credential gate no longer depends on this value. Only affects the Stripe-webhook startup warning |

### 2.2 Admin, auth & quotas

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_ADMIN_USER` | `admin` | Initial admin username |
| `GATEWAY_ADMIN_USERNAME` | unset | Legacy alias, used only when `GATEWAY_ADMIN_USER` is empty |
| `GATEWAY_ADMIN_PASSWORD` | — | Initial admin password. Startup is **fail-closed**: `admin123`, a password shorter than 8 characters, or an empty value aborts the process |
| `GATEWAY_ADMIN_SECRET` | — | HMAC signing secret for admin tokens. Startup is **fail-closed**: an empty value, a value shorter than 32 characters, or one of the placeholder values shipped in sample manifests aborts the process. Use one stable random value (≥ 32 chars) shared by every replica |
| `GATEWAY_ALLOW_INSECURE_DEFAULTS` | `false` | Escape hatch for throwaway local development: `true` downgrades the credential gate to a warning. Never enable in production |
| `GATEWAY_ADMIN_RESET` | `false` | One-shot: force the stored admin password back to `GATEWAY_ADMIN_PASSWORD` on the next start, then set it back to `false` |
| `ALLOW_REGISTRATION` | `false` (shipped config & `.env.example`) | Public self-registration. Off in the shipped `configs/config.yaml`; enable deliberately |
| `INITIAL_USER_BALANCE` | `0` | Trial balance granted on registration. Opt-in: no free credit unless the operator configures a positive value |
| `DEFAULT_TRIAL_BALANCE` | unset | Legacy alias of `INITIAL_USER_BALANCE`, used only when the latter is empty |
| `REQUIRE_EMAIL_VERIFICATION` | `true` (`.env.example`) | Require the e-mail verification code during registration (needs SMTP) |
| `TOKEN_EXPIRY_HOURS` | `168` | Session/token lifetime in hours |
| `GATEWAY_METRICS_TOKEN` | unset | When set, `/metrics` requires `Authorization: Bearer <token>` from every client. When unset, `/metrics` is limited to loopback/private clients |
| `GATEWAY_METRICS_ALLOW_PUBLIC` | `false` | Explicitly open `/metrics` to public clients when no token is configured (not recommended) |
| `GATEWAY_TRUSTED_PROXIES` | unset (trust no forwarding header) | Comma-separated proxy CIDRs whose `X-Forwarded-For`/`X-Real-IP` are trusted for client-IP limits, logging **and the control-plane guard**. Empty trusts nothing, so behind a reverse proxy every request is attributed to the proxy and the control plane fails closed with `403` — set this to the proxy CIDRs. The shipped Compose stack fixes the nginx↔gateway subnet to `172.30.0.0/16` and passes it as the default. An invalid list refuses startup |
| `GATEWAY_ADMIN_ALLOWED_IPS` | unset (loopback + forwarded private clients) | Control-plane CIDR/IP allowlist (`/api/v1/admin`, `/app`). Replaces the default rule; an invalid entry fails closed with 503. A peer that cannot be attributed to a client (no trusted proxy hop) only passes when the peer address itself is on the list |
| `GATEWAY_ADMIN_ALLOWED_HOSTS` | unset | Optional `Host` allowlist for the control plane; supports `*.example.com` |
| `GATEWAY_ADMIN_ALLOW_PUBLIC` | `false` | Explicitly open the control plane to public clients (not recommended) |

### 2.3 Runtime tuning: timeouts, breaker, drain

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_FIRST_TOKEN_TIMEOUT` | derived from the channel timeout | **Global upper bound**: it applies to **every** channel and can only shorten the channel's own `timeout_seconds`, never extend it (the effective window is `min(channel timeout, this value)`). When unset it is derived: 30s, or 60s for reasoning model names such as o1/o3/r1/thinking. Setting it to 10 silently caps a `timeout_seconds: 3600` reasoning channel at 10s |
| `GATEWAY_BREAKER_ENABLED` | `true` | Tri-state circuit breaker. `false` = advisory mode (records state, never rejects traffic) — recommended for single-channel deployments |
| `GATEWAY_BREAKER_FAILURES` | `3` | Consecutive channel failures before the breaker opens. HTTP 429 never counts as a channel failure |
| `GATEWAY_BREAKER_COOLDOWN_SEC` | `30` | Open-state cooldown before half-open probing |
| `GATEWAY_DRAIN_ABORTED_UPSTREAM` | `true` | Keep consuming the upstream after the client disconnects and bill the upstream-reported usage; `false` = cancel immediately and bill an estimate |
| `GATEWAY_DRAIN_TIMEOUT_SEC` | `120` | Maximum time a single aborted request may be drained |
| `GATEWAY_REQUEST_BUDGET_SEC` | `300` | Total budget for a non-streaming dispatch across channel attempts and cross-model fallback; `0` disables it. Streaming requests keep the first-token timeout instead |
| `GATEWAY_SHUTDOWN_TIMEOUT_SEC` | derived: `GATEWAY_DRAIN_TIMEOUT_SEC + 15` (135) | How long in-flight requests may finish after SIGTERM. Must be below the container's `stop_grace_period` / `terminationGracePeriodSeconds` |
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
| `GATEWAY_VERIFICATION_RETENTION_DAYS` | `7` | Retention for e-mail verification codes; `0` keeps every row |
| `GATEWAY_VIDEO_TASK_RETENTION_DAYS` | `30` | Retention for video task records; `0` keeps every row |
| `GATEWAY_TIMESERIES_TTFT_SAMPLE` | `5000` | Row cap for TTFT percentile sampling in the dashboard time series |
| `GATEWAY_MAX_OVERDRAFT_CNY` | `1.0` | Maximum wallet overdraft in CNY; `0` forbids negative balances |
| `GATEWAY_USAGE_SPILL_DIR` | `data` | Disk fallback for billing records when Redis/DB fail (`-` disables, not recommended) |
| `GATEWAY_USAGE_SPILL_MAX_MB` | `256` | Disk fallback size ceiling |
| `GATEWAY_USAGE_SPILL_CLAIM_GRACE_SEC` | `600` | Grace period before another replica may claim a spilled record (shared spill directory) |
| `GATEWAY_BILLING_RECONCILE_HOURS` | `6` | Ledger reconciliation period; `0` disables |
| `GATEWAY_MODERATION_STUB` | `false` | Return a local fake "all safe" moderation response when no moderation channel exists (development only) |
| `GATEWAY_DB_MAX_OPEN_CONNS` | `25` | PostgreSQL pool size **per replica**; keep `replicas × value` below the server `max_connections` |
| `GATEWAY_DB_MAX_IDLE_CONNS` | `8` | PostgreSQL idle connections per replica |
| `STORAGE_DRIVER` | `local` | Artifact storage: `local`, `s3` or `rustfs` |
| `STORAGE_LOCAL_PATH` | `data/storage` | Directory used by the `local` driver |
| `STORAGE_S3_ENDPOINT` / `STORAGE_S3_BUCKET` / `STORAGE_S3_ACCESS_KEY` / `STORAGE_S3_SECRET_KEY` / `STORAGE_S3_REGION` / `STORAGE_S3_USE_SSL` / `STORAGE_S3_PATH_STYLE` / `STORAGE_S3_PUBLIC_URL_PREFIX` | endpoint/bucket/region have defaults; credentials are **empty** | S3-compatible artifact storage. `s3`/`rustfs` without explicit access key + secret key refuses to start (no default credential ships); partial `storage:` blocks are merged field-by-field over the defaults |
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

The admin password and signing secret are validated before the database is opened. Startup aborts with `refusing to start: insecure default credentials detected` when: the signing secret is empty, shorter than 32 characters, or one of the placeholder values that ship in sample manifests; or the password equals `admin123`, is shorter than 8 characters, or is empty. `GATEWAY_ALLOW_INSECURE_DEFAULTS=true` downgrades this to a warning for throwaway local environments only. The Helm chart generates a random password/secret, so it passes by construction.

Note that the 32-character signing-secret floor is a breaking change for an existing deployment whose `GATEWAY_ADMIN_SECRET` was shorter: the process now refuses to start (fail-closed) instead of running with a brute-forcible token signing key. Rotating it invalidates existing admin sessions.

### Timeout chain

There is one operator-facing timeout: each channel's `timeout_seconds`, which also becomes the first-token window for that channel. Everything above it is derived or asserted:

* channel timeout ≤ `model.MaxChannelTimeoutSeconds` (86340s, derived from the 86400s edge budget);
* every shipped edge proxy must allow the same 86400s budget — `cmd/airoute/nginx_timeout_test.go` asserts this for `deploy/nginx-compose.conf`, `deploy/swarm/nginx-swarm.conf`, `deploy/nginx.conf` and the Helm ingress annotations, so a drifting edge turns the test red;
* streaming (SSE) responses clear the Go `http.Server.WriteTimeout` deadline (`write_timeout_sec`, overridable with `GATEWAY_WRITE_TIMEOUT_SEC`; 600s in `configs/config.yaml`, 180s in the Helm chart) so long generations are not cut mid-stream; **non-streaming** responses remain bounded by that value, and the startup self-check now reports `server_write_timeout` on every channel line and warns when `channel timeout > write timeout` — that is the layer that used to cut a request while the report claimed the budget was proven;
* the SIGTERM drain budget (`GATEWAY_SHUTDOWN_TIMEOUT_SEC`, derived from `GATEWAY_DRAIN_TIMEOUT_SEC + 15s`) must fit inside the deployment's stop grace period — the Compose/Swarm services set `stop_grace_period: 150s` and the Helm chart sets `terminationGracePeriodSeconds: 180`.

### Health probes

Two endpoints, on purpose:

| Endpoint | Semantics | Who should use it |
| :--- | :--- | :--- |
| `GET /health` (also `/healthz`, `/v1/health`) | Cheap liveness: the HTTP process answers. Never touches the database or Redis | Edge/nginx probes, humans, liveness where "process wedged" is the failure you care about |
| `GET /health/ready` (also `/healthz/ready`, `/readyz`) | Readiness: 200 only when the data plane has loaded a snapshot **and** the registered dependency probes pass (PostgreSQL/SQLite `PingContext`, Redis `PING` when configured). 503 otherwise, with an unauthenticated-safe per-dependency breakdown (`ok`/`unavailable`) | Kubernetes `readinessProbe`, the Docker/Swarm healthcheck, and the rolling restart gate (`scripts/docker-rebuild.sh` polls the container health status) |

The dependency probes are registered by `cmd/airoute` at startup (`api.RegisterReadinessCheck`). A hung dependency cannot hang the probe: each evaluation is bounded (2s) and a failure is reported as `unavailable` without echoing the raw error (connection strings can contain credentials). Compose/Dockerfile, the Swarm stack, the Helm chart and `deploy/kubernetes.yaml` all probe `/health/ready` for readiness; `cmd/airoute/deploy_readiness_test.go` keeps them aligned with the code.

### Shutdown & drain

On `SIGTERM`/`SIGINT` the process stops accepting connections and waits up to `GATEWAY_SHUTDOWN_TIMEOUT_SEC` for in-flight requests (the aborted-request drain can legitimately take up to `GATEWAY_DRAIN_TIMEOUT_SEC`), then stops the async audit logger (flushing the queue) and closes the database. The signal handler is installed before initialization, so a signal during migrations/bootstrap still runs that cleanup instead of killing the process. Set the orchestrator grace period above the shutdown budget: Compose/Swarm `stop_grace_period: 150s`, Helm `terminationGracePeriodSeconds: 180` (both configurable in the chart).

### Control-plane exposure

The control plane is exactly two path prefixes: **`/api/v1/admin`** (the management API) and **`/app`** (the console shell). It is enforced **inside the gateway**, not by the nginx `Host` header, and the edge configs apply the same allow/deny list as defence in depth (`deploy/nginx-compose.conf`, `deploy/swarm/nginx-swarm.conf`, `deploy/nginx.conf`; `cmd/airoute/nginx_admin_acl_test.go` keeps them aligned).

What is deliberately *not* part of that predicate:

* `GET /` renders the **public landing page and login screen** (`web/src/lib/consoleRoute.js` resolves a bare `/` to `viewMode: 'landing'`), so guarding it would make the public entrance unreachable;
* `/assets` only serves the content-hashed JS/CSS bundles of that shell (no API data);
* `/api/v1/auth/*` is the CLI's remote-login path;
* `/api/v1/user/*` is the tenant self-service API (wallet, usage), reachable by ordinary customers.

The guard decides on the **client identity**, not the raw TCP peer: a request is attributable only when it arrived through a proxy listed in `GATEWAY_TRUSTED_PROXIES`. Loopback clients are always attributable; private clients that came through a trusted proxy are admitted; public clients receive `403`; and a request from a non-loopback peer with no forwarding information (an unconfigured proxy) **fails closed with `403`** — the error names `GATEWAY_TRUSTED_PROXIES` as the fix. This is why the shipped Compose topology fixes the nginx↔gateway subnet (`172.30.0.0/16`) and passes it as the default: without it every request would be attributed to the load balancer, which is exactly how the old guard ended up admitting everyone.

Note the console SPA is also the app served on `/`; the management *data* is only reachable through the guarded `/api/v1/admin` API, so a public visitor gets the landing page but no administrative capability.

A deployment **without** a reverse proxy (bare binary on a LAN, direct in-cluster access, `deploy/kubernetes.yaml` without an ingress) has no forwarding header to attribute, so it must set `GATEWAY_ADMIN_ALLOWED_IPS` to the client CIDR — the explicit list also admits a matching direct TCP peer. Leaving both variables unset restricts the control plane to loopback, which is the intended fail-closed default.

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_TRUSTED_PROXIES` | unset | Proxy CIDRs whose forwarding headers are trusted. Empty means no client attribution, so the control plane fails closed behind a proxy |
| `GATEWAY_ADMIN_ALLOWED_IPS` | unset | Comma-separated CIDR/IP allowlist that replaces the loopback/forwarded-private default. An invalid entry fails closed (`503`); a non-attributable peer passes only if the peer itself is listed |
| `GATEWAY_ADMIN_ALLOWED_HOSTS` | unset | Optional comma-separated `Host` allowlist (`*.example.com` wildcard supported) |
| `GATEWAY_ADMIN_ALLOW_PUBLIC` | `false` | Explicitly allow public clients (not recommended; the guard logs a warning) |

The gateway still authenticates and authorizes every admin request; this guard is an additional network-boundary control. For a stricter posture bind the published port to loopback (`ports: ["127.0.0.1:8080:80"]`) or restrict it with a firewall/VPN.

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

- **Key quota budget** (`budget`, CNY) is a *pre-dispatch* ceiling: the check compares the usage already
  settled against the limit. Requests that are already in flight when the budget is crossed still complete
  and are charged, so a burst can overshoot by up to (in-flight requests × per-request cost); afterwards the
  key answers `402 key_budget_exceeded`. The ledger itself stays exact — `used_cost_units` always equals the
  sum of `usage_logs.cost_units` for that key, verified under concurrent cross-replica load.
- **Multi-replica config propagation**: with `REDIS_URL` set, channel/pricing/key changes are broadcast to
  peers immediately. Without Redis each replica reloads its data-plane snapshot from the database every 10s,
  so a freshly created channel can stay unroutable on a peer for up to ~10s (those requests fail with
  `404 model_not_found`); requests already routed keep working.
- **RPM** is enforced with a 60s sliding window: the same rule is implemented in Redis (cross-replica, `internal/distributed`) and locally in the gateway (when Redis is not configured or a Redis call fails). Both admit at most `rpm` requests in any 60s interval, so falling back to the local limiter does **not** loosen the quota; during an outage the only difference is scope — each replica enforces `rpm` on its own share of the traffic instead of the cluster-wide count.
- **TPM** is enforced with a per-key, one-minute sliding window and reports `X-RateLimit-Limit-Tokens` / `X-RateLimit-Remaining-Tokens`; over-budget requests get `429` with `Retry-After: 60`.
- **Sessions** are cookie-based: login sets an HttpOnly `airoute_session` cookie plus a readable `airoute_csrf` cookie. State-changing requests authenticated by cookie must echo the CSRF value in `X-CSRF-Token`. API clients keep using `Authorization: Bearer`.
- **Login throttling** locks an account/IP pair for 15 minutes after 5 consecutive failures (HTTP 429).
