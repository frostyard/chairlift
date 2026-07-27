# Splitting an oversized chunk means dividing it by concern, not carving off one narrow rule

**When it applies:** Revising a plan after a reviewer objects that a single
chunk is too large to review (e.g. it owns an entire validator, traversal, or
similar multi-concern implementation plus its tests, well past the review
budget) — especially validation/parsing logic that combines structural
traversal, multiple independent rule checks, and defensive guards in one
file.

**What to do:** A response that moves only one narrow, easily-isolated rule
(e.g. "reject unknown names") into its own chunk while leaving the rest of
the traversal, every other parse/type rule, alias/merge expansion, duplicate
handling, and all the denial-of-service guards bundled together does not
meaningfully reduce the size or reviewability of the remaining chunk — a
reviewer will reject it again for the same reason. Instead, split along real
seams in the implementation: separate the structural traversal/decoding step
from the semantic rule checks, separate alias/merge resolution from
post-resolution validation, and separate defensive/DoS guards into their own
chunk with their own regression tests. Each resulting chunk should be
independently reviewable — own a coherent piece of the design with its own
tests — rather than being defined by "everything except the one thing we just
pulled out."

**Learned from:** issue #69's mill run, plan round 4 — a schema-validation
chunk (~450 estimated lines of validate.go plus tests) was rejected as too
large; the plan's attempted fix moved only unknown-name rejection to a new
chunk while validate.go retained the full traversal, all other parse/type
checks, alias/merge expansion, duplicate handling, and three separate DoS
guards plus their tests — the reviewer treated this as not a real split.
