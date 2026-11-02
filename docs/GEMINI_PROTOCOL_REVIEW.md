# Gemini 接入核对（2026-11-02）

适用范围是 Airoute 的 Gemini Developer API、Vertex 上游、Gemma 4 调试显示，以及 Chat / Responses / Messages 与 Gemini 的转换。原生直通保留供应商功能；兼容转换只处理能准确表达的语义。

## Google 官方建议

[面向平台与网关的接入指南](https://ai.google.dev/gemini-api/docs/partner-integration)区分三种接法：

| 接法 | 官方适用场景 | 对 Airoute 的意义 |
| --- | --- | --- |
| Google GenAI SDK | 企业网关、框架，需要现成类型、认证及文件等功能 | 业务应用可以直接用官方 SDK 连接网关；网关采用 SDK 时需要固定版本 |
| 直接 REST / gRPC | 聚合平台、边缘平台，需要控制依赖和完整原生能力 | 保留当前 Go HTTP 转发方式，原生请求与响应不经 Chat 结构体重建 |
| Google 的 OpenAI 兼容接口 | 已有 OpenAI SDK，侧重文本工作流 | 可作为独立 OpenAI 渠道接入；不能据此宣称原生 Gemini 的所有功能均受支持 |

官方同时要求平台请求携带 `x-goog-api-client`。本轮给 Gemini 请求补充 `airoute/<version>`；Developer API 使用 `x-goog-api-key`，Vertex 使用可刷新的 OAuth2 bearer token。

[思考签名文档](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures)要求下一轮按原样返回签名。签名可能出现在工具调用，也可能只出现在空文本 part；因此不能只收集非空文本。Chat 兼容格式使用 `tool_calls[].extra_content.google.thought_signature`。

[Gemma API 文档](https://ai.google.dev/gemma/docs/core/gemma_on_gemini_api)明确 Gemma 4 通过 `high` / `minimal` 开关思考。[提示格式](https://ai.google.dev/gemma/docs/core/prompt-formatting-gemma4)定义了 `<|channel>thought … <channel|>`。这些是显示解析的依据，不应据此改写原生 API 的正文。

## 社区实现

| 项目 | 核对结果 | 可借鉴内容 |
| --- | --- | --- |
| [LiteLLM 原生端点](https://docs.litellm.ai/docs/pass_through/google_ai_studio)及 [Gemini 转换文档](https://docs.litellm.ai/docs/providers/gemini) | 提供独立原生透传路径，也维护 Chat 转换；签名单独随工具调用保存 | 原生转发旁路提取用量；转换路径明确支持参数；客户端丢扩展字段时需另有签名载体 |
| [New API 原生处理器](https://github.com/QuantumNous/new-api/blob/c046559bbd8b154e88e69905f60e71a5f4f1e5aa/relay/channel/gemini/relay-gemini-native.go) | 原生响应保留正文，额外解析用量和 promptFeedback；另设格式转换路径 | 计量与响应重建解耦；内容过滤与上游故障区分 |
| [Magpie Gemini 编解码](https://github.com/yetone/magpie/blob/1aca55909ff94f5c23091ce35f73cb4b7bc46658/internal/gateway/gemini.go)及[签名处理](https://github.com/yetone/magpie/blob/1aca55909ff94f5c23091ce35f73cb4b7bc46658/internal/gateway/gemini_signature.go) | 自行维护 Go 协议适配；保留调用 ID、签名，工具参数组装完整后发送；兼容 ProtoJSON 的两种字段名 | 专门测试并行工具、多轮签名、空内容、流中断；签名载体须考虑客户端截短 ID 的情况 |

源码核对固定在 New API `c046559b`、Magpie `1aca5590`；LiteLLM 同期参考提交为 `ff3983c1`。项目会持续变化，不能把任一项目的实现当成协议规范。

## 本轮代码修复

- 原生 Gemini 请求和响应保留未知字段、签名、工具和多模态内容，流式响应保留 SSE 数据。用量解析独立进行。
- Playground 区分 Gemma 控制标记中的思考和答案，处理跨片段标记；结构化 `thought: true` 不再混进答案。
- 转换路径补充停止序列、候选数、最大输出、JSON Schema、seed、惩罚参数及思考配置；思考映射使用实际上游模型名。
- 工具调用保留原生 ID 与官方 Chat 扩展签名；并行工具分配独立索引，在真实结束帧后给出 finish reason。
- 非流式和流式均保留全部候选；内容过滤映射为 `content_filter`，空的过滤结果可正常完成，不触发换渠道重试。
- Gemini SSE 支持多行 data、完整 JSON 后断连的末帧；错误帧保留限流等状态，避免一律变成 502。
- 原生 `countTokens` 支持 `generateContentRequest`，重写内部模型别名；没有原生渠道的本地估算标记为估算。
- 原生 `embedContent` 保留任务类型、维数、多模态和扩展字段；Chat embeddings 到 Gemini 的单条、批量调用均带输出维数。
- ProtoJSON 的 snake_case 和 camelCase 用解析副本兼容，函数参数和用户 Schema 的字段名保持原样。
- Gemini 专有能力在无法表达的跨协议路径返回错误，避免请求成功却丢失 grounding、缓存或媒体信息。

## 缺口补齐结果

| 项目 | 已实现与使用边界 |
| --- | --- |
| Files / cache / batch / Interactions / Live | 文件上传、断点续传、元数据与删除、下载，缓存 CRUD，批处理提交与状态/取消/删除，独立 Interactions REST/SSE，以及 Live WebSocket 桥接。列表只读调用者自己的资源快照，资源使用原始渠道凭据，跨租户访问在请求上游前拒绝 |
| Vertex AI | `vertex` 渠道支持项目/区域 URL、ADC、服务账号 JSON、OAuth token 缓存刷新与临时 bearer。生成、向量、计数及 Live 使用 Vertex 路径；Developer-only 资源接口仍受实际上游能力限制 |
| 丢弃 Chat 扩展字段的客户端 | 工具 ID 携带 34 字符短引用；完整 parts 与原始调用 ID 存在 PostgreSQL，TTL 24 小时，按调用者、凭据和实际上游模型校验。工具参数或内容被修改时拒绝恢复 |
| 文本/图片签名 | 原生请求保留全部 parts；兼容响应提供 `extra_content.google.state_ref`，恢复时带回原始文本、媒体、空签名 part 和未知字段。客户端至少要保留 state_ref 或工具 ID；丢弃所有引用后无法恢复，不能伪造签名绕过 |
| 模型能力发现 | 优先查询映射后的实际上游模型，按渠道身份缓存，并与网关已实现方法求交；失败时按向量模型名和协议声明保守推断，不再给所有模型共用生成方法列表 |
| Playground 多模态 | Gemini Native 独立面板支持原生 JSON、内联图片/音频/视频/PDF、流式/非流式、思考/工具与媒体展示、PCM 播放及下载；加入下一轮时保留完整原生 parts。私有 Google 媒体通过有归属校验的网关下载 |

批处理和后台 Interactions 使用持久化任务状态、可恢复的计量租约及确定性 billing ID；客户端不轮询也会由后台工作线程完成计量，两个副本不会重复结算。JSONL 结果文件包含的用量也参与计量；上游未返回用量时标记为估算。网关按管理员配置价格结算，不自动同步 Google 批量折扣、缓存存储费用或媒体模态单价。

新增的状态表迁移为 48；双副本必须共用数据库。无数据库时仅有有界内存兼容模式，不能提供跨副本或重启后的任务/签名恢复。更换 ADC 文件背后的 Google 账号前，应先结束使用旧账号的任务和资源；OAuth token 的正常刷新不影响账号归属。

这些路由覆盖上述明确列出的 API，不代表代理 Google 的所有未来接口。File Search 管理、Live 临时令牌签发、Vertex GCS 批处理任务等具有独立授权或存储语义的接口不在本轮覆盖范围。`gs://` URI 由上游处理，浏览器不能直接预览私有 GCS 对象。

## 依据与验证

继续以 Google 的 [Files API](https://ai.google.dev/api/files)、[缓存 API](https://ai.google.dev/api/caching)、[Batch API](https://ai.google.dev/api/batch-api)、[Interactions](https://ai.google.dev/gemini-api/docs/interactions)、[Live WebSocket](https://ai.google.dev/api/live) 和 [Vertex 认证](https://cloud.google.com/vertex-ai/generative-ai/docs/start/gcp-auth)为协议依据。Go 侧保留 HTTP/REST 原生转发，认证采用 Google 维护的 `golang.org/x/oauth2/google`，Live 采用官方 Go GenAI SDK 同样使用的 Gorilla WebSocket。

回归覆盖：空文本签名、完整签名历史及丢扩展字段的工具调用、服务账号/ADC token 刷新、别名与能力目录、原生资源租户隔离和凭据轮换、断点续传 token 隐藏、JSONL 批处理计量、Interactions 签名及思考用量、Live 双向帧与多轮用量、模型和恢复句柄权限，以及浏览器媒体和原生历史保留。共享状态另用真实 PostgreSQL 双连接并发竞争验证，Redis 会话 pin 的比较删除保留集成覆盖。

本次提交前通过前端 lint、743 项测试与生产构建，`go test -race -count=1 ./...`、`go vet ./...` 和单二进制构建；隔离 PostgreSQL 16 / Redis 7 集成测试通过。20 项变异测试均捕获预期失败，并按 SHA-256 确认逐字节还原。新增业务 Go 文件均满足 300 行文件 / 60 行函数约束，Nginx WebSocket 配置校验通过。

线上 Gemma 请求此前遇到 TLS EOF；协议模拟回归、构建与数据库测试和实网上游验证分别记录，不能互相代替。本次提交后按用户授权重建双副本并检查就绪状态。
