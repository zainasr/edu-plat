#!/usr/bin/env bash
# Prints a fingerprint of the working tree's code and config (not docs). `make check` stores it
# when it passes; the Stop hook compares it, so "code changed since the last green check" is a
# fact, not a memory. Same paths in both places.
set -euo pipefail
root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel)}"
cd "$root"
paths=(services api deploy Makefile scripts .github)
{
  git diff HEAD -- "${paths[@]}"
  git ls-files --others --exclude-standard -- "${paths[@]}" | sort | xargs -r sha256sum
} | sha256sum | cut -d' ' -f1
