# 04. API Conventions (API-First)

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | API |

## 1. Principles

- **REQ-API-001** The OpenAPI 3 specification is the source of truth. It lives in the repository, is reviewed like code, and every change is a pull request.
- **REQ-API-002** Every product capability MUST exist as an endpoint. The web dashboard and mobile apps are clients of the same API. No private or hidden APIs.
- **REQ-API-003** Anything doable in the portal is doable from a terminal with an API token and the same permission checks and audit.
- **REQ-API-004** Documentation is generated from the spec; interactive reference with `curl` examples for every operation.
- **REQ-API-005** Typed clients (TypeScript for web and React Native) and server interfaces are generated from the spec.

## 2. Style

| Topic | Rule |
|---|---|
| Base path | `/api/v1/...` (versioned; breaking change means a new major version) |
| Resources | Plural nouns: `/students`, `/families`, `/challans`, `/payments` |
| Methods | `GET` read, `POST` create/commands, `PATCH` partial update, `PUT` replace, `DELETE` only for non-business items (business records are deactivated, never deleted) |
| Commands | State transitions as sub-resources: `POST /challans/{id}/release`, `POST /payments/{id}/void`, `POST /salaries/{id}/approve` |
| IDs | UUID strings |
| Format | JSON; dates ISO-8601; money as `{ "amount": 15000, "currency": "PKR" }` in minor units |
| Pagination | Cursor based: `?limit=50&cursor=...`; response includes `next_cursor` |
| Filtering/sorting | `?filter[status]=unpaid&sort=-due_date` |
| Bulk | Bulk endpoints accept arrays and return per-item results; large jobs are asynchronous |
| Tenant | Derived from the token; never passed by clients as a trusted field |

## 3. Authentication and authorisation

- Bearer token (session token for apps/web, personal access token for terminal/scripts).
- Permission check on every endpoint against the union of the caller's role assignments and scopes (see IAM).
- Responses never leak existence of records outside scope (404 for out-of-scope resources).

Terminal example (illustrative):

```bash
curl -X POST https://api.example.com/api/v1/students \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: 7f3c-..." \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Ali Khan","father_name":"Imran Khan","guardians":[{"name":"Kamran Khan","relationship":"uncle","mobile":"03001234567","primary":true,"fee_payer":true}],"section_id":"..."}'
```

## 4. Reliability rules

- **REQ-API-010 Idempotency.** All `POST` commands that create or move money accept an `Idempotency-Key`; repeats return the original result. Mobile sync operations are idempotent by client-generated IDs.
- **REQ-API-011 Errors** use one format: `{ "error": { "code": "fee.challan.already_released", "message": "...", "details": [...], "request_id": "..." } }`. Codes are stable and documented; messages are human-readable English.
- **REQ-API-012 Rate limits** per token, per tenant and per IP, with `429` and `Retry-After`.
- **REQ-API-013 Concurrency.** Updates use `ETag`/`If-Match` or a `version` field; conflicts return `409`.
- **REQ-API-014 Async jobs.** Long work (bulk PDFs, imports, report exports) returns `202` with a job resource: `GET /jobs/{id}`; results via "Downloads".
- **REQ-API-015 Webhooks.** Outgoing signed webhooks for key events are [1.1]; the payment aggregator's incoming webhooks are signature-verified and replay-safe in 1.0.
- **REQ-API-016 Deprecation.** Deprecated endpoints are marked in the spec and supported for at least 6 months. Mobile apps have a forced-update mechanism for breaking changes.
- **REQ-API-017 Audit.** Every mutating call records actor, token/session, source and request id.

## 5. Resource map (1.0 starting list)

Group → representative operations. The OpenAPI file is authoritative.

| Group | Examples |
|---|---|
| Auth | `POST /auth/otp/request`, `POST /auth/otp/verify`, `POST /auth/login`, `POST /tokens` |
| Structure | `/branches`, `/shifts`, `/sessions`, `/terms`, `/classes`, `/sections`, `/subjects`, `/calendar` |
| People | `/students`, `/families`, `/guardians`, `/enquiries`, `/staff`, `/staff/{id}/assignments` |
| Admission | `POST /students/{id}/withdraw`, `POST /promotions`, `POST /certificates` |
| Attendance | `/attendance/registers`, `PUT /attendance/registers/{id}/records`, `POST /attendance/scan`, `/staff-attendance` |
| Leave | `/leave-types`, `/leave-requests`, `/leave-balances` |
| Fees | `/fee-types`, `/fee-plans`, `POST /challans/generate`, `POST /challans/release`, `/payments`, `POST /payments/{id}/void`, `/receipts`, `/security-deposits` |
| Accounting | `/expenses`, `/cash-sessions`, `/accounts`, `/journal-entries`, `/periods/{m}/close`, `/reports/{name}` |
| Payroll | `POST /salaries/generate`, `/salaries`, `POST /salaries/{id}/approve`, `POST /salaries/{id}/pay`, `/advances`, `/payslips` |
| Academics | `/exam-terms`, `/exams`, `/exams/{id}/marks`, `POST /exams/{id}/publish`, `/class-tests`, `/timetable`, `/diary`, `/resources` |
| Behaviour | `/behaviour/entries`, `/behaviour/categories` |
| Communication | `/announcements`, `/notifications`, `/chats`, `/forms`, `/notification-preferences` |
| Platform | `/subscription`, `/support/tickets`, `/imports`, `/exports`, `/audit-log` |
| Reports | `GET /reports/{name}?format=json|xlsx|pdf` |

## 6. Documentation as a product

- Published interactive reference generated from the OpenAPI file, versioned with releases.
- Each endpoint documents: purpose, permission flag, parameters, example request/response, error codes, idempotency, rate limits.
- A getting-started guide shows authentication and a first terminal workflow (create student, generate and release challans, record payment).

## 7. Decide in design

- OpenAPI authoring style (spec-first vs code-first generation) and code generator choices.
- Token format (opaque vs signed).
- Webhook event list and delivery guarantees [1.1].
