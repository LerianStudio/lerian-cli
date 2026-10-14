package infracli

import (
	"errors"
	"strings"
	"testing"

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
