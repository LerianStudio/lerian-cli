package infra

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ClusterProbe is what one call to the cluster's API said.
type ClusterProbe struct {
	// Version is the Kubernetes version the API server reports, when it answered.
	Version string
	// Problem is the first line of what went wrong, empty when it worked.
	Problem string
	// Hint is what to do about Problem, when the shape of the failure says.
	Hint string
	// Missing is true when kubectl is not installed, which is not a problem with
	// the cluster.
	Missing bool
}

// Works reports whether the cluster answered.
func (p ClusterProbe) Works() bool { return p.Version != "" }

// ProbeCluster asks the API server for its version.
//
// `get --raw /version` rather than `get nodes`: it is one call, it needs no RBAC
// beyond being authenticated, and it fails differently for each of the three
// things that actually go wrong — the endpoint not resolving, the credential not
// being accepted, and the network not reaching it. A nodes listing would blur
// the second and third into "error from server".
//
// The context is named rather than inherited, because the whole point is to
// check the one that was just written, not whatever the file's current-context
// happens to be by the time this runs.
func ProbeCluster(ctx context.Context, kubeContext string) ClusterProbe {
	if _, err := exec.LookPath("kubectl"); err != nil {
		return ClusterProbe{Missing: true}
	}

	// A bounded wait, because the failure this is most likely to meet — an
	// endpoint behind a CIDR allow-list that does not include this machine — is a
	// hang, not a refusal. Ten seconds is long enough for a healthy API server
	// anywhere and short enough to be read as a check rather than a pause.
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := []string{"get", "--raw", "/version", "--request-timeout=8s"}
	if kubeContext != "" {
		args = append(args, "--context", kubeContext)
	}
	command := exec.CommandContext(probe, "kubectl", args...)
	command.Env = os.Environ()

	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return describeProbeFailure(probe, stderr.String(), err)
	}

	var reported struct {
		GitVersion string `json:"gitVersion"`
	}
	if jsonErr := json.Unmarshal(out, &reported); jsonErr != nil || reported.GitVersion == "" {
		// It answered with something, which is the part that matters: reaching the
		// API server and being accepted by it both happened.
		return ClusterProbe{Version: "answered"}
	}
	return ClusterProbe{Version: reported.GitVersion}
}

// describeProbeFailure turns kubectl's output into the one line worth reading,
// plus what to do about it.
//
// The three shapes are worth telling apart because they send somebody to three
// different places, and the symptom of the first one reads like the third: a
// kubeconfig entry for a cluster that was destroyed and recreated fails with
// "no such host", which looks like broken DNS and is not.
func describeProbeFailure(ctx context.Context, stderr string, err error) ClusterProbe {
	message := firstProbeLine(stderr)
	if message == "" {
		message = err.Error()
	}
	probe := ClusterProbe{Problem: message}

	lower := strings.ToLower(message)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		probe.Problem = "the API server did not answer within ten seconds"
		probe.Hint = "the endpoint may be reachable only from allowed addresses: check " +
			"allowed_api_access_cidrs against this machine's address"
	case strings.Contains(lower, "no such host"):
		probe.Hint = "the kubeconfig points at an endpoint that no longer exists — " +
			"a cluster destroyed and recreated keeps its name and gets a new one"
	case strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"):
		probe.Hint = "the credential reached the cluster and was refused: this identity " +
			"needs an EKS access entry"
	case strings.Contains(lower, "i/o timeout"), strings.Contains(lower, "connection refused"):
		probe.Hint = "the endpoint did not accept the connection: check whether this " +
			"machine's address is allowed to reach the API"
	}
	return probe
}

// firstProbeLine is kubectl's complaint without its prefix and without the rest
// of the paragraph.
func firstProbeLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return strings.TrimPrefix(strings.TrimPrefix(line, "error: "), "Error from server: ")
	}
	return ""
}

// ClusterEndpoint is the API server address of one cluster.
//
// Asked for because the kubeconfig comparison needs something to compare
// against: without it, an entry that is already correct is indistinguishable
// from one pointing at a cluster that no longer exists, and both get a
// confirmation. One describe-cluster turns "it will be rewritten, confirm" into
// "nothing to change".
func ClusterEndpoint(ctx context.Context, profile, region, name string) (string, error) {
	args := []string{"eks", "describe-cluster", "--name", name,
		"--query", "cluster.endpoint", "--output", "text"}
	if region != "" {
		args = append(args, "--region", region)
	}

	command := exec.CommandContext(ctx, "aws", args...)
	command.Env = os.Environ()
	if profile != "" {
		command.Env = append(command.Env, "AWS_PROFILE="+profile)
	}

	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
