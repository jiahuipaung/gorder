# Payment Message Idempotency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make duplicate paid-order deliveries converge to one atomic order-state transition with mutually exclusive ACK/NACK behavior and reproducible tests.

**Architecture:** Add an explicit `MarkPaid` repository contract and a narrow application command rather than reusing the generic closure-based update command. MongoDB performs a conditional single-document update from `waiting_for_payment` to `paid`; the consumer treats both the first transition and an already-paid duplicate as success, while malformed or failed processing terminates through one NACK path.

**Tech Stack:** Go 1.23, MongoDB Go Driver, RabbitMQ AMQP delivery interface, standard `testing`, Testify, MongoDB `mtest` mock deployment.

**Spec:** `docs/superpowers/specs/2026-10-26-payment-idempotency-design.md`

## Global Constraints

- Work only in the isolated `codex/payment-idempotency` worktree based on commit `aab3a14`; do not copy, clean, or commit the dirty main checkout.
- Follow strict TDD: add one behavior test, observe the expected failure, implement the minimum, and rerun the relevant suite before the next behavior.
- New tests must not require a live MongoDB, RabbitMQ, HTTP service, or payment provider.
- Scope is limited to paid-message state transition and delivery acknowledgement; Outbox, durable Inbox, inventory, and webhook publish confirms remain out of scope.
- A duplicate is successful only when the same order is already `paid`; missing orders, mismatched customers, and other current states are errors.
- Existing unrelated full-suite failures must remain reported rather than fixed opportunistically.

## Review Focus

- Two concurrent paid commands for one order: exactly one returns `changed=true`, the other is a successful no-op.
- A valid event for the wrong customer must not observe or update another customer's order.
- Malformed JSON is a poison message: NACK once without requeue, no command, no ACK.
- Handler/retry failure must still end with exactly one NACK and never fall through to ACK.
- MongoDB's update filter must contain order ID, customer ID, and `waiting_for_payment`; otherwise the adapter could silently broaden the write.

---

### Task 1: Define the domain transition and in-memory repository contract

**Files:**
- Modify: `internal/order/domain/order/order.go`
- Modify: `internal/order/domain/order/repository.go`
- Modify: `internal/order/adapters/order_inmem_repository.go`
- Create: `internal/order/adapters/order_inmem_repository_test.go`

**Interfaces:**
- Produces: `Repository.MarkPaid(ctx context.Context, orderID, customerID, paymentLink string) (bool, error)`.
- Produces: `StatusWaitingForPayment`, `StatusPaid`, and `InvalidStatusTransitionError` in the order domain.

- [ ] **Step 1: Write the failing first-transition test** — `TestMemoryOrderRepositoryMarkPaidTransitionsWaitingOrder` seeds a waiting order, calls `MarkPaid`, and asserts `changed=true`, status `paid`, and the supplied payment link.
- [ ] **Step 2: Run the test and verify RED** — run `env GOCACHE=/private/tmp/gorder-go-build-cache go test -vet=off ./adapters -run TestMemoryOrderRepositoryMarkPaidTransitionsWaitingOrder -v`; expect compile failure because `MarkPaid` does not exist.
- [ ] **Step 3: Implement the minimal domain constants, error type, repository signature, and locked in-memory transition.**
- [ ] **Step 4: Run the test and verify GREEN.**
- [ ] **Step 5: Write and verify RED for duplicate delivery** — `TestMemoryOrderRepositoryMarkPaidTreatsPaidOrderAsNoOp` must assert `changed=false`, no error, and unchanged payment data on the second call.
- [ ] **Step 6: Implement the duplicate no-op and verify GREEN.**
- [ ] **Step 7: Add table tests for missing order, wrong customer, and invalid current state** — each must return a typed error and leave the store unchanged; observe RED before adding each missing branch.
- [ ] **Step 8: Add the concurrent replay test** — `TestMemoryOrderRepositoryMarkPaidConcurrentReplayChangesOnce` starts multiple goroutines and asserts exactly one `changed=true`; run with `-race` and verify GREEN.
- [ ] **Step 9: Run `env GOCACHE=/private/tmp/gorder-go-build-cache go test -race -vet=off ./adapters ./domain/order` and commit `feat(order): add atomic paid transition contract`.**

### Task 2: Implement and verify the MongoDB conditional update

**Files:**
- Modify: `internal/order/adapters/order_mongo_repository.go`
- Create: `internal/order/adapters/order_mongo_repository_test.go`

**Interfaces:**
- Consumes: `Repository.MarkPaid(...)` and the domain statuses/errors from Task 1.
- Produces: MongoDB adapter behavior equivalent to the in-memory implementation.

- [ ] **Step 1: Write the failing Mongo success test** — use `mtest.ClientType(mtest.Mock)` to return one modified document; assert `changed=true` and inspect the started update command for `_id`, `customer_id`, and `status=waiting_for_payment`.
- [ ] **Step 2: Run the adapter test and verify RED** because `OrderRepositoryMongo.MarkPaid` is missing.
- [ ] **Step 3: Implement `MarkPaid` with a single conditional `UpdateOne`; `ModifiedCount == 1` returns `true, nil`.**
- [ ] **Step 4: Run the test and verify GREEN.**
- [ ] **Step 5: Write failing mock-deployment tests for zero modified rows** — an existing `paid` document returns `false, nil`; missing, wrong-customer, and other-state documents return the corresponding typed error.
- [ ] **Step 6: Implement the read-after-no-match classification; include `customer_id` in the lookup boundary.**
- [ ] **Step 7: Run `env GOCACHE=/private/tmp/gorder-go-build-cache go test -vet=off ./adapters -run 'TestOrderRepositoryMongoMarkPaid' -v` and commit `feat(order): persist paid transition atomically`.**

### Task 3: Add the paid-order application command

**Files:**
- Create: `internal/order/app/command/mark_order_paid.go`
- Create: `internal/order/app/command/mark_order_paid_test.go`
- Modify: `internal/order/app/app.go`
- Modify: `internal/order/service/application.go`

**Interfaces:**
- Consumes: `Repository.MarkPaid(...)` from Task 1.
- Produces: `MarkOrderPaid{Order *order.Order}` and `MarkOrderPaidHandler` returning `(bool, error)`.

- [ ] **Step 1: Write the failing command success/replay table test** — use the real in-memory repository and assert first call `true`, replay `false`, both without error.
- [ ] **Step 2: Run and verify RED because the command does not exist.**
- [ ] **Step 3: Implement the command and constructor; register it in `app.Commands` and `service.newApplication`.**
- [ ] **Step 4: Run and verify GREEN.**
- [ ] **Step 5: Write the failing validation tests** — nil order and an event status other than `paid` return errors without changing repository state.
- [ ] **Step 6: Implement validation and verify GREEN.**
- [ ] **Step 7: Run `env GOCACHE=/private/tmp/gorder-go-build-cache go test -vet=off ./app/command -v` and commit `feat(order): add mark-paid command`.**

### Task 4: Make consumer acknowledgement paths mutually exclusive

**Files:**
- Modify: `internal/order/infrastructure/consumer/consumer.go`
- Create: `internal/order/infrastructure/consumer/consumer_test.go`

**Interfaces:**
- Consumes: `app.Commands.MarkOrderPaid` from Task 3.
- Produces: `RetryHandler func(context.Context, *amqp.Channel, *amqp.Delivery) error`, defaulting to `broker.HandleRetry` in production.

- [ ] **Step 1: Write the failing success test** — a fake AMQP acknowledger and successful command must observe one ACK and zero NACKs.
- [ ] **Step 2: Run and verify RED because the consumer still calls `UpdateOrder`.**
- [ ] **Step 3: Switch the consumer to `MarkOrderPaid` and return immediately after the success ACK; verify GREEN.**
- [ ] **Step 4: Write the failing malformed-payload test** — assert one non-requeue NACK, zero ACKs, and zero command calls.
- [ ] **Step 5: Implement poison-message NACK plus return; verify GREEN.**
- [ ] **Step 6: Write the failing handler-error test** — inject a deterministic retry function; assert one retry call, one NACK, and zero ACKs even when retry itself errors.
- [ ] **Step 7: Inject the default retry handler, terminate the error path after NACK, and verify GREEN.**
- [ ] **Step 8: Run `env GOCACHE=/private/tmp/gorder-go-build-cache go test -race -vet=off ./infrastructure/consumer -v` and commit `fix(order): make delivery acknowledgements exclusive`.**

### Task 5: Produce the reproducible portfolio evidence

**Files:**
- Create: `docs/experiments/payment-redelivery.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: all tests and behavior from Tasks 1–4.
- Produces: a third-party-readable reproduction command, expected outcomes, evidence boundary, and known limitations.

- [ ] **Step 1: Run the targeted package suite** — `env GOCACHE=/private/tmp/gorder-go-build-cache go test -race -vet=off ./adapters ./app/command ./domain/order ./infrastructure/consumer` from `internal/order`; require zero failures.
- [ ] **Step 2: Re-run the order module suite** — `env GOCACHE=/private/tmp/gorder-go-build-cache go test -vet=off ./...`; record the legacy live-server integration failure if it remains, without claiming a green full module.
- [ ] **Step 3: Run `git diff --check`, inspect `git diff --stat`, and verify the original dirty main checkout status hash remains unchanged from the audit snapshot.**
- [ ] **Step 4: Write `payment-redelivery.md` with the failure window, chosen invariant, exact verification command, observed result, and the limitation that this is order-state idempotency rather than a durable event Inbox.**
- [ ] **Step 5: Link the experiment from README and commit `docs: add payment redelivery experiment`.**

## Self-Review

- **Spec coverage:** Tasks 1–4 cover atomic state transition, duplicate no-op, ACK/NACK exclusivity, malformed input, retry failure, and no-external-service tests. Task 5 makes the evidence reproducible and preserves the historical/reconstruction boundary.
- **Step scan:** Each implementation step has one behavior and an immediately preceding failing test; unrelated Outbox, inventory, and webhook work is excluded.
- **Type consistency:** `Repository.MarkPaid(...) (bool, error)` flows unchanged through both adapters and `MarkOrderPaidHandler`; the consumer treats either boolean value as success.
- **Review focus:** Concurrent replay, tenant/customer isolation, poison messages, retry fallthrough, and the Mongo conditional filter each have an owning test.
- **Proportion:** The plan specifies interfaces, assertions, commands, and commits without pre-writing implementation bodies.
