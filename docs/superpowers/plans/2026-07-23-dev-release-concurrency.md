# Dev Release Concurrency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent concurrent snapshot jobs from publishing duplicate assets to the rolling GitHub `dev` release.

**Architecture:** Use GitHub Actions workflow-level concurrency to serialize all publishers targeting the shared `dev` release. Do not cancel an in-progress publisher because interruption during release replacement or artifact upload could leave the rolling release incomplete.

**Tech Stack:** GitHub Actions, GoReleaser Pro

## Global Constraints

- Use the concurrency group name `chairlift-dev-release`.
- Set `cancel-in-progress` to `false`.
- Do not change GoReleaser artifact or nightly configuration.

---

### Task 1: Serialize Dev Release Publishers

**Files:**
- Modify: `.github/workflows/snapshot.yml:12-15`
- Modify: `yeti/OVERVIEW.md:216-217`

**Interfaces:**
- Consumes: GitHub Actions workflow concurrency semantics and the existing GoReleaser `nightly.tag_name: dev` configuration.
- Produces: A workflow in which only one `chairlift-dev-release` publisher runs at a time.

- [ ] **Step 1: Confirm the workflow currently has no concurrency declaration**

Run: `grep -n "^concurrency:" .github/workflows/snapshot.yml`

Expected: no output and exit status 1.

- [ ] **Step 2: Add workflow-level concurrency**

Add after the workflow trigger:

```yaml
concurrency:
  group: chairlift-dev-release
  cancel-in-progress: false
```

- [ ] **Step 3: Document the publishing invariant**

Update the CI/release section of `yeti/OVERVIEW.md` to state that snapshot publishers are serialized by the `chairlift-dev-release` concurrency group so duplicate rolling-release uploads cannot overlap.

- [ ] **Step 4: Validate the workflow configuration**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/snapshot.yml`

Expected: exit status 0 with no validation errors.

- [ ] **Step 5: Confirm overlapping behavior in GitHub Actions**

After the change reaches GitHub, trigger two snapshot runs close enough that their GoReleaser publishing phases would otherwise overlap. Confirm the second run remains pending until the first completes, then inspect both GoReleaser logs.

Expected: the runs do not execute concurrently, the retained runs succeed, and their logs contain no `ReleaseAsset`, `already_exists`, or upload-failure error. If a third run arrives while one is active and another is pending, GitHub may cancel the older pending run in favor of the newest one.
