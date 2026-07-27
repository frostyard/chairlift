# A layered validator's ordering/scope constraints can make two acceptance criteria mutually unsatisfiable — trace one concrete input through the whole pipeline before accepting the plan

**When it applies:** Planning or reviewing a chunk that implements a
multi-stage validation pipeline where an early stage performs synthetic
shape checks (e.g. "reject a non-mapping page value") before falling through
to a later, delegated stage (e.g. a library `Decode` call) — and separate
acceptance criteria (a) require a specific error signature (a wrapped
library error type, reachable via `errors.As`) that only the delegated stage
can produce, while (b) scope that chunk's own test file to assert nothing
about errors originating below the level the early stage already checked.

**What to do:** A decision table fixing the Kind boundary (see
`multi-tier-error-classification-needs-a-decision-table.md`) is necessary but
not sufficient. Once the table is fixed, pick one concrete malformed input
per criterion that names a specific mechanism (an error type, a wrapped
error, a call-stack shape) and hand-trace it through the pipeline in the
exact stage order the spec/chunk defines. If the early stage's synthetic
checks would catch that input before the delegated stage ever runs — or if
the only inputs that reach the delegated stage are barred from assertion by
a separate scope rule in the same chunk — the criteria are unsatisfiable as
written and the plan must change which chunk owns that input/assertion
before writing any code, not after gates fail.

**Learned from:** issue #69 phase1c mill run — plan round 4's chunk c1
required a final whole-document `Decode` failure classified `KindParseType`
with `errors.As` reaching `*yaml.TypeError` (criterion 26), while a separate
criterion (31) forbade `validate_test.go` from asserting any `ErrorKind`
produced below page level. Because c1's own page-shape checks intercept
top-level scalar/sequence failures synthetically (with no `*yaml.TypeError`)
before `Decode` runs, no input could satisfy both criteria at once; this was
only discovered in round 4 of 4, exhausting `plan_rounds` and failing the run.
