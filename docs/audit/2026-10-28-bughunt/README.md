# 真实缺陷排查与代码瘦身（2026-10-28 bughunt）

> **口径**：上一轮 [内部一致性审计](../2026-10-28-inconsistencies/README.md) 的目的很明确——「设计意图与实现/使用之间的断裂」，并**显式不查普通 bug**。本轮补的正是那块：**真实运行期缺陷**（错账、丢功能、崩溃、竞态、鉴权/配额漏洞），外加用户要求的**删减代码**（死代码/重复实现/无用资产）。
>
> **方法**：5 个 teammate 并行（请求/协议、核心运行时/鉴权、存储/计费/遥测、前端、死代码盘点），每条发现都要求**可复现证据**（包内探针测试或精确复现命令）；Lead 独立复核全部 P1 与关键 P2（读代码 + 亲跑探针），再分域派修复；修复完成后由 Lead 在**冻结的树上**跑全量质量门。
>
> **基线**：`381926d`（上一轮一致性审计收尾）。**工作树**：本轮全部修复已按域提交（见 §3）。

---

## 0. 一句话结论

主链路没有 P0。找到 **6 条 P1 + 17 条 P2**（共 23 条 P 级发现），另有 **4 条跨域加固**与 **1 条顺手修**，全部修复并翻转为常规回归测试；死代码面**归零**，死代码/重复实现**净删 −161 行**（死符号 −119、重复管线 −42），文档口径修正 5 处。

> 行数口径：本轮**不是**整仓净减——修复本身新增了防御性代码与注释（生产代码 +1228），测试与文档 +5531（20 个新回归测试文件 + 5 份审计报告）。「净删 −161」只统计被删除的死代码/重复实现。

四个 P1 的实质影响：

| 影响面 | 缺陷 | 用户可感知后果 |
|---|---|---|
| 错账（多扣） | 同一结算批次内重复 `billing_id` 被扣两次（ledger 只记一次） | 客户钱包被多扣、`used_cost_units` 虚高、对账漂移 |
| 错账（少算） | Gemini thinking 模型的 `thoughtsTokenCount` 从未计入 completion | reasoning 输出少计最多 ~90%，系统性少收 |
| 错账（多估） | 内联 base64 图片按文本估算 prompt token（1 张图 23 万 token） | 「上游无 usage / 客户端取消」路径按估算真扣费，高估 300× |
| 丢功能 | Gemini 协议**流式**工具调用被拆成多个空壳 `functionCall` | 工具调用参数永远拼不齐，流式 agent 实际不可用 |
| 鉴权/配额 | 登录成功清零「per-IP 喷洒桶」；in-flight 预授权 120s 过期早于请求生命周期 | 单账号即可无限重置 IP 喷洒预算；长请求期间余额门失效、跨请求误释放 |

---

## 1. 发现总表

P 级按「会不会错账 / 能不能绕过 / 功能是否坏掉」判定，不按代码美观程度。

### 1.1 P1（6）

| ID | 域 | 一句话 | 位置（修复前） | 报告 |
|---|---|---|---|---|
| BUG-DATA-01 | storage/结算 | 同批次重复 `billing_id`：ledger 去重、金额聚合未去重 ⇒ 钱包/计数器双扣 | `internal/storage/settlement.go:231-246` vs `:274-293` | [03-data.md](03-data.md) |
| B-01 | 配额/分布式 | in-flight 预授权 TTL=120s 短于请求生命周期（非流式 600s、流式无写 deadline）⇒ 预授权中途消失 + 跨请求释放 | `internal/distributed/redis_quota.go:42-45,116-119,145-150` | [02-core.md](02-core.md) |
| B-02 | 登录防爆破 | 登录成功清空**所有**桶（含 per-IP）⇒ 自有账号登录一次即可无限重置 IP 喷洒预算 | `internal/controlplane/login_guard.go:343-353` | [02-core.md](02-core.md) |
| BUG-GEMINI-TOOLCALL | api/协议 | Gemini 流式工具调用把每个 delta 翻成一个 `functionCall` part，参数永不拼装 | `internal/api/gemini_inbound_stream.go:206-214` | [01-request.md](01-request.md) |
| BUG-MULTIMODAL-TOKEN-ESTIMATE | model/计费 | 内联 base64 图片按字符估 token（930k 字符 ⇒ 232,529 tokens），在无 usage / 499 路径真扣费 | `internal/model/tokens.go:35-49`、`internal/api/common.go:822-836` | [01-request.md](01-request.md) |
| BUG-WEB-01 | 前端 | 普通用户永远打不开「概览/我的用量」：角色白名单漏 `dashboard`，点击/深链一律改写为钱包页 | `web/src/App.jsx:434-441` | [04-web.md](04-web.md) |

### 1.2 P2（17）

| ID | 域 | 一句话 | 报告 |
|---|---|---|---|
| BUG-DATA-02 | 计费 | 跨零点优惠时段 + `days`：后半夜星期归属错，每天 6h 该折未折、另 6h 错折 | [03-data.md](03-data.md) |
| BUG-DATA-03 | 计费 | 非法时分被静默解析成 00:00 且校验放行（`8:30pm`→08:00、`abc`→00:00） | [03-data.md](03-data.md) |
| BUG-DATA-04 | 计费 | `EstimateCalibrator.Adjust` 锁外读共享样本 × `Observe` 写入 ⇒ 热路径数据竞争（`-race` 实测） | [03-data.md](03-data.md) |
| BUG-DATA-05 | 遥测 | `estimateBiasByModel` 无基数上限（兄弟指标有），客户端可推高内存与 Prometheus 序列数 | [03-data.md](03-data.md) |
| B-03 | 审计 | 客户端 `trace_id`/`traceparent` 原样接受（无长度/字符集，仅查长度）⇒ 审计关联投毒、任意字节入库 | [02-core.md](02-core.md) |
| B-04 | 会话 | 会话 cookie `Secure` 由客户端可控 `X-Forwarded-Proto` 决定 ⇒ https 会话可被诱导签发非 Secure cookie | [02-core.md](02-core.md) |
| B-05 | 限流 | TPM 是固定窗口（注释称滑动）⇒ 边界 2× 突发；准入传 0 token，`used==limit` 仍放行 | [02-core.md](02-core.md) |
| BUG-GEMINI-THOUGHTS | 计费 | `thoughtsTokenCount` 未建模 ⇒ thinking 输出少算（示例 90.9%） | [01-request.md](01-request.md) |
| BUG-RESPONSES-INCOMPLETE | 协议 | Responses 恒 `status:"completed"`，`finish_reason` 丢失 ⇒ 截断对 agent 不可区分 | [01-request.md](01-request.md) |
| BUG-SSE-HALF-FRAME | 协议 | EOF 带数据时丢弃缓冲末行（Gemini/Anthropic）⇒ 终帧丢失、误记渠道失败 | [01-request.md](01-request.md) |
| BUG-REQ-MUTATION | provider | 浅拷贝后原地清空调用方 `ReasoningContent` ⇒ 出站消毒泄漏成请求级副作用（重试/兜底看到被改写历史） | [01-request.md](01-request.md) |
| BUG-WEB-02 | 前端 | 状态页读错熔断字段（`status==='tripped'`，真实字段 `breaker_status`）⇒ 熔断渠道显示健康 | [04-web.md](04-web.md) |
| BUG-WEB-03 | 前端 | 未探测渠道伪造「~35 ms（探针正常）」 | [04-web.md](04-web.md) |
| BUG-WEB-04 | 前端 | 链路快照搜索无竞态守卫 ⇒ 旧回包覆盖新查询、ID 与快照不匹配 | [04-web.md](04-web.md) |
| BUG-WEB-05 | 前端 | 滚动锁按实例恢复、非引用计数 ⇒ 非 LIFO 关闭后 body 永久锁死（当前 UI 未可达，潜在） | [04-web.md](04-web.md) |
| BUG-WEB-06 | 前端 | 代码块按钮/加载态硬编码中文/英文，不随界面语言 | [04-web.md](04-web.md) |
| BUG-WEB-07 | 前端 | 登出漏清 `dashboardSelectedKey`/`showAnnouncementModal`/日志游标 ⇒ 跨账号残留 | [04-web.md](04-web.md) |

### 1.3 跨域加固与顺手修（5）

| ID | 内容 | 报告 |
|---|---|---|
| C-01 | `/metrics` 仍用「私网 peer 即放行」（A-01 同类 fail-open 残留）⇒ 与 admin guard 同口径、fail-closed | [02-core.md](02-core.md) |
| C-02 | 启动播种吞读错误、无重名去重 ⇒ 瞬时 DB 故障可重复播种 | [02-core.md](02-core.md) |
| C-03 | `PublishReload` 忽略 INCR 错误、自身广播被自己消费 | [02-core.md](02-core.md) |
| C-04 | `HasSuffix("/auth/login")` 免鉴权白名单写法脆弱 ⇒ 精确路径 | [02-core.md](02-core.md) |
| §3-1 | 三处流式 usage 覆盖式赋值改 `max()`（与 chat/messages 统一） | [01-request.md](01-request.md) |

---

## 2. 逐域报告

| 文件 | 域 | 内容 |
|---|---|---|
| [01-request.md](01-request.md) | 请求/流式/协议 | 6 条发现 + 修复记录 + 3 条待确认 + 已排除项（含 2 条被证伪的假警报） |
| [02-core.md](02-core.md) | 核心运行时/鉴权/限流/并发 | 5 条发现 + 4 条跨域 + 9 条行为变化 + 真 Redis 集成验证 |
| [03-data.md](03-data.md) | 存储/计费/遥测 | 5 条发现 + 修复记录 + 行为变化（结算去重、夜间折扣语义） |
| [04-web.md](04-web.md) | 前端控制台 | 7 条发现 + 修复记录 + 4 条待确认（低置信） |
| [05-deadcode.md](05-deadcode.md) | 死代码/冗余/无用资产 | 两轮盘点与执行记录、撤回的 2 条误报、字段读写统计、超限文件清单 |

---

## 3. 提交映射

| 提交 | 内容 |
|---|---|
| `d15e6b4` | `refactor: drop five dead symbols the quota/estimate refactor left behind`（Lead 先做的死符号清理，−119 行） |
| `980ab86` | `fix(billing,storage,telemetry): charge each settlement id once and price overnight windows from their start day`（DATA-01…05） |
| `2099b13` | `fix(security,runtime): keep quota reservations alive and close four admission gaps`（B-01…05、C-01…04、nginx map） |
| `3d8728b` | `fix(protocol): assemble Gemini stream tool calls and price non-text prompts by budget`（6 条协议/计费缺陷 + 顺手修） |
| `6e1c333` | `fix(web): restore the non-admin dashboard and correct breaker, trace and i18n defects`（WEB-01…07） |
| `dfd2d0e` | `refactor: one retention purge loop, one env helper, and honest env docs`（瘦身 + 文档口径） |
| 本文件所在提交 | `docs(audit): …`（本轮报告索引与升级说明） |

---

## 4. 行为变化与升级注意（上线前必读）

按「会不会影响线上钱/可用性」排序。

### 4.1 金额与计费

1. **批内重复投递不再双扣**（BUG-DATA-01）。同一批里重复的 `billing_id` 现在与跨批次重投递同语义：只扣一次；被跳过的副本在提交成功后标记 settled。`api_keys.used_cost_units`/`used_tokens` 不再多计，`ledgerDrift` 不再因此漂移。若历史上已有此类双扣，账本（`billing_settlements`）里没有痕迹，需按 `usage_logs` 与钱包流水单独对账。
2. **分时优惠语义明确为 start-day**（BUG-DATA-02）。`days` 表示窗口**开始**那一天；`22:00-06:00 + days=[1]` = 周一 22:00 → 周二 06:00。已按旧语义配置「跨零点 + days」的规则，生效窗口会移动（每个配置夜的后半夜开始打折、配置日的前半夜不再打折）——**上线前请运营复核此类规则**。无 `days` 或非跨零点的规则不受影响。
3. **分时配置变严格**（BUG-DATA-03）。`ValidateOffPeakSlots` 拒绝 `8:30pm`/`abc`/`8:5`/`25:00` 等非 `HH:MM` 值（此前静默保存成别的窗口）。存量 DB 里的非法行不回滚，但读取侧一律按「不计折扣」处理。
4. **Gemini thinking token 计入 completion**（BUG-GEMINI-THOUGHTS）。`completion = candidates + thoughts`，`total` 自洽归一。**对 Gemini 推理模型的结算金额会上升**（此前少算 reasoning 输出）。
5. **无 usage 时的多模态 prompt 估算**（BUG-MULTIMODAL-TOKEN-ESTIMATE）。内联 base64 图片不再按字符数计，图片按 85（low detail）/765 tokens、其它二进制 part 765 tokens 预算；**远程 URL 图片也从「URL 字符数(~30)」变为 765**（同一张图不因传输方式改变计价口径）。纯文本请求估算完全不变；只影响「上游未回 usage」与「客户端取消仍计费」两条路径，以及预授权估算。

### 4.2 鉴权 / 限流 / 运维接入

6. **预授权心跳**（B-01）：Redis 模式下每个在途请求每 `TTL/3`（默认 40s）执行一次 keep-alive（两条 `PEXPIRE`），一个请求通常 0–1 次；键缺失时**不会重建**。预授权 TTL 仍是 120s（**崩溃泄漏上限不变**），但活请求不再丢预留。
7. **登录限流**（B-02）：成功登录不再清 IP 桶 ⇒ 共享出口 IP（公司 NAT、移动网络）在 15 分钟内累计 20 次失败后该 IP 会被锁 15 分钟，且不会因为其中某人登录成功而重置。若出现「整栋楼被锁」，应调大 `maxLoginFailuresPerIP` 或正确配置 `GATEWAY_TRUSTED_PROXIES` 做真实 IP 归因。
8. **trace_id 校验**（B-03）：不符合 `[A-Za-z0-9._-]{1,64}` 的客户端 trace 头、以及非严格 W3C 的 `traceparent`（大写 hex、全零 id、多余字段）不再采纳，改为生成新 id。依赖「网关原样回显我的 request id」的上游需要规范字符集。
9. **cookie Secure**（B-04）：判定改为「TLS 直连 或 受信代理声明 https」。**反代终结 TLS 的部署必须配置 `GATEWAY_TRUSTED_PROXIES` 并让代理设置 `X-Forwarded-Proto`**，否则 cookie 不带 Secure（fail-closed 到「不声称安全」）。边缘 nginx 的 map 已改为 `default $scheme` / `default $http_host`，客户端注入的 `X-Forwarded-Proto` 不再透传。
10. **TPM 语义**（B-05）：固定窗口 → 真 60s 滑动窗口；突发容量下降（不再有边界 2× 突发）；`used == limit` 直接 429（此前多放行一个）。按旧「每分钟重置」节奏压满 TPM 的客户端会看到更多 429。
11. **`/metrics` 访问收紧**（C-01）：未归因 peer（没有可信代理、也不是 loopback）现在 403。**Prometheus 从容器网络/私网直连抓取的部署需要显式配置** `GATEWAY_TRUSTED_PROXIES`、或 `GATEWAY_METRICS_TOKEN`、或 `GATEWAY_METRICS_ALLOW_PUBLIC=true`。
12. **启动播种**（C-02）：DB 读失败时记 WARN **跳过播种、不 fail-fast**（数据面仍由 10s 周期 reload 安装真实配置）；仅在数据库确实无渠道/密钥时播种；配置内同名渠道/同 key 只建第一条。
13. **集群 reload**（C-03）：自身广播不再二次 reload（每次写操作少一次全量 DB 读）；`INCR` 失败会 WARN 但广播照发。
14. **登录免鉴权白名单**（C-04）：仅精确匹配 `/api/v1/auth/login`。

### 4.3 协议

15. **Gemini 流式工具调用形状变化**（BUG-GEMINI-TOOLCALL）：不再逐帧下发 `functionCall`，改为终止帧一次性发完整调用（`arguments` 为空/不可解析时为 `{}`）。依赖「增量 functionCall」的客户端需改为等待终止帧——这正是 Gemini schema 要求的形状。
16. **Responses 截断可区分**（BUG-RESPONSES-INCOMPLETE）：`status:"incomplete"` + `incomplete_details.reason`（`max_output_tokens`/`content_filter`）；一元/流式 usage 都带 `input_tokens_details`/`output_tokens_details`。
17. **SSE 半帧**（BUG-SSE-HALF-FRAME）：无尾随换行的终帧不再丢（更准的 usage 与渠道成功判定）。
18. **流式 usage 累积**（§3-1）：多帧 usage 取 `max` 而不是覆盖（不再回退到 0）。

### 4.4 前端

19. **普通用户可进入「概览/我的用量」**（BUG-WEB-01，功能恢复）；未知 tab 回退到该角色首个允许 tab，不再硬编码钱包页。
20. **状态页**：熔断状态读 `breaker_status`（open/half_open/closed）；未探测渠道显示「未探测」而非伪造延迟。
21. **登出清理更彻底**、链路快照搜索无竞态、弹窗滚动锁引用计数、代码块按钮/加载态跟随语言。

---

## 5. 验证证据（冻结树上实测）

```
gofmt -l internal cmd tests web     → 空
go build ./...                      → OK
go vet ./...                        → OK
go test -race -count=1 ./...        → 16/16 包 ok（cmd 11.4s / api 46.7s / controlplane 108.8s / storage 75.7s / tests 54.4s）
cd web && npm run lint              → 0 warning
cd web && npm test                  → 56 文件 / 685 用例全过（基线 51/666，新增 5 文件 19 例回归）
cd web && npm run build             → 成功（产物未跟踪）
helm lint helm/airoute              → 1 chart linted, 0 failed
go run deadcode ./cmd/airoute       → 仅 4 个已声明测试钩子（forwardJSONRPCToolCall / MCPHandler.authenticateCaller / ResetReadinessChecks / telemetry.ResetWarnGate）
```

**端到端启动冒烟**（本机 SQLite 临时库 + 独立端口，避免与线上 compose 冲突）：

```
/health, /health/ready, /v1/models, /metrics, /app(301)  → 200/200/200/200/301
POST /api/v1/auth/login                                   → 200（JWT）
管理端 /channels /logs /stats/overview /keys /models /mcp/servers → 200
POST /v1/chat/completions（无渠道）                        → 404 {"error":{"code":"model_not_found",...}}
```

真 Redis 集成（`-tags integration`，docker redis:7-alpine，事后已清理）：心跳续期 5 个子用例、集群 reload/自消息过滤全 PASS。

> 备注：`npm test` 在**与 `go test -race ./...` 并行**跑时曾出现 4 个用例超时失败，串行重跑 56/56 全绿；CI 是串行执行（web 先于 Go），无影响。属测试环境 CPU 争用，不是回归。

---

## 6. 未闭环与遗留

| 项 | 说明 | 归属 |
|---|---|---|
| 登录 CSRF | `SameSite=Lax` 已缓解，未加 token；当前不可利用 | 报告 §2 |
| 用户名大小写归一化 | 跨 storage 域语义存疑，未改 | 报告 §2 |
| Responses `reasoning_tokens` 恒 0 | canonical `Usage` 无 reasoning 分量，属字段扩展 | 01-request §3 |
| `gemini_embed` 对 `models/` 前缀模型的 URL 拼装 | 低置信，未构造真实触发 | 01-request §3 |
| 前端 4 条待确认 | 日志批量选择跨页残留、钱包页双发 fetch、全站无 ErrorBoundary、验证码 interval 未清理 | 04-web §3 |
| 3 个「只写不读」非序列化字段候选 | `usageLogPartition.from`、`S3Storage.useSSL`、`EstimateCalibrator.observed`（各 2–3 行）；删字段涉及兼容，未动 | 05-deadcode §7 |
| 51 个文件 > 300 行 | 违反 `docs/ENGINEERING_GUIDELINES.md` 的文件粒度红线；拆分属行为无关大重构，本轮只列清单 | 05-deadcode §5 |
| `POST /admin/mcp/servers/:id/probe` 无前端消费者 | 但具备真实网络探测能力，属功能面而非死代码，登记不删 | 05-deadcode §2 |
| 跨包 env 小助手 4 份 | 统一需新增包、净收益小，未做 | 05-deadcode §2 |
| 4 个已声明测试钩子 | `deadcode` 仍报，但有注释说明、属刻意保留 | 05-deadcode §1 |

---

## 7. 方法学备注（给下一轮省时间）

1. **deadcode 必须喂主入口**：直接喂库包（`./internal/xxx/...`）会打印 `no main packages` 且**不做分析**（假阴性）；正确用法 `deadcode ./cmd/airoute`，`-test` 可区分「仅测试可达」。
2. **字面量 grep 看不见动态拼的变量名**：`GITHUB_CLIENT_SECRET` 差点被当死配置删掉，实际 `oauth_handler.go` 用 `fmt.Sprintf("%s_CLIENT_SECRET", …)` 读取。删配置前要按「值/前缀/拼接模式」多角度验证。
3. **两条被证伪的假警报**（写进 01-request §4，避免下轮重复）：
   - `/v1/completions` 流式断连「记 200」——实测 `prepareUsage` 会把 2xx 翻成 499；
   - 多模态缓冲替换 `resp.Stream` 「不 Close ⇒ cancel 泄漏」——`defer` 注册时求值接收者，捕获的是替换前原流，照常 Close。
4. **`format_validation` 不是死字段**：`repo_keys.go:540 ToConfig()` → `config.ResolveValidationLevel` → `validator/pipeline.go` 三入口，运行时生效；上一轮 §5 已修正过一次，本轮又差点被误报。
5. **写测试钉住 bug 时注意方向**：本轮所有探针先断言「当前错误行为」保证套件不红，修复后**翻转**为断言正确行为并改名去掉 `bugprobe`，避免留下「庆祝 bug 存在」的测试。
