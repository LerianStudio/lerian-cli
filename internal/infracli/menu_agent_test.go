package infracli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flag"
	"github.com/lerian-studio/lerian-cli/internal/infra"
	"io"
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

	if base >= agent || agent >= product || product >= all {
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

// presetHas reports whether a target arrives ticked.
func presetHas(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// agentPreset is what the targets question arrives preticked with.
func agentPreset(t *testing.T, layout infra.Layout, target string) []string {
	t.Helper()
	// No --target typed: the preset is whatever the flag's default produced.
	opts := &options{target: target, environment: "dev"}
	return withAgent(splitList(target), infra.Catalog{}, layout, opts)
}

// agentPresetWithFlag is the same question when --target was actually passed.
func agentPresetWithFlag(t *testing.T, layout infra.Layout, target string) []string {
	t.Helper()
	opts := &options{target: target, environment: "dev", targetGiven: true}
	return withAgent(splitList(target), infra.Catalog{}, layout, opts)
}

// configuredAgent is a checkout whose agent root has variables for dev, which
// is what makes it runnable.
func configuredAgent(t *testing.T) infra.Layout {
	t.Helper()
	layout := checkoutWithAgent(t)
	if err := os.MkdirAll(filepath.Join(layout.AgentDir(), "envs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.AgentDir(), "envs", "dev.tfvars"),
		[]byte("agent_token = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return layout
}

// A cluster without the agent cannot be reached by the control plane, so the
// row arrives ticked rather than waiting to be noticed.
func TestAgentIsPreselectedByDefault(t *testing.T) {
	preset := agentPreset(t, configuredAgent(t), defaultTarget)

	if !presetHas(preset, "agent") {
		t.Errorf("the agent is not preselected: %v", preset)
	}
	if !presetHas(preset, "infra-base") {
		t.Errorf("and it must not displace what was already ticked: %v", preset)
	}
}

// Ticking a target with no tfvars stops the run before Terraform starts, so
// preselecting an unconfigured agent would turn a working deploy of the base
// into a refusal.
func TestUnconfiguredAgentIsNotPreselected(t *testing.T) {
	preset := agentPreset(t, checkoutWithAgent(t), defaultTarget)

	if presetHas(preset, "agent") {
		t.Errorf("an agent with no variables must not be preticked: %v", preset)
	}
}

// Somebody who names what to run has said what they want.
func TestExplicitTargetIsNotExtendedWithTheAgent(t *testing.T) {
	preset := agentPresetWithFlag(t, configuredAgent(t), "midaz")

	if presetHas(preset, "agent") {
		t.Errorf("an explicit --target must be left alone: %v", preset)
	}
}

// The case a comparison against the default value cannot see: "--target
// infra-base" produces the same preset as typing nothing, and silently
// deploying an agent nobody asked for is the opposite of what the flag said.
func TestTargetInfraBaseTypedOnPurposeIsNotExtended(t *testing.T) {
	preset := agentPresetWithFlag(t, configuredAgent(t), defaultTarget)

	if presetHas(preset, "agent") {
		t.Errorf("--target infra-base was typed; the agent must not be added: %v", preset)
	}
	if !presetHas(preset, "infra-base") {
		t.Errorf("and what was asked for must survive: %v", preset)
	}
}

// A checkout without the root has nothing to tick.
func TestNoAgentRootMeansNoPreselection(t *testing.T) {
	preset := agentPreset(t, infra.Layout{Root: t.TempDir()}, defaultTarget)

	if presetHas(preset, "agent") {
		t.Errorf("a checkout without the root must not pretick it: %v", preset)
	}
}

// "agent,agent" would reach Resolve as two stages over one root. It cannot
// happen because a preset naming the agent is an explicit target and is left
// alone — this pins that reasoning, which is what lets withAgent append
// without checking.
func TestAgentIsNotPreselectedTwice(t *testing.T) {
	preset := agentPresetWithFlag(t, configuredAgent(t), "infra-base,agent")

	var count int
	for _, name := range preset {
		if name == "agent" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the agent appears %d times: %v", count, preset)
	}
}

// Driven through askTargetStep rather than through withAgent, because a test of
// the helper proves the helper works and says nothing about whether the step
// calls it — which is exactly the gap a mutation that deleted the call walked
// straight through.
func TestTheTargetQuestionArrivesWithTheAgentTicked(t *testing.T) {
	layout := configuredLayout(t)
	agent := filepath.Join(layout.AgentDir(), "envs")
	if err := os.MkdirAll(agent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.AgentDir(), "main.tf"), []byte("# root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "dev.tfvars"), []byte("agent_token = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{environment: "dev", target: defaultTarget}

	if _, err := askTargetStep(context.Background(), ask, catalog, layout, &opts); err != nil {
		t.Fatalf("askTargetStep = %v\n%s", err, painted.String())
	}

	screen := painted.String()
	if !strings.Contains(screen, "[x] agent") {
		t.Errorf("the agent row is not ticked when the question is asked:\n%s", screen)
	}
	if !strings.Contains(screen, "[x] infra-base") {
		t.Errorf("and infra-base must stay ticked beside it:\n%s", screen)
	}
}

// What a flag's value cannot tell you: whether anybody typed it.
//
// This covers the reading, not the wiring. That run() still calls it is not
// asserted here — reaching the preset through run() needs a terminal, which the
// test binary does not have — so a mutation deleting the call survives. The
// behavior it guards is the one above: with targetGiven set, the agent is not
// added.
func TestOnlyATypedTargetIsRecorded(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"typed", []string{"--target", "infra-base"}, true},
		{"typed as the default value", []string{"--target", defaultTarget}, true},
		{"not typed", []string{"--env", "dev"}, false},
		{"nothing at all", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var opts options
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			flags.SetOutput(io.Discard)
			flags.StringVar(&opts.target, "target", defaultTarget, "")
			flags.StringVar(&opts.environment, "env", "", "")
			if err := flags.Parse(tc.args); err != nil {
				t.Fatal(err)
			}

			recordTypedFlags(flags, &opts)

			if opts.targetGiven != tc.want {
				t.Errorf("targetGiven = %v, want %v (args %v)", opts.targetGiven, tc.want, tc.args)
			}
		})
	}
}
