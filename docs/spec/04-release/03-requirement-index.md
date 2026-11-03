# 03. Requirement Index and ID Scheme

| | |
|---|---|
| Status | Draft v0.1 |
| Module code | IDX |

## Module codes

| Code | Document |
|---|---|
| PRD | 00-product/01-vision-and-scope.md |
| ORG | 02-cross-cutting/01-organisation-and-structure.md |
| IAM | 02-cross-cutting/02-identity-roles-permissions.md |
| DATA | 02-cross-cutting/03-data-model.md |
| API | 02-cross-cutting/04-api-conventions.md |
| NTF | 02-cross-cutting/05-notifications-catalogue.md |
| NFR | 02-cross-cutting/06-non-functional-requirements.md |
| STU | 03-modules/01-students-and-families.md |
| ADM | 03-modules/02-admission-and-lifecycle.md |
| ATT | 03-modules/03-attendance.md |
| FEE | 03-modules/04-fees-and-payments.md |
| ACC | 03-modules/05-accounting-and-reports.md |
| STF | 03-modules/06-staff-leave-and-payroll.md |
| ACD | 03-modules/07-academics-exams-and-diary.md |
| BEH | 03-modules/08-behaviour.md |
| COM | 03-modules/09-communication.md |
| APP | 03-modules/10-mobile-apps.md |
| ANA | 03-modules/11-analytics-and-student-improvement.md |
| ONB | 03-modules/12-onboarding-and-import.md |
| PLT | 03-modules/13-platform-and-plans.md |
| DEC | 01-decisions/01-decision-log.md |
| OI | 04-release/02-open-items.md |

## Guidance for agents implementing this spec

1. Pick one user story (`US-*`); read its module document and the cross-cutting documents it references.
2. Treat `REQ-*` and `BR-*` as constraints; treat acceptance criteria as test cases (write them first).
3. Add or change the endpoint in the OpenAPI file first; generate server and client code.
4. Never bypass tenant isolation, permission checks or audit. Money code is reviewed by a human.
5. If the spec is silent or contradictory, raise an open item; do not guess.
6. Keep this spec updated in the same pull request as the code that changes behaviour.
