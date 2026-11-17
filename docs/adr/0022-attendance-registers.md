# ADR-0022: Attendance registers and the school calendar

| | |
|---|---|
| Status | Accepted (choices made by the product owner on 2026-11-17) |
| Date | 2026-11-17 |
| Spec links | REQ-ATT-001, REQ-ATT-003, REQ-ATT-010, REQ-ATT-015, REQ-ATT-016, BR-ATT-001, BR-ATT-002, BR-ATT-005, US-ATT-005 to 008, REQ-ORG-011, DEC-030 to DEC-032 |

## Context
Attendance must be fast to enter (a few taps for the exceptions), trustworthy (nothing silently overwritten), and honest about what is missing (a register nobody submitted is not "everyone absent"). It also needs to know which days are school days.

## Decision
1. **The calendar belongs to org.** `calendar_settings` holds the weekly off days (default Sunday); `calendar_days` holds holidays and special working days, one active entry per date, ranges up to 60 dates. A listed holiday is never a school day; a listed working day always is; otherwise a weekly off day is not. Entries are removed (kept as history), never edited or deleted. Other modules ask `org.SchoolDay` inside their own transaction (ADR-0013).
2. **One register per section per day** (`attendance_registers`, unique on section and date), `open` then `submitted`, once. Opening pre-fills everyone enrolled that day as present (the roster comes from students: an enrolment began on or before the date and had not ended by it), so only the exceptions are marked. A register cannot be opened for a holiday, a weekly off day, or the future, nor for a date older than the correction window unless the caller has `attendance.edit_any`.
3. **Records are versions.** `attendance_records` is append-only: the first row is the pre-fill, every mark and every correction adds the next version with who, when and (for a correction) why. A student's current status is the latest version. Marking an unchanged status adds nothing.
4. **The lifecycle is enforced by the database**, not only the service: versions count 1, 2, 3 with no gaps; marks only while the register is open; corrections only after it is submitted and only with a reason; a register changes only from open to submitted and never back; nothing is deleted.
5. **Corrections** need `attendance.edit_within_window` (within the school's window, default 7 days) or `attendance.edit_any` (any age). Endpoints that either permission opens use `iam.RequireAny`; the service decides how far back the caller may go.
6. **Only submitted registers count** in the daily summary, the absent list, the low-attendance list and a student's history; an open register is listed but never counted (BR-ATT-005). Attendance is (present + late) / (present + late + absent); leave is excused and shown separately (DEC-031).
7. **Missing registers** are derived: on a school day, once the due time (default 10:00, Pakistan) has passed, every active section with no submitted register is listed, flagged when a register was opened but not submitted.
8. **Roles.** New `attendance_clerk` role (view, mark, correct within the window, read structure). The Receptionist gains view, mark and `org.view`; the Accountant gains view; the Owner gains everything (DEC-032). Teachers come with the staff module's section assignments.
9. **Required query parameters are checked after the permission.** A required query parameter in the OpenAPI file makes the generated layer answer 400 before the handler runs, so an anonymous caller would not get 401. Report dates are therefore optional in the contract and refused by the service with `attendance.date_invalid`; the invariant test that every operation answers 401 and 403 first caught this.

## Found in review
- **A same-day section move counted a student twice** (present in the old register, present in the new one). Now a register is reconciled with the roster when it is submitted: students who joined that day and were never marked are recorded present, students who moved or withdrew are recorded as `left` (counted nowhere), and a student already counted in another section's submitted register that day is left out of a new register. `ended_on` on an enrolment is the first day NOT enrolled, so a move or withdrawal on a day puts the student in the new section (or out) for that whole day. The roster no longer filters on the student's current status, so a student who withdrew yesterday still belongs to the days before.
- **The correction window could be bypassed** by marking or submitting an old register that was left open. Marking and submitting now need `attendance.edit_any` for a register older than the window, and are refused on a day the calendar has since made a holiday; what was already submitted keeps its counts and the summary shows `school_day: false`.
- **Guardian mobiles on the absent list** need the new `attendance.contacts` permission (Owner, Receptionist, Attendance Clerk). The Accountant had been given `attendance.view` without a reason in the spec; it is removed. The branch that omits contacts has no test yet, because no role holds `attendance.view` without `attendance.contacts`.
- **Low attendance compares the exact ratio**, not the rounded percentage: 17 of 21 days is 80.95%, shown as 81.0 and still below an 81% threshold. A register's count field was renamed `students` (it includes leave), distinct from a student's `expected_days` (present + late + absent).
- **Reads no longer write.** The calendar and attendance settings are created when first saved; until then the defaults apply.
- The lifecycle trigger reads the register with `FOR SHARE`, so a mark cannot land in a register submitted a moment earlier by a concurrent transaction (the service's own row lock already covers the normal path). This is not covered by a deterministic test.
- Known and accepted: a student who is moved after their first register of the day was submitted counts only there; a section with no students is still listed as missing; "Class 10" sorts before "Class 2" in a summary.

## Not in this slice
The gate scan and student cards (6b), absent alerts to parents (needs notifications), staff attendance and its geo-fence, offline sync of marks (client-generated ids), the gate-vs-class mismatch list, timing overrides on the calendar, and the printable monthly class register.

## Consequences
- A school can run daily attendance through the API today with the desk and receptionist; teacher self-service waits for assignments.
- The roster of a past day depends on enrolment dates, so a student moved between sections appears in the section they were in that day.
- Reports compute over current-version rows with `DISTINCT ON`; indexed by tenant, register and student. Fine for a school's volume.
- The clerk's right to correct within the window is an assumption to confirm (DEC-032).
