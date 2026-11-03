# 06. Non-Functional Requirements

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 |
| Module code | NFR |

## 1. Availability and resilience

- **NFR-001** Target 99.9% monthly availability for core services.
- **NFR-002** Planned maintenance only 01:00 to 04:00 PKT; none during 07:00 to 14:00 or on the 1st to 10th of a month (fee window).
- **NFR-003** Attendance and marks entry in the staff app work offline and sync later (graceful degradation).
- **NFR-004** High availability: at least two application nodes behind a load balancer, database with standby and tested failover.
- **NFR-005** Backups: RPO 5 minutes or better (point-in-time recovery), RTO 2 hours or better; daily full plus continuous log backups; 35-day retention; copy to a separate region/provider; quarterly restore drills; per-tenant logical restore capability.

## 2. Performance

| Measure | Target |
|---|---|
| First meaningful screen | under 3 s on 3G Android |
| API reads p95 | under 400 ms |
| API writes p95 | under 800 ms |
| Attendance save | under 1 s locally, background sync |
| 1,000 challan PDFs | under 2 minutes (async) |
| Import of 5,000 students | under 3 minutes (async, with progress) |
| Ledger report on 1M rows | under 5 s (pre-aggregated) |

## 3. Scale

- Year 1: 200 to 500 institutions, 100k to 250k students; peak concurrency about 5% of users in the morning window; result-day and fee-day spikes 5 to 10 times.
- Horizon: 10k institutions (year 3); design for horizontal scale and cell/shard-based tenancy from the start.
- Per-tenant rate limits and queues to avoid noisy neighbours.

## 4. Security

- **NFR-010** TLS everywhere; encryption at rest; field-level encryption for CNIC, B-Form, bank accounts and face templates.
- **NFR-011** Secrets in a vault; dependency, container and IaC scanning in CI; third-party penetration test before public launch; responsible-disclosure page.
- **NFR-012** OTP rate limits per number/device/IP; CAPTCHA on abuse; device binding for staff/admin; new-device alerts.
- **NFR-013** File uploads: type/size allowlist, antivirus scan, image resizing, per-tenant storage quotas, signed expiring links (no public file URLs).
- **NFR-014** Tenant isolation proven by automated tests on every release (cross-tenant access attempts must fail).
- **NFR-015** Practices aligned with ISO 27001 / SOC 2 to make certification possible later.

## 5. Privacy and compliance

- **NFR-020** Follow GDPR-like principles: lawful basis/consent, purpose limitation, minimisation, parental consent for under-18s, special handling of biometric data.
- **NFR-021** Access/correction/deletion requests handled within 30 days; internal breach notification target 72 hours.
- **NFR-022** Biometrics: explicit parent consent, face template (not raw photo) stored encrypted, deletable on withdrawal or consent withdrawal, no raw image retention by default.
- **NFR-023** Data ownership: tenant data belongs to the institution; de-identified aggregated use for improving models is covered in the terms with opt-out.
- **NFR-024** Retention: finance 7 years; operational audit 2 years; old sessions archived and queryable; deletion on request except legal hold; backup purge schedule documented.
- **NFR-025** Pakistani legal review (data protection, minors, education-sector) is required before launch.

## 6. Data integrity and audit

- **NFR-030** Append-only audit log and finance journals; edits create new versions; corrections via reversing entries.
- **NFR-031** Receipt numbers sequential with no gaps per institution per fiscal year.

## 7. Usability and compatibility

- **NFR-040** Browsers: latest two versions of Chrome, Edge, Safari, Firefox. Android 11+ (2 GB RAM devices), iOS 15+. Mobile app size under 30 MB.
- **NFR-041** WCAG 2.1 AA for contrast and keyboard on admin web; basic screen-reader labels; large touch targets.
- **NFR-042** Admin web is responsive down to tablet and usable on phones for owner dashboards and quick actions.
- **NFR-043** Plain English, icon-led UI, prominent numbers and amounts.

## 8. Operability

- **NFR-050** SLOs for availability, latency and notification delivery; alerting to on-call; public status page; incident post-mortems.
- **NFR-051** Ops dashboards: message queue lag, payment webhook failures, import failures, PDF job times, login failures; alerts linked to runbooks.
- **NFR-052** Test pyramid with coverage of 80% on business logic; small end-to-end smoke suite for critical flows (login, enrol, generate fees, collect, mark attendance).
- **NFR-053** Hosting region India (Bangalore); sensitive data encrypted; revisit if Pakistani residency rules apply.

## 9. Decide in design

- Observability stack products, on-call arrangement, penetration test vendor.
