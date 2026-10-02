package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
)

// `lerian config reset production --yes` reads like "reset the production
// profile". It is not a thing this command does — and without an argument
// validator cobra accepts the word, ignores it, and removes everything.
//
// A destructive command that silently does more than it was asked is the shape of
// bug worth an explicit test.
func TestResetRefusesAnArgumentItDoesNotUnderstand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".lerian", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("current-profile: default\nprofiles: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"config", "reset", "production", "--yes"})
	t.Cleanup(func() { rootCmd.SetArgs(nil); configResetYes = false })

	err := rootCmd.Execute()

	if err == nil {
		t.Error("an argument nobody understands was accepted")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("the configuration was removed by a command that was not understood: %v", statErr)
	}
}

// The global --profile is a Lerian platform credential; the infra commands take
// an AWS profile under the same flag name. A description that says only "profile
// to use" leaves somebody to guess which.
func TestTheGlobalProfileFlagSaysWhoseProfileItIs(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("profile")
	if flag == nil {
		t.Fatal("the global --profile flag is gone")
	}
	if !strings.Contains(flag.Usage, "Lerian") {
		t.Errorf("the flag does not say whose profile it is: %q", flag.Usage)
	}
	if !strings.Contains(flag.Usage, "AWS") {
		t.Errorf("the flag does not distinguish itself from the AWS one: %q", flag.Usage)
	}
}

// The main menu is built from each command's Short, so that line is where
// somebody decides what a command is for. "Authentication commands" does not say
// authentication with what — and on a menu whose other entry deploys AWS
// infrastructure, the obvious guess is the wrong one.
func TestTheMenuSaysWhatEachCommandAuthenticatesWith(t *testing.T) {
	for _, choice := range menuChoices(rootCmd) {
		switch choice.Value {
		case "auth":
			if !strings.Contains(choice.Note, "Lerian") {
				t.Errorf("auth does not say what it authenticates with: %q", choice.Note)
			}
			if !strings.Contains(choice.Note, "AWS") {
				t.Errorf("auth does not distinguish itself from AWS credentials: %q", choice.Note)
			}
		case "infra":
			if !strings.Contains(choice.Note, "AWS") {
				t.Errorf("infra does not say it is AWS: %q", choice.Note)
			}
		}
	}
}

// Nowhere in this CLI should say AWS credentials must be in ~/.aws. The infra
// commands accept the ones already in the environment, which is what CI has — and
// a machine with none of its own would be sent to create files it does not need.
func TestNothingClaimsAWSCredentialsMustBeInAFile(t *testing.T) {
	texts := map[string]string{
		"profile flag":   rootCmd.PersistentFlags().Lookup("profile").Usage,
		"config command": configCmd.Long,
	}
	for _, command := range rootCmd.Commands() {
		texts[command.Name()+" long"] = command.Long
	}

	for where, text := range texts {
		if strings.Contains(text, "They live in ~/.aws") || strings.Contains(text, "they live in ~/.aws") {
			t.Errorf("%s says AWS credentials must be in a file: %q", where, text)
		}
	}
}

// managedCheckout builds a checkout where init --clone would put one.
func managedCheckout(t *testing.T) string {
	t.Helper()
	managed, err := infra.ManagedCheckoutPath("")
	if err != nil {
		t.Fatal(err)
	}
	return checkoutAt(t, managed)
}

// Somebody deciding whether to reset should not learn afterwards that a clone of
// theirs was in scope. The directories are named before the first question.
func TestResetNamesTheCheckoutsBeforeAsking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	managed := managedCheckout(t)

	configResetYes = true
	t.Cleanup(func() { configResetYes = false })

	var out bytes.Buffer
	if err := resetConfig(context.Background(), &out); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), managed) {
		t.Errorf("the checkout in scope is not named:\n%s", out.String())
	}
}

// With no checkout anywhere there is nothing to ask about, and a question about
// a directory that is not there is noise.
func TestResetIsQuietWithNoCheckoutAnywhere(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configResetYes = true
	t.Cleanup(func() { configResetYes = false })

	var out bytes.Buffer
	if err := resetConfig(context.Background(), &out); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), "templates checkout(s)") {
		t.Errorf("asked about a checkout that is not there:\n%s", out.String())
	}
}

// --yes has meant "forget the configuration" on every machine it already runs
// on. Making it delete a git clone as well would change what those invocations
// do, silently, the next time the CLI is updated.
func TestYesAloneDeletesNoDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	managed := managedCheckout(t)

	configResetYes = true
	t.Cleanup(func() { configResetYes = false })

	var out bytes.Buffer
	if err := resetConfig(context.Background(), &out); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(managed); err != nil {
		t.Fatalf("--yes alone deleted %s: %v", managed, err)
	}
	if !strings.Contains(out.String(), "kept "+managed) {
		t.Errorf("the directory was kept and not said to be:\n%s", out.String())
	}
}

// And the flag that does say it deletes it, for a script that means it — the
// only way to reach the deletion with no terminal to ask at.
func TestDeleteTemplatesRemovesTheDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	managed := managedCheckout(t)

	configResetYes = true
	configResetTemplates = true
	t.Cleanup(func() { configResetYes, configResetTemplates = false, false })

	var out bytes.Buffer
	if err := resetConfig(context.Background(), &out); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Errorf("%s is still there: %v", managed, err)
	}
	if !strings.Contains(out.String(), "starts from nothing") {
		t.Errorf("everything is gone and it does not say so:\n%s", out.String())
	}
}

// "The next run starts from nothing" is false when a managed checkout is about to
// be picked up again, and it sits three lines under the note saying exactly that.
func TestResetDoesNotPromiseNothingWhenSomethingStays(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	managed, err := infra.ManagedCheckoutPath("")
	if err != nil {
		t.Fatal(err)
	}
	checkoutAt(t, managed)
	path := filepath.Join(home, ".lerian", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("current-profile: default\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	configResetYes = true
	t.Cleanup(func() { configResetYes = false })

	var out bytes.Buffer
	if err := resetConfig(context.Background(), &out); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), "starts from nothing") {
		t.Errorf("it promises nothing while a checkout stays:\n%s", out.String())
	}
	// Driven through resetConfig, not through the helper under it: a test that
	// calls the helper proves the helper works and says nothing about whether the
	// command calls it. Mine did exactly that, and stayed green with the call
	// removed.
	if !strings.Contains(out.String(), managed) {
		t.Errorf("reset did not mention the checkout that stays:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "kept "+managed) {
		t.Errorf("reset did not say the directory survives:\n%s", out.String())
	}
}

// Pointing the CLI at a checkout had five forms and none of them stuck: a flag
// for one run, a variable for one shell, standing inside it, a conventional
// path, and an answer the CLI only asks for when every other way has failed. A
// client who cloned the templates somewhere of their own had no way to say so.
func TestSettingTheTemplatesPathRecordsIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	checkout := filepath.Join(home, "somewhere", "foundation")
	for _, marker := range []string{"examples/aws/_modules", "examples/aws/backend"} {
		if err := os.MkdirAll(filepath.Join(checkout, filepath.FromSlash(marker)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := setTemplates(&out, checkout); err != nil {
		t.Fatalf("setTemplates = %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TemplatesCheckout != checkout {
		t.Errorf("recorded %q, want %q", cfg.TemplatesCheckout, checkout)
	}
	if !strings.Contains(out.String(), checkout) {
		t.Errorf("it did not say what it recorded:\n%s", out.String())
	}
}

// A directory that is not a checkout is refused at the prompt rather than
// several steps later, where the failure names a missing file instead of the
// wrong directory.
func TestSettingAPathThatIsNotACheckoutIsRefused(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	err := setTemplates(&out, t.TempDir())

	if err == nil {
		t.Fatal("a directory with no templates in it was accepted")
	}
	if !strings.Contains(err.Error(), "examples/aws/_modules") {
		t.Errorf("the error does not say what makes a checkout: %v", err)
	}

	cfg, _ := config.Load()
	if cfg.TemplatesCheckout != "" {
		t.Errorf("it recorded %q anyway", cfg.TemplatesCheckout)
	}
}

// A relative path means a different directory from the next place it is read, so
// it is resolved before it is written down.
func TestARelativeTemplatesPathIsRecordedAbsolute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	checkout := filepath.Join(home, "rel")
	for _, marker := range []string{"examples/aws/_modules", "examples/aws/backend"} {
		if err := os.MkdirAll(filepath.Join(checkout, filepath.FromSlash(marker)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(home)

	var out bytes.Buffer
	if err := setTemplates(&out, "rel"); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.Load()
	if !filepath.IsAbs(cfg.TemplatesCheckout) {
		t.Errorf("recorded the relative %q", cfg.TemplatesCheckout)
	}
}

// checkoutAt builds the pair of directories the CLI recognizes a checkout by.
func checkoutAt(t *testing.T, root string) string {
	t.Helper()
	for _, dir := range []string{"examples/aws/_modules", "examples/aws/backend"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The menu row has no command line to put a path on, so arriving there with no
// argument has to be a question. Printing the usage of a command nobody typed
// leaves the operator exactly where they started.
func TestChoosingTemplatesOffTheMenuRecordsWhatWasAnswered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	checkout := checkoutAt(t, t.TempDir())

	var out bytes.Buffer
	answered := func(io.Writer) (infracli.TemplatesAnswer, error) {
		return infracli.TemplatesAnswer{Path: checkout}, nil
	}

	if err := askThenRecord(&out, answered); err != nil {
		t.Fatalf("askThenRecord = %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TemplatesCheckout != checkout {
		t.Errorf("config holds %q, want the answered %q", cfg.TemplatesCheckout, checkout)
	}
}

// Forgetting is reachable from the same menu, because --clear is not: a row that
// can only record would strand anybody who wanted to undo one.
func TestForgettingFromTheMenuClearsTheRecordedPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = checkoutAt(t, t.TempDir())
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	forgetting := func(io.Writer) (infracli.TemplatesAnswer, error) {
		return infracli.TemplatesAnswer{Forget: true}, nil
	}

	if err := askThenRecord(&out, forgetting); err != nil {
		t.Fatalf("askThenRecord = %v", err)
	}

	after, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.TemplatesCheckout != "" {
		t.Errorf("config still holds %q", after.TemplatesCheckout)
	}
}

// r and q mean "not this". The session is about to redraw the menu that was
// left, and a red line under it would report a decision not to decide as a
// failure.
func TestLeavingThePromptIsNotAFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, leaving := range []error{infracli.ErrBack, infra.ErrAborted} {
		t.Run(leaving.Error(), func(t *testing.T) {
			var out bytes.Buffer
			left := func(io.Writer) (infracli.TemplatesAnswer, error) {
				return infracli.TemplatesAnswer{}, leaving
			}

			if err := askThenRecord(&out, left); err != nil {
				t.Errorf("askThenRecord = %v, want nil", err)
			}
		})
	}
}

// With nobody to ask — a pipe, CI — the usage text is still the right answer,
// and it is the only one left.
func TestWithNoTerminalTheCommandStillSaysWhatItNeeds(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"config", "templates"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("no path, no terminal, and no error")
	}
	if !strings.Contains(err.Error(), "--clear") {
		t.Errorf("the error does not mention how to clear it:\n%v", err)
	}
}
