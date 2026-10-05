package infracli

import (
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
	got, err := askForEnvironment(ask, layout, "111122223333")
	if err != nil {
		t.Fatalf("askForEnvironment = %v", err)
	}

	if got != "dev" {
		t.Errorf("enter took %q, want the first environment", got)
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

	options := freeEnvironmentOptions(layout)

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
	_, err := askForEnvironment(ask, layout, "444444444444")

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
