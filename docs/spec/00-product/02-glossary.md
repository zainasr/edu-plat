# 02. Glossary

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | GLO |

| Term | Meaning in this product |
|---|---|
| **Organisation (Tenant)** | The customer account. Holds subscription, billing, owner(s), data isolation. |
| **Institution** | A school, academy or college inside an organisation. |
| **Branch (Campus)** | A physical campus of an institution. Cash books, classes and attendance are scoped to a branch. |
| **Shift** | A time block of a branch (Morning, Evening) with its own timings. A shift is a property of a section. |
| **Session** | An academic year (e.g. 2026-27). One is "current" per branch. |
| **Term** | A part of a session (3 or 6) used to group exams and report cards. |
| **Class** | A grade level (e.g. Class 5). For academies it can be labelled Batch/Course. |
| **Section** | A division of a class (5-A). The unit of attendance, timetable and exams. |
| **Subject** | A course taught in a class. Colleges: Course (with optional credit hours). |
| **Enrolment** | A student's membership of a section in a session. A student has one enrolment per session (two only with the advanced dual-enrolment option). |
| **Registration number** | Institution-unique student identifier; format configurable (prefix + year + sequence), imports may bring existing numbers. |
| **Family** | The group of siblings sharing one guardian login and one family fee statement. Identified by a family number. |
| **Guardian** | The responsible adult who pays the fees and is the school's contact (parent, uncle, grandparent, sibling or other), not necessarily a parent. The primary guardian's mobile is the family login. |
| **Admission information pack / Fee quotation** | A branded PDF given to a prospective parent with school details, documents needed and the customised charges for that child, with a validity date. Accepted quotation amounts carry into the admission challan. |
| **Guardian tree** | A tree view of a family: guardians as branches with their relationship and flags, students under each guardian they are linked to. |
| **Challan** | A fee voucher listing fee lines, amount and due date. Has a unique consumer/voucher number. |
| **Fee head / Fee type** | A named charge (tuition, admission, security deposit...). |
| **Fee plan** | The set of fee heads and amounts that apply to a class. |
| **Receipt** | Proof of a payment, numbered `PREFIX-YEAR-NNNNNN`, sequential, no gaps. |
| **Defaulter** | A student with dues overdue by 30 days or more (configurable). |
| **Security deposit** | Refundable money held on behalf of a family; a liability, never income. | will be optional it not common in pakistan at school level.
| **Advance** | A payment made against a future challan. |
| **Concession / Discount** | A reduction of fees: sibling, staff-child, scholarship, need-based, custom. |
| **Cash session** | A collector's shift of money handling: opening, collections, closing count. |
| **Period close** | Freezing a month's books; later postings go to the next open month. |
| **Maker-checker** | A control where one person proposes and a different person approves. |
| **SLC** | School Leaving Certificate. |
| **B-Form / CRC** | Child registration certificate used as a child's identity number. |
| **CNIC** | National identity card number for adults. |
| **Diary** | Daily homework/notes a teacher posts per section/subject. |
| **PTM** | Parent-teacher meeting. |
| **Consumer number (1Bill/1Link)** | A payment reference allowing a challan to be paid from any bank app. |
| **BSP** | WhatsApp Business Solution Provider (a Meta-approved reseller of the WhatsApp Business API). |
| **RLS** | Row-level security (database feature enforcing tenant isolation). |
| **Reference product** | The existing product studied as a behavioural baseline (Edflo). |
