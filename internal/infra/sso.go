package infra

import (
	"context"
	"errors"
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

// ErrUnsafeTargetName is returned for a name that would reach the AWS CLI as an
// option rather than as a value.
var ErrUnsafeTargetName = errors.New("infra: unusable SSO target name")

// validate refuses a name that would be read as a flag.
//
// There is no shell here — exec.Command passes argv straight through, so quoting
// is not the worry. The worry is argument parsing: a session called "--debug"
// arrives at the AWS CLI as an option, and "log into session X" becomes "run aws
// sso login with an option nobody chose". Names come out of ~/.aws/config, which
// the operator owns, so this is a guard rather than a boundary — but there is no
// escaping for argv, and no legitimate session is called --debug.
func (t SSOTarget) validate() error {
	name := t.Name()
	if name == "" {
		return fmt.Errorf("%w: empty", ErrUnsafeTargetName)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("%w: %q begins with a dash, so the AWS CLI would read it as an option.\n"+
			"Rename the profile or sso-session in ~/.aws/config.", ErrUnsafeTargetName, name)
	}
	return nil
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
	if err := target.validate(); err != nil {
		return err
	}

	// #nosec G204 -- the binary is the literal "aws", and the one variable part is
	// the target name, checked just above for the only thing argv is vulnerable to:
	// a value that parses as an option. No shell is involved.
	command := exec.CommandContext(ctx, "aws", target.Args()...)
	command.Stdin = in
	command.Stdout = out
	command.Stderr = errOut

	if err := command.Run(); err != nil {
		return fmt.Errorf("infra: %s failed: %w", target, err)
	}
	return nil
}

// SSOLogout ends the SSO session, so the next login is a fresh one.
//
// This reaches further than this tool: the token it clears lives in
// ~/.aws/sso/cache, which every AWS client on the machine reads — the AWS CLI in
// another terminal, Terraform, anything using the shared config. Ending it is a
// decision about the machine, not about this command, which is why nothing here
// does it without being asked.
func SSOLogout(ctx context.Context, in io.Reader, out, errOut io.Writer) error {
	command := exec.CommandContext(ctx, "aws", "sso", "logout")
	command.Stdin = in
	command.Stdout = out
	command.Stderr = errOut

	if err := command.Run(); err != nil {
		return fmt.Errorf("infra: aws sso logout failed: %w", err)
	}
	return nil
}

// ConfigureAWS runs the AWS CLI's own setup, so a machine with nothing in
// ~/.aws can be given credentials without leaving.
//
// mode is "sso" for IAM Identity Center — what an organization hands out, and
// what the rest of this tool assumes — or "keys" for a long-lived access key,
// which is what somebody gets when there is no portal to log into.
//
// Shelled out for the same reason the login is: the answers land in ~/.aws in the
// layout every AWS client reads, and the CLI that owns that file is already a
// verified dependency here. The streams pass through because both flows are
// conversations — one opens a browser, the other asks four questions.
func ConfigureAWS(ctx context.Context, mode string, in io.Reader, out, errOut io.Writer) error {
	var args []string
	switch mode {
	case "sso":
		args = []string{"configure", "sso"}
	case "keys":
		args = []string{"configure"}
	default:
		return fmt.Errorf("infra: unknown AWS setup %q", mode)
	}

	// #nosec G204 -- args is one of the two literal slices above. mode selects
	// between them and anything else is refused, so nothing from outside reaches
	// the command line.
	command := exec.CommandContext(ctx, "aws", args...)
	command.Stdin = in
	command.Stdout = out
	command.Stderr = errOut

	if err := command.Run(); err != nil {
		return fmt.Errorf("infra: aws %s failed: %w", strings.Join(args, " "), err)
	}
	return nil
}
