#!/usr/bin/env bash
# Blocks writing secrets into the repository: private keys, cloud and GitHub tokens, real-shaped
# API tokens of this product, and .env files. Dev passwords like app:app are not secrets and are
# allowed. Docs use placeholders such as edu_pat_xxxxxxxx, which do not match.
payload=$(cat)
f=$(jq -r '.tool_input.file_path // empty' <<<"$payload")

case "$f" in
  */.env|*/.env.*) case "$f" in *.example) ;; *)
      echo "Blocked: do not write $f. Environment files with real values never enter the repo (.env.example documents names)." >&2
      exit 2 ;;
  esac ;;
esac

text=$(jq -r '(.tool_input.content // "") + "\n" + (.tool_input.new_string // "")' <<<"$payload")
scanner="$(dirname "$0")/../../scripts/secret-scan.sh"
if ! found=$("$scanner" <<<"$text"); then
  echo "Blocked: this content looks like a real secret ($f): ${found#found: }." >&2
  echo "Use a placeholder (edu_pat_xxxxxxxx) or read the value from the environment. If it is a real token, rotate it." >&2
  exit 2
fi
exit 0
