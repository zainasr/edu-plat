# Module 03. Attendance

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 (face recognition pilot 1.1; period-wise 1.1) |
| Revised | v0.3: school-controlled channels (phones are banned in many schools); gate scan runs in the normal web portal on the reception computer, no extra device |
| Module code | ATT |

## 1. Purpose
Fast, trustworthy daily attendance for students and staff, with offline entry, parent alerts and clean registers.

## 2. Statuses and granularity
- Student statuses: `present`, `absent`, `late`, `leave`, `holiday` (auto). Staff also `half_day`.
- **REQ-ATT-001** Daily: one register per section per day in 1.0. The register has an optional `period` field so period-wise attendance for secondary classes can be added in 1.1.
- **REQ-ATT-002** Late and absent cutoffs are configurable per shift (for example late after start + 10 min).
- **REQ-ATT-003** A teacher can edit the same day; an admin within N days (default 7) with a reason; older edits need elevated permission. Every edit stores old value, new value, user, time and reason, visible in the student's attendance history and the audit log.

## 3. Marking methods (1.0)
- **REQ-ATT-010** Manual: teacher in the app (one tap per student, "mark all present" shortcut), admin/receptionist on the web.
- **REQ-ATT-011** **Web scan page at reception.** A page in the normal web portal (`/attendance/scan`) opens the computer's webcam. The reception computer, which is already open all day, is used: as students arrive they pass the desk and show their ID card in front of the webcam; the page reads the QR code and marks the student present. No phone and no special app are needed. Details in section 3.1.
- **REQ-ATT-014** **Channels are school-controlled.** Many schools forbid teacher phones. Attendance is one record that can be created from several channels, enabled per institution: (1) web scan page at reception, (2) class register on a school-owned computer or tablet (browser), (3) attendance desk entry by a clerk from teachers' paper slips, (4) teacher mobile app (optional, only where allowed). A strict school can run on channels 1 to 3 with no phones at all.
- **REQ-ATT-015** **Absentees-only register.** The class register opens with every student pre-set to present (or pre-filled from gate scans, approved leaves and holidays). The user changes only the exceptions, then submits. The bulk of the work is a few taps per class.
- **REQ-ATT-016** **Register submission.** A section's register has states `open → submitted`. Absent alerts to parents are sent only for submitted registers (see REQ-ATT-020). A register not submitted by a configurable time (default 10:00) is escalated to the Admin as "Register missing" and never silently treated as all-absent.
- **REQ-ATT-017** **Gate vs class mismatch.** When both the gate scan and the class register exist, a student scanned in at the gate but marked absent in class (or the reverse) is flagged on a mismatch list for the Admin.
- **REQ-ATT-012** Face recognition [1.1 pilot]: simple, the same reception scan page with a webcam; liveness required; no raw image retention by default; consent per student; templates encrypted and deletable.
- **REQ-ATT-013** Offline: marks and scans queue on the device with timestamps and sync idempotently. Conflict priority: scan > teacher mark > admin edit within window; all versions kept in history. A visible "synced / pending" indicator is shown.

### 3.1 How the reception scan page works
1. **No extra gadget or set-up.** The scan page is just another page of the same web portal, opened on the reception computer's browser and its built-in or plug-in webcam. The staff member at reception signs in as usual; anyone with the `attendance.scan` permission (for example a Receptionist) can open it. It can stay open all day in its own browser tab while the person keeps doing other portal work (fee collection, enquiries) in another tab.
2. **Scanning.** The page uses the browser's camera (HTTPS required; the browser asks for camera permission once). QR decoding happens in the browser, so no video leaves the computer. Each scan sends the card code to the API (`POST /attendance/scan`) using the signed-in user's session.
3. **Feedback in under a second.** The screen shows the student's photo, name, class and a large green "Present" (or amber "Late", red "Card disabled / unknown"), with a beep. The receptionist can visually match the photo to the child.
4. **Duplicate scans** within 2 minutes are ignored with a message.
5. **Weak internet.** The page is a progressive web app: it caches the list of valid cards, checks the card's signature locally, queues scans with timestamps and syncs them when the connection returns. A badge shows "n scans waiting to sync".
6. **Staff cards** can use the same scanner, which gives staff attendance without any phone (see section 5).
7. **Security.** The card code is signed (checksum), cards can be disabled when lost, and the photo on screen reduces card-sharing. Face recognition on the same page is the 1.1 upgrade.
8. **Result.** A scan marks the student present or late for that day in their section's register and the register shows "Arrived 07:52 (gate)". Students not scanned by the late cutoff stay "not arrived" until the register is submitted by a teacher or the attendance desk, who decide absent, leave, or arrived without scanning.

## 4. Notifications
- **REQ-ATT-020** Absent alerts after the late cutoff and only for **submitted** registers, batched at a configurable time (default 09:30); late alerts; optional check-in/out alert for scans. Channel order push → WhatsApp (important) → SMS fallback. Parents can mute low-priority alerts but not the absent alert.

## 5. Staff attendance
- **REQ-ATT-030** Staff check in through the app with a geo-fence (radius around the campus; verified on the device; coarse location stored). Per-shift rules: start time, grace, absent cutoff, half-day threshold. Overtime is deferred.
- **REQ-ATT-031** Monthly summary feeds payroll (late and absent deductions per policy).
- **REQ-ATT-032** Approved leaves auto-create `leave` status for covered days; holidays auto-mark; the marking screen is pre-filled.

## 6. Reports (1.0)
Daily summary, class register (monthly, printable), low-attendance list (default below 75%, configurable), staff punctuality, absent-today call list. The eligibility threshold is shown on report cards; there is no automatic blocking.

## 7. User stories

### US-ATT-001 Mark attendance in under a minute
As a **Teacher** I want to mark my class on my phone offline.

- Given no internet, when I open 5-A and tap "all present" then change 2 students to absent, then the register saves locally in under 1 second and shows "pending sync".
- Given the network returns, then the register syncs and shows "synced" without duplicates.

### US-ATT-002 Parent gets an absent alert
- Given a student is marked absent, when the alert time arrives, then each guardian receives one notification; marking the student present later retracts the pending alert if not yet sent.

### US-ATT-003 Gate scan on the web portal
As a **Receptionist** I want students to show their card in front of the reception computer's webcam so that attendance is marked without anyone using a phone.

- Given the scan page is open on the reception computer and the camera is allowed, when a valid card is shown, then within 1 second the screen shows the student's photo, name and class with "Present" and a beep; the student is marked present with the scan time and method `qr`.
- Given a second scan of the same card within 2 minutes, then it is ignored with "Already scanned".
- Given a disabled or unknown card, then the screen shows a red warning and nothing is marked.
- Given no internet, then scans are stored on the computer, shown as "waiting to sync", and uploaded later without duplicates.

### US-ATT-006 Absentees-only register
As a **Teacher** I want to mark only the absent students on a school computer.

- Given 40 students where 35 were scanned at the gate, when I open the register, then those 35 are already present and I only decide the other 5 (absent, leave, or arrived without scan) and press Submit.

### US-ATT-007 Attendance desk
As a **clerk** I want to enter all sections' absentees from the paper slips teachers send.

- Given slips from 12 sections, when I open the attendance desk screen, then I see all 12 registers on one page and can submit each in seconds.

### US-ATT-008 Missing register
- Given section 6-B has no submitted register at 10:00, then the Admin sees "Register missing: 6-B" and no absent alerts are sent for 6-B.

### US-ATT-004 Staff check-in
- Given I am within the geo-fence at 07:55 and the shift starts at 08:00, when I tap check-in, then I am `present`; at 08:20 with 10-minute grace I am `late`.

### US-ATT-005 Admin correction
- Given a record from 3 days ago, when an admin changes absent to present with a reason, then history shows old/new/reason/user and the pending alert, if any, is not resent.

## 8. Business rules
- **BR-ATT-001** No attendance on holidays or for non-active students.
- **BR-ATT-002** All edits are versioned; nothing is overwritten silently.
- **BR-ATT-003** Gate pass, early-leave and pickup lists are NOT built (not common in Pakistan).
- **BR-ATT-004** Face images are not retained after template creation unless the school opts in.
- **BR-ATT-005** Absent alerts are sent only from submitted registers; a missing register is never treated as all-absent.
- **BR-ATT-006** Scanning needs the `attendance.scan` permission; every scan is audited with the signed-in user.
- **BR-ATT-007** Students do not mark attendance for each other; a class representative, if used, only helps by preparing the paper slip.

## 9. API (representative)
`GET /attendance/registers?section=&date=`, `PUT /attendance/registers/{id}/records`, `POST /attendance/scan`, `POST /staff-attendance/check-in`, `GET /reports/attendance-summary`.

## 10. Events
`EVT-ATT-record.marked`, `record.edited`, `scan.accepted`, `student.absent_alert_due`.

## 11. Decide in design
Offline sync library and conflict implementation; face recognition vendor vs self-hosted [1.1].
