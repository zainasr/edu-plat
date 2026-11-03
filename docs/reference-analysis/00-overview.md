# Reference product overview (Edflo school admin portal)

Studied read-only as a human user on a trial account (demo data). Notes record function, workflow and data shape only.

## Sitemap (from sidebar, all groups expanded)
- OVERVIEW: Dashboard /admin
- ACADEMICS: Curriculum (sessions, classes, sections, subjects), Timetable, Exams (terms, exams, results, grading policies), Class Tests, Events & Holidays
- PEOPLE: Admission (enroll student, promote, demote), Students, Families, Behaviour (log, analytics, settings), Staff (create, list, work calendar), ID Cards (templates, staff, students)
- ATTENDANCE & LEAVES: Attendance (classes, students, staff, settings), Leaves (staff requests/history/balances/types, student leaves)
- FINANCE: Fee (fees, settings, collect), Salary (salaries, pay, settings), Expense Management (list, create, categories)
- ACCOUNTING & REPORTS: Financial Reports (all, owner dashboard, unit economics, P&L, balance sheet, cash flow, equity); Analysis & Planning (comparison, budget, bridge, trend); Receivables & Operations (receivables, defaulters, fee collections, collections, concessions, payroll, expenses, assets); Ledger & Controls (trial balance, ledger, journal, periods, accounts)
- COMMUNICATION: Announcements, Chats, Forms (forms, new, leads), Daily Diary, Resources, Support
- SYSTEM: Documents, App Access (staff, parents), Administration (events-holidays, school, admins, audit log)
- Top bar: school name, session selector, trial-days-left badge + Upgrade (/admin/subscription), user menu, notifications.
- Sidebar is an accordion (one group open at a time) and collapsible.

## Dashboard (see screenshots/00-dashboard)
KPI tiles: total students, staff, classes, sections, today's attendance rate; fee collected / pending (overdue count), salary paid / pending. Financial health bars (fee collection, salary disbursement, expenses). Today's attendance ring (students/staff). "Action items" (e.g. salaries not generated). Quick actions: enroll student, add staff, create class, generate fees, generate salary, create timetable. Recent activity feed (audit-style).

## Module files
01 curriculum, 02 timetable, 03 exams, 04 class tests, 05 events/holidays, 06 admission, 07 students, 08 families, 09 behaviour, 10 staff, 11 id cards, 12 attendance, 13 leaves, 14 fees, 15 salary, 16 expenses, 17 accounting & reports, 18 communication, 19 system; 90 inferred data model; 95 gaps and opportunities. Screenshots in screenshots/<NN-module>/.

## Core user flows observed
1. **Setup**: school profile and operations -> session -> classes (with standard fee) -> sections -> subjects -> staff (+ app login) -> timetable -> grading scale -> fee types/settings.
2. **Enrolment**: enroll one student (3-step wizard) or bulk CSV import (up to 5,000) -> auto reg no and family (by father mobile) -> parent app login created from father's mobile -> monthly fee from class or discounted.
3. **Monthly fee cycle**: auto-draft challans on the 1st -> admin reviews/approves -> released with due date (10th) -> collect at counter (per student or family), advance or additional fees -> receipts/invoices -> overdue/defaulter worklist -> reports.
4. **Daily attendance**: teachers mark in app; admin monitors class registers; staff check in via app (geo-fenced) with grace and absent cutoff; parent leave requests approved by admin mark "leave".
5. **Exams**: create term (wizard) -> draft exams auto-generated per section with subjects -> marks entry -> publish results -> letter grades via grading scale. Class tests are teacher-created in the app.
6. **Payroll**: generate draft salaries -> review/approve -> pay -> history; policies for deductions/allowances/security.
7. **Communication**: announcements by audience, teacher-parent chats (admin monitor), forms/enquiries for admission leads, daily diary, resources.

## Overall observations
- Platform is school-centric with a single campus, a single admin role, and parent and staff apps only.
- Strengths: automation with human approval, family-centric billing, accounting engine, audit log, clean consistent UI (stat tiles, filters, CSV/PDF export on lists).
- Weaknesses: no SMS/WhatsApp, local payment channels not set up, thin academic analytics, no AI, no onboarding, slow loading on many pages (several took ~10 s or stayed on a spinner).
- Consistent UI patterns worth copying for simplicity: top stat tiles, search plus filters, one primary action per page, empty states with a next step, wizard for multi-step creation, "draft -> review -> release" for risky bulk actions.

## Coverage notes and caveats
- Read-only study: no data was created, edited, submitted or deleted. Forms were opened but not filled; wizard steps 2 and 3 of Enroll Student could not be viewed without filling required fields.
- Pages that did not render content in the session (spinner or empty text): fees/generate, fees/deposits, fees/history, salaries/generate, salaries/history, salaries/pay, forms/leads, documents, staff leave history, behaviour pages partly (read via fallback). Also not opened: class tests detail, individual timetable grid, individual family page, balance sheet, cash flow, equity, comparison, budget, bridge, trend, receivables, fee-collections, collections, concessions, payroll, expenses, assets, trial balance, ledger, journal reports (described from the reports index only).
- Demo data appears to contain real-looking names and phone numbers; none were copied into these notes.
