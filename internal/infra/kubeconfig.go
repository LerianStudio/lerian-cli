package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// KubeconfigPath is the file `aws eks update-kubeconfig` would write.
//
// KUBECONFIG may name several files separated by the path separator; the AWS CLI
// writes to the first, so that is the one to inspect. An empty entry is skipped
// rather than treated as a path, because "a:b" split on an empty middle yields
// one, and the empty string would resolve to the working directory.
func KubeconfigPath() (string, error) {
	for _, candidate := range filepath.SplitList(os.Getenv("KUBECONFIG")) {
		if strings.TrimSpace(candidate) != "" {
			return candidate, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("infra: cannot resolve the home directory: %w", err)
	}
	return filepath.Join(home, ".kube", "config"), nil
}

// KubeEntry is what a kubeconfig already says about one cluster.
type KubeEntry struct {
	// Name is the cluster entry's name, which update-kubeconfig sets to the
	// cluster ARN.
	Name string
	// Server is the API endpoint it points at.
	Server string
}

// ReadKubeconfig returns the entry for name, and whether there was one.
//
// A file that does not exist is not an error: it is the normal state before the
// first cluster, and the answer — nothing would be replaced — is the same either
// way. A file that cannot be parsed IS an error, because the alternative is
// telling somebody nothing will be overwritten in a file this cannot read.
func ReadKubeconfig(path, name string) (KubeEntry, bool, error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return KubeEntry{}, false, nil
	}
	if err != nil {
		return KubeEntry{}, false, fmt.Errorf("infra: cannot read %s: %w", path, err)
	}

	var parsed struct {
		Clusters []struct {
			Name    string `yaml:"name"`
			Cluster struct {
				Server string `yaml:"server"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		return KubeEntry{}, false, fmt.Errorf("infra: cannot read %s: %w", path, err)
	}

	for _, cluster := range parsed.Clusters {
		if cluster.Name == name {
			return KubeEntry{Name: cluster.Name, Server: cluster.Cluster.Server}, true, nil
		}
	}
	return KubeEntry{}, false, nil
}

// KubeSummary is what a kubeconfig says, for a page that reports where things
// are.
type KubeSummary struct {
	// Path is the file, whether or not it exists.
	Path string
	// Exists is false for a machine that has never pointed kubectl anywhere.
	Exists bool
	// Current is the context kubectl would use.
	Current string
	// Clusters is every cluster entry, in file order.
	Clusters []string
	// Problem is set when the file is there and cannot be read, which is worth
	// saying rather than reporting an empty file.
	Problem string
}

// DescribeKubeconfig reads the file kubectl would use.
//
// Names only — contexts and clusters. A kubeconfig can hold client certificates
// and tokens, and a page that exists to say WHERE things are has no business
// printing what is in them.
func DescribeKubeconfig() KubeSummary {
	path, err := KubeconfigPath()
	if err != nil {
		return KubeSummary{Problem: err.Error()}
	}
	summary := KubeSummary{Path: path}

	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return summary
	}
	if err != nil {
		summary.Problem = err.Error()
		return summary
	}
	summary.Exists = true

	var parsed struct {
		CurrentContext string `yaml:"current-context"`
		Clusters       []struct {
			Name string `yaml:"name"`
		} `yaml:"clusters"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		summary.Problem = "cannot be read: " + err.Error()
		return summary
	}

	summary.Current = parsed.CurrentContext
	for _, cluster := range parsed.Clusters {
		summary.Clusters = append(summary.Clusters, cluster.Name)
	}
	return summary
}

// UpdateKubeconfig points kubectl at a cluster, by running the AWS CLI rather
// than writing the file.
//
// The file has a format this tool has no business owning — contexts, users, exec
// credential plugins, the arguments that plugin needs for this CLI version — and
// the AWS CLI writes it correctly for the version of itself that is installed.
// Reading it to see what is there is one thing; authoring it is another.
func UpdateKubeconfig(ctx context.Context, cluster, region, profile string) error {
	args := []string{"eks", "update-kubeconfig", "--name", cluster}
	if region != "" {
		args = append(args, "--region", region)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}

	command := exec.CommandContext(ctx, "aws", args...)
	command.Env = os.Environ()

	var stderr strings.Builder
	command.Stderr = &stderr
	if _, err := command.Output(); err != nil {
		return fmt.Errorf("aws eks update-kubeconfig failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ClusterLister finds the EKS clusters an account holds in a region. An
// interface so the menu that uses it can be tested without credentials.
type ClusterLister interface {
	ListClusters(ctx context.Context, profile, region string) ([]string, error)
}

// CLIClusters asks the AWS CLI.
type CLIClusters struct {
	// Binary is the aws executable. Empty means "aws" from PATH.
	Binary string
}

// ListClusters returns the cluster names in one region.
//
// Per region because EKS is: a cluster belongs to a region, and "the clusters in
// this account" is not a question the API answers. The caller decides which
// region to ask about, and says so on screen — a menu that silently showed one
// region's clusters would read as the whole account.
func (c CLIClusters) ListClusters(ctx context.Context, profile, region string) ([]string, error) {
	binary := c.Binary
	if binary == "" {
		binary = "aws"
	}

	args := []string{"eks", "list-clusters", "--query", "clusters[]", "--output", "text"}
	if region != "" {
		args = append(args, "--region", region)
	}

	command := exec.CommandContext(ctx, binary, args...)
	command.Env = os.Environ()
	if profile != "" {
		command.Env = append(command.Env, "AWS_PROFILE="+profile)
	}

	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("aws eks list-clusters failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.Fields(string(out)), nil
}

// ClusterARN is what update-kubeconfig names the entry, built from the three
// facts that identify a cluster.
//
// Built rather than read back from describe-cluster: this is used to work out
// whether the kubeconfig already holds this cluster, which has to be answerable
// before deciding to call AWS again.
func ClusterARN(partition, region, account, name string) string {
	if partition == "" {
		partition = "aws"
	}
	return "arn:" + partition + ":eks:" + region + ":" + account + ":cluster/" + name
}

// InitializedFor reports the state bucket a root's working directory is
// currently initialized against, and whether there is one at all.
//
// It exists because the answer outlives a run. This CLI passes -reconfigure on
// every init, so what IT does is always right — but between runs the directory
// keeps whatever the last environment left there, and a `terraform plan` typed
// by hand reads that. Two environments in one checkout is the normal case here,
// so the directory that says "stg" while somebody is thinking "prd" is not an
// edge case; it is Tuesday.
func InitializedFor(unit Unit) (string, bool) {
	body, err := os.ReadFile(filepath.Join(unit.Dir, ".terraform", "terraform.tfstate"))
	if err != nil {
		return "", false
	}

	var parsed struct {
		Backend struct {
			Config struct {
				Bucket string `json:"bucket"`
			} `json:"config"`
		} `json:"backend"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false
	}
	if parsed.Backend.Config.Bucket == "" {
		return "", false
	}
	return parsed.Backend.Config.Bucket, true
}
