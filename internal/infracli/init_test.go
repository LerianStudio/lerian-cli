package infracli

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// isolateAWS points the AWS CLI at empty configuration.
//
// Every test here passes --profile acme, and resolveCredentials calls
// CLIIdentity.CallerIdentity whenever a profile is named — --account only rescues
// the run after that call fails. So the comment claiming "no STS call" was wrong,
// and on a machine or runner that happens to have a profile named acme the lookup
// SUCCEEDS, returns a different account, and buildInitPlan fails on the mismatch.
// Empty files make the failure deterministic everywhere.
func isolateAWS(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, pair := range [][2]string{
		{"AWS_CONFIG_FILE", filepath.Join(dir, "config")},
		{"AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials")},
	} {
		if err := os.WriteFile(pair[1], nil, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(pair[0], pair[1])
	}
	// Neither an ambient region nor ambient keys may leak in.
	for _, name := range []string{
		"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		// The key variables are not the only source the AWS CLI tries. On an EC2
		// or EKS runner it falls through to the instance role, the container
		// credential endpoint and web identity, and then a test that passes
		// --profile '' resolves real credentials: resolveCredentials calls STS
		// whenever the profile is set, and buildInitPlan fails on the account
		// mismatch. Tests with a named profile stall on IMDS timeouts instead.
		// This is why it has not bitten us: CI runs on Blacksmith, not EC2.
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN",
		"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN",
	} {
		t.Setenv(name, "")
	}
	// IMDS is reached by address, not by variable, so emptying something does not
	// switch it off. This is the documented kill switch.
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// initCheckout builds a repository with committed examples and no real config,
// which is what a client's first clone looks like.
func initCheckout(t *testing.T) string {
	t.Helper()
	isolateAWS(t)
	root := t.TempDir()

	mkExample := func(dir, env, body string) {
		full := filepath.Join(root, "examples", "aws", dir, "envs")
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, env+".tfvars-example"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		main := filepath.Join(root, "examples", "aws", dir, "main.tf")
		if err := os.WriteFile(main, []byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "examples", "aws", "_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "examples", "aws", "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	mkExample("infra-base/vpc", "dev", "cidr = \"10.50.0.0/16\"\n")
	mkExample("infra-base/eks", "dev", "cidrs = [\"<PUT-YOUR-EGRESS-IP-HERE>/32\"]\n")
	mkExample("products/midaz/valkey", "dev", "node_type = \"cache.t4g.micro\"\n")
	return root
}

// Outside a terminal, a missing decision must name the flag rather than default.
// The environment is the one value with no safe default: it decides which account
// the guard will demand.
func TestInitWithoutTerminalRequiresEnv(t *testing.T) {
	root := initCheckout(t)
	var out, errOut bytes.Buffer

	err := runInit(context.Background(),
		[]string{"--repo", root}, &out, &errOut)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "--env is required") {
		t.Errorf("error should name --env, got: %v", err)
	}
}

func TestInitWritesConfigAndVarFiles(t *testing.T) {
	root := initCheckout(t)
	var out, errOut bytes.Buffer

	// --account makes the STS call fail deterministically, which is what lets this run in CI.
	err := runInit(context.Background(), []string{
		"--repo", root,
		"--env", "dev",
		"--profile", "acme",
		"--account", "123456789012",
		"--region", "us-east-2",
		"--targets", "infra-base,midaz",
		"--api-cidr", "203.0.113.7",
		"--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	config, err := os.ReadFile(filepath.Join(root, "examples", "aws", "environments.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[dev]", "123456789012", "acme", "us-east-2"} {
		if !strings.Contains(string(config), want) {
			t.Errorf("environments.conf missing %q:\n%s", want, config)
		}
	}

	eks, err := os.ReadFile(filepath.Join(root, "examples", "aws", "infra-base", "eks", "envs", "dev.tfvars"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(eks), "203.0.113.7/32") {
		t.Errorf("the egress placeholder was not filled:\n%s", eks)
	}

	// The product named in --targets must be materialized too, not just infra-base.
	if _, err := os.Stat(filepath.Join(root, "examples", "aws", "products", "midaz", "valkey", "envs", "dev.tfvars")); err != nil {
		t.Errorf("the product's tfvars was not written: %v", err)
	}
}

func TestInitIsIdempotentAndRefusesToClobber(t *testing.T) {
	root := initCheckout(t)
	args := []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "infra-base", "--api-cidr", "203.0.113.7", "--auto-approve",
	}

	var out, errOut bytes.Buffer
	if err := runInit(context.Background(), args, &out, &errOut); err != nil {
		t.Fatalf("first run: %v", err)
	}

	out.Reset()
	if err := runInit(context.Background(), args, &out, &errOut); err != nil {
		t.Fatalf("re-running with the same inputs must be safe: %v", err)
	}
	if !strings.Contains(out.String(), "nothing to do") {
		t.Errorf("second run should be a no-op, got:\n%s", out.String())
	}

	// Pointing the same environment at a different account is the dangerous edit.
	moved := append([]string{}, args...)
	for i, a := range moved {
		if a == "123456789012" {
			moved[i] = "210987654321"
		}
	}
	out.Reset()
	err := runInit(context.Background(), moved, &out, &errOut)
	if err == nil {
		t.Fatal("repointing an environment at another account must not be silent")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the error should say how to proceed deliberately, got: %v", err)
	}
	config, _ := os.ReadFile(filepath.Join(root, "examples", "aws", "environments.conf"))
	if strings.Contains(string(config), "210987654321") {
		t.Error("a refused write must leave the file untouched")
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	root := initCheckout(t)
	var out, errOut bytes.Buffer

	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "infra-base", "--api-cidr", "203.0.113.7", "--dry-run",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out.String(), "dry run") {
		t.Errorf("output should say it wrote nothing:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "examples", "aws", "environments.conf")); !os.IsNotExist(err) {
		t.Error("dry run wrote environments.conf")
	}
}

// A malformed --account is rejected before anything is written: a config carrying
// "12345" produces a guard that refuses every later run, and the operator finds out
// at apply time instead of now.
//
// This is the FORMAT check, not the mismatch check. A real mismatch — a well-formed
// account the credentials do not reach — needs a caller identity to compare against,
// and runInit resolves that through the live AWS CLI. It is covered where it can be
// driven deterministically, with a stub identity:
// pkg/infra TestVerifyAccountRefusesTheWrongAccount.
func TestInitRejectsAMalformedAccountID(t *testing.T) {
	root := initCheckout(t)
	var out, errOut bytes.Buffer

	// --profile is named on purpose: without it this run now fails at "no --profile
	// given", which is a different refusal, and the test would pass while proving
	// nothing about the account.
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "12345",
		"--region", "us-east-2", "--targets", "infra-base",
		"--api-cidr", "203.0.113.7", "--auto-approve",
	}, &out, &errOut)
	if err == nil {
		t.Fatal("a malformed account id must be rejected")
	}
	if !strings.Contains(err.Error(), "12345") {
		t.Errorf("the refusal must quote the id it rejected:\n%v", err)
	}
	if strings.Contains(err.Error(), "no --profile given") {
		t.Errorf("this test must reach the account check, not stop before it:\n%v", err)
	}
}

func TestInitSetFillsArbitraryPlaceholder(t *testing.T) {
	root := initCheckout(t)
	// A token this build has no special knowledge of.
	dir := filepath.Join(root, "examples", "aws", "products", "midaz", "valkey", "envs")
	if err := os.WriteFile(filepath.Join(dir, "dev.tfvars-example"),
		[]byte("name = \"<PUT-YOUR-CLUSTER-NAME-HERE>\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "midaz",
		"--set", "<PUT-YOUR-CLUSTER-NAME-HERE>=midaz-dev",
		"--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}
	written, err := os.ReadFile(filepath.Join(dir, "dev.tfvars"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "midaz-dev") {
		t.Errorf("--set did not fill the token:\n%s", written)
	}
}

// Mixing dedicated and shared inside one product is out of scope for now, so the
// mode is one decision for the whole target rather than one per datastore.
func TestInitAppliesOneModeToEveryDatastore(t *testing.T) {
	root := initCheckout(t)
	// Two datastores under one product, both carrying the switch, plus the tier
	// roots they resolve in shared mode.
	for _, name := range []string{"valkey", "postgres"} {
		td := filepath.Join(root, "examples", "aws", "products", "shared-resources", name)
		if err := os.MkdirAll(filepath.Join(td, "envs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(td, "envs", "dev.tfvars-example"), []byte("size = 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(td, "main.tf"), []byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "examples", "aws", "products", "midaz", name, "envs")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "dev.tfvars-example"),
			[]byte("mode = \"dedicated\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "examples", "aws", "products", "midaz", name, "main.tf"),
			[]byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "midaz", "--mode", "shared", "--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	for _, name := range []string{"valkey", "postgres"} {
		path := filepath.Join(root, "examples", "aws", "products", "midaz", name, "envs", "dev.tfvars")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `mode = "shared"`) {
			t.Errorf("%s was not switched to shared:\n%s", name, content)
		}
	}

	// Shared mode creates nothing and resolves a tier that no product target
	// applies. Saying so here is the difference between a clear instruction and a
	// bare "not found" from a data source ten minutes later.
	if !strings.Contains(out.String(), "shared-resources/valkey") {
		t.Errorf("the notice should name the tier to apply first:\n%s", out.String())
	}
}

func TestInitRejectsUnknownMode(t *testing.T) {
	root := initCheckout(t)
	var out, errOut bytes.Buffer

	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "infra-base", "--api-cidr", "203.0.113.7",
		"--mode", "hybrid", "--auto-approve",
	}, &out, &errOut)
	if err == nil {
		t.Fatal("an unknown mode must be refused")
	}
	if !strings.Contains(err.Error(), "dedicated") {
		t.Errorf("the error should list valid values, got: %v", err)
	}
}

// init must configure bootstrap even when nobody named it. It is the first command
// anybody runs, and leaving it out made the golden path — init, then bootstrap —
// fail on its second step with a missing tfvars this tool had just had the chance
// to write.
func TestInitAlsoConfiguresBootstrap(t *testing.T) {
	root := initCheckout(t)
	dir := filepath.Join(root, "examples", "aws", "bootstrap", "envs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dev.tfvars-example"),
		[]byte("environment = \"dev\"\nregion = \"us-east-1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "infra-base", "--api-cidr", "203.0.113.7", "--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	written, err := os.ReadFile(filepath.Join(dir, "dev.tfvars"))
	if err != nil {
		t.Fatalf("bootstrap tfvars was not written: %v", err)
	}
	// The region retarget has to reach it too, or bootstrap creates the state bucket
	// in a different region from everything else.
	if !strings.Contains(string(written), `region = "us-east-2"`) {
		t.Errorf("bootstrap tfvars was not retargeted:\n%s", written)
	}
}

// The shared tier holds datastores. The VPC and the cluster are foundation and are
// shared by definition, so naming "shared-resources/vpc" pointed the operator at a
// root that does not exist.
func TestSharedTierNoticeListsOnlyProductEngines(t *testing.T) {
	root := initCheckout(t)
	for _, name := range []string{"valkey", "postgres"} {
		for _, product := range []string{"midaz", "shared-resources"} {
			dir := filepath.Join(root, "examples", "aws", "products", product, name, "envs")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "dev.tfvars-example"),
				[]byte("mode = \"dedicated\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "examples", "aws", "products", product, name, "main.tf"),
				[]byte("# root\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "infra-base,midaz", "--mode", "shared",
		"--api-cidr", "203.0.113.7", "--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	got := out.String()
	for _, want := range []string{"shared-resources/valkey", "shared-resources/postgres"} {
		if !strings.Contains(got, want) {
			t.Errorf("notice should name %s:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"shared-resources/vpc", "shared-resources/eks"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("%s does not exist and must not be suggested:\n%s", forbidden, got)
		}
	}
}

// Shared mode makes the products resolve a tier they do not create. Configuring
// them without configuring that tier left a checkout that could not be applied,
// and made the operator run init twice to discover it.
func TestInitConfiguresTheSharedTierWithTheProducts(t *testing.T) {
	root := initCheckout(t)

	mk := func(product, service, body string) {
		dir := filepath.Join(root, "examples", "aws", "products", product, service)
		if err := os.MkdirAll(filepath.Join(dir, "envs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "envs", "dev.tfvars-example"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("midaz", "valkey", "mode = \"dedicated\"\n")
	// The tier root owns the instance, so it has no mode of its own.
	mk("shared-resources", "valkey", "node_type = \"cache.t4g.micro\"\n")
	// An engine the product does not use must not be dragged in.
	mk("shared-resources", "msk", "brokers = 3\n")

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "midaz", "--mode", "shared", "--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	tier := filepath.Join(root, "examples", "aws", "products", "shared-resources", "valkey", "envs", "dev.tfvars")
	written, err := os.ReadFile(tier)
	if err != nil {
		t.Fatalf("the tier the products point at was not configured: %v", err)
	}
	// Writing mode here would set a variable the tier root does not declare.
	if strings.Contains(string(written), "mode =") {
		t.Errorf("the tier owns its instance and has no mode to choose:\n%s", written)
	}

	msk := filepath.Join(root, "examples", "aws", "products", "shared-resources", "msk", "envs", "dev.tfvars")
	if _, err := os.Stat(msk); err == nil {
		t.Error("an engine no chosen product uses must not be configured")
	}

	// Applying the tier stays explicit: it is a blast radius, not a side effect.
	if !strings.Contains(out.String(), "shared-resources/valkey --action apply") {
		t.Errorf("the notice must name the apply command:\n%s", out.String())
	}
}

// Dedicated mode must not pull the tier in at all.
func TestInitLeavesTheSharedTierAloneWhenDedicated(t *testing.T) {
	root := initCheckout(t)
	for _, p := range [][2]string{{"midaz", "valkey"}, {"shared-resources", "valkey"}} {
		dir := filepath.Join(root, "examples", "aws", "products", p[0], p[1])
		if err := os.MkdirAll(filepath.Join(dir, "envs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "envs", "dev.tfvars-example"),
			[]byte("mode = \"dedicated\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out, errOut bytes.Buffer
	if err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "midaz", "--mode", "dedicated", "--auto-approve",
	}, &out, &errOut); err != nil {
		t.Fatalf("init: %v", err)
	}

	tier := filepath.Join(root, "examples", "aws", "products", "shared-resources", "valkey", "envs", "dev.tfvars")
	if _, err := os.Stat(tier); err == nil {
		t.Error("dedicated products own their datastores; the tier must not be configured")
	}
}

// A root with no mode switch is not reached by shared mode, and the s3 roots are
// exactly that: a bucket is never shared between products, so the tier has no s3
// root and the product's own root still creates one. The no-op was correct and
// silent, which left the table claiming the root "creates nothing" while it was
// about to create a bucket.
func TestSharedModeNamesTheRootsItDoesNotReach(t *testing.T) {
	root := initCheckout(t)

	mk := func(product, service, body string) {
		dir := filepath.Join(root, "examples", "aws", "products", product, service)
		if err := os.MkdirAll(filepath.Join(dir, "envs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "envs", "dev.tfvars-example"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("reporter", "valkey", "mode = \"dedicated\"\n")
	// No mode line, and deliberately so: this is what an s3 root looks like.
	mk("reporter", "s3", "bucket_suffix = \"objects\"\n")
	mk("shared-resources", "valkey", "node_type = \"cache.t4g.micro\"\n")

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "reporter", "--mode", "shared", "--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}
	got := out.String()

	if !strings.Contains(got, "products/reporter/s3") {
		t.Errorf("the notice must name the root shared mode did not reach:\n%s", got)
	}
	if !strings.Contains(got, "creates its own, not shareable") {
		t.Errorf("the s3 row must not be described as creating nothing:\n%s", got)
	}

	// The switch must be absent from the written file, not set to dedicated: adding
	// a mode line to a root that has no mode variable would fail at plan time.
	written, err := os.ReadFile(filepath.Join(
		root, "examples", "aws", "products", "reporter", "s3", "envs", "dev.tfvars"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "mode") {
		t.Errorf("shared mode must not inject a mode switch into an s3 root:\n%s", written)
	}
}

// The switch is a property of the template, so it is read from the example: the
// real tfvars may not exist yet when the question is asked.
func TestSupportsModeReadsTheExample(t *testing.T) {
	root := initCheckout(t)

	mk := func(service, body string) infra.Unit {
		dir := filepath.Join(root, "examples", "aws", "products", "reporter", service)
		if err := os.MkdirAll(filepath.Join(dir, "envs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "envs", "dev.tfvars-example"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return infra.Unit{Name: "products/reporter/" + service, Dir: dir}
	}

	cases := []struct {
		service string
		body    string
		want    bool
	}{
		{"valkey", "mode = \"dedicated\"\n", true},
		{"s3", "bucket_suffix = \"objects\"\n", false},
		// The switch must not be seen in a comment explaining it, nor in a variable
		// that merely ends in "mode" — valkey carries transit_encryption_mode.
		{"documentdb", "# mode = \"shared\" would resolve the tier\ntransit_encryption_mode = \"preferred\"\n", false},
	}
	for _, tc := range cases {
		unit := mk(tc.service, tc.body)
		got, err := infra.SupportsMode(unit, "dev")
		if err != nil {
			t.Fatalf("%s: %v", tc.service, err)
		}
		if got != tc.want {
			t.Errorf("SupportsMode(%s) = %v, want %v", tc.service, got, tc.want)
		}
	}
}

// The templates append an IPv4 /32 to whatever this value is, so anything that is
// not a bare IPv4 address is written into the tfvars as nonsense and fails at plan
// time, far from the flag that caused it.
func TestAPICIDRTakesABareIPv4Address(t *testing.T) {
	for _, test := range []struct {
		value   string
		wantErr string
	}{
		{"203.0.113.7", ""},
		{"203.0.113.0/24", "not a CIDR block"},
		{"203.0.113.7/32", "not a CIDR block"},
		{"host.example", "IPv4 address"},
		{"2001:db8::1", "IPv4 address"},
		{"", ""},
	} {
		got, err := validateBareAddress(test.value)
		switch {
		case test.wantErr == "" && err != nil:
			t.Errorf("%q: unexpected error %v", test.value, err)
		case test.wantErr == "" && got != test.value:
			t.Errorf("%q: returned %q", test.value, got)
		case test.wantErr != "" && err == nil:
			t.Errorf("%q: expected a refusal", test.value)
		case test.wantErr != "" && !strings.Contains(err.Error(), test.wantErr):
			t.Errorf("%q: error should mention %q:\n%v", test.value, test.wantErr, err)
		}
	}
}

// init reaches AWS on every path — it asks who the operator is, or lists the
// profiles it would ask about — so a missing AWS CLI is reported as itself, not as
// the credential error it used to surface as.
func TestInitWithoutTheAWSCLIExplainsTheDependency(t *testing.T) {
	root := fakeCheckout(t, "", "")
	t.Setenv("PATH", t.TempDir())

	_, stderr, err := runCLI(t, "init", "--repo", root, "--env", "dev",
		"--profile", "lerian-dev", "--region", "us-east-2")
	if err == nil {
		t.Fatal("expected a refusal naming the AWS CLI")
	}
	combined := err.Error() + stderr
	for _, want := range []string{"AWS CLI", "aws configure sso", "aws configure --profile"} {
		if !strings.Contains(combined, want) {
			t.Errorf("the refusal must mention %q:\n%s", want, combined)
		}
	}
}

// --account is the documented way to write configuration on a machine that cannot
// reach AWS, and that has to keep working when the reason it cannot reach AWS is
// that the CLI is not installed at all.
func TestInitWithAnExplicitAccountStillWritesWithoutTheAWSCLI(t *testing.T) {
	root := initCheckout(t)
	t.Setenv("PATH", t.TempDir())

	_, stderr, err := runCLI(t, "init", "--repo", root, "--env", "dev",
		"--profile", "lerian-dev", "--region", "us-east-2",
		"--account", "123456789012", "--targets", "infra-base",
		"--api-cidr", "203.0.113.7", "--auto-approve")
	if err != nil {
		t.Fatalf("--account should not need the AWS CLI: %v\n%s", err, stderr)
	}
	config, readErr := os.ReadFile(filepath.Join(root, "examples", "aws", "environments.conf"))
	if readErr != nil {
		t.Fatalf("the configuration was not written: %v", readErr)
	}
	if !strings.Contains(string(config), "123456789012") {
		t.Errorf("the account passed explicitly is what gets written:\n%s", config)
	}
}

// Without a region there is nothing to infer it from: ~/.aws/config is read by the
// tool that is missing. The refusal has to say so rather than writing a tfvars with
// an empty region.
func TestInitWithoutTheAWSCLIStillDemandsARegion(t *testing.T) {
	root := fakeCheckout(t, "", "")
	t.Setenv("PATH", t.TempDir())

	_, stderr, err := runCLI(t, "init", "--repo", root, "--env", "dev",
		"--profile", "lerian-dev", "--account", "123456789012")
	if err == nil {
		t.Fatal("expected a refusal about the region")
	}
	if !strings.Contains(err.Error()+stderr, "--region") {
		t.Errorf("the refusal must name --region:\n%v\n%s", err, stderr)
	}
}

// --profile ” and no --profile at all are opposite statements, and the flag package
// stores both as "". The empty string is a decision — use the credentials already in
// the environment — while an absent flag is a question nobody answered, and outside a
// terminal there is nobody to ask. Collapsing them made a CI run provision with
// whatever identity happened to be ambient.
func TestOmittedProfileIsNotTheSameAsAnEmptyOne(t *testing.T) {
	t.Run("omitted outside a terminal is an error", func(t *testing.T) {
		root := initCheckout(t)
		_, stderr, err := runCLI(t, "init", "--repo", root, "--env", "dev",
			"--account", "123456789012", "--region", "us-east-2",
			"--targets", "infra-base", "--api-cidr", "203.0.113.7", "--auto-approve")
		if err == nil {
			t.Fatal("a run with no --profile must say so, not pick an identity")
		}
		combined := err.Error() + stderr
		for _, want := range []string{"--profile", "--profile ''", "ambient"} {
			if !strings.Contains(combined, want) {
				t.Errorf("the refusal must mention %q:\n%s", want, combined)
			}
		}
	})

	t.Run("explicitly empty is ambient credentials", func(t *testing.T) {
		root := initCheckout(t)
		// --account keeps this off the network: what is under test is the routing,
		// not whether ambient credentials happen to resolve on this machine.
		_, stderr, err := runCLI(t, "init", "--repo", root, "--env", "dev",
			"--profile", "", "--account", "123456789012", "--region", "us-east-2",
			"--targets", "infra-base", "--api-cidr", "203.0.113.7", "--auto-approve")
		if err != nil {
			t.Fatalf("--profile '' is a decision and must be honored: %v\n%s", err, stderr)
		}
		config, readErr := os.ReadFile(filepath.Join(root, "examples", "aws", "environments.conf"))
		if readErr != nil {
			t.Fatalf("the configuration was not written: %v", readErr)
		}
		if !strings.Contains(string(config), "123456789012") {
			t.Errorf("the account was not written:\n%s", config)
		}
		// An empty profile is what ambient credentials look like on disk: the key is
		// there, with nothing after it, so a run does not export AWS_PROFILE.
		if strings.Contains(string(config), "profile    = lerian") {
			t.Errorf("no profile should have been invented:\n%s", config)
		}
	})
}

// --set is documented as "the escape hatch for any token this build does not
// know". In shared mode it was not: the tier request carried no Replacements, so
// a tier template with a placeholder was written with the token still in it and
// CheckReadiness then refused to plan the root.
func TestInitSetAlsoReachesTheSharedTier(t *testing.T) {
	root := initCheckout(t)

	mk := func(product, service, body string) {
		dir := filepath.Join(root, "examples", "aws", "products", product, service)
		if err := os.MkdirAll(filepath.Join(dir, "envs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "envs", "dev.tfvars-example"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("midaz", "valkey", "mode = \"dedicated\"\n")
	mk("shared-resources", "valkey", "name = \"<PUT-YOUR-CLUSTER-NAME-HERE>\"\n")

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "midaz", "--mode", "shared",
		"--set", "<PUT-YOUR-CLUSTER-NAME-HERE>=shared-valkey-dev",
		"--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	tier := filepath.Join(root, "examples", "aws", "products", "shared-resources", "valkey", "envs", "dev.tfvars")
	written, err := os.ReadFile(tier)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "<PUT-YOUR-CLUSTER-NAME-HERE>") {
		t.Errorf("--set did not reach the shared tier; the token is still there:\n%s", written)
	}
	if !strings.Contains(string(written), "shared-valkey-dev") {
		t.Errorf("the tier tfvars did not get the value:\n%s", written)
	}
}

// The egress scan walked only the product units, so a placeholder living only in
// a tier root never triggered the --api-cidr resolution, and the token survived
// into the written tfvars.
func TestInitResolvesTheEgressAddressForATierOnlyPlaceholder(t *testing.T) {
	root := initCheckout(t)

	mk := func(product, service, body string) {
		dir := filepath.Join(root, "examples", "aws", "products", product, service)
		if err := os.MkdirAll(filepath.Join(dir, "envs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "envs", "dev.tfvars-example"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The product asks for nothing; only the tier carries the egress token.
	mk("midaz", "valkey", "mode = \"dedicated\"\n")
	mk("shared-resources", "valkey", "allowed_cidr = \"<PUT-YOUR-EGRESS-IP-HERE>/32\"\n")

	var out, errOut bytes.Buffer
	err := runInit(context.Background(), []string{
		"--repo", root, "--env", "dev", "--profile", "acme",
		"--account", "123456789012", "--region", "us-east-2",
		"--targets", "midaz", "--mode", "shared",
		"--api-cidr", "203.0.113.7", "--auto-approve",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	tier := filepath.Join(root, "examples", "aws", "products", "shared-resources", "valkey", "envs", "dev.tfvars")
	written, err := os.ReadFile(tier)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "<PUT-YOUR-EGRESS-IP-HERE>") {
		t.Errorf("the tier-only egress placeholder was never resolved:\n%s", written)
	}
	if !strings.Contains(string(written), "203.0.113.7/32") {
		t.Errorf("the tier tfvars did not get the address:\n%s", written)
	}
}

// Every exit of resolveAPICIDR ends in validateBareAddress except one: the branch
// where detection fails and the operator types the address returned the answer
// raw. A typed "203.0.113.0/24" was then substituted into
// "<PUT-YOUR-EGRESS-IP-HERE>/32", producing "203.0.113.0/24/32" — which fails only
// at plan time, long after init reported success.
func TestAPICIDRValidatesTheTypedAddressWhenDetectionFails(t *testing.T) {
	// A canceled context makes DetectEgressIP fail without touching the network.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, typed := range []string{"203.0.113.0/24", "not-an-address", "2001:db8::1"} {
		t.Run(typed, func(t *testing.T) {
			var out bytes.Buffer
			ask := &prompter{
				interactive: true,
				in:          bufio.NewReader(strings.NewReader(typed + "\n")),
				out:         &out,
			}
			got, err := resolveAPICIDR(ctx, initOptions{}, ask)
			if err == nil {
				t.Fatalf("a typed %q was accepted and would be written as %q/32", typed, got)
			}
		})
	}
}

// The same branch must still accept what it is supposed to accept.
func TestAPICIDRTakesATypedBareAddressWhenDetectionFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out bytes.Buffer
	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader("203.0.113.7\n")),
		out:         &out,
	}
	got, err := resolveAPICIDR(ctx, initOptions{}, ask)
	if err != nil {
		t.Fatalf("a bare address was refused: %v", err)
	}
	if got != "203.0.113.7" {
		t.Errorf("got %q, want %q", got, "203.0.113.7")
	}
}

// A region is a closed set, so it is chosen rather than typed. Typing it means
// remembering whether it is eu-west-1 or eu-west-01, and a typo here is caught at
// the first API call rather than at the prompt.
func TestTheRegionsAreAListToChooseFrom(t *testing.T) {
	options := regionOptions("", "")

	if len(options) < 15 {
		t.Errorf("only %d regions offered", len(options))
	}

	byValue := map[string]option{}
	for _, opt := range options {
		byValue[opt.value] = opt
	}
	for _, want := range []string{"us-east-1", "us-east-2", "eu-west-1", "sa-east-1", "ap-southeast-1"} {
		if _, ok := byValue[want]; !ok {
			t.Errorf("%s is not in the list", want)
		}
	}
	// Named, because "sa-east-1" is not where most people know São Paulo to be.
	if !strings.Contains(byValue["sa-east-1"].note, "São Paulo") {
		t.Errorf("sa-east-1 is not named: %q", byValue["sa-east-1"].note)
	}
}

// The list is a convenience, not a gate. AWS adds regions, and this list is a
// copy that will age — so there is always a way past it.
func TestARegionNotInTheListCanStillBeGiven(t *testing.T) {
	options := regionOptions("", "")

	var escape *option
	for index, opt := range options {
		if opt.value == typedRegionChoice {
			escape = &options[index]
		}
	}
	if escape == nil {
		t.Fatal("a region this list has never heard of cannot be entered")
	}
	if !strings.Contains(escape.note, "type") {
		t.Errorf("the escape does not say what it does: %q", escape.note)
	}
}

// A region already known — from --region, or from the profile — is what the
// cursor opens on, and it is in the list even if this copy has never heard of it.
func TestAKnownRegionIsWhereTheCursorOpens(t *testing.T) {
	options := regionOptions("me-central-1", "")

	if options[0].value != "me-central-1" {
		t.Errorf("the list opens on %q, not on the region already known", options[0].value)
	}
}

// Choosing a region is a pick, and "another region" falls through to typing one.
func TestAskForRegionPicksOrFallsThroughToTyping(t *testing.T) {
	// Enter on the first row, which is the region already known.
	ask, _ := selectorFor(t, keyEnterSeq)
	chosen, err := askForRegion(ask, "eu-west-2", "")
	if err != nil {
		t.Fatal(err)
	}
	if chosen != "eu-west-2" {
		t.Errorf("chose %q", chosen)
	}
}

func TestAskForRegionRejectsSomethingThatIsNotARegion(t *testing.T) {
	// The typed path validates: a region code has a shape, and a run with a
	// misspelled one fails at the first API call rather than here.
	if err := validateRegion("not a region"); err == nil {
		t.Error("anything at all was accepted as a region")
	}
	if err := validateRegion("me-central-1"); err != nil {
		t.Errorf("a real region was rejected: %v", err)
	}
	// One this list has never heard of, but shaped like a region, is fine: the
	// list ages and the shape does not.
	if err := validateRegion("ap-southeast-9"); err != nil {
		t.Errorf("a well-formed region this copy does not know was rejected: %v", err)
	}
}

// With an address detected there are two answers, not a blank line: use it, or
// give another. Typing it back character by character is the work the detection
// just did.
func TestTheDetectedAddressIsAChoice(t *testing.T) {
	options := egressOptions("203.0.113.7")

	if len(options) != 2 {
		t.Fatalf("got %d rows, want the detected one and the escape: %+v", len(options), options)
	}
	if options[0].value != "203.0.113.7" {
		t.Errorf("the first row is %q, want what was detected", options[0].value)
	}
	if !strings.Contains(options[0].note, "this machine") {
		t.Errorf("the row does not say where the address came from: %q", options[0].note)
	}
	if options[1].value != typedAddressChoice {
		t.Errorf("there is no way to give a different address: %+v", options)
	}
}

// bootstrap is not on the list because it is not a choice: init configures it
// whatever else is picked, since it is the first thing that has to run and it
// needs a tfvars like every other root. The question says so, rather than leaving
// its absence to be read as an oversight.
func TestTheConfigureQuestionSaysBootstrapIsIncluded(t *testing.T) {
	purpose := configurePurpose()

	if !strings.Contains(purpose, "bootstrap") {
		t.Errorf("the question does not mention bootstrap at all: %q", purpose)
	}
	if !strings.Contains(strings.ToLower(purpose), "always") {
		t.Errorf("the question does not say bootstrap is not optional: %q", purpose)
	}
}

// And once the files are written, the next step is named: nothing else can run
// until the state backend exists, and bootstrap is what creates it.
func TestInitSaysWhatToRunNext(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	printNextStep(&out, layout, "dev")

	if !strings.Contains(out.String(), "bootstrap") {
		t.Errorf("the next step is not named:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "--action apply") {
		t.Errorf("the command to run is not given:\n%s", out.String())
	}
}

// With the backend already there, bootstrap has run and saying so would be noise.
func TestInitIsQuietWhenBootstrapHasRun(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeBackendFile(t, checkout, "dev")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	printNextStep(&out, layout, "dev")

	if strings.Contains(out.String(), "bootstrap") {
		t.Errorf("an already-bootstrapped environment was told to bootstrap:\n%s", out.String())
	}
}

// The profile's region is a suggestion, which means the cursor opens on it — not
// an answer given on somebody's behalf.
//
// Two paths got this wrong in opposite ways: one used the profile's region
// without asking at all, the other asked but opened the list on us-east-1
// because it never read the profile. Both end with infrastructure in a region
// nobody chose out loud.
func TestTheProfileRegionIsASuggestionNotAnAnswer(t *testing.T) {
	// The list opens on the profile's region when that is all we know, and says so
	// — it used to claim "already chosen", which was not true of a value read off
	// ~/.aws.
	options := regionOptions("ap-northeast-1", "from the sandbox profile")
	if options[0].value != "ap-northeast-1" {
		t.Errorf("the list opens on %q, not on the profile's region", options[0].value)
	}
	if !strings.Contains(options[0].note, "sandbox profile") {
		t.Errorf("the row does not say why it is first: %q", options[0].note)
	}
}

// And asking happens even when the profile declares one: interactive means there
// is somebody to confirm it with.
func TestARegionIsAskedForEvenWhenTheProfileHasOne(t *testing.T) {
	ask, painted := selectorFor(t, keyEnterSeq)

	chosen, err := askForRegion(ask, "sa-east-1", "")
	if err != nil {
		t.Fatal(err)
	}

	if chosen != "sa-east-1" {
		t.Errorf("chose %q", chosen)
	}
	if !strings.Contains(painted.String(), "Which AWS region") {
		t.Errorf("the region was taken without asking:\n%s", painted.String())
	}
}

// The region of a chosen profile is asked about, not inherited.
//
// This is the path that matters — the one resolveCredentials takes after the
// profile list — and the earlier test for it exercised askForRegion directly,
// which is the half that was never broken. It stayed green with the fix removed.
func TestTheRegionOfAChosenProfileIsAskedAbout(t *testing.T) {
	ask, painted := selectorFor(t, keyEnterSeq)

	chosen, err := regionFor(ask, "some-profile", "", "ap-northeast-1")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(painted.String(), "Which AWS region") {
		t.Errorf("the profile's region was taken without asking:\n%s", painted.String())
	}
	if chosen != "ap-northeast-1" {
		t.Errorf("chose %q; the profile's region is what the list opens on", chosen)
	}
}

// A region that was stated is not asked about again.
func TestAStatedRegionIsNotAskedAboutAgain(t *testing.T) {
	ask, painted := selectorFor(t, "")

	chosen, err := regionFor(ask, "some-profile", "eu-west-3", "ap-northeast-1")
	if err != nil {
		t.Fatal(err)
	}

	if chosen != "eu-west-3" {
		t.Errorf("chose %q, want the one that was passed", chosen)
	}
	if strings.Contains(painted.String(), "Which AWS region") {
		t.Errorf("a stated region was asked about anyway:\n%s", painted.String())
	}
}

// "already chosen" was not true: nobody had chosen it. The region came off the
// profile in ~/.aws, and calling that a choice invites somebody to press enter
// believing they are confirming their own earlier decision.
func TestTheSuggestedRegionSaysWhereItCameFrom(t *testing.T) {
	fromProfile := regionOptions("us-east-2", "from the lerian-sandbox profile")
	if fromProfile[0].value != "us-east-2" {
		t.Fatalf("the list does not open on the suggestion: %+v", fromProfile[0])
	}
	if strings.Contains(fromProfile[0].note, "already chosen") {
		t.Errorf("the row claims a choice nobody made: %q", fromProfile[0].note)
	}
	if !strings.Contains(fromProfile[0].note, "lerian-sandbox profile") {
		t.Errorf("the row does not say where it came from: %q", fromProfile[0].note)
	}
	// The place is still named, because that is the part a code does not say.
	if !strings.Contains(fromProfile[0].note, "Ohio") {
		t.Errorf("the row does not name the place: %q", fromProfile[0].note)
	}
}

// The purpose line is one line, truncated to the terminal's width — so a long one
// loses its end, which is where I had put the part nobody knows yet.
func TestTheConfigurePurposeFitsOnOneLine(t *testing.T) {
	purpose := configurePurpose()

	if len(purpose) > 100 {
		t.Errorf("the purpose is %d characters and the selector shows one line:\n%s", len(purpose), purpose)
	}
	if !strings.Contains(purpose, "bootstrap") {
		t.Errorf("the part that is not obvious was cut: %q", purpose)
	}
}
