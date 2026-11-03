# Module 07. Academics, Exams, Timetable and Diary

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 simplified; 1.1 full (datesheet, syllabus tree, teacher/student apps full) |
| Module code | ACD |

## 1. Purpose
Run the academic cycle: subjects, timetable, exams and results, class tests, homework diary and resources.

## 2. Requirements

### Curriculum
- **REQ-ACD-001** Subjects belong to a class (name, total marks, pass %). Subject groups at class level (for example Science/Arts for Class 9+) with per-student electives (`StudentSubject`); compulsory subjects are auto-assigned.
- **REQ-ACD-002** Subject-teacher assignment per section (see Staff).

### Timetable
- **REQ-ACD-010** One timetable per section per session: weekly grid (periods by days), break slots, shift timings, coverage % tile.
- **REQ-ACD-011** (ours) **Clash warning** when a teacher is double-booked; shift-aware periods; substitutions per day.
- **REQ-ACD-012** Visible to teacher, student and parent apps. Auto-generation is [2.0].

### Exams and results
- **REQ-ACD-020** Model: Term → Exam (type: monthly, mid, final, quarterly, class test, custom) → Exam subject with components (theory, practical, oral/assignment) and marks. Annual weightage per term (should total 100%), configurable per class group. (ref)
- **REQ-ACD-021** Creating a term uses a 3-step wizard: details, classes and sections,  
. Draft exams are auto-created per section with subjects. (ref)
- **REQ-ACD-022** Exam status: `draft`, `scheduled`, `ongoing`, `result_published`, `cancelled`.
- **REQ-ACD-023** Marks entry by the subject teacher in the app or web, or by Excel upload; flags absent/exempt; validated against maximum; edit locked after Submitted; unlock by Admin with a reason.
- **REQ-ACD-024** Result flow: `draft → marks entered → submitted by teacher → reviewed/approved by Admin/Head → published` (immediate or scheduled) → parent notification. The review step is an extension to the reference. Optional policy: hide result when dues exceed X (off by default).
- **REQ-ACD-025** Grade scale is school-wide with a "Load standard scale" opxtion + customizable: A+ ≥ 90, A ≥ 80, B ≥ 71, C ≥ 56, D ≥ 33, F below. Editable. Also percentage, total/obtained, position in class/section, pass rules (overall % and subject-wise), optional grace marks, supplementary exam as a re-exam entry. GPA/division per board later.
- **REQ-ACD-026** Exam dates per subject in 1.0 (simple); full datesheet (date, time, room), roll-number slips in 1.1.
- **REQ-ACD-027** Report cards: 2 or 3 templates (compact, detailed, with graph) with school branding, attendance %, behaviour summary, remarks, principal/teacher comment; bulk PDF per section; parent view in the app.

### Class tests
- **REQ-ACD-030** Teachers create class tests in the app (section, subject, title, total marks); status `scheduled`, `marks_published`, `cancelled`. Simple mode: one total mark. Detailed mode: question-level marks and optional **topic tags** (free,/.0; a syllabus tree in 1.1). Class tests may roll into a "continuous assessment" component.

### Diary and resources (revised v0.2)
Teachers are lecture-based: each teacher is responsible for their own subject in the classes they teach, prepares the diary in advance, and delivers it daily. The system turns those individual entries into **one class diary per day** that goes to parents at the end of the school day.

- **REQ-ACD-040** **Diary planner.** A teacher plans diary entries ahead (for example the next two weeks) per section and subject and date, in the web portal (or the app where allowed). Each entry has text, optional photos/PDFs, optional homework deadline. Helpers: plan several dates at once, copy to the other sections of the same class, copy the previous week. Only the teacher assigned to that section-subject (or a substitute, or an Admin) can edit it.
- **REQ-ACD-041** **Entry status.** `planned → delivered`, or `partly_done`, `moved` (to another date, carries forward) or `skipped` (with reason). The default is "assume delivered": the teacher only changes the exceptions, and the confirmation screen lists today's lectures from the timetable.
- **REQ-ACD-042** **Teacher absence rule.** If the teacher is absent or on leave and no substitute confirms, the entry is not sent automatically; it is held and shown to the Admin as "Not delivered?" to confirm, move or skip.
- **REQ-ACD-043** **Daily class diary.** At a configurable time before school ends (default 30 minutes before the shift end time, shift-aware), the system builds one **class diary** per section for the day: all subjects in timetable order, each with the day's topic, homework and attachments. A subject with no entry is listed as "No diary" and also shown to the Admin.
- **REQ-ACD-044** **Delivery.** The class diary is delivered to the guardians (and the student, if they have an app) of every active student in that section as an in-app diary entry plus push notification. A **picture/PDF version** of the class diary is generated by the server for printing, sharing, and optionally WhatsApp (important-message channel, paid, off by default). Delivery is grouped per child.
- **REQ-ACD-045** **Admin view.** A "Today's diary" page for Admin/Owner shows every section's diary, which teachers have not confirmed, which subjects have no entry, and the send status. The Admin receives a copy or summary at send time.
- **REQ-ACD-046** **Reminders to teachers.** An hour before the send time, teachers with unconfirmed or missing entries get a reminder in the portal (and push where an app is used).
- **REQ-ACD-047** **Preview and hold.** An Admin may preview and hold or send a class diary early. A correction after sending re-sends the card marked "Updated" with the change.
- **REQ-ACD-048** Parents and students see the diary with "seen"; an optional "done" marker for homework and a deadline reminder remain.
- **REQ-ACD-049** No diary is generated on holidays, weekends or days the Admin marks as no-diary (for example exam days).
- **REQ-ACD-041** Resources scoped to institution/class/section/subject (PDF, images, links; 25 MB per file; storage quota per plan); teachers publish with admin visibility; a past-papers category is kept for later.

### Syllabus tree [1.1]
- **REQ-ACD-050** Topic tree per board/class/subject, seeded for popular boards (Punjab Textbook Board, Federal, Cambridge outlines) by us; schools may edit. It is the backbone for AI hints and the later paper maker.

## 3. User stories

### US-ACD-001 Create a term and exams
- Given session 2026-27, when I create Mid-term with classes 5 to 8 and weightage 30%, then draft exams exist for every section with its subjects.

### US-ACD-002 Enter marks
As a **Teacher** I want to enter marks fast.

- Given an exam in `ongoing`, when I enter marks for 5-A Maths and one student is absent, then the total is validated against the maximum, saved offline if needed, and I submit to the Head.
- Given I submitted, then I can no longer edit; an Admin can unlock with a reason.

### US-ACD-003 Publish results
- Given all subjects are approved, when the Admin publishes, then results are visible to parents, notifications are sent and report cards can be generated in bulk.

### US-ACD-004 Report card
- Given published results, when I generate cards for 5-A, then one PDF per student (or a combined PDF) is created asynchronously with grade, position, attendance % and comments.

### US-ACD-005 Plan the diary two weeks ahead
As a **Teacher** I want to prepare my subject's diary for the next two weeks for each of my classes.

- Given I teach Maths in 5-A and 5-B, when I enter Monday's topic and homework and choose "copy to 5-B", then both sections have a planned entry for Monday.
- Given a planned entry for tomorrow, then I can still edit it until the class diary is sent.

### US-ACD-007 Daily class diary to parents
As a **Parent** I want to see what was taught today in all subjects.

- Given the shift ends at 13:00 and the send time is 12:30, when the time arrives, then one class diary for 5-A with all subjects (in timetable order) is sent to every guardian of 5-A students, as an app entry and push, with a downloadable picture/PDF.
- Given Urdu is typed in an entry, then it displays correctly in the card.

### US-ACD-008 Teacher confirms the day
- Given I taught 3 lectures, when I open "Today" at 12:00, then I see 3 planned entries pre-set to delivered; I mark one as "moved to Thursday" and the other two stay delivered.

### US-ACD-009 Absent teacher
- Given Mr. Ali is on leave and no substitute exists, then his entries are held, the Admin sees "Not delivered?", and the class diary shows that subject as "No diary" unless the Admin confirms or moves it.

### US-ACD-010 Admin checks coverage
- Given 20 sections, when the Admin opens "Today's diary" at 12:15, then they see which sections are complete, which subjects are missing and which teachers have not confirmed.

### US-ACD-006 Timetable clash
- Given Mr. Ali teaches 5-A period 2 on Monday, when I put him in 6-B period 2 on Monday, then the system warns of a clash and requires confirmation.

## 4. Business rules
- **BR-ACD-001** Marks cannot exceed the maximum.
- **BR-ACD-002** Published results are changed only by an audited correction with a reason and re-notification.
- **BR-ACD-003** Annual weightage across terms should total 100% and the UI warns otherwise.
- **BR-ACD-004** The result hold on dues is a school policy (off by default), never automatic.
- **BR-ACD-005** A diary entry belongs to exactly one section, subject and date, and to the teacher assigned to that section-subject.
- **BR-ACD-006** The class diary is sent once per section per day; later corrections are re-sent as "Updated".
- **BR-ACD-007** Entries from an absent teacher with no substitute are never auto-sent as delivered.

## 5. API (representative)
`/subjects`, `/timetable`, `/exam-terms`, `/exams`, `PUT /exams/{id}/marks`, `POST /exams/{id}/submit`, `POST /exams/{id}/publish`, `POST /report-cards/generate`, `/class-tests`, `/diary-entries`, `POST /diary-entries/bulk`, `POST /diary-entries/{id}/status`, `GET /class-diaries?section=&date=`, `POST /class-diaries/{id}/send`, `/resources`.

## 6. Events
`EVT-ACD-marks.submitted`, `result.published`, `classtest.published`, `diary.entry_status_changed`, `class_diary.sent`.

## 7. Decide in design
Report-card template designs; marks-entry offline conflict handling; syllabus tree seeding sources and licensing.
