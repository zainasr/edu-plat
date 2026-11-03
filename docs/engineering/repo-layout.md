# Repository layout and conventions

```
.
├── CLAUDE.md                  always-loaded agent rules (short)
├── .claude/                   rules, skills, agents, hooks, shared settings
├── .mcp.json                  project MCP servers
├── Makefile                   single entry point for humans, agents and CI
├── api/openapi/               OpenAPI source of truth (edited first)
├── services/api/              Go modular monolith (own go.mod)
│   ├── cmd/api/               main: wiring only
│   ├── internal/<module>/     one package per bounded context (students, fees, ...)
│   │   ├── (domain + service)  business rules, no HTTP or SQL here beyond the store interface
│   │   ├── http.go             handlers, generated types from OpenAPI
│   │   └── store/              sqlc queries and implementation
│   ├── internal/platform/     shared plumbing: httpx, db, tenant, auth, jobs, config
│   └── migrations/            goose SQL migrations (forward-only)
├── apps/web/                  Next.js (admin/staff portal)
├── apps/mobile/               React Native (parent, teacher, student)
├── ai/                        Python AI services (separate from the Go core)
├── deploy/                    docker-compose, Terraform, deploy scripts
├── .github/workflows/         CI
└── docs/
    ├── spec/                  locked product spec (what and why)
    ├── adr/                   architecture decisions (how, and why this way)
    ├── engineering/           this file and engineering guides
    ├── learning/              Go and engineering learning notes tied to real code
    ├── runbooks/              operations procedures
    ├── open-questions/        stakeholder Q&A history
    └── reference-analysis/    reference product study
```

## Rules of the layout
- Modules mirror `docs/spec/03-modules/`. A module never reads another module's tables; it calls that module's exported interface or listens to its events.
- `internal/` keeps everything private to the service; nothing outside imports it.
- Generated code lives in `gen/` subfolders and is never edited by hand (a hook enforces this).
- Config and secrets come from environment variables; `.env.example` documents them; real values never enter the repo.
