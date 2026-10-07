package infra

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A root names its modules with a relative path, and a module can name another.
// A list held in Go would be a second copy of what the HCL says, and it goes
// stale the first time a root picks up a dependency — producing a terraform init
// that cannot find a directory, after the repository has been handed over.
func TestModulesAreFollowedThroughEachOther(t *testing.T) {
	layout := exportCheckout(t)

	// root -> alpha -> beta, and a registry source that must be left alone.
	writeRoot(t, layout, "products/sample/thing", `
module "alpha" { source = "../../../_modules/alpha" }
module "remote" { source = "terraform-aws-modules/vpc/aws" }
`)
	writeModule(t, layout, "alpha", `module "beta" { source = "../beta" }`)
	writeModule(t, layout, "beta", `output "x" { value = 1 }`)
	writeModule(t, layout, "unused", `output "y" { value = 2 }`)

	plan, err := PlanExport(layout, []Unit{{
		Name: "products/sample/thing",
		Dir:  filepath.Join(layout.AWSDir(), "products", "sample", "thing"),
	}})
	if err != nil {
		t.Fatal(err)
	}

	names := baseNamesOf(plan.Modules)
	for _, want := range []string{"alpha", "beta"} {
		if !slices.Contains(names, want) {
			t.Errorf("%q was not followed: %v", want, names)
		}
	}
	if slices.Contains(names, "unused") {
		t.Errorf("a module nothing references was exported: %v", names)
	}
	// A registry source is fetched, not copied, and a directory for it does not
	// exist to copy.
	if slices.Contains(names, "vpc") {
		t.Errorf("a registry module was treated as a local one: %v", names)
	}
}

// The configuration is what makes the copy runnable: without it, somebody who
// clones the repository has Terraform and no idea which account it is for.
func TestTheConfigurationTravelsWithIt(t *testing.T) {
	layout := exportCheckout(t)
	writeRoot(t, layout, "bootstrap", `output "x" { value = 1 }`)
	writeFile(t, layout.ConfigFile(), "[dev]\naccount_id = 111122223333\n")
	writeFile(t, layout.BackendFile("dev"), `bucket = "example"`)

	plan, err := PlanExport(layout, []Unit{{Name: "bootstrap", Dir: filepath.Join(layout.AWSDir(), "bootstrap")}})
	if err != nil {
		t.Fatal(err)
	}

	names := baseNamesOf(plan.Config)
	for _, want := range []string{"environments.conf", "dev.hcl"} {
		if !slices.Contains(names, want) {
			t.Errorf("%q did not travel with the export: %v", want, names)
		}
	}
}

// .terraform is a download cache — 1.5 GB against 6.5 MB of content — and state
// belongs in the bucket, not in a repository somebody is about to push.
func TestTheCacheAndTheStateStayBehind(t *testing.T) {
	layout := exportCheckout(t)
	root := filepath.Join(layout.AWSDir(), "bootstrap")
	writeRoot(t, layout, "bootstrap", `output "x" { value = 1 }`)
	writeFile(t, filepath.Join(root, ".terraform", "modules", "cached.tf"), "cached")
	writeFile(t, filepath.Join(root, "terraform.tfstate"), "{}")
	writeFile(t, filepath.Join(root, "dev.tfplan"), "plan")
	writeFile(t, filepath.Join(root, "envs", "dev.tfvars-example"), "example = true")
	writeFile(t, filepath.Join(root, "envs", "dev.tfvars"), "real = true")

	destination := t.TempDir()
	plan, err := PlanExport(layout, []Unit{{Name: "bootstrap", Dir: root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Export(layout, plan, destination, "v1.11.0"); err != nil {
		t.Fatal(err)
	}

	for _, gone := range []string{
		".terraform/modules/cached.tf", "terraform.tfstate", "dev.tfplan",
		"envs/dev.tfvars-example",
	} {
		path := filepath.Join(destination, "bootstrap", gone)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s was exported", gone)
		}
	}
	// And the answer did travel: it is what this repository exists to hold.
	if _, err := os.Stat(filepath.Join(destination, "bootstrap", "envs", "dev.tfvars")); err != nil {
		t.Errorf("the configured tfvars did not travel: %v", err)
	}
}

// The templates ignore envs/*.tfvars, backend/*.hcl and environments.conf. That
// is right there and exactly backwards here, where those files are the content.
func TestTheGeneratedIgnoreKeepsTheConfiguration(t *testing.T) {
	destination := t.TempDir()
	if err := WriteExportMeta(destination, "v1.11.0", ExportPlan{Roots: []string{"examples/aws/bootstrap"}}); err != nil {
		t.Fatal(err)
	}

	ignore, err := os.ReadFile(filepath.Join(destination, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mustNotIgnore := range []string{"*.tfvars", "backend/*.hcl", "environments.conf"} {
		for _, line := range strings.Split(string(ignore), "\n") {
			if strings.TrimSpace(line) == mustNotIgnore {
				t.Errorf("the generated .gitignore still ignores %q, which is the content here", mustNotIgnore)
			}
		}
	}
	for _, mustIgnore := range []string{"*.tfstate", "**/.terraform/*"} {
		if !strings.Contains(string(ignore), mustIgnore) {
			t.Errorf("the generated .gitignore does not ignore %q", mustIgnore)
		}
	}
}

// Six months from now, "what changed in the templates since" has no answer
// unless the tag is written down.
func TestTheReadmeRecordsWhereTheCopyCameFrom(t *testing.T) {
	destination := t.TempDir()
	plan := ExportPlan{Roots: []string{"examples/aws/infra-base/vpc"}, Modules: []string{"examples/aws/_modules/naming"}}
	if err := WriteExportMeta(destination, "v1.11.0", plan); err != nil {
		t.Fatal(err)
	}

	readme, err := os.ReadFile(filepath.Join(destination, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"v1.11.0", "infra-base/vpc", "naming"} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("the README does not mention %q", want)
		}
	}
}

func exportCheckout(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"examples/aws/_modules", "examples/aws/backend"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func writeRoot(t *testing.T, layout Layout, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(layout.AWSDir(), filepath.FromSlash(name), "main.tf"), body)
}

func writeModule(t *testing.T, layout Layout, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(layout.AWSDir(), "_modules", name, "main.tf"), body)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func baseNamesOf(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, filepath.Base(path))
	}
	return out
}

// `examples/aws/` is the templates' name and the wrong one in a repository that
// is somebody's actual estate — the first thing a client reads in their own
// infrastructure should not be a word saying it is a demonstration.
//
// The test that matters is not where the files land but whether they still work
// where they landed: every module source is relative, and this walks each one
// from its new home to prove it resolves.
func TestTheExportDropsTheTemplatesDirectory(t *testing.T) {
	layout := exportCheckout(t)
	writeRoot(t, layout, "infra-base/vpc", `module "naming" { source = "../../_modules/naming" }`)
	writeRoot(t, layout, "bootstrap", `module "naming" { source = "../_modules/naming" }`)
	writeModule(t, layout, "naming", `module "tags" { source = "../tagging" }`)
	writeModule(t, layout, "tagging", "")
	writeFile(t, layout.ConfigFile(), "dev=111111111111\n")

	plan, err := PlanExport(layout, []Unit{
		{Name: "vpc", Dir: filepath.Join(layout.AWSDir(), "infra-base", "vpc")},
		{Name: "bootstrap", Dir: filepath.Join(layout.AWSDir(), "bootstrap")},
	})
	if err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "estate")
	if _, err := Export(layout, plan, destination, "v1.11.0"); err != nil {
		t.Fatal(err)
	}

	// Nothing anywhere in the tree says "example".
	if err := filepath.WalkDir(destination, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(destination, path)
		if strings.Contains(rel, "example") {
			t.Errorf("the export still has %s in it", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// The roots are at the top, where the README says they are.
	for _, want := range []string{
		"infra-base/vpc/main.tf",
		"bootstrap/main.tf",
		"_modules/naming/main.tf",
		"_modules/tagging/main.tf",
		"environments.conf",
	} {
		if _, err := os.Stat(filepath.Join(destination, filepath.FromSlash(want))); err != nil {
			t.Errorf("%s is not in the export: %v", want, err)
		}
	}

	// And the whole point: every relative source still resolves from where the
	// file now sits. Removing the same leading directories from both ends of a
	// relative path leaves it pointing at the same place — this proves it rather
	// than asserting it.
	assertSourcesResolve(t, destination)
}

// assertSourcesResolve walks the exported tree and follows every local module
// source from the directory it was found in.
func assertSourcesResolve(t *testing.T, destination string) {
	t.Helper()
	checked := 0

	err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".tf") {
			return err
		}
		content, readErr := os.ReadFile(path) // #nosec G304 -- a path this test wrote
		if readErr != nil {
			return readErr
		}
		for _, match := range localSource.FindAllStringSubmatch(string(content), -1) {
			target := filepath.Clean(filepath.Join(filepath.Dir(path), match[1]))
			if _, statErr := os.Stat(target); statErr != nil {
				rel, _ := filepath.Rel(destination, path)
				t.Errorf("%s points at %s, which the export does not have", rel, match[1])
			}
			checked++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no module source was checked — the fixture proves nothing")
	}
}

// Not stripped when anything sits outside it: a module reached from above that
// directory would have its relative source broken by the move, and a repository
// that reads nicely and does not terraform init is worse than one with an
// awkward directory name.
func TestTheTemplatesDirectoryStaysWhenSomethingIsOutsideIt(t *testing.T) {
	layout := exportCheckout(t)
	writeRoot(t, layout, "infra-base/vpc", `module "shared" { source = "../../../../shared/naming" }`)
	writeFile(t, filepath.Join(layout.Root, "shared", "naming", "main.tf"), "")

	plan, err := PlanExport(layout, []Unit{
		{Name: "vpc", Dir: filepath.Join(layout.AWSDir(), "infra-base", "vpc")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Base != "" {
		t.Errorf("stripped %q with a module outside it — every source under it would break", plan.Base)
	}

	destination := filepath.Join(t.TempDir(), "estate")
	if _, err := Export(layout, plan, destination, "v1.11.0"); err != nil {
		t.Fatal(err)
	}
	assertSourcesResolve(t, destination)
}

// The README has to describe the layout that was written, not the one the
// templates have: a client following it would cd into a directory that is not
// there.
func TestTheReadmeDescribesWhereThingsLanded(t *testing.T) {
	layout := exportCheckout(t)
	writeRoot(t, layout, "infra-base/vpc", `module "naming" { source = "../../_modules/naming" }`)
	writeModule(t, layout, "naming", "")
	writeFile(t, layout.ConfigFile(), "dev=111111111111\n")

	plan, err := PlanExport(layout, []Unit{
		{Name: "vpc", Dir: filepath.Join(layout.AWSDir(), "infra-base", "vpc")},
	})
	if err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "estate")
	if _, err := Export(layout, plan, destination, "v1.11.0"); err != nil {
		t.Fatal(err)
	}
	if err := WriteExportMeta(destination, "v1.11.0", plan); err != nil {
		t.Fatal(err)
	}

	readme, err := os.ReadFile(filepath.Join(destination, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	// The directory is named once, to say where this came from. What must not
	// survive is any instruction to go there — a client following those lands in
	// a directory the export does not have.
	for _, wrong := range []string{"cd examples", "- `examples", "`examples/aws/environments.conf`",
		"`examples/aws/backend/"} {
		if strings.Contains(string(readme), wrong) {
			t.Errorf("the README still sends the reader to %q:\n%s", wrong, readme)
		}
	}
	if !strings.Contains(string(readme), "- `infra-base/vpc`") {
		t.Errorf("the roots are not listed where they landed:\n%s", readme)
	}
	if !strings.Contains(string(readme), "cd infra-base/vpc") {
		t.Errorf("the worked example does not match the layout:\n%s", readme)
	}
}

// Six months on, "where did this root come from, and can I pull a newer one" is
// asked with a .tf file on the screen. The README is read once; these are read
// every time somebody opens the estate.
func TestEveryTerraformFileSaysWhereItCameFrom(t *testing.T) {
	previous := generatorVersion
	generatorVersion = func() string { return "v9.9.9" }
	t.Cleanup(func() { generatorVersion = previous })

	layout := exportCheckout(t)
	writeRoot(t, layout, "bootstrap", `output "x" { value = 1 }`)
	writeFile(t, filepath.Join(layout.AWSDir(), "bootstrap", "envs", "dev.tfvars"), "size = 1\n")
	writeFile(t, layout.BackendFile("dev"), "bucket = \"b\"\n")
	writeFile(t, layout.ConfigFile(), "dev=111111111111\n")
	// Not Terraform's, and not this tool's to rewrite.
	writeFile(t, filepath.Join(layout.AWSDir(), "bootstrap", "notes.md"), "# mine\n")

	plan, err := PlanExport(layout, []Unit{
		{Name: "bootstrap", Dir: filepath.Join(layout.AWSDir(), "bootstrap")},
	})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "estate")
	if _, err := Export(layout, plan, destination, "v1.11.0"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"bootstrap/main.tf", "bootstrap/envs/dev.tfvars", "backend/dev.hcl", "environments.conf",
	} {
		body := readExported(t, destination, name)
		if !strings.HasPrefix(body, "# Generated by lerian-cli v9.9.9 (Lerian Studio)") {
			t.Errorf("%s does not say what generated it:\n%s", name, firstLine(body))
		}
		if !strings.Contains(body, "lerian-terraform-foundation v1.11.0") {
			t.Errorf("%s does not say which templates it came from:\n%s", name, firstLine(body))
		}
	}

	// A copy that rewrote arbitrary files would be a copy nobody can trust.
	if body := readExported(t, destination, "bootstrap/notes.md"); body != "# mine\n" {
		t.Errorf("a file that is not Terraform's was rewritten:\n%s", body)
	}

	// And the content survived the banner — the whole file, not just its first
	// line.
	if body := readExported(t, destination, "bootstrap/main.tf"); !strings.Contains(body, `output "x"`) {
		t.Errorf("the banner replaced the content:\n%s", body)
	}
}

func readExported(t *testing.T, destination, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("%s is not in the export: %v", name, err)
	}
	return string(body)
}

func firstLine(body string) string {
	if cut := strings.IndexByte(body, '\n'); cut >= 0 {
		return body[:cut]
	}
	return body
}

// The commit is a generated tree, not the work of whoever ran the export.
// Attributing it to them makes `git log` read as though they wrote four thousand
// lines of Terraform; everything after it is theirs.
func TestTheFirstCommitIsAttributedToLerianStudio(t *testing.T) {
	previous := generatorVersion
	generatorVersion = func() string { return "v9.9.9" }
	t.Cleanup(func() { generatorVersion = previous })

	destination := t.TempDir()
	writeFile(t, filepath.Join(destination, "main.tf"), "")

	git, err := NewGitCLI()
	if err != nil {
		t.Skipf("no git here: %v", err)
	}
	if err := InitRepository(context.Background(), git, destination, "v1.11.0"); err != nil {
		t.Fatal(err)
	}

	author, err := git.runRaw(context.Background(), destination, "log", "-1", "--format=%an <%ae>")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(author) != "lerian-studio <noreply@lerian.studio>" {
		t.Errorf("the commit is authored by %q", strings.TrimSpace(author))
	}

	body, err := git.runRaw(context.Background(), destination, "log", "-1", "--format=%B")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lerian-cli v9.9.9", "lerian-terraform-foundation v1.11.0"} {
		if !strings.Contains(body, want) {
			t.Errorf("the commit message does not mention %q:\n%s", want, body)
		}
	}
}
