package agent

import (
	"strings"
	"testing"
	"time"
)

// Whether --wait is there decides whether an install against a control plane
// that does not exist yet finishes or hangs for the whole timeout: readiness is
// /health/ready, and the agent is not ready until it has registered.
func TestWaitingIsOnUnlessTurnedOff(t *testing.T) {
	opts := Options{Namespace: DefaultNamespace}

	got := strings.Join(installArgs(opts, "/values.yaml", time.Minute), " ")
	if !strings.Contains(got, "--wait") {
		t.Errorf("an install must wait for readiness by default:\n  %s", got)
	}

	opts.NoWait = true
	got = strings.Join(installArgs(opts, "/values.yaml", time.Minute), " ")
	if strings.Contains(got, "--wait") {
		t.Errorf("--wait survived NoWait, so the install would hang:\n  %s", got)
	}
	// And the rest of the command is unchanged by it.
	if !strings.Contains(got, "--install") || !strings.Contains(got, "--create-namespace") {
		t.Errorf("NoWait changed more than the waiting:\n  %s", got)
	}
}

// An unpinned chart takes whatever is newest, which is the right default for a
// cluster being set up today. A pinned one must reach helm.
func TestTheChartVersionIsPassedOnlyWhenPinned(t *testing.T) {
	unpinned := strings.Join(installArgs(Options{}, "/values.yaml", time.Minute), " ")
	if strings.Contains(unpinned, "--version") {
		t.Errorf("an empty chart version was sent as a pin:\n  %s", unpinned)
	}

	pinned := strings.Join(installArgs(Options{ChartVersion: "1.0.2"}, "/values.yaml", time.Minute), " ")
	if !strings.Contains(pinned, "--version 1.0.2") {
		t.Errorf("the pin did not reach helm:\n  %s", pinned)
	}
}

// The credential goes in a file, so the command line must carry its path and
// never the token itself.
func TestTheCommandLineCarriesTheValuesFileAndNotTheToken(t *testing.T) {
	opts := Options{Token: EnrollmentPrefix + "s3cret", Namespace: DefaultNamespace}

	got := strings.Join(installArgs(opts, "/tmp/x/values.yaml", time.Minute), " ")

	if !strings.Contains(got, "--values /tmp/x/values.yaml") {
		t.Errorf("the values file is not passed:\n  %s", got)
	}
	if strings.Contains(got, "s3cret") {
		t.Errorf("the token reached the command line:\n  %s", got)
	}
	if strings.Contains(got, "--set") {
		t.Errorf("--set puts values in the process list and the shell history:\n  %s", got)
	}
}
