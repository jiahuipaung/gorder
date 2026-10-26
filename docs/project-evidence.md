# 证据与公开边界

[返回文档地图](README.md)

本文定义 GOrder 的事实来源和公开表述边界。它的目的不是降低项目价值，而是把“工作经历
中的问题背景”“个人重建代码”“可运行证据”和“未来设计”分开，使每个技术结论都经得起
继续追问。

## 项目来源

GOrder 是对订单、库存、支付和履约问题的个人脱敏重建。仓库不包含原任职公司的生产源码、
业务数据、内部配置、域名、密钥或 Git 历史。本仓库能证明的是个人对问题的建模、实现和
验证；它不能替代原生产系统的实现证据。

## 证据分级

| 状态 | 判定标准 | 在文档中的用法 |
|---|---|---|
| 当前实现 | 可定位到当前分支的源码、配置或接口定义 | 描述代码现在怎样工作 |
| 已验证行为 | 有当前执行过的自动化测试或验证命令 | 描述在已覆盖输入下观察到的行为 |
| 已知缺口 | 源码未实现、链路未闭环或存在直接可见的失败窗口 | 明确风险，不使用“已支持” |
| 演进方案 | 为解决已知缺口提出，但尚未落地 | 单独放在演进章节，用将来时 |
| 未知 | 缺少源码、运行数据或生产上下文 | 保留未知，不用合理猜测补齐事实 |

“当前实现”不自动等于“生产可用”，“通过单元测试”也不自动等于“完整链路已经验证”。

## 当前能力矩阵

| 能力 | 状态 | 证据与边界 |
|---|---|---|
| Order、Stock、Payment、Kitchen 服务骨架 | 当前实现 | 源码位于 `internal/` 下的独立 Go module；不代表所有接口均已完成 |
| HTTP、gRPC、RabbitMQ 混合协作 | 当前实现 | API、client/server adapter 和 consumer 可定位；完整外部依赖链路未纳入默认验证 |
| 订单保存于 MongoDB | 当前实现 | Order Mongo repository 负责创建、读取和状态更新 |
| 库存数量读取自 MySQL | 当前实现 | Stock repository 实现 `GetStock`；`GetItems` 仍未实现 |
| 支付链接创建与 Stripe webhook 验签 | 当前实现 | Stripe adapter 与 webhook handler 已存在；离线测试使用 stripe-go 真实签名生成器，真实 Provider 集成不属于默认验证 |
| `waiting_for_payment -> paid` 原子条件迁移 | 已验证行为 | [Mongo repository 测试](../internal/order/adapters/order_mongo_repository_test.go)验证条件更新；[内存 repository 测试](../internal/order/adapters/order_inmem_repository_test.go)验证状态语义 |
| 重复支付消息成为成功 no-op | 已验证行为 | [应用命令测试](../internal/order/app/command/mark_order_paid_test.go)和 repository 测试覆盖顺序重放 |
| 并发支付重放只发生一次状态变化 | 已验证行为 | [内存 repository 并发测试](../internal/order/adapters/order_inmem_repository_test.go)在 race detector 下运行 |
| 支付事件的客户/订单隔离 | 已验证行为 | repository 测试覆盖 `order_id` 与 `customer_id` 共同约束 |
| Payment、Order、Kitchen consumer 的 ACK/NACK 互斥 | 已验证行为 | 三个 consumer 测试覆盖成功、畸形消息、retry 成功与失败；Kitchen 另覆盖非 `paid` 消息 |
| retry 交接失败保留原 delivery | 已验证行为 | [Payment](../internal/payment/infrastructure/consumer/consumer_test.go)、[Order](../internal/order/infrastructure/consumer/consumer_test.go)和 [Kitchen](../internal/kitchen/infrastructure/consumer/consumer_test.go)测试断言 `Nack(requeue=true)` |
| Payment command 错误传播 | 已验证行为 | [command 测试](../internal/payment/app/command/create_payment_test.go)覆盖 nil order、Stripe error、Order gRPC error 和成功更新 |
| webhook publish 失败返回 HTTP 503 | 已验证行为 | [webhook 测试](../internal/payment/http_test.go)覆盖合法 signed paid event、publish error 和 invalid signature |
| RabbitMQ 重试计数和 DLQ 路径 | 当前实现 | broker helper 使用 `x-retry-count` 并在达到上限后发布到 DLQ；尚无端到端 Broker 故障测试 |
| 库存可用性检查 | 当前实现 | 比较请求数量与 MySQL 当前数量，并取得支付 Price ID |
| 库存预占、确认、释放 | 已知缺口 | 当前 Stock 流程不扣减、不预占，也没有超时回收 |
| 通用持久化 Inbox | 已知缺口 | 事件没有统一稳定身份和消费去重表 |
| Transactional Outbox | 已知缺口 | 订单落库与发布 `order.create` 仍是两个独立操作 |
| RabbitMQ publisher confirms | 已知缺口 | consumer retry 与 webhook 只检查 publish 调用返回值，未等待 Broker confirm |
| Kitchen 履约副作用幂等 | 已知缺口 | delivery 终态已修复，但 `cook` 没有持久化履约记录或 Inbox |
| 自动对账与补偿 | 已知缺口 | 未实现订单、支付、库存之间的周期性核对与修复 |
| 完整业务服务一键启动 | 已知缺口 | Compose 当前只编排基础设施，业务进程需要分别启动 |

支付重投和 Consumer 正确性切片的命令、覆盖范围和限制详见
[支付消息重复投递实验](experiments/payment-redelivery.md)与
[Consumer correctness 实验](experiments/consumer-correctness.md)。

## 可以公开说明的内容

下面的描述与当前证据一致：

> GOrder 是一个 Go 微服务交易流程的脱敏重建项目，包含订单、库存、支付和履约服务。
> 项目当前重点验证了 RabbitMQ 至少一次投递下的支付终态幂等：通过持久化层条件更新限制
> `waiting_for_payment -> paid`，把重复支付事件处理为 no-op，并用并发测试和 race detector
> 验证状态只变化一次。Payment、Order、Kitchen consumer 还通过显式终态分支保证已覆盖
> 路径只 ACK 或 NACK 一次；retry 发布失败会 requeue 原消息，webhook 发布失败会返回 503。

也可以进一步说明：项目主动保留了 Outbox、持久化 Inbox、库存预占和对账作为演进项，并在
文档中分析它们分别解决哪个失败窗口。

## 不能当作现状宣称的内容

- 不能说这是原公司的生产系统、生产源码或等比例复刻；
- 不能给出未经本仓库压测或观测证明的用户数、订单量、QPS、延迟、可用性和故障恢复时间；
- 不能说已经实现完整 Saga、分布式事务或 exactly-once delivery；
- 不能说已经通过 Outbox 消除了订单落库与发消息的双写问题；
- 不能说已经通过 Inbox 对所有消息完成去重；
- 不能说库存已经预占、扣减、回滚或超时释放；
- 不能把 Redis 描述为当前业务正确性的最终边界；
- 不能把 consumer 单一确认终态描述为 exactly-once、消息零丢失或履约副作用幂等；
- 不能说 `docker compose up` 会启动全部业务服务。

## 从经历到作品的引用规则

| 信息来源 | 能支撑什么 | 不能支撑什么 |
|---|---|---|
| 工作经历 | 为什么关注交易一致性、服务协作和支付异常 | 当前仓库具体实现已经用于原公司生产 |
| 当前源码 | 当前数据结构、接口、状态更新和消费者控制流 | 没有运行证据的性能、稳定性或生产结果 |
| 自动化测试 | 测试输入覆盖下的确定行为 | 未覆盖的外部服务、崩溃点或全链路故障 |
| 架构演进文档 | 对缺口的技术判断和设计取舍 | 已完成、已上线或已经产生业务收益 |

后续每落地一个演进阶段，应同时补充实现、失败场景测试和本矩阵，而不是只修改宣传文字。
