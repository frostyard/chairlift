# Derive schema/validation rules from the canonical struct, not from a shadow parse representation

**When it applies:** Planning or reviewing any chunk that validates a
config/data file's shape (e.g. rejecting unknown keys, checking required
fields, enumerating valid top-level names) by walking an intermediate
representation — a `rawConfig` map, a `yaml.Node` tree, a generic
`map[string]interface{}` — instead of the Go struct (e.g. `Config`) that the
spec names as authoritative.

**What to do:** An intermediate representation built and maintained
separately from the canonical struct can drift from it: a field added to
`Config` isn't automatically reflected in `rawConfig`'s key list, so legitimate
fields get rejected (or unknown ones silently accepted) without any planned
test catching it, because the test was written against the same drifting
intermediate representation. Derive validation sets (page names, required
keys, allowed keys) directly from the canonical struct — via reflection,
generation, or a single hand-maintained list colocated with the struct and
covered by a test asserting the two stay in sync — rather than introducing a
second source of truth that has to be kept in lockstep by hand across future
changes.

**Learned from:** issue #69's mill run on `internal/config` — plan round 3 was
rejected (high severity) because chunk c3 derived and tested top-level page
keys from `rawConfig` while the spec defined `Config` as authoritative,
meaning legitimate `Config` fields could be rejected or omitted without the
planned acceptance test failing.
