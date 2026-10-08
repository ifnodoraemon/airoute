# Airoute 🚀

> **A self-hosted LLM gateway in Go.** Bidirectional OpenAI / Anthropic / Gemini protocol translation, multi-provider failover with circuit breaking, session affinity, and per-request cost & channel audit — one binary with an embedded React admin console, SQLite or PostgreSQL.

[![CI Quality Gate & Automated Testing](https://github.com/ifnodoraemon/airoute/actions/workflows/ci.yml/badge.svg)](https://github.com/ifnodoraemon/airoute/actions/workflows/ci.yml)
[![Build & Publish Multi-Arch Docker Image](https://github.com/ifnodoraemon/airoute/actions/workflows/docker.yml/badge.svg)](https://github.com/ifnodoraemon/airoute/actions/workflows/docker.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/ifnodoraemon/airoute)](https://goreportcard.com/report/github.com/ifnodoraemon/airoute)

生产级 LLM 与全模态智能网关，Go 语言构建。原生支持 **Any-to-Any 协议双向转译**（OpenAI ⇄ Anthropic ⇄ Gemini）、**级联模型命名与通配符前缀映射**、**下游智能探测自动建渠道**、**多渠道优先级 failover 与三态熔断**。

数据面（高并发转发内核）与控制面（运维治理）彻底解耦，单二进制内嵌现代化管理台（React 19 + Tailwind CSS），支持 SQLite 单机与 PostgreSQL 集群两种部署形态。

---

## 📚 完整文档索引 (Documentation)

- [📖 API 接口详尽参考手册 (API Reference)](docs/API_REFERENCE.md)
- [⚙️ 系统全量配置与调优指南 (Configuration & Tuning)](docs/CONFIGURATION.md)
- [🔌 供应商与下游对接指南 (Provider Integration Guide)](docs/PROVIDER_GUIDE.md)
- [🛡️ 高可用集群与容灾部署架构 (HA Architecture)](docs/HA_ARCHITECTURE.md)
- [📦 发版与镜像发布流程 (Release Guide)](docs/RELEASE_GUIDE.md)

---

## 🌟 核心特性与架构优势

### 1. 🔍 一键智能探测下游服务 (Downstream Auto-Probe)
- 只需输入下游服务的 Base URL 与可选 API Key，网关后台**毫秒级探测并自动读取**：
  - 自动发现所有挂载模型 ID；
  - 自动识别厂商类型（GPUStack、Sub2API、Google Gemini、Anthropic Claude、vLLM、Ollama）；
  - 自动匹配并勾选下游支持的协议与模态（对话、生图、TTS、Whisper、视频）；
  - 彻底免去用户手动配置模型列表与协议的繁琐操作。

### 2. 🔄 全双工协议自动转译 (Bidirectional Protocol Translation)
- **纯补全下游自动适应**：当下游模型仅支持传统的 `/v1/completions` 接口时，网关透明地将入站的 Chat 请求与 SSE 流转译为补全 Prompt，并将响应包装回标准 Chat Choices 与 SSE Deltas。
- **Claude 原生双向互转**：客户端可直接使用原生 Anthropic SDK 请求 `/v1/messages`，网关自动将请求转化为 OpenAI / Gemini 格式，并在返回时转换为 Claude Messages 协议。
- **Gemini 原生格式转译**：官方 Gemini Developer API 专用协议透明转译，支持多模态多轮会话。

### 3. 🎨 文本/图像/语音/视频全模态支持 (Unified Multimodal Pipeline)
- **对话、智能体响应与补全**：`/v1/chat/completions`, `/v1/responses` (OpenAI 官方新代智能体协议), `/v1/completions`, `/v1/messages`
- **AI 图像生成**：`/v1/images/generations`（DALL-E 3、Flux、SD3）
- **语音合成 TTS**：`/v1/audio/speech`（流式二进制直连，零常驻内存）
- **语音识别 STT**：`/v1/audio/transcriptions`（Whisper Multipart 流式转录）
- **视频生成与轮询**：`/v1/videos/generations`, `/v1/videos/tasks/:id`（Sora / CogVideoX 异步任务轮询）
- 所有模态均采用**软件工程 Strategy 统一管道 (`DispatchHTTP`)**，共享三态熔断与安全容灾。

### 4. 🔗 级联模型源与无限制模型命名 (Cascading Models & Namespaces)
- **多层级斜杠无限制支持**：下游模型形如 `xxx/xx`，对外可任意包装为 `yy/xxx/xx`、`org/team/project/model` 或任意级联路径，网关不设任何层级限制。
- **通配符前缀映射 (Prefix Wildcards)**：支持配置 `"yy/*": "*"` 或 `"org/dept/*": "*"`，自动完成前缀剥离与转发重写。
- **自动渠道名称前缀匹配**：当请求模型形如 `<provider_name>/<upstream_model>` 时，网关自动定位该 Provider 并透明透传 `<upstream_model>`。

### 5. ⚡ 极致性能与零延迟 (Extreme Performance)
- **代理热路径零数据库查询 (Zero-DB Hot Path)**：路由表与密钥常驻内存读写锁结构，微秒级路由决策（$< 50\mu s$），0 次 SQL 查询。
- **预热长连接池 (HTTP Transport Pool)**：预热 2048 个复用连接、单 Host 256 并发长连接、HTTP/2 多路复用，杜绝 TCP 握手开销。
- **异步环形缓冲审计日志器 (AsyncLogger)**：10000 容量缓冲队列，500ms 批量异步持久化入库，慢磁盘 IO 绝不拖累转发延迟。

### 6. 🛡️ 极致高可用与容灾 (Extreme High Availability)
- **三态智能熔断器 (Tri-State Circuit Breaker)**：每个 Provider 独立跟踪健康度。发生 3 次连续故障（500/429/超时）即刻熔断 30 秒，旁路死节点避免雪崩；冷却后自动进行半开探活与自愈。
- **首字前无感容灾窗 (Safe Fallback Window)**：在首个有效 Token 或完整结果返回前遇到上游异常，毫秒级无缝漂移到下一个优先级 Provider，客户端业务完全无感知。
- **集群 10 秒定期无锁自同步**：任意实例修改配置，多副本集群后台自动热重载，无需人工重启服务。

---

## 🏛️ 系统架构图

```mermaid
flowchart TD
    Client["下游客户端 (OpenAI SDK / Claude SDK / 业务 Agent)"] --> LB["负载均衡器 (Nginx / K8s Ingress)"]
    
    subgraph Cluster["Airoute 高可用集群 (无状态水平扩展)"]
        Node1["Gateway 节点 1"]
        Node2["Gateway 节点 2"]
        NodeN["Gateway 节点 N"]
    end
    LB --> Node1
    LB --> Node2
    LB --> NodeN

    subgraph DataPlane["1. 数据面内核 (Data Plane - 极致性能与低延迟)"]
        Adapter["Any-to-Any 协议矩阵 (Chat / Text / Messages / 多模态)"]
        Dispatcher["统一分发管道与协议路由 (DispatchHTTP / DispatchStream)"]
        Breaker["三态熔断器 (Tri-State Circuit Breaker)"]
        Fallback["首字前无感容灾窗 (Safe Fallback Window)"]
        Pool["预热长连接池 (2048 闲置连接复用)"]
    end
    Node1 --- DataPlane
    Node2 --- DataPlane
    NodeN --- DataPlane

    subgraph UpstreamProviders["下游模型源 (Providers - 支持协议过滤与智能探测)"]
        GPUStack["🖥️ GPUStack (私有化集群)"]
        Sub2API["⚡ Sub2API (聚合通道)"]
        Gemini["🌐 Google Gemini (专用协议)"]
        Claude["🧠 Anthropic Claude"]
        OpenAI["🤖 OpenAI 官方"]
        DeepSeek["🚀 DeepSeek"]
        Custom["🛠️ 自定义下游 (Custom)"]
    end
    DataPlane --> UpstreamProviders

    subgraph ControlPlane["2. 控制面与同步内核 (Control Plane)"]
        SyncWorker["集群自动定期热加载 (10s 自动无锁同步)"]
        Prober["智能下游探测引擎 (Auto-Probe)"]
        AdminAPI["RESTful 运维治理 API (/channels, /keys, /logs, /stats)"]
    end

    subgraph StorageLayer["3. 存储与异步审计引擎 (Storage Engine)"]
        SQLite[(纯 Go SQLite WAL 模式)]
        AsyncQueue["异步环形缓冲队列 (10,000 容量)"]
    end
    DataPlane -.->|无阻塞异步投递| AsyncQueue
    AsyncQueue -->|批量刷盘| SQLite
    ControlPlane <-->|读写热加载| SQLite
```

---

## 🚀 极速部署指南

### 方式 A: 预编译二进制直接运行
```bash
# 1. 编译自包含单二进制（包含内嵌 Web UI）
make build

# 2. 启动网关
./bin/airoute -config configs/config.yaml
```
- 工作台地址：`http://localhost:8080/app/` 或 `http://localhost:8080/`
- Prometheus 指标：`http://localhost:8080/metrics`
- 健康检查：`http://localhost:8080/health`

### 方式 B: Docker Compose 多副本高可用集群
```bash
# 一键启动 2 副本 Gateway + Nginx 负载均衡器
docker compose up -d --build
```
Nginx 会自动在 `http://localhost:8080` 暴露统一入口（宿主端口见 `docker-compose.yml` 的 `8080:80` 映射），并以 `least_conn` 算法向双节点分发流量，自动关闭 SSE 缓冲 (`proxy_buffering off`)。

### 方式 C: Kubernetes Helm Chart 部署
```bash
# 使用 Helm 一键部署至 K8s
helm install airoute ./helm/airoute -n gateway --create-namespace
```

---

## 🧪 自动化测试体系

### 1. 单元与组件集成测试 (Unit & Component Tests)
无需外部依赖，全部核心逻辑均在并发竞态检测 (`-race`) 模式下运行：
```bash
make test
```
包含：
- API 健康度与 Prometheus `/metrics` 校验
- 控制面动态 CRUD 与数据面热重载
- 级联模型命名与通配符前缀剥离 (`TestCascadingAndUnrestrictedModelMapping`)
- 三态智能熔断器状态机跃迁 (`CLOSED -> OPEN -> HALF-OPEN -> CLOSED`)
- 首字前无感容灾窗口（非流式与流式）
- Google Gemini 协议与 Anthropic Claude 双向流式转换
- 多模态生图、TTS、Whisper STT 与视频生成管道
- 纯文本补全下游全双工协议转译与智能探测 (`TestAutoProbe_OpenAIAndGPUStack`)

### 2. 服务启动后的集群端到端测试 (Cluster End-to-End Tests)
在服务或 Docker Compose 集群启动后，一键对真实运行环境进行全链路验收：
```bash
# 启动集群
docker compose up -d

# 方式 A: 零依赖 Shell 脚本自动化套件（支持 CI/CD 与容器环境，带色彩诊断报告）
make test-e2e
# 或指定目标地址：AIROUTE_URL=http://localhost:8080 ./scripts/test-e2e.sh

# 方式 B: Go 原生 E2E 驱动套件
make test-e2e-go
# 或：go test -v -tags=e2e ./tests/e2e/...
```

E2E 套件覆盖的 8 大核心验收维度（共 25 项自动化测试）：
1. **集群健康 & Web 入口**：负载均衡器 `/health` (healthy)、状态探针 `/api/v1/public/status`、SPA HTML 渲染
2. **安全边界 & 鉴权校验**：非鉴权拦截 (401)、错误口令拒绝、Admin JWT 签发与 `/api/v1/auth/me` 身份自省
3. **MCP 生态集成**：内置 15 款核心 MCP 服务器（Dev / Search / Data / AI / Ops / Enterprise）元数据核验与动态启停
4. **Skills Hub & 边缘缓存**：Skill ZIP 包网关权威流式下载（PK 结构校验）与 Nginx 边缘缓存 (`X-Cache-Status: HIT`)
5. **模型拓扑与计费引擎**：路由矩阵、上游渠道、分级计费规则与实时运行遥测
6. **密钥治理与节点热同步**：虚拟密钥动态签发、配额设置与 Redis Pub/Sub 多节点毫秒级同步
7. **数据面代理与分布式追踪**：`/v1/models`、`/v1/chat/completions` 与全链路 `X-Airoute-Trace-Id` 注入校验
8. **高可用并发稳定性**：高并发批量探针零丢包、负载均衡无损转发验证

---

## 📄 开源许可证

本项目基于 [MIT 许可证](LICENSE) 开源。
