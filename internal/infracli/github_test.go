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
