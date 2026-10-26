# GOrder Portfolio Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a source-grounded, navigable architecture and development documentation set for the public GOrder portfolio repository.

**Architecture:** Keep the root README as a short entry point and move technical depth into focused documents under `docs/`. Every document separates current implementation, verified behavior, known gaps, and evolution proposals so public claims remain auditable.

**Tech Stack:** Markdown, Mermaid, Go source and tests as evidence, GitHub Actions, Docker Compose

**Spec:** `docs/superpowers/specs/2026-10-26-gorder-documentation-design.md`

## Global Constraints

- This round documents and explains the existing project; it does not add business functionality.
- Do not claim company production architecture, internal implementation, business data, Git history, scale, performance, or availability metrics.
- Do not present Saga, Transactional Outbox, persistent Inbox, inventory reservation, reconciliation, Redis locking, or one-command full-service startup as implemented.
- Use Chinese prose while preserving protocol names, patterns, and code identifiers in English.
- Every major claim must be classified as current implementation, verified behavior, known gap, evolution proposal, or unknown.
- Update the existing `codex/payment-idempotency` branch and PR; do not modify the dirty primary checkout.

## Review Focus

- Broken relative links: run a local Markdown-link checker and require zero unresolved repository-local targets.
- Future design presented as current behavior: scan for Outbox, Inbox, reservation, reconciliation, Saga, Redis, and verify each mention is explicitly scoped.
- Service behavior generalized from one verified slice: ensure ACK/NACK exclusivity is attributed only to the Order payment-result consumer.
- Deployment overstatement: ensure Compose is described as infrastructure-only and app processes as separately started.
- Stale facts after edits: rerun source searches and project verification before committing.

---

### Task 1: Evidence Map and Documentation Navigation

**Files:**
- Create: `docs/README.md`
- Create: `docs/project-evidence.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: current repository layout, existing payment redelivery experiment, source and test evidence.
- Produces: the canonical documentation map and evidence vocabulary used by all later architecture pages.

- [ ] **Step 1: Create `docs/README.md` with audience-based reading paths**

Include a recruiter path, architecture-review path, local-verification path, and links to every main document specified by the design.

- [ ] **Step 2: Create `docs/project-evidence.md`**

Record provenance, five evidence states, a current capability matrix, non-claims, and safe public descriptions. Link each verified payment-idempotency claim to its experiment or tests.

- [ ] **Step 3: Update `README.md` navigation and wording**

Keep the three-minute overview intact, add a concise documentation index, and align terminology with `project-evidence.md`.

- [ ] **Step 4: Verify navigation targets exist or are explicitly scheduled in later tasks**

Run: `find docs -maxdepth 3 -type f | sort`

Expected: the new map and evidence document exist; remaining links exactly match paths created by Tasks 2-5.

- [ ] **Step 5: Commit**

```bash
git add README.md docs/README.md docs/project-evidence.md
git commit -m "docs: define gorder evidence and navigation"
```

### Task 2: System Overview and Runtime Boundaries

**Files:**
- Create: `docs/architecture/overview.md`

**Interfaces:**
- Consumes: evidence vocabulary from Task 1; service composition, API contracts, adapters, Compose and observability configuration.
- Produces: canonical component names, service responsibilities, protocol boundaries, and deployment statements used by Tasks 3-5.

- [ ] **Step 1: Document system context and non-goals**

Add a Mermaid context diagram and prose defining Client, GOrder, payment Provider, data stores, RabbitMQ, Consul, and tracing/monitoring infrastructure.

- [ ] **Step 2: Document service/container architecture**

Add a Mermaid container diagram and tables for Order, Stock, Payment, Kitchen, shared Common code, APIs, storage, synchronous calls, and asynchronous events.

- [ ] **Step 3: Document runtime and observability boundaries**

State that Compose starts infrastructure only, apps start separately, Consul and Jaeger are wired into the skeleton, and metrics maturity must not be overstated.

- [ ] **Step 4: Cross-check named components against source**

Run: `rg -n "order\.create|order\.paid|RegisterService|NewTracerProvider|prometheus|redis" internal docker-compose.yml`

Expected: every implementation claim in the overview has a matching source/config location, or is rewritten as a gap/proposal.

- [ ] **Step 5: Commit**

```bash
git add docs/architecture/overview.md
git commit -m "docs: describe gorder system architecture"
```

### Task 3: Business Flows, Data Ownership, and State Model

**Files:**
- Create: `docs/architecture/business-flow.md`
- Create: `docs/architecture/data-and-state.md`

**Interfaces:**
- Consumes: canonical service names and protocol boundaries from Task 2.
- Produces: canonical flow names, state-transition vocabulary, and data ownership used by reliability and evolution docs.

- [ ] **Step 1: Document Create Order and Create Payment sequences**

Use Mermaid sequence diagrams and accompanying tables to show Stock check, Mongo write, `order.create`, payment link creation, and Order gRPC update. Mark the Mongo/RabbitMQ double-write window.

- [ ] **Step 2: Document Payment Result and Fulfillment sequences**

Show Stripe signature verification, `order.paid`, atomic `MarkPaid`, ACK/NACK, Kitchen processing, and generic Order update to `ready`. Mark ignored publish errors and consumer-control-flow gaps.

- [ ] **Step 3: Document data ownership and event semantics**

Describe Mongo order ownership, MySQL stock ownership, RabbitMQ notification semantics, Provider responsibility, and why cross-store global transactions are absent.

- [ ] **Step 4: Document the actual state model**

Add a state diagram for explicit `waiting_for_payment -> paid`; separately record `ready` as an observed generic update that is not fully modeled by domain constants. Contrast `MarkPaid` with generic `UpdateOrder`.

- [ ] **Step 5: Cross-check flows and states against source**

Run: `rg -n "waiting_for_payment|StatusPaid|ready|MarkPaid|UpdateOrder|CheckIfItemsInStock|PublishWithContext" internal`

Expected: every state and transition is traceable; stock is described as check-only.

- [ ] **Step 6: Commit**

```bash
git add docs/architecture/business-flow.md docs/architecture/data-and-state.md
git commit -m "docs: map gorder flows and state ownership"
```

### Task 4: Reliability Model and Evolution Roadmap

**Files:**
- Create: `docs/architecture/reliability.md`
- Create: `docs/architecture/evolution.md`

**Interfaces:**
- Consumes: flow failure windows, data owners, state semantics, and payment-redelivery experiment.
- Produces: the canonical current guarantees, residual risks, and explicitly non-current target architecture.

- [ ] **Step 1: Define current invariants and proven mechanisms**

For each invariant, record mechanism, evidence, and remaining risk. Cover conditional paid transition, duplicate no-op, tenant filter, concurrent replay, and Order consumer ACK/NACK exclusivity.

- [ ] **Step 2: Document broker retry and DLQ behavior**

Explain retry headers, delay, republish, max-attempt routing, and caller NACK behavior without claiming stronger delivery guarantees than the implementation supplies.

- [ ] **Step 3: Build the failure-window register**

Cover Create Order DB/MQ double-write, Stripe webhook publish failure, missing stable Provider event ID, Payment consumer fall-through, generic update races, check-only stock, and Kitchen duplicate side effects.

- [ ] **Step 4: Write the staged evolution roadmap**

Specify phases for consumer control-flow cleanup, Provider event identity and persistent Inbox, Transactional Outbox, inventory reservation/confirm/release, reconciliation, and fault-injection evidence. For each phase state problem, design, non-goals, and acceptance evidence.

- [ ] **Step 5: Audit terminology boundaries**

Run: `rg -n "Outbox|Inbox|Saga|Redis|预占|对账|保证|exactly.once|恰好一次" docs README.md`

Expected: each future mechanism is labeled as a proposal or unimplemented gap; guarantees are limited to tested behavior.

- [ ] **Step 6: Commit**

```bash
git add docs/architecture/reliability.md docs/architecture/evolution.md
git commit -m "docs: explain reliability boundaries and evolution"
```

### Task 5: Local Development and Full Documentation Verification

**Files:**
- Create: `docs/development/local-development.md`
- Modify: `docs/README.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: all preceding documents, Makefile, module files, configuration, Compose and CI workflow.
- Produces: a reproducible operator path and the final linked public documentation set.

- [ ] **Step 1: Document prerequisites, configuration, and safety boundaries**

List Go and container requirements, local-only defaults, Stripe environment variables, and the prohibition on committing real credentials.

- [ ] **Step 2: Document infrastructure and application startup paths**

Explain `docker compose up -d`, module-level process startup, known dependencies, ports/endpoints found in config, and the difference between infrastructure readiness and full business-flow readiness.

- [ ] **Step 3: Document verification and troubleshooting**

Explain `make verify`, integration-test exclusions, Compose validation, common missing-service/config failures, and how to identify which layer failed.

- [ ] **Step 4: Check all local Markdown links**

Run a repository-local script that extracts Markdown links, ignores external/anchor-only targets, resolves paths relative to each source file, and exits nonzero for missing files.

Expected: zero unresolved local targets.

- [ ] **Step 5: Run source and content audits**

Run: `rg -n "TP-?Link|Bybit|七牛|million|百万|千万|生产源码|真实生产" README.md docs`

Expected: no proprietary claim, invented metric, or wording that confuses the reconstruction with production source.

- [ ] **Step 6: Run project verification**

Run: `GOCACHE=/private/tmp/gorder-go-cache make verify`

Expected: all selected Go modules pass; Order payment-redelivery packages pass under the race detector.

- [ ] **Step 7: Validate Compose and patch hygiene**

Run: `docker compose config --quiet`

Expected: exit code 0.

Run: `git diff --check`

Expected: exit code 0.

- [ ] **Step 8: Commit and push to the existing PR branch**

```bash
git add README.md docs
git commit -m "docs: complete gorder engineering handbook"
git push
```
