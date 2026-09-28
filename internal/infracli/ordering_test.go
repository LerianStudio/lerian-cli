package infracli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Everything the guided questions can lead to ends in terraform and the AWS CLI,
// so a machine missing one cannot finish whatever is chosen. The tools are
// therefore verified before the first question, not after the last: asking first
// throws away three answers and reports the failure where it reads as a problem
// with the choices rather than with the machine.
//
// The assertion is positional rather than behavioural because the ordering is
// the whole point: a run on a machine with no tools must produce the check
// report and nothing that looks like a question.
func TestTheToolsAreVerifiedBeforeTheQuestions(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", checkout)
	t.Setenv("PATH", t.TempDir()) // no terraform, no aws

	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--target", "infra-base"}, &stdout, &stderr)

	if err == nil {
		t.Fatal("a run with no terraform and no aws in PATH returned nil")
	}

	report := stdout.String() + stderr.String()
	if !strings.Contains(report, "terraform") {
		t.Errorf("the failure does not name the missing tool:\n%s", report)
	}
	// guidedRun asks this first. Seeing it means the questions ran ahead of the
	// verification, which is the ordering this test exists to pin.
	if strings.Contains(report, "Which environment?") {
		t.Errorf("the run asked its questions before checking the tools:\n%s", report)
	}
}

// --list resolves the catalog from the checkout and prints it. It reaches no
// tool, so it must not be gated on one: gating it would demand terraform to
// answer a question about what is on disk.
func TestListingTargetsNeedsNoTools(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	if err := os.MkdirAll(filepath.Join(checkout, "examples", "aws", "products", "midaz", "postgres"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LERIAN_TF_REPO", checkout)
	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--list"}, &stdout, &stderr); err != nil {
		t.Fatalf("--list on a machine with no tools = %v\n%s", err, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "not usable") {
		t.Errorf("--list was gated on a tool it never calls:\n%s", stdout.String()+stderr.String())
	}
}
