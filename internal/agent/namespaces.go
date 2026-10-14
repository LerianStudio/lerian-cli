package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultManagedNamespaces is where the agent may install, unless told
// otherwise.
//
// A Lerian convention rather than a question: the namespaces an agent writes to
// are decided by what Lerian deploys, not by whoever runs the installer, and
// asking produced a prompt whose right answer nobody outside this repository
// could know. --managed-namespaces remains for a cluster that keeps its
// workloads somewhere else.
//
// lerian-apps beside lerian-system, which is where the agent itself runs: one
// namespace for the agent, one for what it installs.
var DefaultManagedNamespaces = []string{"lerian-apps"}

// EnsureNamespaces creates the managed namespaces that are not there yet.
//
// The chart puts a Role and a RoleBinding in each, and Kubernetes has nowhere
// to put a Role in a namespace that does not exist — so an install against a
// fresh cluster failed on a namespace the operator was never asked about and
// had no reason to have created. Making them is this command's job precisely
// because choosing them was.
//
// Each is created on its own: one that exists already must not stop the rest.
func EnsureNamespaces(ctx context.Context, kubeContext string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	binary, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl is not on PATH, and the agent's namespaces (%s) have to exist "+
			"before the chart can place a Role in them:\n"+
			"  macOS  brew install kubectl\n"+
			"  Linux  https://kubernetes.io/docs/tasks/tools/",
			strings.Join(names, ", "))
	}

	for _, name := range names {
		args := []string{"create", "namespace", name}
		if kubeContext != "" {
			args = append(args, "--context", kubeContext)
		}
		// #nosec G204 -- the binary is whatever LookPath found for kubectl, and
		// the names are this package's own constant or a validated flag value.
		out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		if err == nil {
			continue
		}
		// Already there is the expected case on every run after the first, and
		// is not a failure. kubectl says so in its own words rather than with an
		// exit code worth distinguishing.
		if strings.Contains(string(out), "AlreadyExists") {
			continue
		}
		return fmt.Errorf("cannot create namespace %s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}
