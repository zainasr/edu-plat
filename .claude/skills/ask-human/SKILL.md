---
name: ask-human
description: The human-in-the-loop protocol. Use whenever a decision belongs to the product owner, when the spec, the code and the owner's words disagree, or before anything hard to undo. Defines when to stop, how to ask, and how to record the answer.
argument-hint: "[the decision or conflict]"
allowed-tools: AskUserQuestion Read Grep
---
Decision needed: $ARGUMENTS

**Stop and ask (never decide alone):**
- Money rules, rounding, fees, discounts, refunds, anything that changes what a family pays.
- Security: a new public endpoint, token or cookie handling, a permission added or widened, who may see what.
- Spec change or conflict: the spec says one thing, the code or the owner says another (`docs/spec` is locked; changing it needs a DEC row).
- Scope: adding or removing a requirement of release 1.0.
- A new dependency, a breaking API change, a schema change that is hard to reverse, a destructive command (drop, delete, reset, force push).
- A real fork where both options are sound and cost the owner differently.

**Do not ask:** naming, file layout, formatting, or anything `CLAUDE.md`, the rules or an ADR already settles. State the assumption in one line and continue.

**How to ask.** Use `AskUserQuestion`: 2 to 4 options, your recommendation first and marked, each option with its trade-off in plain words. Make the question stand alone (name the files, IDs or numbers it is about), because the owner may see only the dialog.
For a **conflict**, quote both sides with their sources (`file:line`, DEC or REQ ID) and say which you would follow and why. Do not pick the side that is easier for you.

**While waiting.** Do the parts of the work that do not depend on the answer. Do not guess and build the blocked part.

**After the answer.** Record it where the next session will find it: an ADR for a technical choice, a DEC row plus the spec edit for a product decision, a line in the plan. Then say, in one sentence, what you will now do.

**If the owner is unavailable and the work is blocked**, finish everything else, open your final message with what is blocked and the exact question, and stop. Never route around a blocking decision by changing a rule, a test or a gate.
