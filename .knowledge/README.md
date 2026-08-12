# `.knowledge/` - cross-session knowledge index

This directory is ChairLift's stable discovery point for knowledge that should
survive an individual agent session. It indexes the repository's canonical
artifacts instead of copying their contents, which prevents guidance from
drifting between stores.

## Read before working

1. Read `AGENTS.md` for current repository invariants and required workflows.
2. Read every file in `docs/agents/skills/` for durable lessons from prior
   automated runs.
3. Read `.memory/README.md` and `.memory/corrections.jsonl`, when present, for
   verified corrections.
4. Read `.claude/session-summary.md` for the latest in-progress handoff.
5. Read `docs/design/overview.md` and the relevant `docs/design/` subsystem
   documents (formerly `yeti/`) for architecture and decision rationale.

## Record knowledge in its canonical location

- Put verified corrections in `.memory/corrections.jsonl`.
- Replace `.claude/session-summary.md` when another session must continue
  active work.
- Promote stable operating rules to `AGENTS.md`, reusable lessons to
  `docs/agents/skills/`, architecture to `docs/design/`, and repo-local
  decision rationale to `docs/adr/`.

Keep this file as an index rather than a duplicate knowledge store. Record only
verified repository context, and never commit secrets, credentials, personal
data, or unverified speculation.
