# Assertions on a generated "line N" detail must parse and check N, not just check the substring is present

**When it applies:** Writing or reviewing a test that asserts an error
message/detail contains a dynamically computed line number (e.g. a validator
or parser error like `"unknown page (line 3)"`), especially in table-driven
tests generated across many rows/levels where the same assertion helper is
reused.

**What to do:** Don't assert only `strings.Contains(detail, "line ")` (or an
equivalent loose substring/regex with no capture). That passes even when the
implementation emits a wrong, non-positive, or hardcoded value (e.g.
`"line 0"` or `"line -1"`), silently defeating the assertion's actual
purpose — proving the reported line matches the offending node. Extract the
number (regex capture or `fmt.Sscanf`) and assert it is a specific expected
value, or at minimum `> 0`, for every row that claims to check "a positive
line number." When a single assertion helper is shared across many
table-driven cases, verify it actually fails for a deliberately-wrong
fixture (line 0, negative, or omitted) before trusting it across the whole
matrix.

**Learned from:** issue #69 phase1c1 mill run, chunk 3 revision round 1 — a
reviewer objection found that several `Path`/line assertions in
`validate_test.go` only checked for the substring `"line "`, which would
silently accept `"line 0"` or a negative value at multiple table rows (row
12, row 14, and the validator-shape path matrix).
