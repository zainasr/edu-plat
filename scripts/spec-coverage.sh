#!/usr/bin/env bash
# Spec traceability: which requirement IDs of a module appear in tests and in code?
#
#   scripts/spec-coverage.sh FEE        (module codes: STU ORG IAM FEE API ADM ATT ACC STF ...)
#
# An ID counts as "tested" when it appears in a *_test.go file (test name or comment). This is a
# map for a human or the spec-auditor agent, not proof: an ID in a test name still needs reading.
# Exit code is always 0; the table is the output.
set -uo pipefail
cd "$(dirname "$0")/.."
mod=${1:-}
[ -n "$mod" ] || { sed -n '2,10p' "$0"; exit 2; }

# An ID is DEFINED where the spec writes it bold or as a heading: **REQ-FEE-001**, ### US-FEE-001.
ids=$(grep -rhoE "(\*\*|### )(REQ|BR|US)-$mod-[0-9]+" docs/spec 2>/dev/null | sed -E 's/^(\*\*|### )//' | sort -u)
[ -n "$ids" ] || { echo "no requirement IDs defined for module $mod under docs/spec"; exit 0; }

printf '%-14s %-8s %-8s\n' "ID" "TESTED" "IN CODE"
tested=0; coded=0; total=0
while read -r id; do
  [ -n "$id" ] || continue
  total=$((total+1))
  t="no"; c="no"
  grep -rlE "\b$id\b" services/api --include='*_test.go' >/dev/null 2>&1 && { t="yes"; tested=$((tested+1)); }
  grep -rlE "\b$id\b" services/api --include='*.go' 2>/dev/null | grep -v '_test.go' | grep -q . && { c="yes"; coded=$((coded+1)); }
  printf '%-14s %-8s %-8s\n' "$id" "$t" "$c"
done <<<"$ids"
echo
echo "module $mod: $total IDs defined, $tested referenced in tests, $coded referenced in code"
echo "IDs marked no/no are usually not built yet; IDs marked yes in code but no in tests are the gaps to close."
