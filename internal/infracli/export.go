package infracli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// ExportRepository copies what was configured into a repository of its own.
//
// The templates are somebody else's repository on somebody else's release
// cycle, and an estate that lives inside a checkout of them has to ask
// permission to change anything. This makes the copy that does not: the roots
// that were configured, the modules they reach, the configuration that makes
// them runnable, and a git history with one commit in it.
//
// It stops before `git push`. The remote is the operator's to choose, and a tool
// that guesses where somebody's infrastructure gets published has guessed about
// the wrong thing.
func ExportRepository(ctx context.Context, out io.Writer, destination string) error {
	layout, source, err := resolveLayout("", os.Getenv("LERIAN_TF_REPO"), "")
	if err != nil {
		return err
	}

	absolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %w", destination, err)
	}
	if err := refuseNonEmpty(absolute); err != nil {
		return err
	}

	catalog, err := infra.Discover(layout)
	if err != nil {
		return err
	}
	units := configuredUnits(layout, catalog)
	if len(units) == 0 {
		return fmt.Errorf("nothing is configured in %s yet\n"+
			"A root is exported once it has an envs/<env>.tfvars — run lerian infra first,\n"+
			"or point at the checkout that holds your configuration with --repo",
			layout.RepoRel(layout.AWSDir()))
	}

	plan, err := infra.PlanExport(layout, units)
	if err != nil {
		return err
	}

	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> Exporting"))
	fmt.Fprintf(out, "  from      %s  %s\n", layout.Root, theme.dim("("+explainSource(source)+")"))
	fmt.Fprintf(out, "  to        %s\n", absolute)
	fmt.Fprintf(out, "  roots     %s\n", nameAFew(plan.Roots))
	fmt.Fprintf(out, "  modules   %s\n", nameAFew(baseNames(plan.Modules)))
	fmt.Fprintf(out, "  config    %s\n\n", nameAFew(baseNames(plan.Config)))

	written, err := infra.Export(layout, plan, absolute)
	if err != nil {
		return err
	}

	ref := templatesRefOf(ctx, layout)
	if err := infra.WriteExportMeta(absolute, ref, plan); err != nil {
		return err
	}

	git, err := infra.NewGitCLI()
	//nolint:nilerr // The copy is done and it is what was asked for; only the
	// history is missing, and the operator can run git init themselves. Failing
	// here would report "export failed" over a tree that is sitting there
	// complete.
	if err != nil {
		fmt.Fprintf(out, "  %d file(s) written. git is not installed, so no repository was made.\n\n", written)
		return nil
	}
	// Same: the files are there either way, and what failed is printed rather
	// than swallowed.
	if err := infra.InitRepository(ctx, git, absolute, ref); err != nil {
		fmt.Fprintf(out, "  %d file(s) written, but the repository was not initialized: %v\n\n", written, err)
		return nil
	}

	fmt.Fprintf(out, "  %d file(s), one commit, branch main.\n\n", written+2)
	offerGitHub(ctx, newPrompter(out), out, absolute)
	return nil
}

// pushByHand is what to do with the repository when this tool is not doing it.
func pushByHand(out io.Writer, destination string) {
	fmt.Fprintf(out, "  Next:\n    cd %s\n    git remote add origin <url>\n    git push -u origin main\n\n",
		destination)
}

// gitHub is what the offer below needs from gh, as an interface so the whole
// flow can be exercised without a GitHub account and without creating anything.
type gitHub interface {
	GHStatus(ctx context.Context) (infra.GHAccount, bool)
	GHAccounts(ctx context.Context) []infra.GHAccount
	GHLogin(ctx context.Context, in io.Reader, out, errOut io.Writer) error
	GHSwitch(ctx context.Context, account infra.GHAccount) error
	GHLogout(ctx context.Context, account infra.GHAccount) error
	GitProtocol(ctx context.Context) string
	SetGitProtocol(ctx context.Context, protocol string) error
	CreateRepository(ctx context.Context, dir string, repo infra.GHRepo) (string, error)
}

// newGitHub is a variable for the same reason.
var newGitHub = func() (gitHub, error) {
	gh, err := infra.NewGHCLI()
	if err != nil {
		return nil, err
	}
	return gh, nil
}

// offerGitHub asks whether to put the export on GitHub, and does it.
//
// The export deliberately stops before the remote: where somebody's
// infrastructure gets published is not a guess to make. Asking is not guessing —
// and the alternative, which is what this replaces, was a three-line recipe
// ending in a repository they still had to go and create in a browser first.
//
// Nothing happens without three separate answers: yes, this name, this
// visibility. Each one is a thing somebody could want different, and the last is
// the one that cannot be taken back by deleting a directory.
func offerGitHub(ctx context.Context, ask *prompter, out io.Writer, destination string) {
	if ask == nil || !ask.interactive {
		pushByHand(out, destination)
		return
	}

	gh, err := newGitHub()
	if err != nil {
		// Said, not hidden. The row in `lerian infra check` says the same thing,
		// and somebody who expected to be offered this is owed the reason.
		fmt.Fprintf(out, "  %s\n\n", newStyle(out).dim(
			"gh is not installed, so the repository cannot be created from here"))
		pushByHand(out, destination)
		return
	}

	wanted, err := ask.pick("Create this on GitHub and push it?",
		"Creates the repository and pushes this commit to it. Nothing else is published.", "",
		[]option{
			{value: "yes", label: "create it on GitHub", note: "gh repo create, then push"},
			{value: "no", label: "not now", note: "leaves the commands below"},
		}, "")
	if err != nil || wanted != "yes" {
		pushByHand(out, destination)
		return
	}

	if !signedIntoGitHub(ctx, ask, out, gh) {
		pushByHand(out, destination)
		return
	}
	createOnGitHub(ctx, ask, out, gh, destination)
}

// signedIntoGitHub makes sure gh can act, logging in if it cannot.
//
// Offered rather than reported. "Run gh auth login and start over" is a round
// trip through a second program, for a command this one can run — and it is run
// with the terminal handed over, because gh prints a code to paste into a
// browser and waits for it.
func signedIntoGitHub(ctx context.Context, ask *prompter, out io.Writer, gh gitHub) bool {
	if _, loggedIn := gh.GHStatus(ctx); loggedIn {
		return true
	}

	answer, err := ask.pick("gh is not logged in. Log in now?",
		"Runs gh auth login, which prints a code and opens a browser.", "",
		[]option{
			{value: "yes", label: "log in now", note: "hands this terminal to gh"},
			{value: "no", label: "cancel", note: "leaves the commands below"},
		}, "")
	if err != nil || answer != "yes" {
		return false
	}

	fmt.Fprintln(out)
	if err := gh.GHLogin(ctx, os.Stdin, out, out); err != nil {
		fmt.Fprintf(out, "\n  %v\n\n", err)
		return false
	}

	// Asked again rather than assumed: gh auth login exits zero on paths that
	// leave no usable credential, and the next command would fail for a reason
	// that reads as a bug in this one.
	_, loggedIn := gh.GHStatus(ctx)
	if !loggedIn {
		fmt.Fprintf(out, "\n  %s\n\n", "gh still reports no login, so nothing was created.")
	}
	return loggedIn
}

// createOnGitHub asks for the name and the visibility, then creates it.
func createOnGitHub(ctx context.Context, ask *prompter, out io.Writer, gh gitHub, destination string) {
	name, err := ask.ask("What should the repository be called?",
		"<name> for your own account, or <owner>/<name> for an organization.",
		filepath.Base(destination), "")
	if err != nil {
		pushByHand(out, destination)
		return
	}

	// Private first, and private is where the cursor starts. This repository is
	// not a secret in the sense of holding credentials, but it is a map of an
	// estate — account numbers, VPC layout, cluster names — and public is a
	// decision somebody should arrive at deliberately.
	visibility, err := ask.pick("Public or private?",
		"The next answer creates "+name+" and pushes this commit.", "",
		[]option{
			{value: "private", label: "private", note: "only you and who you invite"},
			{value: "public", label: "public", note: "anyone can read your infrastructure"},
		}, "")
	if err != nil {
		pushByHand(out, destination)
		return
	}

	url, err := gh.CreateRepository(ctx, destination, infra.GHRepo{
		Name:    name,
		Private: visibility == "private",
	})
	if err != nil {
		fmt.Fprintf(out, "\n  %v\n\n", err)
		pushByHand(out, destination)
		return
	}

	if url == "" {
		fmt.Fprintf(out, "  Created and pushed.\n\n")
		return
	}
	fmt.Fprintf(out, "  Created and pushed: %s\n\n", url)
}

// refuseNonEmpty keeps the export from writing into somebody's existing work.
//
// An export is a copy of a whole tree; landing it on top of a repository that is
// already there mixes two histories and leaves no obvious way back.
func refuseNonEmpty(path string) error {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s is not empty\n"+
			"An export writes a whole tree and a git history; landing it on top of\n"+
			"something else mixes the two. Give a path that does not exist yet", path)
	}
	return nil
}

// configuredUnits is every root this checkout has variables for, in the order
// they would run.
//
// Configured rather than all: the repository holds what somebody set up, which
// is the thing they asked to take with them. The twenty-seven products they
// never touched would be twenty-seven directories of someone else's decisions.
func configuredUnits(layout infra.Layout, catalog infra.Catalog) []infra.Unit {
	stages, err := infra.Resolve(layout, catalog, "all")
	if err != nil {
		return nil
	}

	var configured []infra.Unit
	for _, unit := range infra.Units(stages) {
		for _, env := range infra.Environments {
			// #nosec G304 G703 -- VarFile(unit, env): unit.Dir comes from walking
			// the checkout for main.tf and env is one of Environments. Neither is
			// operator input, and this only asks whether the file is there.
			if _, statErr := os.Stat(infra.VarFile(unit, env)); statErr == nil {
				configured = append(configured, unit)
				break
			}
		}
	}
	return configured
}

// templatesRefOf is the tag the checkout is on, or "" when it is not on one.
func templatesRefOf(ctx context.Context, layout infra.Layout) string {
	git, err := infra.NewGitCLI()
	if err != nil {
		return ""
	}
	return git.DescribeRef(ctx, layout.Root)
}

// baseNames shortens paths to their last segment, for a line that names things
// rather than locating them.
func baseNames(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, filepath.Base(path))
	}
	return out
}
