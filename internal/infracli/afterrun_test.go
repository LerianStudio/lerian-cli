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
	rows := afterOptions(infra.ActionPlan, false)

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
	for _, opt := range afterOptions(infra.ActionApply, false) {
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
		},
		func() (func() error, string) { return nil, "" })

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

// Taking an environment down is the other half of putting one up, and the only
// way to reach it was to leave and answer every question again.
func TestDestroyIsOfferedAfterARun(t *testing.T) {
	for _, done := range []infra.Action{infra.ActionPlan, infra.ActionApply} {
		t.Run(string(done), func(t *testing.T) {
			rows := afterOptions(done, false)

			var values []string
			for _, opt := range rows {
				values = append(values, opt.value)
			}
			if !contains(values, string(infra.ActionDestroy)) {
				t.Fatalf("destroy is not offered after %s: %v", done, values)
			}

			// Never beside apply, and never where the cursor starts: everything
			// above it is done repeatedly, and this one cannot be undone.
			destroy := indexOf(values, string(infra.ActionDestroy))
			if destroy == 0 {
				t.Error("the cursor starts on destroy")
			}
			if apply := indexOf(values, string(infra.ActionApply)); apply >= 0 && destroy == apply+1 {
				t.Error("destroy sits one keypress below apply")
			}
			if values[len(values)-1] != afterDone {
				t.Errorf("destroy is not above the exit row: %v", values)
			}
		})
	}
}

// A stack that planned and found nothing is a fact about the run. Counting only
// the stacks WITH changes made the report contradict itself: a list of "no
// changes" followed by "nothing to show".
func TestAPlanWithNoChangesDoesNotAlsoSayThereWasNoPlan(t *testing.T) {
	var out bytes.Buffer
	reportPlanDetail(&out, []unitDetail{
		{name: "infra-base/vpc"},
		{name: "infra-base/eks"},
	})

	painted := out.String()
	if !strings.Contains(painted, "no changes") {
		t.Errorf("the stacks are not reported:\n%s", painted)
	}
	if strings.Contains(painted, "nothing to show") {
		t.Errorf("it says no stack produced a plan, right after listing two:\n%s", painted)
	}
}

// And with nothing at all, the closing line is the only thing there is to say.
func TestNoStackAtAllSaysSo(t *testing.T) {
	var out bytes.Buffer
	reportPlanDetail(&out, nil)

	if !strings.Contains(out.String(), "nothing to show") {
		t.Errorf("an empty report says nothing:\n%s", out.String())
	}
}

func indexOf(values []string, want string) int {
	for index, value := range values {
		if value == want {
			return index
		}
	}
	return -1
}

// The apply that creates a cluster is chosen FROM this menu, so a decision taken
// once before the first draw is a decision taken when there was nothing to point
// at. That is exactly the run where the offer matters, and it was missing from
// it.
func TestTheClusterIsLookedUpAgainAfterEachAction(t *testing.T) {
	var out bytes.Buffer

	// There is no cluster until an apply runs, which is what the real sequence
	// does.
	applied := false
	cluster := func() (func() error, string) {
		if !applied {
			return nil, ""
		}
		return func() error { return nil }, ""
	}

	// Enter on the first row, which is "apply" after a plan; then read what the
	// second menu offers; then leave.
	ask, painted := selectorFor(t, keyDownSeq+keyEnterSeq+keyEnterSeq+"q")
	done := 0

	_ = afterRun(context.Background(), ask, nil, nil, nil, infra.ActionPlan, &out,
		func(action infra.Action) error {
			done++
			if action == infra.ActionApply {
				applied = true
			}
			return nil
		}, cluster)

	if done == 0 {
		t.Fatalf("no action ran:\n%s", painted.String())
	}
	if !strings.Contains(painted.String(), "point kubectl at the cluster") {
		t.Errorf("the offer never appeared, though a cluster existed after the apply:\n%s",
			painted.String())
	}
}

// And before anything has run, there is nothing to offer.
func TestNoOfferBeforeThereIsACluster(t *testing.T) {
	var out bytes.Buffer
	ask, painted := selectorFor(t, "q")

	_ = afterRun(context.Background(), ask, nil, nil, nil, infra.ActionPlan, &out,
		func(infra.Action) error { return nil },
		func() (func() error, string) { return nil, "" })

	if strings.Contains(painted.String(), "point kubectl at the cluster") {
		t.Errorf("an offer was made with no cluster:\n%s", painted.String())
	}
}
