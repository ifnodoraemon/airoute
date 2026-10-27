# 功能流程与软件细节审查（2026-10-26）

> 更新：本报告记录修复前的发现。18 项问题的处理结果与当前验证见 [2026-10-27 修复记录](functional-details-remediation-2026-10-27.md)。

本轮发现 **18 项问题：6 项 P1、12 项 P2**。其中 **17 项有定向自动复现，1 项为静态确认的集群兼容性问题**。主要风险集中在身份关联、密码变更后的会话、计费币种、配置保存和多模态操作链路。

审查基于提交 `6aec1ab66fd51c2bf645bc8a637e14c39c42707a` 及本轮检查时的工作区。期间其他工作更新了计费、路由、配置和前端 lint；本报告核对了相关源码哈希，并复跑了测试基线。收尾时又出现渠道超时和 nginx 超时调整，未重新运行这些后续改动的完整测试；已核对它们没有改变下述问题对应的逻辑，17 个归档源码哈希仍全部匹配。此次审查仅新增报告和复现材料，没有实施业务修复或部署。

P1 表示应优先处理的账号安全、金额正确性或主要功能故障；P2 表示有明确触发条件的流程、数据呈现或操作反馈缺陷。不把静态代码风格问题计入这 18 项。

验证结果：

| 验证 | 最终结果 | 范围 |
|---|---|---|
| `go test -race -count=1 ./...` | 通过 | 上述审查基线的默认 Go 测试集及竞态检测；不含收尾时出现的后续超时改动 |
| `npm run lint` | 通过 | 首轮曾有 8 条告警；并行发生的 lint 清理后已消除 |
| `npm test` | 37 个文件、503 项通过 | 现有 jsdom 单元/组件测试 |
| `npm run build` | 通过 | React 生产构建 |
| 本轮定向探针 | 17 项暴露缺陷 | 8 项 Go、9 项前端；断言期望行为，所以修复前预期失败 |

既有测试通过与上述缺陷并不矛盾：既有测试多数覆盖正常路径，没有覆盖本报告中的同名身份、读取失败、响应乱序和跨渠道任务等边界。[复现材料与运行方法](2026-10-26-repro/README.md)包含探针源码、原始结果和源码哈希，探针以 `.txt` 归档，不进入正常 CI。

1. **F01 · P1 · Google OAuth 使用显示名称关联身份，可能登录到其他账号。**

   位置：[oauth_providers.go:125](/home/ifnodoraemon/myagent/airoute/internal/controlplane/oauth_providers.go:125)、[oauth_handler.go:192](/home/ifnodoraemon/myagent/airoute/internal/controlplane/oauth_handler.go:192)。

   Google 返回的 `name` 被用作用户名，随后优先按 `gg_<name>` 查找已有账号；查中后没有确认 provider subject 或邮箱是否属于同一人。显示名称相同的两个独立身份因此可能合并。既有账号若是管理员，签发的也是其管理员权限。

   复现：已有 `gg_Common Name` 管理员，模拟 Google 返回不同 `id`、不同邮箱但相同 `name`，最终得到该已有管理员的登录结果。探针：`TestAuditReviewGoogleDisplayNameDoesNotIdentifyAccount`，真实交换分支、合法 state/cookie，未使用模拟登录捷径。

   修复方向：使用唯一的 `(provider, subject)` 绑定账号；显示名称仅作资料。绑定已有本地账号需要验证其所有权，不能因用户名相同自动关联。GitHub 分支也应使用稳定的 provider ID。

2. **F02 · P1 · 修改密码不会撤销旧会话。**

   位置：[account_handler.go:180](/home/ifnodoraemon/myagent/airoute/internal/controlplane/account_handler.go:180)、[auth.go:90](/home/ifnodoraemon/myagent/airoute/internal/controlplane/auth.go:90)。

   修改密码只更新密码哈希。登录凭证没有会话版本，认证中间件只检查账号状态、角色和凭证有效期。用户修改密码或管理员重置密码后，已持有的旧凭证仍然有效；界面要求重新登录无法撤销其他设备的会话。

   复现：先签发会话，调用真实改密处理器成功，再用原凭证访问受中间件保护的 `/me`，仍为 HTTP 200。探针：`TestAuditReviewPasswordChangeRevokesOldSession`。

   修复方向：为用户维护凭证版本或撤销时间，改密/重置时原子更新，并在每次认证时校验；同步处理当前浏览器会话。

3. **F03 · P1 · USD 定价直接按原数值扣减人民币钱包。**

   位置：[pricing_service.go:159](/home/ifnodoraemon/myagent/airoute/internal/controlplane/pricing_service.go:159)、[engine.go:415](/home/ifnodoraemon/myagent/airoute/internal/billing/engine.go:415)、[wallet_deduct.go:49](/home/ifnodoraemon/myagent/airoute/internal/storage/wallet_deduct.go:49)。

   定价接口接受 `currency=USD`，价格列表也显示美元符号，但计费计算不换算币种。最终费用作为人民币金额扣入钱包，审计快照仍保留 USD，造成展示、账单和余额的语义不一致。

   复现：通过 PricingService 保存每百万输入 token 为 USD 1 的价格，计算并走真实批量记账，人民币余额从 10 变为 9。探针：`TestAuditReviewForeignCurrencyDoesNotDebitCNYAtFaceValue`。此结论不假定任何实时汇率，而是确认了没有换算或拒绝处理。

   修复方向：当前人民币钱包应拒绝非 CNY 定价，或明确实现计价币种到结算币种的转换，并将所用汇率、原币金额、结算金额写入快照。

4. **F04 · P1 · 读取旧定价失败后，局部更新仍可能把其他费率清零。**

   位置：[route_service.go:152](/home/ifnodoraemon/myagent/airoute/internal/controlplane/route_service.go:152)。

   `GetModelPriceWithGroup` 的错误被忽略。读取失败和没有旧价格被当成同一种情况，未提供的输入、输出和缓存价格保持零值，后续仍可提交。短暂查询故障恢复后，这次写入就会覆盖原有价格。

   复现：注入读取失败，仅修改固定费用，仍执行一次保存，生成的 prompt/completion 价格均为 0。探针：`TestAuditReviewPriceReadFailureDoesNotOverwriteRates`。

   修复方向：只允许明确的“没有记录”进入新建默认值路径；其他读取错误必须终止更新。部分更新所依赖的读取和保存应保持一致性。

5. **F05 · P1 · 常见 `/v1` Base URL 在多模态请求中被重复拼接。**

   位置：[dispatcher_modality.go:73](/home/ifnodoraemon/myagent/airoute/internal/router/dispatcher_modality.go:73)。

   聊天请求在 Base URL 后追加 `/chat/completions`，多模态则直接追加含 `/v1` 的完整请求路径。同一渠道配置 `https://host/v1` 时，聊天可用，图像、语音、视频会访问 `/v1/v1/...`。带其他 API 前缀的渠道也会多出不需要的 `/v1`。

   复现：调用实际图像处理器，模拟 Transport 捕获到 `/v1/v1/images/generations`。探针：`TestAuditReviewMultimodalPreservesAPIBasePath`。既有多模态测试仅用不带 `/v1` 的根地址，未覆盖该配置。

   修复方向：统一 Base URL 的含义和路径拼接函数，并覆盖根地址、`/v1`、供应商自定义前缀与末尾斜杠。

6. **F06 · P1 · 视频任务查询没有绑定创建任务的渠道。**

   位置：[video_handler.go:61](/home/ifnodoraemon/myagent/airoute/internal/api/video_handler.go:61)、[dispatcher_modality.go:49](/home/ifnodoraemon/myagent/airoute/internal/router/dispatcher_modality.go:49)。

   创建和查询分别执行普通路由选择，网关没有保存 task ID 对应的创建渠道。多个供应商提供同一视频模型时，查询会负载均衡到另一个供应商，该供应商当然没有这个任务。即便前端补上正确模型参数也无法解决。

   复现：两个同权重渠道，真实视频处理器在 A 创建，下一次查询发到 B，返回 404。探针：`TestAuditReviewVideoPollReturnsToCreatingProvider`。

   修复方向：保存 task ID、创建渠道、模型及所属用户的关联；查询按该关联访问上游，并校验任务所有权。双节点部署需要共享关联信息。

7. **F07 · P2 · 保存模型映射会截断含冒号的模型名。**

   位置：[useChannelActions.js:162](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useChannelActions.js:162)、[ModelMappingEditor.jsx:18](/home/ifnodoraemon/myagent/airoute/web/src/components/ModelMappingEditor.jsx:18)。

   表格允许输入 `qwen3:32b`，但提交时用 `split(':')` 后只取两个元素。`public-model:qwen3:32b` 最终变为 `public-model → qwen3`，保存成功提示会掩盖实际改写。来源模型本身含冒号时，字符串格式也不能无歧义表示。

   复现：检查真实保存动作的 JSON 请求，期望 `qwen3:32b`，实际 `qwen3`。探针：`preserves model tags in a channel mapping payload`。

   修复方向：表单中使用结构化的映射数组/对象，避免通过冒号和逗号往返序列化；补充来源和目标均含冒号的用例。

8. **F08 · P2 · 已关闭渠道的探测响应会覆盖另一渠道的编辑表单。**

   位置：[useChannelActions.js:43](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useChannelActions.js:43)。

   探测完成后直接修改当前 `newChannel`，没有请求版本、渠道 ID、URL 一致性检查或取消机制。探测 A 后关闭弹窗再编辑 B，A 的迟到结果会写入 B 的模型、协议、类型，部分情况还会改地址。

   复现：延迟 A 的响应，打开 B 后再返回，B 的 `second-model` 变成 A 的 `from-first-probe`。探针：`ignores a channel probe response after a different channel is opened`。

   修复方向：探测绑定编辑实例及请求参数；切换、关闭和更改地址时取消请求，返回时再校验版本。

9. **F09 · P2 · 渠道体检遗漏协议配置，正常纯补全渠道被判失败。**

   位置：[admin_channels_probe.go:100](/home/ifnodoraemon/myagent/airoute/internal/controlplane/admin_channels_probe.go:100)。

   体检构造 `ChannelConfig` 时没有复制 `Protocols`。实际转发支持的 `openai_text` 适配因此未触发，体检调用 `/chat/completions`，而渠道只提供 `/completions`。这会误导用户修改或停用正常渠道。

   复现：配置只支持 `openai_text` 的渠道，模拟上游正常响应 `/v1/completions`；体检实际访问 `/v1/chat/completions` 并报告失败。探针：`TestAuditReviewChannelTestRespectsTextProtocol`。

   修复方向：体检复用实际配置和协议选择逻辑。图像、音频、视频专用渠道也应按能力检查，不能默认等同于聊天渠道。

10. **F10 · P2 · 日志导出中途失败，仍提示导出了全部匹配记录。**

    位置：[useLogsData.js:203](/home/ifnodoraemon/myagent/airoute/web/src/hooks/useLogsData.js:203)、[LogsView.jsx:339](/home/ifnodoraemon/myagent/airoute/web/src/components/LogsView.jsx:339)。

    分页导出遇到 HTTP 错误直接 `break`，返回已取得的数据且 `truncated=false`。调用方据此生成 CSV 并提示“全部匹配记录”。这类静默缺行尤其影响对账和事故追踪。

    复现：第一页返回 200 行，第二页返回 503，导出仍成功返回 200 行且未标记不完整。探针：`reports a failed second export page instead of returning a complete export`。

    修复方向：区分完成、上限截断、请求失败、会话取消；失败时拒绝完整性声明，允许用户明确选择保留部分结果。

11. **F11 · P2 · 调试台吞掉大多数 SSE 错误。**

    位置：[PlaygroundView.jsx:550](/home/ifnodoraemon/myagent/airoute/web/src/components/PlaygroundView.jsx:550)。

    收到 SSE `error` 时抛出其 message，随后只在 message 包含英文 `stream error` 时提示。实际 message 如 `upstream overloaded`、超时和限流通常不包含这个字符串，界面会显示空输出或把半截回答当作完成。

    复现：发送合法错误事件及 `[DONE]`，没有任何错误提示。探针：`surfaces a stream error even when its message does not contain stream error`。

    修复方向：按事件结构识别错误，分开处理 JSON 解析失败和业务错误；标记本轮失败并保留部分内容，避免写入完整 assistant 历史。

12. **F12 · P2 · 从密钥页进入仪表盘或返回仪表盘时，统计卡片可能显示全局数据。**

    位置：[DashboardView.jsx:63](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:63)、[DashboardView.jsx:154](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:154)、[DashboardView.jsx:245](/home/ifnodoraemon/myagent/airoute/web/src/components/DashboardView.jsx:245)。

    `selectedKey` 初始化为传入值，effect 仅在传入值与内部值不同时请求该密钥概览。首次挂载时两者相等，不请求 key overview，卡片回退到全局 `stats`；趋势和日志却按密钥过滤。同页因此存在两个不同范围。另外，15 秒自动刷新没有更新 `keyStats`，点击选定密钥后卡片也会长期停留在旧值。

    复现：携带 `selectedKey` 挂载，没有发出带 key 的 overview 请求。探针：`loads selected-key overview when returning to the dashboard`。

    修复方向：数据请求统一由当前选择驱动，覆盖首次挂载、改变选择和刷新；响应也要校验选择版本。

13. **F13 · P2 · 公告编辑在配置加载完成前可操作，迟到响应会覆盖草稿。**

    位置：[AnnouncementModal.jsx:20](/home/ifnodoraemon/myagent/airoute/web/src/components/AnnouncementModal.jsx:20)。

    打开公告后立即允许输入和保存，加载结果无条件写回所有字段。慢网络下新输入被旧配置覆盖；若加载失败或尚未完成，默认关闭、空白内容也可能被保存成新的公告配置。

    复现：先输入 `My new draft` 再返回加载结果，标题变为 `Old published title`。探针：`does not overwrite an announcement draft with a late initial load`。

    修复方向：区分加载、可编辑、失败状态，未取得可信配置前禁用保存；已编辑后不要应用旧加载响应，并对非 2xx 显示错误。

14. **F14 · P2 · 模型路由编辑缺少未保存退出保护。**

    位置：[ModelRouteModal.jsx:58](/home/ifnodoraemon/myagent/airoute/web/src/components/ModelRouteModal.jsx:58)、[ModelRouteModal.jsx:270](/home/ifnodoraemon/myagent/airoute/web/src/components/ModelRouteModal.jsx:270)。

    渠道、用户和密码等表单已使用未保存保护，模型路由弹窗仍在 Escape、背景点击、关闭按钮中直接关闭。修改多个服务商绑定或价格后，一次误触就丢失草稿。

    复现：价格从 2 改为 7，按 Escape，直接触发 `onClose`。探针：`guards unsaved model routing changes when Escape is pressed`。

    修复方向：接入现有 `useFormDirty` / `useUnsavedGuard`，保存成功直接关闭，其他退出方式统一经过保护。

15. **F15 · P2 · 模型路由局部更新会把原币种重置为 CNY。**

    位置：[route_service.go:141](/home/ifnodoraemon/myagent/airoute/internal/controlplane/route_service.go:141)。

    币种在读取旧记录前被默认成 CNY，并未继承原值。客户端仅修改一个价格而省略 `currency` 时，原有 USD 记录会被改为 CNY。此问题独立于 F03：即使日后实现汇率转换，也仍需正确保留局部更新中遗漏的字段。

    复现：已有 USD 记录，仅提交 `prompt_price`，待写入记录的币种变为 CNY。探针：`TestAuditReviewPartialPriceUpdatePreservesCurrency`。

    修复方向：遗漏字段继承现有值，只有新建时使用默认值；币种变更需要显式输入及校验。

16. **F16 · P2 · 视频调试的轮询请求遗漏模型参数。**

    位置：[PlaygroundView.jsx:805](/home/ifnodoraemon/myagent/airoute/web/src/components/PlaygroundView.jsx:805)、[video_handler.go:52](/home/ifnodoraemon/myagent/airoute/internal/api/video_handler.go:52)。

    生成任务使用用户选择的模型，查询却只发送 task ID。后端缺少 `model` 时按 `cogvideox` 路由。选择其他模型会查询错误模型或被模型白名单拒绝；前端又未检查轮询 HTTP 状态，可能把错误响应当作处理中持续轮询。

    复现：选 `sora-audit` 创建任务，查询 URL 中 `model` 为 null。探针：`includes the selected video model in the task polling request`。

    修复方向：创建时冻结任务参数，查询传入对应模型；更完整的解决方案是配合 F06，由服务端任务记录确定模型和渠道。轮询应正确处理 401/403/404。

17. **F17 · P2 · 视频任务开始轮询后，“停止生成”不再生效。**

    位置：[PlaygroundView.jsx:797](/home/ifnodoraemon/myagent/airoute/web/src/components/PlaygroundView.jsx:797)。

    停止按钮中止的是创建请求的 AbortController。后续三秒计时器和查询请求未绑定 signal，也不检查停止状态，仍然递归轮询。切换页面后的清理也无法取消这些查询。

    复现：创建任务成功后立即点击停止，推进三秒，仍发出第二次网络请求。探针：`stops video polling after the user presses Stop`。

    修复方向：创建、等待和查询共用取消机制；停止后清理计时器并立即结束本地加载状态。如果不能取消上游任务，按钮应明确表示停止查询。

18. **F18 · P2 · OAuth 一次性交换码只存于单进程，双节点会出现登录失败提示。**

    位置：[oauth_state.go:131](/home/ifnodoraemon/myagent/airoute/internal/controlplane/oauth_state.go:131)、[deploy/nginx-compose.conf:23](/home/ifnodoraemon/myagent/airoute/deploy/nginx-compose.conf:23)。证据级别：静态确认，未运行真实双节点 OAuth 登录。

    交换码保存在进程内 `sync.Map`，部署使用 `least_conn`。回调在 A 生成的交换码若在 B 兑换，会得到 401。回调已设置 cookie，浏览器恢复会话可能仍成功，因此表现可能是“已经登录却提示 OAuth 登录失败”，而非每次完全无法登录。

    另一个同域问题：未显式设置共享 `GATEWAY_ADMIN_SECRET`、而依赖数据库生成共享密钥时，`oauthStateKey` 在包初始化阶段已经由随机密钥派生，后续 `EnsureAdminSecret` 不会重算它，跨节点 state 校验仍可能失败。当前 Compose/Helm 显式传入共享密钥，后一个条件主要影响其他部署方式。

    修复方向：交换码使用具备原子消费能力的共享存储，或在 cookie 已建立后统一通过受保护的会话恢复完成登录；state 密钥应从最终生效的共享密钥派生。

推荐修复顺序：

| 批次 | 问题 | 验收重点 |
|---|---|---|
| 1 | F01、F02 | 独立 OAuth 身份永不串号；改密后旧会话立即失效 |
| 2 | F03、F04、F15 | 定价币种与钱包一致；读取失败不产生写入；局部更新保留字段 |
| 3 | F05、F06、F09、F16、F17 | 同一渠道聊天/多模态都可用；视频创建、查询和停止形成完整流程 |
| 4 | F07、F08、F10—F14 | 保存数据不被改写；迟到请求不覆盖编辑；导出和错误提示准确 |
| 5 | F18 | 双节点交叉回调/兑换、进程重启和会话恢复专项验证 |

范围与未覆盖项：

| 功能链路 | 本轮检查方式 | 边界 |
|---|---|---|
| 登录、注册、账号与会话 | 代码、现有回归、OAuth/改密定向复现 | OAuth 服务由模拟 Transport 代替 |
| 渠道、模型、路由、定价 | 前后端契约、真实动作 payload、内存 DB、错误注入 | 没有修改线上配置 |
| 密钥、权限、钱包、充值 | 入口权限与所有权代码、现有回归、真实内存账务路径 | 未执行真实 Stripe 付款、SMTP 发信 |
| 调试台聊天和多模态 | jsdom 交互、模拟 SSE/任务、实际 Go 处理器 | 未调用收费模型服务 |
| 仪表盘、日志、导出、公告 | 组件状态、响应乱序、分页失败、表单交互 | 未执行真实浏览器像素、手机布局和读屏器验收 |
| MCP、Skills、拓扑及运维接口 | 默认回归套件、相关入口和权限代码 | 未逐一联通第三方 MCP/技能服务 |
| 集群与部署 | 配置和状态存储审阅 | 本轮未运行 `-tags=integration` 的真实 PostgreSQL/Redis 测试，未进行负载测试 |

以上结论是对本轮已检查路径的可验证审查，不表示所有环境与操作组合都已覆盖。优先将本轮探针随对应修复转入正式回归套件，再补真实浏览器及双节点验收。
