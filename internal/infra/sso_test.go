package infra

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// A session or profile name is read out of ~/.aws/config, and it becomes an
// argument to another program. There is no shell involved, so the usual quoting
// worry does not apply — but a name that begins with a dash is read by the AWS
// CLI as a flag rather than as a value, which is how "log into session X" turns
// into "run aws sso login with an option nobody chose".
//
// Rejected rather than escaped: there is no escaping for argv, and no legitimate
// session is called --debug.
func TestANameThatWouldBecomeAFlagIsRefused(t *testing.T) {
	refused := []SSOTarget{
		{Session: "--debug"},
		{Profile: "--region"},
		{Session: "-x"},
		{Session: ""},
		{Profile: ""},
	}

	for _, target := range refused {
		err := SSOLogin(context.Background(), target, strings.NewReader(""), io.Discard, io.Discard)

		// Asserted on the sentinel, not on "an error happened": running
		// `aws sso login --sso-session --debug` fails too, so a bare non-nil check
		// passes against no validation at all.
		if !errors.Is(err, ErrUnsafeTargetName) {
			t.Errorf("%+v gave %v, want ErrUnsafeTargetName", target, err)
		}
	}
}

// And an ordinary name still builds the command it always built.
func TestAnOrdinaryNameBuildsTheUsualCommand(t *testing.T) {
	session := SSOTarget{Session: "acme-sso"}
	if got := strings.Join(session.Args(), " "); got != "sso login --sso-session acme-sso" {
		t.Errorf("args = %q", got)
	}

	profile := SSOTarget{Profile: "acme-sandbox"}
	if got := strings.Join(profile.Args(), " "); got != "sso login --profile acme-sandbox" {
		t.Errorf("args = %q", got)
	}
}
