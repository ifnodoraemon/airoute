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
docker compose build gateway-1 gateway-2

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
