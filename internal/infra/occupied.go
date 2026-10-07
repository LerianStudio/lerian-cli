package infra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Occupant is what is already sitting where an export was asked to go.
//
// Read before anything is offered, because "replace it" is a decision nobody can
// make from the word "not empty". Whether that directory is a scratch copy or
// six months of somebody's work is the whole question, and git knows the answer.
type Occupant struct {
	Path string
	// Files is how many files are there, excluding .git — the count somebody
	// recognizes their own directory by.
	Files int
	// Repository marks a git repository.
	Repository bool
	// Commits is how many it has. Zero for a repository with none.
	Commits int
	// Dirty marks changes that were never committed. These are the ones no clone
	// anywhere has.
	Dirty bool
	// Unpushed marks commits with no remote, or ahead of the one they track.
	// Also the ones nothing else holds.
	Unpushed bool
}

// Empty is a directory with nothing in it, or no directory at all.
func (o Occupant) Empty() bool { return o.Files == 0 && !o.Repository }

// AtRisk is whether replacing this loses work that exists nowhere else.
//
// A repository whose commits are all pushed is recoverable from its remote; one
// with uncommitted changes or unpushed commits is not, and that is the
// difference worth putting in front of somebody about to type yes.
//
// Files with no repository around them are the plainest case of all: nothing
// anywhere holds them. Leaving that out was a real mistake — it made the
// directory with the weakest claim to safety the one that got the gentler
// question.
func (o Occupant) AtRisk() bool {
	return o.Dirty || o.Unpushed || (!o.Repository && o.Files > 0)
}

// Describe is the summary printed before the question.
func (o Occupant) Describe() string {
	if o.Empty() {
		return "empty"
	}

	parts := []string{fmt.Sprintf("%d file(s)", o.Files)}
	switch {
	case !o.Repository:
		parts = append(parts, "not a git repository — nothing here is recoverable from a remote")
	case o.Commits == 0:
		parts = append(parts, "a git repository with no commits")
	default:
		parts = append(parts, fmt.Sprintf("a git repository, %d commit(s)", o.Commits))
	}
	if o.Dirty {
		parts = append(parts, "with changes that were never committed")
	}
	if o.Unpushed {
		parts = append(parts, "and commits no remote has")
	}
	return strings.Join(parts, ", ")
}

// Inspect reads what is at a path.
//
// git is asked rather than assumed present: a machine without it still gets the
// file count, which is enough to recognize a directory by, and the repository
// questions simply go unanswered rather than taking the whole check down.
func Inspect(ctx context.Context, path string) (Occupant, error) {
	occupant := Occupant{Path: path}

	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return occupant, nil
	}
	if err != nil {
		return occupant, fmt.Errorf("infra: cannot read %s: %w", path, err)
	}

	for _, entry := range entries {
		if entry.Name() == ".git" {
			occupant.Repository = true
			continue
		}
		count, countErr := countFiles(filepath.Join(path, entry.Name()))
		if countErr != nil {
			return occupant, countErr
		}
		occupant.Files += count
	}

	if occupant.Repository {
		readRepository(ctx, &occupant)
	}
	return occupant, nil
}

// countFiles is one entry's share of the count: itself, or everything under it.
func countFiles(path string) (int, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, fmt.Errorf("infra: cannot read %s: %w", path, err)
	}
	if !info.IsDir() {
		return 1, nil
	}

	count := 0
	err = filepath.WalkDir(path, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// readRepository fills in what git knows. Every question is best-effort: this is
// a report, and a git that will not answer one of them is not a reason to refuse
// to describe the directory at all.
func readRepository(ctx context.Context, occupant *Occupant) {
	git, err := NewGitCLI()
	if err != nil {
		return
	}

	if out, err := git.runRaw(ctx, occupant.Path, "rev-list", "--count", "HEAD"); err == nil {
		occupant.Commits = atoi(strings.TrimSpace(out))
	}
	if out, err := git.runRaw(ctx, occupant.Path, "status", "--porcelain"); err == nil {
		occupant.Dirty = strings.TrimSpace(out) != ""
	}
	if occupant.Commits == 0 {
		return
	}

	// Unpushed is read as "not reachable from any remote". A repository with no
	// remote at all answers this with every commit, which is right: there is
	// nowhere else holding them.
	out, err := git.runRaw(ctx, occupant.Path, "rev-list", "--count", "--all", "--not", "--remotes")
	if err == nil {
		occupant.Unpushed = atoi(strings.TrimSpace(out)) > 0
	}
}

// atoi is strconv.Atoi without the error, because every caller here treats an
// unreadable count as zero.
func atoi(value string) int {
	count := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0
		}
		count = count*10 + int(char-'0')
	}
	return count
}

// ErrProtectedPath is a path this will not empty whatever anybody answers.
var ErrProtectedPath = errors.New("infra: refusing to empty that directory")

// RefuseToEmpty names the directories no confirmation can authorize removing.
//
// A confirmation is consent to lose what was described, and these are places
// where what would be lost is not what was described: a home directory holds
// everything, a templates checkout is shared with every other run, and a
// directory containing the shell's own working directory is the floor being
// stood on. None of them is a plausible answer to "where should the export go",
// so refusing costs nobody anything.
func RefuseToEmpty(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("infra: cannot resolve %q: %w", path, err)
	}
	clean := filepath.Clean(absolute)

	if parent := filepath.Dir(clean); parent == clean {
		return fmt.Errorf("%w: %s is the root of a filesystem", ErrProtectedPath, clean)
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == clean {
		return fmt.Errorf("%w: %s is your home directory", ErrProtectedPath, clean)
	}
	if IsCheckout(clean) {
		return fmt.Errorf("%w: %s is a lerian-terraform-foundation checkout,\n"+
			"which other runs read", ErrProtectedPath, clean)
	}
	if working, err := os.Getwd(); err == nil && within(clean, working) {
		return fmt.Errorf("%w: %s holds the directory this command is running in", ErrProtectedPath, clean)
	}
	return nil
}

// within is whether child is inside parent, or is parent.
func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// EmptyDirectory removes everything inside a directory, leaving the directory.
//
// The contents rather than the directory itself: the path may be a mount point,
// or have permissions somebody set, and recreating it is not the same thing as
// having left it alone.
func EmptyDirectory(path string) error {
	if err := RefuseToEmpty(path); err != nil {
		return err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("infra: cannot read %s: %w", path, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return fmt.Errorf("infra: cannot remove %s: %w", entry.Name(), err)
		}
	}
	return nil
}
