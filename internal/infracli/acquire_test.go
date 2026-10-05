package infracli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Newest first, because that is the one somebody picking from a list wants, and
// the cursor starts on the first row.
func TestTheNewestTemplatesTagIsOfferedFirst(t *testing.T) {
	choices := refChoices([]string{"v1.9.0", "v1.11.0", "v1.10.0"})

	if len(choices) != 3 {
		t.Fatalf("offered %d tags, want 3", len(choices))
	}
	if choices[0].Value != "v1.11.0" {
		t.Errorf("first row is %q, want v1.11.0", choices[0].Value)
	}
	if choices[0].Note != "newest" {
		t.Errorf("the newest row is not named as such: %q", choices[0].Note)
	}
}

// A tag below the floor is refused by this binary two steps later. Offering a
// row somebody can pick and then be told no about is worse than not offering it.
func TestTagsThisBinaryCannotReadAreNotOffered(t *testing.T) {
	below := "v1.2.0"
	if !infra.RefBelowMin(below) {
		t.Fatalf("%s is not below the floor %s; pick another for this test", below, infra.TemplatesMinRef)
	}

	for _, choice := range refChoices([]string{below, "v1.11.0"}) {
		if choice.Value == below {
			t.Errorf("%s is offered and would be refused after the clone", below)
		}
	}
}

// Prereleases are not releases. Somebody choosing which templates to run should
// not have to know that -rc1 means "not yet".
func TestPrereleaseTagsAreNotOffered(t *testing.T) {
	for _, choice := range refChoices([]string{"v1.11.0", "v1.12.0-rc1"}) {
		if strings.Contains(choice.Value, "-") {
			t.Errorf("a prerelease is offered: %q", choice.Value)
		}
	}
}

// With nothing usable there is no menu to draw, and the caller falls back to the
// error that explains why.
func TestNoUsableTagMeansNoMenu(t *testing.T) {
	if got := refChoices([]string{"v1.0.0", "v1.1.0"}); got != nil {
		t.Errorf("refChoices = %v, want none", got)
	}
}

// The managed path is found by convention on every run. Recording it would add
// nothing, and would make `config reset` the thing that loses a checkout that is
// still sitting there.
func TestTheManagedDestinationIsNotRecorded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	managed, err := infra.ManagedCheckoutPath("")
	if err != nil {
		t.Fatal(err)
	}
	if recordAfterClone(managed) {
		t.Error("the managed path would be written into the config")
	}
}

// Anywhere else is found by nothing at all, so it has to be written down or the
// next run will not see it.
func TestAChosenDestinationIsRecorded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if !recordAfterClone(filepath.Join(t.TempDir(), "my-templates")) {
		t.Error("a directory of somebody's own choosing would be forgotten")
	}
}

// A menu row is one line wide, and the home prefix is the part nobody needs to
// read.
func TestHomePathsAreShortened(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	inside := filepath.Join(home, ".lerian", "lerian-terraform-foundation")
	if got := tilde(inside); got != filepath.Join("~", ".lerian", "lerian-terraform-foundation") {
		t.Errorf("tilde(%q) = %q", inside, got)
	}

	outside := filepath.Join(os.TempDir(), "elsewhere")
	if got := tilde(outside); got != outside {
		t.Errorf("tilde(%q) = %q, want it unchanged", outside, got)
	}

	// Not a prefix match on the string: a sibling directory whose name merely
	// starts with the home path is not inside it.
	sibling := home + "-backup"
	if got := tilde(sibling); got != sibling {
		t.Errorf("tilde(%q) = %q, want it unchanged", sibling, got)
	}
}
