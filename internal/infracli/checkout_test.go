package infracli

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// isolatedHome points the config at a temporary directory. Without it these
// tests would read and rewrite the config of whoever runs them.
func isolatedHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func answering(reply string) *prompter {
	return &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader(reply + "\n")),
		out:         &bytes.Buffer{},
	}
}

// Enter accepts the working directory, which is the common case: somebody
// standing in the clone they just made.
func TestTheWorkingDirectoryIsWhatEnterAccepts(t *testing.T) {
	isolatedHome(t)
	checkout := fakeCheckout(t, "", "")
	t.Chdir(checkout)

	var out bytes.Buffer
	answered, err := askForCheckout(answering(""), &out, t.TempDir())
	if err != nil {
		t.Fatalf("askForCheckout = %v", err)
	}

	if answered != checkout {
		t.Errorf("answered %q, want the working directory %q", answered, checkout)
	}
}

// And a path typed instead of accepted is what gets used, for the operator whose
// clone is somewhere else.
func TestATypedPathIsUsed(t *testing.T) {
	isolatedHome(t)
	elsewhere := fakeCheckout(t, "", "")
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	answered, err := askForCheckout(answering(elsewhere), &out, t.TempDir())
	if err != nil {
		t.Fatalf("askForCheckout = %v", err)
	}

	if answered != elsewhere {
		t.Errorf("answered %q, want the typed path %q", answered, elsewhere)
	}
}

// An answer that is not a checkout is rejected at the prompt rather than
// several steps later, where the failure would name a missing file instead of
// the wrong directory.
func TestAnAnswerThatIsNotACheckoutIsRejected(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	_, err := askForCheckout(answering(t.TempDir()), &out, t.TempDir())

	if err == nil {
		t.Fatal("a directory that is not a checkout was accepted")
	}
}

// The answer is written down, which is the part an environment variable cannot
// do: a process cannot export into the shell that started it, so LERIAN_TF_REPO
// set here would last exactly as long as the run.
func TestTheAnswerIsRememberedForTheNextRun(t *testing.T) {
	isolatedHome(t)
	checkout := fakeCheckout(t, "", "")
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	if _, err := askForCheckout(answering(checkout), &out, t.TempDir()); err != nil {
		t.Fatalf("askForCheckout = %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("reading the config back: %v", err)
	}
	if cfg.TemplatesCheckout != checkout {
		t.Errorf("config holds %q, want %q", cfg.TemplatesCheckout, checkout)
	}
	if got := rememberedCheckout(); got != checkout {
		t.Errorf("rememberedCheckout = %q, want %q", got, checkout)
	}
	// The export line is printed rather than promised: only the operator can run
	// it, and saying otherwise would be a lie about what a child process can do.
	if !strings.Contains(out.String(), "export LERIAN_TF_REPO=") {
		t.Errorf("the operator was not shown how to set it in their own shell:\n%s", out.String())
	}
}

// A remembered path that no longer holds a checkout is ignored rather than
// returned: the clone was moved or deleted, and pointing at it would fail later
// with a missing file.
func TestARememberedPathThatWentAwayIsIgnored(t *testing.T) {
	isolatedHome(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = filepath.Join(t.TempDir(), "gone")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	if got := rememberedCheckout(); got != "" {
		t.Errorf("rememberedCheckout = %q, want empty for a path that is no longer a checkout", got)
	}
}

// Nothing remembered is not an error. It only means the question has not been
// answered yet.
func TestNoConfigMeansNothingRemembered(t *testing.T) {
	isolatedHome(t)
	_ = os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".lerian"))

	if got := rememberedCheckout(); got != "" {
		t.Errorf("rememberedCheckout = %q, want empty", got)
	}
}

// The export line is printed for the operator to run, so it has to survive being
// run. %q is Go quoting: it yields a double-quoted string, and a path holding
// $(...) or a backtick is substituted by the shell the moment it is pasted.
//
// Asserted by running it: the shell is the only authority on what its own
// quoting means.
func TestTheExportLineSurvivesTheShellThatRunsIt(t *testing.T) {
	// Owned by this test, so a machine that happens to have a file where the
	// sentinel used to be hardcoded neither fails the run nor loses the file.
	sentinel := filepath.Join(t.TempDir(), "substitution-ran")

	awkward := []string{
		"/tmp/$(touch " + sentinel + ")/foundation",
		"/tmp/`id`/foundation",
		`/tmp/it's a checkout/foundation`,
		`/tmp/plain/foundation`,
	}

	for _, path := range awkward {
		t.Run(path, func(t *testing.T) {
			script := "LERIAN_TF_REPO=" + shellQuote(path) + `; printf %s "$LERIAN_TF_REPO"`

			out, err := exec.Command("sh", "-c", script).Output()
			if err != nil {
				t.Fatalf("the printed line did not run: %v", err)
			}
			if string(out) != path {
				t.Errorf("the shell read back %q, want %q", string(out), path)
			}
		})
	}

	if _, err := os.Stat(sentinel); err == nil {
		t.Error("the command substitution in the path ran")
	}
}

// A relative answer validates against the directory the operator was standing
// in, and then means something else from the next one. It is resolved before it
// is checked, saved or returned.
func TestARelativeAnswerIsSavedAbsolute(t *testing.T) {
	isolatedHome(t)
	checkout := fakeCheckout(t, "", "")
	t.Chdir(filepath.Dir(checkout))

	var out bytes.Buffer
	answered, err := askForCheckout(answering(filepath.Base(checkout)), &out, t.TempDir())
	if err != nil {
		t.Fatalf("askForCheckout with a relative answer = %v", err)
	}

	if !filepath.IsAbs(answered) {
		t.Errorf("answered %q, which is relative and means a different directory from anywhere else", answered)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.TemplatesCheckout) {
		t.Errorf("config holds the relative %q", cfg.TemplatesCheckout)
	}
}

// Enter is the answer a prompt invites, so offering a default that cannot work
// turns the most likely keypress into a guaranteed failure. The command is run
// from the project being deployed, not from the templates, so the working
// directory is usually not a checkout.
func TestNoDefaultIsOfferedWhenTheWorkingDirectoryIsNotACheckout(t *testing.T) {
	t.Chdir(t.TempDir())

	if got := defaultCheckout(filepath.Join(t.TempDir(), "no-managed-clone")); got != "" {
		t.Errorf("defaultCheckout = %q, want empty: pressing Enter on it would fail", got)
	}
}

func TestTheWorkingDirectoryIsOfferedWhenItIsACheckout(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Chdir(checkout)

	if got := defaultCheckout(filepath.Join(t.TempDir(), "no-managed-clone")); got != checkout {
		t.Errorf("defaultCheckout = %q, want the working directory %q", got, checkout)
	}
}

// With nothing to offer, the prompt says what to do about not having a clone
// rather than leaving the operator to guess.
func TestWithNoDefaultThePromptSaysHowToGetOne(t *testing.T) {
	t.Chdir(t.TempDir())

	purpose := checkoutPurpose(filepath.Join(t.TempDir(), "no-managed-clone"))

	if !strings.Contains(purpose, "--clone") {
		t.Errorf("the prompt does not say how to obtain a checkout:\n%s", purpose)
	}
}

// A wrong path is a typo or the wrong clone, and the operator is right there.
// Ending the run to make them retype one line is the round trip this question
// exists to remove.
func TestAWrongAnswerIsAskedAboutAgain(t *testing.T) {
	isolatedHome(t)
	checkout := fakeCheckout(t, "", "")
	t.Chdir(t.TempDir())

	wrong := t.TempDir()
	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader(wrong + "\n" + checkout + "\n")),
		out:         &bytes.Buffer{},
	}

	var out bytes.Buffer
	answered, err := askForCheckout(ask, &out, t.TempDir())
	if err != nil {
		t.Fatalf("a corrected answer still failed: %v", err)
	}
	if answered != checkout {
		t.Errorf("answered %q, want the second answer %q", answered, checkout)
	}
	if !strings.Contains(out.String(), "is not a checkout") {
		t.Errorf("the operator was not told why the first answer was rejected:\n%s", out.String())
	}
}

// It does not ask forever: a stream of wrong answers ends with the error the
// command would have given anyway.
func TestItGivesUpAfterEnoughWrongAnswers(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	wrong := t.TempDir()
	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader(strings.Repeat(wrong+"\n", 10))),
		out:         &bytes.Buffer{},
	}

	if _, err := askForCheckout(ask, &bytes.Buffer{}, t.TempDir()); err == nil {
		t.Error("askForCheckout accepted a directory that is not a checkout")
	}
}

// Picking the row and then being asked to type the path by hand would be the
// menu asking a question it already knows the answers to. The machine has at
// most three checkouts and they are the likely one.
func TestTheCheckoutsThisMachineHasAreOffered(t *testing.T) {
	working := fakeCheckout(t, "", "")
	t.Chdir(working)
	recorded := fakeCheckout(t, "", "")

	choices := templatesChoices(recorded)

	offered := make([]string, 0, len(choices))
	for _, choice := range choices {
		offered = append(offered, choice.Value)
	}

	for _, want := range []string{recorded, working, templatesTypeAPath, templatesForget} {
		if !slices.Contains(offered, want) {
			t.Errorf("%q is not offered; the menu has %v", want, offered)
		}
	}
}

// The recorded path is very often also the one being stood in, and the same
// directory on three rows reads as three answers.
func TestTheSameCheckoutIsOfferedOnce(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Chdir(checkout)

	seen := 0
	for _, choice := range templatesChoices(checkout) {
		if choice.Value == checkout {
			seen++
		}
	}

	if seen != 1 {
		t.Errorf("the same checkout is offered %d times", seen)
	}
}

// With nothing recorded there is nothing to forget, and a row that clears what
// was never set is an answer to a question nobody has.
func TestForgettingIsNotOfferedWhenNothingIsRecorded(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, choice := range templatesChoices("") {
		if choice.Value == templatesForget {
			t.Error("forgetting is offered with nothing recorded")
		}
	}
}

// A path that is not a checkout is not offered, by the same test the command
// would apply to it anyway. Offering one guarantees the rejection two keypresses
// later.
func TestARecordedPathThatWentAwayIsNotOffered(t *testing.T) {
	t.Chdir(t.TempDir())
	gone := filepath.Join(t.TempDir(), "gone")

	for _, choice := range templatesChoices(gone) {
		if choice.Value == gone {
			t.Error("a path with no checkout at it is offered")
		}
	}
	// Forgetting still is: a recorded path that no longer exists is precisely
	// when somebody wants it dropped.
	found := false
	for _, choice := range templatesChoices(gone) {
		if choice.Value == templatesForget {
			found = true
		}
	}
	if !found {
		t.Error("the path that went away cannot be forgotten")
	}
}

// Who writes the answer down differs between the two callers, so the question
// does not. `config templates` records it itself, and saving here too would
// print the confirmation twice.
func TestAskingForThePathDoesNotRecordIt(t *testing.T) {
	isolatedHome(t)
	checkout := fakeCheckout(t, "", "")
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	answered, err := promptCheckoutPath(answering(checkout), &out, t.TempDir())
	if err != nil {
		t.Fatalf("promptCheckoutPath = %v", err)
	}
	if answered != checkout {
		t.Fatalf("answered %q, want %q", answered, checkout)
	}

	if got := recordedCheckout(); got != "" {
		t.Errorf("the config holds %q; the prompt wrote it down", got)
	}
}

// Leaving outranks the last wrong answer. Typing a path that is not a checkout
// and then deciding not to continue reported the validation error — answering a
// question that was withdrawn, and turning a clean exit into a red line.
func TestLeavingAfterAWrongAnswerIsStillLeaving(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	wrong := t.TempDir()
	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader(wrong + "\nq\n")),
		out:         &bytes.Buffer{},
	}

	_, err := promptCheckoutPath(ask, &bytes.Buffer{}, t.TempDir())

	if !errors.Is(err, infra.ErrAborted) {
		t.Errorf("promptCheckoutPath = %v, want ErrAborted", err)
	}
}

// And with no wrong answer before it, which is the path that already worked.
func TestLeavingStraightAwayIsLeaving(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader("q\n")),
		out:         &bytes.Buffer{},
	}

	if _, err := promptCheckoutPath(ask, &bytes.Buffer{}, t.TempDir()); !errors.Is(err, infra.ErrAborted) {
		t.Errorf("promptCheckoutPath = %v, want ErrAborted", err)
	}
}
