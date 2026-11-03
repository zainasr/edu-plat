# 05 Attendance

1. (BLOCKER) Methods in 1.0: manual by teacher in app, manual by admin/reception on web, QR/digital ID scan, face recognition. Which are in 1.0 and which later? Where does scanning happen (gate kiosk, classroom device, teacher phone)?
2. Statuses and rules: Present, Absent, Late, Leave, Half-day, Holiday. Cutoff times for Late/Absent per shift? Who may change status after the day closes?
3. Granularity: daily (once per section) or period-wise? Is daily enough for 1.0?
4. Face recognition: on-device vs server, what hardware (phone camera, tablet kiosk, CCTV-style cameras)? Accuracy and liveness expectations? Fallback when face fails? Minimum viable version?
5. Digital ID card scan: QR in student app shown to a camera vs printed card. Anti-sharing/screenshot protection (rotating QR)?
6. Offline: teacher marks attendance without internet and syncs later. Conflict rules if two people mark the same register?
7. Parent notifications: absent alert, late alert, check-in/check-out alert. Timing (immediately, after cutoff), channel (push, WhatsApp, SMS), parent opt-out?
8. Check-out/gate pass and early leave: needed? Who picks up the child (authorised pickup list)?
9. Staff attendance: app check-in with geo-fence (reference), plus face/QR kiosk at gate? Shift-aware rules, grace minutes, half-day, overtime, link to payroll deductions?
10. Leave interplay: approved student leave -> auto "Leave"; staff leave -> auto status. Holiday auto-handling.
11. Reports: daily summary, monthly register, defaulters (low attendance %), chronic absence, staff punctuality. Attendance % thresholds for exam eligibility?
12. Correction/audit: edit window, reason mandatory, history visible?
13. Biometric hardware (fingerprint devices) support: yes/no/later?
14. Data to capture for AI: timestamps, method, device, location; any privacy limits?
