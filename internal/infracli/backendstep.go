package infracli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// resolveBackend settles what the state backend is before anything is chosen to
// run against it.
//
// The local file alone cannot answer this. Its absence means "nobody has
// bootstrapped this checkout", which is not the same as "nobody has bootstrapped
// this account" — a fresh clone, or a colleague who ran bootstrap from their own
// machine, produces the first without the second. Treating them as the same thing
// sent somebody to create a second state bucket beside the one already holding
// their infrastructure's state.
//
// So when the file is missing, the account is asked. What comes back is fact: the
// buckets that exist, their regions, and whether each has a lock table.
func resolveBackend(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	lister infra.BackendLister,
	layout infra.Layout,
	environment, profile, region, account string,
) error {
	if backendExists(layout, environment) {
		return nil
	}

	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> State backend"))
	fmt.Fprintf(out, "  %s\n", theme.dim(
		"no "+layout.RepoRel(layout.BackendFile(environment))+" here — asking the account"))

	found, err := lister.ListStateBackends(ctx, profile, region, account)
	if err != nil {
		// Not fatal. The answer to "does a backend exist" is then unknown rather
		// than no, and bootstrap — which creates one — is still the right next step
		// either way. Failing here would block a run over a question that is only
		// there to save somebody from a duplicate bucket.
		fmt.Fprintf(out, "  could not list the buckets in this account: %v\n", err)
		fmt.Fprintf(out, "  %s\n\n", theme.dim(
			"bootstrap creates one; it is safe to run on an account that already has it"))
		return nil
	}

	if len(found) == 0 {
		fmt.Fprintf(out, "  none in account %s. bootstrap creates it.\n\n", account)
		return nil
	}

	// Only this environment's own bucket is a candidate. The environment was
	// chosen two questions ago, and lerian-tfstate-stg-<account> is stg's state —
	// adopting it as prd's would point two environments at one state file, which
	// is the one mistake this screen must not make possible.
	var own *infra.StateBackend
	for index := range found {
		if found[index].Matches(environment, account) {
			own = &found[index]
			break
		}
	}

	if own == nil {
		fmt.Fprintf(out, "  %d state bucket(s) here, none of them %s's. bootstrap creates it.\n\n",
			len(found), environment)
		return nil
	}

	fmt.Fprintf(out, "  found     %s%s\n", own.Bucket, describeBackend(*own))
	fmt.Fprintf(out, "  %s\n", theme.dim(
		"this is "+environment+"'s own backend; adopting writes "+
			layout.RepoRel(layout.BackendFile(environment))))

	// No "create a new one" beside it: the bucket exists, so bootstrap would stop
	// at "bucket already exists". Adopting is the only thing that can work, and a
	// second row would be offering a failure.
	if err := ask.confirm(ctx, out, "Use it as "+environment+"'s state backend?"); err != nil {
		return err
	}

	path, adoptErr := infra.AdoptBackend(layout, environment, *own)
	if adoptErr != nil {
		return adoptErr
	}
	fmt.Fprintf(out, "\n  wrote %s\n", layout.RepoRel(path))
	if own.LockTable == "" {
		fmt.Fprintf(out, "  %s\n", theme.dim("no lock table beside it: concurrent runs are not protected"))
	}
	fmt.Fprintln(out)
	return nil
}

// describeBackend is the part that decides whether adopting is safe: where it is,
// and whether concurrent runs are protected.
func describeBackend(backend infra.StateBackend) string {
	parts := []string{backend.Region}
	if backend.Region == "" {
		parts = []string{"region unknown"}
	}
	if backend.LockTable == "" {
		parts = append(parts, "no lock table")
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}
