# 01. Release Plan

| | |
|---|---|
| Status | Draft v0.1 (dates are placeholders to be set) |
| Module code | REL |

## 1. Releases

| Release | Theme | Contents |
|---|---|---|
| **1.0** | Core admin, money, attendance, parents | Structure, students/families, admission and lifecycle, attendance (manual + QR), fees and challans, counter payments, online payments via one aggregator, accounting engine and core reports, staff + leave + payroll, exams (simplified), class tests, diary, announcements, chat, forms/enquiries, parent app, teacher app, light student app, audit log, Excel import, trial/plan basics, rule-based hints |
| **1.1** | Academics and refinement | Full exams (datesheet, slips), syllabus topic tree, period-wise attendance, face recognition pilot, quarterly/term fees, bank statement auto-match, student app complete, remaining reports, outgoing webhooks, per-lecture pay, PTM feedback |
| **1.2** | Intelligence | First ML models (fee default risk, early warning), peer benchmarks, budgets, fixed assets, OCR, gamification |
| **2.0** | Paper/test maker | Question bank, paper builder, exam prep, auto-marking, timetable generator |
| **3.0** | Colleges and others | Semester/credit hours/CGPA, transport, library, hostel, hardware partnerships |

## 2. Delivery process
- Phase gates: requirements (SRS signed), design (architecture + threat model), validation (clickable prototypes of key flows with 5 to 8 schools), planning, build (2-week sprints), test (UAT with pilot schools), release (go/no-go).
- Critical flows to prototype: signup/setup, enrol, mark attendance, generate and release challans, collect fee, parent pays, results publish, owner dashboard, import, notifications.
- Definition of ready: story linked to spec, acceptance criteria, UX ready, dependencies clear. Definition of done: reviewed, tests passed (unit + integration), docs/spec updated, security and performance checks for sensitive stories, demoed, deployed to staging.
- Release gates: no open S1/S2 bugs, smoke end-to-end pass, performance targets met, backup restore tested. Severity: S1 (data loss/security/money) immediately, S2 within 3 days.
- Cadence: weekly web deploys (feature flags), bi-weekly mobile releases, staged rollouts.

## 3. Key dates (to set)
- SRS sign-off: TBD. Pilot start: TBD. Public 1.0: aligned to the next admission season (Mar-Apr for Punjab private schools), working backwards. 1.1: about 3 months after launch. 1.2: about 3 months after 1.1.

## 4. Top risks

| Risk | Mitigation |
|---|---|
| Payment aggregator onboarding delay | Start early; counter payments work without it |
| WhatsApp/BSP approvals | Start early; in-app notifications as fallback |
| Offline sync complexity | Narrow scope; idempotent design; heavy tests |
| Data quality at import | Auto-clean, preview, undo |
| School adoption / parent activation | Onboarding specialist, invite tools, activation levers |
| Legal and data protection | Lawyer review before pilot |
| Team capacity | Small modular scope; AI agents with human review for money code |
| Funding runway | Phased scope; pilots before heavy ML spend |
