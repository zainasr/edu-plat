# 02 Timetable
Route: /admin/timetables, /admin/timetables/create. Screenshots: screenshots/02-timetable/.

- One timetable per section per session (13 timetables for 13 sections, 244 period entries). Tiles: timetables, sections, coverage % (sections that have a timetable), entries (period slots).
- List: search, class and section filters, table (section, class, entry count, view/edit/delete), Download CSV. Buttons: **Add break** (defines break slots) and **Create Timetable**.
- Create: choose a section, then a weekly grid ("build a weekly schedule using the grid") of periods x days; each cell presumably subject + teacher.
- Entries counts differ widely (6 vs 46) so period structure is per section, not school-wide; demo data looks inconsistent.
- Implied: Timetable(section_id, session_id), TimetableEntry(timetable_id, day, period_no, start, end, subject_id, teacher_id, is_break).
- Gaps/ideas: no shift or branch awareness, no room/resource clash detection visible, no teacher-conflict view or auto-generation. Auto-generate timetable from subject periods per week and teacher availability would be a strong differentiator; teachers/students/parents see "today's schedule" in their apps.
