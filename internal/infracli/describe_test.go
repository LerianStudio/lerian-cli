package infracli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The question "what does this machine have" has four answers that live in four
// places, and reading them meant four commands and knowing which. One page,
// grouped by who owns each thing — because "profile" means one thing under Lerian
// and another under AWS, and a flat list puts them next to each other.
func TestDescribeMachineGroupsByOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	DescribeMachine(context.Background(), &out)

	for _, heading := range []string{"This tool", "Templates", "AWS", "Tools"} {
		if !strings.Contains(out.String(), heading) {
			t.Errorf("no %q section:\n%s", heading, out.String())
		}
	}
}

// Each section says where it read from, so somebody can go and look.
func TestDescribeMachineNamesEveryFileItRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	aws := filepath.Join(home, ".aws")
	if err := os.MkdirAll(aws, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "[profile one]\nsso_session = acme-sso\nregion = us-east-1\n\n[profile two]\nregion = us-east-2\n"
	if err := os.WriteFile(filepath.Join(aws, "config"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	DescribeMachine(context.Background(), &out)

	for _, want := range []string{".lerian/config.yaml", ".aws/config"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the page does not name %s:\n%s", want, out.String())
		}
	}
	// Profiles are counted and a few named; the whole list is the account menu's job.
	if !strings.Contains(out.String(), "2") || !strings.Contains(out.String(), "one") {
		t.Errorf("the AWS profiles are not reported:\n%s", out.String())
	}
}

// It reads files and asks no AWS API: this is the page somebody opens to find
// out where things are, and it should not take a round trip per profile — nor
// fail on a machine with no network.
func TestDescribeMachineSaysWhoResolvesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	aws := filepath.Join(home, ".aws")
	if err := os.MkdirAll(aws, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aws, "config"), []byte("[profile one]\nsso_session = acme-sso\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	DescribeMachine(context.Background(), &out)

	// It must say that it did not check, or somebody reads a list of profiles as a
	// list of working profiles.
	if !strings.Contains(out.String(), "infra check") {
		t.Errorf("the page does not say what would verify them:\n%s", out.String())
	}
}

// "found by managed" is the name of a constant, not a sentence. The five ways of
// pointing at a checkout each need a phrase somebody can act on — knowing it came
// from $LERIAN_TF_REPO tells you which to unset.
func TestTheCheckoutSourceReadsAsASentence(t *testing.T) {
	for source, want := range map[checkoutSource]string{
		sourceManaged:    "managed path",
		sourceRemembered: "remembered",
		sourceEnv:        "LERIAN_TF_REPO",
		sourceFlag:       "--repo",
		sourceWorkingIn:  "working directory",
	} {
		got := explainSource(source)
		if !strings.Contains(got, want) {
			t.Errorf("%q reads as %q, want it to mention %q", source, got, want)
		}
		if got == string(source) && source == sourceManaged {
			t.Errorf("%q is still the bare constant", source)
		}
	}
}
