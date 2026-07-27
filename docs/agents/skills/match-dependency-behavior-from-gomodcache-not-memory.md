# When a spec requires matching a dependency's exact parsing/semantics, read the dependency's source in `GOMODCACHE`, not memory

**When it applies:** Planning or reviewing any chunk whose acceptance
criteria say new code must behave "the same as," "consistent with," or
"matching" a third-party library's parsing, tagging, or resolution semantics
— e.g. yaml.v3's merge-key (`<<`) detection, tag normalization, duplicate-key
rules, or any other undocumented-but-load-bearing internal predicate. This
generalizes beyond YAML to any spec that asks new code to reproduce a Go
module dependency's exact behavior.

**What to do:** The dependency's actual source is already on disk and
trivially findable — `go env GOMODCACHE` (typically
`~/go/pkg/mod/<module>@<version>/`) — and is authoritative in a way that
memory or a summary of "how yaml.v3 handles merge keys" is not. For example,
yaml.v3's real merge-key predicate (`resolve.go`/`decode.go`) is `n.Kind ==
ScalarNode && n.Value == "<<" && (n.Tag == "" || n.Tag == "!" ||
shortTag(n.Tag) == mergeTag)`, where `mergeTag = "!!merge"` and `shortTag`
also normalizes the canonical long form `tag:yaml.org,2002:merge`. A plan
written from an approximate recollection of this (e.g. checking only
`Tag == "" || Tag == "!!merge"`) will look plausible but silently reject
valid explicitly-tagged merge keys, and reviewers will keep raising the same
class of objection — each with a slightly different missed edge (short tag
form, long tag form, duplicate-key precedence) — across multiple revision
rounds because each fix patches the symptom the previous round complained
about rather than the actual source predicate. Before drafting or revising
any chunk with a "must match library X" criterion, locate and read the
relevant function in `GOMODCACHE` directly and cite the exact predicate in
the plan; this resolves the whole class of objections in one round instead
of one round per missed edge case.

**Learned from:** issue #69 phase-1 validator mill run (second attempt) —
four consecutive `plan_revise` rounds all objected to variations of the same
merge-key tag-matching gap (missing canonical long-tag form, missing
duplicate-`<<`-key precedence relative to yaml.v3's unique-key check), each
fixed just enough to satisfy the prior objection's literal wording without
consulting yaml.v3's actual source, exhausting the plan-round limit and
causing the run to terminate as FAILED before any implementation began.
