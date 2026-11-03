---
paths:
  - "apps/web/**"
---
# Next.js web rules
- TypeScript strict. API calls go through the client generated from `api/openapi/openapi.yaml`; no hand-written fetch to our own API.
- All UI strings are externalised (English now, Urdu later). No hard-coded product name.
- Plain-language labels from the spec glossary; the accountant never sees "debit/credit" in primary screens.
- Accessibility: keyboard reachable forms, visible focus, sufficient contrast.
