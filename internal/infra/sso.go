package infra

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// SSOTarget is one thing that can be logged into.
//
// A session where there is one, because profiles that share an [sso-session]
// share a login: an organization with a dozen account profiles behind one
// session is revived by a single command. A profile otherwise, for the older
// per-profile SSO configuration.
type SSOTarget struct {
	Session string
	Profile string
}

// Args is the AWS CLI invocation this target needs.
func (t SSOTarget) Args() []string {
	if t.Session != "" {
		return []string{"sso", "login", "--sso-session", t.Session}
	}
	return []string{"sso", "login", "--profile", t.Profile}
}

// Name is the target as an operator would say it.
func (t SSOTarget) Name() string {
	if t.Session != "" {
		return t.Session
	}
	return t.Profile
}

// String makes a target readable in test failures and logs.
func (t SSOTarget) String() string { return strings.Join(t.Args(), " ") }

// SSOLogin runs the AWS CLI's own login.
//
// Shelled out rather than reimplemented. The device authorization flow means
// opening a browser, polling for the grant and writing a token into
// ~/.aws/sso/cache in the layout every other AWS tool expects to find it —
// reimplementing that would put this code in the business of managing somebody
// else's credential cache, and the CLI that owns that cache is already a verified
// dependency of this command.
//
// The streams are passed through because the flow is a conversation: the CLI
// prints a code and a URL, opens a browser, and waits. Capturing that output
// would leave the operator staring at a stopped command with the code they need
// held in a buffer.
func SSOLogin(ctx context.Context, target SSOTarget, in io.Reader, out, errOut io.Writer) error {
	command := exec.CommandContext(ctx, "aws", target.Args()...)
	command.Stdin = in
	command.Stdout = out
	command.Stderr = errOut

	if err := command.Run(); err != nil {
		return fmt.Errorf("infra: %s failed: %w", target, err)
	}
	return nil
}
