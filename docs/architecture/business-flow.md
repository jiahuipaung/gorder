# 核心业务流程

[返回文档地图](../README.md) · [系统架构总览](overview.md) ·
[数据所有权与状态模型](data-and-state.md)

本文沿着当前代码实际执行路径，说明创建订单、创建支付、处理支付结果和模拟履约四段流程。
每段都把成功路径与直接可见的失败窗口放在一起，避免“流程图能走通”等同于“异常已经
闭环”。

## 1. 总体事件链

```mermaid
flowchart LR
    A[Client 创建订单] --> B[Order 查询 Stock]
    B --> C[Order 写 MongoDB]
    C -->|order.create| D[Payment 创建 Stripe Checkout]
    D --> E[Payment 经 gRPC 回写支付链接]
    E --> F[Client 在 Stripe 完成支付]
    F --> G[Payment 验证 webhook]
    G -->|order.paid| H[Order 原子标记 paid]
    G -->|同一 fanout event| I[Kitchen 模拟履约]
    I --> J[Kitchen 经 gRPC 写入 ready]
```

这条链路没有跨 MongoDB、MySQL、RabbitMQ 与 Stripe 的全局事务。当前最完整的可靠性闭环
是 `order.paid` 重复投递时的 Order 终态幂等；其他阶段的失败窗口仍需显式处理。

## 2. 创建订单

### 2.1 当前成功路径

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant OrderHTTP as Order HTTP
    participant OrderApp as CreateOrder handler
    participant Stock as Stock gRPC
    participant MySQL
    participant Stripe as Stripe Product API
    participant Mongo
    participant MQ as RabbitMQ

    Client->>OrderHTTP: POST /api/customer/{id}/orders/
    OrderHTTP->>OrderApp: customer_id + items
    OrderApp->>OrderApp: 合并重复商品 ID 的数量
    OrderApp->>Stock: CheckIfItemsInStock(items)
    Stock->>MySQL: BatchGetStockByID(ids)
    MySQL-->>Stock: 当前数量
    Stock->>Stripe: Get product default Price ID
    Stripe-->>Stock: Price ID
    Stock-->>OrderApp: validated items
    OrderApp->>Mongo: InsertOne(order)
    Mongo-->>OrderApp: ObjectID
    OrderApp->>MQ: persistent order.create
    OrderApp-->>OrderHTTP: order_id
    OrderHTTP-->>Client: order_id + local success URL
```

具体步骤：

1. HTTP handler 解析请求，把客户 ID 和商品数量交给 `CreateOrder`；
2. `packItems` 按商品 ID 合并重复项；
3. Order 同步调用 Stock 的 `CheckIfItemsInStock`；
4. Stock 从 MySQL 读取当前数量，逐项比较请求数量；
5. Stock 再从 Stripe Product API 获取默认 Price ID，返回用于支付的商品快照；
6. Order 把订单插入 MongoDB，使用 MongoDB ObjectID 的十六进制字符串作为公开订单 ID；
7. Order 将完整订单 JSON 作为持久化 `order.create` 消息发布到同名队列；
8. HTTP 返回订单 ID 和本地成功页 URL。

### 2.2 当前语义边界

- Stock 的“有库存”只表示检查时的当前数量足够，不会锁定或扣减数量；
- 新插入订单没有显式初始状态，`waiting_for_payment` 在后续 Payment 流程中写入；
- Stock 获取某个商品 Price ID 失败时会记录 warning 并跳过该商品，当前不会让整个检查立即失败；
- OpenAPI 路径包含 `customer_id`，创建 handler 实际使用 request body 中的 `customer_id`，当前没有
  对两个来源做一致性校验。

### 2.3 失败窗口

| 位置 | 当前结果 | 风险 |
|---|---|---|
| Stock/MySQL/Stripe 调用失败 | 创建订单返回错误 | 没有订单落库；客户端可重试，但没有请求幂等键 |
| MongoDB 写入失败 | 不发布消息，创建失败 | 结果清晰 |
| MongoDB 写入成功，`order.create` 发布失败 | API 返回错误，但订单已经存在 | 客户端重试可能创建第二笔订单；原订单不会自动进入支付流程 |
| 消息发布成功，响应返回前进程退出 | 客户端可能认为失败 | 重试可能创建重复订单；没有创建请求去重记录 |

核心问题是 MongoDB 插入和 RabbitMQ 发布是两个独立操作。当前没有 Transactional Outbox，
因此“订单已保存”和“支付流程一定会启动”不是一个原子承诺。

## 3. 创建支付链接

### 3.1 当前成功路径

```mermaid
sequenceDiagram
    autonumber
    participant MQ as RabbitMQ order.create queue
    participant Consumer as Payment consumer
    participant PaymentApp as CreatePayment handler
    participant Stripe as Stripe Checkout API
    participant OrderGRPC as Order gRPC
    participant Mongo

    MQ->>Consumer: deliver order.create
    Consumer->>PaymentApp: CreatePayment(order)
    PaymentApp->>Stripe: create Checkout Session
    Stripe-->>PaymentApp: payment URL
    PaymentApp->>OrderGRPC: UpdateOrder(waiting_for_payment, URL)
    OrderGRPC->>Mongo: generic Update
    Mongo-->>OrderGRPC: success or error
    OrderGRPC-->>PaymentApp: success or error
    Note over PaymentApp: 当前实现未向 consumer 返回 gRPC error
    PaymentApp-->>Consumer: URL + nil error
    Consumer->>MQ: ACK
```

Stripe Checkout Session metadata 保存 `orderID`、`customerID`、商品 JSON、创建会话时看到的状态
和支付链接字段，用于 webhook 阶段还原订单引用。创建出新的 URL 后，Payment 通过 Order
gRPC 的通用 `UpdateOrder` 写入：

```text
status = waiting_for_payment
payment_link = Stripe Checkout URL
```

### 3.2 Consumer 控制流缺口

Payment consumer 当前在以下两条错误路径中执行 `Nack`，但没有立即 `return`：

- `order.create` JSON 解析失败；
- `CreatePayment` 返回错误，并完成重试发布或 DLQ 发布后。

函数随后仍会继续执行 `Ack`。因此该 consumer **不具备** ACK/NACK 互斥保证，可能对同一
delivery 先 NACK 再 ACK。Order 支付结果 consumer 的互斥修复不能外推到这里。

此外，`CreatePayment` 虽然接收到了 Order gRPC 的返回错误，却最终固定返回 `link, nil`。
因此“支付会话已创建但订单支付链接回写失败”不会进入 consumer 的重试分支，而会被当作
成功 ACK。这会使 Provider 已存在可支付会话，而本地订单仍保留旧状态和空支付链接。

### 3.3 其他失败窗口

| 位置 | 当前结果 | 风险 |
|---|---|---|
| Stripe 已创建 Checkout Session，Order gRPC 回写失败 | `CreatePayment` 仍返回 `nil` error，consumer ACK | Provider 与本地订单立即产生状态分叉 |
| Payment 在 gRPC 成功后、ACK 前退出 | Broker 重投 | 再次创建支付链接并覆盖旧链接 |
| 通用 `UpdateOrder` 与其他更新并发 | 读取后按 ID/客户 ID 写入，无版本前置条件 | 较晚写入可能覆盖较新的状态或支付链接 |
| Stripe metadata 构造 | 使用创建 Checkout 前收到的订单字段 | metadata 中 `paymentLink` 是旧值，通常为空 |

## 4. 支付成功与订单终态迁移

### 4.1 Webhook 接收与事件发布

```mermaid
sequenceDiagram
    autonumber
    participant Stripe
    participant Webhook as Payment /api/webhook
    participant MQ as RabbitMQ order.paid exchange
    participant OrderConsumer as Order consumer
    participant OrderApp as MarkOrderPaid handler
    participant Mongo

    Stripe->>Webhook: checkout.session.completed
    Webhook->>Webhook: 限制 body <= 64 KiB
    Webhook->>Webhook: 验证 Stripe-Signature
    Webhook->>Webhook: 确认 session.payment_status == paid
    Webhook->>MQ: persistent order.paid
    Webhook-->>Stripe: HTTP 200
    MQ->>OrderConsumer: deliver paid event
    OrderConsumer->>OrderApp: MarkOrderPaid(order)
    OrderApp->>OrderApp: 要求 event.status == paid
    OrderApp->>Mongo: conditional UpdateOne
    alt 首次有效迁移
        Mongo-->>OrderApp: ModifiedCount = 1
        OrderApp-->>OrderConsumer: changed = true
        OrderConsumer->>MQ: ACK
    else 已经 paid
        Mongo-->>OrderApp: current.status = paid
        OrderApp-->>OrderConsumer: changed = false, nil
        OrderConsumer->>MQ: ACK
    else 非法旧状态/不存在/存储错误
        OrderApp-->>OrderConsumer: error
        OrderConsumer->>MQ: retry publish or DLQ, then NACK original
    end
```

持久化层的条件是：

```text
_id = order_id
customer_id = event.customer_id
status = waiting_for_payment
```

只在条件命中时设置 `status=paid` 和 event 携带的 `payment_link`。若没有修改，repository
重新读取订单：当前已经是 `paid` 时返回成功 no-op，其他状态返回非法迁移错误。

### 4.2 已验证的范围

- 首次 `waiting_for_payment -> paid` 返回一次真实变化；
- 顺序重复投递返回成功 no-op；
- 并发重放只有一个调用观察到真实变化，并在 race detector 下运行；
- `order_id + customer_id` 一起限制更新目标；
- Order consumer 成功只 ACK，解析失败或 handler 失败只 NACK。

可复现命令和测试范围见[支付消息重复投递实验](../experiments/payment-redelivery.md)。

### 4.3 Webhook 与事件缺口

| 位置 | 当前结果 | 风险 |
|---|---|---|
| Stripe event ID | 未写入 `order.paid` 消息，也没有持久化 Inbox | 只能依赖状态迁移吸收这一种重复副作用，不能审计每个 Provider event |
| RabbitMQ publish 返回错误 | 返回值被忽略 | webhook 仍返回 HTTP 200，Stripe 不会因该响应主动重试，支付结果可能永久丢失 |
| metadata `paymentLink` | 来自创建会话前的订单快照 | paid event 通常携带空链接，`MarkPaid` 可能覆盖已经保存的支付链接 |
| metadata 商品 JSON | 序列化的是 Stripe line item params，再反序列化为 order protobuf item | 字段模型不完全对应，履约消息中的商品信息可能不完整 |
| 其他 Stripe event type / 未 paid session | 返回 HTTP 200，不发布业务事件 | 这是当前筛选策略，不是完整支付状态机 |

## 5. 模拟履约

```mermaid
sequenceDiagram
    autonumber
    participant MQ as RabbitMQ order.paid exchange
    participant Kitchen as Kitchen consumer
    participant OrderGRPC as Order gRPC
    participant Mongo

    MQ->>Kitchen: deliver order.paid
    Kitchen->>Kitchen: parse JSON and require status == paid
    Kitchen->>Kitchen: sleep 5s to simulate cook
    Kitchen->>OrderGRPC: UpdateOrder(status = ready)
    OrderGRPC->>Mongo: generic Update
    alt update success
        Kitchen->>MQ: ACK
    else update fails
        Kitchen->>MQ: retry publish or DLQ
        alt retry publish succeeds
            Kitchen->>MQ: defer ACK original
        else retry publish fails
            Kitchen->>MQ: defer NACK original
        end
    end
```

Kitchen 是演示性的履约 worker，不代表真实仓储、发货或制作系统。

当前缺口：

- `cook` 是不可追踪的五秒 sleep，没有独立履约记录或幂等键；
- 若执行完 `cook` 后、Order gRPC 成功前后发生重投，模拟副作用会再次执行；
- 通用 `UpdateOrder` 没有要求旧状态必须为 `paid`，也没有版本条件；
- `ready` 在 Kitchen 中以字符串写入，但 Order domain 没有对应常量或正式迁移方法；
- JSON 解析失败时函数会先显式 NACK，defer 中又依据错误再次 NACK，存在重复确认调用；
- Order gRPC 失败后，defer 最终 ACK 还是 NACK 取决于 `HandleRetry` 是否成功，因为重试结果
  覆盖了原始业务错误；
- Kitchen 使用 server-named fanout queue，运行期行为依赖 RabbitMQ 声明与绑定成功。

## 6. 跨流程不变量

当前代码能够明确表达或验证的不变量只有一部分：

| 不变量 | 当前机制 | 证据状态 |
|---|---|---|
| 支付终态只能从 `waiting_for_payment` 首次迁移到 `paid` | MongoDB 条件更新 | 已验证 |
| 同一支付消息重投不会重复改变订单终态 | 已支付时成功 no-op | 已验证 |
| 支付事件不能更新另一客户的同 ID 订单 | `_id + customer_id` 条件 | 已验证 |
| Order 支付 consumer 一次 delivery 只 ACK 或 NACK | 错误路径立即返回 | 已验证 |
| 创建订单后一定启动支付 | 无对应机制 | 不成立：存在 DB/MQ 双写窗口 |
| 有库存检查成功后一定能履约 | 无预占或扣减 | 不成立 |
| Provider 已支付一定反映到订单 | publish error 被忽略且无对账 | 不成立 |
| 履约副作用只执行一次 | 无履约 Inbox/记录 | 不成立 |

这些不变量和失败窗口会在[可靠性模型](reliability.md)中进一步按“机制—证据—剩余风险”
展开。
