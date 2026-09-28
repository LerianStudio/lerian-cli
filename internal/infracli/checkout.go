package infracli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// rememberedCheckout is where the last answer to "where are the templates?" was
// written. It is read after the automatic sources and before giving up: a remembered
// answer is a default, not an override, so being inside a checkout still wins.
func rememberedCheckout() string {
	cfg, err := config.Load()
	if err != nil {
		// A config that cannot be read is not an error here. It only means there is
		// nothing remembered, and the question below still has an answer.
		return ""
	}
	if cfg.TemplatesCheckout == "" || !infra.IsCheckout(cfg.TemplatesCheckout) {
		return ""
	}
	return cfg.TemplatesCheckout
}

// rememberCheckout writes the answer so the question is asked once per machine
// rather than once per shell.
//
// This is what an environment variable cannot do. A process cannot export into
// the shell that started it — the environment is copied to children and never
// travels back — so `LERIAN_TF_REPO` set here would live exactly as long as this
// run. Written to the config it outlives the terminal, which is what "remembered"
// has to mean to be worth anything.
func rememberCheckout(out io.Writer, root string) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(out, "  could not read the config to remember it: %v\n", err)
		return
	}
	cfg.TemplatesCheckout = root
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(out, "  could not save the checkout: %v\n", err)
		return
	}

	// Set for this run too. Everything downstream reads the environment, and the
	// export line is there for an operator who wants it in the shell as well —
	// printed rather than promised, because only they can run it.
	_ = os.Setenv("LERIAN_TF_REPO", root)

	fmt.Fprintf(out, "\n  Remembered: %s\n", root)
	fmt.Fprintf(out, "  Later runs find it without asking. For this shell too:\n")
	fmt.Fprintf(out, "    export LERIAN_TF_REPO=%s\n", shellQuote(root))
}

// shellQuote renders a value for a command line the operator is invited to run.
//
// Not %q: that is Go quoting, and it produces a double-quoted string, where a
// path holding $(...) or backticks is still substituted by the shell the moment
// it is pasted. Single quotes suspend all of it; the only character that needs
// care inside them is the single quote itself, which cannot be escaped and has
// to be closed, emitted, and reopened.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// askForCheckout asks where the templates are, and keeps asking while the
// operator is still answering.
//
// The default is only offered when it would work. Offering the working directory
// unconditionally made Enter — the one answer a prompt invites — a guaranteed
// failure whenever the operator happened to be standing somewhere else, which is
// most of the time: the command is run from the project being deployed, not from
// the templates.
//
// A wrong answer is asked about again rather than ending the run. The operator is
// right there, and the path was a typo or the wrong clone; making them start the
// command over to correct one line is the round trip this question exists to
// remove.
func askForCheckout(ask *prompter, out io.Writer, templatesDir string) (string, error) {
	const attempts = 3

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		answer, err := ask.ask(
			"Where is the lerian-terraform-foundation checkout?",
			checkoutPurpose(templatesDir),
			defaultCheckout(templatesDir), "--repo")
		if err != nil {
			if lastErr != nil {
				return "", lastErr
			}
			return "", err
		}

		absolute, err := filepath.Abs(answer)
		if err != nil {
			return "", fmt.Errorf("cannot resolve %q: %w", answer, err)
		}
		if infra.IsCheckout(absolute) {
			rememberCheckout(out, absolute)
			return absolute, nil
		}

		lastErr = notACheckout(absolute, "the answer")
		if attempt < attempts {
			fmt.Fprintf(out, "\n  %s is not a checkout: the directories examples/aws/_modules\n"+
				"  and examples/aws/backend are what identifies one.\n", absolute)
		}
	}
	return "", lastErr
}

// defaultCheckout is the answer Enter accepts, and it is empty unless there is
// one that works.
//
// The working directory first, because somebody standing in the clone is the
// case worth one keypress. Then the managed path, which is where --clone puts it
// and therefore where a previous operator's clone tends to be.
func defaultCheckout(templatesDir string) string {
	if working, err := os.Getwd(); err == nil && infra.IsCheckout(working) {
		return working
	}
	if managed, err := infra.ManagedCheckoutPath(templatesDir); err == nil && infra.IsCheckout(managed) {
		return managed
	}
	return ""
}

// checkoutPurpose says what the answer decides, and — when there is nothing to
// offer — what to do about not having one.
func checkoutPurpose(templatesDir string) string {
	const base = "The Terraform templates every stack is rendered from."
	if defaultCheckout(templatesDir) != "" {
		return base + " Enter takes the one found."
	}
	return base + " No clone found here; git clone it and give the path, or leave" +
		" this and run: lerian infra init --clone --templates-ref <tag>"
}
