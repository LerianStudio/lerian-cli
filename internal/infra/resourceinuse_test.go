package infra

import (
	"strings"
	"testing"
)

// The two messages below are the shapes AWS actually produced in the incident
// this exists for — a node group created in AWS whose state write never
// happened, which failed one apply and then one destroy, in that order. Cluster
// names are anonymized; the structure around them is not, because that
// structure is what the parsing depends on.
const (
	applyFailure = `infra: terraform apply failed for infra-base/eks: exit status 1

  Error: creating EKS Node Group (example-eks:example-eks-default): operation error EKS: CreateNodegroup, https response error StatusCode: 409, RequestID: cc10213a-7308-4eb7-9a0f-38c46a3547b6, ResourceInUseException: NodeGroup already exists with name example-eks-default and cluster name example-eks

    with module.eks.module.eks_managed_node_group["default"].aws_eks_node_group.this[0],
    on .terraform/modules/eks/modules/eks-managed-node-group/main.tf line 458, in resource "aws_eks_node_group" "this":
   458: resource "aws_eks_node_group" "this" {`

	destroyFailure = `infra: terraform apply failed for infra-base/eks: exit status 1

  Error: deleting EKS Cluster (example-eks): operation error EKS: DeleteCluster, https response error StatusCode: 409, RequestID: 51f52ddc-98a9-4e87-8626-b85f9579ed26, ResourceInUseException: Cluster has nodegroups attached`
)

// The apply case is the one with a command behind it: the address and the id are
// both in the message, so the import can be written out ready to run.
func TestTheApplyFailureYieldsAnImportCommand(t *testing.T) {
	drift := ReadResourceInUse(applyFailure)
	if drift == nil {
		t.Fatal("the failure was not recognized")
	}
	if !drift.Orphan {
		t.Error("a resource AWS has and the state does not is an orphan")
	}
	if drift.ImportID != "example-eks:example-eks-default" {
		t.Errorf("import id = %q", drift.ImportID)
	}
	want := `module.eks.module.eks_managed_node_group["default"].aws_eks_node_group.this[0]`
	if drift.Address != want {
		t.Errorf("address = %q, want %q", drift.Address, want)
	}

	advice := drift.Advice("/checkout/examples/aws/infra-base/eks")
	if !strings.Contains(advice, "terraform -chdir='/checkout/examples/aws/infra-base/eks' import '"+want+"' 'example-eks:example-eks-default'") {
		t.Errorf("the advice does not carry a runnable import:\n%s", advice)
	}
}

// The destroy case has no import to offer — the thing in the way is not in the
// state either — so it must not pretend otherwise.
func TestTheDestroyFailureDoesNotOfferAnImport(t *testing.T) {
	drift := ReadResourceInUse(destroyFailure)
	if drift == nil {
		t.Fatal("the failure was not recognized")
	}
	if drift.Orphan {
		t.Error("nothing here is importable: the attached resource may not be in the state")
	}
	if !drift.Blocked {
		t.Error("something attached is blocking the removal, and that is its own case")
	}

	advice := drift.Advice("/checkout/examples/aws/infra-base/eks")
	// No runnable import: there is no address to import, and offering one would
	// be offering a command that cannot be completed. Saying the word while
	// explaining the options is fine; printing `terraform ... import '<addr>'`
	// is not.
	if strings.Contains(advice, " import '") {
		t.Errorf("it offers an import command for a resource it cannot name:\n%s", advice)
	}
	if !strings.Contains(advice, "list-nodegroups") {
		t.Errorf("and it does not say how to find what is attached:\n%s", advice)
	}
}

// Both messages must say why, because that is the part Terraform leaves out and
// the part that stops somebody re-running the same command a third time.
func TestBothFailuresExplainTheCause(t *testing.T) {
	for name, text := range map[string]string{"apply": applyFailure, "destroy": destroyFailure} {
		drift := ReadResourceInUse(text)
		if drift == nil {
			t.Fatalf("%s: not recognized", name)
		}
		advice := drift.Advice("/dir")
		if !strings.Contains(advice, "killed") {
			t.Errorf("%s: does not say what produces this:\n%s", name, advice)
		}
	}
}

// Everything else has to pass through untouched, or every unrelated failure
// grows a paragraph about drift.
func TestUnrelatedFailuresAreNotRecognized(t *testing.T) {
	for _, text := range []string{
		"infra: terraform apply failed: exit status 1\n\n  Error: creating EKS Cluster: AccessDeniedException",
		"Error: Error acquiring the state lock\n\nLock Info:\n  ID: 7f3e",
		"",
	} {
		if drift := ReadResourceInUse(text); drift != nil {
			t.Errorf("recognized a failure that is not this one: %q -> %+v", text, drift)
		}
	}
}

// The exception says something is attached. It does not say the state is
// missing it: AWS refuses this whenever node groups exist, including when
// Terraform knows about them and is simply removing things in an order AWS will
// not take. Stating drift as fact sent somebody to import what was already
// there.
func TestTheDestroyFailureDoesNotAssertDrift(t *testing.T) {
	advice := ReadResourceInUse(destroyFailure).Advice("/dir")

	for _, claim := range []string{"This is drift", "the state does not"} {
		if strings.Contains(advice, claim) {
			t.Errorf("it asserts %q, which the exception does not establish:\n%s", claim, advice)
		}
	}
	if !strings.Contains(advice, "state list") {
		t.Errorf("it does not ask for the state to be checked first:\n%s", advice)
	}
	if !strings.Contains(advice, "often") {
		t.Errorf("it does not present drift as one explanation among others:\n%s", advice)
	}
}

// A ResourceInUseException this does not recognize is neither importable nor
// known to be blocked by node groups. Telling somebody to list node groups for
// a failure that has nothing to do with EKS is worse than saying less.
func TestAnUnrecognizedExceptionGetsNoEKSAdvice(t *testing.T) {
	drift := ReadResourceInUse("Error: deleting Something Else: ResourceInUseException: in use")
	if drift == nil {
		t.Fatal("not recognized at all")
	}
	if drift.Blocked || drift.Orphan {
		t.Errorf("an unrecognized exception was classified: %+v", drift)
	}

	advice := drift.Advice("/dir")
	if strings.Contains(advice, "list-nodegroups") {
		t.Errorf("it offers an EKS command for a failure that may not be EKS:\n%s", advice)
	}
	if !strings.Contains(advice, "state list") {
		t.Errorf("it does not say how to find out:\n%s", advice)
	}
}

// A checkout path with a space must survive into a command somebody can run.
func TestTheImportCommandSurvivesASpaceInThePath(t *testing.T) {
	advice := ReadResourceInUse(applyFailure).Advice("/Users/a b/infrastructure")

	if !strings.Contains(advice, "-chdir='/Users/a b/infrastructure'") {
		t.Errorf("the path is not quoted, so the command splits:\n%s", advice)
	}
}

// A for_each key can hold an apostrophe: module.x["o'brien"] is a legal
// address. One unquoted value is enough to break the command this promises is
// ready to run.
func TestTheImportCommandSurvivesAnApostropheInTheAddress(t *testing.T) {
	failure := `Error: creating EKS Node Group (example-eks:example-eks-default): ` +
		`ResourceInUseException: NodeGroup already exists with name example-eks-default and cluster name example-eks

    with module.eks.module.eks_managed_node_group["o'brien"].aws_eks_node_group.this[0],`

	advice := ReadResourceInUse(failure).Advice("/checkout")

	// Closed, emitted, reopened — the only way a single quote survives inside
	// single quotes.
	if !strings.Contains(advice, `'module.eks.module.eks_managed_node_group["o'\''brien"].aws_eks_node_group.this[0]'`) {
		t.Errorf("the apostrophe breaks the command:\n%s", advice)
	}
}
