# Module 01. Students and Families

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | STU |

## 1. Purpose
Keep one correct record of every student and their family: identity, guardians, documents, enrolment history, status, and the family login.

## 2. Requirements

- **REQ-STU-001** Student record fields. Mandatory: full name, father's name (as on the B-Form; may be set to "Not available" with a reason such as deceased or unknown), at least one guardian with a mobile number, class, section, gender, DOB (DOB mandatory from Class 1, optional below). Optional: B-Form/CRC, photo, religion, nationality, blood group, address, birth place, secondary phone, previous school, emergency contact, medical notes/allergies, transport need (yes/no), siblings (auto).
- **REQ-STU-002** Guardians: a guardian is any responsible adult, not only a parent. Each has name, mobile, optional CNIC, occupation, and a **relationship to each linked student**: father, mother, grandfather, grandmother, uncle, aunt, brother, sister, other (free text). At least one guardian is required; the **father is not required to be a guardian**. Up to 4 linked guardians per family; each has own mobile + OTP login. (ours; the reference assumes a father.)
- **REQ-STU-014** **Father's name vs guardian.** Father's name is an identity field used on certificates and ID cards. The **guardian is the responsible adult who pays the fees** and is the school's contact for the child; usually also the person who looks after the child. The original parents may play no financial role at all and need not be linked as guardians. Example: a child's father is Imran Khan (on records), but the uncle Kamran Khan pays the fees and is the guardian. Imran is not a guardian unless the school chooses to add him.
- **REQ-STU-015** **Guardian link flags** (per guardian per student): `primary` (the family login; exactly one per family), `fee_payer` (challans and receipts are addressed to this person; default the primary), `receives_notifications`, `emergency_contact`, and an optional `access` level (`full`, `view_only`, `none`) for cases such as a parent who must not be contacted. Changing flags is audited.
- **REQ-STU-016** **Guardian tree.** The family page offers a **tree view**: the family at the root, its guardians as branches (name, relationship, mobile, flags as small badges), and the students under each guardian they are linked to. A student linked to several guardians appears under each. Staff can switch between tree view and list view. The tree is read-only to teachers and editable by users with `family.manage`. It is also shown in a simplified form in the parent app ("My family").
- **REQ-STU-017** **Related families** [1.1]: optional link between two separate families (for example cousins studying in the school) shown as a dotted relation in the tree. No shared billing.
- **REQ-STU-003** Registration number: Auto (configurable prefix + year + sequence) or Manual. Unique per institution.
- **REQ-STU-004** Family: students sharing a guardian are linked into one family (number `F-nnnnn`) automatically by matching the primary guardian mobile; the user confirms on screen. Admins can merge or split families (audited).
- **REQ-STU-005** Student list MUST support search by name, registration number, mobile, family; filters by branch, class, section, status; CSV/Excel export.
- **REQ-STU-006** Duplicate detection: warn on same B-Form, or same name + DOB + guardian mobile; allow override with a reason; an audited merge tool for duplicates.
- **REQ-STU-007** Documents: document checklist configurable per institution (B-Form, photos, previous result, leaving certificate); upload optional; status received/pending; files stored privately with signed links.
- **REQ-STU-008** Enrolment history: every change of class, section, shift or branch creates a history entry with reason and date; no data is lost.
- **REQ-STU-009** Student profile ("Student 360") shows: details, family, enrolment history, attendance summary, fee status, results, behaviour (staff-permissioned), documents.
- **REQ-STU-010** Optional face enrolment with parent consent (see Attendance).
- **REQ-STU-011** Student ID card (QR) printable and shown in the app; static printed QR has a checksum and can be disabled if lost.
- **REQ-STU-012** Bulk Excel import of students and families (see Onboarding and Import module).
- **REQ-STU-013** Sibling discount is configured per institution and applies from family links (see Fees).

## 3. Student status
`active`, `promoted` (enrolment closed, new one created), `held_back`, `transferred_out`, `withdrawn`, `struck_off`, `graduated`. Detailed transitions in the Admission and Lifecycle module.

## 4. User stories

### US-STU-001 Enrol a student
As a **Receptionist** I want a short enrolment form so that admission takes minutes.

- Given the form requires name, father's name (or "Not available"), a guardian with relationship and mobile, class, section, gender, DOB, when I save with those, then a registration number is generated and the student appears under the family matching that mobile.
- Given a student with the same B-Form exists, when I save, then I see a duplicate warning and may override with a reason.

### US-STU-002 Find a student fast
As a **Receptionist** I want to type any part of a name, registration number or mobile and see matches instantly.

- Given 800 students, when I type a mobile number, then matching students and their family are listed in under 1 s.

### US-STU-003 See the whole family
As an **Accountant** I want one family page showing all siblings and total dues so that I can take one payment.

- Given a family with 3 children, when I open the family, then I see each child's dues and a combined total.

### US-STU-004 Merge families
As an **Admin** I want to merge two families created by mistake so that billing is correct.

- Given two families, when I merge, then siblings move to the target family, history is kept and the merge is audited.

### US-STU-006 Child raised by an uncle
As a **Receptionist** I want to enrol a child whose guardian is the uncle, not the father.

- Given the child's father is Imran Khan, abroad, and the uncle Kamran Khan pays and receives messages, when I enter father's name Imran Khan and add Kamran Khan as guardian with relationship "Uncle", flags primary and fee payer, then the family login is Kamran's mobile, challans are addressed to Kamran, and certificates show Imran Khan as the father.
- Given Imran Khan never pays and the school does not need to contact him, then he is not added as a guardian at all and appears only as the father's name on records.
- Given the school wants him informed, when I add him as a guardian with `view_only` access and `fee_payer` off, then he sees results but cannot pay or change data and no challan is addressed to him.

### US-STU-007 See the family as a tree
As an **Accountant** I want a tree view of the family so that I understand who is who.

- Given a family with uncle Kamran (primary, fee payer) linked to his son Ali and his nephew Bilal, and Bilal's mother linked to Bilal only, when I open the tree view, then I see Kamran with Ali and Bilal under him, Bilal's mother with Bilal under her, and each guardian's relationship and flags as badges.
- Given I switch to list view, then the same data appears as a table.

### US-STU-005 Change section
As an **Admin** I want to move a student to another section with a reason so that the history is kept.

- Given a student in 5-A, when I move to 5-B with reason "correction", then attendance history stays and a history entry is created.

## 5. Business rules

- **BR-STU-001** A student has exactly one family.
- **BR-STU-005** A family has at least one guardian and exactly one `primary` guardian. The primary guardian need not be the father.
- **BR-STU-006** The fee payer is always one of the student's linked guardians. If the fee payer is removed, billing falls back to the primary guardian.
- **BR-STU-007** A guardian can only see the students they are linked to, even when the family contains other students (for example a nephew's mother does not see her cousin's data).
- **BR-STU-002** Students are never hard-deleted; "delete" means withdraw/deactivate. A deletion request goes through maker-checker if enabled.
- **BR-STU-003** Changing the primary guardian mobile requires OTP verification of new number or admin verification.
- **BR-STU-004** Two active students in the same institution cannot share a registration number.

## 6. API (representative)
`POST /students`, `GET /students?filter[...]`, `PATCH /students/{id}`, `POST /students/{id}/change-section`, `GET /families/{id}`, `GET /families/{id}/tree`, `PUT /students/{id}/guardians/{gid}` (relationship and flags), `POST /families/merge`, `POST /students/{id}/documents`.

## 7. Events
`EVT-STU-student.created`, `student.updated`, `student.section_changed`, `family.merged`.

## 8. Reference notes
(ref) Enrol wizard: photo, name, father name (we also allow a non-father guardian), auto/manual registration number, father's mobile as family login lookup, session/class/section, "single student" or "new family"; optional personal/address/previous-school/fee sections. (ours) DOB rule, duplicate detection, merge tool, document checklist, and change-history with reasons.
