package infracli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	return exportRepository(ctx, newPrompter(out), out, destination)
}

// exportRepository is the same thing with the prompter supplied.
//
// Split out because the post-run menu already has one, and building a second
// from the same writer is a guess about whether anybody is there — a guess the
// caller does not have to make. It is also what lets the questions below be
// answered in a test.
func exportRepository(ctx context.Context, ask *prompter, out io.Writer, destination string) error {
	layout, source, err := resolveLayout("", os.Getenv("LERIAN_TF_REPO"), "")
	if err != nil {
		return err
	}

	absolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %w", destination, err)
	}
	if err := clearTheWay(ctx, ask, out, absolute, checkoutsInUse(layout)); err != nil {
		return err
	}

	catalog, err := infra.Discover(layout)
	if err != nil {
		return err
	}
	units := configuredUnits(layout, catalog)
	if len(units) == 0 {
		// Not --repo: this command does not take one, and neither does the menu
		// row that reaches it. A hint naming a flag that does not exist costs
		// somebody an unknown-flag error on top of the problem they already have.
		return fmt.Errorf("nothing is configured in %s yet\n"+
			"A root is exported once it has an envs/<env>.tfvars — run lerian infra first,\n"+
			"or point at the checkout that holds your configuration:\n"+
			"  lerian config templates <path>     (records it)\n"+
			"  LERIAN_TF_REPO=<path> lerian …     (this shell only)",
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
	fmt.Fprintf(out, "  roots     %s\n", nameAFew(targetsOf(plan, plan.RootPaths())))
	fmt.Fprintf(out, "  modules   %s\n", nameAFew(baseNames(plan.Modules)))
	fmt.Fprintf(out, "  config    %s\n\n", nameAFew(baseNames(plan.Config)))

	// Read before the copy: every Terraform file gets a provenance banner naming
	// it, so the tag has to be known before the first byte is written.
	ref := templatesRefOf(ctx, layout)

	written, err := infra.Export(layout, plan, absolute, ref)
	if err != nil {
		return err
	}

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
	offerGitHub(ctx, ask, out, absolute, describeExport(ref))
	return nil
}

// targetsOf is a set of repo-relative paths as the export writes them. The
// report has to name where the files land, not where they came from: "roots
// examples/aws/bootstrap" describes a directory the exported repository does
// not have.
func targetsOf(plan infra.ExportPlan, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, plan.Target(path))
	}
	return out
}

// describeExport is the one line GitHub shows beside the repository name.
//
// Written rather than asked for. It is the same sentence every time — what this
// is and where it came from — and a prompt whose answer is always the same
// answer is a prompt that should have been a default. The templates tag is in
// it because "which version of the templates was this taken at" has no other
// home once the repository leaves this machine.
func describeExport(templatesRef string) string {
	description := "Terraform infrastructure, generated by " + infra.Generator() +
		" (Lerian Studio) from lerian-terraform-foundation"
	if templatesRef != "" {
		description += " " + templatesRef
	}
	return description
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
	GHOwners(ctx context.Context) []infra.GHOwner
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
func offerGitHub(ctx context.Context, ask *prompter, out io.Writer, destination, description string) {
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

	// Local first, and local is where the cursor starts. The repository on disk is
	// finished and is a complete answer on its own; the other row publishes an
	// estate's layout to a server, and a stray enter must not be what does that.
	wanted, err := ask.pick("This repository is ready. Publish it?",
		"It is already a git repository with one commit, here on this machine.", "",
		[]option{
			{value: "local", label: "keep it here", note: "nothing leaves this machine"},
			{value: "yes", label: "create it on GitHub and push",
				note: "gh repo create, then push this commit"},
		}, "")
	if err != nil || wanted != "yes" {
		pushByHand(out, destination)
		return
	}

	if !signedIntoGitHub(ctx, ask, out, gh) {
		pushByHand(out, destination)
		return
	}
	createOnGitHub(ctx, ask, out, gh, destination, description)
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
func createOnGitHub(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	gh gitHub,
	destination, description string,
) {
	owner, err := pickOwner(ctx, ask, gh)
	if err != nil {
		pushByHand(out, destination)
		return
	}

	name, err := ask.ask("What should the repository be called?",
		nameUnder(owner), filepath.Base(destination), "")
	if err != nil {
		pushByHand(out, destination)
		return
	}
	name = qualify(owner, name)

	visibility, err := pickVisibility(ctx, ask, out, destination, name)
	if err != nil {
		pushByHand(out, destination)
		return
	}

	createWithRetries(ctx, ask, out, gh, destination, infra.GHRepo{
		Name:        name,
		Private:     visibility == "private",
		Description: description,
	})
}

// ownerTypeAnother is the row for an owner the API did not list.
const ownerTypeAnother = "\x00owner"

// pickOwner asks where the repository goes: the account, or one of the
// organizations it belongs to.
//
// Asked because the alternative is silent. gh creates in the personal account
// when the name is unqualified, and somebody whose work lives in an organization
// finds that out after the push — with the estate on a server under their own
// name, and a second repository to delete.
//
// Skipped when there is one answer. An account with no organizations gets the
// question it would always answer the same way, which is no question at all.
func pickOwner(ctx context.Context, ask *prompter, gh gitHub) (string, error) {
	owners := gh.GHOwners(ctx)
	if len(owners) == 1 {
		return owners[0].Login, nil
	}

	options := make([]option, 0, len(owners)+1)
	for _, owner := range owners {
		note := "your account"
		if owner.Organization {
			note = "organization"
		}
		options = append(options, option{value: owner.Login, label: owner.Login, note: note})
	}
	options = append(options, option{value: ownerTypeAnother, label: "somewhere else",
		note: "an owner this login's token cannot list"})

	picked, err := ask.pick("Where should it be created?",
		ownerPurpose(owners), "", options, "")
	if err != nil {
		return "", err
	}
	if picked != ownerTypeAnother {
		return picked, nil
	}
	return ask.ask("Which owner?", "A user or organization you can create in.", "", "")
}

// ownerPurpose says why the list might be shorter than expected.
//
// The organizations come from the API, and reading them needs the read:org
// scope. Without it the list is the account alone — which looks like the
// organizations do not exist rather than like they were not visible.
func ownerPurpose(owners []infra.GHOwner) string {
	for _, owner := range owners {
		if owner.Organization {
			return "The account signed in to gh, and the organizations it belongs to."
		}
	}
	return "Only your account is listed. Organizations need the read:org scope: gh auth refresh -s read:org"
}

// nameUnder is the purpose line of the name prompt, which has to say where the
// name lands — by then the owner is two questions ago.
func nameUnder(owner string) string {
	if owner == "" {
		return "The name only. Where it goes was the previous answer."
	}
	return "It will be created as " + owner + "/<name>."
}

// qualify puts the owner in front of the name, unless the name already carries
// one. Somebody who typed "acme/estate" at the name prompt meant it, and
// prefixing that would produce an owner nobody has.
func qualify(owner, name string) string {
	if owner == "" || strings.Contains(name, "/") {
		return name
	}
	return owner + "/" + name
}

// pickVisibility asks public or private, and makes public cost a typed answer.
//
// Private first, and private is where the cursor starts. This repository holds
// no credentials, but it is a map of an estate — account numbers, state buckets,
// subnets, cluster names — and public is a decision to arrive at deliberately.
//
// A menu row is not that decision. It is one keypress, it sits next to the other
// one, and it is irreversible in the way that matters: a repository made public
// has been readable by crawlers before anybody notices, and making it private
// afterwards does not unpublish what was already fetched.
func pickVisibility(ctx context.Context, ask *prompter, out io.Writer, destination, name string) (string, error) {
	for {
		visibility, err := ask.pick("Public or private?",
			"The next answer creates "+name+" and pushes this commit.", "",
			[]option{
				{value: "private", label: "private", note: "only you and who you invite"},
				{value: "public", label: "public", note: "anyone can read your infrastructure"},
			}, "")
		if err != nil {
			return "", err
		}
		if visibility != "public" {
			return visibility, nil
		}

		if confirmPublic(ctx, ask, out, destination, name) {
			return visibility, nil
		}
		// Back to the same question rather than out of the flow: somebody who
		// declined "public" almost always wants the other row, and ending the
		// export here would make them start over to say so.
	}
}

// confirmPublic reads back what publishing would disclose, and takes a typed
// answer.
//
// Named rather than alluded to. "It may contain sensitive information" is a
// sentence people click past, because it describes a possibility; the account
// number is a fact, and seeing it is the difference between a warning and a
// decision.
func confirmPublic(ctx context.Context, ask *prompter, out io.Writer, destination, name string) bool {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n  %s\n", theme.alert("A public repository is readable by anyone, including crawlers."))
	fmt.Fprintf(out, "  %s\n", "Pushing this publishes:")
	for _, fact := range infra.WhatPublishingReveals(destination) {
		fmt.Fprintf(out, "    %s\n", fact)
	}
	fmt.Fprintf(out, "  %s\n", theme.dim(
		"Making it private later does not unpublish what was already read."))

	return ask.confirm(ctx, out, "Publish "+name+" publicly?") == nil
}

// createWithRetries creates the repository, asking for another name when that is
// what went wrong.
//
// A taken name is the common failure and the one where stopping is the wrong
// answer: everything is ready, the operator is right there, and the fix is a
// different word. Every other failure stops — there is nothing this screen can
// do about a missing scope, and asking again would only produce it twice.
func createWithRetries(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	gh gitHub,
	destination string,
	repo infra.GHRepo,
) {
	const attempts = 3

	for attempt := 1; ; attempt++ {
		url, err := gh.CreateRepository(ctx, destination, repo)
		if err == nil {
			if url == "" {
				fmt.Fprintf(out, "  Created and pushed.\n\n")
				return
			}
			fmt.Fprintf(out, "  Created and pushed: %s\n\n", url)
			return
		}

		fmt.Fprintf(out, "\n  %v\n", err)

		var failure *infra.CreateFailure
		if !errors.As(err, &failure) || !failure.NameTaken || attempt >= attempts {
			fmt.Fprintln(out)
			// A repository that exists must not be followed by instructions to add
			// a remote: that one is already there, and the push is what failed.
			if failure != nil && failure.Created {
				return
			}
			pushByHand(out, destination)
			return
		}

		another, askErr := ask.ask("Another name?",
			"The repository and its commit are already on disk; only the name is in the way.",
			"", "")
		if askErr != nil {
			fmt.Fprintln(out)
			pushByHand(out, destination)
			return
		}
		// Under the same owner. The owner was chosen and is not what went wrong;
		// asking for it again would be a second question about a settled answer,
		// and dropping it would quietly move the retry to the personal account.
		repo.Name = qualify(ownerOf(repo.Name), another)
	}
}

// clearTheWay makes sure the destination is empty, asking about it when it is
// not.
//
// An export is a copy of a whole tree with a history of its own; landing it on
// top of something else mixes the two. That used to be the end of the matter —
// "give a path that does not exist yet" — which is the right answer when the
// occupant is somebody's work and a pointless obstacle when it is last week's
// export of the same estate, which is the common case.
//
// So it describes what is there and offers to replace it. Described first,
// because "not empty" is not something anybody can decide from: whether that
// directory is a scratch copy or six months of work is the entire question.
func clearTheWay(ctx context.Context, ask *prompter, out io.Writer, path string, inUse []string) error {
	occupant, err := infra.Inspect(ctx, path)
	if err != nil {
		return err
	}
	if occupant.Empty() {
		return nil
	}

	// Nobody to ask. The old refusal is still the right answer here, and a script
	// that meant to replace a directory can empty it itself.
	if !ask.interactive {
		return refuseNonEmpty(path, occupant)
	}
	// And some paths are refused whatever the answer: a confirmation is consent
	// to lose what was described, and in these the two are not the same thing.
	if err := infra.RefuseToEmpty(path, inUse); err != nil {
		return err
	}

	theme := newStyle(out)
	fmt.Fprintf(out, "\n  %s\n", theme.bold(path+" already has something in it"))
	fmt.Fprintf(out, "  %s\n\n", occupant.Describe())

	answer, err := ask.pick("Replace it?", replacePurpose(occupant), "",
		[]option{
			{value: "no", label: "leave that directory alone", note: "nothing is removed"},
			{value: "yes", label: "replace everything there",
				note: "deletes what is in that directory, then writes the export"},
		}, "")
	if err != nil {
		return err
	}
	if answer != "yes" {
		return errBack
	}

	// A second answer, typed, when what is there exists nowhere else. The menu
	// above is one keypress, and one keypress is not the right price for work
	// that no clone anywhere holds.
	if occupant.AtRisk() {
		if err := ask.confirm(ctx, out,
			"Delete "+path+" and everything in it? This cannot be undone"); err != nil {
			return err
		}
	}

	if err := infra.EmptyDirectory(path, inUse); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n  Emptied %s\n", path)
	return nil
}

// replacePurpose says what is at stake, which depends on whether anything else
// in the world holds a copy.
func replacePurpose(occupant infra.Occupant) string {
	if occupant.AtRisk() {
		return "What is there is not on any remote. Replacing it loses it for good."
	}
	if occupant.Repository {
		return "Its commits are on a remote, so that copy survives. This directory does not."
	}
	return "Everything in that directory is deleted first."
}

// refuseNonEmpty keeps the export from writing into somebody's existing work
// when there is nobody to ask about it.
func refuseNonEmpty(path string, occupant infra.Occupant) error {
	return fmt.Errorf("%s is not empty: %s\n"+
		"An export writes a whole tree and a git history; landing it on top of\n"+
		"something else mixes the two. Give a path that does not exist yet, or\n"+
		"run this from a terminal, where it offers to replace what is there",
		path, occupant.Describe())
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

// ownerOf is the owner part of a qualified name, or empty when there is none.
func ownerOf(name string) string {
	if cut := strings.Index(name, "/"); cut >= 0 {
		return name[:cut]
	}
	return ""
}

// checkoutsInUse is every lerian-terraform-foundation checkout something depends
// on: the one this run resolved, the one recorded in the config, and the managed
// paths a later run would discover.
//
// This is the list, rather than "anything shaped like a checkout". An export
// taken before the layout changed has the same examples/aws/_modules and
// examples/aws/backend inside it, and recognizing it by shape made the common
// case — replacing last week's export — impossible to reach.
func checkoutsInUse(layout infra.Layout) []string {
	managed := infra.ManagedCheckoutPaths("")

	paths := make([]string, 0, 3+len(managed))
	paths = append(paths, layout.Root, recordedCheckout(), os.Getenv("LERIAN_TF_REPO"))
	return append(paths, managed...)
}
