# 库存预占与支付边界

GOrder 在 Stock/MySQL 中实现了 `Reserve`、`Confirm`、`Release` 和 `GetReservation`。
Order 创建流程先保存 MongoDB 意图，再以订单 ID 预占库存；预占成功才发布
`order.create`。已签名、已支付的 Stripe webhook 确认预占后发布 `order.paid`。
Order 消费事件时再次核验预占为 `CONFIRMED`；Kitchen 在模拟履约前查询 Order
持久化状态。这里是可验证的交易链路切片，**不是完整支付对账系统**。

## 数据与状态

`o_stock.quantity` 表示可用数量。Stock 使用单个 MySQL 事务按排序后的 SKU
逐项扣减；任一 SKU 不足时整体回滚。`product_id` 必须唯一，启动迁移发现重复行会拒绝继续。
预占 ID 使用订单 ID，绑定客户、归并后的 SKU/数量和到期时间；同一 ID、同一内容重试返回
已有状态，不重复扣减。ID 与客户列使用二进制比较。`Confirm` 只改变预占状态，
`Release` 只对仍为 `RESERVED` 的预占恢复数量一次，两者互斥。

```mermaid
stateDiagram-v2
    [*] --> RESERVED: Reserve / 扣可用数量
    RESERVED --> CONFIRMED: Confirm / 不再扣减
    RESERVED --> RELEASED: Release / 恢复数量
    CONFIRMED --> CONFIRMED: 确认重放
    RELEASED --> RELEASED: 释放重放
```

`Due` 只查到期候选，不自动释放。仅凭本地时间无法断言支付失败；Provider
结果不明时必须保留库存并对账。这会牺牲可用库存，但避免把可能已经支付的订单放回库存。
当前尚未持久化 Provider Session ID 或 event ledger，也没有到期核查与自动释放任务。

## 故障窗口和恢复范围

| 故障位置 | 当前行为 | 未覆盖边界 |
|---|---|---|
| Mongo 意图保存后、预占前 | Order 后台扫描未发布、未到期意图，以同一 ID 重试 | 客户 HTTP 响应丢失时没有请求级幂等键，可能创建另一订单 |
| 预占后、`order.create` 发布前 | 同一 ID 重试预占和发布；Broker 确认后标记已发布 | Mongo 标记与发布不是原子事务，可能重复发布 |
| Stock 明确返回库存不足 | 订单标记 `stock_rejected`，不创建支付会话 | 不负责自动补货或替代商品 |
| 签名 webhook 确认库存后、`order.paid` 发布前 | 发布失败返回 503，依赖 Provider 重试 | 没有 Provider event ledger/Outbox；Provider 不再重试时需要人工对账 |
| `order.paid` 到达 Kitchen 早于 Order 更新 | Kitchen 等待后 NACK/requeue 原消息，不消耗有限 retry 次数 | Order 长时故障会阻塞该消费循环；模拟 `cook` 与 `MarkReady` 之间仍可能重复执行 |
| 预占到期 | 保持原状态、等待支付核查 | 自动取消/释放与晚到支付异常处理未实现 |

RabbitMQ 的 `order.paid` 为 Order 与 Kitchen 分别声明 durable queue。消费者重试
只发回自己的队列；发布确认只证明 Broker 接受了消息，不证明数据库与消息原子提交。
连接关闭时 consumer 让进程失败退出，依赖进程管理器重启；没有进程内自动重连。

## Verification

`make verify` 是无需外部服务的默认验证，包含离线应用状态链、异常路径与 race
检查；它不等于真实 Stripe 联调。隔离的 MySQL/RabbitMQ 集成测试覆盖最后一件并发竞争、
多 SKU 回滚、同键冲突、终态竞争、到期仍占用、持久队列、定向重试、发布确认与不可路由拒绝。

```bash
docker compose -p gorder-review-integration -f docker-compose.integration.yml up -d --wait
cd internal/stock
GORDER_TEST_MYSQL_DSN='root:local_test_only@tcp(127.0.0.1:53308)/gorder_test?parseTime=true&loc=UTC' go test -tags integration -count=1 ./reservation
cd ../common
GORDER_TEST_AMQP_URL='amqp://gorder_test:local_test_only@127.0.0.1:55673/gorder_test' go test -tags integration -count=1 ./broker
```

测试使用专用 `gorder_test` 数据库和 RabbitMQ vhost；不要将这两个测试 DSN
指向已有业务数据。另见[离线订单状态链测试](../../internal/order/app/command/commerce_flow_test.go)。
