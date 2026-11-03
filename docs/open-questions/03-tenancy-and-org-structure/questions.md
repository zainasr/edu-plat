# 03 Tenancy and organisation structure

1. (BLOCKER) Hierarchy: Organisation (owner/group) -> Institution -> Branch/Campus -> Shift -> Class -> Section. Which levels exist in 1.0? Can one owner run several institutions (a school + an academy) under one login?
2. (BLOCKER) What is a "tenant" for billing and data isolation: the organisation or the institution?
3. Branches: do they share students, staff, fee structure, accounting? Which data is branch-scoped vs shared (e.g. one chart of accounts, separate cash books)?
4. Shifts (morning/evening): is a shift a property of a section, a class, or a student enrolment? Do shifts have their own timetable, fees, staff, attendance rules?
5. Academic session: start/end dates, can sessions overlap, can it be edited after start, what is the rollover process (create next session from the previous one)?
6. Terms: 3 or 6 terms per institution. Are terms tied to the session, with date ranges, and optional per class? Do terms drive anything besides exams (fee collection, reports)?
7. Class naming/levels: provide a system-wide template list (Playgroup..Class 10, 1st/2nd Year, O/A levels, ADP, BS). Selectable per institution? Custom classes allowed?
8. Boards/curricula (Punjab Board, Federal, Cambridge, Aga Khan, Montessori): tracked as data (affects result formats, subjects) or ignored in 1.0?
9. Institution types: school, academy/coaching, college, university. Do academies use "batch/course/subject-fee" instead of class/section? How much do we model for them in 1.0?
10. Working calendar: weekly off days, holidays, special working days, Ramadan/summer timing. Per institution or per branch/shift?
11. Institution profile fields needed (registration number, NTN, logo, signature/stamp, bank accounts, letterhead for documents).
12. Tenant lifecycle: create, trial, active, suspended (non-payment), archived, deleted. What happens to data in each state and for how long?
13. Student ID/reg number rules: per institution or per branch? Prefix/year/sequence format? Allow legacy numbers in import?
14. Is a student allowed to be enrolled in two branches/shifts at once (e.g. morning school + evening tuition)?
