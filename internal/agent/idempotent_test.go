package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helmSaying puts a script named helm on PATH that answers each subcommand.
func helmSaying(t *testing.T, values, chart string) *Helm {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1 $2" in
  "get values") cat <<'JSON'
` + values + `
JSON
    ;;
  "show chart") cat <<'YAML'
` + chart + `
YAML
    ;;
esac
exit 0
`
	path := filepath.Join(dir, "helm")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Helm{binary: path}
}

func deployedOpts() Options {
	return Options{
		Namespace:         DefaultNamespace,
		ControlPlaneURL:   "https://control.example.com",
		Token:             EnrollmentPrefix + "abc",
		ManagedNamespaces: []string{"lerian-apps"},
	}
}

// The same install run twice must do nothing the second time.
func TestAnIdenticalInstallIsUnchanged(t *testing.T) {
	opts := deployedOpts()
	installed, err := jsonOf(opts.Values())
	if err != nil {
		t.Fatal(err)
	}
	helm := helmSaying(t, installed, "version: 1.0.1")

	unchanged, err := helm.Unchanged(context.Background(), opts,
		Release{Chart: "agent-helm-1.0.1", Status: "deployed"})
	if err != nil {
		t.Fatal(err)
	}

	if !unchanged {
		t.Error("the same values and the same chart were reported as a change")
	}
}

// A different value is a change, and must not be skipped.
func TestDifferentValuesAreAChange(t *testing.T) {
	opts := deployedOpts()
	other := deployedOpts()
	other.ManagedNamespaces = []string{"somewhere-else"}
	installed, err := jsonOf(other.Values())
	if err != nil {
		t.Fatal(err)
	}
	helm := helmSaying(t, installed, "version: 1.0.1")

	unchanged, err := helm.Unchanged(context.Background(), opts,
		Release{Chart: "agent-helm-1.0.1", Status: "deployed"})
	if err != nil {
		t.Fatal(err)
	}

	if unchanged {
		t.Error("a different managed namespace was treated as no change")
	}
}

// "Newest" is only a version once the registry is asked. A release installed
// last week can be a version behind without anything here having changed, and
// reporting that as "nothing to do" leaves an agent stale while saying so.
func TestANewerChartIsAChange(t *testing.T) {
	opts := deployedOpts()
	installed, err := jsonOf(opts.Values())
	if err != nil {
		t.Fatal(err)
	}
	helm := helmSaying(t, installed, "version: 1.1.0")

	unchanged, err := helm.Unchanged(context.Background(), opts,
		Release{Chart: "agent-helm-1.0.1", Status: "deployed"})
	if err != nil {
		t.Fatal(err)
	}

	if unchanged {
		t.Error("a release a version behind was reported as up to date")
	}
}

// A pinned version is compared against the pin rather than against the
// registry: somebody who pinned does not want the newest.
func TestAPinnedVersionIsComparedAgainstThePin(t *testing.T) {
	opts := deployedOpts()
	opts.ChartVersion = "1.0.1"
	installed, err := jsonOf(opts.Values())
	if err != nil {
		t.Fatal(err)
	}
	// The registry has something newer, which must not matter.
	helm := helmSaying(t, installed, "version: 1.1.0")

	unchanged, err := helm.Unchanged(context.Background(), opts,
		Release{Chart: "agent-helm-1.0.1", Status: "deployed"})
	if err != nil {
		t.Fatal(err)
	}

	if !unchanged {
		t.Error("a release already on the pinned version was reported as a change")
	}
}

// A failed release is not a state worth preserving: re-running is how somebody
// recovers from one.
func TestAFailedReleaseIsAlwaysAChange(t *testing.T) {
	opts := deployedOpts()
	installed, err := jsonOf(opts.Values())
	if err != nil {
		t.Fatal(err)
	}
	helm := helmSaying(t, installed, "version: 1.0.1")

	unchanged, err := helm.Unchanged(context.Background(), opts,
		Release{Chart: "agent-helm-1.0.1", Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}

	if unchanged {
		t.Error("a failed release was left alone, so re-running could not fix it")
	}
}

// The version is read out of what the chart reports.
func TestTheNewestChartVersionIsRead(t *testing.T) {
	helm := helmSaying(t, "{}", "apiVersion: v2\nname: agent-helm\nversion: 1.2.3\nappVersion: 1.2.3")

	got, err := helm.NewestChart(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", got)
	}
}

func jsonOf(values map[string]any) (string, error) {
	rendered, err := roundTrip(values)
	if err != nil {
		return "", err
	}
	encoded, err := marshal(rendered)
	return encoded, err
}

// settled stands in for a cluster that already holds this exact release.
type settled struct {
	release   Release
	unchanged bool
	ran       *bool
}

func (s settled) Install(context.Context, Options, time.Duration) error {
	*s.ran = true
	return nil
}
func (s settled) KubeContext() string { return "" }
func (s settled) Installed(context.Context, string) (Release, bool, error) {
	return s.release, true, nil
}
func (s settled) Unchanged(context.Context, Options, Release) (bool, error) {
	return s.unchanged, nil
}

// A test of Unchanged proves Unchanged works and says nothing about whether
// the install consults it — the gap a mutation deleting the skip walked
// straight through.
func TestInstallSkipsHelmWhenNothingWouldChange(t *testing.T) {
	var ran bool
	helm := settled{Release{Chart: "agent-helm-1.0.1", Status: "deployed"}, true, &ran}

	outcome, err := InstallReporting(context.Background(), helm, deployedOpts(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if ran {
		t.Error("helm was run for an install that would change nothing")
	}
	if outcome != Unmodified {
		t.Errorf("outcome = %v, want Unmodified", outcome)
	}
}

// And when something would change, it runs and says it reconfigured rather
// than installed: the release was already there.
func TestInstallReportsAReconfiguration(t *testing.T) {
	var ran bool
	helm := settled{Release{Chart: "agent-helm-1.0.1", Status: "deployed"}, false, &ran}

	outcome, err := InstallReporting(context.Background(), helm, deployedOpts(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if !ran {
		t.Error("a changed install did not reach helm")
	}
	if outcome != Reconfigured {
		t.Errorf("outcome = %v, want Reconfigured", outcome)
	}
}

// "Move it to another chart version" with no other version to move to rewrites
// the release for nothing — and against an agent that cannot become ready, it
// waits the whole timeout before saying so.
func TestAnUpgradeToTheSameVersionIsNotNeeded(t *testing.T) {
	helm := helmSaying(t, "{}", "version: 1.0.1")

	needed, newest, err := helm.UpgradeNeeded(context.Background(), "",
		Release{Chart: "agent-helm-1.0.1"})
	if err != nil {
		t.Fatal(err)
	}

	if needed {
		t.Error("an upgrade to the version already installed was reported as needed")
	}
	if newest != "1.0.1" {
		t.Errorf("newest = %q", newest)
	}
}

// And a release behind the registry does need one.
func TestAnUpgradeToANewerVersionIsNeeded(t *testing.T) {
	helm := helmSaying(t, "{}", "version: 1.1.0")

	needed, newest, err := helm.UpgradeNeeded(context.Background(), "",
		Release{Chart: "agent-helm-1.0.1"})
	if err != nil {
		t.Fatal(err)
	}

	if !needed {
		t.Error("a release a version behind was reported as up to date")
	}
	if newest != "1.1.0" {
		t.Errorf("newest = %q", newest)
	}
}

// A pinned version is compared against the pin, and the registry is not asked:
// somebody who pinned does not want whatever is newest.
func TestAPinnedUpgradeIgnoresTheRegistry(t *testing.T) {
	helm := helmSaying(t, "{}", "version: 9.9.9")

	needed, newest, err := helm.UpgradeNeeded(context.Background(), "1.0.1",
		Release{Chart: "agent-helm-1.0.1"})
	if err != nil {
		t.Fatal(err)
	}

	if needed {
		t.Error("a release already on the pinned version was reported as needing an upgrade")
	}
	if newest != "1.0.1" {
		t.Errorf("it reported the registry's version rather than the pin: %q", newest)
	}
}

// Readiness means registered. An agent pointed at the placeholder is not ready
// and will not become ready by being given a new chart, so waiting for it
// spends the whole timeout and reports the state it was already in — which is
// exactly what an upgrade did.
func TestAPlaceholderReleaseIsRecognized(t *testing.T) {
	values := `{"agent":{"configmap":{"CONTROL_PLANE_URL":"` + PlaceholderControlPlane + `"}}}`
	helm := helmSaying(t, values, "version: 1.0.1")

	if !helm.PointsAtPlaceholder(context.Background(), DefaultNamespace) {
		t.Error("a release pointed at the placeholder was not recognized")
	}
}

// A real control plane is not a placeholder, so waiting for readiness is right.
func TestARealControlPlaneIsNotAPlaceholder(t *testing.T) {
	values := `{"agent":{"configmap":{"CONTROL_PLANE_URL":"https://control.example.com"}}}`
	helm := helmSaying(t, values, "version: 1.0.1")

	if helm.PointsAtPlaceholder(context.Background(), DefaultNamespace) {
		t.Error("a real control plane was taken for the placeholder")
	}
}

// `helm show chart` prints the whole Chart.yaml, dependencies included, and a
// dependency carries a version line of its own — indented, but the first one in
// the file. Matching the first "version:" read a subchart's 2.2.0 as the agent
// chart's, and an upgrade was offered to a version that does not exist.
func TestTheChartVersionIsNotASubchartVersion(t *testing.T) {
	chart := `apiVersion: v2
name: agent-helm
dependencies:
  - name: common
    repository: https://charts.example.com
    version: 2.2.0
description: The Lerian agent
type: application
version: 1.0.1
appVersion: "1.0.1"`
	helm := helmSaying(t, "{}", chart)

	got, err := helm.NewestChart(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if got != "1.0.1" {
		t.Errorf("version = %q, want the chart's own 1.0.1 rather than a dependency's", got)
	}
}

// Confirming an operation that turns out to write nothing is a question nobody
// needed, and what is already installed is what somebody wants to see before
// saying yes. So the check has to be available before the install runs.
func TestPlanInstallReportsWithoutWriting(t *testing.T) {
	var ran bool
	helm := settled{Release{Chart: "agent-helm-1.0.1", Status: "deployed"}, true, &ran}

	outcome, existing, err := PlanInstall(context.Background(), helm, deployedOpts())
	if err != nil {
		t.Fatal(err)
	}

	if ran {
		t.Error("planning wrote to the cluster")
	}
	if outcome != Unmodified {
		t.Errorf("outcome = %v, want Unmodified", outcome)
	}
	if existing.Chart != "agent-helm-1.0.1" {
		t.Errorf("it did not report what is installed: %+v", existing)
	}
}

// Over something already there, it is a reconfiguration rather than an
// install — and the caller needs the existing release to show what changes.
func TestPlanInstallNamesAReconfiguration(t *testing.T) {
	var ran bool
	helm := settled{Release{Chart: "agent-helm-1.0.1", Revision: "3", Status: "deployed"}, false, &ran}

	outcome, existing, err := PlanInstall(context.Background(), helm, deployedOpts())
	if err != nil {
		t.Fatal(err)
	}

	if ran {
		t.Error("planning wrote to the cluster")
	}
	if outcome != Reconfigured {
		t.Errorf("outcome = %v, want Reconfigured", outcome)
	}
	if existing.Revision != "3" {
		t.Errorf("the existing revision was not reported: %+v", existing)
	}
}

// With nothing installed there is nothing to show and nothing to compare.
func TestPlanInstallOnAnEmptyNamespaceIsAnInstall(t *testing.T) {
	var ran bool
	helm := empty{&ran}

	outcome, existing, err := PlanInstall(context.Background(), helm, deployedOpts())
	if err != nil {
		t.Fatal(err)
	}

	if outcome != Installed {
		t.Errorf("outcome = %v, want Installed", outcome)
	}
	if existing.Chart != "" {
		t.Errorf("it invented a release: %+v", existing)
	}
}

// empty stands in for a namespace with no release in it.
type empty struct{ ran *bool }

func (e empty) Install(context.Context, Options, time.Duration) error { *e.ran = true; return nil }
func (e empty) KubeContext() string                                   { return "" }
func (e empty) Installed(context.Context, string) (Release, bool, error) {
	return Release{}, false, nil
}
func (e empty) Unchanged(context.Context, Options, Release) (bool, error) { return false, nil }
