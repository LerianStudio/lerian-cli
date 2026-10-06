package infracli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// DescribeMachine prints what this machine has, grouped by who owns it.
//
// Four things live in four places, and finding out meant four commands and
// knowing which. Grouped rather than listed flat because the same word means
// different things in different groups: a profile under "This tool" is a Lerian
// platform login, a profile under "AWS" is a credential in ~/.aws, and side by
// side with no headings they read as one kind of thing.
//
// It reads files and makes no AWS call. Somebody opening this wants to know where
// things are, not to wait for a round trip per profile — and it has to work on a
// machine with no network. What that costs is knowing whether the credentials
// work, so the page says which command answers that.
func DescribeMachine(ctx context.Context, out io.Writer) {
	theme := newStyle(out)
	section := func(name string) { fmt.Fprintf(out, "\n%s\n", theme.bold("==> "+name)) }
	line := func(label, value string) { fmt.Fprintf(out, "  %-10s %s\n", label, value) }

	section("This tool")
	describeTool(line)

	section("Templates")
	describeTemplates(ctx, line)
	describeInitializedFor(line)

	section("AWS")
	describeAWS(line)
	fmt.Fprintf(out, "  %s\n", theme.dim("whether they work is an AWS call: lerian infra check makes it"))

	section("Kubernetes")
	describeKubeconfig(line)

	section("Tools")
	for _, name := range []string{"terraform", "aws", "git", "kubectl"} {
		line(name, binaryPath(name))
	}
	fmt.Fprintln(out)
}

// describeTool is the configuration this CLI writes, and nothing else writes.
func describeTool(line func(label, value string)) {
	path, err := config.GetConfigPath()
	if err != nil {
		line("config", "cannot resolve: "+err.Error())
		return
	}
	line("config", path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		line("state", "nothing configured yet")
		return
	}
	cfg, err := config.Load()
	if err != nil {
		line("state", "cannot be read: "+err.Error())
		return
	}

	profile := cfg.CurrentProfile
	if profile == "" {
		profile = "none"
	}
	line("profile", profile+"   (Lerian platform, not AWS)")

	// Named, never printed: a profile holds an API key.
	if len(cfg.Profiles) == 0 {
		line("logins", "none — lerian auth login creates one")
		return
	}
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	line("logins", nameAFew(sorted(names)))
}

// describeTemplates is the checkout a run would use, and which of the five ways
// of pointing at one produced it.
func describeTemplates(ctx context.Context, line func(label, value string)) {
	layout, source, err := resolveLayout("", os.Getenv("LERIAN_TF_REPO"), "")
	switch {
	case errors.Is(err, errNoCheckoutAnywhere):
		line("checkout", "none found — lerian infra init --clone")
		return
	case err != nil:
		// Any other failure is a checkout that was POINTED AT and did not work —
		// usually LERIAN_TF_REPO naming a directory that is not one. Telling that
		// person to clone sends them to fix something that is not broken, while the
		// variable they set stays wrong; this page exists to say where the answer
		// came from, so it has to say when the answer is the problem.
		line("checkout", "none usable — "+firstLine(err.Error()))
		return
	}

	line("checkout", layout.Root)
	line("found by", explainSource(source))

	if git, gitErr := infra.NewGitCLI(); gitErr == nil {
		state := infra.InspectCheckout(ctx, git, layout.Root, source == sourceManaged)
		ref := state.Ref
		if ref == "" {
			ref = "untagged"
		}
		line("version", ref)
	}
}

// describeInitializedFor says which environment's state the checkout's working
// directories are currently wired to.
//
// This CLI passes -reconfigure on every init, so a run is always pointed where
// it says it is. What outlives the run is the directory: it keeps the last
// environment's backend, and a `terraform plan` typed by hand in there reads
// that one. With two environments in one checkout — the normal case — that is
// how somebody ends up reading prd's infrastructure while thinking about stg.
//
// Reported under Templates because it is a fact about the checkout, and only
// when something is initialized: a fresh clone has nothing to say here.
func describeInitializedFor(line func(label, value string)) {
	layout, _, err := resolveLayout("", os.Getenv("LERIAN_TF_REPO"), "")
	if err != nil {
		return
	}
	catalog, err := infra.Discover(layout)
	if err != nil {
		return
	}

	buckets := map[string]bool{}
	for _, stage := range mustResolve(layout, catalog) {
		for _, unit := range stage.Units {
			if bucket, ok := infra.InitializedFor(unit); ok {
				buckets[bucket] = true
			}
		}
	}
	if len(buckets) == 0 {
		return
	}

	names := make([]string, 0, len(buckets))
	for bucket := range buckets {
		names = append(names, bucket)
	}
	sort.Strings(names)
	line("terraform", "directories are initialized against "+strings.Join(names, ", "))
	line("", "a terraform run by hand in them uses that, whatever --env says")
}

// mustResolve is every root the catalog knows, or nothing when it cannot be
// resolved — this is a report, and a checkout it cannot read has nothing to say.
func mustResolve(layout infra.Layout, catalog infra.Catalog) []infra.Stage {
	stages, err := infra.Resolve(layout, catalog, "all")
	// A checkout whose roots cannot be resolved has nothing to say about which
	// backend its directories point at, and the run that actually needs them
	// resolved reports the failure properly. (Not a swallowed error: this returns
	// a slice, and the nil is an empty one.)
	if err != nil {
		return nil
	}
	return stages
}

// describeKubeconfig is the file kubectl reads, which this tool writes into
// after an apply and otherwise only looks at.
//
// Its own group rather than a line under AWS: the file belongs to kubectl, it
// holds clusters from anywhere — a local kind, another cloud — and filing it
// under AWS would say otherwise. Names only; a kubeconfig can carry client
// certificates and tokens, and this page exists to say where things are.
func describeKubeconfig(line func(label, value string)) {
	summary := infra.DescribeKubeconfig()

	switch {
	case summary.Problem != "":
		line("kubeconfig", summary.Path)
		line("state", summary.Problem)
		return
	case !summary.Exists:
		line("kubeconfig", summary.Path+" — not there yet")
		return
	}

	line("kubeconfig", summary.Path)
	if summary.Current == "" {
		line("context", "none selected")
	} else {
		line("context", describeKubeContext(summary.Current))
	}

	short := make([]string, 0, len(summary.Clusters))
	for _, cluster := range summary.Clusters {
		short = append(short, shortKubeName(cluster))
	}
	line("clusters", fmt.Sprintf("%d: %s", len(short), nameAFew(short)))
}

// eksContext matches the name update-kubeconfig gives an entry, which is the
// cluster's ARN.
var eksContext = regexp.MustCompile(`^arn:aws[\w-]*:eks:([\w-]+):(\d+):cluster/(.+)$`)

// shortKubeName is a cluster entry at the width a line has.
//
// An EKS entry is named by its ARN — 60 characters of which the last few are the
// only part anybody reads. Anything else is left exactly as it is: a kubeconfig
// holds clusters from anywhere, and a local kind cluster's name is already its
// name.
func shortKubeName(name string) string {
	if parts := eksContext.FindStringSubmatch(name); parts != nil {
		return parts[3]
	}
	return name
}

// describeKubeContext names the current context and, for an EKS one, where it
// points.
//
// The region and the account are the two facts that tell two same-named clusters
// apart, and "which cluster am I actually talking to" is the question this line
// exists to answer.
func describeKubeContext(name string) string {
	parts := eksContext.FindStringSubmatch(name)
	if parts == nil {
		return name
	}
	return fmt.Sprintf("%s  ·  eks %s · account %s", parts[3], parts[1], parts[2])
}

// describeAWS is what ~/.aws holds, read rather than exercised.
func describeAWS(line func(label, value string)) {
	home, err := os.UserHomeDir()
	if err == nil {
		line("config", filepath.Join(home, ".aws", "config"))
	}

	profiles, err := infra.ListAWSProfiles()
	if err != nil {
		line("profiles", "cannot read: "+err.Error())
		return
	}
	if len(profiles) == 0 {
		line("profiles", "none — aws configure sso, or credentials in the environment")
		return
	}

	names := make([]string, 0, len(profiles))
	sessions := map[string]bool{}
	for _, profile := range profiles {
		names = append(names, profile.Name)
		if profile.SSOSession != "" {
			sessions[profile.SSOSession] = true
		}
	}
	line("profiles", fmt.Sprintf("%d: %s", len(names), nameAFew(sorted(names))))

	if len(sessions) == 0 {
		return
	}
	named := make([]string, 0, len(sessions))
	for session := range sessions {
		named = append(named, session)
	}
	line("sessions", nameAFew(sorted(named)))
}

// explainSource turns the source constant into something somebody can act on:
// knowing a checkout came from $LERIAN_TF_REPO tells you which variable to unset,
// where "env" alone tells you to go and read the code.
func explainSource(source checkoutSource) string {
	switch source {
	case sourceManaged:
		return "the managed path — found by convention, not recorded"
	case sourceRemembered:
		return "remembered in the config above"
	case sourceEnv:
		return "$LERIAN_TF_REPO in this shell"
	case sourceFlag:
		return "the --repo flag"
	case sourceWorkingIn:
		return "the working directory, or one above it"
	case sourceAsked:
		return "answered during this run"
	default:
		return string(source)
	}
}

func sorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
