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

// The errors are where a Makefile either helps or sends somebody to read it.
func TestTheMakefileSaysWhatIsWrong(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	dir := makefileTree(t)

	run := func(args ...string) string {
		t.Helper()
		command := exec.Command("make", args...)
		command.Dir = dir
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("make %v was expected to fail:\n%s", args, output)
		}
		return string(output)
	}

	// `make plan infra-base dev` is the group form and runs both; what must not
	// happen is the answer it used to give — "infra-base has no envs/dev.tfvars",
	// which describes a directory holding two working roots as one somebody
	// forgot to configure. The single-root targets still refuse it, and say why.
	t.Run("a directory is never called a broken root", func(t *testing.T) {
		out := run("one-plan", "ROOT=infra-base", "ENV=dev")

		if strings.Contains(out, "has no envs/") {
			t.Errorf("it called a parent directory a broken root:\n%s", out)
		}
		if !strings.Contains(out, "holds 2 roots") {
			t.Errorf("it does not say what infra-base is:\n%s", out)
		}
	})

	t.Run("a mistyped root lists the short names too", func(t *testing.T) {
		out := run("plan", "boostrap", "dev")

		if !strings.Contains(out, "no such root: boostrap") {
			t.Errorf("it did not name what it could not find:\n%s", out)
		}
		// Somebody who mistyped a short name is looking for the list of those, not
		// only for the paths. `make roots` prints both, each path with its short
		// name beside it, and the error calls it rather than printing its own
		// half-version.
		if !strings.Contains(out, "(eks)") || !strings.Contains(out, "(bootstrap)") {
			t.Errorf("it does not list the short names:\n%s", out)
		}
		if !strings.Contains(out, "groups") {
			t.Errorf("it does not mention the group form:\n%s", out)
		}
	})

	t.Run("a missing environment says which ones exist", func(t *testing.T) {
		out := run("plan", "eks", "producao")

		if !strings.Contains(out, "has no envs/producao.tfvars") {
			t.Errorf("it did not say what is missing:\n%s", out)
		}
		if !strings.Contains(out, "it has: dev stg") {
			t.Errorf("it does not list the environments that exist:\n%s", out)
		}
	})
}

// A word that is itself a verb must not get a do-nothing rule: `make plan eks
// dev plan` would redefine plan, and make warns about overriding commands —
// which reads as a broken Makefile rather than as a stray word.
func TestARepeatedVerbDoesNotRedefineTheTarget(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	dir := makefileTree(t)

	command := exec.Command("make", "-n", "plan", "eks", "dev", "plan")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("make failed: %v\n%s", err, output)
	}

	if strings.Contains(string(output), "overriding commands") {
		t.Errorf("a repeated verb redefined the target:\n%s", output)
	}
	if strings.Contains(string(output), "ignoring old commands") {
		t.Errorf("make ignored the real rule:\n%s", output)
	}
	// And it still did the work.
	if !strings.Contains(string(output), "-chdir=infra-base/eks") {
		t.Errorf("the plan did not run:\n%s", output)
	}
}

// A directory holding roots runs all of them. The order is the part discovery
// cannot work out — nothing in a directory says the network comes before the
// cluster in it — so it is written down and followed.
func TestAGroupRunsEveryRootUnderItInOrder(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	dir := makefileTree(t)
	// A terraform that reports which root it was run in, and does nothing.
	stub := filepath.Join(t.TempDir(), "terraform")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nfor a in \"$@\"; do case $a in -chdir=*) echo \"RAN ${a#-chdir=}\";; esac; done\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	ran := func(args ...string) []string {
		t.Helper()
		// On the command line, not in the environment: a make variable set there
		// wins over anything the host exports, which is what keeps this test from
		// depending on the shell it runs in — and from reaching a real terraform
		// if somebody happens to have the same variable set.
		command := exec.Command("make", append(args, "TERRAFORM="+stub, "GROUP_CONFIRMED=1")...)
		command.Dir = dir
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("make %v failed: %v\n%s", args, err, output)
		}

		var order []string
		for _, line := range strings.Split(string(output), "\n") {
			if root, found := strings.CutPrefix(strings.TrimSpace(line), "RAN "); found {
				// init runs first in each root; only the first mention matters.
				if len(order) == 0 || order[len(order)-1] != root {
					order = append(order, root)
				}
			}
		}
		return order
	}

	t.Run("the network before the cluster in it", func(t *testing.T) {
		order := ran("plan", "infra-base", "dev")

		want := []string{"infra-base/vpc", "infra-base/eks"}
		if strings.Join(order, " ") != strings.Join(want, " ") {
			t.Errorf("ran %v, want %v", order, want)
		}
	})

	t.Run("destroy goes the other way", func(t *testing.T) {
		order := ran("destroy", "infra-base", "dev")

		want := []string{"infra-base/eks", "infra-base/vpc"}
		if strings.Join(order, " ") != strings.Join(want, " ") {
			t.Errorf("ran %v, want %v — a cluster cannot outlive its network", order, want)
		}
	})

	t.Run("all is every root there is", func(t *testing.T) {
		order := ran("plan", "all", "dev")

		want := []string{"bootstrap", "infra-base/vpc", "infra-base/eks"}
		if strings.Join(order, " ") != strings.Join(want, " ") {
			t.Errorf("ran %v, want %v", order, want)
		}
	})

	t.Run("a new service joins its group without being named", func(t *testing.T) {
		addRoot(t, dir, "infra-base/rds")

		order := ran("plan", "infra-base", "dev")

		if len(order) != 3 {
			t.Fatalf("ran %v, want three roots", order)
		}
		// The two that are in ORDER keep their order; what is not runs after.
		want := []string{"infra-base/vpc", "infra-base/eks", "infra-base/rds"}
		if strings.Join(order, " ") != strings.Join(want, " ") {
			t.Errorf("ran %v, want %v", order, want)
		}
	})
}

// A group that writes confirms once, for all of it. Asking per root turns one
// decision into three, and three prompts in a row is a thing people answer
// without reading.
func TestAGroupConfirmsOnceAndListsWhatItWouldDo(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	dir := makefileTree(t)

	// GROUP_CONFIRMED is cleared explicitly, so a value in the environment cannot
	// turn this test — which answers "no" — into one that runs terraform destroy
	// for real.
	command := exec.Command("make", "destroy", "infra-base", "dev", "GROUP_CONFIRMED=", "TERRAFORM=false")
	command.Dir = dir
	command.Stdin = strings.NewReader("no\n")
	output, err := command.CombinedOutput()

	if err == nil {
		t.Fatalf("declining went ahead anyway:\n%s", output)
	}
	out := string(output)
	if strings.Count(out, "Type yes to continue") != 1 {
		t.Errorf("it asked %d times, want once:\n%s", strings.Count(out, "Type yes to continue"), out)
	}
	// And it says what it would take down, in the order it would do it.
	eks := strings.Index(out, "infra-base/eks")
	vpc := strings.Index(out, "infra-base/vpc")
	if eks < 0 || vpc < 0 || eks > vpc {
		t.Errorf("it does not list what it would destroy, cluster first:\n%s", out)
	}
}

// "What can I run this against" is the first question somebody has, and reading
// it out of the error of a command they had to guess at is not an answer.
func TestRootsListsWhatCanBeRun(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	dir := makefileTree(t)

	out := dryRunReal(t, dir, "roots")

	for _, want := range []string{"bootstrap", "infra-base/vpc", "infra-base/eks"} {
		if !strings.Contains(out, want) {
			t.Errorf("it does not list %s:\n%s", want, out)
		}
	}
	// The short names are what gets typed, so they are beside the paths.
	if !strings.Contains(out, "(eks)") || !strings.Contains(out, "(vpc)") {
		t.Errorf("the short names are missing:\n%s", out)
	}
	// And the groups, which is the part nothing else announces.
	if !strings.Contains(out, "infra-base") || !strings.Contains(out, "all") {
		t.Errorf("the groups are missing:\n%s", out)
	}
	// "." is the dir of a top-level root and is not something anybody would type.
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "." {
			t.Errorf("it offers '.' as a group:\n%s", out)
		}
	}
}

// dryRunReal runs make for real, for targets that only print.
func dryRunReal(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("make", args...)
	command.Dir = dir

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("make %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

// When something sat outside the templates' directory the prefix stays, and
// everything built on "the roots are at the top level" is wrong: the backend
// files are under it too. Every non-bootstrap target stopped with "no
// backend/dev.hcl", and no test saw it because they all used Base: "examples/aws".
func TestTheKeptLayoutStillFindsItsBackend(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}

	plan := ExportPlan{
		Roots: []ExportedRoot{
			{Path: "examples/aws/infra-base/vpc", StateKey: "aws/infra-base/vpc/terraform.tfstate"},
		},
		Config: []string{"examples/aws/environments.conf", "examples/aws/backend/dev.hcl"},
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(WriteMakefile(plan)), 0o600); err != nil {
		t.Fatal(err)
	}
	addRoot(t, dir, "examples/aws/infra-base/vpc")
	if err := os.MkdirAll(filepath.Join(dir, "examples/aws/backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "examples/aws/backend/dev.hcl"),
		[]byte("bucket = \"b\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := dryRun(t, dir, "plan", "vpc", "dev")

	if !strings.Contains(out, "examples/aws/backend/dev.hcl") {
		t.Errorf("it looks for the backend at the top level, where it is not:\n%s", out)
	}
	// And the state key still comes off the templates' prefix.
	if !strings.Contains(out, "key=aws/infra-base/vpc/terraform.tfstate") {
		t.Errorf("the state key is wrong for the kept layout:\n%s", out)
	}
}

// A directory that is itself a root — an envs/ of its own, with more roots under
// it — is a root, not a group. `make plan infra-base dev` has to mean the one it
// names rather than being refused.
func TestARootThatAlsoHoldsRootsIsStillARoot(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	dir := makefileTree(t)
	addRoot(t, dir, "infra-base") // now both infra-base and infra-base/vpc are roots

	out := dryRun(t, dir, "plan", "infra-base", "dev")

	if !strings.Contains(out, "-chdir=infra-base ") && !strings.Contains(out, "-chdir=infra-base\n") {
		t.Errorf("it did not run the root it was given:\n%s", out)
	}
	if strings.Contains(out, "-chdir=infra-base/vpc") {
		t.Errorf("it fanned out over a directory that is itself a root:\n%s", out)
	}
}

// terraform test needs the providers and modules installed; on a fresh export
// they are not. -backend=false because the tests run against the configuration,
// not against the deployed state.
func TestTestInitializesTheRootFirst(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	dir := makefileTree(t)

	out := dryRun(t, dir, "test", "eks")

	init := strings.Index(out, "init -backend=false")
	test := strings.Index(out, "-chdir=infra-base/eks test")
	if init < 0 {
		t.Errorf("it runs the tests without installing what they need:\n%s", out)
	}
	if test < 0 || init > test {
		t.Errorf("the init does not come first:\n%s", out)
	}
}
