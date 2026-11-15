#!/usr/bin/env bash
# Blocks float32/float64 in money code (ADR-0004: money is whole paisa in an int64).
# Zero-exception rule, so it is a hook, not a reminder. Exit 2 blocks the edit and shows stderr.
payload=$(cat)
f=$(jq -r '.tool_input.file_path // empty' <<<"$payload")
case "$f" in
  */services/api/internal/ledger/*.go|*/services/api/internal/fees/*.go|*/services/api/internal/payments/*.go|\
  */services/api/internal/accounting/*.go|*/services/api/internal/payroll/*.go|*/services/api/internal/platform/money/*.go) ;;
  *) exit 0 ;;
esac
text=$(jq -r '(.tool_input.content // "") + "\n" + (.tool_input.new_string // "")' <<<"$payload")
if grep -qE '\bfloat(32|64)\b' <<<"$text"; then
  echo "Blocked: float32/float64 in money code ($f)." >&2
  echo "Money is whole paisa in an int64 (money.Money, ADR-0004). Floats round wrongly: 0.1 + 0.2 != 0.3." >&2
  echo "For a percentage, use integer basis points and integer division with an explicit rounding rule." >&2
  exit 2
fi
exit 0
