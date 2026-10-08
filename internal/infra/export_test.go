package infra

import (
	"context"
	"errors"
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
	if err := WriteExportMeta(destination, "v1.11.0", ExportPlan{Roots: []ExportedRoot{{Path: "examples/aws/bootstrap", Bootstrap: true}}}); err != nil {
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
	plan := ExportPlan{
		Roots:   []ExportedRoot{{Path: "examples/aws/infra-base/vpc", StateKey: "aws/infra-base/vpc/terraform.tfstate"}},
		Modules: []string{"examples/aws/_modules/naming"},
	}
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
	if !strings.Contains(string(readme), "**`infra-base/vpc`**") {
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

// The state key cannot be guessed, and a wrong one does not fail: Terraform
// initializes an empty state and plans to create an estate that already exists.
// So the README's key has to be the key this tool itself uses, for every root,
// derived from the same place.
func TestTheReadmeGivesEachRootItsOwnStateKey(t *testing.T) {
	layout := exportCheckout(t)
	writeRoot(t, layout, "infra-base/vpc", "")
	writeRoot(t, layout, "products/midaz/postgres", "")

	units := []Unit{
		{Name: "infra-base/vpc", Dir: filepath.Join(layout.AWSDir(), "infra-base", "vpc")},
		{Name: "products/midaz/postgres",
			Dir: filepath.Join(layout.AWSDir(), "products", "midaz", "postgres")},
	}
	plan, err := PlanExport(layout, units)
	if err != nil {
		t.Fatal(err)
	}

	readme := exportReadme("v1.11.0", plan)

	for _, unit := range units {
		// The authority is Unit.StateKey — the same call the runner makes when it
		// initializes that root. Hard-coding the string here would let both drift
		// together and prove nothing.
		want := `-backend-config="key=` + unit.StateKey() + `"`
		if !strings.Contains(readme, want) {
			t.Errorf("the README does not give %s its own key (%s):\n%s", unit.Name, want, readme)
		}
	}
	// And each root gets its own cd, so nothing is left to be adapted by hand.
	for _, path := range []string{"cd infra-base/vpc", "cd products/midaz/postgres"} {
		if !strings.Contains(readme, path) {
			t.Errorf("the README does not say %q:\n%s", path, readme)
		}
	}
}

// The bootstrap keeps its state locally in a workspace per environment, because
// it is the stack that creates the bucket the others use. Handing it a
// -backend-config points it at a bucket it has not made yet.
func TestTheReadmeDoesNotSendTheBootstrapAtABackend(t *testing.T) {
	layout := exportCheckout(t)
	writeRoot(t, layout, "bootstrap", "")

	plan, err := PlanExport(layout, []Unit{
		{Name: "bootstrap", Dir: layout.BootstrapDir(), Bootstrap: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	readme := exportReadme("v1.11.0", plan)

	if strings.Contains(readme, "-backend-config") {
		t.Errorf("the bootstrap was given a backend it has not created yet:\n%s", readme)
	}
	if !strings.Contains(readme, "terraform workspace select dev") {
		t.Errorf("the bootstrap's workspace step is missing:\n%s", readme)
	}
}

// Who owns this afterwards is the question somebody opening it months later
// actually has, and the answer is not "the tool that wrote it".
func TestTheReadmeSaysItIsABootstrapAndNotMaintainedHere(t *testing.T) {
	readme := exportReadme("v1.11.0", ExportPlan{
		Roots: []ExportedRoot{{Path: "examples/aws/bootstrap", Bootstrap: true}},
	})

	for _, want := range []string{
		"Infrastructure for Lerian applications",
		"not maintained by lerian-cli",
		"bootstrap",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("the README does not say %q:\n%s", want, readme)
		}
	}
}

// The bootstrap's state is local and does not travel: state never belongs in a
// repository. A plan from the copy therefore starts from nothing and proposes to
// create a backend that exists — verified against a real account, where it
// offered to create nine resources that were already there. Somebody who applies
// that is applying to an estate the plan cannot see.
func TestTheReadmeWarnsThatTheBootstrapStateStayedBehind(t *testing.T) {
	readme := exportReadme("v1.11.0", ExportPlan{
		Roots: []ExportedRoot{{Path: "examples/aws/bootstrap", Bootstrap: true}},
	})

	for _, want := range []string{
		"did not travel with this repository",
		"terraform.tfstate.d",
		"Do not apply it against an environment that already has a backend",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("the README does not warn about %q:\n%s", want, readme)
		}
	}
}

// And the general case of the same confusion: an empty state reads as a missing
// estate, and the difference is one `aws s3 ls` away.
func TestTheReadmeSaysHowToTellAnEmptyStateFromAMissingEstate(t *testing.T) {
	readme := exportReadme("v1.11.0", ExportPlan{
		Roots: []ExportedRoot{{
			Path:     "examples/aws/infra-base/vpc",
			StateKey: "aws/infra-base/vpc/terraform.tfstate",
		}},
	})

	if !strings.Contains(readme, "A plan proposes to create everything") {
		t.Errorf("the README does not cover the case:\n%s", readme)
	}
	for _, want := range []string{"aws sts get-caller-identity", "aws s3 ls"} {
		if !strings.Contains(readme, want) {
			t.Errorf("the README does not say to run %q:\n%s", want, readme)
		}
	}
}

// Planning a later root against an environment where an earlier one was never
// applied fails while reading its data sources — verified: the eks root failed
// with "no matching EC2 VPC found" against an account whose vpc had been
// destroyed. Without a line saying so, that reads as a broken configuration.
func TestTheReadmeExplainsTheOrderBetweenRoots(t *testing.T) {
	plan := ExportPlan{Roots: []ExportedRoot{
		{Path: "examples/aws/infra-base/vpc", StateKey: "aws/infra-base/vpc/terraform.tfstate"},
		{Path: "examples/aws/infra-base/eks", StateKey: "aws/infra-base/eks/terraform.tfstate"},
	}}

	readme := exportReadme("v1.11.0", plan)
	if !strings.Contains(readme, "no matching EC2 VPC found") {
		t.Errorf("the README does not name what that failure looks like:\n%s", readme)
	}

	// One root has no order to explain.
	alone := exportReadme("v1.11.0", ExportPlan{Roots: plan.Roots[:1]})
	if strings.Contains(alone, "no matching EC2 VPC found") {
		t.Errorf("it explains an order between one root:\n%s", alone)
	}
}

// A module source can resolve outside the checkout. filepath.Rel answers that
// with a path starting in "..", and joining it to the destination writes the
// module into a sibling of the export — outside the directory somebody named.
func TestAModuleOutsideTheCheckoutIsRefused(t *testing.T) {
	layout := exportCheckout(t)
	// Five levels up from examples/aws/infra-base/vpc leaves the checkout.
	writeRoot(t, layout, "infra-base/vpc", `module "x" { source = "../../../../../elsewhere" }`)
	writeFile(t, filepath.Join(filepath.Dir(layout.Root), "elsewhere", "main.tf"), "")

	_, err := PlanExport(layout, []Unit{
		{Name: "vpc", Dir: filepath.Join(layout.AWSDir(), "infra-base", "vpc")},
	})

	if err == nil {
		t.Fatal("a module outside the checkout was accepted")
	}
	if !strings.Contains(err.Error(), "outside the checkout") {
		t.Errorf("error = %v", err)
	}
}

// With nothing stripped, the README must not say the roots are at the top level
// or name configuration files that are not there: a reader following it lands in
// directories this repository does not have.
func TestTheReadmeDoesNotClaimALayoutItDidNotProduce(t *testing.T) {
	plan := ExportPlan{
		// Base empty: something sat outside examples/aws, so the prefix stayed.
		Roots: []ExportedRoot{{
			Path:     "examples/aws/infra-base/vpc",
			StateKey: "aws/infra-base/vpc/terraform.tfstate",
		}},
		Config: []string{"examples/aws/environments.conf", "examples/aws/backend/dev.hcl"},
	}

	readme := exportReadme("v1.11.0", plan)

	if strings.Contains(readme, "roots sit at the top level") {
		t.Errorf("it claims a layout it did not produce:\n%s", readme)
	}
	if strings.Contains(readme, "- `environments.conf`") {
		t.Errorf("it names a path that is not there:\n%s", readme)
	}
	if !strings.Contains(readme, "examples/aws/environments.conf") {
		t.Errorf("it does not name the path that is:\n%s", readme)
	}
	// And the worked example follows the same layout.
	if !strings.Contains(readme, "cd examples/aws/infra-base/vpc") {
		t.Errorf("the worked example points somewhere else:\n%s", readme)
	}
}

// gh stopping before it wires origin is a different failure from gh failing to
// push: "git push -u origin main" there fails with "'origin' does not appear to
// be a git repository", which reads as a second, unrelated problem.
func TestAddingTheRemoteFailingGetsItsOwnAdvice(t *testing.T) {
	gh := fakeGH(t, `echo "unable to add remote: exit status 128" >&2; exit 1`)

	_, err := gh.CreateRepository(context.Background(), t.TempDir(), GHRepo{Name: "estate"})

	var failure *CreateFailure
	if !errors.As(err, &failure) {
		t.Fatalf("the failure is not readable: %v", err)
	}
	if !failure.Created {
		t.Error("the repository exists and the advice does not say so")
	}
	if !strings.Contains(err.Error(), "git remote add origin") {
		t.Errorf("it does not say to add the remote:\n%v", err)
	}
}

// The page follows the lerian-cli README's shape — badges, what is here, a
// quick start, then usage — because a client who has seen one Lerian repository
// should not have to learn a second layout to find the same facts.
func TestTheReadmeFollowsTheHouseLayout(t *testing.T) {
	plan := ExportPlan{
		Base: "examples/aws",
		Roots: []ExportedRoot{
			{Path: "examples/aws/bootstrap", StateKey: "aws/bootstrap/terraform.tfstate", Bootstrap: true},
			{Path: "examples/aws/infra-base/vpc", StateKey: "aws/infra-base/vpc/terraform.tfstate"},
			{Path: "examples/aws/infra-base/eks", StateKey: "aws/infra-base/eks/terraform.tfstate"},
		},
		Modules: []string{"examples/aws/_modules/naming"},
	}

	readme := exportReadme("v1.11.0", plan)

	// The sections, in order. A page whose headings arrive in a different order
	// from the CLI's reads as a different kind of document.
	sections := []string{
		"# Infrastructure for Lerian applications",
		"## This is a bootstrap, and it is now yours",
		"## What is here",
		"## Core Features",
		"## Quick Start",
		"## Usage",
		"## Adding a service",
		"## Troubleshooting",
	}
	at := 0
	for _, section := range sections {
		found := strings.Index(readme[at:], section)
		if found < 0 {
			t.Fatalf("%q is missing, or out of order:\n%s", section, readme)
		}
		at += found
	}

	// Badges say what this is at a glance, and the scope one is the whole point.
	for _, badge := range []string{
		"img.shields.io/badge/generated%20by-lerian--cli",
		"img.shields.io/badge/templates-v1.11.0",
		"img.shields.io/badge/scope-bootstrap%20only",
	} {
		if !strings.Contains(readme, badge) {
			t.Errorf("the %q badge is missing:\n%s", badge, readme)
		}
	}

	// The scope badge links to the section that explains it, and the anchor has
	// to match the heading GitHub generates.
	if !strings.Contains(readme, "(#this-is-a-bootstrap-and-it-is-now-yours)") {
		t.Errorf("the scope badge does not link to its section:\n%s", readme)
	}
}

// Each root is named with what it is for, not with a count of its resources.
func TestTheReadmeSaysWhatEachRootIsFor(t *testing.T) {
	plan := ExportPlan{Base: "examples/aws", Roots: []ExportedRoot{
		{Path: "examples/aws/bootstrap", StateKey: "aws/bootstrap/terraform.tfstate", Bootstrap: true},
		{Path: "examples/aws/infra-base/vpc", StateKey: "aws/infra-base/vpc/terraform.tfstate"},
		{Path: "examples/aws/products/midaz/postgres",
			StateKey: "aws/products/midaz/postgres/terraform.tfstate"},
	}}

	readme := exportReadme("v1.11.0", plan)

	if !strings.Contains(readme, "**`bootstrap`** — the state bucket and lock table") {
		t.Errorf("the bootstrap is not described:\n%s", readme)
	}
	if !strings.Contains(readme, "**`infra-base/vpc`** — the network") {
		t.Errorf("the vpc is not described:\n%s", readme)
	}
	// One the templates do not ship a description for falls back to its state key
	// rather than to an invented sentence. Checked on the bullet itself: the key
	// appears again under "Running Terraform directly", and asserting on it
	// anywhere in the page passed with the fallback replaced by "a root".
	if !strings.Contains(readme,
		"**`products/midaz/postgres`** — `aws/products/midaz/postgres/terraform.tfstate`") {
		t.Errorf("an unknown root got no honest description:\n%s", readme)
	}
}

// The quick start is three commands, and the first is the one that catches the
// mistake the rest of the page is about: being in the wrong account.
func TestTheQuickStartChecksTheAccountFirst(t *testing.T) {
	readme := exportReadme("v1.11.0", ExportPlan{Base: "examples/aws", Roots: []ExportedRoot{
		{Path: "examples/aws/infra-base/vpc", StateKey: "aws/infra-base/vpc/terraform.tfstate"},
	}})

	start := strings.Index(readme, "## Quick Start")
	usage := strings.Index(readme, "## Usage")
	if start < 0 || usage < 0 {
		t.Fatal("the quick start is missing")
	}
	section := readme[start:usage]

	identity := strings.Index(section, "aws sts get-caller-identity")
	plan := strings.Index(section, "make plan")
	if identity < 0 || plan < 0 {
		t.Fatalf("the quick start does not get as far as a plan:\n%s", section)
	}
	if identity > plan {
		t.Errorf("it plans before checking which account it is in:\n%s", section)
	}
}
