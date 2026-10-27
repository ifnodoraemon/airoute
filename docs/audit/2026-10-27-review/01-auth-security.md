# Auth / Security Review — 2026-10-27

- **Date**: 2026-10-27 (Asia/Shanghai)
- **HEAD**: `64e749a` — "fix: resolve functional audit findings across auth billing and workflows" (remediation commit for the 18 findings in `functional-details-2026-10-26.md`)
- **Scope**: `internal/controlplane/**` (auth/session/token, OAuth handler+providers+state, account/identity/users/API-key governance, admin authorization & route wiring, login guard), `internal/middleware/**` (data-plane auth, rate limiting, TPM/quota, trace/access log), `cmd/airoute/security_check.go`, `internal/api/mcp_handler.go` / `mcp_sse.go` / `mcp_tools*.go` (MCP authz), `internal/api/mcp_auth_test.go`, `mcp_sql_guard_test.go`. Deployment config (`deploy/nginx-compose.conf`, `.env`, `docker-compose.yml`) consulted only to judge reachability. Web session storage inspected for the F18 (HttpOnly) invariant.
- **Method**: code tracing on a detached worktree at HEAD plus live read-only probes against the running Compose cluster (`airoute-lb` → `airoute-1`/`airoute-2`, port 8080). No service was restarted, no tracked file or `.env` modified, nothing committed. Probe `_test.go` files live only in `/tmp/airoute-audit-auth` (private worktree, removed after review).

## Test commands run (verbatim) and results

| # | Command | Result |
|---|---|---|
| 1 | `cd /home/ifnodoraemon/myagent/airoute && git worktree add --detach /tmp/airoute-audit-auth HEAD` | worktree at 64e749a |
| 2 | `cp -r web/dist /tmp/airoute-audit-auth/web/dist` | OK (needed by `internal/api`) |
| 3 | `cd /tmp/airoute-audit-auth && PATH=/usr/local/go/bin:$PATH go vet ./internal/controlplane/ ./internal/middleware/` | clean |
| 4 | `go test ./internal/controlplane/ -run 'TestAuthRejectsDeletedAndDemotedUsers\|TestOAuthState\|TestBindOAuthState\|TestSimulatedOAuth\|TestAuditReviewPasswordChangeRevokesOldSession\|TestPasswordResetRevokesSessionsAcrossHandlers\|TestOAuthStableSubjectAndCrossReplicaCookie\|TestCreateAPIKeyIgnoresClientOwnedFields\|TestAuditReviewGoogleDisplayNameDoesNotIdentifyAccount\|TestDeletingLastKeyKeepsAuthenticationEnforced' -race -count=1` | `ok ... 8.183s` (baseline green) |
| 5 | `go test ./internal/middleware/ ./internal/api/ -run 'TestAuth\|TestMCP' -race -count=1` | `ok middleware 1.020s`, `ok api 1.844s` |
| 6 | `go test ./internal/api/ -run 'TestProbe' -race -count=1 -v` (probe file `internal/api/mcp_authz_probe_test.go`) | 2/2 PASS — evidence for A02, A08 |
| 7 | `go test ./internal/controlplane/ -run 'TestProbe' -race -count=1 -v` (probe files `authz_probe_test.go`, `register_probe_test.go`) | 6/6 PASS — evidence for A03, A05, A09, A10, A17 |
| 8 | `curl -s -X POST http://127.0.0.1:8080/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin123"}'` | HTTP 200 + token, `"is_default_password":true` — A01 |
| 9 | `curl -s "http://127.0.0.1:8080/api/v1/admin/users?token=<session token>"` | HTTP 200 — A01/A07/A14 |
| 10 | `curl -s -D - 'http://127.0.0.1:8080/api/v1/auth/oauth/google/callback?code=abc&state=bad' -H 'X-Forwarded-Host: evil.example.com'` | `302 Location: http://evil.example.com/?oauth_error=...#/login` — A06 |
| 11 | login timing loop (`curl -w %{time_total}` ×3 per case) | admin 0.277–0.282 s vs unknown user 0.0027–0.0029 s — A13 |
| 12 | login loop with rotated / constant `X-Forwarded-For` (7 + 4 rotated attempts, 16 constant) | rotated XFF → 401×11 (no lockout); constant XFF → `401 429 429 … 401 …` — A04 |
| 13 | `curl --max-time 2 "http://127.0.0.1:8080/mcp/sse?apiKey=<key>"` then `POST /mcp/messages?sessionId=<sid>`; `docker logs --since 25s airoute-1` | SSE 200; access log line `"path":"/mcp/messages?sessionId=mcp-2acd2e9a04711f6b"` unredacted — A08 |

## Findings

| ID | Sev | file:line | Title | Confirmed? | Impact |
|---|---|---|---|---|---|
| A01 | P0 | `cmd/airoute/security_check.go:11-17`, `cmd/airoute/server.go:73-82`, `.env:22-23` | Running cluster accepts `admin`/`admin123` and the in-repo placeholder signing secret | CONFIRMED (live login + admin API) | Full control-plane takeover from the published LB port |
| A02 | P1 | `internal/api/mcp_tools_routing.go:146-243` | MCP `airoute_chat` bypasses wallet balance, key budget, account lock, model allowlist and RPM/TPM | CONFIRMED (probe) | Locked/insolvent/denied tenants consume paid inference |
| A03 | P1 | `cmd/airoute/server.go:219-222`, `internal/middleware/auth.go:64-67` | Failed initial DB snapshot leaves data plane in unauthenticated "open mode" | CONFIRMED (probe of resulting state; server branch static) | DB hiccup at boot ⇒ anonymous inference, possibly indefinitely |
| A04 | P1 | `internal/controlplane/account_handler.go:28-33`, `login_guard.go:29-104` | Login brute-force guard bypassed by spoofed `X-Forwarded-For`; counters are per-replica | CONFIRMED (live) | Unlimited password guessing against admin login via public LB |
| A05 | P1 | `internal/controlplane/identity_handler.go:24-108`, `.env:28-30` | Open registration with trial credit, no throttle and no verification | CONFIRMED (probe + live config) | Unlimited synthetic accounts each minting ¥5 + working API key |
| A06 | P2 | `internal/controlplane/admin_handler.go:142-168`, `oauth_handler.go:89-103` | Open redirect after OAuth via `Host` / `X-Forwarded-Host` | CONFIRMED (live 302) | Post-login phishing to attacker-controlled origin |
| A07 | P2 | `internal/controlplane/auth.go:60-71` | Session token accepted from `?token=` on every protected endpoint | CONFIRMED (live 200) | 7-day session credential leaks into URLs/logs/Referer |
| A08 | P2 | `internal/api/mcp_handler.go:71-79`, `mcp_sse.go:104-114`, `internal/middleware/trace.go:106-120` | MCP SSE `sessionId` is a bearer credential and is logged unredacted | CONFIRMED (probe + live log) | Log access ⇒ impersonate key owner's MCP identity (incl. admin SQL tools) |
| A09 | P2 | `internal/controlplane/user_admin_handler.go:222-258` | Any admin can reset the built-in root admin's password | CONFIRMED (probe) | Silent takeover of the root account, audit attribution loss |
| A10 | P2 | `internal/controlplane/user_key_rules.go:49-57` | Self-service key `tenant_id` derived from client-controlled key name | CONFIRMED (probe) | Tenant attribution spoofing in logs/dashboards/exports |
| A11 | P2 | `internal/controlplane/user_key_rules.go:34-47` | Self-service keys default to `RPM=0` (unlimited) | STATIC (code + registration path contrast) | Per-key rate limit is opt-in; tenants self-grant unlimited RPM |
| A12 | P2 | `internal/api/common.go:178`, `internal/storage/repo_keys.go` | API keys and usage-log rows persist cleartext credentials | STATIC | DB/backup/replica read yields usable keys; masking is display-only |
| A13 | P2 | `internal/controlplane/account_handler.go:35-51` | Login timing + locked-state oracle enumerates accounts | CONFIRMED (live timing) | Username/email enumeration; enables targeted attacks |
| A14 | P2 | `deploy/nginx-compose.conf:44-50,129-130`, `internal/api/router.go:143` | "Management domain" isolation bypassable via default nginx vhost | CONFIRMED (live 200 via :8080) | Admin API reachable from any Host/IP; the host barrier is not enforced |
| A15 | P2 | `web/src/components/LoginModal.jsx:80,154`, `AuthPage.jsx:98,179` | Console still writes the raw session token to `localStorage` | STATIC (code contradicts `web/src/api/session.js:1-10`) | Contradicts F18 HttpOnly design; credential transiently in web storage |
| A16 | P2 | `internal/api/mcp_tools_ops.go:16-79` | Any tenant API key can enumerate internal channel inventory via MCP | STATIC | Infrastructure/provider topology disclosure to tenants |
| A17 | P2 | `internal/controlplane/oauth_handler.go:155-166,227-273` | OAuth username (`gh_<id>`,`gg_<sub>`) can be pre-empted by registration | CONFIRMED (probe) | That user's first OAuth login fails with 500 instead of linking |

**Counts: P0 = 1, P1 = 4, P2 = 12 (17 total).**

---

## Per-finding detail

### A01 · P0 · Default admin credentials + placeholder shared secret are live

**Trigger/preconditions**: attacker can reach the gateway LB (published `0.0.0.0:8080`, nginx default vhost proxies everything). No prior access needed.

**Code path**: `cmd/airoute/security_check.go:11-17` lists `airoute-cluster-admin-shared-secret-key-32b` and `admin123` as well-known weak values; `cmd/airoute/server.go:73-82` computes `isRelease` from `GIN_MODE`/log level and only `os.Exit(1)` when that is true. Docker/compose set neither (`Dockerfile` has no `GIN_MODE`; `.env` no `LOG_LEVEL`), so the process logs two `SECURITY WARNING` lines and starts. `.env:22-23` contains exactly those weak values (`GATEWAY_ADMIN_PASSWORD=admin123`, `GATEWAY_ADMIN_SECRET=airoute-cluster-admin-shared-secret-key-32b`), and `.env` is the single source for `docker-compose.yml` (`x-gateway-env`).

**Evidence (live)**:
```
POST /api/v1/auth/login {"username":"admin","password":"admin123"}
-> 200 {"code":0,"data":{"token":"eyJzdWIiOiJhZG1pbiIs...","user":{...,"is_admin":true,"is_default_password":true}}}
GET /api/v1/admin/users?token=<token>  -> 200
```
`is_default_password:true` is computed by the handlers (`account_handler.go:61-67`, `:116-121`) by bcrypt-comparing the stored hash with `admin123`, so this is the stored root credential, not a stale session.

**Impact**: full read/write of the control plane (users, channels, keys, pricing, logs) from the public port; the token-signing secret is also a public constant, so any future relaxation of the `CredentialTag` check (or a leaked password hash) becomes immediately exploitable.

**Suggested fix**: make the weak-credential guard unconditional for the control plane (or at minimum refuse admin login with the default password and force rotation via the existing `is_default_password` flag); rotate both values before any deployment. Code change in `security_check.go`/`server.go`; the `.env` values need operator action.

### A02 · P1 · MCP `airoute_chat` bypasses all data-plane authorization

**Trigger/preconditions**: caller holds any API key with `status=active`, MCP enabled (default), `mcp_enabled` setting not false.

**Code path**: `internal/api/mcp_handler.go:40-52` (`authenticateCaller`) checks only `rec.Status=="active"`. `internal/api/mcp_tools_routing.go:146-243` re-checks only `GetAPIKeyByKey` + `kRec.Status != "active"` (line 165) and then calls `h.dispatcher.Dispatch` (line 190). Nothing checks: key budget/`budget_units`, user `status`/`balance`, key `AllowedModels`, RPM/TPM, or the in-flight wallet reservation that `/v1/**` enforces at `internal/middleware/auth.go:120-260` and `ValidateModelAllowed` (`auth.go:271-294`). Usage is written to `usage_logs` with the real key, so settlement happens afterwards (async), never as a gate.

**Evidence (probe, `internal/api/mcp_authz_probe_test.go`, PASS)**:
```
airoute_chat output for locked/exhausted key: "No response choices returned from upstream model"   <- reached dispatch
data-plane status: 403 {"code":"account_locked"}      <- same key/user on /v1/models
data-plane status: 402 {"code":"key_budget_exceeded"} <- same key/budget on /v1/models
```
The key was `Budget:1.0, UsedCost:500`, user `Status:"locked", Balance:0`, `AllowedModels:["only-this-model"]`, model requested `expensive-model-not-in-allowlist`.

**Impact**: a locked or insolvent tenant keeps consuming billable upstream inference through MCP; the per-key model allowlist and rate limits are bypassed; because settlement is asynchronous the wallet can go negative without any pre-flight refusal.

**Suggested fix**: extract the data-plane authorization (status/budget/balance/model allowlist/rate limits) into a helper and call it from the MCP dispatch path before `Dispatch`; reject `airoute_chat` when any check fails.

### A03 · P1 · Startup snapshot failure ⇒ unauthenticated data plane

**Trigger/preconditions**: `storage.OpenDB` succeeds (otherwise the process exits earlier) but the first `ReloadFromDB` fails — transient DB reset, migration/schema drift, or a failing read.

**Code path**: `cmd/airoute/server.go:219-222` logs `initial sync from db failed, using config file defaults` and continues with `dispatcher.UpdateChannels(cfg.Channels)`. `Synchronizer.ReloadFromDB` only calls `applySnapshot` on a fully successful read (`internal/controlplane/sync.go` load/apply split), so `config.HasConfiguredKeys` keeps its zero value `false` and `cfg.APIKeys` is empty. `internal/middleware/auth.go:64-67` then takes the open-dev-mode branch (`ActiveAPIKeyCount()==0 && !HasRegisteredAPIKeys()`) and calls `c.Next()` without any credential check. The 10s periodic sync is the only recovery path.

**Evidence (probe, `internal/controlplane/authz_probe_test.go`, PASS)**:
```
after failed initial sync: active=0 hasRegistered=false
```
i.e. exactly the state that the pinned `TestAuthMiddlewareOpenModeRequiresNoKeyEverRegistered` documents as "anonymous request → 200".

**Impact**: anonymous, unbilled access to every `/v1/**`, `/v1beta/**` and root data-plane route, on a cluster that has credentials registered in its database, until a reload succeeds (indefinitely if the DB stays unavailable).

**Suggested fix**: fail closed — refuse to start (or stay unready and not serve `/v1`) until the first snapshot loads; if a degraded start is desired, set `HasConfiguredKeys` conservatively (e.g. from a successful `CountAPIKeys`) before enabling the listener.

### A04 · P1 · Login lockout defeated by `X-Forwarded-For`; per-replica counters

**Trigger/preconditions**: unauthenticated attacker reaching the LB. `airoute-lb` publishes nginx :80 on `0.0.0.0:8080`; nginx appends the client-supplied header (`proxy_add_x_forwarded_for`, `deploy/nginx-compose.conf:57,92,113`).

**Code path**: `account_handler.go:28` uses `c.ClientIP()`. No `engine.SetTrustedProxies(...)` exists anywhere in the repo (verified by grep), so Gin's default trusts every proxy and `ClientIP()` returns the **first** `X-Forwarded-For` entry — fully client-controlled. The failure budget itself is a process-local map (`login_guard.go:29-104`), and the cluster runs two replicas behind `least_conn`, so the effective threshold is split and doubled. The per-username bucket is trivially rotated too (usernames in the body).

**Evidence (live)**:
```
constant XFF 198.51.100.7, 16 attempts, distinct usernames:
  401 429 429 429 429 429 429 429 401 429 429 429 429 429 429 429   (lock fires on the spoofed IP; two replicas disagree)
rotating XFF 203.0.113.x (7 + 4 attempts): 401 401 401 401 401 401 401 / 401 401 401 401  (11/11, no 429)
```
The interleaved 401s in the first run are the second replica, which has its own counter.

**Impact**: password guessing against any account (including root, see A01) is not rate-limited; the `ip` field in access logs/audit is attacker-chosen (log poisoning).

**Suggested fix**: configure trusted proxies / derive the client IP only from nginx-set `X-Real-IP` and ignore client XFF; move the failure counters to Redis (the deployment already has it) so replicas share one budget.

### A05 · P1 · Registration with trial credit: no throttle, no verification

**Trigger/preconditions**: `ALLOW_REGISTRATION=true`, `INITIAL_USER_BALANCE=5.0`, `REQUIRE_EMAIL_VERIFICATION=false` — all three are in the live cluster's `.env:28-30` and confirmed by `/api/v1/public/status` (`allow_registration:true, require_email_verification:false, stripe_enabled:false`). nginx has no `limit_req` (grep: none).

**Code path**: `identity_handler.go:24-108`. The only gates are `IsRegistrationAllowed()` (`:25`), an optional verification code (`:50-57`), and uniqueness checks; there is no rate limiting, no CAPTCHA, and no email ownership proof. Every successful registration creates an API key and credits `GetInitialUserBalance()` (`:95-108`).

**Evidence (probe, `internal/controlplane/register_probe_test.go`, PASS)**:
```
5 consecutive unauthenticated registrations succeeded, total trial credit minted: 25.00
```
plus the live config above.

**Impact**: free-credit farming (each account yields ¥5 of inference + a working key), user-table pollution, and amplification of any per-account rate limit (e.g. redemption-code attempts are 10/min **per account**, `redemption_handler.go:150-160`).

**Suggested fix**: rate-limit registration per IP/globally, and grant trial balance only after email verification (or require the code when `INITIAL_USER_BALANCE > 0`, not only when `REQUIRE_EMAIL_VERIFICATION` is set).

### A06 · P2 · Open redirect after OAuth via Host / X-Forwarded-Host

**Preconditions**: `PUBLIC_URL`/`GATEWAY_PUBLIC_URL` empty (`.env:15-16` and the containers confirm both empty); attacker can send a crafted `X-Forwarded-Host` or `Host` (nginx forwards client XFH verbatim: `deploy/nginx-compose.conf:16-19`).

**Code path**: `ResolvePublicBaseURL` (`admin_handler.go:142-168`) prefers config/env, then `X-Forwarded-Proto` + `X-Forwarded-Host`/`Request.Host`; `oauth_handler.go:89-103` builds `base + "/?oauth_error=...#/login"` and `c.Redirect(302, target)`.

**Evidence (live)**:
```
GET /api/v1/auth/oauth/google/callback?code=abc&state=bad  -H 'X-Forwarded-Host: evil.example.com'
-> 302 Location: http://evil.example.com/?oauth_error=%E7%B3%BB%E7%BB%9F...
Host: evil.example.com -> identical 302 to evil.example.com
```
The same function is also returned by `/api/v1/public/status`, `HandleMCPInfo` and `GetMCPSettings` as `public_url`/endpoints, and it feeds the OAuth `redirect_uri` (`oauth_handler.go:30-32`) — provider-side registration blocks the code-leak variant, so this is an open redirect rather than token theft.

**Suggested fix**: when the public URL is not configured, validate the derived host against a configured allowlist (or the request's own server names) before using it in a `Location`; for the SPA result use a relative redirect.

### A07 · P2 · Session token accepted from `?token=`

**Code path**: `auth.go:59-71` order: `Authorization: Bearer` → `x-admin-token` → session cookie (CSRF-checked) → `c.Query("token")` (no CSRF check, no cookie). It applies to every route behind `AdminAuthMiddleware`, including `/api/v1/admin/**` writes.

**Evidence (live)**: `GET /api/v1/auth/me?token=<token>` → 200; `GET /api/v1/admin/users?token=<token>` → 200.

**Impact**: a 7-day admin session credential can be transported in a URL, where it leaks into nginx access logs (the application log redacts `token`, but nginx does not), browser history, and `Referer` to third-party origins that CSP still allows (`fonts.googleapis.com`); a shared link is also a login-equivalent for anyone who receives it.

**Suggested fix**: remove query-string token support (keep header/cookie), or scope it to a dedicated CLI endpoint with short-lived tokens.

### A08 · P2 · MCP SSE `sessionId` is a bearer credential and is logged verbatim

**Code path**: `mcp_sse.go:104-114` stores `apiKey: callerKey` in the in-process session and emits `/mcp/messages?sessionId=<sid>`; `mcp_handler.go:71-79` — if the POST has no credential but carries `?sessionId=`, it adopts `sess.apiKey` and continues. `internal/middleware/trace.go:106-120` redacts `key|apikey|token|secret|password` only.

**Evidence (probe + live)**:
```
probe: POST /mcp/messages?sessionId=mcp-0123456789abcdef (no api key)
 -> 200 and the admin-only `list_tables` tool executed (adopted admin identity)
live:  docker logs airoute-1
 -> "path":"/mcp/messages?sessionId=mcp-2acd2e9a04711f6b"      (unredacted)
```

**Impact**: anyone who can read gateway/system logs, a log pipeline, or the admin log console while a session is open can impersonate that key owner's MCP identity — including the admin-gated SQL tools. Session ids are 64-bit random and only live as long as the SSE stream, which limits but does not remove the exposure.

**Suggested fix**: require the API key on every `/mcp/messages` POST (drop the session adoption), or bind session ids to a per-client secret that is never logged; add `sessionid` to `sanitizeURLQuery`.

### A09 · P2 · Built-in root admin can be taken over via password reset

**Code path**: `user_admin_handler.go:222-258` (`ResetUserPassword`) has no root-admin guard, unlike `DeleteUser` (`:201-205`), `UpdateUserStatus` (`:39-43`) and `UpdateUserRole` (`:179-183`), which all refuse to touch `admin`/`GATEWAY_ADMIN_USER`. Any `role=="admin"` session can reset any other admin's password, including root.

**Evidence (probe, PASS)**:
```
DeleteUser(admin)        -> 400 {"error":"系统初始管理员账号不允许删除"}
UpdateUserStatus(admin)  -> 400 {"error":"系统初始管理员账号不允许被锁定"}
ResetUserPassword(admin) -> 200 {"message":"已成功重置用户 [admin] 的密码"}
root token issued for the new password: true
```

**Impact**: a second administrator can silently seize the built-in super-admin (owner attribution/audit is lost, the real root is locked out). Lower severity than A01 because both accounts already hold `role=="admin"`, but it breaks the explicit invariant the other handlers enforce.

**Suggested fix**: apply the same root-admin check to `ResetUserPassword` (root may only be reset by itself / from config with `GATEWAY_ADMIN_RESET`).

### A10 · P2 · Self-service key `tenant_id` spoof via key name

**Code path**: `user_key_rules.go:44-57`: `tenantID := req.TenantID; if "" { tenantID = req.Name }` — and `CreateUserKey` binds `name` straight from the request body. The admin/rule path (`key_creation.go:109-127`) explicitly treats tenant as server-owned and forces `user.Username`.

**Evidence (probe, PASS)**: `PlanUserKey(user=alice, req.Name="admin")` → `tenant_id="admin"`.

**Impact**: a tenant can label its traffic as another tenant (`admin`) in `usage_logs`, dashboards, exports and any tenant-grouped report; it also interacts with the legacy key→user join in `repo_keys.go:206` (`user_id=0 AND tenant_id=username`).

**Suggested fix**: derive `TenantID` from the authenticated user for non-admins (as `PlanCreate` does); stop accepting tenant from `name`.

### A11 · P2 · Self-service keys default to unlimited RPM

**Code path**: `user_key_rules.go:34-47` keeps `rpm = req.RPM` (0 when omitted; `ratelimit.go:192` treats `RPM<=0` as unlimited) and defaults only TPM to 100000. The registration-issued key uses `RPM:60, TPM:100000` (`identity_handler.go:95-104`), and the admin path passes through whatever the operator set.

**Impact**: a tenant can mint additional keys with no RPM/TPM gate, sidestepping the per-key limit the platform intended (and any aggregate protection that assumes keys are capped).

**Suggested fix**: default self-service keys to the same 60 RPM as the registration key (explicit 0 only for admins / an "inherit" sentinel).

### A12 · P2 · Cleartext API keys at rest (api_keys and usage_logs)

**Code path**: `APIKeyRecord.Key` is stored verbatim (`repo_keys.go`, uniqueness/lookup by equality); data-plane handlers put the raw key into the audit row: `internal/api/common.go:178 APIKey: getRequestAPIKey(c)`; masking is applied only when rendering (`admin_logs.go:190-208`, `middleware/trace.go:171-181`). `DeleteUser`/revocation are fine, but every historical `usage_logs` row keeps the secret.

**Impact**: any read access to the database (backup copy, read replica, a future SQL-injection bug — note MCP ships SQL tools, albeit allowlisted) yields directly usable credentials, and the exposure is multiplied by log retention.

**Suggested fix**: store `HMAC-SHA256(key)` (plus a short display prefix) and look up by hash; persist only the masked value in `usage_logs` (the key id/prefix suffices for audit).

### A13 · P2 · Login timing and locked-state oracle

**Code path**: `account_handler.go:35-51`: unknown user → immediate generic 401 with no bcrypt work; existing user → `bcrypt.CompareHashAndPassword`; `locked` accounts short-circuit at `:42-45` with 403 before the password is verified.

**Evidence (live, 3 samples each)**:
```
existing "admin" + wrong password : 0.282095 / 0.276801 / 0.277717 s
nonexistent user  + wrong password: 0.002850 / 0.002747 / 0.002683 s   (~100x)
locked account                     : 403 without password check
```

**Impact**: reliable username/email enumeration and account-state disclosure; combined with A04 it enables targeted guessing.

**Suggested fix**: run a fixed dummy bcrypt comparison for unknown users, and return the same generic 401 for locked accounts at login (the lock message can stay on authenticated endpoints).

### A14 · P2 · Admin API reachable through the default nginx vhost

**Code/config**: `deploy/nginx-compose.conf:44-50` blocks `^/(api/v1/admin|app)` only inside the `api.*` server block; the catch-all `listen 80 default_server; server_name localhost _;` (`:129-130`) proxies `location /` to the gateway without that restriction, and `internal/api/router.go:143-242` registers `/api/v1/admin/**` for every Host.

**Evidence (live)**: `GET http://127.0.0.1:8080/api/v1/admin/users?token=<token>` → 200 through `airoute-lb` (published port), i.e. the request landed on the default vhost.

**Impact**: the documented "control plane restricted to the private management domain" property is not enforced: the admin API answers on any Host/IP on the published LB port. This is also what makes A01 reachable unauthenticated from the public port.

**Suggested fix**: enforce the management-host restriction in the application (configured admin host allowlist → 404/403 otherwise) or duplicate the `location ~* ^/(api/v1/admin|app)` block into the default vhost; do not rely on domain separation alone.

### A15 · P2 · Console still writes the raw session token to `localStorage`

**Code**: `web/src/components/LoginModal.jsx:80,154` and `AuthPage.jsx:98,179` do `localStorage.setItem('airoute_token', data.data.token)`. `web/src/api/session.js:1-10` states the invariant ("The session token itself lives in an HttpOnly cookie … never written"), `setStoredAuth` explicitly removes the legacy keys (`:39-47`), and `useConsoleSession.signIn` (`:65-74`) calls `setStoredAuth('', user)` right after `onLoginSuccess`, which deletes the key again — so the token survives only if that later call is skipped/fails (e.g. the `onLoginSuccess` callback throws, or a new login entry point is added).

**Impact**: contradicts the F18 HttpOnly design and leaves a 7-day admin credential in web storage that XSS/extensions can read along the failure path; also a latent regression risk as new auth entry points are added.

**Suggested fix**: delete the four `localStorage.setItem('airoute_token', …)` calls; rely on the HttpOnly cookie and `setStoredAuth` for profile hints.

### A16 · P2 · Tenant keys can enumerate channel inventory via MCP

**Code path**: `internal/api/mcp_tools_ops.go:16-79`: `airoute_cluster_status` requires only a valid active key (or `args["api_key"]`), then returns every channel's `name`, `type`, `status`, `breaker_status`, `priority`, `weight`. Only the SQL tools are admin-gated (`mcp_tools.go:188-300`, `callerIsAdmin`).

**Impact**: leaks upstream provider names/capacity/failover posture to every tenant; useful reconnaissance for abuse and for interpreting other tenants' latency.

**Suggested fix**: gate `airoute_cluster_status` on `callerIsAdmin` (return aggregate counts to tenants), consistent with the SQL tools.

### A17 · P2 · OAuth usernames can be pre-empted by local registration

**Code path**: real OAuth signup derives `username = "gh_"+<numeric GitHub id>` / `"gg_"+<Google sub>` (`oauth_handler.go:155-166`) and inserts it (`:227-273`). Local registration accepts arbitrary usernames (`identity_handler.go:36-43`), including `gh_<id>`; the insert then fails on the unique constraint and the callback answers 500 ("创建 OAuth 账号及初始密钥失败，请重试").

**Evidence (probe, PASS)**:
```
CreateOAuthUserWithAPIKey with a pre-empted username -> constraint failed: UNIQUE constraint failed: users.username (2067)
```

**Impact**: anyone can pre-register `gh_<victim's GitHub id>` (ids are public) and permanently block that person's OAuth login (DoS), or squat plausible ids; the OAuth identity itself is still safe (lookup is by `(provider, subject)`), so this is availability, not takeover.

**Suggested fix**: on username collision, generate a free suffix (`gh_<id>_2`) instead of failing, and reserve the `gh_`/`gg_` prefixes against local registration.

---

## Prior findings checked (no regressions found)

| Prior ID | Fix | Verified by |
|---|---|---|
| F01 (OAuth display-name identity merge) | Stable `(provider, subject)`: GitHub numeric `id` (`oauth_providers.go:70`), Google `id`/sub (`:117-128`); `GetUserByOAuthIdentity`, no email auto-link (409 instead, `oauth_handler.go:216-226`) | Baseline tests #4 `TestAuditReviewGoogleDisplayNameDoesNotIdentifyAccount`, `TestOAuthStableSubjectAndCrossReplicaCookie` — PASS. Residual: A17. |
| F02 (password change/reset does not revoke sessions) | Token carries HMAC credential tag over `user.ID|PasswordHash` (`token.go:138-143`); middleware re-checks every request (`auth.go:101-109`), role change and lock also re-checked (`:97-109`); change-password clears cookies (`account_handler.go:185`) | `TestAuditReviewPasswordChangeRevokesOldSession`, `TestPasswordResetRevokesSessionsAcrossHandlers`, `TestAuthRejectsDeletedAndDemotedUsers` — PASS. Residual (by design, documented at `account_handler.go:189-190`): logout cannot revoke a copied stateless token. |
| F18 (OAuth exchange code per-process; state key from stale secret) | Exchange code removed; callback sets HttpOnly cookie; state signed with a key derived *per call* from the current shared secret (`oauth_state.go:29-33`); DB secret init is atomic (`repository.go:156-164` `ON CONFLICT DO NOTHING`) | `TestOAuthStateUsesCurrentClusterSecret`, `TestBindOAuthStateRequiresMatchingCookie`, `TestOAuthStableSubjectAndCrossReplicaCookie` — PASS. Residual: open redirect A06 (the public URL resolver it now depends on), and A15 (HttpOnly invariant undermined client-side). |
| Open-dev-mode gate | `AuthMiddleware` only opens when no key was ever registered, and the flag is refreshed by hot reload | `TestAuthMiddlewareOpenModeRequiresNoKeyEverRegistered`, `TestAuthMiddlewareOpenModeClosesOnceAKeyIsRegistered`, `TestDeletingLastKeyKeepsAuthenticationEnforced` — PASS. Residual: the startup failure path feeding the flag is A03. |
| API key client-owned fields | `CreateAPIKeyRequest` is an explicit whitelist, tenant/usage/status server-owned for non-admins | `TestCreateAPIKeyIgnoresClientOwnedFields` — PASS. Residual: self-service tenant derivation A10 / RPM default A11. |

## Notes / out of scope

- Probe sources are archived in the worktree only (`/tmp/airoute-audit-auth/internal/{api,controlplane}/*probe_test.go`); the worktree was removed after the review, so the repository contains only this report. To re-run them, recreate the worktree at `64e749a`, copy `web/dist`, and restore the probe files from this report's evidence excerpts.
- Billing correctness of the MCP settlement path (whether the async `settleCharges` row attributes MCP usage to a wallet) is left to the billing reviewer; A02 concerns the missing **authorization** gate, which is independent of settlement.
- No live OAuth provider exchange was performed (providers are unconfigured in this deployment: `/api/v1/public/status` reports `oauth_github_enabled:false`, `oauth_google_enabled:false`); OAuth findings A06/A17 are proven via live redirect/DB probes and unit probes.
- The `.env`/compose values are evidence for reachability and severity (A01/A05/A14); the deliverable does not recommend changing them as part of this code review — they need deliberate operator rotation.
