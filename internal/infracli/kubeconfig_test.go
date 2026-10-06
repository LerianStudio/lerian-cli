package infracli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// The row is offered whenever there is a cluster to point at, and never when
// there is not.
func TestKubectlIsOfferedWheneverThereIsACluster(t *testing.T) {
	cases := []struct {
		name    string
		done    infra.Action
		cluster bool
		want    bool
	}{
		{"apply with a cluster", infra.ActionApply, true, true},
		{"apply without one", infra.ActionApply, false, false},
		// A plan changes nothing, but the cluster is there either way, and
		// somebody who has just planned against it may want to look inside it.
		{"plan with a cluster", infra.ActionPlan, true, true},
		{"plan without one", infra.ActionPlan, false, false},
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

// fakeOutputs stands in for terraform, so the two reasons there is no kubectl
// row can be told apart without a cluster.
type fakeOutputs struct {
	values map[string]json.RawMessage
	err    error
	asked  []string
}

func (f *fakeOutputs) Output(_ context.Context, unit infra.Unit) (map[string]json.RawMessage, error) {
	f.asked = append(f.asked, unit.Name)
	return f.values, f.err
}

// A run with no eks root has no cluster, and saying so would be noise.
func TestNoEksRootIsSilent(t *testing.T) {
	outputs := &fakeOutputs{}

	_, found, why := readClusterFacts(context.Background(), outputs,
		[]infra.Unit{{Name: "infra-base/vpc"}, {Name: "products/midaz/postgres"}})

	if found {
		t.Error("a cluster was reported for a run with no eks root")
	}
	if why != "" {
		t.Errorf("it explained an absence that needs no explanation: %q", why)
	}
	if len(outputs.asked) != 0 {
		t.Errorf("it read the outputs of %v", outputs.asked)
	}
}

// An eks root whose outputs cannot be read is a different thing. A long apply
// can outlive the credential that started it, and a row that quietly is not
// there reads as a tool that forgot rather than one that could not.
func TestAnUnreadableEksStackSaysWhy(t *testing.T) {
	outputs := &fakeOutputs{err: errors.New("ExpiredToken: the security token included in the request is expired")}

	_, found, why := readClusterFacts(context.Background(), outputs,
		[]infra.Unit{{Name: "infra-base/vpc"}, {Name: "infra-base/eks"}})

	if found {
		t.Fatal("a cluster was reported from outputs that could not be read")
	}
	if !strings.Contains(why, "ExpiredToken") {
		t.Errorf("why = %q, want what terraform said", why)
	}
	// Only the eks root is consulted: reading every root's outputs to find one
	// name would be minutes of terraform for a menu row.
	if len(outputs.asked) != 1 || outputs.asked[0] != "infra-base/eks" {
		t.Errorf("it read %v", outputs.asked)
	}
}

// And a stack that answers gives the three facts the kubeconfig needs.
func TestTheClusterFactsComeFromTheOutputs(t *testing.T) {
	outputs := &fakeOutputs{values: map[string]json.RawMessage{
		"cluster_name":     json.RawMessage(`"example-prd-eks"`),
		"cluster_arn":      json.RawMessage(`"arn:aws:eks:us-east-2:111122223333:cluster/example-prd-eks"`),
		"cluster_endpoint": json.RawMessage(`"https://ABC.gr7.us-east-2.eks.amazonaws.com"`),
	}}

	facts, found, why := readClusterFacts(context.Background(), outputs,
		[]infra.Unit{{Name: "infra-base/eks"}})

	if !found || why != "" {
		t.Fatalf("found=%v why=%q", found, why)
	}
	if facts.Name != "example-prd-eks" || facts.ARN == "" || facts.Endpoint == "" {
		t.Errorf("facts = %+v", facts)
	}
}

// A stack that answers without a name is not a cluster to point at, and the
// reason is worth saying for the same reason a failed read is.
func TestOutputsWithoutAClusterNameSayWhy(t *testing.T) {
	outputs := &fakeOutputs{values: map[string]json.RawMessage{"something_else": json.RawMessage(`"x"`)}}

	_, found, why := readClusterFacts(context.Background(), outputs,
		[]infra.Unit{{Name: "infra-base/eks"}})

	if found {
		t.Fatal("a cluster with no name was reported")
	}
	if why == "" {
		t.Error("nothing explains why there is no row")
	}
}

// The report has to tell the three outcomes apart, because they mean three
// different things to the person reading them.
func TestTheProbeIsReportedByOutcome(t *testing.T) {
	tests := []struct {
		name  string
		probe infra.ClusterProbe
		want  string
	}{
		{"answered", infra.ClusterProbe{Version: "v1.36.4"}, "the cluster answered: v1.36.4"},
		{"refused", infra.ClusterProbe{Problem: "Unauthorized", Hint: "needs an access entry"},
			"did not answer: Unauthorized"},
		{"no kubectl", infra.ClusterProbe{Missing: true}, "kubectl is not installed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			reportClusterProbe(&out, test.probe)

			if !strings.Contains(out.String(), test.want) {
				t.Errorf("report = %q, want %q", out.String(), test.want)
			}
		})
	}
}

// The hint is the half that says what to do, so it has to survive into the
// report.
func TestTheHintIsPrinted(t *testing.T) {
	var out bytes.Buffer
	reportClusterProbe(&out, infra.ClusterProbe{
		Problem: "no such host",
		Hint:    "a cluster destroyed and recreated keeps its name",
	})

	if !strings.Contains(out.String(), "keeps its name") {
		t.Errorf("the hint is missing:\n%s", out.String())
	}
}

// A missing kubectl is not a problem with the cluster, and reporting it as one
// would send somebody to debug infrastructure that is fine.
func TestAMissingKubectlIsNotAClusterProblem(t *testing.T) {
	var out bytes.Buffer
	reportClusterProbe(&out, infra.ClusterProbe{Missing: true})

	if strings.Contains(out.String(), "did not answer") {
		t.Errorf("a missing kubectl was reported as a cluster failure:\n%s", out.String())
	}
}
