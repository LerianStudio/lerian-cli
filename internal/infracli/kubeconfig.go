package infracli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// clusterFacts is what a run has to know before it can point kubectl anywhere.
type clusterFacts struct {
	Name     string
	ARN      string
	Endpoint string
}

// readClusterFacts reads the EKS stack's outputs, which is the only place the
// cluster's name is a fact rather than a guess.
//
// The name is built inside the templates from a naming module, so reconstructing
// it here would be a second implementation of somebody else's convention — right
// until they rename something.
func readClusterFacts(ctx context.Context, runner *infra.Runner, units []infra.Unit) (clusterFacts, bool) {
	var eks []infra.Unit
	for _, unit := range units {
		if strings.HasSuffix(unit.Name, "/eks") {
			eks = append(eks, unit)
		}
	}
	if len(eks) == 0 {
		return clusterFacts{}, false
	}

	outputs, err := runner.Outputs(ctx, eks)
	if err != nil {
		// No outputs means no cluster to point at — a plan, or an apply that did
		// not get that far. Not a failure: the offer simply is not made.
		return clusterFacts{}, false
	}

	for _, values := range outputs {
		facts := clusterFacts{
			Name:     unquote(values["cluster_name"]),
			ARN:      unquote(values["cluster_arn"]),
			Endpoint: unquote(values["cluster_endpoint"]),
		}
		if facts.Name != "" {
			return facts, true
		}
	}
	return clusterFacts{}, false
}

// unquote turns a JSON output value into the string it holds, or the empty
// string for anything that is not one.
func unquote(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// kubeconfigPlan is what pointing kubectl at this cluster would do to the file.
type kubeconfigPlan struct {
	Path string
	// Replaces is true when an entry for this cluster is already there and points
	// somewhere else — a cluster destroyed and recreated keeps its name and gets a
	// new endpoint, which is exactly the case worth a confirmation.
	Replaces bool
	// Current is the endpoint the file points at now, when it has one.
	Current string
}

// planKubeconfig works out what would change, without changing it.
func planKubeconfig(facts clusterFacts) (kubeconfigPlan, error) {
	path, err := infra.KubeconfigPath()
	if err != nil {
		return kubeconfigPlan{}, err
	}

	entry, found, err := infra.ReadKubeconfig(path, facts.ARN)
	if err != nil {
		return kubeconfigPlan{}, err
	}
	plan := kubeconfigPlan{Path: path}
	if found && entry.Server != facts.Endpoint {
		plan.Replaces = true
		plan.Current = entry.Server
	}
	return plan, nil
}

// pointKubectl offers to run update-kubeconfig, and asks first when doing so
// would overwrite an entry that points somewhere else.
//
// The confirmation is not ceremony. A cluster destroyed and recreated keeps its
// name and gets a new endpoint, so the old entry is stale and replacing it is
// the fix — but an entry pointing elsewhere can also be a second cluster of the
// same name in another account, and overwriting that one silently moves kubectl
// off a cluster somebody is working against.
func pointKubectl(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	facts clusterFacts,
	region, profile string,
) error {
	plan, err := planKubeconfig(facts)
	if err != nil {
		return err
	}

	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> kubectl"))
	fmt.Fprintf(out, "  cluster   %s\n", facts.Name)
	fmt.Fprintf(out, "  file      %s\n", plan.Path)

	if plan.Replaces {
		fmt.Fprintf(out, "  %s\n", theme.dim("an entry for this cluster is already there, pointing elsewhere:"))
		fmt.Fprintf(out, "    now   %s\n", plan.Current)
		fmt.Fprintf(out, "    after %s\n\n", facts.Endpoint)

		if err := ask.confirm(ctx, out, "Replace it?"); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "  %s\n", theme.dim("nothing there to replace"))
	}

	if err := infra.UpdateKubeconfig(ctx, facts.Name, region, profile); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n  kubectl now talks to %s\n\n", facts.Name)
	return nil
}
