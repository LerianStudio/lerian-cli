package infra

import (
	"strings"
	"testing"
)

// The bootstrap root keeps its state locally, in a checkout this CLI clones and
// replaces. Lose that and the bucket and lock table are still there with
// nothing that knows them — so every resource the root declares has to be
// importable, or adopting leaves a plan that still wants to create something.
func TestEveryBootstrapResourceCanBeAdopted(t *testing.T) {
	adoption := BootstrapAdoption("lerian-tfstate-dev-111122223333", "lerian-tfstate-lock-dev")

	// The bucket and everything configured on it, plus the lock table. Eight,
	// because local_file.backend_config is written by the root rather than held
	// in AWS, and a file is recreated rather than imported.
	if len(adoption) != 8 {
		t.Fatalf("got %d resources to adopt: %+v", len(adoption), adoption)
	}

	byAddress := map[string]string{}
	for _, resource := range adoption {
		if resource.Address == "" || resource.ID == "" {
			t.Errorf("a resource is missing an address or an id: %+v", resource)
		}
		if _, repeated := byAddress[resource.Address]; repeated {
			t.Errorf("%q is listed twice, and the second import would fail", resource.Address)
		}
		byAddress[resource.Address] = resource.ID
	}

	// The two that carry different ids: everything else is the bucket.
	if byAddress["aws_dynamodb_table.tfstate_lock"] != "lerian-tfstate-lock-dev" {
		t.Errorf("the lock table is imported by the wrong id: %q",
			byAddress["aws_dynamodb_table.tfstate_lock"])
	}
	if byAddress["aws_s3_bucket.tfstate"] != "lerian-tfstate-dev-111122223333" {
		t.Errorf("the bucket is imported by the wrong id: %q", byAddress["aws_s3_bucket.tfstate"])
	}
}

// Each line is read while an import runs, so it has to say which thing is being
// adopted rather than only its address.
func TestAnAdoptableResourceDescribesItself(t *testing.T) {
	for _, resource := range BootstrapAdoption("a-bucket", "a-table") {
		described := resource.Describe()
		if !strings.Contains(described, resource.ID) {
			t.Errorf("%q does not name what is being imported: %q", resource.Address, described)
		}
		if described == resource.ID {
			t.Errorf("%q says only the id, which reads as a bucket name with no verb", resource.Address)
		}
	}
}
