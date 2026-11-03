# 90 Inferred data model (from the reference product's screens)

Inferred only. Names are our own working names, not theirs. All business tables carry `school_id` (tenant) and audit fields. Confidence is lower where a screen was not opened.

## Tenancy and identity
- **School**(id, name, campus_name, type, level, status, reg_no, established_year, address fields, phone, whatsapp, email, website, timezone, working_days[], start_time, end_time, period_minutes, lunch_break, logo, plan, trial_ends_at)
- **User**(id, school_id, kind[admin|staff|parent], username (mobile or email), password_hash, status). Parent login = father mobile, one login per Family.
- **Admin**(user_id, role_title, permissions[30 flags])
- **AuditLog**(school_id, actor_user_id, role, module, action[created|updated|deleted|access|approval|signin], entity, summary, at)
- **Subscription/UpgradeRequest**(school_id, plan, status, requested_at)

## Academic structure
- **Session**(school_id, name, description, is_current)  (no structured start/end dates)
- **Class**(session_id, name, code, level[early_years..o_a_levels], grade_no, standard_monthly_fee, max_students, stream)
- **Section**(class_id, name, description, capacity)
- **Subject**(class_id, name, total_marks, pass_pct, description)  (subject is per class)
- **Timetable**(section_id, session_id) 1..n **TimetableEntry**(day, period_no, start, end, subject_id, teacher_id, is_break)
- **CalendarEntry**(school_id, type[holiday|event], title, start, end, description)

## People
- **Student**(school_id, reg_no, full_name, gender, dob, birth_place, nationality, religion, b_form_crc, photo, address, status[active...], admission_date, family_id, secondary_phone, email)
- **Enrollment**(student_id, session_id, class_id, section_id, monthly_fee_override, discount_type[SIBLING..], admission_fee, status)
- **Family**(school_id, number F-nnnnn, label, status); **Guardian**(family_id, relation[father|mother|guardian], name, mobile, income, is_primary_contact, has_app_login)
- **StudentDocument**(student_id, type, file, admin_only)
- **Staff**(school_id, type[teacher|manager|administrative|support|security|maintenance|it|finance|other], name, email, phone, joining_date, monthly_salary, experience_years, subjects (free text), qualifications, photo, user_id)
- **StaffDocument**(staff_id, file)
- **BehaviourCategory**(type[positive|negative|note], name, severity, points); **BehaviourEntry**(student_id, category_id, author_staff_id, text, status[waiting_review|open|in_followup|resolved], shared_with_parent); **BehaviourReply**
- **IdCardTemplate**(layout[smart|classic|custom], orientation, colors, audience)

## Attendance and leave
- **StudentAttendance**(student_id, date, status[present|absent|late|leave], marked_by, method)
- **StaffAttendance**(staff_id, date, check_in_at, status[present|late|absent|leave|not_checked_in], method[app|manual], remarks)
- **StaffAttendanceRule**(day, start, grace_min, absent_cutoff); **GeoFence**(lat, lng, radius_m, enforce)
- **LeaveType**(name, cadence[annual|monthly], quota, paid); **LeaveBalance**(staff_id, type, year, allocated, used); **StaffLeaveRequest**; **StudentLeaveRequest**(student_id, parent, from, to, reason, status) (approval writes "leave" attendance)

## Exams
- **GradeScale**(school_id, min_pct, grade)
- **ExamTerm**(session_id, name, type[mid|final|quarterly|monthly|class_test|other], annual_weightage_pct)
- **Exam**(term_id, section_id, status[draft|scheduled|ongoing|result_published|cancelled], total_marks) -> **ExamSubject**(subject_id, marks, date) -> **ExamMark**(student_id, obtained, absent)
- **ClassTest**(created by teacher in app: section, subject, title, total, status[scheduled|marks_published|cancelled]) -> marks

## Finance
- **FeeType**(name, default_amount, behaviour[monthly|on_admission|as_needed|refundable], active, is_system)
- **Challan**(student_id or family_id, period, due_date, status[draft|unpaid|partial|paid|void]) -> **ChallanLine**(fee_type_id, amount, paid)
- **Payment**(challan lines, amount, method[cash|bank|online|cheque|card], account_id, recorded_by, receipt_no APH-YYYY-NNNNNN, voided) ; **Invoice** (family or single, custom or automated)
- **SecurityDeposit**(student_id, amount, status held/refunded)
- **FeeSettings**(auto_generate, generate_day, due_day, challan_layout, invoice_numbering, bank_account_for_challan, online_payments_gateway)
- **Salary**(staff_id, period, base, allowances, deductions, net, status[draft|approved|partial|paid]); **SalaryPayment**; **AllowancePolicy**, **DeductionPolicy**, **StaffSecurityPolicy**; **SalarySettings**(auto_generate)
- **Expense**(category_id, title, amount, date, vendor, paid_from_account_id, on_credit, description); **ExpenseCategory**
- **Account**(code, name, type[asset|liability|equity|income|expense], is_system, hidden, kind[cash|bank]); **JournalEntry**(date, memo, source_type, source_id, period) -> **JournalLine**(account_id, debit, credit)
- **FiscalPeriod**(month, status[open|closed]); **OpeningBalance**; **Budget**(year, account_id, month, amount); **FixedAsset**(category, cost, depreciation)

## Communication and content
- **Announcement**(title, message, audience[everyone|teachers|staff|parents|specific], recipients); **SystemNotification**(user_id, type, payload, read)
- **ChatConversation**(teacher_id, parent_user_id) -> **ChatMessage**
- **Form**(template, status[draft|live|closed], link) -> **FormQuestion**; **FormResponse / Lead**(form_id, answers, status) (lead converts to Student)
- **DiaryEntry**(section_id, subject_id, teacher_id, date, text, is_markable_task); **Resource**(category, title, file/notes, scope class/section/school, status)
- **SupportTicket**(subject, status[open|in_progress|resolved|closed]) -> messages

## Key relationships
School 1-n Session; Session 1-n Class; Class 1-n Section, Subject; Section 1-n Enrollment; Student 1-n Enrollment (one per session); Family 1-n Student, Guardian; Family 1-1 parent login. Fee posting and salary posting and expense -> JournalEntry (double-entry). Attendance(leave) <- approved leave requests. Behaviour/attendance/exam marks are all keyed by student and session, the base for analytics.

## Data we must collect from day one for our AI/ML moat (not in the reference)
Per-question and per-topic marks (needs syllabus topic tree), attendance timestamps and methods, fee payment timing and channel, parent app engagement events (opens, reads, replies), teacher activity, behaviour trends, diary/homework completion, test/paper attempts, and outcome labels (dropout, fee default, grade change).
