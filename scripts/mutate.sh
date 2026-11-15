#!/usr/bin/env bash
# Mutation check for CODE: break one line on purpose and confirm a test notices.
# A test you have never seen fail may not be testing anything.
#
#   scripts/mutate.sh FILE 'exact text to replace' 'replacement' PACKAGE [TEST_REGEX]
#
# Example (from the repo root):
#   scripts/mutate.sh services/api/internal/fees/rules.go \
#     'if v.EffectiveFrom.After(month) {' 'if false && v.EffectiveFrom.After(month) {' \
#     ./internal/fees/ 'TestBR_FEE_001'
#
# Exit: 0 = KILLED (a test failed: good), 1 = SURVIVED (no test failed: add a test), 2 = harness error.
# The file is restored byte-for-byte on every exit path, and the restore is verified with cmp.
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH="$HOME/go/bin:$PATH"
: "${ADMIN_DATABASE_URL:=postgresql://app:app@localhost:5432/app_dev}"
: "${DATABASE_URL:=postgresql://app_rt:app_rt@localhost:5432/app_dev}"
export ADMIN_DATABASE_URL DATABASE_URL

[ $# -ge 4 ] || { sed -n '2,13p' "$0"; exit 2; }
file=$1; search=$2; replace=$3; pkg=$4; regex=${5:-.}

[ -f "$file" ] || { echo "no such file: $file"; exit 2; }
case "$file" in
  */migrations/*|*/gen/*|*_test.go|*.gen.go) echo "refusing to mutate $file (migrations, generated code and tests are not mutated)"; exit 2;;
  services/api/*|api/*) ;;
  *) echo "refusing to mutate $file (only files under services/api or api/)"; exit 2;;
esac

count=$(FILE="$file" SEARCH="$search" python3 -c 'import os;print(open(os.environ["FILE"]).read().count(os.environ["SEARCH"]))')
[ "$count" = "1" ] || { echo "the search text must appear exactly once in $file (found $count)"; exit 2; }

backup=$(mktemp)
cp "$file" "$backup"
restore() { cp "$backup" "$file"; }
trap restore EXIT INT TERM

FILE="$file" SEARCH="$search" REPLACE="$replace" python3 - <<'PY'
import os
p=os.environ["FILE"]; s=open(p).read()
open(p,"w").write(s.replace(os.environ["SEARCH"], os.environ["REPLACE"], 1))
PY

echo "mutated $file"
echo "  - $search"
echo "  + $replace"
( cd services/api && go test -count=1 -run "$regex" "$pkg" ) >/tmp/mutate.$$.log 2>&1
rc=$?

restore
if cmp -s "$file" "$backup"; then echo "restored $file (byte-identical)"; else echo "RESTORE FAILED for $file"; exit 2; fi
trap - EXIT INT TERM   # restored and verified: the backup can go, and the trap must not run again
rm -f "$backup"

if [ $rc -ne 0 ]; then
  echo "RESULT: KILLED. The tests noticed the mutation:"
  grep -E '^(--- FAIL|FAIL|\s+--- FAIL)' /tmp/mutate.$$.log | head -8
  rm -f /tmp/mutate.$$.log; exit 0
fi
echo "RESULT: SURVIVED. No test failed, so nothing protects this line. Add a test that fails when it changes."
rm -f /tmp/mutate.$$.log; exit 1
