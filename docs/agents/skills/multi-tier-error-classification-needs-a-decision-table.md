# A spec introducing multi-tier error classification (parse/schema/value, etc.) needs one explicit decision table before any chunk is written

**When it applies:** Planning or reviewing any spec that introduces more than
one error "Kind" for a single failure surface — e.g. distinguishing a
KindParse (document isn't valid YAML/JSON at all) from a KindSchema (document
parses but has the wrong shape: wrong node type, unknown key, missing
required key) from a KindValue error. Also applies whenever the classification
must stay stable across several chunks that each add tests for different
input shapes.

**What to do:** Before splitting the work into chunks, write down the exact
boundary as a table of input shapes to Kind, covering every edge case a
reviewer could construct: a scalar where a mapping is expected, a null node,
an empty document, an unknown key, a YAML alias resolving to a valid type,
etc. Only after that table is fixed should later chunks add acceptance tests
per Kind — otherwise chunk N's test ("a bare scalar document must be
KindParse") and chunk N+1's test ("any non-mapping page/group/action value
must be KindSchema") silently assume different boundaries for the same kind
of malformed input, and reviewers reject the plan for internal
inconsistency. When a later chunk's classification decision requires
touching a test file whose behavior changes as a result (e.g.
`config_test.go`), list that test file explicitly in that chunk's file list;
don't assume "pre-existing tests keep passing unmodified" is compatible with
also tightening what they test for.

**Learned from:** issue #69's mill run on `internal/config` schema/authority
loading — plan rounds 1-4 were rejected on this same axis repeatedly (round 3:
schema validation authority; round 4: parse-vs-schema boundary for
non-mapping document/page/group/action structures), exhausting the
`plan_rounds` limit (4) with the run terminating as FAILED because no round
ever pinned the classification boundary down before writing per-chunk tests.
