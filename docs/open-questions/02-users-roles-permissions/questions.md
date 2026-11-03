# 02 Users, roles, permissions, identity

1. (BLOCKER) Final role list for 1.0: Owner, Admin, Teacher/Staff, Receptionist, Accountant (new?), Student, Parent, custom roles. Confirm or change. Is there a Principal/Head distinct from Owner?
2. (BLOCKER) Permission model: fixed roles with toggles, or fully custom roles built from permission flags? How granular (module.action, e.g. fees.collect, fees.void)?
3. Can one person hold multiple roles (teacher + admin, parent who is also staff)? How do they switch context?
4. Data scope: can a role be limited to a branch, class, section or own records only (class teacher sees own class)?
5. (BLOCKER) Login identifiers: phone, email, username, password. Which is primary per role? Is OTP (SMS/WhatsApp) login required for parents and students?
6. Student login: separate student account (what identifier at age 6 vs age 16?), or students use a profile under the parent login with PIN? Minimum age for own login?
7. Password policy, reset flow (OTP), 2FA for owners/admins, session timeout, device limits.
8. Parent accounts: one login per family (father's mobile, as in reference) or per guardian? How are mother/guardian added? What if the father's number changes?
9. Who creates accounts and credentials (auto-generated, sent via SMS/WhatsApp, first-login password change)?
10. Approval and maker-checker rules: which actions need a second person (void payment, delete student, salary approval, fee discount above X)?
11. Impersonation/"login as" for support staff: allowed? Audited?
12. Audit log scope and retention (who changed what, sign-ins, exports)?
13. Deactivation rules: what happens to a user's data and access when staff leave or a student withdraws?
14. Platform-level roles (our company): super admin, support agent, sales, finance. What can each see in tenant data?
