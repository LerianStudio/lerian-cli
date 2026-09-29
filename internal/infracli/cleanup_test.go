package infracli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Removing everything is not a default. Outside a terminal there is nobody to
// ask, so the command says what to name rather than deciding on its own.
func TestWithoutATerminalTheGroupsMustBeNamed(t *testing.T) {
	found := []leftover{{name: "plugins", size: 1024}}

	_, err := chooseLeftovers(found, false, false, false, false, &bytes.Buffer{})

	if err == nil {
		t.Fatal("chooseLeftovers decided on its own with no terminal")
	}
	for _, flag := range []string{"--plugins", "--all"} {
		if !strings.Contains(err.Error(), flag) {
			t.Errorf("the error does not name %s: %v", flag, err)
		}
	}
}

func TestNamedGroupsAreTheOnesChosen(t *testing.T) {
	found := []leftover{
		{name: "plugins"}, {name: "logs"}, {name: "remembered", configOnly: true},
	}

	chosen, err := chooseLeftovers(found, true, false, false, false, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("chooseLeftovers = %v", err)
	}

	if len(chosen) != 1 || chosen[0].name != "plugins" {
		t.Errorf("chose %v, want only plugins", names(chosen))
	}
}

func TestAllTakesEverythingFound(t *testing.T) {
	found := []leftover{{name: "plugins"}, {name: "logs"}}

	chosen, err := chooseLeftovers(found, false, false, false, true, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("chooseLeftovers = %v", err)
	}
	if len(chosen) != len(found) {
		t.Errorf("chose %v, want all of them", names(chosen))
	}
}

// The provider caches are what this command is for — hundreds of megabytes per
// stack, all of it restored by terraform init.
func TestProviderCachesAreFoundAndNotTheRepository(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		filepath.Join(root, "examples", "aws", "bootstrap", ".terraform"),
		filepath.Join(root, "examples", "aws", "infra-base", "vpc", ".terraform"),
		filepath.Join(root, ".git", "objects"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	caches := providerCaches(root)

	if len(caches) != 2 {
		t.Errorf("found %d caches, want 2: %v", len(caches), caches)
	}
	for _, path := range caches {
		if strings.Contains(path, ".git") {
			t.Errorf("the repository itself was offered for removal: %s", path)
		}
	}
}

// Forgetting the path clears the note and leaves the clone alone: the directory
// is the operator's, and a command about local caches has no business deleting a
// git repository.
func TestForgettingTheCheckoutLeavesTheCloneAlone(t *testing.T) {
	isolatedHome(t)
	checkout := fakeCheckout(t, "", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = checkout
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	if err := forgetCheckout(); err != nil {
		t.Fatalf("forgetCheckout = %v", err)
	}

	if got := rememberedCheckout(); got != "" {
		t.Errorf("the path is still remembered: %q", got)
	}
	if _, err := os.Stat(checkout); err != nil {
		t.Errorf("the checkout directory was removed: %v", err)
	}
}

// A dry run reports and removes nothing, which is what makes it safe to run
// first on a machine whose caches somebody else is using.
func TestADryRunRemovesNothing(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	// Created rather than named: the glob runLogDirs uses looks in the real temp
	// directory, and a fixed name is one another run — or another copy of this
	// test — may already own and be using.
	logDir, err := os.MkdirTemp("", "lerian-infra-dry-run-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(logDir) }()

	var stdout, stderr bytes.Buffer
	if err := runCleanup(context.Background(), []string{"--dry-run", "--logs"}, &stdout, &stderr); err != nil {
		t.Fatalf("runCleanup --dry-run = %v", err)
	}

	if _, err := os.Stat(logDir); err != nil {
		t.Errorf("a dry run removed %s", logDir)
	}
	if !strings.Contains(stdout.String(), "nothing was removed") {
		t.Errorf("a dry run did not say it removed nothing:\n%s", stdout.String())
	}
}

// Nothing here can reach AWS, and the report says so: an operator reading a
// cleanup command has to know whether their infrastructure is at stake.
func TestTheReportSaysAWSIsNotTouched(t *testing.T) {
	var out bytes.Buffer
	report(&out, []leftover{{name: "logs", size: 10}})

	if !strings.Contains(out.String(), "No AWS resource") {
		t.Errorf("the report does not say AWS is untouched:\n%s", out.String())
	}
}

// q leaves the typed prompt, the same way it leaves the selector. A prompt that
// does not say how to decline is one the operator escapes with ctrl-c, which
// stops the command mid-step instead.
func TestQLeavesTheTypedPrompt(t *testing.T) {
	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader("q\n")),
		out:         &bytes.Buffer{},
	}

	_, err := ask.ask("Where is it?", "", "/some/default", "--repo")

	if !errors.Is(err, infra.ErrAborted) {
		t.Errorf("answering q returned %v, want ErrAborted", err)
	}
}

func TestTheTypedPromptSaysHowToLeave(t *testing.T) {
	out := &bytes.Buffer{}
	ask := &prompter{interactive: true, in: bufio.NewReader(strings.NewReader("\n")), out: out}

	_, _ = ask.ask("Where is it?", "", "/some/default", "--repo")

	if !strings.Contains(out.String(), "q cancel") {
		t.Errorf("the prompt does not say q cancels:\n%s", out.String())
	}
}

func names(items []leftover) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.name)
	}
	return out
}

// A dry run of a named group reports that group. Listing everything found while
// the flags say --plugins describes a removal that is not the one about to
// happen, which is the one question a dry run exists to answer.
func TestADryRunReportsOnlyWhatWasNamed(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	logDir, err := os.MkdirTemp("", "lerian-infra-named-dry-run")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(logDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = fakeCheckout(t, "", "")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runCleanup(context.Background(), []string{"--dry-run", "--logs"}, &stdout, &stderr); err != nil {
		t.Fatalf("runCleanup = %v", err)
	}

	if !strings.Contains(stdout.String(), "logs") {
		t.Errorf("the named group is missing from the report:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "remembered") {
		t.Errorf("a group that --logs does not remove was reported as going:\n%s", stdout.String())
	}
}

// A recorded path whose clone was deleted or moved is the one most worth
// forgetting, and it is exactly the one a validity check hides: the group
// disappears from the list, so --remembered has nothing to clear and the stale
// entry stays in the config for good.
func TestAStaleRecordedPathCanStillBeForgotten(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	gone := filepath.Join(t.TempDir(), "moved-away")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = gone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runCleanup(context.Background(), []string{"--remembered"}, &stdout, &stderr); err != nil {
		t.Fatalf("runCleanup --remembered = %v", err)
	}

	after, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.TemplatesCheckout != "" {
		t.Errorf("the stale path survived: %q", after.TemplatesCheckout)
	}
}

// The run directory of a command still running holds its plans and its log, and
// it is the log somebody reads when that run fails. Offering it for removal is
// offering to delete the evidence of a run in progress.
func TestARunDirectoryInUseIsNotOffered(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)

	mine, err := os.MkdirTemp("", "lerian-infra-")
	if err != nil {
		t.Fatal(err)
	}
	claimRunDir(mine)

	finished, err := os.MkdirTemp("", "lerian-infra-")
	if err != nil {
		t.Fatal(err)
	}
	// A pid no process has. Claimed and then gone is what a finished run leaves.
	if err := os.WriteFile(filepath.Join(finished, runOwnerFile), []byte("4194304"), 0o600); err != nil {
		t.Fatal(err)
	}

	offered := runLogDirs()

	for _, dir := range offered {
		if dir == mine {
			t.Errorf("the directory of a running command was offered for removal: %s", dir)
		}
	}
	var sawFinished bool
	for _, dir := range offered {
		if dir == finished {
			sawFinished = true
		}
	}
	if !sawFinished {
		t.Errorf("a finished run's directory was not offered:\n%v", offered)
	}
}

// A marker that exists but cannot be read is the uncertain case the rule in
// runIsOver already covers in words: it counts as running. On a shared /tmp the
// directory belongs to another user and is mode 0700, so the read fails with a
// permission error — and treating that as finished both offers somebody else's
// live run for removal and breaks the removal loop on the RemoveAll that follows,
// leaving every group after it unprocessed.
func TestAnUnreadableMarkerCountsAsRunning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads regardless of mode, so there is no unreadable file to make")
	}
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)

	locked, err := os.MkdirTemp("", "lerian-infra-")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(locked, runOwnerFile)
	if err := os.WriteFile(marker, []byte("4194304"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(marker, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(marker, 0o600) }()

	for _, dir := range runLogDirs() {
		if dir == locked {
			t.Errorf("a directory whose marker cannot be read was offered for removal: %s", dir)
		}
	}
}

// A run whose marker could not be written is a run no cleanup can recognize as
// running, so it would offer the directory this run is still logging into. The
// write is part of creating the directory, and it fails the same way.
func TestARunThatCannotClaimItsDirectoryFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes regardless of mode, so there is no unwritable directory to make")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()

	if err := claimRunDir(dir); err == nil {
		t.Error("claimRunDir reported success for a directory it could not write to")
	}
}

// The directory is only given its published name once it carries its marker.
//
// MkdirTemp with the "lerian-infra-" prefix creates a directory the cleanup glob
// matches, and the marker is written a syscall later. A cleanup running in that
// window sees a matching directory with no marker, reads it as a leftover from an
// older version, and deletes the plans and log of a run that is just starting.
//
// The window is short and the consequence is destructive, which is the shape of
// bug that gets dismissed as unlikely until it eats somebody's log. Tested by
// looking inside the window rather than by racing it: the hook runs at the moment
// the directory exists but the marker does not, and asserts that nothing the
// cleanup can see has appeared.
func TestARunDirectoryIsNotVisibleBeforeItIsClaimed(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)

	var visibleTooEarly []string
	previous := runDirCreated
	runDirCreated = func(string) { visibleTooEarly = runLogDirs() }
	t.Cleanup(func() { runDirCreated = previous })

	dir, err := newRunDir()
	if err != nil {
		t.Fatal(err)
	}

	if len(visibleTooEarly) != 0 {
		t.Errorf("the cleanup could see %v before it carried a marker", visibleTooEarly)
	}
	// And once published it is a run directory in every respect: matching the
	// glob, carrying its marker, and recognized as this process's own.
	if runIsOver(dir) {
		t.Error("the directory of this very process reads as finished")
	}
	if !strings.HasPrefix(filepath.Base(dir), "lerian-infra-") {
		t.Errorf("the published name does not match the glob: %s", dir)
	}
}

// A marker that cannot be read as a pid is not evidence that the run ended. The
// rule written above runIsOver says uncertainty counts as running, and an
// unparseable marker is uncertainty.
func TestAnUnreadablePidCountsAsRunning(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)

	dir, err := os.MkdirTemp("", "lerian-infra-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, runOwnerFile), []byte("not a pid"), 0o600); err != nil {
		t.Fatal(err)
	}

	if runIsOver(dir) {
		t.Error("a marker that is not a pid was read as proof the run finished")
	}
	for _, offered := range runLogDirs() {
		if offered == dir {
			t.Errorf("it was offered for removal anyway: %s", offered)
		}
	}
}

// Hiding the staging directory from the glob bought safety at the cost of a leak:
// if anything between creating it and renaming it fails, a .lerian-infra-* is left
// in the temp directory that this tool's own cleanup cannot see — the glob is
// anchored and does not match the dot. One per failed run, forever.
//
// Only this process has ever seen that directory, so removing it on the way out
// costs nothing.
func TestAFailedRunDirectoryLeavesNothingBehind(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)

	// A directory where the marker file has to go: the write fails, and the
	// failure lands after the staging directory already exists.
	previous := runDirCreated
	runDirCreated = func(dir string) {
		if err := os.Mkdir(filepath.Join(dir, runOwnerFile), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { runDirCreated = previous })

	if _, err := newRunDir(); err == nil {
		t.Fatal("newRunDir reported success with an unwritable marker")
	}

	left, err := os.ReadDir(temp)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range left {
		t.Errorf("left behind in the temp directory: %s", entry.Name())
	}
}
