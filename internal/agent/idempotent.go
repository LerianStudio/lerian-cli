package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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
func (h *Helm) NewestChart(ctx context.Context) (string, error) {
	out, err := h.output(ctx, "show", "chart", Chart)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if version, ok := strings.CutPrefix(strings.TrimSpace(line), "version:"); ok {
			return strings.TrimSpace(version), nil
		}
	}
	return "", fmt.Errorf("agent: the chart did not report a version")
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
