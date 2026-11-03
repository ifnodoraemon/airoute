# 修复记录：控制面 + 前端（F 域）

> 任务：`task-11`（跨域借用，配置域工程师执行）
> 规格：`docs/audit/2026-10-28-inconsistencies/04-api-console.md`（D 报告）+ `README.md`
> 可写范围：`internal/controlplane/**`、`web/src/**`、本文件
> 基线：`190b382` 起，期间 HEAD 前进到 `c8a2765`（并行会话提交 5 个 commit）；本文件记录的是**当前工作区**的改动

## 一、状态表

| 发现ID | 状态 | 文件:行 | 验证 | 备注 |
|---|---|---|---|---|
| **D-01** 渠道明文 `api_key` 回传 | 已修复 | `internal/controlplane/channel_view.go:1-79`（新增 `channelView`/`channelViewFrom`/`channelViews`）；`admin_channels.go:46-56`（List）、`admin_channels.go:58-64`（`channelMutationPayload`，覆盖 202 热加载告警路径）、`admin_channels.go:141-159`（Create）、`admin_channels.go:161-179`（Update）；`web/src/hooks/useChannelActions.js:261-285,272` | `go test ./internal/controlplane/ -run TestChannelResponsesNeverCarryTheUpstreamCredential`（含 list/create/update 三个子用例，断言响应体不含明文、不含 `api_key` 字段、含 `api_key_configured=true` 与非空 `api_key_masked`）；`TestChannelViewReportsUnconfiguredCredential`；`TestChannelReloadWarningPayloadUsesTheView`；前端 `useChannelActions.test.js`（编辑表单读 `api_key_configured`） | 投影在 controlplane 层完成，**未改 `internal/storage`**。5 条回传渠道的路径全部改走 view：list / create / update / 两条 202 热加载告警 payload。`api_key` 仍只在**请求**方向出现（写入 DB 用），响应方向零明文 |
| **D-04 / E-09** `/stats/*` 的 `key` 参数表示随角色而异 / `ak_…` 被哈希成空 digest | 已修复 | `internal/controlplane/admin_stats_key_filter.go:17-66`（新增 `statsKeyFilter`）；`admin_overview.go:19-23`（overview）、`admin_overview.go:107-111`（timeseries）改为调用它 | `TestStatsKeyParamResolvesPublicIdentifier`（`ak_…` → 与 `key_id` 同结果，overview+timeseries）；`TestStatsKeyParamEnforcesOwnershipForPublicIdentifier`（非管理员仍受限）；`TestStatsKeyParamRejectsUnresolvableReferences`（明文/掩码/未知 `ak_` → 400）；`TestStatsKeyParamAcceptsRawDigest`（旧脚本传 digest 仍可用） | 归一化只在入口做一次：`ak_…` → `repo.APIKeyDigestByPublicID`；64-hex digest 原样接受；其余 → **400**（不再静默归零）。非管理员的 ownership 判定改为比较归一化后的 digest。顺带把两个 key 解析器抽成独立文件，使 `admin_overview.go` 从 348 行降到 325 行（≤ 基线，遵守 300 行约定方向） |
| **D-09** `format_validation` 全链路可设置但无人计算/展示 | 已修复 | `internal/controlplane/key_format_validation.go:1-33`（新增枚举校验）；`admin_keys.go:117-126`（create 校验）；`key_update.go:106-115`（update 校验）+ `admin_keys.go:193-202`（400 映射）；`web/src/components/KeyModal.jsx:303-327`（级别选择）；`web/src/components/KeysView.jsx:478-490`（列表徽标）；`web/src/hooks/useKeyActions.js:47-49,96-100` | `TestNormalizeFormatValidation`（`""/off/lenient/strict` 通过，`stirct` 报错，大小写与空白归一化）；`TestAPIKeyFormatValidationRejectsUnknownLevels`（create/update 非法 → 400，合法值持久化并回显）；前端 `KeyModal.test.jsx` 新增 3 例（管理员可见含「跟随全局」、选择回传 `setNewKey`、自助用户不显示） | 规格的取值集合 `"" | off | lenient | strict` 与 `internal/config.ResolveValidationLevel` 真正理解的集合一致。UI 仅对管理员展示：**自助密钥创建端点（`CreateUserKeyRequest`）不接受该字段**，给普通用户显示会变成新的「UI 承诺了后端做不到」的不一致 |
| **D-11** `PUT /admin/keys/:id` 返回裸 storage 记录 | 已修复 | `internal/controlplane/admin_keys.go:262-266` | `TestUpdateAPIKeyReturnsTheKeyView`（响应 `key_id` 等于 `PublicID`、无 `key` 字段、不含 digest/明文） | 改为 `apiKeyViewFrom(outcome.Record)` 后返回，与 create/list 同一序列化路径 |
| **D-12** `keyDisplayRef` 零调用、8 处重复 | 已修复 | `web/src/components/KeysView.jsx:21,161,227`；`MyUsageView.jsx:3,138`；`PlaygroundView.jsx:35,1018`；`DashboardView.jsx:23,990`；`AiCopilot.jsx:37,1380`；`OnboardingModal.jsx:26,365`；`hooks/useKeyActions.js:10,177` | `npm test`（51 文件 666 用例全绿）；`npm run lint`（0 warning） | 8 处手写 `key_id || #id` 全部改为调用 `keyDisplayRef`，7 个文件各自补 import。`KeysView.keyIdentifier` 保留为一行委托（文件内 4 处调用点不逐个改，减少 diff） |
| **D-14** 渠道更新「零值即忽略」 | **部分修复（后端本层已修；端到端受 storage 层阻断，需 storage 域配合）** | `internal/controlplane/channel_service.go:53-80`（新增 `channelPatch` 类型，声明在 `:69`，数值字段指针化）、`:137`（`Update` 签名）、`:250-306`（`mergeChannelUpdate` 按「是否提供」判定）；`admin_channels.go:80-100`（`ChannelRequest` 数值字段指针化）、`admin_channels.go:124-138`（`toPatch`）；`web/src/components/ChannelModal.jsx:11-23,307,316`（去掉 `|| 1` / `|| 10`）；`web/src/hooks/useChannelActions.js:274-277`（`||` → `??`） | `TestChannelServiceUpdateAppliesExplicitZeroRoutingFields` / `TestChannelServiceUpdateKeepsOmittedRoutingFields`；handler 级 `TestUpdateChannelAppliesExplicitZeroRoutingFields`（fake store 收到 priority=0/weight=0）/ `TestUpdateChannelKeepsOmittedRoutingFields`；前端 `ChannelModal.test.jsx` 新增 2 例；`useChannelActions.test.js:180-195`（不再把存储的 0 改写） | **阻断点（不在我可写范围）**：`internal/storage/repo_model_routes.go:174-181` 的 `updateChannelTx` 仍把 `Priority==0 → 1`、`Weight==0 → 10`、`TimeoutSeconds==0 → 60`，所以经真实 DB 的 `PUT` 仍落成 1/10。控制面已不再「静默丢弃」，语义与创建时一致（0 = 用默认值）。要真正持久化 0，需 storage 域把该归一化从 `updateChannelTx` 移除（或区分「未提供」）——已列入 handoff |
| **E-08** 已撤销密钥的 trace 对本人不可见 | 已修复 | `internal/controlplane/admin_traces.go:32-36` | `TestRequestTracesIncludeRevokedKeys`（同一用户 1 个活跃 key + 1 个 `status='deleted'` key 各产生 1 条 trace，本人查询应看到 2 条） | 改用 `repo.ListHistoricalAPIKeysByUser`，与 `admin_logs.go:66` 的既有样板一致。删除是软删除（`status='deleted'`），活跃列表会过滤掉，历史列表不会 |
| **A-15（控制面侧）** 未文档化的 `STRIPE_BASE_URL` 兜底 | 已修复 | `internal/controlplane/payment_handler.go:84-92` | `go build ./...`、`go vet ./...`（编译级验证；该分支需真实 Stripe 调用，未做集成测试） | 删除 `STRIPE_BASE_URL` 兜底，只读文档化的 `STRIPE_API_BASE`，并对取值做 `TrimSpace` |
| **C-10**（C 域 handoff）离峰时段校验与引擎读取对象不一致 | 已修复 | `internal/controlplane/pricing_service.go:195-202`（改调 `billing.ValidateOffPeakSlots(rec.OffPeakMode, rec.OffPeakSlots)`）；`internal/controlplane/route_service.go:223-230`（改调 `billing.ValidateOffPeakSlots(offMode, offSlots)`） | `TestPricingServiceValidatesOffPeakSlotsOnlyInCustomMode`（6 例：night/fixed 模式忽略 slots、custom 校验 Discount+Days、custom 拒绝重叠、custom 接受合法、模式判定大小写不敏感）；`TestRouteServiceValidatesOffPeakSlotsOnlyInCustomMode`（4 例，同一契约在路由编辑器） | 两处写路径现在都按价目行的 `off_peak_mode` 门控：只有 `custom` 才校验 slots（`CustomOffPeakStrategy` 是唯一读 `OffPeakSlots` 的策略）。核对了引擎侧的兜底：未知/空模式走 `WindowOffPeakStrategy`（`engine.go:365-369`），同样不读 slots，所以「空模式不校验」与执行一致。旧 `billing.ValidateSlotsOverlap` 保留未动（billing 自身测试仍在用，其文档已标注是「没有 mode 可用的调用方」入口）。附带修正两个既有测试：它们原本靠「槽位串不是合法 JSON」间接拿到 400，现改为显式 `off_peak_mode="custom"` + 真实重叠 JSON，测试意图（重叠规则）才真正被覆盖 |
| **D-08**（D 域 handoff）`/admin/system/middlewares` 无前端消费者 | 已修复（注释补注） | `internal/controlplane/admin_overview.go:185-191`（`GetSystemMiddlewares` handler 注释） | `go build ./...`、`go vet ./...`（纯注释改动） | 注释写明：该端点为 ROADMAP B-07「控制台自检页」预留（该页还会用 `/health/ready`），当前无前端消费者，仅为计划页面与脚本化诊断服务；若 B-07 取消，handler 与路由应一并删除。与 D 域已在 `internal/api/router.go`、`docs/ROADMAP.md` 加的说明同口径 |

## 二、验证结果

后端（`export PATH=$PATH:/usr/local/go/bin`）：

```
$ gofmt -l internal/controlplane/          # 空输出
$ go build ./...                           # 通过
$ go vet ./...                             # 通过
$ go test ./internal/controlplane/         # ok  github.com/ifnodoraemon/airoute/internal/controlplane  6.9s
```

> 注：验证期间并行会话在 `internal/provider` / `internal/storage` 的未提交改动曾数次短暂破坏整仓编译（符号搬迁中间态，均非本域代码）。上述结果取自这些改动稳定后的最终一轮；若后续再次出现 `internal/provider` 或 `internal/storage` 的未定义符号，属并行会话中间态，与本域改动无关。
>
> 第二轮（C-10 + D-08 handoff）合入后复跑：`gofmt -l internal/controlplane/` 空、`go build ./...` 通过、`go vet ./...` 通过、`go test ./internal/controlplane/` ok 6.9s（含新增 10 个 C-10 子用例）。

前端（`cd web`）：

```
$ npm run lint                             # eslint src --max-warnings 0，通过（0 warning）
$ npm test                                 # Test Files 51 passed | Tests 666 passed
```

新增/更新测试合计：Go 新增 5 个测试文件（`channel_view`、`admin_channels_view`、`admin_overview_keyparam`、`key_view_validation`、`admin_traces_revoked`），改写 2 处既有测试；前端新增 8 个用例、改写 1 个。所有断言均先在「未修复」行为下确认会失败（D-01 明文断言、D-04 的 400 断言、E-08 的 2 条 trace 断言、D-14 的 0 值断言）。

## 三、改动清单（本域）

新增文件：
- `internal/controlplane/channel_view.go`（渠道 wire 视图，79 行）
- `internal/controlplane/key_format_validation.go`（`format_validation` 枚举校验，33 行）
- `internal/controlplane/admin_stats_key_filter.go`（`key`/`key_id` 归一化，84 行；从 `admin_overview.go` 抽出）
- 测试：`admin_channels_view_test.go`、`admin_overview_keyparam_test.go`、`key_view_validation_test.go`、`admin_traces_revoked_test.go`

修改文件：`admin_channels.go`、`admin_keys.go`、`admin_overview.go`、`admin_traces.go`、`channel_service.go`、`key_update.go`、`payment_handler.go`（后端）；`hooks/useChannelActions.js`、`hooks/useKeyActions.js`、`components/{KeysView,ChannelModal,KeyModal,MyUsageView,PlaygroundView,DashboardView,AiCopilot,OnboardingModal}.jsx`（前端）。

未触碰（按任务约束）：`internal/api`、`internal/storage`、`internal/middleware`、`internal/router`、`internal/billing`、`cmd/`、`web/web.go`、`docs/API_REFERENCE.md`。无任何 git 写操作。

## 四、Handoff

1. **~~D-14 需要 storage 域收尾~~（已闭环）**：`internal/storage` 侧已移除 `updateChannelTx` 的 `priority==0→1`/`weight==0→10`/`timeout_seconds==0→60` 归一化（创建路径 `CreateChannel` 保留默认值语义），并新增 `repo_channels_zero_routing_test.go` 覆盖 `UpdateChannel` 与 `PatchChannel` 两条真实路径（见 `fixes-storage.md`）。本域代码无需再改。
2. **自助密钥的 `format_validation`**：目前只有管理员创建/更新端点支持该字段。若产品要求自助用户也能设置，需要 `CreateUserKeyRequest` + `PlanUserKey` 接受并校验它（约 5 行），再放开 `KeyModal` 里 `isAdmin` 的判断；否则 UI 已刻意不向自助用户展示，避免新的「UI 有、后端无」不一致。
3. **`api_key_masked` 的掩码格式**：按任务要求复用 `storage.MaskAPIKey`，它对 ≤12 字符的短密钥（如本地渠道常用的 `none`）返回 `sk-***`，看起来像 `sk-` 前缀密钥。当前前端不使用该字段（只用 `api_key_configured`），若将来用于辨识展示，可考虑渠道专用掩码。
4. **未做运行时验证**：D-01 是纯响应投影，已由 handler 级测试覆盖；A-15 的 Stripe 分支、D-09 的真实协议校验行为需实际上游/配置，仅做了编译与单元级验证。
