# Module 13. Platform, Plans and Support

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 minimal (console later) |
| Module code | PLT |

Pricing and tiers are decided later (DEC-013). The data model supports plans, flags and limits from day one.

## 1. Requirements

- **REQ-PLT-001** Trial: 14 days (extendable once by sales), full features; a top-bar badge shows "Trial N days left" with an Upgrade button. Upgrade is a request for the sales team in 1.0 (self-serve checkout later). (ref)
- **REQ-PLT-002** At trial expiry: read-only for 7 days, then locked; exports always allowed; one trial per school phone/registration number.
- **REQ-PLT-003** Signup: self-serve (OTP, school basics, demo data option, setup checklist) with sales-assisted onboarding. "Referred by" tagging and coupon codes supported.
- **REQ-PLT-004** Plans: feature flags and limits (students, branches, storage, message credits). Indicative shape: Starter / Growth / Premium with per-student monthly pricing and a minimum fee; final numbers later.
- **REQ-PLT-005** Limits: soft limits with warnings (students 10% over, storage); hard limits for message credits and trial. Overage billed on the next invoice or requires upgrade.
- **REQ-PLT-006** Billing in 1.0: manual recording of bank transfer, JazzCash/Easypaisa or cash with receipt; PDF invoices; dunning at due, +3, +7, then grace and suspension per tenant lifecycle.
- **REQ-PLT-007** Platform console (not on day one): tenant list/status, create/suspend/extend trial, plan assignment, manual billing records, feature flags, message credits, usage, tickets, announcements, audit log, consented view-as, export/deletion requests. In 1.0 these operations are done via the API and minimal tooling.
- **REQ-PLT-008** Tenant health instrumentation (logins, attendance days marked, challans released, parent app adoption) and churn-risk flags for the success team; in-app NPS quarterly.
- **REQ-PLT-009** Legal documents needed before pilot: Terms of Service, Privacy Policy, DPA, Acceptable Use, Refund Policy (a Pakistani lawyer reviews).
- **REQ-PLT-010** Public API/webhooks for Premium after 1.1; internal APIs are API-first from day one.
- **REQ-PLT-011** Subscription pages in the school portal: current plan, usage, invoices, support.

## 2. User stories

### US-PLT-001 Start a trial
- Given a new school, when it signs up with a mobile OTP and basic details, then a tenant in `trial` is created with a setup checklist and optional demo data.

### US-PLT-002 Request upgrade
- Given 3 days remain, when the Owner clicks Upgrade, then a request is sent to sales and the Owner sees "our team will reach out".

### US-PLT-003 Expiry
- Given the trial ended 8 days ago, then the tenant is locked but the Owner can still export all data.

### US-PLT-004 Support ticket
- Given a school raises a ticket, then it appears to our support with the tenant, status and chat.

## 3. Business rules
- **BR-PLT-001** Exports are always allowed, even when suspended.
- **BR-PLT-002** Suspended tenants: parents can still view and pay.
- **BR-PLT-003** Deletion requires an owner request and a 90-day retention, with legal-hold exceptions.
