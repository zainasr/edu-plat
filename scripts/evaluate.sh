#!/usr/bin/env bash
# Independent evaluation of the working tree. Exit 0 only if EVERY gate passes.
#
#   make evaluate            run all gates, print a table, show the log of anything that failed
#   scripts/evaluate.sh -q   quiet: table only
#
# It does not trust cached results (go test -count=1) and it regenerates code to prove the committed
# generated files are current. It never edits tracked files (it runs `make gen` and checks nothing
# changed) and it creates and drops one scratch database.
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH="$HOME/go/bin:$PATH"
: "${ADMIN_DATABASE_URL:=postgresql://app:app@localhost:5432/app_dev}"
: "${DATABASE_URL:=postgresql://app_rt:app_rt@localhost:5432/app_dev}"
export ADMIN_DATABASE_URL DATABASE_URL

quiet=0; [ "${1:-}" = "-q" ] && quiet=1
logdir=$(mktemp -d)
trap 'rm -rf "$logdir"' EXIT
fail=0
rows=()

gate() { # gate NAME COMMAND...
  local name=$1; shift
  if "$@" >"$logdir/$name.log" 2>&1; then
    rows+=("PASS  $name")
  else
    rows+=("FAIL  $name"); fail=1
  fi
}

gen_hash() { find services/api/internal -path '*/gen/*' -name '*.go' | sort | xargs sha256sum | sha256sum | cut -c1-16; }

gate_database() { psql "$ADMIN_DATABASE_URL" -Atc "select 1" >/dev/null || { echo "Database is down. Run: make db-up && make db-roles && make migrate"; return 1; }; }

gate_migrations_applied() {
  local latest applied
  latest=$(ls services/api/migrations/*.sql | sed -E 's#.*/0*([0-9]+)_.*#\1#' | sort -n | tail -1)
  applied=$(psql "$ADMIN_DATABASE_URL" -Atc "select coalesce(max(version_id),0) from goose_db_version")
  [ "$latest" = "$applied" ] || { echo "latest migration file is $latest but the database is at $applied. Run: make migrate"; return 1; }
}

gate_gofmt() { local out; out=$(cd services/api && gofmt -l .); [ -z "$out" ] || { echo "unformatted:"; echo "$out"; return 1; }; }
gate_vet() { (cd services/api && go vet ./...); }
gate_lint() { (cd services/api && golangci-lint run ./...); }

gate_generated_fresh() {
  local before after
  before=$(gen_hash)
  make gen >/dev/null || return 1
  after=$(gen_hash)
  [ "$before" = "$after" ] || { echo "make gen changed generated code. Commit the regenerated files."; return 1; }
}

gate_tests() { (cd services/api && go test -count=1 ./...); }
gate_invariants() { (cd services/api && go test -count=1 -run 'TestINV_' ./internal/integration/); }

gate_migrations_from_scratch() {
  local name="eval_scratch_$$" base url rc=0
  base="${ADMIN_DATABASE_URL%/*}"
  url="$base/$name"
  psql "$ADMIN_DATABASE_URL" -qc "CREATE DATABASE $name" || return 1
  goose -dir services/api/migrations postgres "$url" up || rc=1
  psql "$ADMIN_DATABASE_URL" -qc "DROP DATABASE $name" || rc=1
  return $rc
}

gate_no_hard_deletes() {
  local hits
  hits=$(grep -rniE 'delete +from' services/api/internal --include='*.sql' --include='*.go' | grep -v '_test.go' | grep -v '/gen/' || true)
  [ -z "$hits" ] || { echo "DELETE statements in application code (DEC-011):"; echo "$hits"; return 1; }
}

gate_no_float_money() {
  local hits
  hits=$(grep -rnE '\bfloat(32|64)\b' services/api/internal/ledger services/api/internal/fees services/api/internal/platform/money 2>/dev/null || true)
  [ -z "$hits" ] || { echo "float in money code (ADR-0004):"; echo "$hits"; return 1; }
}

gate_no_secrets() {
  local f hits=""
  while IFS= read -r f; do
    [ -f "$f" ] || continue
    scripts/secret-scan.sh < "$f" >/dev/null || hits="$hits$f"$'\n'
  done < <(git ls-files --cached --others --exclude-standard)  # tracked and new files, so nothing escapes by being uncommitted
  [ -z "$hits" ] || { echo "files that look like they contain a secret:"; printf '%s' "$hits"; return 1; }
}

gate_hooks() { scripts/test-hooks.sh; }

gate database               gate_database
gate migrations-applied     gate_migrations_applied
gate gofmt                  gate_gofmt
gate vet                    gate_vet
gate lint                   gate_lint
gate generated-code-fresh   gate_generated_fresh
gate tests                  gate_tests
gate invariants             gate_invariants
gate migrations-from-scratch gate_migrations_from_scratch
gate no-hard-deletes        gate_no_hard_deletes
gate no-float-money         gate_no_float_money
gate no-secrets             gate_no_secrets
gate hooks                  gate_hooks

echo
echo "EVALUATION"
printf '  %s\n' "${rows[@]}"
echo

if [ "$fail" -ne 0 ] && [ "$quiet" -eq 0 ]; then
  for r in "${rows[@]}"; do
    case "$r" in FAIL*)
      name=${r#FAIL  }
      echo "---- $name (last 30 lines) ----"
      tail -30 "$logdir/$name.log"
      echo
    esac
  done
fi

if [ "$fail" -eq 0 ]; then echo "RESULT: PASS (all gates)"; else echo "RESULT: FAIL"; fi
exit "$fail"
