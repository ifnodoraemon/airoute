# 修复记录：C 域（计费 / 校验 / 遥测 / 分布式）

> 域：`internal/billing`、`internal/validator`、`internal/telemetry`、`internal/distributed`
> 对应任务：`task-8`；审计来源：[03-billing-telemetry.md](03-billing-telemetry.md)
> 基线：审计基线 `1520d99`；本域改动落在工作区（未提交，遵守"禁止 git 写操作"）。
> 行号以本文写作时的工作区为准（另一会话已提交到 `c8a2765`，其提交把她已写好的部分
> `estimate.go`/`metrics*.go`/`redis_quota.go` 一并带走；本域改动复核后仍在工作区中生效）。

## 1. 状态表

| 发现ID | 状态 | 改动文件:行 | 验证 | 备注 |
|---|---|---|---|---|
| C-01 | ✅ 已修 | `internal/billing/engine.go:397`（5 个变体已删，仅剩唯一入口）<br>`units_test.go:40`、`default_price_test.go:31,57`、`cache_and_bias_test.go:48`、`engine_test.go:251-269`<br>`internal/controlplane/functional_details_test.go:172`<br>`tests/pricing_and_model_routes_test.go:49,72`、`tests/wallet_and_group_pricing_test.go:253,254`、`tests/time_of_use_billing_test.go:75,126` | `deadcode ./cmd/airoute` 输出中 5 个符号**已消失**；全仓 grep 0 命中 | 见 handoff 1 |
| C-03 | ✅ 已修 | `internal/billing/calibration_persist.go:50` `SaveCalibration`、`:87` `persistedCalibrationStates`、`:105` `mergeCalibrationStates` | `go test ./internal/billing/ -run 'Calibration'` 4 条通过 | 解析/写失败保留旧值 + WARN 并返回 error |
| C-05 | ✅ 已修 | `internal/billing/calibration.go:102-103`（`Ratio()` 成为指标唯一取数口）、`:128`；`internal/telemetry/metrics.go:357` `EstimateSamplesByModel`；`internal/telemetry/metrics_prometheus.go:159-168` | `go test ./internal/telemetry/ ./internal/billing/` 通过 | 保留 `Ratio()`；样本数可观测（`Adjust` 的 5 样本阈值） |
| C-06 | ✅ 已修 | `internal/validator/openai.go:199` `checkDisallowedTemperature`、`:222` 文案；`anthropic.go:159-163`；`gemini.go:155-158` | `go test ./internal/validator/` 通过（新增 2 条 strict 用例） | 三协议共用一段逻辑；gemini 无 model 字段，单独兜底文案 |
| C-10 | ✅ 已修 | `internal/billing/strategy.go:96-109`；`internal/billing/strategy_validation.go:35` `ValidateOffPeakSlots`（唯一导出入口）、`:45` `validateSlots`、`:152` `validateOffPeakSlot` | `go test ./internal/billing/` 通过（3 条新用例）；`deadcode` 中无 `ValidateSlotsOverlap` | 与行级 "never invent a discount" 同口径；无 mode 兼容壳，见下 |
| C-11 | ✅ 已修（方案 a） | `internal/billing/model_pattern.go:26/41/71`（新文件）；`internal/billing/engine.go:105-136`（索引所有含 `*` pattern）、`:305-325`（统一匹配） | `deadcode` 中 `MatchModelPattern` 可达；`-run 'Wildcard\|MatchModel\|ReloadPrices'` 通过 | `*o1`→`openai/o1`、`gpt-4o*mini` 已覆盖 |
| C-12 | ✅ 已修 | `internal/distributed/redis_quota.go:13-33`（文件头：两套实现**同为 60s 滑动窗口**）、`:54` `rpmWindowMs`、`:63` `slidingWindowLuaSrc`（窗口走 ARGV[3]）、`:210-216` `AllowRPM`、`redis_rpm_window_test.go` | `go test ./internal/distributed/` 通过（2 条新用例） | 本地限流已由 A 域统一为同步长滑动窗口；`rpmWindowMs` 与 middleware `rpmWindowSpan` 双向互指注释 |

**未处理（按分工不属本域）**：C-02、C-04、C-07、C-08、C-09、C-13、C-14。

## 2. 逐条说明

### C-01 五个 `CalculateCost*` 变体是死链
删除 `CalculateCostDetailedWithGroup`、`CalculateCostForUnits`、`CalculateCostDetailed`、
`CalculateCostWithGroup`、`CalculateCost`；保留 `CalculateCostForUnitsWithCacheCreation`
（生产唯一入口，`internal/api/common.go:233`）与 `IsOffPeak` 等真实使用方法。

测试调用点等价改写：`units=1`、`cacheCreationTokens=0`，时间参数沿用原来的固定时刻或
`time.Now()`，因此断言值不变（`0.0136/0.0144`、`18.0/10.0`、`10.0`、`1.08` 等）。
返回值多一个 `snapshot`，用 `_` 接住。

### C-03 校准整表覆盖写库
`SaveCalibration` 改为 **读库 → 本副本 `Snapshot()` → 按 `Samples` 取大者合并 → 写回**：
1. **解析失败不覆盖**：库里 JSON 不可读时 WARN + 返回 error，旧值原样保留（它是跨副本唯一记录）。
2. **写失败也不丢**：`SetSetting` 报错 WARN + 返回 error，库内旧值不动。
3. **本副本顺带收敛**：写成功后 `calibrator.Restore(merged)`，新副本不必等重启即可拿到对端样本；
   `Restore` 只在本地样本更少时覆盖，样本数单调不减，不会震荡。
本副本无样本时只 adopt、不写库（避免用空快照覆盖对端已学到的模型）。

### C-05 `Ratio()` 无消费者
`Observe` 现在经 `Ratio(modelName)` 取 `(ratio, samples)` 再发布；`SetEstimateBias` 原本已带
`samples` 参数但只导出 ppm，本次补 `EstimateSamplesByModel()` 并导出
`airoute_billing_estimate_samples{model=...}`（外加无标签总数）。
选"加指标"而非"删 `Ratio()`"：`Adjust` 在样本 < 5 时静默不生效，样本数是唯一能区分
"校准已生效 / 样本还不够"的可观测信号。

### C-06 `disallow_temperature` 只有 OpenAI 消费
把 openai.go 那段判断抽成 `checkDisallowedTemperature`（strict 报错 / lenient 丢弃参数 + warning），
anthropic（`req.Temperature`）与 gemini（`generationConfig.temperature`）各接一次。
现网 `configs/config.yaml` 只有 `o1*/o3*` 带该字段，默认行为不变；给 anthropic/gemini 规则写该字段
时才真正生效（正是审计指的"配置面与执行面不一致"）。

### C-10 分时优惠非法折扣两套口径
- **槽位侧**：`discount <= 0 || >= 1.0` 钳到 **1.0**（不再回落行级 `ctx.Discount`），并用
  `telemetry.WarnLogger`（key 含 model/slot/时间窗，60s 节流）记 WARN。这是给校验上线前
  已落库的老行的兜底。
- **校验侧**：补 `Discount ∈ (0,1)`、`Days ∈ 1..7`、时间格式、起止相同校验。
  原来 `len(slots) <= 1` 直接 return，单槽位字段完全不校验，现已修正。
- **模式门**：`ValidateOffPeakSlots(mode, slotsJSON)` 只在 `mode=="custom"` 时校验槽位，其它 mode
  **跳过**（引擎只给 `custom` 注册了读 slots 的策略）。F 域已把两处写库调用
  （`pricing_service.go:199`、`route_service.go:227`）切到该函数。
- **无兼容壳**：最初保留的 mode-less `ValidateSlotsOverlap` 包装层在 F 切换完成后成为死符号
  （生产 0 调用、只有测试调用），已删除；billing 自身测试全部改走
  `ValidateOffPeakSlots("custom", …)`。这样"校验对象 = 执行对象"在类型层面只有一个入口，
  调用方无法再漏传 mode。

### C-11 定价通配符只认尾部 `*`
选方案 (a)：统一语义。`MatchModelPattern` 支持 `精确 / x* / *x / x*y / *`，`*` 可跨 `/`
（模型名带 provider 前缀，如 `openai/o1`）。`ReloadPrices` 把所有含 `*` 的 pattern 放进索引，
按"字面字符数降序、星号数升序、字典序"排序，最具体的规则稳定胜出（沿用原注释的承诺）。
删除了原先只认 `grp+":*"` / `"*"` 的分支 4——索引已覆盖 `*`，两处语义重复。

### C-12 Redis RPM 与本地限流语义不等价 → **已由 A 域统一**
两套限流现在**同为严格 60s 滑动窗口**，语义等价：
- Redis 侧（本包）：`slidingWindowLua` 把准入时间戳写进 ZSET，先 `ZREMRANGEBYSCORE` 逐出
  `now-window` 及更早、再 `ZCARD` 计数、计数未满才 `ZADD`，窗口 `(now-window, now]`。
- 本地侧（`internal/middleware/ratelimit.go`）：A 域已把原来的"满桶起步令牌桶"换成同一窗口
  `rpmWindowSpan = time.Minute`（先逐出→再计数→再登记），不再是"空闲 key 可瞬时打满一份额度"。

因此 Redis 不可用（未配置 / 报错 / 超过 `redisRateLimitTimeout`）时，只失去**跨副本视图**：
每个节点退化为自己的窗口，对本站流量同样严格，**额度不被放松**；唯一差异是同一 key 分散到
多副本时，故障期间可能每个副本各放行 `rpm` 次而非集群合计 `rpm` 次，Redis 恢复后计数重新收敛。
`redis_quota.go` 文件头注释已按此改写，且与 `middleware` 的 `rpmWindowSpan` 注释互指
（两个常量必须同宽，改一处要改另一处）。

本域为此做的三件事：
1. 文件头注释从"两套语义不等价、故障期更宽松"改为"同为滑动窗口、只失去跨副本视图"。
2. 窗口宽度提取为 `rpmWindowMs` 并由 `ARGV[3]` 传入 Lua（原来 `60000` 硬编码在脚本里，
   Go 侧与 Lua 侧会漂移）。
3. 窗口语义测试：断言窗口 = 1 分钟、脚本"先逐出→再计数→再登记"顺序、窗口必须来自 ARGV
   而非字面量；另测无 Redis / 未启用 / `rpm<=0` 时 fail-open 语义。

## 3. 验证结果

命令前先 `export PATH=$PATH:/usr/local/go/bin`。

| 检查 | 结果 |
|---|---|
| `gofmt -l`（本域全部文件 + 授权改的 4 个测试文件） | **空** ✅ |
| `go build ./...`（全仓） | **通过** ✅ |
| `go vet ./...`（全仓） | **通过** ✅ |
| `go test ./internal/billing/... ./internal/validator/... ./internal/telemetry/... ./internal/distributed/...` | **4/4 `ok`** ✅ |
| `go test ./tests/...`（整包，`-count=1`） | **`ok`** ✅ |
| `go test ./tests/ -run 'TestBillingEngine_CostCalculationAndCacheSavings\|TestModelGroupPricing\|TestModelRoute_MultiUpstreamConfiguration\|TestTimeOfUseBilling'` | **全部 PASS** ✅（C-01 改写的 4 条断言值不变） |
| `deadcode ./cmd/airoute`（v0.52.0，本域相关输出） | 5 个 `CalculateCost*` **已消失** ✅；本域新增符号全部可达 ✅；`ValidateSlotsOverlap` **0 命中** ✅ |
| 全仓 `grep -rn "ValidateSlotsOverlap"` | 生产/测试代码 **0 命中**（仅本文件的历史叙述提到它）✅ |
| `grep -rn '\.CalculateCostDetailed\|\.CalculateCostForUnits(\|\.CalculateCostWithGroup\|\.CalculateCost('` | **0 命中** ✅ |

验收命令合集（最终一次运行，全绿）：

```
gofmt -l internal/billing internal/validator internal/telemetry internal/distributed \
        internal/controlplane/functional_details_test.go tests   -> 空
go build ./...                                                    -> 通过
go vet ./...                                                      -> 通过
go test ./internal/billing/... ./internal/validator/... \
        ./internal/telemetry/... ./internal/distributed/... ./tests/... -count=1
  ok  internal/billing      0.562s
  ok  internal/validator    0.006s
  ok  internal/telemetry    0.153s
  ok  internal/distributed  0.006s
  ok  tests                 3.613s
```

> 备注：验证过程中一度观察到 `tests/TestAdminAuth_LoginAndProtectedRoutes` 失败
> （D-07 删除重复的 `/api/v1/user/me` 后该测试仍请求旧路径）。该问题属 **D 域**，期间由 D 侧
> 修好（`tests/admin_auth_test.go` 改请求 `/api/v1/auth/me`）；本域未越界修改，最终整包 `ok`。

本域在收尾（deadcode 复查）后仍存在的符号归属说明：
- `internal/billing/estimate.go:83 EstimateCostUnits`：**归属并行结算/配额会话（WIP），不是本域改动**，
  仅测试调用。按 Lead 指示**不动**，由该会话决定删除或接入准入网关。
- `telemetry.Metrics.RecordRequest`：曾被 B-04/C-08 去掉 api 侧重复计数后判死，最新一轮
  `deadcode` 已不再报出（B 侧已处理）。
- `telemetry.ResetWarnGate` 是审计 README §3.5 已判定的"声明保留测试钩子"，保留。

## 4. handoff

1. **C-01**：5 个变体已删除，`deadcode` 中已消失。`tests/audit_remediation_test.go` 的 2 处调用点
   由任务 A（`fix-config-security`）改为 `CalculateCostForUnitsWithCacheCreation`，已复核该文件
   当前无旧调用。后续不要再加"分组/默认组"包装层——这正是本次删除的对象；需要便捷入口请用
   `GetPriceWithGroup` / `ResolvePriceSource` + 唯一入口。
2. **C-12（已完成，无需接手）**：A 域已把 `internal/middleware/ratelimit.go` 的本地限流改为与
   Redis 同步长的 60s 滑动窗口，两套实现语义一致；本域文件头注释已同步改写。
   **维护约定**：`rpmWindowMs`（distributed）与 `rpmWindowSpan`（middleware）必须同宽，
   改一处要同时改另一处（两边注释已互指）。
   仍建议在 `docs/CONFIGURATION.md:318` 补一句：Redis 不可用时只失去跨副本视图，
   多副本场景下同一 key 可能每副本各放行 `rpm` 次。
3. **C-10（已完成，无需接手）**：F 域已把 `pricing_service.go:199`、`route_service.go:227`
   切到 `billing.ValidateOffPeakSlots(mode, slots)`，"校验对象 = 执行对象"已对齐；mode-less 的
   `ValidateSlotsOverlap` 已随之删除（生产与测试均无引用），`deadcode` 复查 0 命中。
   注意：`tests/time_of_use_billing_test.go` 里 `"end":"24:00"` 的槽位**若走保存服务**会被拒
   （`24:00` 解析为 1440，越界）；该测试当前直接 `repo.SaveModelPrice`、不经校验，故不受影响。
4. **C-11 → F 域**：方案 (a) 之后任何含 `*` 的 pattern 都能匹配，`pricing_service.go` 的保存校验
   **不需要**再加 `ValidatePriceModelPattern`（原 (b) 方案作废）。
   仍存一处重复：`internal/config/validation_rules.go:46-56` 内联了同一套通配符语义（不支持 `x*y`），
   该文件不在任何修复域写权限内。建议后续替换为 `billing.MatchModelPattern`，或把该函数下沉到
   `internal/model`。
5. **C-05**：新增指标 `airoute_billing_estimate_samples{model=...}`（gauge）+ 无标签总数。
   若监控侧有"指标清单/数量"断言或 Grafana 面板清单，需要同步登记。
6. **顺手发现（未改，需归属方处理）**：
   - `internal/telemetry/metrics.go:233 Metrics.RecordRequest` 已无生产调用方（B-04/C-08 去掉
     api 侧重复计数后只剩测试）——建议 B 决定"重接到 router 唯一一层"或删除。
   - `internal/billing/estimate.go:83 EstimateCostUnits` 仅测试调用，**归属并行结算/配额会话（WIP）**，
     非本域改动；本域未动，建议该会话决定删除或接入准入网关。
   - `tests/admin_auth_test.go:80` 请求已删除的 `/api/v1/user/me`（D-07），应改为
     `/api/v1/auth/me`——归 D 域。
