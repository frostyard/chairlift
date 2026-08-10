# Reconcile documentation

Use this prompt when behavior or configuration changed, or when documentation
may have drifted.

```text
Reconcile ChairLift's current-state documentation for [TOPIC].

Read AGENTS.md, docs/documentation-consistency.md, and every file in docs/agents/skills/. Treat README-go-port.md and docs/plans/ as historical, not as sources of current behavior.

Trace the topic to its live sources:
- config.yml and internal/config for page/group keys and defaults;
- internal/navigation and page builders for visibility and shortcuts;
- go.mod for dependency versions;
- Makefile, .goreleaser.yaml, helper constants, and PolicyKit files for commands and install paths.

Update every current-state restatement in README.md, CONFIG.md, docs/, and yeti/ that is relevant. Search for old terms and contradictory claims rather than only adding a new paragraph. Run the focused tests named by docs/documentation-consistency.md when applicable, then run make ci. Report the files reconciled and the source used to verify each factual claim.
```
