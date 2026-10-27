# 剩余 P2/P3 修复记录（2026-10-27）

对应 [本轮全面审查](README.md) 中 P0/P1 之外的全部 P2/P3 发现，按两批完成、回归、滚动发布并线上验证。P0+P1 的修复记录见 [remediation-2026-10-27.md](remediation-2026-10-27.md)。

- 当前线上版本：`1.1.0-dev+g4524dc8`
- 发布历程：`g5955133`（P2 批 1）→ `g30b7f0b`（P2 批 2）→ `g427fc63`（A12，停机协调重启 + 迁移 v29）→ `g4524dc8`（配额强制收尾）

## 1. 提交清单

### P2 批 1（路由 / 计费账本 / 存储遥测 / 前端）

| 提交 | 内容 |
|---|---|
| `5e843ba` | R-05 半开 canary 泄漏、R-08 thinking delta、R-09 工具索引、R-10 Gemini 错误帧、R-11 Gemini 工具 ID/并行结果、R-12 统一 API 根 URL、R-14 JSON 整数精度、R-17 moderation 协议别名 |
| `b148081` | B5 无主扣款可观测、B8 legacy 账本可对账、B9 清理基线（auditLag 可归零）、B10 已计费流水不可删、B12 spill 跨副本认领安全、B13 abort 估算走校准、B14 流式 completion/cache-write 计价、B15 偏差指标加权 |
| `5955133` | ST-07 统一日志、ST-08 WARN 限流、ST-09 验证码清理+索引、ST-12 试用余额 fail-closed、ST-13 缓存省钱按真实价格、ST-14 时间序列 SQL 聚合+采样、ST-15/16 运维索引、视频任务保留期与 URL 脱敏（迁移 v28） |
| `b5af020` | W06 对账竞态、W07 公开状态页、W09 Escape 分层、W10 URL 过滤器、W11 渠道 key 不明文进 DOM、W12 公告刷新、W01 端到端回归、`anthropic_native` UI；count_tokens 原生渠道判定 |
| `cd35084` | 时间序列测试隔离（测试-only） |

### P2 批 2（MCP / 部署 / 认证 / 密钥静态安全）

| 提交 | 内容 |
|---|---|
| `dee8bf1` | A11 MCP SSE 401 响应头、A16 MCP 基础设施工具管理员限定（-32002）、A18 会话亲和租户命名空间（MCP 路径）；B7 经核实已在 822f007 修复 |
| `c00b926` | D06 可配置停机预算 + stop_grace/terminationGrace、D07 `/health/ready` 依赖探针（DB/Redis/快照）并接入全部部署产物、D14 安全默认（注册关闭/试用 0/S3 无内置凭证）、D17 有效存储日志、D18 Dockerfile 确定性 + trimpath + .dockerignore、ST-11 存储配置逐字段合并与校验、环境变量/文档补全 |
| `de194ef` | A10 租户由账号派生、A11 自助 key 默认/上限、A12 阶段一指纹与脱敏、A13 登录枚举/时序、A17 OAuth 用户名抢占 |
| `9c006a5` | 控制台：隐藏/只读 tenant、RPM/TPM 默认与上限提示、key 仅创建时显示明文、指纹展示与 CSV/片段去明文 |
| `68eeb3c` | A12 全量：API key 以 SHA-256 摘要落库（迁移 v29），`api_keys/usage_logs/billing_settlements/request_traces` 全部改为摘要，账本 join 不变；创建仅返回一次明文，列表返回掩码+指纹；摘要不可作为 bearer |
| `427fc63` | A12 集成测试格式修正 |
| `4524dc8` | 非管理员经 `/admin/keys` 创建时强制自助配额（rpm 默认 60/上限 6000、tpm 默认 10 万/上限 1000 万、budget 非负、租户服务端派生） |

## 2. P2/P3 覆盖

| 领域 | 已修复 |
|---|---|
| 认证安全 | A06-A17（A12 全量、A14 由 D15 文档化处理、A15 前端） |
| 计费 | B4-B15 |
| 路由/协议 | R-05..R-17 |
| API/流式 | A08-A20（A11/A16/A18-followup/B7 见批 2） |
| 存储/遥测 | ST-05..ST-16 |
| 前端 | W01-W12 |
| 部署 | D01-D18（D15 见 §5） |

## 3. 回归与线上验证

| 检查 | 结果 |
|---|---|
| `go vet ./...` / `go test -race ./...` | 多轮全绿（controlplane 83s / tests 45s / storage 29s / api 20s） |
| web | lint 0 告警；**46 文件 / 563 测试通过**；构建成功 |
| 迁移 | v28（运维索引：验证码/请求追踪/视频任务）、v29（API key 摘要）均已应用 |
| 密钥静态安全 | `api_keys`/`billing_settlements`/`request_traces` 明文 key 计数 = 0；旧明文 key 仍可认证（200），其摘要作 bearer → 401；新 key 创建返回明文一次、列表仅掩码+`kfp_` 指纹；账本漂移仍只有已知基线 ¥2.6475 |
| 自助 key | 非管理员经 `/admin/keys`：tenant=账号、rpm=60、tpm=100000、budget≥0；管理员仍可 0=不限 |
| 就绪/停机 | `/health` 与 `/health/ready` 均 200；启动日志 `storage=postgres data_source=postgres:5432/airoute`；stop_grace/termination 已配置 |
| MCP | 未认证 SSE handshake → 401 `application/json`；租户 key 调 `airoute_cluster_status`/`airoute_model_topology` → -32002；跨 key 会话注入 403 |
| 计费保护 | 删除已计费流水 → 409 且行保留；新指标 `billing_unattributed_total`/`billing_ledger_unlinked_units`/`billing_audit_pruned_units`/`billing_estimate_bias_ppm` 可用 |
| 其它 | `count_tokens` 200 走原生渠道；时间序列 24h 接口 200；发布后 30 分钟 2501 条 200、0 条 5xx；两副本 0 ERROR；临时 key 全部清理（active=4） |
| A12 发布方式 | 停机协调重启（先备份 DB，停止两副本，依次启动以单实例执行迁移 v29，再 reload LB）；发布窗口内 LB 有秒级 502，恢复后真实流量 200 |

## 4. 行为变化（运维需知）

1. **API key 静态哈希化（迁移 v29，一次性）**：数据库不再保存明文 key；控制台只在创建成功时显示一次，之后仅显示掩码与 `kfp_` 指纹；旧 key 继续可用，摘要本身不能当 key 使用。升级必须停机协调（新旧副本不能并存）。
2. **就绪探针**：Docker/K8s/compose 健康检查改用 `/health/ready`（依赖 DB/Redis/快照）；`/health` 仅作廉价存活探针。重建脚本的滚动门禁因此能感知依赖故障。
3. **停机/排空**：默认停机预算 = 排空预算 + 15s（默认 135s），compose/stack `stop_grace_period=150s`，Helm/ K8s `terminationGracePeriodSeconds=180`（可配）。
4. **安全默认**：shipped `config.yaml` 关闭注册、试用额度 0、S3 无内置凭证；存储配置缺 driver/凭证会拒绝启动（不再静默回退 local）。
5. **保留策略**：验证码默认 7 天、视频任务默认 30 天清理；视频任务 URL 去除 userinfo 与凭证类查询参数。
6. **路由 URL 语义**：裸 origin 的 OpenAI 风格渠道现在请求 `/v1/...`（控制台预设与线上渠道均带版本/前缀，不受影响）。
7. **审计不可删**：已计费流水不能删除（409），需冲正流程；报表/导出以指纹标识 key。
8. **前端**：Escape 只关闭最上层；渠道上游 key 不再展示；自助 key 表单隐藏 tenant、显示配额默认/上限与一次性明文+指纹。

## 5. 已知残留（不阻塞目标）

- **D15**：Host 头控制面隔离仍为"文档化限制"而非强制（用户通过 `http://127.0.0.1:8080` 使用控制台，默认拦截会破坏该访问方式）；已有认证、强凭证、metrics token、注册关闭作为补偿控制。若需强制，可按部署批次增加显式 Host/IP allowlist。
- 历史 `usage_logs` 中仍有 3 条**脱敏片段**（如 `sk-aud...27`，length 11，非可用凭证），迁移只重写真实 key；不影响安全。
- 测试健壮性：`TestForeignCurrencyRecordDoesNotPoisonBatch` 在 `-count=3` 下因包级 Prometheus 指标状态串扰偶发失败（单次运行稳定）。
- `off_peak_discount<=0` 仍被强制为 0.5，无法表达"无折扣"。
- MCP SSE 会话仍是单副本内存态（跨副本需共享存储/粘性路由）。
- 外币定价被拒请求的 `X-Airoute-Cost` 响应头仍显示未实际收取的计算金额（审计行以 0 为准）。
- 迁移 v27（邮箱唯一索引）遇历史重复邮箱时只告警、不自动重试。
- API key 摘要为无盐 SHA-256：对 128bit 随机 key 足够，DB 泄露不再得到可用凭证，但会暴露 key 存在性与关联性。

## 6. 证据

- 各领域报告：[01-auth-security.md](01-auth-security.md)、[02-billing.md](02-billing.md)、[03-routing.md](03-routing.md)、[04-api.md](04-api.md)、[05-storage.md](05-storage.md)、[06-web.md](06-web.md)、[07-deploy.md](07-deploy.md)
- 第一批（P0+P1）记录：[remediation-2026-10-27.md](remediation-2026-10-27.md)
- 首批线上验证：[live-verification-2026-10-27.md](live-verification-2026-10-27.md)
