# 02. Identity, Roles and Permissions

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | IAM |

## 1. The core model

```
User (one identity: mobile number)
 └─ Membership (user in one institution/organisation)
     └─ RoleAssignment[]  (role + scope)   ← MANY per membership
          └─ Role = set of permission flags
Effective permissions = UNION of all RoleAssignments (each with its own scope)
```

- **REQ-IAM-001** One user identity (mobile, optionally email) can have memberships in several institutions/organisations. A context switcher exists only between institutions.
- **REQ-IAM-002** Within one membership a user MAY hold **many role assignments simultaneously**. Effective permissions are the **union**. (DEC-006)
- **REQ-IAM-003** Each role assignment has a **scope**: whole institution, specific branch(es), assigned classes/sections, or own records only. For a given permission the effective scope is the union of scopes from all assignments that grant it.
- **REQ-IAM-004** Class teacher and subject teacher are **assignments** (relations to sections/subjects), not roles. A user with the Teacher role sees only what their assignments scope.
- **REQ-IAM-005** Built-in roles ship with sensible defaults. The Owner can clone a role and edit permissions to create custom roles. Roles are additive building blocks.
- **REQ-IAM-006** Role assignments are time-stamped, audited, and revocable. Revoking the last role of a staff member deactivates access immediately; history is kept.
- **REQ-IAM-007** A person holding both "maker" and "checker" permissions MUST NOT approve their own action (see BR-IAM-002).

### Worked example (union)

| Assignment | Role | Scope | Grants (sample) |
|---|---|---|---|
| 1 | Receptionist | Branch: Main | `fee.collect`, `enquiry.manage`, `student.view` |
| 2 | Accountant | Institution | `expense.create`, `report.finance.view`, `payment.void` (needs approver) |
| 3 | Teacher | Sections: 3-A, 3-B | `attendance.mark`, `marks.enter`, `diary.post` |

Result for this person: fee collection for the branch, finance reports for the institution, attendance/marks/diary for 3-A and 3-B only. One navigation menu shows all of it.

## 2. Authentication

| Actor | Method |
|---|---|
| Staff / admin / owner | Mobile (preferred) or email + password; OTP second factor mandatory for Owner, Admin and Accountant roles |
| Parent | Mobile number + OTP (no password) |
| Student | School-issued ID + PIN set by the parent, approved on first use; under 10 years the student view is inside the parent app; student account optional per school |
| API / terminal | Personal access token (scoped, revocable, audited) issued to a user; same permissions as the user |

- **REQ-IAM-010** Passwords: minimum 6 characters for staff; lockout after 5 failed attempts; OTP rate-limited per number/device/IP.
- **REQ-IAM-011** Sessions: mobile 30 days with refresh; web 12 hours idle; staff max 3 active devices; new-device alert by WhatsApp (SMS only as fallback).
- **REQ-IAM-012** Account recovery: OTP to the registered number plus admin-assisted verification.
- **REQ-IAM-013** Parent/student accounts are created automatically at enrolment from the guardian mobile; invite sent by WhatsApp with app link. Staff accounts are created by an Admin; temporary password delivered securely (SMS fallback). The reference's default shared password is explicitly **not** copied (ours).
- **REQ-IAM-014** One family account (login = primary guardian mobile; the primary guardian may be any relative, not necessarily the father) with up to 4 linked guardians, each with own mobile + OTP login. Changing the primary number needs OTP to old and new number or admin verification.
- **REQ-IAM-015** API tokens: created by the user, have a name, scope subset, expiry, last-used; revocable; every call audited with token id.

## 3. Permission model

Permissions are flags named `module.action`, grouped into roles (about 60 to 80 flags). The catalogue below is the 1.0 starting set; exact list is finalised in design.

| Module | Flags |
|---|---|
| org | `org.view`, `org.manage`, `branch.manage`, `session.manage`, `calendar.manage` |
| iam | `user.view`, `user.manage`, `role.manage`, `token.manage`, `audit.view` |
| student | `student.view`, `student.create`, `student.edit`, `student.withdraw`, `student.delete_request`, `student.import`, `student.export`, `family.manage`, `student.merge` |
| admission | `enquiry.view`, `enquiry.manage`, `quotation.create`, `quotation.send`, `quotation.discount.approve`, `admission.create`, `promotion.run`, `certificate.issue` |
| attendance | `attendance.view`, `attendance.mark`, `attendance.edit_within_window`, `attendance.edit_any`, `attendance.settings`, `staff_attendance.view`, `staff_attendance.manage` |
| fee | `fee.view`, `fee.plan.manage`, `fee.generate`, `fee.release`, `fee.collect`, `fee.discount.give`, `fee.discount.approve`, `fee.latefee.waive`, `payment.void`, `payment.void.approve`, `fee.refund`, `fee.settings`, `fee.export` |
| accounting | `report.owner.view`, `report.finance.view`, `ledger.view`, `journal.manual`, `period.close`, `expense.create`, `expense.approve`, `cash.session.manage`, `account.manage` |
| staff | `staff.view`, `staff.manage`, `staff.assign`, `leave.request`, `leave.approve`, `payroll.generate`, `payroll.approve`, `payroll.pay`, `payslip.view_own`, `payslip.view_all` |
| academics | `curriculum.manage`, `timetable.manage`, `exam.manage`, `marks.enter`, `marks.review`, `result.publish`, `reportcard.generate`, `classtest.manage`, `diary.post`, `resource.manage` |
| behaviour | `behaviour.log`, `behaviour.review`, `behaviour.share`, `behaviour.settings` |
| comm | `announce.send`, `announce.emergency`, `chat.use`, `chat.monitor`, `chat.settings`, `notification.settings` |
| analytics | `insight.view_own_scope`, `insight.view_all` |
| platform | `subscription.view`, `subscription.manage`, `support.ticket` |

### Built-in role defaults (summary)

| Role | Key permissions |
|---|---| 
| Owner | All; plus `role.manage`, `subscription.manage`, create/remove Admins |
| Admin | All except removing an Owner or changing billing ownership |
| Accountant | fee.* (except settings), accounting.*, payroll.generate/approve/pay, staff.view, plus own staff features (attendance check-in, leave, payslip) |
| Receptionist | `student.view`, `enquiry.*`, `admission.create`, `fee.collect`, `fee.view`, `cash.session.manage` |
| Teacher | `attendance.mark` (own sections), `marks.enter`, `diary.post`, `classtest.manage`, `behaviour.log`, `chat.use`, own leave/payslip |
| Parent | Read own family's records; `leave.request` (student), pay online, chat |
| Student | Read own timetable, diary, results, progress |

## 4. Maker-checker (configurable)

Optional approval flows, with thresholds set per institution. Default actions covered: void payment, delete/withdraw student, discounts above a limit, salary approval, fee structure change after challans are released, period close.

- **BR-IAM-002** The approver MUST be a different person from the maker. A person holding both permissions can still do the first half, but not approve their own request.
- **BR-IAM-003** Maker-checker is **optional** (owner choice, "can be but not should be"): with it off, the action is allowed directly but still audited.
- If an institution has only one person with the approving permission, the system warns at configuration time.

## 5. Audit

- **REQ-IAM-020** Log all creates, updates, deletes, approvals, exports, role changes, token use and sign-ins: actor, roles active, module, action, entity, before/after summary, time, source (web/app/API), reason where required.
- **REQ-IAM-021** The log is append-only. Retention: 2 years online, 7 years archived for finance-related entries.
- **REQ-IAM-022** Platform support "view as" requires a tenant-consent token, is time-boxed, read-only by default, and fully audited. No write impersonation in 1.0.
- **REQ-IAM-023** Never hard-delete users or records; deactivate. Staff leaving: access revoked immediately, history kept. Withdrawn students: guardians keep read access to history and dues until cleared, then archived.

## 6. User stories

### US-IAM-001 Give one person several roles
As an **Owner** I want to assign Receptionist, Accountant and Teacher to the same person so that they do all three jobs with one login.

- Given Ayesha has Receptionist (branch), Accountant (institution) and Teacher (3-A, 3-B), when she signs in, then she sees fee collection, finance reports and attendance for 3-A and 3-B in one menu.
- Given she lacks `payment.void.approve`, when she voids a payment, then it goes to a different approver.

### US-IAM-002 Create a custom role
As an **Owner** I want to clone "Accountant", remove payroll permissions and name it "Fee Clerk".

- Given the clone exists, when I save, then users assigned "Fee Clerk" cannot open payroll.

### US-IAM-003 Parent logs in with OTP
As a **Parent** I want to enter my mobile number and an OTP so that I never remember a password.

- Given my number is the primary guardian number, when I request an OTP and enter it within 5 minutes, then I see all my children.
- Given 5 wrong OTP attempts, then the number is locked for a cool-down.

### US-IAM-004 Use the API from a terminal
As an **Admin** I want to create a personal access token and call the API so that I can script bulk work.

- Given a token with `student.create`, when I POST a student with `curl`, then the student is created and the action appears in the audit log with the token's name.

### US-IAM-005 Staff leaves
As an **Admin** I want to deactivate a staff member so that access ends instantly but history stays.

- Given a deactivated user with active sessions, then all sessions and tokens are revoked within 1 minute and historical records remain visible.

## 7. Decide in design

- Authorisation engine implementation (attribute/role evaluation in code vs a policy library).
- Exact permission flag list and default role matrices (final matrix is a design deliverable).
- Token format and rotation policy.
