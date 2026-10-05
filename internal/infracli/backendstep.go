package infracli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// Sentinel rows, kept apart from any bucket name.
const (
	backendCreate = "\x00create"
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

	picked, err := ask.pick("A state backend already exists in this account. Use it?",
		"Adopting writes "+layout.RepoRel(layout.BackendFile(environment))+" from what is in the account.",
		"", backendOptions(found, environment, account), "")
	if err != nil {
		return err
	}
	if picked == backendCreate {
		fmt.Fprintf(out, "  leaving it. bootstrap makes a new one.\n\n")
		return nil
	}

	for _, backend := range found {
		if backend.Bucket != picked {
			continue
		}
		path, adoptErr := infra.AdoptBackend(layout, environment, backend)
		if adoptErr != nil {
			return adoptErr
		}
		fmt.Fprintf(out, "\n  wrote %s\n", layout.RepoRel(path))
		if backend.LockTable == "" {
			fmt.Fprintf(out, "  %s\n", theme.dim(
				"no lock table beside it: concurrent runs are not protected"))
		}
		fmt.Fprintln(out)
		return nil
	}
	return nil
}

// backendChoices lists what the account holds, with the one bootstrap would have
// made for this environment first.
//
// Every state bucket the account holds is offered, not only the matching one:
// the ones made for other environments, and the ones whose suffix is not an
// environment this tool knows — those are somebody's deliberate naming, and
// hiding them leaves that person where the old advice did, writing the file by
// hand.
//
// Bounded, though. ListStateBackends only returns buckets carrying the prefix
// the templates give them and this account's id, so "named by hand" means a
// hand-chosen SUFFIX, not any bucket in the account. Offering every bucket an
// account holds would bury three answers in fifty.
func backendOptions(found []infra.StateBackend, environment, account string) []option {
	choices := make([]option, 0, len(found)+1)

	add := func(backend infra.StateBackend, note string) {
		choices = append(choices, option{value: backend.Bucket, label: backend.Bucket, note: note})
	}

	for _, backend := range found {
		if backend.Matches(environment, account) {
			add(backend, "this environment's own backend · "+describeBackend(backend))
		}
	}
	for _, backend := range found {
		if backend.Matches(environment, account) {
			continue
		}
		where := "made for " + backend.Env
		if backend.Env == "" {
			where = "named by hand"
		}
		add(backend, where+" · "+describeBackend(backend))
	}

	choices = append(choices, option{
		value: backendCreate,
		label: "create a new one",
		note:  "bootstrap makes " + infra.StateBucketPrefix + environment + "-" + account,
	})
	return choices
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
	return strings.Join(parts, ", ")
}
