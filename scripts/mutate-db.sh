#!/usr/bin/env bash
# Mutation check for DATABASE rules (triggers, grants, RLS): break one rule on the LOCAL dev
# database and confirm a test notices, then put it back and confirm the tests pass again.
#
#   scripts/mutate-db.sh 'MUTATE SQL' 'RESTORE SQL' TEST_REGEX
#
# Example:
#   scripts/mutate-db.sh 'GRANT DELETE ON students TO app_rt' 'REVOKE DELETE ON students FROM app_rt' 'TestINV_TheAPIRole'
#
# Exit: 0 = KILLED and restored cleanly, 1 = SURVIVED, 2 = harness error (including a failed restore).
# Safety: only runs against a local database, and always runs the restore SQL on every exit path.
# Do not use it for statements that insert rows: append-only tables cannot be cleaned afterwards.
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH="$HOME/go/bin:$PATH"
: "${ADMIN_DATABASE_URL:=postgresql://app:app@localhost:5432/app_dev}"
: "${DATABASE_URL:=postgresql://app_rt:app_rt@localhost:5432/app_dev}"
export ADMIN_DATABASE_URL DATABASE_URL

[ $# -eq 3 ] || { sed -n '2,13p' "$0"; exit 2; }
mutate_sql=$1; restore_sql=$2; regex=$3

case "$ADMIN_DATABASE_URL" in
  *@localhost:*|*@127.0.0.1:*) ;;
  *) echo "refusing: ADMIN_DATABASE_URL is not a local database"; exit 2;;
esac

restored=0
restore() { [ "$restored" = 1 ] && return; psql "$ADMIN_DATABASE_URL" -qc "$restore_sql" >/dev/null 2>&1 && restored=1; }
trap restore EXIT INT TERM

psql "$ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 -qc "$mutate_sql" || { echo "the mutation SQL failed"; exit 2; }
echo "mutated:  $mutate_sql"

( cd services/api && go test -count=1 -run "$regex" ./internal/integration/ ) >/tmp/mutate-db.$$.log 2>&1
rc=$?

psql "$ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 -qc "$restore_sql" || { echo "RESTORE FAILED: run this by hand: $restore_sql"; exit 2; }
restored=1
echo "restored: $restore_sql"

( cd services/api && go test -count=1 -run "$regex" ./internal/integration/ ) >/tmp/mutate-db2.$$.log 2>&1 \
  || { echo "after the restore the tests still fail:"; tail -15 /tmp/mutate-db2.$$.log; rm -f /tmp/mutate-db*.$$.log; exit 2; }

if [ $rc -ne 0 ]; then
  echo "RESULT: KILLED (tests failed under the mutation, passed after the restore). The rule is protected:"
  grep -E '^(--- FAIL|FAIL)' /tmp/mutate-db.$$.log | head -6
  rm -f /tmp/mutate-db*.$$.log; exit 0
fi
echo "RESULT: SURVIVED. Nothing failed while the rule was broken. Add a test that checks it."
rm -f /tmp/mutate-db*.$$.log; exit 1
