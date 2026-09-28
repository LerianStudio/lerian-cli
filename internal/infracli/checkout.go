package infracli

import (
	"fmt"
	"io"
	"os"

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
	fmt.Fprintf(out, "    export LERIAN_TF_REPO=%q\n", root)
}

// askForCheckout asks where the templates are, offering the working directory as
// the answer to accept with Enter.
//
// The working directory is the default because the common case is somebody
// standing in the clone they just made. It is offered even when it is not a
// checkout, so the prompt says what would be accepted rather than making the
// operator guess the shape of the answer.
func askForCheckout(ask *prompter, out io.Writer) (string, error) {
	working, err := os.Getwd()
	if err != nil {
		working = ""
	}

	answer, err := ask.ask(
		"Where is the lerian-terraform-foundation checkout?",
		"The Terraform templates every stack is rendered from. Clone it, then give the path.",
		working, "--repo")
	if err != nil {
		return "", err
	}

	if !infra.IsCheckout(answer) {
		return "", notACheckout(answer, "the answer")
	}

	rememberCheckout(out, answer)
	return answer, nil
}
