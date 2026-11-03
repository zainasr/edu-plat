# Module 12. Onboarding, Import and Migration

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | ONB |

## 1. Goals
Signup to first attendance within 1 day; first fee collected within 3 days; all students and parents loaded within 7 days. Our onboarding specialist (or agent) assists the first 100 schools; later self-serve.

## 2. Requirements

### Set-up wizard (order)
Profile → academic session → branches/shifts → classes/sections → fee plans → subjects → staff → students import → invite parents → go-live checklist (roles, notification channels, receipt format, bank accounts).
- **REQ-ONB-001** A setup checklist and "Readiness score" widget (profile %, students loaded, fee plans set, parents active %, staff accounts) appear on the owner dashboard and to our success team.
- **REQ-ONB-002** A seeded, resettable **demo tenant** with fake data for sales, training and trials.

### Excel import
- **REQ-ONB-010** Import entities: classes/sections/subjects, students + families (+ opening dues), staff, fee plans/overrides. Formats `.xlsx` and CSV. The student sheet carries father's name and, per guardian, name, relationship and mobile (a missing relationship defaults to father/mother by column, and rows with a non-father guardian are supported).
- **REQ-ONB-011** Flow (mirrors the reference's 4 steps): **Upload → Configure** (map columns, auto-detect common headers, defaults) **→ Review** (row-level errors, per-row verification, set registration numbers) **→ Done** (summary). Up to 5,000 rows per batch (ref); commit as one batch.
- **REQ-ONB-012** Auto-clean: trim, mobile normalisation to `03XXXXXXXXX`, name casing, duplicate warnings, class-name mapping suggestions. An error report is downloadable.
- **REQ-ONB-013** **Undo import** within 72 hours (batch rollback while records are untouched).
- **REQ-ONB-014** A "Switch to us" importer for common competitor exports (including the reference product) with manual mapping.
- **REQ-ONB-015** Historical attendance/results import is deferred.

### Opening balances
Opening dues per student/family, cash and bank balances, staff advances, posted as dated opening entries at go-live and locked after owner confirmation (see Accounting).

### Parent activation
- **REQ-ONB-020** Automatic WhatsApp invite with app link; printable class-wise QR poster/sheet for parent meetings; first-login OTP; a "parents not yet active" list per class with a one-tap re-invite; goal-based banner. Target 70% active within 30 days.

### Exit and export
- **REQ-ONB-030** The Owner can download a complete export (Excel per entity, PDF reports, documents zip) at any time. Exit survey on cancellation; certificates remain re-issuable during retention.

### Learning and support
Short role-based Urdu-narrated videos (UI stays English), tooltips, PDF cheat sheets, weekly webinar. Support tiers L1/L2/L3.

## 3. User stories

### US-ONB-001 Import 500 students
As an **Admin** I want to upload our Excel and fix errors before saving.

- Given an Excel with mixed phmixed phone formatsone formats, when I upload and map columns, then the review step shows normalised numbers and 12 rows with errors I can fix or skip.
- Given I commit, then 488 students and their families exist; a batch id is recorded.
- Given a mistake, when I choose Undo within 72 hours, then the batch is removed provided nothing downstream was posted against those records, or the system tells me what blocks it.

### US-ONB-002 Invite parents
- Given students are loaded, when I press Invite, then each primary guardian receives a WhatsApp message with the app link and logs in with OTP.

### US-ONB-003 Go-live checklist
- Given I complete roles, channels, receipt format and bank accounts, then the checklist turns green and the readiness score rises.

## 4. Business rules
- **BR-ONB-001** Imports never overwrite existing students silently; matches are shown for decision.
- **BR-ONB-002** Registration numbers from imports must be unique.
- **BR-ONB-003** Undo is allowed only while no payments or attendance exist for the imported records.
