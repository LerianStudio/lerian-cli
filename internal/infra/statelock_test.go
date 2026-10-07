package infra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
)

// The real message, from a run that hit it: an apply whose process went away
// without releasing the lock, leaving every later run to fail on something that
// looks like a permissions problem and is not.
const lockedOutput = `infra: terraform plan failed for infra-base/eks: exit status 1

Error: Error acquiring the state lock

Error message: operation error DynamoDB: PutItem, https response error
StatusCode: 400, RequestID: O0TA0NR2DD63PDC3JILFPFNLPFVV4KQNSO5AEMVJF66Q9ASUAAJG,
ConditionalCheckFailedException: The conditional request failed
Lock Info:
  ID:        6b012100-a4eb-91d3-d0f2-e0a4ff5085d1
  Path:      lerian-tfstate-dev-524121347244/aws/infra-base/eks/terraform.tfstate
  Operation: OperationTypeApply
  Who:       someone@their-laptop.local
  Version:   1.16.3
  Created:   2026-10-07 19:29:58.635098 +0000 UTC
  Info:
`

func TestTheLockReportIsRead(t *testing.T) {
	lock := ReadStateLock(lockedOutput)
	if lock == nil {
		t.Fatal("the lock went unrecognized")
	}

	if lock.ID != "6b012100-a4eb-91d3-d0f2-e0a4ff5085d1" {
		t.Errorf("id = %q", lock.ID)
	}
	if lock.Operation != "OperationTypeApply" {
		t.Errorf("operation = %q", lock.Operation)
	}
	if lock.Who != "someone@their-laptop.local" {
		t.Errorf("who = %q", lock.Who)
	}
	if lock.Created.IsZero() {
		t.Error("the time was not read, so the advice cannot say how old it is")
	}
	// Not the Path field of the lock, which is the state object — the Info line
	// is empty in this message and must not be mistaken for it.
	if !strings.HasSuffix(lock.Path, "/aws/infra-base/eks/terraform.tfstate") {
		t.Errorf("path = %q", lock.Path)
	}
}

// Anything else is left to terraform, whose message is better than a guess.
func TestOtherFailuresAreNotReadAsLocks(t *testing.T) {
	for _, text := range []string{
		"infra: terraform plan failed: exit status 1\n\nError: no matching EC2 VPC found",
		"",
		// A lock report with no id: there is no command to offer, so there is
		// nothing this can add.
		"Error acquiring the state lock\nLock Info:\n  Who: somebody\n",
		// Fields that look like a lock report and are not. Terraform prints
		// resource attributes in this shape all the time, and offering a
		// force-unlock for somebody's security group id would be worse than
		// saying nothing.
		"Error: creating EC2 Security Group\n  ID:        sg-0abc123\n" +
			"  Operation: create\n  Who:       terraform\n",
	} {
		if lock := ReadStateLock(text); lock != nil {
			t.Errorf("read a lock out of %q: %+v", text, lock)
		}
	}
}

// The advice has to carry the two things somebody would otherwise retype from a
// web page: which directory, and which id.
func TestTheAdviceIsRunnableAsPrinted(t *testing.T) {
	lock := ReadStateLock(lockedOutput)
	now := lock.Created.Add(25 * time.Minute)

	advice := lock.Advice("/checkout/examples/aws/infra-base/eks", now)

	want := "terraform -chdir=/checkout/examples/aws/infra-base/eks force-unlock " +
		"6b012100-a4eb-91d3-d0f2-e0a4ff5085d1"
	if !strings.Contains(advice, want) {
		t.Errorf("the command is not runnable as printed:\n%s", advice)
	}
	// Who and how long ago: the two facts that decide whether breaking it is safe.
	if !strings.Contains(advice, "someone@their-laptop.local") {
		t.Errorf("it does not say who holds it:\n%s", advice)
	}
	if !strings.Contains(advice, "25 minute(s) ago") {
		t.Errorf("it does not say how old it is:\n%s", advice)
	}
	// And the warning that makes the command safe to hand over at all.
	if !strings.Contains(advice, "two writers") {
		t.Errorf("it does not warn about breaking a live lock:\n%s", advice)
	}
}

// An interrupted apply leaves more than a lock, and the plan after the unlock is
// where that shows up.
func TestAnInterruptedApplyGetsTheExtraWarning(t *testing.T) {
	apply := ReadStateLock(lockedOutput).Advice("/x", time.Now())
	if !strings.Contains(apply, "errored.tfstate") {
		t.Errorf("an apply lock does not mention what else it can leave:\n%s", apply)
	}

	plan := ReadStateLock(strings.ReplaceAll(lockedOutput,
		"OperationTypeApply", "OperationTypePlan")).Advice("/x", time.Now())
	if strings.Contains(plan, "errored.tfstate") {
		t.Errorf("a plan lock warns about a write it could not have made:\n%s", plan)
	}
}

// A clock that disagrees — a lock from the future, or no time at all — must not
// produce a nonsense age. The rest of the advice still stands.
func TestAnUnreadableAgeIsLeftOut(t *testing.T) {
	lock := ReadStateLock(lockedOutput)

	before := lock.Advice("/x", lock.Created.Add(-time.Hour))
	if strings.Contains(before, "ago") {
		t.Errorf("a lock from the future got an age:\n%s", before)
	}
	if !strings.Contains(before, "force-unlock") {
		t.Errorf("the command went missing with the age:\n%s", before)
	}
}

// "from a Apply" is the kind of seam that makes a message read as generated
// rather than written. Terraform's OperationTypeApply has to become English.
func TestTheOperationReadsAsEnglish(t *testing.T) {
	lock := ReadStateLock(lockedOutput)

	advice := lock.Advice("/x", lock.Created.Add(time.Minute))
	if strings.Contains(advice, "a Apply") {
		t.Errorf("the operation was printed raw:\n%s", advice)
	}
	// "from an apply", not merely "an apply": the paragraph below the holder line
	// contains that phrase too, and asserting on it alone passed while the holder
	// line still read "from a Apply".
	if !strings.Contains(advice, "from an apply") {
		t.Errorf("it does not say what kind of run holds it:\n%s", advice)
	}

	// Everything else takes "a", and still lowercase.
	plan := ReadStateLock(strings.ReplaceAll(lockedOutput,
		"OperationTypeApply", "OperationTypePlan"))
	if got := plan.Holder(plan.Created); !strings.Contains(got, "a plan") {
		t.Errorf("holder = %q", got)
	}
}

// Holder is the one line the unlock prompt stands on: who, doing what, how long
// ago. Without those it is a question nobody can answer.
func TestTheHolderLineCarriesTheDecision(t *testing.T) {
	lock := ReadStateLock(lockedOutput)

	holder := lock.Holder(lock.Created.Add(48 * time.Minute))

	for _, want := range []string{"someone@their-laptop.local", "an apply", "48 minute(s) ago"} {
		if !strings.Contains(holder, want) {
			t.Errorf("the holder line does not say %q: %q", want, holder)
		}
	}

	// And it still says something when terraform named nobody.
	anonymous := StateLock{ID: "x"}
	if got := anonymous.Holder(time.Now()); got == "" {
		t.Error("an unnamed holder produced an empty line")
	}
}

// The id reaches a command line, and the only way a value there turns into a
// flag is by starting with a dash.
// The unlock has to reach the backend as the run does. Started as a bare
// command it inherits this process's environment, which carries no AWS
// credentials — the CLI resolves them once and exports them to each terraform
// it starts — so it failed with "No valid credential sources found" after the
// operator had already confirmed.
func TestTheUnlockRunsWithTheRunsCredentials(t *testing.T) {
	recorded := filepath.Join(t.TempDir(), "env")
	path := filepath.Join(t.TempDir(), "terraform")
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    version) echo '{"terraform_version":"1.9.0","platform":"test","provider_selections":{},"terraform_outdated":false}'; exit 0 ;;
  esac
done
env > ` + recorded + `
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	cli := &CLI{
		execPath: path,
		byDir:    map[string]*tfexec.Terraform{},
		Credentials: Credentials{
			AccessKeyID:     "AKIAEXAMPLE",
			SecretAccessKey: "secret",
			SessionToken:    "token",
		},
	}

	if err := cli.ForceUnlock(context.Background(), Unit{Name: "eks", Dir: t.TempDir()},
		"6b012100-a4eb-91d3-d0f2-e0a4ff5085d1"); err != nil {
		t.Fatal(err)
	}

	environment, err := os.ReadFile(recorded)
	if err != nil {
		t.Fatalf("terraform was never run: %v", err)
	}
	for _, want := range []string{
		"AWS_ACCESS_KEY_ID=AKIAEXAMPLE",
		"AWS_SECRET_ACCESS_KEY=secret",
		"AWS_SESSION_TOKEN=token",
	} {
		if !strings.Contains(string(environment), want) {
			t.Errorf("the unlock did not get %s:\n%s", want, environment)
		}
	}
}

func TestAnIDArgvWouldMisreadIsRefused(t *testing.T) {
	cli := &CLI{execPath: "/nonexistent/terraform", byDir: map[string]*tfexec.Terraform{}}

	// Checked by the message, not merely by there being an error: the exec path
	// does not exist, so running it fails too, and "it returned an error" would
	// pass with no validation at all.
	for _, id := range []string{"", "   ", "-force", "--help"} {
		err := cli.ForceUnlock(context.Background(), Unit{Dir: "/tmp"}, id)
		if err == nil {
			t.Errorf("%q was accepted", id)
			continue
		}
		if !strings.Contains(err.Error(), "is not a lock id") {
			t.Errorf("%q reached terraform instead of being refused: %v", id, err)
		}
	}
}
