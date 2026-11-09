审查结论：主要精简空间在仍被使用的重复逻辑、少量无效元数据和不必要的内部抽象。建议先做小范围去重，再拆分职责过重的模块。第一批预计可净减约 **150～300 行**；大组件拆分主要改善维护成本，不能把搬走的代码算作删除收益。

用户随后要求全部落实四项清单，实施与验收已经完成，详见 [IMPLEMENTATION.md](IMPLEMENTATION.md) 和 [implementation-evidence.json](implementation-evidence.json)。下文保留初审时的建议、行号和验证基线；其中预计收益与当时失败记录应结合实施结果阅读。初审阶段仅新增审查材料，后续实施已修改业务代码与回归测试。

审查基线为 `dd61f37` 加当前工作区的未提交内容，包括通知中心和思考能力探测等新增文件。开始时工作区已有 90 条状态记录。通过 SHA-256 对照确认，审查期间纳入快照的 1,208 个已有文件未发生变化。汇总证据见 [evidence.json](evidence.json)。

统计只包含 `cmd/`、`internal/`、`web/src/` 下的 Go、JS、JSX 代码，排除测试用例和 `web/src/test/`；行数包含注释及空行，不含 CSS、JSON、依赖或构建产物。

| 范围 | 业务文件 | 物理行数 | 超过 300 行的文件 |
| --- | ---: | ---: | ---: |
| Go | 376 | 64,702 | 52 |
| Web | 114 | 33,666 | 33 |
| 合计 | 490 | 98,368 | 85 |

仅按测试文件后缀过滤时还会计入 18 行前端测试初始化代码，因此此前进度消息的口径为 98,386 行。两个口径的超长文件数一致。

Go AST 扫描得到 1,905 个有函数体的命名函数，其中 140 个超过 60 行，包含迁移和静态声明性质的函数。85 个超长文件需要结合例外判断：DDL 迁移可适用规范例外，`migrations_partition.go` 中的运行逻辑仍需要单独审查，不能因文件名带 migrations 就整体豁免。

全仓检查包括 Go 可达性、函数体 token 比较、前端静态和动态 import 图、参数引用、文件规模与现有测试。人工重点复核请求处理、路由、适配、计费、持久化、异步任务及大型前端页面。该范围不等同于逐条实网调用全部供应商、支付和 MCP 服务。

**优先执行的净减项**

| 编号 | 建议 | 预计净减 | 实施风险 |
| --- | --- | ---: | --- |
| D01 | 复用已有 OpenAI 错误响应函数 | 50～60 行 | 低 |
| D02 | 合并相同投影的数据库行解码 | 40～90 行 | 低至中 |
| D03 | 统一两个渠道新建动作 | 20～35 行 | 低 |
| D04 | 简化 Prometheus 标量输出模板 | 40～80 行 | 低至中 |
| D05 | 删除无消费的预设标记、参数和恒定状态 | 10～20 行 | 低 |
| D06 | 清理仅供测试使用的生产包装函数 | 15～25 行 | 低 |

区间包含辅助函数和调用点的成本，仍需用实际 diff 核算。优先实施 D01、D03、D05，然后独立处理 D02、D04、D06，避免一次变更同时影响多个关键链路。

**D01：三个入口仍手写了已有公共函数负责的错误结构。**

位置：[handler.go:60](../../../internal/api/handler.go#L60)、[responses_handler.go:41](../../../internal/api/responses_handler.go#L41)、[models_handler.go:42](../../../internal/api/models_handler.go#L42)。三个文件合计 10 处完整的 OpenAI `error.message/type/code` 构造，可以直接调用 [RespondOpenAIError](../../../internal/api/common.go#L472)。现有实现只调用 `c.JSON`，不会额外 Abort 或改变控制流。

每个调用保留原有 HTTP 状态、message、type、code 和后续 return。各入口的英文文案目前不同，传入原文即可，无须统一文案或改变校验顺序。不要顺带把 Anthropic、Gemini 的错误格式改成 OpenAI 格式。

验收：复用 `common_test.go` 的响应格式测试，并核对上述入口的错误状态和原始字段。此项主要是替换表达方式，不需要引入新的错误框架。

**D02：相同行投影被反复 Scan 和解码。**

位置：

- [repo_channels.go:23](../../../internal/storage/repo_channels.go#L23) 已有 `scanChannel(*sql.Row)`，但 [ListChannels](../../../internal/storage/repo_channels.go#L72) 又复制了一套 18 列 Scan 和 JSON 解码。
- [repo_keys.go:225](../../../internal/storage/repo_keys.go#L225) 的 `ListAPIKeys`、`ListAPIKeysByUser`、`GetAPIKey` 重复 19 列投影、allowed_models 解码和金额归一化。
- [repo_users.go:56](../../../internal/storage/repo_users.go#L56) 的用户名、ID、邮箱查询重复 11 列解码及 `resolveBalanceUnits`。
- [repo_pricing.go:136](../../../internal/storage/repo_pricing.go#L136) 的列表和精确查询重复 17 列解码及两个布尔转换。

沿用仓库已有的 `interface{ Scan(...any) error }` 模式，让 `*sql.Row` 与 `*sql.Rows` 共享各自领域的解码函数。公告和充值订单仓储已经使用这种写法，无须引入 ORM 或通用 CRUD 层。

只合并解码步骤。查询条件、Reader/写池选择、事务行锁、Rebind 和上下文仍留在原调用点。鉴权查询包含用户状态等额外列，用户列表没有 password_hash，应保留各自的投影边界。保持现有 NULL、空 JSON、坏 JSON、金额归一化、`sql.ErrNoRows`、nil/空列表及 `rows.Err()` 语义。

验收：渠道、密钥、用户、价格仓储回归及真实 PostgreSQL 路径；至少核对列表和单条读取对相同数据的解码结果。禁止为了复用解码而扩大返回字段或改变锁的范围。

**D03：渠道新建动作在 App 与列表页各实现一次。**

位置：[App.jsx:1649](../../../web/src/App.jsx#L1649) 的命令面板回调与 [ChannelsView.jsx:118](../../../web/src/components/ChannelsView.jsx#L118) 的 `openNewChannelModal`，函数体 token 完全相同。列表页为此接收了四个状态 setter。

将新建动作放回已有 [useChannelActions](../../../web/src/hooks/useChannelActions.js#L13)，向两个入口提供同一个 `openCreateChannel` 回调。列表页可同时减少四个 setter 参数及相应装配。

关键边界：这两个入口当前默认 `gpustack` 和五种协议；[emptyChannel](../../../web/src/lib/providerPresets.js#L21) 当前默认 `openai` 和完整协议集，不能直接替换。共用动作应保留现有表单对象、清理探测结果和打开弹窗的顺序。若后续统一初始表单默认值，应另算行为变更。

验收：列表按钮与命令面板打开后的字段、协议选择和探测状态一致，仍可正常编辑已有渠道。

**D04：Prometheus 导出存在 46 组重复的三行标量模板。**

位置：[metrics_prometheus.go:15](../../../internal/telemetry/metrics_prometheus.go#L15)。`ToPrometheusFormat` 共 281 行，其中 46 组依次写 HELP、TYPE、值，适合用一个小型输出函数表达。

保留指标名、HELP 文本、counter/gauge 类型、整数与 `%.2f` 格式和输出顺序。Histogram、带标签序列及快照采集独立保留；不要把整数 counter 统一转成 float64，也不要扩大现有读锁范围。无需新增 Prometheus 依赖。

验收：固定快照下核对全部标量及 histogram 的名称、类型、数值和格式；map 产生的标签序列按集合比较。现有测试只覆盖部分输出，实施时应补充完整输出契约检查。

**D05：少量明确没有业务消费者的代码可直接简化。**

- [providerPresets.js:60](../../../web/src/lib/providerPresets.js#L60) 等六处 `keepEndpoint: true` 无读取者。[buildPresetChannel](../../../web/src/lib/providerPresets.js#L300) 始终按 `previous.base_url || preset.patch.base_url || ''` 合并端点。删除标记可保持当前行为；`keepModels` 有实际读取，继续保留。
- `AiCopilot`、`DashboardView`、`McpIntegrationView`、`PlaygroundView`、`SkillHubView` 的解构参数 `t` 在函数中没有引用，可同步删除参数和 App 中的传值。
- [PlaygroundView.jsx:160](../../../web/src/components/PlaygroundView.jsx#L160) 的 `imgQuality`、`ttsSpeed` 没有 setter，初值也是字面量，可改成常量。
- [AiCopilot.jsx:133](../../../web/src/components/AiCopilot.jsx#L133) 与 144 行的按钮位置修正回调完全重复，可在同一 effect 内复用位置修正函数，收益很小，适合随该文件整理。

不要批量替换所有“没有 setter 的 useState”。`ModelRouteModal.currency` 当前保存初始 props 快照，`WalletManagementView.checkoutReturn` 保存初始 URL 状态；直接改成每次渲染重新计算会改变语义。

**D06：两个包装函数仅被同包测试调用，可让测试直接覆盖实际实现。**

生产入口可达性检测列出四项；纳入默认构建标签下的测试入口后为零：

| 符号 | 当前用途 | 建议 |
| --- | --- | --- |
| [forwardJSONRPCToolCall](../../../internal/api/mcp_client.go#L400) | 一个测试调用，内部仅补空 transport 后转发 | 测试直接调用 `forwardHTTPMCPToolCall`，删除包装与过时历史调用点注释 |
| [MCPHandler.authenticateCaller](../../../internal/api/mcp_handler.go#L102) | 同包测试调用；实际链路使用 digest 版本 | 测试显式 HashAPIKey 后调用 `authenticateCallerDigest`，保留原有鉴权断言 |
| [ResetReadinessChecks](../../../internal/api/ops_handler.go#L56) | 仅 `readiness_test.go` 使用 | 可移入同包测试文件；这只缩小生产代码，不计为总代码净减 |
| [ResetWarnGate](../../../internal/telemetry/context.go#L72) | 多个其他包的测试导入 | 保留；直接移到 telemetry 的 `_test.go` 会破坏其他包测试编译 |

不要删除这些函数对应的回归断言。尤其鉴权测试应继续检查“从未创建密钥才开放匿名访问、停用所有密钥后仍拒绝匿名访问”。

**下一批重构的收益和边界**

| 编号 | 建议 | 收益性质 | 风险 |
| --- | --- | --- | --- |
| R01 | 共用两处 Chat Completions 流读取逻辑 | 预计净减 20～40 行，减少重复修补 | 中 |
| R02 | 收敛前端重复 JSON 请求处理 | 预计净减 50～150 行，依具体错误语义而定 | 中 |
| R03 | 简化内部协议适配器注册与接口 | 预计净减 50～100 行 | 中 |
| R04 | 提炼 Embedding/Rerank 的共同尝试循环 | 预计净减 60～100 行 | 高于前述项目 |
| R05 | 按职责拆分大型文件和组件 | 主要改善职责与可测试性，不承诺净减 | 中至高 |
| R06 | 修正文档与当前门禁的偏差 | 减少后续重复实现和流程误读 | 低 |

**R01：只先合并两处相同协议的流读取。**

[PlaygroundView.jsx:543](../../../web/src/components/PlaygroundView.jsx#L543) 和 [AiCopilot.jsx:702](../../../web/src/components/AiCopilot.jsx#L702) 都自己管理 reader、TextDecoder、残留 buffer、逐行 data、EOF 补帧和释放。

提取只负责读取及交付 data 内容的函数，让 Playground 保留 TTFT、token 速度和内容展示，让 Copilot 保留工具调用拼接及多轮执行。保留首字节计时点、坏 JSON 的处理、流内错误抛出、末尾无换行事件、停止后的部分结果和 finally 释放。

[readGeminiSSE](../../../web/src/lib/geminiPlayground.js#L49) 当前按事件分段并拼接多行 data，其行为与上述逐行解析不同。先保留 Gemini 的解析方式；不能为复用函数而丢掉其多行事件支持。现有 Gemini 分片 UTF-8/多行事件测试及 Copilot 流错误测试应继续通过。

**R02：复用现有请求层，逐个保留业务成功和错误语义。**

已有 [apiJSON](../../../web/src/api/client.js#L114)，但 `useChannelActions`、`useKeyActions`、`UserManagementView`、`WalletManagementView` 和 `PricingManager` 仍有大量 `fetch → json → res.ok/code → toast` 重复实现。

先迁移成功条件与 `apiJSON` 一致的增删改请求。保留返回 envelope，使 202 已保存但热加载未完成时的 warning、创建密钥的一次性凭据和支付 checkout_url/status 仍由原业务处理。原有中英文 fallback 文案也属于行为，需要逐个保留。

不要把所有请求换成同一个 hook。[notificationJSON](../../../web/src/lib/notifications.js#L7) 要求非空响应且 `code === 0`，`apiJSON` 则允许缺少 code；`useConsoleData` 又有自己的权限错误容忍与会话代际检查。SSE、下载、Blob 和上传仍需要原始 Response。

**R03：当前动态适配器注册只服务一个内置实例。**

[adapter.go](../../../internal/adapter/adapter.go#L11) 维护互斥锁、全局 map、Register、Get 和双向 ProtocolAdapter。实际注册点只有 init 中的 Gemini；生产 Get 调用仅为固定字符串 Anthropic 和 Gemini。Anthropic 从未注册，每次都走构造器回退；它为满足双向接口还实现了始终拒绝的 `FromCanonical`。

可以改成明确的、有类型的转换调用，删除注册表、恒定命中的查找/回退和没有职责的出站方法。保留 Anthropic 入站转换与 API 包中的真实 Messages 响应渲染，继续验证签名、缓存 token、工具历史和原生透传。删除拒绝方法应同时消除该出站调用能力，不能恢复早先生成无签名 thinking block 的转换。

这是内部抽象收缩；与 Dispatcher 的多供应商注册和选择不同，不应扩大到删除供应商策略接口。

**R04：两条非流式模态尝试循环高度重复，但需要先固定边界。**

位置：[dispatcher_modality_unary.go:24](../../../internal/router/dispatcher_modality_unary.go#L24) 的 `dispatchEmbeddingOnce` 与 136 行的 `dispatchRerankOnce`。预算检查、熔断准入、provider 查找、通道并发准入、取消分类、404 继续、致命 payload 停止及成功记账结构基本一致。

可提炼两者共同的尝试流程，只由类型明确的调用适配提供能力检查、实际请求和 token 读取。保留各自英文错误文案、Embedding PromptTokens 与 Rerank TotalTokens 的区别，以及 attempted/skipped 的归属。

先评估实际补丁是否减少理解成本。若需要大量回调或布尔开关才能容纳两种请求，就只抽取公共决策步骤。Chat、SSE 和通用 HTTP 响应体的生命周期分别保留：首个有效输出前可否切换、取消释放时机和并发槽持有期均有不同约束。

验收重点是现有模态 404/fatal/fallback、预算截止、取消信息、半开探针、会话亲和及 HTTP body close 前占用通道的回归。

**R05：超长文件应拆分职责，先处理界面和公共杂项。**

| 位置与规模 | 建议边界 | 行为保持重点 |
| --- | --- | --- |
| [App.jsx](../../../web/src/App.jsx#L157)，1,706 行，App 函数 1,550 行 | 会话/导航协调、Console 外壳、页面内容装配、弹窗装配 | 登录与角色跳转、URL 同步、请求代际、退出时清理顺序 |
| [DashboardView.jsx](../../../web/src/components/DashboardView.jsx#L32)，1,875 行 | 数据加载、纯计算、趋势图、注意事项、表格 | key 切换隔离、实时轮询、无样本与零值的区别 |
| [PlaygroundView.jsx](../../../web/src/components/PlaygroundView.jsx#L38)，1,859 行 | Chat/Arena、各模态面板、请求构造、结果显示 | 切换模态后草稿和结果仍保留；停止逻辑、媒体 URL 清理不变 |
| [PricingManager.jsx](../../../web/src/components/PricingManager.jsx#L136)，1,831 行 | 模型价格表、价格编辑弹窗、分组费率、时段转换与校验 | 分组价格、离峰窗口、保存 warning、未保存内容保护 |
| [McpIntegrationView.jsx](../../../web/src/components/McpIntegrationView.jsx#L216)，1,636 行 | 服务列表与操作、连接配置、工具探测、文档展示 | 配置生成继续复用已有 `buildMCPClientConfig` |
| [AiCopilot.jsx](../../../web/src/components/AiCopilot.jsx#L45)，1,383 行 | 对话执行、工具循环、流内容处理、浮窗拖动、展示 | 多轮工具状态、取消、失败后的部分输出 |
| [common.go](../../../internal/api/common.go#L195)，1,052 行 | usage 准备与结算、token 估算、协议错误、身份读取、错误脱敏 | RecordUsage 入口保持；SSE 最终帧与结算的先后关系保持 |
| [response.go](../../../internal/model/response.go#L116)，747 行 | 类型定义、输入转换、会话合并、输出转换 | 未知原生字段与工具调用信息不丢失 |
| [auth.go](../../../internal/middleware/auth.go#L343)，687 行，核心函数 278 行 | 凭据解析、账号准入、额度预授权与释放 | 逐请求停用检查、余额未知时拒绝、预约归还 |
| [thinking.go](../../../internal/provider/thinking.go#L15)，530 行，PrepareThinkingRequest 235 行 | 控制意图解析、策略约束、请求复制、不同 wire 写入 | 每候选独立处理；未显式请求时原生参数不变；原始路由限制不丢失 |
| [async_logger.go](../../../internal/storage/async_logger.go#L462)，727 行，Redis worker 252 行 | 本地队列、Redis 消费、spill 回放与状态协调 | ACK 在成功持久化之后、失败重投、停机 drain 与幂等结算 |
| [db.go](../../../internal/storage/db.go#L320)，627 行 | 方言与 SQL 绑定、连接池初始化、SQLite checkpoint | 读写池分离、超时和事务行为 |
| [server.go](../../../cmd/airoute/server.go#L28)，561 行，runServer 377 行 | 配置与安全检查、基础设施初始化、服务装配、启动与停机 | 提前注册 signal、就绪门禁、Redis/日志初始化依赖与 defer 关闭顺序 |

`GetStatsTimeSeries` 为 259 行，`GetSupplierReconciliation` 为 293 行，可按 SQL 查询与结果装配阶段拆分；先保持 SQL 和聚合口径，不同时优化查询。

前端拆组件尤其要防止状态生命周期变化：目前 Playground 在父组件持有各模态状态；改成条件挂载的独立面板后，未选中的面板会卸载。应保留状态所有权或保持面板实例，先通过切换后草稿、结果与取消行为的回归再调整。

App 的导航还存在小范围重复：[SIDEBAR_ITEMS](../../../web/src/App.jsx#L131)、1083 行起的标题判断、[CommandPalette 导航项](../../../web/src/components/CommandPalette.jsx#L63) 及 `consoleRoute.CONSOLE_TABS`。可共用导航元数据和跳转动作，但保留命令面板的搜索文案、排序及角色差异。现有 `navGroups.allowedTabsFor` 已统一角色可见范围，继续复用。

**R06：工程文档中的门禁描述已落后于实际代码。**

[ENGINEERING_GUIDELINES.md:154](../../ENGINEERING_GUIDELINES.md#L154) 指向不存在的 `.github/workflows/quality-gate.yml`，实际单一门禁定义在 [ci.yml](../../../.github/workflows/ci.yml)。文档还将前端 lint 描述成可有 N 条告警的基线；[package.json](../../../web/package.json#L11) 已要求零告警。修正文档即可，不应再增加一套门禁实现。

旧审查中的部分建议已经完成：AuthPage 与 LoginModal 当前共用 `useAuthForm`，Go 检查也已排除 `web/node_modules`。两种登录外壳仍有相似 JSX，但样式、容器和交互不同，继续合并应以具体字段重复为依据，不重复重做认证状态层。

**本轮不列入优先精简的部分**

- 前端 114 个模块均可由 main.jsx 的静态/动态 import 图到达，没有发现整模块孤立代码。Go 纳入测试后没有不可达函数，这不意味着不存在重复抽象，但不支持大批删除业务模块。
- 三类入口协议、原生透传、流式与非流式、MCP 的不同 transport 都有实际职责。控制台没有按钮不能证明 API 无人使用。
- 计费幂等键、整数金额、settlement、spill、财务归档和历史价格快照继续保留。D02 合并解码不等于删除这些状态。
- `existingUsageBillingIDs` 与 `existingUsageLogIDs` 两个 34/35 行函数可考虑共用分块集合查询，但后者有“仅匹配空 billing_id”的条件。为约二十行收益触及财务保留流程，优先级低于前述项目。
- 保留任务 worker 的两个相似定时循环仅约 19 行，启动延迟分别为 5/15 分钟；先不建立通用后台任务框架。
- 数据库迁移以及当前仍接受的字段、协议名和配置格式不因“兼容代码”标签而删除。删除它们需要明确升级范围，超出本轮功能保持前提。

**验证结果与后续验收依据**

| 检查 | 本轮结果 |
| --- | --- |
| Go 版本 | 1.26.1，与 go.mod 一致 |
| 前端正式验证运行时 | Node.js 22.23.4 |
| 前端 lint | 零告警通过 |
| 前端测试 | 87 个文件、790 项通过 |
| 前端生产构建 | 通过 |
| Go vet | 全部项目包通过，排除 node_modules |
| Go 网关编译 | 通过，产物写入 /tmp，未启动服务 |
| Go 全量 `-race -count=1` | 15 个有测试包通过；storage 的一个用例失败，无 data race 报告 |
| 失败用例单独 `-race -count=3` | 3 次通过 |
| storage 整包单独 `-race -count=1` | 通过，200.458 秒 |
| 默认构建标签的 Go 可达性 | 生产入口只有上述 4 个测试用途符号；纳入测试后为 0 |
| PostgreSQL/Redis 集成、e2e、真实上游调用 | 本轮未执行 |

首次失败为 [usage_spill_replay_test.go:139](../../../internal/storage/usage_spill_replay_test.go#L139) 的 `TestUsageSpillReplayPicksUpAnotherProcessFile`，报 `peer spill file was not replayed by the periodic pass: rows = 0`。单独三次和整包复跑均通过，原因仍未确认，不能把第一次全量结果记为通过。异步日志相关重构开始前应先定位这项间歇失败，保留回归断言，不以延长等待或删除测试替代排查。

后续每批分别验收 HTTP 成功与失败契约、请求头、状态保留、流式取消、账务结果和并发时序。已有测试覆盖较多，但测试通过本身不足以证明拆分前后完全等价；新增或修改测试时按工程指南执行对应性质的变异验证。涉及 SQL、行锁、Redis 和共享状态的批次，再执行真实服务集成及竞态测试。

建议落地顺序为：先固定上述测试基线，再实施 D01/D03/D05 的局部清理；随后独立处理数据库解码与指标输出；再做前端流读取、请求层及适配器收缩；最后逐个拆分大型组件与关键并发模块。每批同时记录生产代码净减、辅助代码增量和行为验证结果。
