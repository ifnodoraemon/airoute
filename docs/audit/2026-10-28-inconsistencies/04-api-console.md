# API/控制面/前端契约一致性审计（2026-10-28）

## 概览

审计基线 commit `1520d99`（域：`internal/api/`、`internal/controlplane/`、`web/src/`、`docs/API_REFERENCE.md`）。共 14 项发现，全部为高置信度；按影响分级：中 5 项、低 9 项。审计期间 HEAD 前进到 `190b382`（并行同伴提交，只改 `docs/CONFIGURATION.md` 与 `internal/middleware/authz.go` 注释，不影响本报告任一条证据），工作区另有多处并行改动尚未提交；本报告的行号与结论一律以 `1520d99` 为准。

最值得先看的三项：**D-01** 渠道列表把上游供应商的 `api_key` 明文回传（同一份代码里客户端密钥已改为 digest + `ak_`，渠道密钥仍是明文，两种凭证两套表示）；**D-02** D15 控制面守卫只按路径判定 `/api/v1/admin` 与 `/app`，但同一份 console shell 又被公开挂在 `/` 与 `/assets`，所以「控制面默认只允许私网」在路径维度上是漏的；**D-03/D-04** 1520d99 修好了 call log/traces 的 digest→`ak_` 映射，但同类读路径仍有两处未覆盖：MCP 的 `airoute_query_logs` 直接返回 `usage_logs` 原始行（含 digest），`/stats/overview|timeseries` 的遗留 `key` 参数在管理员与非管理员之间要求的表示方式还不同。

其余集中在「能力接了一半」：MCP 两个内置工具实现了 handler 却不在 `tools/list` 清单里；MCP「是否管理员」存在两套判定函数且能力不同（SQL 工具在 stdio 通路永远被拒）；`format_validation` 字段请求/存储/回传全通但没有任何代码计算或显示；`/admin/system/middlewares` 是 ROADMAP 里为「控制台自检页」预留的、页面至今没建；前端共享的 `keyDisplayRef` 无人调用、7 个文件里 8 处各写一遍同样的 `key_id || #id`。

裁剪（2026-10-28 v1.1）本身很干净：路由表、handler 文件、`web/src` 导航/路由/i18n、`docs/API_REFERENCE.md` 均未发现 org/budget/ledger/cost 残留（详见排除区）。

## 发现

### D-01 渠道列表把上游供应商 `api_key` 明文回传，且前端只用它推断「是否已配置」
- **类别**：两套表示漂移 / 不对称的密钥处理
- **位置**：`internal/controlplane/channel_service.go:74`、`internal/controlplane/admin_channels.go:43`、`internal/storage/repo_channels.go:44-49`、`internal/storage/repo_channels.go:63-64`；前端 `web/src/hooks/useChannelActions.js:273`、`web/src/lib/providerPresets.js:24-31`
- **设计意图**：客户端 API 密钥这条路已经明确「只在创建时回一次明文，其余一律只给 `ak_` 公开 ID」（`internal/storage/repo_keys.go:26-30`、`internal/controlplane/key_view.go:11-14`、`docs/API_REFERENCE.md:347-356`）。渠道（上游供应商）密钥在同一套设计下被当成「一次写入、之后不再查看」的配置：前端 `providerPresets.js:26-28` 明说「表单从不携带已有凭证，`api_key` 只放本次会话输入的值，`api_key_configured` 表示渠道是否已配置」，`useChannelActions.js:269-273` 因此在编辑时把 `api_key` 清空、只留一个布尔标记。
- **现状**：`ListChannels` 把 `storage.ChannelRecord` 原样序列化返回，`APIKey` 字段的 tag 是 `json:"api_key"`，值是数据库里的明文（`repo_channels.go:49` + `:64` 的 `SELECT … api_key …`）。控制台拿到明文后只做 `Boolean(ch.api_key)`，从不显示；而 `api_key_configured` 这个前端假定的字段在后端根本不存在（全仓 Go 代码零命中），是前端自己用明文存在性算出来的。也就是说，为了让前端知道「有没有配置」，后端每次都把供应商的长期凭证推给了浏览器。
- **证据**：
```go
// internal/storage/repo_channels.go:44-49
type ChannelRecord struct {
	ID             int64              `json:"id"`
	Name           string             `json:"name"`
	APIKey         string             `json:"api_key"`   // 明文供应商密钥
```
```go
// internal/controlplane/admin_channels.go:43-50
func (h *AdminHandler) ListChannels(c *gin.Context) {
	channels, err := h.channels.List()   // 无脱敏，直接序列化
	...
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": channels})
```
```js
// web/src/hooks/useChannelActions.js:269-273（编辑时）
// an unchanged field must not round-trip the secret. `api_key_configured`
api_key: '',
api_key_configured: Boolean(ch.api_key),   // ← 用明文存在性推断,而非后端字段
```
- **影响**：中 — 上游供应商凭证在每次渠道列表/创建/更新响应里都随响应体回到浏览器，任何能读控制台响应的人（包括排障时截图/粘贴响应、抓包、浏览器扩展）都拿到了可直接冒用的上游 key；这与客户端密钥「只回一次明文、其余只给 `ak_`」的既定口径不一致。
- **置信度**：高
- **建议**：渠道读路径改为 `api_key_configured: bool` + 掩码（`sk-…***`），明文只允许在创建/显式轮换的响应里出现一次，并让前端直接读后端这个字段。

### D-02 D15 控制面守卫按路径只挡 `/api/v1/admin` 与 `/app`，但同一份 console shell 公开挂在 `/` 与 `/assets`
- **类别**：不对称鉴权
- **位置**：`internal/api/router.go:92`、`internal/middleware/admin_access.go:50-51,78-86`、`web/web.go:78,85,90-91`；`internal/api/admin_guard_wiring_test.go:44-50`
- **设计意图**：守卫注释写得很明确：「the control plane (`/api/v1/admin` and the embedded console under `/app`) is restricted to loopback/private clients … Host-header routing in nginx is not a security boundary: the guard is enforced inside the gateway, behind every proxy, so the default catch-all vhost can no longer expose it」（`router.go:87-92`）。`docs/CONFIGURATION.md:125` 同口径：只有显式设置 `GATEWAY_ADMIN_ALLOW_PUBLIC` 才「Explicitly open the control plane to public clients (not recommended)」。守卫自测也只断言了 `/app/` 与 `/api/v1/admin/keys` 被拒（`admin_guard_wiring_test.go:44-50`）。
- **现状**：`isControlPlanePath()` 只匹配 `/api/v1/admin` 与 `/app` 前缀。但嵌入的 SPA 入口是**同一个** `serveIndex`（同一份 `index.html`）：`/` 与 `/app/` 都渲染它，静态包同时挂在 `/assets` 与 `/app/assets`。于是公网客户端可以直接 `GET /` 拿到控制台 HTML、`GET /assets/*.js` 拿到控制台 bundle（`AdminAccessGuard` 对这两条路径直接 `c.Next()`）；只有 `/app/*` 被 403。同一批「控制面会话路由」也不在谓词内：`/api/v1/auth/*`（`router.go:145-156`）与 `/api/v1/user/*`（`router.go:168-183`，包含钱包、密钥、改密）全部可公网访问。数据面 `/api/v1/admin` 的守卫仍在，所以这不是数据泄漏，而是「控制面不可达」这个承诺在路径维度上不成立。
- **证据**：
```go
// internal/middleware/admin_access.go:78-86
func isControlPlanePath(path string) bool {
	switch {
	case path == "/api/v1/admin", strings.HasPrefix(path, "/api/v1/admin/"):
		return true
	case path == "/app", strings.HasPrefix(path, "/app/"):
		return true
	}
	return false
}
```
```go
// web/web.go:78,85,90-91 —— 同一个 serveIndex / 同一份 assets
	r.GET("/", serveIndex)          // 公网可达（谓词未覆盖 "/"）
	r.GET("/app/", serveIndex)      // 受 D15 守卫
	static.StaticFS("/assets", http.FS(assetsSub))       // 公网可达
	static.StaticFS("/app/assets", http.FS(assetsSub))
```
```go
// internal/api/router.go:168-183 —— 会话型控制面路由，不在守卫谓词内
userGroup := r.Group("/api/v1/user")
userGroup.Use(adminHandler.AdminAuthMiddleware())
```
- **影响**：中 — 别处（01 域 A-01）已指出守卫在反代拓扑下因 IP 取值失真而恒放行；本条是正交的第二层缺口：即使 IP 判定正确，`/`、`/assets`、`/api/v1/user/*`、`/api/v1/auth/*` 也不在「控制面」谓词里。后果是「控制面默认只允许私网/回环」这条承诺在路径维度上不成立：公网可下载控制台 bundle（暴露全部 admin 路由与参数名），并可对 `/api/v1/user/*` 发起带会话 cookie 的写操作（CSRF 已在 `auth.go:84` 对 cookie 认证的写请求做双提交校验，所以这里不是 CSRF 洞，而是「守卫覆盖面与文档承诺不一致」）。
- **置信度**：高
- **建议**：把「控制面路径」定义收敛成一份清单（含 `/`、`/assets`、`/api/v1/user`、`/api/v1/auth`），或反过来只公开真正的数据面/落地页；同时让 `/` 的落地页与 console 入口分离。

### D-03 MCP `airoute_query_logs` 直接返回 `usage_logs` 原始行，含密钥 digest
- **类别**：两套表示漂移（1520d99 同类读路径未覆盖）
- **位置**：`internal/api/mcp_tools_ops.go:108,153-160`、`internal/storage/repo_logs.go:141`；对照组 `internal/controlplane/admin_logs.go:116,226`
- **设计意图**：1520d99 明确「audit 列继续存 digest 以保住 ledger join，但读路径要把 digest 解析成 `ak_` 公开标识，只有解析不到的历史 digest 才继续掩码」；access log 同样优先输出 `ak_`（`internal/middleware/trace.go:191-199`）。所以「控制台/操作者看到的 key」应统一是 `ak_…`，digest 属于内部表示。
- **现状**：MCP 工具 `airoute_query_logs` 只做了租户隔离（按调用者 key + tenant 过滤，`mcp_tools_ops.go:139-152`），随后把 `[]*storage.UsageLogRecord` 直接 `json.MarshalIndent` 返回。`UsageLogRecord.APIKey` 的 tag 是 `json:"api_key"`，值是 digest（`repo_logs.go:141`），既没有 `presentLogKeys` 也没有掩码。这是 1520d99 修完后**唯一还在向 agent 侧输出原始 digest 的日志读路径**（同一实体：HTTP 控制台给 `ak_`，MCP 给 64 位 digest）。
- **证据**：
```go
// internal/api/mcp_tools_ops.go:155-159
logs, err := h.repo.ListUsageLogsWithFilter(filter)   // 原始记录
if err != nil { ... }
b, _ := json.MarshalIndent(logs, "", "  ")            // 无 presentLogKeys / 无掩码
return string(b), false, true
```
```go
// internal/storage/repo_logs.go:141
APIKey           string                `json:"api_key"`   // digest
```
```go
// internal/controlplane/admin_logs.go:116 —— 已修的那条读路径
h.presentLogKeys(logs, shouldRevealKey(c))
```
- **影响**：中 — agent 侧拿到的 key 标识与密钥列表/调用日志/access log 对不上，无法把 MCP 查到的行与密钥对上；与「List responses contain neither the credential nor its digest」（`docs/API_REFERENCE.md:354`）的口径也冲突（MCP 不是 list，但同样是读路径）。
- **置信度**：高
- **建议**：`airoute_query_logs` 复用 `presentLogKeys`（或抽一个 storage 层的「digest→`ak_`/掩码」投影）后再序列化，并在 MCP 工具描述里写明返回的 `api_key` 是公开 ID。

### D-04 `/stats/overview` 与 `/stats/timeseries` 的遗留 `key` 参数：管理员与非管理员要求不同表示，前端与文档都不用它
- **类别**：两套表示漂移
- **位置**：`internal/controlplane/admin_overview.go:50,139`、`internal/storage/repository.go:189`、`internal/storage/repo_logs_writer.go:52-60`、`internal/storage/repo_timeseries.go:136`；前端 `web/src/components/DashboardView.jsx:92-104`
- **设计意图**：`admin_overview.go:17-27` 的注释说明控制台下钻**只走数字 `key_id`**：「The console drills into a key by its numeric id (`key_id`), because the key list exposes the public identifier and never the stored digest… the old `key=` filter was fed the masked value, which hashed to a digest that matched no row」。`docs/API_REFERENCE.md:356` 也只承认 `key_id`（数字行 id）这一个下钻参数。
- **现状**：两个 stats handler 仍各自读一个遗留 `key` 查询参数，而它的表示方式取决于角色：
  1. **非管理员分支**先把它与 `ownedMap[k.Key]`（digest）做**字符串全等比较**（`admin_overview.go:78-84`），所以只能传 64 位 digest，传 `ak_…` 或掩码一律 403；
  2. **管理员分支**把参数直接交给 `GetStatsOverviewForKeys`，后者逐项 `NormalizeStoredAPIKeyReference`：非 digest 形状的值会被 `HashAPIKey` 一次（`repo_logs_writer.go:56-59`），于是传**明文凭证**能命中，传 `ak_…` 会被哈希成无意义串并**静默返回全 0 统计**（无 404/400）。
  控制台只发 `key_id`（数字），CLI、`docs/API_REFERENCE.md` 都不用 `key`；唯一说明过「`key` 曾是掩码→digest 匹配不到」的注释也说明它已被废弃，但代码没有删。
- **证据**：
```go
// internal/controlplane/admin_overview.go:50
keyParam := strings.TrimSpace(c.Query("key"))     // 遗留参数,无客户端
// :78-84（非管理员）
if !ownedMap[keyParam] {                          // 与 digest 全等比较
	c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "无权查看该密钥指标"})
```
```go
// internal/storage/repo_logs_writer.go:52-59
func NormalizeStoredAPIKeyReference(value string) string {
	if IsAPIKeyHash(value) { return value }
	return HashAPIKey(value)     // 明文会被哈希; ak_… 会被哈希成不存在的 digest
}
```
- **影响**：中 — 同一个 `key` 参数在两个端点、两个角色下语义不同（digest vs 明文），且非法值静默变成「0 流量」，正是 1520d99 注释里描述过的故障模式；`ak_…` 这条「操作者唯一可见的标识」在这里最不工作。
- **置信度**：高
- **建议**：删除遗留 `key` 参数（只留 `key_id`），或在入口统一复用 `APIKeyDigestByPublicID` 支持 `ak_…`，并对无法解析的值返回 400 而不是 0 统计。

### D-05 MCP「调用者是否管理员」有两套实现，SQL 工具在 stdio 通路永远被拒
- **类别**：不对称鉴权 / 两套实现
- **位置**：`internal/api/mcp_tools.go:54`（`callerIsAdmin`）、`:115`（`mcpCallerIsAdmin`）、`:91`（`callerIsAdminForKey`）；调用点 `internal/api/mcp_tools.go:255,314,355`（SQL 工具）与 `internal/api/mcp_handler.go:370`（`adminOnlyMCPTools`）
- **设计意图**：MCP 有两条进入方式——HTTP（`/mcp/messages`、`/mcp/sse` 已把调用者写进 `contextKeyCallerKey`/`contextKeyCallerRecord`）与进程内 stdio（`ProcessRPC` 注释：「public for stdio CLI adapter reuse」）。`mcpCallerIsAdmin` 的注释明确说它要「prefers the caller the transport already resolved … and falls back to the historical key sources so direct in-process invocations (stdio adapter, tool arguments) cannot slip past the check」——即设计上两套入口都必须能判定管理员。
- **现状**：两个函数的能力并不一致：
  - `mcpCallerIsAdmin`（用于 `airoute_cluster_status` / `airoute_model_topology` 两个 infra 披露工具）在 context 缺失时会回退读 `args["api_key"]`，并再经 `callerIsAdminForKey`（会用 `UserRole` 或回查 users 表）；
  - `callerIsAdmin`（用于 `read_query`/`list_tables`/`describe_table` 三个 SQL 反射工具）**只**读 `ctx.Value(contextKeyCallerKey)`，没有任何参数回退。
  结果是：stdio 适配器里哪怕显式传了管理员的 `api_key`，`cluster_status` 能过、`read_query` 恒返回「Security violation: SQL reflection tools are restricted to gateway administrators」。同一份「管理员判定」在同一个包里有两个答案。
- **证据**：
```go
// internal/api/mcp_tools.go:54-60 —— 只有 context,无 args 回退
func (h *MCPHandler) callerIsAdmin(ctx context.Context) bool {
	key, _ := ctx.Value(contextKeyCallerKey).(string)
	if strings.TrimSpace(key) == "" { return false }
```
```go
// internal/api/mcp_tools.go:115-135 —— 有 args["api_key"] 回退
func (h *MCPHandler) mcpCallerIsAdmin(ctx context.Context, args map[string]interface{}) bool {
	if caller := callerFromContext(ctx); caller != nil { ... }
	if strings.TrimSpace(key) == "" && args != nil { key, _ = args["api_key"].(string) }
	return h.callerIsAdminForKey(key)
}
```
```go
// internal/api/mcp_tools.go:255（read_query）vs internal/api/mcp_handler.go:370（infra 工具）
if !h.callerIsAdmin(ctx) { ... }                                   // 无回退
if isAdminOnlyMCPTool(name) && !h.mcpCallerIsAdmin(ctx, args) { ... } // 有回退
```
- **影响**：中 — 功能上 stdio 管理员无法使用三个 SQL 工具（对外文档还写着「Administrator API key required」）；安全上两套判定意味着「哪些工具受管理员约束」这件事有两个真理来源，后续加工具时很容易挂错判定。
- **置信度**：高
- **建议**：合并成一个 `callerIsAdmin(ctx, args)`（或统一走 `mcpCallerIsAdmin`），SQL 工具也接上 args 回退；把「管理员专属工具」收敛成一张表而不是两个函数 + 一个 map。

### D-06 两个内置 MCP 工具实现了 handler 与分发，但不在 `tools/list` 清单里
- **类别**：契约不一致 / 半接线
- **位置**：`internal/api/mcp_schemas.go:26`（工具清单，24 个）、`internal/api/mcp_handler.go:285`（`tools/list` 只发清单）、`internal/api/mcp_tools.go:214,235`（`airoute_get_current_time` / `airoute_calc_eval` 的 handler）
- **设计意图**：`tools/list` 是 MCP 客户端发现工具的唯一入口（`mcp_handler.go:285-330`：`allTools := getBuiltinMCPTools()`，再按 `IsToolEnabled` 过滤、再追加远程 server 的工具）；工具清单的注释把它定义为「declarative tool schemas supported natively by Airoute gateway」（`mcp_schemas.go:25`）。品牌迁移记录也把这两个工具当作现役内置工具（`.opencode/memory/project.md:25`：「时间/计算两个内置工具改为 airoute_get_current_time、airoute_calc_eval」）。
- **现状**：清单里共 24 个工具，既没有 `airoute_get_current_time` 也没有 `airoute_calc_eval`；两者却都有完整的 `case` 分支实现与单测（`mcp_tools.go:214,235`）。由于 `tools/call` 的启用判定对任何 `airoute_` 前缀直接放行（`repo_skills.go:145-147`），它们**能被调用**但**无法被发现**——客户端只能靠猜名字。反过来看清单里另外 8 个（`puppeteer_*`、`search_repositories`、`create_issue`、`get_file_contents`、`create_pull_request`）没有本地 handler，但那是设计内的「广场转发」：`executeDynamicMCPTool` 按已启用 MCP server 的 `Tools` 名单转发，种子数据里也确实注册了这些 server（`internal/storage/repo_mcp_seed.go:51,86`）——这两类问题不是同一回事，前者是漏登记，后者是设计。
- **证据**：
```go
// internal/api/mcp_schemas.go:26（清单 24 项，无 current_time / calc_eval）
func getBuiltinMCPTools() []gin.H { return []gin.H{
	tool("airoute_search_skills", ...), ... tool("create_pull_request", ...) } }
```
```go
// internal/api/mcp_tools.go:214,235 —— 有实现,无清单
case "airoute_get_current_time": ...
case "airoute_calc_eval": ...
```
```go
// internal/storage/repo_skills.go:145-147 —— 因此调用仍被放行
func (r *Repository) IsToolEnabled(toolName string) bool {
	if strings.HasPrefix(toolName, "airoute_") { return true }
```
- **影响**：低 — 两个工具对 MCP 客户端事实上不存在（`tools/list` 不出现），只有知道名字的人能用；清单与实现之间没有任何一致性校验，后续再漏一个也不会有测试失败。
- **置信度**：高
- **建议**：把两个 `tool(...)` 条目补进清单；再加一条测试断言「`executeBuiltinUtilityTools` 的每个 `airoute_*` case 都必须出现在 `getBuiltinMCPTools()` 里」。

### D-07 四个已注册端点全仓无任何调用方（控制台/CLI/文档都没有）
- **类别**：死端点
- **位置**：`internal/api/router.go:171`（`GET /api/v1/user/me`）、`:177`（`GET /api/v1/user/wallet/orders`）、`:181`（`DELETE /api/v1/user/keys/:id`）、`:207`（`GET /api/v1/admin/skills/:id/download`）
- **设计意图**：控制面路由表是唯一注册点（`router.go`，全仓无第二处 gin 注册，`web/web.go` 只挂静态资源）。会话统一入口已有 `/api/v1/auth/me`（`router.go:154`，注释：「Exchanges the one-time code from the browser redirect for a session」），密钥删除在控制台统一走 `/api/v1/admin/keys/:id`（带 ownership 校验，`admin_keys.go:173`），技能包下载已有公开的 `/api/v1/skills/:id/download`（`router.go:162`，`SkillHubView.jsx:500` 用的就是它）。
- **现状**：这四条没有对应调用方：
  - `user/me` 与 `auth/me` 是同一个 handler（`GetMe`），前端只调 `/api/v1/auth/me`（`useSessionRestore.js:45`、`App.jsx:267`）。
  - `user/wallet/orders` 是**列表**，前端只查单个订单 `/orders/:order_no`（`WalletManagementView.jsx:144`）。
  - `DELETE /api/v1/user/keys/:id` 无调用方；普通用户删密钥时 `useKeyActions.js:155` 走的是 `/api/v1/admin/keys/${id}`（该路由对非管理员按 owner 校验，能正常工作）。
  - `GET /api/v1/admin/skills/:id/download` 与公开下载路由重复，前端与 CLI 都用公开那条。
  `docs/API_REFERENCE.md` 一条也没有收录（第 4 节只列 channels/keys/logs/stats/models/supplier-costs）。
- **证据**：
```go
// internal/api/router.go
171: userGroup.GET("/me", adminHandler.GetMe)                    // 与 :154 的 auth/me 重复
177: userGroup.GET("/wallet/orders", adminHandler.ListUserRechargeOrders)  // 前端只用 /orders/:order_no
181: userGroup.DELETE("/keys/:id", adminHandler.DeleteUserKey)   // 前端走 /admin/keys/:id
207: protected.GET("/skills/:id/download", adminHandler.DownloadSkillZip) // 与 :162 公开路由重复
```
```bash
# 全仓（web/src 非测试 + cmd/ 非测试 + docs）grep 结果为空
$ grep -rn "user/me" web/src cmd docs/API_REFERENCE.md   # 0 命中
```
- **影响**：低 — 多一套无客户端的对外表面，且 `user/keys` 的删除路径与 `admin/keys` 的删除路径并存会让读代码的人误以为有两条语义不同的删除流程。
- **置信度**：高
- **建议**：删除四条路由；若 `user/keys` 想保留「用户侧 API」，则与 `admin/keys` 的 ownership 规则明确合并后再对外文档化。

### D-08 `/admin/system/middlewares` 是 ROADMAP 里为「控制台自检页」预留的端点，页面至今没建
- **类别**：死端点（计划未接线）
- **位置**：`internal/api/router.go:217`、`internal/controlplane/admin_overview.go:210`、`docs/ROADMAP.md:119`
- **设计意图**：ROADMAP B-07 明确把控制台「自检报告」页与 `/admin/system/middlewares`、`/health/ready` 绑在一起：「控制台「自检报告」页（复用 `/admin/system/middlewares` 与 `/health/ready`）、一键 e2e 验收」。也就是说这个端点存在的唯一目的就是给那个页面供数。
- **现状**：后端是一个约 100 行的完整实现（数据库/Redis/限流/熔断等组件状态与延迟探测，`admin_overview.go:210` 起），并挂在 `adminOnly` 组里；但控制台没有对应视图（`web/src` 里 `middlewares` 零命中，lazy 组件清单里也没有自检页），ROADMAP 把 B-07 列在计划中而非已完成，`docs/API_REFERENCE.md` 也未收录。端点可用但无人使用——典型的「先建了端点、页面没跟上」。
- **证据**：
```go
// internal/api/router.go:217
adminOnly.GET("/system/middlewares", adminHandler.GetSystemMiddlewares)
```
```markdown
<!-- docs/ROADMAP.md:119（B-07 仍在计划栏） -->
| **B-07** | **接入验收工具链** | … 控制台「自检报告」页（复用 `/admin/system/middlewares` 与 `/health/ready`）、一键 e2e 验收；交付即带验收证据 | M |
```
```bash
$ grep -rn "middlewares|自检" web/src --include=*.jsx --include=*.js   # 0 命中
```
- **影响**：低 — 一个维护面可观的 admin 端点没有消费者；若 B-07 不做，应该删；若要做，它现在是唯一已经存在的部分。
- **置信度**：高
- **建议**：在 ROADMAP 里给 B-07 明确取舍；短期在端点注释里标注「为 B-07 预留、暂无消费者」，避免下次审计重复判定。

### D-09 `format_validation` 字段全链路贯通，但没有任何代码计算或显示它
- **类别**：契约不一致 / 裁剪残留
- **位置**：请求 `internal/controlplane/admin_keys.go:88`、`internal/controlplane/key_update.go:22,102`；存储 `internal/storage/repo_keys.go:42`（迁移 `add_format_validation_to_api_keys`）；响应 `internal/controlplane/key_view.go:34,60`
- **设计意图**：字段名与迁移名（`format_validation`）表明它原计划承载「密钥格式校验结果」（例如把 `sk-airoute-…` 格式不合法/带空格的 key 标出来），并在密钥列表里提示操作者。
- **现状**：它被创建请求接受、被更新请求接受（指针语义，显式写入）、被持久化、被 `apiKeyView` 序列化返回——但全仓没有任何代码**写入非空值**或**读取**它：没有格式校验函数，没有前端展示（`web/src` 里 `format_validation` 零命中），`CreateAPIKeyRequest.toRecord` 只是搬运。数据库列默认 `''`，所以线上它恒为空串、并从响应里被 `omitempty` 省略。一个横跨「请求→存储→响应」三层的字段，功能是空的。
- **证据**：
```go
// 请求侧：两个请求结构都接受它
admin_keys.go:88:      FormatValidation string   `json:"format_validation"`
key_update.go:22:      FormatValidation *string  `json:"format_validation"`
// 存储侧
repo_keys.go:42:       FormatValidation string    `json:"format_validation,omitempty"`
// 响应侧
key_view.go:34:        FormatValidation string    `json:"format_validation,omitempty"`
```
```bash
# 全仓除「搬运」外无消费者：无校验逻辑、无前端引用
$ grep -rn "FormatValidation|format_validation" --include=*.go --include=*.js --include=*.jsx . | grep -v "_test.go" | grep -v "migrations_"
# 仅 repo_keys.go(读/写列)、admin_keys.go(请求字段)、key_update.go(赋值)、key_view.go(序列化)
```
- **影响**：低 — API 看起来支持一个能力，实际是恒空的装饰字段；接入方若依赖它做校验会一直拿到空值。
- **置信度**：高
- **建议**：要么实现校验并在密钥列表显示，要么从请求/响应/迁移里一起删掉（含 `UpdateAPIKeyRequest.FormatValidation`）。

### D-10 MCP 协议版本在两个发现面上不一致（`/v1` 说 2024-11-05，`/mcp` 说 2026-07-28）
- **类别**：两套表示漂移（文档/发现面）
- **位置**：`internal/api/ops_handler.go:138`、`internal/api/mcp_info.go:33-34`、`internal/api/mcp_handler.go:18,40,43`
- **设计意图**：`mcp_handler.go:40` 定义 `mcpSupportedVersions = {"2026-07-28","2025-11-25","2024-11-05"}`、默认 `2026-07-28`（`:43`），`server/discover`/`initialize` 都按此协商；`mcp_info.go:33-34` 也对外声明 `"protocol": "mcp/2026-07-28"` + `supported_versions`。三个发现面应对同一份能力给出同一答案。
- **现状**：`GET /v1`（`HandleV1Info`，供 SDK/工具探测 Base URL 用）里的协议清单仍写着 `Model Context Protocol (MCP 2024-11-05) (/v1/mcp, /v1/mcp/sse, /v1/mcp/messages)`；`mcp_handler.go:18` 的类型注释也还写着「implements the Model Context Protocol (MCP 2024-11-05)」。同一个进程、同一次部署，`/v1` 告诉客户端「只支持 2024-11-05」，`/mcp` 告诉客户端「2026-07-28 + 三版协商」。
- **证据**：
```go
// internal/api/ops_handler.go:138
"Model Context Protocol (MCP 2024-11-05) (/v1/mcp, /v1/mcp/sse, /v1/mcp/messages)",
```
```go
// internal/api/mcp_info.go:33-34
"protocol":           "mcp/2026-07-28",
"supported_versions": []string{"2026-07-28", "2025-11-25", "2024-11-05"},
```
- **影响**：低 — 客户端若以 `/v1` 的发现结果做协议判定，会降级到旧版；排障时也会被误导。
- **置信度**：高
- **建议**：`/v1` 的协议清单改为从 `mcpSupportedVersions`/`mcpDefaultProtocolVersion` 渲染，避免下一次版本升级再漏改。

### D-11 `PUT /admin/keys/:id` 是唯一返回裸 `storage` 记录而不是 `apiKeyView` 的 key 端点
- **类别**：两套表示漂移
- **位置**：`internal/controlplane/admin_keys.go:251`、`internal/controlplane/key_view.go:11-14,40`
- **设计意图**：`key_view.go:11-14` 明确记录了这个决定：「storage.APIKeyRecord also carries the stored digest. Handing the row straight to `c.JSON` meant the digest left the process and had to be masked on the way out, and every new column became an API field by accident. The view names its fields explicitly…」。列表（`admin_keys.go:34,61`）、用户侧（`user_keys_handler.go:28,78`）、创建（`admin_keys.go:138`）都改用了 view。
- **现状**：`UpdateAPIKey` 仍然 `c.JSON(..., "data": outcome.Record)` 直接把 `*storage.APIKeyRecord` 序列化出去。当前没有泄漏（`APIKeyRecord.Key` 的 tag 是 `json:"-"`，`repo_keys.go:26-30`），所以这条是**结构性的**：这个响应里出现哪些字段由 storage 的 tag 决定，而不是由 view 决定——storage 以后加一个带 json tag 的列（或去掉某个 `json:"-"`），这个端点会立刻把新字段/摘要推给控制台，而所有其它 key 端点不会。
- **证据**：
```go
// internal/controlplane/admin_keys.go:251
c.JSON(http.StatusOK, gin.H{"code": 0, "data": outcome.Record, "message": "API 密钥已成功更新"})
```
```go
// internal/controlplane/key_view.go:11-14（既定口径）
// …The view names its fields explicitly and exposes the public identifier (key_id…)
// instead of any credential material.
```
- **影响**：低 — 现在无实际泄漏（摘要字段是 `json:"-"`），但同一实体在 create/list 与 update 之间走了两条序列化路径：`contract_test.go` 只钉住**请求**字段集合、并靠 `json:"-"` 兜住摘要（`:192`、`:334` 的注释「no response may serialize it (see apiKeyView)」），没有任何测试钉住「响应必须经过 view」，所以这条漂移不会被测试发现。
- **置信度**：高
- **建议**：`UpdateAPIKey` 改为 `view := apiKeyViewFrom(outcome.Record)` 后返回。

### D-12 前端共享辅助 `keyDisplayRef` 无任何生产调用方，7 个文件里 8 处各写一遍 `key_id || #id`
- **类别**：前端死代码 / 半接线
- **位置**：`web/src/lib/apiKey.js:176`；重复实现 `web/src/components/KeysView.jsx:159,225`、`MyUsageView.jsx:137`、`PlaygroundView.jsx:1017`、`DashboardView.jsx:989`、`AiCopilot.jsx:1379`、`OnboardingModal.jsx:364`、`web/src/hooks/useKeyActions.js:168`
- **设计意图**：`keyDisplayRef` 的注释把它定位成「Renders the public identifier of a listed key: its `key_id` when the API provides one, otherwise its row number as `#<id>`. Never returns a credential」，并放在 `apiKey.js` 这个「所有控制台界面共用」的模块里（文件头注释：「shared by every console surface…」）。设计上它应该被所有展示 key 的地方调用。
- **现状**：它只在 `web/src/lib/apiKey.test.js:79-83` 被引用，生产代码零调用；同时 7 个文件里共有 8 处各自手写同一表达式（其中 `KeysView.jsx:159` 还额外包了一层命名函数 `keyIdentifier`，同文件 `:225` 又独立写了一行）。表达式分两种：拼 `key_id`，缺失时回退成「井号 + 数字 id」；`useKeyActions.js:168` 是给 toast/确认文案用的同一逻辑。共享辅助与实现同时存在，但没接线。
- **证据**：
```js
// web/src/lib/apiKey.js:176 —— 共享辅助（仅被自己的测试引用）
export function keyDisplayRef(key) {
  if (key?.key_id) return key.key_id;
  if (key?.id != null && key.id !== '') return `#${key.id}`;
  return '—';
}
```
```js
// 8 处手写同样的逻辑（示例三处）
web/src/components/KeysView.jsx:159       const keyIdentifier = (k) => k.key_id || `#${k.id}`;
web/src/components/MyUsageView.jsx:137    {key.key_id || `#${key.id}`}
web/src/hooks/useKeyActions.js:168        const keyRef = k.key_id || `#${k.id}`;
```
- **影响**：低 — 展示口径目前一致，但每加一个 key 展示面就多一份拷贝；`keyDisplayRef` 已经承载了「绝不能把凭证当标识显示」这条约束（`apiKey.js` 文件头），不接线等于这条约束只对 0 个调用者生效。
- **置信度**：高
- **建议**：这 8 处统一改用 `keyDisplayRef`，`KeysView.keyIdentifier` 改为直接引用它；`test` 里保留断言。

### D-13 `docs/API_REFERENCE.md` 缺 `/v1/responses`（README 明确宣传）、`/v1/moderations`、`/mcp`
- **类别**：文档/契约不一致
- **位置**：`docs/API_REFERENCE.md:17-135`（数据面章节）；对照 `README.md:42`、`docs/ROADMAP.md:32`、`internal/api/router.go:20,23,135-140`
- **设计意图**：`API_REFERENCE.md` 自述是「Airoute API Reference Specification」，第 2/3 节逐个列出数据面 ingress（chat、completions、messages、count_tokens、gemini、images、speech、transcriptions、translations、videos、embeddings、rerank），第 4 节列控制面端点——它是「注册了哪些对外端点」的权威清单。
- **现状**：路由表注册了 `POST /v1/responses`（`router.go:20`）、`POST /v1/moderations`（`:23`）、`GET /v1/models`（`:21`）与 MCP 入口（`:135-140`），README 与 ROADMAP 都向用户宣传 `/v1/responses`（「OpenAI 官方新代智能体协议」，`README.md:42`），但 API 参考里完全没有它（连 `/v1/moderations`、`/v1/models`、`/mcp`、`/api/v1/public/status` 也没有）。控制台 Playground 也只覆盖 chat/images/audio/video/embeddings/rerank（`PlaygroundView.jsx:344-405`），所以 `/v1/responses` 既没有参考文档、也没有演练入口，只能靠 README 一句话知道它存在。
- **证据**：
```markdown
<!-- README.md:42 -->
- **对话、智能体响应与补全**：`/v1/chat/completions`, `/v1/responses` (OpenAI 官方新代智能体协议), `/v1/completions`, `/v1/messages`
```
```go
// internal/api/router.go:20-25（数据面注册项）
g.POST("/responses", handler.HandleResponses)
g.GET("/models", handler.HandleModels)
g.POST("/moderations", handler.HandleModerations)
```
```bash
$ grep -n "responses" docs/API_REFERENCE.md   # 只有正文里的英文单词,无该端点章节
```
- **影响**：低 — 一个已实现且对外宣传的主协议入口没有参考文档与演练入口，接入方只能从路由源码反推。
- **置信度**：高
- **建议**：在 API 参考补 `/v1/responses`（请求/流式事件/工具调用）与 `/v1/moderations`、`/v1/models`、`/mcp` 章节；或在 README 里把未文档化的入口标注为「预览」。

### D-14 渠道更新用「零值即忽略」合并，密钥更新用指针「零值也生效」——两套 patch 语义
- **类别**：契约不一致
- **位置**：`internal/controlplane/channel_service.go:226-263`（`mergeChannelUpdate`）、`internal/controlplane/admin_channels.go:63-75`（`ChannelRequest`）；对照 `internal/controlplane/key_update.go:10-22`、`web/src/components/ChannelModal.jsx:295-314`
- **设计意图**：`key_update.go:12-15` 把「省略 vs 显式零值」当成必须区分的契约：「Every field is a pointer (or a nilable slice) so an omitted field means "leave unchanged" while an explicit zero still applies」。同一个控制台里两个更新端点，操作者对「填 0」的预期应是同一个。
- **现状**：渠道更新的合并函数按 Go 零值判断：`if patch.Priority != 0`、`if patch.Weight != 0`、`if patch.TimeoutSeconds != 0`（`channel_service.go:251-259`）。于是通过 `PUT /api/v1/admin/channels/:id` **无法把 priority 或 weight 改成 0**（路由按 `ORDER BY priority ASC` 排序，priority 0 是合法且最高优先的一档），一次「改成 0」的请求会被静默当成「不修改」并返回 200。控制台这侧则用 `parseInt(e.target.value) || 1` / `|| 10` 把 0 直接改写掉（`ChannelModal.jsx:296,305`），等于把同一契约问题在 UI 上复制了一遍——操作者输入 0 看到的是 1。
- **证据**：
```go
// internal/controlplane/channel_service.go:239-259
	if patch.APIKey != "" { existing.APIKey = patch.APIKey }
	...
	if patch.Priority != 0 { existing.Priority = patch.Priority }
	if patch.Weight != 0 { existing.Weight = patch.Weight }
	if patch.TimeoutSeconds != 0 { existing.TimeoutSeconds = patch.TimeoutSeconds }
```
```go
// internal/controlplane/key_update.go:12-15（同一控制台的另一套语义）
// Every field is a pointer (or a nilable slice) so an omitted field means "leave
// unchanged" while an explicit zero still applies.
```
```js
// web/src/components/ChannelModal.jsx:296
onChange={(e) => setNewChannel({ ...newChannel, priority: parseInt(e.target.value) || 1 })}
```
- **影响**：低 — 目前只有 API 直连的调用方能撞上「设 0 无效」；控制台因为前端把 0 改写成 1/10 而看不到这个错误，但它同时也失去了设置 0 的能力（两种失败方式互为掩护）。
- **置信度**：高
- **建议**：`ChannelRequest` 与 `UpdateAPIKeyRequest` 对齐改成指针/可空字段，合并时按「是否提供」而非「是否非零」判断；同步去掉前端 `|| 1` / `|| 10` 的静默改写。

## 已排除的疑似项

- **`admin_logs.go` 新增的 `key=ak_…` 解析分支（1520d99 的一部分）**：核对过控制台只发数字 `key_id`（`useLogsData.js:93-94`、`App.jsx:1299` 都是 `key_id`），全仓没有客户端发 `key=`，所以这个分支目前没有调用方。这是正在合入的修复的一部分，按要求不作为缺陷上报，只记录在此供参考。
- **`MCPHandler.authenticateCaller`（`internal/api/mcp_handler.go:98`，deadcode 报不可达）**：不是安全洞。生产鉴权实际走 `resolveCallerDigest`（`mcp_authz.go:51`，`/mcp/messages` 在 `mcp_handler.go:163`、`/mcp/sse` 在 `mcp_sse.go:96` 调用，且 `resolveCaller` 也复用它），数据面准入规则（key 状态、账号锁、预算、余额）都接上了（`mcp_authz.go:41-90`）。`authenticateCaller` 只是「明文→digest 再查」的薄封装，全仓仅 `mcp_auth_test.go` 使用——属测试专用导出，非生产缺口。
- **`forwardJSONRPCToolCall`（`internal/api/mcp_client.go:400`，deadcode 报不可达）**：无漂移。它只是 `forwardHTTPMCPToolCall(ctx, "", endpoint, …)` 的一行委托（`:400-402`），生产调用点都直接用带 `transport` 参数的 `forwardHTTPMCPToolCall`（`mcp_tools.go:463`），注释也写明「exported within the package for the historical call site (and tests)」。属已声明保留的兼容壳。
- **`ResetReadinessChecks`（`internal/api/ops_handler.go:55`，deadcode 报不可达）**：注释明示「Tests use it to isolate cases; production code registers once at startup」，且就绪状态机确实完整接线：`RegisterReadinessCheck`（`:44`）由 `cmd/airoute` 注册，`HandleReadiness`（`:64-118`）同时校验数据面快照与全部探针。非「一半没接线」。
- **Sandbox 充值（`router.go:175-176`）**：`GET/POST /api/v1/user/wallet/recharge/sandbox` 确实无控制台调用方，但 handler 有显式门控「Admin/Dev/Test only」（`payment_handler.go:208-228`：`gin.TestMode`/`.test` 二进制/`ENABLE_SANDBOX_RECHARGE=true` 三者之一），默认 403，属有意保留的开发/测试通路。
- **数据面 `model` 结构里「无人读」的字段**（`RerankRequest.TopN`/`ReturnDocuments`、`AudioSpeechRequest.Voice`/`Speed`、`ImageGenerationRequest.Quality`/`Style`、`VideoGenerationRequest.AspectRatio` 等）：不是「接受但忽略」。多模态 handler 读取原始 body 后以 `Body: bodyBytes` 原样转发（`multimodal_handler.go:159-187`），rerank 是把结构体整体交给 dispatcher 重新序列化（`rerank_handler.go:54`），这些字段随请求体透传给上游，只是 gateway 自己不需要解释它们。
- **i18n / 组件 / 常量死代码**：`web/src/i18n.js` 的 43 个 key 全部有引用（0 未使用）；`web/src/components/*`、`hooks/*`、`lib/*` 全部有导入方（含 `React.lazy(() => import(...))` 形式），没有孤儿组件；`config.js` 的 `setPublicGatewayUrl`/`getGatewayOrigin`/`resolveGatewayUrl`/`toCSV` 等均被使用。仅 `csvEscape` 作为导出只被同文件的 `toCSV` 使用（无外部调用方），属可忽略的导出面冗余。
- **裁剪残留（v1.1 四项能力）**：路由表无 org/budget/ledger/cost 端点；`internal/` 非迁移代码 `grep org_id|orgs|wallet_transactions|usage_daily|cost_basis|margin` 零命中；`web/src` 的 `org/budget/ledger/margin` 命中全部是合法的 **key 级** budget（`KeysView`/`KeyModal`/`MyUsageView`）与 SLA **error budget**（`DashboardView`），`navGroups.js:8-9` 里的 orgs/budgets 是解释历史分组的一次性注释；`consoleRoute.js` 的 tab 白名单、`ADMIN_NAV_GROUPS`/`USER_NAV_GROUPS` 均无被删页面；`docs/API_REFERENCE.md:345` 已声明这些能力不在范围内。裁剪本身干净。
- **`chat_id` 过滤**：`X-Airoute-Chat-ID` 虽已删（`.opencode/memory/project.md`），但 `chat_id` 仍由上游响应 id 写入（`handler.go:151-155`、`inbound_anthropic.go:119` 等），控制台的 `chat_id` 关联过滤（`TraceSnapshotModal.jsx:76`、`LogDetailModal.jsx:480` 的「查看 trace」）可用，不是死字段。
- **`/api/v1/auth/*` 与 `/api/v1/user/*` 不在 D15 守卫谓词内**：已并入 D-02 作为证据；其中 `/api/v1/auth/login` 被 CLI 远程使用（`cmd/airoute/cli.go:91`），登录接口公开属有意设计，故不单列。

## 覆盖面

- **审计基线**：`1520d99 fix(admin): name keys by public id in the call log and traces`。审计结束时 HEAD 已是 `190b382 docs(billing): …`（并行同伴提交）；`git diff 1520d99..190b382` 只涉及 `docs/CONFIGURATION.md`（新增内容在 315 行之后，未使本报告引用的 `:125` 偏移）与 `internal/middleware/authz.go`（纯注释），故全部结论与行号仍以 `1520d99` 有效。工作区还有多处并行未提交改动（`internal/middleware/auth.go`、`internal/storage/repo_keys.go` 等），其中 `repo_keys.go` 的改动位于 `GetAPIKeyByKeyContext` 的读连接选择，不影响本报告引用的 `:26-30`；报告目录下并行产出的其它域报告不在本报告引用范围。
- **后端（全读）**：`internal/api/router.go` 全 330 行逐条核对，路由表解析出 143 条注册（其中 `/api/v1/*` 100 条）；`internal/api/` 的 `mcp_handler.go`/`mcp_authz.go`/`mcp_tools*.go`/`mcp_schemas.go`/`mcp_sse.go`/`mcp_info.go`/`mcp_client*.go`/`ops_handler.go`/`trace_hooks.go`/`session.go`/`metrics_access.go`/`rerank_handler.go`/`multimodal_handler.go`；`internal/controlplane/` 的 `admin_*.go`（keys/channels/logs/traces/overview/mcp/cost/reconciliation/announcements/routes）、`key_*.go`、`channel_service.go`、`pricing_service.go`、`user_admin_handler.go`、`user_keys_handler.go`、`account_handler.go`、`identity_handler.go`、`payment_handler.go`、`auth.go`、`prober*.go`；`internal/middleware/admin_access.go`/`trace.go`/`auth.go`；`internal/storage/` 中与 key 表示/渠道/日志/facet/结算/触发器相关的读路径（`repo_keys.go`、`repo_logs.go`、`repo_log_facets.go`、`repo_traces.go`、`repo_channels.go`、`money_reconcile.go`、`billing_reconcile_job.go`、`repository.go`、`repo_timeseries.go`、`repo_logs_writer.go`、`repo_mcp_seed.go`、`repo_skills_seed.go`）。
- **前端**：`web/src/api/*`、`App.jsx`（1598 行）、`lib/{apiKey,consoleRoute,navGroups,providerPresets,keyModels,modelModality,escapeLayer}.js`、`config.js`、`i18n.js`、`components/`（逐个核对导入方与 key 展示/筛选）、`hooks/`（尤其 `useLogsData.js`、`useChannelActions.js`、`useKeyActions.js`、`useConsoleData.js`）；`web/dist` 为构建产物未逐行读。
- **文档**：`docs/API_REFERENCE.md` 全 364 行；对照读取 `docs/ROADMAP.md`（B-07）、`docs/CONFIGURATION.md`（`GATEWAY_ADMIN_ALLOW_PUBLIC` 口径）、`README.md`、`docs/design/v1.1-scope-cut-2026-10-28.md`、`.opencode/memory/project.md`。
- **方法**：脚本化对照（路由表 → 前端/CLI/文档字面引用、前端调用 method+path → 路由表、MCP 工具清单 → `tools/*` case 分支、请求结构字段 → 全仓读点、`translation` key → 前端引用、模块 → 导入方），全部结论再逐条人工读上下文复核；未运行 `npm build`，未启动服务做运行时验证（D-02 的 403/200 判定为代码级推导，已给出谓词与路由注册双向证据）。
- **未覆盖**：`web/dist` 构建产物；`internal/storage` 的迁移链细节（属 05 存储域）；`internal/proxy`/`internal/upstream` 的数据面转发细节（属 02 路由协议域）；并行工作中新增的 `docs/audit/**` 与仓库根 `.docx` 未作为证据。
