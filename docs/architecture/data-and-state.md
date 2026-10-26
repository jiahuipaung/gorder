# 数据所有权与状态模型

[返回文档地图](../README.md) · [核心业务流程](business-flow.md)

本文回答三个问题：每类业务数据由谁负责、事件承载什么语义、订单状态迁移在哪里被约束。
结论来自当前分支代码，而不是理想化领域模型。

## 1. 数据所有权

```mermaid
flowchart LR
    Order[Order Service] -->|owns| Mongo[(MongoDB orders)]
    Stock[Stock Service] -->|owns current quantity| MySQL[(MySQL o_stock)]
    Payment[Payment Service] -->|creates sessions / receives result| Stripe[Stripe]
    Order -->|publishes notification| MQ[(RabbitMQ)]
    Payment -->|publishes notification| MQ
    MQ --> Order
    MQ --> Payment
    MQ --> Kitchen[Kitchen Worker]
```

| 数据 | 当前写入者 | 当前读取者 | 权威与限制 |
|---|---|---|---|
| 订单 ID、客户 ID、状态、支付链接、商品快照 | Order | Order API、Order consumer、经 gRPC 的 Payment/Kitchen | MongoDB 是仓库内的持久化权威 |
| 商品当前库存数量 | Stock | Stock query | MySQL `o_stock` 是当前数量来源，但只读不扣减 |
| 商品支付 Price ID | Stripe Product API | Stock | 调用时动态读取，没有本地商品主数据表 |
| Checkout Session 与支付结果 | Stripe | Payment | Provider 是支付事实来源；本地没有支付交易表 |
| 创建/支付通知 | Order、Payment | Payment、Order、Kitchen | RabbitMQ 负责传递，不是业务状态权威 |
| 服务实例地址 | Order、Stock 注册到 Consul | gRPC client | 只用于运行时发现，不属于业务数据 |

### 1.1 为什么不存在全局事务

一笔流程会依次触及 MySQL、Stripe、MongoDB 和 RabbitMQ。它们没有共享事务管理器，代码也
没有实现两阶段提交。因此当前只能在每个系统内部获得局部原子性：

- MongoDB 单次条件更新可以原子判断并推进已建模的订单状态；
- MySQL 查询可以读取当前数量，但没有把数量变化纳入订单事务；
- RabbitMQ 的持久化消息与手动 ACK 管理 delivery，但不能和 MongoDB 提交组成原子动作；
- Stripe webhook 告知 Provider 状态，但本地接收与消息发布之间也不是同一事务。

正确性设计因此应围绕业务不变量、幂等、持久化事件记录和对账展开，而不是假设存在全局
ACID 事务。

## 2. Order 持久化模型

MongoDB model 当前保存：

| 字段 | 来源/用途 | 当前约束 |
|---|---|---|
| `_id` | MongoDB ObjectID；对外转换为订单 ID | `Get` 与三个状态迁移命令的主定位字段 |
| `id` | model 中存在，但创建时没有单独赋值 | 当前读取逻辑使用 `_id`，不依赖该字段 |
| `customer_id` | 创建订单请求 | 与 `_id` 一起隔离客户订单 |
| `status` | 创建时为 `pending`；后续显式命令推进 | 三条迁移都以预期旧状态作为更新条件 |
| `payment_link` | `AttachPaymentSession` 写入 Stripe URL | 同链接重放 no-op；不同链接拒绝覆盖；paid event 空链接保留本地值 |
| `items` | Stock 校验后返回的订单商品快照 | 不是实时库存，也不代表已预占 |

Order repository 暴露五类操作：

- `Create`：插入订单并返回 `_id`；
- `Get`：用 `_id + customer_id` 查询；
- `AttachPaymentSession`：`pending -> waiting_for_payment`，同时挂接非空链接；
- `MarkPaid`：`waiting_for_payment -> paid`；
- `MarkReady`：`paid -> ready`。

## 3. 当前真实状态模型

### 3.1 代码中显式定义的状态

Order domain 定义四个主流程状态：

```text
pending
waiting_for_payment
paid
ready
```

`Order.IsPaid()` 验证支付事件对象是否为 `paid`。状态规则没有放进一个通用
`CanTransitionTo`，而是由三个意图明确的 repository command 分别约束；这避免保留任意目标
状态的通用写入口。

### 3.2 运行路径中观察到的状态

```mermaid
stateDiagram-v2
    [*] --> Pending: Order Create
    Pending --> WaitingForPayment: AttachPaymentSession
    WaitingForPayment --> WaitingForPayment: exact session replay / no-op
    WaitingForPayment --> Paid: Order MarkPaid conditional update
    Paid --> Paid: repeated paid event / no-op
    Paid --> Ready: MarkOrderReady
    Ready --> Ready: repeated ready command / no-op
```

三条首次迁移都通过一个带 `_id + customer_id + expected_status` filter 的 MongoDB
`UpdateOne` 完成。条件不命中时再读取当前值：精确重放返回 `changed=false, nil`，订单不存在
或非法状态分别返回明确错误。`AttachPaymentSession` 只有当前链接与请求链接相同才算精确
重放，不同链接不会覆盖先前结果。

## 4. 显式命令与写入边界

| 命令 | 调用者 | 条件 | 重放语义 |
|---|---|---|---|
| `AttachPaymentSession` | Payment gRPC adapter | `status=pending` | 已 waiting 且链接相同为 no-op；不同链接拒绝 |
| `MarkPaid` | Order `order.paid` consumer | `status=waiting_for_payment` | 已 paid 为 no-op |
| `MarkOrderReady` | Kitchen gRPC adapter | `status=paid` | 已 ready 为 no-op |

protobuf 不再暴露 `UpdateOrder(Order)`。Payment 只能发送订单 ID、客户 ID 与支付链接；Kitchen
只能发送订单 ID 与客户 ID。Order repository 也移除了接收任意 aggregate 和 update function 的
`Update`。此前未实际使用 session context 的 Mongo 伪事务随之删除；单文档条件更新已经提供
这里需要的原子边界。

## 5. 库存数据模型

MySQL 初始化脚本只创建 `o_stock`：

```text
id, product_id, quantity, created_at, updated_at
```

当前 `CheckIfItemsInStock`：

1. 批量读取请求商品的 `quantity`；
2. 把商品 ID 映射到当前数量；
3. 请求数量大于当前数量时返回 `ExceedStockError`；
4. 数量足够时，从 Stripe Product API 取得 Price ID；
5. 返回用于创建订单的商品快照。

它没有写 MySQL，源码保留了“扣库存”的 TODO。`GetItems` 虽然存在 gRPC 契约和 query
handler，但 MySQL repository 对应方法仍为 `panic("implement me")`。

因此 `quantity` 目前只能解释为“检查时读取到的可用数量”，不能解释为已售、已锁、在途、
预占或最终库存。

## 6. 事件语义

### 6.1 `order.create`

| 属性 | 当前实现 |
|---|---|
| 发布者 | Order `CreateOrder` handler |
| 消费者 | Payment consumer |
| 消息体 | 创建后完整 Order JSON |
| 传输 | 发布到默认 exchange，以 `order.create` 为 routing key/queue name |
| 持久性 | `DeliveryMode=Persistent`，队列声明为 durable |
| 业务身份 | 没有独立 event ID；消息也未显式设置 `MessageId` |
| trace | 注入 OpenTelemetry propagation headers |

这条消息的业务含义是“订单记录已创建，请尝试建立支付会话”，不是“支付一定已创建”。

### 6.2 `order.paid`

| 属性 | 当前实现 |
|---|---|
| 发布者 | Payment Stripe webhook handler |
| 消费者 | Order consumer、Kitchen consumer |
| 消息体 | 从 Stripe Checkout Session metadata 还原的 Order-like JSON |
| 传输 | `order.paid` fanout exchange |
| 持久性 | `DeliveryMode=Persistent`；consumer queue 的生命周期配置并不完全一致 |
| 业务身份 | Stripe event ID 没有进入 payload/Inbox，`MessageId` 未显式设置 |
| trace | webhook 发布时注入 propagation headers，consumer 提取 |

这条消息当前同时承担两个意图：推进 Order 支付状态、触发 Kitchen 履约。由于 fanout
consumer 独立确认，一个 consumer 成功不代表另一个成功。

## 7. 数据一致性边界总结

| 边界 | 当前保证 | 当前不保证 |
|---|---|---|
| Order 单文档主流程迁移 | 三条条件更新原子；精确重放为 no-op | 取消、退款、履约中等更完整状态机 |
| Order 与消息 | 消息可持久化、手动确认 | MongoDB commit 与 publish 原子性 |
| Stock 与 Order | 创建前同步检查当前数量 | 预占、扣减、超卖防护、订单取消释放 |
| Stripe 与本地订单 | Checkout 以订单 ID 幂等创建；签名 webhook 能触发本地事件 | 幂等键保留期之外的对账、publish 失败恢复、退款和争议 |
| Kitchen 与 Order | `paid -> ready` 有原子前置条件和重放 no-op | `cook` 履约副作用幂等 |

下一步阅读[可靠性模型](reliability.md)，可以看到这些边界怎样转化为明确的不变量和失败
窗口；[架构演进路线](evolution.md)则说明补齐顺序。
