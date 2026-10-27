# Web Console Review — 2026-10-27

- **Date**: 2026-10-27 (Asia/Shanghai)
- **HEAD**: `64e749a` — *fix: resolve functional audit findings across auth billing and workflows* (the remediation commit)
- **Scope**: `web/src/**` (App.jsx, components/**, hooks/**, api/**, lib/**, i18n.js, config.js), `web/package.json`, `web/vite.config.js`, `web/vitest.config.js`, `web/eslint.config.js`, existing web tests. No file under the repository was modified except this report; probes ran from `/tmp/webprobe` importing the repo modules read-only.
- **Method**: full read of the request layer, all hooks, App.jsx and the data-heavy components; targeted greps for HTML sinks, timers, request/abort patterns, prop-initialised state, credential storage, i18n key coverage (script), money/date formatting. One behavioural probe executed with the repo's own Vitest/React Testing Library (see W05).

## Commands run (verbatim result summary)

| Command | Result |
|---|---|
| `cd web && npm run lint` (`eslint src --max-warnings 0`) | **exit 0**, no output — zero errors, zero warnings |
| `cd web && npm test` (`vitest run`) | **38 files / 507 tests passed**, duration 17.48 s (exit 0) |
| `cd web && npm run build` (`vite build`) | **✓ built in 680 ms**, exit 0 (`web/dist` is gitignored) |
| export probe `/tmp/webprobe/export.probe.js` (run through the repo's Vitest, jsdom, `@web` alias to `web/src`) | **1 passed, 1 failed** — the pass confirms the `truncated` false positive at exactly 5000 rows (W05); the failure refutes my initial hypothesis that rows are silently dropped (see W05) |

Baseline is green, so every finding below is something the existing 507 tests do not cover.

## Prior remediation (F07/F08/F10/F11/F12/F13/F14/F17) — verification

| Fix | Verdict | Evidence in `web/src` |
|---|---|---|
| **F07** model mapping editor | **Fixed** | [ModelMappingEditor.jsx:52-78](/home/ifnodoraemon/myagent/airoute/web/src/components/ModelMappingEditor.jsx:52) renders structured `{from,to}` rows (no colon/comma parsing); [useChannelActions.js:180-199](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useChannelActions.js:180) builds `model_mapping` as an object and rejects empty/duplicate sources. `ModelMappingEditor.test.jsx` (5) + `useChannelActions.test.js` pass. |
| **F08** channel probe abort/staleness | **Fixed** | [useChannelActions.js:24-38](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useChannelActions.js:24) `cancelProbe` is wired into `setNewChannel` (every field edit), `setModalVisibility`, `reset()`; unmount abort at :34; late responses are discarded by `controller.signal.aborted \|\| probeRef.current !== controller` (:63, :101). Switching edit target goes through `setNewChannel` (:235). |
| **F10** log export completeness | **Mostly fixed, boundary bug remains** | [useLogsData.js:199-216](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useLogsData.js:199) throws on any non-2xx/`code!==0` page and on session change; [LogsView.jsx:244-256](/home/ifnodoraemon/myagent/airoute/web/src/components/LogsView.jsx:244) aborts the CSV and shows the failure. Remaining defect: W05. |
| **F11** playground SSE errors | **Fixed in PlaygroundView, NOT applied to AiCopilot** | [PlaygroundView.jsx:545-556](/home/ifnodoraemon/myagent/airoute/web/src/components/PlaygroundView.jsx:545) throws on `parsed.error`, outer catch surfaces it (:604-612), reader released in `finally` (:575-578). The same pattern in [AiCopilot.jsx:649-697](/home/ifnodoraemon/myagent/airoute/web/src/components/AiCopilot.jsx:649) still swallows errors — W02. |
| **F12** dashboard selected-key scoping | **Fixed, with a login-path gap** | [DashboardView.jsx:87-110](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:87) builds every request from `selectedKey`; monotonic `dashboardRequest` guard (:103, :128); `statsKey` gates both cards and derived rows (:195-196); 15 s refresh re-runs the same scoped loader (:119-129). Server honours `key` ([admin_overview.go:20,81,113](/home/ifnodoraemon/myagent/airoute/internal/controlplane/admin_overview.go:20)). Logout/expiry teardown is correct; the auth-page login path is the gap (W01). |
| **F13** announcement modal load/draft | **Fixed** | [AnnouncementModal.jsx:23-46](/home/ifnodoraemon/myagent/airoute/web/src/components/AnnouncementModal.jsx:23) aborts on close, `fieldset disabled={loading \|\| loadError \|\| submitting}` (:149), submit guard (:52), retry re-runs the loader (:148), stale writes blocked by `signal.aborted`. |
| **F14** model route modal unsaved guard | **Fixed** | [ModelRouteModal.jsx:99-102](/home/ifnodoraemon/myagent/airoute/web/src/components/ModelRouteModal.jsx:99) routes Escape/backdrop/X/Cancel through `requestClose`; save success closes directly (:257-260); `ModelRouteModal.test.jsx` (`guards unsaved model routing changes when Escape is pressed`) passes. |
| **F17** video polling cancellation | **Fixed** | [PlaygroundView.jsx:758-816](/home/ifnodoraemon/myagent/airoute/web/src/components/PlaygroundView.jsx:758) create/poll share one `AbortSignal`; the 3 s wait clears its timer on abort, terminal statuses stop, HTTP errors throw once, unmount aborts (:135). |

## Findings

| ID | Sev | Location | Title | Confirmed? | Impact |
|---|---|---|---|---|---|
| W01 | **P2** | [App.jsx:678-683](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:678) (+ dead duplicate at [:1691-1696](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:1691)) | The auth-page login path skips the tenant teardown that every other login path performs | STATIC | On a shared browser, the previous account's keys/logs/stats and its log filters remain in state and can be written back by its in-flight poll |
| W02 | **P1** | [AiCopilot.jsx:649-651](/home/ifnodoraemon/myagent/airoute/web/src/components/AiCopilot.jsx:649), [:694-697](/home/ifnodoraemon/myagent/airoute/web/src/components/AiCopilot.jsx:694), [:772-782](/home/ifnodoraemon/myagent/airoute/web/src/components/AiCopilot.jsx:772) | AI Copilot swallows streamed business errors and silently substitutes canned fallback text | STATIC | Quota/auth/429/5xx failures surface as an empty bubble or fake answer — the diagnosis tool cannot diagnose itself |
| W03 | **P2** | [DashboardView.jsx:568-574](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:568), [:1478-1497](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:1478) | Provider benchmark fabricates `160 ms` latency and `100.0%` SLA for channels with no samples | STATIC | Unused/never-probed channels rank as fast & perfect; operators chase wrong providers |
| W04 | **P2** | [DashboardView.jsx:513-535](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:513), rendered [:1560-1576](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:1560) | Error budget is computed against a 2 % error rate while labelled a 99.9 % (three-nines) target | STATIC | Budget bar reads "healthy" at ~20× the allowed error rate; SLA burn-down is unusable |
| W05 | **P2** | [useLogsData.js:207-216](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useLogsData.js:207), [LogsView.jsx:331-341](/home/ifnodoraemon/myagent/airoute/web/src/components/LogsView.jsx:331) | CSV export reports "exceeds export cap" when the result set is exactly the cap (5000) | **CONFIRMED** (probe) | Operator is told the export is truncated and asked to narrow the range although the CSV is complete |
| W06 | **P2** | [SupplierReconciliationView.jsx:84-115](/home/ifnodoraemon/myagent/airoute/web/src/components/SupplierReconciliationView.jsx:84) | Reconciliation fetches have no abort or request sequencing; an older response overwrites a newer filter's report | STATIC | Switching "today/7d/month" quickly can leave the table showing another time window's totals |
| W07 | **P2** | [ServiceStatus.jsx:65-87](/home/ifnodoraemon/myagent/airoute/web/src/components/ServiceStatus.jsx:65), [App.jsx:719](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:719) | Public status page calls the private console loader every 30 s, and performs its requests inside a `setState` updater | STATIC | 4 unauthorised `/admin/*` requests per visitor per 30 s; duplicate requests when React replays the updater |
| W08 | **P2** | [LoginModal.jsx:80-81](/home/ifnodoraemon/myagent/airoute/web/src/components/LoginModal.jsx:80), [:154-155](/home/ifnodoraemon/myagent/airoute/web/src/components/LoginModal.jsx:154), [AuthPage.jsx:98-99](/home/ifnodoraemon/myagent/airoute/web/src/components/AuthPage.jsx:98), [:179-180](/home/ifnodoraemon/myagent/airoute/web/src/components/AuthPage.jsx:179) | Login/register still write the raw session token to `localStorage` | STATIC | Contradicts the cookie-only invariant in `api/session.js`; a credential briefly lands at rest in web storage |
| W09 | **P2** | [App.jsx:147-158](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:147), [useModalA11y.js:262-283](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useModalA11y.js:262) | `Ctrl/⌘+K` while a dirty form modal is open: one Escape closes the palette **and** the form, raising a discard prompt | STATIC | Closing the palette can silently prompt to discard (and on confirm, destroy) unsaved provider/route edits |
| W10 | **P2** | [App.jsx:343-350](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:343), [useUrlFilters.js:241-255](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useUrlFilters.js:241) | Visiting the landing/status/auth page strips `logs.*` / `channels.*` from the URL, and the filter hook never rewrites them | STATIC | The "shareable filtered audit link" silently loses its filters; in-memory filters and URL disagree |
| W11 | **P2** | [ChannelModal.jsx:200-216](/home/ifnodoraemon/myagent/airoute/web/src/components/ChannelModal.jsx:200), [useChannelActions.js:233-249](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useChannelActions.js:233) | Upstream provider API keys are round-tripped in plaintext into the DOM (and re-submitted on save) | STATIC | Any admin session / console XSS can read every upstream credential; masked lists are a UI illusion only |
| W12 | **P3** | [AnnouncementBanner.jsx:10-44](/home/ifnodoraemon/myagent/airoute/web/src/components/AnnouncementBanner.jsx:10), [App.jsx:1742-1750](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:1742) | Banner keeps the old announcement on non-2xx and only refreshes on a 60 s poll after publishing | STATIC | A withdrawn/urgent notice stays visible (up to a minute after an admin change) |

Severity counts: **1 × P1, 11 × P2, 1 × P3** (13 findings, 1 confirmed by probe).

---

## W01 · P2 · The auth-page login path skips the tenant teardown

**Trigger.** Shared workstation: account A is working in the console → clicks the sidebar logo (returns to the portal) → "登录" (lands on `#/login`, the `AuthPage`) → account B signs in there without A having logged out.

**Code path.** Every login path is supposed to invalidate the previous tenant's data through `resetWorkspaceState` ([App.jsx:270-287](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:270)). It is called by the landing-page handler ([App.jsx:653](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:653)), the public-status handler ([App.jsx:732](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:732)) and logout ([App.jsx:296](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:296)) — but **not** by the auth-page handler ([App.jsx:678-683](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:678)), which only calls `signIn` + `fetchData` + `fetchLogs`. `signIn` and `endSession` deliberately touch only auth state ([useConsoleSession.js:46-52](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useConsoleSession.js:46), [:97-103](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useConsoleSession.js:97)).

**Evidence.** `grep -n resetWorkspaceState App.jsx` → only 270, 296, 653, 732. Because `App` owns the domain hooks and never unmounts, A's `channels`/`keys`/`stats`/`recentLogs`/`selectedKeyIds`/`dashboardSelectedKey` and the URL-bound `logFilters` (including `key_id`) survive into B's console; B's `fetchData` only replaces the slices whose envelope succeeds ([useConsoleData.js:286-306](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useConsoleData.js:286)), and `useConsoleData.generationRef` is never bumped, so an A-session poll still in the air can also resolve and write A's rows after B has signed in.

Two mitigating facts, stated for accuracy: the **session-expiry** path is *not* affected — `endSession` switches to the landing view, and the login dialog the user then sees is the landing one, which does reset; and the console shell's own `LoginModal` handler ([:1691-1696](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:1691)) is unreachable dead code, because `showLoginModal` is only ever set to `true` in `onEnded` ([:123-130](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:123)) together with `viewMode='landing'`. That duplication is why the auth-page copy drifted.

**Suggested fix.** Move the teardown out of the individual handlers into the session layer (clear/reset inside `signIn`, or expose a `beforeSignIn` hook that resets the domain hooks), and delete the unreachable console-shell login dialog so the two copies cannot diverge again.

---

## W02 · P1 · AI Copilot swallows every streamed error that is not literally "stream error"

**Trigger.** Open the copilot (`Ctrl+J`), send a prompt while the selected API key is over quota, rate-limited, or the upstream returns a mid-stream error event.

**Code path.** [AiCopilot.jsx:649-651](/home/ifnodoraemon/myagent/airoute/web/src/components/AiCopilot.jsx:649):

```js
if (parsed.error && !parsed.choices) {
  throw new Error(parsed.error.message || 'stream error');
}
```

is caught 45 lines later by

```js
} catch (e) {
  if (e && /stream error/i.test(e.message || '')) {
    if (showToast) showToast(e.message, 'error');
  }
}   // AiCopilot.jsx:694-697
```

A gateway/data-plane error is `{"error":{"message":"insufficient quota …"}}` (the two shapes are documented in [client.js:71-103](/home/ifnodoraemon/myagent/airoute/web/src/api/client.js:71)), so the thrown message is the real text and the regex never matches: the error is dropped, the read loop continues, `fullContent` stays empty, `streamedToolCalls` stays empty, `shouldContinue=false`, and the assistant bubble stays blank with no toast. HTTP-level failures take the other silent path: the outer `catch` (:772-782) substitutes `executeLocalHarnessDiagnostic()` text and only sets `isFallback: !usedLiveLlm`.

- Before any stream starts (401/429/5xx on the first `fetch`, or a ReAct iteration that throws after a completed one) `usedLiveLlm` is `false`, so the amber "离线本地模板（非实时）" badge at :1022-1031 is shown — the fallback *is* labelled, but the actual HTTP status and gateway message are still discarded (no toast).
- After a stream has started, `usedLiveLlm` is `true`, so a later iteration that fails renders the same canned text **without** the badge, i.e. as a normal model answer.

No `reader.cancel()`/`releaseLock()` exists anywhere in this file (grep), unlike the F11 fix in PlaygroundView.

**Evidence.** Code read of the complete `while (!done)` block (:636-712) and the outer catch (:772-800); no test exercises this path (`copilotHarness.test.js` covers only the local harness).

**Suggested fix.** Mirror PlaygroundView: rethrow `parsed.error` out of the read loop, keep the partial content, release the reader in `finally`, and surface *every* error (toast + inline error bubble) instead of regex-matching its text; only use the local fallback when `!usedLiveLlm`, and label it as a fallback.

---

## W03 · P2 · Provider latency/SLA benchmark fabricates numbers

**Trigger.** Open the dashboard with at least one configured channel that has no traffic in the last page of logs.

**Code path.** [DashboardView.jsx:568-574](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:568):

```js
const avgTtft = p.ttfts.length ? … : (p.latencyMs || 160);
const successRate = p.totalCalls > 0 ? … : '100.0';
```

`p.latencyMs` is `null` unless a probe was run, so an unmeasured channel is displayed as `160 ms` with `SLA 100.0%` in the "Provider Latency & SLA Benchmark" panel (:1478-1497), and it participates in the sort (:580) and the bar scale. The executive report has the same pattern: `stats.avg_ttft_ms || 180` (:587) prints "平均 180ms" even when `percentiles.sampled === 0`.

**Evidence.** Read of the memo and its render sites; the sibling percentile code was already hardened for this (`fmtPct` → "无样本"), which shows the intent.

**Suggested fix.** Return `null` for `avgTtft`/`successRate` when there are no samples and render "n/a · 无样本" (and omit the row from the ranking), matching `fmtPct`.

---

## W04 · P2 · Error budget uses 20× the allowed error rate

**Trigger.** Gateway serves 1 000 requests with 15 failures (1.5 %).

**Code path.** [DashboardView.jsx:513-535](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:513): `targetSla = 99.9`, `allowedErrorRate = 0.001`, then `allowedErrors = Math.max(1, Math.ceil(total * allowedErrorRate * 20))` — 20 errors instead of 1 for 1 000 requests. `remainingBudgetPct` is computed against that inflated denominator and rendered at :1569-1576 as "错误预算剩余 X %" next to the 99.9 % target. The `* 20` has no comment and no other use.

**Evidence.** Arithmetic on the code path; the UI string pairs the bar with `targetSla` (:1560-1576).

**Suggested fix.** Drop the `* 20` (or introduce an explicit, labelled "burn-rate" multiplier and compute the budget window correctly), and derive `actualSla` from the same denominator the label claims.

---

## W05 · P2 · Export truncation flag is wrong when the result set is exactly 5000 rows — **CONFIRMED**

**Trigger.** Active log filters match exactly `EXPORT_MAX_ROWS` (5 000) rows, then "Export CSV" from the logs overflow menu.

**Code path.** [useLogsData.js:199-216](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useLogsData.js:199): each page appends 200 rows, then

```js
if (batch.length < EXPORT_PAGE_SIZE) break;   // complete result set
if (collected.length >= maxRows) { truncated = true; break; }
```

With 5 000 matching rows the loop reaches `collected === 5000` after page 25 (a multiple of 200), so the *first* condition is false and the cap check marks the result truncated — without ever asking for offset 5000 to discover there is nothing more. [LogsView.jsx:331-341](/home/ifnodoraemon/myagent/airoute/web/src/components/LogsView.jsx:331) then shows "筛选结果超过导出上限，已导出前 5000 条；请缩小时间范围后分段导出" although the file is complete.

**Evidence (probe).** `/tmp/webprobe/export.probe.js`, run with the repo's Vitest (jsdom, hook imported from `web/src`):

```
total=5100 -> rows 5000 truncated true pages 25
total=5000 -> rows 5000 truncated true pages 25
✓ reports truncated=true even when the result set is exactly the cap
× reports truncated=true when a partial final page pushes past the 5000-row cap
   AssertionError: expected true to be false
```

Reading the two results honestly:

- **Confirmed defect**: with exactly 5 000 matching rows the call returns `{ rows: 5000, truncated: true }` — 25 successful pages, no probe of offset 5000 — so the UI warns about a truncated export that is in fact complete.
- **Hypothesis refuted**: I expected a partial final page (5 000–5 199 rows) to be silently dropped with `truncated === false`; the probe shows `truncated === true` and the loop stopping on the page boundary, i.e. no rows are lost in that range. The failing assertion above is that discarded hypothesis, not a shipped defect.

The general form of the bug is that `truncated` is inferred from `collected.length >= maxRows` without ever confirming that more rows exist, and `rows: collected.slice(0, maxRows)` would silently trim if `maxRows` were not a multiple of the page size.

**Suggested fix.** After hitting the cap, probe one extra row (`limit=1&offset=collected.length`) and set `truncated` only if it returns a row; or use the server's `total` when `count` is enabled.

---

## W06 · P2 · Reconciliation report races its own filters

**Trigger.** Open "供应商对账" and click through the time presets ("今天" → "近 7 天" → "本月") or change the channel/model filter quickly on a slow link.

**Code path.** [SupplierReconciliationView.jsx:84-115](/home/ifnodoraemon/myagent/airoute/web/src/components/SupplierReconciliationView.jsx:84): `fetchReconciliation` sets `loading`, awaits `adminFetch`, then unconditionally `setReport(data.data)`. The driving effect (:114-116) depends on `timePreset/channelFilter/modelFilter`, so each click starts a new request while the previous one is still in flight; neither an `AbortController` nor a request-generation counter exists (contrast `DashboardView`'s `dashboardRequest` guard and `useLogsData`'s `requestRef`). Errors are also toasted with severity `warning` for genuine failures (:103, :106).

**Evidence.** Full read of the hook block; grep for `AbortController`/`request` in the file returns nothing.

**Suggested fix.** Capture a monotonically increasing request id (or one `AbortController` per fetch) and drop the response when it is no longer the latest, exactly like `useLogsData.fetchLogs`.

---

## W07 · P2 · Public status page polls private admin endpoints; requests inside a state updater

**Trigger.** Open the public status page (`#/status`) as an anonymous visitor and leave it open for a minute.

**Code path.** [ServiceStatus.jsx:65-87](/home/ifnodoraemon/myagent/airoute/web/src/components/ServiceStatus.jsx:65):

```js
const timer = setInterval(() => {
  setCountdown((prev) => {
    if (prev <= 1) {
      fetchPublicStatus();
      if (onRefresh) onRefresh();   // side effects inside an updater
      return 30;
    }
    return prev - 1;
  });
}, 1000);
```

Two defects: (1) state updaters must be pure — the app mounts under `React.StrictMode` ([main.jsx:8-10](/home/ifnodoraemon/myagent/airoute/web/src/main.jsx:8)), so in development this fires duplicate `/api/v1/public/status` *and* duplicate `onRefresh` requests, and any replayed update does the same in production; (2) `onRefresh` is wired to the console loader ([App.jsx:719](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:719) → `useConsoleData.refresh`), which issues `/admin/keys`, `/admin/stats/overview`, `/admin/models`, `/admin/pricing` ([useConsoleData.js:271-278](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useConsoleData.js:271)). An anonymous visitor therefore generates four 401s every 30 s, and a logged-in visitor's status page silently refreshes console data.

**Evidence.** Code read; `handleUnauthorized` explicitly excludes non-console views ([App.jsx:255-260](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:255)), so the 401s are pure noise.

**Suggested fix.** Drive the countdown with a pure decrement and move the fetch into a separate `useEffect` (or compute the deadline from a timestamp), and pass a status-local loader for `onRefresh` instead of the console tenant loader (or gate it on an authenticated session).

---

## W08 · P2 · Login/register still persist the session token in `localStorage`

**Trigger.** Password login or self-registration.

**Code path.** Four sites: [LoginModal.jsx:80-81](/home/ifnodoraemon/myagent/airoute/web/src/components/LoginModal.jsx:80) (login) and :154-155 (register), [AuthPage.jsx:98-99](/home/ifnodoraemon/myagent/airoute/web/src/components/AuthPage.jsx:98) and :179-180. Each does `localStorage.setItem('airoute_token', data.data.token)` before calling `onLoginSuccess(token, user)`; `signIn` → `setStoredAuth` removes the legacy keys again ([session.js:37-46](/home/ifnodoraemon/myagent/airoute/web/src/api/session.js:37)). Meanwhile `session.js` documents the opposite invariant ("only a 'logged in' marker and non-sensitive profile hints are kept in localStorage, so an XSS cannot read a long-lived credential") and `SystemLogConsole` still *reads* that key to build an SSE URL (:91).

**Evidence.** `grep -rn "localStorage.setItem"` over `web/src` shows exactly these four token writes; grep of `LEGACY_TOKEN_KEYS` shows only the removal path. The write is dead state that exists only for the window between the response and `signIn`.

**Suggested fix.** Delete the four `setItem` calls (the HttpOnly cookie is already set by the server, and `signIn` stores only the marker); if a bearer token is genuinely needed for non-browser clients, keep it in memory only.

---

## W09 · P2 · One Escape closes the command palette and the form behind it

**Trigger.** Open the channel/route/key form, type something, press `Ctrl/⌘+K` (the palette opens over the modal), then press `Escape`.

**Code path.** [App.jsx:147-158](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:147) registers a window `keydown` that opens the palette on `mod+k` and, on `Escape`, `setShowCommandPalette(false)` — with no check for an open modal. `CommandPalette` has its own `Escape` handler (:150), and every form modal registers another window listener via [useModalA11y.js:262-283](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useModalA11y.js:262). All three run for the same event (window listeners are not stopped by each other's `stopPropagation`), so the palette closes **and** the modal's `requestClose` runs → `useUnsavedGuard` sees `isDirty` and shows the discard dialog, which a confirm discards the draft.

**Evidence.** Code read; the guard chain is [ChannelModal.jsx:31-42](/home/ifnodoraemon/myagent/airoute/web/src/components/ChannelModal.jsx:31) / [ModelRouteModal.jsx:99-102](/home/ifnodoraemon/myagent/airoute/web/src/components/ModelRouteModal.jsx:99).

**Suggested fix.** Give the shell a single topmost-overlay notion: while `showCommandPalette` is true, the shell's Escape handler should consume the event (`e.preventDefault()`/`stopImmediatePropagation()`) and modal `useModalA11y` handlers should ignore it.

---

## W10 · P2 · Landing/status/auth navigation strips the filter query string

**Trigger.** Set log filters (or a search) → the URL becomes `?logs.q=…&logs.status=error#/logs` → click the sidebar logo / "返回门户首页" / the status badge → click back into the console.

**Code path.** [App.jsx:343-350](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:343):

```js
if (viewMode === 'status' || viewMode === 'landing' || viewMode === 'auth') {
  const cleanUrl = window.location.pathname + targetHash;
  if (window.location.hash !== targetHash || window.location.search) {
    window.history.replaceState(null, '', cleanUrl);   // drops ?logs.*
  }
}
```

`useUrlFilters` only writes the query string when its `filters` state changes ([useUrlFilters.js:268-270](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useUrlFilters.js:268)), so the parameters stay gone after returning to the console — the in-memory filters still apply, but the link/refresh no longer reproduces them. This defeats the documented purpose of the `logs.*`/`channels.*` URL state.

**Evidence.** Code read of both effects; empty-deps hash effect runs for every view-mode change.

**Suggested fix.** Strip only the OAuth/checkout parameters on the public views instead of rebuilding the whole URL, or have `useUrlFilters` re-persist its state when the URL is rewritten.

---

## W11 · P2 · Upstream provider API keys are round-tripped in plaintext

**Trigger.** Admin opens "编辑服务商" on any channel, or a console XSS runs.

**Code path.** `GET /api/v1/admin/channels` serialises the raw `api_key` ([admin_channels.go:67,83](/home/ifnodoraemon/myagent/airoute/internal/controlplane/admin_channels.go:67)); [useChannelActions.js:239](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useChannelActions.js:239) copies it into `newChannel.api_key`; [ChannelModal.jsx:200-216](/home/ifnodoraemon/myagent/airoute/web/src/components/ChannelModal.jsx:200) renders it in an input that is only visually masked (`type="password"`, with an explicit "显示明文" toggle at :208-215) and re-submits it unchanged. Channel payloads also reach `ChannelModal`/`DashboardView`/`CommandPalette` props, so the secret is present in many component trees.

**Evidence.** Code read of the handler struct and the edit path; no masking (`mask`/`****`) anywhere on the channel key (grep).

**Suggested fix.** Mask the key in the list response and accept a sentinel ("unchanged") on save; keep the plaintext only in the dedicated create/edit request the admin explicitly types.

---

## W12 · P3 · Announcement banner keeps stale notices and lags after publishing

**Trigger.** Backend returns 401/500 for `/api/v1/announcement` while a banner is already displayed; or an admin publishes a new announcement in the same console.

**Code path.** [AnnouncementBanner.jsx:10-44](/home/ifnodoraemon/myagent/airoute/web/src/components/AnnouncementBanner.jsx:10) only updates state when `res.ok`; any failure leaves the previously fetched announcement rendered (the catch is silent at :34-36). The 60 s poll is the only refresh path, and the console renders the banner without `onPublished` ([App.jsx:1742-1750](/home/ifnodoraemon/myagent/airoute/web/src/App.jsx:1742)) even though `AnnouncementModal` supports it.

**Evidence.** Code read; `AnnouncementModal` calls `onPublished?.()` on save ([AnnouncementModal.jsx:79](/home/ifnodoraemon/myagent/airoute/web/src/components/AnnouncementModal.jsx:79)).

**Suggested fix.** Clear `announcement` on non-OK responses (or keep it but show a stale marker), and pass an `onPublished` that re-triggers the banner fetch.

---

## Observations (not counted as findings)

- **i18n coverage is complete**: `zh` and `en` both define exactly 253 keys, no key used via `t.<name>` in components/hooks is missing (script over `web/src`, ignoring `t.<local var>` false positives). 50 keys are currently unused — dead weight, not a bug.
- **No XSS sink exists**: `dangerouslySetInnerHTML`/`innerHTML` appear nowhere; `marked` output is walked token-by-token into React nodes, and `safeMarkdownUrl` restricts link/image protocols ([MarkdownRenderer.jsx:19-30](/home/ifnodoraemon/myagent/airoute/web/src/components/MarkdownRenderer.jsx:19)).
- **CSV injection is handled** by `csvEscape` ([config.js:56-65](/home/ifnodoraemon/myagent/airoute/web/src/config.js:56)); money is rendered with `toFixed(2/4)` consistently; ids and key strings are used consistently in the key drill-down.
- **`useConsoleData.refresh` is correctly generation-guarded** against a logout racing an in-flight load ([useConsoleData.js:280-285](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useConsoleData.js:280)) — the same guarantee is what W01 is missing on the login path.
