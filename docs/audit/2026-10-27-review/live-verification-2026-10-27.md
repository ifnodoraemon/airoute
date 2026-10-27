# 线上验证记录 — 2026-10-27（重建重启后）

所有命令在本机执行，敏感值（密码/密钥/token/上游 key）已省略或脱敏。时间均为 Asia/Shanghai。

## 1. 重建与重启

```text
$ make docker-rebuild
==> Building airoute image (1.1.0-dev+g64e749a)...
...
==> Recreating gateway-2...
==> airoute-2 is healthy
==> Recreating gateway-1...
==> airoute-1 is healthy
==> Gateway cluster is running version 1.1.0-dev+g64e749a
[exit code: 0]
```

```text
$ docker ps --filter name=airoute
airoute-1        Up (healthy)   airoute:latest
airoute-2        Up (healthy)   airoute:latest
airoute-postgres Up (healthy)
airoute-redis    Up (healthy)
airoute-storage  Up (healthy)
airoute-lb       Up
```

两副本镜像一致：`sha256:bcd1683c471a…`；启动时间 09:35:38Z / 09:35:46Z。

## 2. 健康检查

```text
$ curl -s http://127.0.0.1:8080/health
{"service":"airoute","status":"healthy","timestamp":1793064963,"version":"1.1.0-dev+g64e749a"}

$ docker exec airoute-1 curl -s http://127.0.0.1:8080/health   → 200
$ docker exec airoute-2 curl -s http://127.0.0.1:8080/health   → 200
```

启动后两副本 `ERROR` 日志计数均为 **0**。真实流量（`POST /v1/messages?beta=true`，租户 `密钥-1`）持续返回 200。

## 3. 基线测试

```text
$ make test           # go test -v -race ./...
PASS  ./tests (45.3s) / ./web (1.1s) / 其余包全部 ok   [exit code: 0]
$ go vet ./...        # 无输出
$ cd web && npm run lint    # 0 warnings
$ cd web && npm test        # 38 files / 507 tests passed
$ cd web && npm run build   # ✓ built
```

## 4. 认证与 CSRF

```text
$ curl -s -X POST /api/v1/auth/login  -d '{"username":"admin","password":"<from .env>"}'
{"code":0,"data":{"user":{"username":"admin","role":"admin","is_default_password":true},"token":"…"}}
        ← P0 证据：线上仍在用默认管理员密码

$ curl -s /v1/models                    → 401 {"error":{"code":"missing_api_key"}}
$ curl -s /v1/models -H 'Bearer sk-not-a-real-key' → 401
$ curl -s /api/v1/admin/keys            → 401
$ curl -s -b cookie -X DELETE /api/v1/admin/keys/999999   → 403 CSRF 校验失败
$ curl -s -b cookie -H 'X-CSRF-Token: <cookie值>' -X DELETE /api/v1/admin/keys/999999 → 404（预期业务响应）
```

## 5. 热加载与分布式限流

```text
# 新建 key 后 1 秒，两副本均可见：
airoute-1 /v1/models → 200
airoute-2 /v1/models → 200

# 删除后：
airoute-1 → 401,  airoute-2 → 401

# rpm=3 的 key，连续 6 次 GET /v1/models：
200 200 200 429 429 429
body: {"error":{"code":"rate_limit_exceeded","message":"Cluster rate limit exceeded (RPM limit reached). ..."}}
```

## 6. 计费（非流式 / 流式）

非流式 `POST /v1/chat/completions`（`deepseek-v4.1-flash`, `max_tokens=1`）：

```text
status=200   X-Airoute-Cost: 0.000035   X-Airoute-Trace-Id: tr-1d9ea42-…
usage: prompt_tokens=31 completion_tokens=1

SQL:
usage_logs:        cost=3.5e-05  cost_units=35000  usage_source=upstream  channel=罗总  billing_id=set
billing_settlements: cost_units=35000
api_keys(tenant=audit-live): used_cost_units=35000, used_tokens=32
```

流式 SSE：

```text
HTTP/1.1 200 OK
Content-Type: text/event-stream
data: {...chat.completion.chunk...}
data: [DONE]                       ← 10 行 SSE，含 [DONE]

SQL: usage_logs cost_units=35000；billing_settlements=35000
```

## 7. 客户端断连 + 上游排空

```text
curl --max-time 0.35 stream=true max_tokens=50  → 客户端 0.35s 断开

等待排空后 SQL:
usage_logs: status=499  prompt_tokens=39  completion_tokens=50  cost=0.000239
            usage_source=upstream  channel=罗总  duration_ms=1125  billing_id=set
billing_settlements: cost_units=239000, total_tokens=89

metrics:
airoute_requests_client_canceled_total 1
airoute_aborted_drained_requests_total 1
```

结论：断连仍按上游真实 usage 计费（499 + 排空），与上一轮修复目标一致。

## 8. 对账与账本

```text
$ docker exec airoute-1 curl -s :8080/metrics | grep billing
airoute_billing_settled_synchronously_total     84（副本1）/ 79（副本2）
airoute_billing_settlements_deferred_total      0
airoute_billing_duplicates_skipped_total        0
airoute_billing_overdrafts_total                0
airoute_billing_ledger_drift_units              2647492000   ← ¥2.6475 已知历史基线
airoute_billing_wallet_drift_units              0
airoute_billing_audit_lag_units                 0
airoute_usage_log_records_spilled_total         0
airoute_usage_log_queue_depth                   0
```

```text
schema_migrations max(version)=26
usage_logs: 50,943 行；billing_id 非空 21,852；billing_settlements 50,598（含 legacy-<id> 29,107）
重复 billing_id = 0
今日状态码分布：200 × 1,860（重启后无 5xx）
created_at 微秒精度：…01:38:23.37668+00（微秒位 376680）
```

已知但非资金问题的历史残差：

```text
带 billing_id 但无结算的审计行 = 429 条，全部 2026-10-26 的零成本错误行（424 条 5xx），
cost_units=0 且 total_tokens=0 → repairSettlementGap 按设计跳过；
本轮新增 4 条同类（401 鉴权失败），见下方 L1。
```

## 9. 本轮实测可见的配置/安全问题

| 项 | 实测结果 |
|---|---|
| 默认管理员凭证 | `admin/admin123` 登录 200，`is_default_password=true`（P0） |
| 管理员签名密钥 | `.env` 中等于仓库公开占位值（弱密钥可伪造 JWT）（P0） |
| 开放注册 | `/api/v1/public/status` → `allow_registration=true`、`require_email_verification=false`；`.env` 初始余额 ¥5 |
| `/metrics` | 未鉴权直接 200（`GATEWAY_METRICS_TOKEN` 未设置） |
| 控制面 Host 屏障 | 默认 Host 访问 `/api/v1/admin/keys` → 401（可达）；`Host: api.airoute.internal` → 403；`/app/` 默认 Host → 200 |
| 渠道上游 key | `GET /api/v1/admin/channels` 返回完整 `api_key`（长度 58/67，明文进控制台） |
| PostgreSQL | `max_connections=100`（compose 声明 200）；`/proc/1/limits` open files soft=1024；容器创建于 2026-10-09，未重建 |
| Redis | `maxclients=10000`；ulimits 未应用（容器创建于 2026-10-08） |
| LB | `nofile` 仍 1024（compose 声明 65536；容器创建于 2026-10-08，滚动发布未重建/重载） |
| 网关副本 | `nofile=65536`（已应用） |
| 网关 DB 池 | 启动日志 `max_open=25, max_idle=8` |

## 10. L1：鉴权失败审计的 usage_source 与结算缺口

实测一条 401 审计行（`usage_logs.id=51167`）：

```text
api_key=sk-aud...27  status_code=401  error_message="Incorrect or unauthorized API key provided"
usage_source=upstream      ← 实为 gateway_auth
cost_units=0  total_tokens=0  billing_id=bill-202d…
```

原因：`internal/middleware/auth.go` 的 `auditAuthFailure` 写入 `UsageSource="gateway_auth"`，但
`internal/storage/repo_logs_writer.go:49` 的 `normalizeUsageSource` 只保留 `estimated`，其余全部归一为
`upstream`。同时写入器会给零成本行也分配 `billing_id`；结算与 gap 修复都跳过 `cost=0 && total_tokens=0`，
于是这些行永久停留在“有审计、无结算”状态（当前 433 条），污染基于 billing_id 的对账查询。

## 11. 清理

- 审计用临时 key id 19–22 已通过管理 API 删除（`status=deleted`，active key 数恢复为 4）。
- 管理员余额、用户表、渠道与价格表未被审计修改；仅新增审计流水与上述软删 key。
- 所有 `/tmp` 探针、日志与私有 worktree 已清理；`git worktree list` 仅剩主仓。
