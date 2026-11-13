# `.memory/` - agent correction capture

This directory is ChairLift's durable, version-controlled store for learning
artifacts produced by AI-assisted work. Read it before changing the repository
so verified corrections and session findings carry forward instead of being
rediscovered.

Use this directory for deltas between what an agent believed and what the
repository, a command, or a maintainer established as true. General
architecture and operating rules still belong in `AGENTS.md`,
`docs/design/engineering-lessons.md`, or `docs/design/`; promote a correction there once it
becomes a stable rule.

## Corrections

Store each correction as 1 file in `.memory/corrections/`, creating the folder
when the first correction is recorded. One file holds one record (core
[ADR-0081](https://github.com/frostyard/core/blob/main/docs/adr/0081-store-each-correction-in-a-file-of-its-own.md)).
2 pull requests that each add a record add 2 different files, so they do not
conflict.

- **Name:** `<date>-<slug>.json`, matching
  `^(\d{4}-\d{2}-\d{2})-([a-z0-9]+(?:-[a-z0-9]+)*)\.json$`. The date part
  equals the record's `date`; the slug has at most 60 characters.
  - A record from a pull request: slug `pr-<N>-<words>`, for example
    `2026-11-13-pr-412-cache-key.json`. 2 records in 1 pull request use
    different words.
  - A record with no pull request: 2 to 5 words and 4 random hexadecimal
    characters, for example `2026-11-13-agent-scope-rule-3f9a.json`.
  - The slug prefix `legacy-` is reserved for the move from the old
    corrections file.
- **Content:** 1 JSON object with exactly 5 string fields, in this order:
  - `date` is the date the correction was established (`YYYY-MM-DD`).
  - `scope` identifies where the correction applies.
  - `correction` states both the mistaken belief and the verified reality.
  - `evidence` identifies how the correction was verified (`file:line`,
    command output, issue, or PR); do not use unsupported impressions.
  - `promoted_to` names the durable documentation or test containing the rule,
    or is the empty string `""` until promotion. Every value is a string
    (core ADR-0018).
- **Canonical form:** the file text is exactly
  `JSON.stringify(record, null, 2) + "\n"`, with the keys in the order
  above. Formatters do not rewrite these files.

  ```json
  {
    "date": "2026-11-13",
    "scope": "internal/views",
    "correction": "The prior belief and the verified reality.",
    "evidence": "frostyard/chairlift#412, cross-review of head 1a2b3c4",
    "promoted_to": "docs/design/engineering-lessons.md"
  }
  ```

- **Permanent:** a record file is never removed or renamed. Its text never
  changes, except that an empty `promoted_to` may be filled in once.
- The folder holds only record files, and an optional empty `.gitkeep`. No
  subfolder, no symbolic link.
- A blocking review point adds a record in the pull request that fixes it.

Do not record secrets, credentials, personal data, or unverified speculation
in this directory.

This repository has 1 form: the folder. The old single file does not come
back. Its records moved here as `legacy-` record files.

## Tools

- `make verify` runs `scripts/check-corrections.mjs`, which checks each record
  file. `make ci BASE=<ref>` adds `--base <ref>`: it also checks that no
  record is removed, renamed or changed, except a filled empty `promoted_to`,
  and that the old single file does not come back.
- Core's `new-correction.mjs` is an optional helper that writes 1 record file
  in canonical form
  ([source](https://github.com/frostyard/core/blob/main/scripts/new-correction.mjs)).
  Run it from a copy outside this repository; it is not vendored here. A
  file written by hand is checked the same way.
