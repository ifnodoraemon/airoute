# Deploy / CLI / Docs Review — 2026-10-27

- **Date**: 2026-10-27 (Asia/Shanghai)
- **HEAD**: `64e749a` ("fix: resolve functional audit findings across auth billing and workflows")
- **Scope**: `cmd/airoute/**` (main.go, server.go, cli.go, cli_*.go — `security_check.go` and `timeout_chain.go` excluded, owned by auth/routing reviewers), `configs/config.yaml`, `deploy/**`, `Dockerfile`, `docker-compose.yml`, `docker-stack.yml`, `helm/**`, `.github/workflows/**`, `scripts/**`, `README.md`, `docs/*.md`, `.env.example` (`.env` read for key names only — no secret values printed).
- **Method**: static cross-reference + read-only probes against the running cluster (no restart/build/mutation). Private worktree `/tmp/airoute-audit-deploy` (detached at HEAD, `web/dist` copied in) used for the CLI/server probes; removed after the review.
- **Verdict**: 18 findings — 1 P0, 4 P1, 13 P2. The remediation commit fixed the *files* it named, but several deployment artifacts still bypass the invariants those fixes established (edge timeout, LB fd limit, release-mode credential gate), and the running cluster is affected by three of them right now.

## Commands run and observed results

| Command | Result |
|---|---|
| `go test ./cmd/airoute/ -run 'Timeout\|Nginx\|Version' -count=1 -v` (worktree) | `TestShippedNginxTimeoutMatchesModelConstant` PASS, `TestEdgeProxyTimeoutExceedsMaxChannelTimeout` PASS, `ok ... 0.005s` |
| `sh scripts/build-version.sh` | `1.1.0-dev+g64e749a`, exit 0 (Go `BaseVersion` == `web/package.json` version == `1.1.0`) |
| `helm lint helm/airoute` | 1 chart linted, 0 failed (only `icon is recommended`) |
| `helm template airoute helm/airoute` / `--set service.port=9090` / `--set ingress.enabled=true` / `--set replicaCount=2` | rendered OK; port-drift and 300s ingress timeouts confirmed; multi-replica guard fails as designed |
| `go build -o /tmp/airoute-cli ./cmd/airoute` (worktree) then CLI probes against `http://127.0.0.1:9` (closed port) | `status` exit 1; `keys` / `models` / `users list` / `skills` / `mcp status` exit **0**; `server -badflag` exit 2; `nosuchcmd` exit 1 |
| `GIN_MODE=release /tmp/airoute-cli -config /tmp/none.yaml -db /tmp/z1.db` | exit 1, `refusing to start: insecure default credentials detected in release mode` |
| `/tmp/airoute-cli -config /tmp/does-not-exist.yaml -db /tmp/probe-missing-config.db` | ran with `DefaultConfig()` (created DB, `admin123` warnings, `artifact storage driver=local`), only died on `bind: address already in use` |
| `docker inspect` / `docker exec printenv` / `docker exec ulimit -n` / `docker logs` / `docker port` (read-only) | see D01, D04, D07, D15 evidence |
| `docker compose config -q` | exit 0 (compose file itself is valid and interpolates against the current `.env`) |
| `curl -s -o /dev/null -w '%{http_code}'` against `127.0.0.1:8080` | `/app/`=200, `/`=200, `/health`=200, `/api/v1/admin/keys` default Host=**401**, `Host: api.airoute.internal`=**403** |
| `docker exec airoute-postgres psql ... "select count(*) from api_keys where key='sk-airoute-production-enterprise-cluster-key'"` | `0` (7 keys total) |
| env drift: `grep os.Getenv` over `internal/` + `cmd/` (77 names) vs `.env.example` (39) vs `.env` (32) vs compose/stack/helm/docs | see D11 |

## Prior findings checked (re-verified, not re-reported)

| Prior claim | Status now |
|---|---|
| `logs.md` item 4 (P0): admin password `admin123` + `GATEWAY_ADMIN_SECRET` = repo-published placeholder `airoute-cluster-admin-shared-secret-key-32b` | **Still open in the live cluster** (D01). `.env` still carries both; the file that was supposed to force remediation is unreachable in the shipped Compose stack. This report's D01 is the *enforcement gap*, not a re-report of the credential itself. |
| README: banner only printed on a TTY (P2 fix) | Holds — `printStartupBanner` checks `os.ModeCharDevice` (`server.go:349-360`); `docker logs` contains 0 banner lines, all JSON |
| README: nginx `proxy_read_timeout/proxy_send_timeout = 86400s` == `model.EdgeProxyTimeoutSeconds` | Holds for `deploy/nginx-compose.conf` only (test PASS), **not** for the Swarm/Helm/legacy configs (D02) |
| README/concurrency.md: `ulimits.nofile = 65536` for gateway/lb/postgres/redis | Holds for gateway (`docker exec airoute-1 ulimit -n` → 65536), **not** for the running LB (1024, D04) |
| `timeout chain` startup invariant log | Present in live logs: `channel=罗总 first_token=5m0s channel_timeout=5m0s edge_timeout=24h0m0s`; container's `/etc/nginx/conf.d/default.conf` holds 86400s |
| Dockerfile ships `curl` so the healthcheck works | Holds — `Dockerfile:49`, live containers `(healthy)` |
| Version derivation (`make docker-rebuild` / CI / Dockerfile ARG agree) | Holds — live binary reports `1.1.0-dev+g64e749a`, same as `build-version.sh`; the only gap is that nothing enforces the Dockerfile/compose defaults after a version bump (D16) |
| Ring-buffer capacity 5000 / `http_access` excluded | Not re-run (out of scope, no deploy impact observed: live `http_access` lines are present and JSON) |

## Findings

| ID | Sev | File:line | Title | Confirmed? | Impact |
|---|---|---|---|---|---|
| D01 | **P0** | `docker-compose.yml:1-22`, `docker-stack.yml:80-87`, `cmd/airoute/server.go:73-82`, `.env` | Release-mode credential gate is unreachable in every shipped Compose/Swarm deployment; live cluster runs `admin/admin123` + published HMAC placeholder | CONFIRMED (live env + logs + probe) | Admin JWT forgeable, console login known-default, and the guard that was added to stop exactly this never fires |
| D02 | P1 | `deploy/swarm/nginx-swarm.conf:65,96,130`; `helm/airoute/values.yaml:42-45`; `deploy/nginx.conf:37`; `cmd/airoute/nginx_timeout_test.go:18` | Edge-timeout invariant is guarded in 1 of 4 deployment targets: Swarm 600s, Helm ingress 300s, legacy conf 300s vs. 86400s budget | CONFIRMED (static + `helm template`) | Channel timeouts up to 86340s are accepted by the API but silently truncated by the deployed edge — the exact bug the remediation claimed to have eliminated |
| D03 | P1 | `deploy/kubernetes.yaml:24-27,40-44,106-110` | Shipped Kubernetes manifest cannot start: `GIN_MODE=release` + no `GATEWAY_ADMIN_PASSWORD` ⇒ `admin123` ⇒ process exits 1 | CONFIRMED (probe exit 1 + rendered manifest) | Following the manifest's own "REQUIRED" replacements still yields CrashLoopBackOff; nothing documents the third required variable |
| D04 | P1 | `scripts/docker-rebuild.sh:56-75`, `docker-compose.yml:44-48,189` | Rolling restart only recreates gateways: the LB container is never recreated or reloaded (live `nofile=1024` although compose declares 65536; nginx resolves upstream IPs only at (re)load) | CONFIRMED (live `docker inspect`: LB `Ulimits=null`, created 2026-10-08; gw `nofile=65536`, created 2026-10-27) | LB is the real concurrency ceiling (~500 proxied streams, not 65536 fds) and can hold a stale gateway IP after a recreate (502/503 history in `airoute-lb` logs) |
| D05 | P1 | `internal/api/mcp_sse.go:84-88`, `internal/controlplane/admin_system_logs.go:62-71`, `configs/config.yaml:11`, `helm/airoute/values.yaml:97` | `http.Server.WriteTimeout` kills long-lived control-plane SSE; only data-plane streams clear the deadline | STATIC (code path) | `/mcp/sse` (MCP clients/Cursor/Claude Desktop) and `/api/v1/admin/system/logs/stream` are cut mid-stream every 600s (Compose) / 180s (Helm) |
| D06 | P2 | `cmd/airoute/server.go:324-338`, `docker-compose.yml`, `docker-stack.yml`, `helm/airoute/templates/deployment.yaml:133` | Graceful drain budget is 10s, hardcoded, and shorter than the 120s aborted-drain budget; no `stop_grace_period`; SIGTERM handler registered only after full init | STATIC | Every deploy kills in-flight >10s requests with no chance to finish the drain; Docker's 10s SIGKILL races the 10s `Shutdown` ctx |
| D07 | P2 | `internal/api/ops_handler.go:18-25`, `docker-compose.yml:117,147`, `helm/.../deployment.yaml:168-179` | `/health` always returns 200 and checks no dependency; it is the Docker healthcheck, the K8s liveness+readiness probe, and the `docker-rebuild.sh` rollout gate | STATIC | A replica with a dead DB/Redis/migration stays "healthy"/"Ready" and keeps receiving traffic; rolling restarts gate on a meaningless signal |
| D08 | P2 | `cmd/airoute/cli_keys.go:22,35,67`; `cli_status.go:95,107`; `cli_users.go:24,88,117,151`; `cli_skills.go:42`; `cli_mcp.go:24,63`; `cli_chat.go:81` | CLI subcommands exit 0 on connection/HTTP failure (only `status` exits 1) | CONFIRMED (probe exit codes) | `airoute keys/models/users/skills/mcp` failures are invisible to shell scripts and CI |
| D09 | P2 | `cmd/airoute/cli_chat.go:76` | `airoute chat` falls back to a hardcoded API key literal `sk-airoute-production-enterprise-cluster-key` that does not exist in the live DB | CONFIRMED (source + `count(*)=0`) | Without `AIROUTE_TOKEN` chat always 401s with a confusing error; source carries a credential-shaped literal and silently ignores the documented token variable |
| D10 | P2 | `internal/config/config.go:172-176` | `-config`/`GATEWAY_CONFIG` pointing at a missing file silently falls back to `DefaultConfig()` instead of failing | CONFIRMED (probe: started, created DB, admin123 warnings, local storage) | A typo'd path / failed mount starts a gateway with the wrong DB, `admin123`, no channels — instead of crash-looping visibly |
| D11 | P2 | `.env.example` (whole file), `docker-stack.yml:87`, `docs/CONFIGURATION.md:76-101` | Env-var drift: 15+ variables read by the code are absent from `.env.example` (and from the docs table), and `docker-stack.yml` passes a `LOG_LEVEL` the Go code never reads | CONFIRMED (77 Go `os.Getenv` names diffed against `.env.example`=39 / `.env`=32 / compose / docs) | Operators cannot discover `GATEWAY_BREAKER_*`, `GATEWAY_DRAIN_*`, `GATEWAY_METRICS_TOKEN`, retention/spill knobs; the Swarm `LOG_LEVEL` knob is dead |
| D12 | P2 | `README.md:57,60,143`; `docs/CONFIGURATION.md:12-13,89`; `configs/config.yaml:10-11` | Docs contradict code: 429 no longer trips the breaker (and the breaker ships advisory), "queue-full backpressure" is now spill-to-disk, and read/write timeouts are 120/600/180 in docs/config/Helm | CONFIRMED (code + rendered chart) | Operators size and tune the system from wrong numbers; e.g. they believe 429 opens the breaker and that a full log queue blocks requests |
| D13 | P2 | `helm/airoute/values.yaml:35,91-97`; `helm/airoute/templates/deployment.yaml:146-179` | Helm `service.port` is unrelated to `config.server.port`: setting `service.port=9090` renders Service/containerPort/probes on 9090 while the app still listens on 8080 | CONFIRMED (`helm template --set service.port=9090`) | Any port change makes the pod permanently un-ready (or silently reachable only via port-forward) |
| D14 | P2 | `configs/config.yaml:30,32,73`; `internal/config/config.go:151-152` | Config-file insecure defaults: `allow_registration: true`, `initial_user_balance: 5.0`, hardcoded S3 access/secret default `airoute_cluster_secret_pass_2026` | STATIC | Bare-binary / missing-`.env` deployments open public signup with free balance and use a repo-published S3 credential |
| D15 | P2 | `deploy/nginx-compose.conf:33-42,128-177` | The "data plane blocks `/api/v1/admin` and `/app`" barrier is Host-header based; the default catch-all vhost serves both to any other Host | CONFIRMED (default Host → admin 401, `Host: api.*` → 403, `/app/` 200 on default Host) | Documented control-plane isolation is cosmetic: an external client just sends a different Host; console/API are exposed on `0.0.0.0:8080` |
| D16 | P2 | `.github/workflows/ci.yml:99-170`, `scripts/build-version.sh:6-12`, `docs/RELEASE_GUIDE.md:39` | CI gaps: no Go linter/vet step (web has lint), no Dockerfile build on PR, no `nginx -t` on the shipped configs, no chart render with non-default values, no e2e suite run, and no check that Dockerfile/compose default versions stay in sync | CONFIRMED (workflow read) | Broken Dockerfile/Helm values/nginx conf merge green; the release-guide "must stay in sync" rule is unenforced |
| D17 | P2 | `cmd/airoute/server.go:61-65` | Startup log always prints the SQLite `-db` path as `db`, even when `DATABASE_URL` (Postgres) is the actual store | CONFIRMED (live: `"db":"/app/data/gateway.db"` while log line 4 shows `postgres:5432/airoute`) | Misleading first line of every container log; operators debugging storage go to the wrong backend |
| D18 | P2 | `Dockerfile:4,16,47` | Build determinism: floating base tags (`node:22-alpine`, `golang:1.26-alpine`, `alpine:3.24`), no digest pinning, no `-trimpath`; `.dockerignore` omits `.env` | STATIC | Two builds of the same commit can differ; `.env` (admin/DB secrets) is uploaded into the build context |

---

### D01 — P0: release-mode credential gate is unreachable in the shipped deployments

**Trigger**: `docker compose up -d` (or `docker stack deploy -f docker-stack.yml`) with the repo's `.env`, i.e. the documented way B. No environment sets `GIN_MODE=release` and `configs/config.yaml` sets `log_level: "info"`.

**Code path**: `cmd/airoute/server.go:73`
```go
isRelease := os.Getenv("GIN_MODE") == "release" || cfg.Server.LogLevel == "warn" || cfg.Server.LogLevel == "error"
```
Only `isRelease` escalates `productionSecurityIssues` from WARN to "refusing to start" (`server.go:74-82`). The shipped Compose stack forwards `GATEWAY_ADMIN_SECRET`/`GATEWAY_ADMIN_PASSWORD` (`docker-compose.yml:5-7`) but no production signal; `docker-stack.yml:81-87` passes neither `GIN_MODE` nor `GATEWAY_ADMIN_PASSWORD` at all. Only `deploy/kubernetes.yaml:26-27` sets `GIN_MODE=release`.

**Evidence (live cluster, started 2026-10-27T09:35)**:
```
{"level":"WARN","msg":"SECURITY WARNING: GATEWAY_ADMIN_SECRET is empty or a well-known placeholder (admin tokens would be forgeable)"...}
{"level":"WARN","msg":"SECURITY WARNING: admin password is the default admin123 or shorter than 8 characters"...}
```
`docker exec airoute-1 printenv`: `GIN_MODE=<unset-or-empty>`, `GATEWAY_ADMIN_PASSWORD` len 8 and sha256 prefix `240be518` == `admin123`, `GATEWAY_ADMIN_SECRET` len 43 and byte-identical to `weakAdminSecrets` entry `airoute-cluster-admin-shared-secret-key-32b` (`cmd/airoute/security_check.go:14`) — the same credential pair already logged as **P0 in `docs/audit/logs.md` item 4**, still effective.

Probe proof that the gate itself works (so the bug is only reachability): `GIN_MODE=release /tmp/airoute-cli -config /tmp/none.yaml -db /tmp/z1.db` → exit 1, `refusing to start: insecure default credentials detected in release mode`.

**Fix (1-3 sentences)**: Set `GIN_MODE=release` (or a dedicated `GATEWAY_PRODUCTION=true`) in `x-gateway-env` and in the Swarm stack, and derive `isRelease` from that explicit flag rather than from `log_level`; additionally refuse a weak `GATEWAY_ADMIN_SECRET`/`GATEWAY_ADMIN_PASSWORD` at startup in *all* modes when `DATABASE_URL` is a cluster DSN, and rotate the two live values.

### D02 — P1: edge-timeout invariant guarded in only one of four deployment targets

**Trigger**: deploy via Docker Swarm (`deploy/swarm/nginx-swarm.conf`) or Helm (`ingress.enabled=true`), and configure a channel timeout > 600s (allowed: `MaxChannelTimeoutSeconds = 86340`, and the startup check only warns above 86400).

**Code path**: `model.EdgeProxyTimeoutSeconds` = 86400 is the single budget (`cmd/airoute/timeout_chain.go:15`), but the only regression guard reads one file:
`cmd/airoute/nginx_timeout_test.go:18` → `filepath.Join("..","..","deploy","nginx-compose.conf")`.

**Evidence**:
```
deploy/swarm/nginx-swarm.conf:65   proxy_read_timeout 600s;   (also :96, :130)
deploy/nginx.conf:37               proxy_read_timeout 300s;
helm/airoute/values.yaml:43-44     proxy-read-timeout: "300" / proxy-send-timeout: "300"
deploy/nginx-compose.conf:65,119,175  86400s   ← the only one the test covers
```
`helm template airoute helm/airoute --set ingress.enabled=true` renders the 300s annotations verbatim (lines 186-187 of the rendered output). The live startup log advertises `edge_timeout=24h0m0s` regardless of which of these is actually deployed.

**Fix**: Make the test iterate over every shipped edge config (`nginx-compose.conf`, `swarm/nginx-swarm.conf`, `deploy/nginx.conf`, Helm ingress annotations) and assert all equal `model.EdgeProxyTimeoutSeconds`; raise the Swarm/Helm values to 86400s so the documented single-knob invariant is true everywhere.

### D03 — P1: shipped Kubernetes manifest crash-loops

**Trigger**: `kubectl apply -f deploy/kubernetes.yaml` after replacing the two placeholders the file itself marks as required (`database-url`, `admin-secret`).

**Code path**: `deploy/kubernetes.yaml:26-27` sets `GIN_MODE=release`; the Deployment env list (lines 30-44) has no `GATEWAY_ADMIN_PASSWORD`, so `config.GetAdminPassword()` falls back to `"admin123"` (`internal/config/config_policies.go:24-32`); `productionSecurityIssues` returns an issue (`security_check.go:29-32`) and `server.go:78-81` calls `os.Exit(1)`.

**Evidence**: probe `GIN_MODE=release /tmp/airoute-cli -config /tmp/none.yaml -db /tmp/z1.db` → exit 1 with the two security WARNs and `refusing to start`; the manifest's Secret (`deploy/kubernetes.yaml:100-110`) contains only `database-url` and `admin-secret`, and its comment ("REQUIRED: replace both placeholders") never mentions an admin password.

**Fix**: Add `GATEWAY_ADMIN_PASSWORD` to the manifest, sourced from `airoute-secret`, and add `admin-password` to the Secret template plus the "REQUIRED" comment; or drop `GIN_MODE=release` from this sample and point users at the Helm chart.

### D04 — P1: rolling restart never recreates/reloads the LB (live drift + stale-DNS class)

**Trigger**: `make docker-rebuild` / `scripts/docker-rebuild.sh` after any change to `docker-compose.yml`'s `lb` definition or `deploy/nginx-compose.conf`.

**Code path**: `scripts/docker-rebuild.sh:58-61` loops only over `gateway-2 gateway-1` and runs `docker compose up -d --no-deps --force-recreate "$service"`; nothing recreates `lb`, and there is no `nginx -s reload` anywhere in the repo. nginx resolves `upstream` hostnames once at config load (no `resolver` directive in any shipped conf — grep confirms), so a recreated gateway can be a stale IP until the LB is reloaded.

**Evidence (live, read-only)**:
```
docker inspect airoute-lb  → HostConfig.Ulimits=null   Created=2026-10-08T04:52:11Z  Started=2026-10-26T00:45:22Z
docker inspect airoute-1   → Ulimits=[{nofile 65536/65536}] Created=2026-10-27T01:35:46Z
docker exec airoute-lb ulimit -n   → 1024        (compose declares 65536, docker-compose.yml:44-48,189)
docker exec airoute-lb grep worker_connections /etc/nginx/nginx.conf → 1024
docker exec airoute-lb ps → master 1d00, workers 16h29  (gateways: 2 min old)
```
So the `ulimits` remediation is only half-applied in the running cluster: the gateway has 65536 fds, the edge still has 1024, giving the LB a hard ceiling of ~500 concurrently proxied streams (2 fds each) regardless of the documented 65536.

Historical proof of the stale-upstream failure mode, from `docker logs airoute-lb` (all read-only):
```
2026/10/08 07:22:34 [error] *354 connect() failed (111: Connection refused) ... upstream: "http://172.22.0.5:8080/api/v1/admin/keys"
2026/10/08 09:20:26 [error] *622 connect() failed (111: Connection refused) ... upstream: "http://172.22.0.4:8080/health"
```
i.e. the LB has previously sent traffic to an IP where no gateway existed, and the access log contains 4×502 + 1×503 overall.

**Fix**: Add `lb` to the recreate/reload step (`docker compose up -d --no-deps --force-recreate lb`, or `docker exec airoute-lb nginx -s reload` after validating `nginx -t`), and add a `resolver 127.0.0.11` + variable `proxy_pass` (or recreate the LB) so upstream DNS is re-resolved; also reload after any edit to the mounted `nginx-compose.conf`.

### D05 — P1: `WriteTimeout` kills long-lived control-plane SSE

**Trigger**: open `/mcp/sse` (MCP transport for Claude Desktop / Cursor) or stream `/api/v1/admin/system/logs/stream`, then keep it open.

**Code path**: `cmd/airoute/server.go:304` sets `WriteTimeout: cfg.Server.WriteTimeoutSec * time.Second` (600s in `configs/config.yaml:11`, 180s in `helm/airoute/values.yaml:97`). `http.Server.WriteTimeout` for HTTP/1 is an absolute deadline from the end of header parsing, not an idle timeout, so an active stream is still cut. `internal/api/common.go:296-307` deliberately clears it with `http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})` — but that helper is only called by the five data-plane stream handlers; the two long-lived SSE endpoints set headers by hand and never clear the deadline (`internal/api/mcp_sse.go:84-88`, `internal/controlplane/admin_system_logs.go:62-71`), and both keep writing heartbeats every 15s forever.

**Evidence**: static code path above; `internal/api/common.go` is the only place in the tree that calls `SetWriteDeadline` (grep). Live nginx side is already 86400s, so Go is the component that truncates — violating the "channel timeout is the only knob / proxy must never end a request first" invariant documented in `docs/audit/README.md` §四之七.

**Fix**: Call `InitSSEStream` (or at least `SetWriteDeadline(time.Time{})`) in `StreamSystemLogs` and `HandleMCPSSE` before the loop, and add a unit test asserting the deadline is cleared for every `text/event-stream` handler.

### D06 — P2: 10s shutdown budget vs 120s drain; no `stop_grace_period`; late signal registration

**Trigger**: `docker compose up -d --force-recreate`, `docker service update`, `kubectl rollout`, Ctrl-C.

**Code path**: `cmd/airoute/server.go:331` hardcodes `context.WithTimeout(..., 10*time.Second)` for `srv.Shutdown`, while the aborted-request drain budget is `GATEWAY_DRAIN_TIMEOUT_SEC=120` (audit README §四之五). No `stop_grace_period` exists in `docker-compose.yml` or `docker-stack.yml`, so Docker's default 10s SIGKILL arrives at the same moment; the chart sets `terminationGracePeriodSeconds: 30` (`deployment.yaml:133`) but the process still gives up at 10s. Separately, `signal.Notify` is only installed at `server.go:326`, after DB open/migrations, Redis init, seeding and admin bootstrap — a SIGTERM during that window terminates the process with no handler (no drain, no `asyncLogger.Stop()`, no spill flush). Same for the fatal-listener `os.Exit(1)` at `server.go:318-321`, which bypasses all deferred cleanup.

**Fix**: Make the shutdown timeout configurable and ≥ `GATEWAY_DRAIN_TIMEOUT_SEC` (or derived from the max channel timeout), set `stop_grace_period`/`terminationGracePeriodSeconds` accordingly, and `signal.Notify` as the very first statement of `main`/`runServer` so late-startup SIGTERM is also graceful.

### D07 — P2: `/health` is unconditional — healthcheck cannot detect dependency loss

**Trigger**: PostgreSQL/Redis/object-storage outage after startup, or a failed migration.

**Code path**: `internal/api/ops_handler.go:18-25` returns `200 {"status":"healthy"}` with no dependency probe. That endpoint is the Docker healthcheck in `docker-compose.yml:117,147` and `Dockerfile:63-64`, the K8s liveness+readiness probe (`helm/.../deployment.yaml:168-179`, `deploy/kubernetes.yaml:55-66`), and the readiness gate that `scripts/docker-rebuild.sh:63-73` polls for 60s.

**Evidence**: `curl http://127.0.0.1:8080/health` → `{"service":"airoute","status":"healthy","timestamp":1793065285,"version":"1.1.0-dev+g64e749a"}` while the same process is configured with `GATEWAY_DB_MAX_OPEN_CONNS` unset etc. — the response contains no dependency state at all. The audit README's 10:03–10:11 502 incident is exactly the class this signal cannot catch.

**Fix**: Keep `/health` shallow for liveness, but make the Docker/compose healthcheck and the chart readiness probe hit `/healthz?deep=1` (or `/readyz`) that returns 503 when the DB ping, Redis ping, or the last migration check fails; keep `docker-rebuild.sh` gated on the deep probe.

### D08 — P2: CLI subcommands exit 0 on failure

**Trigger**: `airoute keys` / `models` / `users list` / `skills` / `mcp status` against an unreachable or misconfigured endpoint (or any 4xx/5xx).

**Code path**: every handler prints the error and `return`s; `main` then falls through to a normal return → exit 0. Only `handleStatus` calls `os.Exit(1)` (`cmd/airoute/cli_status.go:19-22`). Paths: `cli_keys.go:22,35,67`; `cli_status.go:95,107`; `cli_users.go:24,88,117,151`; `cli_skills.go:42`; `cli_mcp.go:24,63`; `cli_chat.go:81,86-87`.

**Evidence (probe binary built at HEAD, `AIROUTE_ENDPOINT=http://127.0.0.1:9`)**:
```
status      → exit 1   ✓
keys        → exit 0   (prints 查询密钥失败: ... connection refused)
models      → exit 0   (prints 请求失败: ... connection refused)
users list  → exit 0
skills      → exit 0
```
`badflag` → exit 2, unknown top-level command → exit 1 (those are fine).

**Fix**: Route handler results through a small error return and `os.Exit(1)` (or a single `fail()` helper) in `main`, so every CLI command reports failure in its exit status.

### D09 — P2: hardcoded stale API key in `airoute chat`

**Trigger**: `airoute chat "hi"` with `AIROUTE_TOKEN` unset.

**Code path**: `cmd/airoute/cli_chat.go:73-76`:
```go
} else {
    // Use default test key
    req.Header.Set("Authorization", "Bearer sk-airoute-production-enterprise-cluster-key")
}
```
Note that `keys/users/mcp/skills` call `ensureAdminToken` / print a hint, but `chat` neither auto-logs-in nor errors out.

**Evidence**: the literal is not in the live DB (`select count(*) from api_keys where key='...'` → 0; 7 keys exist), so the fallback always yields 401 with a generic message. The same literal is also embedded in the local (git-ignored) build artifacts `./airoute` and `./bin/airoute`, i.e. it ships in any binary built from this tree.

**Fix**: Delete the literal, require `AIROUTE_TOKEN`, and print the existing "请设置环境变量 AIROUTE_TOKEN" guidance; optionally use `ensureAdminToken` like `users` does.

### D10 — P2: missing config file silently becomes `DefaultConfig()`

**Trigger**: `-config <typo>`, or `GATEWAY_CONFIG` pointing at a path that a failed mount left empty.

**Code path**: `internal/config/config.go:172-176` — `os.ReadFile` error with `os.IsNotExist` returns `cfg` (pure defaults) and a nil error; `runServer` therefore cannot distinguish "no config" from "config loaded".

**Evidence (probe)**: `-config /tmp/does-not-exist.yaml -db /tmp/probe-missing-config.db` produced
`{"msg":"starting airoute","config":"/tmp/does-not-exist.yaml","db":"/tmp/probe-missing-config.db"}`, `admin123` SECURITY WARNINGs, `artifact storage initialized driver=local`, `gateway server listening addr=0.0.0.0:8080` — it only died because host port 8080 was already taken.

**Fix**: Return an error when an explicitly-specified config path does not exist (keep the implicit default only when `-config` was not passed explicitly and the default path is absent), and log which source produced the effective config.

### D11 — P2: env-var drift across `.env.example` / compose / Swarm / docs / code

**Trigger**: operator provisioning from `.env.example` or reading `docs/CONFIGURATION.md` §2 ("The following environment variables are read by the server").

**Code path / evidence**: 77 distinct `os.Getenv` names in `internal/` + `cmd/`; `.env.example` documents 39; the running `.env` has 32. Read by code but absent from **both** `.env.example` and `.env`:
`GATEWAY_ADMIN_RESET`, `GATEWAY_ADMIN_USERNAME`, `GATEWAY_BLOCK_LOOPBACK_PROBE`, `GATEWAY_BLOCK_PRIVATE_PROBE`, `GATEWAY_BREAKER_COOLDOWN_SEC`, `GATEWAY_BREAKER_ENABLED`, `GATEWAY_BREAKER_FAILURES`, `GATEWAY_CONFIG`, `GATEWAY_DB`, `GATEWAY_DB_DSN`, `GATEWAY_DRAIN_ABORTED_UPSTREAM`, `GATEWAY_DRAIN_TIMEOUT_SEC`, `GATEWAY_FIRST_TOKEN_TIMEOUT`, `GATEWAY_METRICS_TOKEN`, `GATEWAY_PUBLIC_STATUS`, `GATEWAY_USAGE_LOG_PRUNE_INTERVAL_HOURS`, `GATEWAY_USAGE_LOG_RETENTION_DAYS`, `STORAGE_LOCAL_PATH`, `GATEWAY_ALLOW_UNPRICED_MULTIMODAL`.
The current `.env` additionally uses three keys that `.env.example` does not list (`GATEWAY_BREAKER_ENABLED`, `GATEWAY_DRAIN_ABORTED_UPSTREAM`, `GATEWAY_DRAIN_TIMEOUT_SEC`) — the very knobs the audit README tells operators to tune. Conversely `.env.example` lists 12 keys the live `.env` omits entirely (`GATEWAY_TRACE_*`, `GATEWAY_USAGE_SPILL_*`, `GATEWAY_MAX_OVERDRAFT_CNY`, `GATEWAY_DB_MAX_OPEN_CONNS`, `GATEWAY_BILLING_RECONCILE_HOURS`, `GATEWAY_MODERATION_STUB`) — i.e. the running cluster is silently on code defaults for billing/trace/spill/pool sizing. `docker-stack.yml:87` passes `LOG_LEVEL=${LOG_LEVEL:-info}` but nothing in the Go tree reads `LOG_LEVEL` (log level comes only from YAML `server.log_level`), and the Swarm stack never passes `GATEWAY_ADMIN_PASSWORD`.

**Fix**: Generate `.env.example` (or at least a table in `docs/CONFIGURATION.md`) from the actual `os.Getenv` call sites, add the missing keys with their code defaults, remove `LOG_LEVEL` from the Swarm stack (or wire it into `server.log_level`), and add a CI check that every `os.Getenv("GATEWAY_*")` name appears in `.env.example`.

### D12 — P2: docs contradict code (breaker semantics, queue behaviour, timeout numbers)

**Trigger**: operator following `README.md` / `docs/CONFIGURATION.md`.

**Evidence**:
1. `README.md:60` — "发生 3 次连续故障（500/429/超时）即刻熔断 30 秒". Code: `internal/provider/errors.go:79-89` explicitly returns `false` for 429 (only 401/403/408/5xx/network count), and the audit README §四之六 says 429 must never open the breaker; the default is advisory mode (`GATEWAY_BREAKER_ENABLED=false`, live startup log: `circuit breaker runs in advisory mode`).
2. `README.md:57` — "队列满时对请求施加背压". Code: the queue-full path now spills to disk (`internal/storage/usage_spill.go`, `GATEWAY_USAGE_SPILL_DIR`), i.e. the opposite of backpressure.
3. Read/write timeouts: `docs/CONFIGURATION.md:12-13` says 120/120, `configs/config.yaml:10-11` says 600/600, `helm/airoute/values.yaml:96-97` says 180/180, and the code default is 120 (`internal/config/config.go:126-127`).
4. `docs/CONFIGURATION.md:89` lists `GIN_MODE` default `release`; that is true for gin only because `internal/api/router.go:20` hardcodes `gin.SetMode(gin.ReleaseMode)` — gin's own env default is `debug`. The value matters only for the credential gate of D01, where it is unset.
5. `README.md:143` says the host mapping is "8080:80"; the compose file publishes `${GATEWAY_INGRESS_PORT:-${GATEWAY_PORT:-8080}}:80` (`docker-compose.yml:191`), so with `.env`'s `GATEWAY_INGRESS_PORT` the mapping is not necessarily 8080.

**Fix**: Update the README/CONFIGURATION claims to match the current breaker (429 passthrough, advisory default), spill-to-disk behaviour, and the single set of actual timeout values; state that gin release mode is forced in code and that `GIN_MODE` only gates the credential check.

### D13 — P2: Helm `service.port` drifts from `config.server.port`

**Trigger**: `helm install --set service.port=9090` (a normal-looking chart value).

**Code path**: `helm/airoute/templates/deployment.yaml:147-149` sets `containerPort`/probes from `.Values.service.port`, while the app reads its listen port from the rendered `config.yaml` (`values.yaml:94 port: 8080`, `configmap.yaml:8-9`). Nothing cross-validates the two.

**Evidence (`helm template airoute helm/airoute --set service.port=9090`)**: rendered `service` port 9090, `containerPort: 9090`, probes `port: http` → 9090, while the ConfigMap still renders `port: 8080`. The pod would be killed by liveness and never Ready.

**Fix**: Derive `containerPort`/probes from `.Values.config.server.port` (or have the deployment inject `GATEWAY_PORT` from `service.port`), and `fail` when the two values disagree.

### D14 — P2: insecure defaults in the shipped `configs/config.yaml`

**Trigger**: `./bin/airoute -config configs/config.yaml` with no `.env` (README "方式 A", the documented bare-binary path).

**Evidence**: `configs/config.yaml:30 allow_registration: true` and `:32 initial_user_balance: 5.0` — public self-registration with a free ¥5 balance, while `.env.example:28` sets `ALLOW_REGISTRATION=false` (compose deployments get the safe value, bare-binary users do not). `configs/config.yaml:73` and `internal/config/config.go:151-152` hardcode S3 credentials `airoute` / `airoute_cluster_secret_pass_2026` as the default fallback; that string is a published repo constant, so any deployment that leaves `STORAGE_S3_*` unset but switches `STORAGE_DRIVER=s3` uses a known secret. `admin.password: "${GATEWAY_ADMIN_PASSWORD}"` expands to `""`, which `GetAdminPassword()` converts back to `admin123` (`config_policies.go:28-31`).

**Fix**: Default `allow_registration` to false in `configs/config.yaml` (matching `.env.example`), give `initial_user_balance` a 0 default, and remove the hardcoded S3 secret/access-key defaults (require the env vars or fail when `driver=s3`).

### D15 — P2: Host-header control-plane barrier bypassed by the default vhost

**Trigger**: any client that reaches the LB with a Host that is not `api.*` — e.g. `curl http://<host>:8080/api/v1/admin/keys` or `Host: 127.0.0.1`.

**Code path**: `deploy/nginx-compose.conf:33-42` blocks `^/(api/v1/admin|app)` only inside the `server_name api.airoute.internal ~^api\..+$` vhost; the `default_server` catch-all (`:128-177`) proxies everything, including `/api/v1/admin/*` and `/app/`, with only the Go auth layer in front.

**Evidence (live)**: `default` Host `/api/v1/admin/keys` → **401** (reachable, needs auth); `Host: api.airoute.internal` → **403** (barrier); `Host: api.evil.com` → **403**; `/app/` default Host → **200**, `Host: api.airoute.internal` → 403. The compose LB publishes `0.0.0.0:8080` (`docker port airoute-lb` → `80/tcp -> 0.0.0.0:8080`, `[::]:8080`), so the control plane is internet-reachable on every interface and only auth protects it.

**Fix**: Move the `/api/v1/admin` + `/app` 403 rule into the default server (or make the default server data-plane-only), or bind the management listener to a separate internal port/interface; document that the Host split is routing convenience, not a security boundary.

### D16 — P2: CI does not validate the deployment artifacts it ships

**Trigger**: PR that breaks `Dockerfile`, `deploy/*.conf`, `helm/**` values, or the version defaults.

**Evidence** (`.github/workflows/ci.yml`): steps are checkout → `build-version.sh` → node lint/test/build → go mod verify → `go test -race` → `go build` → helm lint. Missing: (a) no Go static analysis (`go vet` beyond what `go test` implies, no golangci-lint/staticcheck) although the web side gets `npm run lint`; (b) the Docker image is only built in `docker.yml` on push-to-main/tags, never for a PR — a broken `Dockerfile` merges green; (c) no `nginx -t` against `deploy/nginx-compose.conf` / `nginx-swarm.conf` (the Go test only regex-checks two numbers); (d) `helm lint` is the only chart check — no `helm template` with non-default values, which is how D13 slips through; (e) `tests/e2e` and `scripts/test-e2e.sh` are never executed by CI even though README §"自动化测试体系" presents them as the acceptance suite; (f) `scripts/build-version.sh:8-12` only compares `internal/buildinfo/version.go` with `web/package.json`, so the Dockerfile `ARG VERSION=1.1.0-dev` (`Dockerfile:21`) and compose `${VERSION:-1.1.0-dev}` (`docker-compose.yml:34`) silently drift after a `BaseVersion` bump — `docs/RELEASE_GUIDE.md:39` says they "must stay in sync" but no check enforces it.

**Fix**: Add a `go vet ./...` (or golangci-lint) step, a `docker build --target go-builder .` (or full buildx) step on PRs, `nginx -t` for both shipped configs (run in the `nginx:alpine` image), `helm template` with `--set service.port=9090,ingress.enabled=true`, an optional e2e job against the compose stack, and extend `build-version.sh` to assert the Dockerfile/compose default versions equal `BaseVersion + "-dev"`.

### D17 — P2: startup log names the wrong database

**Trigger**: every container start with `DATABASE_URL` set (i.e. the whole Compose cluster).

**Code path**: `cmd/airoute/server.go:61-65` logs `db = *dbPath` (the SQLite `-db` value) before `dataSource` is overridden by `DATABASE_URL` at line 67-70.

**Evidence (live)**: line 1 `{"msg":"starting airoute",...,"db":"/app/data/gateway.db"}` while line 4 of the same startup shows `initializing distributed PostgreSQL connection ... postgres:5432/airoute`. Operators looking at the first line conclude SQLite is in use.

**Fix**: Log the effective `dataSource` (or a `provider=postgres|sqlite` field) after the `DATABASE_URL` override.

### D18 — P2: Dockerfile build determinism

**Trigger**: repeated builds of the same commit / supply-chain review.

**Evidence**: `Dockerfile:4` `FROM node:22-alpine`, `:16` `FROM golang:1.26-alpine`, `:47` `FROM alpine:3.24` — all floating tags, no `@sha256` pins, so the same commit can produce different binaries/OS packages over time (and `apk add` at build time pulls current packages). No `-trimpath` in the `go build` (`Dockerfile:40-42`), so build paths leak into the binary; `.dockerignore` (7 lines) omits `.env`, so the Compose secrets file is uploaded into the build context on every build (not copied into the image, but it reaches the daemon/BuildKit cache).

**Fix**: Pin the three base images by digest (Dependabot `docker` ecosystem already tracks them), add `-trimpath`, and add `.env`/`*.local` to `.dockerignore`.
