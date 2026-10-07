package infra

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Running one root by hand is five arguments, two of which cannot be guessed:
// the backend file for the environment and the state key for that root. The
// Makefile is generated where both are known, so this checks it carries them.
// A generated list of roots is accurate on the day of the export and wrong the
// first time somebody adds a service — and adding services is the point of
// handing this repository over. The shape is the contract instead.
func TestANewServiceNeedsNoChangeToTheMakefile(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}

	dir := makefileTree(t)

	// A service that did not exist when the Makefile was written, in the same
	// shape as the others.
	addRoot(t, dir, "products/midaz/postgres")

	out := dryRun(t, dir, "plan", "products/midaz/postgres", "dev")

	if !strings.Contains(out, "-chdir=products/midaz/postgres") {
		t.Errorf("the new root was not found:\n%s", out)
	}
	// Its key is its path — derived, not listed, which is why it works at all.
	if !strings.Contains(out, "key=aws/products/midaz/postgres/terraform.tfstate") {
		t.Errorf("the new root's state key was not derived:\n%s", out)
	}
	// And the short name works, because it is unambiguous.
	if out := dryRun(t, dir, "plan", "postgres", "dev"); !strings.Contains(out, "-chdir=products/midaz/postgres") {
		t.Errorf("the short name of a new root does not resolve:\n%s", out)
	}
	// Including its generated shortcut target.
	if out := dryRun(t, dir, "postgres-plan"); !strings.Contains(out, "-chdir=products/midaz/postgres") {
		t.Errorf("no shortcut was generated for the new root:\n%s", out)
	}
}

// A name two roots would answer to is left out rather than given to one of them:
// silently picking the first is how somebody plans the wrong estate.
func TestAnAmbiguousShortNameIsNotOffered(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}

	dir := makefileTree(t)
	addRoot(t, dir, "products/midaz/postgres")
	addRoot(t, dir, "products/ledger/postgres")

	command := exec.Command("make", "-n", "plan", "postgres", "dev")
	command.Dir = dir
	output, _ := command.CombinedOutput()

	// It resolves to nothing, so the guard refuses rather than picking one.
	if !strings.Contains(string(output), "no such root: postgres") {
		t.Errorf("an ambiguous short name resolved to something:\n%s", output)
	}
	// And no shortcut is generated for it. This is the part that tells a dropped
	// name from one that resolved to both: the guard refuses either way, but a
	// target that does not exist cannot be run at all.
	shortcut := exec.Command("make", "-n", "postgres-plan")
	shortcut.Dir = dir
	shortcutOutput, shortcutErr := shortcut.CombinedOutput()
	if shortcutErr == nil {
		t.Errorf("an ambiguous shortcut was generated:\n%s", shortcutOutput)
	}
	if !strings.Contains(string(shortcutOutput), "No rule to make target") {
		t.Errorf("it failed for some other reason:\n%s", shortcutOutput)
	}
	// Both are still reachable by their full paths.
	for _, path := range []string{"products/midaz/postgres", "products/ledger/postgres"} {
		if out := dryRun(t, dir, "plan", path, "dev"); !strings.Contains(out, "-chdir="+path) {
			t.Errorf("%s is not reachable:\n%s", path, out)
		}
	}
}

// The positional form is the whole point, and the catch-all rule it needs must
// not swallow typos: `make pln eks dev` has to say there is no such target.
func TestTheMakefileRunsTheRightCommands(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}

	dir := makefileTree(t)

	t.Run("a short name resolves and carries its own key", func(t *testing.T) {
		out := dryRun(t, dir, "plan", "eks", "dev")

		if !strings.Contains(out, "-chdir=infra-base/eks") {
			t.Errorf("it did not resolve the short name:\n%s", out)
		}
		if !strings.Contains(out, "key=aws/infra-base/eks/terraform.tfstate") {
			t.Errorf("it did not pass that root's key:\n%s", out)
		}
		if !strings.Contains(out, "backend/dev.hcl") {
			t.Errorf("it did not pass the environment's backend:\n%s", out)
		}
		if !strings.Contains(out, "-var-file=envs/dev.tfvars") {
			t.Errorf("it did not pass the environment's variables:\n%s", out)
		}
	})

	t.Run("the environment defaults to dev and the argument wins", func(t *testing.T) {
		if out := dryRun(t, dir, "plan", "eks"); !strings.Contains(out, "dev.tfvars") {
			t.Errorf("the default environment is not dev:\n%s", out)
		}
		out := dryRun(t, dir, "plan", "eks", "stg")
		if !strings.Contains(out, "stg.tfvars") || !strings.Contains(out, "backend/stg.hcl") {
			t.Errorf("the environment argument was ignored:\n%s", out)
		}
	})

	t.Run("the full path works too", func(t *testing.T) {
		if out := dryRun(t, dir, "plan", "infra-base/vpc", "dev"); !strings.Contains(out, "-chdir=infra-base/vpc") {
			t.Errorf("the full path did not resolve:\n%s", out)
		}
	})

	t.Run("a typo is still an error", func(t *testing.T) {
		command := exec.Command("make", "-n", "pln", "eks", "dev")
		command.Dir = dir
		output, err := command.CombinedOutput()

		if err == nil {
			t.Errorf("a typo ran silently:\n%s", output)
		}
		if !strings.Contains(string(output), "pln") {
			t.Errorf("it did not name the target it could not find:\n%s", output)
		}
	})

	t.Run("apply and destroy ask first, plan does not", func(t *testing.T) {
		// The prompt alone is not the confirmation — the read is. Asserting on the
		// printed question passed with the answer never being read.
		for _, verb := range []string{"apply", "destroy"} {
			out := dryRun(t, dir, verb, "eks", "dev")
			if !strings.Contains(out, "Type yes to continue") {
				t.Errorf("%s does not ask:\n%s", verb, out)
			}
			if !strings.Contains(out, "read -r answer") || !strings.Contains(out, `= yes ]`) {
				t.Errorf("%s asks and does not wait for the answer:\n%s", verb, out)
			}
		}
		if out := dryRun(t, dir, "plan", "eks", "dev"); strings.Contains(out, "Type yes to continue") {
			t.Errorf("plan confirms something that writes nothing:\n%s", out)
		}
	})

	t.Run("the bootstrap gets a workspace and no backend", func(t *testing.T) {
		// make -n prints the whole shell script, both branches of the if included,
		// so "it mentions workspace" is true even when the branch is never taken.
		// The condition is what decides, and it is what this reads.
		out := dryRun(t, dir, "plan", "bootstrap", "dev")
		if !strings.Contains(out, `if [ -n "bootstrap" ]`) {
			t.Errorf("the bootstrap is not recognized as local:\n%s", out)
		}

		// And a normal root takes the other branch.
		other := dryRun(t, dir, "plan", "eks", "dev")
		if !strings.Contains(other, `if [ -n "" ]`) {
			t.Errorf("a backend-using root was treated as local:\n%s", other)
		}
	})
}

// dryRun is `make -n`, which prints what would run without running it.
func dryRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("make", append([]string{"-n"}, args...)...)
	command.Dir = dir

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("make %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

// makefileTree writes a Makefile beside the directories it refers to, because
// its own guards check that the root and the tfvars exist.
func makefileTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	plan := makefilePlan()

	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(WriteMakefile(plan)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, root := range plan.Roots {
		addRoot(t, dir, plan.Target(root.Path))
	}
	// The backend files the non-local roots are initialized against.
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"dev", "stg"} {
		if err := os.WriteFile(filepath.Join(dir, "backend", env+".hcl"),
			[]byte("bucket = \"b\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// addRoot writes a root in the shape the Makefile looks for: a directory with
// an envs/ in it.
func addRoot(t *testing.T, dir, path string) {
	t.Helper()
	envs := filepath.Join(dir, filepath.FromSlash(path), "envs")
	if err := os.MkdirAll(envs, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"dev", "stg"} {
		if err := os.WriteFile(filepath.Join(envs, env+".tfvars"), []byte("x = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func makefilePlan() ExportPlan {
	return ExportPlan{Base: "examples/aws", Roots: []ExportedRoot{
		{Path: "examples/aws/bootstrap", StateKey: "aws/bootstrap/terraform.tfstate", Bootstrap: true},
		{Path: "examples/aws/infra-base/vpc", StateKey: "aws/infra-base/vpc/terraform.tfstate"},
		{Path: "examples/aws/infra-base/eks", StateKey: "aws/infra-base/eks/terraform.tfstate"},
	}}
}
