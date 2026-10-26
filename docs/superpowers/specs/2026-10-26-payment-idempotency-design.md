# Payment Message Idempotency Design

## Intent

Close the smallest demonstrable payment-message failure loop: when the broker
redelivers the same paid-order message, the order reaches `paid` once, duplicate
delivery is a successful no-op, and one delivery is never both negatively and
positively acknowledged.

This is an independent reconstruction improvement. It is not evidence that this
exact implementation existed in a former production system.

## Goals

- Make `not-paid -> paid` one atomic repository operation.
- Treat an already-paid order as an idempotent success.
- ACK only after the business transition succeeds or is already complete.
- NACK malformed messages without requeue and return immediately.
- On a transient handler error, invoke the existing retry path, NACK the original
  delivery, and return without ACK.
- Prove first delivery, duplicate delivery, malformed payload, and handler-error
  behavior with deterministic unit tests.

## Non-goals

- Transactional Outbox for order creation.
- A durable Inbox table keyed by an external payment event ID.
- Inventory reservation, release, or reconciliation.
- Stripe webhook deduplication or publish-confirm handling.
- Repairing unrelated repository-wide vet failures and the existing live-server
  integration test.

## Constraints

- Work starts from clean commit `aab3a14` in an isolated worktree; the dirty main
  checkout remains untouched.
- The current order store is MongoDB. A single-document conditional update is the
  atomicity boundary for this slice.
- The paid event does not currently carry a durable external event ID. Therefore
  idempotency is scoped to the order's terminal paid transition, not arbitrary
  side effects attached to distinct payment events.
- No external service is required by the new unit tests.

## Approaches Considered

### 1. Consumer control-flow fix only

Add missing returns after `Nack`. This prevents `Nack` followed by `Ack`, but it
does not make the business effect idempotent. Rejected as insufficient.

### 2. Atomic order-state transition plus consumer acknowledgement discipline

Add a repository operation that updates one order only when it is not already
paid. A duplicate sees the terminal state and returns success without a second
write. Add a narrow paid-order command and make the consumer acknowledge based on
that command result. Selected for the first slice because it closes the observed
redelivery failure with a small, locally testable change.

### 3. Full durable Inbox with payment event ID

Propagate a stable provider event ID, insert it under a unique index, and update
the order in the same MongoDB transaction. This gives event-level deduplication,
but requires changes across webhook publishing, event schema, MongoDB deployment,
and transaction tests. Deferred to the next slice.

## Design

### Domain and repository

Extend the order repository with:

```go
MarkPaid(ctx context.Context, orderID, customerID, paymentLink string) (changed bool, err error)
```

MongoDB uses one conditional `UpdateOne` filter containing order ID, customer ID,
and `status == waiting_for_payment`. `ModifiedCount == 1` means the transition
occurred. If no document changed, the repository reads the current order: already
paid returns `false, nil`; a missing order or any other state returns an error.

The in-memory repository implements the same contract under its mutex so command
tests exercise real state transitions without MongoDB.

### Application command

Introduce `MarkOrderPaid` and `MarkOrderPaidHandler`. The handler validates the
event claims a paid state and delegates to `Repository.MarkPaid`. The returned
`changed` flag supports observability but both `true` and `false` are successful
consumer outcomes.

### Consumer

The order consumer follows one terminal path per delivery:

```text
malformed JSON         -> Nack(requeue=false) -> return
command success        -> Ack                 -> return
command error          -> retry handler
                       -> Nack(requeue=false) -> return
```

The existing retry function is injected behind a small function type so the
error path can be tested without a live RabbitMQ channel. Production construction
still defaults to the existing broker retry implementation.

## Verification

- Repository/command test: first paid message changes the order.
- Repository/command test: replaying the same paid message reports unchanged and
  preserves one paid result.
- Command test: a non-paid event is rejected without a repository write.
- Consumer test: valid success ACKs once and never NACKs.
- Consumer test: malformed JSON NACKs once and never invokes the command.
- Consumer test: command failure invokes retry, NACKs once, and never ACKs.
- Run targeted order-module tests with `go test -vet=off` and a writable temporary
  `GOCACHE`.

## Baseline Limitations

Before this change, the repository-wide test run is not green: unrelated vet
format errors exist in multiple modules, the legacy order integration test expects
a running HTTP service, and one payment dependency is not available in the
sandbox module cache. These failures remain recorded separately and are not used
to claim success or failure of this slice.

## Evolution Trigger

Move to the full Inbox design when a stable provider event ID is available or
when paid-message processing gains additional non-idempotent side effects that
cannot be protected by the order's terminal state transition alone.
