# 19 System (App Access, Administration, Documents, Subscription, top bar)
Screenshots: screenshots/19-system/.

## App Access
- **Parents** (/app-access/parents): "One login per family unlocks the mobile app for every sibling." Tiles: families (98, with 2 unlinked students), with access, pending. Table: family #, students (+N more), father name + mobile (the username), status Active; row actions reset credentials, edit, add parent. Section "Unlinked students": students without a family number, with a "Link to family" action to inherit an existing login.
- **Staff** (/app-access/staff): username/password per staff for the teacher app ("classes, attendance, salary and leave"); tiles staff, admins on staff (use admin login), with access, pending setup; masked credentials, status, actions.
- Only parent and staff apps exist here; there is no separate student login (students are reached via the family login). Our plan differs: separate student, parent and teacher apps.

## Administration
- **School profile** (/administration/school): logo, name, campus name ("Main Campus"), city/province, type (Private), level (Secondary), status, email, phone, year of establishment, registration number, address (street, area, city, province, country), contact (phone shown on documents, official email, website, WhatsApp). **School Operations**: timezone (Asia/Karachi), working days (Mon-Sun toggles), school hours (start/end), period duration (35 min), lunch break. Single campus per account: no multi-branch, no shifts.
- **Admins** (/administration/admins): admin users with role/title and a **permissions meter (30/30)**; Add Admin; CSV; edit. Granular permission flags exist (30), but only the built-in "Admin" role was seen; custom roles not seen.
- **Audit Log** (/administration/audit-log): every change (who, when, what). Filters: period (Today, 7/30/90 days, all, custom), role, user, module, action (Created, Updated, Deleted, Access, Approvals, Sign-ins); tiles entries/deletions/access changes; Export CSV; expandable rows (810 entries, e.g. "Marked attendance" in module Attendance).
- **Events & Holidays**: see 05.

## Documents (/admin/documents)
Opened but content did not render in the session. Likely a school-level document vault, separate from per-student documents.

## Subscription (/admin/subscription) and top bar
- Plan card "Free trial" with end date and days remaining (6). "Upgrade to Premium" is a **request flow**: "our team will reach out to finalize the move", a manual sales-assisted upgrade; listed benefits: unlimited access to every module, no trial expiry, priority support, feature parity. Shows "Request pending" with submission date. No self-serve pricing, tiers or payment.
- Top bar: school logo and name, **academic session selector** (2026-27, scopes all data), trial badge with days left and Upgrade button, user menu (name, role). No notification bell was found on this layout.
- Dashboard "Recent activity" feed is a view of the audit log.

## Gaps/ideas
Platform needs: plans and quotas by student count, self-serve checkout (JazzCash/Easypaisa/bank/card), SaaS invoices, trial extension, in-app onboarding checklist (this account shows many empty states), data import wizard at first login, role templates, multi-branch/shift setup, 2FA, phone-OTP login, and a separate platform-admin console.

## Hands-on addendum (2026-10-12)
- **Documents vault** (/documents): upload and keep contracts, certificates and records for staff and students. Tiles: documents, staff, students, expiring (expired or within 30 days). Categories: Contract, Educational, Identity, Experience, Medical, Legal, Financial, Correspondence, Other. Filters: owner type (Everyone, Staff, Students, Not linked), visibility (Shared / Private to me), Expiring. Row: file name/size/uploader, category, belongs to (or not linked), issue and expiry dates with an "Expired" flag, Download/Edit/Delete.
- Navigation quirk: several pages render blank on direct URL load and only work when reached by clicking through the app.
