#!/usr/bin/env bash
# Checks every NEW migration file before it is written (guard-paths.sh already blocks edits to
# existing ones). Enforces .claude/rules/sql-migrations.md and DEC-011:
#   - has a goose Up marker
#   - forward-only and backward-compatible: no DROP TABLE, DROP COLUMN or TRUNCATE in Up
#   - no DELETE grant (business records are never hard-deleted)
#   - a table with tenant_id must switch on row level security in the same file
payload=$(cat)
f=$(jq -r '.tool_input.file_path // empty' <<<"$payload")
case "$f" in
  */services/api/migrations/*.sql) ;;
  *) exit 0 ;;
esac
content=$(jq -r '.tool_input.content // empty' <<<"$payload")
[ -n "$content" ] || exit 0

up=$(awk '/^-- \+goose Down/{exit} {print}' <<<"$content")

block() { echo "Blocked migration $f: $1" >&2; echo "$2" >&2; exit 2; }

grep -q '^-- +goose Up' <<<"$content" || block "no '-- +goose Up' marker." "Start the file with: -- +goose Up"

if grep -qiE '\b(DROP[[:space:]]+TABLE|DROP[[:space:]]+COLUMN|TRUNCATE)\b' <<<"$up"; then
  block "destructive statement in the Up section." \
    "Migrations are forward-only and backward-compatible: expand, migrate data, then contract in a LATER release. Ask the human before any DROP."
fi

if grep -qiE 'GRANT[^;]*\bDELETE\b' <<<"$up"; then
  block "grants DELETE." "Business records are never hard-deleted (DEC-011). Archive, deactivate, revoke or reverse instead."
fi

if grep -qiE 'CREATE[[:space:]]+TABLE' <<<"$up" && grep -q 'tenant_id' <<<"$up" && ! grep -qi 'ROW LEVEL SECURITY' <<<"$up"; then
  block "a table with tenant_id but no row level security." \
    "Every tenant table needs ENABLE ROW LEVEL SECURITY and the tenant_isolation policy (copy the DO block from migration 00004), a tenant-leading index and composite tenant foreign keys."
fi
exit 0
