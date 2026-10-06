package infra

import (
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
	if _, err := Export(layout, plan, destination); err != nil {
		t.Fatal(err)
	}

	for _, gone := range []string{
		".terraform/modules/cached.tf", "terraform.tfstate", "dev.tfplan",
		"envs/dev.tfvars-example",
	} {
		path := filepath.Join(destination, "examples", "aws", "bootstrap", gone)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s was exported", gone)
		}
	}
	// And the answer did travel: it is what this repository exists to hold.
	if _, err := os.Stat(filepath.Join(destination,
		"examples", "aws", "bootstrap", "envs", "dev.tfvars")); err != nil {
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
