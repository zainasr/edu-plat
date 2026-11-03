---
paths:
  - "apps/mobile/**"
---
# React Native rules
- Offline-first for attendance, marks and diary drafts: local DB, outbox of operations with client-generated IDs, idempotent server endpoints (REQ-API-010).
- Use the generated API client. Handle forced-update responses (REQ-API-016).
- Low-end Android first: small bundle, no heavy animations, test on a slow device profile.
