---
paths:
  - "ai/**"
---
# Python AI service rules
- Separate service; talks to the core only through the API and the event log. It never connects to the core database directly.
- Pinned dependencies, typed code, tests with fixed seeds. Every model output that reaches a user must be explainable in plain language (DEC-012).
