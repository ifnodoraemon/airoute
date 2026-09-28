# Airoute Configuration & Tuning Manual

This document details the configuration options, cascading model rewrite syntax, circuit breaker parameters, and performance tuning for Airoute.

---

## 1. Configuration File (`configs/config.yaml`)

```yaml
server:
  port: 8080
  read_timeout_sec: 120
  write_timeout_sec: 120
  max_header_bytes: 1048576

database:
  driver: "sqlite"
  dsn: "airoute.db"
  # For PostgreSQL:
  # driver: "postgres"
  # dsn: "postgres://user:password@localhost:5432/airoute?sslmode=disable"

# High-Performance HTTP Connection Pool
connection_pool:
  max_idle_conns: 2048
  max_idle_conns_per_host: 256
  idle_conn_timeout_sec: 90
  tls_handshake_timeout_sec: 10
  disable_compression: false

# Circuit Breaker & Safe Fallback Window
circuit_breaker:
  consecutive_failures: 3       # Trip breaker to OPEN after 3 consecutive failures
  cooldown_seconds: 30          # Wait 30s in OPEN state before canary HALF-OPEN probe
  half_open_probes: 2           # Successful canary probes needed to return to CLOSED

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
  - key: "sk-nano-8f92a1c4b7e3"
    tenant_id: "engineering-dept"
    allowed_models: []          # Empty means all models permitted
    rpm: 120                    # Rate limit: 120 requests/minute
    tpm: 500000                 # Token limit: 500,000 tokens/minute
```

---

## 2. Environment Variables

All settings can be overridden via environment variables for cloud-native Docker / K8s deployments:

| Environment Variable | Default Value | Description |
| :--- | :--- | :--- |
| `GATEWAY_PORT` | `8080` | HTTP listen port |
| `GATEWAY_CONFIG` | `configs/config.yaml` | Path to YAML configuration file |
| `GATEWAY_DB_DRIVER` | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `GATEWAY_DB_DSN` | `airoute.db` | Database connection DSN or SQLite file path |
| `GIN_MODE` | `release` | Web engine mode (`release` or `debug`) |

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
- Zero error frames or disconnects are experienced by the client.

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

