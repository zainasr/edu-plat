# 03. Data Model (Logical)

| | |
|---|---|
| Status | Draft v0.1 (logical model; physical schema is a design deliverable) |
| Release | 1.0 |
| Module code | DATA |

## 1. Conventions

- Every business table has `id` (UUID), `tenant_id`, `created_at`, `created_by`, `updated_at`, `updated_by`, and `deleted/archived` flags instead of hard deletes.
- Money is stored as integer minor units (paisa) or exact decimal; never floating point. Currency is a tenant setting (default PKR).
- Timestamps in UTC; display in the institution time zone (Asia/Karachi).
- Mobile numbers stored normalised (`03XXXXXXXXX` display, E.164 internally).
- State changes of important entities are recorded as **events** (immutable). Finance journals and receipts are append-only.
- Enumerations are lower-case strings with documented values.
- Names in the model are working names.

## 2. Entities by module

### Organisation and identity
`Organisation`, `Institution`, `Branch`, `Shift`, `Session`, `Term`, `CalendarEntry`, `User`, `Membership`, `Role`, `RolePermission`, `RoleAssignment(role, scope)`, `PersonalAccessToken`, `AuditLog`, `Device`.

### Academic structure
`Class`, `Section(shift, class_teacher)`, `Subject`, `SubjectGroup`, `SectionSubject(teacher)`, `StudentSubject (electives)`.

### People
`Student`, `Enrollment(student, session, section, status)`, `Family`, `Guardian`, `GuardianLink(student, guardian, relationship, primary, fee_payer, receives_notifications, emergency_contact, access)`, `StudentDocument`, `Enquiry`, `Quotation(enquiry, version, valid_until)`, `QuotationLine`, `EnquiryContact`, `Staff`, `StaffAssignment`, `StaffDocument`.

### Attendance and leave
`AttendanceRegister(section, date, period?)`, `AttendanceRecord(student, status, method, marked_by, source)`, `AttendanceEdit(old,new,reason)`, `StaffAttendance`, `StaffAttendanceRule`, `GeoFence`, `LeaveType`, `LeaveBalance`, `LeaveRequest(staff|student)`.

### Fees and finance
`FeeType`, `FeePlan(class)`, `FeePlanLine(fee_type, amount, frequency)`, `StudentFeeOverride`, `Discount`, `Challan(student|family, period, due_date, status, consumer_no)`, `ChallanLine`, `Payment(method, account, receipt_no)`, `PaymentAllocation`, `Receipt`, `SecurityDeposit`, `LateFeePolicy`, `FeeSettings`, `CashSession`, `Expense`, `ExpenseCategory`, `Account(chart)`, `JournalEntry`, `JournalLine`, `FiscalPeriod`, `OpeningBalance`.

### Payroll
`SalaryRecord`, `SalaryLine`, `SalaryPayment`, `AllowancePolicy`, `DeductionPolicy`, `StaffSecurityPolicy`, `StaffAdvance`, `Payslip`.

### Academics
`GradeScale`, `ExamTerm`, `Exam`, `ExamSubject`, `ExamMark`, `ReportCard`, `ClassTest`, `ClassTestQuestion`, `ClassTestMark`, `TimetableEntry`, `DiaryEntry(section, subject, teacher, date, status)`, `ClassDiary(section, date, sent_at)`, `Resource`, `SyllabusTopic` [1.1].

### Behaviour and communication
`BehaviourCategory`, `BehaviourEntry`, `BehaviourReply`, `Announcement`, `Notification`, `NotificationDelivery`, `NotificationPreference`, `ChatThread`, `ChatMessage`, `Form`, `FormResponse`.

### Platform
`Plan`, `Subscription`, `FeatureFlag`, `UsageCounter`, `SupportTicket`, `ImportBatch`, `DataExport`, `EventLog`.

## 3. Key relationships

- Organisation 1-n Institution 1-n Branch 1-n Section; Session 1-n Term; Class 1-n Section.
- Student 1-n Enrollment (one per session); Family 1-n Student, Guardian; one family login.
- Fee: FeePlan(class) → StudentFeeOverride → Challan(1-n lines) → Payment allocations → Receipt → JournalEntry.
- Every financial event (challan release, payment, void, expense, salary, refund) creates a balanced JournalEntry via the posting engine.
- Attendance(leave) is created from approved LeaveRequest.
- EventLog receives domain events (attendance, payments, marks, behaviour, messaging, app engagement) for analytics.

## 4. State machines (summary)

| Entity | States |
|---|---|
| Student lifecycle | `applied` → `active` → `promoted` / `held_back` / `transferred_out` / `withdrawn` / `struck_off` / `graduated` |
| Enquiry | `new` → `contacted` → `visit_scheduled` → `applied` → `admitted` / `lost` |
| Challan | `draft` → `released` (unpaid) → `partial` → `paid`; `void`; (`overdue` is derived from due date) |
| Payment | `recorded` → `voided` (reversing entry); online: `initiated` → `succeeded` / `failed` |
| Salary | `draft` → `approved` (unpaid) → `partial` → `paid` |
| Exam | `draft` → `scheduled` → `ongoing` → `result_published` / `cancelled`; marks: `entered` → `submitted` → `approved` |
| Leave request | `pending` → `approved` / `rejected` / `cancelled` |
| Behaviour concern | `waiting_review` → `open` → `in_followup` → `resolved` |
| Tenant | `trial` → `active` → `grace` → `suspended` → `archived` → `deleted` |
| Import batch | `uploaded` → `mapped` → `validated` → `committed` / `undone` |

Detailed diagrams are in each module document.

## 5. Data that must be captured from day one (for AI)

Attendance events (time, method), payment events (time, channel, delay), marks per subject/component and topic when available, behaviour entries, app engagement (opens, reads, actions), message delivery, teacher actions. All go to an immutable event log (raw retention 5 years).

## 6. Decide in design

- Physical schema, indexes, partitioning of large tables (attendance, ledger, event log).
- Whether the analytics store is Postgres-based first or a separate columnar database.
