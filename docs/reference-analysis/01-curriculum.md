# 01 Curriculum (Sessions, Classes, Sections, Subjects)

Routes: /admin/sessions, /admin/classes, /admin/sections, /admin/subjects (+ /create for each). Screenshots: screenshots/01-curriculum/.

## Purpose
Defines the academic structure all other modules hang off. Everything is scoped to the selected session (top-bar selector).

## Screens
- **Sessions**: stat tiles (sessions, classes, sections, students), search, table (name + "Current" badge + created date, description, counts of classes/sections/students, edit/delete icons), CSV export, page-size selector. Form fields: Session Name*, Description. Description is free text (used here to describe differing primary/secondary year durations), so there are no structured start/end dates.
- **Classes**: tiles (classes, sections, subjects, students); filter by level tabs/dropdown (Early Years, Primary, Middle, Secondary, Higher Secondary; form also has O & A Levels); table: class name + code, level badge, "Grade n" numeric, session, counts of sections/subjects/students. Form: Class Name*, Code/Short Name*, Education Level*, Grade/Numeric Level*, Academic Session*, Standard Fee (Monthly)*, Max Students (optional), Subject Stream (optional).
- **Sections**: tiles (sections, classes with sections, students, avg per section); filter by class; table: section, class, description, students (x / capacity). Form: Select Class*, Section Names* (multi-chip entry, create A,B,C at once), Description.
- **Subjects**: tiles (subjects, classes covered, avg total marks, avg passing %); filter by class; table: subject, class, description, total marks, passing %. Form: Subject Name*, Select Classes* (multi-select, add one subject to many classes at once), Total Marks*, Passing Percentage*, Description.

## Observations
- Subject is per class (class-subject), with marks and pass% attached to it. 119 subject rows for 13 classes. No teacher assignment visible here (likely in timetable).
- Standard monthly fee lives on the class, the base for fee generation.
- Section descriptions are free text (demo data holds generic boilerplate), a weak field.
- Rows have edit/delete icons; delete presumably has confirmation (not triggered).
- No branch/campus or shift concept is visible at this level (a gap vs. our requirement).

## Entities implied
Session(id, name, description, is_current); Class(id, session_id, name, code, level, grade_no, standard_fee, max_students, stream); Section(id, class_id, name, description, capacity?); Subject(id, class_id, name, total_marks, pass_pct, description).

## Ideas
- Branch and shift as first-class parents of class/section; structured session dates and terms; subject-teacher mapping; class templates by Pakistani board (Matric, FSc, Cambridge); curriculum/syllabus topic tree that later powers the paper maker and AI weak-topic analysis.
