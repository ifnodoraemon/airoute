# 功能流程与细节问题修复记录（2026-10-27）

对应 [2026-10-26 审查报告](functional-details-2026-10-26.md) 的 18 项问题。本次直接替换有缺陷的流程，不保留旧交换码、旧映射字符串或旧凭证的兼容分支。修复已写入当前工作区，未部署。

## 逐项结果

| 编号 | 修复后的行为 | 主要回归证据 |
|---|---|---|
| F01 | OAuth 按 `(provider, subject)` 查询独立身份表；创建账号、身份绑定和初始密钥在同一事务中完成。相同显示名称不会合并账号；已有邮箱不会自动关联。 | `TestAuditReviewGoogleDisplayNameDoesNotIdentifyAccount`、`TestOAuthStableSubjectAndCrossReplicaCookie` |
| F02 | 会话签名包含当前账号凭证的 HMAC 标识；每次认证从数据库核对。修改或重置密码立即撤销旧会话，改密响应清除浏览器 cookie。 | `TestAuditReviewPasswordChangeRevokesOldSession`、`TestPasswordResetRevokesSessionsAcrossHandlers` |
| F03 | 钱包只接受 CNY 定价，普通保存和事务保存使用相同验证。已有外币价格在模型调用前被拒绝，账务结算也拒绝外币快照扣款。 | `TestAuditReviewForeignCurrencyDoesNotDebitCNYAtFaceValue`、`TestLegacyForeignPriceIsRejectedBeforeUpstream` |
| F04 | 旧定价读取失败立即终止；仅明确的记录不存在可进入新建路径。数据库读取错误返回服务端错误，不再继续保存零值。 | `TestAuditReviewPriceReadFailureDoesNotOverwriteRates` |
| F05 | 多模态请求按 API 根路径拼接；覆盖根地址、`/v1`、尾斜杠、自定义前缀和查询参数。 | `TestAuditReviewMultimodalPreservesAPIBasePath`、`TestAPIURLPreservesConfiguredPrefix` |
| F06 | 网关生成独立视频任务 ID，将上游 ID、模型、创建渠道、地址和所属账号持久化。查询不参与负载均衡或跨渠道重试；校验任务归属和当前模型权限。 | `TestAuditReviewVideoPollReturnsToCreatingProvider`、`TestVideoTaskOwnershipAndPinnedFailure` |
| F07 | 映射表使用结构化行，提交时生成对象；删除冒号/逗号解析和往返序列化。未填完整或来源重复的映射不能保存。 | `ModelMappingEditor.test.jsx`、`useChannelActions.test.js`、`functional-details.test.jsx` |
| F08 | 渠道探测绑定 AbortController；切换编辑、编辑字段、关闭弹窗、重置会话及卸载均取消探测。迟到结果不会写入新表单。 | `ignores a channel probe response after a different channel is opened` |
| F09 | 渠道体检把实际协议配置传给 provider，纯 `openai_text` 渠道使用补全协议。 | `TestAuditReviewChannelTestRespectsTextProtocol` |
| F10 | 日志导出任何分页失败或会话变化均返回失败/取消；不会生成不完整 CSV 后提示完整导出成功。 | 分页 503 探针及 `useLogsData.test.js` 的会话切换测试 |
| F11 | SSE 解析错误和业务错误分开处理；所有合法错误事件都会提示，保留部分内容但不写入完整 assistant 历史，并释放读取器。 | `surfaces a stream error even when its message does not contain stream error` |
| F12 | 仪表盘请求统一由所选密钥驱动，首次挂载、手动刷新和周期刷新均加载对应概览。所有卡片和派生统计使用相同范围；旧请求不能覆盖新选择。 | 首次加载、响应乱序、统计卡片和周期刷新测试 |
| F13 | 公告配置加载期间和加载失败后禁用编辑及保存；失败可重试，关闭时取消请求。 | 公告加载前禁用及失败重试测试 |
| F14 | 模型路由弹窗的 Escape、背景点击和关闭/取消按钮统一经过未保存确认；保存成功直接关闭。 | `guards unsaved model routing changes when Escape is pressed` |
| F15 | 局部改价先继承已有币种，再校验；外币记录不会因遗漏币种而自动变为 CNY，必须显式修正。其他未提交价格保持不变。 | `TestAuditReviewPartialPriceUpdatePreservesCurrency` |
| F16 | 视频轮询携带创建时选择的模型；后端最终以任务记录的模型为准，拒绝冲突参数。 | 视频模型参数测试及任务模型不匹配测试 |
| F17 | 视频创建、轮询等待和查询共用取消信号；停止或卸载会清理计时器，HTTP 错误及终止状态不再重试。 | 停止、卸载、轮询 404 测试 |
| F18 | 删除进程内 OAuth 交换码及交换接口，浏览器回调设置 HttpOnly cookie 后通过 `/auth/me` 恢复会话。state 从最终共享密钥派生；数据库以原子插入确定各节点的唯一密钥。 | 跨 handler 会话恢复、state 密钥切换、并发共享密钥测试 |

## 直接替换的接口与行为

- 旧登录凭证不再被接受，升级后需要重新登录。
- 删除 `/api/v1/auth/oauth-exchange`，OAuth 重定向使用无凭证的 `oauth_complete=1` 标记；账号身份不再按旧用户名或邮箱推断绑定。
- 定价只支持 CNY。已有 USD 等价格不会自动换算或改标币种；需要明确重新定价。
- 视频生成必须提供模型和 API key。客户端必须使用生成响应中的网关任务 ID；原始上游任务 ID 不再作为查询入口。任务归属账号可通过其有效密钥查询，密钥的模型权限仍会校验。
- 数据库迁移 25 创建 `oauth_identities`，迁移 26 创建 `video_tasks`，均提供 SQLite/PostgreSQL 迁移入口。
- 渠道表单删除 `mapping_str`，内部使用 `mapping_rules`，对外仍提交结构化的 `model_mapping`。

## 验证

| 检查 | 结果 |
|---|---|
| `go test -race -count=1 ./...` | 全部通过 |
| `npm run lint` | 通过，无告警 |
| `npm test` | 38 个文件，507 项通过 |
| `npm run build` | 生产构建通过 |
| `git diff --check` | 通过 |

Go 测试包括新加入的 OAuth 跨节点 cookie 恢复、身份稳定性、改密/重置撤销、原子共享密钥初始化、视频任务归属及禁止跨渠道重试。前端用例覆盖实际交互、延迟返回、取消和失败场景。

新增回归已进入正常 Go/Vitest 测试集。原 [审查探针归档](2026-10-26-repro/README.md) 保留为修复前证据，不应再用原始探针结果代表当前状态。

本轮验证使用 SQLite、模拟上游服务及 jsdom，并以共享仓库的多个 handler 验证节点切换。没有部署线上服务、连接真实 OAuth/付费模型、启动真实双节点集群，或执行真实 PostgreSQL/Redis 与浏览器视觉验收。
