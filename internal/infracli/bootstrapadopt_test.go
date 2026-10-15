package infracli

import (
	"context"
	"errors"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

type listing struct {
	found []infra.StateBackend
	err   error
}

func (l listing) ListStateBackends(context.Context, string, string, string) ([]infra.StateBackend, error) {
	return l.found, l.err
}

// Two facts, not one. A plan that would create the bucket is ordinary on a
// fresh account; a bucket that answers is ordinary once bootstrap has run. Only
// together do they mean the state has lost track of what exists.
func TestAnOrphanedBackendNeedsBothFacts(t *testing.T) {
	const account = "111122223333"
	present := listing{found: []infra.StateBackend{{
		Bucket: "lerian-tfstate-dev-" + account, Env: "dev", Region: "us-east-2",
		LockTable: "lerian-tfstate-lock-dev",
	}}}

	// Both: the backend is there and bootstrap wants to build it.
	if _, orphaned := orphanedBackend(context.Background(), present,
		"work", "us-east-2", account, "dev", true); !orphaned {
		t.Error("a backend that exists while bootstrap plans to create it is orphaned state")
	}

	// A plan with nothing to create is bootstrap already knowing about it.
	if _, orphaned := orphanedBackend(context.Background(), present,
		"work", "us-east-2", account, "dev", false); orphaned {
		t.Error("a plan that creates nothing was called orphaned")
	}

	// And nothing in AWS is a first run, which is what bootstrap is for.
	if _, orphaned := orphanedBackend(context.Background(), listing{},
		"work", "us-east-2", account, "dev", true); orphaned {
		t.Error("an empty account was called orphaned")
	}
}

// Another environment's bucket is not this one's. Adopting it would point two
// environments at one state file.
func TestABackendForAnotherEnvironmentIsNotAdopted(t *testing.T) {
	const account = "111122223333"
	other := listing{found: []infra.StateBackend{{
		Bucket: "lerian-tfstate-prd-" + account, Env: "prd", Region: "us-east-2",
	}}}

	if _, orphaned := orphanedBackend(context.Background(), other,
		"work", "us-east-2", account, "dev", true); orphaned {
		t.Error("prd's backend was offered as dev's")
	}
}

// AWS being unreadable is not a reason to claim anything about the state: the
// plan stands on its own and the operator can act on it.
func TestAnUnreadableAccountClaimsNothing(t *testing.T) {
	broken := listing{err: errors.New("AccessDenied")}

	if _, orphaned := orphanedBackend(context.Background(), broken,
		"work", "us-east-2", "111122223333", "dev", true); orphaned {
		t.Error("an account that could not be read was reported as orphaned")
	}
}

// The offer is about the bootstrap stack and nothing else, so a run that did
// not plan it has nothing to say.
func TestOnlyABootstrapPlanIsConsidered(t *testing.T) {
	stages := []infra.Stage{{Name: "infra-base/vpc", Units: []infra.Unit{
		{Name: "infra-base/vpc", Dir: "/x/vpc"},
	}}}

	if _, _, ok := bootstrapCreating(stages, nil); ok {
		t.Error("a run with no bootstrap stack was considered")
	}
}

// And when it did, what matters is whether that plan would create anything: a
// bootstrap with nothing to create is one that already knows its backend.
func TestABootstrapPlanReportsWhetherItWouldCreate(t *testing.T) {
	unit := infra.Unit{Name: "bootstrap", Dir: "/x/bootstrap", Bootstrap: true}
	stages := []infra.Stage{{Name: "bootstrap", Units: []infra.Unit{unit}}}

	creating := []infra.StageResult{{Plans: []infra.UnitResult{
		{Unit: unit, Changes: infra.Changes{Create: 9}},
	}}}
	got, would, ok := bootstrapCreating(stages, creating)
	if !ok || !would {
		t.Errorf("a bootstrap planning 9 creations: ok=%v would=%v", ok, would)
	}
	if got.Name != "bootstrap" {
		t.Errorf("the wrong unit came back: %+v", got)
	}

	settled := []infra.StageResult{{Plans: []infra.UnitResult{
		{Unit: unit, Changes: infra.Changes{}},
	}}}
	if _, would, _ := bootstrapCreating(stages, settled); would {
		t.Error("a bootstrap with nothing to create was reported as creating")
	}
}
