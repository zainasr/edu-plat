#!/usr/bin/env bash
# Keeps Go files gofmt-clean after every edit.
f=$(jq -r '.tool_input.file_path // empty')
case "$f" in
  *.go) gofmt -w "$f" ;;
esac
exit 0
