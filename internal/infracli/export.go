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
	fmt.Fprintf(out, "  Next:\n    cd %s\n    git remote add origin <url>\n    git push -u origin main\n\n", absolute)
	return nil
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
