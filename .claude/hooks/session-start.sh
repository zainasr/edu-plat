#!/usr/bin/env bash
# Runs at session start. Its stdout is added to Claude's context, so the first thing a fresh
# session knows is the real state of the repo, not an assumption.
root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)}"
cd "$root" 2>/dev/null || exit 0
export PATH="$HOME/go/bin:$PATH"
admin="${ADMIN_DATABASE_URL:-postgresql://app:app@localhost:5432/app_dev}"

branch=$(git branch --show-current 2>/dev/null)
dirty=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
ahead=$(git rev-list --count '@{u}..HEAD' 2>/dev/null || echo "?")
behind=$(git rev-list --count 'HEAD..@{u}' 2>/dev/null || echo "?")

echo "== Repo state (from the session-start hook) =="
echo "branch: $branch | uncommitted files: $dirty | unpushed commits: $ahead | behind origin: $behind (run 'git fetch' to refresh)"

if ver=$(timeout 3 psql "$admin" -Atc "select coalesce(max(version_id),0) from goose_db_version" 2>/dev/null); then
  latest=$(ls services/api/migrations/*.sql 2>/dev/null | sed -E 's#.*/0*([0-9]+)_.*#\1#' | sort -n | tail -1)
  if [ "$ver" = "$latest" ]; then echo "database: up, migrations current (version $ver)"
  else echo "database: up, but at migration $ver and the repo has $latest. Run: make migrate"; fi
else
  echo "database: DOWN. Tests need it. Run: make db-up && make db-roles && make migrate"
fi

if command -v gh >/dev/null 2>&1; then
  ci=$(timeout 4 gh run list --limit 1 --json conclusion,status,displayTitle --jq '.[0] | "\(.status)/\(.conclusion) \(.displayTitle)"' 2>/dev/null)
  [ -n "$ci" ] && echo "last CI run: $ci"
fi

echo
echo "Start every task by running /dev-context (reading order and standards), then /plan-slice for"
echo "anything that is more than a small fix. Money, security and scope decisions go to the human: /ask-human."
exit 0
