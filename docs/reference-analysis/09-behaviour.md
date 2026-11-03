# 09 Behaviour
Routes: /admin/behaviour, /admin/behaviour/analytics (Summary), /admin/behaviour/settings. Screenshots: screenshots/09-behaviour/.

## Log
- Staff log appreciation, concerns, or notes about students and choose whether parents are told. Tiles: appreciation entries, concerns (open count), to review (held until admin approves), parent replies (unread conversations). Filters: type (Positive, Negative, Note), category, class, status (Waiting for review, Open, In follow-up, Resolved), search by title/student/teacher.
- Default categories: positive (Kindness, Helping others, Excellent work, Leadership, Honesty); negative (Late to class, Incomplete homework, Uniform, Disruption, Disrespect, Damage to property, Bullying, Fighting, Injury or accident, Health concern); General note. Each has a severity (Minor etc.) and points (+2/+3, -1...).
- Status lifecycle for concerns: Waiting for review -> Open -> In follow-up -> Resolved.

## Settings
- Parents: share entries with parents (off by default until parent app is updated), let parents reply (two-way thread, logger notified).
- Review before parents see it: hold teacher's concerns at severity thresholds (Never, Minor+, Moderate+, Serious+, Critical only); appreciation and admin entries go out immediately.
- Points and alerts: merit points toggle with running session total visible to staff and parents; alerts (e.g. on-watch threshold 3+ concerns in 30 days). Categories editable (add/edit/delete, with points).
- Summary page: filters class and period (session / 30 / 90 days); tiles entries, appreciation, concerns, on watch (3+ concerns in 30 days).

## Entities
BehaviourCategory(type, name, severity, points), BehaviourEntry(student_id, category_id, author_id, text, status, shared_with_parent, created_at), BehaviourReply(entry_id, author, text).

## Ideas
Strong base for AI: early-warning (behaviour + attendance + marks), positive-reinforcement streaks and parent badges. Add counsellor workflow, attachments, incident reports.
