# 本地开发指南

[返回文档地图](../README.md) · [系统架构总览](../architecture/overview.md)

GOrder 有两种不同的本地使用方式：

1. **离线验证核心工程切片**：不启动外部服务，运行 `make verify`；这是当前持续验证路径；
2. **探索完整业务链路**：启动基础设施和四个业务进程，并配置 Stripe test mode；这条路径
   依赖外部资源，当前不是一键集成测试。

建议第一次接触项目先运行第一种，再根据调试目标启动第二种。

## 1. 环境要求

### 核心验证

- Go `1.23.3` 或兼容的更新版本；根 module 声明 `1.23.2`，部分子 module 声明 `1.23.3`；
- GNU Make；
- 能写入 Go build cache 的本地目录。

### 完整链路调试

除上面工具外还需要：

- Docker Engine 或 Docker Desktop；
- Docker Compose v2（`docker compose`）；
- Stripe test-mode secret key；
- 用于 webhook 签名验证的 endpoint secret；
- 至少一个有 default Price 的 Stripe test Product，并在 MySQL 中有对应库存行。

生成 protobuf/OpenAPI 代码还需要 `protoc`、Go protobuf plugins 和 `oapi-codegen`，但仓库已经
提交生成结果，运行与验证现有代码不要求重新生成。

## 2. 最快验证路径

在仓库根目录执行：

```bash
GOCACHE=/private/tmp/gorder-go-cache make verify
```

`GOCACHE` 只用于确保受限环境存在可写缓存；普通本地终端可以直接执行：

```bash
make verify
```

命令包含：

| 阶段 | 实际命令 | 目的 |
|---|---|---|
| Common | `cd internal/common && go test -count=1 ./...` | 编译并测试共享基础模块 |
| Payment | `cd internal/payment && go test -count=1 ./...` | 编译并测试 Payment module |
| Stock | `cd internal/stock && go test -count=1 ./...` | 编译并测试 Stock module |
| Kitchen | `cd internal/kitchen && go test -count=1 ./...` | 编译并测试 Kitchen module |
| Payment redelivery | `cd internal/order && go test -count=1 -race ./adapters ./app/command ./domain/order ./infrastructure/consumer` | 验证支付终态幂等和 Order consumer 确认语义 |

为什么没有执行 `cd internal/order && go test ./...`：`internal/order/tests` 包含历史端到端测试，
它会请求正在运行的本地 HTTP 服务。默认验证刻意只选择无外部依赖、可确定复现的包；这不是
在暗示整个 Order module 的所有集成路径都已经通过。

CI 在 push 和 pull request 上使用同一个 `make verify`，Go 版本读取根目录 `go.mod`。

## 3. 配置模型

共享配置位于 `internal/common/config/global.yaml`，通过 Viper 在各 module 初始化时读取。代码
启用了 `AutomaticEnv` 和 `-` → `_` 的 key 替换；当前明确依赖环境覆盖的是两个顶层 Stripe
配置。嵌套配置通过 `viper.Sub(...)` 读取且没有逐项 `BindEnv`，本文不把任意嵌套环境变量
覆盖当作已验证能力。

### 3.1 必须作为环境变量提供的值

```bash
export STRIPE_KEY=sk_test_replace_me
export ENDPOINT_STRIPE_SECRET=whsec_replace_me
```

只使用 Stripe test mode。不要把真实 key、webhook secret、客户信息或支付 payload 写进
`global.yaml`、README、测试 fixture、日志示例或 Git 历史。

### 3.2 本地默认配置

| YAML 配置路径 | 默认值 | 用途 |
|---|---|---|
| `consul.addr` | `127.0.0.1:8500` | 服务注册与发现 |
| `rabbit-mq.host` / `rabbit-mq.port` | `127.0.0.1` / `5672` | AMQP |
| `rabbit-mq.user` / `rabbit-mq.password` | `guest` / `guest` | 仅限本地 RabbitMQ |
| `mongo.host` / `mongo.port` | `127.0.0.1` / `27017` | Order MongoDB |
| `mongo.username` / `mongo.password` | `root` / `password` | 仅限本地 Compose |
| `mysql.host` / `mysql.port` | `localhost` / `3307` | Stock MySQL |
| `mysql.user` / `mysql.password` | `root` / `root` | 仅限本地 Compose |
| `jaeger.url` | `http://127.0.0.1:14268/api/traces` | Jaeger collector |

这些是开发默认值，不可复制到公网部署。当前仓库没有生产配置管理、secret manager 或环境
分层方案。

## 4. 启动基础设施

先检查 Compose 文件：

```bash
docker compose config --quiet
```

再启动依赖：

```bash
docker compose up -d
docker compose ps
```

Compose 当前启动：

| 组件 | 地址/入口 | 说明 |
|---|---|---|
| Consul | `http://127.0.0.1:8500` | UI 和 HTTP API |
| RabbitMQ | `amqp://127.0.0.1:5672` | AMQP；管理台 `http://127.0.0.1:15672` |
| Jaeger | `http://127.0.0.1:16686` | Trace 查询 UI |
| MongoDB | `127.0.0.1:27017` | Order store |
| mongo-express | `http://127.0.0.1:8082` | 本地 MongoDB UI，无鉴权配置只适合本机 |
| MySQL | `127.0.0.1:3307` | 容器内端口 3306；启动时执行 `init.sql` |
| Prometheus | `http://127.0.0.1:9090` | 当前没有业务服务 scrape target |
| Grafana | `http://127.0.0.1:3000` | 当前没有预置业务 dashboard |

这里不会启动业务进程，也没有 Redis 服务。

停止基础设施：

```bash
docker compose down
```

不要轻易添加 `-v`：它会删除 Compose volumes，包括本地数据库数据。

## 5. 准备 Stripe Product 与库存

Stock 把请求中的商品 ID 当作 Stripe Product ID，并调用 Stripe 查询该 Product 的 default
Price。MySQL 的 `o_stock.product_id` 必须使用同一个 test Product ID。

示意 SQL（替换占位符）：

```sql
INSERT INTO o_stock (product_id, quantity)
VALUES ('prod_replace_me', 100);
```

当前项目没有商品后台或 seed script。数据不存在时，MySQL 查询会把对应商品视为数量 0；
Product 没有 default Price 或 Stripe key 不可用时，Stock 无法组装完整订单商品。

## 6. 启动业务进程

每个目录是独立 Go module。建议按以下顺序在四个终端中运行。

### 6.1 Stock

```bash
cd internal/stock
go run .
```

启动时会连接 MySQL、初始化 Stripe client、注册到 Consul，并监听
`127.0.0.1:5003` gRPC。Stock 必须先于 Order 启动，因为 Order 初始化会等待 Stock gRPC。

### 6.2 Order

```bash
cd internal/order
go run .
```

Order 启动时会：

- 等待并通过 Consul 发现 Stock；
- 连接 MongoDB 与 RabbitMQ；
- 注册自己的 gRPC 地址到 Consul；
- 监听 HTTP `127.0.0.1:8282` 和 gRPC `127.0.0.1:5002`；
- 消费 `order.paid`。

### 6.3 Payment

```bash
cd internal/payment
go run .
```

Payment 等待并通过 Consul 发现 Order，连接 RabbitMQ 与 Stripe，监听 HTTP
`127.0.0.1:8284`，并消费 `order.create`。

Webhook endpoint：

```text
POST http://127.0.0.1:8284/api/webhook
```

Stripe 必须能把 test webhook 转发到这个地址，并使用与
`ENDPOINT_STRIPE_SECRET` 一致的签名 secret。具体转发工具不属于仓库自动化内容。

### 6.4 Kitchen

```bash
cd internal/kitchen
go run .
```

Kitchen 没有 HTTP/gRPC server。它通过 Consul 发现 Order，连接 RabbitMQ，消费
`order.paid`，sleep 五秒模拟处理后调用 Order gRPC 写入 `ready`。

## 7. 手动请求示例

假设已经准备好 `prod_replace_me` 对应的 Stripe test Product 与 MySQL 库存：

```bash
curl -X POST 'http://127.0.0.1:8282/api/customer/customer-demo/orders/' \
  -H 'Content-Type: application/json' \
  -d '{
    "customer_id": "customer-demo",
    "items": [
      {"id": "prod_replace_me", "quantity": 1}
    ]
  }'
```

创建接口当前同时在 path 和 body 中出现客户 ID；代码使用 body 的 `customer_id`，调试时让
两者保持一致。返回结果中的 `order_id` 可用于查询：

```bash
curl 'http://127.0.0.1:8282/api/customer/customer-demo/orders/ORDER_ID'
```

完整支付需要打开 Payment 创建的 Stripe Checkout URL，而该 URL 当前只能从订单查询结果或
日志中取得。不要在公开 issue、截图或提交中暴露真实支付会话链接。

## 8. 运行状态检查

| 检查 | 方法 | 能证明什么 |
|---|---|---|
| 容器状态 | `docker compose ps` | 基础设施容器是否在运行，不证明业务进程可用 |
| Consul UI | 查看 `order`、`stock` service | 注册和 TTL 是否正常 |
| RabbitMQ UI | 查看 `order.create`、`order.paid`、`order_dlq` | queue/exchange、consumer 和积压情况 |
| Jaeger UI | 按服务名查询 | 已导出的 HTTP/gRPC trace；不证明全部消息 trace 完整 |
| Mongo Express | 查看 `order.order` | 订单文档当前值 |
| MySQL | 查询 `gorder_v2.o_stock` 与 reservation 表 | `o_stock.quantity` 是可用数量；预占会扣减，确认不再扣减 |
| `make verify` | 查看退出码和各 module 输出 | 离线代码验证结果，不证明外部链路可用 |

当前没有统一 `/healthz` 或 `/readyz` endpoint，也没有应用 metrics endpoint。判断完整链路是否
可用，需要分别检查进程日志、Consul、RabbitMQ 和数据存储。

## 9. 常见故障与定位

### `timeout waiting for stock/order grpc client`

- 确认 Stock/Order 已按顺序启动；
- 确认 `global.yaml` 中固定 gRPC 地址可连接；
- 确认服务已经注册到 Consul 且 TTL 为 passing；
- 注意 client 在执行 Consul discovery 前先探测配置中的固定地址。

### `connect to mysql failed`

- 检查 MySQL 容器和宿主机端口 `3307`；
- 检查 `gorder_v2` 和 `o_stock` 是否由 `init.sql` 创建；
- 如果已有旧 volume，初始化脚本不会按全新容器方式再次执行，需要自行检查 schema；
- 不要用生产数据库凭据替换本地默认值后提交。

### MongoDB 连接或订单更新异常

- 检查 `order-mongo` 是否监听 `27017`，账号是否与本地配置一致；
- 当前通用 `Update` 启动 transaction session，却没有用 `mongo.SessionContext` 执行实际读写；
- Compose MongoDB 也是 standalone。完整链路遇到事务相关行为时，应按
  [可靠性登记 R11](../architecture/reliability.md#5-失败窗口登记表)理解，不要把它归因于支付
  幂等切片。

### Stripe key、Product 或 webhook 错误

- Stock 和 Payment 都需要 `STRIPE_KEY`；
- Product ID 必须属于同一个 test account，并拥有 default Price；
- webhook signature 失败会返回 HTTP 400；
- webhook body 超过 64 KiB 或无法读取会返回错误；
- RabbitMQ publish 未获 Broker confirm 时 webhook 返回 HTTP 503；当前仍没有
  Payment event ledger 或 Outbox，边界记录在[可靠性模型](../architecture/reliability.md)。

### RabbitMQ 消息没有推进流程

- 检查 exchange/queue 是否存在，以及 consumer 数量；
- 检查 `order_dlq` 和 `x-retry-count`；
- 区分 producer 没发布、消息积压、consumer 失败和业务状态拒绝；
- 当前有 publisher confirms，但没有 Outbox 或自动 DLQ replay；不能只根据 API 返回推断
  消息已被业务消费者处理。

### `internal/order/tests/TestCreateOrder_success` 失败

该历史 smoke test 请求真实本地 HTTP 地址和 Stripe test 商品，已用 `live` build tag
从默认离线验证中隔离。先使用 `make verify` 验证当前承诺；若目标是调试端到端链路，
再按本页顺序启动依赖和服务，执行 `cd internal/order && go test -tags live ./tests`。

## 10. 修改后的提交前检查

```bash
GOCACHE=/private/tmp/gorder-go-cache make verify
docker compose config --quiet
git diff --check
```

如果修改了架构或能力边界，还要同步检查：

- [证据与公开边界](../project-evidence.md)是否需要更新；
- [可靠性模型](../architecture/reliability.md)中的失败窗口是否已经关闭；
- [架构演进路线](../architecture/evolution.md)中的 proposal 是否能移动到“当前实现/已验证”；
- README 是否仍保持三分钟可读，而不是复制全部技术细节。
