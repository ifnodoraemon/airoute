# 前端控制台深层缺陷排查（2026-10-29，Bug 排查 H 域）

> **范围**：`web/src` 全部（`App.jsx`、`components/`、`hooks/`、`lib/`、`i18n.js`）与 `web/web.go`；对照后端真实响应形状（`internal/controlplane`、`internal/api/router.go`、`internal/storage`、`internal/router`、`internal/billing`）。
> **基线**：HEAD `2f43d28`，`web/src` 56 文件 / 685 用例全绿，`npm run lint` 0 warning。
> **本轮任务**：① 闭环 `docs/audit/2026-10-28-bughunt/04-web.md` §3 的 4 条待确认（T-1…T-4）；② 再找上一轮未覆盖的真实缺陷。
> **不查**：纯样式、死代码、上一轮 §5 已排除项、上一轮 §6 已修复项（本文不重复）。
> **方法**：静态推理 + 后端字段逐条对照 + jsdom/vitest 复现（无浏览器）。**未改任何生产代码**，只新增 7 个 `*__bugprobe.test.jsx`。
> **两阶段**：① 排查阶段只写报告 + `*__bugprobe.test.jsx` 探针（断言错误行为，保持全绿）；② 同日进入修复阶段，8 条全部修复（含可选的 T-5）、探针**翻转成回归测试**（断言正确行为），改动与验证见 **§7 修复记录**。
> **验证（修复后）**：`npm run lint` 0 warning；`npm test` **65 文件 / 710 用例全绿**。

---

## 1. 结论摘要

4 条待确认**全部闭环**：T-1、T-2、T-3 判为**真 bug**，T-4 判为**真缺陷但极低危**（并纠正了原假设：双击发不出两个 interval）。另外 4 条新发现集中在**AI Copilot 的数据契约**与**两条没有请求序号的刷新路径**上。

| 编号 | 类别 | 一句话 | 严重度 | 置信度 | 探针 |
|---|---|---|---|---|---|
| BUG-H01（原 T-1） | 状态残留 / 误删风险 | 日志页批量选择跨页、跨筛选残留，批量删除作用于**看不见的行** | P2 | 高 | `useLogsData.selection__bugprobe` |
| BUG-H02（原 T-2） | 竞态 / 金额显示 | 回到钱包标签页一次动作发**两个** `/user/wallet` 请求，且旧响应会覆盖新余额 | P2 | 高 | `WalletManagementView.refreshRace__bugprobe` |
| BUG-H03（原 T-3） | 白屏 / 缺兜底 | 全站无 ErrorBoundary：任一视图渲染抛错（含 lazy chunk 加载失败）⇒ 整树卸载、**真白屏** | P2（潜在） | 中高 | `App.errorBoundary__bugprobe` |
| BUG-H04（原 T-4） | 定时器泄漏 | 验证码倒计时 `setInterval` 未随卸载清理（双击产不出两个 interval，原假设不成立） | P3 | 高 | `AuthPage.countdownTimer__bugprobe` |
| BUG-H05 | 契约漂移（4 处） | AI Copilot「集群健康巡检」读错字段：熔断大小写、`cache_hit_rate` 不存在、CNY 当 USD、无样本编造 120ms | P2 | 高 | `copilotHarness.contract__bugprobe` |
| BUG-H06 | 契约漂移 | AI Copilot SPOF 审计读 `providers[].channel`（实际字段 `channel_name`）⇒ 主通道恒为 `default` | P2 | 高 | 同上文件 |
| BUG-H07 | 金额显示 / 计费口径 | 日志详情「计费核算公式」漏掉 prompt-cache 写入溢价项；Token 明细把 cache-write tokens 算作「未缓存输入」 | P2 | 高 | `LogDetailModal.cacheWrite__bugprobe` |
| BUG-H08 | 轮询竞态 | 控制台 8s 轮询 `useConsoleData.refresh` 无请求序号：旧轮询响应覆盖刚由操作触发的刷新，新建/修改的行消失到下一拍 | P2 | 高 | `useConsoleData.pollRace__bugprobe` |

**建议修复顺序**：BUG-H05/H06（同一文件、改字段与判定即可，直接影响 AI 给出的运维结论）→ BUG-H08（照 `SupplierReconciliationView` 的 seq 范式）→ BUG-H02（合并两个监听 + 加 seq）→ BUG-H07（公式补 cache-write 项）→ BUG-H01（翻页/换筛选清选择或加"跨页已选"提示）→ BUG-H03（顶层 ErrorBoundary + 重试）→ BUG-H04（timer 挪进 effect/清理）。

---

## 2. 上轮 §3 四条待确认的闭环

### T-1 = BUG-H01 日志批量选择跨页/跨筛选残留 —— **真 bug（P2，高）**

- **位置**：`web/src/hooks/useLogsData.js:44`（`selectedLogIds` 状态，仅在 `reset()` 与删除成功后清空，见 `:251`、`:285`、`:302`）；`web/src/App.jsx:649-655`（`handleLogPageChange` 只改 offset，不清选择）；`web/src/components/LogsView.jsx:813`（只要有选中就渲染批量操作条，不区分"选中行是否在当前页"）。
- **触发条件**：在有 50 行/页的审计日志页勾选任意行 → 翻到第 3 页（或改任意筛选，使结果集完全变化）→ 点击「批量删除」。
- **根因**：选择集与会话/结果集没有生命周期绑定。翻页与筛选变化只动了 `logOffset`/`filters`，`selectedLogIds` 原样保留；UI 也没有"已选 N 条，其中 M 条不在本页"的提示。
- **影响**：**误删**。操作员看到的表格里没有任何一行被勾选（`checked={selectedLogIds.includes(log.id)}` 对当前页全 false），但顶部条仍写着「已选中 2 条调用日志」并给出删除/清空按钮；点下去删除的是上一页/上一筛选里的行。已计费记录后端会拒删（`ErrChargedUsageLogImmutable`），但未计费的审计行会被真删。
- **证据（探针）**：`web/src/hooks/useLogsData.selection__bugprobe.test.jsx`（2 例）
  ```
  cd web && npx vitest run --configLoader runner src/hooks/useLogsData.selection__bugprobe.test.jsx
  ```
  1. 第 1 页选中 `[100,101]` → `fetchLogs({offset:100})` 后表格变成第 3 页 `[300,301,302]`，`selectedLogIds` 仍为 `[100,101]`；随后 `handleBatchDeleteLogs()` 发出的 POST body 正是 `{"ids":[100,101]}`（不可见行）。
  2. 换筛选后结果集变成 `[999]`，`selectedLogIds` 仍是 `[100]`。
- **修复建议**：翻页/改筛选时清空选择；若产品要"跨页累计选择"，则必须（a）在选择条上标出"其中 N 条不在当前结果集"，(b) 批量删除前二次确认里列出前几个 `id`。

### T-2 = BUG-H02 钱包页 focus + visibilitychange 双发 —— **真 bug（P2，高）**

- **位置**：`web/src/components/WalletManagementView.jsx:119-127`（同一个 `refresh` 同时挂在 `window.focus` 与 `document.visibilitychange`）；`fetchWallet` 在 `:66-94`，无 abort / 无请求序号。
- **触发条件**：切到别的浏览器标签页再切回钱包页（浏览器会同时派发 `focus` 与 `visibilitychange`）。
- **根因**：两个事件都代表"页面重新可见"，却被当成两个独立的刷新信号分别触发；而 `fetchWallet` 用 `setWalletData(prev => ({...prev, ...data.data}))` 无条件合并，谁最后返回谁生效。
- **影响**：① 一次"回到标签页"发 2 个同样的 `/api/v1/user/wallet`（放大后端压力，配合其它视图的 poll 更明显）；② 两个响应无序时，**旧快照覆盖新快照**，页面显示的是过期余额——用户正要据此决定是否充值。
- **证据（探针）**：`web/src/components/WalletManagementView.refreshRace__bugprobe.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/components/WalletManagementView.refreshRace__bugprobe.test.jsx
  ```
  初始加载展示 ¥100 → 派发 `focus` + `visibilitychange` → 断言 `/user/wallet` 被调用 **2 次**（挂载那次之外）→ 先让"较新"响应给 ¥300，再让"较旧"响应给 ¥200 ⇒ 最终屏幕是 **¥200**（旧值），¥300 消失。
- **修复建议**：只保留 `visibilitychange`（`focus` 是它的子集，且切换窗口不改变可见性时不该刷新）；`fetchWallet` 加 `requestSeqRef`（照 `SupplierReconciliationView.jsx:47-118`），并在卸载/下一次请求时 abort。

### T-3 = BUG-H03 全站无 ErrorBoundary —— **确认为真（P2，潜在，中高）**

- **位置**：全仓 `grep -r "ErrorBoundary|componentDidCatch|getDerivedStateFromError" web/src` → **0 命中**；`web/src/App.jsx:49-79` 有 30 个 `lazy()` 视图/弹窗，`App.jsx:1197` 起是它们的 `<Suspense>`（只处理等待，不处理抛错）。
- **触发条件（两种真实场景）**：
  1. **懒加载 chunk 失败**：部署切换瞬间旧 shell 引用已删除的 hashed chunk、或网络/CDN 抖动 ⇒ `import()` 拒绝 ⇒ lazy 组件在渲染期抛出（`web/web.go` 的 no-cache 只能减少、不能消除这种情况）；
  2. 某视图收到它没预料到的行形状（项目里大量 `.toFixed()/.map()` 直接作用于接口字段，例如 `SupplierReconciliationView.jsx:851 m.total_cost.toFixed(4)`、`ModelRoutesManager.jsx:608 price.prompt_price.toFixed(1)`），一旦字段缺失就抛。
- **根因**：没有错误边界。React 在渲染期抛错且无边界时**卸载整棵树**，页面留白。
- **影响**：真白屏——没有报错文案、没有重试入口，用户只能刷新（若原因是坏部署，刷新也无用）。这是"缺失的兜底能力"，不是某个具体数据错误，故标 P2 潜在而非 P1。
- **证据（探针）**：`web/src/App.errorBoundary__bugprobe.test.jsx`（把 `./components/LogsView` 换成渲染即抛的组件，等价于 chunk 失败）
  ```
  cd web && npx vitest run --configLoader runner src/App.errorBoundary__bugprobe.test.jsx
  ```
  进入 `#/logs`（管理员会话，数据加载已开始）后断言 `container.textContent === ''`：整页空白、无任何兜底 UI。
- **修复建议**：在 `App` 外层（`main.jsx`）加一个 class/函数 ErrorBoundary：捕获后渲染「控制台加载失败」+「重新加载」按钮 + 折叠的错误摘要；对 `Suspense`/lazy 再加一层"chunk 加载失败请刷新"提示（可用 `React.lazy` 的 catch + 重试）。

### T-4 = BUG-H04 验证码倒计时 interval 未清理 —— **真缺陷，但降级为 P3（原假设需纠正）**

- **位置**：`web/src/components/AuthPage.jsx:131-140`、`web/src/components/LoginModal.jsx:112-121`（`setInterval` 在 async 发送处理函数里创建，靠 state updater 内 `clearInterval(timer)` 自停）。
- **先纠正一条**：原 T-4 猜测"双击可产生两个 interval、60s 变 30s"——**不成立**。发送按钮是 `disabled={sendingCode || codeCountdown > 0}`（`AuthPage.jsx:412`、`LoginModal.jsx:431`），第一次点击后 `sendingCode=true` 立即禁用，第二次 click 被丢弃。
- **仍然为真的部分**：定时器与组件生命周期无关。倒计时期间关闭/离开认证页（或登出、`endSession` 卸载），interval 继续跑满 60 秒，对已卸载实例 setState（React 19 静默忽略，无警告），闭包与定时器在这段时间内不被 GC。
- **影响**：低——不产生错误数据、不阻断操作，只是定时器泄漏（≤60s/次）；若用户频繁在验证码页面来回切换，会叠加多个（互不影响的）定时器。
- **证据（探针）**：`web/src/components/AuthPage.countdownTimer__bugprobe.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/components/AuthPage.countdownTimer__bugprobe.test.jsx
  ```
  假定时器下：填邮箱 → 点「获取验证码」→ 断言按钮变为「Ns 后重发」且 `vi.getTimerCount() > 0` → `unmount()` 后仍 `> 0`，再 `advanceTimersByTime(3000)` 仍在跑。
- **修复建议**：把倒计时改成 `useEffect` 驱动（`codeCountdown > 0` 时 `setInterval`，cleanup `clearInterval`），或持有 `timerRef` 并在卸载 cleanup 里清除；`LoginModal` 同样处理。

---

## 3. 新发现

### BUG-H05 AI Copilot「集群健康巡检」读错 4 个字段，直接给出错误的运维结论（P2，高）

- **位置**：`web/src/components/copilotHarness.js:203-224`（`get_cluster_telemetry`）；报告模板在 `web/src/components/AiCopilot.jsx:559`。
- **触发条件**：任意一次「集群巡检 / 健康检查 / 通道诊断」类提问（Copilot 内置基准任务 `HEALTH-01` 就是它），或模型自行调用该工具。
- **根因与 4 个缺陷**：

| # | 代码 | 后端事实 | 后果 |
|---|---|---|---|
| a | `:207` `c.breaker_status === 'open'` | `breaker_status` 取值是 **`"OPEN"`**（`internal/router/circuit_breaker.go:24`；`channel_view.go` 原样透出；`ChannelsView`/`DashboardView`/`ServiceStatus` 都先 `.toUpperCase()`） | `tripped_channels` 恒 0、`health_score` 恒 100、`health_eval` 恒 `HEALTHY`；同一份 payload 里 `breaker_status:"OPEN"` 与 "健康" 自相矛盾 |
| b | `:223` `stats?.cache_hit_rate \|\| '0%'` | `storage.StatsOverview` 无此字段（只有 `total_cached_tokens` / `prompt_tokens`） | 报告永远写「缓存命中率 0%」 |
| c | `:224` `total_cost_usd: stats?.total_cost` | `StatsOverview.total_cost` 是 **CNY**（`repository.go:20`；钱包/账单全链路 CNY） | 把人民币金额标成 USD，对外汇报差一个汇率量级 |
| d | `:222` `avg_ttft_ms: stats?.avg_ttft_ms \|\| 120` | 无样本时后端给 0（配 `ttft_samples`） | 编造 120ms 基线延迟（与上轮 BUG-WEB-03 同类：用假数据填空白） |

- **影响**：Copilot 的巡检结论是**自然语言写给运维看的**："当前所有上游通道均处于健康活跃状态"——即使真的有渠道在熔断。同时"缓存命中率 0%"与"成本 $x"会让 FinOps 判断跑偏。这是"看到错数据"里最容易被直接采信的一类。
- **证据（探针）**：`web/src/components/copilotHarness.contract__bugprobe.test.jsx`（4 例）
  ```
  cd web && npx vitest run --configLoader runner src/components/copilotHarness.contract__bugprobe.test.jsx
  ```
  - 两个渠道（一个 `breaker_status:'OPEN'`）⇒ `tripped_channels === 0`、`health_score === 100`，而同 payload 的 `channels[0].breaker_status === 'OPEN'`；
  - `STATS`（50% cached）⇒ `cache_hit_rate === '0%'`；
  - `total_cost: 88.5` ⇒ `total_cost_usd === 88.5`；`avg_ttft_ms: 0` ⇒ 输出 `120`。
- **修复建议**：`const breakerOf = (c) => String(c.breaker_status || '').toUpperCase();` 用 `['OPEN','HALF-OPEN'].includes(...)`；缓存命中率由 `total_cached_tokens / prompt_tokens` 现算（无样本就写「无样本」）；字段改名 `total_cost`（单位 CNY）或换算后才叫 usd；TTFT 无样本显示「无样本」而不是 120。

### BUG-H06 AI Copilot 的 SPOF 审计读错 provider 字段，主通道名恒为 `default`（P2，高）

- **位置**：`web/src/components/copilotHarness.js:311,319`（`audit_routing_spof`：`r.providers?.[0]?.channel` 与 `p.channel`）；对照 `web/src/components/ModelRoutesManager.jsx` 用的是 `p.channel_name`。
- **后端事实**：`/api/v1/admin/models/routes` 的 provider 结构是 `router.ModelProviderInfo`，字段为 **`channel_name`**（`internal/router/topology.go:27-39`：`channel_id/channel_name/channel_type/priority/weight/weight_percent/mapped_model/...`），没有 `channel`。
- **触发条件**：任意一次「路由/单点故障/拓扑」类提问（基准任务 `ROUTE-03`）。
- **根因**：字段名猜错。`r.providers?.[0]?.channel` 恒 `undefined`，`?? 'default'` 把它兜成字符串 `'default'`；`providers.map(p => ({ channel: p.channel }))` 则输出一堆 `channel: undefined`。
- **影响**：SPOF 建议点名"仅由 `default` 提供"——一个不存在的渠道名；冗余通道样例里的 provider 列表全是 `undefined`。运维按提示去渠道管理里找不到 `default`，且无法判断真正的主备是哪个渠道。
- **证据**：同上探针文件的 `describe('BUG-H06 ...')`
  - 单 provider 路由 ⇒ `spof_vulnerabilities[0].primary_channel === 'default'`（真实为 `volc-primary`）；
  - 双 provider 路由 ⇒ `safe_routes_sample[0].providers.map(p => p.channel)` 为 `[undefined, undefined]`。
- **修复建议**：改读 `channel_name`（并兼容 `channel_id`），或把 provider 归一化抽成共享函数与 `ModelRoutesManager` 复用。

### BUG-H07 日志详情「计费核算公式」漏掉 prompt-cache 写入溢价，Token 明细把写入量并进「未缓存输入」（P2，高）

- **位置**：`web/src/components/LogDetailModal.jsx:565-603`（公式只用 `prompt_price`/`completion_price`/`cache_read_price`/`fixed_price`）；`:534-536`（Token 明细：`未缓存输入 = prompt_tokens - cached_tokens`，从不显示 `cache_creation_tokens`）。
- **后端事实**：计费包含 prompt-cache **写入**项——`internal/billing/engine.go:492` 把 `cacheWritePrice`/`cacheCreationTokens` 写进计价快照；`storage.UsagePricingSnapshot` 明确带 `cache_write_price` 与 `cache_creation_tokens`（`repo_logs.go:113-120`，注释：`CacheCreationTokens is how many of PromptTokens were written to the cache`）。
- **触发条件**：任意一次命中 prompt-cache **写入**的请求（Anthropic 系缓存、或网关的 cache-creation 路径），在「调用日志 → 详情」里查看该行。
- **根因**：详情面板按"prompt/输出/缓存读"三项重建公式，漏了缓存写入这一项；Token 明细用 `prompt - cached` 当"未缓存"，而 `prompt_tokens` 按后端注释**包含**被写入缓存的那部分。
- **影响**：面板明确写着"＝¥已扣金额"，但公式算不出这个金额——检查账单的人会认为计费错了（或反过来放过真实错账）；同时"未缓存输入 800"里混着 300 个 cache-write tokens，token 结构被误读（成本归因、容量规划都会偏）。
- **证据（探针）**：`web/src/components/LogDetailModal.cacheWrite__bugprobe.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/components/LogDetailModal.cacheWrite__bugprobe.test.jsx
  ```
  构造 `prompt=1000, cached=200, cache_creation=300, cost=0.0123`，快照含 `cache_write_price: 2.5`，断言渲染出的公式恰为
  `(未缓存 800 × ¥2/1M + 缓存 200 × ¥0.2/1M + 输出 100 × ¥8/1M) × 1 = ¥0.0123`
  且整段**没有**「缓存写入」项；Token 明细为 `未缓存输入: 800`（把 300 写入量算成了普通输入）。
- **修复建议**：公式增加 `缓存写入 cache_creation × ¥cache_write_price/1M` 项；Token 明细拆成「未缓存输入（prompt − cached − cache_creation）／缓存命中／缓存写入／输出」；平价展示 `cache_write_price`。

### BUG-H08 8s 轮询响应覆盖刚完成的操作刷新（`useConsoleData.refresh` 无请求序号）（P2，高）

- **位置**：`web/src/hooks/useConsoleData.js:72-156`（`refresh` 只有 `generationRef` 的"登出"守卫，无"谁是最新一次"的序号/abort）；调度方 `web/src/App.jsx:589-597`（每 8s 调 `fetchData()`），而所有写操作成功后也调 `refresh()` 回显结果（`useChannelActions.js:257`、`useKeyActions.js` 多处 `refresh()`）。
- **触发条件**：8s 轮询请求在途时执行写操作（新建/编辑/启停渠道、密钥、定价等）。慢轮询的响应（旧快照）晚于写操作触发的刷新才返回。
- **根因**：两个调用没有序关系。`Promise.all` 的 6 个读接口谁慢谁晚落盘；`setChannels/setKeys/...` 无条件覆盖。
- **影响**：**"操作成功但列表里没有"**——新建的渠道/密钥消失 8 秒后（下一拍）又出现；编辑后看到的还是旧值。用户会重复提交（产生重复实体）或认为保存失败。这是本轮明确在查的"8s 轮询 × 操作"竞态。
- **证据（探针）**：`web/src/hooks/useConsoleData.pollRace__bugprobe.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/hooks/useConsoleData.pollRace__bugprobe.test.jsx
  ```
  第一次 `refresh()`（模拟轮询）挂起 → 第二次 `refresh()`（模拟创建后的回显）返回含 `just-created` 的列表 ⇒ `channels === ['just-created']` → 放行第一次的旧列表 ⇒ `channels` 变回 `['only-before-create']`（新行消失）。
- **修复建议**：`refresh` 内加 `const seq = ++requestSeqRef.current`，落盘前比对（照 `SupplierReconciliationView.jsx:88-118`）；更彻底的做法是轮询与"操作后刷新"共用同一 inflight（进行中则复用其 promise，或操作后先 abort 轮询请求）。

---

## 4. 待确认（本轮未定级，需产品口径或重探针）

| 编号 | 现象 | 位置 | 说明 |
|---|---|---|---|
| T-5 | **关闭 AI Copilot 面板不会中止生成**：`isOpen=false` 只是渲染成悬浮按钮，组件仍挂载、`fetch` 与 ReAct 工具循环继续跑（最多 4 轮，每轮一次上游请求）；只有显式「停止生成」按钮会 `abort()`，卸载时也没有 cleanup | `AiCopilot.jsx:452-458`（stop 才 abort）、`:986`（关闭仅 return null）、无 `abortControllerRef` 的卸载清理 | 影响=浪费上游 token（成本）与工具副作用；复现步骤：打开 Copilot→选免密/贴 key→发送→生成中点头部 ✕ 或 ESC 关闭→观察网络面板仍有 `/v1/chat/completions` 及后续工具轮次。要写成探针需要导出内部 controller，故先列待确认 |
| T-6 | **Playground 忽略 `finish_reason`**：`max_tokens` 截断（`finish_reason:"length"`）时界面与完整回答无区别 | `PlaygroundView.jsx:579-590`（只取 `delta.content`/`usage`） | 属"UI 诚实性"问题：用户可能把截断的答案当完整结论。需产品口径决定是否显示"已达输出上限" |

---

## 5. 本轮已排除（复核过、未发现问题）

- **8s 轮询 × 日志分页/筛选**：`App.jsx:589-597` 的轮询只刷新控制台数据与 dashboard 最近日志，不碰日志页；日志页自己的 10s 轮询走 `fetchLogsRef` + `useLogsData` 的 `logOffsetRef`/`filtersRef`/`requestRef`（`useLogsData.js:49-62,124-127,145`），取的是**当前页当前筛选**，旧响应被序号丢弃。除 BUG-H08（控制台共享数据切片）外无竞态。
- **金额单位（整数 units → CNY）**：`api_keys` 同时写 `used_cost`(float) 与 `used_cost_units`(int64)，读路径 `repo_keys.go:207-216` 两者互相回填；`settlement.go:436` 两条一起 `+=`。前端只消费 float CNY 字段，**未发现**把 units 当元或反向换算的地方；`timeseries` 的驼峰 `avgTtft` 与后端结构体一致（上轮已核）。唯一金额问题是 BUG-H07（口径漏项）与 BUG-H05c（CNY 标 USD）。
- **权限分支**：复核 `KeysView`（`canUserSwitch`/`userGuaranteedGroup`）、`PricingManager`（`isAdmin` 隐藏写入口）、`WalletManagementView`（`isRedemptions` 仅 admin 拉取）、`UserManagementView`、`App.jsx` 角色守卫（上轮修好的 `allowedTabsFor` 单一真源）——未发现新的写反/漏判。
- **API 契约（字段名/分页/状态码）**：把 `web/src` 全部 `/api/...` 路径与 `router.go` 再对一遍仍全部存在；`/admin/models` = `[]string`、`/admin/models/routes` = `ModelRouteDetail[]`、`/admin/stats/*`、`/user/wallet` 字段与前端读取一致。**例外**即本轮 BUG-H05/H06（读 `/admin/*` 数据时的字段假设错误）与 BUG-H07（计价快照里被忽略的字段）。
- **localStorage / 登出残留**：`airoute_dismissed_attention`、`airoute_dismissed_announcement`、`airoute_copilot_btn_pos` 等均为纯 UI 偏好（不含凭证、不含他人数据）；上轮 §6 补齐的会话态复位仍在。`getStoredToken/getStoredUser` 的 `getItem` 无 try/catch（仅 `JSON.parse` 有）属背景风险，未定级（需浏览器存储被策略禁用才触发）。
- **大列表渲染 key/抖动**：`LogsView`/`PricingManager`/`ModelRoutesManager` 关键行都用业务键（`log.id`、`itemKey`、`channel_id`）；`ModelMappingEditor.jsx:53` 虽用 index key，但输入是受控的（值来自父 state），不会串值。未发现因 key 导致的错行编辑。
- **i18n**：`i18n.js` 的 zh/en 键集合仍完全对称，生产代码使用的 `t.*` 无缺失；上轮修复的 `MarkdownRenderer`/`ViewLoader` 语言接入仍在。本轮未发现新的键缺失。

---

## 6. 探针/回归用例清单与运行方法

> 约定（阶段 ② 之后）：下列文件已是**正式回归用例**，断言的是修复后的正确行为（在修复前代码上会失败）；每个文件头注明它对应的 BUG-ID。修复阶段把 7 个 `*__bugprobe` 翻转并改名，另补 2 个用例文件（ErrorBoundary 单元、LoginModal 定时器）。

| 文件（修复后） | 覆盖 | 用例 |
|---|---|---|
| `web/src/hooks/useLogsData.selection.test.jsx` | BUG-H01（翻页/换筛选清空选择、同查询轮询保留、失效后不发批量删除） | 4 |
| `web/src/components/WalletManagementView.refreshRace.test.jsx` | BUG-H02（一次手势一个请求、旧响应不覆盖新余额） | 2 |
| `web/src/App.errorBoundary.test.jsx` | BUG-H03（视图崩溃显示兜底并可重试恢复；chunk 文案；英文界面） | 3 |
| `web/src/components/AuthPage.countdownTimer.test.jsx` | BUG-H04（倒计时推进、卸载后定时器归零） | 1 |
| `web/src/components/LoginModal.countdownTimer.test.jsx` | BUG-H04 第二处（关闭对话框后定时器归零） | 1 |
| `web/src/components/copilotHarness.contract.test.jsx` | BUG-H05（熔断计数/半开、缓存命中率、CNY、无样本 TTFT）、BUG-H06（channel_name、channel_id 兜底） | 8 |
| `web/src/components/LogDetailModal.cacheWrite.test.jsx` | BUG-H07（公式含缓存写入项且与账面一致、token 拆分、无写入时保持两段公式） | 3 |
| `web/src/hooks/useConsoleData.pollRace.test.jsx` | BUG-H08（旧轮询不覆盖新刷新；无竞争时正常生效） | 2 |
| `web/src/components/AiCopilot.closeAborts.test.jsx` | T-5（关闭面板 abort 在途模型请求） | 1 |

单跑（与全量互不影响）：

```bash
cd web
npx vitest run --configLoader runner src/hooks/useLogsData.selection.test.jsx
npx vitest run --configLoader runner src/components/WalletManagementView.refreshRace.test.jsx
npx vitest run --configLoader runner src/App.errorBoundary.test.jsx
npx vitest run --configLoader runner src/components/AuthPage.countdownTimer.test.jsx
npx vitest run --configLoader runner src/components/LoginModal.countdownTimer.test.jsx
npx vitest run --configLoader runner src/components/copilotHarness.contract.test.jsx
npx vitest run --configLoader runner src/components/LogDetailModal.cacheWrite.test.jsx
npx vitest run --configLoader runner src/hooks/useConsoleData.pollRace.test.jsx
npx vitest run --configLoader runner src/components/AiCopilot.closeAborts.test.jsx
```

## 7. 修复记录（2026-10-29 同日）

见下方。

**范围**：只改前端（`web/src`），未触碰 Go 后端、未 `npm run build`（`web/dist` 由 Lead 统一重建）、未 commit。

### 7.1 逐条结果

| 编号 | 状态 | 生产改动 | 回归测试（断言修复后行为） |
|---|---|---|---|
| BUG-H01 | ✅ fixed | `hooks/useLogsData.js:134-145` 引入"选择所属查询"签名（offset + 各筛选输入 + 相关度字段，**不含**实时换算的时间戳）：签名变化即清空 `selectedLogIds`（`:263` `reset()` 同步复位）。同查询的轮询签名不变，选择保留 | `useLogsData.selection.test.jsx`：翻页清空 / 换筛选清空 / 同查询轮询保留 / 失效后不再发 batch-delete（4 例） |
| BUG-H02 | ✅ fixed | `WalletManagementView.jsx:66-118` `fetchWallet` 加请求序号 + `AbortController`（旧响应丢弃、`finally` 只由最新一次收尾）；`:136-152` 把 `focus`+`visibilitychange` 合并为一次手势（500ms 窗口内只发一次），并新增卸载 abort 的 cleanup | `WalletManagementView.refreshRace.test.jsx`：一次手势 = 1 个请求；先新后旧落盘时仍显示 ¥300（2 例） |
| BUG-H03 | ✅ fixed | 新增 `components/ErrorBoundary.jsx`（`getDerivedStateFromError` + `componentDidCatch`，兜底 UI 含「重试」/「刷新页面」/错误摘要，识别 chunk 加载失败文案，zh/en）；`main.jsx` 顶层包裹全站；`App.jsx:1202-1452` 用 `key={currentTab}` 包裹控制台视图区（单视图崩溃不再拖垮侧栏/头部，切 tab 自动复位） | `App.errorBoundary.test.jsx`：崩溃后显示兜底且侧栏仍在、修好后点「重试」恢复；chunk 文案；英文界面（3 例） |
| BUG-H04 | ✅ fixed | `AuthPage.jsx:74-88`、`LoginModal.jsx:64-77`：倒计时改为 effect 驱动（`isCountingDown` 变化时建/清 interval）。AuthPage 随卸载清理；LoginModal 组件关闭时仍挂载，故条件同时含 `isOpen` | `AuthPage.countdownTimer.test.jsx`（推进 + 卸载后 0 个定时器）、`LoginModal.countdownTimer.test.jsx`（关闭后 0 个定时器） |
| BUG-H05 | ✅ fixed | `copilotHarness.js:211-253`：`breakerOf()` 统一 `toUpperCase()` 后判 `OPEN`/`HALF-OPEN`/`TRIPPED`；`cache_hit_rate` 由 `total_cached_tokens / prompt_tokens` 现算（无 prompt 样本 → `n/a`）；`avg_ttft_ms` 无样本返回 `null` 并带 `ttft_samples`；金额字段改名 `total_cost` + `currency: 'CNY'`。`AiCopilot.jsx:551-572` 报告模板相应渲染「无样本」与 `CNY x.xxxx` 累计消费 | `copilotHarness.contract.test.jsx`（熔断计数/半开、命中率与 n/a、CNY 与无样本 TTFT、渠道列表状态）；既有 `copilotHarness.test.js` 的旧契约断言已同步修正（32 例） |
| BUG-H06 | ✅ fixed | `copilotHarness.js:327-343`：新增 `providerChannel()` 读 `channel_name`（回退 `name` → `channel#id`），SPOF 主通道与冗余 provider 列表均用它；`probe_upstream_channel` 的 `health_eval` 同口径大写判定 | 同 `copilotHarness.contract.test.jsx`（channel_name / channel_id 兜底 / 冗余列表） |
| BUG-H07 | ✅ fixed | `LogDetailModal.jsx:531-552` Token 明细拆出「缓存写入」（未缓存 = prompt − cache-read − cache-write）；`:577-620` 计费公式新增 `缓存写入 × ¥cache_write_price/1M` 项，三段/四段自适应 | `LogDetailModal.cacheWrite.test.jsx`：公式逐字匹配且等于账面 0.0026、token 拆分、无写入时保持两段公式（3 例） |
| BUG-H08 | ✅ fixed | `hooks/useConsoleData.js:44,84-85,117`：`refresh` 增加请求序号 `requestSeqRef`，`isCurrent()` 同时校验序号与登出 generation，被更新的一次取代后丢弃结果 | `useConsoleData.pollRace.test.jsx`：旧轮询晚到不覆盖新列表；无竞争时刷新正常生效（2 例） |
| T-5（可选） | ✅ fixed | `AiCopilot.jsx:407-424`：`isOpen` true→false 时 abort 在途生成并复位 loading；另加卸载 abort cleanup（登出/会话失效也不再把 ReAct 循环跑完） | `AiCopilot.closeAborts.test.jsx`：关闭面板后 `/v1/chat/completions` 的 signal 变为 aborted（1 例） |

### 7.2 顺带修正的既有用例

- `components/copilotHarness.test.js` 的「reports the performance metrics it was given」原先按**错误契约**断言（传 `cache_hit_rate:'42%'`、断言 `total_cost_usd`）。已改为传真实字段（`prompt_tokens`/`total_cached_tokens`）并断言派生命中率 `42.0%`、`total_cost` 与 `currency: 'CNY'`。

### 7.3 明确没做的（避免超范围）

- 未把 H05 的"无样本"口径推广到其它视图（`SupplierReconciliationView` 的 `avg_ttft_ms || 0` 等）：属同类但不在本轮 8 条内，留作后续统一。
- 未给 `useConsoleData` 换成 AbortController（序号已足以丢弃旧结果；中止省流量但改动面更大）。
- 未处理 §4 的 T-6（Playground 忽略 `finish_reason`）：需产品口径。
- 未动 `web/web.go` 与任何 Go 文件。

### 7.4 验证（修复后，2026-10-29 本机）

```
cd web && npm run lint  -> 0 warning
cd web && npm test      -> 65 files / 710 tests passed
```

- 基线 56 文件 / 685 用例；本轮新增 9 个回归文件 / 25 个用例（4+2+3+1+1+8+3+2+1），并删除了 7 个已翻转的 `*__bugprobe` 探针（11 例）。
- 每条回归用例都对应修复前的错误行为，在修复前代码上会失败（排查阶段的探针版本断言了相反结果且当时全绿，即为此证明）。
- 未运行 `npm run build`；`web/dist` 未变；未 commit。
