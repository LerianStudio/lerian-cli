package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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
