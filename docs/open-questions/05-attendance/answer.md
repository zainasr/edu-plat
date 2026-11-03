# 05 Answers

1: [accepted] 1.0: manual by teacher (app) and by admin/reception (web), plus QR digital-ID scan on a staff/gate device (tablet or phone). Face recognition = 1.1 pilot (single entry point kiosk). Scanning devices: a "Kiosk mode" in the teacher/staff app on a tablet/phone at the gate or classroom.
2: [accepted] Statuses: Present, Absent, Late, Leave, Holiday (auto), Half-day (staff only). Late/Absent cutoffs configurable per shift (e.g. late after start+10 min). Teacher can edit same day; admin can edit within N days (default 7) with reason; older edits need Owner/Admin permission.
3: [accepted] Daily (one register per section per day) in 1.0; period-wise as an optional setting for secondary classes in 1.1 (keeps data model ready: register has optional period).
4: [no need to make such complex system just simple face recognition is enough] simple face recognition is enough.
5: [accepted simple card contain scanable qr] Student app shows rotating QR (refreshes every 30-60 s) tied to the student and device; printed ID QR is static but with a checksum and can be disabled if lost. Duplicate scan within 2 minutes is ignored.
6: [accepted] Offline queue on device with timestamped events; sync idempotently. Conflicts: latest verified source wins by priority (scan > teacher mark > admin edit within window), all versions kept in history; teacher sees a "synced / pending" indicator.
7: [accepted] Notifications on Absent (after the late cutoff, batched at a configurable time, e.g. 09:30) and on Late; optional check-in/out alert for scans. Channel order: push -> WhatsApp -> SMS (cost-aware, per institution setting). Parents can mute low-priority alerts but not absent alerts.
8: [no need of this it is not common in paksitan] 1.1: gate check-out and early-leave with reason and authorised pickup list (names + mobile + photo); parent gets alert. Not in 1.0.
9: [accepted] Staff: app check-in with geo-fence (as reference). Per-shift rules (start, grace, absent cutoff, half-day threshold); overtime deferred; monthly attendance summary feeds payroll (late and absent deductions per policy).
10: [accepted] Approved leaves auto-create Leave status for covered days; holidays auto-mark; teacher marking screen shows pre-filled statuses.
11: [accepted] 1.0 reports: daily summary, class register (monthly, printable), low-attendance list (< threshold), staff punctuality, absent-today call list. Eligibility threshold configurable (default 75%) and shown on report cards (no automatic blocking).
12: [accepted] Every edit stores old/new value, user, time, reason; visible in student attendance history and audit log.
13: [accepted] Later (Phase 3) via a device-integration layer; not in 1.0/1.1.
14: [accepted] Capture: event time, method, device id, branch, optional coarse location for staff, verifier. Face images are not retained after template creation unless the school opts in; templates encrypted and deletable on withdrawal or consent withdrawal.
