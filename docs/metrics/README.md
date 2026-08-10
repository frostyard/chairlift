# Public metrics catalog

ChairLift publishes its engineering and contribution signals through public,
read-only services. This catalog points to live sources and reproducible metric
definitions instead of committing snapshots that become stale.

| Signal | Public source | Interpretation |
|---|---|---|
| Pull request acceptance | [Definition and reproducible 90-day query](../metrics.md) | Accepted and closed counts plus their ratio. It is descriptive, not a target. |
| CI results | [Tests workflow](https://github.com/frostyard/chairlift/actions/workflows/test.yml) | Lint, unit tests, race detection, verification, and cross-architecture builds for each run. |
| Nightly compliance | [Nightly workflow](https://github.com/frostyard/chairlift/actions/workflows/nightly-compliance.yml) | Default-branch CI, E2E, and vulnerability checks run on a schedule. |
| Test coverage | [Codecov](https://app.codecov.io/gh/frostyard/chairlift) | Coverage for the tested `internal/...` scope; consult the upload status before interpreting missing data. |
| Releases | [GitHub Releases](https://github.com/frostyard/chairlift/releases) | Published versions, timestamps, and release assets. |
| Review activity | [Pull requests](https://github.com/frostyard/chairlift/pulls) | Public review discussions, outcomes, checks, and merge history. |

The [quality dashboard](../quality.md) explains what each signal establishes,
which checks are enforced, and the limitations of the coverage and artifact
feeds. GitHub also exposes the underlying public repository data through its
[REST API](https://api.github.com/repos/frostyard/chairlift); authenticated
queries are recommended for higher rate limits.

## Agent observability boundary

These repository-wide signals include human, dependency-bot, and
agent-assisted contributions. ChairLift does not currently attach a reliable
provenance marker to every agent-assisted pull request, so account names,
branch names, and issue labels must not be used to manufacture an agent-only
metric. If explicit provenance is introduced, publish that segment alongside
the repository-wide result and document its collection rules here.

ChairLift does not collect application usage telemetry. This catalog describes
public development and quality signals only; it does not imply collection from
installed systems.
