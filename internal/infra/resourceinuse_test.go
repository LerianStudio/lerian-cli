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
	if !strings.Contains(advice, "terraform -chdir=/checkout/examples/aws/infra-base/eks import '"+want+"' example-eks:example-eks-default") {
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
		t.Error("nothing here is importable: the attached resource is not in the state")
	}

	advice := drift.Advice("/checkout/examples/aws/infra-base/eks")
	if strings.Contains(advice, "terraform -chdir") && strings.Contains(advice, "import") {
		t.Errorf("it offers an import for a resource it cannot name:\n%s", advice)
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
		if !strings.Contains(advice, "This is drift, not a conflict with somebody else.") {
			t.Errorf("%s: does not rule out the obvious wrong guess:\n%s", name, advice)
		}
		if !strings.Contains(advice, "killed between") {
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
