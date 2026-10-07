package infra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// ErrNoGH is a machine without the GitHub CLI. Separate from the other missing
// binaries because it is the only one nothing requires: everything this tool
// does works without it, and it is reached only by somebody who asked for a
// repository on GitHub.
var ErrNoGH = errors.New("infra: gh is not installed")

// GHCLI runs the GitHub CLI.
//
// Through gh rather than the GitHub API directly. gh already holds the
// credential — in the system keyring, shared with every other gh on this
// machine — and a token this CLI stored itself would be a second place for a
// secret to live and a second place to revoke it from. It also already solves
// the parts that are not interesting here: device-code login, enterprise hosts,
// SSH versus HTTPS remotes.
type GHCLI struct {
	// Binary overrides the executable, for tests.
	Binary string
}

// NewGHCLI resolves the binary, or says what to install.
func NewGHCLI() (GHCLI, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return GHCLI{}, fmt.Errorf("%w\nIt is only needed to create the repository on GitHub for you.\n"+
			"Install it (https://cli.github.com), or create the repository yourself\n"+
			"and push — the export already left a git repository with one commit", ErrNoGH)
	}
	return GHCLI{}, nil
}

func (g GHCLI) binary() string {
	if g.Binary != "" {
		return g.Binary
	}
	return "gh"
}

// GHAccount is who gh is logged in as, on which host.
type GHAccount struct {
	Host  string
	Login string
	// Active marks the one gh acts as. With several logins only one is, and it
	// is the one that decides where a created repository lands.
	Active bool
}

// String is what the reports print.
func (a GHAccount) String() string {
	if a.Login == "" {
		return "not logged in"
	}
	if a.Host == "" || a.Host == "github.com" {
		return a.Login
	}
	return a.Login + " at " + a.Host
}

// ghAccountLine reads the login out of `gh auth status`.
//
// Parsed from the human output because there is no machine-readable form of this
// one: `gh auth status` has no --json, and `gh api user` answers a different
// question — it would say who the token belongs to while skipping the hosts gh
// knows about and the state of their tokens. The line is stable across the 2.x
// series:
//
//	✓ Logged in to github.com account octocat (keyring)
var ghAccountLine = regexp.MustCompile(`Logged in to (\S+) account (\S+)`)

// GHStatus reports whether gh can act on this machine's behalf.
//
// A non-zero exit is the documented way gh says "nobody is logged in", so it is
// not an error here: the question being asked is exactly that, and the answer is
// no.
func (g GHCLI) GHStatus(ctx context.Context) (GHAccount, bool) {
	output, ok := g.authStatus(ctx)
	if !ok {
		return GHAccount{}, false
	}

	accounts := parseGHAccounts(output)
	if len(accounts) == 0 {
		// Exit zero with nothing recognizable: gh is logged in to something this
		// cannot name. Usable, unnamed — reporting "not logged in" would be wrong
		// in the direction that costs somebody a login they already have.
		return GHAccount{}, true
	}
	return accounts[0], true
}

// authStatus runs the command both readers need.
//
// gh writes the status to stderr, which is where it has always written it.
// CombinedOutput rather than Output for that reason alone.
func (g GHCLI) authStatus(ctx context.Context) (string, bool) {
	output, err := exec.CommandContext(ctx, g.binary(), "auth", "status").CombinedOutput()
	if err != nil {
		return "", false
	}
	return string(output), true
}

// GHLogin runs the interactive login and hands it the terminal.
//
// Handed over rather than captured: gh prints a one-time code to paste into a
// browser and waits, and a login whose output this tool buffered would be a
// prompt nobody can see.
func (g GHCLI) GHLogin(ctx context.Context, in io.Reader, out, errOut io.Writer) error {
	command := exec.CommandContext(ctx, g.binary(), "auth", "login")
	command.Stdin = in
	command.Stdout = out
	command.Stderr = errOut

	if err := command.Run(); err != nil {
		return fmt.Errorf("infra: gh auth login failed: %w", err)
	}
	return nil
}

// GHAccounts is every login gh holds, across every host.
//
// More than one is normal: a personal account and a work account on github.com,
// or an enterprise host beside the public one. Which of them is active decides
// where `gh repo create` puts a repository, so naming them all is the only way
// to make that visible before it matters.
func (g GHCLI) GHAccounts(ctx context.Context) []GHAccount {
	output, ok := g.authStatus(ctx)
	if !ok {
		return nil
	}
	return parseGHAccounts(output)
}

// ghActiveLine is how gh marks the login it acts as.
var ghActiveLine = regexp.MustCompile(`Active account:\s*true`)

// parseGHAccounts reads the logins out of `gh auth status`, active one first.
//
// Scanned line by line rather than by pulling every "Logged in to" out at once,
// because which account is active is on a line of its own underneath. Read
// rather than assumed from the order: gh happens to print the active one first
// today, and a tool that silently creates a repository under the wrong account
// should not rest on where a line happened to land.
func parseGHAccounts(output string) []GHAccount {
	var accounts []GHAccount
	for _, line := range strings.Split(output, "\n") {
		if match := ghAccountLine.FindStringSubmatch(line); match != nil {
			accounts = append(accounts, GHAccount{Host: match[1], Login: match[2]})
			continue
		}
		if len(accounts) > 0 && ghActiveLine.MatchString(line) {
			accounts[len(accounts)-1].Active = true
		}
	}

	for i, account := range accounts {
		if account.Active {
			accounts[0], accounts[i] = accounts[i], accounts[0]
			break
		}
	}
	return accounts
}

// GHSwitch makes one of those logins the active one.
func (g GHCLI) GHSwitch(ctx context.Context, account GHAccount) error {
	if err := account.validate(); err != nil {
		return err
	}

	// #nosec G204 -- the binary is the literal "gh", the flags are literals, and
	// the two variable parts are checked above for the only thing argv is
	// vulnerable to: a value that parses as an option. No shell is involved.
	command := exec.CommandContext(ctx, g.binary(), "auth", "switch",
		"--hostname", account.Host, "--user", account.Login)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("infra: gh auth switch failed: %w\n%s", err,
			strings.TrimSpace(string(output)))
	}
	return nil
}

// GHLogout ends one login.
//
// This reaches further than this tool, the way `aws sso logout` does: the
// credential it removes is in the system keyring, and every gh on this machine
// reads it. Nothing here does it without being asked.
func (g GHCLI) GHLogout(ctx context.Context, account GHAccount) error {
	if err := account.validate(); err != nil {
		return err
	}

	// #nosec G204 -- see GHSwitch above; same literals, same check.
	command := exec.CommandContext(ctx, g.binary(), "auth", "logout",
		"--hostname", account.Host, "--user", account.Login)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("infra: gh auth logout failed: %w\n%s", err,
			strings.TrimSpace(string(output)))
	}
	return nil
}

// validate rejects a host or login argv would read as an option. Both reach a
// command line, and both come from gh's own output — which is not a reason to
// skip the check, only a reason to expect it to pass.
func (a GHAccount) validate() error {
	for _, value := range []string{a.Host, a.Login} {
		if strings.TrimSpace(value) == "" {
			return errors.New("infra: the account needs a host and a login")
		}
		if strings.HasPrefix(value, "-") {
			return fmt.Errorf("infra: %q cannot start with a dash", value)
		}
	}
	return nil
}

// GitProtocol is how gh writes the remote of a repository it creates: ssh or
// https. It decides whether the exported repository can be pushed to without a
// password prompt, which makes it the one gh setting worth surfacing here.
func (g GHCLI) GitProtocol(ctx context.Context) string {
	command := exec.CommandContext(ctx, g.binary(), "config", "get", "git_protocol")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// SetGitProtocol writes it.
func (g GHCLI) SetGitProtocol(ctx context.Context, protocol string) error {
	if protocol != "ssh" && protocol != "https" {
		return fmt.Errorf("infra: %q is not a git protocol gh understands", protocol)
	}

	// #nosec G204 -- every argument is a literal: the protocol is one of exactly
	// two values, checked immediately above.
	command := exec.CommandContext(ctx, g.binary(), "config", "set", "git_protocol", protocol)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("infra: gh config set git_protocol failed: %w\n%s", err,
			strings.TrimSpace(string(output)))
	}
	return nil
}

// GHRepo is the repository to create.
type GHRepo struct {
	// Name is what gh is given: "my-infrastructure", or "org/my-infrastructure"
	// to create it somewhere other than the logged-in account.
	Name string
	// Private is the default everywhere it is offered. This repository holds
	// account numbers, VPC layouts, cluster names and the shape of an estate —
	// not credentials, but a map of where they are worth stealing from.
	Private bool
}

// validate rejects a name argv would read as an option.
//
// The name reaches a command line. Everything else here is a literal, and the
// only way a value turns into a flag is by starting with a dash.
func (r GHRepo) validate() error {
	switch {
	case strings.TrimSpace(r.Name) == "":
		return errors.New("infra: the repository needs a name")
	case strings.HasPrefix(r.Name, "-"):
		return fmt.Errorf("infra: %q cannot start with a dash", r.Name)
	case strings.ContainsAny(r.Name, " \t\n"):
		return fmt.Errorf("infra: %q cannot contain spaces", r.Name)
	case strings.Count(r.Name, "/") > 1:
		return fmt.Errorf("infra: %q should be <name> or <owner>/<name>", r.Name)
	}
	return nil
}

// CreateRepository creates the repository on GitHub and pushes what is in dir.
//
// One command rather than create-then-remote-then-push: gh --source --push does
// all three, and it does the part that is genuinely annoying — matching the
// remote protocol to how this machine authenticates, SSH or HTTPS.
//
// It returns the URL, which is the only thing anybody wants afterwards.
func (g GHCLI) CreateRepository(ctx context.Context, dir string, repo GHRepo) (string, error) {
	if err := repo.validate(); err != nil {
		return "", err
	}

	visibility := "--public"
	if repo.Private {
		visibility = "--private"
	}

	// #nosec G204 -- the binary is the literal "gh", every argument but the name
	// is a literal, and the name is checked above for the only thing argv is
	// vulnerable to: a value that parses as an option. No shell is involved.
	command := exec.CommandContext(ctx, g.binary(), "repo", "create", repo.Name,
		visibility, "--source", ".", "--remote", "origin", "--push")
	command.Dir = dir

	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("infra: gh repo create failed: %w\n%s", err,
			strings.TrimSpace(stderr.String()))
	}

	// The URL is on stdout when there is one, and on stderr when gh decided the
	// run was interactive. Both are read rather than guessed at.
	if url := firstRepoURL(string(output) + "\n" + stderr.String()); url != "" {
		return url, nil
	}
	return "", nil
}

var repoURL = regexp.MustCompile(`https://\S+`)

// firstRepoURL pulls the repository URL out of whatever gh said.
func firstRepoURL(text string) string {
	return repoURL.FindString(text)
}
