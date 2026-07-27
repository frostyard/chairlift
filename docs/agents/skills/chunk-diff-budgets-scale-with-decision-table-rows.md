# Chunk diff-size budgets must scale with decision-table row count, not a flat per-chunk guess

**When it applies:** Planning or executing a chunk whose acceptance criteria
require test coverage for every applicable cell of a decision table (e.g. one
schema-name/shape/declared-type assertion per validation level, or one row
per collection entry), while the plan also states a flat estimated line count
(e.g. "≈ 300–400 lines") for that chunk.

**What to do:** A flat per-chunk line estimate made before enumerating the
table's applicable cells is close to meaningless once the cells are counted:
each cell typically needs its own fixture, assertion block, and often a
`Path`/line check, so the real diff size is roughly (applicable cells ×
lines-per-assertion-block) plus the shared traversal/dispatch code, not a
constant. Before committing to a chunk boundary, count the exact cells that
chunk owns (using the "yes" cells of the applicable-category matrix, not the
full N×M product) and multiply by the measured cost of one existing
assertion block in the same file/style. If that product already approaches
or exceeds the plan's stated budget, split the chunk along a real seam
(e.g. by schema level, or structural-walk vs. field-decode) before
implementation starts, rather than discovering the overrun only when the
gate rejects an oversized diff.

**Learned from:** issue #69 phase1c1 mill run, chunk 3 revision round 1 — the
plan estimated the actions/action-field chunk at ≈ 390 lines, but the actual
diff was 634 changed lines (608 additions), rejected by the reviewer as far
exceeding the 300–400-line scope; the overrun traced directly to the number
of decision-table cells (schema-name, shape, declared-type across multiple
levels) the chunk's tests needed to cover.
