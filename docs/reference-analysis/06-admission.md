# 06 Admission (Enroll, Promote, Demote)
Routes: /admin/students/create, /admin/students/promote, /admin/students/demote. Screenshots: screenshots/06-admission/. (Online admission *forms/leads* are under Communication > Forms, see 18.)

## Enroll Student (3-step wizard: Basic Info, Details, Parents)
- Header actions: Download Form (printable paper form), Bulk Upload.
- Step 1 Basic Info: photo upload (JPG/PNG, 5 MB) or capture with device camera; Full Name*, Father Name*; Registration Number (Auto, estimated next value shown, or Manual); Father's mobile* (this becomes the family login for the parent app, system checks if it already belongs to a family); Secondary number (backup contact only, no login); Academic session*, Class*, Section* (depends on class).
- Steps 2 and 3 (Details, Parents) are gated by required fields; inferred from the student detail page: gender, DOB, place of birth, nationality, religion, B-Form/CRC, address, mother/guardian info, monthly income, fee/discount (admission fee, discounted fee, discount type e.g. SIBLING). Not opened (would require filling the form).
- Key rule: **father's mobile number = family account**; siblings auto-link to the same family via that number.

## Promote / Demote
- Promote: pick source session + source class, target session + target class; button shows count "Promote N Students" (disabled at 0). Bulk class-to-class move into a new session. Demote page mirrors it (not rendered separately).
- Gaps: no per-student result-based promote/hold-back, no pass/fail rule, no transfer/withdrawal/leaving certificate flow seen under Admission.
