# Learning

Learning material that grows with the code. Two audiences: the product owner learning Go from fundamentals to product level, and any engineer joining later.

## Start here
1. `go/` for Go, from a TypeScript background (read 01 to 08 in order).
2. `architecture/` for the three tiers and a full request flow.
3. `database/` for transactions, standby, PITR and backups.
4. `../engineering/getting-started.md` to set up the machine.

## Notes index
- Go: `go/README.md` (01 to 08)
- Architecture: `architecture/01-three-tier-and-request-flow.md`
- Database: `database/01-transactions-replication-and-recovery.md`

## How this folder works
- Each finished slice adds a note in `go/` (use the `/learning-note` skill). A note links to the exact files in this repo, so you learn from real product code.
- `00-roadmap.md` maps concepts to the slices where they first appear.
- Notes are short, plain language, and end with an exercise or a reading link.

## Core references
- A Tour of Go: https://go.dev/tour/
- Effective Go: https://go.dev/doc/effective_go
- Go standard library docs: https://pkg.go.dev/std
- Organising a Go module: https://go.dev/doc/modules/layout
- Go Proverbs: https://go-proverbs.github.io/
- Book: "Learning Go" (Jon Bodner), "100 Go Mistakes" (Teiva Harsanyi), "Let's Go" (Alex Edwards)
- PostgreSQL docs: https://www.postgresql.org/docs/current/
- PostgreSQL row-level security: https://www.postgresql.org/docs/current/ddl-rowsecurity.html
- OpenAPI specification: https://spec.openapis.org/oas/latest.html
- Claude Code docs: https://code.claude.com/docs
