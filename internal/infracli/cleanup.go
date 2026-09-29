package infracli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

const cleanupUsage = `lerian infra cleanup — remove what this tool left on this machine

Usage:
  lerian infra cleanup [flags]

Removes local leftovers: cached provider plugins, run logs, and the note of
where the templates are. It touches no AWS resource and no Terraform state —
state lives in S3, and nothing here can reach it. Destroying infrastructure is
'--action destroy', and it is a different command on purpose.

With no flags it lists what it found, with sizes, and asks which to remove.
Outside a terminal, name the groups instead.

Flags:
  --plugins     the .terraform provider caches inside the checkout
  --logs        the run log directories under the system temp directory
  --remembered  the checkout path recorded in the config
  --all         all of the above
  --dry-run     list what would go, remove nothing
  -h, --help    this message
`

// leftover is one group the cleanup can remove. Size is bytes, counted so the
// operator can tell the 2 GB of provider plugins from the handful of log files
// before choosing.
type leftover struct {
	name    string
	summary string
	paths   []string
	size    int64
	// configOnly marks the group that clears a config entry rather than files.
	configOnly bool
}

func runCleanup(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var opts struct {
		plugins, logs, remembered, all, dryRun bool
	}

	flags := flag.NewFlagSet("lerian infra cleanup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, cleanupUsage) }
	flags.BoolVar(&opts.plugins, "plugins", false, "the .terraform provider caches")
	flags.BoolVar(&opts.logs, "logs", false, "the run log directories")
	flags.BoolVar(&opts.remembered, "remembered", false, "the checkout path in the config")
	flags.BoolVar(&opts.all, "all", false, "all of the above")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "list what would go, remove nothing")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if rest := flags.Args(); len(rest) > 0 {
		return fmt.Errorf("unexpected argument %q\nRun lerian infra cleanup --help", rest[0])
	}

	found := findLeftovers(ctx)
	if len(found) == 0 {
		fmt.Fprintf(stdout, "\n  Nothing to clean up.\n")
		return nil
	}

	// What the flags name, so a dry run describes the removal about to happen
	// rather than everything that happens to be on the machine.
	shown := found
	if opts.all || opts.plugins || opts.logs || opts.remembered {
		shown = namedGroups(found, opts.plugins, opts.logs, opts.remembered, opts.all)
	}

	report(stdout, shown)
	if opts.dryRun {
		fmt.Fprintf(stdout, "\n  dry run — nothing was removed.\n")
		return nil
	}

	chosen, err := chooseLeftovers(found, opts.plugins, opts.logs, opts.remembered, opts.all, stderr)
	if err != nil {
		if errors.Is(err, infra.ErrAborted) {
			return nil
		}
		return err
	}
	if len(chosen) == 0 {
		fmt.Fprintf(stdout, "\n  Nothing chosen.\n")
		return nil
	}

	return remove(stdout, chosen)
}

// findLeftovers looks only where this tool writes. Everything it can propose is
// reproducible: plugins come back with terraform init, logs are written by the
// next run, and the remembered path is asked for again.
func findLeftovers(_ context.Context) []leftover {
	var found []leftover

	if layout, _, err := resolveLayout("", os.Getenv("LERIAN_TF_REPO"), ""); err == nil {
		if caches := providerCaches(layout.Root); len(caches) > 0 {
			found = append(found, leftover{
				name:    "plugins",
				summary: "provider caches under the checkout, restored by terraform init",
				paths:   caches,
				size:    totalSize(caches),
			})
		}
	}

	if logs := runLogDirs(); len(logs) > 0 {
		found = append(found, leftover{
			name:    "logs",
			summary: "run logs in the system temp directory",
			paths:   logs,
			size:    totalSize(logs),
		})
	}

	// Read as recorded, not as validated. A path whose clone was moved or deleted
	// is the one most worth forgetting, and rememberedCheckout hides exactly that
	// one: it returns empty for a path that is no longer a checkout, the group
	// vanishes from the list, and --remembered has nothing left to clear.
	if recorded := recordedCheckout(); recorded != "" {
		summary := "the checkout path in the config: " + recorded
		if !infra.IsCheckout(recorded) {
			summary += " (no longer a checkout)"
		}
		found = append(found, leftover{
			name:       "remembered",
			summary:    summary,
			configOnly: true,
		})
	}

	return found
}

// providerCaches finds the .terraform directories terraform init fills. They are
// the bulk of what this command exists for — hundreds of megabytes per stack,
// and every one of them re-downloadable.
func providerCaches(root string) []string {
	var caches []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		//nolint:nilerr // A directory this cannot read is one it cannot propose;
		// stopping the walk over it would hide every cache below it instead.
		if err != nil {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".terraform" {
			caches = append(caches, path)
			return filepath.SkipDir
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		return nil
	})
	return caches
}

// runOwnerFile names the process that created a run directory. A run writes its
// plans and its log there, and the log is what somebody reads when that run
// fails — so a directory another lerian infra is still using is not one to
// offer for removal.
const runOwnerFile = ".owner-pid"

// runDirCreated is called with the staging directory the moment it exists, before
// it carries a marker or its published name. It is a variable so a test can look
// inside that window instead of trying to race it.
var runDirCreated = func(string) {}

// newRunDir creates the directory a run writes its plans and log into, and does
// not give it a name the cleanup looks for until it carries its marker.
//
// Creating it as lerian-infra-* and writing the marker afterwards leaves a window
// — short, but a window — in which a concurrent cleanup sees a matching directory
// with no marker, reads it as a leftover from a version that did not write one,
// and removes the plans and log of a run that is just starting. Staged under a
// name the glob does not match and renamed once claimed, the directory is never
// visible to the cleanup in an unclaimed state.
//
// The leading dot is what keeps it out: the glob is lerian-infra-*, anchored at
// the start of the name.
func newRunDir() (string, error) {
	staging, err := os.MkdirTemp("", ".lerian-infra-")
	if err != nil {
		return "", fmt.Errorf("cannot create the run directory: %w", err)
	}
	runDirCreated(staging)

	//nolint:gosec // G302 is written for files; a directory without the execute bit
	// cannot be traversed, so 0700 is already the tightest usable mode here.
	if err := os.Chmod(staging, 0o700); err != nil {
		return "", fmt.Errorf("cannot restrict the run directory: %w", err)
	}
	if err := claimRunDir(staging); err != nil {
		return "", err
	}

	// The published name is the staged one without the dot, so it inherits the
	// uniqueness MkdirTemp already established.
	published := filepath.Join(filepath.Dir(staging), strings.TrimPrefix(filepath.Base(staging), "."))
	if err := os.Rename(staging, published); err != nil {
		return "", fmt.Errorf("cannot publish the run directory: %w", err)
	}
	return published, nil
}

// claimRunDir records this process as the owner of a run directory.
//
// Its failure is the run's failure. A directory with no marker reads as finished,
// so a run that could not claim its own is a run whose plans and log a concurrent
// cleanup is free to delete while it is still writing them.
func claimRunDir(dir string) error {
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(filepath.Join(dir, runOwnerFile), []byte(pid), 0o600); err != nil {
		return fmt.Errorf("cannot claim the run directory: %w", err)
	}
	return nil
}

func runLogDirs() []string {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "lerian-infra-*"))
	if err != nil {
		return nil
	}

	finished := make([]string, 0, len(matches))
	for _, dir := range matches {
		if !runIsOver(dir) {
			continue
		}
		finished = append(finished, dir)
	}
	return finished
}

// runIsOver reports whether the process that claimed this directory is gone.
//
// A directory with no marker at all counts as finished: it was left by a version
// that did not write one, and those are the oldest leftovers there are. Anything
// else uncertain counts as running, because the cost of being wrong is asymmetric —
// keeping a directory wastes disk, deleting one takes a running command's log.
func runIsOver(dir string) bool {
	// #nosec G304 -- the path is this package's own constant joined to a directory
	// the glob above found in os.TempDir(). Nothing outside this process chooses
	// it, and the content is parsed as an integer and discarded — nothing read here
	// is executed, echoed or returned.
	//
	// This spelling and not the one used elsewhere in the package: gosec reads
	// #nosec whether it runs standalone for code scanning or inside golangci-lint,
	// so it covers both. The suppression the rest of this repo uses covers only the
	// second, which is why an alert kept appearing on every push.
	recorded, err := os.ReadFile(filepath.Join(dir, runOwnerFile))
	if err != nil {
		// Missing is the only error that means unclaimed. On a shared /tmp another
		// user's run directory is mode 0700, and reading through it fails with a
		// permission error — which says nothing about whether that run is over.
		return errors.Is(err, fs.ErrNotExist)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(recorded)))
	if err != nil || pid <= 0 {
		// A marker that cannot be read as a pid is not evidence that the run ended.
		// The rule above holds here too: uncertainty counts as running.
		return false
	}
	if pid == os.Getpid() {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	// Signal 0 asks about the process without disturbing it. ErrProcessDone is
	// the only answer that means gone; every other error leaves the question open,
	// and an open question is not grounds for deleting a log.
	err = process.Signal(syscall.Signal(0))
	return errors.Is(err, os.ErrProcessDone)
}

func totalSize(paths []string) int64 {
	var total int64
	for _, path := range paths {
		_ = filepath.WalkDir(path, func(p string, entry os.DirEntry, err error) error {
			//nolint:nilerr // An unreadable entry contributes nothing to the size
			// and is not worth abandoning the count for.
			if err != nil || entry.IsDir() {
				return nil
			}
			if info, statErr := entry.Info(); statErr == nil {
				total += info.Size()
			}
			return nil
		})
	}
	return total
}

func report(out io.Writer, found []leftover) {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> Local leftovers"))

	width := 0
	for _, item := range found {
		if len(item.name) > width {
			width = len(item.name)
		}
	}
	for _, item := range found {
		size := ""
		if !item.configOnly {
			size = fmt.Sprintf("%-8s", humanSize(item.size))
		} else {
			size = fmt.Sprintf("%-8s", "-")
		}
		fmt.Fprintf(out, "  %-*s  %s%s\n", width, item.name, size, theme.dim(item.summary))
	}
	fmt.Fprintf(out, "\n  %s\n", theme.dim("No AWS resource and no Terraform state is touched by any of these."))
}

// chooseLeftovers takes the groups from flags, or asks when none were named and
// there is a terminal. Outside one, naming nothing is an error rather than a
// guess: removing more than was asked for is not a default worth having.
func chooseLeftovers(
	found []leftover,
	plugins, logs, remembered, all bool,
	stderr io.Writer,
) ([]leftover, error) {
	if all || plugins || logs || remembered {
		return namedGroups(found, plugins, logs, remembered, all), nil
	}

	ask := newPrompter(stderr)
	if !ask.interactive {
		return nil, errors.New("name what to remove: --plugins, --logs, --remembered or --all\n" +
			"There is no terminal to ask, and removing everything by default is not a\n" +
			"decision this makes for you.")
	}

	options := make([]option, 0, len(found))
	for _, item := range found {
		note := item.summary
		if !item.configOnly {
			note = humanSize(item.size) + "  " + note
		}
		options = append(options, option{value: item.name, label: item.name, note: note})
	}

	values, err := ask.pickMany("What should be removed?",
		"Everything here comes back: plugins on the next terraform init, "+
			"logs on the next run, the path by asking again.",
		"--all", options, nil)
	if err != nil {
		return nil, err
	}

	picked := map[string]bool{}
	for _, value := range values {
		picked[value] = true
	}
	var chosen []leftover
	for _, item := range found {
		if picked[item.name] {
			chosen = append(chosen, item)
		}
	}
	return chosen, nil
}

// namedGroups is the found groups the flags select, in the order they were
// found.
func namedGroups(found []leftover, plugins, logs, remembered, all bool) []leftover {
	named := map[string]bool{"plugins": plugins, "logs": logs, "remembered": remembered}

	chosen := make([]leftover, 0, len(found))
	for _, item := range found {
		if all || named[item.name] {
			chosen = append(chosen, item)
		}
	}
	return chosen
}

func remove(out io.Writer, chosen []leftover) error {
	var freed int64
	for _, item := range chosen {
		if item.configOnly {
			if err := forgetCheckout(); err != nil {
				return fmt.Errorf("clearing the remembered checkout: %w", err)
			}
			fmt.Fprintf(out, "  forgot the remembered checkout\n")
			continue
		}
		for _, path := range item.paths {
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("removing %s: %w", path, err)
			}
		}
		freed += item.size
		fmt.Fprintf(out, "  removed %s (%s)\n", item.name, humanSize(item.size))
	}
	if freed > 0 {
		fmt.Fprintf(out, "\n  Freed %s.\n", humanSize(freed))
	}
	return nil
}

// forgetCheckout clears the recorded path and leaves the clone alone. The
// directory is the operator's, cloned by them or by --clone, and a command about
// local caches has no business deleting a git repository.
func forgetCheckout() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.TemplatesCheckout = ""
	return cfg.Save()
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
