package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig puts a config where config.Load will find it, and only for this
// test: HOME is the one thing that decides where that is.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".lerian")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The value the operator already gave `lerian auth login`. Asking for it again
// sends somebody to look up something they have already told this CLI.
func TestTheControlPlaneComesFromTheSignedInProfile(t *testing.T) {
	writeConfig(t, `current-profile: work
profiles:
  work:
    api-url: https://platform.example.com
    api-key: k
    tenant-id: t
`)

	if got := ControlPlaneFromProfile(""); got != "https://platform.example.com" {
		t.Errorf("control plane = %q, want the profile's api-url", got)
	}
}

// A named profile wins over the current one: somebody passing --profile is
// saying which platform they mean.
func TestANamedProfileIsUsed(t *testing.T) {
	writeConfig(t, `current-profile: work
profiles:
  work:
    api-url: https://work.example.com
  lab:
    api-url: https://lab.example.com
`)

	if got := ControlPlaneFromProfile("lab"); got != "https://lab.example.com" {
		t.Errorf("control plane = %q, want the named profile's api-url", got)
	}
}

// A trailing slash would make every path the agent builds contain "//".
func TestTheTrailingSlashIsDropped(t *testing.T) {
	writeConfig(t, `current-profile: work
profiles:
  work:
    api-url: https://platform.example.com/
`)

	if got := ControlPlaneFromProfile(""); got != "https://platform.example.com" {
		t.Errorf("control plane = %q, want no trailing slash", got)
	}
}

// Nothing signed in is a real state, not an error: the agent can be installed
// against a control plane this machine has no profile for.
func TestNoProfileMeansNoDefault(t *testing.T) {
	writeConfig(t, "profiles: {}\n")

	if got := ControlPlaneFromProfile(""); got != "" {
		t.Errorf("control plane = %q, want empty so the question is asked", got)
	}

	writeConfig(t, "this is not yaml: [")
	if got := ControlPlaneFromProfile(""); got != "" {
		t.Errorf("an unreadable config produced %q instead of no default", got)
	}
}

// The control plane belongs to Lerian, not to the cluster being installed
// into. Asking for it asks somebody to supply a value they have no way of
// choosing — so it is a constant here, and this is the only line that changes
// when the address exists.
func TestTheControlPlaneIsACarriedConstantNotAQuestion(t *testing.T) {
	writeConfig(t, `current-profile: work
profiles:
  work:
    api-url: https://signed-in.example.com
`)

	got := DefaultControlPlane("")

	switch ControlPlane {
	case "":
		// Nothing published yet, so what the operator signed in to is the best
		// answer there is — a per-machine value chosen on purpose.
		if got != "https://signed-in.example.com" {
			t.Errorf("with no published address it must fall back to the profile, got %q", got)
		}
	default:
		// Once published, it wins: a machine signed in to a different platform
		// must not quietly point its agent somewhere else.
		if got != ControlPlane {
			t.Errorf("the published address must win over the profile, got %q", got)
		}
	}
}

// With neither, an install has nothing to point at, and the caller substitutes
// the placeholder rather than asking a question with no answer.
func TestNoAddressAndNoProfileYieldsNothing(t *testing.T) {
	writeConfig(t, "profiles: {}\n")

	if ControlPlane == "" && DefaultControlPlane("") != "" {
		t.Error("an unconfigured machine produced an address from nowhere")
	}
}
