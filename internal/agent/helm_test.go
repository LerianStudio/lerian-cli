package agent

import (
	"context"
	"os"
	"path/filepath"
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

// Helm's own output is a chart's NOTES.txt in full — a page of headings and
// example commands that buries the one line saying what happened. On failure it
// is exactly what somebody needs, so it has to survive into the error rather
// than being swallowed with the stream.
func TestAFailureCarriesWhatHelmSaid(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'Error: UPGRADE FAILED: no deployed releases' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "helm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	helm := &Helm{binary: filepath.Join(dir, "helm")}
	err := helm.run(context.Background(), "upgrade")

	if err == nil {
		t.Fatal("a failing helm returned nil")
	}
	if !strings.Contains(err.Error(), "no deployed releases") {
		t.Errorf("what helm said was lost:\n%v", err)
	}
}

// With no Out set, what helm printed is kept rather than shown — the caller
// prints a summary instead. Asserted through the capture rather than through
// the terminal: proving nothing reached os.Stdout would mean redirecting it,
// and the behavior that matters is that the output is held and available.
func TestASuccessKeepsTheOutputWithoutAnOut(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'NOTES: a page of headings'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "helm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	helm := &Helm{binary: filepath.Join(dir, "helm")}
	err := helm.run(context.Background(), "upgrade")

	// It failed, so the kept output surfaces — which is the only way to observe
	// that it was kept at all.
	if err == nil || !strings.Contains(err.Error(), "a page of headings") {
		t.Errorf("the output was not kept with no Out set: %v", err)
	}
}

// Out is still honored for a caller that wants the stream as it happens.
func TestTheStreamIsHonoredWhenAsked(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'live output'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "helm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	helm := &Helm{binary: filepath.Join(dir, "helm"), Out: &out}
	if err := helm.run(context.Background(), "upgrade"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "live output") {
		t.Errorf("a caller that asked for the stream got nothing: %q", out.String())
	}
}
