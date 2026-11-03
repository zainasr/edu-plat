# 02. Open Items

| | |
|---|---|
| Status | Living list |
| Module code | OI |

Open items are not gaps; they are decisions deliberately postponed. Each has an owner and a trigger.

| ID | Item | Owner | Decide by |
|---|---|---|---|
| OI-001 | Product name and brand identity | Owner / designer | Before pilot launch |
| OI-002 | Payment aggregator selection (fees, settlement time, KYC, sandbox, webhooks) | Owner + engineering | Design phase |
| OI-003 | WhatsApp BSP selection and template approval; check Meta Pakistan pricing | Owner | Design phase |
| OI-004 | SMS provider for OTP fallback | Engineering | Design phase |
| OI-005 | Pricing numbers and plan boundaries | Owner | After pilot feedback |
| OI-006 | Real dates: pilot start, public launch | Owner | Before planning gate |
| OI-007 | Legal entity, SECP, NTN, bank account, trademark, domains, app-store accounts | Owner | On the go; before public launch |
| OI-008 | Legal documents (ToS, Privacy, DPA, AUP, refund) reviewed by a Pakistani lawyer | Owner | Before pilot |
| OI-009 | Accountant review: tax treatment, trust/non-profit schools, audit formats | Owner | Design phase |
| OI-010 | Offline sync approach and library | Engineering | Design phase |
| OI-011 | Face recognition vendor vs self-hosted [1.1] | Engineering | Before 1.1 |
| OI-012 | Observability, IaC and CI/CD tooling products | Engineering | Design phase |
| OI-013 | Self-run HA Postgres vs managed Postgres on DigitalOcean | Engineering | Design phase |
| OI-014 | Final permission flag list and role matrices | Product + engineering | Design phase |
| OI-015 | Mobile app screens for parent/teacher/student (not observable in the reference) | Product / designer | Validation phase |
| OI-016 | Pilot schools (2 to 3) and stakeholders for SRS review | Owner | Before SRS sign-off |
| OI-017 | Budget and runway; team growth plan | Owner | On the go |
| OI-018 | Data residency: confirm no Pakistani rule blocks hosting in India | Owner + lawyer | Before pilot |
| OI-019 | Enrolment wizard step 3 (parents), Pay Salary and Salary History screens not observed in the reference; specified from our answers | Product | Validation phase |
| OI-020 | Whether the school-facing admin portal needs any day-one limits (the platform console is deferred) | Owner | Confirm now |
| OI-021 | Assumption: every school has a computer with a webcam at reception (confirmed by the owner). Confirm webcam quality guidance (position, lighting) with pilot schools. Staff-room/class computers for the absentees-only register are optional (the attendance desk or a teacher's phone can be used instead) | Owner + pilot schools | Validation phase |
| OI-022 | Diary send time and shift end rule per school; whether WhatsApp picture delivery is offered at launch | Owner | Design phase |

## Known reference weaknesses we deliberately do not copy
Default shared parent password; deposit schedule vs ledger mismatch; payment method "not stated"; no online payments set up; no payslip or bank file; CSV-only import; no student login; no offline support; no SMS/WhatsApp; slow pages.
