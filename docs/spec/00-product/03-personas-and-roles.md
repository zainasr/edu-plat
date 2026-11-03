# 03. Personas and Roles

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | IAM (see also 02-cross-cutting/02-identity-roles-permissions.md) |

## 1. The real-world rule: one person, many jobs

In Pakistani institutions, job titles are blurry. One person is often the receptionist, the accountant and a teacher at the same time. Therefore:

- **Roles are building blocks, not job titles.**
- **A person can hold many roles at once in the same institution.**
- **Effective permissions are the union of all of the person's roles** (see DEC-006).
- No role switching is needed inside one institution.

## 2. Roles shipped with the product

| Role | Typical person | Main purpose | Primary client |
|---|---|---|---|
| **Owner** | School owner / principal | Everything including billing; can create or remove Admins | Web, phone |
| **Admin** | Vice principal / office manager | Everything in the portal except removing an Owner | Web |
| **Accountant** | Accounts clerk | Fees, collections, expenses, payroll, accounting reports; plus own staff features | Web |
| **Receptionist** | Front desk | Fee collection, enquiries, admission, student look-up | Web (tablet) |
| **Teacher / Staff** | Teacher, coordinator | Attendance, marks, diary, own leave and payslips | Teacher app |
| **Parent** | Guardian | Child's attendance, fees, results, diary, chat, payments | Parent app |
| **Student** | Student (optional per school) | Timetable, homework, results, progress | Student app |
| **Custom roles** | Any | Owner clones a role and edits permissions | Web |

Platform-side roles (our staff): Super Admin, Support (read-only plus consented "view as"), Sales/Onboarding, Finance. Deferred beyond minimal tooling in 1.0.

## 3. Personas

### P1. The Owner ("see everything, get paid")
- Goals: know collection %, defaulters, cash position, attendance, staff cost.
- Pains: no numbers, late fees, staff using personal registers.
- Needs: an owner dashboard that leads with "what needs my action"; WhatsApp weekly summary.

### P2. The front-desk all-rounder (receptionist + accountant)
- Goals: collect fees fast, issue receipts, record walk-in enquiries, answer parents.
- Needs: keyboard-fast fee collection with search by name/registration/mobile/family; 80 mm thermal receipts; day-close report.
- Note: this is the most important user for daily adoption. May also teach.

### P3. The teacher
- Goals: mark attendance and enter marks in under a minute, post diary, see timetable.
- Needs: offline operation, one-tap "mark all present", visible sync status.

### P4. The parent
- Goals: know today's attendance, dues, results; pay from phone; talk to the teacher.
- Needs: child switcher, everything within two taps, mobile number + OTP login only, plain English with prominent numbers.

### P5. The student
- Goals: see timetable, homework, results and simple hints.
- Under 10 years: student view lives inside the parent app.

## 4. Example: one person, three hats

> Ayesha is the receptionist, does the accounts, and teaches Class 3 English.
> Her single login holds roles Receptionist + Accountant + Teacher.
> She sees fee collection, expenses and reports (Accountant), enquiries (Receptionist) and her classes' attendance (Teacher) in one navigation.
> Her teaching scope is only the sections she is assigned to; her finance scope is the whole branch.

Scopes (institution / branch / assigned sections / own) are attached to each role assignment, so the union also unions scopes per permission.
