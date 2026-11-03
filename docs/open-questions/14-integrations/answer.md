# 14 Answers

1: [ACCEPTED] Owner decision (2026-10-19): ONE Pakistani payment aggregator exposing Easypaisa, JazzCash, bank/Raast and cards under a single API, settling to each school's bank account. Shortlist and final selection in design phase (fees, settlement time, KYC, sandbox, webhook reliability).
2: [ACCEPTED-DEFAULT] 1.0: 1Bill/1Link-style consumer numbers on challans (any bank app can pay) plus statement CSV upload in 1.1. Bank-specific integrations only on demand per large school. Ask pilot schools which banks they use.
3: [ACCEPTED-DEFAULT] One platform WhatsApp business number (display name = our brand + school name in message) via an official BSP; templates managed by us. School's own number option later for Premium.
4: [ACCEPTED-DEFAULT] Owner decision (2026-10-19): SMS is not a main channel. One Pakistani SMS provider with a registered sender ID, used only for OTP fallback and rare critical fallback. OTP primarily via WhatsApp/in-app where possible. Per-tenant rate limits.
5: [ACCEPTED-DEFAULT] Pilot with a managed cloud face-recognition API (fast time-to-market, privacy review), design an abstraction layer, evaluate self-hosted open-source models (InsightFace/ArcFace family) for cost and data sovereignty before scaling. Liveness required. No raw image retention by default.
6: [ACCEPTED-DEFAULT] Google Maps or an open alternative (Mapbox/OSM) only for the admin geo-fence picker; staff check-in uses device GPS locally verified; store minimal coordinates.
7: [ACCEPTED-DEFAULT] FCM for Android (and iOS via FCM/APNs). Huawei HMS not in 1.0 (fallback to WhatsApp/SMS).
8: [ACCEPTED-DEFAULT] Managed transactional email (e.g. SES/Postmark/SendGrid) with branded domain, SPF/DKIM.
9: [ACCEPTED-DEFAULT] Object storage with CDN on DigitalOcean Spaces (S3-compatible); PDF generation self-hosted headless renderer inside our stack; OCR deferred to 1.2 using a managed API.
10: [ACCEPTED-DEFAULT] Not needed in 1.0: no NADRA verification, no social login. Phone OTP is the identity anchor.
11: [ACCEPTED-DEFAULT] Long-term (2.0+): board result/registration exports; 1.0 provides Excel exports of student registers in common formats.
12: [ACCEPTED-DEFAULT] 1.0 must-haves: Excel import/export only. Others later by demand (biometric, RFID, transport tracking, accounting export).
13: [ACCEPTED-DEFAULT] Provide a "Switch to us" Excel importer aimed at common competitors' export formats (including the reference product) to ease migration.
14: [ACCEPTED-DEFAULT] After 1.1: embeddable admission form and public endpoints for institutions' websites (rate-limited, signed).
15: [ACCEPTED-DEFAULT] Self-hosted open-source observability (metrics, logs, traces, uptime) on our DigitalOcean infrastructure from day one; a managed error tracker may be added. Decided in design.
