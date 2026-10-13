package infracli

import (
	"strings"
	"testing"
)

// Every agent command reaches a real cluster. Saying which one is the
// difference between an operator knowing where a workload landed and finding
// out later.
func TestTheTargetSaysWhichClusterAndAccount(t *testing.T) {
	target := AgentTarget{
		Profile: "sandbox",
		Region:  "us-east-2",
		Account: "524121347244",
		Cluster: "lerian-dev-eks",
	}

	described := target.Described()
	for _, want := range []string{"lerian-dev-eks", "524121347244", "us-east-2"} {
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

	CarryAgentTargetARN("sandbox", "us-east-2", "524121347244", "lerian-dev-eks", "")

	got, _ := CarriedAgentTarget()
	if !strings.Contains(got.Context, "cluster/lerian-dev-eks") {
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
