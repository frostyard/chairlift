# Tests must not touch GTK/GLib — the gate and CI are headless

**When it applies:** Writing or reviewing any Go test under `internal/views`,
`internal/window`, `internal/app`, or anywhere that constructs GTK/Libadwaita
widgets or calls back onto the GTK main thread.

**The constraint:** ChairLift uses puregotk, which resolves GTK/Libadwaita/GLib
symbols by `dlopen` **at runtime**. The build is pure Go and needs no native
libraries — but *running* any GTK code does. The mill's gate host and GitHub's
CI runners have **no display and no GTK/GLib shared libraries installed**.
Therefore any test that reaches GTK will fail or crash the gate, and there is no
way to "install GTK" to route around it — headless is the target environment,
not an accident.

Two specific tripwires:

1. **Widget constructors** — `adw.NewExpanderRow()`, `gtk.NewButtonWithLabel()`,
   `adw.NewPreferencesGroup()`, etc. Each calls into dlopen'd GTK on first use.
2. **`sgtk.RunOnMainThread(fn)`** — schedules `fn` via `glib.IdleAdd`, which
   calls into dlopen'd GLib immediately (before any loop runs) and needs a
   running GLib main loop to ever execute `fn`. Neither exists in a test. So a
   test that reaches a `RunOnMainThread` call fails there, and `fn`'s body never
   runs even if it would.

**What to do:**

- Test **logic and guards**, never rendering. Construct a zero-value
  `&UserHome{}` (its widget fields are nil pointers — that allocates nothing in
  GTK) and call the method under test. Assert it returns / does not panic.
- Make nil-guards the **first statement** in a view method, so the guarded call
  short-circuits *before* any widget constructor or `RunOnMainThread`. A guard
  placed after a `RunOnMainThread` call is untestable headless and useless as a
  regression test — the test would die at the `RunOnMainThread` before reaching
  the guard.
- Keep the pure decision separate from the GTK effect where practical: a small
  predicate (`func (uh *UserHome) shouldX() bool`) or an early
  `if uh.someExpander == nil { return }` is directly unit-testable; a decision
  buried inside a `RunOnMainThread` closure is not.
- Name regression tests `Test…` (not `TestI…`) and keep "Integration" out of the
  name, so they run under CI's `-run "^Test[^I]" -skip "Integration"` filter.
  Do **not** hide a required regression behind the integration filter to make it
  "pass" — a skipped test protects nothing.
- The existing GTK-free tests in `internal/bootc` and `internal/homebrew`
  (parser and dry-run logic) are the model: pure functions, table-driven, no
  widgets.

**Why it matters:** A regression test that constructs the real page (or whose
guard sits behind `RunOnMainThread`) turns green intent into a gate deadlock —
the chunk can never pass the headless test gate no matter how correct the fix
is. Design the production code so the guard is reachable and assertable without
a live GTK runtime.
