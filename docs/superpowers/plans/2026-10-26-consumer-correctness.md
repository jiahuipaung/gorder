# Consumer Correctness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repair the verified consumer and webhook control-flow gaps so every RabbitMQ delivery has one explicit terminal outcome, downstream failures remain visible to the consumer, and a paid Stripe webhook returns success only after its RabbitMQ publish succeeds.

**Architecture:** Keep the existing service boundaries and RabbitMQ topology. Make delivery outcomes explicit inside each consumer, inject only the narrow retry/publish/cook seams required for deterministic tests, and retain deferred Outbox/Inbox/publisher-confirm work as documented limitations rather than simulating guarantees that do not exist.

**Tech Stack:** Go, RabbitMQ AMQP (`amqp091-go`), gRPC/protobuf, Gin, Stripe Go SDK v81, Testify, Make, Docker Compose.

**Spec:** `docs/superpowers/specs/2026-10-26-consumer-correctness-design.md`

## Global Constraints

- Preserve the existing public service boundaries, routing keys, protobuf contracts, and RabbitMQ topology.
- Apply test-driven development: add or change the focused test first, run it and observe the expected failure, then write the minimum production change.
- A delivery may call `Ack` or `Nack` at most once. Do not add a fallback confirmation after either call.
- A successful retry/DLQ handoff ACKs the original delivery; a failed handoff NACKs the original with `requeue=true`.
- Malformed or permanently invalid messages NACK once with `requeue=false`.
- Do not add Outbox, Inbox, publisher confirms, new storage, inventory behavior, or Kitchen side-effect deduplication in this slice.
- Keep production defaults unchanged where behavior is not part of the defect: Payment still uses Stripe, Kitchen still uses the five-second simulated cook, and consumers still use the existing broker retry helper.
- Commit each completed task separately so the PR tells a reviewable story.

## Review Focus

1. Nil payment orders must fail before Stripe or gRPC is called.
2. Malformed deliveries must stop immediately and receive exactly one non-requeue NACK.
3. Retry publication failure must preserve the original message via exactly one requeue NACK.
4. A paid webhook with an invalid signature or failed RabbitMQ publish must never return HTTP 200.
5. Kitchen must reject non-paid orders without cooking, gRPC calls, or retry publication.

---

## Task 1: Propagate Payment Command Failures

**Files:**

- Create: `internal/payment/app/command/create_payment_test.go`
- Modify: `internal/payment/app/command/create_payment.go`

- [ ] **Step 1: Add focused command tests with fakes**

  Cover these cases through the public command handler:

  - nil `Order` returns an error and calls neither `Processor` nor `OrderService`;
  - processor failure is returned and does not call `OrderService`;
  - `OrderService.UpdateOrder` failure is returned rather than swallowed;
  - both dependencies succeeding returns the payment link and a nil error.

  Record call counts and the order passed to `UpdateOrder`, including the expected `waiting_for_payment` status and generated payment link.

- [ ] **Step 2: Run the focused tests and confirm RED**

  Run:

  ```bash
  cd internal/payment && go test -count=1 ./app/command
  ```

  Expected: the nil-order or gRPC-error tests fail against the current implementation.

- [ ] **Step 3: Make command error handling explicit**

  In `create_payment.go`:

  - return immediately with a descriptive error for a nil order;
  - leave processor errors unchanged;
  - return the `UpdateOrder` error instead of returning `link, nil`;
  - return `link, nil` only after both operations succeed.

- [ ] **Step 4: Re-run the command package tests**

  ```bash
  cd internal/payment && go test -count=1 ./app/command
  ```

  Expected: PASS.

- [ ] **Step 5: Commit the command fix**

  ```bash
  git add internal/payment/app/command/create_payment.go internal/payment/app/command/create_payment_test.go
  git commit -m "fix(payment): propagate payment creation errors"
  ```

## Task 2: Give the Payment Consumer One Terminal Outcome

**Files:**

- Create: `internal/payment/infrastructure/consumer/consumer_test.go`
- Modify: `internal/payment/infrastructure/consumer/consumer.go`

- [ ] **Step 1: Add a retry seam and delivery spies in tests**

  Build the consumer with a command-handler fake and an injected retry function. Use an AMQP acknowledgement spy to count and capture `Ack`/`Nack` calls. Test:

  - valid message + command success: handler once, ACK once, no retry/NACK;
  - malformed JSON: handler/retry never, NACK once with `requeue=false`;
  - command error + retry success: retry once, ACK once, no NACK;
  - command error + retry failure: retry once, NACK once with `requeue=true`, no ACK.

- [ ] **Step 2: Run the Payment consumer tests and confirm RED**

  ```bash
  cd internal/payment && go test -count=1 ./infrastructure/consumer
  ```

  Expected: current code double-confirms malformed/error deliveries or lacks the injection seam.

- [ ] **Step 3: Implement explicit branches**

  In `consumer.go`:

  - define a narrow `RetryHandler` function type matching `broker.HandleRetry`;
  - store it on the consumer and default it to `broker.HandleRetry` in production construction;
  - after malformed JSON, NACK once without requeue and return;
  - after command success, ACK once and return;
  - after command failure, call retry once; ACK the original if retry succeeds, otherwise NACK once with requeue and return;
  - log payment creation success only on the true success branch.

- [ ] **Step 4: Re-run tests, including the command package**

  ```bash
  cd internal/payment && go test -count=1 ./app/command ./infrastructure/consumer
  ```

  Expected: PASS.

- [ ] **Step 5: Commit the consumer fix**

  ```bash
  git add internal/payment/infrastructure/consumer/consumer.go internal/payment/infrastructure/consumer/consumer_test.go
  git commit -m "fix(payment): make consumer outcomes explicit"
  ```

## Task 3: Preserve Order Deliveries When Retry Publication Fails

**Files:**

- Modify: `internal/order/infrastructure/consumer/consumer_test.go`
- Modify: `internal/order/infrastructure/consumer/consumer.go`

- [ ] **Step 1: Update and extend retry-path tests**

  Keep the existing success and malformed-message cases, then split handler failure into:

  - retry succeeds: retry once, ACK once, no NACK;
  - retry fails: retry once, NACK once with `requeue=true`, no ACK.

  Assert total confirmation calls remain one in every case.

- [ ] **Step 2: Run the focused tests and confirm RED**

  ```bash
  cd internal/order && go test -count=1 ./infrastructure/consumer
  ```

  Expected: the current retry-success and retry-failure terminal outcomes do not match the delivery contract.

- [ ] **Step 3: Implement the retry decision**

  In `consumer.go`, keep malformed and successful handler branches unchanged, but for handler failure:

  - ACK and return when the retry helper succeeds;
  - NACK with `requeue=true` and return when the retry helper fails.

- [ ] **Step 4: Verify the package with the race detector**

  ```bash
  cd internal/order && go test -race -count=1 ./infrastructure/consumer
  ```

  Expected: PASS with no race report.

- [ ] **Step 5: Commit the Order consumer fix**

  ```bash
  git add internal/order/infrastructure/consumer/consumer.go internal/order/infrastructure/consumer/consumer_test.go
  git commit -m "fix(order): retain deliveries when retry fails"
  ```

## Task 4: Make Kitchen Processing and Confirmation Deterministic

**Files:**

- Create: `internal/kitchen/infrastructure/consumer/consumer_test.go`
- Modify: `internal/kitchen/infrastructure/consumer/consumer.go`

- [ ] **Step 1: Add injectable cook/retry test seams and path tests**

  Use a fake cook function so tests do not sleep, a fake Order gRPC client, an injected retry handler, and an AMQP acknowledgement spy. Cover:

  - malformed JSON: no cook/gRPC/retry, one non-requeue NACK;
  - non-paid order: no cook/gRPC/retry, one non-requeue NACK;
  - paid order + gRPC success: cook once, gRPC once, ACK once;
  - paid order + gRPC error + retry success: cook once, retry once, ACK once;
  - paid order + gRPC error + retry failure: cook once, retry once, NACK once with `requeue=true`.

- [ ] **Step 2: Run the Kitchen consumer tests and confirm RED**

  ```bash
  cd internal/kitchen && go test -count=1 ./infrastructure/consumer
  ```

  Expected: current deferred confirmation logic double-confirms malformed input or cannot be tested without the real cook delay.

- [ ] **Step 3: Replace implicit deferred confirmation with explicit outcomes**

  In `consumer.go`:

  - introduce narrow `RetryHandler` and `CookHandler` function types;
  - default them to `broker.HandleRetry` and the existing `cook` behavior in production;
  - remove the shared-error/deferred ACK/NACK logic;
  - return immediately after malformed JSON or a non-paid status with one non-requeue NACK;
  - call cook once, then Order gRPC once for valid paid orders;
  - ACK on gRPC success;
  - on gRPC error, ACK after a successful retry handoff or NACK with requeue after retry failure.

- [ ] **Step 4: Re-run the focused package tests**

  ```bash
  cd internal/kitchen && go test -count=1 ./infrastructure/consumer
  ```

  Expected: PASS without a five-second test delay.

- [ ] **Step 5: Commit the Kitchen consumer fix**

  ```bash
  git add internal/kitchen/infrastructure/consumer/consumer.go internal/kitchen/infrastructure/consumer/consumer_test.go
  git commit -m "fix(kitchen): enforce one delivery outcome"
  ```

## Task 5: Return Webhook Success Only After Event Publication

**Files:**

- Create: `internal/payment/http_test.go`
- Modify: `internal/payment/http.go`

- [ ] **Step 1: Add signed webhook tests around a publisher fake**

  Generate authentic Stripe test signatures with `webhook.GenerateTestSignedPayload`. Use a paid `checkout.session.completed` payload with order/customer/item metadata and cover:

  - publish success: HTTP 200, one publish to the existing `order.paid` exchange/routing target;
  - publish failure: HTTP 503, one publish attempt;
  - invalid signature: HTTP 400, no publish attempt.

  Restore any Viper configuration changed by the test so package order cannot affect the result.

- [ ] **Step 2: Run the Payment HTTP tests and confirm RED**

  ```bash
  cd internal/payment && go test -count=1 .
  ```

  Expected: the current handler cannot accept a fake publisher or returns HTTP 200 after a publish error.

- [ ] **Step 3: Introduce the minimal publisher interface**

  In `http.go`:

  - define a `Publisher` interface containing the existing `PublishWithContext` signature;
  - change `PaymentHandler` and its constructor to depend on that interface; `*amqp.Channel` remains the production implementation;
  - check the publication error;
  - log and return HTTP 503 immediately on failure;
  - return HTTP 200 only after successful publication for a paid session;
  - preserve current behavior for invalid signatures, unhandled event types, and unpaid sessions.

- [ ] **Step 4: Re-run all Payment tests**

  ```bash
  cd internal/payment && go test -count=1 ./...
  ```

  Expected: PASS.

- [ ] **Step 5: Commit the webhook fix**

  ```bash
  git add internal/payment/http.go internal/payment/http_test.go
  git commit -m "fix(payment): surface webhook publish failures"
  ```

## Task 6: Align Portfolio Documentation and Verify the Repository

**Files:**

- Create: `docs/experiments/consumer-correctness.md`
- Modify: `README.md`
- Modify: `docs/architecture/business-flow.md`
- Modify: `docs/architecture/reliability.md`
- Modify: `docs/architecture/evolution.md`
- Modify: `docs/project-evidence.md`

- [ ] **Step 1: Update the implementation evidence**

  Document only what the tests and source now prove:

  - the single-terminal-outcome delivery contract;
  - retry-success ACK and retry-failure requeue behavior;
  - Payment command error propagation;
  - webhook HTTP 503 on publication failure;
  - Kitchen's remaining side-effect duplication window;
  - the continued absence of Outbox, Inbox, publisher confirms, inventory reservation, and provider event ledgers.

- [ ] **Step 2: Add a reproducible consumer-correctness experiment**

  In `docs/experiments/consumer-correctness.md`, include:

  - the failure hypotheses;
  - the focused test commands for Payment, Order, Kitchen, and webhook;
  - a decision table mapping each failure to ACK/NACK/requeue;
  - expected observations and known limitations;
  - the distinction between verified unit behavior and unverified broker-level guarantees.

- [ ] **Step 3: Run formatting and focused module tests**

  ```bash
  gofmt -w internal/payment/app/command/create_payment.go internal/payment/app/command/create_payment_test.go internal/payment/infrastructure/consumer/consumer.go internal/payment/infrastructure/consumer/consumer_test.go internal/payment/http.go internal/payment/http_test.go internal/order/infrastructure/consumer/consumer.go internal/order/infrastructure/consumer/consumer_test.go internal/kitchen/infrastructure/consumer/consumer.go internal/kitchen/infrastructure/consumer/consumer_test.go
  make verify
  ```

  Expected: formatting makes no semantic changes and all repository verification targets pass.

- [ ] **Step 4: Run additional integrity checks**

  ```bash
  cd internal/order && go test -race -count=1 ./...
  docker compose config --quiet
  git diff --check main..HEAD
  ```

  Run the repository's existing local Markdown-link checker. If no checker target exists, enumerate Markdown links and verify relative paths without adding a new dependency.

  Inspect the final diff for accidental secrets, company-only identifiers, generated artifacts, and claims not backed by code/tests.

  Expected: all checks pass and no sensitive or unsupported content appears.

- [ ] **Step 5: Commit the evidence update**

  ```bash
  git add README.md docs/architecture/business-flow.md docs/architecture/reliability.md docs/architecture/evolution.md docs/project-evidence.md docs/experiments/consumer-correctness.md
  git commit -m "docs: record consumer correctness guarantees"
  ```

- [ ] **Step 6: Perform final verification from a clean tree**

  ```bash
  git status --short
  make verify
  docker compose config --quiet
  git diff --check main..HEAD
  ```

  Expected: clean worktree and all commands PASS.

- [ ] **Step 7: Push the branch and verify PR checks**

  Push `codex/payment-idempotency` to the existing PR #1 using the repository's authenticated GitHub path, then wait for all PR checks to reach a terminal state. Do not report completion until required checks pass; report any external or infrastructure failure separately from code failures.
