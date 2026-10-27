# GOrder 文档

这组文档用于回答三个问题：GOrder 当前真正实现了什么、关键链路为什么这样设计、如果继续
向生产级系统演进还需要补齐什么。建议先按自己的阅读目的选择入口，而不是从头顺序阅读
全部文件。

## 推荐阅读路径

### 招聘方或第一次浏览

1. [项目首页](../README.md)：三分钟了解项目定位、核心问题和验证方式；
2. [系统架构总览](architecture/overview.md)：服务边界、协议和运行组件；
3. [证据与公开边界](project-evidence.md)：哪些能力已经实现，哪些只是演进设计。

### 架构与后端面试

1. [系统架构总览](architecture/overview.md)；
2. [核心业务流程](architecture/business-flow.md)；
3. [数据所有权与状态模型](architecture/data-and-state.md)；
4. [可靠性模型](architecture/reliability.md)；
5. [架构演进路线](architecture/evolution.md)。

这条路径覆盖服务拆分、同步与异步边界、状态机、消息至少一次投递、幂等、跨存储事务和
故障恢复等常见追问。

### 本地验证与继续开发

1. [本地开发指南](development/local-development.md)；
2. [支付消息重复投递实验](experiments/payment-redelivery.md)；
3. [Consumer correctness 实验](experiments/consumer-correctness.md)；
4. [显式订单迁移实验](experiments/explicit-order-transitions.md)；
5. [证据与公开边界](project-evidence.md)。

其中 `make verify` 是当前持续验证路径；完整业务链路需要外部 Stripe test resources 和分别
启动四个进程，属于探索性调试路径。

## 文档地图

| 文档 | 回答的问题 |
|---|---|
| [系统架构总览](architecture/overview.md) | 系统由什么组成，各服务通过什么协议协作？ |
| [核心业务流程](architecture/business-flow.md) | 从创建订单到支付、履约，代码实际经过哪些步骤？ |
| [数据所有权与状态模型](architecture/data-and-state.md) | 哪个服务拥有哪类数据，订单状态如何迁移？ |
| [可靠性模型](architecture/reliability.md) | 当前守住了哪些不变量，还存在哪些失败窗口？ |
| [架构演进路线](architecture/evolution.md) | 怎样分阶段补齐 Inbox、Outbox、库存预占和对账？ |
| [本地开发指南](development/local-development.md) | 怎样启动依赖、运行模块、验证和排障？ |
| [证据与公开边界](project-evidence.md) | 哪些是源码事实、测试证据、缺口或方案？ |
| [支付重投实验](experiments/payment-redelivery.md) | 怎样复现并验证支付消息重复投递下的幂等？ |
| [Consumer correctness 实验](experiments/consumer-correctness.md) | 怎样验证三个 consumer 的单一确认终态和 webhook 发布失败反馈？ |
| [显式订单迁移实验](experiments/explicit-order-transitions.md) | 怎样验证显式状态迁移、gRPC 错误分类与 Stripe 幂等键？ |

`superpowers/` 保存实施设计和计划，属于项目建设记录，不是理解系统的前置阅读材料。

## 阅读约定

主文档统一使用以下状态：

- **当前实现**：可直接定位到本仓库当前分支源码；
- **已验证行为**：有当前可运行的测试或命令支持；
- **已知缺口**：当前没有闭环，或代码存在明确失败窗口；
- **演进方案**：为解决缺口提出的下一阶段设计，不是已有能力；
- **未知**：没有足够证据，公开材料不作推断。

完整定义和使用边界见[证据与公开边界](project-evidence.md)。
