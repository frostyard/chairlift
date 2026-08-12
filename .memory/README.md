# `.memory/` - agent correction capture

This directory is ChairLift's durable, version-controlled store for learning
artifacts produced by AI-assisted work. Read it before changing the repository
so verified corrections and session findings carry forward instead of being
rediscovered.

Use this directory for deltas between what an agent believed and what the
repository, a command, or a maintainer established as true. General
architecture and operating rules still belong in `AGENTS.md`,
`docs/agents/skills/`, or `docs/design/`; promote a correction there once it
becomes a stable rule.

## Corrections

Store corrections in `corrections.jsonl`, creating it when the first
correction is recorded. Keep it append-only, with one JSON object per line:

```json
{"date":"YYYY-MM-DD","scope":"repo-relative path or subsystem","correction":"the prior belief and verified reality","evidence":"file:line, command output, issue, or PR","promoted_to":null}
```

- `date` is the date the correction was established.
- `scope` identifies where the correction applies.
- `correction` states both the mistaken belief and the verified reality.
- `evidence` identifies how the correction was verified; do not use
  unsupported impressions.
- `promoted_to` names the durable documentation containing the rule, or is
  `null` until promotion.

Do not record secrets, credentials, personal data, or unverified speculation
in this directory.
