# 上线后 17 小时实测复核（2026-10-29 08:43 +08）

> 对象：`1.1.0-dev+g7fee32f`（2026-10-28 15:24 滚动发布，含上一轮 24 条缺陷修复 + 代码瘦身）。
> 目的：用**真实运行数据**验证修复是否成立、有没有引入新问题，而不是只看测试。
> 方式：只读（docker logs / Prometheus / PostgreSQL SELECT / Redis SCAN），不改任何数据。

---

## 1. 结论

**没有回归、没有新错误、账实一致。**

- 两个副本 + LB + postgres + redis + storage 全部 healthy，已连续运行 17 小时；
- 网关日志 **0 条 ERROR**，8 条 WARN 全部是启动期既定提示（见 §3），**0 条控制面拒绝**；
- 对外端点：`/health/ready` 200、`/app/` 200（控制台可用）、`/v1/models` 401（需鉴权，符合预期）、`/metrics` 带 token 200；
- 金额侧：重复 `billing_id` **0**、负钱包 **0**、**可计费但未结算**的审计行 **0**、`api_keys.used_cost_units` 与 `billing_settlements` 汇总**零漂移**；
- 结算/配额侧：`settlement_queue_depth=0`、`settlement_wait_timeouts=0`、`settlement_lock_timeouts=0`、`billing_reserved_units=0`、Redis `airoute:inflight:*` 键 **0 个**（心跳续期与释放配对正常，无泄漏）；
- 数据面持续可用（本机 harness 经 LB 的 `/v1/messages` 持续 200）。

> 口径说明：复核窗口内夜间无业务流量（部署后 17h 只有 6 条 `usage_logs`，均为 200/upstream），所以本轮证据主要证明**空闲态一致性**（无泄漏、无丢单、无漂移）；高并发正确性由单测/`-race`/真 Redis 集成测试覆盖。上一轮 24h 内 4400 条 `usage_logs` 来自部署前的旧容器。

---

## 2. 采集命令（可复现）

```bash
cd /home/ifnodoraemon/myagent/airoute
# 容器与端点
docker compose ps
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/health/ready
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/app/

# 日志：错误 / 告警 / 控制面拒绝
for c in airoute-1 airoute-2; do
  echo -n "$c ERROR="; docker logs $c 2>&1 | grep -c '"level":"ERROR"'
  echo -n "$c denial="; docker logs $c 2>&1 | grep -c "control-plane access denied"
done

# 指标
TOKEN=$(grep -E '^GATEWAY_METRICS_TOKEN=' .env | cut -d= -f2)
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/metrics | grep -E '^airoute_(settlement|billing_reserved)'

# 金额一致性（只读）
source <(grep -E '^POSTGRES_(USER|PASSWORD|DB)=' .env | sed 's/^/export /')
psql() { docker exec -e PGPASSWORD="$POSTGRES_PASSWORD" airoute-postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "$1"; }
psql "select count(*) from (select billing_id from usage_logs where billing_id<>'' group by 1 having count(*)>1) t;"   # 重复计费键
psql "select count(*) from users where balance_units < 0;"                                                              # 负钱包
psql "select count(*) from usage_logs l left join billing_settlements s on s.billing_id=l.billing_id
      where l.billing_id<>'' and l.cost_units>0 and s.billing_id is null;"                                               # 漏结算
psql "select count(*) from api_keys k left join (select api_key,sum(cost_units) t from billing_settlements group by 1) s
      on s.api_key=k.key where k.used_cost_units <> coalesce(s.t,0);"                                                    # 账本漂移

# 预授权残留
docker exec airoute-redis sh -c 'redis-cli --scan --pattern "airoute:inflight:*" | wc -l'
```

---

## 3. 观测结果

### 3.1 容器与端点

| 对象 | 状态 |
|---|---|
| airoute-1 / airoute-2 | Up 17 hours (healthy)，版本 `1.1.0-dev+g7fee32f` |
| airoute-lb / postgres / redis / storage | Up 17–46 hours (healthy) |
| `/health/ready` · `/app/` · `/v1/models` · `/metrics`(token) | 200 · 200 · 401 · 200 |

### 3.2 日志

- ERROR：**0**（两副本合计）。
- 控制面拒绝：**0**（DEPLOY-01 修复后未再出现）。
- WARN：8 条 = 每副本 4 条启动期提示，与配置状态一致，均非异常：
  1. `SECURITY WARNING: STRIPE_WEBHOOK_SECRET is unset`（本机未启用支付）；
  2. `circuit breaker runs in advisory mode`（`GATEWAY_BREAKER_ENABLED=false`）；
  3. `models served without a configured price`（4 个模型走兜底价，属数据配置）；
  4. `upstream models served without their own price`（1 个 served model 走别名价）。
- 访问日志：29 条（200×26、301×1、401×2），**0 条 5xx**。

### 3.3 金额与结算

| 检查 | 结果 |
|---|---|
| `usage_logs` 总数 / 24h | 66,675 / 4,400（24h 内绝大多数来自部署前） |
| `billing_settlements` 总数 / 24h | 66,238 / 4,400 |
| 差值 437 | 不可计费审计行（4xx/零成本），符合设计 |
| 重复 `billing_id` | **0** |
| 负钱包 | **0** |
| 可计费却无结算行 | **0** |
| `used_cost_units` vs 结算汇总漂移 | **0 行**（完全一致） |
| 部署后 17h 的审计行 | 6 条，全部 `200 / upstream` |

### 3.4 结算与配额指标

```
airoute_requests_total                 7
airoute_settlement_batches_total       7     ← 每个请求都有结算批次
airoute_settlement_batch_records_total 7     ← 无重复计数
airoute_settlement_queue_depth         0
airoute_settlement_wait_timeouts_total 0     ← 没有请求因等结算而 defer
airoute_settlement_lock_timeouts_total 0
airoute_billing_reserved_units         0     ← 结束态回到 0：预授权全部释放
settlement_duration p99                ≈25–50ms
Redis airoute:inflight:* keys          0
```

`billing_reserved_units == 0` 与 Redis 无 in-flight 键是**本轮重点验证项**：上一轮 B-01 的修复（心跳续期 + 先停心跳再释放）如果配对错了，空闲时必然留下残留额度；这里为 0，说明 reserve/release 在真实流量下是对齐的。

### 3.5 数据面

- 本会话（08:43 起）经 LB 的 `/v1/messages` 持续 200，网关侧 `usage_logs` 同步落库（200/upstream），说明鉴权、路由、计费、审计整条链路在新镜像上正常。
- 夜间无业务流量，未观察到限流/熔断相关事件（`GATEWAY_BREAKER_ENABLED=false`，熔断本就只记录不拦截）。

---

## 4. 本轮未覆盖 / 待下轮观察

1. **高并发真实流量**：空闲窗口无法验证 TPM/RPM 滑动窗口、结算批处理在压力下的行为；需要一次限流或压测窗口。
2. **Gemini 思考 token 计费**：窗口内没有 Gemini 推理模型流量，账单变化待有流量后抽查。
3. **分时折扣 start-day 语义**：窗口内没有跨零点折扣规则命中，建议运营复核既有规则配置（见上一轮 §4.2 第 2 条）。
4. **长请求预授权心跳**：窗口内没有 >40s 的请求，心跳路径只在单测/真 Redis 集成测试中验证过。
