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
//
// The second return is why there is nothing to offer. "No eks root in this run"
// and "the outputs could not be read" are different things: the first is silence,
// the second is worth a line. Swallowing both meant an apply whose credentials
// expired during its twenty-three minutes simply had no kubectl row, with
// nothing on screen to say so.
func readClusterFacts(
	ctx context.Context,
	outputs outputReader,
	units []infra.Unit,
) (clusterFacts, bool, string) {
	var eks []infra.Unit
	for _, unit := range units {
		if strings.HasSuffix(unit.Name, "/eks") {
			eks = append(eks, unit)
		}
	}
	if len(eks) == 0 {
		return clusterFacts{}, false, ""
	}

	// terraform.Output rather than runner.Outputs: the runner paints into the
	// checklist, and "reading the outputs" under a finished apply is a second
	// progress report for something nobody asked to watch. The directory was
	// initialized by the run that just finished, so there is nothing to set up.
	for _, unit := range eks {
		values, err := outputs.Output(ctx, unit)
		if err != nil {
			return clusterFacts{}, false, err.Error()
		}
		facts := clusterFacts{
			Name:     unquote(values["cluster_name"]),
			ARN:      unquote(values["cluster_arn"]),
			Endpoint: unquote(values["cluster_endpoint"]),
		}
		// All three, not just the name. The ARN is what finds the existing
		// kubeconfig entry, and with it empty planKubeconfig looks up "", finds
		// nothing, reports no replacement — and aws eks update-kubeconfig then
		// overwrites the real entry without the confirmation this flow exists to
		// make. The endpoint is what the probe reaches afterwards.
		if facts.Name != "" && facts.ARN != "" && facts.Endpoint != "" {
			return facts, true, ""
		}
	}
	return clusterFacts{}, false,
		"the eks stack reported no cluster_name, cluster_arn or cluster_endpoint"
}

// outputReader is the half of terraform this needs: the outputs of one root.
// An interface so the two reasons there is no cluster can be told apart in a
// test, which needs no terraform and no AWS.
type outputReader interface {
	Output(ctx context.Context, unit infra.Unit) (map[string]json.RawMessage, error)
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

	switch {
	case plan.Replaces && facts.Endpoint != "":
		fmt.Fprintf(out, "  %s\n", theme.dim("an entry for this cluster is already there, pointing elsewhere:"))
		fmt.Fprintf(out, "    now   %s\n", plan.Current)
		fmt.Fprintf(out, "    after %s\n\n", facts.Endpoint)

		if err := ask.confirm(ctx, out, "Replace it?"); err != nil {
			return err
		}
	case plan.Replaces:
		// No endpoint to compare against, so whether this changes anything is
		// unknown. Saying "pointing elsewhere" would be a claim; saying it will be
		// rewritten is what is actually true.
		fmt.Fprintf(out, "  %s\n    %s\n\n", theme.dim("an entry for this cluster is already there:"),
			plan.Current)
		if err := ask.confirm(ctx, out, "Rewrite it?"); err != nil {
			return err
		}
	default:
		fmt.Fprintf(out, "  %s\n", theme.dim("nothing there to replace"))
	}

	if err := infra.UpdateKubeconfig(ctx, facts.Name, region, profile); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n  kubectl now talks to %s\n", facts.Name)

	reportClusterProbe(out, infra.ProbeCluster(ctx, facts.ARN))
	return nil
}

// reportClusterProbe says whether the context that was just written works.
//
// Written entries are easy to get right and easy to be wrong about: the file can
// name a cluster that no longer exists, an identity the cluster does not accept,
// or an endpoint this machine cannot reach — and all three look the same from
// the outside, which is "kubectl does not work". One call tells them apart while
// somebody is still here to act on it.
func reportClusterProbe(out io.Writer, probe infra.ClusterProbe) {
	theme := newStyle(out)

	switch {
	case probe.Missing:
		fmt.Fprintf(out, "  %s\n\n", theme.dim("kubectl is not installed, so nothing was checked"))
	case probe.Works():
		fmt.Fprintf(out, "  %s\n\n", theme.dim("the cluster answered: "+probe.Version))
	default:
		fmt.Fprintf(out, "  %s\n", theme.dim("the cluster did not answer: "+probe.Problem))
		if probe.Hint != "" {
			fmt.Fprintf(out, "  %s\n", theme.dim(probe.Hint))
		}
		fmt.Fprintln(out)
	}
}
