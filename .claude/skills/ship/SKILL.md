---
name: ship
description: Commit and push verified work safely, then watch CI. Use when a slice is built and /verify says ship. Commits are small and conventional; push needs the owner's approval; force pushes are never done.
argument-hint: "[what is being shipped]"
disable-model-invocation: true
allowed-tools: Bash(git *) Bash(make *) Bash(scripts/*) Bash(gh run *) Read
---
Ship: $ARGUMENTS

**Pre-flight (all must be true).** `/verify` returned ship; `make check` passes on the current tree; `git status --short` shows only files that belong to this change (leave unrelated changes alone and say so); no secrets (the hook and `make evaluate` check).

**1. Sync.** `git fetch origin`. If behind: `git pull --rebase origin main` (a rebase conflict is a human decision: stop and explain). Re-run `make check` after any pull that changed code.

**2. Commit small.** One idea per commit, in dependency order, each one building and passing on its own (a good test: `git worktree add <scratch dir> HEAD~1` and `go build ./... && go vet ./...` there). Message: `type(scope): summary` (feat, fix, test, ci, chore, docs, refactor), then a short body saying what and why, then the attribution trailer from the session's reminder. Stage files by path, never `git add -A` over unrelated work. Never `--no-verify`.

**3. Ask before pushing.** State what will go out (`git log --oneline origin/main..HEAD`) and that the checks passed, and get the owner's yes in this conversation. A yes for an earlier push does not cover this one. The permission prompt for `git push` is a second gate, not a replacement.

**4. Push.** `git push origin main`. The guard hook runs `make check` first and blocks a red push. Never force. Never delete remote branches without asking.

**5. Watch CI.** `gh run list --limit 1` then `gh run watch <id> --exit-status`. Report the result honestly. If it fails: read `gh run view <id> --log-failed`, fix forward in a new commit, run `/verify`, ship again. Do not rewrite pushed history.

**6. Report**: commits pushed (hash and subject), CI result with run link, and what is still open.
