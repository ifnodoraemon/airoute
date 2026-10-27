# 审查发现修复记录 — P0 + P1 第一批（2026-10-27）

对应 [本轮全面审查](README.md) 的 P0 与第一批 P1。代码已滚动发布，版本 **`1.1.0-dev+gc1fc7eb`**。

## 1. 已修复项与提交

| 批次 | 提交 | 内容 |
|---|---|---|
| P0 安全 | `48fdd4e` | 弱凭证启动门禁改为默认拒绝（显式 `GATEWAY_ALLOW_INSECURE_DEFAULTS` 才放行）；Compose/Swarm/Helm/K8s 补 `GIN_MODE=release`、admin password 密钥引用；`docker-rebuild.sh` 收敛依赖/LB 配置并 reload nginx |
| P0 配置 | 环境配置（未提交 `.env`） | 轮换管理员密码与 HMAC 签名密钥；关闭注册（`ALLOW_REGISTRATION=false`）、开启邮箱验证；设置 `GATEWAY_METRICS_TOKEN`；启动日志已无弱凭证告警 |
| P1-A MCP | `822f007` | `airoute_chat` 复用数据面鉴权（key 状态/账户锁定/钱包/key 预算/模型白名单/RPM/TPM/在途预留）；计费改走统一 `RecordUsage`（估算兜底 + 定价快照 + 同步结算 + 渠道/租户归属）；SSE 会话与 key 强绑定、sessionId 提升到 128bit；访问日志脱敏 sessionId/api_key |
| P1-B 计费边界 | `cbbe550` + `c1fc7eb`（+`aa7faab` 测试） | 外币快照记录按条隔离（不再毒化整批，成本置零并保留审计）；Redis entry / spill 行 / 内容哈希三层稳定幂等键（零时间戳记录不合并）；视频先持久化再计费；多模态取消→499、其余保持 502 契约（分离上游上下文 + drain）；流式 prompt 估算纳入 tools/图像；completion 估算统一 `/4`；moderation 透传上游 5xx 且成功计费/审计；`usage_logs.error_message` 脱敏 |
| P1-C 稳定性 | `38e4e1a` | 环形缓冲区广播与退订同锁，消除 `send on closed channel` panic（-race 回归测试）；移除 off-peak 合成 slot，恢复运维窗口语义；trace 脱敏覆盖 `Bearer <token>` 等形式；Redis URL / libpq DSN 日志脱敏；配置展开支持 `$$` 字面量；`users.email` 大小写不敏感唯一索引（迁移 v27）+ 注册冲突 409 |
| P1-E 前端 | `56b4fe8` | AiCopilot 结构化流式错误必达、reader 必释放、兜底文本带标记；auth 页登录补租户状态清理；移除 localStorage 明文 token 与 SSE URL token；Dashboard 不再伪造延迟/SLA，错误预算按 99.9% 计算；导出截断边界修正 |
| 文档 | `e61d35e` | 审查报告与线上验证证据 |

## 2. 回归与发布验证

| 检查 | 结果 |
|---|---|
| `go vet ./...` | 通过 |
| `go test -race -count=1 ./...` | 全部包 ok（api 11.7s / storage 20.7s / controlplane 24.8s / tests 42.5s） |
| web | `npm run lint` 0 告警；**41 文件 / 521 测试通过**；`npm run build` 成功 |
| 滚动发布 | 镜像 `1.1.0-dev+gc1fc7eb`；两副本 healthy；发布期间依赖容器与 LB 按新配置重建；`ERROR` 日志 0 |
| 迁移 | `schema_migrations=27`；`uq_users_email_lower` 已创建 |
| 容量配置 | PostgreSQL `max_connections=200`；postgres/redis/lb/gateway 均 `nofile=65536`（容器内 `/proc/limits` 确认） |
| 凭证 | 旧密码 `admin123` → 401；新密码登录 200 且 `is_default_password=false`；启动无弱凭证 SECURITY WARNING |
| 暴露面 | `/metrics` 无 token → 401，带 token → 200；注册 → 403；`/v1/models` 无 key → 401 |
| 数据面 | 热加载两副本一致；RPM 3 → `200,200,200,429,429,429`；聊天计费 `cost=0.000035`、结算/账本一致 |
| 断连排空 | `499`、`completion=20`、`cost=0.000117`、结算 1 条 |
| MCP | 受限 key 调未授权模型 → 拒绝；允许模型 → 成功且 `pricing_snapshot` 非空、有 `billing_id`、已结算；访问日志显示 `sessionId=[REDACTED]` |
| MCP 会话绑定 | 同一副本内跨 key POST → **403**；同 key → 200（LB 跨副本时进程内 session 不共享为已知限制） |
| ring buffer | 管理台日志 SSE 连接 2s 正常关闭，两副本 0 panic / 0 `send on closed channel` |
| 对账 | worker 3 分钟后运行：`ledger_drift=¥2.647492`（已知历史基线）、`wallet_drift=0`、`audit_lag=0`、重复 `billing_id=0`、`settlements_deferred=0` |
| 清理 | 临时审计 key 全部软删，active key 恢复为 4；临时 worktree 与 /tmp 凭证已清理；仓库仅新增文档提交 |

## 3. 尚未修复（下一批）

- **路由/超时 P1**：R-01（Transport 60s 截断渠道超时）、R-02/A19（排空 120s 绝对 deadline）、R-03（多模态 429 熔断）、R-04（`anthropic_messages` 协议误切换）、R-07（流式 preamble 丢首 token）、R-13（模态路径忽略渠道超时）。
- **认证 P1 残留**：A04（X-Forwarded-For 伪造绕过登录限速）、A05（注册限流/试用额度策略——当前仅关闭注册，代码级限流未加）、A03（DB 快照失败后数据面 fail-open）、A06（OAuth 开放重定向）、A09（root admin 密码可被其他 admin 重置）、A07（`?token=` 会话凭证）。
- **部署/运维 P1/P2**：D05（控制面 SSE 被 WriteTimeout 截断）、D07（`/health` 不检查依赖）、D08（CLI 失败仍 exit 0）、D09（硬编码旧 key）、D10（配置缺失静默回退）、D11（环境变量漂移）、D12（文档与代码不一致）、D13（Helm port 漂移）、D15（Host 头边界）、D16（CI 不校验部署产物）。
- **本批修复的后续观察项**：MCP 会话跨副本不共享；被拒外币请求的 `X-Airoute-Cost` 仍显示未收金额；迁移 v27 若因历史重复邮箱失败只告警不重试；`off_peak_discount<=0 → 0.5` 仍无法表达“无折扣”；W01 缺端到端测试。
- 其余 P2/P3 按各领域报告逐项处理。

## 4. 运维注意

- 本轮轮换了 `GATEWAY_ADMIN_SECRET`：所有旧管理台会话失效，需要重新登录。
- `/metrics` 需要 `Authorization: Bearer $GATEWAY_METRICS_TOKEN`（抓取端需同步配置）。
- 注册通道已关闭、邮箱验证已开启；如需开放注册，应先完成代码级限流与验证流程。
- 依赖容器与 LB 在本次发布中按新配置重建（有秒级中断）；后续 `make docker-rebuild` 在配置无变化时不再重建它们。
- 本地开发若使用默认弱凭证，需显式设置 `GATEWAY_ALLOW_INSECURE_DEFAULTS=true`，否则进程拒绝启动。
