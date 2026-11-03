# Module 11. Analytics and Student Improvement

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 rule-based baseline; 1.1 topic-level hints; 1.2 first models |
| Module code | ANA |

Scope beyond the baseline is decided as we build (DEC-012). The defaults below are what the SRS commits to.

## 1. Requirements

- **REQ-ANA-001** Role dashboards. Owner: collection %, defaulters, cash position, attendance %, enrolment trend, staff cost. Admin: tasks needing action, pending approvals, absent students/staff, upcoming exams. Teacher: class attendance %, marks distribution, students needing attention. Parent: child's attendance, results trend, dues, homework. Student: progress and upcoming tests.
- **REQ-ANA-002** Student progress snapshot: strong/weak subjects and trend versus the previous exam/term.
- **REQ-ANA-003** Rule-based flags (transparent, with reasons): "3 absences this month", "result dropped in Maths", fee-risk flags. Staff see flags; parents see supportive guidance only, never risk scores.
- **REQ-ANA-004** Weekly parent digest with 2 to 3 plain suggestions (template-based) via push and WhatsApp.
- **REQ-ANA-005** Topic-level hints appear once teachers tag class-test topics [1.1].
- **REQ-ANA-006** Insight cards carry an action button; a monthly owner summary PDF is generated.
- **REQ-ANA-007** **Event log from day one:** attendance events (time, method), payments (time, channel, delay), marks (subject, component, topic when available), behaviour, app engagement (opens, reads, actions), message delivery, teacher actions. Immutable, raw retention 5 years, consent flags stored.
- **REQ-ANA-008** AI guardrails: no automated decision about a child (admission, discipline, promotion); predictions are advisory with reasons and human review; no public labelling.
- **REQ-ANA-009** Generative AI, if used, is for drafting only (parent summaries, diary drafts), with minimised/pseudonymised data, a per-tenant monthly cost cap and a switchable provider; never used for grading.
- **REQ-ANA-010** Data use: tenant data is owned by the institution; de-identified aggregated data may improve models with an opt-out for institutions; no sale or sharing of identifiable data. Needs legal review.
- **REQ-ANA-011** Later priorities: fee default risk and nudge timing, early warning for chronic absence/dropout, weak-topic detection, face recognition (managed first), forecasting, timetable optimiser. Peer benchmarks are opt-in (minimum cohort 10 institutions) [1.2].
- **REQ-ANA-012** Scheduled reports can be sent to email/WhatsApp.

## 2. User stories

### US-ANA-001 Weekly digest
- Given a student's Maths marks fell 15 points, when the weekly digest runs, then the parent receives plain text such as "Maths dropped; practise fractions" and no risk score.

### US-ANA-002 Teacher attention list
- Given class 5-A, when I open my dashboard, then I see students with low attendance or falling marks and the reason for each.

### US-ANA-003 Owner summary
- Given month end, then the Owner receives a PDF summary of collection, attendance, expenses and the top three actions.

## 3. Business rules
- **BR-ANA-001** Every insight shows why it was produced.
- **BR-ANA-002** Sensitive flags are staff-only.
- **BR-ANA-003** Models activate only after thresholds (for example two terms of data, 300 or more students) and per-tenant calibration.

## 4. Decide in design
Event schema and data dictionary; warehouse choice; consent flags; when to introduce a managed ML platform (1.2).
