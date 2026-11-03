# 支付审查：2026-11-02

此文件保留修复前的审查基线。2026-11-03 的实现已按新支付流程修正，使用[支付配置与运行说明](../../PAYMENTS.md)及[修复记录](../payment-fixes-2026-11-03.md)。

结论：Stripe Checkout 的收款与入账主路径方向正确，验签和数据库并发判重已有有效实现；退款/拒付、测试通道隔离、漏通知补偿仍未闭环。建议先处理下述 P1，再补齐 P2，之后用独立 Stripe 测试账号验收。当前没有支付账号，不影响本次代码审查与本地验证，但无法据此确认真实 Stripe 集成已经可用。

审查基线为 `7c695cd746985eadccfb2ff930bc0ca14361849a`。范围包含 Stripe 会话创建、Webhook、充值与兑换入账、前端返回页及部署清单；未修改支付业务代码。本次新增文件均为审查说明或可重复运行的隔离验证程序。

## 已确认正确的部分

| 环节 | 代码与验证 |
| --- | --- |
| 服务端确定入账 | 前端返回参数不会直接加余额；后端仅接受签名验证通过且支付状态为 `paid` 的目标 Checkout 事件。见 [payment_webhook.go](../../../internal/controlplane/payment_webhook.go#L88)。 |
| 验签 | 对原始请求体计算 HMAC-SHA256，使用常量时间比较，支持多个 v1 签名，检查五分钟时间窗；生产运行未配置密钥时拒绝通知。已有验签回归通过。 |
| 订单匹配 | 同时检查本地渠道、Session ID、币种和金额；不会仅凭订单号入账。见 [payment_webhook.go](../../../internal/controlplane/payment_webhook.go#L142)。 |
| 金额单位 | 当前限定 CNY 和两位小数，创建会话使用整数分，余额权威字段采用整数单位。19.99 元对应 1999 分的回归通过。Stripe 对金额使用最小货币单位，见[官方货币文档](https://docs.stripe.com/currencies#minor-units)。 |
| 重复与乱序通知 | 订单判重和钱包增加在同一事务中，通过条件 UPDATE 竞争入账资格；迟到的失败/过期事件不会覆盖已支付状态。见 [repo_wallet.go](../../../internal/storage/repo_wallet.go#L206)。 |
| PostgreSQL 并发 | 64 次并发完成同一订单只入账一次；32 个不同订单并发充值同一钱包没有丢失更新；钱包更新失败会回滚订单；32 次并发兑换同一码仅一次成功。见 [postgres-evidence.json](postgres-evidence.json)。 |
| 权限和失败行为 | 用户查单受所有权约束；Stripe 出错或本地存单失败时不返回付款链接；未配置通道时不会自动降级成模拟充值。相关后端和前端回归通过。 |

这些实现符合 Stripe 对服务端确认、异步事件及幂等履约的核心要求。官方还建议以服务端查询 Session 的公共履约函数供 Webhook 和返回页复用，这部分对应下面的补偿缺口。见[官方 Checkout 履约指南](https://docs.stripe.com/checkout/fulfillment?ui=full-page)。

## 需要修复的问题

### 1. [P1，配置条件成立时] 发布模式的沙箱可以被普通用户调用

位置：[payment_handler.go:222](../../../internal/controlplane/payment_handler.go#L222)、[payment_handler.go:266](../../../internal/controlplane/payment_handler.go#L266)、[router.go:185](../../../internal/api/router.go#L185)。

`ENABLE_SANDBOX_RECHARGE=true` 会在发布模式放行沙箱，路由只要求登录，未要求管理员身份；还允许通过 GET 改余额。创建订单的错误被忽略，然后直接按客户端给出的订单号完成订单，没有检查既有订单渠道。

已用正常编译的可执行文件、发布模式和普通用户有效令牌复现：该用户已有的 50 元未支付 Stripe 订单，通过 GET 沙箱接口变为 `paid`，余额从 10 增至 60，全程没有支付平台请求。开关为 false 的同一程序返回 403，因此这是明确依赖配置的缺口，不代表默认部署已开放此能力。普通 `go test` 程序名以 `.test` 结尾，会触发代码中的另一条放行分支，不能用于证明发布构建的开关行为。

建议生产支付实例不注册模拟充值路由；联调使用独立实例和数据库。保留开发沙箱时，要求管理员、POST、合法金额和订单归属/渠道检查，创建订单失败立即返回。仅用 `.env.example` 的“生产必须 false”注释无法约束运行行为。

### 2. [P1] 退款和拒付通知被确认接收，余额及使用权限未处理

位置：[payment_webhook.go:130](../../../internal/controlplane/payment_webhook.go#L130)、[repo_wallet.go:163](../../../internal/storage/repo_wallet.go#L163)。

处理器只分派四种 Checkout 事件，退款和争议事件都会落到最终的 HTTP 200。订单也未保存 PaymentIntent/Charge 关联、退款金额或争议处理状态。

已复现：50 元充值入账后，投递有效签名的退款成功事件、全额退款的 Charge 事件及争议创建事件，均返回 200；余额仍为 60，订单仍为 `paid`，用户仍为 `active`。如果运营在 Stripe 后台退款，或持卡人发起拒付，本地额度将继续可用。Stripe 会通过事件报告退款状态，发生争议时还可能扣减商户资金。见[官方退款事件说明](https://docs.stripe.com/refunds#refund-events)、[官方争议生命周期](https://docs.stripe.com/disputes/how-disputes-work)。

建议在订单保留支付关联和退款/争议处理记录，确定退款后的额度回收与余额不足策略、争议期间的冻结或人工处置策略。对退款 ID、争议 ID 及累计金额做幂等处理，覆盖部分退款、重复事件、退款失败和争议撤销。处理通知时应有实际状态更新或可追踪的人工任务。

### 3. [P2] 漏通知后，前端查单不能恢复到账

位置：[payment_handler.go:192](../../../internal/controlplane/payment_handler.go#L192)、[WalletManagementView.jsx:163](../../../web/src/components/WalletManagementView.jsx#L163)。

用户查单只读本地数据库，前端最多轮询约两分钟。没有查询 Stripe Session 的后端补偿，也没有针对充值订单的定时对账。现有模型调用费用的 billing reconcile worker 不覆盖收款订单。

模拟平台端已支付但通知未投递，本地查单三次均保留 `pending`，没有任何平台查询请求。这证明查单端点不提供恢复能力；没有用这三次轮询推断长时间故障。Stripe 的生产通知会自动重试最多三天，但重试不是永久恢复机制，官方提供漏事件恢复流程。见[官方漏事件处理](https://docs.stripe.com/events/manage-webhook-endpoints#process-undelivered-events)。

建议返回页及后台补偿任务从本地订单取 Session ID，服务端查询 Stripe，核对渠道、金额、币种、环境和支付状态后调用同一个幂等入账函数；对长时间未决订单记录告警和人工恢复入口。浏览器只传订单标识，不提供可信支付结论。

### 4. [P2] 创建 Checkout 缺少业务幂等，远端成功与本地落单之间无法恢复

位置：[payment_handler.go:57](../../../internal/controlplane/payment_handler.go#L57)、[payment_handler.go:114](../../../internal/controlplane/payment_handler.go#L114)、[payment_handler.go:166](../../../internal/controlplane/payment_handler.go#L166)。

每次请求重新生成订单号；没有处理用户请求的业务幂等标识，也未向 Stripe 发送 `Idempotency-Key`。相同请求及相同客户端幂等头重试两次，已复现创建两个远端会话和两个本地订单。创建会话本身不会扣款，但多个可支付会话会增加重复支付及客服核对成本。Stripe 为 POST 提供幂等键，见[官方幂等请求文档](https://docs.stripe.com/api/idempotent_requests)。

当前先创建远端 Session，再插入本地订单。另一个复现中，本地订单表不可写，远端已创建一个 Session，接口返回 500 且确实没有交付付款链接；代码没有取消该会话或保留可恢复记录。不能据此断言客户已经扣款，但“未产生有效账单”的错误提示无法准确表达远端状态。

建议先持久化充值意图，按用户、业务幂等键及金额/币种建立约束，以稳定订单标识调用 Stripe 幂等创建，再回写会话并交付链接。网络超时和回写失败使用可恢复状态，支持查询已有会话或主动过期，避免每次重试创建新意图。

### 5. [P2，部署范围] Swarm 与 Helm 清单没有提供 Stripe 配置注入

位置：[docker-stack.yml:81](../../../docker-stack.yml#L81)、[Helm deployment.yaml:51](../../../helm/airoute/templates/deployment.yaml#L51)。

Compose 的网关服务使用 `env_file: [.env]`，可以接收这些支付变量；仓库 Swarm 清单的容器 environment 未列出任何 `STRIPE_*`，Helm 也没有支付 Secret 或通用额外环境变量入口。因此按这些默认清单部署时，只填写主机 `.env` 或其他未被模板引用的值不会启用支付。

这是部署清单的静态结论，未检查用户当前服务的真实环境。建议为相应清单增加明确的密钥及配置入口，文档说明支付通知地址、事件类型、所需环境变量和回调公网地址，并在隔离环境检查容器实际收到的变量名。密钥值放 Secret 或部署环境。

### 6. [P2，配置条件成立时] 缺少测试支付与真实额度的环境匹配校验

位置：[wallet_handler.go:254](../../../internal/controlplane/wallet_handler.go#L254)、[payment_webhook.go:112](../../../internal/controlplane/payment_webhook.go#L112)。

支付入口只检查两个环境变量非空，Webhook 未读取 `livemode`。发布模式下，使用对应的测试签名密钥发送 `livemode=false` 的匹配付款事件，已复现给同一钱包加余额。该问题需要配置测试 API key/测试通知密钥或混用测试数据；没有发现绕过有效签名的方式。

建议显式配置支付环境，校验密钥、会话及事件环境，测试实例使用独立数据和模型额度。Stripe 自身隔离两类对象，但应用需要保护自己的余额。见[官方测试与生产模式说明](https://docs.stripe.com/keys#sandbox-versus-live-mode)。

## 其他兼容性事项

- **API 版本未固定。** 手写 HTTP 请求未设置 `Stripe-Version`，也未使用固定版本的 Stripe Go SDK，行为随账号默认版本变化。Webhook 版本同样需要配置并验证。建议使用受支持的固定 SDK 版本，或明确固定 HTTP/API 版本；现有自写 HMAC 的算法本身未发现错误。见[官方升级与版本固定建议](https://docs.stripe.com/upgrades#define-the-target-version-for-your-upgrade)。
- **最低充值额尚未匹配实际商户结算币种。** 前后端业务校验允许 0.01 元，Stripe 卡支付最低金额依赖结算币种及换汇。应以商户实际能力配置下限，不能直接认定 CNY 统一最低金额为某个固定值。见[官方最低收款金额](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts)。
- **自定义成功返回 URL 丢失自动确认上下文。** 配置 `STRIPE_SUCCESS_URL` 后直接原样发送，未自动附加当前 `order_no`；当前前端需要 `recharge=success` 与订单号才会轮询。资金仍可由 Webhook 正常入账，但会影响支付返回体验。见 [payment_handler.go:79](../../../internal/controlplane/payment_handler.go#L79) 和 [WalletManagementView.jsx:169](../../../web/src/components/WalletManagementView.jsx#L169)。
- **当前实际只请求 card。** 没有支付宝/微信支付实现；是否增加其他支付方式，应根据商户可用渠道和产品目标决定，不属于当前卡支付的正确性缺陷。

## 社区实现对照

本次直接阅读了同类项目 New API 的 [Stripe 控制器](https://github.com/QuantumNous/new-api/blob/main/controller/topup_stripe.go) 和[充值模型](https://github.com/QuantumNous/new-api/blob/main/model/topup.go)。它使用 Stripe Go SDK、最低充值限制、金额计算与额度上限保护、回跳域名检查，并在数据库事务中校验渠道、锁定订单、更新额度。本项目的事务判重已具备类似的核心保护，实际 PostgreSQL 并发验证也通过。可借鉴其 SDK 和业务约束；它的会话创建路径也先调用 Stripe 再插入订单，因此本报告没有把“社区也这样写”当作失败恢复已经完善的证据。

另阅读了 [Stripe 官方 Go Checkout 示例](https://github.com/stripe-samples/checkout-one-time-payments/blob/main/server/go/server.go)，用于核对会话 API、环境密钥和官方验签函数的用法。该示例未提供完整的钱包、退款和订单补偿逻辑，适合作为接口样例。

阅读日期为 2026-11-02。上述三个源文件的 Git blob SHA 分别为 `35b1bcf09fb9524b9b7755a7ab0679206921cd7c`、`17d4df35c3c35e3aabc4ed8b238b1f77658909e9`、`44dd0c5a21e7dabdc3f2d3b1c4e261047f50c013`，用于标识本次阅读内容。

## 基线验证记录

既有支付、充值与兑换相关 Go 用例共 14 个通过；三个前端测试文件共 29 个用例通过。另运行六组账号无关的边界复现，以及四组本地 PostgreSQL 并发/回滚验证。正常回归通过和缺口复现成功是不同结果；`evidence.json` 的 `issue_reproduced=true` 表示现有缺口确实出现。

| 内容 | 入口与证据 |
| --- | --- |
| 六组边界复现 | [原始输出](evidence.json)。内存 SQLite、真实应用路由与认证、合成密钥，拦截全部支付 HTTP，不请求 Stripe。 |
| PostgreSQL 四组验证 | [原始输出](postgres-evidence.json)。临时 PostgreSQL 17 容器，仅绑定随机回环端口，使用内存临时卷，运行后删除。 |

原来的缺口演示程序已转换为正式[支付回归](../../../internal/controlplane/payment_lifecycle_test.go)和[仓储并发回归](../../../internal/storage/payment_lifecycle_test.go)。当前验证入口见[支付说明](../../PAYMENTS.md#验证)，PostgreSQL 使用[隔离测试脚本](../../../scripts/test-payments-postgres.sh)。

仍需 Stripe 账号才能验收：商户地区及 CNY/card 能力、真实 API 版本兼容性、托管 Checkout 与 3DS、外部 HTTPS 通知可达性、退款/拒付的完整往返，以及实际部署的密钥配置。届时先使用独立 Stripe 测试环境和隔离钱包按[官方测试流程](https://docs.stripe.com/testing)验收；本次本地结果不能替代这一步。
