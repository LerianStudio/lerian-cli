package infracli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Sentinel rows for the menu that follows a run.
const (
	afterDetail  = "\x00detail"
	afterExport  = "\x00export"
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
	// cluster reports whether there is a cluster to point kubectl at, right now,
	// and why there is not. Called before every draw: an apply chosen from this
	// menu is what creates one, so an answer taken once, before the first draw,
	// is an answer about the world as it was.
	cluster func() (func() error, string),
) error {
	if !ask.interactive {
		return nil
	}

	// Whether the copy has already been taken in this sitting. The reminder below
	// is worth one interruption and not two: somebody who has exported and then
	// applies again does not need to be asked a second time.
	exported := false

	for {
		point, why := cluster()
		if point == nil && why != "" && done == infra.ActionApply {
			// An apply that made a cluster this cannot read is worth a line: the row
			// being absent otherwise looks like the tool forgetting.
			fmt.Fprintf(out, "  %s\n", newStyle(out).dim(
				"cannot offer to point kubectl at the cluster: "+why))
		}

		picked, err := ask.pick("What now?", afterPurpose(done), "", afterOptions(done, point != nil), "")
		switch {
		// Leaving is leaving: q, r and ctrl-c all arrive as an error here, and so
		// does a selector that could not draw. The run already happened and was
		// already reported — this question is an offer, not a step — so returning
		// the error would turn declining an offer into a failed command.
		case err != nil:
			// r — "back" — is another way of saying "back to the menu", so it gets
			// the same last question. q and ctrl-c are not: those mean stop now,
			// and answering them with a prompt is the opposite of what they ask.
			if errors.Is(err, errBack) && !leaving(ctx, ask, out, done, &exported) {
				continue
			}
			return nil
		case picked == afterDone:
			if !leaving(ctx, ask, out, done, &exported) {
				continue
			}
			return nil
		case picked == afterDetail:
			printPlanDetail(ctx, terraform, runner, stages, out)
			continue
		case picked == afterExport:
			if err := exportFromMenu(ctx, ask, out); err != nil {
				fmt.Fprintf(out, "\n  %v\n\n", err)
			} else {
				exported = true
			}
			continue
		case picked == afterKubectl:
			if err := point(); err != nil {
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

// leaving is the last question before the run is left behind, and it reports
// whether to go.
//
// It exists because what this menu offers stops being reachable the moment it
// closes. The stack is up, the operator reads "back to the menu" as the way out
// of a finished job, and the copy they are entitled to — the one that frees them
// from the templates — is a row they scrolled past without knowing what it was
// for. Afterwards it means finding `lerian config repo`, which nobody does.
//
// Only after an apply, and only once. A plan built nothing to take away, a
// destroy took it down, and a second asking after the copy exists is a prompt
// that trains people to dismiss prompts.
func leaving(ctx context.Context, ask *prompter, out io.Writer, done infra.Action, exported *bool) bool {
	if done != infra.ActionApply || *exported {
		return true
	}

	// A choice rather than a yes/no. "Are you sure?" puts the work of remembering
	// what was missed back on the person who just showed they had not; the row
	// that does the thing is the reminder.
	picked, err := ask.pick("Before you go", leavingPurpose, "", []option{
		{value: afterExport, label: "copy this into a repository of your own",
			note: "the roots you configured, their modules, and a git history"},
		{value: afterDone, label: "leave", note: "nothing else is written"},
	}, "")
	if err != nil || picked == afterDone {
		return true
	}

	if err := exportFromMenu(ctx, ask, out); err != nil {
		fmt.Fprintf(out, "\n  %v\n\n", err)
		// Staying, so the failure is on a screen with something to do about it.
		// Leaving on the error would report it to somebody already walking away.
		return false
	}
	*exported = true
	return true
}

// leavingPurpose says why this question is being asked at all, since nothing has
// gone wrong and the operator asked to leave.
// Kept short on purpose: the line is truncated to the terminal width, and a
// purpose whose only concrete fact — the command — falls off the end says
// nothing the heading did not.
const leavingPurpose = "Last offer; afterwards: lerian config repo <path>"

// exportFromMenu asks where the repository goes and writes it.
//
// Asked rather than defaulted to a path: this creates a directory somebody is
// going to push, and guessing where their infrastructure lives on disk is the
// kind of guess that gets a tree written somewhere they did not expect.
func exportFromMenu(ctx context.Context, ask *prompter, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	// Asked again when the destination was occupied and the answer was to leave
	// it alone. That answer means "somewhere else", not "never mind" — ending the
	// step there would make somebody reopen the menu to say the same thing with a
	// different path.
	for {
		where, err := ask.ask("Where should the repository go?",
			"A directory that does not exist yet. Nothing is pushed; the remote stays yours.",
			filepath.Join(home, "infrastructure"), "lerian config repo <path>")
		if err != nil {
			return err
		}

		err = exportRepository(ctx, ask, out, where)

		// Both of these are verdicts on the path that was just typed, not on the
		// run: one says that directory is staying, the other that it can never be
		// emptied. Returning either would close the step and send somebody back
		// through the menu to type a different path — which is what the question
		// coming round again does for them.
		if errors.Is(err, infra.ErrProtectedPath) {
			fmt.Fprintf(out, "\n  %v\n", err)
			continue
		}
		if !errors.Is(err, errBack) {
			return err
		}
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

	// Whenever there is a cluster, whatever the action was. It was once gated on
	// apply, on the reasoning that a plan changes nothing — true, and beside the
	// point: the cluster is there either way, and somebody who has just planned
	// against it is exactly somebody who may want to look inside it.
	if cluster {
		options = append(options, option{value: afterKubectl, label: "point kubectl at the cluster",
			note: "runs aws eks update-kubeconfig"})
	}
	options = append(options,
		option{value: string(infra.ActionOutput), label: "output", note: "reads terraform output"},
		// Here because this is when the configuration is complete and somebody is
		// looking at what it just built. Offering it only under `config` meant
		// finding a command whose existence nobody had a reason to suspect.
		option{value: afterExport, label: "copy this into a repository of your own",
			note: "the roots you configured, their modules, and a git history"},
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
	// unreadable is a plan file that exists and could not be read, which is not
	// normal and must not be reported as the line above.
	unreadable error
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
			detail := unitDetail{name: unit.Name, changes: changes}
			if err != nil {
				// A plan file that is there and will not read is a different thing
				// from one that was never made. Reporting both as "not planned in
				// this run" hid the failure while the menu still offered apply.
				if _, statErr := os.Stat(runner.PlanFile(unit)); statErr == nil {
					detail.unreadable = err
				} else {
					detail.unplanned = true
				}
			}
			details = append(details, detail)
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
		case detail.unreadable != nil:
			fmt.Fprintf(out, "\n  %s\n    %s\n", detail.name,
				theme.alert("its saved plan could not be read: "+detail.unreadable.Error()))
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
