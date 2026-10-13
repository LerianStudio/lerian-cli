package infra

import (
	"fmt"
	"regexp"
	"strings"
)

// ResourceInUse is AWS refusing a change because something already exists, or
// still exists, that Terraform's state does not account for.
//
// It is the signature of a run that was killed between creating a resource and
// writing the state — see the grace period in tfexec.go for how that happens.
// The resource is real, the state is behind, and every later run repeats the
// same failure: an apply tries to create what is there, a destroy tries to
// remove the parent of something it does not know about.
//
// Terraform's own message is accurate and stays on screen. What it does not say
// is why a resource it has no record of already exists, or that one command
// fixes it.
type ResourceInUse struct {
	// Address is the Terraform address the failure named, when it named one.
	Address string
	// Kind is what AWS refused to touch, for the sentence that explains it.
	Kind string
	// ImportID is the id `terraform import` wants, when it can be built.
	ImportID string
	// Orphan is true when the resource exists and the state does not know it —
	// the importable case. False when the state is the thing holding a reference
	// to something that must go first.
	Orphan bool
}

var (
	// "ResourceInUseException: NodeGroup already exists with name X and cluster name Y"
	nodeGroupExists = regexp.MustCompile(
		`NodeGroup already exists with name ([^\s]+) and cluster name ([^\s]+)`)
	// "ResourceInUseException: Cluster has nodegroups attached"
	clusterHasNodeGroups = regexp.MustCompile(`Cluster has nodegroups attached`)
	// The "with <address>," line Terraform prints under the error.
	erroredAddress = regexp.MustCompile(`(?m)^\s*with ([^\s,]+),`)
)

// ReadResourceInUse recognizes the failure in a terraform error, or returns nil.
func ReadResourceInUse(text string) *ResourceInUse {
	if !strings.Contains(text, "ResourceInUseException") {
		return nil
	}

	address := ""
	if match := erroredAddress.FindStringSubmatch(text); match != nil {
		address = match[1]
	}

	if match := nodeGroupExists.FindStringSubmatch(text); match != nil {
		name, cluster := match[1], match[2]
		return &ResourceInUse{
			Address:  address,
			Kind:     fmt.Sprintf("the node group %s", name),
			ImportID: cluster + ":" + name,
			Orphan:   true,
		}
	}

	if clusterHasNodeGroups.MatchString(text) {
		return &ResourceInUse{
			Address: address,
			Kind:    "the cluster's node groups",
			Orphan:  false,
		}
	}

	// Some other ResourceInUseException. Saying the general shape is still worth
	// more than nothing: the cause is the same even when the resource is one
	// this does not have a command for.
	return &ResourceInUse{Address: address, Kind: "the resource named above"}
}

// Advice is what to do about it, with this run's directory already in it.
func (r ResourceInUse) Advice(dir string) string {
	var out strings.Builder

	out.WriteString("This is drift, not a conflict with somebody else.\n")
	out.WriteString(fmt.Sprintf(
		"AWS has %s; the state does not. That happens when a run is killed between\n"+
			"creating a resource and writing the state down — the API call completed, the\n"+
			"record never landed. Every later run repeats this failure until the state\n"+
			"catches up.\n", r.Kind))

	switch {
	case r.Orphan && r.Address != "" && r.ImportID != "":
		out.WriteString("\nTo put it back in the state:\n")
		out.WriteString(fmt.Sprintf("  terraform -chdir=%s import '%s' %s\n",
			dir, r.Address, r.ImportID))
		out.WriteString("\nIf that import is refused by a count the module cannot resolve, and the\n" +
			"intent was to destroy anyway, deleting the resource in AWS reaches the same\n" +
			"place: the state never knew about it.\n")
	case !r.Orphan:
		out.WriteString("\nThe thing in the way is not in the state either, so Terraform cannot remove\n" +
			"it first. List what is attached and deal with it before re-running:\n")
		out.WriteString("  aws eks list-nodegroups --cluster-name <cluster> --region <region>\n")
	default:
		out.WriteString("\nCompare what AWS has against `terraform state list`, and import what is\n" +
			"missing before re-running.\n")
	}
	return strings.TrimRight(out.String(), "\n")
}
