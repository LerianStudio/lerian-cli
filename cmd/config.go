// Package cmd — the config command.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
)

var (
	configResetYes       bool
	configResetTemplates bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or reset what this tool remembers",
	Long: `What the CLI has written down about this machine, and how to clear it.

The configuration is one file, ~/.lerian/config.yaml. It holds where the
templates checkout is and the profiles 'lerian auth login' creates — nothing
else, and nothing belonging to another tool.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		return showConfig(cmd.OutOrStdout())
	},
	SilenceUsage: true,
}

var configTemplatesCmd = &cobra.Command{
	Use:   "templates <path>",
	Short: "Record which lerian-terraform-foundation checkout to use",
	Long: `Writes the path into ~/.lerian/config.yaml, so every later run finds it
without a flag or a variable.

It beats the managed path at ~/.lerian/lerian-terraform-foundation — one is a
decision, the other a directory that happens to exist somewhere conventional —
and loses to standing inside a checkout, which is the one you are looking at.

  lerian config templates /path/to/lerian-terraform-foundation
  lerian config templates --clear`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runSetTemplates,
	SilenceUsage: true,
}

var configTemplatesClear bool

func runSetTemplates(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	if configTemplatesClear {
		return clearTemplates(out)
	}
	if len(args) == 0 {
		// Reached off the menu, where there is no command line to put a path on.
		// The usage text of a command nobody typed is not an answer to that.
		if infracli.CanAsk(out) {
			return askThenRecord(out, infracli.AskForTemplates)
		}
		return fmt.Errorf("give the path to a lerian-terraform-foundation checkout\n" +
			"  lerian config templates /path/to/lerian-terraform-foundation\n" +
			"  lerian config templates --clear")
	}
	return setTemplates(out, args[0])
}

// askThenRecord asks and then does what the answer says.
//
// Leaving the prompt is not a failure: r and q mean "not this", and the session
// is about to redraw the menu that was left. Reporting an error there would put
// a red line under a decision not to decide.
// The prompt arrives as an argument so a test can answer it. A test cannot
// reach the real one: it needs a terminal on both ends, and under go test there
// is none — which is also why the CanAsk line above has no test of its own.
func askThenRecord(out io.Writer, ask func(io.Writer) (infracli.TemplatesAnswer, error)) error {
	answer, err := ask(out)
	switch {
	case errors.Is(err, infracli.ErrBack), errors.Is(err, infra.ErrAborted):
		return nil
	case err != nil:
		return err
	case answer.Forget:
		return clearTemplates(out)
	}
	return setTemplates(out, answer.Path)
}

// setTemplates records a checkout, after checking it is one.
//
// Checked here rather than at the next run: a path accepted now and rejected
// later fails naming a missing file, several steps from the moment somebody could
// have fixed the typo.
func setTemplates(out io.Writer, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %w", path, err)
	}
	if !infra.IsCheckout(absolute) {
		return fmt.Errorf("no lerian-terraform-foundation checkout at %s\n"+
			"A checkout is recognized by the directories examples/aws/_modules and\n"+
			"examples/aws/backend; at least one of them is missing there", absolute)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.TemplatesCheckout = absolute
	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(out, "\n  templates  %s\n", absolute)
	fmt.Fprintf(out, "  Recorded. Later runs use it without a flag.\n\n")
	return nil
}

// clearTemplates forgets the path and leaves the directory alone: it is a clone
// somebody made, possibly with work in it.
func clearTemplates(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.TemplatesCheckout == "" {
		fmt.Fprintf(out, "\n  Nothing was recorded.\n\n")
		return nil
	}

	previous := cfg.TemplatesCheckout
	cfg.TemplatesCheckout = ""
	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(out, "\n  Forgot %s\n", previous)
	fmt.Fprintf(out, "  The directory is untouched. Later runs discover a checkout again.\n\n")
	return nil
}

var configKubeconfigCmd = &cobra.Command{
	Use:   "kubeconfig",
	Short: "Point kubectl at a cluster that already exists",
	Long: `Asks which account and which cluster, then runs aws eks update-kubeconfig.

The post-run menu offers this after an apply, where the CLI knows the cluster
because it just made it. This is for the rest of the time: a cluster somebody
else created, or one from a run long finished, in an account this checkout may
know nothing about.

It asks before overwriting an entry that points somewhere else — usually the
same cluster destroyed and recreated, which is the case worth fixing, but also
possibly a different cluster of the same name in another account.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := infracli.PointKubectlAtACluster(cmd.Context(), cmd.OutOrStdout(), infra.CLIClusters{})
		// Leaving is not a failure: the session redraws the menu that was left.
		if errors.Is(err, infracli.ErrBack) || errors.Is(err, infra.ErrAborted) {
			return nil
		}
		return err
	},
	SilenceUsage: true,
}

var configRepoCmd = &cobra.Command{
	Use:   "repo <path>",
	Short: "Copy what you configured into a repository of its own",
	Long: `Writes the roots you configured, the modules they use and the configuration
that makes them runnable into a new directory, and makes it a git repository
with one commit.

It is a copy, not a link. The templates are another repository on another
release cycle; this one is yours to edit, and nothing reaches back.

It stops before the remote: 'git remote add origin <url>' and 'git push' are
yours, because where your infrastructure gets published is not this tool's
guess to make.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return infracli.ExportRepository(cmd.Context(), cmd.OutOrStdout(), args[0])
	},
	SilenceUsage: true,
}

var configGitHubCmd = &cobra.Command{
	Use:   "github",
	Short: "Sign in to GitHub and set how repositories are created",
	Long: `Who gh is signed in as on this machine, and how to change it.

'lerian config repo' offers to create the exported repository on GitHub, and
will sign you in on the way past. This is for the questions that come up
afterwards: signed in as the wrong account, needing a second one for an
organization, or taking the credential off a machine being handed on.

It changes gh, not this tool. The credential lives in the system keyring and
every gh on the machine reads it — signing out here signs out the one in your
other terminal too.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := infracli.ConfigureGitHub(cmd.Context(), cmd.OutOrStdout())
		// Leaving is not a failure: the session redraws the menu that was left.
		if errors.Is(err, infracli.ErrBack) || errors.Is(err, infra.ErrAborted) {
			return nil
		}
		return err
	},
	SilenceUsage: true,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the configuration and where it lives",
	// First in the submenu: it is the one that only reads, and the cursor starts
	// on the first row.
	Annotations: map[string]string{menuAnnotation: menuFirst},
	// Nothing here takes an argument, and without this cobra accepts and ignores
	// them — so `lerian config reset production`, which reads like "reset the
	// production profile", would quietly remove everything instead.
	Args:         cobra.NoArgs,
	RunE:         func(cmd *cobra.Command, _ []string) error { return showConfig(cmd.OutOrStdout()) },
	SilenceUsage: true,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Forget everything, as if the CLI had never run here",
	// Last in the submenu: the cursor starts on the first row, and this is the one
	// that removes things.
	Annotations: map[string]string{menuAnnotation: menuLast},
	Long: `Removes ~/.lerian/config.yaml, so the next run asks what it asked the first
time: where the templates are, which account to deploy into.

It then offers to delete the templates checkouts on this machine — one question
per directory, answered separately from the one above, because forgetting a path
is undone by the next run and deleting a git clone is not. Declining is the
default, and --delete-templates deletes without asking, for a script that means
it.

~/.aws is never touched. It belongs to the AWS CLI and every tool on this
machine reads it.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return resetConfig(cmd.Context(), cmd.OutOrStdout())
	},
	SilenceUsage: true,
}

// noteManagedCheckout says what reset will not change.
//
// reset forgets what the config records. A checkout sitting in the managed path
// is not recorded — it is found by convention, which is the whole point of that
// path — so the next run keeps using it. Somebody who has just been told the tool
// forgot everything, and then watches it carry on with a checkout, is owed the
// sentence that explains why.
// deleteTemplatesFound offers to remove the checkouts on disk, one at a time,
// and reports how many are still there afterwards.
//
// A question of its own, after the configuration has already gone. Folding it
// into the first one would make a single "yes" mean both "forget what you know"
// — which the next run simply asks again — and "delete a git clone", which no
// run can undo. They are not the same decision and must not share an answer.
//
// The default row is the one that deletes nothing.
func deleteTemplatesFound(out io.Writer, found []infracli.TemplatesOnDisk) int {
	staying := 0
	for _, checkout := range found {
		if !configResetTemplates && !infracli.CanAsk(out) {
			// Nobody to ask, and no flag saying to go ahead. Said rather than passed
			// over in silence: "reset" was just reported as done, and a directory
			// this command mentioned by name must not be left in an unstated state.
			fmt.Fprintf(out, "  kept %s — pass --delete-templates to remove it\n", checkout.Path)
			staying++
			continue
		}

		if !configResetTemplates {
			fmt.Fprintf(out, "  %s\n  %s\n", checkout.Path, checkout.Describe())
			fmt.Fprintf(out, "  Everything in it goes, committed or not, and no later run can bring it back.\n\n")
			// Short enough to survive the width the purpose line is cut at. The
			// sentence above carries the detail, where nothing truncates it.
			answer, err := infracli.Choose(out, "Delete this templates checkout?",
				"Deletes the directory and everything in it. This cannot be undone.",
				[]infracli.Choice{
					{Value: "no", Label: "keep the directory", Note: "nothing is deleted"},
					{Value: "yes", Label: "delete it", Note: "the directory and all its contents"},
				})
			if err != nil || answer != "yes" {
				fmt.Fprintf(out, "  kept %s\n", checkout.Path)
				staying++
				continue
			}
		}

		if err := infracli.RemoveTemplates(checkout.Path); err != nil {
			// Said and carried on. The configuration is already gone, the other
			// checkout may still be removable, and the operator can delete this one
			// themselves — there is nothing here worth abandoning the command for.
			fmt.Fprintf(out, "  could not delete %s: %v\n", checkout.Path, err)
			staying++
			continue
		}
		fmt.Fprintf(out, "  deleted %s\n", checkout.Path)
	}
	return staying
}

func showConfig(out io.Writer) error {
	infracli.DescribeMachine(context.Background(), out)
	return nil
}

// resetConfig asks before removing, because "as if it had never run" is not
// something to do to somebody by accident. --yes is the way to mean it in a
// script, and outside a terminal there is nobody to ask.
//
//nolint:nilerr // see the comment on the declining branch below
func resetConfig(ctx context.Context, out io.Writer) error {
	described, err := config.Describe()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%s\n", described)

	// Before the file goes: the recorded path is in it, and afterwards there is
	// nothing left saying where that checkout was.
	found := infracli.TemplatesFound(ctx)
	noteTemplatesFound(out, found)

	if !configResetYes {
		if !infracli.CanAsk(out) {
			return fmt.Errorf("this removes the configuration above and there is no terminal to ask\n" +
				"Pass --yes if that is what you want")
		}
		answer, err := infracli.Choose(out, "Forget this configuration?",
			"The next run asks what it asked the first time. No directory is removed by this answer.",
			[]infracli.Choice{
				{Value: "no", Label: "keep it", Note: "changes nothing"},
				{Value: "yes", Label: "forget it", Note: "removes ~/.lerian/config.yaml"},
			})
		// Declining is not a failure, and neither is a selector that could not draw:
		// either way nothing was removed, which is the safe outcome and the one the
		// operator can see. Reporting "the prompt failed" instead would be alarming
		// about a file that is still exactly where it was.
		if err != nil || answer != "yes" {
			fmt.Fprintf(out, "  kept.\n\n")
			return nil
		}
	}

	removed, err := config.Reset()
	if err != nil {
		return err
	}
	switch {
	case len(removed) == 0 && len(found) == 0:
		fmt.Fprintf(out, "  nothing to forget — there was no configuration.\n\n")
		return nil
	case len(removed) == 0:
		fmt.Fprintf(out, "  there was no configuration to forget.\n")
	}
	for _, path := range removed {
		fmt.Fprintf(out, "  removed %s\n", path)
	}

	staying := deleteTemplatesFound(out, found)

	// Not "from nothing" when something is about to be picked up again — that
	// sentence would sit a few lines under the note saying otherwise.
	if staying > 0 {
		fmt.Fprintf(out, "\n  The next run asks again, and finds the checkout that stayed.\n\n")
		return nil
	}
	fmt.Fprintf(out, "\n  The next run starts from nothing.\n\n")
	return nil
}

// noteTemplatesFound says, before anything is decided, which directories are
// about to be asked about. Somebody deciding whether to reset at all should not
// learn only afterwards that a clone of theirs was in scope.
func noteTemplatesFound(out io.Writer, found []infracli.TemplatesOnDisk) {
	if len(found) == 0 {
		return
	}
	fmt.Fprintf(out, "  %d templates checkout(s) on this machine. Each is asked about separately\n", len(found))
	fmt.Fprintf(out, "  after the configuration is forgotten:\n")
	for _, checkout := range found {
		fmt.Fprintf(out, "    %s\n", checkout.Path)
	}
	fmt.Fprintf(out, "\n")
}

func init() {
	configResetCmd.Flags().BoolVar(&configResetYes, "yes", false, "do not ask")
	// Separate from --yes on purpose. A script that already passes --yes means
	// "forget the configuration"; making it also delete a git clone would be a
	// change of meaning nobody asked for, applied to every machine it already runs
	// on.
	configResetCmd.Flags().BoolVar(&configResetTemplates, "delete-templates", false,
		"also delete the templates checkouts, without asking")
	configTemplatesCmd.Flags().BoolVar(&configTemplatesClear, "clear", false, "forget the recorded path")
	configCmd.AddCommand(configShowCmd, configTemplatesCmd, configKubeconfigCmd,
		configGitHubCmd, configRepoCmd, configResetCmd)
	rootCmd.AddCommand(configCmd)
}
