package infracli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// An export writes a whole tree and a git history. Landing it on top of
// something else mixes two histories and leaves no obvious way back.
func TestExportRefusesADirectoryWithAnythingInIt(t *testing.T) {
	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "something.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := refuseNonEmpty(occupied)

	if err == nil {
		t.Fatal("an export was allowed on top of existing files")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("error = %v", err)
	}
}

// A path that does not exist yet is the normal case, and the one the message
// above asks for.
func TestExportAcceptsAPathThatIsNotThere(t *testing.T) {
	if err := refuseNonEmpty(filepath.Join(t.TempDir(), "new")); err != nil {
		t.Errorf("refuseNonEmpty = %v, want it to accept a fresh path", err)
	}
}

// The repository holds what somebody set up. The twenty-seven products they
// never touched would be twenty-seven directories of someone else's decisions.
func TestOnlyRootsWithVariablesAreExported(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := infra.Discover(layout)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing configured yet.
	if units := configuredUnits(layout, catalog); len(units) != 0 {
		t.Fatalf("an unconfigured checkout exported %d roots", len(units))
	}

	// One root gets variables; only it travels.
	vpc := filepath.Join(layout.AWSDir(), "infra-base", "vpc", "envs")
	if err := os.MkdirAll(vpc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vpc, "dev.tfvars"), []byte("region = \"us-east-1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	units := configuredUnits(layout, catalog)
	if len(units) != 1 || !strings.HasSuffix(units[0].Name, "infra-base/vpc") {
		names := make([]string, 0, len(units))
		for _, unit := range units {
			names = append(names, unit.Name)
		}
		t.Errorf("exported %v, want the one root with variables", names)
	}
}

// With nothing configured there is nothing to take, and the message says what
// would make it work rather than reporting an empty copy as success.
func TestExportingAnUnconfiguredCheckoutSaysWhatIsMissing(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", checkout)
	t.Setenv("HOME", t.TempDir())

	var out bytes.Buffer
	err := ExportRepository(context.Background(), &out, filepath.Join(t.TempDir(), "repo"))

	if err == nil {
		t.Fatal("an empty export reported success")
	}
	if !strings.Contains(err.Error(), "tfvars") {
		t.Errorf("the error does not say what makes a root exportable: %v", err)
	}
}
