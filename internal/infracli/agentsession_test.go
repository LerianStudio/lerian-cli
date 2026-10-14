package infracli

import (
	"errors"
	"strings"
	"testing"

	"context"

	"github.com/lerian-studio/lerian-cli/internal/agent"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Every agent command reaches a real cluster. Saying which one is the
// difference between an operator knowing where a workload landed and finding
// out later.
func TestTheTargetSaysWhichClusterAndAccount(t *testing.T) {
	target := AgentTarget{
		Profile: "example",
		Region:  "us-east-2",
		Account: "524121347244",
		Cluster: "example-eks",
	}

	described := target.Described()
	for _, want := range []string{"example-eks", "524121347244", "us-east-2"} {
		if !strings.Contains(described, want) {
			t.Errorf("the description omits %q: %q", want, described)
		}
	}
}

// Without a chosen cluster it must not invent one: "the current kubeconfig
// context" is the honest answer, and it is what a scripted run gets.
func TestAnUnchosenTargetSaysSo(t *testing.T) {
	if got := (AgentTarget{}).Described(); !strings.Contains(got, "current kubeconfig") {
		t.Errorf("an empty target described as %q", got)
	}
}

// The deploy that just ran answered all of this. The ARN it reported is the
// kubeconfig context verbatim, so it is carried rather than reassembled — a
// built one guesses the partition, and guesses wrong in GovCloud and in China.
func TestACarriedTargetKeepsTheReportedARN(t *testing.T) {
	t.Cleanup(func() { carried = nil })

	const arn = "arn:aws-us-gov:eks:us-gov-west-1:111122223333:cluster/gov-eks"
	CarryAgentTargetARN("gov", "us-gov-west-1", "111122223333", "gov-eks", arn)

	got, ok := CarriedAgentTarget()
	if !ok {
		t.Fatal("nothing was carried")
	}
	if got.Context != arn {
		t.Errorf("context = %q, want the ARN the stack reported", got.Context)
	}
	if got.Cluster != "gov-eks" {
		t.Errorf("cluster = %q", got.Cluster)
	}
}

// With no ARN reported it falls back to building one, which is right for the
// commercial partition and is all the information there is.
func TestACarriedTargetFallsBackToABuiltARN(t *testing.T) {
	t.Cleanup(func() { carried = nil })

	CarryAgentTargetARN("example", "us-east-2", "524121347244", "example-eks", "")

	got, _ := CarriedAgentTarget()
	if !strings.Contains(got.Context, "cluster/example-eks") {
		t.Errorf("no usable context was built: %q", got.Context)
	}
}

// Nothing carried is the normal state: an agent command run on its own has no
// deploy behind it and must ask.
func TestNothingIsCarriedByDefault(t *testing.T) {
	carried = nil

	if _, ok := CarriedAgentTarget(); ok {
		t.Error("a target was carried with no run behind it")
	}
}

// Every session expires, usually overnight. A menu where every row reads
// "(unavailable)" and the only way forward is a command this tool could have
// run is a dead end.
func TestAnExpiredSessionIsChoosableSoItCanBeRevived(t *testing.T) {
	options, targets := agentAccountOptions([]infra.ResolvedProfile{{
		Profile: infra.AWSProfile{Name: "work", CanSignIn: true, SSOSession: "work-sso"},
		Err:     errors.New("aws sts get-caller-identity failed: exit status 255"),
	}})

	if len(options) != 1 {
		t.Fatalf("got %d rows", len(options))
	}
	if options[0].disabled {
		t.Error("an expired session cannot be chosen, so it cannot be revived")
	}
	if !strings.Contains(options[0].note, "log in") {
		t.Errorf("the row does not say what choosing it does: %q", options[0].note)
	}
	if got := targets["work"]; len(got) != 1 || got[0].Session != "work-sso" {
		t.Errorf("no login target was recorded for the row: %+v", got)
	}
}

// A [default] section with a region and nothing else has no session to revive.
// Calling that "expired" sends somebody to a login that fails with "Unable to
// locate credentials", which names neither the problem nor the fix.
func TestAProfileWithNothingToReviveIsDisabled(t *testing.T) {
	options, targets := agentAccountOptions([]infra.ResolvedProfile{{
		Profile: infra.AWSProfile{Name: "default"},
		Err:     errors.New("no credentials"),
	}})

	if !options[0].disabled {
		t.Error("a profile with no credentials at all was offered as choosable")
	}
	if !strings.Contains(options[0].note, "aws configure") {
		t.Errorf("the row does not say how to fix it: %q", options[0].note)
	}
	if len(targets) != 0 {
		t.Errorf("a login was offered for a profile that cannot log in: %+v", targets)
	}
}

// The selector adds "(unavailable)" to a disabled row itself. Adding it to the
// note as well printed it twice.
func TestTheRowDoesNotRepeatUnavailable(t *testing.T) {
	options, _ := agentAccountOptions([]infra.ResolvedProfile{{
		Profile: infra.AWSProfile{Name: "default"},
		Err:     errors.New("no credentials"),
	}})

	if strings.Contains(options[0].note, "unavailable") {
		t.Errorf("the note repeats what the selector already says: %q", options[0].note)
	}
}

// A working profile shows what choosing it means: the account, and the region
// when the profile names one.
func TestAUsableProfileShowsItsAccount(t *testing.T) {
	options, _ := agentAccountOptions([]infra.ResolvedProfile{{
		Profile: infra.AWSProfile{Name: "work", Region: "us-east-2"},
		Caller:  infra.Caller{Account: "111122223333"},
	}})

	if options[0].disabled {
		t.Error("a usable profile was disabled")
	}
	for _, want := range []string{"111122223333", "us-east-2"} {
		if !strings.Contains(options[0].note, want) {
			t.Errorf("the row omits %q: %q", want, options[0].note)
		}
	}
}

// Removing an agent is reached from a menu, where there is no command line to
// add --yes to. Answering that with "re-run with --yes" leaves somebody exactly
// where they started — the same mistake the install made before it asked.
func TestRemovalRefusesWithoutATerminalAndNamesTheFlag(t *testing.T) {
	var out strings.Builder

	err := ConfirmRemoval(context.Background(), &out, "remove the agent from example-eks")

	if err == nil {
		t.Fatal("it removed without confirming and without a terminal")
	}
	if !strings.Contains(err.Error(), "not a terminal") {
		t.Errorf("the error does not say why it could not ask: %v", err)
	}
	// And it repeats what was pending, so a scripted run says what it declined
	// to do rather than only that it declined.
	if !strings.Contains(err.Error(), "example-eks") {
		t.Errorf("the error does not name what was pending: %v", err)
	}
	// The flag this command actually has. The shared confirmation names
	// --auto-approve, which belongs to the infra run and does not exist here.
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("the error does not name the flag to pass: %v", err)
	}
	if strings.Contains(err.Error(), "--auto-approve") {
		t.Errorf("it names a flag this command does not have: %v", err)
	}
}

// An install reaches one cluster in one account. A report that names neither
// leaves somebody to remember which — across seven AWS profiles, that is a
// thing to be told rather than recalled.
func TestTheInstalledReportSaysWhereAndWhat(t *testing.T) {
	var out strings.Builder

	PrintInstalled(&out,
		AgentTarget{Cluster: "example-eks", Account: "111122223333", Region: "us-east-2", Profile: "work"},
		agent.Options{
			Namespace:         agent.DefaultNamespace,
			ControlPlaneURL:   "https://control.example.com",
			Token:             agent.EnrollmentPrefix + "abc",
			ManagedNamespaces: []string{"lerian-apps"},
		},
		agent.Release{Chart: "agent-helm-1.0.1", AppVersion: "1.0.1", Revision: "2", Status: "deployed"})

	// Anchored on the row rather than on the value: "1.0.1" also appears inside
	// "agent-helm-1.0.1", so a substring check passed with the agent's own
	// version missing entirely.
	for _, want := range []string{
		"cluster           example-eks",
		"account           111122223333  (work)",
		"region            us-east-2",
		"namespace         lerian-system",
		"chart             agent-helm-1.0.1",
		"agent             1.0.1",
		"revision          2  ·  deployed",
		"control plane     https://control.example.com",
		"may install into  lerian-apps",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report is missing the row %q:\n%s", want, out.String())
		}
	}
	if !strings.Contains(out.String(), "lerian agent status") {
		t.Errorf("the report does not say what to do next:\n%s", out.String())
	}
}

// A placeholder install must not promise a heartbeat it cannot send.
func TestAPlaceholderReportDoesNotPromiseRegistration(t *testing.T) {
	var out strings.Builder

	PrintInstalled(&out, AgentTarget{Cluster: "example-eks"},
		agent.Options{
			Namespace:       agent.DefaultNamespace,
			ControlPlaneURL: agent.PlaceholderControlPlane,
			Token:           agent.PlaceholderToken,
		},
		agent.Release{Chart: "agent-helm-1.0.1"})

	if strings.Contains(out.String(), "first heartbeat lands") {
		t.Errorf("it promises a registration that cannot happen:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "nowhere to register") {
		t.Errorf("it does not say why it will stay unready:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "logs deploy/"+agent.ReleaseName) {
		t.Errorf("it does not say where to look:\n%s", out.String())
	}
}

// Run without the preflight — scripted, with --kube-context — there is no
// account or region to report, and inventing them would be worse than omitting.
func TestTheReportOmitsWhatItDoesNotKnow(t *testing.T) {
	var out strings.Builder

	PrintInstalled(&out, AgentTarget{},
		agent.Options{Namespace: agent.DefaultNamespace, ControlPlaneURL: "https://c",
			Token: agent.EnrollmentPrefix + "x"},
		agent.Release{})

	if strings.Contains(out.String(), "account") || strings.Contains(out.String(), "region") {
		t.Errorf("it reported a cluster it was never told about:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "namespace") {
		t.Errorf("it dropped what it does know:\n%s", out.String())
	}
}
