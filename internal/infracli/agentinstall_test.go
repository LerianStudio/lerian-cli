package infracli

import (
	"strings"
	"testing"

	"slices"

	"github.com/lerian-studio/lerian-cli/internal/agent"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// The offer belongs to the cluster, not to the run that happened to precede it.
// Somebody who has just planned against a running cluster is exactly somebody
// who may want to reach the agent in it — the same reasoning that moved the
// kubectl row off this gate.
func TestTheAgentIsOfferedWheneverThereIsACluster(t *testing.T) {
	if !agentOffer(true) {
		t.Error("a cluster is there and the agent was not offered")
	}
	if agentOffer(false) {
		t.Error("the agent was offered with no cluster to reach")
	}
}

// Which means it survives a plan, which is where this was wrong: planning
// against a running cluster hid the row.
func TestTheAgentRowSurvivesAPlan(t *testing.T) {
	var found bool
	for _, opt := range afterOptions(infra.ActionPlan, true) {
		if opt.value == afterAgent {
			found = true
		}
	}
	if !found {
		t.Error("planning against a running cluster hid the agent row")
	}
}

// And it has to actually reach the menu, not just pass its own predicate.
func TestTheMenuCarriesTheAgentRow(t *testing.T) {
	rows := afterOptions(infra.ActionApply, true)

	var found bool
	for _, opt := range rows {
		if opt.value == afterAgent {
			found = true
			if !strings.Contains(opt.label, "agent") {
				t.Errorf("the row does not read as the agent: %q", opt.label)
			}
		}
	}
	if !found {
		t.Fatalf("no agent row after an apply with a cluster: %+v", rows)
	}

	// And with no cluster there is nothing to reach, whatever just ran.
	for _, opt := range afterOptions(infra.ActionApply, false) {
		if opt.value == afterAgent {
			t.Error("the agent was offered with no cluster")
		}
	}
}

// The credential is described, never printed: this line goes on a terminal that
// may be shared, recorded, or pasted into an issue.
func TestTheSummaryDescribesTheCredentialWithoutPrintingIt(t *testing.T) {
	enrollment := agent.Options{Token: agent.EnrollmentPrefix + "s3cret-value"}
	if got := describeCredential(enrollment); strings.Contains(got, "s3cret-value") {
		t.Errorf("the token is in the summary: %q", got)
	} else if !strings.Contains(got, "enrollment") {
		t.Errorf("and the form is not named: %q", got)
	}

	perAgent := agent.Options{Token: "lerian_agent_s3cret", AgentID: "a1"}
	if got := describeCredential(perAgent); strings.Contains(got, "s3cret") {
		t.Errorf("the token is in the summary: %q", got)
	}

	existing := agent.Options{ExistingSecret: "agent-creds"}
	if got := describeCredential(existing); !strings.Contains(got, "agent-creds") {
		t.Errorf("a Secret name is not a credential and should be shown: %q", got)
	}
}

// An empty list is a real answer with a real consequence, so it says the
// consequence instead of printing nothing.
func TestNoManagedNamespacesSaysWhatThatMeans(t *testing.T) {
	got := describeNamespaces(agent.Options{})

	if !strings.Contains(got, "install nowhere") {
		t.Errorf("an empty list does not say what it costs: %q", got)
	}
	if got := describeNamespaces(agent.Options{ManagedNamespaces: []string{"apps", "data"}}); got != "apps, data" {
		t.Errorf("the list is not rendered: %q", got)
	}
}

// `lerian agent install` with no flags reaches this. Outside a terminal it must
// refuse and name the flag to add, rather than block on a question nobody will
// answer — a flag missing in CI is a flag to add to the script.
//
// The control plane is not among the questions: it belongs to Lerian and is a
// constant this CLI carries, so the first thing it cannot work out alone is
// which namespaces the agent may install into.
func TestAskingForAnInstallOutsideATerminalSaysWhatIsMissing(t *testing.T) {
	var out strings.Builder

	_, err := AskAgentOptions(&out)

	if err == nil {
		t.Fatal("it tried to ask with no terminal")
	}
	if !strings.Contains(err.Error(), "--managed-namespaces") {
		t.Errorf("the error does not name the flag to add: %v", err)
	}
	if !strings.Contains(err.Error(), "no terminal") {
		t.Errorf("and does not say why it could not ask: %v", err)
	}
	if strings.Contains(err.Error(), "--control-plane") {
		t.Errorf("it asks for a value this CLI should already know: %v", err)
	}
	if out.String() != "" {
		t.Errorf("it printed a prompt anyway:\n%s", out.String())
	}
}

// Somebody who has just planned against a running cluster most often wants to
// know what is in it. An offer that can only install answers a question they
// did not ask — and asks for a control plane URL to do it.
func TestWithAnAgentInstalledEveryOperationIsOffered(t *testing.T) {
	values := make([]string, 0, 4)
	for _, opt := range agentActions(true) {
		values = append(values, opt.value)
	}

	for _, want := range []string{"status", "upgrade", "install", "uninstall"} {
		if !slices.Contains(values, want) {
			t.Errorf("%q is not offered for an installed agent: %v", want, values)
		}
	}
	// Reading comes first: it is the one that changes nothing, and the one most
	// often wanted.
	if values[0] != "status" {
		t.Errorf("the first row is %q, want the one that changes nothing", values[0])
	}
}

// With nothing installed, an upgrade and a removal are operations on something
// that is not there.
func TestWithNoAgentOnlyInstallAndStatusAreOffered(t *testing.T) {
	for _, opt := range agentActions(false) {
		if opt.value == "upgrade" || opt.value == "uninstall" {
			t.Errorf("%q was offered with nothing installed", opt.value)
		}
	}
}

// The line under the question says what is there, so the choice is made with
// the state in view rather than from memory.
func TestTheMenuSaysWhatIsInstalled(t *testing.T) {
	got := agentState(agent.Release{Chart: "agent-helm-1.0.1", Status: "deployed", Revision: "3"}, true)
	for _, want := range []string{"agent-helm-1.0.1", "deployed", "3"} {
		if !strings.Contains(got, want) {
			t.Errorf("the state line omits %q: %q", want, got)
		}
	}

	if got := agentState(agent.Release{}, false); !strings.Contains(got, "Nothing is installed") {
		t.Errorf("an empty namespace is described as %q", got)
	}
}
