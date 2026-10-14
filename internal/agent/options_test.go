package agent

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// valid is an Options that passes, so each test can break exactly one thing.
func valid() Options {
	return Options{
		Namespace:       DefaultNamespace,
		ControlPlaneURL: "https://control.example.com",
		Token:           EnrollmentPrefix + "abc",
	}
}

// The four rules the chart enforces, or regrets not enforcing, checked here so
// the message can name the flag to change rather than a line in a template.
func TestValidateRefusesWhatTheChartWould(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*Options)
		says   string
	}{
		"no credential at all": {
			func(o *Options) { o.Token = "" },
			"--token",
		},
		"both credential forms": {
			func(o *Options) { o.ExistingSecret = "creds" },
			"--existing-secret",
		},
		"agent id with an enrollment token": {
			func(o *Options) { o.AgentID = "agent-1" },
			"--agent-id must be empty",
		},
		// The other half of the same rule: a per-agent token carries no identity
		// of its own, so without the id the agent installs and cannot
		// authenticate.
		"per-agent token with no agent id": {
			func(o *Options) { o.Token = "lerian_agent_abc" },
			"--agent-id is required",
		},
		"cleartext control plane": {
			func(o *Options) { o.ControlPlaneURL = "http://control.example.com" },
			"--allow-insecure-http",
		},
		"image outside the allowlist": {
			func(o *Options) { o.ImageRepository = "docker.io/lerianstudio/agent" },
			"self-update",
		},
	} {
		t.Run(name, func(t *testing.T) {
			opts := valid()
			tc.mutate(&opts)

			err := opts.Validate()
			if err == nil {
				t.Fatalf("accepted: %+v", opts)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("the message does not name what to change (%q): %v", tc.says, err)
			}
		})
	}
}

// And accepts every shape that is actually legitimate.
func TestValidateAcceptsTheLegitimateShapes(t *testing.T) {
	for name, mutate := range map[string]func(*Options){
		"an enrollment token on its own":   func(o *Options) {},
		"a per-agent token with its id":    func(o *Options) { o.Token = "lerian_agent_x"; o.AgentID = "agent-1" },
		"a Secret already in the cluster":  func(o *Options) { o.Token = ""; o.ExistingSecret = "creds" },
		"cleartext with the flag":          func(o *Options) { o.ControlPlaneURL = "http://c"; o.AllowInsecureHTTP = true },
		"the chart's own default registry": func(o *Options) { o.ImageRepository = "ghcr.io/lerianstudio/agent" },
		"docker hub with the list widened": func(o *Options) {
			o.ImageRepository = "docker.io/lerianstudio/agent"
			o.AllowedRegistries = "ghcr.io/lerianstudio,docker.io/lerianstudio"
		},
	} {
		t.Run(name, func(t *testing.T) {
			opts := valid()
			mutate(&opts)
			if err := opts.Validate(); err != nil {
				t.Errorf("refused a legitimate configuration: %v", err)
			}
		})
	}
}

// The allowlist is compared by path prefix. These are the inputs that a
// segment-counting version accepted and should not have.
func TestTheRegistryAllowlistIsAPathPrefix(t *testing.T) {
	for repository, want := range map[string]bool{
		"ghcr.io/lerianstudio/agent":         true,
		"ghcr.io/lerianstudio/sub/agent":     true,
		"ghcr.io/lerianstudio-staging/agent": false, // prefix of the string, not the path
		"docker.io/agent":                    false, // two segments, no namespace
		"agent":                              false, // one segment, no host
		"/evil/agent":                        false,
	} {
		opts := valid()
		opts.ImageRepository = repository

		if got := opts.imageAllowed(); got != want {
			t.Errorf("%q allowed = %v, want %v", repository, got, want)
		}
	}
}

// A list written with a trailing comma must not contribute the prefix "/",
// which would allow anything starting with one.
func TestEmptyAllowlistEntriesAreDropped(t *testing.T) {
	opts := valid()
	opts.AllowedRegistries = "ghcr.io/lerianstudio,"
	opts.ImageRepository = "/evil/agent"

	if opts.imageAllowed() {
		t.Error("a trailing comma widened the allowlist to everything")
	}

	opts.ImageRepository = "ghcr.io/lerianstudio/agent"
	if !opts.imageAllowed() {
		t.Error("and the real entry stopped working")
	}
}

// Empty means "keep the chart's default", not "allow anything".
func TestAnEmptyAllowlistKeepsTheChartDefault(t *testing.T) {
	opts := valid()
	opts.AllowedRegistries = ""

	if got := opts.effectiveRegistries(); got != ChartDefaultRegistries {
		t.Errorf("effective registries = %q, want the chart default %q", got, ChartDefaultRegistries)
	}
}

// The two credential forms are mutually exclusive in the rendered values too:
// sending a token alongside useExistingSecret writes a Secret nothing reads.
func TestValuesCarryOneCredentialFormOnly(t *testing.T) {
	withToken := valid().Values()["agent"].(map[string]any)
	if _, ok := withToken["secrets"]; !ok {
		t.Error("a token must be rendered as secrets")
	}
	if _, ok := withToken["useExistingSecret"]; ok {
		t.Error("and must not also claim an existing Secret")
	}

	opts := valid()
	opts.Token = ""
	opts.ExistingSecret = "creds"
	withSecret := opts.Values()["agent"].(map[string]any)
	if _, ok := withSecret["secrets"]; ok {
		t.Error("an existing Secret must not be accompanied by inline secrets")
	}
	if withSecret["existingSecretName"] != "creds" {
		t.Errorf("the Secret name did not reach the values: %+v", withSecret)
	}
}

// An empty image repository must be absent rather than sent as "", which would
// override the chart's default with nothing.
func TestEmptyImageFieldsAreOmitted(t *testing.T) {
	rendered := valid().Values()["agent"].(map[string]any)

	if _, ok := rendered["image"]; ok {
		t.Errorf("an unset image was sent anyway: %+v", rendered["image"])
	}

	opts := valid()
	opts.ImageDigest = "sha256:abc"
	image := opts.Values()["agent"].(map[string]any)["image"].(map[string]string)
	if _, ok := image["repository"]; ok {
		t.Error("a digest on its own must not invent a repository")
	}
	if image["digest"] != "sha256:abc" {
		t.Errorf("digest = %q", image["digest"])
	}
}

// The credential reaches helm through a file, never a command line: --set puts
// it in the shell history, in `ps` while helm runs, and in any CI log that
// echoes commands.
func TestValuesAreWrittenPrivately(t *testing.T) {
	opts := valid()

	path, cleanup, err := opts.WriteValues()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the file holding a credential is mode %o, want 600", mode)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("what was written is not yaml: %v\n%s", err, body)
	}
	if !strings.Contains(string(body), opts.Token) {
		t.Error("the token did not reach the file helm will read")
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the file holding the credential outlived the install")
	}
}

// Waiting means waiting for /health/ready, and the agent is not ready until it
// has registered. Against a control plane that does not exist yet, that is a
// wait for something that cannot happen — so it has to be possible to skip it,
// and it has to be off by default everywhere else.
func TestWaitingIsTheDefaultAndCanBeTurnedOff(t *testing.T) {
	if valid().NoWait {
		t.Error("an install waits for readiness unless somebody says otherwise")
	}

	opts := valid()
	opts.NoWait = true
	if !opts.NoWait {
		t.Error("the option did not take")
	}
}

// "The control plane does not exist yet" is a real answer, and the chart still
// requires a URL and a credential — so it is answered with values, not with an
// exemption. They have to be values that cannot accidentally be real.
func TestThePlaceholdersAreUnroutableAndRecognizable(t *testing.T) {
	opts := Options{
		Namespace:       DefaultNamespace,
		ControlPlaneURL: PlaceholderControlPlane,
		Token:           PlaceholderToken,
	}

	// It must pass the same validation as any other install: the chart is going
	// to render these.
	if err := opts.Validate(); err != nil {
		t.Errorf("the placeholder install does not validate: %v", err)
	}
	if !opts.IsPlaceholder() {
		t.Error("a placeholder install is not recognized as one")
	}
	// .invalid is reserved by RFC 2606 and never resolves, so the agent retries
	// against something nobody could have meant.
	if !strings.HasSuffix(PlaceholderControlPlane, ".invalid") {
		t.Errorf("the placeholder could resolve to a real host: %q", PlaceholderControlPlane)
	}
	// https, or the cleartext rule would have to be waived for it.
	if !strings.HasPrefix(PlaceholderControlPlane, "https://") {
		t.Errorf("the placeholder would need --allow-insecure-http: %q", PlaceholderControlPlane)
	}
	// An enrollment token, so it does not also demand an agent id.
	if !strings.HasPrefix(PlaceholderToken, EnrollmentPrefix) {
		t.Errorf("the placeholder token would require --agent-id: %q", PlaceholderToken)
	}

	if (Options{ControlPlaneURL: "https://real.example.com", Token: "lerian_enroll_x"}).IsPlaceholder() {
		t.Error("a real install was taken for a placeholder")
	}
}

// A nil slice renders as JSON null and an empty one as [], so a release
// installed with no managed namespaces compared unequal to an install that
// would set none — and every repeat run reported a change it had not made.
func TestNoManagedNamespacesRendersAsAnEmptyList(t *testing.T) {
	opts := valid()
	opts.ManagedNamespaces = nil

	rendered := opts.Values()["agent"].(map[string]any)["managedNamespaces"]

	namespaces, ok := rendered.([]string)
	if !ok {
		t.Fatalf("managedNamespaces rendered as %T, want a slice", rendered)
	}
	if namespaces == nil {
		t.Error("a nil slice reached the values, where it renders as null")
	}
	if len(namespaces) != 0 {
		t.Errorf("it invented namespaces: %v", namespaces)
	}
}
