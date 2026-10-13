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
