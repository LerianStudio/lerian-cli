package infracli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

type fakeZones struct {
	zones []infra.HostedZone
	err   error
	calls int
}

func (f *fakeZones) ListHostedZones(context.Context, string) ([]infra.HostedZone, error) {
	f.calls++
	return f.zones, f.err
}

// ExternalDNS and cert-manager publish records the internet has to resolve. A
// private zone would be accepted by IAM and then never serve the record anybody
// was waiting for.
func TestOnlyPublicZonesAreOffered(t *testing.T) {
	options := hostedZoneOptions([]infra.HostedZone{
		{Name: "sandbox.example.io", ID: "Z1", Private: false},
		{Name: "internal.example", ID: "Z2", Private: true},
	})

	if len(options) != 1 {
		t.Fatalf("offered %d zones, want the public one alone: %+v", len(options), options)
	}
	if options[0].value != "arn:aws:route53:::hostedzone/Z1" {
		t.Errorf("value = %q, want the ARN an IAM policy wants", options[0].value)
	}
	if options[0].label != "sandbox.example.io" {
		t.Errorf("label = %q, want the domain somebody recognizes", options[0].label)
	}
}

// The templates are a separate repository on their own release cycle. Matching a
// fixed list of token spellings would stop helping the day they add
// <ROUTE53-PUBLIC-ZONE-ARN>.
func TestZoneTokensAreRecognizedByTheirWords(t *testing.T) {
	for _, token := range []string{"<ROUTE53-ZONE-ARN>", "<ROUTE53-PUBLIC-ZONE-ARN>", "<route53-zone-id>"} {
		if !isHostedZoneToken(token) {
			t.Errorf("%q was not recognized as a hosted zone", token)
		}
	}
	for _, token := range []string{"<VPC-ID>", "<YOUR-EGRESS-CIDR>", "<ACCOUNT-ID>"} {
		if isHostedZoneToken(token) {
			t.Errorf("%q was taken for a hosted zone", token)
		}
	}
}

// Asking again for something already answered is how a wizard becomes something
// to click through.
func TestAlreadyAnsweredTokensAreNotAskedAgain(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	unit := writeExampleWithTokens(t, layout, "<ROUTE53-ZONE-ARN>", "<SOMETHING-ELSE>")

	pending, err := pendingTokens([]infra.Unit{unit}, "dev",
		map[string]string{"<ROUTE53-ZONE-ARN>": "arn:already:answered"})
	if err != nil {
		t.Fatal(err)
	}

	if len(pending) != 1 || pending[0] != "<SOMETHING-ELSE>" {
		t.Errorf("pending = %v, want only the unanswered one", pending)
	}
}

// The egress address has a question of its own, asked before this runs.
func TestTheEgressTokenIsNotAskedForHere(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	egress := infra.EgressIPPlaceholders[0]
	unit := writeExampleWithTokens(t, layout, egress)

	pending, err := pendingTokens([]infra.Unit{unit}, "dev", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %v, want the egress token left to its own question", pending)
	}
}

// A lookup that fails is a convenience that did not work, not a setup that
// cannot continue: the value is still typeable.
func TestAFailedZoneLookupFallsBackToTyping(t *testing.T) {
	var out bytes.Buffer
	ask := &prompter{interactive: true, out: &out}
	zones := &fakeZones{err: errors.New("no permission")}

	value, ok, err := askForHostedZones(context.Background(), ask, zones, "<ROUTE53-ZONE-ARN>", "p")

	if err != nil {
		t.Fatalf("askForHostedZones = %v, want a fallback rather than a failure", err)
	}
	if ok || value != "" {
		t.Errorf("it reported an answer it does not have: %q", value)
	}
	if !strings.Contains(out.String(), "could not list the hosted zones") {
		t.Errorf("the failure is not reported:\n%s", out.String())
	}
}

// An account with only private zones has nothing to offer, and an empty menu is
// worse than being asked to type.
func TestNoPublicZoneFallsBackToTyping(t *testing.T) {
	var out bytes.Buffer
	ask := &prompter{interactive: true, out: &out}
	zones := &fakeZones{zones: []infra.HostedZone{{Name: "internal", ID: "Z9", Private: true}}}

	_, ok, err := askForHostedZones(context.Background(), ask, zones, "<ROUTE53-ZONE-ARN>", "p")

	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a private zone was offered for a public record")
	}
	if !strings.Contains(out.String(), "no public hosted zone") {
		t.Errorf("it does not say why nothing was offered:\n%s", out.String())
	}
}

// Nobody to ask means the token is written as it is, which is what happened
// before this existed — and --set is how a scripted run answers it.
func TestWithNoTerminalNothingIsAsked(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	unit := writeExampleWithTokens(t, layout, "<ROUTE53-ZONE-ARN>")

	zones := &fakeZones{}
	replacements := map[string]string{}
	ask := &prompter{interactive: false, out: &bytes.Buffer{}}

	if err := resolvePlaceholders(context.Background(), ask, zones,
		[]infra.Unit{unit}, "dev", "p", replacements); err != nil {
		t.Fatal(err)
	}

	if zones.calls != 0 {
		t.Errorf("the account was called %d times with nobody to answer", zones.calls)
	}
	if len(replacements) != 0 {
		t.Errorf("a value was invented: %v", replacements)
	}
}

// writeExampleWithTokens makes a root whose committed example still carries the
// given tokens, which is what PlaceholdersIn reads.
func writeExampleWithTokens(t *testing.T, layout infra.Layout, tokens ...string) infra.Unit {
	t.Helper()

	dir := filepath.Join(layout.ProductsDir(), "sample", "component")
	if err := os.MkdirAll(filepath.Join(dir, "envs"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "region = \"us-east-1\"\n"
	for index, token := range tokens {
		body += fmt.Sprintf("value_%d = [\"%s\"]\n", index, token)
	}
	unit := infra.Unit{Name: "sample/component", Dir: dir}
	if err := os.WriteFile(infra.VarFile(unit, "dev")+"-example", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return unit
}
