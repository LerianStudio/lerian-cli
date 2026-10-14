package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Values renders what the chart reads.
//
// Every key is omitted when it has nothing to say, so the chart's own defaults
// apply. An explicit empty string would override them with nothing, which is
// how an image repository of "" once replaced a working default.
func (o Options) Values() map[string]any {
	configmap := map[string]any{
		"CONTROL_PLANE_URL":         o.ControlPlaneURL,
		"AGENT_ALLOW_INSECURE_HTTP": fmt.Sprintf("%t", o.AllowInsecureHTTP),
	}
	if o.AllowedRegistries != "" {
		configmap["AGENT_ALLOWED_IMAGE_REGISTRIES"] = o.AllowedRegistries
	}

	// Never nil: a nil slice renders as JSON null and an empty one as [], so a
	// release installed with no managed namespaces compared unequal to an
	// install that would set none — and every repeat run reported a change.
	namespaces := o.ManagedNamespaces
	if namespaces == nil {
		namespaces = []string{}
	}
	agent := map[string]any{
		"configmap":         configmap,
		"managedNamespaces": namespaces,
	}

	if len(o.ImagePullSecrets) > 0 {
		pull := make([]map[string]string, 0, len(o.ImagePullSecrets))
		for _, name := range o.ImagePullSecrets {
			pull = append(pull, map[string]string{"name": name})
		}
		agent["imagePullSecrets"] = pull
	}

	// One or the other, never both: the chart treats useExistingSecret as the
	// switch, and sending a token alongside it writes a Secret nothing reads.
	if o.UsesExistingSecret() {
		agent["useExistingSecret"] = true
		agent["existingSecretName"] = o.ExistingSecret
	} else {
		secrets := map[string]string{"AGENT_TOKEN": o.Token}
		if o.AgentID != "" {
			secrets["AGENT_ID"] = o.AgentID
		}
		agent["secrets"] = secrets
	}

	if o.ImageRepository != "" || o.ImageDigest != "" {
		image := map[string]string{}
		if o.ImageRepository != "" {
			image["repository"] = o.ImageRepository
		}
		if o.ImageDigest != "" {
			image["digest"] = o.ImageDigest
		}
		agent["image"] = image
	}

	return map[string]any{"agent": agent}
}

// WriteValues puts the values in a file for helm to read, and returns the path
// and the way to remove it.
//
// A file, never --set. --set puts every value on a command line: in the shell
// history, in `ps` output while helm runs, and in the log of any CI that echoes
// its commands. One of these values is a credential.
//
// 0600 and a directory only this user can enter, because the file still exists
// for as long as helm takes to read it.
func (o Options) WriteValues() (path string, cleanup func(), err error) {
	body, err := yaml.Marshal(o.Values())
	if err != nil {
		return "", nil, fmt.Errorf("agent: cannot render the chart values: %w", err)
	}

	dir, err := os.MkdirTemp("", "lerian-agent-")
	if err != nil {
		return "", nil, fmt.Errorf("agent: cannot create a directory for the values: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	path = filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("agent: cannot write the chart values: %w", err)
	}
	return path, cleanup, nil
}
