---
name: learning-note
description: Write a short learning note linking a concept to the real code just written. Use after finishing a slice, or when the owner wants to learn a Go or engineering concept from this repo.
argument-hint: "[concept, e.g. context cancellation]"
disable-model-invocation: true
allowed-tools: Read Write Bash(ls *)
---
Write a learning note on: $ARGUMENTS

Create `docs/learning/go/NN-topic.md` (next number). Structure:
1. **What and why** in plain words (what problem the concept solves).
2. **In this repo**: link the exact files and lines where it appears.
3. **Minimal example** that compiles.
4. **Common mistakes** and how to spot them in review.
5. **Read next**: 2-3 references from `docs/learning/README.md`.
6. **Try it**: one small exercise on this codebase.
Add the note to the table in `docs/learning/README.md`. Write for an engineer new to Go but experienced in product and DevOps.
