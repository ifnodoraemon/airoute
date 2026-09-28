# Airoute 企业级高可用多机部署方案 (Docker Swarm)

本文档介绍 **Airoute** 基于 **Docker Swarm** 的多机分布式高可用（HA）、高性能、高扩展部署方案，涵盖分布式数据库（PostgreSQL）、分布式缓存与消息队列（Redis Streams）、负载均衡与流式穿透（NGINX Ingress）、以及全链路 `TraceID` 日志归集架构。

---

## 1. 架构总览

```
                                  [ 客户端流量 / SDK / Web 控制台 ]
                                                 │
                                                 ▼
                        ┌─────────────────────────────────────────────────┐
                        │   Swarm Ingress Mesh / NGINX Load Balancer      │
                        │   (HTTP/2, SSE Stream unbuffered, 2 Replicas)   │
                        └────────────────────────┬────────────────────────┘
                                                 │ (Round-Robin / LeastConn)
                     ┌───────────────────────────┴───────────────────────────┐
                     ▼                                                       ▼
        ┌─────────────────────────┐                             ┌─────────────────────────┐
        │  Airoute 节点 1    │  ... (可水平扩缩至 N 个节点)  │  Airoute 节点 N    │
        │  - TraceID 注入与传播    │                             │  - TraceID 注入与传播    │
        │  - 分布式滑动窗口限流    │                             │  - 分布式滑动窗口限流    │
        │  - 会话粘滞与分时计费   │                             │  - 会话粘滞与分时计费   │
        └───────┬─────────┬───────┘                             └───────┬─────────┬───────┘
                │         │                                             │         │
                │         │       XADD nano:stream:logs (亚毫秒异步投递)  │         │
                │         └───────────────────────┬─────────────────────┘         │
                │                                 │                               │
                │                                 ▼                               │
                │             ┌───────────────────────────────────────┐           │
                │             │  Redis 7 (分布式缓存 / 消息队列)        │           │
                │             │  - nano:stream:logs (日志消息队列)    │           │
                │             │  - nano:cluster:reload (缓存广播)     │           │
                │             │  - nano:ratelimit:rpm (集群滑动窗口)  │           │
                │             │  - nano:session:* (集群会话路由)      │           │
                │             └───────────────────┬───────────────────┘           │
                │                                 │                               │
                │                    XREADGROUP 消费者组批量消费入库                │
                │                                 ▼                               │
                │             ┌───────────────────────────────────────┐           │
                └────────────►│  PostgreSQL 16 (分布式关系数据库)      │◄──────────┘
                              │  - 共享渠道/密钥/计费/用户元数据      │
                              │  - usage_logs 归集审计表 (含 TraceID) │
                              └───────────────────────────────────────┘
```

### 核心特性亮点
1. **多机跨节点容器编排 (Docker Swarm Overlay Network)**:
   - 网关服务 `airoute` 跨多台物理机/虚拟机无缝扩缩容（默认 3 副本，支持一键 `scale=10`）。
   - Swarm 内部加密 Overlay 专用子网 `nano-swarm-net`，节点间通信完全内网隔离。
2. **分布式数据库架构 (PostgreSQL 16 + Dialect Rebinding)**:
   - 从单机 SQLite 扩展至企业级分布式 PostgreSQL，彻底解决 SQLite 多节点并发写锁死问题。
   - 网关内置驱动自适应引擎（`db.Rebind`），自动兼容 SQL 占位符、时间函数与冲突更新机制。
3. **分布式日志消息队列 (Redis Streams Zero-Loss Log Queue)**:
   - 面对每秒数万并发的 LLM 请求，网关不直接同步或单机内存落库，而是通过 Redis Streams（`XADD`）高并发异步推入队列。
   - 队列采用 Consumer Group 机制多节点分摊消费，批量入库后 `XACK` 确认，确保高并发流量洪峰下不卡顿、节点重启零丢日志。
4. **全链路分布式 TraceID 追踪与日志归集**:
   - 统一规范: 自动识别或生成 `X-Nano-Trace-ID` / `X-Request-ID` / W3C `traceparent`。
   - 上下游打通: 客户端 -> 网关 -> 上游大模型服务商 -> 审计数据库 -> Web 控制台一键复制并秒级过滤定位。
   - 日志收集体系: 每个 HTTP 请求自动输出标准化 JSON 结构化日志，无缝接入 Loki、Promtail、FluentBit、Filebeat 或 ELK。

---

## 2. 快速部署步骤

### 前置条件
- 已安装 Docker 24.0+，且各节点之间网络端口互通（Swarm 端口：2377, 7946, 4789）。

### 步骤 1：初始化 Swarm 管理节点（Master Node）
```bash
# 在主节点上执行
docker swarm init --advertise-addr <主节点IP>

# （可选）如需多机加入，在其他从节点（Worker Nodes）运行输出的 join 命令：
# docker swarm join --token SWMTKN-... <主节点IP>:2377
```

### 步骤 2：一键部署 Stack
```bash
cd airoute

# 自动创建 overlay 网络并部署服务
./deploy/swarm/deploy.sh --build
```

### 步骤 3：查看运行状态
```bash
# 查看所有集群服务与副本数
docker stack services nano-stack

# 查看各个容器实例在不同机器上的分布
docker stack ps nano-stack
```

预期输出示例：
```
ID             NAME                       MODE         REPLICAS   IMAGE                 PORTS
o12a9bc3def    nano-stack_airoute    replicated   3/3        airoute:latest   
p456def789a    nano-stack_nano-lb         replicated   2/2        nginx:alpine          *:8080->80/tcp
q789abc012d    nano-stack_nano-postgres   replicated   1/1        postgres:16-alpine    
r012def345e    nano-stack_nano-redis      replicated   1/1        redis:7-alpine        
```

---

## 3. 高可用运维指南

### 3.1 零停机滚动升级 (Zero-Downtime Rolling Update)
`docker-stack.yml` 已配置 `order: start-first` 与 `parallelism: 1`：
```bash
# 重新构建新版本镜像并滚动部署
docker build -t airoute:v1.1.0 .
docker service update --image airoute:v1.1.0 nano-stack_airoute
```
Swarm 将先拉起健康的新容器，等待 `/health` 健康检查通过后，再优雅销毁旧容器；若升级失败将自动回滚（`rollback`）。

### 3.2 动态水平扩缩容 (Horizontal Scaling)
业务流量突增时，一键秒级扩容网关计算节点：
```bash
# 将网关节点扩容到 10 个实例
docker service scale nano-stack_airoute=10
```

### 3.3 全链路 TraceID 排查日志
当客户端调用报错或排查单次请求延迟时：
1. 打开 Web 控制台 -> **调用审计日志** -> 点击详情，顶部显示醒目的 **Trace ID**，支持一键点击复制与“追踪全链路”。
2. 或者在服务器端直接通过 `Trace ID` 检索集群容器日志：
```bash
docker service logs nano-stack_airoute | grep "tr-xxxx-yyyy"
```
JSON 日志将完整展示该请求的请求时间、上游耗时、TTFT、扣减费用、客户端 IP 以及上游错误上下文。
