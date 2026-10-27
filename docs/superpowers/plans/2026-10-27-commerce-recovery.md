# GOrder Commerce Recovery Implementation Plan

**Goal:** Close the current order/payment workflow defects and add finite-SKU reservation, confirmation, release and recoverable local demonstrations.

**Architecture:** Keep Go services, MySQL stock ownership, Mongo order ownership and RabbitMQ transport. Stock reservations use MySQL transactions and stable request identity. Payment uncertainty must delay inventory release until resolved.

**Spec:** Internal project review dated 2026-10-27 and the approved reservation scope. This file records decisions and implementation progress; README capabilities advance only with working code and verification.

## Constraints

- Preserve existing user changes; work on `codex/commerce-recovery` in the existing isolated checkout.
- Fix real paths before adding abstractions. No Redis locks, general Saga framework, UI or Kubernetes deployment.
- Preserve all-or-nothing item validation; nonpositive or overflowing quantities cannot reach stock/payment.
- Reservation identity binds the complete normalized SKU/quantity request. Multi-SKU acquisition is atomic.
- Confirmation and release are mutually exclusive, durable and idempotent. Local expiry alone never proves unpaid.
- New cross-service actions require durable intent/recovery. A deferred compensation call is insufficient.
- Verify by unit tests and isolated MySQL/RabbitMQ integration tests. Fake provider tests do not imply Stripe production verification.

## Tasks

- [x] 1. Fix cart validation, failed/missing prices, and the exposed unimplemented Stock RPC. Add regressions first; run the complete existing verify target.
- [x] 2. Fix stable durable subscriptions and consumer-specific retry routing. Cover actual RabbitMQ topology and confirms. Consumer channel closure is fail-fast and expects process-manager restart; in-process reconnection is not implemented.
- [x] 3. Add durable MySQL Reserve/Confirm/Release and contention tests. Persist stable identity, conflict detection, terminal transitions and expiry/recovery queries.
- [x] 4. Wire reservation into order/payment state and pending-intent recovery; gate paid/fulfilment on confirmed stock. Expiry release is intentionally disabled until Provider resolution exists; late payment after release remains a future exception path.
- [x] 5. Add isolated credential-free application-flow and fault scenarios, plus real MySQL/RabbitMQ integration suites. Update architecture, operational instructions and limitations. Live Stripe/Mongo end-to-end failure injection remains outside this slice.
- [x] 6. Review final diff, run full verification, create PR with exact implementation scope and evidence. Do not merge without user authorization for this code change.

## Verification gates

Task 1: negative/zero/nil/overflow cart items; incomplete/malformed Stock response; failed price cannot create a partial order; no-price product is an error; unsupported RPC cannot panic. Run `make verify` and compile Order main.

Tasks 2–5: reconnect/offline queues; a failing consumer cannot rebroadcast to a successful consumer; unavailable broker cannot masquerade as durable acceptance; last-unit contention; multi-SKU rollback; same-key mismatch; concurrent confirm/release; uncertain payment never releases inventory; crash after reserve recovers; demo reaches ready and stock conservation holds.

## Execution log

- Baseline: existing `make verify` and Order main compilation passed during review. Local tree equals published main `85f3981`. No implementation changes at plan creation.
- Cart/price regressions and exposed Stock RPC repaired; `make verify` passed after the changes.
- Isolated RabbitMQ suite passed for durable paid subscriptions, targeted retry, publisher confirms and unroutable rejection. The application consumer does not yet reconnect automatically after its channel closes.
- Isolated MySQL suite passed for atomic reservation, last-unit contention, idempotent replay, conflict, terminal-state race, and read-only expiry scan. Stock RPCs and Order checkout Reserve are wired.
- Order persists pending intent and retries unconfirmed creation events before expiry. Signed paid webhook confirms stock; Order verifies confirmed stock before paid; Kitchen verifies persisted paid before simulated work.
- Offline application flow and real isolated MySQL/RabbitMQ suites passed on 2026-10-27. Live Stripe, full Mongo crash-point and Provider-reconciliation tests have not been run.
- Independent review found an uncertain publisher-confirmation reuse bug and finite Kitchen retry that could strand a valid paid event. Both were fixed and retested. PR: https://github.com/jiahuipaung/gorder/pull/2; push and PR test/integration jobs passed. No merge performed.
