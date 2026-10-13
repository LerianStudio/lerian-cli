package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout is how long an install may take before helm gives up. The
// chart waits for the Deployment to be ready, and a cluster pulling the image
// for the first time is the slow case.
const DefaultTimeout = 5 * time.Minute

// Helm drives the helm binary.
//
// Shelling out rather than linking the Helm SDK, which is what this CLI already
// does for terraform, aws, gh and kubectl. The SDK would remove a dependency
// the operator can see and understand, and replace it with one pinned inside
// our binary that upgrades on our schedule rather than theirs.
type Helm struct {
	binary string
	// Context is the kubeconfig context to act in. Empty uses the current one,
	// which is what somebody running this by hand expects.
	Context string
	// Out receives helm's own output while it runs, so an install that is slow
	// does not look like one that is stuck.
	Out io.Writer
}

// NewHelm finds the helm binary, or says what to install.
func NewHelm() (*Helm, error) {
	path, err := exec.LookPath("helm")
	if err != nil {
		return nil, fmt.Errorf("agent: helm is not on PATH. It installs the agent into the cluster:\n" +
			"  macOS  brew install helm\n" +
			"  Linux  https://helm.sh/docs/intro/install/")
	}
	return &Helm{binary: path}, nil
}

// Release is what helm knows about an installed agent.
type Release struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
	Updated    string `json:"updated"`
}

// Installed reports the agent's release, or false when there is none.
//
// `helm list` rather than `helm status`, because status exits non-zero for a
// release that is absent and that is not an error here — "not installed" is an
// answer this has to be able to give.
func (h *Helm) Installed(ctx context.Context, namespace string) (Release, bool, error) {
	// The states named rather than --all: helm 4 dropped that flag, and these
	// three exist in both 3 and 4. They are also the ones worth seeing — a
	// superseded or uninstalled revision is history, not something installed.
	out, err := h.output(ctx, "list", "--namespace", namespace,
		"--deployed", "--failed", "--pending", "--output", "json")
	if err != nil {
		return Release{}, false, err
	}
	var releases []Release
	if err := json.Unmarshal([]byte(out), &releases); err != nil {
		return Release{}, false, fmt.Errorf("agent: cannot read what helm reported: %w", err)
	}
	for _, release := range releases {
		if release.Name == ReleaseName {
			return release, true, nil
		}
	}
	return Release{}, false, nil
}

// Install puts the agent in the cluster, or upgrades the one already there.
//
// One operation for both, via `helm upgrade --install`: the alternative is
// asking "is it installed?" and racing whoever answers differently a second
// later. Helm settles it atomically.
func (h *Helm) Install(ctx context.Context, opts Options, timeout time.Duration) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	valuesPath, cleanup, err := opts.WriteValues()
	if err != nil {
		return err
	}
	defer cleanup()

	// Not --atomic. A rollback deletes the pod, and the pod is the evidence:
	// `kubectl logs` on a failed install is how anybody finds out whether the
	// token was wrong or a managed namespace was missing.
	if err := h.run(ctx, installArgs(opts, valuesPath, timeout)...); err != nil {
		return fmt.Errorf("agent: helm could not install the agent: %w", err)
	}
	return nil
}

// installArgs is the command line Install runs.
//
// Built apart from running it so a test can read it: whether --wait is there
// decides whether an install against a control plane that does not exist yet
// finishes or hangs for the whole timeout, and that is worth pinning without a
// cluster.
func installArgs(opts Options, valuesPath string, timeout time.Duration) []string {
	args := []string{
		"upgrade", ReleaseName, Chart,
		"--install",
		"--namespace", opts.Namespace,
		"--create-namespace",
		"--values", valuesPath,
		"--timeout", timeout.String(),
	}
	if !opts.NoWait {
		args = append(args, "--wait")
	}
	if opts.ChartVersion != "" {
		args = append(args, "--version", opts.ChartVersion)
	}
	return args
}

// Reuse upgrades the release while keeping every value it already has.
//
// --reuse-values, so an upgrade does not have to be handed the credential
// again. The alternative is asking for a token to change a version, which
// invites somebody to paste one into a shell for no reason.
func (h *Helm) Reuse(ctx context.Context, version, namespace string, timeout time.Duration, noWait bool) error {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	args := []string{
		"upgrade", ReleaseName, Chart,
		"--namespace", namespace,
		"--reuse-values",
		"--timeout", timeout.String(),
	}
	// An agent that was never able to register is not ready and will not become
	// ready by being upgraded. Waiting for it spends the whole timeout and then
	// reports the upgrade as failed, when what actually happened is that the
	// chart was replaced and the pod is doing exactly what it was doing before.
	if !noWait {
		args = append(args, "--wait")
	}
	if version != "" {
		args = append(args, "--version", version)
	}
	if err := h.run(ctx, args...); err != nil {
		err = fmt.Errorf("agent: helm could not upgrade the agent: %w", err)
		if advice := adviseOn(err, namespace); advice != "" {
			return fmt.Errorf("%w\n\n%s", err, advice)
		}
		return err
	}
	return nil
}

// Uninstall removes the release, leaving the identity Secret behind.
//
// Keeping it is the chart's own behavior and the right one: the control plane
// knows this agent by an identity it was given once, and a reinstall that threw
// it away would arrive as a stranger.
func (h *Helm) Uninstall(ctx context.Context, namespace string) error {
	if err := h.run(ctx, "uninstall", ReleaseName, "--namespace", namespace, "--wait"); err != nil {
		return fmt.Errorf("agent: helm could not remove the agent: %w", err)
	}
	return nil
}

// run executes helm with its output going to the operator.
func (h *Helm) run(ctx context.Context, args ...string) error {
	// #nosec G204 -- the binary is whatever LookPath found for "helm", and every
	// argument is built in this file from values Options.Validate has already
	// accepted. Nothing an operator types becomes a command name, and the values
	// that do come from them reach helm through a file rather than argv.
	command := exec.CommandContext(ctx, h.binary, h.withContext(args)...)
	command.Stdout = h.Out
	command.Stderr = h.Out
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return fmt.Errorf("canceled; helm may have left a release behind — `helm list -A` says")
		}
		return err
	}
	return nil
}

// output executes helm and returns what it printed, for the commands that are
// read rather than watched.
func (h *Helm) output(ctx context.Context, args ...string) (string, error) {
	// #nosec G204 -- see run above: fixed binary, arguments built here.
	command := exec.CommandContext(ctx, h.binary, h.withContext(args)...)
	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("agent: helm %s failed: %s", args[0], message)
	}
	return string(out), nil
}

// withContext adds --kube-context when one was chosen.
//
// Chosen, not assumed. An install that lands in "whichever cluster the operator
// last selected" is the accident worth one flag.
func (h *Helm) withContext(args []string) []string {
	if h.Context == "" {
		return args
	}
	return append(args, "--kube-context", h.Context)
}
