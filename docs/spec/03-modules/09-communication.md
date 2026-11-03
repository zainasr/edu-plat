# Module 09. Communication Hub

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | COM |

Channels, templates, preferences and the event catalogue are defined in `02-cross-cutting/05-notifications-catalogue.md`. This module covers announcements, chat, forms and the support channel.

## 1. Requirements

### Announcements
- **REQ-COM-001** Announcements have a title, text, optional image/PDF, target audience, pin option, optional "acknowledge", and read counts. (ref: audience Everyone / Teachers / All staff / Parents / Specific people; split between Admin announcements and auto-generated system notifications.)
- **REQ-COM-002** (ours) Class/section targeting, scheduling (send later) and attachments, which the reference lacks.
- **REQ-COM-003** Emergency flag (Admin/Owner only) sends on all channels.
- **REQ-COM-004** The audience can be all, branch, class, section, individual/family, staff group, or a saved list (defaulters, low attendance).

### Teacher-parent chat
- **REQ-COM-010** Threads per student, started by a parent or a teacher; text and images; allowed within school-set hours; admin can read (read-only monitor with conversation list, search and history) and disable chat per school/class; users can report and block; retention 12 months. No voice or video. (ref + ours)

### Forms and enquiries
- **REQ-COM-020** A forms builder with admission/enquiry and job-application templates; each form has a public link; responses become leads/enquiries (see Admission). Staff recruitment uses the same builder (no separate module). PTM feedback survey in 1.1.

### Support
- **REQ-COM-030** In-app support: new ticket, status `open / in progress / resolved / closed`, chat with the vendor team until resolved. WhatsApp business number for support. Hours 9-6 PKT weekdays; first response target 4 business hours (Starter) / 1 hour (Premium). (ref)

### Delivery management
- **REQ-COM-040** Per-message status, fallback, usage and cost reports, dead-letter list for support, content moderation list, rate limits.

## 2. User stories

### US-COM-001 Targeted announcement
- Given I am an Admin, when I post "School closed tomorrow" to Class 5 parents with scheduling for 18:00, then it is sent at 18:00 to those guardians and read counts are visible.

### US-COM-002 Chat with teacher
- Given a parent opens a thread with the class teacher during allowed hours, then messages are delivered; outside allowed hours the parent sees the school's chat hours.
- Given an Admin monitors, then they can read but not post.

### US-COM-003 Emergency
- Given I am the Owner, when I send an emergency alert, then all parents and staff receive it on every channel and delivery status is shown.

### US-COM-004 Raise a ticket
- Given I hit a problem, when I create a ticket in the app, then I can chat with support and see its status.

## 3. Business rules
- **BR-COM-001** Parents cannot opt out of absent alerts, overdue fee notices or emergency messages.
- **BR-COM-002** Chat is moderated by reporting and a keyword list; admin read access is disclosed to users.
- **BR-COM-003** Each sent message records who sent it (audit).

## 4. API (representative)
`/announcements`, `/notifications`, `/chats`, `/forms`, `/forms/{id}/responses`, `/support/tickets`, `/notification-preferences`.
