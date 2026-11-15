#!/usr/bin/env bash
# Records that `make check` passed for the current working tree. Called by the Makefile after
# lint and test succeed, never by hand. The Stop hook reads this file.
set -euo pipefail
root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel)}"
mkdir -p "$root/.claude/state"
"$root/scripts/fingerprint.sh" > "$root/.claude/state/check.stamp"
