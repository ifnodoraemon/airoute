# P1 / P2 / P3 修复交付记录（2026-10-27）

- **基线**：`1.1.0-dev` / `4ab6cef` + 工作区未提交增量
- **范围**：本轮 21 项（P1 × 5、P2 × 15、P3 × 1）
- **不含**：并发会话正在进行的 `org_governance` 组织治理功能（见 §5）
- **状态**：21/21 已修复；全量 Go、前端、race 验证通过

## 1. 完成情况

| # | 级别 | 修复摘要 | 回归测试 |
|---|---|---|---|
| **1** | P1 | 控制台不再把密钥列表的显示掩码当作凭证：Playground / 首次接入向导 / AI Copilot 改为“粘贴本次会话明文密钥”（内存态），列表仅按 `id` 提供租户/模型授权上下文；所有代码示例统一 `<YOUR_API_KEY>`；无可用明文时拒绝发起调用并给出提示 | `web/src/lib/apiKey.test.js`、`PlaygroundView.test.jsx`、`OnboardingModal.test.jsx`、`AiCopilot.test.jsx`、`copilotHarness.test.js` |
| **2** | P1 | 密钥更新与创建共用同一套自助配额钳制：非管理员 `rpm/tpm <= 0` 回落到安全默认值，超过自助上限则封顶；管理员仍保留 `0 = 不限` | `internal/controlplane/key_update_clamp_test.go` |
| **3** | P1 | Responses、Gemini、旧版 Completions 三条流式入口读取 `Usage.GetCachedTokens()` 并传入计费层；900 缓存 token 现在按缓存价计费（0.000460，而非 0.002080） | `internal/api/cached_stream_billing_test.go`、`responses_context_test.go` |
| **4** | P1 | 多模态在上游响应体读取成功后才计费/回写：先做视频持久化转换，再在 64 MiB 上限内缓冲正文；读取失败记 502 `upstream_read_error`，不再记成功、不再扣费 | `internal/api/multimodal_body_read_test.go` |
| **5** | P1 | Responses `function_call_output` 转为 `role:"tool"` + `tool_call_id` + output（字符串或 JSON）；`function_call` 转为 assistant `tool_calls`，多轮工具调用不再中断 | `internal/model/response_input_test.go` |
| **6** | P2 | 仪表盘下钻改用数值 `key_id`：前端选项/查询全部 `key_id=`，后端 overview/timeseries 解析 `key_id` 并做归属校验、用存储摘要查询；普通用户不再 403，管理员能查到真实统计 | `web/src/components/DashboardView.test.jsx`、`internal/controlplane/admin_overview_keyid_test.go` |
| **7** | P2 | `previous_response_id` 真正恢复上下文：新增 `response_contexts` 表（迁移 31，7 天 TTL、按 owner 隔离、4 MiB/会话上限），成功响应后持久化，后续请求回放历史；找不到/过期/非本人返回 400 `previous_response_not_found`，不再静默忽略 | `internal/api/responses_context_test.go`、`internal/storage/{migrations_response_contexts,repo_response_contexts}_test.go` |
| **8** | P2 | Responses `input_text` / `input_image` 内容块转换为 Chat `text` / `image_url`（保留 `detail`）；纯文本块折叠为字符串，未知 item 跳过而不是生成空 user 消息 | `internal/model/response_input_test.go` |
| **9** | P2 | multipart STT 请求也做渠道模型别名改写：解析 boundary、替换 `model` 字段、重建合法 multipart 并更新 Content-Type，文件与其他字段保持不变 | `internal/router/multipart_model_rewrite_test.go` |
| **10** | P2 | 视频任务查询（仅精确 `GET /v1/videos/tasks/:id` 与 `/videos/tasks/:id`）豁免钱包余额、单密钥预算和并发预留；认证、密钥停用、账号锁定仍生效，已付费视频在余额归零后仍可查询 | `internal/middleware/free_read_video_task_test.go` |
| **11** | P2 | 外部 MCP 的 `/mcp` 地址不再被误判为本机：本地判定改为“ID = airoute-gateway / 相对路径 / 与自身公网 MCP URL 完全一致”；外部地址原样展示并真实拨测，不可达返回 offline | `internal/controlplane/admin_mcp_local_detection_test.go` |
| **12** | P2 | 外部 MCP 调用补齐传输流程：Streamable HTTP（initialize → `Mcp-Session-Id` → initialized → tools/call，JSON/SSE 解析）、旧版 HTTP/SSE（endpoint 事件发现 + message 事件匹配）、stdio（握手 → 调用 → 回收）；保留 8 MiB / 30s 边界并传播 `isError` | `internal/api/mcp_client_http_test.go`、`mcp_client_stdio_test.go` |
| **13** | P2 | 渠道探测在全部探测均网络失败时返回错误（handler 400），前端显示失败且不再自动填写协议；任一 HTTP 响应（含 401/404）才算可达，提示不再宣称“已连通” | `internal/controlplane/prober_customurl_test.go`、`web/src/hooks/useChannelActions.test.js` |
| **14** | P2 | Gemini/Anthropic 探测使用配置的 `baseURL`（规范化 `/`、`/v1`、`/v1beta`），代理/私有转发地址下探测的就是实际渠道；无 Key 探测也会真实发请求，离线代理会失败 | 同上 |
| **15** | P2 | 创建密钥请求进行中禁用关闭（遮罩/X/Esc/取消均无效）；成功回调总是重新打开弹窗并展示一次性明文，慢网络下不会再“创建成功但用户没看到密钥” | `web/src/components/KeyModal.test.jsx`、`web/src/hooks/useKeyActions.test.js` |
| **16** | P2 | 复制失败不再提示成功：`useKeyActions`、KeysView 的 cURL/指纹复制均检查布尔返回值，失败提示错误且不置“已复制”状态 | `useKeyActions.test.js`、`KeysView.test.jsx` |
| **17** | P2 | 注册时先检查用户名/邮箱重复，再验证并消费邮箱验证码；因重名失败后修改用户名重试，原验证码仍有效 | `internal/controlplane/register_verification_order_test.go` |
| **18** | P2 | 管理员重置本人密码时弹窗新增“当前登录密码”，提交 `old_password` 并通过前端校验；重置他人不变 | `web/src/components/UserManagementView.test.jsx` |
| **19** | P2 | 模型模态推断顺序修正：`whisper/sensevoice/transcription/asr` 先于 `tts/speech/voice` 判定，SenseVoice 不再被当成语音合成 | `web/src/lib/modelModality.test.js` |
| **20** | P2 | Skill 工件改为内容寻址：存储键与 ETag 均包含内容哈希（`id-<16hex>.zip`），内容变更即使不升版本也会重新打包；`Cache-Control: max-age=0, must-revalidate`，正确处理弱/列表 `If-None-Match`，保存/删除/切换时清理旧工件 | `internal/controlplane/admin_skills_etag_test.go`、`tests/artifact_storage_test.go` |
| **21** | P3 | 定价编辑弹窗接入 `useFormDirty + useUnsavedGuard + DiscardChangesDialog`：遮罩/X/Esc/取消都会提示未保存；保存成功直接关闭且不误报 | `web/src/components/PricingManager.test.jsx` |

## 2. 行为变化（运维/客户端需知）

1. **控制台不再回显可用密钥**：密钥列表仍是掩码（数据库只存摘要）。Playground、接入向导、AI Copilot 需要用户粘贴创建时保存的明文；`key_id` 仍用于授权范围与仪表盘筛选。
2. **管理台 API**：`GET /api/v1/admin/stats/overview` 与 `/stats/timeseries` 新增 `key_id`（数值主键）筛选；旧的 `key=`（摘要）仍保留给脚本兼容，但控制台不再发送掩码。
3. **限额**：非管理员通过 `PUT /api/v1/admin/keys/:id` 更新时，`rpm/tpm` 与创建一致地被钳制（默认 60 / 100000，上限 6000 / 10000000）；管理员不受影响。
4. **Responses 上下文**：`previous_response_id` 现在有意义，会话保留 7 天；无法解析时返回 400（不再静默当新会话）。单次会话超过 4 MiB 不持久化（响应仍成功，但不可续接）。无 API Key 的开放模式不支持该能力。
5. **视频查询**：余额/单密钥预算耗尽后，精确的视频任务状态 GET 仍可查询；禁用密钥、锁定/删除账号仍被拒绝。
6. **多模态缓冲**：图片/音频响应在计费前整体缓冲，首字节延迟与内存占用上升（单请求 ≤ 64 MiB）；超过上限返回 502 `upstream_read_error`。
7. **MCP**：调用外部 HTTP/SSE/stdio MCP 服务现在会先执行 initialize 握手；依赖会话/SSE 端点的服务可用。探测仍以 GET 发起，拒绝 GET 的纯 Streamable HTTP 服务可能显示 offline（工具调用不受影响）。
8. **Skill 下载**：缓存文件路径变为内容寻址；浏览器每次会重新校验（304 仅在同内容时出现），内容更新后下载立即生效。
9. **注册**：重名/邮箱冲突在验证码消费前返回；调用方持有有效验证码时可能据此判断用户名/邮箱是否已存在（为保住一次性验证码的取舍）。

## 3. 验证记录

| 检查 | 命令 | 结果 |
|---|---|---|
| 格式 | `gofmt -l internal cmd tests` | 无输出 |
| 静态检查 | `go vet ./...` | 通过 |
| 全量单测 | `go test ./... -count=1 -timeout 600s` | 全部包 `ok`（api 10.3s / controlplane 6.5s / storage 4.1s / tests 3.7s） |
| 竞态检测 | `go test -race ./internal/api ./internal/controlplane ./internal/storage ./internal/middleware ./internal/model ./internal/router -count=1` | 全部 `ok`（controlplane 96s / storage 44.5s） |
| 前端测试 | `cd web && npx vitest run --configLoader runner` | 49 文件 / 630 用例通过 |
| 前端 lint | `cd web && npm run lint` | 0 warning |
| 前端构建 | `cd web && npm run build` | 成功 |

## 4. 主要变更文件

**后端修复**：`internal/controlplane/{key_update,key_creation,user_key_rules,prober,prober_adapters,identity_handler,admin_skills,admin_overview,admin_mcp}.go`、`internal/api/{responses_handler,responses_stream,completions_stream_handler,gemini_inbound_stream,multimodal_handler,mcp_tools}.go`、`internal/model/response.go`、`internal/router/dispatcher_modality.go`、`internal/middleware/{auth,authz}.go`、`internal/storage/{migrations_sqlite,migrations_postgres}.go` + 新增 `migrations_response_contexts.go`、`repo_response_contexts.go`、`internal/api/mcp_client*.go`。

**前端修复**：`web/src/lib/apiKey.js`（新）、`web/src/components/{PlaygroundView,OnboardingModal,AiCopilot,DashboardView,KeysView,KeyModal,UserManagementView,PricingManager}.jsx`、`web/src/hooks/{useChannelActions,useKeyActions}.js`、`web/src/lib/modelModality.js`、`web/src/App.jsx`、`web/src/components/copilotHarness.js`，以及对应的新增/更新测试；三份共享审计测试同步到新契约。

## 5. 与并发 `org_governance` 会话的边界

工作区同时存在另一个会话未提交的 `org_governance` 组织治理改动（迁移 30、`OrgID` 字段、`repo_org*` 等，非本轮 21 项范围）。本轮**未改动其功能逻辑**，仅做了三处让全量测试成立的兼容处理：

1. `internal/storage/migrations_orgs.go`：迁移 30 对缺失表做存在性判断（legacy/部分 schema 不再因 `ALTER TABLE users` 失败），回填时跳过缺失表及其子查询；组织表与默认组织创建逻辑不变。
2. `internal/controlplane/contract_test.go`：把 `org_id` 纳入“服务端归属字段”（请求不接受）并在 `APIKeyRecord` 固定契约中登记。
3. `tests/artifact_storage_test.go`：Skill 缓存断言改为内容寻址键（与本轮 #20 一致）。

迁移编号保持唯一且升序：`30 org_governance` → `31 response_contexts`。

## 6. 已知限制 / 后续

- 控制台明文密钥只存在于用户粘贴的当前页面内存；刷新或关闭后需重新粘贴（明文设计上只显示一次）。
- `previous_response_id` 不支持跨 API Key/账号共享；开放模式（无 Key）暂不支持；4 MiB 上限外的会话不可续接。
- 多模态 64 MiB 缓冲上限与延迟属于本次修复的明确取舍，如后续需要流式大文件，应设计“边转发边记账”的分段计费语义。
- 外部 MCP 探测只做 GET 可达性判断；纯 Streamable HTTP（拒绝 GET）服务会显示 offline，但工具调用走 initialize 流程正常。
- 注册接口在重名/邮箱冲突时不消耗验证码的代价是存在性探测面扩大，已记录；如需彻底消除，可改为“验证码通过后统一返回模糊错误”。
