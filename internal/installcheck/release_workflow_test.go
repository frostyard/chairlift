package installcheck

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The release and snapshot contracts below follow frostyard/core ADR-0055
// (frostyard/apt-publisher is the only writer of Frostyard's Debian metadata)
// and ADR-0056 (apt-publisher, not the producer, dispatches image rebuilds
// after the packages are live). Both workflows run only on tags or after
// main's Tests, so these tests are their pull-request gate.

type workflowStep struct {
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	If              string            `yaml:"if"`
	With            map[string]string `yaml:"with"`
	ContinueOnError yaml.Node         `yaml:"continue-on-error"`
}

type workflowJob struct {
	If          string            `yaml:"if"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
}

// actionSequence returns the action of every step, without its pinned ref, in
// step order. A step without `uses` (a `run` step) is reported as "run:".
func actionSequence(steps []workflowStep) []string {
	actions := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Uses == "" {
			actions = append(actions, "run:")
			continue
		}
		name, _, _ := strings.Cut(step.Uses, "@")
		actions = append(actions, name)
	}
	return actions
}

func TestActionSequenceStripsRefsAndMarksRunSteps(t *testing.T) {
	sha := strings.Repeat("a", 40)
	got := actionSequence([]workflowStep{
		{Uses: "actions/checkout@" + sha},
		{Run: "make build"},
		{Uses: "./.github/actions/local"},
	})
	want := []string{"actions/checkout", "run:", "./.github/actions/local"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("actionSequence = %v, want %v", got, want)
	}
}

type releaseWorkflow struct {
	On map[string]struct {
		Tags           []string `yaml:"tags"`
		Branches       []string `yaml:"branches"`
		BranchesIgnore []string `yaml:"branches-ignore"`
	} `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

func loadReleaseWorkflow(t *testing.T) (releaseWorkflow, workflowJob) {
	t.Helper()
	path := filepath.Join(".github", "workflows", "release.yml")
	var workflow releaseWorkflow
	if err := yaml.Unmarshal([]byte(readRepoFile(t, path)), &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(workflow.Jobs) != 1 {
		t.Fatalf("release workflow has %d jobs, want only goreleaser", len(workflow.Jobs))
	}
	job, ok := workflow.Jobs["goreleaser"]
	if !ok {
		t.Fatal("release workflow is missing the goreleaser job")
	}
	return workflow, job
}

// TestReleaseWorkflowRequestsAptPublicationForTags pins the tag release to
// exactly this sequence: build and upload the release, attest its assets, then
// ask frostyard/apt-publisher to publish the .deb files with an unguarded
// `publish-deb` repository_dispatch. Asserting the exact step sequence (rather
// than the absence of particular steps) also refuses any other publishing
// step, any direct image-build dispatch, and any run step.
func TestReleaseWorkflowRequestsAptPublicationForTags(t *testing.T) {
	workflow, job := loadReleaseWorkflow(t)

	if len(workflow.On) != 1 {
		t.Errorf("release workflow triggers on %d events, want only push", len(workflow.On))
	}
	push, ok := workflow.On["push"]
	if !ok || len(push.Tags) == 0 {
		t.Fatal("release workflow must run on tag pushes")
	}
	if len(push.Branches) != 0 || len(push.BranchesIgnore) != 0 {
		t.Errorf("release workflow push has branch filters %v %v, want a tag-only trigger", push.Branches, push.BranchesIgnore)
	}
	if job.If != "" {
		t.Errorf("goreleaser job has guard %q; every tag release must reach the publication request", job.If)
	}

	wantActions := []string{
		"actions/checkout",
		"actions/setup-go",
		"goreleaser/goreleaser-action",
		"actions/attest-build-provenance",
		"peter-evans/repository-dispatch",
	}
	if got := actionSequence(job.Steps); !reflect.DeepEqual(got, wantActions) {
		t.Fatalf("goreleaser job steps = %v, want exactly %v (attestation before the publication request, which is last)", got, wantActions)
	}

	request := job.Steps[len(job.Steps)-1]
	if request.If != "" {
		t.Errorf("publication request has guard %q; every tag release must reach it", request.If)
	}
	if !request.ContinueOnError.IsZero() {
		t.Error("publication request sets continue-on-error; a failed request must fail the release")
	}
	for key, want := range map[string]string{
		"token":      "${{ secrets.APT_PUBLISH_TOKEN }}",
		"repository": "frostyard/apt-publisher",
		"event-type": "publish-deb",
	} {
		if got := request.With[key]; got != want {
			t.Errorf("publication request with.%s = %q, want %q", key, got, want)
		}
	}
	if len(request.With) != 4 {
		t.Errorf("publication request with = %v, want only token, repository, event-type and client-payload", request.With)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(request.With["client-payload"]), &payload); err != nil {
		t.Fatalf("publication request client-payload %q is not a JSON object of strings: %v", request.With["client-payload"], err)
	}
	wantPayload := map[string]string{
		"repo": "${{ github.repository }}",
		"tag":  "${{ github.ref_name }}",
	}
	if !reflect.DeepEqual(payload, wantPayload) {
		t.Errorf("publication request client-payload = %v, want %v", payload, wantPayload)
	}
}

// TestReleaseWorkflowAttestsBuildProvenance pins the provenance apt-publisher
// verifies: the workflow may mint OIDC tokens and write attestations (and no
// more than it needs), and the attestation covers checksums.txt, the archives
// and every package format .goreleaser.yaml builds, including every .deb.
func TestReleaseWorkflowAttestsBuildProvenance(t *testing.T) {
	workflow, job := loadReleaseWorkflow(t)

	wantPermissions := map[string]string{
		"contents":     "write",
		"id-token":     "write",
		"attestations": "write",
	}
	if !reflect.DeepEqual(workflow.Permissions, wantPermissions) {
		t.Errorf("release workflow permissions = %v, want exactly %v", workflow.Permissions, wantPermissions)
	}
	if job.Permissions != nil {
		t.Errorf("goreleaser job overrides permissions with %v; keep the workflow-level set", job.Permissions)
	}

	var attest *workflowStep
	for i := range job.Steps {
		if strings.HasPrefix(job.Steps[i].Uses, "actions/attest-build-provenance@") {
			attest = &job.Steps[i]
		}
	}
	if attest == nil {
		t.Fatal("release workflow must run actions/attest-build-provenance over the release assets")
	}

	var got []string
	for _, line := range strings.Split(attest.With["subject-path"], "\n") {
		if line = strings.TrimSpace(line); line != "" {
			got = append(got, line)
		}
	}
	sort.Strings(got)

	wantSet := map[string]bool{"dist/checksums.txt": true, "dist/*.tar.gz": true}
	cfg := loadGoreleaserConfig(t)
	if len(cfg.Nfpms) == 0 {
		t.Fatal(".goreleaser.yaml has no nfpms entries")
	}
	for _, nfpm := range cfg.Nfpms {
		for _, format := range nfpm.Formats {
			wantSet["dist/*."+format] = true
		}
	}
	if !wantSet["dist/*.deb"] {
		t.Fatal(".goreleaser.yaml builds no .deb package; the APT publication contract expects one")
	}
	var want []string
	for path := range wantSet {
		want = append(want, path)
	}
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("attestation subject-path = %v, want exactly %v", got, want)
	}
}

// TestGoreleaserDebPackagesAreRegisteredForApt keeps the .deb assets of a
// release to the package names registered for frostyard/chairlift in
// apt-publisher's config/producers.tsv. apt-publisher downloads every .deb
// asset of the tag and refuses the whole release if any package name is not
// registered, so a new deb-producing nFPM entry needs a registration first.
func TestGoreleaserDebPackagesAreRegisteredForApt(t *testing.T) {
	registered := []string{fullPackageName, integrationPackageName}

	cfg := loadGoreleaserConfig(t)
	if len(cfg.Nfpms) == 0 {
		t.Fatal(".goreleaser.yaml has no nfpms entries")
	}
	var debs []string
	for _, nfpm := range cfg.Nfpms {
		for _, format := range nfpm.Formats {
			if format == "deb" {
				debs = append(debs, nfpm.PackageName)
			}
		}
	}
	sort.Strings(debs)
	if !reflect.DeepEqual(debs, registered) {
		t.Errorf(".deb package names = %v, want exactly %v (the names registered for frostyard/chairlift in frostyard/apt-publisher config/producers.tsv)", debs, registered)
	}
}

// TestSnapshotWorkflowPublishesOnlyTheDevPrerelease keeps nightly builds out of
// the APT repository and out of image rebuilds: the snapshot job builds the
// rolling `dev` prerelease and does nothing else. Image repositories install
// chairlift from the APT repository, which carries tag releases only, and
// apt-publisher dispatches their builds after it publishes one.
func TestSnapshotWorkflowPublishesOnlyTheDevPrerelease(t *testing.T) {
	path := filepath.Join(".github", "workflows", "snapshot.yml")
	var workflow struct {
		Jobs map[string]workflowJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, path)), &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(workflow.Jobs) != 1 {
		t.Fatalf("snapshot workflow has %d jobs, want only snapshot", len(workflow.Jobs))
	}
	job, ok := workflow.Jobs["snapshot"]
	if !ok {
		t.Fatal("snapshot workflow is missing the snapshot job")
	}

	wantActions := []string{
		"actions/checkout",
		"actions/setup-go",
		"goreleaser/goreleaser-action",
	}
	if got := actionSequence(job.Steps); !reflect.DeepEqual(got, wantActions) {
		t.Fatalf("snapshot job steps = %v, want exactly %v", got, wantActions)
	}
	if got, want := job.Steps[len(job.Steps)-1].With["args"], "release --nightly --clean"; got != want {
		t.Errorf("snapshot GoReleaser args = %q, want %q", got, want)
	}
}
