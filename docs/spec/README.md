# Education Management Platform: Product Specification (MVP 1.0)

| | |
|---|---|
| Status | **Baseline v1.0, LOCKED 2026-10-28 (stakeholder-reviewed). Changes after this date go through the change rule below.** |
| Product | Multi-tenant education management SaaS (working codename: `TBD`, brand decided later) |
| Market | Private schools and academies in Pakistan first; colleges and universities later |
| Audience | Product owner, pilot-school stakeholders, engineers, AI coding agents |
| Source of truth | This folder. Where it conflicts with `docs/open-questions/`, this folder wins once signed off. |

## 1. How to read this specification

Read in this order:

1. `00-product/` explains what we are building, for whom, and the vocabulary.
2. `01-decisions/` records decisions already made (stack, hosting, principles).
3. `02-cross-cutting/` holds rules that apply to every module (roles, structure, data, API, NFRs).
4. `03-modules/` holds one specification per module, with user stories and acceptance criteria.
5. `04-release/` holds scope per release, the traceability index and the open items.

## 2. Folder map

```
spec/
  README.md                       this file
  00-product/
    01-vision-and-scope.md        problem, goals, success metrics, in/out of scope
    02-glossary.md                domain vocabulary (Pakistan school terms)
    03-personas-and-roles.md      who uses the system, multi-role model
  01-decisions/
    01-decision-log.md            numbered decisions (DEC-001...) with reasons
    02-technology-stack.md        stack, hosting, delivery, API-first principle
  02-cross-cutting/
    01-organisation-and-structure.md   tenant, institution, branch, shift, session, class
    02-identity-roles-permissions.md   login, multi-role union model, permission catalogue
    03-data-model.md              entities, relationships, conventions
    04-api-conventions.md         API-first rules, OpenAPI, auth, errors, idempotency
    05-notifications-catalogue.md events, channels, templates, preferences
    06-non-functional-requirements.md   availability, performance, security, privacy
  03-modules/
    01-students-and-families.md
    02-admission-and-lifecycle.md
    03-attendance.md
    04-fees-and-payments.md
    05-accounting-and-reports.md
    06-staff-leave-and-payroll.md
    07-academics-exams-and-diary.md
    08-behaviour.md
    09-communication.md
    10-mobile-apps.md
    11-analytics-and-student-improvement.md
    12-onboarding-and-import.md
    13-platform-and-plans.md
  04-release/
    01-release-plan.md            1.0 / 1.1 / 1.2 / 2.0 scope
    02-open-items.md              what is not decided yet, owners
    03-requirement-index.md       how IDs work, module codes
```

## 3. Conventions (for humans and agents)

**Document header.** Every file starts with a table: `Status`, `Release`, `Module code`.

**Identifiers.** All requirements are traceable:

| Prefix | Meaning | Example |
|---|---|---|
| `REQ-<MOD>-nnn` | Functional requirement | `REQ-FEE-014` |
| `BR-<MOD>-nnn` | Business rule (must always hold) | `BR-FEE-003` |
| `US-<MOD>-nnn` | User story with acceptance criteria | `US-STU-002` |
| `DEC-nnn` | Decision in the decision log | `DEC-007` |
| `OI-nnn` | Open item | `OI-004` |
| `EVT-<MOD>-name` | Domain event | `EVT-FEE-payment.recorded` |

**Release tags.** `[1.0]` MVP. `[1.1]`, `[1.2]`, `[2.0]` later. Untagged text applies to 1.0.

**Requirement keywords.** MUST (mandatory), SHOULD (expected unless a reason is recorded), MAY (optional).

**Acceptance criteria** use Given / When / Then so they can become automated tests.

**Provenance tags.** Where a rule was copied from the reference product's observed behaviour it is marked `(ref)`. Where we deliberately differ it is marked `(ours)`. This keeps "mimic the reference" and "improve on it" visible.

**Status words.** `Draft` (written, not reviewed), `Reviewed` (stakeholders commented), `Signed off` (frozen; changes by change request).

## 4. Reading the "Decide in design" notes

Items marked **Decide in design** are intentionally left open. They are listed in `04-release/02-open-items.md`. They are not gaps; they are choices we will make with better information.

## 5. Source material

- `docs/open-questions/`: 19 sections of questions and accepted answers (the decisions behind this spec).
- `docs/reference-analysis/`: notes from a hands-on study of the reference product (Edflo), used as a behavioural baseline.Given

## Change rule after lock
- Minor corrections and open items (`04-release/02-open-items.md`) are resolved during the phase that needs them, by editing the spec in the same pull request as the code.
- Each such change adds a line to `01-decisions/01-decision-log.md` (new DEC-nnn) and keeps IDs stable; a requirement is never renumbered.
- Scope additions or removals of a [1.0] item need the product owner's approval and a decision-log entry.
