# 01. Vision and Scope

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 and roadmap |
| Module code | PRD |

## 1. Problem

Private schools and academies in Pakistan run on paper registers, Excel sheets and WhatsApp groups. The results:

1. Fee recovery is slow and defaulters are hard to see.
2. Attendance is on paper; parents learn about absence late or never.
3. Owners have no clear numbers (money, attendance, staff cost).
4. Staff repeat the same work in registers, Excel and messages.
5. Parents are poorly informed about how their child is doing.

## 2. Product vision

A simple, reliable, mobile-first system that lets an owner **see the whole school's money and attendance at a glance and get paid on time**, lets a teacher **mark attendance and marks in under a minute**, lets a parent **know how their child is doing today and pay fees from a phone**, and lets a student **see progress and what to practise next**.

Differentiator (over time): real AI/ML on a platform's own data, starting with transparent rule-base d insights and growing into models.

## 3. Principles

1. **Simplicity first.** One primary action per page; "what needs my action today" leads every role.
2. **Mature and secure technology.** Boring, proven components; no building of security primitives ourselves.
3. **Performance and availability.** Works on low-end Android phones and weak networks; staff attendance and marks work offline.
4. **API-first.** Everything the UI can do is an API endpoint (see DEC-004).
5. **Draft, review, release** for money. Automation proposes; a person confirms.
6. **Never destroy history.** Deactivate, version and reverse; do not hard-delete business records.
7. **Close to the reference where it works** (proven workflows); different where the reference is weak (technology, hosting, mobile, messaging, security).

## 4. Target customers

- **Primary:** private schools with 150 to 1,500 students (ideal 200 to 800).
- **Same release where the structure fits:** academies (batch/course maps to class + section).
- **Later:** colleges and universities (semesters, credit hours, CGPA).

## 5. Success metrics (first 6 to 12 months)

| Metric | Target |
|---|---|
| School set-up time | 3 days or less |
| Parents active monthly | 70% |
| On-time fee payment | +15% within 3 months |
| Trial to paid | 25% |
| 90-day retention | 85% |
| Support load | fewer than 1 ticket per school per week after onboarding |

## 6. Scope of MVP 1.0

### 6.1 In scope

| Area | Included in 1.0 |
|---|---|
| Structure | Organisation, institution, branches, shifts, sessions, terms, classes, sections, subjects |
| People | Students, families/guardians, staff, enquiries and admission, withdrawal, promotion |
| Attendance | Student and staff attendance, QR scan, leave integration, parent alerts |
| Fees | Fee plans, challans (draft-review-release), counter collection, family billing, discounts, late fees, receipts, defaulters, security deposits, online payment through one aggregator |
| Accounting | Hidden double-entry engine, plain-language reports, expenses, cash sessions, period close |
| Staff | Staff records, assignments, leave, monthly payroll with payslips |
| Academics | Terms, exams, marks entry, results, report cards, class tests, simple timetable, diary, resources |
| Communication | In-app notifications, WhatsApp for important reminders, announcements, teacher-parent chat, enquiries inbox |
| Apps | Parent app, teacher/staff app, light student app (React Native); everyone else on web |
| Insight | Owner dashboard, rule-based student hints, weekly parent digest |
| Platform | Trial, plans, support tickets, audit log, bulk Excel import |

### 6.2 Deferred (not in 1.0)

Paper/test maker, full accounting UI (balance sheet and advanced views are 1.1), fixed assets UI, budgets, transport, library, hostel, inventory, colleges/universities, ML beyond rules, Urdu UI, gate check-out/pickup, platform admin console (API/tooling only at first), hardware integrations, public API for third parties.

### 6.3 Explicitly out of scope

Live classes, recorded courses, an LMS, proctored online exams, e-commerce.

## 7. Assumptions and constraints

- Single main language of delivery: English UI. Urdu text must be typeable and displayable in content fields.
- Hosting is self-managed on DigitalOcean, India (Bangalore) region.
- Team: AI agents plus 3 to 4 engineers; budget and legal formation are handled as the product grows.
- Payments go through one Pakistani aggregator; the school receives funds directly.
