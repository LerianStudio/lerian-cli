package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Unchanged reports that an install would leave the cluster exactly as it is.
//
// helm upgrade --install converges on the same state either way, so a second
// install is already safe — but it writes a revision, re-applies every manifest
// and reports as though something happened. Running it again should be free and
// should say so.
//
// Two things have to match: the values, which is what the operator controls,
// and the chart version, which is what "newest" resolved to the first time and
// may have moved since.
func (h *Helm) Unchanged(ctx context.Context, opts Options, release Release) (bool, error) {
	if release.Status != "deployed" {
		// A failed or pending release is not a state to preserve. Re-running is
		// how somebody recovers from one.
		return false, nil
	}

	same, err := sameValues(ctx, h, opts)
	if err != nil || !same {
		return false, err
	}
	return sameChart(ctx, h, opts, release)
}

// sameValues compares what the release was given with what this install would
// give it.
func sameValues(ctx context.Context, helm *Helm, opts Options) (bool, error) {
	out, err := helm.output(ctx, "get", "values", ReleaseName,
		"--namespace", opts.Namespace, "--output", "json")
	if err != nil {
		// Not an error worth stopping for: if the values cannot be read, the
		// install runs, which is what it would have done anyway.
		return false, nil //nolint:nilerr // see above: unreadable means "go ahead".
	}

	var installed map[string]any
	if err := json.Unmarshal([]byte(out), &installed); err != nil {
		return false, nil
	}

	// Through JSON on both sides so the comparison is of data rather than of Go
	// types: the values are built here as map[string]any with typed leaves, and
	// come back from helm with everything JSON-shaped.
	wanted, err := roundTrip(opts.Values())
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(installed, wanted), nil
}

// roundTrip renders a value the way helm would have stored it.
func roundTrip(value map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("agent: cannot compare the chart values: %w", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, fmt.Errorf("agent: cannot compare the chart values: %w", err)
	}
	return decoded, nil
}

// sameChart reports whether the release is already on the version this install
// would put there.
func sameChart(ctx context.Context, helm *Helm, opts Options, release Release) (bool, error) {
	wanted := opts.ChartVersion
	if wanted == "" {
		// "Newest" is only a version once somebody asks the registry which. A
		// release installed last week may be a version behind without anything
		// here having changed.
		newest, err := helm.NewestChart(ctx)
		if err != nil {
			// Unknown means run: being wrong in this direction costs a revision,
			// and being wrong in the other leaves an agent a version behind
			// while the CLI says there is nothing to do.
			return false, nil //nolint:nilerr // see above.
		}
		wanted = newest
	}
	// release.Chart is "agent-helm-1.0.1"; the version is what follows the name.
	return strings.TrimPrefix(release.Chart, "agent-helm-") == wanted, nil
}

// NewestChart asks the registry what version "unpinned" means today.
//
// Parsed rather than grepped. `helm show chart` prints the whole Chart.yaml,
// dependencies included, and a dependency carries a version line of its own —
// indented, but the first one in the file. Matching the first "version:" read a
// subchart's 2.2.0 as the agent chart's, and an upgrade was offered to a
// version that does not exist.
func (h *Helm) NewestChart(ctx context.Context) (string, error) {
	out, err := h.output(ctx, "show", "chart", Chart)
	if err != nil {
		return "", err
	}

	var chart struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(out), &chart); err != nil {
		return "", fmt.Errorf("agent: cannot read what the chart reported: %w", err)
	}
	if chart.Version == "" {
		return "", fmt.Errorf("agent: the chart did not report a version")
	}
	return chart.Version, nil
}

// marshal renders values the way helm's own output is shaped, for tests and for
// anything that needs to compare against what helm stored.
func marshal(values map[string]any) (string, error) {
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("agent: cannot render the chart values: %w", err)
	}
	return string(encoded), nil
}

// PointsAtPlaceholder reports that the installed release was never going to
// connect.
//
// Worth knowing before an upgrade: readiness means registered, so an agent
// pointed at a placeholder is not ready and will not become ready by being
// given a new chart. Waiting for it spends the whole timeout and then reports
// a failure that is really the state it was already in.
func (h *Helm) PointsAtPlaceholder(ctx context.Context, namespace string) bool {
	out, err := h.output(ctx, "get", "values", ReleaseName,
		"--namespace", namespace, "--output", "json")
	if err != nil {
		return false
	}
	var values struct {
		Agent struct {
			ConfigMap struct {
				ControlPlaneURL string `json:"CONTROL_PLANE_URL"`
			} `json:"configmap"`
		} `json:"agent"`
	}
	if err := json.Unmarshal([]byte(out), &values); err != nil {
		return false
	}
	return values.Agent.ConfigMap.ControlPlaneURL == PlaceholderControlPlane
}

// UpgradeNeeded reports whether a release is already on the version an upgrade
// would move it to, and which version that is.
//
// Asked before the upgrade runs, because "move it to another chart version"
// with no other version to move to is a Helm run that rewrites the release for
// nothing — and, against an agent that cannot become ready, one that waits the
// whole timeout before saying so.
func (h *Helm) UpgradeNeeded(ctx context.Context, wanted string, release Release) (bool, string, error) {
	if wanted == "" {
		newest, err := h.NewestChart(ctx)
		if err != nil {
			return false, "", err
		}
		wanted = newest
	}
	current := strings.TrimPrefix(release.Chart, "agent-helm-")
	return current != wanted, wanted, nil
}
