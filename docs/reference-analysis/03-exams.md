# 03 Exams (Terms, All exams, Results, Grading scale)
Routes: /admin/terms, /admin/terms/create, /admin/exams, /admin/results, /admin/grading-policies. Screenshots: screenshots/03-exams/.

## Flow
1. **Create exam term** (3-step wizard: Term details, Classes & sections, Review). Fields: term name*, academic session*, exam type (Mid Term, Final, Quarterly, Monthly, Class Test, Other), annual weightage % (published terms in a session should sum to 100). Step 2 picks classes/sections; on confirm the system **auto-generates one draft exam per selected section, pre-filled with that class's subjects** (marks from subject definition).
2. **Exams list**: tiles (exams, drafts awaiting publish, in progress, results published), filters by term and status, search, CSV; checkboxes for bulk action; row shows exam, term, type, number of subjects, marks (entered/total), status, actions.
   Status lifecycle: Draft -> Scheduled -> Ongoing -> Result published (and Cancelled).
3. **Marks entry** then **publish results** (done from Exams page, per message on Results page). 
4. **Results**: published exam result sheets, viewable/downloadable, filter by term; empty state explains how to publish.
5. **Grading scale**: school-wide percentage-to-letter mapping. Empty state offers "Load standard scale" (A+ >=90, A >=80, B >=71, C >=56, D >=33, F >=0) or "Add manually". Without it, results have no letter grades.
- Term list: name + exam count, type, weightage, classes count, sections count, delete.
- Implied: ExamTerm(session_id, name, type, weightage), Exam(term_id, section_id, status, total_marks), ExamSubject(exam_id, subject_id, marks, date?), ExamMark(exam_subject_id, student_id, obtained, absent?), GradeScale(school_id, min_pct, grade).
- Gaps/ideas: no datesheet/roll-number slip visible at this level, no paper/question bank, no analysis per topic. Our moat: link marks to syllabus topics for AI weakness detection, trend per student, parent-facing progress with suggestions, auto-rank/position and report-card PDF in local formats.
