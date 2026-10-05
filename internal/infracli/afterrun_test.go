package infracli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// After a plan the next thing anybody wants is to read it or to apply it. Both
// used to mean walking the account, backend, target and action questions again to
// arrive back where they already were.
func TestAfterAPlanTheNextActionsAreOffered(t *testing.T) {
	rows := afterOptions(infra.ActionPlan)

	offered := make([]string, 0, len(rows))
	for _, opt := range rows {
		offered = append(offered, opt.value)
	}

	for _, want := range []string{afterDetail, string(infra.ActionApply), afterDone} {
		if !contains(offered, want) {
			t.Errorf("%q is not offered after a plan: %v", want, offered)
		}
	}
	// Reading comes before applying: it is the safe one, and the cursor starts on
	// the first row.
	if offered[0] != afterDetail {
		t.Errorf("the first row is %q, want the detail", offered[0])
	}
}

// With no plan there is nothing to detail, and a row that opens an empty report
// is an answer to a question nobody has.
func TestTheDetailIsNotOfferedWithoutAPlan(t *testing.T) {
	for _, opt := range afterOptions(infra.ActionApply) {
		if opt.value == afterDetail {
			t.Error("the plan detail is offered after an apply")
		}
	}
}

// The destructive actions are the ones worth finding in forty lines, so they come
// first — and the order is stable, so the same plan reads the same way twice.
func TestThePlanDetailPutsDestructionFirst(t *testing.T) {
	sorted := sortedChanges([]infra.PlanChange{
		{Action: "create", Address: "aws_subnet.b"},
		{Action: "destroy", Address: "aws_db_instance.old"},
		{Action: "create", Address: "aws_subnet.a"},
		{Action: "replace", Address: "aws_eks_node_group.main"},
		{Action: "update", Address: "aws_security_group.api"},
	})

	order := make([]string, 0, len(sorted))
	for _, change := range sorted {
		order = append(order, change.Action+" "+change.Address)
	}

	want := []string{
		"destroy aws_db_instance.old",
		"replace aws_eks_node_group.main",
		"update aws_security_group.api",
		"create aws_subnet.a",
		"create aws_subnet.b",
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("row %d is %q, want %q\nfull order: %v", i, order[i], want[i], order)
		}
	}
}

// Nobody there means nothing offered and nothing run. The run already reported
// itself, and a menu painted into a pipe is a command that has stopped for no
// visible reason.
//
// This pins the outcome, not the guard: ask.pick also refuses without a
// terminal, so the explicit check above it is belt and braces and a test cannot
// tell the two apart.
func TestNothingIsOfferedWithoutATerminal(t *testing.T) {
	var out bytes.Buffer
	ask := &prompter{interactive: false, out: &out}

	err := afterRun(context.Background(), ask, nil, nil, nil, infra.ActionPlan, &out,
		func(infra.Action) error {
			t.Error("an action ran with nobody to ask")
			return nil
		})

	if err != nil {
		t.Errorf("afterRun = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("it printed into a pipe:\n%s", out.String())
	}
}

// The purpose line has to say what the rows operate on, because "apply" on its
// own does not say which account is about to be written to.
func TestTheOfferSaysWhatItWouldRunAgainst(t *testing.T) {
	if !strings.Contains(afterPurpose(infra.ActionPlan), "Same targets, same account") {
		t.Errorf("afterPurpose = %q", afterPurpose(infra.ActionPlan))
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
