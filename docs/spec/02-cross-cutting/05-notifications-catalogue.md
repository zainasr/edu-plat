# 05. Notifications Catalogue

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | NTF |

## 1. Channel policy (DEC-009)

| Channel | Use | Cost |
|---|---|---|
| In-app notification centre + push (FCM) | Default for everything | Free |
| WhatsApp (official API via a BSP, approved templates) | Important reminders only (absence, fee due/overdue, emergency, receipt) | Paid per template message outside the 24-hour reply window; replies to a parent's own message within 24 hours are free. Exact rates to be checked with Meta and the BSP |
| SMS (one provider, registered sender) | OTP fallback and rare critical fallback | Paid per SMS |
| Email | Staff/owner only: invoices, password reset fallback, report digests. Parents do not need email | Low |

- **REQ-NTF-001** Channel order per event: push first; WhatsApp for events marked "important"; SMS only if configured as fallback and push/WhatsApp failed.
- **REQ-NTF-002** Per-institution usage and cost report; message credits can be capped (hard limit) with warnings.

## 2. Event catalogue (1.0)

| Event | Audience | Important (WhatsApp)? | Notes |
|---|---|---|---|
| Student absent | Guardians | Yes | Sent after the late cutoff, batched at a configurable time (default 09:30) |
| Student late | Guardians | No | Optional check-in/out alert for scans |
| Fee challan released | Guardians | Yes | Includes amount, due date, consumer number |
| Fee due soon (3 days before) | Guardians | Yes | Reminder schedule configurable |
| Fee due today / overdue (+3, +7, +15) | Guardians | Yes | Escalation to owner list at +30 |
| Payment received / receipt | Guardians | Yes | Includes receipt number |
| Result published | Guardians, students | No | |
| Daily class diary | Guardians, students | Optional (picture/PDF, paid, off by default) | One per section per day near school end |
| Register missing | Admin | No | Section register not submitted by cutoff |
| Diary not confirmed / missing | Teacher, Admin | No | Reminder before send time |
| Announcement | Targeted audience | No (Yes if emergency) | Emergency sent on all channels |
| Leave decision | Applicant | No | |
| Behaviour entry (if sharing on) | Guardians | No | After review threshold |
| Exam datesheet | Guardians, students | No | |
| Birthday (optional) | Student, guardians | No | |
| Emergency | Everyone in scope | Yes | Admin/Owner only |
| Staff: salary approved/paid, leave decision, timetable change | Staff | No | |
| Enquiry follow-up due | Staff owner | No | |
| New device login | The user | Yes | |

## 3. Templates

- **REQ-NTF-010** System templates are maintained by us in English with Urdu/Roman-Urdu variants and variables (`{student}`, `{amount}`).
- **REQ-NTF-011** Schools can edit the wording of non-WhatsApp templates within safe limits. WhatsApp templates are a fixed set approved centrally with the BSP.
- **REQ-NTF-012** Content fields accept Urdu Unicode.

## 4. Rules and scheduling

- **REQ-NTF-020** 1.0 uses a fixed catalogue with per-event toggles, timing, channel selection and audience. A general rules builder is [2.0].
- **REQ-NTF-021** Audience targeting: all, branch, class, section, individual/family, staff group, saved lists (defaulters, low attendance).
- **REQ-NTF-022** Scheduling ("send later") in 1.0; recurrence only for system reminders.
- **REQ-NTF-023** Holidays suppress attendance and fee-due reminders.

## 5. Preferences and consent

- **REQ-NTF-030** Parents choose per category (fees, academics, attendance, general), channel and quiet hours.
- **REQ-NTF-031** Mandatory (cannot be disabled): absent alert, overdue fee notice, emergency.
- **REQ-NTF-032** Parent consent for WhatsApp/SMS is captured at admission or first login; opt-out is honoured; per-tenant rate limits; content moderation list for school-authored messages; audit of who sent what.
- **REQ-NTF-033** An app-level setting lets users stop receiving non-mandatory notifications.

## 6. Delivery tracking and reliability

- **REQ-NTF-040** Each message has a status: `queued`, `sent`, `delivered`, `read`, `failed`.
- **REQ-NTF-041** Automatic fallback after failure per channel order; retry with exponential backoff; a dead-letter list visible to support.
- **REQ-NTF-042** In-app notification centre in all apps with deep links and unread badges; the web admin has a bell and an activity feed. Notifications are grouped per child.

## 7. User stories

### US-NTF-001 Absent alert
As a **Parent** I want to be told if my child is absent so that I can respond.

- Given a student is unmarked-present after the cutoff, when the batch time arrives, then a push notification is sent; if WhatsApp is enabled for important events, a WhatsApp template is also sent.
- Given I muted general notifications, then I still receive the absent alert.

### US-NTF-002 Fee reminders
As an **Accountant** I want reminders to follow a schedule so that I do not chase parents by hand.

- Given a challan due on the 10th, when the 7th arrives, then guardians receive a reminder; after payment no further reminders are sent.

### US-NTF-003 Emergency
As an **Owner** I want to send an emergency message on all channels so that every parent is reached.

- Given I am Owner/Admin, when I send an emergency announcement, then it is sent via push, WhatsApp and (fallback) SMS, and delivery status is visible.

## 8. Decide in design

- BSP and SMS provider selection; template catalogue approval timeline.
- Notification service split from the monolith only if volume requires.
