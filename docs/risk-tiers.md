# Change risk tiers

Classify every pull request before review by choosing the highest tier that
matches any part of the change. The tier reflects potential impact rather than
diff size or author: a one-line permission change can be more consequential
than a large documentation update.

| Tier | Typical changes | Required handling |
|---|---|---|
| **Tier 1 - Low** | Documentation, comments, test fixtures, or metadata that cannot change executable, build, release, configuration, or workflow behavior | Normal review and the relevant repository checks |
| **Tier 2 - Moderate** | Routine implementation, tests, or configuration changes that stay outside security-sensitive and operational boundaries | Required CI, regression coverage for changed behavior, and normal code review |
| **Tier 3 - High** | Build or CI behavior, packaging, dependency additions or major upgrades, configuration schemas, external command execution, concurrency, persistent state, or broad validation changes | Tier 2 controls plus targeted failure-path validation, a reviewer familiar with the affected area, and deployment or rollback notes when applicable |
| **Tier 4 - Critical** | PolicyKit or root operations, destructive system mutations, release publication or signing, secrets and token permissions, execution derived from untrusted input, or automation that can approve or merge changes | Tier 3 controls plus explicit maintainer security review, threat or abuse-case analysis, a rollback plan, and confirmation that permissions remain least-privilege |

## Classification rules

- Choose the highest applicable tier; do not average multiple kinds of change.
- Treat changes to a safeguard at the same tier as the behavior it protects.
- Raise the tier when impact is uncertain until the uncertainty is resolved.
- Update the pull request classification if its scope changes during review.
- Never lower a tier to bypass required review or validation.

Record the selected tier and a short rationale in the pull request template.
Reviewers confirm that the classification and evidence match the final diff
before approval.
