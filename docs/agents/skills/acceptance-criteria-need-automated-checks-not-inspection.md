# "Verified by inspection" does not satisfy a spec's testing acceptance criterion

**When it applies:** Drafting or revising a plan chunk where the spec's
acceptance criteria say something must be "tested," "tested for consistency,"
or otherwise verified — and part of that surface (a config file, a generated
package layout, a second code path) is already correct today, tempting a plan
to mark it as satisfied by manual inspection rather than by adding it to the
gated check.

**What to do:** If an acceptance criterion asks for a consistency check
across multiple sources of truth (e.g. "the nFPM/package layout and the
source-install layout must agree"), every part of that criterion needs a real
executable, gated check — not a plan note like "`.goreleaser.yaml` is already
correct, verified by inspection, not modified." Inspection notes don't run in
CI and don't fail when the underlying file later drifts, so a reviewer will
treat them as leaving the acceptance criterion uncovered and reject the plan.
When part of the surface is already correct, that's a reason to write a check
that currently passes without needing a code fix — not a reason to skip
writing the check. Scope the chunk to add the automated assertion even for
the "already fine" half of a consistency requirement.

**Learned from:** issue #59's mill run, plan round 1 — the plan added an
automated Makefile/DESTDIR install-path check but handled the nFPM package
layout only by inspection, claiming `.goreleaser.yaml` was "already correct."
The reviewer rejected the plan because the spec's nFPM consistency criterion
had no concrete check behind it. Round 2 fixed it by adding
`TestGoreleaserNfpmLayoutMatchesUsrPrefix`, a real gated test parsing the live
`.goreleaser.yaml`.
