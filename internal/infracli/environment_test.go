package infracli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// The first free slot, taken in order and in silence, is a decision about
// capacity and cost wearing the clothes of an implementation detail: a production
// account set up first in a fresh checkout got dev — db.t4g.micro, single-AZ, one
// day of backups, no deletion protection.
func TestTheEnvironmentIsAskedForWhenAnAccountIsSetUp(t *testing.T) {
	layout := checkoutLayout(t)

	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{account: "111122223333", profile: "p"}

	previous := runInitCommand
	runInitCommand = func(_ context.Context, _ []string, _, _ io.Writer) error {
		writeEnvConfig(t, layout.Root, map[string]string{
			"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = p",
		})
		return nil
	}
	t.Cleanup(func() { runInitCommand = previous })

	asked, err := askEnvironmentStep(context.Background(), ask, infra.Catalog{}, layout, &opts)
	if err != nil {
		t.Fatalf("askEnvironmentStep = %v\n%s", err, painted.String())
	}

	if !asked {
		t.Error("the environment was decided without asking")
	}
	if opts.environment != "dev" {
		t.Errorf("enter took %q, want the first environment", opts.environment)
	}
	if !strings.Contains(painted.String(), "Which environment is this account?") {
		t.Errorf("the question was not asked:\n%s", painted.String())
	}
	// The sizing is the reason to choose one over another, so it has to be on the
	// row rather than in documentation.
	for _, want := range []string{"cheapest", "multi-AZ"} {
		if !strings.Contains(painted.String(), want) {
			t.Errorf("the rows do not say what the choice costs (%q):\n%s", want, painted.String())
		}
	}
}

// A slot already configured belongs to a different account, and a checkout holds
// one account per environment. Shown rather than hidden: its absence would be the
// next question somebody asks.
func TestATakenEnvironmentIsShownAndCannotBeChosen(t *testing.T) {
	layout := checkoutLayout(t)
	writeEnvConfig(t, layout.Root, map[string]string{
		"dev": "account_id = 905418424496\nregion = us-east-1\nprofile = other",
	})

	options := environmentChoices(layout, "111122223333")

	if options[0].value != "dev" {
		t.Fatalf("the first row is %q", options[0].value)
	}
	if !options[0].disabled {
		t.Error("an environment another account holds can be chosen")
	}
	if !strings.Contains(options[0].note, "905418424496") {
		t.Errorf("the row does not say who holds it: %q", options[0].note)
	}
	// And the cursor starts on one that can be answered.
	if at := firstEnabled(options); options[at].value != "stg" {
		t.Errorf("the cursor starts on %q", options[at].value)
	}
}

// With all three taken there is no question to ask, and the error has to say why
// rather than offering a menu of three refusals.
func TestAFullCheckoutIsAnErrorRatherThanAMenu(t *testing.T) {
	layout := checkoutLayout(t)
	writeEnvConfig(t, layout.Root, map[string]string{
		"dev": "account_id = 111111111111\nregion = us-east-1\nprofile = a",
		"stg": "account_id = 222222222222\nregion = us-east-1\nprofile = b",
		"prd": "account_id = 333333333333\nregion = us-east-1\nprofile = c",
	})

	ask, _ := selectorFor(t, keyEnterSeq)
	opts := options{account: "444444444444", profile: "d"}
	_, err := askEnvironmentStep(context.Background(), ask, infra.Catalog{}, layout, &opts)

	if err == nil {
		t.Fatal("a fourth account was offered an environment")
	}
	if !strings.Contains(err.Error(), "three") || !strings.Contains(err.Error(), "444444444444") {
		t.Errorf("the error does not explain the limit or name the account: %v", err)
	}
}

// The notes describe what the templates ship, and templates change. This reads
// the examples and fails when the description stops being true — which is the
// cost of writing it in Go instead of parsing HCL in front of a menu.
func TestTheNotesMatchWhatTheTemplatesShip(t *testing.T) {
	checkout := os.Getenv("LERIAN_TF_REPO")
	if checkout == "" {
		checkout = filepath.Join(os.Getenv("HOME"), ".lerian", "lerian-terraform-foundation")
	}
	sample := filepath.Join(checkout, "examples", "aws", "products", "midaz", "postgres", "envs")
	if _, err := os.Stat(sample); err != nil {
		t.Skip("no checkout to read the templates from")
	}

	for _, test := range []struct {
		env      string
		contains []string
	}{
		{"dev", []string{"multi_az              = false", "deletion_protection     = false"}},
		{"prd", []string{"multi_az              = true", "deletion_protection     = true"}},
	} {
		body, err := os.ReadFile(filepath.Join(sample, test.env+".tfvars-example"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range test.contains {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s no longer ships %q, so the menu note for it is wrong:\n  %q",
					test.env, want, environmentNotes[test.env])
			}
		}
	}
}

// checkoutLayout is a checkout with no environment configured in it.
func checkoutLayout(t *testing.T) infra.Layout {
	t.Helper()
	layout, err := infra.NewLayout(fakeCheckout(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

// An account is not an environment. Deriving one from the other meant an account
// could only ever be one: a sandbox configured as dev could never also be stg in
// the same checkout, because the lookup found dev and stopped.
func TestAnAccountCanBeMoreThanOneEnvironment(t *testing.T) {
	layout := checkoutLayout(t)
	writeEnvConfig(t, layout.Root, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = same",
	})

	options := environmentChoices(layout, "111122223333")

	if options[0].value != "dev" || options[0].disabled {
		t.Errorf("the environment this account already has is not offered: %+v", options[0])
	}
	// And the free ones are offered to the same account, which is the case that
	// was unreachable before.
	for _, opt := range options[1:] {
		if opt.disabled {
			t.Errorf("%q is not offered to an account that already has one environment: %q",
				opt.value, opt.note)
		}
	}
}

// The row says what this environment is for THIS account, because that decides
// what happens next: configured means the run goes straight on, free means init
// runs first, held means not in this checkout.
func TestEachEnvironmentSaysWhereItStands(t *testing.T) {
	layout := checkoutLayout(t)
	writeEnvConfig(t, layout.Root, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = mine",
		"stg": "account_id = 905418424496\nregion = us-east-1\nprofile = theirs",
	})

	byName := map[string]option{}
	for _, opt := range environmentChoices(layout, "111122223333") {
		byName[opt.value] = opt
	}

	if !strings.Contains(byName["dev"].note, "configured here") {
		t.Errorf("dev: %q", byName["dev"].note)
	}
	if !strings.Contains(byName["stg"].note, "905418424496") || !byName["stg"].disabled {
		t.Errorf("stg is not reported as held by the other account: %+v", byName["stg"])
	}
	if !strings.Contains(byName["prd"].note, "not set up here yet") {
		t.Errorf("prd: %q", byName["prd"].note)
	}
}

// Whether there is a state backend is the difference between a run that can do
// anything and one that can only bootstrap, so it belongs on the row.
func TestTheRowSaysWhetherTheBackendIsReady(t *testing.T) {
	layout := checkoutLayout(t)
	writeEnvConfig(t, layout.Root, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = mine",
	})

	before := environmentChoices(layout, "111122223333")[0]
	if !strings.Contains(before.note, "no state backend yet") {
		t.Errorf("note = %q", before.note)
	}

	writeBackendFile(t, layout.Root, "dev")

	after := environmentChoices(layout, "111122223333")[0]
	if !strings.Contains(after.note, "state backend ready") {
		t.Errorf("note = %q", after.note)
	}
}

// The dry-run path asks from the file, because no credentials were resolved and
// the sections are all there is to read. It has already answered this, so asking
// again would put the same question on screen twice in one run.
func TestAnEnvironmentAlreadyDecidedIsNotAskedAgain(t *testing.T) {
	layout := checkoutLayout(t)
	writeEnvConfig(t, layout.Root, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = mine",
	})

	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{environment: "dev"}

	asked, err := askEnvironmentStep(context.Background(), ask, infra.Catalog{}, layout, &opts)
	if err != nil {
		t.Fatalf("askEnvironmentStep = %v", err)
	}

	if asked {
		t.Error("the environment was asked for a second time")
	}
	if strings.Contains(painted.String(), "Which environment") {
		t.Errorf("the menu was painted anyway:\n%s", painted.String())
	}
	if opts.environment != "dev" {
		t.Errorf("environment = %q, want the one already decided", opts.environment)
	}
}
