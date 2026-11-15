#!/usr/bin/env bash
# Tests the Claude hooks by feeding them the JSON Claude Code sends and checking the exit code.
# Exit 2 means "block"; 0 means "allow". Run it after ANY change to .claude/hooks or settings.json:
#   make test-hooks
set -uo pipefail
cd "$(dirname "$0")/.."
ROOT=$PWD
H=$ROOT/.claude/hooks
pass=0; fail=0

# expect NAME WANT HOOK JSON [ENV...]
expect() {
  local name=$1 want=$2 hook=$3 json=$4; shift 4
  local out got
  out=$(env CLAUDE_PROJECT_DIR="$ROOT" HOOK_SKIP_CHECK=1 "$@" "$H/$hook" <<<"$json" 2>&1); got=$?
  if [ "$got" = "$want" ]; then pass=$((pass+1)); else
    fail=$((fail+1)); echo "FAIL  $name: want exit $want, got $got"; echo "      $out" | head -3
  fi
}

write() { jq -nc --arg f "$1" --arg c "$2" '{tool_name:"Write",tool_input:{file_path:$f,content:$c}}'; }
edit()  { jq -nc --arg f "$1" --arg c "$2" '{tool_name:"Edit",tool_input:{file_path:$f,new_string:$c}}'; }
shell() { jq -nc --arg c "$1" '{tool_name:"Bash",tool_input:{command:$c}}'; }

API=$ROOT/services/api

# ---- guard-paths (existing hook) ---------------------------------------------------------------
expect "paths: generated code is blocked"       2 guard-paths.sh "$(edit "$API/internal/api/gen/api.gen.go" x)"
expect "paths: existing migration is blocked"   2 guard-paths.sh "$(edit "$API/migrations/00001_walking_skeleton.sql" x)"
expect "paths: new migration file is allowed"   0 guard-paths.sh "$(write "$API/migrations/09999_new.sql" x)"
expect "paths: ordinary Go file is allowed"     0 guard-paths.sh "$(edit "$API/internal/fees/rules.go" x)"

# ---- guard-money ---------------------------------------------------------------------------------
expect "money: float64 in ledger is blocked"    2 guard-money.sh "$(write "$API/internal/ledger/x.go" 'var total float64')"
expect "money: float32 in fees is blocked"      2 guard-money.sh "$(edit  "$API/internal/fees/x.go" 'x := float32(1)')"
expect "money: float64 in the Money type"       2 guard-money.sh "$(write "$API/internal/platform/money/x.go" 'var f float64')"
expect "money: float64 outside money code ok"   0 guard-money.sh "$(write "$API/internal/org/x.go" 'var ratio float64')"
expect "money: the word float alone is allowed" 0 guard-money.sh "$(write "$API/internal/fees/x.go" '// no floating point near money')"
expect "money: int64 paisa is allowed"          0 guard-money.sh "$(write "$API/internal/fees/x.go" 'var total int64')"

# ---- guard-migration -----------------------------------------------------------------------------
good_mig='-- +goose Up
CREATE TABLE things (id uuid PRIMARY KEY, tenant_id uuid NOT NULL);
ALTER TABLE things ENABLE ROW LEVEL SECURITY;
-- +goose Down
DROP TABLE things;'
expect "migration: valid file is allowed"            0 guard-migration.sh "$(write "$API/migrations/09999_x.sql" "$good_mig")"
expect "migration: no goose Up marker is blocked"    2 guard-migration.sh "$(write "$API/migrations/09999_x.sql" 'CREATE TABLE a (id int);')"
expect "migration: tenant table without RLS"         2 guard-migration.sh "$(write "$API/migrations/09999_x.sql" $'-- +goose Up\nCREATE TABLE a (id uuid, tenant_id uuid);')"
expect "migration: DROP TABLE in Up is blocked"      2 guard-migration.sh "$(write "$API/migrations/09999_x.sql" $'-- +goose Up\nDROP TABLE a;')"
expect "migration: DROP COLUMN in Up is blocked"     2 guard-migration.sh "$(write "$API/migrations/09999_x.sql" $'-- +goose Up\nALTER TABLE a DROP COLUMN b;')"
expect "migration: TRUNCATE in Up is blocked"        2 guard-migration.sh "$(write "$API/migrations/09999_x.sql" $'-- +goose Up\nTRUNCATE a;')"
expect "migration: GRANT DELETE is blocked"          2 guard-migration.sh "$(write "$API/migrations/09999_x.sql" $'-- +goose Up\nGRANT SELECT, DELETE ON a TO app_rt;')"
expect "migration: REVOKE DELETE is allowed"         0 guard-migration.sh "$(write "$API/migrations/09999_x.sql" $'-- +goose Up\nREVOKE DELETE ON a FROM app_rt;\n-- +goose Down\nGRANT DELETE ON a TO app_rt;')"
expect "migration: DROP in the Down section is ok"   0 guard-migration.sh "$(write "$API/migrations/09999_x.sql" $'-- +goose Up\nCREATE TABLE a (id int);\n-- +goose Down\nDROP TABLE a;')"
expect "migration: non-migration file is ignored"    0 guard-migration.sh "$(write "$API/internal/x.sql" 'DROP TABLE a;')"

# ---- guard-secrets -------------------------------------------------------------------------------
long="Qx9kLm2pR7vTz4WnB8yHc1JdF5sGa3UeO6iYtMr0XqjKLMNOP"
tok="edu_pat_${long:0:43}"
expect "secrets: private key is blocked"        2 guard-secrets.sh "$(write "$ROOT/docs/x.md" '-----BEGIN RSA PRIV''ATE KEY-----')"
expect "secrets: AWS key is blocked"            2 guard-secrets.sh "$(write "$ROOT/docs/x.md" 'key=AKI''AABCDEFGHIJKLMNOP')"
expect "secrets: real-shaped PAT is blocked"    2 guard-secrets.sh "$(write "$ROOT/docs/x.md" "token: $tok")"
expect "secrets: .env file is blocked"          2 guard-secrets.sh "$(write "$ROOT/.env" 'A=1')"
expect "secrets: .env.example is allowed"       0 guard-secrets.sh "$(write "$ROOT/.env.example" 'A=')"
expect "secrets: placeholder token is allowed"  0 guard-secrets.sh "$(write "$ROOT/docs/x.md" 'edu_pat_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx')"
expect "secrets: dev database URL is allowed"   0 guard-secrets.sh "$(write "$ROOT/docs/x.md" 'postgresql://app:app@localhost:5432/app_dev')"

# ---- guard-git -----------------------------------------------------------------------------------
expect "git: force push is blocked"                     2 guard-git.sh "$(shell 'git push --force origin main')"
expect "git: short -f force push is blocked"            2 guard-git.sh "$(shell 'git push -f origin main')"
expect "git: force-with-lease is blocked"               2 guard-git.sh "$(shell 'git push --force-with-lease')"
expect "git: +refspec force push is blocked"            2 guard-git.sh "$(shell 'git push origin +main')"
expect "git: deleting a remote branch is blocked"       2 guard-git.sh "$(shell 'git push origin --delete old')"
expect "git: --no-verify is blocked"                    2 guard-git.sh "$(shell 'git commit --no-verify -m x')"
expect "git: reset --hard is blocked"                   2 guard-git.sh "$(shell 'git reset --hard HEAD~1')"
expect "git: clean -fd is blocked"                      2 guard-git.sh "$(shell 'git clean -fd')"
expect "git: checkout . is blocked"                     2 guard-git.sh "$(shell 'git checkout .')"
expect "git: stash drop is blocked"                     2 guard-git.sh "$(shell 'git stash drop')"
expect "git: branch -D is blocked"                      2 guard-git.sh "$(shell 'git branch -D feature')"
expect "git: chained force push is blocked"             2 guard-git.sh "$(shell 'make check && git push -f')"
expect "git: normal push is allowed (check skipped)"    0 guard-git.sh "$(shell 'git push origin main')"
expect "git: status is allowed"                         0 guard-git.sh "$(shell 'git status --short')"
expect "git: commit is allowed"                         0 guard-git.sh "$(shell 'git commit -m "feat: add x"')"
expect "git: a message that mentions git push --force"  0 guard-git.sh "$(shell 'git commit -m "docs: git push --force and git reset --hard are blocked"')"
expect "git: a heredoc message that mentions them"      0 guard-git.sh "$(shell $'git commit -m "$(cat <<\'EOF\'\nfix\n\nnever run git reset --hard or git push -f\nEOF\n)"')"
expect "git: non-git command is ignored"               0 guard-git.sh "$(shell 'rm -f /tmp/x; echo git push --force')"

# push gate with a REAL check: a sandbox whose `make check` fails, then one that passes
sb=$(mktemp -d)
printf 'check:\n\t@exit 1\n' > "$sb/Makefile"
out=$(CLAUDE_PROJECT_DIR="$sb" "$H/guard-git.sh" <<<"$(shell 'git push origin main')" 2>&1); got=$?
[ "$got" = 2 ] && pass=$((pass+1)) || { fail=$((fail+1)); echo "FAIL  git: push is blocked when make check fails (got $got)"; }
printf 'check:\n\t@true\n' > "$sb/Makefile"
out=$(CLAUDE_PROJECT_DIR="$sb" "$H/guard-git.sh" <<<"$(shell 'git push origin main')" 2>&1); got=$?
[ "$got" = 0 ] && pass=$((pass+1)) || { fail=$((fail+1)); echo "FAIL  git: push is allowed when make check passes (got $got)"; }
rm -f "$sb/Makefile"; rmdir "$sb"

# ---- stop-check, in a throwaway git repo ----------------------------------------------------------
sb=$(mktemp -d)
(
  cd "$sb" && git init -q && git config user.email t@t && git config user.name t
  mkdir -p scripts services .claude/state
  cp "$ROOT/scripts/fingerprint.sh" "$ROOT/scripts/stamp.sh" scripts/
  echo base > services/a.go; git add -A && git commit -qm base
)
stop() { env CLAUDE_PROJECT_DIR="$sb" "$H/stop-check.sh" <<<"$1" >/dev/null 2>&1; echo $?; }
chk() { [ "$1" = "$2" ] && pass=$((pass+1)) || { fail=$((fail+1)); echo "FAIL  stop: $3 (want $2, got $1)"; }; }
chk "$(stop '{}')" 0 "a clean tree may stop"
echo changed >> "$sb/services/a.go"
chk "$(stop '{}')" 2 "changed code with no passing check is blocked"
chk "$(stop '{"stop_hook_active":true}')" 0 "a second stop attempt is allowed (no loop)"
CLAUDE_PROJECT_DIR="$sb" "$sb/scripts/stamp.sh"
chk "$(stop '{}')" 0 "changed code after a passing check may stop"
echo more >> "$sb/services/a.go"
chk "$(stop '{}')" 2 "a further change invalidates the stamp"
echo doc > "$sb/NOTES.md"
git -C "$sb" checkout -q -- services/a.go
chk "$(stop '{}')" 0 "a docs-only change outside the code paths may stop"
rm -rf "$sb" 2>/dev/null || true

# ---- session-start -------------------------------------------------------------------------------
out=$(CLAUDE_PROJECT_DIR="$ROOT" "$H/session-start.sh" 2>&1); got=$?
if [ "$got" = 0 ] && grep -q "Repo state" <<<"$out" && grep -q "/dev-context" <<<"$out"; then pass=$((pass+1)); else fail=$((fail+1)); echo "FAIL  session-start: exit $got, output: $out" | head -3; fi

echo "hooks: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
