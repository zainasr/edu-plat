#!/usr/bin/env bash
# Reads text on stdin. Exit 1 if it contains something that looks like a real secret, 0 if not.
# It prints only the KIND of secret it found, never the secret, so logs stay clean.
# Shared by the guard-secrets hook and the evaluation gate, so both agree on what a secret is.
#
# Placeholders are fine: edu_pat_xxxxxxxx...x is one repeated character, a real token is random.
# Dev passwords such as app:app are not secrets.
text=$(tr -d "\000")

scan() { # scan LABEL REGEX
  grep -qE -- "$2" <<<"$text" && { echo "found: $1"; exit 1; }
}
scan "a private key"          '-----BEGIN [A-Z ]*PRIVATE KEY-----'
scan "an AWS access key"      'AKIA[0-9A-Z]{16}'
scan "a GitHub token"         'ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{40,}'
scan "an API secret key"      'sk-[A-Za-z0-9]{32,}'

# This product's own tokens: only a token with more than one distinct character counts.
if grep -oE -- 'edu_(pat|session)_[A-Za-z0-9_-]{43}' <<<"$text" \
   | sed -E 's/^edu_(pat|session)_//' | grep -qvE '^(.)\1*$'; then
  echo "found: an edu_ access token"; exit 1
fi
exit 0
