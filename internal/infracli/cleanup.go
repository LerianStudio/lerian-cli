package infracli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

	report(stdout, found)
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

	if remembered := rememberedCheckout(); remembered != "" {
		found = append(found, leftover{
			name:       "remembered",
			summary:    "the checkout path in the config: " + remembered,
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

func runLogDirs() []string {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "lerian-infra-*"))
	if err != nil {
		return nil
	}
	return matches
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
	named := map[string]bool{"plugins": plugins, "logs": logs, "remembered": remembered}

	if all || plugins || logs || remembered {
		var chosen []leftover
		for _, item := range found {
			if all || named[item.name] {
				chosen = append(chosen, item)
			}
		}
		return chosen, nil
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
