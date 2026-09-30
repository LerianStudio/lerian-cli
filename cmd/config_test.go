package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
