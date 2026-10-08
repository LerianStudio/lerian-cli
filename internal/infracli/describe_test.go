package infracli

import (
	"bytes"
	"context"
	"fmt"
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

// LERIAN_TF_REPO naming a directory that is not a checkout is the failure this
// page exists to explain: it said "none found — lerian infra init --clone",
// sending somebody to clone while the variable they set stayed wrong.
func TestABadRepoVariableIsNamedRatherThanHiddenBehindClone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("LERIAN_TF_REPO", t.TempDir()) // exists, is not a checkout

	var out bytes.Buffer
	DescribeMachine(context.Background(), &out)

	if strings.Contains(out.String(), "none found — lerian infra init --clone") {
		t.Errorf("a bad LERIAN_TF_REPO is reported as having no checkout to clone:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "none usable") {
		t.Errorf("the real cause is not shown:\n%s", out.String())
	}
}

// And with nothing set anywhere, cloning IS the answer.
func TestWithNothingAnywhereCloningIsStillTheAdvice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("LERIAN_TF_REPO", "")

	var out bytes.Buffer
	DescribeMachine(context.Background(), &out)

	if !strings.Contains(out.String(), "none found — lerian infra init --clone") {
		t.Errorf("an empty machine is not told how to get a checkout:\n%s", out.String())
	}
}

// An EKS entry is named by its ARN — sixty characters of which the last few are
// the only part anybody reads.
func TestEksClusterNamesAreShortened(t *testing.T) {
	short := shortKubeName("arn:aws:eks:us-east-2:524121347244:cluster/example-dev-eks")
	if short != "example-dev-eks" {
		t.Errorf("shortKubeName = %q", short)
	}

	// Anything else is already its own name: a kubeconfig holds clusters from
	// anywhere, and a local one has nothing to shorten.
	for _, name := range []string{"kind-local", "minikube", "some-cluster"} {
		if got := shortKubeName(name); got != name {
			t.Errorf("shortKubeName(%q) = %q, want it untouched", name, got)
		}
	}
}

// "Which cluster am I actually talking to" is what this line exists to answer,
// and the region and account are what tell two same-named clusters apart.
func TestTheCurrentContextSaysWhereItPoints(t *testing.T) {
	described := describeKubeContext("arn:aws:eks:sa-east-1:905418424496:cluster/example-eks")

	for _, want := range []string{"example-eks", "sa-east-1", "905418424496"} {
		if !strings.Contains(described, want) {
			t.Errorf("describeKubeContext does not mention %q: %q", want, described)
		}
	}
}

// A context that is not an EKS ARN is shown as it is.
func TestANonEksContextIsShownAsItIs(t *testing.T) {
	if got := describeKubeContext("kind-local"); got != "kind-local" {
		t.Errorf("describeKubeContext = %q", got)
	}
}

// The page says where things are. A machine that has never run kubectl has no
// file, and saying so beats an empty group.
func TestAMissingKubeconfigIsReportedAsMissing(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nowhere", "config"))

	var out bytes.Buffer
	describeKubeconfig(func(label, value string) {
		fmt.Fprintf(&out, "%s=%s\n", label, value)
	})

	if !strings.Contains(out.String(), "not there yet") {
		t.Errorf("a missing kubeconfig is not reported:\n%s", out.String())
	}
}

// A file that is there and cannot be read is a third thing, and reporting it as
// empty would be a guess about contents nobody could see.
func TestAnUnreadableKubeconfigIsReportedAsSuch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("clusters: [broken: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)

	var out bytes.Buffer
	describeKubeconfig(func(label, value string) {
		fmt.Fprintf(&out, "%s=%s\n", label, value)
	})

	if !strings.Contains(out.String(), "cannot be read") {
		t.Errorf("an unparsable kubeconfig is not reported:\n%s", out.String())
	}
	if strings.Contains(out.String(), "clusters=0") {
		t.Errorf("it reported an empty file it could not read:\n%s", out.String())
	}
}

// Names only. A kubeconfig can carry client certificates and tokens, and a page
// that exists to say where things are has no business printing them.
func TestNoCredentialFromTheKubeconfigIsPrinted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: ctx
  cluster:
    server: https://example
    certificate-authority-data: SECRETCADATA
users:
- name: ctx
  user:
    token: SECRETTOKEN
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)

	var out bytes.Buffer
	describeKubeconfig(func(label, value string) {
		fmt.Fprintf(&out, "%s=%s\n", label, value)
	})

	for _, secret := range []string{"SECRETTOKEN", "SECRETCADATA", "https://example"} {
		if strings.Contains(out.String(), secret) {
			t.Errorf("the page printed %q:\n%s", secret, out.String())
		}
	}
}
