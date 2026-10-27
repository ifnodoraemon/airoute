# Routing / Provider Review — 2026-10-27

- **Date**: 2026-10-27 (Asia/Shanghai)
- **HEAD**: `64e749a` “fix: resolve functional audit findings across auth billing and workflows” (working tree clean apart from an untracked docx)
- **Runtime reviewed**: Docker Compose cluster already running; nothing restarted, no tracked file modified, no `.env` edit. Read-only source review + throwaway worktree probes (`git worktree add --detach /tmp/airoute-audit-routing HEAD`, removed afterwards).
- **Scope**: `internal/router/**`, `internal/provider/**`, `internal/adapter/**`, `internal/model/**`, `cmd/airoute/timeout_chain.go`; cross-package files are cited only as evidence for a routing/timeout defect.
- **Focus**: channel selection weights/priority, breaker tri-state + advisory mode, failover accounting, model naming/cascade/wildcard, base-URL joining, protocol selection, upstream SSE parsing, first-token vs channel timeout, drain/abort lifecycle, connection reuse, retry storms, status mapping, multimodal/video dispatch.

## Test commands and results

| Command | Result |
|---|---|
| `go vet ./internal/router/ ./internal/provider/ ./internal/adapter/ ./internal/model/` | clean |
| `go test ./internal/router/ ./internal/provider/ ./internal/adapter/ ./internal/model/ -race -count=1` | **ok** ×4 packages |
| `go test ./cmd/airoute/ -run 'TestEdgeProxyTimeoutExceedsMaxChannelTimeout\|TestShippedNginxTimeoutMatchesModelConstant' -v` | PASS ×2 |
| Prior-round regressions: `TestAuditReviewMultimodalPreservesAPIBasePath`, `TestAuditReviewVideoPollReturnsToCreatingProvider` (`internal/api`), `TestVideoTaskOwnershipAndPinnedFailure`, `TestAuditReviewChannelTestRespectsTextProtocol` (`internal/controlplane`) | PASS ×4 |
| 16 targeted probes in `/tmp/airoute-audit-routing` (see per-finding evidence) | 15 probes demonstrated the defect behaviour, 1 confirmed a bounded retry; 13 findings reproduced at runtime, R-15 from shipped files, R-13/R-16/R-17 static |

The shipped suite was green before probing; every confirmed finding below is outside its coverage (no tests exist for `gemini_stream.go`, `anthropic_stream.go`, the stream preamble cap, `Dispatch`’s non-health error branch, SWRR/session affinity, or the multimodal breaker classification).

## Prior findings checked (remediation commit)

| Prior ID | Claim | Verification | Status |
|---|---|---|---|
| F05 | multimodal requests preserve the API base path (root, `/v1`, trailing slash, custom prefix, query) | `TestAPIURLPreservesConfiguredPrefix` (in suite) + `TestAuditReviewMultimodalPreservesAPIBasePath`: PASS; `provider.APIURL` covers the listed cases | **fixed**, but the same class still exists in *other* URL builders — see R-12 |
| F06 | video poll returns to the creating channel; gateway task ID; ownership enforced | `TestAuditReviewVideoPollReturnsToCreatingProvider`, `TestVideoTaskOwnershipAndPinnedFailure` (5 subtests): PASS. `DispatchHTTP` pinned path matches id + base URL and bypasses selectors | **fixed** |
| F09 | channel probe passes the real protocol config (`openai_text`) | `TestAuditReviewChannelTestRespectsTextProtocol`: PASS | **fixed** |
| F16 | video poll carries/validates the model stored on the task | `TestVideoTaskOwnershipAndPinnedFailure/model_mismatch`, `model_revoked`: PASS; `HandleVideoTask` rejects a conflicting `model` query | **fixed** |
| F11 (adjacent) | SSE business errors and parse errors separated | Anthropic path fixed; **Gemini path still swallows inline error frames** — see R-10 | **partially fixed** |

## Findings

| ID | Severity | File:line | Title | Confirmed? | Impact |
|---|---|---|---|---|---|
| R-01 | P1 | `internal/provider/client.go:23` | Shared transport’s 60 s `ResponseHeaderTimeout` overrides any channel timeout above 60 s | **confirmed** (60.06 s abort with 120 s budget) | Non-streaming chat/embeddings/rerank/images aborted at 60 s; long reasoning or synchronous image/video generation fails and, being a transport timeout, also counts as a channel failure and can trip breakers |
| R-02 | P1 | `internal/api/upstream_lifecycle.go:56-65`, `.env:69` | The “drain” context is an absolute 120 s deadline on **every** request; channel timeouts > 120 s are unreachable and streams are cut at 120 s | **confirmed** (deadline = 2 m 0 s) | Operator’s `timeout_seconds` (allowed up to 86340 s) silently capped at 120 s; long streams/agents killed mid-response |
| R-03 | P1 | `internal/router/dispatcher_modality.go:135-140` | Multimodal dispatch charges 429 to the circuit breaker, contradicting the 429-is-not-a-failure contract | **confirmed** (3×429 ⇒ breaker OPEN) | With the breaker enabled (default), upstream rate limiting becomes a 30 s 502 blackout for image/audio/video traffic; malformed health state either way |
| R-04 | P1 | `internal/router/dispatcher.go:137-142` | `anthropic_messages` capability is treated as “upstream speaks Anthropic”, so OpenAI/Gemini channels that merely accept Claude traffic are called with Anthropic JSON | **confirmed** (OpenAI provider skipped; Anthropic provider called on `api.openai.com/v1/messages`) | Claude-Code style `/v1/messages` requests to non-Anthropic channels (default console protocol set) get 404 instead of protocol adaptation |
| R-05 | P2 | `internal/router/dispatcher.go:256-265` | Non-streaming `Dispatch` leaks the half-open canary slot on non-health errors (429/404) | **confirmed** (CanaryInFlight=1, next request never reaches upstream) | Channel is locked out of rotation/reported unhealthy for up to 2× cooldown after merely rate limiting |
| R-06 | P2 | `internal/router/circuit_breaker.go:288-297`, `circuit_breaker_config.go:24-31` | Advisory breaker (`GATEWAY_BREAKER_ENABLED=false`) records **nothing**, though documented as “records failures but never rejects traffic” | **confirmed** (5 failures ⇒ CLOSED/0) | Live `.env` runs advisory mode: console/status always shows every channel healthy; outages invisible |
| R-07 | P2 | `internal/router/dispatcher_stream.go:119-126` | 64-event preamble cap drops the very event that commits the stream | **confirmed** (client received `""`) | First content token silently lost when an upstream emits >64 non-content frames before content |
| R-08 | P2 | `internal/provider/anthropic_stream.go:209-260` | Adapted Anthropic stream drops `thinking_delta`/`signature_delta` | **confirmed** (`reasoning=""`) | Claude extended-thinking output vanishes for OpenAI-shaped clients; billing still charges for it |
| R-09 | P2 | `internal/provider/anthropic_stream.go:171-207,233-259` | Anthropic content-block index is emitted as OpenAI tool-call index (not zero-based) | **confirmed** (indexes `[1 1]`) | Clients that key tool deltas by index see a gap/wrong call when a text/thinking block precedes `tool_use` |
| R-10 | P2 | `internal/provider/gemini_stream.go:98-101`, `gemini_types.go:94-97` | Gemini mid-stream error frames are unmarshalled into an empty `GeminiResponse` and emitted as a normal empty chunk | **confirmed** (`sawErr=false`) | Truncated/rate-limited Gemini streams look like successful completions to client and billing |
| R-11 | P2 | `internal/adapter/gemini.go:93-96` | Gemini `functionResponse` loses its call id (`ToolCallID=""`) | **confirmed** | `/v1beta` tool round-trips cannot be replayed to OpenAI (400 invalid tool message) or Anthropic (`tool_use_id` empty) |
| R-12 | P2 | `internal/provider/api_url.go:9-23`, `openai.go:68-74`, `openai_embed.go:26,74-80`, `gemini.go:38-50` | Base-URL semantics resolved 4 different ways; bare origin means `/v1` only on some paths | **confirmed** (`/embeddings` vs `/v1/rerank`; `…/v1beta/v1beta/…`) | One channel config cannot be consistently correct: embeddings/rerank disagree, Gemini doubles `/v1beta`, chat drops `/v1` |
| R-13 | P2 | `internal/router/dispatcher_modality.go:114,225,298` | Images/audio/video/embeddings/rerank/moderation never apply the channel timeout nor the drain context | static | A hung upstream holds the request until client disconnect/nginx 24 h; `timeout_seconds` is inert for these modalities |
| R-14 | P2 | `internal/router/topology.go:179-199` (used at `dispatcher_modality.go:85-87`) | `RewriteJSONModel` round-trips JSON through `map[string]any` → integer precision loss | **confirmed** (`9007199254740993` → `…992`) | Model-mapped image/video bodies with 64-bit `seed`/timestamps are silently altered |
| R-15 | P2 | `cmd/airoute/nginx_timeout_test.go:17-41`; `deploy/swarm/nginx-swarm.conf:65-66`; `deploy/nginx.conf:37-38` | Timeout-chain invariant test covers only `nginx-compose.conf`; shipped swarm/standalone configs are 600 s/300 s | **confirmed** (file contents; test only reads the compose file) | Swarm deployment cuts a 900 s channel timeout at 600 s; the “cannot drift apart” claim in `model/channel.go:184-186` is one file too narrow |
| R-16 | P2 | `internal/model/channel.go:172-174`, `internal/storage/repo_channels.go:110-111,207-208`, `internal/router/dispatcher_stream.go:257-273`, `docs/CONFIGURATION.md:88` | `GATEWAY_FIRST_TOKEN_TIMEOUT` and the reasoning-model 60 s window are dead for DB channels (timeout always set, default 60) | static | Documented tuning knob has no effect; operators cannot shorten the first-token window without changing the whole channel budget |
| R-17 | P2 | `internal/model/channel.go:41-85`, `internal/api/moderation_handler.go:90` | `moderation` is not a known protocol token: explicit `protocols` lists never match it, so capability restrictions are bypassed | static | Moderation can be routed to a channel that declares only `openai_chat`; no way to mark a channel moderation-capable |

Severity summary: **4 × P1, 13 × P2** (17 findings; 13 reproduced with runtime probes, R-15 confirmed from the shipped config files, R-13/R-16/R-17 static-only — each labelled in the table).

---

## Details

### R-01 · P1 · 60 s transport header timeout defeats configured channel timeouts

**Trigger.** Any non-streaming upstream call that takes longer than 60 s to send response headers: long non-stream reasoning completions (o1/R1), synchronous image/video generation endpoints, or slow embeddings. Set `timeout_seconds=300` on the channel and the request still dies at 60 s.

**Code path.** `provider.SharedDefaultHTTPClient` (`internal/provider/client.go:11-30`) sets `Transport.ResponseHeaderTimeout: 60 * time.Second` and `Timeout: 0` with the comment “No client-level body deadline…; ResponseHeaderTimeout protects headers”. Every provider client reuses it. Non-streaming chat is bounded by `channelCallContext` (`internal/router/dispatcher.go:152-157`) with the operator’s `TimeoutSeconds` (accepted up to `MaxChannelTimeoutSeconds = 86340`, `internal/model/channel.go:190-192`), but the transport aborts first. The error is a `net.Error` timeout, so `provider.IsChannelFailure` returns true (`internal/provider/errors.go:91-97`) and the breaker accrues a failure for a perfectly healthy slow model.

**Evidence.** Probe (worktree `internal/provider/audit_slowheader_test.go`): httptest server delays headers 70 s, provider called with a 120 s channel-budget context through `SharedDefaultHTTPClient`:

```
elapsed=1m0.056745153s err=... net/http: timeout awaiting response headers
(channel budget was 120s, header delay was 1m10s)
```

Static cross-check: `TestProbeResponseHeaderTimeoutCapsChannelTimeout` logs `ResponseHeaderTimeout=1m0s MaxChannelTimeoutSeconds=23h59m0s`.

**Suggested fix.** Make the header timeout a variable that is derived from (and always ≥) the channel timeout, or apply the per-channel timeout at the request level and leave the transport header timeout above `MaxChannelTimeoutSeconds` (or 0 for calls under a channel context).

### R-02 · P1 · The drain context is an absolute 120 s request deadline

**Trigger.** Any chat/completions/responses/messages request whose total duration exceeds `GATEWAY_DRAIN_TIMEOUT_SEC` (`.env:69` = 120) — long agentic streams, long generations, or a channel configured with `timeout_seconds` between 121 s and 86340 s.

**Code path.** `upstreamRequestContext` (`internal/api/upstream_lifecycle.go:56-66`) always wraps the provider context with `context.WithTimeout(..., upstreamDrainTimeout())`, both in draining and non-draining mode. It is documented as a *drain* budget for requests the client abandoned, but it is applied before the request starts and is inherited by `DispatchStream` (streams are cut when `ctx.Done()` fires in `pipeStreamEvents`, `internal/router/dispatcher_stream.go:203-221`). Meanwhile `cmd/airoute/timeout_chain.go:20-49` reports “one operator-facing knob: the channel timeout … first-token window is derived from it” and only asserts `edgeProxyTimeout > maxChannelTimeout`; the 120 s bound is absent from that chain. Since channel timeouts default to 60 s and validation allows up to 24 h, the drain value is the real ceiling.

**Evidence.** Probe in the worktree (`internal/api/audit_probe_test.go`):

```
provider-call context deadline in 2m0s (drain timeout 2m0s)
NOTE: channel timeout may be configured up to 86340s; this context forbids anything above 2m0s
```

`.env:69 GATEWAY_DRAIN_TIMEOUT_SEC=120` is wired into both replicas via `env_file: [.env]` (`docker-compose.yml:64,108`).

**Suggested fix.** Either derive the provider context from `max(channel timeout, drain)` per attempt (applying the drain bound only after the client is gone), or make the startup timeout-chain check reject/report channel timeouts above the drain budget instead of claiming the channel timeout decides.

### R-03 · P1 · Multimodal path trips the breaker on upstream 429

**Trigger.** Three consecutive 429 responses from an image/audio/video upstream (normal rate limiting under load).

**Code path.** `DispatchHTTP` explicitly includes `http.StatusTooManyRequests` in the “server error” bucket and calls `recordChannelFailure`, which calls `CircuitBreaker.RecordFailure` (`internal/router/dispatcher_modality.go:135-140`). The chat path deliberately excludes 429: `provider.IsChannelFailure` returns false for 429 (`internal/provider/errors.go:75-90`) and there is a regression test asserting it (`internal/provider/channel_failure_test.go:7-13`). Err text: “A rate limit is backpressure from a healthy upstream, not an outage.”

**Evidence.** Probe `TestProbeDispatchHTTP429TripsBreaker` (httptest 429 + `NewCircuitBreaker(3, 30s)`):

```
breaker after three 429s from the image endpoint: {State:OPEN Failures:3 ... CooldownRemainingMs:29999}
BUG CONFIRMED: three upstream 429s opened the breaker on the multimodal path
```

Downstream effect of OPEN: `DispatchHTTP:75-81` skips the only channel and returns 502 for the cooldown (30 s default) even though the upstream was healthy. The running cluster currently sets `GATEWAY_BREAKER_ENABLED=false` (advisory, see R-06), so it does not black out today — but any deployment with the default (enabled) does, and the failure accounting/reporting is wrong in both modes.

**Suggested fix.** Build an `*provider.UpstreamError` and classify with `provider.IsChannelFailure` (or special-case 429) instead of the ad-hoc status list, so multimodal behaves like chat.

### R-04 · P1 · `anthropic_messages` capability switches the upstream wire protocol

**Trigger.** A `/v1/messages` request routed to a channel that declares the `anthropic_messages` protocol but whose upstream speaks OpenAI (or Gemini) — which is the **default** in the console: `emptyChannel().protocols = ALL_PROTOCOLS` and the openai/sub2api presets use `ALL_PROTOCOLS`, all of which include `anthropic_messages` (`web/src/lib/providerPresets.js:12-16,29,122,198`).

**Code path.** `getProviderForChannel` (`internal/router/dispatcher.go:133-146`) uses the native `AnthropicProvider` when `ch.Type == ProviderAnthropic` **or** `ch.HasExplicitProtocol("anthropic_messages")`; `HasExplicitProtocol` treats `"*"` and alias as well (`internal/model/channel.go:109-122`). `AnthropicProvider.ChatComplete` then posts Anthropic JSON to `BuildAnthropicURL(channel.BaseURL)` (`internal/provider/anthropic.go:41-56,64-90`), e.g. `<openai base>/v1/messages`. The same overloading drives `AnthropicNativeOnly` candidate filtering (`dispatcher.go:296-312`), so a channel that only *accepts* Claude inbound traffic passes the “native” filter. The existing test `TestNativeAnthropicRequestMatchesChannelWithAnthropicMessagesProtocol` confirms this is current intent for “upstream speaks anthropic”, but the capability name/UI use it as “channel serves this endpoint”.

**Evidence.** Probe `TestProbeAnthropicCapabilitySelectsAnthropicProvider`: one `openai` channel with protocols `["openai_chat","openai_response","anthropic_messages"]`, model `gpt-x`; a normal chat request hits the OpenAI provider, an Anthropic-Messages request hits the Anthropic provider:

```
calls: openai 1->1 anthropic 0->1; upstream URL would be https://api.openai.com/v1/messages
BUG CONFIRMED: /v1/messages to an openai channel is sent with the Anthropic protocol to https://api.openai.com/v1/messages
```

**Suggested fix.** Separate the two meanings: pick `AnthropicProvider` only when the channel’s *upstream* is Anthropic (`ch.Type == ProviderAnthropic`, or an explicit `upstream_protocol` field), and treat `anthropic_messages` in `protocols` purely as inbound capability so the OpenAI/Gemini provider performs the adaptation.

### R-05 · P2 · Half-open canary slot leaked in non-streaming `Dispatch`

**Trigger.** Breaker enabled, channel in HALF-OPEN (after cooldown), and the canary attempt returns a non-health error such as 429 or 404.

**Code path.** `Dispatch` (`internal/router/dispatcher.go:196-267`): `CanExecute` takes the only canary slot in HALF-OPEN; the `!provider.IsChannelFailure(err)` branch (lines 256-265) logs and `continue`s **without** `AbortCanary`, unlike the equivalent streaming branch (`dispatcher_stream.go:85-89`) and its own cancel/fatal branches. `CanExecute` then refuses the channel (`halfOpenState.CanExecute`, `circuit_breaker.go:137-149`) until the 2×cooldown stale-canary reset.

**Evidence.** Probe `TestProbeDispatchLeaksHalfOpenCanaryOn429` (500 then 429 through a fake provider, threshold 1, cooldown 5 ms):

```
half-open snapshot after non-channel-failure: {State:HALF-OPEN Failures:1 CanaryInFlight:1 CanExecute:false ...}
third dispatch err=... every candidate channel for model m is temporarily disabled by the circuit breaker ... upstream_calls_delta=0
BUG CONFIRMED: canary slot leaked (inflight=1); following request skipped the upstream entirely
```

**Suggested fix.** Call `d.circuitBreaker.AbortCanary(ch.Name)` in the non-health-error branch of `Dispatch`, mirroring `DispatchStream`.

### R-06 · P2 · Advisory breaker records nothing

**Trigger.** `GATEWAY_BREAKER_ENABLED=false` — the value shipped in `.env:64` and therefore active in the running compose cluster.

**Code path.** `CircuitBreaker.RecordFailure` returns immediately when `cb.disabled` (`internal/router/circuit_breaker.go:288-297`), before `getOrCreateStatusLocked`, so no state is created or updated. `newConfiguredCircuitBreaker` documents the opposite: “it records failures but never rejects traffic” (`internal/router/circuit_breaker_config.go:24-31`); `CanExecute` also returns early (no status), and `RecordSuccess` is a no-op for unknown channels. Consequently `GetStatus`, `Snapshot`, `GetBreakerStatus` and the console routing snapshot (`routing_snapshot.go:87-97`, `Eligible`) always report `CLOSED/0 failures/eligible`.

**Evidence.** Probe `TestProbeAdvisoryBreakerRecordsNothing`:

```
advisory snapshot after 5 failures: {State:CLOSED Failures:0 CanaryInFlight:0 CanExecute:true ...}
BUG CONFIRMED: advisory breaker recorded nothing; operators see a permanently healthy channel
```

The existing test only asserts traffic is not rejected (`internal/router/breaker_config_test.go:11-21`), so this gap is unprotected.

**Suggested fix.** In disabled mode still run `getOrCreateStatusLocked(...).handler.RecordFailure(...)`/`RecordSuccess(...)` but keep `CanExecute` returning true; or update the comment and surface advisory health from a separate counter.

### R-07 · P2 · Stream preamble cap drops the committing content event

**Trigger.** An upstream that sends ≥64 frames with no content/reasoning/tool-call before its first content delta (empty keep-alive data frames, repeated role chunks, native-Anthropic preamble on adapted paths).

**Code path.** `DispatchStream`’s safe-fallback loop (`internal/router/dispatcher_stream.go:105-135`) stores events in `preamble` only `if len(preamble) < 64` (line 121) and *then* marks `committed` when the current event has content (line 124). When the cap is already reached, the committing event is neither buffered nor re-sent: the pipe starts from the next upstream event.

**Evidence.** Probe `TestProbeFirstContentDroppedAfterPreambleCap` (fake provider: 64 empty-delta chunks, then `"FIRST-TOKEN"`):

```
client received content ""
BUG CONFIRMED: the committing content delta was dropped by the 64-event preamble cap
```

**Suggested fix.** Raise the cap and always append/emit the committing event (e.g. if `len(preamble) == cap`, drop the oldest preamble event, not the new one; or bypass the buffer for the committing event).

### R-08 · P2 · Adapted Anthropic stream drops thinking deltas

**Trigger.** A Claude model with extended thinking served through a channel whose upstream is reached via the OpenAI-shaped adapted path (`req.AnthropicOriginal == nil`, e.g. OpenAI-protocol clients or `/v1/messages` from a non-native channel).

**Code path.** `content_block_delta` only handles `delta.text` and `delta.partial_json` (`internal/provider/anthropic_stream.go:209-260`); `thinking_delta` and `signature_delta` fall through and are never emitted. The native path forwards the raw event, so only the adapted path loses it.

**Evidence.** Probe `TestProbeAnthropicThinkingDeltaDropped`: SSE with `content_block_delta {"type":"thinking_delta","thinking":"let me think"}`:

```
adapted stream: reasoning="" content=""
BUG CONFIRMED: thinking_delta was dropped (reasoning_content empty)
```

**Suggested fix.** Translate `thinking_delta → Delta.ReasoningContent` (and expose `signature_delta` as a passthrough/extension) in the adapted branch.

### R-09 · P2 · Anthropic block index used as OpenAI tool-call index

**Trigger.** Claude emits a text/thinking block (index 0) before a `tool_use` block (index 1) — the common “comment then call tool” pattern.

**Code path.** `content_block_start`/`content_block_delta` read Anthropic’s content-block `index` and put it into `model.ToolCall.Index` (`internal/provider/anthropic_stream.go:171-207,233-259`). OpenAI tool-call indexes count tool calls only and must start at 0.

**Evidence.** Probe `TestProbeAnthropicToolCallIndexNotZeroBased` (text block 0, tool_use block 1):

```
tool_call indexes emitted downstream: [1 1]
BUG CONFIRMED: first tool call emitted with index 1 instead of 0
```

**Suggested fix.** Maintain a per-message map from Anthropic block index to a monotonically increasing tool-call index and rewrite `Index` for both the start and `partial_json` deltas.

### R-10 · P2 · Gemini mid-stream error frames swallowed

**Trigger.** Gemini (or a Gemini-compatible proxy) returns a data frame containing an error object mid-stream, e.g. `data: {"error":{"code":500,"message":"..."}}`.

**Code path.** `GeminiProvider.ChatCompleteStream` unmarshals every `data:` payload into `GeminiResponse` (`internal/provider/gemini_stream.go:98-101`); unknown fields are ignored and `GeminiResponse` has no error field (`internal/provider/gemini_types.go:94-97`). The frame becomes a chunk with an empty choice and the loop continues. Compare `OpenAIProvider`, which detects inline error frames (`internal/provider/openai.go:303-318`) and `AnthropicProvider` (`anthropic_stream.go:115-123`).

**Evidence.** Probe `TestProbeGeminiStreamErrorFrameSwallowed`:

```
events: [chunk choices=1 done=false] sawErr=false
BUG CONFIRMED: upstream error frame was swallowed as a normal (empty) chunk
```

**Suggested fix.** Add an `Error *struct{...}` field to `GeminiResponse` (or parse the frame generically) and emit `StreamEvent.Err`, as the other two providers do.

### R-11 · P2 · Gemini adapter drops tool call ids on function responses

**Trigger.** A `/v1beta` conversation containing `functionResponse` parts (Gemini tool calling) routed to a non-Gemini channel.

**Code path.** `GeminiAdapter.ToCanonical` sets `role = "tool"` and the response text, but never sets `ToolCallID` (`internal/adapter/gemini.go:93-96`); the Gemini response has no id, and the corresponding `FunctionCall.ID` was set to the function name only (`gemini.go:75-92`). Downstream, `convertOpenAIToAnthropic` needs `msg.ToolCallID` for `tool_use_id` (`internal/provider/anthropic_convert.go:31-37`) and OpenAI upstreams require `tool_call_id` on tool messages.

**Evidence.** Probe `TestProbeGeminiFunctionResponseLosesToolCallID`:

```
msg[2] role="tool" tool_call_id="" content=map[temp:21] calls=0
BUG CONFIRMED: Gemini functionResponse produced role=tool with empty tool_call_id
```

**Suggested fix.** Track the last `functionCall` per name during conversion and synthesise a stable id (or reuse the function name as the id on both sides), then set `ToolCallID`.

### R-12 · P2 · Base-URL semantics resolved four different ways

**Trigger.** One channel config, different endpoints. Documented convention (docs/PROVIDER_GUIDE.md) is that Base URL includes the API root (`…/v1`, `…/v1-openai`, `…/api/paas/v4`), but the new multimodal joiner treats a bare origin as “origin + /v1”.

**Code path / evidence.** Probe `TestProbeBareOriginConventionDiverges` and `TestProbeBaseURLConventionPerEndpoint`, using `https://host.invalid` without `/v1`:

| Endpoint | Builder | Result with bare origin | Result with `/v1` base |
|---|---|---|---|
| chat | `buildURL` (`provider/openai.go:68-74`) | `/chat/completions` | `/v1/chat/completions` |
| text completion | `buildCompletionURL` (`openai_text_adapter.go:17-26`) | `/completions` | `/v1/completions` |
| embeddings | `openai_embed.go:26` | **`/embeddings`** | `/v1/embeddings` |
| rerank | `openai_embed.go:74-80` | **`/v1/rerank`** | `/v1/rerank` |
| images/audio/video | `APIURL` (`provider/api_url.go:9-23`) | **`/v1/images/...`** | `/v1/images/...` |
| anthropic messages | `BuildAnthropicURL` (`anthropic.go:41-56`) | `/v1/messages` | `/v1/messages` |
| gemini | `buildGeminiURL` (`gemini.go:38-50`) | `/v1beta/models/...` | **`/v1beta/v1beta/models/...`** |

Probe output: `paths: bare-origin embed/rerank=[/embeddings /v1/rerank]`, `gemini … base=https://generativelanguage.googleapis.com/v1beta -> https://generativelanguage.googleapis.com/v1beta/v1beta/models/m:streamGenerateContent?alt=sse`.

**Impact.** A bare-origin channel works for images but not chat; embeddings and rerank on the *same* channel disagree; a Gemini base URL that includes the version (a common copy-paste from docs) doubles `/v1beta` and 404s. `APIURL` also strips only `/v1/`, so any future `/v1beta/...` path through `DispatchHTTP` would be double-prefixed.

**Suggested fix.** Route every builder through one API-root-aware helper with an explicit endpoint-to-version map (e.g. `APIURL(base, endpoint, version)`), and decide/document whether a bare origin implies `/v1`; add a table test covering all provider families.

### R-13 · P2 · Modality paths ignore the channel timeout and the drain context

**Trigger.** A slow/hung image, audio, video, embeddings, rerank or moderation upstream.

**Code path.** `DispatchHTTP` (`internal/router/dispatcher_modality.go:113-114`), `DispatchEmbedding` (line 225) and `DispatchRerank` (line 298) call the provider with the caller’s context unchanged; only chat uses `channelCallContext` (`dispatcher.go:152-157`). The handlers pass `c.Request.Context()` directly (`internal/api/multimodal_handler.go:46`, `embedding_handler.go:42`, `rerank_handler.go:47`, `moderation_handler.go:84`), i.e. neither `timeout_seconds` nor `upstreamRequestContext` applies. The only bound is client disconnect or nginx’s 86400 s.

**Impact.** `timeout_seconds` is advertised as the single per-request upstream budget (docs/CONFIGURATION.md, `model/channel.go:172-192`) but is inert for every non-chat modality; a stalled video/image upstream can hold a request for a day, and aborted multimodal requests are not drained for billing.

**Suggested fix.** Wrap the provider call with `channelCallContext(ctx, ch)` in all three dispatch entry points and pass the same upstream lifecycle context the chat handlers use.

### R-14 · P2 · `RewriteJSONModel` loses integer precision

**Trigger.** A channel with `model_mapping` serving an image/video/audio JSON request that carries a 64-bit integer (commonly `seed`, sometimes timestamps/`created`).

**Code path.** `DispatchHTTP` rewrites the body through `RewriteJSONModel` (`internal/router/dispatcher_modality.go:85-87`); that function unmarshals into `map[string]any` and re-marshals (`internal/router/topology.go:179-199`), so every number becomes `float64`.

**Evidence.** Probe `TestProbeRewriteJSONModelPrecision`:

```
in={"model":"alias","seed":9007199254740993,"n":1}
out={"model":"upstream-model","n":1,"seed":9007199254740992}
BUG CONFIRMED: seed 9007199254740993 was rewritten as 9.007199254740992e+15
```

**Suggested fix.** Use `json.Decoder.UseNumber()` and re-marshal `json.Number`, or patch only the `"model"` value with a targeted decode into `map[string]json.RawMessage`.

### R-15 · P2 · Timeout-chain invariant covers only the compose nginx config

**Trigger.** Deployment via `docker-stack.yml` (swarm) or the standalone `deploy/nginx.conf`, with a channel timeout above 600 s / 300 s.

**Code path.** `internal/model/channel.go:172-192` derives `EdgeProxyTimeoutSeconds = 86400` and claims “a test asserts that the shipped nginx config matches it exactly, so the two cannot drift apart”. The test only reads `deploy/nginx-compose.conf` (`cmd/airoute/nginx_timeout_test.go:17-41`). The repo also ships `deploy/swarm/nginx-swarm.conf` (`proxy_read_timeout/proxy_send_timeout 600s`, lines 65-66/96-97/130-131, referenced by `docker-stack.yml:155`) and `deploy/nginx.conf` (300s, lines 37-38). `cmd/airoute/timeout_chain.go:26-65` reports the same 86400 s edge budget regardless of which manifest is deployed.

**Impact.** On swarm, a channel timeout of 900 s (well inside the documented 24 h allowance) is cut by nginx at 600 s — exactly the failure mode the remediation claimed to eliminate.

**Suggested fix.** Assert every shipped proxy config in the timeout test (or generate them from the constant), and have `reportTimeoutChain` name the actual deployed budget.

### R-16 · P2 · Documented first-token tuning is dead for DB-backed channels

**Trigger.** Operator sets `GATEWAY_FIRST_TOKEN_TIMEOUT=15` (documented as an upper bound, docs/CONFIGURATION.md:88) expecting faster failover, or relies on the 60 s reasoning-model window.

**Code path.** `GetFirstTokenTimeout` returns `ch.TimeoutSeconds` whenever it is > 0 (`internal/router/dispatcher_stream.go:257-264`); the model-name reasoning heuristic (lines 265-269) and `firstTokenTimeoutOverride` (lines 270-272) are therefore only reachable when a channel has **no** timeout. Channel loading and saving always store a timeout (default 60): `internal/storage/repo_channels.go:110-111,207-208`, `internal/storage/repo_model_routes.go:180-181`, and validation defaults it (`internal/controlplane/channel_service.go:190-205`). So for every console-created channel the env var and the 60 s reasoning window never apply — while the startup log (`cmd/airoute/timeout_chain.go:41-49`) presents them as the derived first-token value.

**Suggested fix.** Either treat `GATEWAY_FIRST_TOKEN_TIMEOUT` as a cap on the channel-timeout-derived window (`min(channel, override)`) and document the precedence, or drop the dead branches and the doc entry.

### R-17 · P2 · `moderation` has no protocol alias, so capability lists are bypassed

**Trigger.** `/v1/moderations` with channels that declare explicit `protocols`.

**Code path.** The moderation handler requests protocol `"moderation"` (`internal/api/moderation_handler.go:90`), but `SupportsProtocol` has no `moderation` group (`internal/model/channel.go:41-85`) and the console capability list has no moderation entry (`web/src/components/CapabilitiesSelector.jsx:3-14`). No channel with a non-empty `protocols` list can match, so `ResolveCandidateChannels` (`internal/router/dispatcher_routing.go:123-129`) falls back to `proto = ""` and selects any channel serving the model — including channels explicitly limited to `openai_chat`/`images`. There is also no way to declare moderation capability.

**Suggested fix.** Add `moderation` to the protocol alias table and to the console capability list; a channel declaring only chat should not silently serve moderation.

---

## Notes, coverage and non-findings

- **Verified deliberate and therefore not reported**: half-open canary admission is mutating by design and `Healthy`/`Snapshot` are read-only (comments + `routing_snapshot_test.go` cover it); elapsed-time `Healthy` for OPEN is intentional; 400/404/422 are intentionally non-breaker failures (chat path); cross-model fallback depth/cycle guard looks correct (`dispatcher_fallback.go:23-39`); audit-trail truncation keeps the serving hop (`dispatcher_fallback.go:99-126`); `SWRRSelector`/`SessionAffinitySelector` internal state is guarded by `wrrMu` at the only call site (`dispatcher_routing.go:104-116`) and sweeps stale weights on channel update — no data race found; `-race` suite is clean.
- **Not covered by probes** (static-only, labelled): R-13, R-16, R-17. R-15 is confirmed from file contents, the assertion gap is from the test source.
- **Probe location**: throwaway worktree at `/tmp/airoute-audit-routing` (removed after the review); probes were `*_test.go` files only, never added to the repo. Key snippets and full observed output are quoted in each finding so they are reproducible.
- **Deliberately out of scope** (other reviewers): web console defaults (`web/src/lib/providerPresets.js` only cited as evidence for R-04), `internal/api` handler logic beyond routing evidence, billing DB writes.
