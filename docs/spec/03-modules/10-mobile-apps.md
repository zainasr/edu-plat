# Module 10. Mobile Apps and Web UX

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 parent + staff; 1.1 student full |
| Module code | APP |

## 1. Structure
- Separate apps built on React Native: **Parent**, **Teacher/Staff**, **Student** (light). Everyone else uses the **web** portal (Next.js). School logo, name and colour appear inside our apps; separate branded store apps are a premium add-on later.
- Android first (majority in Pakistan); iOS in 1.0 if the framework allows without extra cost. Android 11+, 2 GB RAM, under 30 MB, works on 3G/4G/5G.

## 2. Requirements

### Offline and sync
- **REQ-APP-004** The staff app is optional per school: where phones are banned, teachers use the web portal on school devices for attendance and diary, and parents still use the parent app.
- **REQ-APP-001** Staff app: attendance, marks entry, diary drafts and timetable cache work offline with a sync queue and visible status.
- **REQ-APP-002** Parent/student app: cached last-seen data read-only (dues, attendance, timetable, results); payments online only.
- **REQ-APP-003** Admin web is online only.

### Parent app home (everything within two taps)
Child switcher → today's attendance → fees due + Pay → diary/homework → announcements → results/progress → ID card → leave request. Notifications grouped per child, deep links, badges, quiet hours.

### Student app
Today's timetable, homework, results and simple progress hints, appreciation points, ID card QR, resources. Light gamification (streaks, badges) is [1.2].

### Teacher app home
Mark attendance (one tap per class, quick "present all"), today's timetable, diary, marks entry / class tests, leave and payslip, messages, check-in.

### Web by role
- **Owner:** owner dashboard (money, attendance, alerts). **Admin:** operations "needs attention". **Receptionist:** fee collect + enquiries. **Accountant:** collections and expenses. Every landing page leads with "what needs my action today". For a person with several roles the landing page merges the relevant widgets.
- Receptionist: keyboard-fast collect, search by name/registration/mobile/family, 80 mm thermal receipt via browser print, USB/Bluetooth scanner or webcam QR.
- Responsive admin web down to tablet; usable on phones for owner dashboards and quick actions.

### Design and content
- **REQ-APP-010** Our own brand and design system (tokens, components, AA contrast, large touch targets). Placeholder tokens until the brand exists.
- **REQ-APP-011** English-only UI with Urdu Unicode input/display (RTL blocks) in diary, announcements, messages and names. All strings externalised.
- **REQ-APP-012** Onboarding checklist (profile, classes, import students, fee plan, invite parents), contextual empty states with a next action, sample demo tenant, short video links.
- **REQ-APP-013** PDFs generated server-side with fixed templates/custom template engine as well (challan, receipt, ID card, report card); thermal receipt via browser print; bulk PDFs run asynchronously and appear in a "Downloads" list.
- **REQ-APP-014** Plain English, icon-led UI, prominent amounts, no voice help in 1.0.
- **REQ-APP-015** A forced-update mechanism for breaking API changes; staged rollouts.

## 3. User stories

### US-APP-001 Parent opens the app
- Given I have two children, when I open the app, then I see a child switcher, today's attendance for the selected child, dues with a Pay button and unread diary, all without scrolling more than one screen.

### US-APP-002 Switch child
- Given I tap the second child, then all sections (attendance, dues, diary, results) switch to that child.

### US-APP-003 Teacher on a weak network
- Given 2G connectivity, when I mark attendance, then it saves locally immediately and uploads when possible.

### US-APP-004 Student ID QR
- Given I open the ID card, then a rotating QR is shown that stops working if I screenshot it after refresh time.

## 4. Decide in design
Offline sync library; push provider setup; accessibility audit; Huawei no need to target it devices handled by WhatsApp fallback.
