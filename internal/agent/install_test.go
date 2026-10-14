package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// The failure this CLI has actually produced, verbatim from helm: the chart
// places a Role per managed namespace, and reports one error per namespace that
// is not there.
const missingNamespaces = `agent: helm could not install the agent: Error: Helm release error

2 errors occurred:
    * namespaces "midaz" not found
    * namespaces "apps" not found
`

// Helm says what failed. What it does not say is which thing the operator
// controls is wrong, or that one command fixes it.
func TestMissingNamespacesBecomeCommands(t *testing.T) {
	advice := adviseOn(errors.New(missingNamespaces), DefaultNamespace)

	if advice == "" {
		t.Fatal("the failure was not recognized")
	}
	for _, want := range []string{"kubectl create namespace midaz", "kubectl create namespace apps"} {
		if !strings.Contains(advice, want) {
			t.Errorf("the advice does not offer %q:\n%s", want, advice)
		}
	}
	if !strings.Contains(advice, "must exist before") {
		t.Errorf("it does not say why:\n%s", advice)
	}
}

// The same namespace named twice is one namespace. Helm reports it once per
// object it could not place, which is not the same number.
func TestARepeatedNamespaceIsListedOnce(t *testing.T) {
	advice := adviseOn(errors.New(`* namespaces "apps" not found
* namespaces "apps" not found`), DefaultNamespace)

	if got := strings.Count(advice, "kubectl create namespace apps"); got != 1 {
		t.Errorf("the same namespace is offered %d times:\n%s", got, advice)
	}
}

// A release left behind by a failed install is the other failure worth
// naming — it is what the operator hits on the retry, and the fix is not
// obvious from helm's own wording.
func TestANameAlreadyInUsePointsAtUninstall(t *testing.T) {
	advice := adviseOn(errors.New("Error: cannot re-use a name that is still in use"), DefaultNamespace)

	if !strings.Contains(advice, "lerian agent uninstall") {
		t.Errorf("the advice does not say how to clear it:\n%s", advice)
	}
	if !strings.Contains(advice, IdentitySecretName) {
		t.Errorf("and does not say the identity survives:\n%s", advice)
	}
}

// Everything else passes through untouched, or every unrelated failure grows a
// paragraph about namespaces.
func TestUnrelatedFailuresGetNoAdvice(t *testing.T) {
	for _, text := range []string{
		"Error: connection refused",
		"Error: UPGRADE FAILED: another operation is in progress",
		"",
	} {
		if advice := adviseOn(errors.New(text), DefaultNamespace); advice != "" {
			t.Errorf("invented advice for %q:\n%s", text, advice)
		}
	}
}

// failing stands in for helm when the question is what the operator is told.
type failing struct{ err error }

func (f failing) Install(context.Context, Options, time.Duration) error { return f.err }
func (f failing) KubeContext() string                                   { return "" }
func (f failing) Installed(context.Context, string) (Release, bool, error) {
	return Release{}, false, nil
}
func (f failing) Unchanged(context.Context, Options, Release) (bool, error) { return false, nil }

// A test of adviseOn proves adviseOn works and says nothing about whether
// Install calls it — which is exactly the gap a mutation deleting the call
// walked straight through.
func TestInstallCarriesTheAdviceToTheOperator(t *testing.T) {
	opts := Options{
		Namespace:       DefaultNamespace,
		ControlPlaneURL: "https://control.example.com",
		Token:           EnrollmentPrefix + "abc",
	}

	err := Install(context.Background(), failing{errors.New(missingNamespaces)}, opts, time.Minute)
	if err == nil {
		t.Fatal("a failed install returned nil")
	}

	// Helm's own words stay: they are accurate, and this adds to them.
	if !strings.Contains(err.Error(), `namespaces "midaz" not found`) {
		t.Errorf("helm's own message was replaced:\n%v", err)
	}
	if !strings.Contains(err.Error(), "kubectl create namespace midaz") {
		t.Errorf("the advice did not reach the operator:\n%v", err)
	}
}

// And a failure with nothing to advise is passed through as it is.
func TestInstallDoesNotDecorateWhatItCannotExplain(t *testing.T) {
	opts := Options{ControlPlaneURL: "https://c", Token: EnrollmentPrefix + "x"}
	original := errors.New("Error: connection refused")

	err := Install(context.Background(), failing{original}, opts, time.Minute)

	if err.Error() != original.Error() {
		t.Errorf("an unexplained failure was decorated:\n%v", err)
	}
}

// Validation runs before helm does: a wrong flag should cost nothing.
func TestInstallValidatesBeforeReachingTheCluster(t *testing.T) {
	var reached bool
	helm := reaching{&reached}

	err := Install(context.Background(), helm, Options{ControlPlaneURL: "https://c"}, time.Minute)

	if err == nil {
		t.Fatal("an install with no credential was attempted")
	}
	if reached {
		t.Error("helm was called with options that could not work")
	}
}

type reaching struct{ called *bool }

func (r reaching) Install(context.Context, Options, time.Duration) error {
	*r.called = true
	return nil
}

func (r reaching) KubeContext() string { return "" }
func (r reaching) Installed(context.Context, string) (Release, bool, error) {
	return Release{}, false, nil
}
func (r reaching) Unchanged(context.Context, Options, Release) (bool, error) { return false, nil }

// The failure somebody hits while the control plane is still being built. Helm
// says "timed out waiting for the condition", which is true and says nothing
// about what was waited for or that the agent is in fact running.
func TestATimeoutExplainsThatReadinessMeansRegistered(t *testing.T) {
	advice := adviseOn(errors.New("agent: helm could not install the agent: "+
		"Error: timed out waiting for the condition"), "infra")

	if advice == "" {
		t.Fatal("a readiness timeout was not recognized")
	}
	if !strings.Contains(advice, "registered") {
		t.Errorf("it does not say what readiness depends on:\n%s", advice)
	}
	if !strings.Contains(advice, "--wait=false") {
		t.Errorf("it does not offer the way out:\n%s", advice)
	}
	// What the error proves is that it timed out. Saying the pod is running is a
	// guess, and a wrong one when the image could not be pulled — or when the
	// context expired before helm applied anything, leaving no pod at all.
	for _, claim := range []string{"The objects were created", "the pod is running"} {
		if strings.Contains(advice, claim) {
			t.Errorf("it states %q, which the error does not establish:\n%s", claim, advice)
		}
	}
	// So it asks what exists before diagnosing why it is not ready.
	if !strings.Contains(advice, "helm list -n") {
		t.Errorf("it does not ask whether the release exists at all:\n%s", advice)
	}
	if !strings.Contains(advice, "CrashLoopBackOff") {
		t.Errorf("it does not admit the other reasons readiness never came:\n%s", advice)
	}
	// The namespace that was installed into, not the default.
	if !strings.Contains(advice, "kubectl -n infra") {
		t.Errorf("it reads logs from the wrong namespace:\n%s", advice)
	}
}
