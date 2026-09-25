package infra

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
)

// secretInState is what Terraform hands back unredacted in both JSON documents:
// show -json carries planned_values and prior state, output -json carries the
// values themselves. Neither is redacted, whatever the root marked sensitive.
const secretInState = "scram-password-do-not-log-me"

// stubTerraform writes a script that answers the three subcommands these tests
// reach, printing the JSON on stdout exactly as terraform does.
func stubTerraform(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "terraform")
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    version) echo '{"terraform_version":"1.9.0","platform":"test","provider_selections":{},"terraform_outdated":false}'; exit 0 ;;
    show)    echo '{"format_version":"1.2","planned_values":{"root_module":{"resources":[{"address":"random_password.scram","values":{"result":"` + secretInState + `"}}]}},"resource_changes":[]}'; exit 0 ;;
    output)  echo '{"db_password":{"sensitive":true,"type":"string","value":"` + secretInState + `"}}'; exit 0 ;;
  esac
done
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// cliWithLog returns a CLI whose unit log is a real file, like a run has.
func cliWithLog(t *testing.T) (*CLI, Unit, *FileLogs, string) {
	t.Helper()

	logDir := t.TempDir()
	logs := NewFileLogs(logDir)
	t.Cleanup(func() { _ = logs.Close() })

	unit := Unit{Name: "shared-resources/msk", Dir: t.TempDir()}
	cli := &CLI{
		execPath: stubTerraform(t),
		Logs:     logs.Writer,
		byDir:    map[string]*tfexec.Terraform{},
	}
	return cli, unit, logs, logs.Path(unit)
}

// The defect this guards: tfexec's runTerraformCmdJSON merges the configured
// stdout with its own capture buffer, so every byte of `terraform show -json`
// used to land in the unit's log — a file the CLI keeps on purpose and prints the
// path to. Any root that generates a credential leaked it there on every plan.
func TestShowPlanKeepsThePlanJSONOutOfTheLog(t *testing.T) {
	cli, unit, logs, logPath := cliWithLog(t)

	planFile := filepath.Join(unit.Dir, "plan.tfplan")
	if err := os.WriteFile(planFile, []byte("saved plan"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := cli.ShowPlan(context.Background(), unit, planFile); err != nil {
		t.Fatalf("ShowPlan: %v", err)
	}
	if err := logs.Close(); err != nil {
		t.Fatal(err)
	}

	assertLogHasNoSecret(t, logPath)
}

// Same defect, the other JSON-producing call.
func TestOutputKeepsTheOutputJSONOutOfTheLog(t *testing.T) {
	cli, unit, logs, logPath := cliWithLog(t)

	values, err := cli.Output(context.Background(), unit)
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	// The value still has to reach the caller: this is about the log, not about
	// blinding the CLI.
	if _, ok := values["db_password"]; !ok {
		t.Fatalf("Output dropped the value it was asked for: %v", values)
	}
	if err := logs.Close(); err != nil {
		t.Fatal(err)
	}

	assertLogHasNoSecret(t, logPath)
}

// The log must keep working for everything else, or the fix would have traded a
// leak for a blind operator.
func TestTheLogStillReceivesNonJSONOutput(t *testing.T) {
	cli, unit, logs, logPath := cliWithLog(t)

	client, err := cli.terraform(unit)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Version(context.Background(), true); err != nil {
		t.Fatalf("Version: %v", err)
	}
	// A JSON call in between must leave the writer as it found it.
	if _, err := cli.Output(context.Background(), unit); err != nil {
		t.Fatalf("Output: %v", err)
	}
	if _, err := io.WriteString(cli.logWriter(unit), "plan output the operator reads\n"); err != nil {
		t.Fatal(err)
	}
	if err := logs.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "plan output the operator reads") {
		t.Fatalf("the log lost the output an operator needs:\n%s", content)
	}
}

func assertLogHasNoSecret(t *testing.T, logPath string) {
	t.Helper()

	content, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return // nothing was written at all, which is stricter than required
	}
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), secretInState) {
		t.Fatalf("the unit log carries a value read from state:\n%s", content)
	}
}
