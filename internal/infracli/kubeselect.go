package infracli

import (
	"context"
	"fmt"
	"io"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// PointKubectlAtACluster asks which account and which cluster, and points
// kubectl at it.
//
// The same thing the post-run menu offers, reachable without a run. After an
// apply the CLI knows the cluster because it just made it; the rest of the time
// somebody wants to talk to a cluster that already exists — made by a colleague,
// by an older run, or in an account this checkout knows nothing about. That used
// to mean knowing the aws eks update-kubeconfig incantation, including which
// region the cluster is in.
func PointKubectlAtACluster(ctx context.Context, out io.Writer, clusters infra.ClusterLister) error {
	ask := newPrompter(out)
	if !ask.interactive {
		return fmt.Errorf("choosing a cluster needs a terminal\n" +
			"  aws eks update-kubeconfig --name <cluster> --region <region> --profile <profile>")
	}

	profile, account, err := askWhichAccount(ctx, ask, out)
	if err != nil {
		return err
	}

	region, name, err := askWhichCluster(ctx, ask, out, clusters, profile)
	if err != nil {
		return err
	}

	return pointKubectl(ctx, ask, out, clusterFacts{
		Name: name,
		ARN:  infra.ClusterARN("", region, account, name),
		// No endpoint: it would take a describe-cluster call, and the only use for
		// it here is comparing against the kubeconfig. An entry for this cluster
		// that is already there is reported as a replacement either way, which is
		// the careful side to be on.
	}, region, profile)
}

// askWhichAccount offers the profiles in ~/.aws, with the account each one
// reaches.
//
// Resolved rather than listed: a profile name says nothing about which account
// it leads to, and the account is what somebody is choosing. It is also what
// tells two profiles for the same company apart.
func askWhichAccount(ctx context.Context, ask *prompter, out io.Writer) (string, string, error) {
	profiles, err := infra.ListAWSProfiles()
	if err != nil {
		return "", "", err
	}
	if len(profiles) == 0 {
		return "", "", fmt.Errorf("no profile in ~/.aws to choose from\n" +
			"  aws configure sso --profile <name>      # IAM Identity Center\n" +
			"  aws configure --profile <name>          # access key and secret")
	}

	fmt.Fprintf(out, "\n  %s\n", newStyle(out).dim("checking which account each profile reaches..."))
	resolved := infra.ResolveProfiles(ctx, checkIdentity, profiles, "")

	options := make([]option, 0, len(resolved))
	for _, entry := range resolved {
		row := option{value: entry.Profile.Name, label: entry.Profile.Name}
		switch {
		case !entry.Usable():
			row.disabled = true
			row.note = "does not resolve — " + firstLine(entry.Err.Error())
		default:
			row.note = "account " + entry.Caller.Account
			if entry.Profile.Region != "" {
				row.note += "  ·  " + entry.Profile.Region
			}
		}
		options = append(options, row)
	}
	if firstEnabled(options) < 0 || !options[firstEnabled(options)].selectable() {
		return "", "", fmt.Errorf("none of the %d profile(s) in ~/.aws resolve right now\n"+
			"  lerian infra check  says why", len(options))
	}

	picked, err := ask.pick("Which AWS account?",
		"The clusters offered next are the ones this account holds.", "--profile", options, "")
	if err != nil {
		return "", "", err
	}

	for _, entry := range resolved {
		if entry.Profile.Name == picked {
			return picked, entry.Caller.Account, nil
		}
	}
	return "", "", fmt.Errorf("profile %q is not one of the profiles offered", picked)
}

// askWhichCluster offers the clusters in a region, asking for another region
// when that one holds none.
//
// It starts with the profile's own region because that is where somebody's
// clusters usually are, and asking for a region before showing anything would
// make the common case pay for the rare one.
func askWhichCluster(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	clusters infra.ClusterLister,
	profile string,
) (string, string, error) {
	region := profileRegion(profile)

	for {
		if region == "" {
			chosen, err := ask.pick("Which region should it look in?",
				"EKS is per region: a cluster belongs to one, and this asks about one.",
				"--region", regionOptions("", ""), "")
			if err != nil {
				return "", "", err
			}
			region = chosen
		}

		fmt.Fprintf(out, "\n  %s\n", newStyle(out).dim("looking for clusters in "+region+"..."))
		found, err := clusters.ListClusters(ctx, profile, region)
		if err != nil {
			return "", "", err
		}

		if len(found) > 0 {
			name, pickErr := ask.pick("Which cluster?",
				"kubectl will talk to this one until it is pointed somewhere else.",
				"", clusterOptions(found, region), "")
			return region, name, pickErr
		}

		fmt.Fprintf(out, "  %s\n", newStyle(out).dim("no cluster in "+region))
		again, err := Choose(out, "Look in another region?", "",
			[]Choice{
				{Value: "yes", Label: "another region", Note: "the account may hold clusters elsewhere"},
				{Value: "no", Label: "stop here", Note: "nothing is changed"},
			})
		if err != nil || again != "yes" {
			return "", "", infra.ErrAborted
		}
		region = ""
	}
}

// clusterOptions is the clusters of one region, named.
func clusterOptions(names []string, region string) []option {
	options := make([]option, 0, len(names))
	for _, name := range names {
		options = append(options, option{value: name, label: name, note: region})
	}
	return options
}
