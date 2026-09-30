package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func atHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// Reset puts the machine back to never having run this tool: the config file is
// gone, so the next run asks what it asked the first time.
func TestResetLeavesNoConfiguration(t *testing.T) {
	atHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = "/somewhere/lerian-terraform-foundation"
	cfg.CurrentProfile = "staging"
	cfg.Profiles = map[string]Profile{"staging": {APIURL: "https://api", APIKey: "secret", TenantID: "t"}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	removed, err := Reset()
	if err != nil {
		t.Fatalf("Reset = %v", err)
	}

	path, _ := GetConfigPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the config file is still there: %v", err)
	}
	if len(removed) != 1 || removed[0] != path {
		t.Errorf("Reset reported %v, want the file it removed", removed)
	}

	// And a fresh Load is a fresh machine, not an error. CurrentProfile comes back
	// as "default" because that is what Load gives a machine with no file — the
	// factory setting, not a leftover.
	after, err := Load()
	if err != nil {
		t.Fatalf("loading after a reset: %v", err)
	}
	if after.TemplatesCheckout != "" {
		t.Errorf("the templates checkout survived the reset: %q", after.TemplatesCheckout)
	}
	if len(after.Profiles) != 0 {
		t.Errorf("a profile survived the reset: %+v", after.Profiles)
	}
	fresh, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.CurrentProfile != fresh.CurrentProfile {
		t.Errorf("the current profile is not what a fresh machine has: %q", after.CurrentProfile)
	}
}

// Resetting a machine that has nothing to reset is not a failure. It is the
// state Reset exists to produce.
func TestResetOnAFreshMachineIsQuiet(t *testing.T) {
	atHome(t)

	removed, err := Reset()
	if err != nil {
		t.Fatalf("Reset on a machine with no config = %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("Reset claims to have removed %v", removed)
	}
}

// It touches the CLI's own configuration and nothing else. ~/.aws belongs to the
// AWS CLI and is read by every tool on the machine; a checkout is a git clone
// somebody made. Neither is this command's to delete, and a "reset" that took
// them would be a very expensive surprise.
func TestResetTouchesNothingItDoesNotOwn(t *testing.T) {
	home := atHome(t)

	aws := filepath.Join(home, ".aws")
	if err := os.MkdirAll(aws, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aws, "config"), []byte("[profile x]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	checkout := filepath.Join(home, "lerian", "lerian-terraform-foundation")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, _ := Load()
	cfg.TemplatesCheckout = checkout
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	if _, err := Reset(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(aws, "config")); err != nil {
		t.Errorf("~/.aws was touched: %v", err)
	}
	if _, err := os.Stat(checkout); err != nil {
		t.Errorf("the templates checkout was removed: %v", err)
	}
}

// Describe is what the machine currently holds, for somebody asking "what does
// this tool think it knows".
func TestDescribeNamesThePathAndWhatIsInIt(t *testing.T) {
	atHome(t)

	cfg, _ := Load()
	cfg.TemplatesCheckout = "/somewhere/foundation"
	cfg.CurrentProfile = "default"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	described, err := Describe()
	if err != nil {
		t.Fatal(err)
	}

	path, _ := GetConfigPath()
	for _, want := range []string{path, "/somewhere/foundation", "default"} {
		if !strings.Contains(described, want) {
			t.Errorf("the description does not mention %q:\n%s", want, described)
		}
	}
}

// On a machine with no config it says so, rather than printing an empty shape.
func TestDescribeOnAFreshMachineSaysItIsFresh(t *testing.T) {
	atHome(t)

	described, err := Describe()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(described, "nothing") {
		t.Errorf("a machine with no configuration is described as:\n%s", described)
	}
}

// A configuration too damaged to parse is the one somebody most needs to reset,
// and describing it must not be what stops them.
//
// Describe used to load the file, so invalid YAML came back as an error — and the
// reset command printed the description before removing anything, so the command
// that exists to recover from a broken config refused to run because the config
// was broken.
func TestDescribeReadsPastAFileItCannotParse(t *testing.T) {
	atHome(t)

	path, _ := GetConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("this: is: not: yaml: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	described, err := Describe()
	if err != nil {
		t.Fatalf("Describe on a damaged file = %v", err)
	}
	if !strings.Contains(described, path) {
		t.Errorf("the description does not name the file:\n%s", described)
	}
	if !strings.Contains(described, "cannot be read") {
		t.Errorf("the description does not say the file is damaged:\n%s", described)
	}
}

// And Reset removes it, because removing is exactly what it is for.
func TestResetRemovesAFileItCannotParse(t *testing.T) {
	atHome(t)

	path, _ := GetConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{{{\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := Reset()
	if err != nil {
		t.Fatalf("Reset on a damaged file = %v", err)
	}
	if len(removed) != 1 {
		t.Errorf("Reset removed %v", removed)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the damaged file survived: %v", err)
	}
}
