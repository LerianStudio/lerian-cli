package infracli

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/menu"
)

// answers replays a session: one line per question, in order.
func answers(lines ...string) *bufio.Reader {
	return menu.NewReader(strings.NewReader(strings.Join(lines, "\n") + "\n"))
}

// The menu writes a command line; it does not reimplement the run. Every case
// here pins the arguments it produces, because that string is the contract — it
// is what the operator could have typed, and what the flag path then parses.
func TestComposeArgsBuildsTheCommandLine(t *testing.T) {
	tests := []struct {
		name    string
		replies []string
		want    string
	}{
		{"list needs nothing else", []string{"6"}, "--list"},
		{"check needs nothing else", []string{"7"}, "check"},
		{"init asks only for the environment", []string{"8", "1"}, "init --env dev"},
		{"plan asks environment and target", []string{"1", "2", "1"}, "--env stg --target infra-base --action plan"},
		{"output on prd", []string{"4", "3", "3"}, "--env prd --target all --action output"},
		{"a typed target", []string{"1", "1", "4", "midaz"}, "--env dev --target midaz --action plan"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer

			args, err := composeArgs(answers(test.replies...), &out)
			if err != nil {
				t.Fatalf("composeArgs = %v", err)
			}
			if got := strings.Join(args, " "); got != test.want {
				t.Errorf("composeArgs = %q, want %q", got, test.want)
			}
		})
	}
}

// apply and destroy change real infrastructure, so the menu offers the dry run
// ahead of them rather than after.
func TestComposeArgsOffersADryRunBeforeWriting(t *testing.T) {
	var out bytes.Buffer

	args, err := composeArgs(answers("2", "1", "1", "1"), &out) // apply, dev, infra-base, dry-run first
	if err != nil {
		t.Fatalf("composeArgs = %v", err)
	}

	// The action has to travel with --dry-run: without it the flag path falls back
	// to plan, so the preview would describe something other than what the
	// operator is about to run.
	if got := strings.Join(args, " "); got != "--env dev --target infra-base --action apply --dry-run" {
		t.Errorf("composeArgs = %q", got)
	}
	if !strings.Contains(out.String(), "changes real infrastructure") {
		t.Errorf("apply was not called out as destructive:\n%s", out.String())
	}
}

// destroy is the case where a preview without the action is actively
// misleading: the flag path would resolve the stages in forward order and skip
// the destroy backend warnings.
func TestADryRunOfDestroyPreviewsDestroy(t *testing.T) {
	var out bytes.Buffer

	args, err := composeArgs(answers("3", "1", "1", "1"), &out) // destroy, dev, infra-base, dry-run first
	if err != nil {
		t.Fatalf("composeArgs = %v", err)
	}

	got := strings.Join(args, " ")
	if !strings.Contains(got, "--action destroy") {
		t.Errorf("the dry run of a destroy does not carry the action: %q", got)
	}
	if !strings.Contains(got, "--dry-run") {
		t.Errorf("the dry run lost its flag: %q", got)
	}
}

// helm-values is rejected for infra-base, bootstrap and all, and infra-base is
// what Enter picks — so offering that menu walked every default answer into a
// guaranteed failure.
func TestHelmValuesAsksForAProductInsteadOfOfferingTargetsItRejects(t *testing.T) {
	var out bytes.Buffer

	args, err := composeArgs(answers("5", "1", "midaz"), &out) // helm-values, dev, typed product
	if err != nil {
		t.Fatalf("composeArgs = %v", err)
	}

	if got := strings.Join(args, " "); got != "--env dev --target midaz --action helm-values" {
		t.Errorf("composeArgs = %q", got)
	}
	if strings.Contains(out.String(), "infra-base") {
		t.Errorf("helm-values was still offered a target it cannot use:\n%s", out.String())
	}
}

func TestComposeArgsTakesApplyWhenConfirmed(t *testing.T) {
	var out bytes.Buffer

	args, err := composeArgs(answers("2", "1", "1", "2"), &out) // apply, dev, infra-base, go ahead
	if err != nil {
		t.Fatalf("composeArgs = %v", err)
	}

	if got := strings.Join(args, " "); got != "--env dev --target infra-base --action apply" {
		t.Errorf("composeArgs = %q", got)
	}
}

// Quitting is a clean exit, not a failure: backing out of a menu is not the
// same as a bad flag.
func TestComposeArgsReportsQuitAsCanceled(t *testing.T) {
	var out bytes.Buffer

	_, err := composeArgs(answers("q"), &out)

	if !errors.Is(err, menu.ErrCanceled) {
		t.Errorf("quitting returned %v, want ErrCanceled", err)
	}
}

// One reader has to span the session: a buffered reader created per question
// swallows the answers to every question after the first.
func TestASessionKeepsItsAnswersAcrossQuestions(t *testing.T) {
	var out bytes.Buffer

	args, err := composeArgs(answers("1", "3", "2"), &out) // plan, prd, bootstrap
	if err != nil {
		t.Fatalf("composeArgs = %v", err)
	}

	got := strings.Join(args, " ")
	if !strings.Contains(got, "--env prd") || !strings.Contains(got, "--target bootstrap") {
		t.Errorf("answers after the first were lost: %q", got)
	}
}
