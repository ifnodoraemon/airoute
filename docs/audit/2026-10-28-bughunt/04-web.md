# 前端控制台真实缺陷排查（2026-10-28，Bug 排查 D 域）

> **范围**：`web/src` 全部（`App.jsx`、`components/`、`hooks/`、`lib/`、`api/`、`i18n.js`、CSS）与 `web/web.go` 的静态服务/embed。
> **判据**（用户口径）：**用户会不会看到错数据 / 白屏 / 丢操作**。不查样式观感、不查死代码（另有专人负责）、不重复上报历史审计已修项。
> **方法**：静态推理 + 逐条对照后端真实响应形状（`internal/controlplane`、`internal/api/router.go`、`internal/storage`）+ jsdom/vitest 复现（无浏览器可用）。
> **基线**：`web/src` 51 文件 / 666 用例全绿（`npm test`），`npm run lint` 0 warning。
> **两阶段**：① 排查阶段只写报告 + `*__bugprobe.test.jsx` 探针（断言错误行为，保持全绿）；② 同日进入修复阶段，7 条全部修复、探针**翻转成回归测试**（断言正确行为），改动与验证见 **§6 修复记录**。修复后基线：**56 文件 / 685 用例全绿，lint 0 warning**。
> **前置阅读**：`docs/audit/2026-10-28-inconsistencies/README.md` §3.2/§3.3/§6 与 `04-api-console.md`（D-01…D-14 已全部 fixed）——本报告不与那 14 条重复。

---

## 1. 结论摘要

控制台的**请求层与数据层已被上一轮硬化得很到位**（统一 `apiFetch`/`apiJSON`、`useLogsData` 的 generation+request 双序、`useConsoleData` 的登出 generation、`SupplierReconciliationView` 的 abort+seq、`SystemLogConsole` 的 SSE 重连与清理、`MarkdownRenderer` 全程不走 `dangerouslySetInnerHTML`）。因此本轮找到的不是"漏了鉴权/漏了掩码"那类问题，而是**五处"两套事实/字段不一致"**（01、02、03、06、07）、**一处缺失的请求竞态守卫**（04）和**一处顺序语义错误的钩子**（05，潜在）：

| 严重度 | 编号 | 一句话 | 位置 |
|---|---|---|---|
| **P1** | BUG-WEB-01 | 普通用户**永远打不开「概览 / 我的用量」页**：侧边栏有入口、路由表有该 tab、组件也写了，但角色守卫白名单漏了 `dashboard`，点击/深链一律被改写成钱包页 | `App.jsx:434-441` |
| P2 | BUG-WEB-02 | 对外状态页**读错熔断字段**：用 `ch.status === 'tripped'`（该字段只有 `active/disabled`），真正的 `breaker_status` 不看 ⇒ 已熔断渠道显示「健康闭合」，页面汇总显示「未发现熔断」 | `ServiceStatus.jsx:167,507` |
| P2 | BUG-WEB-03 | 状态页给**从未探测过**的渠道显示伪造延迟「~35 ms (探针正常)」，与本文件 :170 注释「no synthetic latency values」自相矛盾 | `ServiceStatus.jsx:554` |
| P2 | BUG-WEB-04 | 链路快照搜索**无请求竞态守卫**：慢的旧查询回包会覆盖新查询结果，表格里的快照属于另一个 trace/chat | `TraceSnapshotModal.jsx:21-58` |
| P2（**潜在**，机制已复现） | BUG-WEB-05 | 弹窗 body 滚动锁**按实例保存/恢复、非引用计数**：只要清理顺序不是严格 LIFO，`overflow:hidden` 就会永久留在 `<body>` 上，下一页无法滚动。当前 UI 流程恰好没有踩中（见 §2 的触达分析），但钩子语义与同一套"栈"设计的 `escapeLayer` 不一致 | `hooks/useModalA11y.js:29-45` |
| P2 | BUG-WEB-06 | i18n 双向混入：`MarkdownRenderer` 的代码块按钮硬编码中文且组件**无法接收 lang**；`App.jsx` 的 `ViewLoader` 硬编码英文 | `MarkdownRenderer.jsx:243-263`、`App.jsx:85` |
| P2 | BUG-WEB-07 | 登出清理**漏两处会话态**：`dashboardSelectedKey`（仪表盘密钥下钻）与 `showAnnouncementModal` 不在 `resetWorkspaceState` 里，跨账号/跨会话残留 | `App.jsx:318-335` vs `:356-357` |

置信度：BUG-WEB-01/02/03/04/06 **高**（均有可运行探针或后端字段对照）；BUG-WEB-05 **机制高、真实触达低**（探针证明机制，但当前 UI 流程下未找到用户可达路径，见 §2 触达分析）；BUG-WEB-07 **中高**（静态证据确凿，但触发需两次会话，未自动化，给精确步骤）。

**修复顺序（已按此执行，结果见 §6）**：BUG-WEB-01（单一真源 + 角色首个允许 tab 兜底，用户直接丢功能）→ BUG-WEB-02/03（状态页改读 `breaker_status` + 删除伪造延迟）→ BUG-WEB-04（照抄 `SupplierReconciliationView` 的 seq+abort）→ BUG-WEB-07（补齐会话级 reset）→ BUG-WEB-06（组件接入 i18n）→ BUG-WEB-05（滚动锁改引用计数）。全部 7 条已 fixed。

---

## 2. 发现详情

> **状态：7 条已于同日全部修复**（见 §6）。以下各条保留排查阶段的原始证据与根因，行号以修复前代码为准；修复后的位置与新行号见 §6。

### BUG-WEB-01 普通用户无法打开「概览 / 我的用量」——角色守卫白名单与导航/路由表不一致

- **严重度**：P1 ｜ **置信度**：高 ｜ **类别**：权限分支不一致（可访问页面被静默重定向）
- **位置**：
  - 守卫：`web/src/App.jsx:434-441`
  - 导航：`web/src/lib/navGroups.js:46-49`（`USER_NAV_GROUPS.account.tabs = ['dashboard','wallet','keys']`）
  - 渲染：`web/src/App.jsx:1176-1178`（`currentTab === 'dashboard' && role !== 'admin'` → `<MyUsageView/>`）
  - 深链解析：`web/src/lib/consoleRoute.js:5`（`USER_TABS` 含 `dashboard`）
- **触发条件**：任意 `role !== 'admin'` 的账号，点击侧边栏「概览」，或直接访问 `#/dashboard`。
- **根因**：同一个"普通用户可访问哪些 tab"的事实存在三份，其中守卫这份漏了 `dashboard`：

```js
// App.jsx:434-441
useEffect(() => {
  if (!restoringSession && adminUser && adminUser.role !== 'admin') {
    const allowedTabs = ['wallet', 'keys', 'playground', 'pricing', 'logs', 'docs'];
    if (!allowedTabs.includes(currentTab)) {
      setCurrentTab('wallet');     // ← 'dashboard' 不在白名单 ⇒ 每次都被改写
    }
  }
}, [adminUser, currentTab, setCurrentTab, restoringSession]);
```
```js
// lib/navGroups.js:46-49 —— 侧边栏把 dashboard 列在"我的账户"组首位
export const USER_NAV_GROUPS = [
  { id: 'account', zh: '我的账户', en: 'My Account', tabs: ['dashboard', 'wallet', 'keys'] },
  ...
```
```js
// lib/consoleRoute.js:5 —— 深链解析也认为普通用户能用 dashboard
const USER_TABS = ['dashboard', 'wallet', 'keys', 'playground', 'pricing', 'logs', 'docs'];
```
- **影响**：普通用户**完全看不到自己的用量面板**（`MyUsageView`：请求数 / Tokens / 累计费用 / 钱包余额 / 每个密钥的预算水位），而侧边栏仍然展示「概览」入口。点击后页面"闪一下"落到钱包页，地址栏变成 `#/wallet`——用户会认为"概览坏了"。这是**整页功能丢失**，不是显示瑕疵，故定 P1。
- **证据（原探针；修复后已翻转为回归断言）**：`web/src/App.roleTabs.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/App.roleTabs.test.jsx
  ```
  - `control` 用例：管理员深链 `#/dashboard` → 保持 `#/dashboard`（证明解析本身没问题）；
  - `BUG-WEB-01` 用例：普通用户深链 `#/dashboard` → 侧边栏能找到「概览」，但最终 `location.hash === '#/wallet'` 且 `我的用量` 永不出现。
- **修复建议**（一行）：把 `'dashboard'` 加进 `allowedTabs`；更好的做法是直接复用 `navTabs(false)`（`lib/navGroups.js:97`）或 `consoleRoute` 的 `USER_TABS`，让"普通用户可访问 tab"只有一处定义，并用一条单测钉住 `allowedTabs ⊆ navTabs(false)` 且 `navTabs(false) ⊆ allowedTabs`。
- **回归防线**：修好后把探针里的 `expect(hash).toBe('#/wallet')` 改成 `toBe('#/dashboard')`、并把 `queryByText('我的用量')` 改成 `getByText('我的用量')`。

---

### BUG-WEB-02 对外状态页读错熔断字段：熔断渠道一律显示「健康闭合」

- **严重度**：P2（对外状态页说假话；不涉及写操作） ｜ **置信度**：高 ｜ **类别**：字段名与后端契约不一致
- **位置**：`web/src/components/ServiceStatus.jsx:167`（页面级 `anyTripped`）、`:506-507`（逐行 `isTripped`）
- **后端事实**：渠道的**启用状态**是 `status`（`active` / `disabled`），**熔断状态**是另一个字段 `breaker_status`（`CLOSED` / `OPEN` / `HALF-OPEN`）——`internal/controlplane/channel_view.go:24-40` 明确同时序列化两者；控制台其它两处都读对了：`ChannelsView.jsx:62-63`、`DashboardView.jsx:343-344`。
- **触发条件**：任一渠道 `status: 'active'` 且 `breaker_status: 'OPEN'`（即"启用但已熔断"，这是网关最需要被看到的运行状态）。
- **根因**：拿启用状态去和 `'tripped'` 比较——这个取值在 `status` 字段上**永远不可能出现**，分支恒假：

```js
// ServiceStatus.jsx:167
const anyTripped = channels.some((c) => c.status === 'tripped' || (probeLatencies[c.id] && probeLatencies[c.id].error))
  || (publicData && publicData.status === 'degraded');

// ServiceStatus.jsx:505-507
{channels.map((ch) => {
  const isDisabled = ch.status === 'disabled';
  const isTripped  = ch.status === 'tripped';   // ← 恒 false
```
- **影响**：状态页头部横幅显示「**当前未发现熔断渠道**」+ 绿色图标、「当前熔断状态：**未发现熔断**」，渠道表里已熔断渠道显示绿色「**健康闭合**」。运维/客户在对外状态页上看到的是"一切正常"，而实际上流量正在被摘除。`publicData.status === 'degraded'` 的兜底只在"某模型**全部**渠道 unhealthy"时才为 `degraded`（`internal/api/ops_handler.go`），部分熔断时救不回来。
- **证据（原探针；修复后已翻转为回归断言）**：`web/src/components/ServiceStatus.statusPage.test.jsx`（3 个用例）
  ```
  cd web && npx vitest run --configLoader runner src/components/ServiceStatus.statusPage.test.jsx
  ```
  1. `BUG-WEB-02`：`{status:'active', breaker_status:'OPEN'}` ⇒ 断言页面出现「当前未发现熔断渠道」「未发现熔断」「健康闭合」，且**不出现**「已熔断跳闸」；
  2. `BUG-WEB-02 control`：把 `status` 故意写成 `'tripped'` ⇒ 页面正确显示「存在已熔断渠道」「已熔断跳闸」——证明分支渲染没问题，缺陷就是字段名；
  3. 同时覆盖 BUG-WEB-03（见下）。
- **修复建议**：统一用 `String(ch.breaker_status || '').toUpperCase()` 判定 `OPEN`/`HALF-OPEN`（与 `DashboardView.jsx:343` 一致），并复用 `ChannelsView.jsx:22-67` 的 `channelHealth()` 归一化函数（把 health 判定抽到 `lib/`，三个视图共用）。

---

### BUG-WEB-03 状态页为"从未探测"的渠道伪造延迟「~35 ms (探针正常)」

- **严重度**：P2（展示编造的运行数据） ｜ **置信度**：高 ｜ **类别**：兜底值冒充真实测量值
- **位置**：`web/src/components/ServiceStatus.jsx:544-556`（伪造值在 `:554`）；自相矛盾的注释在 `:170`
- **触发条件**：打开状态页但**未点过"一键探测全部渠道"**（`probeLatencies` 为空）——这是默认状态，即绝大多数访问。
- **根因**：
```js
// ServiceStatus.jsx:170 —— 本文件自己的承诺
// Show only actual probe measurements; no synthetic SLA or latency values.
...
// ServiceStatus.jsx:544-555
{isDisabled ? (...) : lat ? (
  lat.error ? <...超时 / 探测失败...> : <...{lat.latency_ms} ms...>
) : (
  <span className="text-[#656B75] ...">{isZh ? '~35 ms (探针正常)' : '~35 ms (Normal)'}</span>
)}
```
- **影响**：面板"实时探测延迟"列对一个从未被探测的渠道显示 `~35 ms (探针正常)`，读者会把它当成实测值（正是状态页存在的意义）。同一页面在其它位置已经做到了"无样本就显示无样本"（`DashboardView` 的 `fmtPct` → 「无样本」），这里没有。数据本身是**编造的**，属于"看到错数据"。
- **证据**：同上探针的第 3 个用例：`probeLatencies={{}}` ⇒ 断言 `~35 ms (探针正常)` 出现。
- **修复建议**：无测量时显示「未探测 / 点击"一键探测"获取实测延迟」，与 `lat.error`/无样本的处理保持一致。

---

### BUG-WEB-04 链路快照搜索无竞态守卫：旧查询回包覆盖新查询结果

- **严重度**：P2 ｜ **置信度**：高 ｜ **类别**：请求竞态（旧响应覆盖新数据）
- **位置**：`web/src/components/TraceSnapshotModal.jsx:21-45`（`load`）、`:48-58`（挂载即 `load`）
- **触发条件**：为一个 trace 打开快照（慢请求在途），立刻在弹窗里换一个关联 ID 再查（chat_id/session_id），新结果先回、旧结果后回。
- **根因**：`load` 只做 `setRows(data.data)`，没有 AbortController、没有请求序号、没有"我是否仍是当前查询"的判断；挂载时的请求与"查询"按钮的请求共用同一份 `rows`：

```js
const load = async (field = queryField, value = queryValue || traceId) => {
  ...
  const res = await adminFetch(`/api/v1/admin/traces?${params.toString()}`);
  const data = await res.json();
  if (!res.ok || data.code !== 0) throw new Error(...);
  setRows(data.data || []);        // ← 无论是否是当前查询的最新响应
};
useEffect(() => { if (isOpen) { ...; load(initialQueryField, traceId); } }, [isOpen, traceId, initialQueryField]);
```
  对照：同项目 `SupplierReconciliationView.jsx:47-51,88-118` 已经用 `requestSeqRef` + `AbortController` 把这件事做对了（并且注释写明"abort 有时打不过已经进入解析的响应"），`useLogsData.js:55,67,124-127` 也有 `requestRef` 双序守卫——只有这个弹窗漏了。
- **影响**：**弹窗头部/搜索框显示的 ID 与下面渲染的快照不匹配**（探针里搜索框是 `chat-B`，表格是 trace A 的 500 快照）。排障者会拿着错误的请求体/响应体去定位问题，或据此判定"这条 chat 就是报错的"。
- **证据（原探针；修复后已翻转为回归断言）**：`web/src/components/TraceSnapshotModal.race.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/components/TraceSnapshotModal.race.test.jsx
  ```
  用两个可控 deferred 的 `adminFetch`：第 1 次（挂载，`trace_id=tr-slow-A`）挂起；第 2 次（搜索 `chat-B`）先回 `model-from-B`；再放行第 1 次回 `model-from-A` ⇒ 断言屏幕上出现 `model-from-A` 且 `model-from-B` 消失、搜索框仍是 `chat-B`。
- **修复建议**：拷贝 `SupplierReconciliationView` 的写法（`requestSeqRef`/`isCurrent()` + `signal` 传入 `adminFetch`），组件卸载/关闭弹窗时递增序号并 abort。
- **同类（未单独立项，建议一并加守卫）**：`WalletManagementView.jsx:66-94` 的 `fetchWallet`（`focus` 与 `visibilitychange` 会同时触发两次并发请求，见待确认 T-2）。

---

### BUG-WEB-05 弹窗 body 滚动锁按实例保存/恢复（潜在：非 LIFO 时把页面永久锁死）

- **严重度**：P2（**潜在缺陷**：机制已用探针复现，但当前 UI 流程下未找到用户可达的触发路径，见下方"触达分析"） ｜ **置信度**：机制 高 / 真实触达 低
- **位置**：`web/src/hooks/useModalA11y.js:29-45`
- **触发条件（机制层面）**：同时打开两个使用该 hook 的弹窗，且清理顺序不是严格后进先出（先关"先打开的那个"，或两者在同一提交里被卸载）。
- **根因**：每个实例把"打开时的 `overflow` 值"存进自己的闭包，清理时无条件写回。只有严格 LIFO 才正确：
```js
useEffect(() => {
  if (!isOpen) return undefined;
  const unregister = pushEscapeLayer(() => closeRef.current?.());
  const prevOverflow = document.body.style.overflow;   // 各实例各自一份
  document.body.style.overflow = 'hidden';
  return () => { unregister(); document.body.style.overflow = prevOverflow; };
}, [isOpen]);
```
  A 开（prev=''）→ B 开（prev='hidden'）→ 先关 A：A 写回 `''`（**B 还开着，背景却能滚**）→ 再关 B：B 写回 `'hidden'`（**已无弹窗，页面永久不能滚**）。这与同项目 `lib/escapeLayer.js` 刻意做对的"层栈/只有栈顶响应"是两套语义：Esc 是栈语义，滚动锁是"最后一个写回者赢"。
- **触达分析（为什么本轮不判为 P1/现实 bug）**：应用里确实存在**同时打开两个弹窗**的路径——`SkillHubView.jsx:825` 在"技能清单弹窗"（`manifestSkill`）里再打开"客户端接入弹窗"（`clientGuideModalOpen`）；`PricingManager.jsx` 的定价弹窗内也能打开"新建分组"；`McpIntegrationView.jsx:286-288` / `UserManagementView.jsx:115-117` / `SkillHubView.jsx:55-58` 各自持有 3–4 个 hook。但逐一核对后：
  - **正常关闭**只能是 LIFO（内层遮罩盖住外层，Esc 又按 `escapeLayer` 栈顶派发），结果正确；
  - **同批卸载**（切换 tab / 会话失效 / 登出）时 React 按 hook 声明顺序跑清理；上述几处"内层弹窗的 hook 恰好声明在外层之前"（如 SkillHubView `clientGuide`=55 < `manifest`=56），于是清理顺序天然变成"内层先、外层后"，最终写回的是外层的 `''`，**碰巧正确**。
  也就是说当前没有用户可达的卡死路径，但钩子语义是错的：任何一次新增嵌套弹窗（把内层的 hook 写在外层之后）、或把两个 modal 的一次性批量关闭引入，就会立刻变成"页面无法滚动"，且这类故障**只能靠刷新恢复**、极难从现象定位。
- **影响（一旦触发）**：内容还在、滚轮/触摸全失效，用户会认为控制台崩了。
- **证据（原探针；修复后已翻转为回归断言）**：`web/src/hooks/useModalA11y.scrollLock.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/hooks/useModalA11y.scrollLock.test.jsx
  ```
  两个子组件各用一次 `useModalA11y`，同批挂载；先关 A ⇒ 断言 `document.body.style.overflow === ''`（B 仍开着却是可滚的）；再关 B ⇒ 断言 `=== 'hidden'`（无弹窗却锁死）。
- **修复建议**（顺带消除上面那条"碰巧"）：把锁做成**模块级引用计数**（`openModal()`/`closeModal()`：0→1 记录原值并置 `hidden`，1→0 恢复），`useModalA11y` 只负责计数进出；或直接复用 `lib/escapeLayer.js` 的层栈（它已天然是栈语义）。

---

### BUG-WEB-06 i18n 双向混入：Markdown 代码块硬编码中文（组件无法接收语言），加载态硬编码英文

- **严重度**：P2（用户可见的"语言错乱"） ｜ **置信度**：高 ｜ **类别**：i18n 缺失
- **位置**：
  - `web/src/components/MarkdownRenderer.jsx:243-263`（`Wrap/Nowrap` 按钮 title 中英混排、「复制代码」、「复制/已复制」全部硬编码中文）、`:235-239`（`(N lines)` 硬编码英文）、`:567`（签名只有 `content`，**根本没有 lang 入参**）
  - 调用方都有 `lang` 却传不进去：`AiCopilot.jsx:1303`、`PlaygroundView.jsx:1569/1595/1629/1659`
  - 反向：`web/src/App.jsx:81-88` `ViewLoader` 硬编码 `Loading module...`（中文控制台里每个懒加载视图都闪现英文）
- **触发条件**：把控制台语言切到 English，然后打开 AI Copilot 或调试演练页里带代码块的回答（Copilot 的欢迎语、工具结果、模型输出基本都含代码块）。
- **根因**：两个组件都没有走 `I18nContext`/`lang` prop，字符串直接写在 JSX 里；`i18n.js` 的 `translations` 里也没有这些键（现有 43 个键与生产代码里使用的 `t.*` 完全一一对应，**没有键名错位/显示键名的问题**——缺的是这两个组件没接入）。
- **影响**：英文界面里出现「复制」「切换自动换行」「复制代码」等中文按钮；中文界面里加载态是 `Loading module...`。属于本轮明确在查的 "i18n 键缺失导致语言混入"，但只影响文案、不影响数据，故 P2 而非 P1。
- **证据（原探针；修复后已翻转为回归断言）**：`web/src/components/MarkdownRenderer.i18n.test.jsx`
  ```
  cd web && npx vitest run --configLoader runner src/components/MarkdownRenderer.i18n.test.jsx
  ```
  渲染一段 ```js 代码块 ⇒ 断言存在 `title="复制代码"`、文本「复制」、`title="切换自动换行"`，且**不存在** `Copy`/`Copy code`（组件无语言入参，任何界面语言都只能是中文）。
- **修复建议**：给 `MarkdownRenderer` 加 `lang`（或直接读 `I18nContext`），把 `Wrap/Nowrap/复制/已复制/复制代码/(N lines)` 挪进 `i18n.js`；`ViewLoader` 同理接入 `t`。

---

### BUG-WEB-07 登出清理漏两处会话态：密钥下钻与公告弹窗跨账号残留

- **严重度**：P2 ｜ **置信度**：中高（静态证据确定；触发需两次会话，未自动化） ｜ **类别**：登出后残留 state
- **位置**：
  - 清理函数：`web/src/App.jsx:318-335`（`resetWorkspaceState`）
  - 未被清理的 state：`web/src/App.jsx:356` `dashboardSelectedKey`、`:357` `showAnnouncementModal`
- **根因**：`resetWorkspaceState` 逐个列举要清的东西（`resetConsoleData/resetLogsData/resetChannelActions/resetKeyActions/resetSelections/resetOverlays` + 四个选择态 + 两个弹窗布尔），但 App 自己持有的另外两个状态漏了；`useConsoleOverlays.reset()`（`hooks/useConsoleOverlays.js:25-35`）也不含公告弹窗（它是 App 局部 state，不在该 hook 内）。
- **影响 / 两个可复现症状**：
  1. **仪表盘被残留的密钥下钻静默"清零"**：管理员在仪表盘用密钥选择器下钻到某个 key（`DashboardView.jsx:94` 把 `selectedKey` 拼成 `&key_id=`；App 侧状态在 `App.jsx:1205-1206` 由 `dashboardSelectedKey` 传入）→ 退出登录 → 再次登录（或另一个管理员在同一浏览器登录）→ 仪表盘**仍然带着上一位操作者的 `key_id`** 打开；若该 key 已被删除，`GET /stats/overview?key_id=` 返回 404（`admin_stats_key_filter.go:78-82`），`DashboardView.jsx:199-200` 的 `stats = statsKey === selectedKey ? keyStats || {} : {}` 于是拿到 `{}` ⇒ **整套 KPI/图表显示 0**，而下钻选择器仍显示那个 key。用户会以为网关没有流量。
  2. **公告发布弹窗在下次登录后自动弹出**：`showAnnouncementModal` 保持 `true`，下一位管理员进入控制台时 `{showAnnouncementModal && role==='admin' && <AnnouncementModal/>}` 直接渲染（表单是空的）。触发条件是"弹窗开着时发生会话失效（后台 8s 轮询 401 → `endSession`）"。
- **精确复现步骤**（无浏览器环境，给出可核对步骤）：
  1. 以 admin 登录 → 仪表盘点右上角密钥选择器选任一 key（URL 不变，但仪表盘请求带 `key_id`）；
  2. 点右上角退出登录（`handleLogout`）；
  3. 在同浏览器再次以 admin 登录 → 仪表盘首屏请求即为 `/api/v1/admin/stats/overview?key_id=<上一步的 id>`，KPI 区显示上一位操作者选中的密钥（或该 key 已删除时全 0）。
- **修复建议**：把这两个状态搬进已有的 `useConsoleOverlays` / `useConsoleChrome`（或给 `resetWorkspaceState` 补 `setDashboardSelectedKey(''); setShowAnnouncementModal(false);`），并加一条"登出后所有会话态归零"的单测（可参照 `App.test.jsx` 的 `does not show the previous account data…` 用例）。

---

## 3. 待确认（低置信 / 需产品口径）

| 编号 | 现象 | 位置 | 为什么先不判 bug |
|---|---|---|---|
| T-1 | 日志页的**批量选择跨页/跨筛选残留**：第 1 页选中若干行后翻到第 3 页（或改筛选），选中数仍显示、批量删除会作用在**看不见的行**上 | `hooks/useLogsData.js:44`（`selectedLogIds` 只在 `reset`/删除成功后清）、`App.jsx:626-632`（`handleLogPageChange` 不清选择） | 也可能是刻意"跨页累计选择"；但当前 UI 没有任何"已选行不在本页"的提示，容易误删。建议加"跨页选择 N 条"提示或翻页即清空 |
| T-2 | 钱包页回到标签页时**重复请求**：`focus` 与 `visibilitychange` 同时触发两次 `fetchWallet()`，且两次响应无序 | `WalletManagementView.jsx:112-127`（`fetchWallet` 无 abort/seq） | 影响仅是双倍请求 + 极短窗口内的旧余额覆盖；余额随后会被下一次刷新纠正 |
| T-3 | 全站**没有 Error Boundary**：任何一处渲染期异常（例如某行数据形状不符预期）都会让整个 React 树卸载 ⇒ 真白屏，只能刷新 | 全仓 `grep -r "ErrorBoundary\|componentDidCatch"` 零命中 | 不是某个具体 bug，而是"缺失的兜底"；建议加一个顶层 ErrorBoundary + "重新加载"按钮 |
| T-4 | 手机验证码倒计时以 `setInterval` 在 async 处理函数里启动，未随卸载清理（LoginModal/AuthPage 各一处） | `LoginModal.jsx:113-119`、`AuthPage.jsx:132-140` | 若发送按钮在请求期间已被禁用（需逐像素确认 JSX），则无法双击产生两个 interval；否则会双倍倒计时（60s 变 30s），只是体验问题 |

---

## 4. 探针清单与运行方法

新增文件（**只新增测试，未改任何生产代码**）：

排查阶段的 5 个 `*__bugprobe.test.jsx` 已在修复阶段**翻转成回归测试**并改名为正式用例（断言修复后的正确行为，在修复前代码上会失败）：

| 文件（修复后） | 覆盖 | 用例数 |
|---|---|---|
| `web/src/App.roleTabs.test.jsx` | BUG-WEB-01（含 admin 对照/点击入口/兜底 tab）、BUG-WEB-06（`ViewLoader` 语言）、BUG-WEB-07（登出清理） | 6 |
| `web/src/components/ServiceStatus.statusPage.test.jsx` | BUG-WEB-02（open/half-open/closed 三态）、BUG-WEB-03（未探测不再伪造） | 5 |
| `web/src/components/TraceSnapshotModal.race.test.jsx` | BUG-WEB-04（先慢后快 + 关闭即 abort） | 2 |
| `web/src/hooks/useModalA11y.scrollLock.test.jsx` | BUG-WEB-05（非 LIFO 关闭 / 同批卸载 / 还原原值） | 3 |
| `web/src/components/MarkdownRenderer.i18n.test.jsx` | BUG-WEB-06（en 无中文、zh、默认 zh、行数文案） | 3 |

约定（阶段 ② 之后）：这些文件已是**正式回归用例**，断言的是修复后的正确行为；每个文件头都注明它原本对应哪条 BUG-ID。

单跑一个（与全量互不影响）：

```bash
cd web
npx vitest run --configLoader runner src/App.roleTabs.test.jsx
npx vitest run --configLoader runner src/components/ServiceStatus.statusPage.test.jsx
npx vitest run --configLoader runner src/components/TraceSnapshotModal.race.test.jsx
npx vitest run --configLoader runner src/hooks/useModalA11y.scrollLock.test.jsx
npx vitest run --configLoader runner src/components/MarkdownRenderer.i18n.test.jsx
```

验证结果（2026-10-28 本机，修复后）：

```
npm run lint  -> 0 warning
npm test      -> 56 files / 685 tests passed   （基线 51 / 666 + 回归用例 5 文件 / 19 例）
```

---

## 5. 已排除项（看着像、实为设计如此 / 已被上一轮修好，不重复上报）

- **XSS（Markdown / `dangerouslySetInnerHTML`）**：全仓 `grep dangerouslySetInnerHTML|innerHTML|document.write|eval(` 零命中。`MarkdownRenderer` 走 `marked.lexer` 得到 token 后**逐 token 渲染成 React 元素**，`html` token 也以文本输出（`:177-191`）；链接/图片经 `safeMarkdownUrl` 限定 `http/https/mailto` 与 `data:image/(png|jpeg|gif|webp)`，`javascript:` 被丢弃。**不是漏洞**。
- **`apiFetch` 的 401 / CSRF / 会话世代**：`api/client.js:47-69` 用注册式 token provider（无 stale closure）、`SESSION_MARKER` 不当 Bearer、401 只在"会话世代未变"时触发登出；`api/session.js` 只存"已登录"标记与 profile，旧 token 键在登录/登出时删除。与 `docs/API_REFERENCE.md` 的"明文只回一次"口径一致。
- **8s 轮询的 stale closure**：`App.jsx:562-575` 的 interval 只调用稳定身份的 `fetchData`/`fetchRecentLogs`，`currentTab` 在依赖里；`useLogsData` 用 `logOffsetRef` + `filtersRef` + `requestRef`/`generationRef`（`:49-62,124-127,145`）保证"轮询刷当前页当前筛选"且旧响应不覆盖新数据。
- **日志分页/筛选竞态**：`App.jsx:586-623` 的 350ms 防抖 + `useLogsData` 的 request 序号，快速切页/切筛选不会取错页（`useLogsData.test.js` 也已覆盖）。
- **`useConsoleData` 的跨账号泄漏**：`generationRef` 在 `reset()` 时自增、在途响应比对世代后丢弃（`:42,77,109-113`）。
- **供应商对账的竞态**：`SupplierReconciliationView.jsx:47-51,88-118` 已用 abort + 序号双保险（这是 BUG-WEB-04 应该照抄的范本）。
- **`SystemLogConsole` 的 SSE / 轮询清理**：`active` 标志 + `stopPolling`/`clearTimeout`/`source.close()`，卸载时全清（`:69-156`）；EventSource 用 HttpOnly cookie（不把凭证塞进 URL）。
- **localStorage 解析崩溃**：所有 `JSON.parse` 均有 try/catch（`session.js:30-34`、`DashboardView.jsx:66-72`、`useTableControls.js:25-31`、`AiCopilot.jsx:88-99`、`PricingManager.jsx:103-120/976-984`）；`useUiPreferences` 读写均包 try/catch。`getStoredToken/getStoredUser` 的 `getItem` 未包 try/catch（仅 `JSON.parse` 包了），但只在"存储被策略禁用"这种场景才会抛，影响是整页白屏——列为背景风险，未定级。
- **登录/会话契约**：`POST /auth/login` 既返回 `token` 也 Set-Cookie（`account_handler.go:110-137`），前端 `AuthPage/LoginModal` 要求 `data.data.token` 与后端一致；`GET /auth/me` 返回 `data` 即 user 对象，`useSessionRestore` 已同时兼容 `data.user || data`；OAuth 回调后重新走 `/auth/me`。
- **API 路径契约**：把 `web/src` 里出现的全部 `/api/...` 路径（去重后 60 余条）与 `internal/api/router.go` 逐条比对，**全部存在**（含 `/admin/supplier-costs/*`、`/admin/mcp/servers/:id/toggle`、`/user/wallet/orders/:order_no`、`/skills/:id/download`、`/admin/traces`）；未见调用已删除的旧端点（上一轮 D-07 删除的 `/user/me`、`/admin/skills/:id/download` 等前端也无引用）。
- **金额字段**：`/admin/stats/overview|timeseries`、`/admin/keys`(`apiKeyView`)、`/user/wallet` 的 JSON tag 与前端读取的字段名逐一核对一致（含 timeseries 的**驼峰** `avgTtft`——后端刻意如此）；金额统一是 CNY float（`budget`/`used_cost`/`cost`/`amount`），未发现 units→CNY 的隐式换算点或 `toFixed` 用错；`used_cost_units`/`budget_units` 前端不读，后端另有权威字段。`csvEscape` 已做 RFC 4180 + 公式注入防护。
- **权限分支**：admin/user 分支逐条核对（LogsView/KeysView/PricingManager/Wallet/UserManagement/App 的 `isAdmin` 用法）只发现 BUG-WEB-01 一处不一致；`/admin/channels`、`/admin/models/routes` 对普通用户不发（`useConsoleData.js:96-107`，有测试钉住）。
- **i18n 键完整性**：`i18n.js` 的 zh/en 各 43 个键完全对称；把生产代码里所有 `t.<key>` 使用与字典对比，**无缺失键、无显示键名**（剩下的 5 个假阳性是 `t.name`/`t.trim()` 这类非翻译对象访问）。真正的 i18n 缺陷是 BUG-WEB-06 那种"组件没接入 i18n"。
- **死代码 / 未引用组件**：按任务约定不管（另有 teammate 负责）。

---

## 6. 修复记录（2026-10-28 同日）

**范围**：只改前端（`web/src`），未触碰 Go 后端、未 `npm run build`（`web/dist` 由 Lead 统一重建）、未 commit。

### 6.1 逐条结果

| 编号 | 状态 | 生产改动 | 回归测试（断言修复后行为） |
|---|---|---|---|
| BUG-WEB-01 | ✅ fixed | `lib/navGroups.js:119` 新增 `allowedTabsFor(isAdmin)`（组内 tab + 无组 `docs`）作为**唯一真源**；`App.jsx:460` 角色守卫改读它，未知 tab 回退 `allowedTabs[0]`（角色首个允许 tab，不再硬编码钱包页）；`lib/consoleRoute.js:8,10,30` 的 `USER_TABS`/兜底同样改为读该真源 | `App.roleTabs.test.jsx`：4 例 —— admin 深链保持 `#/dashboard`；普通用户深链 `#/dashboard` 渲染「我的用量」；普通用户点侧边栏「概览」可用；普通用户深链 `#/channels` 回退到 dashboard 且从不请求 `/admin/channels` |
| BUG-WEB-02 | ✅ fixed | `ServiceStatus.jsx:35` 新增 `channelBreakerState(channel)`（`breaker_status` → open/half_open/closed）；`:192` 页面级 `anyTripped` 与 `:532` 行级判定改用该函数；熔断/半开/闭合三态分别渲染「已熔断跳闸」「半开探测中」「健康闭合」 | `ServiceStatus.statusPage.test.jsx`：OPEN 显示熔断且页面报「存在已熔断渠道」；HALF-OPEN 显示「半开探测中」；CLOSED 显示「健康闭合」 |
| BUG-WEB-03 | ✅ fixed | `ServiceStatus.jsx:595`：无探测数据时显示「未探测/Not probed」，删除伪造的「~35 ms (探针正常)」 | 同上文件 2 例：未探测显示「未探测」且无 `~35 ms`；有测量时显示实测 `128 ms` |
| BUG-WEB-04 | ✅ fixed | `TraceSnapshotModal.jsx:21-55`：`requestSeqRef` 序号 + `AbortController` 双守卫（照 `SupplierReconciliationView` 范式），过期响应丢弃、关闭/切换 trace 时 abort；`:60-71` 卸载清理 | `TraceSnapshotModal.race.test.jsx`：先慢后快两查询 ⇒ 最终保留后发结果；关闭弹窗 ⇒ 在途请求 signal 被 abort |
| BUG-WEB-05 | ✅ fixed | `hooks/useModalA11y.js:20-47` 改为模块级引用计数（首个打开记录原值、最后一个关闭还原），新增 `resetBodyScrollLock()` 测试钩子；`:76-80` 接入 | `useModalA11y.scrollLock.test.jsx`：非 LIFO 关闭（先关先开的）后仍锁、最后关解锁；3 个弹窗同批卸载可解锁；还原原 inline 值（`scroll`） |
| BUG-WEB-06 | ✅ fixed | `MarkdownRenderer.jsx` 增 `lang` 入参（`:584`）+ 局部 `MarkdownLangContext`（`:25-29`）供深层 token 渲染读取，代码块按钮/标题/行数文案走 `i18n`（`:251,261,269`）；`App.jsx:91-96` `ViewLoader` 接收 `lang` 并用 `t.loadingModule`（5 处调用点均传 `lang={lang}`）；`i18n.js` 新增 `loadingModule` 与 `md*` 共 8 键 ×2 语言；`AiCopilot.jsx:1303`、`PlaygroundView.jsx:1569/1595/1629/1659` 传 `lang={lang}` | `MarkdownRenderer.i18n.test.jsx`：en 界面无中文（`Copy`/`Copy code`/`Toggle line wrap`/`(2 lines)`）、zh 界面无英文、缺省为 zh；`App.roleTabs.test.jsx` 的「renders the loader in the console language」覆盖 `ViewLoader` |
| BUG-WEB-07 | ✅ fixed | `App.jsx:353-355` `resetWorkspaceState` 补齐 `setDashboardSelectedKey('')`、`setShowAnnouncementModal(false)`，并把账号作用域的日志筛选 `logs.key_id` 与分页游标 `logs.page` 一并复位（`setLogFilters({ key_id: '', page: '1' })`）；已复核 `useConsoleOverlays.reset()`/`useConsoleSelections.reset()`/`useConsoleData.reset()` 覆盖其余会话态 | `App.roleTabs.test.jsx`「does not inherit the previous session's key drill-down or announcement dialog」：admin 下钻 key 11 + 打开公告弹窗 → 登出 → 重新登录 ⇒ 无 `key_id` 请求、无「密钥实时速率遥测」、公告弹窗不自动弹 |

### 6.2 顺带处理的既有用例

- `components/MarkdownRenderer.test.jsx` 的 "does not add a blank line to the code it closes" 原先断言中文界面里出现英文 `3 lines`；语言可配后给该渲染显式传 `lang="en"`（该用例关心的是围栏闭合逻辑，不是语言），断言不变。

### 6.3 明确没做的（避免超范围）

- **未** 把 `channelBreakerState` 提升到 `lib/` 与 `ChannelsView.channelHealth()` 合并：两者语义已一致（都读 `breaker_status`），但合并会同时改动渠道页的健康排序/筛选逻辑，超出本轮 7 条范围；建议作为后续小重构（报告 §6.4 记一笔）。
- **未** 处理 §3 的 4 条待确认（T-1…T-4）：T-1/T-2 需产品口径，T-3（全站无 ErrorBoundary）是跨页面兜底能力，T-4 是体验问题。
- **未** 动 `web/web.go`：静态服务/ETag/SPA fallback 本轮核对无缺陷。

### 6.4 验证（修复后，2026-10-28 本机）

```
cd web && npm run lint  -> 0 warning
cd web && npm test      -> 56 files / 685 tests passed
```

- 基线 51 文件 / 666 用例；本轮新增 5 个回归文件 / 19 个用例（6+5+2+3+3），并翻转了原 5 个 `*__bugprobe` 探针（探针文件已删除，改为下表 5 个正式用例文件）。
- 每条回归用例都对应修复前的错误行为，**在修复前代码上会失败**（排查阶段的探针版本恰好断言了相反的结果且当时全绿，即为此证明）。
- 未运行 `npm run build`；`web/dist` 未变。

## 7. 附：本轮核对过但未发现问题的关键文件

`App.jsx`（除上述两处）、`api/client.js`、`api/session.js`、`hooks/`（`useApiSession`、`useSessionRestore`、`useConsoleSession`、`useConsoleData`、`useLogsData`、`useChannelActions`、`useKeyActions`、`useUrlFilters`、`useRoutingTopology`、`useTableControls`、`useFormDirty`、`useUnsavedGuard`、`useToast`、`useConsoleOverlays`、`useConsoleSelections`、`useConsoleChrome`、`useUiPreferences`、`useModalA11y`←BUG-WEB-05）、`lib/`（`apiKey`、`consoleRoute`、`escapeLayer`、`keyModels`、`modelModality`、`navGroups`←BUG-WEB-01 引用、`providerPresets`）、`components/`（下列均已通读并核对契约）：`LogsView`、`LogDetailModal`、`KeysView`、`KeyModal`、`ChannelModal`、`ChannelsView`、`PricingManager`、`ModelRoutesManager`、`ModelRouteModal`、`UserManagementView`、`WalletManagementView`、`SupplierReconciliationView`、`SupplierCostPanel`、`PlaygroundView`、`AiCopilot`、`copilotHarness`、`McpIntegrationView`、`SkillHubView`、`CommandPalette`、`AccountManageModal`、`AuthPage`、`LoginModal`、`LandingPage`、`DocsView`、`AnnouncementBanner`、`AnnouncementModal`、`TraceSnapshotModal`（BUG-WEB-04）、`SystemLogConsole`、`ServiceStatus`（BUG-WEB-02/03）、`TableControls`、`Toast`、`config.js`、`utils/clipboard.js`、`web/web.go`（embed/缓存头/ETag/SPA fallback 均无问题）。
