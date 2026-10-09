package infracli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// checkoutWithAgent is a layout whose agent root exists, which is what the rows
// below are conditional on.
func checkoutWithAgent(t *testing.T) infra.Layout {
	t.Helper()
	layout := infra.Layout{Root: t.TempDir()}
	if err := os.MkdirAll(layout.AgentDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.AgentDir(), "main.tf"), []byte("# root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return layout
}

func rowValues(options []option) []string {
	values := make([]string, 0, len(options))
	for _, opt := range options {
		values = append(values, opt.value)
	}
	return values
}

// The row the whole feature is for: installing the agent has to be choosable
// from the list, not only by typing --target.
func TestRunMenuOffersTheAgent(t *testing.T) {
	layout := checkoutWithAgent(t)

	rows := rowValues(runTargetOptions(infra.Catalog{}, layout, "dev"))

	var found bool
	for _, value := range rows {
		if value == "agent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the run menu has no agent row: %v", rows)
	}
}

// And it must not be offered by a checkout that cannot run it: the row would
// resolve to a directory that is not there.
func TestRunMenuHidesTheAgentWithoutTheRoot(t *testing.T) {
	layout := infra.Layout{Root: t.TempDir()}

	for _, value := range rowValues(runTargetOptions(infra.Catalog{}, layout, "dev")) {
		if value == "agent" {
			t.Error("a checkout without the agent root must not offer the row")
		}
	}
}

// init is where tfvars are written. Without the row there, the agent could be
// selected in the run menu and would never be anything but "not configured".
func TestInitOffersTheAgentToConfigure(t *testing.T) {
	layout := checkoutWithAgent(t)

	rows := rowValues(targetOptions(infra.Catalog{}, layout))

	var found bool
	for _, value := range rows {
		if value == "agent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("init cannot configure the agent: %v", rows)
	}
}

// Order carries meaning in this list: the agent installs into the cluster
// infra-base builds, so it reads below it and above the products.
func TestAgentRanksBetweenInfraBaseAndTheProducts(t *testing.T) {
	configured := map[string]bool{"midaz": true}

	base := rankTarget(option{value: "infra-base"}, configured)
	agent := rankTarget(option{value: "agent"}, configured)
	product := rankTarget(option{value: "midaz"}, configured)
	all := rankTarget(option{value: "all"}, configured)

	if !(base < agent && agent < product && product < all) {
		t.Errorf("ranks out of order: infra-base=%d agent=%d midaz=%d all=%d",
			base, agent, product, all)
	}
}

// --list is how somebody finds out what they can type. A target missing from it
// exists only for whoever already knew about it.
func TestListNamesTheAgentTarget(t *testing.T) {
	var out strings.Builder
	printTargets(&out, checkoutWithAgent(t), infra.Catalog{})

	if !strings.Contains(out.String(), "\n  agent ") {
		t.Errorf("--list does not name the agent target on a row of its own:\n%s", out.String())
	}
}

// And a checkout that cannot run it must not advertise it.
func TestListOmitsTheAgentWithoutTheRoot(t *testing.T) {
	var out strings.Builder
	printTargets(&out, infra.Layout{Root: t.TempDir()}, infra.Catalog{})

	if strings.Contains(out.String(), "\n  agent ") {
		t.Errorf("--list names a target this checkout cannot run:\n%s", out.String())
	}
}
