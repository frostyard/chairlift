# Pull request metrics

ChairLift tracks pull request acceptance as an outcome signal for its
contribution and agent feedback loops. The metric is descriptive: it helps
maintainers investigate changes in review outcomes, but it is not a target and
must not be used to discourage valid experimental or difficult work.

## PR acceptance rate

The canonical metric uses a rolling 90-day cohort:

- **Cohort:** every pull request whose `closedAt` timestamp falls within the
  window, including merged pull requests.
- **Accepted:** a cohort pull request with a non-null `mergedAt` timestamp.
- **Not accepted:** a cohort pull request closed without merging.
- **Rate:** `accepted / closed * 100`. When no pull requests closed in the
  window, report the rate as unavailable rather than zero.

Always report the accepted and closed counts with the percentage. The counts
make small samples visible and prevent a high or low percentage from being
interpreted without its denominator. Open pull requests are excluded because
their outcome is not known. The cohort includes dependency and other automated
pull requests so the repository-wide result cannot drift based on subjective
authorship classification.

This is currently a repository-wide metric. ChairLift does not yet attach a
reliable provenance marker to every agent-assisted pull request, so branch
names, contributor accounts, or issue labels must not be used to infer an
agent-only cohort. If explicit PR provenance is introduced later, report that
segment alongside—not instead of—the repository-wide rate.

## Reproduce the metric

The following command queries GitHub directly and calculates the current
90-day value. It requires authenticated `gh`, GNU `date`, and `jq`:

```bash
since="$(date -u -d '90 days ago' +%F)"

gh pr list \
  --repo frostyard/chairlift \
  --state closed \
  --search "closed:>=$since" \
  --limit 1000 \
  --json number,closedAt,mergedAt |
  jq --arg window_start "$since" '
    . as $prs
    | ($prs | length) as $closed
    | ($prs | map(select(.mergedAt != null)) | length) as $accepted
    | {
        window_start: $window_start,
        accepted: $accepted,
        closed: $closed,
        acceptance_percent: (
          if $closed == 0 then null
          else (($accepted * 10000 / $closed) | round / 100)
          end
        )
      }
  '
```

If more than 1,000 pull requests close during a window, replace this
convenience query with paginated GitHub API collection before reporting the
metric; silently truncating the cohort is invalid.

## Interpretation

Review the metric as a trend over successive 90-day snapshots, not as a pass
or fail threshold. When it changes materially, inspect the underlying pull
requests and record concrete causes such as duplicate work, scope changes,
failed quality gates, or review findings. Acceptance alone does not measure
correctness: CI results, review findings, regressions, and time-to-feedback
remain necessary context, as described in [the quality dashboard](quality.md).
