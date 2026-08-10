# Implement an issue

Use this prompt for a scoped ChairLift feature, fix, or maintenance issue.

```text
Implement [ISSUE OR TASK] in ChairLift.

Before editing:
1. Read AGENTS.md, every file in docs/agents/skills/, and the relevant current-state documentation.
2. Inspect the existing implementation and tests; do not infer behavior from historical plans.
3. State the intended change and identify affected repository invariants.

While implementing:
- Keep the GTK main thread free of external tool calls and marshal UI updates through sgtk.RunOnMainThread.
- Preserve the fixed PolicyKit/helper privilege boundary; do not introduce arbitrary privileged execution.
- Respect config-driven group visibility and nil-guard cross-group widgets.
- Put non-UI logic in a pure package where it can be tested headlessly.
- Add regression tests that exercise the actual failure mode and all affected collection entries.
- Update current-state documentation when behavior, configuration, dependencies, or install layout changes.

Before finishing:
1. Run focused tests while iterating.
2. Run make ci.
3. Review git diff and git status for accidental or missing files.
4. Summarize behavior changed, tests run, and any remaining limitation.

Do not weaken tests or repository invariants to make the change pass.
```
