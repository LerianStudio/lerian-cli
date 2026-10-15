package infracli

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// orphanedBackend reports a backend that exists in AWS while the bootstrap root
// has no state for it.
//
// The bootstrap root is the one stack that cannot keep its state in the bucket
// it creates, so it keeps it locally, in a checkout this CLI clones and
// replaces. Lose that and the bucket and the lock table are still there with
// nothing that knows them — and every operation lies: plan offers to create
// what exists, apply fails on a name already taken, destroy removes nothing.
//
// Detected by the pair of facts rather than by either alone: a plan that would
// create the bucket, and a bucket that answers.
func orphanedBackend(
	ctx context.Context,
	backends infra.BackendLister,
	profile, region, account, environment string,
	wouldCreate bool,
) (infra.StateBackend, bool) {
	if !wouldCreate {
		return infra.StateBackend{}, false
	}
	found, err := backends.ListStateBackends(ctx, profile, region, account)
	if err != nil {
		return infra.StateBackend{}, false
	}
	for _, backend := range found {
		if backend.Matches(environment, account) {
			return backend, true
		}
	}
	return infra.StateBackend{}, false
}

// offerAdoption explains the orphaned backend and imports it when told to.
//
// Asked, not done: importing writes to the state of the stack every other
// stack's state depends on. It is also the only answer that works — deleting
// the bucket to let the bootstrap recreate it would delete every other stack's
// state with it, which is why that is not offered.
func offerAdoption(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	terraform *infra.CLI,
	unit infra.Unit,
	backend infra.StateBackend,
) error {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> The backend is already there"))
	fmt.Fprintf(out, "  bucket      %s\n", backend.Bucket)
	fmt.Fprintf(out, "  lock table  %s\n", backend.LockTable)
	fmt.Fprintf(out, "  region      %s\n\n", backend.Region)
	fmt.Fprintf(out, "  bootstrap plans to create them, which means its own state does not know\n")
	fmt.Fprintf(out, "  they exist. That state is local to this checkout, and a checkout is a\n")
	fmt.Fprintf(out, "  thing this CLI clones and replaces.\n\n")
	fmt.Fprintf(out, "  Until it does know, apply fails on a name already taken and destroy\n")
	fmt.Fprintf(out, "  removes nothing. Importing them is the way back — deleting the bucket\n")
	fmt.Fprintf(out, "  would take every other stack's state with it.\n")

	answer, err := ask.pick("Import the existing backend into bootstrap's state?",
		"Reads AWS and writes bootstrap's local state. Creates and deletes nothing.", "",
		[]option{
			{value: "yes", label: "import it", note: "bootstrap can manage it afterwards"},
			{value: "no", label: "leave it", note: "the plan above still says 'to add'"},
		}, "")
	if err != nil || answer != "yes" {
		//nolint:nilerr // Declining is not a failure: the plan is on screen and
		// the operator can act on it by hand. Returning the error here would
		// turn an offer into a failed command.
		return nil
	}

	return adoptBackend(ctx, out, terraform, unit, backend)
}

// adoptBackend imports each resource, saying which as it goes.
func adoptBackend(
	ctx context.Context,
	out io.Writer,
	terraform *infra.CLI,
	unit infra.Unit,
	backend infra.StateBackend,
) error {
	resources := infra.BootstrapAdoption(backend.Bucket, backend.LockTable)

	var mu sync.Mutex
	for _, resource := range resources {
		spin := newSpinner(&mu, out, "importing "+resource.Describe())
		err := terraform.Import(ctx, unit, resource.Address, resource.ID)
		spin.Stop()
		if err != nil {
			// Said and stopped. A partial import is a real state that the next
			// plan describes accurately, so there is nothing to undo — but the
			// rest should not be attempted over a failure nobody has read.
			fmt.Fprintf(out, "\n  %v\n\n", err)
			return err
		}
		fmt.Fprintf(out, "  ok  %s\n", resource.Describe())
	}

	fmt.Fprintf(out, "\n  %s\n", newStyle(out).bold("Adopted."))
	fmt.Fprintf(out, "  bootstrap now knows the backend, so plan, apply and destroy describe it\n")
	fmt.Fprintf(out, "  rather than offering to build it again.\n\n")
	return nil
}
