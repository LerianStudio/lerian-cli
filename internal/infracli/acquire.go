package infracli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Sentinel rows, kept apart from any path the operator might type.
const (
	acquireHere      = "\x00here"
	acquireElsewhere = "\x00elsewhere"
	acquireExisting  = "\x00existing"
)

// obtainCheckout is what runs when there is no checkout anywhere and somebody is
// at the keyboard.
//
// The question used to be "where is the checkout?", which has no answer when the
// machine has none: the prompt rejected every empty line, named a command to run
// instead, and could not be left for it. Somebody who had just installed the CLI
// could not get past the first screen without quitting.
//
// So the first question is what to do, not where something is, and the default
// row is the one that needs no further decision.
func obtainCheckout(ctx context.Context, ask *prompter, out io.Writer, templatesDir string) (string, error) {
	managed, err := infra.ManagedCheckoutPath(templatesDir)
	if err != nil {
		return "", err
	}

	picked, err := Choose(out, "There is no templates checkout on this machine. Get one?",
		"The Terraform templates every stack is rendered from. About 30 MB, cloned with git.",
		[]Choice{
			{Value: acquireHere, Label: "Clone it into " + tilde(managed), Note: "this tool's own directory"},
			{Value: acquireElsewhere, Label: "Clone it somewhere else", Note: "you choose the directory"},
			{Value: acquireExisting, Label: "I already have a clone", Note: "give the path to it"},
		})
	if err != nil {
		return "", err
	}

	destination := managed
	switch picked {
	case acquireExisting:
		// The question this screen used to open with, now reached by choosing it.
		return askForCheckout(ask, out, templatesDir)
	case acquireElsewhere:
		answered, askErr := askWhereToClone(ask, managed)
		if askErr != nil {
			return "", askErr
		}
		destination = answered
	}

	return cloneInto(ctx, out, destination)
}

// askWhereToClone asks for a directory that does not have to exist yet. It is
// deliberately not promptCheckoutPath: that one rejects anything that is not
// already a checkout, which is every possible answer to this question.
func askWhereToClone(ask *prompter, managed string) (string, error) {
	answer, err := ask.ask("Where should the clone go?",
		"A directory that does not exist yet, or an empty one. The clone is put there as is.",
		managed, "--templates-dir")
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(answer)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", answer, err)
	}
	return absolute, nil
}

// cloneInto picks the tag and clones.
//
// The tag is asked for rather than defaulted. The chart mapping compiled into
// this binary and the HCL in the templates are two halves of one contract, so
// which release is running is a decision — but one made from a list of the tags
// that actually exist and that this binary can read, with the newest first,
// rather than from a flag somebody has to go and look up.
func cloneInto(ctx context.Context, out io.Writer, destination string) (string, error) {
	git, err := infra.NewGitCLI()
	if err != nil {
		return "", err
	}

	ref, err := askForTemplatesRef(ctx, git, out)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", filepath.Dir(destination), err)
	}

	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> Templates"))
	fmt.Fprintf(out, "  git clone %s\n    %s\n    %s\n\n", ref, infra.TemplatesRepoURL(), destination)

	if err := infra.CloneTemplates(ctx, git, destination, ref); err != nil {
		return "", err
	}
	fmt.Fprintf(out, "  cloned.\n")

	// Written down only when it is not where this tool looks by default. The
	// managed path is found by convention on every later run, and recording it
	// would make `config reset` the thing that loses a checkout it can see.
	if recordAfterClone(destination) {
		rememberCheckout(out, destination)
	} else {
		fmt.Fprintf(out, "  Later runs find it here without a flag.\n\n")
	}
	return destination, nil
}

// askForTemplatesRef offers the tags this binary can actually drive, newest
// first. Tags below the floor are left out: offering one the tool would refuse
// two steps later is worse than not offering it.
func askForTemplatesRef(ctx context.Context, git infra.Git, out io.Writer) (string, error) {
	tags, err := git.RemoteTags(ctx, infra.TemplatesRepoURL())
	if err != nil {
		return "", fmt.Errorf("cannot list the templates tags: %w\n"+
			"Pass one yourself with: lerian infra init --clone --templates-ref <tag>", err)
	}

	choices := refChoices(tags)
	if len(choices) == 0 {
		return "", missingRefError(ctx, git, "--clone")
	}
	return Choose(out, "Which templates release?",
		"The clone is pinned to this tag, never to a branch: this binary and the HCL "+
			"are two halves of one contract.", choices)
}

// refChoices is the tags offered, newest first.
//
// Tags this binary cannot read are left out rather than shown and refused: the
// floor exists because the chart mapping compiled in here was written against a
// layout, and a row somebody can pick and then be told no about is worse than a
// row that was never there.
func refChoices(tags []string) []Choice {
	usable := usableRefs(tags)
	if len(usable) == 0 {
		return nil
	}
	infra.SortRefs(usable)

	choices := make([]Choice, 0, len(usable))
	for i, tag := range usable {
		note := ""
		if i == 0 {
			note = "newest"
		}
		choices = append(choices, Choice{Value: tag, Label: tag, Note: note})
	}
	return choices
}

// recordAfterClone reports whether the destination has to be written down.
//
// The managed path is found by convention on every later run, so recording it
// would add nothing and would make `config reset` the thing that loses a
// checkout still sitting there. Anywhere else is found by nothing at all.
func recordAfterClone(destination string) bool {
	managed, err := infra.ManagedCheckoutPath("")
	return err != nil || destination != managed
}

// tilde shortens a path under the home directory, for display only. A menu row
// is one line wide and the home prefix is the part nobody needs to read.
func tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	prefix := home + string(filepath.Separator)
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	return "~" + string(filepath.Separator) + strings.TrimPrefix(path, prefix)
}
