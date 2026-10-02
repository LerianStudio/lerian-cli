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

// recordedCheckout is the path in the config as written, valid or not. The
// cleanup reads this one: forgetting a recorded path is a thing to do precisely
// when the clone it names is gone.
func recordedCheckout() string {
	cfg, err := config.Load()
	if err != nil {
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
	answered, err := promptCheckoutPath(ask, out, templatesDir)
	if err != nil {
		return "", err
	}
	rememberCheckout(out, answered)
	return answered, nil
}

// promptCheckoutPath is the question and the retrying, without the writing down.
//
// Split out because the two callers disagree about who saves. The run above has
// nowhere else to put the answer, so it records it on the way past; `config
// templates` has a command whose whole job is to record it, and saving here too
// would print the confirmation twice and say it in two voices.
func promptCheckoutPath(ask *prompter, out io.Writer, templatesDir string) (string, error) {
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
	if managed := infra.FirstManagedCheckout(templatesDir); managed != "" {
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

// TemplatesAnswer is what somebody decided at the templates prompt.
type TemplatesAnswer struct {
	// Path is the checkout to record. Empty when Forget is set.
	Path string
	// Forget asks for the recorded path to be dropped.
	Forget bool
}

// Sentinel rows. Values a filesystem path cannot collide with, because every
// other row in the menu is one.
const (
	templatesTypeAPath = "\x00type"
	templatesForget    = "\x00forget"
)

// AskForTemplates asks which checkout to record, for the menu row that arrives
// with no path to give.
//
// The row exists because somebody picked `config templates` off a list, where
// there is no command line to put an argument on. Answering that with the usage
// text of a command they did not type leaves them where they started: the
// question has an answer they know, and this is where to ask it.
//
// The machine's own checkouts are offered rather than only a blank line. There
// are at most three, they are the likely answer, and a path typed by hand is a
// typo waiting to be rejected.
func AskForTemplates(out io.Writer) (TemplatesAnswer, error) {
	recorded := recordedCheckout()

	picked, err := Choose(out, "Which lerian-terraform-foundation checkout?",
		"Written to ~/.lerian/config.yaml, so later runs find it without a flag.",
		templatesChoices(recorded))
	if err != nil {
		return TemplatesAnswer{}, err
	}

	switch picked {
	case templatesForget:
		return TemplatesAnswer{Forget: true}, nil
	case templatesTypeAPath:
		path, err := promptCheckoutPath(newPrompter(out), out, "")
		if err != nil {
			return TemplatesAnswer{}, err
		}
		return TemplatesAnswer{Path: path}, nil
	default:
		return TemplatesAnswer{Path: picked}, nil
	}
}

// templatesChoices lists the checkouts this machine has, each said to be a
// checkout by the same test the command would apply anyway.
//
// Deduplicated, because the recorded path is very often also the one being stood
// in, and a menu offering the same directory three times reads as three answers.
func templatesChoices(recorded string) []Choice {
	choices := make([]Choice, 0, 5)
	seen := map[string]bool{}

	add := func(path, note string) {
		if path == "" || seen[path] || !infra.IsCheckout(path) {
			return
		}
		seen[path] = true
		choices = append(choices, Choice{Value: path, Label: path, Note: note})
	}

	add(recorded, "recorded now")
	if working, err := os.Getwd(); err == nil {
		add(working, "the directory you are in")
	}
	for _, managed := range infra.ManagedCheckoutPaths("") {
		add(managed, "cloned by infra init --clone")
	}

	choices = append(choices, Choice{
		Value: templatesTypeAPath,
		Label: "Type a path",
		Note:  "a clone somewhere else",
	})
	if recorded != "" {
		choices = append(choices, Choice{
			Value: templatesForget,
			Label: "Forget the recorded path",
			Note:  "the directory stays; later runs discover one again",
		})
	}
	return choices
}
