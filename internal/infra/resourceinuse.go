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
	// Blocked is true when something attached to the resource must go first.
	// Separate from !Orphan: a ResourceInUseException this does not recognize is
	// neither, and telling somebody to list node groups for a failure that has
	// nothing to do with EKS is worse than saying less.
	Blocked bool
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
			Kind:    "node groups attached to the cluster",
			Blocked: true,
		}
	}

	// Some other ResourceInUseException. Saying the general shape is still worth
	// more than nothing: the cause is the same even when the resource is one
	// this does not have a command for.
	return &ResourceInUse{Address: address, Kind: "the resource named above"}
}

// shellQuote renders a path for a command line somebody is invited to run.
//
// Single quotes, not %q: a checkout under "/Users/a b/work" splits into two
// arguments otherwise, and the ready-to-run command this promises would not
// run. Inside single quotes nothing is substituted; the only character needing
// care is the quote itself, which has to be closed, emitted and reopened.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// Advice is what to do about it, with this run's directory already in it.
func (r ResourceInUse) Advice(dir string) string {
	var out strings.Builder

	switch {
	case r.Orphan && r.Address != "" && r.ImportID != "":
		// Terraform planned to create it, so the state does not have it, and AWS
		// says it is there. That pair is drift, and this one is safe to assert.
		// The name goes on a line of its own: it is as long as the resource
		// somebody named, and wrapping around it is how a paragraph ends up at
		// 120 columns in a terminal that is 80.
		fmt.Fprintf(&out, "AWS already has %s, and Terraform planned to create it.\n\n", r.Kind)
		out.WriteString("So it is missing from the state. A run killed between creating a resource\n" +
			"and writing the state down does this: the API call completed, the record\n" +
			"never landed. Every later run repeats the failure until the state catches up.\n")
		out.WriteString("\nTo put it back in the state:\n")
		// Every one of the three quoted, not just the directory: a for_each key
		// can hold an apostrophe — module.x["o'brien"] is a legal address — and
		// one unquoted value is enough to break the command this promises is
		// ready to run.
		fmt.Fprintf(&out, "  terraform -chdir=%s import %s %s\n",
			shellQuote(dir), shellQuote(r.Address), shellQuote(r.ImportID))
		out.WriteString("\nIf that import is refused by a count the module cannot resolve, and the\n" +
			"intent was to destroy anyway, deleting the resource in AWS reaches the same\n" +
			"place — but only once you have confirmed it is yours to delete.\n")

	case r.Blocked:
		// Here the cause is genuinely open. Drift is one explanation and a common
		// one, but AWS refuses this whenever something is attached — including
		// when Terraform knows about it perfectly well and is removing things in
		// an order AWS will not take.
		fmt.Fprintf(&out,
			"AWS will not remove it while %s exist.\n\n"+
				"That is often drift — something created by a run that was killed before it\n"+
				"wrote the state — but it is not the only reason, so check before concluding:\n",
			r.Kind)
		fmt.Fprintf(&out, "  terraform -chdir=%s state list\n", shellQuote(dir))
		out.WriteString("  aws eks list-nodegroups --cluster-name <cluster> --region <region>\n")
		out.WriteString("\nIn the state as well: re-run, and let Terraform remove them in order.\n" +
			"Missing from the state: import them, or remove them in AWS once you have\n" +
			"confirmed they are yours to remove.\n")

	default:
		fmt.Fprintf(&out,
			"AWS is refusing to touch %s because something is still using it. Compare what\n"+
				"AWS has against the state before concluding which:\n", r.Kind)
		fmt.Fprintf(&out, "  terraform -chdir=%s state list\n", shellQuote(dir))
	}
	return strings.TrimRight(out.String(), "\n")
}
