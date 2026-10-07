package infracli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Sentinel rows for the GitHub menu.
const (
	githubLogin    = "\x00login"
	githubSwitch   = "\x00switch"
	githubProtocol = "\x00protocol"
	githubLogout   = "\x00logout"
)

// ConfigureGitHub is the `config` row for GitHub: who this machine is signed in
// as, and how to change it.
//
// It is here for the same reason the kubeconfig row is. The export offers to
// create the repository and will log somebody in on the way past, but that is a
// question asked in the middle of doing something else — and it cannot answer
// the ones that come up afterwards: signed in as the wrong account, needing a
// second one for an organization, or wanting the credential off a machine being
// handed on. Those belong where the other "what does this machine know about me"
// answers are.
func ConfigureGitHub(ctx context.Context, out io.Writer) error {
	gh, err := newGitHub()
	//nolint:nilerr // Not an error of this command: the machine simply does not
	// have gh, and the message already says what to install and what it is for.
	// Returning it would put a red line under a screen whose whole content is
	// that explanation — and would fail the menu row that opened it.
	if err != nil {
		fmt.Fprintf(out, "\n  %v\n\n", err)
		return nil
	}

	ask := newPrompter(out)
	for {
		accounts := gh.GHAccounts(ctx)
		describeGitHub(ctx, out, gh, accounts)

		if !ask.interactive {
			// Nothing to ask with. The status above is the whole answer, and the
			// commands are gh's own.
			fmt.Fprintf(out, "  gh auth login · gh auth switch · gh auth logout\n\n")
			return nil
		}

		picked, err := ask.pick("GitHub", "Changes what gh does, for every tool on this machine.", "",
			githubOptions(accounts), "")
		if err != nil {
			return err
		}

		if done, err := applyGitHubChoice(ctx, ask, out, gh, accounts, picked); err != nil {
			// Said and carried on: the menu is still the right place to be, and
			// what failed is on the screen with something to do about it.
			fmt.Fprintf(out, "\n  %v\n", err)
		} else if done {
			return nil
		}
	}
}

// applyGitHubChoice runs one row and reports whether the menu is finished.
func applyGitHubChoice(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	gh gitHub,
	accounts []infra.GHAccount,
	picked string,
) (bool, error) {
	switch picked {
	case githubLogin:
		fmt.Fprintln(out)
		return false, gh.GHLogin(ctx, os.Stdin, out, out)
	case githubSwitch:
		return false, switchGitHubAccount(ctx, ask, gh, accounts)
	case githubProtocol:
		return false, setGitProtocol(ctx, ask, gh)
	case githubLogout:
		return false, logOutOfGitHub(ctx, ask, out, gh, accounts)
	}
	return true, nil
}

// describeGitHub prints who gh is, before anything is chosen.
func describeGitHub(ctx context.Context, out io.Writer, gh gitHub, accounts []infra.GHAccount) {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> GitHub"))

	switch len(accounts) {
	case 0:
		fmt.Fprintf(out, "  %-10s %s\n", "signed in", "nobody")
	case 1:
		fmt.Fprintf(out, "  %-10s %s\n", "signed in", accounts[0])
	default:
		// The first is the active one: gh reports the active account first, and
		// which one is active is what decides where a repository gets created.
		fmt.Fprintf(out, "  %-10s %s  %s\n", "active", accounts[0], theme.dim("used by gh repo create"))
		for _, account := range accounts[1:] {
			fmt.Fprintf(out, "  %-10s %s\n", "also", account)
		}
	}

	protocol := gh.GitProtocol(ctx)
	if protocol == "" {
		protocol = "not set — gh asks at login"
	}
	fmt.Fprintf(out, "  %-10s %s  %s\n\n", "protocol", protocol,
		theme.dim("how the remote of a created repository is written"))
}

// githubOptions is the menu, with the rows that need a login only where there is
// one, and the switch only where there is something to switch between.
func githubOptions(accounts []infra.GHAccount) []option {
	options := make([]option, 0, 5)

	label, note := "sign in to GitHub", "gh auth login — prints a code and opens a browser"
	if len(accounts) > 0 {
		label, note = "sign in to another account", "gh auth login — an organization's host, or a second account"
	}
	options = append(options, option{value: githubLogin, label: label, note: note})

	if len(accounts) > 1 {
		options = append(options, option{value: githubSwitch, label: "switch the active account",
			note: "decides where gh repo create puts a repository"})
	}
	options = append(options, option{value: githubProtocol, label: "set how remotes are written",
		note: "ssh or https — gh config set git_protocol"})
	if len(accounts) > 0 {
		options = append(options, option{value: githubLogout, label: "sign out",
			note: "removes the credential from the keyring, for every tool"})
	}
	return append(options, option{value: "\x00back", label: "back", note: "changes nothing"})
}

// switchGitHubAccount asks which login becomes the active one.
func switchGitHubAccount(ctx context.Context, ask *prompter, gh gitHub, accounts []infra.GHAccount) error {
	picked, err := pickGitHubAccount(ask, accounts, "Which account should gh use?",
		"It is the one a created repository lands in.")
	if err != nil || picked == nil {
		return err
	}
	return gh.GHSwitch(ctx, *picked)
}

// logOutOfGitHub asks which login goes, and confirms before it does.
//
// Confirmed because this reaches past this tool: the credential is in the system
// keyring, and every gh on the machine reads it. Somebody signing out here has
// signed out of the one in their other terminal too.
func logOutOfGitHub(ctx context.Context, ask *prompter, out io.Writer, gh gitHub, accounts []infra.GHAccount) error {
	picked, err := pickGitHubAccount(ask, accounts, "Which account should be signed out?",
		"The credential is removed from the system keyring.")
	if err != nil || picked == nil {
		return err
	}

	answer, err := ask.pick("Sign out of "+picked.String()+"?",
		"Every gh on this machine loses it, not only this command.", "",
		[]option{
			{value: "no", label: "keep it", note: "changes nothing"},
			{value: "yes", label: "sign out", note: "signing in again needs a browser"},
		}, "")
	if err != nil || answer != "yes" {
		return nil
	}

	if err := gh.GHLogout(ctx, *picked); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n  Signed out of %s\n", picked)
	return nil
}

// pickGitHubAccount asks which of the logins to act on, and skips the question
// when there is only one. A menu of one row is a keypress that decides nothing.
//
// A nil account with a nil error is somebody leaving the question.
func pickGitHubAccount(
	ask *prompter,
	accounts []infra.GHAccount,
	question, purpose string,
) (*infra.GHAccount, error) {
	if len(accounts) == 0 {
		return nil, nil
	}
	if len(accounts) == 1 {
		return &accounts[0], nil
	}

	options := make([]option, 0, len(accounts))
	for _, account := range accounts {
		options = append(options, option{value: account.Host + "/" + account.Login, label: account.String()})
	}

	picked, err := ask.pick(question, purpose, "", options, "")
	if err != nil {
		return nil, err
	}
	for i, account := range accounts {
		if account.Host+"/"+account.Login == picked {
			return &accounts[i], nil
		}
	}
	return nil, nil
}

// setGitProtocol asks ssh or https.
//
// It is the one gh setting worth a row here: it decides how the remote of the
// exported repository is written, and therefore whether pushing to it asks for a
// password every time.
func setGitProtocol(ctx context.Context, ask *prompter, gh gitHub) error {
	current := gh.GitProtocol(ctx)

	// The current one is named in the note rather than marked fixed: fixed makes a
	// row unselectable, which is right for "already part of the answer" and wrong
	// here, where re-picking what is already set is a harmless no-op and
	// disabling it would read as "you may not have this".
	options := []option{
		{value: "ssh", label: "ssh", note: noteCurrent("git@github.com:… — needs a key on your account",
			current == "ssh")},
		{value: "https", label: "https", note: noteCurrent("https://github.com/… — gh supplies the credential",
			current == "https")},
	}

	picked, err := ask.pick("How should remotes be written?",
		"Applies to repositories gh creates from now on, including the exported one.", "",
		options, "")
	if err != nil {
		return err
	}
	return gh.SetGitProtocol(ctx, picked)
}

// noteCurrent marks the row that is already in force.
func noteCurrent(note string, current bool) string {
	if current {
		return note + "  (current)"
	}
	return note
}
