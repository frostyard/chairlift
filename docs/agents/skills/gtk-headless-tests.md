# Never put a `_test.go` in a package that imports puregotk

**When it applies:** Adding or reviewing any Go test anywhere in this repo,
especially for `internal/views`, `internal/window`, or `internal/app`.

**The hard constraint (read this twice):** puregotk resolves GTK / Libadwaita /
GLib / **graphene** shared libraries by `dlopen` **at package-init time** — it
panics during package initialization if the libraries (or their pkg-config
metadata) are absent. The mill's gate host and GitHub's CI runners are
**headless and have none of these libraries**. Therefore:

> A test binary for *any* package that imports puregotk — directly or
> transitively — **panics before a single test function runs**. It is not about
> what the test body does. Merely having a `_test.go` file in such a package
> builds a test binary whose startup dies with, e.g.:
>
> ```
> panic: Path for library: graphene not found ...
>   codeberg.org/puregotk/puregotk/v4/graphene/graphene-box.go:305
> FAIL github.com/frostyard/chairlift/internal/views
> ```

`internal/views`, `internal/window`, and `internal/app` all import puregotk, so
they must stay **test-free** (that is why they show `[no test files]`). This is
an existing repo convention — do not break it by adding a test there, no matter
how carefully the test avoids constructing widgets or calling
`sgtk.RunOnMainThread`. Guarding, nil-checking, or constructing a zero-value
`&UserHome{}` does **not** save you: the panic is at init, before your code.

**The LOCAL TRAP that hides this:** a dev machine with GTK/graphene installed
(e.g. via linuxbrew) will run these tests *green*, and `make ci` will pass
locally, because puregotk finds the `.so` files. CI then fails because it has
nothing to load. **Never trust local success for a test in a puregotk package.**
The only safe signal is: the package does not import puregotk at all.

**What to do instead — extract the pure logic:**

- Move the decidable, widget-free logic into a small package that imports **no
  puregotk** (only `fmt`, `strings`, domain packages like `internal/homebrew`,
  etc.), and unit-test it there. Example from issue #57: the untrusted-tap
  upgrade message lives in `internal/views/trustmsg` (imports only `fmt`) and is
  table-tested headlessly; `internal/views` calls `trustmsg.UpgradeMessage(...)`.
- Test in the puregotk-free package, following the model of the existing
  `internal/bootc` and `internal/homebrew` tests (pure functions, table-driven).
- Name tests `Test…` (not `TestI…`, no "Integration") so they run under CI's
  `-run "^Test[^I]" -skip "Integration"` filter. That filter is **not** an
  escape hatch for GTK-needing tests — a skipped test still lives in its
  package's binary, which still panics at init. Skipping does not help; moving
  the logic out does.

**Widget-bound methods are not headlessly unit-testable.** A guard like
`if uh.someExpander == nil { return }` on a method of a puregotk-holding struct
(`UserHome`) cannot be tested without importing `views`. Cover it by the fix
itself plus the compliance review — do not add a `views`-package test for it.
If a behavior genuinely needs a test, that is a signal to extract its decidable
core into a puregotk-free package.

**Learned from:** issue #57's first mill run — a `_test.go` added to
`internal/views` passed locally (linuxbrew had graphene) but panicked on CI at
graphene load, failing the Unit Tests and Race jobs. Fixed by extracting the
pure message to `internal/views/trustmsg` and removing the views-package test.
