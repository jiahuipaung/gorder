# 支付消息重复投递实验

[返回文档地图](../README.md) · [可靠性模型](../architecture/reliability.md)

## 问题与故障窗口

RabbitMQ 使用手动确认时，消费者可能已经完成订单更新，但在 ACK 到达 Broker
之前发生进程退出或连接中断。Broker 随后会再次投递同一条支付成功消息。原实现采用
“读取订单、在应用层修改、再写回”的通用更新流程，而且错误分支在 NACK 后仍会继续
ACK；它既没有把状态前置条件放入原子写入，也没有保证一条 delivery 只走一条确认
路径。

本实验验证的目标不是“消息永不重复”，而是重复发生后仍满足业务不变量。

## 选择的不变量

```text
waiting_for_payment --(首次 paid 消息)--> paid
paid                --(重复 paid 消息)--> paid（成功但不再写入）
```

MongoDB 以单文档条件更新作为原子边界：过滤条件同时包含订单 ID、客户 ID 和
`status == waiting_for_payment`。只有 `ModifiedCount == 1` 表示本次完成了状态
迁移；若当前状态已经是 `paid`，返回成功的 no-op。

消费者的每条 delivery 只有一个终态：

```text
JSON 不合法   -> Nack(requeue=false) -> return
命令成功      -> Ack                 -> return
命令失败      -> retry -> Nack       -> return
```

## 可复现验证

不需要启动 MongoDB、RabbitMQ 或 HTTP 服务。在仓库根目录执行：

```bash
cd internal/order
GOCACHE=/private/tmp/gorder-go-build-cache \
  go test -count=1 -race -vet=off \
  ./adapters ./app/command ./domain/order ./infrastructure/consumer
```

2026-10-26 的本地验证结果：

```text
ok   order/adapters
ok   order/app/command
?    order/domain/order [no test files]
ok   order/infrastructure/consumer
```

测试覆盖：

- 首次支付将订单从 `waiting_for_payment` 原子迁移到 `paid`；
- 并发或顺序重复投递只有一次返回 `changed=true`，且保留第一次支付链接；
- Mongo 更新过滤器包含订单、客户和原状态三个条件；
- 非 `paid` 事件不会修改订单；
- 成功、畸形消息、处理错误分别只触发预期的 ACK/NACK 路径；
- 处理失败时即使 retry 函数自身报错，也不会继续 ACK 原消息。

## 证据边界

这是脱敏后的独立重建设计，用来证明失败分析、幂等边界和可测试性；不表示原公司
生产系统曾采用完全相同的实现。

已验证的是“订单终态迁移幂等”，不是通用的事件幂等：当前事件没有稳定的支付服务商
event ID，也没有持久化 Inbox。因此，如果支付消费未来增加积分、通知等额外副作用，
仅依靠订单状态不足以保证这些副作用只执行一次。下一阶段应把 provider event ID
贯穿事件模型，并以唯一约束的 Inbox（必要时配合事务）保护整个消费事务。

完整的 `internal/order` 测试仍包含一个依赖本地 HTTP 服务的历史集成测试；未启动
服务时 `go test -vet=off ./...` 会在 `order/tests/TestCreateOrder_success`
失败。该既有失败与本实验的无外部依赖测试分开记录，不能据此宣称整个模块全绿。
