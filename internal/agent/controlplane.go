package agent

import (
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/config"
)

// ControlPlaneFromProfile is where `lerian auth login` already pointed.
//
// The agent connects to the Lerian platform, and this CLI knows where that is:
// auth login writes api-url into a profile, and `lerian midaz` reads it. Asking
// for it again treats a known value as a new one, and the operator who answers
// has to go and look up something they already told us.
//
// Empty when nothing is signed in, which is a real state — the agent can be
// installed against a control plane this machine has no profile for.
func ControlPlaneFromProfile(profileName string) string {
	cfg, err := config.Load()
	if err != nil {
		// A config that cannot be read is not an error here: it only means there
		// is no default to offer, and the question below still has an answer.
		return ""
	}
	profile, err := cfg.GetProfile(profileName)
	if err != nil || profile == nil {
		return ""
	}
	return strings.TrimRight(profile.APIURL, "/")
}
