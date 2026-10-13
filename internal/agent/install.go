package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Installer is the half of Helm that Install drives. An interface so a test can
// hand it a failure and see what the operator is told — which is the whole
// point of this function, and is not visible from a test of the advice alone.
type Installer interface {
	Install(ctx context.Context, opts Options, timeout time.Duration) error
}

// Install is the one installation, whichever way somebody arrived at it.
//
// Both entry points come through here: `lerian agent install`, and the offer at
// the end of a cluster deploy. They differ only in where the values came from —
// flags in one case, a run that already knows the cluster in the other — and
// nothing about the install itself should depend on which it was.
func Install(ctx context.Context, helm Installer, opts Options, timeout time.Duration) error {
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	// Here, not only inside the Helm call: this is the door both entry points
	// come through, and a wrong flag should cost nothing — not a chart pull, not
	// a partial release, not three minutes of waiting.
	if err := opts.Validate(); err != nil {
		return err
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	err := helm.Install(ctx, opts, timeout)
	if err == nil {
		return nil
	}
	// Helm's own message is accurate and stays. What it leaves out is which
	// thing the operator controls is wrong, and these are the two that are
	// wrong often enough to be worth naming.
	if advice := adviseOn(err); advice != "" {
		return fmt.Errorf("%w\n\n%s", err, advice)
	}
	return err
}

// missingNamespace catches the chart failing to place a Role, which it does
// once per managed namespace that does not exist.
var missingNamespace = regexp.MustCompile(`namespaces "([^"]+)" not found`)

// adviseOn turns the failures worth recognizing into what to do about them.
func adviseOn(err error) string {
	text := err.Error()

	if matches := missingNamespace.FindAllStringSubmatch(text, -1); matches != nil {
		seen := map[string]bool{}
		var names []string
		for _, match := range matches {
			if !seen[match[1]] {
				seen[match[1]] = true
				names = append(names, match[1])
			}
		}
		var out strings.Builder
		out.WriteString("Every managed namespace must exist before the agent is installed: the chart\n")
		out.WriteString("puts a Role and a RoleBinding in each, and Kubernetes has nowhere to put them\n")
		out.WriteString("otherwise. Create them, or leave them out of --managed-namespaces:\n")
		for _, name := range names {
			fmt.Fprintf(&out, "  kubectl create namespace %s\n", name)
		}
		return strings.TrimRight(out.String(), "\n")
	}

	// The timeout somebody hits when the control plane is not there yet. The
	// install did almost everything: the release exists and the pod is running.
	// What did not happen is readiness, and readiness means registered.
	if strings.Contains(text, "timed out waiting") || strings.Contains(text, "context deadline exceeded") {
		return "The objects were created and the pod is running; what did not happen is readiness.\n" +
			"The agent reports ready on /health/ready only once it has registered with the\n" +
			"control plane, so a control plane that is unreachable — or does not exist yet —\n" +
			"means this wait can never end.\n\n" +
			"What it is doing right now:\n" +
			"  kubectl -n " + DefaultNamespace + " logs deploy/" + ReleaseName + "\n\n" +
			"To install without waiting for readiness, which is what you want while the\n" +
			"control plane is still being built:\n" +
			"  lerian agent install --wait=false ..."
	}

	if strings.Contains(text, "cannot re-use a name that is still in use") {
		return "A release called " + ReleaseName + " is already there and helm will not reuse the name.\n" +
			"`lerian agent status` says what it is; `lerian agent uninstall` removes it, keeping\n" +
			"the Secret " + IdentitySecretName + " so a reinstall is still the same agent."
	}

	return ""
}
