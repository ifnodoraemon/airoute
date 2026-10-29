# 修复后一致性抽查（2026-10-29）

> **触发**：在 `2c...`（第 3 轮）之后的又一次「前后不一致」抽查，视角仍是**「设计了但别处没用 / 文档承诺与代码不符」**，不是新一轮 bug hunt。
> **基线**：`14c97e1`（第 3 轮最后一个提交）。**范围**：跨轮交付物之间的一致性 + 我这轮第 2 批修复与后续 15 个提交的衔接。
> **方法**：机械对账（文档↔代码↔部署清单↔导出指标）+ 关键修复的独立代码复核。本轮不新增 bug 排查。

## 0. 结论

**发现 5 处不一致，全部已修**（4 处文档/口径、1 处隐式约束未写明）。**没有发现运行时缺陷**；被抽验的 4 条 P1 修复与 3 个「登记未修」项均与文档描述一致。

## 1. 已修（本轮）

| # | 不一致 | 处理 |
|---|---|---|
| S-1 | `GATEWAY_USAGE_SPILL_REPLAY_SEC` 被代码读取（60s 默认，控制 spill 重放周期），但 `.env.example` 与 `docs/CONFIGURATION.md` **都没有** | 两处补齐，说明「数据库恢复后不必等重启」的语义 |
| S-2 | `GATEWAY_ENABLE_TABLE_PARTITIONING` 只在 CONFIGURATION.md 有专章，`.env.example` 模板里没有（同类开关都在模板里） | 模板补注释条目（默认关闭，注明 ACCESS EXCLUSIVE + 全表拷贝，指向 CONFIGURATION.md 的分区章节） |
| S-3 | 我上轮审计 README §8.2 第 1 条仍说「Compose 出厂默认信任 edge 网段」，而 `2f43d28` 已改为只信任代理自身地址（`172.30.0.10/32`）；照旧文档配置会让宿主机自访问控制台 403 | README 该条加更正说明并指向第 3 轮 DEPLOY-01；同时把 §8.3 的「5 个遗留死符号」标注为已被 `d15e6b4` 清除 |
| S-4 | `docs/ROADMAP.md` 质量行写「729 Go 测试函数 / 222 测试文件 / 597 前端用例」，实测 1159 / 346 / 666 | 更新为实测值 |
| S-5 | 迁移 runner 对 `errMigrationDeferred` 是 `continue`（后续版本照常应用），但**「迁移不得依赖更早版本已应用」这条隐式约束没有写下来**；第 3 轮把「deferred 后仍继续」登记为未修项 | 在 `migrations_runner.go` 该分支补注释：为什么必须 continue（升级不能把网关留在 down 状态、v44/v45 携带安全修复），以及由此产生的约束与当前 v43 无依赖的事实 |

## 2. 已验证为「一致」（无需改动）

| 检查 | 方法 | 结果 |
|---|---|---|
| 文档 → 代码：CONFIGURATION.md 表格里的开关 | 提取 71 个 `GATEWAY_*` 表格项，逐个 grep 代码读取点 | **71/71 全部被读取** |
| 代码 → 清单：代码读取但模板未列 | 提取 Go 里所有大写 env 字面量 vs `.env.example` | 仅剩 `AIROUTE_TEST_*`（测试专用）、`AIROUTE_PARTITION_DEMO_DSN`（演示）、`GATEWAY_DB`（旧别名，文档已注明） |
| 告警规则 → 导出指标 | `airoute-alerts.yml` 里 26 个指标名 vs `metrics_prometheus.go` 实际写出的 58 个 | **26/26 可解析**（3 个是直方图派生序列 `_bucket/_sum/_count`，基名存在） |
| 迁移注册表两方言一致性 | 逐版本号比对 PG 与 SQLite 的 `Version+Name` | 共 44 个共同版本**全部同名同义**；SQLite 缺 v43 是分区迁移（PG 专属、条件注册、注释写明），与既有的 30/35/36/38 空洞同模式 |
| 死代码 | `deadcode ./...`（main 入口） | 仅 4 条已声明测试钩子（`authenticateCaller`/`forwardJSONRPCToolCall`/`ResetReadinessChecks`/`ResetWarnGate`）；上轮遗留的 5 个死符号已被 `d15e6b4` 清除 |
| 新增 TODO/FIXME | 全仓 grep | 无新增（命中的都是既有的中文错误文案） |
| `AutoClaimLogs`/`ReadPendingPEL` 是否只定义不接线 | 找生产调用点 | **已接线**：`async_logger.go:569/620` 在消费循环里分别做 PEL 重读与 30s minIdle 的 XAUTOCLAIM，崩溃消费者的 pending 消息会被认领（无「静默丢审计行」风险） |

## 3. 独立复核的既有修复（代码级）

- **CP-01（管理员停用态）**：`api_keys.admin_disabled` 列存在，`repo_keys.go` 的注释明确「管理员停用与租户自停是两种状态，租户不得复活管理员停用的 key」，读取/插入/更新全部带该列。
- **F-01（typed content）**：`ParseMessageContent` 已分支处理 `[]ContentPart`/`ContentPart`/`*ContentPart`，注释写明了「JSON 回退会把 data: URI 图片当文本，导致 ~100× 高估与出站当散文发送」的原因。
- **JOBS-05（consumer 条目不回收）**：确认全仓无 `DELCONSUMER`，且 `StreamGroupStats` 注释直言「每次进程重启都会新增，无回收」——与第 3 轮登记一致，属已知运维视野问题，非静默故障。
- **JOBS-08（deferred 后继续）**：见 S-5，现已在代码里写明约束。

## 4. 建议（未做，需产品/运维决策）

1. **Redis consumer 回收**（JOBS-05）：把 consumer 名从「每进程唯一」改为「每副本稳定名」（如 node id），可把条目数从「每次重启 +1」压到「每副本 1 条」；若要彻底回收，再加一个 idle > 24h 的 `XGROUP DELCONSUMER` 清扫。注意 consumer 名变更后旧条目的 PEL 需要一次 XAUTOCLAIM 兜底（现有循环已具备）。
2. **SQLite 补 v43 no-op**：让两方言的版本列表形状一致（当前靠「注册表按方言独立」豁免，无实害但读代码的人会疑惑）。
3. **`dispatcher_modality.go` 473 行**：仍超 300 行指引，属行为无关拆分，建议单独立项。

## 5. 验证

```
gofmt -l internal cmd tests        → 空
go build ./...                     → OK
go vet ./...                       → OK
go test ./...                      → 16/16 包 ok
go run deadcode@v0.52.0 ./...      → 4 条（均为已声明测试钩子）
```
