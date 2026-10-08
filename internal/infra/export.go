package infra

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	cliversion "github.com/lerian-studio/lerian-cli/internal/version"
)

// localSource matches a module source that points inside this repository.
// Registry and git sources are left alone: they are fetched, not copied.
var localSource = regexp.MustCompile(`source\s*=\s*"(\.{1,2}/[^"]+)"`)

// ExportPlan is what an export would copy, worked out before anything is
// written.
type ExportPlan struct {
	// Roots are the Terraform roots being exported.
	Roots []ExportedRoot
	// Modules are the local modules those roots reach, directly or through each
	// other.
	Modules []string
	// Config is environments.conf and the backend files, by repo-relative path.
	Config []string
	// Missing names a root that was asked for and is not there.
	Missing []string
	// Base is the directory every path above sits under, and which becomes the
	// root of the exported repository. Normally "examples/aws".
	//
	// Stripping it is safe precisely because it is common to all of them: a
	// module source like "../../_modules/naming" is relative, and removing the
	// same leading directories from both ends of a relative path leaves it
	// pointing at the same place. Empty means nothing was stripped, and the
	// template's layout is reproduced as it was.
	Base string
}

// Target is where a repo-relative path lands in the export.
func (p ExportPlan) Target(rel string) string {
	if p.Base == "" {
		return rel
	}
	out, err := filepath.Rel(p.Base, rel)
	if err != nil || out == ".." || strings.HasPrefix(out, ".."+string(filepath.Separator)) {
		// Outside the base. Cannot happen while Base is chosen by the rule below,
		// and if it ever does, reproducing the path unchanged is the answer that
		// cannot put a file somewhere surprising.
		return rel
	}
	return out
}

// Targets is Files, as the export writes them.
func (p ExportPlan) Targets() []string {
	files := p.Files()
	out := make([]string, 0, len(files))
	for _, rel := range files {
		out = append(out, p.Target(rel))
	}
	return out
}

// ExportedRoot is one root, with what somebody needs to run it after the copy
// has left this machine.
//
// The state key travels with it because it cannot be guessed. It is derived from
// the directory, and a README that showed one worked example left every other
// root to be inferred — and an inferred key that is wrong does not fail: it
// initializes an empty state and plans to create an estate that already exists.
type ExportedRoot struct {
	// Path is the repo-relative path in the templates.
	Path string
	// StateKey is the object key inside the state bucket.
	StateKey string
	// Bootstrap marks the root that runs on local state with a workspace per
	// environment, because it is the stack that creates the backend. Its init
	// takes no -backend-config at all.
	Bootstrap bool
}

// RootPaths is the roots by path, for the callers that only name them.
func (p ExportPlan) RootPaths() []string {
	paths := make([]string, 0, len(p.Roots))
	for _, root := range p.Roots {
		paths = append(paths, root.Path)
	}
	return paths
}

// Files is everything the plan copies, in a stable order.
func (p ExportPlan) Files() []string {
	all := make([]string, 0, len(p.Roots)+len(p.Modules)+len(p.Config))
	all = append(all, p.RootPaths()...)
	all = append(all, p.Modules...)
	all = append(all, p.Config...)
	sort.Strings(all)
	return all
}

// PlanExport works out what to copy for a set of roots.
//
// The modules are followed rather than listed: a root names them with a relative
// path, and a module can name another. A list held here would be a second copy
// of what the HCL says and would go stale the first time a root picks up a
// dependency — and the failure that produces is a terraform init that cannot
// find a directory, after the repository has been handed over.
func PlanExport(layout Layout, units []Unit) (ExportPlan, error) {
	plan := ExportPlan{}
	seen := map[string]bool{}

	var follow func(dir string) error
	follow = func(dir string) error {
		if seen[dir] {
			return nil
		}
		seen[dir] = true

		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("infra: cannot read %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
				continue
			}
			content, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
			if readErr != nil {
				return fmt.Errorf("infra: cannot read %s: %w", entry.Name(), readErr)
			}
			for _, match := range localSource.FindAllStringSubmatch(string(content), -1) {
				target := filepath.Clean(filepath.Join(dir, match[1]))
				if _, statErr := os.Stat(target); statErr != nil {
					// A source that does not resolve is the template's problem, and
					// terraform reports it better than this can.
					continue
				}
				if err := follow(target); err != nil {
					return err
				}
				rel, relErr := filepath.Rel(layout.Root, target)
				if relErr != nil {
					continue
				}
				// A source like ../../../../../shared resolves outside the
				// checkout, and filepath.Rel happily returns a path starting with
				// "..". Joined to the destination, copyTree would then write the
				// module into a sibling of the export — outside the directory the
				// operator named. Refused rather than clamped: a copy that silently
				// drops a module terraform needs is not better than one that says
				// what it cannot do.
				if escapesRoot(rel) {
					return fmt.Errorf("infra: %s uses a module outside the checkout (%s);"+
						" it cannot be exported", layout.rel(dir), rel)
				}
				plan.Modules = append(plan.Modules, rel)
			}
		}
		return nil
	}

	for _, unit := range units {
		if _, err := os.Stat(unit.Dir); err != nil {
			plan.Missing = append(plan.Missing, unit.Name)
			continue
		}
		if err := follow(unit.Dir); err != nil {
			return plan, err
		}
		if rel, err := filepath.Rel(layout.Root, unit.Dir); err == nil {
			if escapesRoot(rel) {
				return plan, fmt.Errorf("infra: %s is outside the checkout; it cannot be exported", rel)
			}
			plan.Roots = append(plan.Roots, ExportedRoot{
				Path:      rel,
				StateKey:  unit.StateKey(),
				Bootstrap: unit.Bootstrap,
			})
		}
	}

	// The configuration that makes the copy runnable: which account each
	// environment is, and where its state lives.
	for _, path := range append([]string{layout.ConfigFile()}, backendFiles(layout)...) {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if rel, err := filepath.Rel(layout.Root, path); err == nil {
			plan.Config = append(plan.Config, rel)
		}
	}

	plan.Modules = distinct(plan.Modules)
	sort.Strings(plan.Modules)
	plan.Base = exportBase(layout, plan.Files())
	return plan, nil
}

// exportBase is the directory that becomes the root of the exported repository.
//
// `examples/aws` when everything is under it, which is the layout the templates
// have. The name is the templates' — these are examples of how to deploy — and
// it is wrong in a repository that is somebody's actual estate: the first thing
// they read in their own infrastructure should not be a word saying it is a
// demonstration.
//
// Not stripped when anything sits outside it. A module reached from above that
// directory would have its relative source broken by the move, and a repository
// that reads nicely and does not `terraform init` is worse than one with an
// awkward directory name.
func exportBase(layout Layout, files []string) string {
	base, err := filepath.Rel(layout.Root, layout.AWSDir())
	if err != nil || base == "." || base == "" {
		return ""
	}

	prefix := base + string(filepath.Separator)
	for _, rel := range files {
		if !strings.HasPrefix(rel, prefix) {
			return ""
		}
	}
	return base
}

// escapesRoot is whether a repo-relative path leaves the repository.
//
// filepath.Rel answers "how do I get there from here" and is perfectly happy to
// answer with "..", which is the whole problem: every caller here treats its
// result as a path inside the tree.
func escapesRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// backendFiles is every backend/<env>.hcl that exists.
func backendFiles(layout Layout) []string {
	found := make([]string, 0, len(Environments))
	for _, env := range Environments {
		found = append(found, layout.BackendFile(env))
	}
	return found
}

// distinct keeps the first of each value.
func distinct(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// Generator names what wrote the export, for the banner and the commit. A
// variable so a test can pin it: the version is stamped at build time and is
// "dev" under go test.
var Generator = func() string { return "lerian-cli " + generatorVersion() }

// generatorVersion is swapped in tests. It is a function rather than a constant
// because the linker sets the version, and reading it at package init would
// capture the value before any of that happens in some build modes.
var generatorVersion = func() string { return cliversion.Version }

// bannerFor is the provenance comment put at the top of every Terraform file in
// the export.
//
// In the files rather than only in the README, because the README is read once
// and these are read every time somebody opens the estate. Six months on, the
// question "where did this root come from, and can I pull a newer one" is asked
// with a .tf file on the screen, and this is the only place that answers it
// there.
//
// A comment, so terraform neither parses nor cares about it, and so deleting it
// is a one-line edit by somebody who would rather not have it.
func bannerFor(templatesRef string) string {
	var banner strings.Builder
	fmt.Fprintf(&banner, "# Generated by %s (Lerian Studio)\n", Generator())
	fmt.Fprintf(&banner, "# Source: lerian-terraform-foundation")
	if templatesRef != "" {
		fmt.Fprintf(&banner, " %s", templatesRef)
	}
	fmt.Fprintf(&banner, "\n#\n# This is a copy, not a link. Edit it freely — nothing reaches back.\n\n")
	return banner.String()
}

// bannerable is the file types the banner goes into: every one of them takes a
// # comment on its first line, and every one of them is Terraform's to read.
// Nothing else is touched — a copy that rewrote arbitrary files would be a copy
// somebody cannot trust.
func bannerable(name string) bool {
	switch filepath.Ext(name) {
	case ".tf", ".tfvars", ".hcl":
		return true
	}
	return filepath.Base(name) == "environments.conf"
}

// Export copies a plan into destination and reports how many files it wrote.
//
// Everything under each directory except the working files: .terraform is a
// cache of downloaded modules — 1.5 GB against 6.5 MB of actual content — and
// state belongs in the bucket, not in a repository somebody is about to push.
func Export(layout Layout, plan ExportPlan, destination, templatesRef string) (int, error) {
	banner := bannerFor(templatesRef)

	written := 0
	for _, rel := range plan.Files() {
		source := filepath.Join(layout.Root, rel)
		target := filepath.Join(destination, plan.Target(rel))

		info, err := os.Stat(source)
		if err != nil {
			return written, fmt.Errorf("infra: cannot read %s: %w", rel, err)
		}
		if !info.IsDir() {
			if err := copyFile(source, target, banner); err != nil {
				return written, err
			}
			written++
			continue
		}

		count, err := copyTree(source, target, banner)
		if err != nil {
			return written, err
		}
		written += count
	}
	return written, nil
}

// copyTree copies a directory, skipping what does not belong in a repository.
func copyTree(source, target, banner string) (int, error) {
	written := 0
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == ".terraform" || name == "terraform.tfstate.d" {
				return filepath.SkipDir
			}
			return nil
		}
		if skipFromExport(name) {
			return nil
		}

		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			return relErr
		}
		if copyErr := copyFile(path, filepath.Join(target, rel), banner); copyErr != nil {
			return copyErr
		}
		written++
		return nil
	})
	return written, err
}

// skipFromExport is what never belongs in the generated repository.
func skipFromExport(name string) bool {
	switch {
	case strings.HasPrefix(name, "terraform.tfstate"):
		return true
	case strings.HasSuffix(name, ".tfplan"), name == "tfplan":
		return true
	case strings.HasSuffix(name, ".tfvars-example"):
		// The example is the question; the .tfvars beside it is the answer, and
		// this repository ships answers. Keeping both invites editing the one
		// terraform does not read.
		return true
	case strings.HasPrefix(name, "crash."), name == "crash.log":
		return true
	default:
		return false
	}
}

// copyFile writes one file, creating the directories above it.
func copyFile(source, target, banner string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("infra: cannot create %s: %w", filepath.Dir(target), err)
	}

	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("infra: cannot read %s: %w", source, err)
	}
	defer func() { _ = in.Close() }()

	// 0o600 for everything: these files hold account ids and sizing, and the
	// repository they land in is the operator's to share deliberately.
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("infra: cannot write %s: %w", target, err)
	}
	defer func() { _ = out.Close() }()

	// The banner goes in before the content, and only into the files Terraform
	// reads. Written here rather than appended afterwards so a file is never on
	// disk without it: an interrupted export leaves a tree, and every Terraform
	// file in it should say where it came from.
	if banner != "" && bannerable(source) {
		if _, err := io.WriteString(out, banner); err != nil {
			return fmt.Errorf("infra: cannot write %s: %w", target, err)
		}
	}

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("infra: cannot write %s: %w", target, err)
	}
	return nil
}

// exportIgnore is the .gitignore the generated repository gets.
//
// Deliberately NOT the templates' own. That one ignores envs/*.tfvars,
// backend/*.hcl and environments.conf, which is right for a repository of
// templates and exactly backwards here: in this repository those files are the
// content. What stays ignored is what no repository should carry — state, the
// module cache, saved plans.
const exportIgnore = `# Terraform working files. State lives in S3; .terraform is a download cache.
**/.terraform/*
*.tfstate
*.tfstate.*
terraform.tfstate.d/
*.tfplan
tfplan
crash.log
crash.*.log

# Nothing else: envs/*.tfvars, backend/*.hcl and environments.conf ARE this
# repository. The templates ignore them because there they are somebody's local
# answers; here they are the configuration this repository exists to hold.
`

// WriteExportMeta writes the .gitignore and the README that say what this
// repository is and where it came from.
//
// The provenance matters more than it looks: this is a copy taken at one tag,
// and six months from now the question "what has changed in the templates since"
// has no answer unless the tag is written down.
func WriteExportMeta(destination, templatesRef string, plan ExportPlan) error {
	if err := os.WriteFile(filepath.Join(destination, ".gitignore"), []byte(exportIgnore), 0o600); err != nil {
		return fmt.Errorf("infra: cannot write .gitignore: %w", err)
	}

	readme := exportReadme(templatesRef, plan)
	if err := os.WriteFile(filepath.Join(destination, "README.md"), []byte(readme), 0o600); err != nil {
		return fmt.Errorf("infra: cannot write README.md: %w", err)
	}

	// 0o700: a Makefile is read, not executed, but the rest of this tree is 0o600
	// and a file nobody can read is worse than one nobody runs.
	if err := os.WriteFile(filepath.Join(destination, "Makefile"),
		[]byte(WriteMakefile(plan)), 0o600); err != nil {
		return fmt.Errorf("infra: cannot write Makefile: %w", err)
	}
	return nil
}

// InitRepository makes the copy a git repository with one commit, so the next
// step is genuinely `git remote add` and `git push`.
//
// It stops short of the remote on purpose: the URL is the operator's, pushing is
// irreversible in a way copying is not, and a tool that guesses where somebody's
// infrastructure should be published has guessed about the wrong thing.
func InitRepository(ctx context.Context, git GitCLI, destination, templatesRef string) error {
	message := "chore: infrastructure as configured\n\nGenerated by " + Generator() +
		" from lerian-terraform-foundation"
	if templatesRef != "" {
		message += " " + templatesRef
	}
	message += ".\n\nThis is a copy, not a link: nothing here reaches back into the templates."

	for _, args := range [][]string{
		{"init", "-q", "--initial-branch=main"},
		{"add", "--all"},
		// -c rather than global config: a machine with no user.name configured
		// fails at commit, and this is not the place to set it permanently.
		//
		// The author is the organization rather than whoever ran the export. This
		// commit is not their work — it is a generated tree, and attributing it to
		// the person at the terminal makes `git log` read as though they wrote
		// four thousand lines of Terraform. Everything after it is theirs.
		{"-c", "user.name=lerian-studio", "-c", "user.email=noreply@lerian.studio",
			"commit", "-q", "-m", message},
	} {
		if _, err := git.runRaw(ctx, destination, args...); err != nil {
			return fmt.Errorf("infra: git %s failed in %s: %w", args[0], destination, err)
		}
	}
	return nil
}

// WhatPublishingReveals names the identifiers a reader of this repository would
// come away with.
//
// For the confirmation before a public repository. "It may contain sensitive
// information" is a sentence people click past, because it describes a
// possibility. The account number they are about to publish is a fact, and
// reading it back is the difference between a warning and a decision.
//
// Read from the export itself rather than from what this run happens to know:
// the question is what is in that directory, and the directory is the thing
// being published.
func WhatPublishingReveals(destination string) []string {
	facts := make([]string, 0, 4)

	accounts := distinct(matchesIn(filepath.Join(destination, "environments.conf"), accountLine))
	if len(accounts) > 0 {
		facts = append(facts, "AWS account "+strings.Join(accounts, ", "))
	}

	var buckets []string
	entries, err := os.ReadDir(filepath.Join(destination, "backend"))
	if err == nil {
		for _, entry := range entries {
			buckets = append(buckets, matchesIn(
				filepath.Join(destination, "backend", entry.Name()), bucketLine)...)
		}
	}
	if buckets = distinct(buckets); len(buckets) > 0 {
		facts = append(facts, "state bucket "+strings.Join(buckets, ", "))
	}

	// Said last and said plainly. The two above are the ones that can be read off
	// a file; the rest is the shape of the estate, which is not quotable and is
	// the part somebody mapping a target would actually want.
	return append(facts, "and the layout of the estate: subnets, cluster names, sizing")
}

var (
	accountLine = regexp.MustCompile(`(?m)^\s*account_id\s*=\s*"?(\d+)"?`)
	bucketLine  = regexp.MustCompile(`(?m)^\s*bucket\s*=\s*"([^"]+)"`)
)

// matchesIn pulls one capture group out of a file, or nothing when the file
// cannot be read — this builds a warning, and a warning that fails to print
// because a file was missing would be worse than a shorter one.
func matchesIn(path string, pattern *regexp.Regexp) []string {
	content, err := os.ReadFile(path) // #nosec G304 -- a path inside the export this run just wrote
	if err != nil {
		return nil
	}

	var found []string
	for _, match := range pattern.FindAllStringSubmatch(string(content), -1) {
		found = append(found, match[1])
	}
	return found
}
