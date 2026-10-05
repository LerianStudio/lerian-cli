package infracli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// TemplatesOnDisk is a templates checkout this tool knows the location of, along
// with what somebody needs in order to decide whether deleting it is safe.
//
// Managed and Recorded are the difference between a directory this tool made and
// one somebody chose. Dirty is the part that cannot be reconstructed: a clone can
// be made again from the remote, and work that was never committed cannot.
type TemplatesOnDisk struct {
	Path     string
	Managed  bool
	Recorded bool
	Dirty    []string
	Size     int64
}

// TemplatesFound lists the checkouts this tool would use, newest decision first:
// the recorded one, then the managed path.
//
// Only these two. The working directory is not included — `config reset` is run
// from wherever somebody happens to be standing, and a reset that deletes the
// repository you are sitting in because you were sitting in it is not a reset.
func TemplatesFound(ctx context.Context) []TemplatesOnDisk {
	recorded := recordedCheckout()
	// Whose directory it is follows from where it sits, not from which lookup
	// found it: a recorded path very often IS the managed one, and reporting that
	// as "you cloned this one" tells somebody deciding whether to delete it the
	// opposite of the truth.
	managedPaths := infra.ManagedCheckoutPaths("")
	isManaged := func(path string) bool { return slices.Contains(managedPaths, path) }

	var found []TemplatesOnDisk
	seen := map[string]bool{}

	add := func(path string) {
		if path == "" || seen[path] || !infra.IsCheckout(path) {
			return
		}
		seen[path] = true
		found = append(found, TemplatesOnDisk{
			Path:     path,
			Managed:  isManaged(path),
			Recorded: path == recorded,
			Dirty:    uncommitted(ctx, path),
			Size:     totalSize([]string{path}),
		})
	}

	add(recorded)
	for _, managed := range managedPaths {
		add(managed)
	}
	return found
}

// uncommitted is the work that deleting the directory would destroy for good.
//
// No git, or a directory git will not answer about, reports nothing rather than
// failing: the question here is "is there anything to lose", and an unanswerable
// one must not stop somebody from being asked whether to delete.
func uncommitted(ctx context.Context, path string) []string {
	git, err := infra.NewGitCLI()
	if err != nil {
		return nil
	}
	dirty, err := git.DirtyTracked(ctx, path)
	if err != nil {
		return nil
	}
	return dirty
}

// RemoveTemplates deletes a checkout, after making sure it is one.
//
// The check is the guard, not a formality. This is the one place in the CLI that
// removes a directory it did not necessarily create, and it is reached from a
// path that was typed or recorded months ago; a config holding a mistyped or
// since-reused path would otherwise turn "reset" into a recursive delete of
// whatever is there now.
func RemoveTemplates(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %w", path, err)
	}
	if !infra.IsCheckout(absolute) {
		return fmt.Errorf("refusing to remove %s: it is not a lerian-terraform-foundation checkout", absolute)
	}
	if err := os.RemoveAll(absolute); err != nil {
		return fmt.Errorf("cannot remove %s: %w", absolute, err)
	}
	return nil
}

// Describe says what the directory is and what is in it, in the order somebody
// deciding would ask: whose it is, whether anything would be lost, how big it is.
func (t TemplatesOnDisk) Describe() string {
	origin := "you cloned this one"
	if t.Managed {
		origin = "cloned by this tool"
	}

	loss := "nothing uncommitted"
	switch len(t.Dirty) {
	case 0:
	case 1:
		loss = "1 file changed and not committed"
	default:
		loss = fmt.Sprintf("%d files changed and not committed", len(t.Dirty))
	}

	return fmt.Sprintf("%s · %s · %s", origin, loss, humanSize(t.Size))
}
