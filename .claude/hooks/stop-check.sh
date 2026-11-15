#!/usr/bin/env bash
# Runs when Claude is about to finish a turn. If code or config changed since `make check` last
# passed, it tells Claude to run the check first (exit 2 sends the message back and continues).
# It nags once per turn: stop_hook_active stops it from looping.
payload=$(cat)
[ "$(jq -r '.stop_hook_active // false' <<<"$payload")" = "true" ] && exit 0

root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)}"
cd "$root" 2>/dev/null || exit 0

# Nothing under the code paths differs from HEAD: nothing to verify.
[ -z "$(git status --porcelain -- services api deploy Makefile scripts .github 2>/dev/null)" ] && exit 0

now=$("$root/scripts/fingerprint.sh" 2>/dev/null)
stamp=$(cat "$root/.claude/state/check.stamp" 2>/dev/null || true)
[ -n "$now" ] && [ "$now" = "$stamp" ] && exit 0

echo "Code or config changed since 'make check' last passed. Before you finish:" >&2
echo "  1. run: make check   (the database must be up: make db-up)" >&2
echo "  2. if you changed behaviour, run /verify; if you only edited docs, say so." >&2
echo "If you cannot run it, say exactly why in your final message instead of claiming the work is done." >&2
exit 2
