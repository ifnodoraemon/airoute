# Airoute 部署与故障转移

Airoute 在进程内缓存渠道、模型路由和密钥配置。控制面和审计仍使用数据库；当审计数据库写入失败时，本地队列会保留批次并重试，队列满时请求会等待可用空间。因此数据库故障可能影响请求时延。

## 单机 SQLite

Helm 默认运行一个副本，使用 ReadWriteOnce PVC 保存 SQLite 数据，并使用 `Recreate` 更新策略避免新旧 Pod 同时打开数据库。SQLite WAL 不支持跨机器共享网络文件系统；不要把同一个 SQLite 文件挂给多个网关副本。Ingress 默认关闭。Chart 创建包含随机 `admin-password` 与 `admin-secret` 的 Secret；升级时保留此 Secret，避免登录凭证失效。

```bash
helm install airoute ./helm/airoute -n gateway --create-namespace
kubectl get secret airoute-auth -n gateway -o jsonpath='{.data.admin-password}' | base64 -d
```

## 多副本 PostgreSQL

多副本需要集中式 PostgreSQL、所有副本共享的管理员签名密钥、禁用 SQLite PVC，并为需跨副本访问的文件配置共享对象存储。Chart 会拒绝开启多副本但仍挂载 SQLite PVC 或未提供 PostgreSQL Secret 的配置。

```bash
# 事先创建含键 url 的 PostgreSQL Secret，以及可选的管理员认证 Secret。
helm upgrade --install airoute ./helm/airoute -n gateway --create-namespace \
  --set replicaCount=3 \
  --set persistence.enabled=false \
  --set database.existingSecret=airoute-database
```

各副本每 10 秒从数据库同步配置。熔断状态仍由各进程独立维护，切换取决于具体请求的上游耗时；没有固定的故障转移毫秒数或可用率保证。流式请求在首个内容块或工具调用前可以尝试备用渠道；已发送内容后的错误会返回给客户端。

## 健康检查与就绪探针

- `GET /health`（以及 `/healthz`、`/v1/health`）是**存活**探针：只要 HTTP 进程能应答就返回 200，不访问数据库/Redis，用于边缘代理和人工快速判断。
- `GET /health/ready`（以及 `/healthz/ready`、`/readyz`）是**就绪**探针：数据面已加载配置快照、且已注册的依赖探针（PostgreSQL/SQLite `PingContext`、配置了 Redis 时的 `PING`）全部通过才返回 200，否则 503，并给出脱敏的逐项状态（`ok` / `unavailable`）。单次评估有 2s 上限，原始错误不会返回给未认证调用方。

Compose/Dockerfile、Swarm stack、Helm chart 与 `deploy/kubernetes.yaml` 的**就绪/健康检查**都使用 `/health/ready`；Kubernetes 的 `livenessProbe` 仍使用 `/health`，避免数据库短暂抖动导致容器被重启。`scripts/docker-rebuild.sh` 轮转副本时轮询的容器健康状态因此也是就绪状态。

## 连接与停机

HTTP Transport 按需建立连接，最多保留 2048 个空闲连接、每 Host 最多 512 个空闲连接；这些数字不是预热连接数或并发上限。

收到 `SIGTERM`/`SIGINT` 后进程停止接收新连接，最多等待 `GATEWAY_SHUTDOWN_TIMEOUT_SEC`（默认由 `GATEWAY_DRAIN_TIMEOUT_SEC + 15s` 派生为 135s）让在途请求完成——其中包括客户端断连后继续排空上游的 `GATEWAY_DRAIN_TIMEOUT_SEC`（默认 120s）预算——随后停止审计队列（落盘/刷库）并关闭数据库。信号处理在初始化之前注册，因此迁移/启动阶段的信号也会走同一套清理流程，而不是直接杀死进程。

编排层必须给出更大的宽限窗口：Compose/Swarm 服务使用 `stop_grace_period: 150s`（Docker/Swarm 默认 10s 会在排空完成前 SIGKILL），Helm 使用 `terminationGracePeriodSeconds: 180`（可配置）。若数据库持续不可用直到进程退出，内存中未写入的审计记录仍有丢失风险；对严格计费场景应配置 Redis Streams、监控持久化错误，并确保数据库在停机前恢复。
