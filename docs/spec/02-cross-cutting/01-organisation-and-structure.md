# 01. Organisation and Structure

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | ORG |

## 1. Hierarchy

```
Organisation (tenant: subscription, billing, owner, data isolation)
 └─ Institution (school / academy / college)
     └─ Branch (campus)
         ├─ Shift (Morning, Evening)
         ├─ Session (academic year) ── Term (3 or 6)
         └─ Class ── Section ── (Subject assignments)
```

The data model supports every level from day one. The 1.0 UI shows one organisation with one institution and multiple branches and shifts; an owner with two institutions gets one login with a simple institution switcher.

## 2. Requirements

- **REQ-ORG-001** The system MUST isolate tenant data; every business row carries `tenant_id`, enforced by row-level security.
- **REQ-ORG-002** Institution profile MUST hold: name, short name, optional registration no and NTN, logo, principal signature image, optional stamp, address, phones, WhatsApp, email, optional website, bank accounts (shown on challans), board/curriculum label, institution type, established year.
- **REQ-ORG-003** Branch-scoped data: classes, sections, timetables, attendance, fee collection and cash books, expenses, staff assignment. Shared per institution: students, staff directory, fee types, chart of accounts.
- **REQ-ORG-004** Reports MUST be viewable per branch or consolidated.
- **REQ-ORG-005** A **shift** is a property of a section. "Class 5-A Morning" and "Class 5-A Evening" are different sections. A shift carries start/end, period length, attendance rules and timetable. Fees default from the class and can be overridden per shift.
- **REQ-ORG-006** A **session** has start and end dates; one is current per branch. Dates are editable until the first attendance is marked; afterwards the end date may only be extended. Overlap is allowed only during rollover.
- **REQ-ORG-007** A **term** belongs to a session (3 or 6) with name, dates and weightage. Terms group exams and report cards; they are not tied to fees in 1.0.
- **REQ-ORG-008** Provide class templates by level (early years, primary, middle, secondary, higher secondary, O/A levels, college ADP/BS). Custom classes are always allowed. A numeric grade is kept for ordering and promotion mapping.
- **REQ-ORG-009** Board/curriculum (Punjab Board, Federal, Cambridge, other) is a label on class/institution; no board-specific logic in 1.0.
- **REQ-ORG-010** Academies use the same class/section structure, labelled Batch/Course per institution type, with per-student fee override. Subject-wise fees and multi-batch enrolment are [1.1].
- **REQ-ORG-011** A **calendar** is kept at institution level with branch overrides: weekly off days, holidays/events (date range), special working days, optional timing overrides (for example Ramadan). Holidays suppress attendance and fee-due reminders.
- **REQ-ORG-012** Registration numbers are configurable per institution: prefix + year + sequence (branch code optional). Imports may bring existing numbers, which MUST be unique within the institution.
- **REQ-ORG-013** A student may have two enrolments (different sections, shifts or branches) under one student record; fees and attendance are per enrolment. Hidden behind an advanced toggle.
- **REQ-ORG-014** Tenant lifecycle: Trial, Active, Grace (7 days overdue, full access with banner), Suspended (read-only; parents may still view and pay), Archived (30 days, export offered), Deleted (after 90 days on request, legal-hold exceptions). Data export is always available to the owner. *(Not a priority for the first release; implement states and enforcement later.)*

## 3. User stories

### US-ORG-001 Set up the institution
As an **Owner** I want to enter my school's profile and add branches, shifts, a session, classes and sections so that the rest of the system can be configured.

- Given I have just signed up, when I complete the profile and add Class 5 with sections A and B, then both sections appear in the class list.
- Given a session has attendance marked, when I try to change its start date, then the system refuses and offers only extending the end date.

### US-ORG-002 Run two shifts
As an **Admin** I want Class 5-A Morning and Class 5-A Evening as separate sections so that attendance and timetable do not mix.

- Given a branch with Morning and Evening shifts, when I create section 5-A in each, then each has its own timetable, attendance register and (optionally) fee override.

### US-ORG-003 Roll over to a new session
As an **Admin** I want a rollover wizard that copies classes, sections, subjects and fee structure, then promotes students, so that a new year starts quickly.

- Given session 2026-27 is current, when I run rollover to 2027-28, then classes, sections, subjects and fee plans are copied and students are promoted per the promotion wizard (see admission module).

### US-ORG-004 Mark holidays
As an **Admin** I want to add holidays and special working days so that attendance and fee reminders respect them.

- Given a holiday on 14 August, when attendance for that day is requested, then the register shows Holiday and no absent alerts are sent.

## 4. Business rules

- **BR-ORG-001** A branch has exactly one current session at a time (except during rollover).
- **BR-ORG-002** Registration numbers are unique within an institution.
- **BR-ORG-003** Deleting a class or section that has enrolments is not allowed; it can only be archived.

## 5. Decide in design

- Exact tenant-isolation mechanism for large groups (dedicated database or cell).
- Class template contents per level and board.
