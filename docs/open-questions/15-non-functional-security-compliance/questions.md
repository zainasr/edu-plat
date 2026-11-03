# 15 Non-functional requirements, security, privacy, compliance

1. (BLOCKER) Availability target (99.5/99.9%)? Maintenance windows? Downtime tolerance during school hours (07:00-14:00 PKT) and fee deadlines (1st-10th)?
2. Performance targets: page load on 3G/low-end Android, API p95 latency, attendance save time, bulk PDF generation (1,000 challans), import of 5,000 students, report time for 1M+ ledger rows.
3. Scale assumptions: first-year institutions/students, peak concurrent users (morning attendance, fee days, result day), 10k and 100k institution horizon, growth of data per tenant.
4. Data residency: must data stay in Pakistan, or is an overseas region acceptable? Any customer/regulatory pressure? Impact on cloud region choice and latency.
5. Backup and DR: RPO/RTO targets, backup frequency, retention, per-tenant restore capability, DR region?
6. Security baseline: encryption in transit/at rest, field-level encryption (B-Form, CNIC, bank details), secrets management, vulnerability scanning, pentest before launch, bug bounty later?
7. Tenant isolation: logical isolation (row-level), schema per tenant, or database per tenant for large customers? Noisy-neighbour controls.
8. Authentication security: OTP abuse prevention, rate limiting, device binding, suspicious login alerts, account recovery without email.
9. Privacy and legal: Pakistan Personal Data Protection Bill status, minors' data, parental consent, face data as biometric (sensitive), data subject requests (access/delete), cookie/consent for web, breach notification process.
10. Audit and traceability: immutable audit log, finance traceability, retention periods (see section 02).
11. Accessibility and compatibility: browser support, minimum devices, screen readers (basic).
12. Logging, monitoring and alerting: SLOs, on-call model, status page for customers?
13. Data lifecycle: archival of old sessions, deletion on request, legal hold, anonymisation for analytics.
14. Compliance certifications aspirations (ISO 27001/SOC 2 later) to win larger school groups?
15. Content safety: user-uploaded files (virus scanning, size/type limits), chat abuse moderation, image content?
16. Observability of business metrics for ops (message failures, payment webhook lag, job queues)?
