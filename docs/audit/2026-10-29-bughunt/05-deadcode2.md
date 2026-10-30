# 05 · 代码瘦身 phase 2：未用字段 / 重复实现 / 无消费端点

- **日期**：2026-10-29
- **基线**：`2f43d28`（全绿；前两轮成果已由 `dfd2d0e` 落库）
- **环境**：Go 1.26.1（`/usr/local/go/bin`）、deadcode `v0.52`（`/tmp/deadcode-bin52`）
- **本轮写入范围**：`internal/{api,middleware,telemetry,billing,config,model}`、`cmd/`
  —— `internal/{storage,router,provider,adapter,controlplane,distributed}` 与 web 一律不动（报告型），避免与并行审计冲突。
- **目标**：真正拿到净减，且零行为变化；先报告后删除；不改包外 API。

---

## 0. 方法与基线复核

| 手段 | 命令 / 做法 | 结果 |
| :--- | :--- | :--- |
| 函数级死代码 | `/tmp/deadcode-bin52 ./cmd/airoute` | 仅 4 个**已声明测试钩子**（`forwardJSONRPCToolCall`、`MCPHandler.authenticateCaller`、`ResetReadinessChecks`、`telemetry.ResetWarnGate`）→ 与前两轮一致，**无新增死函数** |
| 真死代码（含测试根） | `/tmp/deadcode-bin52 -test ./...` | **0 条** |
| 复制粘贴检测 | stdlib `go/scanner` 归一化函数体（标识符→`I`、字面量→`L`）后哈希 | 33 组，其中 11 组触及本轮可写包 |
| 近似重复检测 | 归一化函数体两两 `difflib` 相似度 ≥0.82 | 2 对（1 对真重复、1 对为逻辑不同的误报） |
| 字段级盘点 | 自写 `go/ast` 工具（`&x.Field` 单列，避免把 `rows.Scan`/`json.Unmarshal` 当读取） | 1,624 字段，本轮复核 3 个「只写不读」候选 |
| 端点消费方 | 路由表 ↔ `web/src` / `cmd` / `docs` | 见 §4 |
| 常量重复 | 仅扫描 `const` 声明（排除函数内局部变量） | 12 组同值常量，逐个判定，见 §3.3 |

**结论**：函数级死代码仍是 0；本轮收益全部来自「重复实现合并」与「无用状态删除」。

---

## 1. ① 已删清单（本轮自行删除/合并，净 −33 行）

```
$ git diff --stat -- <本轮 8 个文件>
 internal/api/common.go                    | 36 ++++++++++++++-------------
 internal/api/mcp_client_stdio.go          |  5 ++---
 internal/api/moderation_handler.go        |  7 +------
 internal/api/responses_handler.go         | 22 ++------------------
 internal/api/video_handler.go             | 16 +---------------
 internal/billing/calibration.go           |  6 ++----
 internal/storage/migrations_partition.go  |  2 +-
 internal/storage/s3_storage.go            |  0 ---
 8 files changed, 33 insertions(+), 63 deletions(-)     → 净 −33 行
```

> D1–D5 来自 Lead 首轮批准范围（`internal/{api,middleware,telemetry,billing,config,model}`、`cmd/`）；
> **D6–D7 为 Lead 追加批准**的 `internal/storage` 两处删除（仅允许改 `migrations_partition.go`、`s3_storage.go`）。

### D1. `internal/billing/calibration.go`：删除只写不读的 `EstimateCalibrator.observed`（净 −2）

- **证据**：字段声明 `calibration.go:24`；全仓唯一引用是 `calibration.go:81` 的 `c.observed++`；`grep -rn '\.observed\b' internal` 在生产与测试中都没有读取（测试里出现的 `.observed` 属于 `internal/provider` 的**另一个**结构体）。
- **语义**：持久化节奏实际由 `c.pending`/`calibrationPersistEvery` 控制，字段注释也说明"persist is invoked every calibrationPersistEvery observations"，`observed` 从未参与任何判断 → 纯累计死状态。
- **改动**：删字段 + 删自增语句。**影响面**：包内私有字段；无 API 变化；无行为变化。
- **验证**：`go build ./internal/billing/`、`go test ./internal/billing/` PASS。

### D2. `internal/api/mcp_client_stdio.go`：删除 `_ = waitErr` 残留（净 −1）

- **证据**：`case waitErr := <-done:` 分支内唯一语句是 `_ = waitErr`（`:147`），`waitErr` 无任何实际用途；注释已说明"a server may exit non-zero after answering… the exit status is not itself a tool failure"。
- **改动**：改为 `case <-done:` 并把注释前移，删掉 `_ = waitErr`。**影响面**：无（仍等待子进程退出并置 `waited = true`）。
- **验证**：`go test ./internal/api/` PASS（含 MCP stdio 相关用例）。

### D3. `internal/api`：`responseContextOwner` 与 `videoOwner` 合并为 `callerOwner`（净约 −13）

- **两侧证据**：
  - `internal/api/responses_handler.go:176-186` `responseContextOwner`（调用点 `:193`、`:225`）
  - `internal/api/video_handler.go:23-33` `videoOwner`（调用点 `:36`）
  - 两者函数体**逐字节相同**（归一化哈希一致）：`getAPIKeyConfig` → 空判 → `user:<id>` → `sha256(key)` 的 `key:<hex>`。
- **改动**：合并为一个 `callerOwner(c *gin.Context) string`，落在 `internal/api/common.go`（`getAPIKeyConfig` 旁边，公共辅助函数的既有归属），注释改为同时覆盖 Responses 会话与 video 任务；删除 `videoOwner`；三处调用点统一。
- **附带清理**：`responses_handler.go` 的 `crypto/sha256`、`encoding/hex`、`strconv` 与 `video_handler.go` 的 `crypto/sha256`、`encoding/hex` 因函数搬移而不再使用，一并删除（否则编译失败）。
- **影响面**：两个符号均**未导出**，包外 API 不变；行为不变（同一实现、同一 owner 字符串）。
- **验证**：`go build ./internal/api/`、`go test ./internal/api/` PASS。

### D4. `internal/api` + `internal/model`：删除重复的 rerank prompt 估算（净约 −10）

- **两侧证据**：
  - `internal/api/common.go:809-818` `localRerankPromptEstimate`：`len(req.Query)` + `GetDocumentTexts()` 求和 → `estimateTokens(chars)`
  - `internal/model/tokens.go:134-143` `(*RerankRequest).EstimatedPromptTokens()`：同样的求和 → `EstimateTokensFromChars(chars)`
  - 而 `internal/api/common.go:625` 的 `estimateTokens` 本身就是 `return model.EstimateTokensFromChars(chars)` 的薄包装 → **两处逻辑完全等价**。
- **改动**：删除 `localRerankPromptEstimate`，唯一调用点（`resolveRerankTokens`）改为 `req.EstimatedPromptTokens()`（该方法自带 nil 保护，与原函数一致）。
- **影响面**：未导出函数；public API 不变；数值不变。
- **验证**：`go test ./internal/api/ ./internal/model/` PASS。

### D5. `internal/api/moderation_handler.go`：`moderationStubEnabled` 复用同包 `envFlagEnabled`（净 −5）

- **两侧证据**：
  - `internal/api/moderation_handler.go:22-28`：`switch strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_MODERATION_STUB")))` + `"1","true","yes","on"`
  - `internal/api/metrics_access.go:71-77` `envFlagEnabled(name string) bool`：**同一个真值判定**，只是把 env 名参数化
  - 同包（`internal/api`）、同一真值语法、同一语义。
- **改动**：`moderationStubEnabled()` 改为 `return envFlagEnabled("GATEWAY_MODERATION_STUB")`，保留原文档注释；删除因此不再使用的 `os` 导入。
- **影响面**：未导出函数；行为完全一致（同样的真值集合）。
- **验证**：`go build ./internal/api/`、`go test ./internal/api/` PASS。

### D6. `internal/storage/migrations_partition.go`：删除只写不读的 `usageLogPartition.from`（净 −1，**追加批准**）

- **证据（删除前）**：字段声明 `migrations_partition.go:586`；唯一写入是 `:633` 的 `partition.from, partition.to = from, to`；全仓 `\.from\b` 只命中该赋值。消费方 `dropExpiredUsageLogPartitions` 只用 `.name`、`.to`、`.isDefault`（判断"整个分区区间都早于 cutoff"只依赖上界）。
- **改动**：
  - 删除字段 `from time.Time`；
  - `from, fromErr := parsePartitionBoundTime(match[1])` 改为 `_, fromErr := ...`——**下界仍被解析**，因此"bound 形状异常则跳过该分区"的既有保护（上一行注释所述语义）完全保留；
  - `partition.from, partition.to = from, to` 改为 `partition.to = to`。
- **影响面**：包内私有结构体字段；无 API、无序列化、无反射消费；分区列举/保留/预创建行为不变。
- **验证**：`go build ./...`、`go vet ./...`、`go test -count=1 ./internal/storage/` PASS。

### D7. `internal/storage/s3_storage.go`：删除只写不读的 `S3Storage.useSSL`（净 −2，**追加批准**）

- **证据（删除前）**：字段声明 `s3_storage.go:25`；唯一写入是构造器 `:73` 的 `useSSL: cfg.UseSSL || u.Scheme == "https"`；全仓 `useSSL` 仅这 2 处。所有请求与 SigV4 签名都走 `s.endpoint`（`*url.URL`，其 scheme 在构造时已确定），字段从未被读取。
- **改动**：删除字段声明与构造器赋值。
- **保留的公共配置面（说明）**：`config.S3Config.UseSSL` **不动**——它仍在 `NewS3Storage` 里参与把无 scheme 的 endpoint 补成 `https://`/`http://`（`if cfg.UseSSL { endpointStr = "https://" + endpointStr }`），是有效的公开配置项；本次只删"把结果再存一份到实例字段"的死状态。
- **影响面**：包内私有字段；S3/RustFS 端点解析与签名不变；无 API 变化。
- **验证**：`go build ./...`、`go vet ./...`、`go test -count=1 ./internal/storage/` PASS（含 `TestNewS3StorageAcceptsDocumentedEndpoint`、`TestNewS3StorageIgnoresGenericS3Endpoint`）。

> 说明：本轮**没有**删除任何函数级死代码——deadcode 已证明生产可达面没有多余函数；所有净减都来自重复实现与死状态。

---

## 2. ② 待 Lead 批准清单

### A. `internal/storage` 的两个「只写不读」字段 —— **追加批准并已执行（§1-D6/D7）**

| 字段 | 位置 | 引用链证据 | 状态 |
| :--- | :--- | :--- | :--- |
| `usageLogPartition.from` | `internal/storage/migrations_partition.go:586`（赋值 `:633`：`partition.from, partition.to = from, to`） | 全仓 `.from` 仅命中该赋值；消费方只用 `.name`（`EnsureUsageLogMonthPartitions`）、`.to`/`.isDefault`（`dropExpiredUsageLogPartitions`）。`from` 是解析 `pg_get_expr` 分区边界后**从不回读**的一半 | **已删（净 −1）**，见 §1-D6；下界仍解析以保留"畸形 bound 跳过"保护 |
| `S3Storage.useSSL` | `internal/storage/s3_storage.go:25`（赋值 `:73`：`useSSL: cfg.UseSSL \|\| u.Scheme == "https"`） | 全仓 `useSSL` 仅 2 处（声明 + 构造时赋值）；签名与请求都走 `s.endpoint`（其 scheme 已在构造时解析） | **已删（净 −2）**，见 §1-D7；`config.S3Config.UseSSL` 保留（仍用于补全 endpoint scheme） |

> 另一个候选 `EstimateCalibrator.observed` 已在首轮删除（§1-D1）。三个候选至此全部处置完毕。

### B. `internal/api` 内两个 body 读取器的近似重复（**建议保留，不合并**）

- `internal/api/mcp_client.go:247` `mcpReadBody`（上限 `mcpClientMaxResponseBytes`=8 MiB，错误文案 `"MCP response exceeds the %d byte limit"`）
- `internal/api/multimodal_handler.go:45` `readMultimodalBody(stream, limit)`（上限 64 MiB 传参，错误文案 `"upstream response exceeds %d bytes"`）
- **差异**：上限来源、错误文案、调用约定都不同。参数化会把**用户可见的错误文案**变成模板拼接（例如丢掉 `the … limit` 形态），属于可观测行为变化 → 不满足"零行为变化"，保留两份（各 ~9 行）。

### C. `cmd/airoute` 的 env 读取重复（**净收益 ≈0，建议不做**）

- `cmd/airoute/server.go:404` `drainBudget()`、`:420` `shutdownTimeout()`、`server_env.go:31/36/41` 共 4 处「`TrimSpace(os.Getenv)` → `Atoi` → `>0` → 秒转 Duration」。
- 统一需要一个带 3 个返回值 `(seconds int, raw string, ok bool)` 的 helper（`shutdownTimeout` 需要在"设置了但非法"时告警，必须拿回原始文本）。helper 约 12 行、可省约 12 行 → **净 ≈0，且三元返回降低可读性**，故本轮不做，仅登记。

### D. 重复常量（逐个判定，**均建议保留**）

| 同值常量 | 位置 | 判定 |
| :--- | :--- | :--- |
| `liveCharsPerToken = 4` vs `model.CharsPerToken = 4` | `internal/telemetry/live.go:22` vs `internal/model/tokens.go:15` | **刻意镜像**：`live.go:18-20` 注释明确写「telemetry deliberately does not import the business `model` package to keep the observability layer dependency-free; keep the two values in sync」→ 属架构决策，不合并 |
| `maxResponseContextBytes = 4 << 20` vs `maxResponseModelRestoreBytes = 4 << 20` | `internal/api/responses_handler.go`、`internal/api/response_model_restore.go` | 同为 4 MiB 但保护对象不同（会话上下文 / 模型恢复载荷）；合并需改名并耦合两者上限 → 语义收益为负，保留 |
| `tokenWindowSpan` / `rpmWindowSpan` 均为 `time.Minute` | `internal/middleware/tpm_tracker.go`、`ratelimit.go` | 分别是 TPM 与 RPM 两个限流器的窗口；同值但独立可调，保留 |
| `2 * time.Second` ×3、`300 * time.Millisecond` ×2、`DefaultChannelTimeoutSeconds`/`edgeProxyMarginSeconds` 均 60、`ImageTokensDefault`/`NonTextPartTokens` 均 765 | 见 §3.3 | 语义不同（超时/余量/图片 token 预算），保留 |

### E. `internal/billing.MatchModelPattern` 薄包装（**不改，理由在注释里**）

- `internal/billing/model_pattern.go:12` 仅是 `return model.MatchModelPattern(pattern, modelName)`，被 `engine.go:310` 与 `billing/wildcard_test.go` 使用。
- 注释已说明存在理由：`internal/config` 需要同一套 glob 语义，但 billing（间接）依赖 config，config 不能反向 import billing；`internal/model` 是两者都能 import 的叶子包，"keeping this wrapper means pricing call sites and their tests are unchanged"。
- 删除需改 engine + 测试 → 改动面大于收益（约 −4 行），且属包外 API，保留。

### F. `internal/middleware` 的 CIDR/IP 列表解析重复（**评估后放弃合并**）

- `internal/middleware/admin_access.go:138-151`（控制面 allowlist）与 `internal/middleware/authz.go:347-359`（受信代理）各自实现「CIDR 或裸 IP → `*net.IPNet`」，语法与 IPv4 归一化逻辑相同。
- **尝试过合并**：抽出 `parseIPOrCIDR(entry)` 后实测 `git diff --numstat` 为 **净 +1 行**（helper 的声明成本 ≥ 两处内联节省），属于「零收益纯重构」，与"只做净减"冲突，已 **revert**。
- 另有行为差异：两者分隔符集合不同（受信代理额外切 `\n`/`\r`），沿用受信代理的列表解析会改变控制面 allowlist 对换行输入的处理（当前为报错）→ 也不是零行为变化。
- **结论**：确认重复但保留；若将来统一，应作为"可读性/一致性"专项，而非瘦身专项。

### G. `internal/telemetry` 常量/字段复核

- 字段盘点中 `LiveSnapshot.IdleSeconds/Nodes`、`Metrics.*` 等均为「只写不读 selector 或原子计数器」，实际由 Prometheus exposition（`metrics_prometheus.go`）与 `/stats/live` JSON 消费 → 保留（第一轮 §7-R4 已论证同类）。

---

## 3. ③ 重复实现清单（本轮范围）

### 3.1 已合并（详见 §1）

| # | 侧 A | 侧 B | 差异 | 结果 |
| :--- | :--- | :--- | :--- | :--- |
| 1 | `api/responses_handler.go:176` `responseContextOwner` | `api/video_handler.go:23` `videoOwner` | **逐字节相同** | 合并为 `api/common.go` 的 `callerOwner`，−13 行 |
| 2 | `api/common.go:809` `localRerankPromptEstimate` | `model/tokens.go:134` `(*RerankRequest).EstimatedPromptTokens` | 同语义，api 侧重复实现 | 删除 api 副本，−10 行 |
| 3 | `api/moderation_handler.go:22` `moderationStubEnabled` | `api/metrics_access.go:71` `envFlagEnabled` | 同包、同真值语法，仅 env 名硬编码 | 改为委托，−5 行 |

### 3.2 报告型重复（不合并，理由见 §2）

| # | 侧 A | 侧 B | 差异 / 不合并原因 |
| :--- | :--- | :--- | :--- |
| 4 | `api/mcp_client.go:247` `mcpReadBody` | `api/multimodal_handler.go:45` `readMultimodalBody` | 上限与错误文案不同；合并会改用户可见文案 |
| 5 | `middleware/admin_access.go:138` IP 列表解析 | `middleware/authz.go:347` `ParseTrustedProxies` | 合并后净 +1 行（已实测），零收益 |
| 6 | `api/upstream_lifecycle.go:38` `upstreamDrainTimeout` | `router/circuit_breaker_config.go:42` `breakerCooldown` | 跨包（router 本轮不可写），且 env 名/默认值不同 |
| 7 | `api/metrics_access.go:71` `envFlagEnabled` | `storage/migrations_partition.go:138` `tablePartitioningEnabled` | 跨包（storage 不可写） |
| 8 | `cmd/airoute/security_check.go:15` `allowInsecureDefaults` | `api/moderation_handler.go:22`（已委托给 `envFlagEnabled`） | cmd 无法 import internal/api 反向复用；统一需新公共包 |
| 9 | `billing/model_pattern.go:12` 包装 | `model/model_pattern.go` 实现 | 断依赖环的有意包装（§2-E） |

### 3.3 常量复核（12 组同值，逐个判定 → 全部保留）

判定通过的标准：**是否同一语义**。12 组中：
- 1 组刻意镜像（`telemetry.liveCharsPerToken` ↔ `model.CharsPerToken`，注释已声明）；
- 11 组为"不同对象恰好同值"（超时窗口、并发上限、token 预算、退出码等），合并会把独立可调的参数耦合在一起。

---

## 4. ④ 端点与符号再确认

### 4.1 `POST /admin/mcp/servers/:id/probe` —— **结论：保留（不删）**

| 维度 | 证据 |
| :--- | :--- |
| 实现 | `internal/controlplane/admin_mcp.go:263` `ProbeMCPServer`，注释明确「tests connectivity to an MCP Server with genuine network probes」；非 stub |
| 路由 | `internal/api/router.go:297` `adminOnly.POST("/mcp/servers/:id/probe", ...)`（admin 鉴权 + 控制面 CIDR 守卫之内） |
| 控制台消费 | `web/src` 0 命中（`McpIntegrationView.jsx` 只用 `/toggle`） |
| 其他消费 | `cmd/` 0 命中；文档 0 命中；仅 `controlplane/admin_mcp_local_detection_test.go:103/151` 两个用例覆盖 |
| 去重性 | MCP 服务保存（`SaveMCPServer`）**不会**自动探测，因此该端点是"配置后单独验证连通性"的唯一入口；`/channels/probe`、`/models/topology/probe` 服务于其他对象 |
| 结论 | **保留**：它是可用的管理能力（curl/自动化脚本可调用），删除等于移除功能 + 删除 2 个测试，不属于死代码清理。无消费者是控制台尚未接线的产品缺口，建议记入 ROADMAP 而非删除 |

### 4.2 `GET /admin/system/middlewares` —— **保留（ROADMAP B-07 预留）**

- `internal/api/router.go:235` 注册；`router.go:231` 注释与 `docs/ROADMAP.md:119`（B-07）明确：控制台「自检报告」页将复用它，页面落地前不得删除。本轮未动。

### 4.3 仅测试可达的导出符号 —— **无新增候选**

- `deadcode ./cmd/airoute` 与 `deadcode -test ./...` 的差集仍然**恰好**是 4 个已声明的测试钩子：`api.forwardJSONRPCToolCall`、`MCPHandler.authenticateCaller`、`api.ResetReadinessChecks`、`telemetry.ResetWarnGate`。这些是 `*_test.go` 的测试入口，按约定保留。
- 本轮没有发现"仅测试可达且无保留理由"的导出符号。

---

## 5. ⑤ 超限文件清单（>300 行，本轮不拆）

`docs/ENGINEERING_GUIDELINES.md` §二：单文件 ≤300 行为绝对红线（纯静态 Schema 与 SQL DDL 迁移除外）。当前 **276 个非测试 `.go` 中 53 个超限**（19.2%），比上一轮（275 中 51）略增。

### 5.1 优先拆分（收益/风险比最高）

| 行数 | 文件 | 拆分建议（目标文件 / 预估行数） |
| ---: | :--- | :--- |
| 1020 | `internal/api/common.go` | 已含 20+ 跨 handler 辅助函数。拆为 `api/respond_errors.go`（RespondOpenAIError / oauth 错误映射，≈220）＋ `api/audit_usage.go`（RecordUsage/AuditRecordParams/账务调用，≈260）＋ `api/token_estimate.go`（estimateTokens/calibrate/canonical 估算，≈200）＋ `api/context_helpers.go`（getAPIKeyConfig/callerOwner/会话与 trace 取值，≈180）＋ 余下 ≈160 留 `common.go` |
| 744 | `internal/model/response.go` | 拆 `model/response_request.go`（ResponseRequest/Input 类型与解码，≈260）＋ `model/response_result.go`（ResponseResult/Output item，≈260）＋ `model/response_convert.go`（与 chat/completions 互转，≈220） |
| 652 | `internal/middleware/auth.go` | 拆 `auth_identity.go`（凭据解析 + 快照/回源，≈230）＋ `auth_wallet.go`（余额与预扣、SettledCost，≈220）＋ `auth_denial.go`（AccessDenial 构造与审计，≈200） |
| 524 | `internal/middleware/authz.go` | 拆 `authz_trusted_proxy.go`（ParseTrustedProxies/ConfigureTrustedProxies/ClientIdentity，≈200）＋ `authz_quota.go`（额度/限流检查，≈180）＋ `authz_denial.go`（拒绝响应构造，≈140） |
| 502 | `internal/billing/engine.go` | 拆 `engine_rates.go`（rate card 解析/存储，≈180）＋ `engine_lookup.go`（GetPrice*/wildcard 匹配，≈180）＋ `engine_calc.go`（Calculate*/off-peak 应用，≈140） |
| 549 | `cmd/airoute/server.go` | 拆 `server_bootstrap.go`（启动装配，≈220）＋ `server_shutdown.go`（drain/shutdown/超时预算，≈180）＋ 已有 `server_env.go`/`timeout_chain.go`，余下 ≈150 |

### 5.2 其余超限文件（按包分组，建议按职责切分）

**internal/api（10 个）**
- `inbound_anthropic_stream.go` 549 → `inbound_anthropic_stream_parse.go`（SSE 帧解析）+ `inbound_anthropic_stream_events.go`（事件→OpenAI 翻译）
- `mcp_tools.go` 471 → `mcp_tools_registry.go` + `mcp_tools_read.go`（只读校验）+ `mcp_tools_impl.go`
- `mcp_client.go` 441 → `mcp_client_transport.go` + `mcp_client_jsonrpc.go`
- `mcp_handler.go` 415 → `mcp_handler_session.go` + `mcp_handler_sse.go`
- `inbound_anthropic.go` 412 → `inbound_anthropic_request.go` + `inbound_anthropic_response.go`
- `gemini_inbound.go` 395 → `gemini_inbound_request.go` + `gemini_inbound_response.go`
- `gemini_inbound_stream.go` 313 → 事件解析/翻译分离
- `router.go` 348 → `router_middleware.go`（中间件装配）+ `router_admin.go`（admin 路由组）+ `router_public.go`
- `mcp_client_http.go` 385 → 与 `mcp_client.go` 同域，按传输/编解码重划
- `responses_stream_events.go` 303 → 事件构造/发送分离
- `mcp_tools_routing.go` 331 → 与 `mcp_tools.go` 合并后按路由/参数校验切

**internal/storage（15 个）**
- `migrations_partition.go` 758、`migrations_postgres.go` 526、`migrations_sqlite.go` 506 → **规范豁免**（DDL 迁移）；若要瘦身，把 43 号迁移的 SQL 与回滚脚本移到 `migrations_partition_sql.go`
- `billing_reconcile_job.go` 631 → `billing_reconcile_run.go` + `_drift.go` + `_config.go`
- `settlement.go` 628 → `settlement_batch.go` + `settlement_retry.go` + `settlement_wallet.go`
- `db.go` 627 → `db_open.go` + `db_time.go` + `db_helpers.go`
- `repo_logs.go` 625 → `repo_logs_query.go` + `repo_logs_facets.go` + `repo_logs_stats.go`
- `repo_keys.go` 591 → `repo_keys_crud.go` + `repo_keys_query.go` + `repo_keys_batch.go`
- `billing_ledger_retention.go` 511 → 配置解析 / worker / prune 三段
- `async_logger.go` 465 → writer / spill / stats
- `repo_timeseries.go` 432 → 查询 / 缓存 / 聚合
- `repo_reconciliation.go` 417 → 报表查询 / 差异计算
- `repo_users.go` 405 → CRUD / 钱包 / 唯一冲突
- `repo_logs_writer.go` 395 → 单条 / 批量 / 编解码
- `repo_channels.go` 342 → CRUD / 批量 / 视图
- `usage_spill.go` 333 → 写入 / 重放 / 回收
- `settlement_batcher.go` 306 → group-commit 循环 / 等待 / 停止

**internal/controlplane（7 个）**
- `oauth_handler.go` 412 → 授权跳转 / 回调 / 用户名分配
- `sync.go` 389 → 快照类型 / 同步器 / 指纹
- `login_guard.go` 384 → 本地存储 / 策略 / 计数
- `admin_mcp.go` 354 → CRUD / probe / 本地判定
- `admin_skills.go` 349 → CRUD / artifact / manifest
- `prober_adapters.go` 348 → 各 provider 适配器分文件
- `admin_overview.go` 330 → 总览聚合 / 中间件拓扑
- `route_service.go` 315 → 校验 / 写入 / 价格重载
- `admin_keys.go` 312 → CRUD / 批量 / 视图
- `pricing_service.go` 306 → 校验 / 落库 / 重载

**internal/router（3 个）**
- `dispatcher_modality.go` 473 → modality 分发 / 跨模型回退 / 错误映射
- `dispatcher.go` 396 → 候选发现 / 选择 / 执行
- `circuit_breaker.go` 364 → 状态机已分，可再拆探针/指标

**其它**
- `internal/telemetry/metrics.go` 419 → 计数器 / 分位数 / 校准样本（`metrics_prometheus.go` 已是导出层）
- `internal/telemetry/ring_buffer.go` 366 → ring / broadcast handler
- `internal/provider/anthropic_stream.go` 385 / `openai.go` 334 → 事件解析 / 帧重组 或 URL/unary/stream
- `internal/distributed/redis_quota.go` 445 → Lua 常量 / 额度读写 / 心跳

> 拆分顺序建议：先 `api/common.go`、`middleware/auth.go`、`billing/engine.go`（三者都是"多职责集中"且测试覆盖充分），每次拆分跑全量 `go test ./...`。

---

## 6. ⑥ 净减统计与验证

### 净减

| 项 | 数值 |
| :--- | :--- |
| 生产代码（本轮合计，含追加批准） | **+33 / −66 → 净 −33 行**（首轮 −30，追加 storage 两处 −3） |
| 文件数 | 8（`internal/api` ×5、`internal/billing` ×1、`internal/storage` ×2） |
| 新增文件 | 0 |
| 删除函数 | 2（`videoOwner`、`localRerankPromptEstimate`）+ 1 函数体（`moderationStubEnabled` 改为委托）+ 3 个私有字段（`EstimateCalibrator.observed`、`usageLogPartition.from`、`S3Storage.useSSL`）+ 1 条 `_ =` 语句 |
| 未计入 | 已 revert 的 middleware CIDR 合并（净 +1，见 §2-F） |

```
$ git diff --stat -- <本轮 8 个文件>
 internal/api/common.go                    | 36 ++++++++++++++-------------
 internal/api/mcp_client_stdio.go          |  5 ++---
 internal/api/moderation_handler.go        |  7 +------
 internal/api/responses_handler.go         | 22 ++------------------
 internal/api/video_handler.go             | 16 +---------------
 internal/billing/calibration.go           |  6 ++----
 internal/storage/migrations_partition.go  |  2 +-
 internal/storage/s3_storage.go            |  0 ---
 8 files changed, 33 insertions(+), 63 deletions(-)     → 净 −33 行
```

### 验证命令与结果

| 命令 | 结果 |
| :--- | :--- |
| `gofmt -l internal cmd tests` | **无输出**（clean） |
| `go build ./...` | **PASS** |
| `go vet ./...` | **PASS** |
| `go test ./...` | **16 个测试包全 `ok`，FAIL=0**：`cmd/airoute`、`internal/{adapter,api,billing,config,controlplane,distributed,middleware,model,provider,router,storage,telemetry,validator}`、`tests`、`web`（另有 `internal/buildinfo` 与 vendored `flatted` 无测试文件） |
| `go test ./internal/api/ ./internal/billing/`（重点包） | PASS（api 10.8s，billing cached） |
| `go test -count=1 ./internal/storage/`（追加批准后重跑） | **PASS**（6.8s；含 S3 端点解析用例与分区相关用例） |
| `go test -count=1 -run 'S3\|Partition\|Purge' ./internal/storage/`（隔离验证本轮改动） | **PASS**（0.3s） |
| `/tmp/deadcode-bin52 ./cmd/airoute` | 仅 4 个既定测试钩子（无新增） |

> **并行写入期间的验证说明**：追加删除执行时，其他 agent 正在同一 `internal/storage` 内做批量重构（`async_logger.go`、`repo_keys.go`、`migrations_sqlite.go`/`migrations_postgres.go` 新增 migration 44 等）。期间出现过 3 次**瞬时** `go build`/`go test` 失败，均已确认与本次两处删除无关（报错符号分别为 `boolToDB`、`spillReplayLoop`、`resolveUsageSpillReplayInterval`、`telemetry.Metrics.BillingLedgerPruneContended`、以及新 migration 44 的 `no such table: users`，全部来自对方在写文件，不在本轮改动范围）。待对方写入收敛后，上表四条命令**全部重跑通过**；本轮的隔离验证（`-run 'S3|Partition|Purge'`）亦为 PASS。

---

## 7. ⑦ 低风险清理执行情况

| 类别 | 结果 |
| :--- | :--- |
| 注释掉的死代码块 | **0 处**（扫描 `internal/{api,middleware,telemetry,billing,config,model}` + `cmd` 的 `// func/if/for/return/var/const/defer` 开头行，9 处命中全部是自然语言注释，如 `// for the current caller. …`） |
| `_ = x` 多余赋值 | 复核 23 处，**仅 1 处为真冗余**（`mcp_client_stdio.go` 的 `_ = waitErr`，已删，§1-D2）；其余为有意的错误丢弃（`_ = body.Close()`、`_ = persist()`、`_ = fs.Parse(args)` 等），保留 |
| 重复常量 | 12 组同值常量逐个判定，**全部保留**（1 组刻意镜像 + 11 组语义不同，§3.3） |
| 三行合一 | 未做：本轮范围内的候选都会牺牲可读性或改变错误文案，属"纯重构"，与净减目标冲突 |
| 单调用方薄包装 | 复核 32 个 ≤8 行、单调用点的未导出辅助函数：均为有名字价值的谓词/访问器（如 `shouldFailClosedOnUnknownAccount`、`isMCPNotificationMethod`），内联会降低可读性 → 不做 |

---

## 8. 结论

1. **本轮净减 −33 行**（8 文件，+33/−66）：首轮重复实现合并/死状态删除 −30，追加批准的 storage 两处删除 −3。行为不变、包外 API 不变。
2. **函数级死代码仍为 0**：deadcode 主入口只剩 4 个既定测试钩子，`-test` 模式 0 条。
3. **3 个「只写不读」字段候选全部处置完毕**：`EstimateCalibrator.observed`、`usageLogPartition.from`、`S3Storage.useSSL` 均已删除（其中后两者为 Lead 追加批准，仅改 `internal/storage/migrations_partition.go`、`s3_storage.go`；`config.S3Config.UseSSL` 作为有效公开配置保留，仍参与 endpoint scheme 补全）。
4. **两处经实测否决的合并**（含数据）：middleware CIDR 解析统一净 +1 行；`mcpReadBody`/`readMultimodalBody` 合并会改错误文案。已 revert / 保留并留证。
5. **端点结论**：`POST /admin/mcp/servers/:id/probe` **保留**（真实探测能力、admin API、有测试；无控制台消费属产品缺口，建议进 ROADMAP）；`/admin/system/middlewares` 按 B-07 保留。
6. **超限文件 53 个**，优先拆 `api/common.go`(1020)、`model/response.go`(744)、`middleware/auth.go`(652)、`middleware/authz.go`(524)、`billing/engine.go`(502)、`cmd/airoute/server.go`(549)。
