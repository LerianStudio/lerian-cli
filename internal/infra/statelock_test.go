package infra

import (
	"strings"
	"testing"
	"time"
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
