# ADR-0023: Gate scan and student ID cards

| | |
|---|---|
| Status | Accepted (choices made by the product owner on 2026-11-17) |
| Date | 2026-11-17 |
| Spec links | REQ-ATT-002, REQ-ATT-011, REQ-ATT-014, REQ-ATT-017, BR-ATT-006, US-ATT-003, DEC-033, DEC-034, ADR-0022 |

## Context
Students pass the reception desk and show their ID card to the computer's webcam. The browser reads the QR; the API must decide fast whether the code is real, whose it is, and whether it counts. The class register must stay the official record.

## Decision
1. **A card has a signed code.** `student_cards` holds one row per card (one active per student, enforced by a partial unique index). The QR holds `<card id as 32 hex digits>.<10-character keyed checksum>`. The checksum (16 base32 characters, 80 bits) is an HMAC-SHA-256 of the school and the card id, keyed with a card key derived from the server's secret (`AUTH_SECRET`, never used raw), so a code made for one school or one card verifies for no other, only the server can make one, and a garbled or forged scan is refused before any lookup (`attendance.card_invalid`). The code is the same every time, so an owner can reprint a card; nothing secret is stored. Treat a code as a credential: disabling a card stops it at once (`attendance.card_disabled`), and issuing a new card disables the old one in the same step.
2. **Cards can only be disabled, never edited or deleted** (database trigger), with a reason. Issuing, listing and disabling need `student.edit` (Owner for now).
3. **A scan is a log row, not a register change.** `gate_scans` is append-only: student, card, the section they were in, the day, the scan time, on time or late, and who scanned (BR-ATT-006: the signed-in user, and also an audit entry). The class register is not modified; its records show the student's first scan of the day ("arrived 07:52"), and `GET /attendance/mismatches` lists scanned-but-absent and present-but-not-scanned students for review (REQ-ATT-017), from submitted registers only, and "not scanned" only for sections that had a scan that day.
4. **What a scan checks, in order:** the signature; the card exists in this school and is active; the day is a school day; the student is enrolled in a section that day (the roster rule of ADR-0022); then a 2-minute duplicate window (a repeat inside it, in either direction, is `outcome: duplicate` and stores nothing). A per-student-per-day advisory lock makes simultaneous taps record one arrival.
5. **Late is per shift.** Attendance settings gain `morning_start`, `evening_start` (defaults 08:00 and 13:00) and `late_after_minutes` (10): a scan after start plus grace is late. The section's shift picks the start.
6. **Offline computers.** A scan may carry its own time (within the last 24 hours, not in the future beyond 2 minutes of clock skew) and a `client_scan_id`. The same id again, even at the same moment from several uploads, returns the first answer and stores nothing (a unique index is the backstop).
7. **Permission:** `attendance.scan` (Owner, Receptionist). Gate scans and mismatches are read with `attendance.view`.

## Found in review
- **Replays answer from the stored scan,** for the day that scan was made (so a retry after midnight, or with no `scanned_at`, still gets the first answer), and a `client_scan_id` reused for a different card is refused (`attendance.client_scan_id_reused`), including when two screens race with one id.
- **Offline gaps:** the age limit on `scanned_at` is the school's correction window (default 7 days), not 24 hours, so a computer offline over a weekend can still upload.
- **Key separation:** the signing key is derived from `AUTH_SECRET` (`HMAC(secret, "edu-plat/student-card-key/v1")`); rotating `AUTH_SECRET` still invalidates every printed card, which has to be planned before a production rotation (no key id or overlap yet).
- **Disable is immediate:** a scan reads the card `FOR SHARE`, so a disable in flight makes the scan wait and then see the card disabled; deterministic test.
- **Refusals are audited:** a genuine card refused as disabled, not enrolled or on a non-school day is written to the audit log with the user and the reason; a forged or unknown code is logged only, so probing cannot fill the audit table.
- **Codes are case-insensitive** (a scanner may change case); whitespace is still refused.
- **Mismatch list after a transfer:** a section counts as "using the gate" only if a student still counted in its register was scanned in it, so a student who was scanned and then moved does not make their old section's other students look unscanned.
- **Settings stay compatible:** the three shift fields are optional on a PUT and keep their saved values when left out.
- **Issuing a card** honours the Idempotency-Key, so a double click makes one card.
- Known and accepted: a scan's lateness ignores seconds (08:10:59 is on time with a 10 minute grace); a shift start plus grace past midnight can never be late; two scans 30 seconds apart across 00:00 are both stored; anyone with `student.edit` can read an active card's code and so could forge an arrival (the receptionist's photo check is the control).

## Not in this slice
Reading the QR (the browser app does it), the printable card, a photo on the gate screen (students have no photo field yet), face recognition (1.1), staff cards and check-in, parent alerts, and an approved-leave pre-fill.

## Consequences
- A shared or test scan can never change official attendance; a person decides what a mismatch means.
- Reprinting needs no stored secret, but anyone with the code can pass at the gate until the card is disabled; scan-time photo matching by the receptionist is the control (spec §3.1).
- Rotating `AUTH_SECRET` invalidates every printed card. That is deliberate (it is the same secret that signs OTP codes) and must be planned before a production rotation.
- A scan is rejected if the student has no enrolment that day, so a student enrolled the same morning is scannable from that day.
