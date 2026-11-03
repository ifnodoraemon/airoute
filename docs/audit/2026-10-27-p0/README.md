# P0 补口交付记录（2026-10-27）

- **基线**：`1.1.0-dev` / `a6ef42a`，工作区增量（未提交）
- **范围**：P0-01 / P0-02 / P0-03 / P0-04 / P0-05 / P0-07
- **不含**：P0-06（1.1.0 正式发版）与 v1.1 运营治理（下一阶段）

## 1. 完成情况

| ID | 状态 | 实现摘要 | 测试 |
|---|---|---|---|
| **P0-01** 协议字段无损透传 | 完成 | `internal/model/json_extras.go` 通用「未知字段捕获 + 按序回写」；覆盖请求顶层、消息级、响应顶层/choice/message、流式 chunk/choice/delta、usage/prompt details。已知字段优先，网关改写（model/stream/max_tokens）不会被客户端字段覆盖；无额外字段时输出与原来字节级一致；跨协议仍走显式映射 | `json_extras_test.go`、`provider/openai_passthrough_test.go`（httptest 验证请求真的转发、响应/chunk 真的回传） |
| **P0-02** Responses 工具链 | 完成 | 扁平 function tools → chat tools；`tool_choice` 字符串/对象；`parallel_tool_calls` 建模；非流式 `tool_calls` → `function_call` 输出项（纯工具轮次不再补空 message）；若内置工具全部被跳过、没有 function tools 存活，则丢弃 `tool_choice`（否则上游必 400）；流式补 `function_call` 输出项、`function_call_arguments.delta/done`、`output_item.done`，并纳入 `response.completed` | `response_tools_test.go`、`responses_tools_stream_test.go`（item/output_index/arguments/完成态/序列号） |
| **P0-03** 请求级总预算 | 完成（非流式） | `GATEWAY_REQUEST_BUDGET_SEC`（默认 300s，0 关闭）由渠道尝试与跨模型兜底共享；耗尽立即停止换渠道并返回 504 `request_budget_exceeded`；调用方更短 deadline 优先且不误报；不污染熔断。流式保持渠道超时 + 首字超时语义 | `request_budget_test.go`、`request_budget_status_test.go`（含 mock 阻塞 provider） |
| **P0-04** 审计残留收口 | 完成 | ① `off_peak_discount<=0` 现在表示「无折扣」（存储与引擎双层）；② 外币价被拒请求在写响应头前归零，`X-Airoute-Cost` 与审计一致；③ 迁移 v27 改为「延迟迁移」：失败不记版本、不阻断启动、下次启动重试；④ `-count=3` 的 Prometheus 计数串扰改为基线差值断言；⑤ MCP SSE 跨副本边界写入 HA 文档。`prompt=10` 硬编码经核实已在审计修复中消失 | `repo_pricing_offpeak_test.go`、`engine_test.go`、`billing_currency_test.go`、`migrations_deferred_test.go`、`billing_edge_fix_test.go` |
| **P0-05** 部署与可观测小缺陷 | 完成 | `/metrics` 默认仅 loopback/私网（`GATEWAY_METRICS_TOKEN`/`GATEWAY_METRICS_ALLOW_PUBLIC` 可调）；修复 k8s 清单重复 `admin-password`；新增只读 `make check-deploy-drift`（nofile / PG `max_connections` 漂移）；控制台「开发文档」改为真正的 `docs` 标签页 | `metrics_access_test.go`、真实环境漂移脚本执行、YAML 解析 |
| **P0-07** 国产厂商探测预设 | 完成 | prober 识别 `bigmodel.cn`/`volces.com`/`moonshot.cn`/`dashscope`/`baidubce.com`/`tencentcloudapi.com` 并生成可读名称；新增阿里百炼、百度千帆、腾讯混元前端预设（OpenAI 兼容端点） | `prober_heuristics_test.go`、`providerPresets.test.js`、`ChannelModal` 现有渲染测试 |

## 2. 行为变化（运维需知）

1. **协议透传**：同一协议（OpenAI→OpenAI 兼容上游）会转发网关未建模的字段（`response_format`、`seed`、`logprobs`、`service_tier` 等）。跨协议（OpenAI→Anthropic/Gemini）仍只做显式字段映射，不会把无法表达的字段塞给上游。`parallel_tool_calls` 已升级为建模字段。
2. **非流式总预算**：默认 300s。超长非流式生成如确实需要更久，调大 `GATEWAY_REQUEST_BUDGET_SEC` 或设为 `0`。流式不受此预算影响。
3. **`/metrics` 默认私网**：Prometheus 侧车/Compose 私网抓取无需配置；公网抓取返回 403。需要公网时设置 `GATEWAY_METRICS_ALLOW_PUBLIC=true`，或配置 `GATEWAY_METRICS_TOKEN`（令牌优先）。
4. **off-peak 折扣 0**：现在等于「无折扣（factor 1.0）」，不再是静默 50% 折扣。已有显式 `0.5` 的规则不受影响；依赖旧默认值的规则应显式填 0.5。
5. **迁移 v27**：重复邮箱导致唯一索引失败时，版本不记录、启动继续、下次启动重试；需运维清理重复邮箱后重启生效。
6. **`X-Cache-Status` 保留**：该头由 nginx 为 Skills 边缘缓存设置，e2e 有验证，不是未实现功能。
7. **MCP SSE** 仍为单副本内存会话；多副本需粘性路由或共享存储（已写入 HA 文档）。

## 3. 验证记录

| 检查 | 命令 | 结果 |
|---|---|---|
| 格式 | `gofmt -l internal cmd tests` | 无输出 |
| 静态检查 | `go vet ./...` | 通过 |
| 全量单测 | `go test ./...` | 全部包 `ok` |
| 全量竞态 | `go test -race $(go list ./... \| grep -v '/web/node_modules')` | 全部包 `ok`（controlplane 79s / storage 28s / tests 42s；新增 tool_choice 保护后复跑仍全绿） |
| 前端 lint | `cd web && npm run lint` | 0 warning（`--max-warnings 0`） |
| 前端测试 | `cd web && npm test -- --run` | 46 文件 / 569 用例通过 |
| 前端构建 | `cd web && npm run build` | 成功 |
| 部署清单 | PyYAML 解析 `deploy/kubernetes.yaml` / `docker-compose.yml` / `docker-stack.yml` | 通过 |
| 漂移脚本 | `sh -n scripts/check-deploy-drift.sh` + 实跑 | 语法通过；真实环境报告无漂移（nofile 65536、PG max_connections 200） |
| 偶发用例 | `go test ./internal/storage -run TestForeignCurrencyRecordDoesNotPoisonBatch -count=3` | 通过 |

## 4. 变更文件

新增：

- `internal/model/json_extras.go` / `json_extras_test.go`
- `internal/model/response_tools.go` / `response_tools_test.go`
- `internal/provider/openai_passthrough_test.go`
- `internal/router/request_budget.go` / `request_budget_test.go`
- `internal/api/metrics_access.go` / `metrics_access_test.go`
- `internal/api/request_budget_status_test.go`
- `internal/api/responses_stream_events.go` / `responses_tools_stream_test.go`
- `internal/api/billing_currency_test.go`
- `internal/controlplane/prober_heuristics_test.go`
- `internal/storage/migrations_deferred_test.go`
- `scripts/check-deploy-drift.sh`
- `docs/ROADMAP.md`
- `docs/audit/2026-10-27-p0/README.md`（本文）

修改（核心）：

- 模型/协议：`internal/model/chat.go`、`response.go`、`usage.go`、`internal/provider/reasoning_adapter.go`（无需改）
- 路由：`internal/router/dispatcher.go`
- API：`internal/api/router.go`、`common.go`、`responses_stream.go`
- 计费/存储：`internal/billing/engine.go`、`internal/storage/repo_pricing.go`、`migrations_runner.go`、`migrations_users_email_unique.go`
- 探测：`internal/controlplane/prober_heuristics.go`
- 部署/文档：`deploy/kubernetes.yaml`、`.env.example`、`docs/CONFIGURATION.md`、`docs/HA_ARCHITECTURE.md`、`docs/RELEASE_GUIDE.md`、`Makefile`
- 前端：`web/src/App.jsx`、`web/src/components/ChannelModal.jsx`、`web/src/lib/providerPresets.js`、`providerPresets.test.js`
- 测试调整：`tests/api_test.go`、`internal/storage/billing_edge_fix_test.go`、`internal/billing/engine_test.go`、`internal/storage/repo_pricing_offpeak_test.go`

## 5. 已知限制与后续

- **P0-06 未执行**：1.1.0 正式发版（CHANGELOG、镜像 digest、发布说明）留待代码评审后。
- **预算只覆盖非流式**：流式由渠道超时与首字超时约束；若未来需要端到端流式总预算，应设计「首字后可续期」的语义，避免误杀长生成。
- **国产预设的模型列表是种子**：厂商模型迭代后以「探测」结果为准；探测不到 `/models` 时保留手工列表回退。
- **迁移 v27 仍需人工清理重复邮箱**：自动重试不等同于自动合并账号。
- **MCP SSE 会话仍单副本**：未在本轮引入粘性路由/共享会话存储。
- 工作区改动**未提交**，便于先评审；可按 P0-01 / P0-02 / P0-03 / P0-04 / P0-05+07 拆分提交。
