# Module 08. Behaviour

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 (counselling workflow later) |
| Module code | BEH |

## 1. Purpose
Record positive and negative behaviour with controls on what parents see. The reference model is kept entirely.

## 2. Requirements

- **REQ-BEH-001** Categories are positive/negative/note with a name, severity and points; schools can edit.
- **REQ-BEH-002** Entries: student, category, author (staff), text, date, status; positive entries add merit points (appreciation points).
- **REQ-BEH-003** Concern lifecycle: `waiting_review → open → in_followup → resolved`.
- **REQ-BEH-004** Parent-sharing controls: a review-before-parents threshold (Never / Minor+ / Moderate+ / Serious+ / Critical); only reviewed entries are shared; parents can reply in a thread. (ref)
- **REQ-BEH-005** On-watch alert: a student with 3 or more concerns in 30 days is flagged to staff (never publicly).
- **REQ-BEH-006** Appreciation/behaviour sharing with parents is part of 1.0 (positive notes and points).
- **REQ-BEH-007** Staff-only visibility: negative entries and risk flags are visible to staff only; parents see supportive guidance, not scores.
- **REQ-BEH-008** Analytics: behaviour trend per student/class feeds the improvement engine.

## 3. User stories

### US-BEH-001 Log and share appreciation
- Given I am a teacher, when I log "Helped a classmate" (positive, 5 points) and sharing is on, then the guardian receives a notification and the student's points increase.

### US-BEH-002 Concern with review
- Given the threshold is "Moderate+", when I log a Moderate concern, then it is `waiting_review` and not visible to parents until a reviewer approves.

### US-BEH-003 Watch list
- Given a student gets a third concern within 30 days, then an on-watch alert is shown to authorised staff only.

## 4. Business rules
- **BR-BEH-001** No automated disciplinary decision (promotion, strike-off) is made from behaviour data.
- **BR-BEH-002** Children are never labelled publicly.

## 5. API (representative)
`/behaviour/categories`, `/behaviour/entries`, `POST /behaviour/entries/{id}/review`, `/behaviour/entries/{id}/replies`.

## 6. Events
`EVT-BEH-entry.created`, `entry.shared`, `student.on_watch`.
