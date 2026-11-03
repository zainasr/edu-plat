# 07 Students
Routes: /admin/students, /admin/students/:id, /admin/students/bulk-upload. Screenshots: screenshots/07-students/. (Student PII in demo data intentionally not copied.)

## List
- Tiles: students, classes, sections, sessions. Search by name or ID; filters class and section; CSV and PDF export; row checkboxes (bulk). Columns: student (photo avatar, name, reg no), parent name + mobile, family #, class/section, monthly fee (discounted, with original struck through), actions (view, edit, delete). 10 to 100 per page.
- Buttons: Reg-No Settings, Bulk Upload, Enroll Student.
- **Reg-No Settings** modal: only the prefix is editable (letters/digits/dashes, max 16); format PREFIX-YYYY-NNNN, counter resets each calendar year; shows next number.

## Bulk import (CSV, up to 5,000 rows, 4 steps: Upload, Configure (map columns & defaults), Review (per-row verify, set reg no), Done summary)
- Template download. Columns: Name*, Father Name*, Father Mobile*, Secondary Number, Family Number, Registration Number, Monthly Fee, Remaining Fee, Remaining Fee Due Date, Email, Gender, DOB, B-Form/CRC, Nationality, Religion, Place of Birth, Address 1/2, City, Province, Postal Code.
- Rules: mobiles auto-normalised to 03XXXXXXXXX (fixes Excel dropping the 0, converts +92); matching Family Number makes the student inherit existing parents; blank monthly fee = class fee, 0 = waived, cannot exceed class fee; **Remaining Fee creates an opening-balance "Remaining dues" challan** due on the given date.
- Note: CSV only here, while our requirement is Excel (.xlsx) import.

## Student detail
Header (photo, name, reg no, status Active), Edit Student. Quick info (class, section, grade, session, admission date, primary contact, secondary number, family #). Sections A Student info, B Admission, C Parent/guardian (father, mobile, monthly income, primary contact used for SMS/WhatsApp/OTP, mobile-app login username = mobile, one login per family), D Address, E Fee & Discount (admission fee, discounted fee, discount type), Documents (admin-only upload), Behaviour summary (positive/negative/notes this session, log entry).

## Statuses and entities
Student status: Active (others e.g. withdrawn/transferred not seen). Enrollment is per session. Entities: Student, Enrollment(student, session, class, section), Family (see 08), Document, DiscountType.

## Gaps/ideas
Siblings auto-discount exists (SIBLING). No visible student timeline, attendance/exam/fee summaries on detail page, no health/transport fields, no withdrawal/transfer flow, no duplicate detection beyond mobile/family. Opportunity: single student 360 profile with AI insight cards; Excel import with smart column auto-mapping and error report; B-Form validation.

## Enroll wizard (observed hands-on, 2026-10-12)

Three steps: Basic Info, Details, Parents. "Enroll Student" is available from step 1, so steps 2 and 3 are optional.

Step 1 Basic Info: photo upload or camera capture, full name*, father name*, registration number (Auto with estimate, or Manual), father's mobile* (this is the family's parent-app login; the form looks up whether the mobile already belongs to a family), optional secondary number (record only, no login), academic session*, class*, section* (sections load after class).
- New mobile shows a required choice: "Single student" (one login for this student, can group into a family later) or "Create a new family" (later siblings on the same mobile join automatically, one login and one fee invoice).
- A parent login is created either way with a fixed default password shown on screen. Weakness: do not copy; use OTP or a one-time link.

Step 2 Details (all optional): gender, date of birth, place of birth, nationality, religion (Islam/Christian/Hindu/Other), B-Form/CNIC, special needs flag; address lines, city, province (Punjab, Sindh, KP, Balochistan, GB, ICT), postal code; previous school name and reason for leaving.
- Fee and discount section: "Charges on admission" prefilled from Fees > Fee Types, each row tickable and editable: Admission Fee (default 500), Registration Fee (default 500), Security Deposit (refundable, held, not income), Monthly Tuition for the current month (off until ticked; a figure typed there is a one-off charge and does not change the monthly fee). Shows "N charges on this challan" and "Total billed": enrolling generates an admission challan.
- Standard monthly fee (class default, e.g. 3,000), discount type (None, Sibling, Staff, Merit), discounted fee.

Step 3 Parents: not yet observed (click blocked by the automation permission check).
