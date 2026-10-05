package infracli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// A cluster destroyed and recreated keeps its name and gets a new endpoint, so
// the entry already in the file points at a host that no longer resolves. That
// is the case this exists for — and it is also indistinguishable, from the file
// alone, from a second cluster of the same name in another account that somebody
// is working against right now. Hence the confirmation.
func TestReplacingAnEntryThatPointsElsewhereIsFlagged(t *testing.T) {
	path := writeKubeconfig(t, "arn:aws:eks:us-east-2:111122223333:cluster/example-dev-eks",
		"https://OLD.gr7.us-east-2.eks.amazonaws.com")
	t.Setenv("KUBECONFIG", path)

	plan, err := planKubeconfig(clusterFacts{
		Name:     "example-dev-eks",
		ARN:      "arn:aws:eks:us-east-2:111122223333:cluster/example-dev-eks",
		Endpoint: "https://NEW.gr7.us-east-2.eks.amazonaws.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !plan.Replaces {
		t.Error("overwriting an entry that points somewhere else was not flagged")
	}
	if !strings.Contains(plan.Current, "OLD") {
		t.Errorf("the endpoint being replaced is not reported: %q", plan.Current)
	}
}

// The same endpoint is not a replacement: running update-kubeconfig again is how
// somebody repairs a context, and asking to confirm a no-op trains them to say
// yes without reading.
func TestTheSameEndpointIsNotAReplacement(t *testing.T) {
	const endpoint = "https://SAME.gr7.us-east-2.eks.amazonaws.com"
	path := writeKubeconfig(t, "arn:aws:eks:us-east-2:111122223333:cluster/c", endpoint)
	t.Setenv("KUBECONFIG", path)

	plan, err := planKubeconfig(clusterFacts{
		Name: "c", ARN: "arn:aws:eks:us-east-2:111122223333:cluster/c", Endpoint: endpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replaces {
		t.Error("pointing at the endpoint already there was treated as a replacement")
	}
}

// Another cluster in the file is none of this one's business.
func TestAnUnrelatedEntryIsNotTouched(t *testing.T) {
	path := writeKubeconfig(t, "arn:aws:eks:eu-west-1:999988887777:cluster/other", "https://OTHER.example")
	t.Setenv("KUBECONFIG", path)

	plan, err := planKubeconfig(clusterFacts{
		Name: "mine", ARN: "arn:aws:eks:us-east-2:111122223333:cluster/mine", Endpoint: "https://MINE.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replaces {
		t.Error("an unrelated cluster counted as a replacement")
	}
}

// No file at all is the normal state before the first cluster, and the answer —
// nothing would be replaced — is the same as for a file without this entry.
func TestNoKubeconfigIsNotAnError(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nowhere", "config"))

	plan, err := planKubeconfig(clusterFacts{Name: "c", ARN: "arn:x", Endpoint: "https://e"})
	if err != nil {
		t.Fatalf("planKubeconfig = %v", err)
	}
	if plan.Replaces {
		t.Error("a file that does not exist reported a replacement")
	}
}

// A file this cannot read must not produce "nothing will be overwritten": that
// sentence would be a guess about a file whose contents are unknown.
func TestAnUnreadableKubeconfigIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("clusters: [this is not: valid: yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)

	if _, err := planKubeconfig(clusterFacts{Name: "c", ARN: "arn:x"}); err == nil {
		t.Error("an unparsable kubeconfig was reported as holding nothing")
	}
}

// KUBECONFIG may name several files; the AWS CLI writes the first, so that is
// the one to inspect.
func TestTheFirstKubeconfigInTheListIsTheOneInspected(t *testing.T) {
	first := writeKubeconfig(t, "arn:first", "https://FIRST.example")
	second := writeKubeconfig(t, "arn:second", "https://SECOND.example")
	t.Setenv("KUBECONFIG", first+string(os.PathListSeparator)+second)

	path, err := infra.KubeconfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != first {
		t.Errorf("KubeconfigPath = %q, want the first entry %q", path, first)
	}
}

// The row is offered after an apply that produced a cluster, and not after a
// plan: there is nothing new to point at, and offering it would suggest the plan
// changed something.
func TestKubectlIsOfferedOnlyAfterAnApplyWithACluster(t *testing.T) {
	cases := []struct {
		name    string
		done    infra.Action
		cluster bool
		want    bool
	}{
		{"apply with a cluster", infra.ActionApply, true, true},
		{"apply without one", infra.ActionApply, false, false},
		{"plan with a cluster", infra.ActionPlan, true, false},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			offered := false
			for _, opt := range afterOptions(test.done, test.cluster) {
				if opt.value == afterKubectl {
					offered = true
				}
			}
			if offered != test.want {
				t.Errorf("offered = %v, want %v", offered, test.want)
			}
		})
	}
}

// writeKubeconfig makes a kubeconfig holding one cluster.
func writeKubeconfig(t *testing.T, name, server string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	body := "apiVersion: v1\nkind: Config\nclusters:\n- name: " + name +
		"\n  cluster:\n    server: " + server + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
