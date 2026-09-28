package infracli

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/config"
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
	answered, err := askForCheckout(answering(""), &out)
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
	answered, err := askForCheckout(answering(elsewhere), &out)
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
	_, err := askForCheckout(answering(t.TempDir()), &out)

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
	if _, err := askForCheckout(answering(checkout), &out); err != nil {
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
	answered, err := askForCheckout(answering(filepath.Base(checkout)), &out)
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
