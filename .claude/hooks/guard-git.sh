#!/usr/bin/env bash
# Guards git commands run through the Bash tool.
#   - blocks force pushes, remote branch deletion, --no-verify, and history-destroying commands:
#     these need the human, always
#   - on a normal `git push`, runs `make check` first and blocks the push if it fails, so a red
#     commit never leaves the machine
# Quoted text (commit messages) is stripped first, so a message that mentions "git push" is safe.
payload=$(cat)
cmd=$(jq -r '.tool_input.command // empty' <<<"$payload")
[ -n "$cmd" ] || exit 0

stripped=$(python3 -c '
import re, sys
c = sys.stdin.read()
c = re.sub(r"<<-?\s*\x27?(\w+)\x27?.*?\n\1\b", "", c, flags=re.S)   # heredoc bodies
c = re.sub(r"\"(?:\\.|[^\"\\])*\"|\x27[^\x27]*\x27", "\"\"", c, flags=re.S)  # quoted strings
print(c)' <<<"$cmd")

# `git` counts only where a command starts: line start, after ; && || | ( $( or backtick, optionally
# after VAR=value, sudo or env, and optionally with -C dir / -c k=v before the subcommand.
GIT='(^|[;&|(`]|\$\()[[:space:]]*([A-Za-z_][A-Za-z0-9_]*=[^[:space:]]*[[:space:]]+|sudo[[:space:]]+|env[[:space:]]+)*git[[:space:]]+(-[Cc][[:space:]]+[^[:space:]]+[[:space:]]+)*'

grep -qE "${GIT}[a-z]" <<<"$stripped" || exit 0

block() { echo "Blocked: $1" >&2; echo "$2" >&2; exit 2; }

grep -qE -- '--no-verify|--no-gpg-sign' <<<"$stripped" && \
  block "git hook bypass." "Do not skip checks. Fix the failure, or ask the human."

if grep -qE "${GIT}push" <<<"$stripped"; then
  grep -qE -- '--force|--force-with-lease|[[:space:]]-f([[:space:]]|$)|[[:space:]]\+[A-Za-z0-9_./-]+' <<<"$stripped" && \
    block "force push." "Rewriting remote history needs the human. Explain why and ask."
  grep -qE -- '--delete|[[:space:]]:[A-Za-z0-9_./-]+' <<<"$stripped" && \
    block "deleting a remote branch." "Ask the human."

  if [ -z "${HOOK_SKIP_CHECK:-}" ]; then
    root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)}"
    log=$(mktemp)
    if ! (cd "$root" && timeout 280 make check >"$log" 2>&1); then
      echo "Blocked: make check fails, so this push would put a red commit on the remote." >&2
      echo "If the database is down, run: make db-up && make db-roles && make migrate" >&2
      tail -25 "$log" >&2
      rm -f "$log"; exit 2
    fi
    rm -f "$log"
  fi
fi

grep -qE "${GIT}(reset[[:space:]]+--hard|clean[[:space:]]+-[a-z]*f|checkout[[:space:]]+(--[[:space:]]|\.)|restore[[:space:]]+(--source[^[:space:]]*[[:space:]]+)?\.|stash[[:space:]]+(drop|clear)|branch[[:space:]]+-D)" <<<"$stripped" && \
  block "a command that can destroy uncommitted or unmerged work." "Ask the human first, and say exactly what would be lost."

exit 0
