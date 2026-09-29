package cmd

import (
	"errors"
	"testing"

	infrapkg "github.com/lerian-studio/lerian-cli/internal/infra"
)

// The menu is a session, not a launcher: running the thing that was picked and
// exiting means choosing a second command costs a second start-up, banner and
// all. It asks again once the command is done.
func TestTheMenuComesBackAfterACommandRuns(t *testing.T) {
	answers := []string{"version", "infra"}
	var ran []string

	code := session(
		func() (string, error) {
			if len(answers) == 0 {
				return "", infrapkg.ErrAborted
			}
			next := answers[0]
			answers = answers[1:]
			return next, nil
		},
		func(name string) error {
			ran = append(ran, name)
			return nil
		},
	)

	if len(ran) != 2 || ran[0] != "version" || ran[1] != "infra" {
		t.Errorf("ran %v, want both commands", ran)
	}
	if code != 0 {
		t.Errorf("exit code %d for a session that ended cleanly", code)
	}
}

// Leaving is what q does, and it is not a failure.
func TestLeavingTheMenuExitsCleanly(t *testing.T) {
	code := session(
		func() (string, error) { return "", infrapkg.ErrAborted },
		func(string) error { t.Fatal("nothing was chosen and something ran"); return nil },
	)

	if code != 0 {
		t.Errorf("exit code %d for leaving the menu", code)
	}
}

// A command that fails does not end the session. The operator is sitting there,
// and the usual next step after a failure is another command — reading the logs,
// checking the environment — not starting the CLI again.
//
// The failure it ends on is still the answer the shell gets, so `lerian && deploy`
// does not run deploy.
func TestAFailedCommandKeepsTheSessionOpen(t *testing.T) {
	answers := []string{"version", "infra"}
	var ran []string

	code := session(
		func() (string, error) {
			if len(answers) == 0 {
				return "", infrapkg.ErrAborted
			}
			next := answers[0]
			answers = answers[1:]
			return next, nil
		},
		func(name string) error {
			ran = append(ran, name)
			if name == "infra" {
				return errors.New("terraform is not installed")
			}
			return nil
		},
	)

	if len(ran) != 2 {
		t.Errorf("the session ended before the second command: ran %v", ran)
	}
	if code == 0 {
		t.Error("a session whose last command failed reported success")
	}
}

// A failure followed by a command that works is a session that succeeded: the
// code reports the last thing that happened, not the worst.
func TestTheCodeIsTheLastCommandsCode(t *testing.T) {
	answers := []string{"infra", "version"}

	code := session(
		func() (string, error) {
			if len(answers) == 0 {
				return "", infrapkg.ErrAborted
			}
			next := answers[0]
			answers = answers[1:]
			return next, nil
		},
		func(name string) error {
			if name == "infra" {
				return errors.New("terraform is not installed")
			}
			return nil
		},
	)

	if code != 0 {
		t.Errorf("exit code %d after a successful last command", code)
	}
}

// Anything other than a clean exit from the menu itself — a terminal that went
// away mid-question — ends the session rather than asking into the void.
func TestABrokenMenuEndsTheSession(t *testing.T) {
	var ran []string
	code := session(
		func() (string, error) { return "", errors.New("the terminal went away") },
		func(name string) error { ran = append(ran, name); return nil },
	)

	if len(ran) != 0 {
		t.Errorf("ran %v after the menu failed", ran)
	}
	if code == 0 {
		t.Error("a broken menu reported success")
	}
}
