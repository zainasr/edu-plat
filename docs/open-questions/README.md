# Open questions before SRS / Spec 1.0

Purpose: close every remaining gap before we write the SRS and spec-driven development document (MVP 1.0). Then: design (architecture, stack) -> validation -> planning -> development -> testing.

## How to use
1. Open a section folder, read `questions.md`, compare with `answer.md`.
2. `answer.md` is pre-filled with **my suggested answer** under the same number (`1:`, `2:` ...). Each starts with a tag:
   - `[SUGGESTED]` my recommendation, not yet confirmed by you
   - `[CONFIRMED]` you agree (change the tag when you accept it)
   - `[CHANGED]` you replaced it with your own answer
   - `[LATER]` deliberately deferred (say to which phase)
3. Edit only what differs from your mind. Anything left `[SUGGESTED]` at the end will be treated as accepted.
4. Questions marked **(BLOCKER)** must be settled before SRS; others can be defaulted.

## Sections (suggested order)
| # | Folder | Focus |
|---|---|---|
| 01 | 01-product-scope-and-mvp | MVP boundary, release plan, success metrics |
| 02 | 02-users-roles-permissions | Roles, permission model, login and identity |
| 03 | 03-tenancy-and-org-structure | Institution, branches, shifts, sessions, terms |
| 04 | 04-admission-and-student-lifecycle | Admission, enrolment, promotion, transfer, withdrawal |
| 05 | 05-attendance | Marking methods, face/QR, offline, rules, alerts |
| 06 | 06-fees-and-payments | Fee rules, challans, discounts, collection, online payments |
| 07 | 07-accounting-and-finance | Ledger depth, reports, fiscal year, expenses |
| 08 | 08-staff-hr-payroll | Staff, leave, payroll rules |
| 09 | 09-academics-exams-timetable | Timetable, exams, results, report cards, diary |
| 10 | 10-communication-notifications | Channels, templates, rules, chats |
| 11 | 11-mobile-apps-and-ux | Student/parent/teacher apps, UX principles, offline |
| 12 | 12-analytics-ai-student-improvement | Dashboards, AI roadmap, data collection |
| 13 | 13-saas-plans-and-platform-admin | Plans, billing, trial, our own admin console |
| 14 | 14-integrations | SMS, WhatsApp, payments, face recognition, others |
| 15 | 15-non-functional-security-compliance | Performance, availability, security, privacy, legal |
| 16 | 16-onboarding-migration-support | Import, onboarding, support, training |
| 17 | 17-technical-preferences | Constraints and preferences going into the design phase |
| 18 | 18-delivery-process-team | Team, timeline, quality bar, environments, governance |
| 19 | 19-future-phases | Paper maker, colleges, later modules |

Reference material: `../reference-analysis/` (module notes, inferred data model, gaps).

## Reference-derived tags (added 2026-10-12)
- `[CONFIRMED-BY-REFERENCE]` the Edflo portal (Ahsan Public tenant) shows this behaviour in practice; each carries a "REFERENCE EVIDENCE" note. Treated as accepted unless you object.
- `[ADJUSTED-FROM-REFERENCE]` my earlier suggestion was changed to mimic the portal; the note says what changed and what we add on top.
- Answers you tagged yourself were not touched.

## Accepted 2026-10-12
Owner accepted all answers in sections 01-10 (suggestions and reference-derived ones alike). Items still with the owner's own wording keep it. Sections 11-19 remain [SUGGESTED] pending review.


## Decisions 2026-10-19
Sections 10-15: owner decided in-app notifications first with WhatsApp for important reminders only; AI scope and pricing decided as we build (`[ACCEPTED-DEFAULT]` = use this default in the SRS, refine in sprints); payments via common Pakistani rails (aggregator vs direct to confirm); self-hosted on DigitalOcean; team is AI agents plus 3-4 engineers. Sections 12-15 non-blockers tagged `[ACCEPTED-DEFAULT]`. Sections 16 and 18 and 19 still pending; 17 only Q1 and Q5 updated.


## Final MVP decisions 2026-10-19
Aggregator for payments; DigitalOcean India (Bangalore); advance payments follow the reference model; Go engineer available; budget, team and legal handled on the go; brand later; platform console not needed day one. Sections 16-19 set to `[ACCEPTED-DEFAULT]` (revisable). Requirements stage considered unblocked for MVP; next step is the SRS and spec documentation.
