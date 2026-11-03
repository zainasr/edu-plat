# 13 Leaves
Routes: /admin/staff/leaves/{requests,history,balances,types}, /admin/students/leaves. Screenshots: screenshots/13-leaves/.

## Staff leave
- **Leave types** (policy): "annual/monthly, paid/unpaid"; Add Leave Type; empty on this trial account (nothing preconfigured).
- **Balances**: pick staff member + year; allocated, used, remaining per type; "Initialize all staff" button creates yearly balances for everyone.
- **Requests**: pending approval list with search (by name or leave type), approve/reject; "View history" page for past decisions. Requests are raised by staff in the teacher app.
## Student leave
- Parents file absence requests in the parent app; admin tabs Pending / Approved / Rejected, search by student, parent or reason. **Approving auto-marks the student "on leave" in attendance for those dates.** 

## Entities
LeaveType(name, cadence annual/monthly, quota, is_paid), LeaveBalance(staff_id, type, year, allocated, used), StaffLeaveRequest(staff, type, from, to, reason, status, decided_by), StudentLeaveRequest(student, parent, from, to, reason, status).

## Gaps/ideas
Leave-to-payroll link (unpaid leave deduction) is implied but unverified; no substitute-teacher assignment on staff leave; no medical-document attachment. For students: notify parent on decision, auto-excuse in attendance analytics so AI does not flag authorised absences.

## Hands-on addendum (2026-10-12)
Leave menu: Staff Requests, Student Requests, Leave History (filter All/Approved/Rejected; empty here), Leave Balances, Leave Types.
