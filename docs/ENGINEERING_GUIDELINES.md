# Airoute 软件工程规范与架构约束指南 (Engineering Guidelines)

> 本文档是 Airoute 项目核心开发、模块重构、代码审查（Code Review）与持续集成的**权威规范指南**。所有后续代码贡献与功能扩展均须严格遵守本文档所定义的工程哲学、架构分层、文件组织与部署约束。

---

## 一、 软件工程核心哲学与设计原则

Airoute 作为一个企业级、高并发的 AI 路由网关，系统稳定性、可维护性与代码自解释性处于第一优先级。设计必须贯彻以下工程哲学：

### 1. 单一职责原则 (Single Responsibility Principle, SRP)
- **文件与结构体级别**：每个 `.go` 文件、每个结构体必须只负责一件清晰、独立的事情。严禁出现集“请求校验、路由计算、协议转换、审计落盘、指标统计”于一身的“上帝类”或臃肿巨石文件。
- **拆分范式**：
  - 流式长连接逻辑与一元（Unary）请求必须分离（如 `handler.go` 与 `chat_stream_handler.go`）。
  - 核心调度流水线与候选发现/会话亲和必须分离（如 `dispatcher.go` 与 `dispatcher_routing.go`）。
  - 算法/策略实现与输入合规校验必须分离（如 `strategy.go` 与 `strategy_validation.go`）。

### 2. 关注点分离 (Separation of Concerns, SoC)
- **业务内核 vs 导出适配**：运行时数据采集与外部协议导出严格解耦。例如 `telemetry/metrics.go` 纯粹负责原子计数器与分位数近似插值计算，Prometheus exposition 文本格式化必须独立在 `metrics_prometheus.go` 中。
- **存储驱动 vs 业务仓储**：SQL 执行与对象存储驱动严格通过接口隔离，业务层只面向 `Repository` 和 `ArtifactStorage` 接口编程。

### 3. 经典设计模式规范化应用
- **策略模式 (Strategy Pattern)**：用于非高峰阶梯计费（`OffPeakStrategy`）、加权轮询与会话亲和选择（`ChannelSelectorStrategy`）、限流排队治理。
- **状态模式 (State Pattern)**：用于三态断路器（`closedState`、`openState`、`halfOpenState`），将状态迁移和探针试探逻辑内聚在状态对象内。
- **命令模式 (Command Pattern)**：用于 CLI 命令行工具（`cmd/airoute`），每个子命令（`status`、`chat`、`keys`、`users`、`skills`、`mcp`）拥有独立的文件与执行上下文。
- **适配器模式 (Adapter Pattern)**：用于多提供商协议互转（OpenAI ⇄ Anthropic ⇄ Gemini）。
- **装饰器与流水线模式 (Pipeline & Middleware)**：统一在 Gin 中间件处理租户认证、额度预扣、动态限流与全局审计染色。

### 4. 架构对称性 (Architectural Symmetry)
同层级、同类型的模块在结构上必须保持视觉和概念上的对称性：
- `anthropic.go`（一元转换） $\leftrightarrow$ `anthropic_stream.go`（流式解析）
- `gemini.go`（一元转换） $\leftrightarrow$ `gemini_stream.go`（流式解析）
- `handler.go`（对话一元处理） $\leftrightarrow$ `chat_stream_handler.go`（对话 SSE 流处理）
- `completions_handler.go`（文本补全一元） $\leftrightarrow$ `completions_stream_handler.go`（文本补全流）

---

## 二、 代码组织与文件粒度硬性约束

为了杜绝代码膨胀和难以维护的巨石文件，项目设定了严格的物理代码规模红线：

| 约束项 | 指标要求 | 说明与例外 |
| :--- | :--- | :--- |
| **单文件行数硬上限** | **$\le 300$ 行** | **绝对红线**。除不可分割的纯静态 Schema 声明（如 `mcp_schemas.go`）与 SQL DDL 迁移文件外，任何业务逻辑文件严禁超过 300 行。 |
| **建议文件规模** | **100 ~ 200 行** | 理想的代码粒度，保持极佳的阅读心智负担与单一职责。 |
| **单函数行数上限** | **$\le 60$ 行** | 超过 60 行的函数必须按逻辑段提炼私有辅助函数（Extract Method）。 |
| **嵌套深度** | **$\le 4$ 层** | 优先使用卫语句（Guard Clauses）提早返回，杜绝层层递进的深层嵌套。 |

---

## 三、 信息输出与日志语言规范 (三层隔离法则)

系统信息输出必须严格区分为**三层领域边界**，杜绝中英文混杂：

```
                ┌────────────────────────────────────────────────────────┐
                │             Airoute 系统信息输出三层架构边界               │
                └────────────────────────────────────────────────────────┘
                                            │
       ┌────────────────────────────────────┼───────────────────────────────────┐
       ▼                                    ▼                                   ▼
【数据面 Data Plane】               【观测面 Observability】              【人机控制面 Control/CLI】
  • 接口: /v1/chat/completions        • 接口: telemetry.Logger.*           • 接口: /api/v1/admin/*、CLI
  • 受众: Cursor、LangChain、SDK      • 受众: Loki、ELK、报警规则引擎       • 受众: 运维工程师、Web 管理员
  • 规范: 100% 纯英文标准协议          • 规范: 100% 纯英文小写键值对         • 规范: 100% 规范技术中文
```

### 1. 数据面 (Data Plane)：100% 保持 OpenAI 英文标准协议
- **受众**：第三方 LLM SDK（LangChain、LlamaIndex、Python `openai`、Cursor、Continue）。
- **规范**：
  - 错误格式必须符合 `{"error":{"message":"...","type":"...","code":"..."}}`。
  - `code` 与 `type` 必须为全英文机器码（例如 `missing_model`、`model_not_allowed`、`invalid_payload`）。
  - **严禁在数据面响应中输出中文错误**，否则第三方 SDK 会因无法识别标准枚举值而引发未捕获异常。

### 2. 观测面日志 (Observability Telemetry)：100% 保持规范英文小写键值对
- **受众**：Logstash、Loki、Datadog、Kubernetes 运维系统与报警规则。
- **规范**：
  - `telemetry.Logger.Info / Warn / Error` 的 `msg` 必须为全英文小写短语（如 `gateway server listening`、`channel execution failed`）。
  - 所有动态参数必须通过结构化键值对传递（如 `"addr", addr, "duration_ms", dur`）。
  - **严禁在底层日志的 `msg` 中插入 Emoji 或长句中文**，确保云原生日志系统的正则解析与警报规则稳定运行。

### 3. 控制面与人机终端 (Control Plane & CLI)：100% 保持专业技术中文
- **受众**：系统管理员、Web 控制台用户、终端运维工程师。
- **规范**：
  - `/api/v1/admin/*` 与 `/api/v1/user/*` 的用户消息必须为规范中文提示（如 `{"code":0,"message":"渠道配置已成功更新"}`）。
  - `cmd/airoute` 终端 CLI 命令（`airoute status`、`airoute users` 等）必须提供清晰的中文提示、表头与排版。
  - 技术专有名词或协议标识保留标准双语对照（如 `在线正常 (200 OK)`、`健康 (HEALTHY)`、`熔断 (TRIPPED)`、`已启用 (ENABLED)`）。
  - 网关服务启动完成时，通过标准输出打印面向运维操作员的中文就绪横幅。

---

## 四、 运行环境与 Docker Compose 纳管规范

Airoute 生产与标准测试环境采用 **Docker Compose 高可用容器集群**进行编排，**禁止脱离容器在宿主机随意启动未纳管的进程**。

### 1. 集群组件编排架构
- `airoute-1` / `airoute-2`：基于双节点的无状态网关实例（暴露端口 8080）。
- `airoute-lb`：Nginx 7 层负载均衡器，支持 SSE 流式长连接与会话黏性转发（外部映射端口 8080）。
- `airoute-postgres`：PostgreSQL 16 分布式主数据库。
- `airoute-redis`：Redis 7 分布式缓存与秒级集群配置失效发布订阅总线。
- `airoute-storage`：RustFS S3 兼容对象存储，承载 Agent 技能包与工件。

### 2. 变更部署与热重载操作规范 (SOP)
代码重构或新增功能后，必须按以下步骤平滑发布并检验：

```bash
# 1. 重新构建容器镜像（基于多阶段编译 Dockerfile）
docker compose build --build-arg VERSION="$(sh scripts/build-version.sh)" gateway-1 gateway-2

# 2. 滚动热重载网关容器
docker compose up -d gateway-1 gateway-2

# 3. 验证集群容器运行状态（必须全部处于 healthy 状态）
docker compose ps

# 4. 容器内自检与健康探针测试
docker exec airoute-1 /app/airoute status
docker exec airoute-1 /app/airoute users list
docker exec airoute-1 /app/airoute skills list
```

### 3. Go 编译指令规范
- 由于 `cmd/airoute/` 采用了子命令解耦模式，构建指令**必须指定包目录而非单个入口文件**：
  ```bash
  # 正确用法
  go build -o bin/airoute ./cmd/airoute

  # 错误用法（会导致丢弃其他 cli_*.go 文件编译）
  go build -o bin/airoute cmd/airoute/main.go
  ```

---

## 五、 测试驱动与防回归规范

1. **测试全量覆盖**：
   - 核心路由分发、前缀缓存会话亲和、状态熔断、闲时计费算法必须配备完整的单元测试。
   - 数据平面的协议互转（OpenAI ⇄ Anthropic ⇄ Gemini）必须通过集成测试用例检验。
2. **零回归准则 (Zero-Regression)**：
   - 在任何 Git 提交（Commit）与推送（Push）前，必须运行全量测试套件并保证 100% 通过：
     ```bash
     go test -count=1 ./...
     ```
   - 严禁提交任何会导致 `go test` 报错或带有未解决并发数据竞争（Data Race）的代码。

### 5.1 本地门禁（与 CI 同源）

`.github/workflows/quality-gate.yml` 是"什么算通过"的**唯一定义**，`ci.yml` / `docker.yml` / `release.yml` 都复用它。本地对应命令：

```bash
# Go 侧：竞态检测 + 全量测试（CI 使用 -race，本地也应如此）
go test -race -count=1 ./...

# 真实 PostgreSQL / Redis 集群路径（需要容器，未配置 DSN 时自动跳过）
make test-integration

# 前端：lint（带告警基线）+ 单元/组件测试 + 构建
cd web && npm run lint && npm run test && npm run build
```

**顺序有依赖**：Go 编译经由 `web/web.go` 的 `//go:embed dist/*` 传递依赖前端产物，因此必须先构建前端。`make build` / `make test` 已自动处理；直接调用 `go build` 的脚本需要自行先跑 `make build-web`。

### 5.2 集成测试必须进真实集群路径

仅靠内存实现（SQLite + 本地缓存）无法覆盖 PostgreSQL 方言分支、`FOR UPDATE` 行锁语义与 Redis 广播路径。凡涉及这些分支的改动，必须在 `-tags=integration` 下验证：

```bash
AIROUTE_TEST_DATABASE_URL='postgres://...' \
AIROUTE_TEST_REDIS_URL='redis://...' \
go test -race -tags=integration ./internal/storage/ ./internal/distributed/
```

**未配置 DSN 时集成测试会静默跳过**，因此"本地全绿"不等于"集成路径已验证"——改动相关代码时必须显式提供 DSN。

### 5.3 变异测试：证明测试真的能失败

新增或修改测试后，必须**故意把被测代码改坏**，确认测试失败，再恢复并校验源文件字节一致：

```bash
cp file.go /tmp/file.bak
# ... 应用变异 ...
go test -run TestX ./...        # 必须失败
cp /tmp/file.bak file.go
sha256sum -c <<< "$(sha256sum file.go)"
```

要求：

- 变异必须**确实应用成功**（用断言或先确认代码已改），否则"测试通过"是假象。
- 变异应命中**该测试声称守护的那条性质**，而不是顺带触发别的失败。
- 安全规则（鉴权、租户隔离、额度）必须做 **fail-open 变异**：让规则放行所有调用者，确认测试能捕获。

### 5.4 契约测试：把线协议钉死

请求/响应结构体的 JSON 字段名是**对外契约**，用反射做机器可校验的清单（见 `internal/controlplane/contract_test.go`）：

- 管理端请求体不得直接绑定数据库 Record（客户端可注入 `id` / `created_at` / `used_cost` 等输出专用列）。
- 前端读取的字段名必须与后端发送的一致。历史上 `PricingManager` 读 `is_currently_off_peak` 而后端发 `is_current_off_peak`，导致离峰折扣列**永远显示原价**且无任何报错。
- 改名任一侧都必须同时改另一侧，且两侧各有测试守护。

### 5.5 前端告警基线（棘轮，非目标）

前端 lint 采用**棘轮**策略：`eslint src --max-warnings N`，N 为当前基线，只允许下降。

- 基线只能**收紧**，不得上调。
- 新增告警会直接使门禁失败——这是刻意设计。
- 现存告警是**待办清单**，不是可忽略的噪声。清理前先确认它代表的是死代码还是**未接完的功能**；后者应补全而非删除。

JSX 中的标识符引用由 `react/jsx-uses-vars` 识别。缺少该规则会让每个仅用于渲染的组件都被误报为未使用变量（曾产生 448 条误报）。

### 5.6 并发与数据竞争

- 任何涉及共享状态的改动必须以 `-race` 验证。
- 跨事务的"读取—修改—写入"必须收进单个事务（参见 `PatchChannel`、`ApplyModelRouteBundle`）：事务外的读取会让并发编辑被静默回滚。
- 新增跨进程共享状态时，补充**真实并发写入者**的测试，而非只测串行路径。

### 5.7 注释必须与实现一致

审查中反复发现注释描述的行为与实际代码不符（例如声称"Up 与版本记录在同一事务提交"而实际是两条独立语句；声称"清除每项租户数据"而漏清三项）。**注释是契约的一部分**：改动行为时必须同步注释，发现不符的注释应按缺陷处理。



## 五、版本递进约束

后续开发和发布必须遵循 [版本与发布规范](RELEASE_GUIDE.md)：兼容修复升 PATCH，兼容新能力升 MINOR，兼容性破坏升 MAJOR；一次发布按最高变更级别定版本。已有正式标签不得覆盖。数据库迁移序号和 API 路径版本独立管理。提交前执行 `sh scripts/build-version.sh`，确保 Go、前端与构建版本一致。
