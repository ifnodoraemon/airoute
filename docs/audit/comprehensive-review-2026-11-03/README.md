# 全面审查：代码删减与用户操作闭环（2026-11-03）

审查基线：`fe2675be411800a54d28a04a2757f954fd8bdd2a`。本文记录修复前状态；后续实现与验收见 [修复记录](remediation.md)。

结论：**账号、兑换、调用、扣费、审计和权限撤销的主链路已经闭环；异常输入、排障跳转、原生 Gemini 反馈、配置变更和部署操作仍有断点。** 本轮确认 8 项 P2、1 项 P3。未发现新的 P0/P1，不代表对所有外部供应商或配置组合的保证。

函数级死代码已经很少，删减重点应放在重复入口、无效兼容分支和展示占位内容。按文件大小或“没有控制台按钮”直接删除协议处理器，会损害现有 SDK/API 能力。

## 已完成的提交与部署

- 原工作区支付重构与项目文档已提交并推送到 `origin/main`，提交为 `fe2675b`。
- 根目录原有 Word 申报书保留未跟踪，未纳入提交。
- 已执行 `make docker-rebuild`，依次替换两个网关副本；Nginx 配置检查和重载通过。
- 两个副本分别返回 `ready`，版本均为 `2.0.0-dev+gfe2675b`，数据库、Redis、数据面检查均为 `ok`；统一入口也通过就绪检查。
- 容器描述符上限均为 65536，PostgreSQL `max_connections=200`，部署漂移检查通过。

本报告及证据在部署后产生。**本轮没有修改或删除业务代码，以下问题均为待处理项。**

## 已确认的问题

### R01 · P2：损坏的创建密钥请求仍成功写入默认密钥

位置：[user_keys_handler.go:59](../../../internal/controlplane/user_keys_handler.go#L59)。

普通用户向 `POST /api/v1/user/keys` 提交 `Content-Type: application/json`、请求体 `{`，接口返回 200，密钥数量从 1 变为 2，新增密钥模型范围为 `['*']`、预算为 0（不设密钥预算上限）。处理器忽略 `ShouldBindJSON` 的错误，随后按默认参数签发密钥。该请求仍需有效登录，未发现鉴权绕过。

影响：调用方收到成功响应，实际创建的凭证范围却不是它提交的限制；重试还会继续增加密钥。

建议：解析失败立即返回 400，且不得写库。如果按下文 C04 合并创建入口，则直接删除这条重复写路径。验收需检查错误请求前后密钥数量不变。

### R02 · P2：损坏的 Skills/MCP 开关请求会反转状态

位置：[admin_skills.go:40](../../../internal/controlplane/admin_skills.go#L40)、[admin_mcp_mutations.go:104](../../../internal/controlplane/admin_mcp_mutations.go#L104)。

分别创建 `enabled=false` 的技能和 MCP 服务，再向各自 `/toggle` 提交 `{`。两个接口均成功返回，两个对象都变为 `enabled=true`。当前代码把解析失败、缺少字段和旧式“反转开关”合并处理。

影响：格式错误没有被拒绝，已停用的能力反而被启用；旧式反转请求在网络重试后也会再次改变状态。此问题限于已有管理权限的调用方。

建议：删除自动反转分支，要求明确的布尔值 `enabled`，重复提交同一目标状态应保持不变。现有控制台和 Skills CLI 已发送明确布尔值。验收包括损坏 JSON、缺失字段、错误字段类型和重复请求。

### R03 · P2：日志跳转 Playground 没有携带被检查的模型

位置：[LogDetailModal.jsx:333](../../../web/src/components/LogDetailModal.jsx#L333)、[App.jsx:856](../../../web/src/App.jsx#L856)。

在隔离实例创建 `audit-alpha`、`audit-omega` 两个模型，调用后打开 `audit-omega` 的真实审计行，点击标题为“在 Playground 演练测试此模型”的按钮。实际选中的模型为 `audit-alpha`。

影响：用户以为在复核原故障模型，实际测试了另一个模型，排障结果无法对应原记录。

建议：跳转同时传递模型和模态；模型不可用时明确说明。完整原始请求重放可按产品规划单独实现，本项只要求当前按钮准确选择它承诺的模型。

### R04 · P2：Gemini 原生调试吞掉安全拦截反馈

位置：[geminiPlayground.js:5](../../../web/src/lib/geminiPlayground.js#L5)、[GeminiPlayground.jsx:46](../../../web/src/components/GeminiPlayground.jsx#L46)。

浏览器经过真实网关和受控 Gemini 上游发起流式请求。网关返回 HTTP 200，响应包含 `promptFeedback.blockReason=SAFETY` 和相应 `safetyRatings`；前端不显示原因，也没有错误提示。输出处理器只保存内容、工具、媒体和用量。

影响：用户无法区分“被拦截”“空回复”和“仍在等待”，也不知道应修改输入还是排查服务。

建议：保存并显示 `promptFeedback`、`finishReason` 等终止信息，在没有内容时给出明确结果。补充安全拦截、其他终止原因和空流结束的验收。

### R05 · P2：渠道改名没有处理以名称关联的供应商报价

位置：[channel_service.go:141](../../../internal/controlplane/channel_service.go#L141)、[repo_channels.go:188](../../../internal/storage/repo_channels.go#L188)、[repo_supplier_costs.go](../../../internal/storage/repo_supplier_costs.go)。

先为 `review-provider-before + audit-omega` 保存报价，再通过现有渠道修改接口把渠道改名为 `review-provider-after`。渠道列表显示新名称，报价仍属于旧名称，接口没有提醒或迁移步骤。

影响：用户完成一次渠道编辑后，当前配置与报价的对应关系断开，需要手动发现并修复旧名称记录。历史调用中的渠道名称应继续作为历史事实保留。

建议：为当前报价建立稳定渠道关联，或在改名事务中处理当前报价及名称冲突；如果报价允许独立于渠道存在，至少在改名前展示影响和后续操作。验收包含改名成功、报价冲突回滚和历史记录保留。

### R06 · P2：“我的用量”把累计 Tokens 当作一分钟用量

位置：[MyUsageView.jsx:128](../../../web/src/components/MyUsageView.jsx#L128)、[MyUsageView.jsx:155](../../../web/src/components/MyUsageView.jsx#L155)、[settlement.go:436](../../../internal/storage/settlement.go#L436)。

页面“已用 / 限额 (TPM)”的分子来自 `api_keys.used_tokens`。结算 SQL 对该字段持续累加；实际 TPM 使用独立的 60 秒滑动窗口，见 [tpm_tracker.go](../../../internal/middleware/tpm_tracker.go)。

影响：长时间使用后，页面会持续显示累计消耗超过一分钟配额，无法用于判断何时可以重试。这里是展示口径错误，未发现限流算法因此失效。

建议：最小修复是分开显示“累计 Tokens”和“TPM 上限”，删除这个错误比例；如果需要实时水位，再使用真正的滑动窗口数据。

### R07 · P2：重建脚本把可选对象存储变成必需服务

位置：[docker-rebuild.sh:21](../../../scripts/docker-rebuild.sh#L21)。

Compose 将 RustFS 声明为 `object-storage` 可选 profile，本地存储模式不需要它；重建脚本却始终启动并等待 `storage`。用命令替身运行原脚本，模拟本地存储模式下对象存储不健康，脚本在更新任一网关之前退出。

影响：使用默认本地存储的部署也被迫启动额外服务；与当前存储驱动无关的对象存储故障能够阻断网关更新。本次现有部署使用 S3 且存储健康，因而实际重建成功。

建议：根据实际启用的存储服务决定是否启动及等待 RustFS。本项证据为原脚本的隔离命令测试，未通过改变线上存储配置来复现。

### R08 · P2：重建成功不代表负载均衡 Compose 配置已更新

位置：[docker-rebuild.sh:67](../../../scripts/docker-rebuild.sh#L67)。

脚本只有在 Nginx 文件内容哈希变化时才对 `lb` 执行 Compose `up`。单独修改端口映射、ulimits 等容器设置时，文件哈希不变，只执行 `nginx -s reload`，不会应用这些容器设置。

隔离命令测试设置新的 `GATEWAY_INGRESS_PORT`、保持文件哈希相同，原脚本成功结束，但没有任何 `compose up ... lb` 调用。没有修改真实入口端口。

影响：运维按“修改配置 → make docker-rebuild”操作后得到成功提示，入口仍可能保持旧端口或旧容器配置。

建议：常规协调 `lb` 的 Compose 配置，保留文件 inode 漂移时强制重建的处理。分别验收端口变化、ulimit 变化和绑定文件内容变化。

### R09 · P3：技能工件展示与 ZIP 不一致，自动脚本仅输出文字

位置：[SkillHubView.jsx:911](../../../web/src/components/SkillHubView.jsx#L911)、[admin_skill_artifacts.go:48](../../../internal/controlplane/admin_skill_artifacts.go#L48)。

页面显示根目录 `metadata.json` 和 `references/context.md`。实际 HTTP 下载的 ZIP 为 `SKILL.md`、`scripts/run.sh`、`references/metadata.json`，没有前述两个展示路径。生成的 `run.sh` 只有一条 `echo`，运行后退出 0 并声称正在执行工具，没有实际执行逻辑。

建议：删除自动生成的空转脚本，让文件树对应实际产物；技能 SOP 由接入客户端执行的边界应继续明确。无需为占位脚本增加一套新的执行框架。

## 建议删减与保留的代码

| 编号 | 建议 | 证据与边界 |
| --- | --- | --- |
| C01 | 删除真正未使用的常量与冗余语句 | `internal/api/stream_usage_accumulation_test.go:13` 的 `streamUsageModel` 无引用；`internal/api/gemini_inbound_embed.go:112` 的末尾 `return` 冗余。Staticcheck 分别报 U1000、S1023。收益很小，可顺手处理。 |
| C02 | 删除两个开关处理器的隐式反转分支 | 对应 R02，同时减少代码和状态不确定性；明确目标状态覆盖现有控制台调用。 |
| C03 | 删除技能的空转脚本和失真的产物树 | 对应 R09，删去没有实际执行职责的生成及展示逻辑。 |
| C04 | 合并密钥创建写入口 | 控制台 `useKeyActions.js` 和 CLI `cli_keys.go` 都使用 `POST /admin/keys`，它已有普通用户所有权约束。`POST /user/keys` 在这些生产客户端中没有消费者，却单独维护请求 DTO 和 `PlanUserKey`，且已发生 R01。保留 `/user/keys` 的 GET：`MyUsageView` 正在使用它。删除公开 POST 前应核对仓库外调用约定。 |
| C05 | 合并登录页与登录弹窗的请求和表单状态逻辑 | `AuthPage.jsx` 与 `LoginModal.jsx` 各自维护登录、注册、验证码、倒计时、OAuth 和一次性密钥交付。可保留两种外壳，共用认证表单与流程。两者已有差异：开发验证码回填、注册入口禁用后的处理并不一致。 |
| C06 | 按职责拆分大型前端组件 | Playground 1919 行、Dashboard 1875 行、Pricing 1819 行、MCP 1636 行、App 1635 行、Copilot 1488 行。先分离 Playground 各模态、定价各子视图与页面外壳；这属于职责整理，不是删除功能。项目规范的业务文件 300 行约束目前没有贯彻。 |
| C07 | 收紧 Go 检查的包范围 | 当前 `go test ./...` 会扫描 `web/node_modules/flatted/golang/pkg/flatted`。CI/vet/可达性统计应排除第三方前端依赖目录，避免把其 Go 文件算作项目业务代码；无需手工修改 node_modules。 |

本轮可达性核验：

- `deadcode ./cmd/airoute` 只有 4 个测试钩子：`forwardJSONRPCToolCall`、`MCPHandler.authenticateCaller`、`ResetReadinessChecks`、`ResetWarnGate`。纳入测试根后，项目自己的未可达函数为 0；另有 7 条来自上述第三方目录。
- 前端从 `main.jsx` 的静态及动态 import 图覆盖 85 个 JS/JSX 业务模块，没有孤立模块。15 个“没有外部生产 import 的导出”经复核是文件内部使用、默认导出别名或测试辅助，不能据此删除函数。
- 充值租约、幂等键、退款调整、整数金额字段、结算记录、迁移、停用检查、会话代际控制仍有实际职责；不能当作冗余兼容层删除。
- 原生协议、文件、批处理、Live、MCP 等 API 的用户包括外部 SDK 和智能体，控制台没有入口不等于无人使用。
- 本次没有重新加入范围裁剪中已经删除的组织、预算、资金流水账本或毛利功能。

## 用户操作闭环覆盖

| 链路 | 本轮结果 |
| --- | --- |
| 现网更新 → 双副本就绪 → 统一入口访问 | 通过；另确认 R07/R08 的其他部署组合缺口 |
| 管理员控制台导航 | 现网与隔离实例分别打开 13 个页面，均无页面异常和失败 HTTP 请求 |
| 移动端导航 | 两个实例分别抽查 6 个页面，390px 视口无整页横向溢出；未把此检查当作全部交互验收 |
| 普通用户登录、导航与权限边界 | 隔离实例通过，7 个允许页面可访问，渠道与用户管理深链回到允许页面 |
| 新账号零余额 → 兑换 → 调用 → 核账 | 通过：零余额在到达上游前返回 402；兑换 10 元，重复兑换拒绝；两次调用精确扣除 0.008 元，剩余 9.992 元 |
| 非流式/流式调用 → 审计 | 经受控上游真实 HTTP 请求通过，两次调用均产生本人可查询的审计行 |
| 管理员停用密钥 → 用户再次调用/自行恢复 | 调用立即返回 401，用户恢复请求返回 403 |
| 修改密码 → 旧登录凭证失效 | 通过，原 Bearer 会话立即返回 401 |
| 日志 → 复核模型 | 未闭环，见 R03 |
| Gemini 原生请求 → 无内容终止原因 | 未闭环，见 R04 |
| 渠道编辑 → 报价关联 | 未闭环，见 R05 |
| Skills 创建、下载、停用 → 本地使用 | 下载可用，格式错误开关和产物说明仍有 R02/R09 |
| 支付创建、补偿、退款、争议、并发 | 本地生命周期测试及真实 PostgreSQL 回归通过；现网没有配置 Stripe，外部支付验收仍未完成 |
| MCP/供应商 CSV/多协议与多模态 | 既有全量和集成回归通过；本轮新增 MCP 开关边界复现，未重新对每个外部 MCP/模型服务执行实网验收 |

## 检查结果与验收边界

| 检查 | 结果 |
| --- | --- |
| `go test -race -count=1 ./...` | 16 个有测试的项目包通过，无竞态报告；另扫描到 1 个 node_modules 内的无测试 Go 包 |
| `go vet ./...` | 通过 |
| 前端 Lint、生产构建 | 通过 |
| `npm --prefix web test -- --maxWorkers=2` | 75 个文件、746 项通过 |
| `go test -race -count=1 -tags=integration ./internal/storage ./internal/distributed` | 独立 PostgreSQL 16 和 Redis 7 临时容器通过，包含 PostgreSQL 支付生命周期测试 |
| Staticcheck | 21 条待处理：16 条错误字符串风格、2 条 TrimSuffix 简化、1 条 append 简化、1 条未使用常量、1 条冗余 return；未将其记为通过 |
| 可达性、浏览器和业务流程 | 结果及实际复现字段见 [evidence.json](evidence.json) |

现网只进行了健康检查、登录和页面读取；业务写入、合成调用、异常请求与 ZIP 执行均在无生产挂载卷的独立临时实例中完成。部署脚本边界使用命令替身，没有调整线上端口或存储配置。临时 PostgreSQL/Redis 使用独立容器与临时数据。

现网当前关闭自主注册及 GitHub/Google OAuth，Stripe 未配置；SMTP 投递、真实 OAuth、真实 Stripe Checkout/公网通知/退款，以及真实模型供应商的账号能力不在本轮实网验收结果中。其受控回归通过不能替代真实账号验收。

复核材料保存在本机 `/tmp/airoute-full-review-20261103/`，包括原始检查日志、业务流程、部署替身和生态边界验证脚本，以及 `log-to-playground.png`、`gemini-blocked.png`。仓库证据文件只记录结果，不包含现网凭据或业务请求内容。

建议处理顺序：先修 R01/R02 的错误输入写入、R07/R08 的部署行为；再修 R03/R04 的用户反馈、R05/R06 的数据对应与展示；最后清理 R09、重复认证表单和大型组件职责。
