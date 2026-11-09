四项清单已落实：直接清理、重复逻辑收敛、大模块拆分、文档与回归验收。以下记录本轮实际实现，初审的建议和基线见 [README.md](README.md)。机器可读结果见 [implementation-evidence.json](implementation-evidence.json)。

统计基于开始实施时的工作区完整备份，包含当时已有的 90 条未提交状态；不以 Git HEAD 的差异冒充本轮改动。原有通知中心、思考能力探测等工作继续保留在当前工作区。

## 清单完成情况

| 编号 | 实现 | 保持的边界 |
| --- | --- | --- |
| D01 | 三个入口的 10 处手写错误结构改用 `RespondOpenAIError` | HTTP 状态、英文文案、type/code、return 顺序 |
| D02 | Channel、API Key、User、Price 各自共享投影和行解码 | 查询条件、读写池、上下文、事务锁；用户列表仍不读密码 |
| D03 | 列表页和命令面板共用 `openCreateChannel` | GPUStack 与原有五种协议默认值，探测状态清理 |
| D04 | 46 组 Prometheus 标量模板共用输出函数，快照与格式化分段 | uint64 整数精度、两位小数、HELP/TYPE、输出顺序和 histogram |
| D05 | 删除未消费的 `keepEndpoint`、五个组件的无效 `t`、恒定状态和重复定位回调 | `keepModels` 与依赖初始 props/URL 的 state 快照继续保留 |
| D06 | 删除 MCP 两个测试包装；readiness reset 移入同包测试 | Digest 鉴权实现和所有原有回归断言；跨包测试使用的 `ResetWarnGate` 保留 |
| R01 | Playground/Copilot 共用逐行 SSE 读取器 | UTF-8 分片、EOF 补帧、首字节时间、部分输出、cancel/release；Gemini 多行事件独立 |
| R02 | 20 处匹配的增删改请求共用 `apiMutation` | 严格 `code === 0`、完整 envelope、202 warning、一次性凭据和业务错误文案 |
| R03 | 删除内部协议注册表，改为有类型的转换函数 | 工具历史、签名、缓存 token 与原生透传；Anthropic 响应仍由真实 Messages 渲染路径负责 |
| R04 | Embedding/Rerank 共用一元尝试循环 | 404/fatal/预算/取消分类、半开准入、attempted/skipped、各自 token 口径 |
| R05 | 按页面展示、状态、数据加载、执行流程，以及后端领域阶段拆分 | 状态所有权、请求代际与取消、账务时序、ACK、spill、启动和停机顺序 |
| R06 | 工程文档指向现有 `ci.yml`，明确零 lint 告警 | 复用现有门禁实现 |

App 的侧栏、面包屑与命令面板共用导航元数据，深链 tab 集合复用角色导航配置。命令面板的排序、搜索文案和角色差异保持原样。

渠道、钱包、用户和技能页面随请求层收敛拆出展示与领域 hook。Playground 的各模态状态仍无条件持有；切换面板不会重建草稿和结果。Copilot 按响应读取、工具迭代与运行状态分段，工具签名和跨轮 reasoning token 累计有独立回归。

## 大模块规模

此表是原入口文件的规模，移出的代码仍计入下面的总量；不把文件变短当作代码删除。

| 文件 | 实施前行数 | 实施后行数 |
| --- | ---: | ---: |
| `web/src/App.jsx` | 1,706 | 14 |
| `web/src/components/DashboardView.jsx` | 1,875 | 141 |
| `web/src/components/PlaygroundView.jsx` | 1,859 | 163 |
| `web/src/components/PricingManager.jsx` | 1,831 | 201 |
| `web/src/components/McpIntegrationView.jsx` | 1,636 | 148 |
| `web/src/components/AiCopilot.jsx` | 1,383 | 159 |
| `internal/api/common.go` | 1,052 | 85 |
| `internal/model/response.go` | 747 | 160 |
| `internal/middleware/auth.go` | 687 | 130 |
| `internal/provider/thinking.go` | 530 | 138 |
| `internal/storage/async_logger.go` | 727 | 198 |
| `internal/storage/db.go` | 627 | 230 |
| `cmd/airoute/server.go` | 561 | 58 |
| `internal/storage/repo_timeseries.go` | 432 | 136 |
| `internal/storage/repo_reconciliation.go` | 417 | 37 |

本轮新建的业务源文件均不超过 300 行。函数粒度仍有部分较长的 React 组合与 JSX 展示函数；全仓库既有文件也仍有规模技术债。以上结果不代表已经满足全仓库的全部文件和函数规模约束。

## 验证

- Node.js 22.23.4：ESLint 零告警；92 个测试文件、804 项测试通过；Vite 生产构建通过。
- Go 1.26.1：全项目 `go test -race -count=1` 的 16 个有测试包通过；`go vet`、依赖校验与静态网关编译通过。范围排除 `web/node_modules`。
- 独立 Docker Compose 的 PostgreSQL 16 / Redis 7：`-race -tags=integration` 存储和分布式套件通过；新增的投影/解码契约在真实 PostgreSQL 上通过。
- Go e2e 的 7 组用例在独立测试网关容器内通过，覆盖认证、MCP、技能下载、控制面读取、密钥生命周期、trace header 和并发健康请求。
- 18 项变异验证均捕获预期错误，每次恢复后校验源码字节一致。覆盖指标整数精度、SSE EOF、业务 code、模态切换重置、工具签名、创建默认值、四类解码、周期回放、鉴权 fail-open、readiness、MCP transport fallback、工具 ID 和图片来源。
- 8 个页面使用同一 Chrome、固定视口/时钟和相同 API 夹具对比：Dashboard、Playground、Pricing、MCP、Channels、Users、Wallet、Skills。全部无页面异常，页面文本与 PNG 文件逐字节一致；SHA-256 记录在 evidence 中。
- 版本元数据、8 项发布脚本测试和 Helm lint 通过；`git diff --check` 通过。测试网关通过 SIGTERM 正常退出，退出码为 0。最终格式整理后的生产 bundle 与浏览器验证时逐字节一致。

Prometheus 完整输出 fixture 从实施前的导出函数生成，再由重构后的实现核对。它包含全部指标、超出 float64 精确整数范围的 counter、负 gauge、带转义字符的模型标签以及 histogram；只归一化原本由 map 决定顺序的模型序列。

此前间歇失败的 `TestUsageSpillReplayPicksUpAnotherProcessFile` 存在夹具竞态风险：零 claim grace 使回放有机会看到 peer 尚未写入/fsync 的空文件。夹具现在先等待启动标记落盘，再通过 rename 发布已写完的 peer 文件，不发送 replay kick；周期回放被禁用的变异会使该测试失败。修正后该用例独立 `-race -count=3` 与全量竞态验证均通过。

最初从宿主机经 Docker 端口映射运行 e2e，管理面按现有网络准入规则返回 403。确认原因后将测试二进制放进测试网关容器，使用回环地址完成全套验证，业务准入规则未改动。此次运行环境为单个网关和真实 PostgreSQL/Redis；双副本负载均衡切换与真实供应商推理不在这次 e2e 的覆盖范围内。

## 代码总量

本轮涉及的生产 Go/JS/JSX 源码由 26,581 行变为 28,447 行，**净增 1,866 行**；测试源码净增 402 行。文档、220 行指标 fixture 和构建产物不在该统计中。

重复逻辑和无效抽象已经删减；模块拆分增加了 imports、显式参数、hook 返回值和领域辅助函数，导致本轮总行数增长。收益主要是职责边界与回归覆盖，不能表述为“全仓库净删减”。生成的 `web/dist` 和测试二进制均未加入版本控制。

相同依赖与构建配置下，JS 资源的 gzip 总量由 396,910 bytes 变为 415,024 bytes，增加 18,114 bytes（约 17.7 KiB）。这也是本次职责拆分的实际体积成本。

本地测试日志、实施前备份和完整截图保存在 `/tmp/airoute-refactor-implementation-ospwhysb/` 及 `/tmp/airoute-refactor-*.log`。业务变更留在当前工作区供审阅，未创建提交。
