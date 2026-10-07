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
	// Roots are the Terraform roots being exported, by their repo-relative path.
	Roots []string
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

// Files is everything the plan copies, in a stable order.
func (p ExportPlan) Files() []string {
	all := make([]string, 0, len(p.Roots)+len(p.Modules)+len(p.Config))
	all = append(all, p.Roots...)
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
				if rel, relErr := filepath.Rel(layout.Root, target); relErr == nil {
					plan.Modules = append(plan.Modules, rel)
				}
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
			plan.Roots = append(plan.Roots, rel)
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

	var readme strings.Builder
	fmt.Fprintf(&readme, "# Infrastructure\n\n")
	fmt.Fprintf(&readme, "Terraform for this estate, generated by **%s** (Lerian Studio) "+
		"from lerian-terraform-foundation", Generator())
	if templatesRef != "" {
		fmt.Fprintf(&readme, " %s", templatesRef)
	}
	fmt.Fprintf(&readme, ".\n\nIt is a copy, not a link: edit anything here. "+
		"Nothing reaches back into the templates, and nothing from them reaches in.\n\n")
	fmt.Fprintf(&readme, "Every Terraform file carries the same note at the top, because "+
		"that is where the question gets asked.\n\n")

	fmt.Fprintf(&readme, "## What is here\n\n")
	fmt.Fprintf(&readme, "- `environments.conf` — which AWS account each environment may touch\n")
	fmt.Fprintf(&readme, "- `backend/*.hcl` — where each environment keeps its state\n")
	fmt.Fprintf(&readme, "- `envs/<env>.tfvars` under each root — the sizing and the values that were chosen\n")
	fmt.Fprintf(&readme, "\nRoots:\n\n")
	for _, root := range plan.Roots {
		fmt.Fprintf(&readme, "- `%s`\n", plan.Target(root))
	}
	if len(plan.Modules) > 0 {
		fmt.Fprintf(&readme, "\nShared modules they use: ")
		names := make([]string, 0, len(plan.Modules))
		for _, module := range plan.Modules {
			names = append(names, "`"+filepath.Base(module)+"`")
		}
		fmt.Fprintf(&readme, "%s.\n", strings.Join(names, ", "))
	}

	fmt.Fprintf(&readme, "\n## Running it\n\n```bash\ncd infra-base/vpc\n"+
		"terraform init -backend-config=../../backend/dev.hcl \\\n"+
		"  -backend-config=\"key=aws/infra-base/vpc/terraform.tfstate\"\n"+
		"terraform plan -var-file=envs/dev.tfvars\n```\n\n")
	fmt.Fprintf(&readme, "The roots sit at the top level. In the templates they live under "+
		"`examples/aws/`, which is the right name there and the wrong one here — this is "+
		"not an example of an estate, it is yours. The directory was dropped from every "+
		"path at once, so each `source = \"../../_modules/...\"` still points where it "+
		"did: removing the same leading directories from both ends of a relative path "+
		"leaves it unchanged.\n\n")
	fmt.Fprintf(&readme, "## Adding a root the copy does not have\n\n"+
		"This repository holds the roots that were configured, not all of them. "+
		"To add one, copy its directory out of the templates at the same tag and "+
		"write its `envs/<env>.tfvars`.\n")

	if err := os.WriteFile(filepath.Join(destination, "README.md"), []byte(readme.String()), 0o600); err != nil {
		return fmt.Errorf("infra: cannot write README.md: %w", err)
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
