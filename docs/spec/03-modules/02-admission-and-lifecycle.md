# Module 02. Admission and Student Lifecycle

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 (test/interview steps in 1.1) |
| Module code | ADM |

## 1. Purpose
Capture enquiries, admit students, move them through classes each year, and close their time at the school (transfer, withdrawal, graduation) without losing history.

## 2. Lifecycle states

```
Enquiry: new → quotation_sent → follow_up → applied → admitted
               (contacted / visit_scheduled can occur at any step)
                                                  └→ lost
Student: applied → active ─┬→ promoted (new enrolment next session)
                           ├→ held_back
                           ├→ transferred_out
                           ├→ withdrawn
                           ├→ struck_off
                           └→ graduated (last class)
```

Offered/test/interview steps are added in 1.1; 1.0 stores a free "test result/notes" field.

## 3. Requirements

### Enquiries and admission
- **REQ-ADM-001** Enquiries come from a public link form and a receptionist walk-in form (same fields). Mandatory: student name, guardian name, guardian relationship, guardian mobile, class applied. Father's name is optional at enquiry stage and required at enrolment (or "Not available"). Everything else optional.
- **REQ-ADM-002** Enquiry inbox with statuses `new`, `contacted`, `visit_scheduled`, `quotation_sent`, `follow_up`, `applied`, `admitted`, `lost` (with a lost reason); owner; source; next follow-up date with a staff reminder; possible-duplicate hint. (ref: statuses New, In review, Contacted, Enrolled, Not proceeding, Spam.)
- **REQ-ADM-003** Converting an enquiry to a student pre-fills the enrolment form and the admission challan from the accepted quotation (see 3.1).
- **REQ-ADM-004** Admission fee heads configurable per class: admission fee (default 500), registration fee (default 500, optional), refundable security deposit (optional), first-month tuition, annual fee (optional). Each is tickable and editable per student. An **admission challan** is created on enrolment with the total shown. (ref)
- **REQ-ADM-005** Documents produced: admission form PDF, ID card, fee challan/receipt, bonafide certificate, leaving certificate. Branded templates with limited wording blocks in 1.0.
- **REQ-ADM-006** Optional face enrolment with consent (checkbox + date); consent revocable.

### 3.1 Enquiry pack and fee quotation (how admission usually starts)
In practice a parent enquires first, receives one complete information pack with the total charges for that child, goes away, and the office follows up individually after some days to turn the enquiry into an admission.

- **REQ-ADM-030** **Admission information pack (PDF).** From an enquiry, staff generate one branded PDF containing: school details (name, logo, address, phones, WhatsApp, timings, bank accounts and payment instructions, principal signature), the class and session applied for, the **fee quotation**, documents required (the school's checklist), key terms (refund/security/fee policy text set by the school), the quotation validity date, and a contact person.
- **REQ-ADM-031** **Fee quotation, customised per student.** Lines come from the class fee plan (admission, registration, security deposit if used, first-month tuition, annual fee, other heads) and are **editable for this child**: change amounts, remove or add lines, add a discount (sibling, staff-child, scholarship, concession or custom with a reason). The pack shows each line, discounts, the one-time total at admission, the monthly fee, and an estimate of the first-year total. A quotation can be re-issued; every version is kept with who changed what.
- **REQ-ADM-032** **Approval of special terms.** Discounts or fee changes above the school's limit need approval (maker-checker, if enabled) before the PDF can be shared.
- **REQ-ADM-033** **Sharing.** The PDF can be downloaded, printed, or sent to the guardian's WhatsApp number (a staff-initiated message; template cost applies), and the enquiry records when and how it was sent. Sending moves the enquiry to `quotation_sent`.
- **REQ-ADM-034** **Individual follow-up.** After the pack is given, staff set a follow-up date; the system reminds the enquiry owner on that date and shows an overdue-follow-up list, with a notes history. Each contact attempt can be logged (call, visit, WhatsApp) with outcome. Quotation validity: when it expires the enquiry shows "Quotation expired" and staff can re-issue.
- **REQ-ADM-035** **Conversion.** When the guardian decides, "Convert to admission" opens the enrolment form pre-filled with the student, guardian and the accepted quotation. The admission challan is created with the **quoted** amounts and discounts so what was promised is what is charged; differences need a reason.
- **REQ-ADM-036** **Enquiry funnel.** Counts by status, source, class and owner; conversion rate; average days to convert; lost reasons. Used for the owner dashboard and later analytics.

### Promotion and rollover
- **REQ-ADM-010** A **promotion wizard** per class: default "all promote to next class"; mark individuals as Held back or Left; optional rule "promote only if result is pass" if results exist; dues carry forward (never blocks, shows a warning). The last class → Graduated.
- **REQ-ADM-011** Promotion runs into the new session creating new enrolments; the old enrolment closes with an outcome.
- **REQ-ADM-012** There is no "demote". Explicit operations instead: change section, change class (correction or hold-back), change shift/branch, each with reason and date.

### Withdrawal and transfer
- **REQ-ADM-020** Withdrawal/transfer form: reason (list), last attendance date, dues-cleared check (warn), SLC number auto-generated, leaving certificate PDF, optional security-deposit refund flow.
- **REQ-ADM-021** After withdrawal: student inactive, excluded from attendance and new challans; existing dues remain collectable; guardian app access limited to history, dues and certificates.
- **REQ-ADM-022** Graduated students are kept as archived records for reporting and certificate re-issue.

## 4. User stories

### US-ADM-001 Record a walk-in enquiry
As a **Receptionist** I want to log an enquiry in under a minute and set a follow-up date.

- Given I enter student name, guardian name and relationship, mobile and class, when I save, then the enquiry is `new` with a follow-up date I chose and a reminder appears on that date.

### US-ADM-006 Send an information pack with a customised quote
As a **Receptionist** I want to give a visiting parent one complete PDF with the fees for their child.

- Given a walk-in enquiry for Class 3, when I generate the pack, then the quotation lines are pre-filled from the Class 3 fee plan with the totals and the school's details.
- Given the parent mentions a second child already at the school, when I add a sibling discount and a note, then the pack shows the discount line and updated totals; if the discount exceeds my limit it waits for approval before I can share the PDF.
- Given the pack is ready, when I choose Send on WhatsApp (or Print), then the enquiry becomes `quotation_sent` and records the date, user and method.

### US-ADM-007 Follow up after some days
As an **Admin** I want to follow up each enquiry individually and convert it.

- Given an enquiry in `quotation_sent` with follow-up date 5 days later, when that date arrives, then the owner sees it on their list; after I log "called, will decide Friday" and set a new date, the history shows both entries.
- Given the guardian agrees, when I press Convert, then the enrolment form and the admission challan use the quoted amounts.
- Given the quotation expired, then I can re-issue it, and the new version is kept beside the old.

### US-ADM-002 Admit from an enquiry
- Given an enquiry in `applied`, when I click Enrol, then the form is pre-filled; after saving, the enquiry becomes `admitted` and an admission challan is created.

### US-ADM-003 Promote a class
As an **Admin** I want to promote Class 5 to Class 6 with two exceptions.

- Given Class 5 has 40 students, when I mark 2 as Held back and run, then 38 get new enrolments in Class 6 of the new session and 2 are re-enrolled in Class 5; dues remain; a summary shows counts.

### US-ADM-004 Withdraw with unpaid dues
- Given a student has dues of 8,000, when I start withdrawal, then I see a warning, may continue with a reason, and the dues remain collectable after withdrawal.

### US-ADM-005 Generate leaving certificate
- Given a withdrawn student, when I generate the SLC, then a PDF with the next SLC number and school branding is produced and logged.

## 5. Business rules
- **BR-ADM-001** Promotion never deletes history; it creates a new enrolment and closes the old one.
- **BR-ADM-002** Withdrawn, transferred, struck-off and graduated students get no attendance and no new monthly challans.
- **BR-ADM-003** Security deposit at withdrawal can be adjusted against dues or refunded; refund requires approval if maker-checker is on and creates ledger entries.
- **BR-ADM-005** A quotation is a promise: the admission challan uses the accepted quotation's amounts unless a reason for change is recorded.
- **BR-ADM-006** A quotation is never edited after sending; changes create a new version.
- **BR-ADM-004** The reference's "demote" is replaced by explicit corrections with reasons.

## 6. API (representative)
`/enquiries`, `POST /enquiries/{id}/quotations`, `GET /quotations/{id}/pdf`, `POST /quotations/{id}/send`, `POST /enquiries/{id}/contacts`, `POST /enquiries/{id}/convert`, `POST /promotions/preview`, `POST /promotions`, `POST /students/{id}/withdraw`, `POST /certificates`.

## 7. Events
`EVT-ADM-enquiry.created`, `quotation.sent`, `enquiry.followup_due`, `enquiry.status_changed`, `student.admitted`, `student.promoted`, `student.withdrawn`.

## 8. Decide in design
Public form hosting and anti-spam; entry test scheduling and scoring [1.1].
