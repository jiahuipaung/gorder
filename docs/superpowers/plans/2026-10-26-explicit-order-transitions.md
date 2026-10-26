# Explicit Order Transitions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace unrestricted order snapshot updates with atomic intent-specific transitions and make Stripe Checkout creation retry-safe by order ID.

**Architecture:** The Order repository becomes the authority for `pending -> waiting_for_payment -> paid -> ready`. Payment and Kitchen call narrow gRPC commands instead of sending whole aggregates, and Stripe receives the order ID as its idempotency key. The slice adds no new storage or worker.

**Tech Stack:** Go 1.23, MongoDB driver, gRPC/protobuf, Stripe Go v81, RabbitMQ consumer tests

**Spec:** `docs/superpowers/specs/2026-10-26-explicit-order-transitions-design.md`

## Global Constraints

- Do not add Inbox, Outbox, Saga, inventory reservation, or a Kitchen lease protocol.
- Do not retain a generic repository or gRPC update escape hatch.
- Mongo first applications must use one conditional `UpdateOne`; no transaction is required.
- Exact replay is a successful no-op; conflicting or out-of-order writes are errors.
- Every behavior change follows RED-GREEN-REFACTOR.

## Review Focus

- An empty payment link must be rejected before any persistence call.
- A different link replay must not replace the first stored payment link.
- A wrong customer ID must not mutate another customer's order.
- gRPC must distinguish not-found, failed-precondition, invalid input, and infrastructure errors.
- Stripe retry attempts for one order must carry the same non-empty idempotency key.

---

### Task 1: Domain transitions and repositories

**Files:**
- Modify: `internal/order/domain/order/order.go`
- Modify: `internal/order/domain/order/repository.go`
- Modify: `internal/order/adapters/order_inmem_repository.go`
- Modify: `internal/order/adapters/order_mongo_repository.go`
- Test: `internal/order/adapters/order_inmem_repository_test.go`
- Test: `internal/order/adapters/order_mongo_repository_test.go`

**Interfaces:**
- Produces: `AttachPaymentSession(context.Context, string, string, string) (bool, error)` and `MarkReady(context.Context, string, string) (bool, error)` on `order.Repository`.
- Produces: `StatusPending`, `StatusWaitingForPayment`, `StatusPaid`, and `StatusReady`.

- [ ] **Step 1: Write failing repository tests**

Add literal assertions for pending creation inputs, first application, exact replay,
conflicting link, wrong status, wrong customer, and Mongo filters containing the
expected old status.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `cd internal/order && go test -count=1 ./adapters ./domain/order`

Expected: FAIL because the new statuses and repository methods do not exist.

- [ ] **Step 3: Implement the minimal repository transitions**

Remove `Repository.Update`; add the two methods above. Use shared no-match
classification only where it keeps error semantics identical and readable.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `cd internal/order && go test -count=1 -race ./adapters ./domain/order`

Expected: PASS.

- [ ] **Step 5: Commit**

Commit: `feat(order): enforce explicit state transitions`

### Task 2: Application commands and gRPC contract

**Files:**
- Create: `internal/order/app/command/attach_payment_session.go`
- Create: `internal/order/app/command/attach_payment_session_test.go`
- Create: `internal/order/app/command/mark_order_ready.go`
- Create: `internal/order/app/command/mark_order_ready_test.go`
- Delete: `internal/order/app/command/update_order.go`
- Modify: `internal/order/app/app.go`
- Modify: `internal/order/service/application.go`
- Modify: `api/orderpb/order.proto`
- Regenerate: `internal/common/genproto/orderpb/order.pb.go`
- Regenerate: `internal/common/genproto/orderpb/order_grpc.pb.go`
- Modify: `internal/order/ports/grpc.go`
- Create: `internal/order/ports/grpc_test.go`

**Interfaces:**
- Consumes: repository methods from Task 1.
- Produces: `AttachPaymentSessionRequest` and `MarkOrderReadyRequest` protobuf messages and matching RPCs.

- [ ] **Step 1: Write failing command tests**

Cover valid invocation, empty identifiers/link, no-op propagation, not-found, and
invalid transition. The command returns the repository `changed` flag.

- [ ] **Step 2: Run command tests and verify RED**

Run: `cd internal/order && go test -count=1 ./app/command`

Expected: FAIL because the handlers do not exist.

- [ ] **Step 3: Implement commands and application wiring**

Add `AttachPaymentSession` and `MarkOrderReady` handlers and remove `UpdateOrder`.

- [ ] **Step 4: Run command tests and verify GREEN**

Run: `cd internal/order && go test -count=1 ./app/command`

Expected: PASS.

- [ ] **Step 5: Write failing gRPC tests and update the protobuf contract**

Test valid dispatch plus `InvalidArgument`, `NotFound`, `FailedPrecondition`, and
`Internal` mappings, then add the two request messages/RPCs and remove `UpdateOrder`.

- [ ] **Step 6: Regenerate protobuf code and implement the gRPC methods**

Run `protoc` with the repository's existing source-relative options, then route each
RPC to its matching application command.

- [ ] **Step 7: Run Order tests and verify GREEN**

Run: `cd internal/order && go test -count=1 -race ./adapters ./app/command ./domain/order ./ports`

Expected: PASS.

- [ ] **Step 8: Commit**

Commit: `refactor(order): expose intent-specific grpc commands`

### Task 3: Payment and Kitchen callers

**Files:**
- Modify: `internal/payment/app/command/service.go`
- Modify: `internal/payment/app/command/create_payment.go`
- Modify: `internal/payment/app/command/create_payment_test.go`
- Modify: `internal/payment/adapters/order_grpc.go`
- Modify: `internal/payment/infrastructure/processor/stripe.go`
- Create: `internal/payment/infrastructure/processor/stripe_test.go`
- Modify: `internal/kitchen/adapter/order_grpc_client.go`
- Modify: `internal/kitchen/infrastructure/consumer/consumer.go`
- Modify: `internal/kitchen/infrastructure/consumer/consumer_test.go`

**Interfaces:**
- Consumes: generated intent-specific Order gRPC client from Task 2.
- Produces: Payment `OrderService.AttachPaymentSession` and Kitchen `OrderService.MarkOrderReady` boundaries.

- [ ] **Step 1: Write failing Payment tests**

Assert that CreatePayment sends only order/customer/link through
`AttachPaymentSession`, propagates its error, and that Checkout params use the order
ID as a stable idempotency key.

- [ ] **Step 2: Run Payment tests and verify RED**

Run: `cd internal/payment && go test -count=1 ./app/command ./infrastructure/processor`

Expected: FAIL against the old generic update and missing params builder.

- [ ] **Step 3: Implement Payment adapter and Stripe idempotency**

Replace `UpdateOrder` with `AttachPaymentSession`. Extract a pure Checkout params
builder used by `CreatePaymentLink`, with `params.SetIdempotencyKey(order.ID)`.

- [ ] **Step 4: Run Payment tests and verify GREEN**

Run: `cd internal/payment && go test -count=1 -race ./...`

Expected: PASS.

- [ ] **Step 5: Write failing Kitchen consumer tests**

Change the service double to expose only `MarkOrderReady` and assert the paid event
passes the two identifiers while preserving all existing ACK/NACK behavior tests.

- [ ] **Step 6: Implement the Kitchen adapter and consumer call**

Replace the full-order update with `MarkOrderReady(ctx, orderID, customerID)`.

- [ ] **Step 7: Run Kitchen tests and verify GREEN**

Run: `cd internal/kitchen && go test -count=1 -race ./...`

Expected: PASS.

- [ ] **Step 8: Commit**

Commit: `refactor(workflow): use explicit order commands`

### Task 4: Documentation and whole-branch verification

**Files:**
- Modify: `docs/architecture/business-flow.md`
- Modify: `docs/architecture/data-and-state.md`
- Modify: `docs/architecture/reliability.md`
- Modify: `docs/architecture/evolution.md`
- Modify: `docs/project-evidence.md`
- Create: `docs/experiments/explicit-order-transitions.md`

**Interfaces:**
- Consumes: verified behavior from Tasks 1-3.
- Produces: source-grounded portfolio narrative and reproducible evidence commands.

- [ ] **Step 1: Update architecture and reliability claims**

Describe the four-state happy path, conditional updates, Stripe idempotency scope,
closed R2/R7/R11 portions, and the still-open Kitchen side-effect window.

- [ ] **Step 2: Add a reproducible experiment note**

Record the exact focused and full verification commands and expected invariants.

- [ ] **Step 3: Run full verification**

Run: `make verify`

Expected: PASS. Also run full Payment and Kitchen race suites, Order focused race
suites including ports, `docker compose config`, generated-code diff checks, Markdown
link checks, and privacy/secret scans.

- [ ] **Step 4: Commit**

Commit: `docs: explain explicit order transitions`
