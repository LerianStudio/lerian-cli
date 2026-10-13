package agent

import (
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/agent"
)

// Picking "install" off a menu passes no flags, and answering that with usage
// text leaves somebody exactly where they started.
func TestNothingGivenMeansAsk(t *testing.T) {
	if !needsAsking(agent.Options{}) {
		t.Error("a command with no flags at all must ask")
	}
	if needsAsking(agent.Options{ControlPlaneURL: "https://cp.example.com"}) {
		t.Error("a caller who named the control plane is driving; do not interrupt them")
	}
}

// The questions cover three values. Every other flag has no question, so taking
// the answers wholesale would drop what the caller already said.
func TestFlagsSurviveTheQuestions(t *testing.T) {
	flags := agent.Options{
		Namespace:         "infra",
		ChartVersion:      "1.0.2",
		ImageRepository:   "ghcr.io/lerianstudio/agent",
		ImageDigest:       "sha256:abc",
		AllowedRegistries: "ghcr.io/lerianstudio",
		ImagePullSecrets:  []string{"pull"},
		AllowInsecureHTTP: true,
	}
	asked := agent.Options{
		Namespace:         agent.DefaultNamespace, // the questions never ask this
		ControlPlaneURL:   "https://cp.example.com",
		Token:             agent.EnrollmentPrefix + "abc",
		ManagedNamespaces: []string{"apps"},
	}

	merged := keepFlags(flags, asked)

	if merged.Namespace != "infra" {
		t.Errorf("--namespace was lost: %q", merged.Namespace)
	}
	if merged.ChartVersion != "1.0.2" {
		t.Errorf("--chart-version was lost: %q", merged.ChartVersion)
	}
	if merged.ImageDigest != "sha256:abc" || merged.ImageRepository == "" {
		t.Errorf("the image flags were lost: %+v", merged)
	}
	if !merged.AllowInsecureHTTP {
		t.Error("--allow-insecure-http was lost")
	}
	if len(merged.ImagePullSecrets) != 1 {
		t.Errorf("--image-pull-secrets was lost: %v", merged.ImagePullSecrets)
	}

	// And the answers are kept, or the questions were for nothing.
	if merged.ControlPlaneURL != "https://cp.example.com" {
		t.Errorf("the answer was discarded: %q", merged.ControlPlaneURL)
	}
	if merged.Token == "" || len(merged.ManagedNamespaces) != 1 {
		t.Errorf("the answers were discarded: %+v", merged)
	}
}

// The questions ask about waiting, and --wait defaults to true. Carrying the
// flag over would overrule an answer with a value nobody typed.
func TestTheWaitAnswerSurvivesTheFlagDefault(t *testing.T) {
	flags := agent.Options{Namespace: agent.DefaultNamespace}
	asked := agent.Options{
		ControlPlaneURL: "https://cp.example.com",
		Token:           agent.EnrollmentPrefix + "abc",
		NoWait:          true, // answered "do not wait"
	}

	merged := keepFlags(flags, asked)

	if !merged.NoWait {
		t.Error("the answer about waiting was overruled by the flag's default")
	}
}
