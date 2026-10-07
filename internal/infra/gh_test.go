package infra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGH writes a gh on PATH that says whatever the test needs it to say.
func fakeGH(t *testing.T, script string) GHCLI {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return GHCLI{Binary: path}
}

// The login is read off the human output because gh has no machine-readable
// form of this one, which makes the parsing worth a test of its own: the whole
// offer turns on it, and a format drift reads as "not logged in" — sending
// somebody through a login they already have.
func TestTheLoggedInAccountIsRead(t *testing.T) {
	// On stderr, where gh writes it.
	gh := fakeGH(t, `cat >&2 <<'EOF'
github.com
  ✓ Logged in to github.com account octocat (keyring)
  - Active account: true
EOF
exit 0`)

	account, loggedIn := gh.GHStatus(context.Background())
	if !loggedIn {
		t.Fatal("a successful gh auth status was read as logged out")
	}
	if account.Login != "octocat" || account.Host != "github.com" {
		t.Errorf("read %+v, want octocat at github.com", account)
	}
	if account.String() != "octocat" {
		t.Errorf("printed %q — github.com is the default and does not need saying", account)
	}
}

// An enterprise host is part of the answer: two accounts of the same name on
// different hosts are different people.
func TestAnotherHostIsNamed(t *testing.T) {
	gh := fakeGH(t, `echo "✓ Logged in to github.acme.example account octocat (keyring)" >&2; exit 0`)

	account, _ := gh.GHStatus(context.Background())
	if got := account.String(); got != "octocat at github.acme.example" {
		t.Errorf("printed %q, want the host named", got)
	}
}

// A non-zero exit is how gh says nobody is logged in. Not an error here: that is
// the question being asked.
func TestNoLoginIsAnAnswerNotAFailure(t *testing.T) {
	gh := fakeGH(t, `echo "You are not logged into any GitHub hosts." >&2; exit 1`)

	account, loggedIn := gh.GHStatus(context.Background())
	if loggedIn {
		t.Error("a failed gh auth status was read as a login")
	}
	if account.String() != "not logged in" {
		t.Errorf("printed %q", account)
	}
}

// Exit zero with nothing recognizable means gh is logged in to something this
// cannot name. Usable is the honest answer: the other one costs somebody a login
// they already have.
func TestAnUnreadableStatusIsStillALogin(t *testing.T) {
	gh := fakeGH(t, `echo "something new"; exit 0`)

	if _, loggedIn := gh.GHStatus(context.Background()); !loggedIn {
		t.Error("a successful status was read as logged out because the wording changed")
	}
}

// The name reaches a command line, and the only way a value there turns into a
// flag is by starting with a dash.
func TestNamesArgvWouldMisreadAreRefused(t *testing.T) {
	for _, name := range []string{"", "   ", "--private", "-x", "my repo", "a/b/c"} {
		if err := (GHRepo{Name: name}).validate(); err == nil {
			t.Errorf("%q was accepted", name)
		}
	}
	for _, name := range []string{"infrastructure", "lerian/infrastructure", "infra-2026"} {
		if err := (GHRepo{Name: name}).validate(); err != nil {
			t.Errorf("%q was refused: %v", name, err)
		}
	}
}

// What gh is asked to do, exactly. Visibility is the argument that cannot be
// undone by deleting a directory, and --source/--push are what make this one
// command instead of three.
func TestTheCreateIsPrivateAndPushesFromTheExport(t *testing.T) {
	recorded := filepath.Join(t.TempDir(), "argv")
	gh := fakeGH(t, `printf '%s\n' "$@" > `+recorded+`
echo "https://github.com/octocat/infrastructure"
exit 0`)

	dir := t.TempDir()
	url, err := gh.CreateRepository(context.Background(), dir,
		GHRepo{Name: "infrastructure", Private: true})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/octocat/infrastructure" {
		t.Errorf("read the url as %q", url)
	}

	argv, err := os.ReadFile(recorded)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"repo", "create", "infrastructure", "--private", "--source", "--push"} {
		if !strings.Contains(string(argv), want) {
			t.Errorf("gh was not given %q:\n%s", want, argv)
		}
	}
	if strings.Contains(string(argv), "--public") {
		t.Errorf("a private repository was created public:\n%s", argv)
	}
}

// And public when that is what was chosen — the two must not collapse into one.
func TestPublicIsPassedWhenAskedFor(t *testing.T) {
	recorded := filepath.Join(t.TempDir(), "argv")
	gh := fakeGH(t, `printf '%s\n' "$@" > `+recorded+`; exit 0`)

	if _, err := gh.CreateRepository(context.Background(), t.TempDir(),
		GHRepo{Name: "infrastructure"}); err != nil {
		t.Fatal(err)
	}
	argv, err := os.ReadFile(recorded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(argv), "--public") || strings.Contains(string(argv), "--private") {
		t.Errorf("the visibility was not the one chosen:\n%s", argv)
	}
}

// gh says why it failed on stderr, and an exit status alone is not a diagnosis:
// "name already exists" and "no permission in that org" are different problems
// with different answers.
func TestWhatGHSaidSurvivesTheFailure(t *testing.T) {
	gh := fakeGH(t, `echo "GraphQL: Name already exists on this account" >&2; exit 1`)

	_, err := gh.CreateRepository(context.Background(), t.TempDir(), GHRepo{Name: "taken"})
	if err == nil {
		t.Fatal("a failed create reported success")
	}
	if !strings.Contains(err.Error(), "Name already exists") {
		t.Errorf("the reason was dropped: %v", err)
	}
}
