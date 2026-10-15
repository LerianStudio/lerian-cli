package infra

import "fmt"

// AdoptableResource is one piece of an existing backend and the address the
// bootstrap root knows it by.
type AdoptableResource struct {
	// Address is the Terraform address inside the bootstrap root.
	Address string
	// ID is what `terraform import` takes for that resource type.
	ID string
	// What it is, for the line printed while it is imported.
	What string
}

// BootstrapAdoption is every resource an existing backend consists of.
//
// The bootstrap root is the one stack that cannot keep its state in the bucket
// it creates, so it runs on local state in the checkout — and a checkout is a
// thing this CLI clones, replaces and throws away. Lose it and the bucket and
// the lock table are still there with nothing that knows them: `plan` offers to
// create what exists, `apply` fails on a name already taken, and `destroy`
// removes nothing.
//
// Importing them back is what makes the backend manageable again, and it is
// mechanical: the addresses are fixed by the root, and the ids are the bucket
// and table names.
func BootstrapAdoption(bucket, lockTable string) []AdoptableResource {
	return []AdoptableResource{
		{"aws_s3_bucket.tfstate", bucket, "the state bucket"},
		{"aws_s3_bucket_versioning.tfstate", bucket, "its versioning"},
		{"aws_s3_bucket_server_side_encryption_configuration.tfstate", bucket, "its encryption"},
		{"aws_s3_bucket_public_access_block.tfstate", bucket, "its public access block"},
		{"aws_s3_bucket_ownership_controls.tfstate", bucket, "its ownership controls"},
		{"aws_s3_bucket_lifecycle_configuration.tfstate", bucket, "its lifecycle rules"},
		{"aws_s3_bucket_policy.tfstate", bucket, "its policy"},
		{"aws_dynamodb_table.tfstate_lock", lockTable, "the lock table"},
	}
}

// Describe renders one resource for the line printed while it is imported.
func (r AdoptableResource) Describe() string {
	return fmt.Sprintf("%s (%s)", r.What, r.ID)
}
