#!/usr/bin/env bash
# Blocks edits that must never be done by hand: generated code, and migrations
# that already exist (new migration files are allowed). Zero-exception rules
# live in hooks, not CLAUDE.md, because hooks are enforced.
f=$(jq -r '.tool_input.file_path // empty')
[ -z "$f" ] && exit 0

case "$f" in
  */gen/*|*.gen.go|*_generated.go)
    echo "Generated code: edit api/openapi/openapi.yaml or the .sql source, then run 'make gen'." >&2
    exit 2 ;;
  */migrations/*.sql)
    if [ -e "$f" ]; then
      echo "Applied migrations are immutable. Add a new migration file instead." >&2
      exit 2
    fi ;;
esac
exit 0
