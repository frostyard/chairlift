# Comparing or deduplicating yaml.v3 scalar keys must include the Tag, not just Value

**When it applies:** Writing or reviewing any Go code that walks `*yaml.Node`
trees (merge-key resolution, alias expansion, duplicate-key detection, map
flattening, etc.) and needs to decide whether two mapping keys are "the same
key." This applies any time a helper builds a key identity from a scalar
node — not just in `internal/config`, but anywhere yaml.v3 nodes are compared.

**What to do:** A `yaml.Node`'s `.Value` is the raw string form of a scalar,
so an explicit string key `"1"` and an implicit integer key `1` both have
`Value == "1"` but are semantically distinct YAML keys (different `.Tag`,
e.g. `!!str` vs `!!int`). Any key-identity function (`mappingKeyID`-style
helpers) that hashes or compares only `Value` will silently collide these,
letting one key mask or overwrite the other during merge/dedup — a real bug,
not just a style nit. Build key identity from `(Tag, Value)` (or the decoded
Kind) together, and add a test with two same-`Value`-different-`Tag` keys
(e.g. quoted `"1"` vs bare `1`) to prove they remain distinct. Reviewers will
flag a `Value`-only comparator as high severity and will keep flagging it
across revision rounds if a fix only adds a nil-guard or comment instead of
actually incorporating the tag.

**Learned from:** issue #69 phase-1 validator mill run — chunk 1's
`mappingKeyID` in `internal/config/resolve.go` compared merge/mapping keys by
`.Value` alone. The same high-severity objection ("integer 1 and string \"1\"
collide") was raised in review rounds 1, 2, and 3 without being fixed,
exhausting the review-round limit and causing the run to terminate as FAILED.
