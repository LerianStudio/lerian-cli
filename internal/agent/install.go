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
	// KubeContext is the cluster the install will reach, so the namespaces are
	// created in the same one rather than in whatever is current.
	KubeContext() string
	// Installed is what is there now, so a run that would change nothing can
	// say so and stop.
	Installed(ctx context.Context, namespace string) (Release, bool, error)
	// Unchanged reports that this install would leave the release as it is.
	Unchanged(ctx context.Context, opts Options, release Release) (bool, error)
}

// Outcome is what an install turned out to be, so a report can say it rather
// than calling every run an installation.
type Outcome int

const (
	// Installed is a release that was not there before.
	Installed Outcome = iota
	// Reconfigured is a release that was there and now holds different values
	// or a different chart.
	Reconfigured
	// Unmodified is a release already in the state this install describes.
	// Nothing ran.
	Unmodified
)

// Install is the one installation, whichever way somebody arrived at it.
//
// Both entry points come through here: `lerian agent install`, and the offer at
// the end of a cluster deploy. They differ only in where the values came from —
// flags in one case, a run that already knows the cluster in the other — and
// nothing about the install itself should depend on which it was.
func Install(ctx context.Context, helm Installer, opts Options, timeout time.Duration) error {
	_, err := InstallReporting(ctx, helm, opts, timeout)
	return err
}

// PlanInstall reports what an install would do, without doing any of it.
//
// Asked before the confirmation, because confirming an operation that turns out
// to write nothing is a question nobody needed — and because what is already
// installed is the thing somebody wants to see before saying yes.
func PlanInstall(ctx context.Context, helm Installer, opts Options) (Outcome, Release, error) {
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if err := opts.Validate(); err != nil {
		return Installed, Release{}, err
	}

	// An unreadable release is not a reason to refuse: the install then runs,
	// which is what it would have done anyway.
	release, installed, err := helm.Installed(ctx, opts.Namespace)
	if err != nil || !installed {
		return Installed, Release{}, nil //nolint:nilerr // unreadable means "go ahead".
	}

	unchanged, err := helm.Unchanged(ctx, opts, release)
	if err != nil {
		return Reconfigured, release, err
	}
	if unchanged {
		return Unmodified, release, nil
	}
	return Reconfigured, release, nil
}

// InstallReporting is Install, and says what the run turned out to be.
//
// Running it twice has always converged on the same state — helm upgrade
// --install sees to that — but it wrote a revision, re-applied every manifest
// and reported as though something had happened. A second run should be free,
// and should say that nothing was needed.
func InstallReporting(
	ctx context.Context,
	helm Installer,
	opts Options,
	timeout time.Duration,
) (Outcome, error) {
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	// Here, not only inside the Helm call: this is the door both entry points
	// come through, and a wrong flag should cost nothing — not a chart pull, not
	// a partial release, not three minutes of waiting.
	if err := opts.Validate(); err != nil {
		return Installed, err
	}

	// What is there now, so a run that would change nothing can say so and stop.
	outcome, _, err := PlanInstall(ctx, helm, opts)
	if err != nil {
		return outcome, err
	}
	if outcome == Unmodified {
		return Unmodified, nil
	}

	// Before helm, because the chart places a Role in each and Kubernetes has
	// nowhere to put one in a namespace that does not exist. This used to be the
	// operator's problem, discovered three minutes into an install.
	if err := EnsureNamespaces(ctx, helm.KubeContext(), opts.ManagedNamespaces); err != nil {
		return outcome, err
	}

	if err := helm.Install(ctx, opts, timeout); err != nil {
		// Helm's own message is accurate and stays. What it leaves out is which
		// thing the operator controls is wrong, and these are the two that are
		// wrong often enough to be worth naming.
		if advice := adviseOn(err, opts.Namespace); advice != "" {
			return outcome, fmt.Errorf("%w\n\n%s", err, advice)
		}
		return outcome, err
	}
	return outcome, nil
}

// missingNamespace catches the chart failing to place a Role, which it does
// once per managed namespace that does not exist.
var missingNamespace = regexp.MustCompile(`namespaces "([^"]+)" not found`)

// adviseOn turns the failures worth recognizing into what to do about them.
func adviseOn(err error, namespace string) string {
	if namespace == "" {
		namespace = DefaultNamespace
	}
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
		// What the error proves is that readiness did not arrive in time. Why is
		// a separate question — the pod may be running and unable to register, or
		// may not have started at all — and the pod is the only thing that says
		// which. Claiming registration outright sent people to debug a control
		// plane when the image could not be pulled.
		// Not "the objects were created": a context that expires before helm
		// applies the chart reaches here too, with nothing created at all. What
		// is true in both cases is that it timed out, and the two commands below
		// say which happened.
		return "Timed out. Whether the release and the pod exist is the first thing to\n" +
			"check — a context that expires before helm applies the chart arrives here\n" +
			"too, with nothing created at all.\n\n" +
			"What is there:\n" +
			"  helm list -n " + namespace + "\n" +
			"  kubectl -n " + namespace + " get pods -l app.kubernetes.io/name=" + ReleaseName + "\n\n" +
			"A pod that is running but not ready is usually registration: the agent\n" +
			"reports ready on /health/ready only once it has registered, so an\n" +
			"unreachable control plane — or one that does not exist yet — makes this a\n" +
			"wait that cannot end. For that case, --wait=false returns once the objects\n" +
			"exist, and --no-control-plane installs against a placeholder.\n\n" +
			"  kubectl -n " + namespace + " logs deploy/" + ReleaseName + "\n\n" +
			"Pending, CrashLoopBackOff or ImagePullBackOff is something else, and the\n" +
			"commands above say which."
	}

	if strings.Contains(text, "cannot re-use a name that is still in use") {
		return "A release called " + ReleaseName + " is already there and helm will not reuse the name.\n" +
			"`lerian agent status` says what it is; `lerian agent uninstall` removes it, keeping\n" +
			"the Secret " + IdentitySecretName + " so a reinstall is still the same agent."
	}

	return ""
}
