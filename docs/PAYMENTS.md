# Stripe 支付

钱包仅以人民币计价，通过 Stripe 托管 Checkout 收款。模拟充值接口已删除，GET 和 POST 均不会创建或完成模拟订单。测试支付使用 Stripe 的独立测试环境和独立网关数据库、模型额度。

## 配置收款

| 变量 | 配置 |
| --- | --- |
| `PUBLIC_URL` | 网关的公网 HTTPS 地址，例如 `https://gateway.example.com`。也可通过 `server.public_url` 配置。支付返回地址使用明确配置，不从请求头推断 |
| `STRIPE_MODE` | `live`（默认）或 `test` |
| `STRIPE_API_KEY` | 与模式匹配的 `sk_live_` / `rk_live_` 或 `sk_test_` / `rk_test_` 密钥 |
| `STRIPE_WEBHOOK_SECRET` | 本端点的 `whsec_` 签名密钥 |
| `STRIPE_MIN_RECHARGE_CNY` | 默认 10 元。业务下限必须满足商户实际结算币种与换汇后的最低收款要求，不能把 0.01 元视为 Stripe 卡支付的统一下限 |
| `STRIPE_SUCCESS_URL` | 可选，同源返回页；系统自动附加 `tab=wallet`、`recharge=success`、`order_no`，保留其他查询参数 |

两个密钥都为空时关闭线上支付，兑换码仍可使用；配置不完整、密钥模式不匹配、返回地址无效等情况会拒绝启动。数据库首次启用支付时绑定模式，此后不允许在同一数据库切换 live/test。测试与生产分别使用数据库和密钥。

服务端 HTTP 请求固定 `Stripe-Version: 2026-10-28.endive`。在 Stripe 为下列地址注册 **Snapshot** Webhook，并选择同一个 API 版本：

```text
https://gateway.example.com/api/v1/public/stripe/webhook
```

订阅事件：

- `checkout.session.completed`、`checkout.session.async_payment_succeeded`、`checkout.session.async_payment_failed`、`checkout.session.expired`
- `refund.created`、`refund.updated`、`refund.failed`、`charge.refunded`
- `charge.dispute.created`、`charge.dispute.updated`、`charge.dispute.closed`、`charge.dispute.funds_withdrawn`、`charge.dispute.funds_reinstated`

事件验签使用原始请求体，要求五分钟时间窗、环境与 API 版本匹配。处理器把事件保存后从 Stripe 查询当前状态，不直接用通知里的金额或状态改余额。未知事件类型可以忽略；暂时失败的已保存事件由后台恢复，同时向 Stripe 返回可重试的失败状态。参考 [Stripe 官方通知指南](https://docs.stripe.com/webhooks)。

## 下单、重试与补偿

`POST /api/v1/user/wallet/recharge/stripe/session` 接收 `{"amount":50}`，必须携带 8–128 位字母、数字、短横线或下划线组成的 `Idempotency-Key`。同一用户、同一支付意图在超时或错误后复用该键；改变金额时创建新的意图。控制台在当前浏览器会话中保存该标识，跨页面重载仍可重试。

服务端先保存订单、金额、环境、返回地址和会话到期时间，再以稳定订单号作为 Stripe 幂等键创建会话。用户与幂等键有数据库唯一约束；复用键时改变金额或模式返回 409。响应丢失或会话回写失败后，按服务端订单引用查找既有会话，或者在有效幂等期内重试原创建请求，完成核验和持久化后才交付链接。过期且无法恢复的意图标记失败，不在原键下无限创建新会话。参考 [Stripe 幂等请求](https://docs.stripe.com/api/idempotent_requests)。

Webhook、用户返回页查单及后台补偿共用服务端核验路径。核对 Session ID、服务端订单引用、币种、金额、支付模式和支付状态，查询完整的退款/争议列表后，再在一个事务中更新订单和余额。数据库租约与版本号隔离跨实例并发，过期请求无法覆盖新核验结果。

后台每分钟尝试恢复已保存的失败事件及到期订单，每轮最多读取 20 个事件、20 个订单，轮次预算 45 秒；待支付查单至少间隔 5 秒，已入账订单按小时复核退款和争议。故障写入 `last_sync_error` 或 `stripe_events.last_error` 并记录 `payment` 日志；恢复不依赖用户始终停留在返回页。参考 [Stripe 履约指南](https://docs.stripe.com/checkout/fulfillment?ui=full-page)。

## 退款和争议的余额规则

待处理退款预留对应额度，成功后转为退款；退款失败或取消释放预留。开放、审核中及败诉的争议预留对应额度，胜诉、预防成功或询问关闭后释放。成功退款仍然保留扣减。同一原订单的退款与争议合计最多扣留该订单已经入账的本金，重复事件不会重复调整，迟到的付款成功通知也不会恢复已退款金额。

争议金额可能因汇率变化超过原付款。非 CNY 争议先预留本单全部本金，保留 Stripe 原始金额和币种供核对；不自行假定汇率。参考 [Stripe 对争议金额差异的说明](https://support.stripe.com/questions/amount-of-customer-dispute-is-greater-than-original-charge)。

已消费的额度发生退款时，余额可以变为负数，普通用户的计费请求由现有余额检查拒绝；补足余额后恢复。支付手续费与争议手续费不加入用户本金余额。手动锁定的账户不会因退款失败或争议胜诉被自动解锁。订单状态区分 `pending`、`paid`、`expired`、`failed`、`refund_pending`、`partially_refunded`、`refunded`、`disputed`。

退款和争议在 Stripe 后台发起或处理，网关负责接收状态并调整额度。参考 [Stripe 退款事件](https://docs.stripe.com/refunds#refund-events)、[争议对象及状态](https://docs.stripe.com/api/disputes/object)。

## 部署

Compose 通过网关服务的 `env_file: [.env]` 读取变量。Swarm 在 `docker-stack.yml` 显式透传支付变量和公网地址；在管理节点的部署环境中设置这些变量，再执行 `docker stack deploy`。Swarm 不会自动读取主机 `.env`。

Helm 启用 `payments.enabled=true`，设置 `payments.publicURL`、`payments.existingSecret`，以及需要的 `payments.mode`、`payments.minRechargeCNY`、`payments.successURL`。引用的 Secret 包含 `api-key` 和 `webhook-secret`，模板不生成或保存支付密钥。

原生 Kubernetes 清单可读取 Secret `airoute-stripe` 的相同两项，以及 ConfigMap `airoute-config` 的 `public-url`。不配置 Secret 时支付关闭；两个密钥只配置一个会拒绝启动。

`STRIPE_API_BASE` 仅用于显式 test 模式的回环 HTTP/HTTPS 模拟服务器，正式收款使用 `https://api.stripe.com`。自定义支付 API 地址不会接收生产密钥。

## 验证

账号无关的测试覆盖幂等创建、响应丢失、数据库失败、漏通知恢复、部分/全额退款、争议恢复、负余额、环境与 API 版本拒绝，以及重复/乱序通知。支付仓储场景在 SQLite 和 PostgreSQL 运行同一组验证。

```bash
go test -race ./internal/controlplane ./internal/storage -run 'TestPayment|TestStripeSignature'
npm --prefix web test -- src/components/WalletManagementView.test.jsx src/components/FunctionalJourneys.test.jsx src/functional-audit.test.jsx
bash scripts/test-payments-postgres.sh
```

PostgreSQL 脚本创建临时 PostgreSQL 17 容器，使用临时内存卷及随机回环端口，结束后删除容器；不会读取项目的 `DATABASE_URL` 或复用项目数据。

托管 Checkout、商户 CNY/card 能力、3DS、外部 HTTPS 通知和退款完整往返仍需在持有账号后，按 [Stripe 测试文档](https://docs.stripe.com/testing)使用独立测试环境验证。
