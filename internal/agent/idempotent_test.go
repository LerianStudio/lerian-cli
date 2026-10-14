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
