# A spec requiring errors "anywhere reachable" to be rejected is not satisfied by resolving precedence before validating

**When it applies:** Planning or reviewing a chunk that implements
precedence/override resolution (merge keys, config overlays, priority-based
selection) where the spec separately requires malformed data or
resource-limit violations to be rejected wherever they are reachable in the
input — including values a later, higher-priority source will end up
discarding.

**What to do:** Don't plan traversal as "pick the winning value per key,
then validate/emit just the winners" — that order skips validation of
losing branches entirely, silently accepting malformed or over-limit data
that never surfaces in the result. When the spec says errors must be caught
"anywhere reachable" or similar, the plan must describe (and a test must
cover) walking every branch that participates in precedence resolution —
including ones a later source overrides — for error/limit checks, even
though only the winning branch's value is ever emitted. Where the spec is
ambiguous about which specific resource limits apply to discarded branches
(e.g. a limit defined in terms of the emitted result vs. one defined in
terms of anything parsed), call that out explicitly as an interpretation
rather than picking silently, since reviewers will flag the ambiguity as a
"note"-severity objection if it isn't addressed.

**Learned from:** issue #69 phase-1b mill run, plan review round 2 — the
plan's c5/c6 chunks tested duplicate/cycle/limit errors only for winning
values, but the spec required merge-losing values (participants in `<<`
precedence that a later key or document overrides) to be rejected too when
malformed or over resource limits, even though their content never appears
in the effective result.
