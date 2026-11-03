# Module 06. Staff, Leave and Payroll

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 (per-lecture/hourly pay: 1.1) |
| Module code | STF |

## 1. Purpose
Keep staff records, assign teachers to sections and subjects, manage leave, and pay salaries through a reviewed monthly run with payslips.

## 2. Requirements

### Staff records
- **REQ-STF-001** Mandatory: name, phone, designation, joining date, pay type and amount. Optional: email, CNIC, DOB, address, qualification, experience, department, contract type, bank account/IBAN, emergency contact, photo, documents (contract, degrees, CNIC copy).
- **REQ-STF-002** Staff categories (default list, editable per institution): Teaching, Administrative, Support, Security, Maintenance, IT, Finance, Management (the reference has Teacher, Manager, Administrative, Support, Security, Maintenance, IT, Finance, Other), plus free-text designation and optional departments.
- **REQ-STF-003** App access is optional per staff member; the person's mobile becomes the login. Roles are assigned per the identity module (several allowed at once).
- **REQ-STF-004** Excel import of staff.
- **REQ-STF-005** Assignments are structured: class teacher per section; subject teacher per section-subject (drives timetable, marks entry access and diary); workload (periods per week) shown. A **substitute** can be assigned for a day from the timetable; the substitute gets temporary access; logged.
- **REQ-STF-006** Leaving: mark as left (date, reason); final settlement computed in a payroll run (prorated salary + security refund); experience letter PDF.

### Leave
- **REQ-STF-010** Leave types configurable (casual, sick, annual, unpaid) with cadence (annual/monthly), paid/unpaid flag, quota, optional carry-forward, half-day support. (ref + ours)
- **REQ-STF-011** Per-staff balances (allocated, used, remaining) with "initialise all staff". Requests are raised in the staff app; approval by Admin/Principal with history; approved leave auto-reflects in attendance and payroll.
- **REQ-STF-012** Student leave: parents request in the app; an approver decides; approval marks attendance `leave` for the covered days.

### Payroll
- **REQ-STF-020** Lifecycle: **Generate → Draft → (adjust) → Approve → Unpaid → Paid**, partial payment allowed. Drafts are always reviewed. Auto-generation is optional and OFF by default. (ref)
- **REQ-STF-021** Components: basic, fixed named allowances, deductions (absent/late per policy, advance recovery, income tax as manual amount, EOBI/provident as manual amount, security). Statutory auto-calculation is later.
- **REQ-STF-022** Monthly run on a set date. Joiner/leaver proration by calendar days. Absent/unpaid-leave deduction uses day rate = monthly salary ÷ institution working days in the month (configurable alternative ÷ 30).
- **REQ-STF-023** Payment methods: cash or bank (recorded); a generic bank-transfer CSV; payslip PDF and in-app view; WhatsApp share. The reference lacks payslips and a bank file (ours).
- **REQ-STF-024** Staff advances: amount and instalments, recovered automatically from future salaries; ledger as staff receivable.
- **REQ-STF-025** Optional staff security-deduction policy (one active; off by default): withheld from new staff for a period after joining; returned on leaving.
- **REQ-STF-026** Salary approval can require maker-checker. Payment records method, date and account and posts to the ledger.
- **REQ-STF-027** Per-lecture/hourly pay is supported by the data model (pay type per staff); the feature ships in 1.1.

### Staff app (teacher)
Check-in/out with geo-fence, today's timetable, mark attendance, enter marks, class tests, diary, leave request, payslips, announcements, chats, student list with parent contact. Excludes payroll admin and finance.

## 3. User stories

### US-STF-001 Add a teacher with app access
- Given I enter name, phone, joining date, salary and tick App Access, when I save, then the staff record exists and the teacher can sign in with the phone number.

### US-STF-002 Monthly payroll
As an **Accountant** I want to generate, review and pay salaries.

- Given 14 staff and attendance for October, when I generate, then 14 drafts are created with absent/late deductions and advance recoveries; I adjust one, approve all, pay in cash from "Cash in Hand"; payslips are available in each staff app.
- Given generation already ran for a staff member, when I generate again, then that staff member is skipped.

### US-STF-003 Leave request
- Given I am a teacher with 8 casual leave days left, when I request 2 days and the Admin approves, then my balance shows 6 and attendance shows `leave` for those days.

### US-STF-004 Substitute teacher
- Given Mr. Ali is absent, when the Admin assigns Ms. Sara to his 3rd period, then Ms. Sara sees that class for the day and can mark attendance; the substitution is logged.

### US-STF-005 Advance recovery
- Given a Rs 30,000 advance in 3 instalments, when payroll runs, then Rs 10,000 is deducted each month and the receivable falls to zero after the third.

## 4. Business rules
- **BR-STF-001** A salary is generated once per staff per period.
- **BR-STF-002** Approved salaries are not edited; adjustments need reversal or the next period.
- **BR-STF-003** Staff are never deleted; marking as left ends access but keeps history.
- **BR-STF-004** The approver of a salary cannot be the generator when maker-checker is on.

## 5. API (representative)
`/staff`, `/staff/{id}/assignments`, `/leave-types`, `/leave-requests`, `POST /salaries/generate`, `POST /salaries/{id}/approve`, `POST /salaries/{id}/pay`, `/advances`, `/payslips`.

## 6. Events
`EVT-STF-salary.generated`, `salary.approved`, `salary.paid`, `leave.approved`, `staff.left`.

## 7. Decide in design
Statutory deductions (EOBI/tax) automation timing; bank file formats on demand.
