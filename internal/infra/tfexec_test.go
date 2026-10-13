package infra

import (
	"testing"

	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// countChanges replaces the jq the shell version counted plans with. The rule it
// has to keep: an action counts once per resource that carries it, so a
// replacement shows up as both a delete and a create — which is exactly what the
// operator needs to see before confirming.
func TestCountChanges(t *testing.T) {
	change := func(actions ...tfjson.Action) *tfjson.ResourceChange {
		return &tfjson.ResourceChange{Change: &tfjson.Change{Actions: actions}}
	}

	tests := []struct {
		name string
		plan *tfjson.Plan
		want Changes
	}{
		{
			name: "nil plan",
			plan: nil,
			want: Changes{},
		},
		{
			name: "no changes",
			plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{
				change(tfjson.ActionNoop),
			}},
			want: Changes{},
		},
		{
			name: "one of each",
			plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{
				change(tfjson.ActionCreate),
				change(tfjson.ActionUpdate),
				change(tfjson.ActionDelete),
			}},
			want: Changes{Create: 1, Update: 1, Delete: 1},
		},
		{
			name: "a replacement counts in both columns",
			plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{
				change(tfjson.ActionDelete, tfjson.ActionCreate),
			}},
			want: Changes{Create: 1, Delete: 1},
		},
		{
			name: "a resource with no change block is skipped",
			plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{
				{Address: "aws_db_instance.this"},
				nil,
				change(tfjson.ActionCreate),
			}},
			want: Changes{Create: 1},
		},
		{
			name: "reading is not a change",
			plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{
				change(tfjson.ActionRead),
			}},
			want: Changes{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := countChanges(test.plan)
			if got != test.want {
				t.Errorf("countChanges = %+v, want %+v", got, test.want)
			}
			if got.Empty() != (test.want == Changes{}) {
				t.Errorf("Empty() = %v for %+v", got.Empty(), got)
			}
		})
	}
}

// The grace period has one job: outlive the slowest thing Terraform can be in
// the middle of when it is interrupted. What it is protecting against is a real
// incident — an EKS node group created in AWS at 19:44:39 whose last state write
// was 19:44:31, because terraform-exec's 60-second default killed Terraform
// before it could record what it had just made.
//
// This asserts the one thing a unit test can: that the value is still well clear
// of that default, and that tfexec accepts it. Whether an interrupted Terraform
// actually finishes inside it cannot be proven without a real binary and a real
// apply; what this catches is somebody tuning the constant back down to
// something that does not survive a node group.
func TestTheShutdownGraceOutlivesTheDangerousDefault(t *testing.T) {
	const tfexecDefault = 60 * time.Second

	if gracefulShutdown <= tfexecDefault {
		t.Fatalf("the grace period is %v, no better than terraform-exec's own %v default",
			gracefulShutdown, tfexecDefault)
	}
	// An EKS node group takes three to five minutes to come up, and it is the
	// slowest single resource these roots create.
	if gracefulShutdown < 10*time.Minute {
		t.Errorf("%v does not outlive an EKS node group being created", gracefulShutdown)
	}
	// Not unbounded either: a Terraform that is genuinely stuck has to die.
	if gracefulShutdown == 0 {
		t.Error("a zero grace period waits forever on a stuck Terraform")
	}

	client, err := tfexec.NewTerraform(t.TempDir(), "/usr/bin/false")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetWaitDelay(gracefulShutdown); err != nil {
		t.Errorf("tfexec rejects the grace period: %v", err)
	}
}

// And that every client actually gets it. A constant nobody applies is the
// mutation a check on the value alone walks straight past — the client would go
// back to terraform-exec's 60-second default and nothing would say so.
func TestEveryClientIsGivenTheShutdownGrace(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "terraform")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var got []time.Duration
	previous := setWaitDelay
	setWaitDelay = func(client *tfexec.Terraform, delay time.Duration) error {
		got = append(got, delay)
		return previous(client, delay)
	}
	t.Cleanup(func() { setWaitDelay = previous })

	cli := &CLI{execPath: fake, byDir: map[string]*tfexec.Terraform{}}
	if _, err := cli.terraform(Unit{Dir: t.TempDir(), Name: "infra-base/eks"}); err != nil {
		t.Fatalf("terraform() = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("the grace period was set %d times, want once: %v", len(got), got)
	}
	if got[0] != gracefulShutdown {
		t.Errorf("set to %v, want %v", got[0], gracefulShutdown)
	}
}
