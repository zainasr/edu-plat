# 95 Gaps and opportunities (vs our goals)

## Where the reference is strong (worth matching)
- Draft-then-approve automation for fees and salaries (auto-generated monthly drafts, review, release).
- One family login across siblings, keyed by father's mobile; family billing and sibling discount.
- Real double-entry accounting hidden behind simple fee/salary/expense actions, with owner dashboard, aging, defaulter worklist, period close, audit log.
- Bulk CSV import with column mapping, mobile normalisation (03XXXXXXXXX), opening-balance ("remaining dues") import.
- Form builder with admission/enquiry templates; behaviour module with parent-sharing controls; geo-fenced staff check-in.

## Gaps against our requirements
| Area | Gap in reference | Our response |
|---|---|---|
| Structure | One campus, no branches, no shifts; single admin role | Branch and shift as first-class; custom roles with permissions; branch-level reporting |
| Apps | Only parent and staff logins; no student app | Student, parent, teacher apps; admin/reception on web |
| Attendance | No face/QR/ID-card scan, no offline, no parent alert settings, no period-wise | Camera/ID-card (QR) kiosk with offline queue, auto parent push/SMS, chronic-absence alerts |
| Import | CSV only, students only | Excel (.xlsx) import for students, staff, fees, opening balances, with error report |
| Payments | Payment method often "Not stated"; online payments not set up; no JazzCash/Easypaisa/1Link/Raast | Local channels, QR on challan, reconciliation, payment links in parent app |
| Messaging | In-app announcements only; no SMS/WhatsApp, templates, scheduling | Notification hub with rules, templates, channels, delivery tracking, cost control |
| Academics | Subject-teacher mapping and syllabus/topic tree absent; no datesheet, no report-card/position logic seen; no paper maker | Topic tree per subject; datesheet; report cards; later: paper/test maker and exam prep |
| Promotion | Bulk class-to-class only; no result-based rules, transfer, withdrawal, leaving certificate | Full lifecycle: admit, promote/hold back by rules, transfer, withdraw, certificates |
| Staff | Free-text subjects, no import, no CNIC/bank/department, no per-lecture pay | Structured assignment, Excel import, per-lecture payroll for academies |
| Payroll | No payslip, no bank file, no tax/EOBI, no advances | Payslips, bank bulk file, advances, local statutory options |
| Analytics | Finance reports are deep, but academic/engagement analytics are thin (behaviour summary only) | Student 360, class/teacher analytics, early-warning, parent engagement |
| AI | No ML-driven insight visible | See below |
| Platform | Manual upgrade request; no tiers, billing, quotas | Plans by student count, self-serve checkout, trial management, platform-admin console |
| Fiscal year | Calendar-year fiscal period in reports | Configurable fiscal year (July-June / April-March) |
| Onboarding | Many empty states; no guided setup | Setup wizard and checklist; sample data; import-first onboarding |
| Usability | Finance area has many pages; some pages slow to load (up to ~10 s here) and some stayed on a spinner | Plain-language owner view first; fast, low-end-device-friendly pages; offline-tolerant mobile |

## Opportunities tied to our moat (AI/ML on our own data)
1. **Fee default prediction** and next-best-action (who to call/message today) using payment history, family size, class, month.
2. **Student improvement engine**: marks by topic + attendance + behaviour + diary completion -> weak-topic detection, plain-language hints for students and parents, suggested practice (links to later paper/test maker).
3. **Early-warning** for dropout/chronic absence/behaviour escalation with explainable reasons.
4. **Timetable generator** from subjects-per-week, teacher availability, rooms, shifts.
5. **Smart attendance**: face/ID-card recognition, anomaly detection (proxy, patterns by weekday).
6. **Owner copilot**: "explain this number", weekly summary in WhatsApp, cash-flow forecast, budget suggestions.
7. **Parent engagement**: weekly digest, celebration of positives, targeted nudges that raise reads and on-time payments.
8. **Teacher assistance**: auto-draft diary, test generation, marks-entry via photo/OCR of mark sheets.

## Simplicity guidance from what we observed
- Lead every role with a "today" screen (what needs action) rather than module menus; the reference's 13 fee quick actions and 25+ reports would overwhelm non-technical users.
- Keep draft-and-approve for money, but make approval one tap with bulk select.
- Hide accounting until needed; default chart of accounts is automatic.

## Open questions for you (to carry into Step 3)
1. Student login: separate student app/login (phone or ID) in MVP, or student access through the parent account first?
2. Branches and shifts: is multi-branch required at MVP, or schema-ready but UI later?
3. Accounting depth for MVP: keep a full double-entry core with simple screens, or a simpler income/expense ledger first?
4. Messaging: do we commit to SMS/WhatsApp in MVP, and who pays for message costs (institution credits)?
5. Fiscal and academic calendar: should fees follow the academic session (e.g. April-March) with configurable start month?
