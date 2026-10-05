package infra

import (
	"strings"
	"testing"
)

// Everything between the prefix and the account is NOT an environment. A bucket
// named lerian-tfstate-sandbox-<account> yielded "sandbox", so the menu called
// it "made for sandbox" and the scan spent a DynamoDB call looking for
// lerian-tfstate-lock-sandbox, which cannot exist.
func TestOnlyOurEnvironmentNamesAreReadOutOfABucketName(t *testing.T) {
	for _, env := range Environments {
		if got := knownEnvironment(env); got != env {
			t.Errorf("knownEnvironment(%q) = %q", env, got)
		}
	}
	for _, name := range []string{"sandbox", "dev-eu", "production", "", "foo-bar"} {
		if got := knownEnvironment(name); got != "" {
			t.Errorf("knownEnvironment(%q) = %q, want empty — it is not one of ours", name, got)
		}
	}
}

// Matches is what decides which row goes first, and it compares the whole name
// rather than the parsed environment.
func TestMatchesComparesTheWholeBucketName(t *testing.T) {
	backend := StateBackend{Bucket: StateBucketPrefix + "dev-123456789012"}

	if !backend.Matches("dev", "123456789012") {
		t.Error("the convention-named bucket does not match its own environment")
	}
	if backend.Matches("stg", "123456789012") {
		t.Error("a dev bucket matched stg")
	}
	if backend.Matches("dev", "999988887777") {
		t.Error("a bucket matched an account it does not belong to")
	}
}

// The lock table name follows the environment, and is only predictable for a
// bucket that follows the convention.
func TestTheLockTableNameFollowsTheEnvironment(t *testing.T) {
	if got := LockTableFor("dev"); !strings.HasSuffix(got, "-dev") {
		t.Errorf("LockTableFor(dev) = %q", got)
	}
}
