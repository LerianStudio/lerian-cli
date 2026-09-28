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

	logDir := filepath.Join(os.TempDir(), "lerian-infra-dry-run-test")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(logDir) }()

	var stdout, stderr bytes.Buffer
	if err := runCleanup(context.Background(), []string{"--dry-run"}, &stdout, &stderr); err != nil {
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
