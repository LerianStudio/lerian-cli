package infracli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// fakeGitHub stands in for gh: it records what it was asked to do, without a
// GitHub account and without creating anything.
type fakeGitHub struct {
	loggedIn  bool
	loginRan  bool
	loginGive bool // whether the login leaves a usable credential
	created   *infra.GHRepo
	dir       string
	err       error

	// For the config screen.
	accounts    []infra.GHAccount
	switched    *infra.GHAccount
	loggedOut   *infra.GHAccount
	protocol    string
	protocolSet bool
}

func (f *fakeGitHub) GHStatus(context.Context) (infra.GHAccount, bool) {
	if !f.loggedIn {
		return infra.GHAccount{}, false
	}
	return infra.GHAccount{Host: "github.com", Login: "octocat"}, true
}

func (f *fakeGitHub) GHLogin(context.Context, io.Reader, io.Writer, io.Writer) error {
	f.loginRan = true
	f.loggedIn = f.loginGive
	return nil
}

func (f *fakeGitHub) GHAccounts(context.Context) []infra.GHAccount {
	if len(f.accounts) > 0 {
		return f.accounts
	}
	if !f.loggedIn {
		return nil
	}
	return []infra.GHAccount{{Host: "github.com", Login: "octocat", Active: true}}
}

func (f *fakeGitHub) GHSwitch(_ context.Context, account infra.GHAccount) error {
	f.switched = &account
	return nil
}

func (f *fakeGitHub) GHLogout(_ context.Context, account infra.GHAccount) error {
	f.loggedOut = &account
	return nil
}

func (f *fakeGitHub) GitProtocol(context.Context) string { return f.protocol }

func (f *fakeGitHub) SetGitProtocol(_ context.Context, protocol string) error {
	f.protocol = protocol
	f.protocolSet = true
	return nil
}

func (f *fakeGitHub) CreateRepository(_ context.Context, dir string, repo infra.GHRepo) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.created, f.dir = &repo, dir
	return "https://github.com/octocat/" + repo.Name, nil
}

// withGitHub swaps the real gh for the duration of one test.
func withGitHub(t *testing.T, gh gitHub, err error) {
	t.Helper()
	previous := newGitHub
	newGitHub = func() (gitHub, error) { return gh, err }
	t.Cleanup(func() { newGitHub = previous })
}

// The default is private, and it has to be the default rather than a row
// somebody has to find: this repository is a map of an estate — account numbers,
// VPC layout, cluster names — and public is a decision to arrive at on purpose.
func TestCreatingOnGitHubIsPrivateUnlessAskedOtherwise(t *testing.T) {
	gh := &fakeGitHub{loggedIn: true}
	withGitHub(t, gh, nil)

	var out bytes.Buffer
	// yes · the default name (a bare newline takes it) · private, the row the
	// cursor starts on.
	ask, painted := selectorFor(t, keyEnterSeq+"\n"+keyEnterSeq)

	offerGitHub(context.Background(), ask, &out, "/tmp/some-export")

	if gh.created == nil {
		t.Fatalf("nothing was created:\n%s", painted.String())
	}
	if !gh.created.Private {
		t.Error("the repository was created public by default")
	}
	if gh.created.Name != "some-export" {
		t.Errorf("named it %q, want the directory's name offered as the default", gh.created.Name)
	}
	if gh.dir != "/tmp/some-export" {
		t.Errorf("pushed from %q, not from the export", gh.dir)
	}
	if !strings.Contains(painted.String()+out.String(), "https://github.com/octocat/some-export") {
		t.Errorf("the url was not reported:\n%s%s", painted.String(), out.String())
	}
}

// Declining creates nothing and leaves the commands behind, which is what the
// export did before this offer existed.
func TestDecliningCreatesNothing(t *testing.T) {
	gh := &fakeGitHub{loggedIn: true}
	withGitHub(t, gh, nil)

	var out bytes.Buffer
	ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq) // "not now"

	offerGitHub(context.Background(), ask, &out, "/tmp/some-export")

	if gh.created != nil {
		t.Errorf("a repository was created after declining: %+v", gh.created)
	}
	if !strings.Contains(out.String(), "git remote add origin") {
		t.Errorf("declining left no way to do it by hand:\n%s", out.String())
	}
}

// Not logged in is offered a login rather than told to go and run one. And a
// login that leaves no credential must not be followed by a create that fails
// for a reason reading as a bug in this tool.
func TestLoggingInIsOfferedAndChecked(t *testing.T) {
	t.Run("a login that works is followed by the create", func(t *testing.T) {
		gh := &fakeGitHub{loginGive: true}
		withGitHub(t, gh, nil)

		var out bytes.Buffer
		// yes · log in now · the default name · private
		ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq+"\n"+keyEnterSeq)

		offerGitHub(context.Background(), ask, &out, "/tmp/some-export")

		if !gh.loginRan {
			t.Fatalf("no login was offered:\n%s", painted.String())
		}
		if gh.created == nil {
			t.Error("the create did not follow the login")
		}
	})

	t.Run("a login that leaves nothing stops there", func(t *testing.T) {
		gh := &fakeGitHub{} // login runs, credential does not appear
		withGitHub(t, gh, nil)

		var out bytes.Buffer
		ask, _ := selectorFor(t, keyEnterSeq+keyEnterSeq+"\n"+keyEnterSeq)

		offerGitHub(context.Background(), ask, &out, "/tmp/some-export")

		if gh.created != nil {
			t.Error("it created a repository with no login behind it")
		}
		if !strings.Contains(out.String(), "git remote add origin") {
			t.Errorf("it stopped without saying how to finish by hand:\n%s", out.String())
		}
	})

	t.Run("declining the login creates nothing", func(t *testing.T) {
		gh := &fakeGitHub{}
		withGitHub(t, gh, nil)

		var out bytes.Buffer
		ask, _ := selectorFor(t, keyEnterSeq+keyDownSeq+keyEnterSeq) // yes, then cancel

		offerGitHub(context.Background(), ask, &out, "/tmp/some-export")

		if gh.loginRan {
			t.Error("it logged in after the login was declined")
		}
		if gh.created != nil {
			t.Error("it created a repository anyway")
		}
	})
}

// gh absent is said rather than passed over. Somebody who expected the offer is
// owed the reason, and the export itself succeeded either way.
func TestNoGHSaysSoAndLeavesTheCommands(t *testing.T) {
	withGitHub(t, nil, infra.ErrNoGH)

	var out bytes.Buffer
	ask, _ := selectorFor(t, "")

	offerGitHub(context.Background(), ask, &out, "/tmp/some-export")

	if !strings.Contains(out.String(), "gh is not installed") {
		t.Errorf("the absence was not explained:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "git remote add origin") {
		t.Errorf("no way to finish by hand:\n%s", out.String())
	}
}

// A failed create is reported with what gh said, and still leaves the manual
// route: the files and the commit are there regardless.
func TestAFailedCreateReportsAndFallsBack(t *testing.T) {
	gh := &fakeGitHub{loggedIn: true, err: errCreateFailed}
	withGitHub(t, gh, nil)

	var out bytes.Buffer
	ask, _ := selectorFor(t, keyEnterSeq+"\n"+keyEnterSeq)

	offerGitHub(context.Background(), ask, &out, "/tmp/some-export")

	if !strings.Contains(out.String(), "Name already exists") {
		t.Errorf("what gh said was dropped:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "git remote add origin") {
		t.Errorf("no way to finish by hand:\n%s", out.String())
	}
}

// Outside a terminal there is nobody to ask, and the export must not stall on a
// question. It prints what it always printed.
func TestWithoutATerminalItJustSaysWhatToRun(t *testing.T) {
	gh := &fakeGitHub{loggedIn: true}
	withGitHub(t, gh, nil)

	var out bytes.Buffer
	offerGitHub(context.Background(), &prompter{out: &out}, &out, "/tmp/some-export")

	if gh.created != nil {
		t.Error("it created a repository with nobody to ask")
	}
	if !strings.Contains(out.String(), "git remote add origin") {
		t.Errorf("it said nothing:\n%s", out.String())
	}
}

// errCreateFailed is what gh says when the name is taken, which is the common
// failure and the one whose message carries the fix.
var errCreateFailed = errCreate("infra: gh repo create failed: exit status 1\n" +
	"GraphQL: Name already exists on this account")

type errCreate string

func (e errCreate) Error() string { return string(e) }

// The menu has to fit what the machine actually has. Rows that need a login
// where there is none are keypresses that can only fail, and a switch between
// one account is a question with one answer.
func TestTheGitHubMenuFitsWhatIsThere(t *testing.T) {
	tests := []struct {
		name     string
		accounts []infra.GHAccount
		want     []string
		gone     []string
	}{
		{
			name: "nobody signed in",
			want: []string{githubLogin, githubProtocol},
			gone: []string{githubSwitch, githubLogout},
		},
		{
			name:     "one account",
			accounts: []infra.GHAccount{{Host: "github.com", Login: "octocat", Active: true}},
			want:     []string{githubLogin, githubProtocol, githubLogout},
			gone:     []string{githubSwitch},
		},
		{
			name: "two accounts",
			accounts: []infra.GHAccount{
				{Host: "github.com", Login: "octocat", Active: true},
				{Host: "github.com", Login: "robot"},
			},
			want: []string{githubLogin, githubSwitch, githubProtocol, githubLogout},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := make([]string, 0, 5)
			for _, opt := range githubOptions(test.accounts) {
				values = append(values, opt.value)
			}
			for _, want := range test.want {
				if indexOf(values, want) < 0 {
					t.Errorf("%q is not offered: %v", want, values)
				}
			}
			for _, gone := range test.gone {
				if indexOf(values, gone) >= 0 {
					t.Errorf("%q is offered with nothing behind it: %v", gone, values)
				}
			}
		})
	}
}

// Signing out reaches past this tool: the credential is in the system keyring,
// and every gh on the machine reads it. It is confirmed, and declining the
// confirmation removes nothing.
func TestSigningOutIsConfirmed(t *testing.T) {
	accounts := []infra.GHAccount{{Host: "github.com", Login: "octocat", Active: true}}

	t.Run("declining removes nothing", func(t *testing.T) {
		gh := &fakeGitHub{accounts: accounts}
		var out bytes.Buffer
		ask, painted := selectorFor(t, keyEnterSeq) // "keep it", the first row

		if err := logOutOfGitHub(context.Background(), ask, &out, gh, accounts); err != nil {
			t.Fatal(err)
		}
		if gh.loggedOut != nil {
			t.Errorf("it signed out anyway: %+v", gh.loggedOut)
		}
		if !strings.Contains(painted.String(), "Sign out of octocat?") {
			t.Errorf("it was not asked:\n%s", painted.String())
		}
	})

	t.Run("confirming removes the one chosen", func(t *testing.T) {
		gh := &fakeGitHub{accounts: accounts}
		var out bytes.Buffer
		ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq) // "sign out"

		if err := logOutOfGitHub(context.Background(), ask, &out, gh, accounts); err != nil {
			t.Fatal(err)
		}
		if gh.loggedOut == nil || gh.loggedOut.Login != "octocat" {
			t.Errorf("signed out of %+v", gh.loggedOut)
		}
	})
}

// With one login there is nothing to choose between, and a menu of one row is a
// keypress that decides nothing.
func TestOneAccountIsNotAQuestion(t *testing.T) {
	accounts := []infra.GHAccount{{Host: "github.com", Login: "octocat", Active: true}}
	ask, painted := selectorFor(t, "")

	picked, err := pickGitHubAccount(ask, accounts, "Which?", "")
	if err != nil {
		t.Fatal(err)
	}
	if picked == nil || picked.Login != "octocat" {
		t.Fatalf("picked %+v", picked)
	}
	if painted.String() != "" {
		t.Errorf("it asked with one answer available:\n%s", painted.String())
	}
}

// And with two it asks, and switches to the one chosen rather than to the first.
func TestSwitchingUsesTheAccountChosen(t *testing.T) {
	accounts := []infra.GHAccount{
		{Host: "github.com", Login: "octocat", Active: true},
		{Host: "github.acme.example", Login: "robot"},
	}
	gh := &fakeGitHub{accounts: accounts}
	ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq)

	if err := switchGitHubAccount(context.Background(), ask, gh, accounts); err != nil {
		t.Fatal(err)
	}
	if gh.switched == nil {
		t.Fatal("nothing was switched to")
	}
	if gh.switched.Login != "robot" || gh.switched.Host != "github.acme.example" {
		t.Errorf("switched to %+v, want the second row", gh.switched)
	}
}

// The protocol decides whether pushing to the exported repository asks for a
// password every time, so the one in force is named — and re-picking it stays
// available, because disabling a row reads as "you may not have this".
func TestTheProtocolInForceIsNamedAndStillSelectable(t *testing.T) {
	gh := &fakeGitHub{protocol: "https"}
	ask, painted := selectorFor(t, keyEnterSeq) // ssh, the first row

	if err := setGitProtocol(context.Background(), ask, gh); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(painted.String(), "(current)") {
		t.Errorf("the protocol in force was not marked:\n%s", painted.String())
	}
	if gh.protocol != "ssh" || !gh.protocolSet {
		t.Errorf("the choice was not written: %q", gh.protocol)
	}
}

// Without gh there is no screen to show, and the message says what to install
// and what it is for. Not an error of this command: the machine simply does not
// have it.
func TestTheGitHubScreenWithoutGH(t *testing.T) {
	withGitHub(t, nil, infra.ErrNoGH)

	var out bytes.Buffer
	if err := ConfigureGitHub(context.Background(), &out); err != nil {
		t.Fatalf("a machine without gh failed the command: %v", err)
	}
	if !strings.Contains(out.String(), "gh is not installed") {
		t.Errorf("it did not say why:\n%s", out.String())
	}
}

// Outside a terminal it reports and stops, rather than hanging on a menu nobody
// can answer.
func TestTheGitHubScreenWithoutATerminal(t *testing.T) {
	gh := &fakeGitHub{accounts: []infra.GHAccount{
		{Host: "github.com", Login: "octocat", Active: true},
		{Host: "github.com", Login: "robot"},
	}, protocol: "ssh"}
	withGitHub(t, gh, nil)

	var out bytes.Buffer
	if err := ConfigureGitHub(context.Background(), &out); err != nil {
		t.Fatal(err)
	}

	report := out.String()
	for _, want := range []string{"octocat", "robot", "ssh", "gh auth login"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not mention %q:\n%s", want, report)
		}
	}
	// Which one gh acts as is the fact that decides where a repository lands.
	if !strings.Contains(report, "active") {
		t.Errorf("it does not say which account is the active one:\n%s", report)
	}
}
