# ADR-0008: Repository designed for AI-agent-assisted development

| | |
|---|---|
| Status | Accepted |
| Date | 2026-10-28 |
| Spec links | DEC-019 |

## Context
Work is done by 3-4 engineers plus AI agents. Agents need current rules, fast feedback and guardrails; humans need the same standards.

## Options considered
1. **Short CLAUDE.md + path-scoped rules + hooks + a few skills/agents + one `make check`**: context loads only when relevant; deterministic rules are enforced by hooks.
2. **One large instruction file**: wastes context, rules get ignored.
3. **No agent configuration**: inconsistent output.

## Decision
Option 1, following Claude Code's documented structure: CLAUDE.md under 200 lines for always-true rules and commands; `.claude/rules/` per stack; hooks for zero-exception rules; skills for repeatable procedures; a read-only reviewer agent; project MCP servers in `.mcp.json`. Add more only when a problem repeats.

## Consequences
- Good: low context cost, consistent output, same commands for CI.
- Bad: configuration needs upkeep; review it each quarter with `/doctor`.
