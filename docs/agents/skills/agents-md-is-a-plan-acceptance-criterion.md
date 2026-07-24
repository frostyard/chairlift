# A chunk that changes what AGENTS.md asserts must list AGENTS.md itself in `files`

**When it applies:** Planning or reviewing any chunk that changes a fixed
path, default, or invariant that AGENTS.md's "Repository invariants" or
"Build, test, lint" sections currently describe in prose — e.g. the
`chairlift-updex-helper` install path, the `pkexec` targets, the default
`make install` `PREFIX`, or any other fact AGENTS.md states as fact rather
than pointing to code for.

**What to do:** AGENTS.md's own documentation rule ("after any change to
source code, update relevant documentation in AGENTS.md, README.md, and the
`yeti/` folder") is enforced by plan review as a real, per-chunk acceptance
criterion — not just a closing-checklist reminder. Updating README.md and
`yeti/` while leaving AGENTS.md's prose making the old claim (a bare helper
name, an unstated default prefix, etc.) will be rejected even though README
and yeti were both updated, because AGENTS.md itself is now stale and it is
one of the three named locations. When drafting a chunk's `files` list, check
each changed invariant against AGENTS.md's current wording specifically (not
just "did I update docs somewhere") and add AGENTS.md whenever the chunk
changes a fact AGENTS.md states — but don't pad an unrelated chunk with an
AGENTS.md edit it doesn't need just to be safe; "relevant" is the bar, not
"every chunk touches every doc."

**Learned from:** issue #59's mill run, plan round 2 — a chunk changed the
updex helper invocation to a fixed `/usr/bin/chairlift-updex-helper` path and
changed the supported source-install prefix, updating README.md and yeti/ but
omitting AGENTS.md from its `files`/acceptance criteria. The reviewer rejected
the plan because AGENTS.md's privilege-boundary and build/install text were
now directly stale, even though the other two doc locations were current.
