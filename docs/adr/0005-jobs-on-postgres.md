# ADR-0005: Background jobs on a Postgres-backed queue

| | |
|---|---|
| Status | Proposed |
| Date | 2026-10-28 |
| Spec links | DEC-005, REQ-API-014 |

## Context
PDF generation, imports, notifications and reports must run outside the request, reliably, with retries. A broker is more to operate.

## Options considered
1. **Postgres-backed queue (e.g. River)**: transactional enqueue with business writes, no extra broker.
2. **Redis queue (asynq)**: fast, but not transactional with Postgres writes.
3. **RabbitMQ/Kafka**: powerful, far more than needed.

## Decision
Use a Postgres-backed queue, enqueued in the same transaction as the business change. Library confirmed with a spike during the first async feature.

## Consequences
- Good: no lost jobs between commit and enqueue, one less moving part.
- Bad: queue load shares the database; revisit if job volume grows large.
