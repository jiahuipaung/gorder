# Explicit Order Transitions Design

## Context

GOrder currently lets Payment and Kitchen send a complete `Order` snapshot to the
generic `UpdateOrder` RPC. The Mongo repository then overwrites `status` and
`payment_link` without an expected-state predicate. It also starts a transaction
without using a `mongo.SessionContext`, so the transaction does not protect the
read/write sequence. This is failure window R7/R11 in the reliability document.

The previous slice made consumer acknowledgement paths explicit and made
`waiting_for_payment -> paid` atomic. This slice applies the same idea to the two
remaining writes without introducing Inbox, Outbox, Saga, or new infrastructure.

## Goals

- Persist new orders with the explicit status `pending`.
- Replace the generic cross-service update with two intent-specific RPCs:
  `AttachPaymentSession` and `MarkOrderReady`.
- Enforce each transition atomically in MongoDB and consistently in the in-memory
  repository.
- Make legitimate redelivery a no-op and reject conflicting or out-of-order writes.
- Reuse the order ID as Stripe's idempotency key so Payment redelivery requests the
  same Checkout Session rather than creating an unbounded series of sessions.
- Keep the change small enough to explain and verify as one portfolio slice.

## Non-goals

- No Inbox, Outbox, event envelope, publisher confirms, or reconciliation worker.
- No inventory reservation or cancellation/refund flow.
- No claim/lease protocol for Kitchen. `cook` can still run twice if the worker
  crashes after the side effect and before the ready update.
- No public HTTP API redesign and no attempt to preserve the internal generic
  `UpdateOrder` RPC; all callers live in this repository and migrate together.

## State model and invariants

The modeled happy path becomes:

```text
pending -> waiting_for_payment -> paid -> ready
```

The repository owns these rules:

| Command | First application | Exact replay | Other current state |
|---|---|---|---|
| `AttachPaymentSession(orderID, customerID, link)` | `pending -> waiting_for_payment`, stores non-empty link | `waiting_for_payment` with the same link is a no-op | not found or `InvalidStatusTransitionError` |
| `MarkPaid(orderID, customerID, link)` | `waiting_for_payment -> paid` | `paid` is a no-op; an empty event link does not erase the stored link | not found or `InvalidStatusTransitionError` |
| `MarkReady(orderID, customerID)` | `paid -> ready` | `ready` is a no-op | not found or `InvalidStatusTransitionError` |

`AttachPaymentSession` rejects an empty link before persistence. A replay carrying a
different link is a conflict, not an overwrite. This makes stale snapshots visible.

Each Mongo first application is one `UpdateOne` with `_id`, `customer_id`, and the
expected current `status` in the filter. A zero-match result is classified with a
read: exact replay returns `changed=false`; otherwise the repository returns the
existing not-found or invalid-transition error. No multi-document transaction is
needed.

## Service boundary

The protobuf service exposes:

```text
AttachPaymentSession(order_id, customer_id, payment_link)
MarkOrderReady(order_id, customer_id)
```

Both return `google.protobuf.Empty`. Application commands call matching repository
methods. The Payment and Kitchen adapters expose the same intent-specific methods;
they no longer construct mutable order snapshots.

The legacy `UpdateOrder(Order)` RPC, application command, and repository method are
removed because keeping an unrestricted escape hatch would defeat the invariant.

## Stripe idempotency

Checkout Session creation sets `Idempotency-Key` to the stable order ID. The request
parameters for a given order must remain stable across delivery retries. This closes
the common retry window where Stripe accepted the first request but the Order gRPC
write or RabbitMQ acknowledgement failed.

This does not replace a local payment ledger or provider reconciliation. It only
makes identical Checkout Session creation attempts safe within Stripe's idempotency
retention semantics.

## Error and retry semantics

- Invalid protobuf input is returned as `InvalidArgument`.
- Missing orders return `NotFound`.
- Invalid or conflicting transitions return `FailedPrecondition`.
- Infrastructure failures return `Internal`.
- Payment and Kitchen continue returning adapter errors to their consumers, which
  use the already-tested retry/DLQ path.

## Verification

- Repository tests cover first apply, exact replay, wrong state, conflicting link,
  wrong customer, and Mongo conditional filters.
- Command and gRPC tests cover validation and status-code mapping.
- Payment tests prove the explicit RPC is used and Checkout Session params carry the
  stable idempotency key.
- Kitchen tests prove only `MarkOrderReady` is called for a paid event.
- `make verify`, focused race tests, generated-code cleanliness, and documentation
  link checks remain the final gates.
