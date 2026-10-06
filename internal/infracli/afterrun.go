package infracli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Sentinel rows for the menu that follows a run.
const (
	afterDetail  = "\x00detail"
	afterKubectl = "\x00kubectl"
	afterDone    = "\x00done"
)

// afterRun is what happens when a run finishes and somebody is still sitting
// there.
//
// It exists because the run used to end by returning to the top menu, which threw
// away every answer that produced it. After a plan the next thing anybody wants is
// one of two things — to read what it would change, or to apply it — and both of
// those used to mean walking the account, backend, target and action questions
// again to arrive at the same place.
//
// It also has to happen HERE, inside the run: the saved plans are deleted when
// this function's caller returns, and they are what "show me the detail" reads.
func afterRun(
	ctx context.Context,
	ask *prompter,
	runner *infra.Runner,
	terraform *infra.CLI,
	stages []infra.Stage,
	done infra.Action,
	out io.Writer,
	again func(infra.Action) error,
	kubectl func() error,
) error {
	if !ask.interactive {
		return nil
	}

	for {
		picked, err := ask.pick("What now?", afterPurpose(done), "", afterOptions(done, kubectl != nil), "")
		switch {
		//nolint:nilerr // Leaving is leaving: q, r and ctrl-c all arrive as an
		// error here, and so does a selector that could not draw. The run already
		// happened and was already reported — this question is an offer, not a
		// step — so returning the error would turn declining an offer into a
		// failed command.
		case err != nil:
			return nil
		case picked == afterDone:
			return nil
		case picked == afterDetail:
			printPlanDetail(ctx, terraform, runner, stages, out)
			continue
		case picked == afterKubectl:
			if err := kubectl(); err != nil {
				// Said and carried on. Nothing was deployed differently because
				// kubectl could not be pointed, and the menu is still the right
				// place to be.
				fmt.Fprintf(out, "\n  %v\n\n", err)
			}
			continue
		}

		if err := again(infra.Action(picked)); err != nil {
			return err
		}
		done = infra.Action(picked)
	}
}

// afterPurpose says what the rows below operate on, which is the part that makes
// them safe to pick: the same targets, in the same account, that just ran.
func afterPurpose(done infra.Action) string {
	if done == infra.ActionPlan {
		return "Same targets, same account. apply runs them for real, after one confirmation."
	}
	return "Same targets, same account."
}

// afterOptions is the menu, with the detail row only where there is a plan to
// detail.
func afterOptions(done infra.Action, cluster bool) []option {
	options := make([]option, 0, 7)
	if done == infra.ActionPlan {
		options = append(options,
			option{value: afterDetail, label: "show what the plan would change",
				note: "resource by resource, from the plan just made"},
			option{value: string(infra.ActionApply), label: "apply",
				note: "writes, after one confirmation"},
		)
	} else {
		options = append(options, option{value: string(infra.ActionPlan), label: "plan again",
			note: "changes nothing"})
	}

	// Only after an apply, and only when the run produced a cluster. After a plan
	// there is nothing new to point at, and offering it would suggest the plan
	// changed something.
	if cluster && done == infra.ActionApply {
		options = append(options, option{value: afterKubectl, label: "point kubectl at the cluster",
			note: "runs aws eks update-kubeconfig"})
	}
	options = append(options,
		option{value: string(infra.ActionOutput), label: "output", note: "reads terraform output"},
		option{value: string(infra.ActionHelmValues), label: "helm-values", note: "merges helm_values onto stdout"},
	)

	// destroy last among the actions, and never beside apply. Everything above is
	// something somebody does repeatedly; this is the one that cannot be undone,
	// and a list where it sits one keypress from "apply" is a list that will
	// eventually be mis-pressed. It is offered at all because taking an
	// environment down is the other half of putting one up, and the only way to
	// reach it was to leave and answer every question again.
	options = append(options,
		option{value: string(infra.ActionDestroy), label: "destroy",
			note: "removes what these targets created, after one confirmation"},
		option{value: afterDone, label: "back to the menu", note: "leaves this account and target"},
	)
	return options
}

// unitDetail is one stack's share of the report: what it would do, or that it
// has no saved plan to read.
type unitDetail struct {
	name    string
	changes []infra.PlanChange
	// unplanned marks a stack that never produced a plan in this run — normal for
	// anything blocked behind an earlier stage.
	unplanned bool
}

// printPlanDetail reads the saved plans and reports them.
//
// Split in two so the report can be tested: reading needs terraform and a run
// directory, and what somebody reads is the part that has been wrong.
func printPlanDetail(
	ctx context.Context,
	terraform *infra.CLI,
	runner *infra.Runner,
	stages []infra.Stage,
	out io.Writer,
) {
	var details []unitDetail
	for _, stage := range stages {
		for _, unit := range stage.Units {
			changes, err := terraform.PlanDetail(ctx, unit, runner.PlanFile(unit))
			details = append(details, unitDetail{
				name:      unit.Name,
				changes:   changes,
				unplanned: err != nil,
			})
		}
	}
	reportPlanDetail(out, details)
}

// reportPlanDetail prints what the plans would do, grouped by stack.
//
// Grouped and sorted rather than streamed: a first run of infra-base is forty
// resources, and forty lines with no shape to them is output somebody scrolls
// past. The destructive actions come first within each stack for the same reason
// they are counted separately — "2 to destroy" is the line worth finding.
func reportPlanDetail(out io.Writer, details []unitDetail) {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> What the plan would change"))

	for _, detail := range details {
		switch {
		case detail.unplanned:
			fmt.Fprintf(out, "\n  %s\n    %s\n", detail.name,
				theme.dim("no saved plan — it was not planned in this run"))
		case len(detail.changes) == 0:
			fmt.Fprintf(out, "\n  %s\n    %s\n", detail.name, theme.dim("no changes"))
		default:
			fmt.Fprintf(out, "\n  %s\n", detail.name)
			for _, change := range sortedChanges(detail.changes) {
				fmt.Fprintf(out, "    %-8s %s\n", change.Action, change.Address)
			}
		}
	}

	// Only when there was nothing to report at all. A stack that planned and found
	// nothing is a fact about this run, and saying "nothing to show" under a list
	// of them made the report contradict itself.
	if len(details) == 0 {
		fmt.Fprintf(out, "\n  %s\n", theme.dim("nothing to show: no stack in this run produced a plan"))
	}
	fmt.Fprintln(out)
}

// sortedChanges puts the destructive actions first and sorts by address inside
// each group, so the same plan always reads the same way.
func sortedChanges(changes []infra.PlanChange) []infra.PlanChange {
	weight := map[string]int{"destroy": 0, "replace": 1, "update": 2, "create": 3, "read": 4}

	sorted := make([]infra.PlanChange, len(changes))
	copy(sorted, changes)
	sort.SliceStable(sorted, func(i, j int) bool {
		if weight[sorted[i].Action] != weight[sorted[j].Action] {
			return weight[sorted[i].Action] < weight[sorted[j].Action]
		}
		return strings.Compare(sorted[i].Address, sorted[j].Address) < 0
	})
	return sorted
}
