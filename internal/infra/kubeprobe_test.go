package infra

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The three failures send somebody to three different places, and the first one
// reads like the third: a kubeconfig entry for a cluster that was destroyed and
// recreated fails with "no such host", which looks like broken DNS and is not.
func TestEachFailureShapeGetsItsOwnHint(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name:   "endpoint that no longer exists",
			stderr: `Unable to connect to the server: dial tcp: lookup ABC.gr7.us-east-2.eks.amazonaws.com: no such host`,
			want:   "no longer exists",
		},
		{
			name:   "credential the cluster refuses",
			stderr: "error: You must be logged in to the server (Unauthorized)",
			want:   "access entry",
		},
		{
			name:   "endpoint that will not accept the connection",
			stderr: "Unable to connect to the server: dial tcp 10.0.0.1:443: i/o timeout",
			want:   "allowed to reach the API",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := describeProbeFailure(context.Background(), test.stderr, errors.New("exit status 1"))

			if probe.Works() {
				t.Fatal("a failure was reported as working")
			}
			if !strings.Contains(probe.Hint, test.want) {
				t.Errorf("hint = %q, want something about %q", probe.Hint, test.want)
			}
			if probe.Problem == "" {
				t.Error("nothing says what happened")
			}
		})
	}
}

// A hang is the failure an endpoint behind an address allow-list produces, and
// it needs the deadline to be recognized: kubectl's own message says nothing
// about why nobody answered.
func TestATimeoutIsNamedAsOne(t *testing.T) {
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	// A canceled context is not a deadline; this pins that only a deadline is
	// read as "nobody answered in time".
	plain := describeProbeFailure(expired, "something else", errors.New("x"))
	if strings.Contains(plain.Problem, "ten seconds") {
		t.Errorf("a canceled context was reported as a timeout: %q", plain.Problem)
	}

	deadline, stop := context.WithTimeout(context.Background(), 0)
	defer stop()
	<-deadline.Done()

	probe := describeProbeFailure(deadline, "", errors.New("signal: killed"))
	if !strings.Contains(probe.Problem, "ten seconds") {
		t.Errorf("problem = %q", probe.Problem)
	}
	if !strings.Contains(probe.Hint, "allowed addresses") {
		t.Errorf("hint = %q, want the CIDR allow-list", probe.Hint)
	}
}

// An unrecognized failure still says what happened: a hint nobody can give is
// no reason to withhold the message.
func TestAnUnrecognizedFailureStillReportsIt(t *testing.T) {
	probe := describeProbeFailure(context.Background(),
		"error: something nobody has seen before", errors.New("exit status 1"))

	if !strings.Contains(probe.Problem, "something nobody has seen before") {
		t.Errorf("problem = %q", probe.Problem)
	}
	if probe.Hint != "" {
		t.Errorf("it invented a hint: %q", probe.Hint)
	}
}

// kubectl's own prefixes are noise in a one-line report.
func TestTheMessageLosesKubectlsPrefix(t *testing.T) {
	if got := firstProbeLine("error: nope\nmore detail\n"); got != "nope" {
		t.Errorf("firstProbeLine = %q", got)
	}
	if got := firstProbeLine("\n\nError from server: forbidden\n"); got != "forbidden" {
		t.Errorf("firstProbeLine = %q", got)
	}
}
