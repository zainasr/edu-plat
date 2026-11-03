# 10 Communication and notifications

1. (BLOCKER) Channels in 1.0: in-app push, SMS, WhatsApp, email. Which are used for what (OTP, fee reminders, absent alerts, announcements, results)? Which are mandatory vs optional?
2. Who pays for SMS/WhatsApp: included in plan quota, institution buys credits, pass-through billing? Margin policy?
3. Provider choices: WhatsApp Business API (official BSP) vs SMS aggregators in Pakistan (masking sender ID/PTA registration). Preference or past experience?
4. Notification catalogue: confirm events (absent, late, fee due/overdue/paid, result published, diary posted, announcement, leave decision, behaviour entry, exam schedule, birthday, emergency). Who can trigger manual ones?
5. Template management: system templates (English, plus Urdu/Roman Urdu text), editable by the school? Approval of WhatsApp templates by Meta/BSP handled by us?
6. Rules engine: should schools configure "when X then notify Y via Z at time T" or is a fixed catalogue with toggles enough for 1.0?
7. Targeting: whole school, branch, class, section, individual, family, staff group, custom lists, fee defaulters list. Scheduling and recurrence?
8. Parent preferences: opt-in/out per category, quiet hours, preferred channel, language. Mandatory notices that cannot be disabled?
9. Two-way messaging: teacher-parent chat (reference has monitoring). Rules: working hours, attachments, moderation, report/abuse, admin visibility, retention, disable per school?
10. Announcements: attachments, read receipts, acknowledgements, emergency broadcasts, pinned notices?
11. Delivery tracking: sent/delivered/read/failed, retry/fallback chain (push -> WhatsApp), cost per message report.
12. Anti-spam and compliance: consent for WhatsApp, DND, opt-out keywords, rate limits per school.
13. Notification center in app and web (history, unread, deep links)?
14. Email: needed at all (receipts, reports, onboarding)?
