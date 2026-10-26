# 显式订单迁移实验

[返回文档地图](../README.md) · [数据所有权与状态模型](../architecture/data-and-state.md) ·
[可靠性模型](../architecture/reliability.md)

本文给出不依赖 live Stripe、RabbitMQ 或 MongoDB 的复现路径，用于验证 GOrder 当前主流程写
模型，而不是证明完整分布式链路已经生产可用。

## 1. 要验证的不变量

```text
pending             --AttachPaymentSession--> waiting_for_payment
waiting_for_payment --MarkPaid-------------> paid
paid                --MarkOrderReady--------> ready
```

- 每条首次迁移必须带预期旧状态做原子条件更新；
- 精确重放返回成功 no-op；
- 不同支付链接、错误客户或错误旧状态不能覆盖当前订单；
- Payment/Kitchen 不能再提交任意 Order snapshot；
- 同一订单的 Stripe Checkout 请求使用稳定且非空的订单 ID 作为幂等键。

## 2. 聚焦验证

### 2.1 Order repository、command 与 gRPC

```bash
cd internal/order
go test -count=1 -race ./adapters ./app/command ./domain/order ./ports
```

这些测试覆盖：

- 内存 repository 的首次迁移、重放、链接冲突、错误客户和错误状态；
- Mongo mock 捕获的 `UpdateOne` filter 包含 `_id`、`customer_id` 与预期旧状态；
- command 对空订单 ID、客户 ID、支付链接的拒绝；
- gRPC 把输入错误、not found、非法迁移和存储错误分别映射为
  `InvalidArgument`、`NotFound`、`FailedPrecondition` 与 `Internal`；
- race detector 下的现有并发支付重放测试。

### 2.2 Payment 与 Stripe 参数

```bash
cd internal/payment
go test -count=1 -race ./...
```

`CreatePayment` 测试确认下游收到的只有订单 ID、客户 ID 和创建出的链接，并继续传播 Stripe
或 Order gRPC 错误。`StripeProcessor` 参数测试确认：

```text
Idempotency-Key = order_id
```

同时检查 metadata 与 line items 仍从同一订单请求构造。

### 2.3 Kitchen

```bash
cd internal/kitchen
go test -count=1 -race ./...
```

Kitchen consumer 测试确认 paid event 只把订单 ID 与客户 ID交给 `MarkOrderReady`，并保留
Consumer correctness 切片已经验证的单一 ACK/NACK 终态。

## 3. 全仓回归

```bash
make verify
docker compose config --quiet
```

`make verify` 编译所有 Go module，并对 Order 的关键 repository、command、domain 与 consumer
包运行 race 测试。额外的 Payment/Kitchen/Order 聚焦 race 命令用于覆盖本切片新增端口与参数
测试。

重新生成 protobuf 后应没有差异：

```bash
protoc \
  -I=/usr/local/include/ \
  -I=./api \
  --go_out=internal/common/genproto \
  --go_opt=paths=source_relative \
  --go-grpc_out=internal/common/genproto \
  --go-grpc_opt=paths=source_relative \
  --go-grpc_opt=require_unimplemented_servers=false \
  api/orderpb/order.proto
git diff --exit-code -- internal/common/genproto/orderpb
```

## 4. 结论边界

本实验能够支持“已建模的订单迁移用数据库条件更新保护”“精确重放为 no-op”“Stripe 创建
请求带稳定幂等键”。它不能支持以下说法：

- 所有业务消息 exactly-once；
- Stripe 幂等键永久有效，或已经具备支付 ledger/对账；
- Kitchen 的 `cook` 副作用不会重复；
- Order 与 RabbitMQ 已消除双写窗口；
- 库存已经预占或不会超卖。
