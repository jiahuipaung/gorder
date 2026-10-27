# GOrder Portfolio Documentation Design

## Goal

把 GOrder 整理成一个可公开审阅、可本地验证、能够支撑后端面试讨论的工程作品。文档必须
让读者快速理解系统解决什么问题，同时允许技术面试官继续下钻到服务边界、事件语义、状态
迁移、事务边界、失败窗口和后续演进。

这轮只整理并解释现有工程，不借文档把未实现能力包装成已实现能力，也不为了展示复杂度而
新增业务功能。

## Audience

- 在 GitHub 上快速浏览项目的招聘方或面试官；
- 希望复现实验、继续开发或审阅实现的工程师；
- 项目作者本人，用于准备架构追问和保持公开表述一致。

## Documentation Architecture

采用“短入口 + 分层下钻”的结构：

| 文件 | 单一职责 |
|---|---|
| `README.md` | 三分钟入口、项目定位、核心问题、验证命令和文档导航 |
| `docs/README.md` | 文档地图和推荐阅读路径 |
| `docs/architecture/overview.md` | 系统边界、服务职责、组件关系、协议和运行拓扑 |
| `docs/architecture/business-flow.md` | 下单、支付链接、支付回调、订单支付和履约时序 |
| `docs/architecture/data-and-state.md` | 数据所有权、订单状态、事件结构和同步/异步边界 |
| `docs/architecture/reliability.md` | 幂等、ACK/NACK、重试、DLQ、事务边界和失败窗口 |
| `docs/architecture/evolution.md` | 从当前实现到 Outbox、Inbox、库存预占、对账的演进路径 |
| `docs/development/local-development.md` | 环境要求、基础设施、启动顺序、验证命令和排障 |
| `docs/project-evidence.md` | 来源说明、证据分级、能力矩阵和公开表达边界 |

已有的 `docs/experiments/payment-redelivery.md` 保留为支付重投实验记录；已有设计和执行计划
保留在 `docs/superpowers/`，不混入面向普通读者的主阅读路径。

## Source-of-Truth Rules

文档中的每条重要结论必须属于以下一种状态：

1. **Current implementation**：能从当前分支源码直接定位；
2. **Verified behavior**：有当前可运行的测试或命令证明；
3. **Known gap**：源码中不存在、未闭环或存在明确失败窗口；
4. **Evolution proposal**：为了补齐已知缺口提出的设计，不属于当前能力；
5. **Unknown**：缺少生产背景或运行证据，不能推断。

`project-evidence.md` 统一解释这套分级。架构文档使用“当前实现”和“演进方案”两个明确
章节，避免读者把未来设计误认为已交付能力。

禁止写入以下内容，除非本轮获得新的可核验证据：

- 原公司生产架构、内部实现、业务数据或 Git 历史；
- 用户规模、订单量、吞吐量、延迟、可用性等未经测试的指标；
- 已实现完整 Saga、Transactional Outbox、通用 Inbox、库存预占或自动对账；
- Redis 分布式锁、缓存或限流已经参与核心链路；
- Docker Compose 已经能够一键启动全部业务服务。

## Content Requirements

### Overview

- 用上下文图说明 Client、GOrder 服务、支付 Provider 与基础设施的关系；
- 用容器图说明 Order、Stock、Payment、Kitchen 的职责和依赖；
- 列出 HTTP、gRPC、RabbitMQ 三类交互；
- 明确 Compose 当前只编排基础设施，业务进程仍需分别启动；
- 说明 Consul、Jaeger、Prometheus/Grafana、Redis 在代码或配置中的真实成熟度。

### Business Flow

- Create Order：HTTP/gRPC 请求、Stock gRPC 校验、MongoDB 写入、`order.create` 发布；
- Create Payment：Payment 消费 `order.create`、创建支付链接、Order gRPC 更新；
- Payment Result：Stripe webhook、`order.paid` 发布、Order 原子状态迁移；
- Fulfillment：Kitchen 消费 `order.paid`、模拟处理、Order gRPC 更新为 `ready`；
- 每个流程必须注明成功路径和直接可见的失败窗口。

### Data and State

- 标明 MongoDB 是订单记录所有者，MySQL 是当前库存数量所有者；
- 说明 RabbitMQ 事件是跨服务通知，不是业务状态最终权威；
- 记录代码中显式定义的 `waiting_for_payment`、`paid`，以及 Kitchen 实际写入但领域常量
  未完整建模的 `ready`；
- 区分支付终态专用 `MarkPaid` 与通用 `UpdateOrder` 的并发语义；
- 明确 Stock 当前只检查库存，不预占、不扣减。

### Reliability

- 以“不变量—当前机制—证据—剩余风险”组织内容；
- 解释数据库提交后、Broker ACK 前退出造成重复投递；
- 解释 `waiting_for_payment -> paid` 条件更新和重复消息 no-op；
- 解释 Order 支付消费者已保证 ACK/NACK 互斥，但不能扩展为所有消费者均已修复；
- 记录重试 header、退避、重新发布和 DLQ 的当前实现；
- 列出 Order DB/MQ 双写、Stripe webhook 发布失败、Payment 消费者控制流、Kitchen 重复副作用、
  库存 check-only 等真实风险。

### Evolution

- 演进顺序遵循风险闭环，而不是技术堆砌：先修消费确认语义，再补稳定事件身份与 Inbox，
  再补 Outbox，随后建设库存预占状态机，最后增加对账与故障注入；
- 每个阶段说明解决的问题、不解决的问题、数据模型变化和验收证据；
- Redis 锁不得作为订单、库存或支付正确性的最终边界。

### Local Development

- 给出 Go、Docker、Compose 等前置条件；
- 罗列环境变量与本地默认配置的安全边界；
- 说明基础设施启动、业务模块启动和 `make verify` 的差别；
- 给出端口、健康检查和常见失败的源码依据；
- 不承诺未实际验证的一键全链路启动。

## Presentation Rules

- 主体使用中文，保留协议、模式和代码标识符的英文原名；
- Mermaid 只用于上下文、容器、时序和状态迁移，不重复正文；
- 表格用于职责、接口、失败窗口和能力状态；
- 每份文件开头先说明读者从该页能得到什么；
- 重要边界使用显式提示，不用模糊的“支持高并发”“保证一致性”等宣传句；
- 文件之间使用相对链接，README 能进入所有主要文档，所有主要文档能返回文档地图。

## Acceptance Criteria

- 新增文档结构完整，README 和 `docs/README.md` 不存在断链；
- Mermaid 代码块结构可被 GitHub 渲染；
- 关键架构结论可定位到当前源码、测试或现有实验文档；
- Current implementation、known gap、evolution proposal 不混写；
- 文档不包含公司专有信息、真实密钥或未经证明的规模指标；
- `make verify`、`docker compose config --quiet`、`git diff --check` 全部通过；
- 改动提交并推送到现有 `codex/payment-idempotency` PR。
