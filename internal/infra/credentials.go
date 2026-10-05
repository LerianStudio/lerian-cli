package infra

// Credentials are resolved once for a whole run and handed to every Terraform
// process, instead of letting each one resolve the profile for itself.
//
// The reason is a race we caused. With AWS_PROFILE set, every terraform process
// resolves the profile independently, and for an SSO profile that means each one
// may decide the cached token needs refreshing. Four of them starting together —
// which is exactly what a parallel stage does — refresh concurrently and fight
// over the same file:
//
//	failed to replace old cached SSO token file, rename
//	~/.aws/sso/cache/<hash>.json.tmp-1787066430403425000 ->
//	~/.aws/sso/cache/<hash>.json: no such file or directory
//
// One process renames its temporary file into place, the next finds the one it
// wrote already gone. The failure is intermittent, depends on how many units share
// a stage, and reads like a broken credential rather than a collision.
//
// Resolving once removes the race by construction: the children receive concrete
// keys and never touch the SSO cache at all.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Credentials is a resolved, short-lived AWS credential set.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// Expiration is when the session dies, zero when the profile yielded a
	// credential that does not expire. It matters because these credentials are
	// resolved ONCE for the whole run: whatever lifetime was left at the start is
	// all the run gets, and an apply that outlives it loses the state write while
	// AWS goes on to finish the change.
	Expiration time.Time
}

// RemainingLifetime is how long the session still has, and whether that is known
// at all. A zero Expiration means it does not expire, so there is nothing to
// warn about.
func (c Credentials) RemainingLifetime(now time.Time) (time.Duration, bool) {
	if c.Expiration.IsZero() {
		return 0, false
	}
	return c.Expiration.Sub(now), true
}

// Environment renders the credentials as environment variables for a child
// process. AWS_PROFILE is deliberately absent from the result: a child that sees
// both would resolve the profile and go back to the SSO cache.
func (c Credentials) Environment() map[string]string {
	if c.AccessKeyID == "" {
		return nil
	}
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     c.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": c.SecretAccessKey,
	}
	if c.SessionToken != "" {
		env["AWS_SESSION_TOKEN"] = c.SessionToken
	}
	return env
}

// ErrNoAWSCLI is returned when the AWS CLI is absent.
//
// It is checked early, by name, for the same reason ErrNoGit and
// MinTerraformVersion are: without it the first AWS call fails as
// `exec: "aws": executable file not found in $PATH`, buried inside a credentials
// error, and the operator reads that as a broken profile rather than a missing tool.
var ErrNoAWSCLI = errors.New("infra: the AWS CLI (aws) was not found in PATH")

// ErrOldAWSCLI is returned when the AWS CLI found is too old to be usable.
var ErrOldAWSCLI = errors.New("infra: the AWS CLI found is version 1")

// RequireAWSCLI verifies a usable AWS CLI is installed before anything tries to use
// it.
//
// The CLI is not optional and not replaceable by a Go SDK here: it is the only
// implementation that can complete an SSO refresh, and it is what resolves whichever
// credential mechanism the operator's profile happens to use.
//
// V2 SPECIFICALLY, and that is why this costs one `aws --version`. The command
// ResolveCredentials depends on — `configure export-credentials` — landed in v2 and
// was never backported, so a v1 installation passes a presence check and then fails
// at the first profile with "argument operation: Invalid choice". That reads as a
// broken profile, which is the diagnosis this whole function exists to prevent.
//
// A version string that cannot be parsed is ACCEPTED. What cannot be measured is not
// judged: a vendored wrapper, a shim, or a future format change should not be
// refused by a parser guessing at it, and the real command's own error is a better
// last word than a false one here.
//
// The message names both ways a profile is configured. SSO is common at Lerian and
// not universal — a profile with an access key and secret in ~/.aws/credentials is
// just as valid, and telling somebody who uses one to run `aws configure sso` sends
// them to fix something that is not broken.
func RequireAWSCLI(ctx context.Context) error {
	path, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("%w\n"+
			"Install it:\n%s\n"+
			"Then configure one profile per AWS account you deploy into:\n"+
			"  aws configure sso --profile <name>      # IAM Identity Center\n"+
			"  aws configure --profile <name>          # access key and secret\n"+
			"Either works. Ambient credentials in the environment work too: pass\n"+
			"--profile '' with --account to say which account they reach.",
			ErrNoAWSCLI, installAWSCLI())
	}
	if major, ok := awsCLIMajor(ctx, path); ok && major < 2 {
		return fmt.Errorf("%w, and this tool needs v2\n"+
			"  %s\n"+
			"v2 is where 'aws configure export-credentials' exists, which is how a\n"+
			"profile becomes credentials here — v1 fails at the first profile with an\n"+
			"invalid-choice error that looks like a broken profile.\n"+
			"  https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
			ErrOldAWSCLI, path)
	}
	return nil
}

// installAWSCLI is how to get the AWS CLI on the machine this is running on.
//
// The command rather than only the page: a link is something to read, and the
// line for the machine in front of somebody is something to run. Installing it
// here is not on offer — that needs a package manager and usually a password, and
// a tool that installs other tools without being asked is a tool nobody trusts.
//
// The page stays for the platforms with no line here.
func installAWSCLI() string {
	const page = "  https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"

	switch runtime.GOOS {
	case "darwin":
		return "  brew install awscli\n" + page
	case "linux":
		archive := linuxArchive(runtime.GOARCH)
		if archive == "" {
			return page
		}
		return "  curl -s 'https://awscli.amazonaws.com/awscli-exe-linux-" + archive + ".zip' -o awscliv2.zip\n" +
			"  unzip -q awscliv2.zip && sudo ./aws/install\n" + page
	default:
		return page
	}
}

// linuxArchive names the AWS CLI archive for a Go architecture, or returns the
// empty string when there is no archive to name.
//
// AWS publishes one per architecture and no generic one, so handing an ARM
// machine the x86_64 zip produces a binary that cannot run — reported as "cannot
// execute binary file", which reads as a broken download rather than the wrong
// download. Anything else gets the documentation page instead: guessing a URL
// for an architecture AWS may not publish is worse than linking the list.
//
// A function of its own so the mapping can be tested for every architecture
// rather than only for the one the test happens to run on.
func linuxArchive(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return ""
	}
}

// awsCLIMajor reads the major version from `aws --version`, whose first field is
// "aws-cli/<version>". The second return is false when there is nothing to read:
// the command failed, or its output is not in that shape.
func awsCLIMajor(ctx context.Context, path string) (int, bool) {
	command := exec.CommandContext(ctx, path, "--version")
	// The AWS CLI has written this banner to stdout since v2 and to stderr in some
	// v1 builds, and v1 is exactly the case being detected — so both are read.
	output, err := command.CombinedOutput()
	if err != nil {
		return 0, false
	}
	_, rest, found := strings.Cut(string(output), "aws-cli/")
	if !found {
		return 0, false
	}
	major, _, _ := strings.Cut(strings.TrimSpace(rest), ".")
	number, convErr := strconv.Atoi(major)
	if convErr != nil {
		return 0, false
	}
	return number, true
}

// ResolveCredentials asks the AWS CLI to export the profile's current credentials,
// refreshing the SSO token once if it needs it.
//
// The AWS CLI is used rather than a Go SDK for one reason that matters here: it is
// the only implementation that can complete an SSO refresh, and it is already a
// hard requirement of this repository. An empty profile means ambient credentials,
// which the children inherit as they are — there is nothing to resolve and no cache
// to race over.
func ResolveCredentials(ctx context.Context, profile string) (Credentials, error) {
	if profile == "" {
		return Credentials{}, nil
	}

	// --format process, not env-no-export: it is the only shape that carries
	// Expiration, and the lifetime left is what decides whether an apply should
	// start at all. The payload is the credential_process contract, so the field
	// names are fixed by AWS rather than by this code.
	command := exec.CommandContext(ctx, "aws", "configure", "export-credentials",
		"--profile", profile, "--format", "process")
	// stdout and stderr are kept apart, and only stdout is parsed. Merging them had
	// two consequences: a stderr line shaped like KEY=VALUE was read as a credential,
	// and a non-zero exit after the CLI had already printed credentials put
	// AWS_SECRET_ACCESS_KEY into the error text, which the caller then prints.
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return Credentials{}, fmt.Errorf("infra: cannot resolve credentials for profile %q: %w\n%s\n\n"+
			"An expired SSO session is the usual cause:\n  aws sso login --profile %s",
			profile, err, strings.TrimSpace(stderr.String()), profile)
	}

	var payload struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		SessionToken    string `json:"SessionToken"`
		Expiration      string `json:"Expiration"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return Credentials{}, fmt.Errorf("infra: cannot read the credentials of profile %q: %w\n"+
			"  aws configure export-credentials --profile %s --format process",
			profile, err, profile)
	}
	credentials := Credentials{
		AccessKeyID:     payload.AccessKeyID,
		SecretAccessKey: payload.SecretAccessKey,
		SessionToken:    payload.SessionToken,
	}
	// A profile backed by a long-lived access key has no Expiration, and that is
	// not an error: it is a session that never dies. An unparseable one is left
	// zero for the same reason — a lifetime this code cannot read must not become
	// a reason to refuse a run.
	if payload.Expiration != "" {
		if when, err := time.Parse(time.RFC3339, payload.Expiration); err == nil {
			credentials.Expiration = when
		}
	}
	if credentials.AccessKeyID == "" {
		return Credentials{}, fmt.Errorf(
			"infra: profile %q produced no credentials\n"+
				"  aws configure export-credentials --profile %s --format process",
			profile, profile)
	}
	return credentials, nil
}
