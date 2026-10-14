package agent

import (
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/config"
)

// ControlPlane is the Lerian control plane every agent connects to.
//
// It belongs to Lerian, not to the cluster being installed into, so it is a
// constant here rather than a question put to whoever runs this: asking for it
// is asking somebody to supply a value they have no way of choosing, and that
// they would get wrong far more easily than this file can.
//
// Empty because the address has not been decided yet. While it is empty, an
// install with nothing else to go on uses the placeholder instead and says so.
// When the address exists, this is the only line that changes.
const ControlPlane = ""

// DefaultControlPlane is where an install points when nobody says otherwise.
//
// In order: the address Lerian publishes, then whatever `lerian auth login`
// recorded for this machine — which is the one case where a per-machine answer
// is right, because somebody signed in to a specific platform on purpose.
func DefaultControlPlane(profileName string) string {
	if ControlPlane != "" {
		return ControlPlane
	}
	return ControlPlaneFromProfile(profileName)
}

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
