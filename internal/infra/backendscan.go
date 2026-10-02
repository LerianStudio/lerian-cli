package infra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// StateBucketPrefix is what the bootstrap root names its bucket with:
// lerian-tfstate-<env>-<account_id>, and lerian-tfstate-lock-<env> for the lock
// table.
//
// It is a property of the TEMPLATES, not of whoever is running this. A client
// deploying from this repository gets buckets with this prefix in their own
// account, because bootstrap/main.tf builds the name — so searching for it is
// searching for "a backend these templates made", not for anything about an
// organization.
const StateBucketPrefix = "lerian-tfstate-"

// LockTableFor is the lock table bootstrap pairs with an environment's bucket.
func LockTableFor(env string) string { return StateBucketPrefix + "lock-" + env }

// StateBackend is a state bucket that exists in the account right now, as the
// AWS API reports it — not as a local file claims.
type StateBackend struct {
	Bucket string
	Region string
	// Env is read back out of the bucket name when it follows the convention, and
	// is empty for a bucket somebody named by hand.
	Env string
	// LockTable is the table found beside it, empty when there is none. A backend
	// without one still works; concurrent runs stop being safe, which is worth
	// saying before anybody adopts it.
	LockTable string
}

// Matches reports whether this backend is the one bootstrap would have made for
// env in account.
func (s StateBackend) Matches(env, account string) bool {
	return s.Bucket == StateBucketPrefix+env+"-"+account
}

// BackendLister finds the state backends an account already holds. It is an
// interface so the menus that depend on it can be tested without credentials.
type BackendLister interface {
	ListStateBackends(ctx context.Context, profile, region, account string) ([]StateBackend, error)
}

// CLIBackends asks the AWS CLI, the same way everything else here does: the
// credentials, the SSO session and the proxy configuration are already resolved
// by it, and a second implementation would resolve them differently.
type CLIBackends struct {
	// Binary is the aws executable. Empty means "aws" from PATH.
	Binary string
}

// ListStateBackends returns the state buckets this account holds.
//
// Buckets are listed globally and then filtered by the account suffix rather
// than by asking per environment. One call answers "what exists here" for every
// environment at once, including the ones nobody has configured yet — and that
// is the question somebody has when the local file is missing and they do not
// know whether anyone has bootstrapped before them.
func (c CLIBackends) ListStateBackends(ctx context.Context, profile, region, account string) ([]StateBackend, error) {
	names, err := c.run(ctx, profile, region,
		"s3api", "list-buckets", "--query", "Buckets[].Name", "--output", "text")
	if err != nil {
		return nil, err
	}

	var found []StateBackend
	for _, name := range strings.Fields(names) {
		if !strings.HasPrefix(name, StateBucketPrefix) || !strings.HasSuffix(name, "-"+account) {
			continue
		}
		backend := StateBackend{
			Bucket: name,
			Env:    strings.TrimSuffix(strings.TrimPrefix(name, StateBucketPrefix), "-"+account),
			Region: c.bucketRegion(ctx, profile, region, name),
		}
		// A lock table name is only predictable for a bucket that follows the
		// convention. For one named by hand there is nothing to look up.
		if backend.Env != "" && backend.Region != "" {
			backend.LockTable = c.lockTable(ctx, profile, backend.Region, backend.Env)
		}
		found = append(found, backend)
	}
	return found, nil
}

// bucketRegion resolves where a bucket lives. The answer matters: a backend in
// another region is still usable, and a run configured for the wrong one fails
// at terraform init with a redirect nobody reads as a region problem.
func (c CLIBackends) bucketRegion(ctx context.Context, profile, region, bucket string) string {
	out, err := c.run(ctx, profile, region,
		"s3api", "get-bucket-location", "--bucket", bucket,
		"--query", "LocationConstraint", "--output", "text")
	if err != nil {
		return ""
	}
	// us-east-1 is reported as a null constraint, for historical reasons: it was
	// the only region when the API was designed. Every other region names itself.
	switch answer := strings.TrimSpace(out); answer {
	case "", "None", "null":
		return "us-east-1"
	default:
		return answer
	}
}

// lockTable reports the lock table beside a bucket, or the empty string.
func (c CLIBackends) lockTable(ctx context.Context, profile, region, env string) string {
	name := LockTableFor(env)
	out, err := c.run(ctx, profile, region,
		"dynamodb", "describe-table", "--table-name", name,
		"--query", "Table.TableName", "--output", "text")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (c CLIBackends) run(ctx context.Context, profile, region string, args ...string) (string, error) {
	binary := c.Binary
	if binary == "" {
		binary = "aws"
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	command := exec.CommandContext(ctx, binary, args...)
	command.Env = os.Environ()
	if profile != "" {
		command.Env = append(command.Env, "AWS_PROFILE="+profile)
	}

	// stdout parsed, stderr kept for the error only: the AWS CLI writes warnings
	// and deprecation notices to stderr, and merging them would put one of those
	// into a bucket name.
	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("aws %s failed: %w\n%s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// AdoptBackend writes backend/<env>.hcl for a bucket that already exists.
//
// This is the file bootstrap normally produces, and its absence is the only
// reason a checkout where somebody else already bootstrapped looks unbootstrapped.
// Writing it by hand is what the LoadBackend error has always told people to do;
// this does the same thing without the typing, from values read off the account
// rather than remembered.
//
// It refuses to overwrite. A backend file that is already there is the one the
// state is under, and replacing it points a stack at a different bucket — which
// is how a stack ends up applying against state it has never seen.
func AdoptBackend(layout Layout, env string, backend StateBackend) (string, error) {
	if backend.Bucket == "" {
		return "", fmt.Errorf("infra: no bucket to adopt for %s", env)
	}
	path := layout.BackendFile(env)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("infra: %s already exists\n"+
			"It is the backend this environment's state is under. Remove it yourself if\n"+
			"you mean to point %s somewhere else", layout.RepoRel(path), env)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("infra: cannot read %s: %w", layout.RepoRel(path), err)
	}

	region := backend.Region
	if region == "" {
		return "", fmt.Errorf("infra: the region of %s could not be read\n"+
			"Without it terraform init cannot reach the bucket. Write %s by hand",
			backend.Bucket, layout.RepoRel(path))
	}

	var file strings.Builder
	fmt.Fprintf(&file, "bucket         = %q\n", backend.Bucket)
	fmt.Fprintf(&file, "region         = %q\n", region)
	// Only when there is one. A line naming a table that does not exist fails at
	// terraform init, which is worse than running without locking — and the menu
	// that offered this said which case it was.
	if backend.LockTable != "" {
		fmt.Fprintf(&file, "dynamodb_table = %q\n", backend.LockTable)
	}
	fmt.Fprintf(&file, "encrypt        = true\n")

	if err := os.MkdirAll(layout.BackendDir(), 0o755); err != nil {
		return "", fmt.Errorf("infra: cannot create %s: %w", layout.RepoRel(layout.BackendDir()), err)
	}
	if err := os.WriteFile(path, []byte(file.String()), 0o600); err != nil {
		return "", fmt.Errorf("infra: cannot write %s: %w", layout.RepoRel(path), err)
	}
	return path, nil
}
