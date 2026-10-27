# Consumer Correctness Design

## Intent

关闭当前三个 RabbitMQ consumer 和 Stripe webhook 中已经由源码确认的控制流缺口，使每条
delivery 只有一个明确终态，使下游错误能够返回到消费层，并使 webhook 只在支付结果事件
成功交给 RabbitMQ 后返回 HTTP 2xx。

这是把 GOrder 做完整的第一段可靠性工作，不在本轮引入 Outbox、Inbox、新数据库或完整
Saga。

## Current Problems

1. Payment consumer 在 JSON 解析失败或 handler 失败后执行 NACK，但没有 return，随后继续
   ACK；
2. `CreatePayment` 收到 Order gRPC 错误后仍返回 `nil`，导致 Payment consumer 无法重试；
3. Kitchen 在 JSON 解析失败时可能 NACK 两次，并用 retry 的返回值覆盖原始业务错误；
4. Order consumer 的 retry publish 失败后仍 `Nack(requeue=false)`，原消息可能没有恢复路径；
5. Stripe webhook 忽略 `PublishWithContext` error，仍返回 HTTP 200；
6. consumer 与 webhook 依赖 concrete AMQP channel，难以无 Broker 验证错误路径；
7. Kitchen 测试如果直接执行 `cook` 会固定等待五秒，副作用也无法计数。

## Goals

- Payment、Order、Kitchen 对同一 delivery 最多调用一次 ACK 或 NACK；
- JSON 非法或业务上不可处理的消息只 NACK 一次且不 requeue；
- handler 成功时 ACK 原消息；
- handler 失败且 retry/DLQ publish 成功时 ACK 原消息，因为替代消息已经交给 retry 路径；
- handler 失败且 retry publish 失败时 NACK 原消息并 `requeue=true`，避免主动丢弃唯一副本；
- `CreatePayment` 对 nil order 返回 error，并透传 Order gRPC error；
- Kitchen 只对合法 `paid` 消息执行一次 `cook`，且测试不等待真实 sleep；
- Stripe paid webhook 发布失败时返回 HTTP 503，成功时才返回 HTTP 200；
- 使用真实 JSON、真实 command/handler 边界和真实 Stripe webhook 签名测试，不需要启动
  RabbitMQ、Stripe、MongoDB 或 HTTP server process；
- 同步更新架构、可靠性和实验文档，使其描述修复后的现状。

## Non-goals

- publisher confirms；
- Transactional Outbox 或持久化 Inbox；
- Provider event ID 的存储与去重；
- Payment-owned database；
- 库存预占、确认或释放；
- Kitchen 真实履约幂等；
- 重构 RabbitMQ topology、延迟队列或 DLQ replay；
- 把所有 consumer 合并进一个通用框架。

## Approaches Considered

### 1. 只补 `return`

在 Payment/Kitchen 的 NACK 后补 return，并让 `CreatePayment` 返回 gRPC error。改动最小，但
retry publish 失败仍会主动丢弃原消息，webhook publish 失败仍可能返回 200，且错误路径缺乏
可测试接口。不能形成完整 delivery contract，因此不采用。

### 2. 显式 delivery outcome + 小接口注入

每个 consumer 保留自己的业务处理逻辑，但统一 terminal semantics；通过小型 `RetryHandler`、
`Publisher` 和 `CookHandler` 接口/函数注入测试依赖。成功 retry 后 ACK 原消息，retry 自身失败
时 NACK 并 requeue。Webhook 对 publish error 返回 503。此方案关闭当前已知窗口，不引入新
基础设施，作为本轮选择。

### 3. 直接建设 Inbox/Outbox

能够提供更强的事件级恢复能力，但要求稳定 event ID、存储模型、worker、事务和故障注入，
范围远大于这轮控制流修复。它仍是后续阶段，不能替代先把 ACK/NACK 和 error propagation
修正确。

## Delivery Contract

三个 consumer 对一条 delivery 使用同一决策表：

| 条件 | Retry | 原 delivery 终态 |
|---|---|---|
| JSON 无法解析 | 不调用 | `Nack(requeue=false)` |
| 业务消息永久无效 | 不调用 | `Nack(requeue=false)` |
| handler 成功 | 不调用 | `Ack` |
| handler 失败，retry/DLQ publish 成功 | 调用一次 | `Ack` |
| handler 失败，retry/DLQ publish 失败 | 调用一次 | `Nack(requeue=true)` |

这里的 ACK 只表示当前 delivery 已处理或已被替代消息接管，不等于业务最终成功。由于当前
没有 publisher confirms，retry publish 成功仍不是端到端“不丢消息”证明；这一限制继续保留
在可靠性文档中。

## Component Changes

### Payment command

`CreatePayment.Handler`：

- nil order 立即返回 error；
- Stripe payment link 创建失败原样返回；
- Order gRPC `UpdateOrder` 失败原样返回；
- 只有 Stripe 与 Order 更新都成功才返回 link 和 nil。

### Payment consumer

- 增加可注入 `RetryHandler`，生产默认使用 `broker.HandleRetry`；
- JSON error 只 NACK 一次并 return；
- handler success ACK；
- handler error 按 delivery contract 处理 retry 成功/失败；
- 不在错误分支之后继续记录 `payment created`。

### Order consumer

- 保留现有 `MarkOrderPaid` 幂等逻辑；
- handler error 时，retry 成功改为 ACK 原 delivery；
- retry 失败改为 `Nack(requeue=true)`；
- malformed 与成功路径保持现有单一确认语义。

### Kitchen consumer

- 移除由共享 `err` 和 defer 隐式决定确认结果的控制流；
- JSON error 与非 `paid` 消息各自只 NACK 一次并 return；
- 增加可注入 `RetryHandler` 与 `CookHandler`；
- production `CookHandler` 仍调用现有五秒模拟；测试 handler 只计数；
- `cook` 与 Order gRPC 成功时 ACK；gRPC error 按 delivery contract 处理。

本轮不会解决“`cook` 已执行但 Order 更新前退出”的副作用重复问题。该问题需要 Kitchen
Inbox/履约记录，而不是仅靠 ACK 控制流。

### Stripe webhook

- `PaymentHandler` 依赖最小 `Publisher` interface，`*amqp.Channel` 继续满足该接口；
- 使用 stripe-go 提供的真实测试签名生成器构造 webhook 测试；
- paid event publish 成功返回 200；
- publish 失败记录 error，返回 503，并终止 handler；
- 非法签名仍返回 400；
- 非目标事件或未 paid session 仍返回 200 且不发布。

HTTP 503 允许 Stripe 按 Provider 策略重试，但在持久化 webhook ledger/Outbox 落地前，仍不能
保证每个已支付事件最终进入 RabbitMQ。

## Testing Strategy

### Payment command tests

- nil order 不调用 processor/gRPC；
- processor error 不调用 gRPC；
- gRPC error 被原样返回；
- 两步成功返回 payment link。

### Payment consumer tests

- valid success：handler 1 次、ACK 1 次、NACK 0；
- malformed JSON：handler 0、retry 0、ACK 0、NACK 1 且不 requeue；
- handler error + retry success：retry 1、ACK 1、NACK 0；
- handler error + retry error：retry 1、ACK 0、NACK 1 且 requeue。

### Order consumer tests

- 更新现有 handler-error case，分别覆盖 retry success 与 retry error；
- malformed 与 success 断言保持。

### Kitchen consumer tests

- malformed JSON：不 cook、不调用 gRPC、NACK 一次；
- non-paid：不 cook、不调用 gRPC、NACK 一次；
- paid success：cook 一次、gRPC 一次、ACK 一次；
- gRPC error + retry success：cook 一次、retry 一次、ACK 一次；
- gRPC error + retry error：cook 一次、retry 一次、NACK requeue。

### Webhook tests

- 签名正确的 paid Checkout event + publish success → HTTP 200、发布一次 `order.paid`；
- 同样事件 + publish error → HTTP 503；
- invalid signature → HTTP 400、不发布。

## Documentation Updates

修复完成后同步更新：

- `docs/architecture/business-flow.md`；
- `docs/architecture/reliability.md`；
- `docs/architecture/evolution.md` 中 P0 状态；
- `docs/project-evidence.md` 能力矩阵；
- `docs/experiments/` 新增 consumer correctness 实验记录；
- `README.md` 的已实现/未实现矩阵。

P0 可以标记为已实现，但 Outbox、Inbox、publisher confirms、库存预占和履约副作用幂等仍
必须保持为未实现。

## Acceptance Criteria

- Payment、Order、Kitchen 的每条测试路径 ACK/NACK 调用总数最多为 1；
- retry publish 失败不会主动丢弃原 delivery，而是请求 requeue；
- Order gRPC error 能从 `CreatePayment` 到达 Payment consumer；
- paid webhook publish error 返回 HTTP 503；
- 新增测试在实现前能够针对当前行为失败，实现后通过；
- `make verify` 纳入 Payment 与 Kitchen 新测试并全部通过；
- Order 相关包继续通过 race detector；
- `docker compose config --quiet`、本地 Markdown 链接和 `git diff --check main..HEAD` 通过；
- 文档与代码对 P0 状态一致；
- 改动推送到现有 PR #1。
