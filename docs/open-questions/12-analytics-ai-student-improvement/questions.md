# 12 Analytics, AI and student improvement

1. (BLOCKER) What exactly is "student improvement" in 1.0? Rule-based hints from marks/attendance/behaviour, or real ML? Define 3 concrete outputs a parent/student sees on day 1.
2. AI roadmap: list the ML models we intend to own (fee default prediction, dropout/early-warning, weak-topic detection, timetable optimiser, face recognition, forecasting). Priority order and target release for each?
3. Data readiness: what minimum data volume/quality is needed before a model is switched on, and how do we handle cold start (new school)? Shared models across tenants vs per-tenant models?
4. Data ownership and consent: can we train on pooled, anonymised data across institutions? What is stated in the contract, parent consent text, opt-out? This is legally and commercially critical to the moat.
5. Children's data privacy: restrictions on profiling minors, explainability for parents, human-in-the-loop for sensitive predictions (e.g. dropout risk)?
6. Third-party LLM use: allowed for text generation (parent summaries, diary drafts)? Data minimisation, PII redaction, vendor choice, cost cap per tenant?
7. Dashboards per role: define KPIs for owner, principal/admin, teacher, parent, student.
8. Benchmarks: show a school its metrics vs anonymous peers (collection %, attendance %)? Value vs privacy risk?
9. Insight delivery: in-app cards, weekly digest, WhatsApp summary? How actionable (links to action)?
10. Evaluation and quality: how do we measure model accuracy and usefulness (precision, adoption of recommendations, outcome change)? Who reviews?
11. Event/data collection design: which product events and fields must be captured from day one (we need a data dictionary and event schema)? Retention policy?
12. ML ops: feature store/warehouse, training pipeline, model registry, monitoring (drift), rollback. Appetite for investing early vs after PMF?
13. Reporting exports: scheduled reports (PDF/Excel by email/WhatsApp)?
14. Student improvement content: do we author practice material/hints (needs syllabus tree) or only show analytics? Link to the later paper/test maker.
