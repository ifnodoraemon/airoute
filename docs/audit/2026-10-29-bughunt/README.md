# 第 3 轮：真实缺陷排查与代码瘦身（2026-10-29）

> **本轮口径**：继续「找真实 bug + 删代码」。与上一轮的分工：上一轮扫的是**请求主链路 + 核心运行时 + 计费/存储 + 前端**；本轮去扫上一轮没深挖的地方——**管理端 CRUD 与权限边界、router/适配器并发、异步任务与存储维护路径、前端深层**，并闭环上一轮留下的 8 条待确认项。
>
> **方法**：4 个域并行排查 + 1 个瘦身盘点；每条发现必须有可复现探针；Lead 独立复核（P1 全部亲验，另有 2 条由 Lead 自查发现后交域内复核）；修复分域并行，最后由 Lead 在冻结树上跑全量门禁。
>
> **基线**：`2f43d28`。**上线前复核**：见 [00-live-verification.md](00-live-verification.md)（上一轮修复在真实流量下 17 小时的实测，无回归）。
> **Lead 自查**：[00-lead-findings.md](00-lead-findings.md)（LEAD-01，后并入 F-01）。

---

## 0. 结论

**没有 P0。** 找到 **4 条 P1 + 19 条 P2 + 5 条 P3**（共 28 条），其中 **25 条已修**（含 1 条可选），3 条登记未修（consumer 回收、迁移 deferred 策略、Playground 截断口径需产品决策）。

4 条 P1 的共同根因仍然是本仓反复出现的**「同一规则两处实现，只落了一处」**：

| ID | 一句话 | 后果 |
|---|---|---|
| **CP-01** | 自助租户可一行 `PATCH /admin/keys/:id` 复活**管理员停用**的 key（创建路径有角色 clamp，更新路径没有；数据面只认 `status=="active"`） | 吊销形同虚设，被盗 key 可被漏洞方自行恢复 |
| **CP-02** | 用户名唯一性**大小写敏感**，而登录锁桶按**小写**归一 ⇒ 注册 `Admin` 与 `admin` 并存，登录自己的大小写变体即可清零受害者的失败计数 | 5 次锁定失效；直连部署（无 per-IP 桶）下 **无上限定向爆破** |
| **F-01**（LEAD-01） | `ParseMessageContent` 不认 typed `[]ContentPart`（协议适配器正是这样构造的）⇒ 走 JSON 回退把 base64 当文本 | 图片按文本计费 **~100×**（实测 76,829 vs 771 tokens）、喂坏校准器、跨协议图片被当文本发给上游 |
| **JOBS-01** | Redis 消费组只启动建一次且错误被丢；组丢失后永久 `NOGROUP`，读错误裸 sleep 无日志无指标 | 审计入库**静默永久停摆**、stream 不 trim；defer 的结算钱不入账，只能重启 |

---

## 1. 发现与修复总表

### 1.1 P1（4，全部已修）

| ID | 域 | 位置（修复前） | 报告 |
|---|---|---|---|
| CP-01 | controlplane | `key_update.go:102-104` vs `key_creation.go:157-160` | [01-controlplane.md](01-controlplane.md) |
| CP-02（=上轮 C-06 升级） | controlplane/storage | `repo_users.go:47-48`、`login_guard.go:262-268,368-378` | [01-controlplane.md](01-controlplane.md) |
| F-01（=LEAD-01） | model/adapter/provider | `model/multimodal.go:38`、`adapter/anthropic.go:132`、`provider/anthropic_convert.go:84` | [02-router-protocol.md](02-router-protocol.md) + [00-lead-findings.md](00-lead-findings.md) |
| JOBS-01 | storage/distributed | `async_logger.go:291,423-427` | [03-jobs-storage.md](03-jobs-storage.md) |

### 1.2 P2（19，全部已修）

| ID | 一句话 |
|---|---|
| CP-03 | 批量状态接口把非法 status 静默改成 `active`（想停用却启用，fail-open） |
| CP-04 | 供应商成本 CSV 导出未中和公式注入，而控制台下载的正是这份 |
| CP-05（=上轮 C-05 翻转） | 登录/注册/登出无 CSRF 防护（`SameSite=Lax` 只管「带 cookie」不管「收 Set-Cookie」），`text/plain` 表单可直投 JSON |
| F-02 | Responses `reasoning_tokens` 恒 0（值其实在 `Usage.Extra` 里被丢弃） |
| F-03 | `gemini_embed` 对 `models/` 前缀模型拼出双前缀 URL（body 却已归一化） |
| F-04 | `tool_choice` 跨协议全丢（Anthropic 有字段不填、Gemini 无 `toolConfig`）⇒ 强制工具调用静默失效 |
| F-05 | Anthropic **流式**丢 `cache_creation_input_tokens` ⇒ 缓存写按 1× 而非 1.25× 计费，系统性少收 |
| F-06 | 会话亲和是 `%N` 非一致哈希：加 1 个渠道重映射 **74.8%** 会话，上游 prompt cache 大面积失效 |
| F-07 | URL 图片双向损坏：入站 Anthropic `source.type=url` 丢弃；出站 https 图片被塞进 base64 字段 → 上游 400 |
| JOBS-02 | 毒丸消息被 ACK 后静默丢弃，无计数无日志 ⇒ 可计费审计行无声消失 |
| JOBS-03 | spill 回放只在启动时一次；运行期落盘记录永不回放，涨满 256MB 后直接丢（账单丢失） |
| JOBS-04 | `TraceCapture.Stop()` 后 `Record` 仍返回 true 但 worker 已退出 ⇒ 快照永久丢且调用方无法区分 |
| H01（上轮 T-1） | 日志批量选择跨页/跨筛选残留，删除作用于看不见的行 |
| H02（上轮 T-2） | 钱包页一次回标签页发 2 个请求 + 无竞态保护，旧响应覆盖新余额 |
| H03（上轮 T-3） | 全站无 ErrorBoundary：任一渲染异常或 chunk 404 即整页白屏 |
| H05 | AI Copilot 集群巡检读错 4 个字段（`breaker_status` 大小写、不存在的 `cache_hit_rate`、CNY 标 USD、无样本编造 120ms）⇒ 熔断渠道仍报「全部健康」 |
| H06 | 同文件 SPOF 审计读 `provider.channel`（实际 `channel_name`）⇒ 主通道恒 `default` |
| H07 | 日志详情计费公式漏 prompt-cache 写入溢价，Token 明细把写入量算作「未缓存输入」⇒ 公式加不出账面金额 |
| H08 | 8s 轮询无请求序号 ⇒ 旧轮询覆盖写操作后的刷新，新建行消失 8 秒 |

### 1.3 P3（5：4 条已修 + 1 条登记）

已修：H04（验证码倒计时 interval 卸载不清理，原假设「双击产生两个 interval」已证伪并降级）、JOBS-06（账本保留并发冲突让整趟静默 break）、JOBS-07（清理失败静默停 1 小时）、T-5（关闭 Copilot 面板不中止生成，浪费额度）。
登记未修：**JOBS-05**（Redis consumer 条目永不回收，线上实测 136 个、最老 idle≈3 天——只影响运维视野）、**JOBS-08**（迁移 deferred 后仍继续后续版本；当前 v43/v44/v45 在末位无实害）。

### 1.4 代码瘦身

`05-deadcode2.md`：**净减 −33 行**（8 文件，+33/−66）。两种「重复实现」合并（owner 判定逐字节相同、rerank 估算与 model 完全相同）+ 3 个只写不读字段/状态删除 + 2 处残留清理。死代码面仍为 0（deadcode 主入口只剩 4 个既定测试钩子）；53 个超限文件只给拆分建议，本轮不拆。

---

## 2. 逐域报告

| 文件 | 域 |
|---|---|
| [00-live-verification.md](00-live-verification.md) | 上线后 17h 实测复核（金额/结算/配额一致性） |
| [00-lead-findings.md](00-lead-findings.md) | Lead 自查：F-01 的完整影响链（含校准器边界的修正） |
| [01-controlplane.md](01-controlplane.md) | 管理端 CRUD 与权限边界（2 P1 + 3 P2） |
| [02-router-protocol.md](02-router-protocol.md) | router 并发与协议适配（1 P1 + 6 P2） |
| [03-jobs-storage.md](03-jobs-storage.md) | 异步任务与存储维护（1 P1 + 3 P2 + 4 P3） |
| [04-web.md](04-web.md) | 前端深层（上轮 4 条闭环 + 4 条新发现） |
| [05-deadcode2.md](05-deadcode2.md) | 瘦身 phase 2 |

---

## 3. 提交映射

| 提交 | 内容 |
|---|---|
| `be8643a` | `fix(security,controlplane): make key revocation, account identity and the auth endpoints unambiguous`（CP-01…05 + 迁移 v44/v45 + CSRF 守卫） |
| `cee2864` | `fix(protocol,router): price multimodal prompts correctly and carry the fields clients rely on`（F-01…07） |
| `63e0f95` | `fix(storage): make the audit pipeline recover, report and replay its failures`（JOBS-01…04/06/07） |
| `9b210af` | `fix(web): keep console state consistent under paging, polling and failure`（H01…H08 + T-5） |
| `5d57ed7` | `refactor: drop dead state and the duplicated ownership helper`（瘦身 −33） |
| 本文件所在提交 | `docs(audit): …` |

---

## 4. 行为变化与升级注意（重要）

### 4.1 数据库迁移（启动时自动执行）

1. **v44 `uq_users_username_lower`**：用户名大小写不敏感唯一。存量若存在「仅大小写不同」的账号，**不会自动改名/合并**——日志会列出冲突用户名，迁移 deferred（网关照常启动、每次重启重试），运维改名/删号后重启即通过。SQLite 的 `LOWER()` 仅 ASCII，非 ASCII 用户名需业务层另校验。
2. **v45 `api_keys.admin_disabled`**（默认 `false`）：区分「管理员停用」与「用户自停」。**存量被停用的 key 升级后一律视为「用户自愿停用」**——历史上由运维手工停用、且需要防止用户自行复活的 key，升级后需管理员再停用一次以打标记。

### 4.2 鉴权与身份

3. **租户不能复活管理员停用的 key**：自助 `PUT /admin/keys/:id` 置 `active` 会被拒（`KeyUpdateAdminRevoked`）；自助批量启用会在 SQL 层跳过被管理员停用的行。管理员启用会清除标记。
4. **账号身份大小写不敏感**：注册 `Admin`（已存在 `admin`）会被拒（400/409）；登录按不区分大小写命中。
5. **未鉴权 auth 端点要求 JSON + 同源**：`/api/v1/auth/{login,register,logout}` 非 `application/json` 返回 415；带 `Origin`/`Referer` 时必须同源，否则 403。**CLI/脚本（不带 Origin、发 JSON）不受影响；用表单编码打这些接口的第三方脚本会失败。**
6. **非法状态值不再静默变 active**：批量/单条状态接口对未知 status 返回 400。
7. **CSV 导出中和公式注入**：以 `= + - @ \t \r`（含前导空格）开头的单元格前缀 `'`。

### 4.3 协议与计费

8. **多模态 prompt 估算修正**（F-01）：Anthropic/Gemini 入口的带图请求在「上游无 usage / 499」路径估算从 ~100× 高估降为正常量级（实测 76,829 → 771 tokens）→ **这些场景的计费会下降**；同时不再污染该模型的估算校准比例（校准有 0.4 下限，最坏影响 ≤2.5×）。
9. **跨协议图片修复**：入站 Anthropic `source.type=url` 与出站 https 图片现在按 URL 传递（此前被丢弃 / 被塞进 base64 字段报 400）；Gemini fileData 同理。
10. **Anthropic 流式缓存写按 1.25× 计费**（F-05）：此处**账单会上升**（此前少收）。
11. **`tool_choice` 生效**（F-04）：强制工具调用现在真正落到上游；此前静默变成普通补全。
12. **Responses `reasoning_tokens` 透出真值**（F-02，此前恒 0）；一元/流式 usage 形状一致。
13. **gemini URL 单前缀**（F-03）：`models/` 前缀模型不再拼出双前缀（此前上游 400）。
14. **会话亲和改为 rendezvous 哈希**（F-06）：渠道增减时重映射从 74.8% 降到 ~1/N（实测 26.2%）；升级瞬间会有一批会话迁移一次，之后显著更稳、上游 prompt cache 命中更好。

### 4.4 审计管道

15. **Redis 消费组自愈**（JOBS-01）：组丢失后自动重建；新增指标 `airoute_usage_log_stream_read_failures_total` / `..._group_recreated_total`；启动日志打印 group health（pending/consumers/lag）。
16. **毒丸进入死信流**（JOBS-02）：不可解析条目写 `airoute:stream:dead`（MAXLEN 1000，payload 截 4KB）并计数 `airoute_usage_log_stream_poisoned_total`，然后 ACK。
17. **spill 周期回放**（JOBS-03）：默认 60s（`GATEWAY_USAGE_SPILL_REPLAY_SEC` 可调）+ 落盘后立即触发；失败保留文件，单飞防并发。
18. **TraceCapture 停止后 `Record` 返回 false**（JOBS-04）：调用方不再误以为快照已归档。

### 4.5 前端

19. 日志批量选择随查询失效；钱包刷新去重 + 竞态保护；**新增全局/按视图 ErrorBoundary**（白屏变成可重试的兜底页）；验证码倒计时随卸载清理；Copilot 集群/SPOF 报告读对字段并显示 CNY/无样本；日志详情计费公式补缓存写项；8s 轮询加序号；关闭 Copilot 中止生成。

---

## 5. 验证证据（冻结树上实测）

```
gofmt -l internal cmd tests web        → 空
go build ./...                         → OK
go vet ./...                           → OK
go test -race -count=1 ./...           → 16/16 包 ok（含 tests/ 集成）
cd web && npm run lint                 → 0 warning
cd web && npm test                     → 65 文件 / 710 用例全过（基线 56/685）
helm lint helm/airoute                 → 0 failed
deadcode ./cmd/airoute                 → 仅 4 个既定测试钩子
```

域内额外验证（各自报告有完整命令）：

- **真 PostgreSQL**（临时 `postgres:16-alpine`，用后删除，未触碰线上库）：`-tags integration` 迁移套件 PASS，`schema_migrations` 记录 v44/v45，`uq_users_username_lower` 与 `api_keys.admin_disabled` 均建立；PG 上大小写变体被拒、布尔列全链路读写、自助/管理员批量启用分野全 PASS。
- **真 Redis**（临时 `redis:7-alpine`）：删组后 6s 内自愈入库、毒丸进 DLQ 且有界、PEL 已 ACK、spill 回放。
- **前端**：7 个旧探针全部翻转为正式回归并删除；新增 9 个回归文件。

> 一处测试健壮性修正：`TestUsageSpillReplayPicksUpAnotherProcessFile` 的等待窗口从 5s 放宽到 20s（并行负载 + `-race` 下会抖），不影响断言语义。

---

## 6. 未闭环与遗留

| 项 | 说明 |
|---|---|
| JOBS-05 | Redis consumer 条目永不回收（`XGROUP DELCONSUMER`），线上 136 个、最老 idle≈3 天；只影响运维视野，未修 |
| JOBS-08 | 迁移 deferred 后仍继续注册/执行后续版本；当前版本号在末位无实害，未改策略 |
| T-6 | Playground 忽略 `finish_reason`，`max_tokens` 截断与正常结束无区别（需产品口径） |
| 迁移 v44 冲突路径 | 无自动化测试（构造「已记录 v1–v43 且有重复账号」的库成本高于收益），人工核对 + PG 手工验证 |
| 前端未接 `admin_disabled` | 后端 view 已暴露，控制台仍只展示 `status`；本轮按约定不动 UI |
| 53 个超限文件 | 违反 300 行红线，已给拆分目标与行数估算，未拆 |
| 后端 `total_cost` 字段名 | 前端已按 CNY 展示，字段名 `total_cost` 保持兼容未改 |

---

## 7. 方法学备注（给下一轮省时间）

1. **「上一轮的修复也可能只修了一半」**：F-01 正是上一轮 `BUG-MULTIMODAL-TOKEN-ESTIMATE` 只覆盖 `[]any` 表示、漏了协议适配器构造的 typed `[]ContentPart`。修一个「表示/形状」类缺陷时，要把**所有产生该值的构造点**列出来逐一覆盖。
2. **线上只读证据比推理可靠**：本轮先用 17h 的 PG/Redis/Prometheus 数据把「无泄漏、无丢单、零漂移」钉死，再开始找新问题；`billing_reserved_units=0` 与 `airoute:inflight:*` 键为 0 直接验证了上一轮心跳改造的配对。
3. **栈溢出/锁/并发类结论要用 `-race` 或真服务验证**：本轮 `TestUsageSpillReplayPicksUpAnotherProcessFile` 的抖动就是并行 `-race` 下调度延迟，已把窗口放宽。
4. **不要用字面量 grep 判定配置/字段死否**：`GITHUB_CLIENT_SECRET` 是 `fmt.Sprintf` 动态拼的（上一轮差点误删）；同理 `usageLogPartition.from` 这类「只写不读」要 AST + 消费方双向确认。
