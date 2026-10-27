# Consumer correctness 实验

[返回文档地图](../README.md) · [核心业务流程](../architecture/business-flow.md) ·
[可靠性模型](../architecture/reliability.md)

## 1. 实验目的

本实验验证 RabbitMQ consumer 和 Stripe webhook 的**进程内控制流契约**：同一条 delivery
最多选择一个 ACK/NACK 终态，下游错误能够回到消费层，替代消息发布失败时保留原消息，且
webhook 不会在业务事件发布失败时向 Stripe 返回成功。

它不验证真实 Broker 的持久化、publisher confirms、网络断连恢复或 Stripe 的实际重试调度。

## 2. 故障假设

本切片针对五个由修复前源码可以直接观察到的故障：

1. Payment consumer 在 NACK 后继续执行并 ACK 同一条 delivery；
2. `CreatePayment` 吞掉 Order gRPC error，consumer 看不到失败；
3. Order consumer 在 retry publish 失败后 `Nack(requeue=false)`，可能丢弃唯一副本；
4. Kitchen 依赖共享 `err` 和 defer 决定终态，畸形消息可能 NACK 两次；
5. Stripe webhook 忽略 RabbitMQ publish error 并返回 HTTP 200。

修复后的目标不是 exactly-once，而是让当前 delivery 的决策可预测、可测试，并为后续
Inbox/Outbox 提供清晰输入。

## 3. Delivery 决策表

| 输入或处理结果 | retry/DLQ 行为 | 原 delivery 终态 | 自动化证据 |
|---|---|---|---|
| JSON 无法解析 | 不调用 | `Nack(requeue=false)` | Payment、Order、Kitchen consumer tests |
| Order/Kitchen 状态不是 `paid` | 不调用 handler/retry；Kitchen 也不 cook/gRPC | `Nack(requeue=false)` | Order、Kitchen consumer tests |
| handler/downstream 成功 | 不调用 | `Ack` | 三个 consumer tests |
| handler/downstream 失败，retry publish 成功 | 调用一次 | `Ack` | 三个 consumer tests |
| handler/downstream 失败，retry publish 失败 | 调用一次 | `Nack(requeue=true)` | 三个 consumer tests |

这里 ACK 的含义是“当前 delivery 已处理，或已经由替代消息接管”，不是业务最终成功，也
不是 Broker 已经持久化替代消息的证明。

## 4. 可复现命令

所有命令都不需要启动 RabbitMQ、MongoDB、MySQL、Stripe 或业务进程。

### 4.1 Payment command 与 consumer

```bash
cd internal/payment
go test -count=1 ./app/command ./infrastructure/consumer
```

预期观察：

- nil order 在调用 Stripe 或 Order gRPC 前返回 error；
- Stripe error 与 Order gRPC error 都由 command 返回；
- Payment consumer 的成功、畸形、retry 成功、retry 失败路径确认次数均为 1；
- retry publish 失败使用 `requeue=true`。

### 4.2 Order consumer

```bash
cd internal/order
go test -race -count=1 ./infrastructure/consumer
```

预期观察：成功与 retry 交接成功 ACK 原消息；畸形或非 `paid` 消息 NACK 且不 requeue；retry
交接失败 NACK 并 requeue。所有路径的 ACK/NACK 调用总数均为 1，race detector 无报告。

### 4.3 Kitchen consumer

```bash
cd internal/kitchen
go test -count=1 ./infrastructure/consumer
```

预期观察：

- 畸形和非 `paid` 消息不会 cook、调用 Order gRPC 或 retry；
- 合法消息只 cook 一次并调用 Order gRPC 一次；
- retry 成功 ACK，retry 失败 requeue NACK；
- 测试注入无延迟 cook handler，不执行生产代码中的五秒 sleep。

### 4.4 Stripe webhook

```bash
cd internal/payment
go test -count=1 -run 'Test(PaidWebhook|Webhook)' .
```

测试使用 stripe-go 的 `webhook.GenerateTestSignedPayload` 生成合法签名，不绕过验签逻辑：

| 场景 | 预期 HTTP | publish 次数 |
|---|---:|---:|
| signed paid event，publish 成功 | 200 | 1 |
| signed paid event，publish 返回 error | 503 | 1 |
| 非法签名 | 400 | 0 |

### 4.5 仓库级验证

```bash
make verify
docker compose config --quiet
git diff --check main..HEAD
```

`make verify` 会测试 Common、Payment、Stock、Kitchen，并对支付重投相关 Order 包启用 race
detector。

## 5. 测试设计边界

测试保留真实 JSON 解析、command handler 边界、AMQP `Delivery` 确认接口、Gin route 和 Stripe
签名验证；只替换必须隔离的外部或慢依赖：

- `RetryHandler` 替代真实 RabbitMQ republish；
- `Publisher` 替代真实 AMQP channel；
- `CookHandler` 替代五秒 sleep；
- Payment processor 与 Order gRPC 使用窄 stub 注入错误。

因此这些测试能够证明当前 Go 函数在给定返回值下怎样分支，不能证明 RabbitMQ/Stripe 的
远端行为。

## 6. 已知限制与下一步

- 没有 publisher confirms，`PublishWithContext` 返回 nil 不等于 Broker 已持久化；
- publish 使用 `mandatory=false`，无法路由的消息没有应用级 return 处理；
- 没有稳定 `event_id`、Provider event ledger 或 consumer Inbox；
- 没有 Transactional Outbox，数据库提交与消息发布仍有双写窗口；
- Payment 重投可能重复创建 Stripe Checkout Session；
- Kitchen 在 cook 后、订单更新或 ACK 前失败会重复履约副作用；
- retry publish 持续失败会 requeue 原消息，可能形成热循环，需要退避、隔离与告警；
- ACK/NACK 自身的返回错误目前只由 AMQP 连接语义兜底，没有应用级恢复记录。

下一阶段应优先引入稳定事件契约、Payment event ledger/Outbox 和各 consumer 的 Inbox，而
不是把本实验扩大解释为“消息不丢”或“全链路 exactly-once”。
