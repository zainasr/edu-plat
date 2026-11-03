# 14 Integrations

1. (BLOCKER) Payments: which aggregator/gateway (e.g. PayFast, Safepay, Paymob, JazzCash/Easypaisa direct, 1Link/1Bill, Raast)? Fees, settlement time, KYC burden on each school, collection from parents who have no wallets? Do we want a single aggregated merchant or school-owned merchants?
2. Bank integrations: statement import formats, bank-specific fee collection (challan with bank branches / HBL ConnectPay, UBL Omni, etc.)? Which banks do pilot schools use?
3. WhatsApp: BSP choice, template approvals, number ownership (one platform number vs school's own number), cost.
4. SMS: aggregators, sender ID/masking registration process, PTA/DND rules, OTP reliability and fallback.
5. Face recognition: build vs buy (cloud API vs open-source model on our servers), accuracy targets, legal issues, hardware compatibility?
6. Maps/geo-fence: provider, cost, privacy.
7. Push notifications: FCM/APNs; any Huawei devices (HMS) concerns?
8. Email delivery provider and domain setup.
9. Storage/CDN, PDF generation, document scanning/OCR vendors: managed services vs self-hosted?
10. Identity: Google/Apple sign-in (not needed?), CNIC verification (NADRA) - needed? Not likely, confirm.
11. Government/board integrations: PEC/BISE/Punjab School Education Department reporting, board result import, registration data export. Needed in the long run?
12. Third-party apps: accounting (Tally/QuickBooks), Google Workspace/Microsoft for staff email, Zoom (not LMS), biometric devices, CCTV, RFID, transport trackers. Any 1.0 must-haves?
13. Import/export connectors: Excel templates (students, staff, fee, opening balances), compatibility with reference product export (switching customers)?
14. Webhooks/API for schools' own websites (admission form embed, fee status)?
15. Monitoring/observability vendors: preference for managed (Datadog/New Relic/Sentry) vs open source?
