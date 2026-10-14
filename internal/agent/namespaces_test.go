package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeKubectl puts a script named kubectl at the front of PATH and returns the
// file its invocations are recorded in.
func fakeKubectl(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.txt")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n" + body
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func calls(t *testing.T, log string) string {
	t.Helper()
	body, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// The chart places a Role in each managed namespace, and Kubernetes has nowhere
// to put one in a namespace that does not exist. An install against a fresh
// cluster failed on a namespace the operator was never asked about — so making
// them is this command's job, precisely because choosing them is.
func TestTheManagedNamespacesAreCreated(t *testing.T) {
	log := fakeKubectl(t, "exit 0\n")

	if err := EnsureNamespaces(context.Background(), "", []string{"lerian-apps", "other"}); err != nil {
		t.Fatal(err)
	}

	got := calls(t, log)
	for _, want := range []string{"create namespace lerian-apps", "create namespace other"} {
		if !strings.Contains(got, want) {
			t.Errorf("kubectl was not asked to create it: %q in\n%s", want, got)
		}
	}
}

// Already there is the expected case on every run after the first, and is not a
// failure.
func TestANamespaceThatExistsIsNotAFailure(t *testing.T) {
	fakeKubectl(t, `echo 'Error from server (AlreadyExists): namespaces "lerian-apps" already exists' >&2
exit 1
`)

	if err := EnsureNamespaces(context.Background(), "", []string{"lerian-apps"}); err != nil {
		t.Errorf("an existing namespace was treated as a failure: %v", err)
	}
}

// Anything else is: a namespace that could not be made is a namespace the chart
// will fail on, three minutes later and with a worse message.
func TestARealFailureIsReported(t *testing.T) {
	fakeKubectl(t, `echo 'Error from server (Forbidden): namespaces is forbidden' >&2
exit 1
`)

	err := EnsureNamespaces(context.Background(), "", []string{"lerian-apps"})
	if err == nil {
		t.Fatal("a namespace that could not be created was accepted")
	}
	if !strings.Contains(err.Error(), "Forbidden") {
		t.Errorf("the error does not carry what kubectl said: %v", err)
	}
}

// The cluster chosen, not whatever is current: creating a namespace in the
// wrong cluster is as wrong as installing into it.
func TestTheNamespacesAreCreatedInTheChosenCluster(t *testing.T) {
	log := fakeKubectl(t, "exit 0\n")

	if err := EnsureNamespaces(context.Background(), "arn:aws:eks:us-east-2:1:cluster/x", []string{"ns"}); err != nil {
		t.Fatal(err)
	}

	if got := calls(t, log); !strings.Contains(got, "--context arn:aws:eks:us-east-2:1:cluster/x") {
		t.Errorf("the chosen cluster did not reach kubectl: %s", got)
	}
}

// Nothing to create means kubectl is not needed at all, so a machine without it
// can still install an agent that manages nothing.
func TestNoNamespacesNeedsNoKubectl(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if err := EnsureNamespaces(context.Background(), "", nil); err != nil {
		t.Errorf("an install with no managed namespaces demanded kubectl: %v", err)
	}
}

// And when it is needed and missing, the message says what for and how to get
// it, rather than "executable file not found in $PATH".
func TestMissingKubectlSaysWhatItIsFor(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := EnsureNamespaces(context.Background(), "", []string{"lerian-apps"})
	if err == nil {
		t.Fatal("namespaces were created without kubectl")
	}
	if !strings.Contains(err.Error(), "lerian-apps") {
		t.Errorf("the error does not say which namespaces: %v", err)
	}
	if !strings.Contains(err.Error(), "brew install kubectl") {
		t.Errorf("the error does not say how to get it: %v", err)
	}
}

// recording stands in for helm when the question is what happened before it ran.
type recording struct{ called *bool }

func (r recording) Install(context.Context, Options, time.Duration) error {
	*r.called = true
	return nil
}
func (r recording) KubeContext() string { return "" }
func (r recording) Installed(context.Context, string) (Release, bool, error) {
	return Release{}, false, nil
}
func (r recording) Unchanged(context.Context, Options, Release) (bool, error) { return false, nil }

// A test of EnsureNamespaces proves EnsureNamespaces works and says nothing
// about whether Install calls it — which is the gap a mutation deleting the
// call walked straight through. The namespaces must exist before helm places a
// Role in them, so the order matters as much as the fact.
func TestInstallMakesTheNamespacesBeforeHelmRuns(t *testing.T) {
	log := fakeKubectl(t, "exit 0\n")
	var installed bool

	opts := Options{
		Namespace:         DefaultNamespace,
		ControlPlaneURL:   PlaceholderControlPlane,
		Token:             PlaceholderToken,
		ManagedNamespaces: []string{"lerian-apps"},
	}
	if err := Install(context.Background(), recording{&installed}, opts, time.Minute); err != nil {
		t.Fatal(err)
	}

	if !installed {
		t.Fatal("helm was never reached")
	}
	if got := calls(t, log); !strings.Contains(got, "create namespace lerian-apps") {
		t.Errorf("the namespace was not created before the install:\n%s", got)
	}
}

// And a namespace that cannot be made stops the install, rather than letting
// helm discover it three minutes later with a worse message.
func TestAnUnmakeableNamespaceStopsTheInstall(t *testing.T) {
	fakeKubectl(t, "echo 'Error from server (Forbidden)' >&2\nexit 1\n")
	var installed bool

	opts := Options{
		Namespace:         DefaultNamespace,
		ControlPlaneURL:   PlaceholderControlPlane,
		Token:             PlaceholderToken,
		ManagedNamespaces: []string{"lerian-apps"},
	}
	err := Install(context.Background(), recording{&installed}, opts, time.Minute)

	if err == nil {
		t.Fatal("the install proceeded with a namespace that could not be created")
	}
	if installed {
		t.Error("helm was run anyway")
	}
}
