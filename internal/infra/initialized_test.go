package infra

import (
	"os"
	"path/filepath"
	"testing"
)

// This CLI passes -reconfigure on every init, so a run is pointed where it says.
// What outlives the run is the directory: it keeps the last environment's
// backend, and a terraform typed by hand in there reads that one.
func TestTheDirectoryRemembersWhichBackendItWasInitializedFor(t *testing.T) {
	unit := initializedUnit(t, `{"backend":{"config":{"bucket":"example-tfstate-stg-111122223333"}}}`)

	bucket, ok := InitializedFor(unit)

	if !ok {
		t.Fatal("an initialized directory reported nothing")
	}
	if bucket != "example-tfstate-stg-111122223333" {
		t.Errorf("bucket = %q", bucket)
	}
}

// A fresh clone has nothing to say, and reporting an empty answer as a fact
// would put a blank line in a page that exists to be read.
func TestADirectoryThatWasNeverInitializedSaysNothing(t *testing.T) {
	unit := Unit{Name: "fresh", Dir: t.TempDir()}

	if _, ok := InitializedFor(unit); ok {
		t.Error("a directory with no .terraform reported a backend")
	}
}

// Local state — what bootstrap uses — has no bucket, and inventing one would
// name a backend that does not exist.
func TestLocalStateReportsNoBucket(t *testing.T) {
	unit := initializedUnit(t, `{"backend":{"type":"local","config":{}}}`)

	if _, ok := InitializedFor(unit); ok {
		t.Error("a local backend was reported as a bucket")
	}
}

// A file that cannot be parsed is not a backend either.
func TestAnUnreadableTerraformStateSaysNothing(t *testing.T) {
	unit := initializedUnit(t, `{not json`)

	if _, ok := InitializedFor(unit); ok {
		t.Error("an unparsable .terraform/terraform.tfstate reported a backend")
	}
}

func initializedUnit(t *testing.T, body string) Unit {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".terraform", "terraform.tfstate"),
		[]byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return Unit{Name: "sample", Dir: dir}
}
