# 18 Communication hub (Announcements, Chats, Forms/Enquiries, Daily Diary, Resources, Support)
Routes: /admin/announcements(+/create), /admin/communication/chats, /admin/forms (+/new, /leads), /admin/daily-diary, /admin/resources, /admin/support. Screenshots: screenshots/18-communication/.

## Announcements
- Tiles: announcements, active, audiences used, this month. Tabs/filter: **Admin Announcements** (created by the school) vs **System Notifications** (378 auto-generated, e.g. fee/attendance events) vs All.
- Create: Title*, Message* (plain text), Recipient type: Everyone, Teachers (14), All staff (14), Parents (94), Specific (choose individuals). No scheduling, attachments, class-level targeting or channel choice (SMS/WhatsApp/push) visible. Delivery appears to be in-app/push only.

## Teacher-Parent Chats
Admin read-only monitor of teacher-parent conversations (list with search, message history pane, conversation count). Implies in-app teacher-parent messaging exists in the mobile apps.

## Forms and Enquiries (admission lead capture)
- Form builder: "Build a form, share one link, read what comes back". List: status Draft/Live/Closed, responses, last response, public link, edit/archive. Starts from **templates**: Admission enquiry (12 questions; captures a lead that converts into an enrolment), Admission application full (20 questions with attachments; the office types nothing twice), Parent feedback survey (8), Event registration (6), Transport request (8), Consent/permission slip (7, wording frozen once published), Staff job application (10, CV attached), Complaint/suggestion box (4, anonymous), Blank form.
- **Enquiries** (/forms/leads): inbox of leads from enquiry forms (did not render text in the session).
- Covers our MVP "admission forms" and goes beyond (surveys, consent).

## Daily Diary
Post daily diary entries (homework / class notes) per class and section and review entries teachers created. Filters: class, section, teacher, subject, date range. Tiles: entries, classes, subjects, teachers. Toggle "Markable tasks" (tasks that can be ticked off); New entry.

## Resources
Library of notes, worksheets, syllabus, past papers, books, other; add resource, scope to whole school/class/section, Published vs Draft; visible to parents and teachers in the app. (We are not an LMS, but sharing study material is a light feature; past papers link to our later paper/test maker.)

## Support
Ticketing with the vendor: new ticket, status (Open, In progress, Resolved, Closed), search, chat with the vendor team until resolved.

## Observations / gaps / ideas
- Missing for Pakistan: SMS and WhatsApp channels, message templates (fee reminder, absent alert, result), delivery reports and cost control, scheduled/recurring broadcasts, class-wise targeting, read receipts, emergency broadcast.
- Ideas: central notification hub with rules ("absent -> notify parent within 10 min", "fee due in 3 days"), per-parent channel preference and quiet hours, AI-drafted messages and weekly parent digests, engagement analytics (who reads what), FAQ chatbot.

## Hands-on addendum (2026-10-12)
- **Enquiries inbox** (/forms/leads): "everyone who has filled in one of your forms, and where each has got to". Tiles: enquiries, new, contacted, enrolled. Status list: New, In review, Contacted, Enrolled, Not proceeding, Spam. Filters: form, status, owner (staff member assigned), "Possible duplicates". Empty state: share a form link and enquiries arrive here. Forms list shows the Admission enquiry template with response count and edit/responses links.
