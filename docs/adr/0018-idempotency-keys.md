# ADR-0018: Idempotency keys as middleware with a per-school key table

| | |
|---|---|
| Status | Accepted (policies chosen by the product owner on 2026-11-15) |
| Date | 2026-11-15 |
| Spec links | REQ-API-010, DEC-004, ADR-0011, ADR-0017 |

## Context
A phone loses its connection after the server has acted. The app retries. Without a guard, a release would answer 409 for work that did happen, and a payment would be taken twice. REQ-API-010 says POSTs that create or move money accept an `Idempotency-Key` and repeats return the original result.

## Decision
1. **Middleware, not per-handler code.** `platform/idempotency` wraps the generated routes (registered through the generator's per-route middleware, so the matched route is known). Handlers and services know nothing about it.
2. **One table**, `idempotency_keys`, tenant-scoped with RLS, unique on `(tenant_id, key)`. It stores the endpoint, a SHA-256 of the user id, the concrete path, the query and the body, a status (`in_progress`, `done`, `failed`) and the response to replay. The API role can insert and update, never delete.
3. **Flow.** Claim the key (one atomic insert-or-take-over statement that mints an owner token) → run the handler into a buffer → store the response → send it. A repeat of the same request gets the stored response with `Idempotent-Replay: true`. Only the owner of the key can complete or free it, so a run that lost the key to a takeover cannot overwrite the new run's answer.
4. **Refusals.** Same key, different user, path, query, body or endpoint: 422 `idempotency.key_reused`. Same key while the first request is running: 409 `idempotency.in_progress` (owner choice: no waiting). Malformed key: 400 `idempotency.key_invalid`.
5. **What is stored.** Successful responses only (below 400). A client error did nothing, and a 5xx or panic did not complete, so both mark the key `failed` and a retry runs again against the current state. This also means a 403 is never replayed to someone who has since been given the permission. Keys older than 24 hours are treated as absent and taken over.
6. **Optional for now** (owner choice). A request without a key behaves as before. Payments will reject a request without a key.
7. **Which routes.** `idempotency.Operations` (generate, release, void, enrol student). The OpenAPI file declares the same set with the `IdempotencyKey` parameter; `TestINV_IdempotencyOperationsMatchTheContract` keeps the two equal.
8. **Anonymous requests and routes not on the list** pass through untouched.

## Found in review
A replay runs before the handler, so it skips the handler's permission check. Binding the key to the user (inside the request hash) closes that: another user gets 422, never the stored response. The route template is not the request either (`/challans/{id}/void`), so the concrete path is in the hash too. Both have tests, and mutation checks that remove them are killed.

## Options considered
- Per-handler checks inside each service transaction: gives exactly-once even across a crash, but every handler repeats the logic and every service needs the key. Rejected for now; revisit for payments if the crash window below proves a problem.
- Wait for the first request instead of 409: friendlier, but needs advisory locks and a timeout. Rejected by the owner.

## Consequences
- **5xx after a commit.** A 5xx or panic frees the key. If a handler ever committed money and then failed, a same-key retry would run it again. Today services commit as their last step, so this is not expected; payments should be built so that the money posting is the last step, or this decision revisited (owner decision 2026-11-15: keep freeing the key on a 5xx or panic; payments must make the money posting the last step of the transaction).
- **Crash window.** The business transaction and the stored response are separate commits. If the server dies between them, the key stays `in_progress` and retries get 409 for up to 24 hours; the client should use a new key. The business change itself happened exactly once, so no money is doubled. This is the price of keeping handlers unaware of keys.
- **No cleanup yet.** The API role cannot delete, so expired rows stay until an admin job removes them (needs the job queue, ADR-0005). They are harmless: expired rows are taken over.
- Responses are held in the database as bytes; the largest is a release of 200 challans.
- Adding a money-moving POST means adding the parameter to the contract and the route to `Operations`.
- Any authenticated caller can create key rows (before the permission check runs), and the API role cannot delete them. Row counts are bounded by request rate limits (not built yet) and by the cleanup job (ADR-0005).
- A claim that cannot settle after three attempts answers 409 `idempotency.in_progress`; it only happens under heavy expiry churn.
