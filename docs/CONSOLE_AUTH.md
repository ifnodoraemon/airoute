# 控制台调用与 Key 选择

调试演练、副驾驶和接入引导的在线握手使用已登录会话。可用 Key 自动选中，用户也可以切换；无需查找、复制或粘贴 API Key 明文。没有可用 Key 时直接引导创建。

## 请求与权限

浏览器通过现有 HttpOnly 登录 Cookie 访问 `/api/v1/playground/v1/*` 或 `/api/v1/playground/v1beta/*`，在 `X-Airoute-Key-ID` 中携带所选 Key 的数据库记录 ID。写请求同时通过现有 CSRF 校验。这个 ID 只是选择项，不是鉴权凭证。

后端先验证登录会话、账号状态和凭证版本，再从数据库读取所选 Key 的最新状态。普通用户只能选择自己拥有的 Key；管理员沿用密钥管理页的管理权限。停用或删除、模型权限、计费分组、预算、钱包余额、RPM、TPM 和预授权都沿用数据面现有规则。费用与用量记录到实际选择的 Key，管理员代测也按所选 Key 的归属结算。

后端不需要还原 Key 明文。数据库仍只保留密钥摘要；会话与归属校验建立身份后，服务端直接构建所选 Key 的授权上下文，进入与外部 API 请求相同的路由、计费和审计流程。摘要不会成为可在公开接口重放的凭证。浏览器不接收、不保存、不发送 API Key 明文或摘要；登录会话按现有部署通过 HTTPS 的 Secure/HttpOnly Cookie 保护。

流式输出、双模型对比、图片、语音合成/转写、视频任务轮询、向量、重排、Gemini 原生请求与私有生成媒体下载使用同一选择方式。轮询、工具循环和媒体下载保留发起请求时的 Key ID，避免界面切换影响已经发起的任务。Gemini 模式也始终显示 Key 选择器。

控制台会话过期会进入登录恢复流程；上游提供方返回 401 只作为调用错误展示，不会将已登录用户退出。所选 Key 被撤销返回 403，用户可选择其他可用 Key。

## 外部 SDK 接入

公开 `/v1/*`、`/v1beta/*` 的鉴权方式不变：外部客户端使用创建时一次性显示的 API Key。控制台 Cookie、Key ID 或数据库摘要都不能替代该凭证。复制的 cURL 和 SDK 示例始终使用 `<YOUR_API_KEY>` 占位符，避免分享示例时携带控制台会话。

原有的手工粘贴、每个选择项保存明文、免密调用选项和客户端明文格式诊断代码已移除。

## 参考

- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)：会话标识、HttpOnly/Secure Cookie 和后端代管凭证。
- [LiteLLM Chat UI](https://docs.litellm.ai/docs/proxy/ui/chat_ui)：使用登录会话调用网关，并应用模型权限、预算和日志。Airoute 在此基础上保留按所选 Key 结算的现有产品语义。

## 验证

`internal/api/console_playground_test.go` 覆盖真实会话 Cookie、CSRF、越权选择、实时停用/删除、模型与额度限制、RPM/TPM、登录过期和撤销、流式及非流式请求的精确扣费、Gemini 入口、视频免费查询，以及公开 SDK 鉴权兼容性。

前端测试覆盖无需粘贴即可调用、切换 Key 同时更新模型和请求归属、异步加载、无 Key 的创建入口、接入引导、协议切换、媒体下载，以及上游 401 与登录过期的区分。

2026-11-03 验收结果：16 个 Go 测试包通过 race 检查，前端 76 个文件共 735 项测试通过，PostgreSQL 16 / Redis 7 集成测试通过，vet 与 staticcheck 无问题，版本发布脚本 8 项测试通过。真实浏览器使用隔离数据库和合成上游完成调试演练、切换 Key、副驾驶、Gemini 原生流式请求及移动端操作；日志与实际所选 Key 一致，合计费用 0.012018 元，钱包余额由 10 元变为 9.987982 元，未出现页面异常。
