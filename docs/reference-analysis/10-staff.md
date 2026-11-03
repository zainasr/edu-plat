# 10 Staff
Routes: /admin/staff, /admin/staff/create, /admin/staff/work-calendar. Screenshots: screenshots/10-staff/.

- List: tiles (staff, teachers, non-teaching, new this month); search by name/email/type; filter Staff Type (Teacher, Manager, Administrative, Support, Security, Maintenance, IT, Finance, Other); CSV/PDF; columns member (avatar, name, email), phone, type, joined, view/edit/delete. No bulk import visible for staff.
- Add staff form: photo (upload/capture), **App Access** (username + password, optional; signs into the teacher app for classes, attendance, salary, leave), Full Name*, Email*, Phone, Staff Type*, Joining Date (used for security-deposit deduction calculation), Years of experience, Monthly Salary (PKR), Subjects* (comma-separated free text), Qualifications, Documents (admin-only: contracts, degrees, CNIC scans, uploaded after save).
- Work Calendar: pick a teacher, see a monthly overview with attendance and timetable views and activities.
- Entities: Staff(type, email, phone, joining_date, salary, subjects[], qualifications, experience), StaffAccount(username, password), StaffDocument.
- Observations: roles are just "Staff Type"; no custom roles/permissions here (see Administration > Admins). Subjects are free text, not linked to the Subject table. Email required, phone optional (opposite of our phone-first login). No CNIC field, bank account, designation, department, or branch.
- Ideas: structured subject-class-teacher assignment, workload view, performance/feedback insights, phone-first login, staff import via Excel, role templates.
