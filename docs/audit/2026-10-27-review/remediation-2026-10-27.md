# 审查发现修复记录 — P0 + P1 全部完成（2026-10-27）

对应 [本轮全面审查](README.md) 的 **P0（1 个根因）与全部 27 项 P1**，分两批修复、回归并滚动发布线上验证。

- 第一批发布版本：`1.1.0-dev+gc1fc7eb`
- 第二批发布版本：`1.1.0-dev+gbb43d92`（当前线上）
- P2/P3 仍未处理（清单见 §5），不在本轮目标内。

## 1. 提交清单

### 第一批（P0 + 安全/资金/稳定性/前端）

| 提交 | 内容 |
|---|---|
| `48fdd4e` | 弱凭证启动门禁默认拒绝（显式 `GATEWAY_ALLOW_INSECURE_DEFAULTS` 才放行）；Compose/Swarm/Helm/K8s 补 release 模式与 admin password；`docker-rebuild.sh` 收敛依赖/LB 并 reload nginx |
| 环境配置（未提交 `.env`） | 轮换管理员密码与 HMAC 密钥；关闭注册、开启邮箱验证；设置 `GATEWAY_METRICS_TOKEN` |
| `822f007` | MCP `airoute_chat` 复用数据面鉴权（锁定/预算/钱包/白名单/RPM/TPM/预留）+ 统一定价快照与结算；SSE 会话与 key 强绑定（128bit）；sessionId 日志脱敏 |
| `cbbe550`+`c1fc7eb`+`aa7faab` | 计费边界：外币按条隔离、三层稳定幂等键、视频先持久化后计费、取消→499/其余 502、流式 prompt 含 tools、completion 统一 `/4`、moderation 透传并计费、审计错误脱敏 |
| `38e4e1a` | ring buffer panic、off-peak 合成 slot、trace/Redis/DSN 脱敏、`$$` 配置展开、`users.email` 唯一索引（v27） |
| `56b4fe8` | 前端：AiCopilot 流式错误必达、租户状态清理、移除 localStorage 明文 token、Dashboard SLA/错误预算 |
| `e61d35e` | 审查报告与线上验证证据 |

### 第二批（路由/认证/API/部署）

| 提交 | 内容 |
|---|---|
| `7ec106a` | 路由/超时：移除 60s Transport 截断（预算由渠道超时派生）、排空窗口仅在断连后计时、多模态 429 不计熔断、`anthropic_messages` 与 `anthropic_native` 语义拆分、流式 preamble 保提交事件、模态路径纳入渠道超时、advisory 熔断记录状态、first-token 上限生效 |
| `3b92c74` | 认证：`GATEWAY_TRUSTED_PROXIES` + 跨副本 Redis 登录限速（防 XFF 伪造）、DB 快照未就绪时数据面 fail-closed（503）、注册限流/验证门禁、OAuth 重定向仅用配置的公网地址、root 管理员密码保护、移除 `?token=` 会话凭证 |
| `879e6dd` | API：Responses SSE 完整事件序列（item/item_id/output_index/content_index/sequence_number）、Gemini `:embedContent` 与模型白名单、MCP notification/协议版本、trace session/chat id、count_tokens 404、413 映射、tool_use `input`、会话亲和按租户命名空间、completions token-id 拒绝与错误流 `[DONE]` |
| `dffa7c6` | 部署：Swarm/Helm/旧 nginx 超时对齐并纳入测试、控制面 SSE 清除 WriteTimeout、CLI 失败退出码、移除硬编码 key、缺失配置 fail-fast、env/文档漂移修正、Helm 端口单一来源、Host 屏障如实文档化、CI 校验部署产物 |
| `a4770e1` | `GATEWAY_TRUSTED_PROXIES` 接入 Compose/Swarm/Helm/.env.example/文档 |
| `eadfc7e` | gofmt 修正 |
| `bb43d92` | 记录入站 `anthropic_messages` 与上游 `anthropic_native` 标记的语义与用法 |

## 2. P1 覆盖（27/27）

| 领域 | 已修复 P1 |
|---|---|
| 认证安全 | A02、A03、A04、A05 |
| 计费 | B1、B2、B3 |
| 路由/超时 | R-01、R-02、R-03、R-04 |
| API/流式 | A01、A02、A03、A04、A05、A06、A07 |
| 存储/遥测 | ST-01、ST-02、ST-03、ST-04 |
| 前端 | W02 |
| 部署 | D01（P0）、D02、D03、D04、D05 |

每项均附"HEAD 失败、修复后通过"的定向回归测试；两批合计新增/更新测试覆盖 Go（router/provider/api/storage/controlplane/middleware/config/telemetry/cmd）与前端（Vitest）。

## 3. 回归与发布验证

| 检查 | 结果 |
|---|---|
| `go vet ./...` | 通过 |
| `go test -race -count=1 ./...` | 全部包 ok（controlplane 53s / tests 42s / api 16s / storage 21s） |
| web | lint 0 告警；41 文件 / 521 测试通过；生产构建成功 |
| 第一批发布 | `gc1fc7eb`；两副本 healthy；迁移 v27；PG `max_connections=200`；全部 nofile 65536 |
| 第二批发布 | `gbb43d92`；两副本 healthy；依赖/LB 配置无漂移；`ERROR` 日志 0 |
| 凭证/暴露面 | 旧密码 401、新密码 200 且非默认；`/metrics` 401→200（带 token）；注册 403；`GATEWAY_TRUSTED_PROXIES=172.22.0.0/16` 已生效 |
| Anthropic 原生路径（R-04 重点） | 带 `cache_control` + tools 的 `/v1/messages` 非流式 200（279 prompt tokens）；流式 200，事件含 message_start/content_block_delta/message_stop |
| Responses SSE（A07） | 200，事件含 `sequence_number`、`content_index`、`item_id` 与完整 output_item/content_part/text 序列 |
| MCP | 受限 key 调未授权模型被拒；允许模型与普通 key 调用成功；同副本跨 key 会话注入 403、同 key 200 |
| 限流 | rpm=2 → `200,200,429,429` |
| 断连排空 | 499、completion=20、cost=0.000116、`pricing_snapshot`/`billing_id` 均在、已结算 |
| 账单 | 临时租户 7 条流水全部有快照与结算；发布后 10 分钟 5xx = 0 |
| CLI（D08） | 失败命令 exit=1，成功命令 exit=0 |
| 控制面 SSE（D05） | 管理台日志流 2s 正常；两副本 0 panic |
| 清理 | 临时 key 全删（active=4）；worktree/临时凭证清理；仓库干净（仅遗留 docx） |

## 4. 行为变化（上线需知）

1. **管理台会话**：P0 轮换了签名密钥，旧会话失效，需重新登录。
2. **`/metrics`**：需要 `Authorization: Bearer $GATEWAY_METRICS_TOKEN`。
3. **注册**：默认关闭；开启时受 IP/用户名限流，且 `REQUIRE_EMAIL_VERIFICATION=true` 时未验证不发 key/试用额度。
4. **登录限速**：默认不信任任何转发头（安全默认）；Compose 已设 `GATEWAY_TRUSTED_PROXIES=172.22.0.0/16`，其它部署需按实际代理网段配置。
5. **启动 fail-closed**：首次数据库快照完成前 `/v1` 返回 503 `data_plane_unready`（毫秒级；DB 不可用则持续 503）。
6. **单一超时旋钮**：流式总时长现在由渠道 `timeout_seconds` 约束（上线渠道为 300s）；需要更长请调渠道超时，而不是排空变量。
7. **Anthropic 原生上游**：仅 Provider Type=`anthropic` 或 `protocols` 含 `anthropic_native` 的渠道走原生 `/v1/messages`；线上 `罗总` 渠道已加该标记（原行为保持不变），OpenAI 渠道默认转译。
8. **OpenAI 兼容性收紧**：`/v1/completions` 的 token-id prompt 返回 400；Responses SSE 改为官方事件 schema；查询参数 `?token=` 不再作为管理会话凭证；root 管理员密码只能由自身凭据重置。
9. **依赖容器/LB**：首次应用配置时重建过一次（秒级中断），之后配置不变不再重建。

## 5. 剩余 P2/P3（未在本目标内）

- 计费/账本：B5（无主扣款静默）、B7（MCP 无 usage 兜底）、B8（legacy 键不可 join）、B9/B10（保留期/硬删破坏对账）、B12-B15。
- 存储/遥测：ST-05 之后的 P2（trace 缺 api_key 索引、video_tasks 索引/保留期、时间序列全量聚合、S3 默认凭证、验证码清理等）。
- 路由：R-05（半开 canary 泄漏）、R-08/R-09（thinking/tool index）、R-10/R-11（Gemini 错误与 tool id）、R-12（四种 base URL 约定）、R-14/R-15/R-17。
- API：A08-A20 中除本批已修项外的边缘项（Gemini 其它方法、MCP SSE 头、会话亲和细节等）。
- 前端：W01 缺端到端测试、W03-W12 其余项、`anthropic_native` 尚无可视化勾选（当前需管理 API/保留字段）。
- 部署：D06（10s 优雅停机 vs 120s 排空）、D07（`/health` 不检查依赖）、D14（config.yaml 默认值）、D15 强制隔离、D17/D18。
- 已记录的实现后续：MCP 会话跨副本不共享、被拒外币请求的 `X-Airoute-Cost` 仍显示未收金额、迁移 v27 历史重复邮箱只告警不重试、`off_peak_discount<=0` 无法表达"无折扣"。

## 6. 证据

- [live-verification-2026-10-27.md](live-verification-2026-10-27.md)（第一批详细命令与输出）
- 各领域报告内附 HEAD 失败/修复后通过的定向探针与 `file:line`。
