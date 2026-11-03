# 12 Attendance
Routes: /admin/attendance/classes, /admin/attendance/students, /admin/attendance/staff, /admin/attendance/staff/settings. Screenshots: screenshots/12-attendance/.

## Class attendance (register view)
Date picker + search; status chips All / Not marked / Partly marked / Marked; tiles: registers marked (0/13), students marked, present (late), absent (on leave); a card per section: "Not marked", "x of N marked", counters P/A/L/Lv (Present, Absent, Late, Leave). Download report. Marking itself is done per register (in teacher app or admin); statuses: Present, Absent, Late, Leave.

## Student attendance
Per-day list with tiles marked/present/absent/on-leave; search by name, ID, father, mobile, class; filters class, section, status. Admin can mark/edit by hand (on the day's list).

## Staff attendance
"Daily app check-ins plus manual marking": tiles checked-in, present (late), absent, on leave; filters type and status (Present, Late, Absent, Leave, Not checked in); columns check-in time (GMT+5), status, method (app / manual), remarks, edit and history actions; CSV.

## Staff attendance settings
- Per working day (Mon-Sat): start time, grace minutes, absent cutoff (e.g. 07:00 / 10 min / 09:00) with copy-to-other-days. Days off come from school settings.
- Allow manual override by admins.
- **Geo-fencing**: enforce location-based check-in from the app within a radius (>=30 m) of a school pin on a map (lat/long/radius).

## Observations / gaps
- Dashboard activity shows "Marked attendance" entries so teachers mark via mobile app.
- No face recognition, QR/ID-card scan, biometrics or offline marking seen. No parent notification settings (absent SMS/push) visible here. No period-wise attendance. No late-arrival policy for students.
- Ideas: face/QR scan kiosk with offline queue, auto parent notification, absentee follow-up workflow, attendance-pattern AI alerts (chronic absence, Monday/Friday patterns), shift/branch aware rules.
- Entities: AttendanceRegister(section_id, date), StudentAttendance(student_id, date, status, marked_by, method), StaffAttendance(staff_id, date, check_in, status, method, remarks), StaffAttendanceRule(day, start, grace, cutoff), GeoFence(lat, lng, radius).
