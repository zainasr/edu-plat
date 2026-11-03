# 05 Events & Holidays
Route: /admin/administration/events-holidays. Screenshots: screenshots/05-events/.
- School-wide calendar entries, two types: Holiday and Event. Buttons: Add holiday, Add event. Tiles: upcoming holidays, upcoming events, total, past. Toggle Upcoming/Past; two lists with empty states and "Add one".
- Holidays likely suppress attendance and fee-due logic (to verify); events likely notify parents.
- Implied: CalendarEntry(school_id, type, title, start_date, end_date, audience, description).
- Ideas: per-branch/class audience, import Pakistani national/religious holiday defaults (moon-sighting dates adjustable), sync to parent app calendar, auto reminders.
