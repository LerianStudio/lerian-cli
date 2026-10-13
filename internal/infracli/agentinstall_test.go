package infracli

import (
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/agent"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// The offer belongs where the cluster exists and was just built. Offering it
// after a plan would be a question with nothing to install into.
func TestTheAgentIsOfferedOnlyAfterAnApplyWithACluster(t *testing.T) {
	for name, tc := range map[string]struct {
		done    infra.Action
		cluster bool
		want    bool
	}{
		"applied, cluster there": {infra.ActionApply, true, true},
		"applied, no cluster":    {infra.ActionApply, false, false},
		"only planned":           {infra.ActionPlan, true, false},
		"destroyed":              {infra.ActionDestroy, true, false},
		"output":                 {infra.ActionOutput, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := agentOffer(tc.done, tc.cluster); got != tc.want {
				t.Errorf("offer = %v, want %v", got, tc.want)
			}
		})
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

	for _, opt := range afterOptions(infra.ActionPlan, true) {
		if opt.value == afterAgent {
			t.Error("the agent was offered after a plan")
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
