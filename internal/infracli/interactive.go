package infracli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
	"github.com/lerian-studio/lerian-cli/internal/menu"
)

// runInteractive is what `lerian infra` with no arguments does on a terminal.
//
// It starts with the setup check, and refuses to go on unless it passes. Every
// path this menu can take shells out to terraform or the AWS CLI, so offering
// them on a machine that is missing one only defers the failure to a point where
// the operator has already answered three questions.
//
// It then asks its way to an argument list and hands that to the ordinary flag
// path, rather than calling the internals directly. The menu is a way to write
// the command line, not a second implementation of it: whatever it runs is what
// the operator could have typed, and it prints that line before running so the
// next time they can.
func runInteractive(ctx context.Context, in io.Reader, stdout, stderr io.Writer) error {
	// One reader for the whole session; see menu.Select on why.
	reader := menu.NewReader(in)
	results := []checkResult{
		checkAWSCLI(ctx),
		checkTerraform(ctx),
		checkGit(),
		checkTemplates(ctx, "", repoFromEnvironment(), ""),
	}
	for _, result := range results {
		if !result.ok {
			_ = reportChecks(stdout, results)
			return errors.New("the environment is not ready; fix the above and run again")
		}
	}
	fmt.Fprintf(stdout, "\n  environment ok — aws, terraform, git and the templates checkout\n")

	args, err := composeArgs(reader, stdout)
	if errors.Is(err, menu.ErrCancelled) {
		return nil
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\n  running: lerian infra %s\n", joinArgs(args))
	return run(ctx, args, stdout, stderr)
}

// composeArgs walks the questions an action needs, and returns the command line
// they add up to.
func composeArgs(in *bufio.Reader, out io.Writer) ([]string, error) {
	action, err := menu.Select(in, out, "What do you want to do?", []menu.Option{
		{Name: "plan", Description: "resolve and plan every root, change nothing"},
		{Name: "apply", Description: "plan, confirm, then apply"},
		{Name: "destroy", Description: "destroy, in reverse dependency order"},
		{Name: "output", Description: "read terraform output from every root"},
		{Name: "helm-values", Description: "merge a product's outputs into one values document"},
		{Name: "list", Description: "list the deployable targets, without touching AWS"},
		{Name: "check", Description: "re-run the environment check"},
		{Name: "init", Description: "write the configuration of an environment"},
	})
	if err != nil {
		return nil, err
	}

	switch action.Name {
	case "list":
		return []string{"--list"}, nil
	case "check":
		return []string{"check"}, nil
	}

	environment, err := chooseEnvironment(in, out)
	if err != nil {
		return nil, err
	}

	if action.Name == "init" {
		return []string{"init", "--env", environment}, nil
	}

	target, err := menu.Select(in, out, "Which target?", []menu.Option{
		{Name: "infra-base", Description: "the VPC and the cluster"},
		{Name: "bootstrap", Description: "the state bucket and lock table, once per environment"},
		{Name: "all", Description: "every target, in dependency order"},
		{Name: "other", Description: "name one — run 'list' first to see them"},
	})
	if err != nil {
		return nil, err
	}
	if target.Name == "other" {
		typed, err := readLine(in, out, "Target: ")
		if err != nil {
			return nil, err
		}
		target.Name = typed
	}

	// A dry run is offered ahead of anything that writes, because it is the step
	// that shows the execution plan and the state keys without an AWS call.
	if action.Name == "apply" || action.Name == "destroy" {
		confirm, err := menu.Select(in, out, fmt.Sprintf("%s changes real infrastructure. Continue?", action.Name),
			[]menu.Option{
				{Name: "dry-run first", Description: "print the execution plan and stop"},
				{Name: action.Name, Description: "go ahead"},
			})
		if err != nil {
			return nil, err
		}
		if confirm.Name == "dry-run first" {
			return []string{"--env", environment, "--target", target.Name, "--dry-run"}, nil
		}
	}

	return []string{"--env", environment, "--target", target.Name, "--action", action.Name}, nil
}

func chooseEnvironment(in *bufio.Reader, out io.Writer) (string, error) {
	options := make([]menu.Option, 0, len(infra.Environments))
	for _, environment := range infra.Environments {
		options = append(options, menu.Option{Name: environment})
	}
	chosen, err := menu.Select(in, out, "Which environment?", options)
	if err != nil {
		return "", err
	}
	return chosen.Name, nil
}

// repoFromEnvironment is the same source the flag path reads, so the check the
// menu runs resolves the checkout a run would resolve.
func repoFromEnvironment() string { return os.Getenv("LERIAN_TF_REPO") }

func readLine(in *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("cannot read the answer: %w", err)
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", errors.New("nothing typed")
	}
	return answer, nil
}

// joinArgs renders the command line the menu built, so the operator can type it
// directly next time instead of walking the questions again.
func joinArgs(args []string) string { return strings.Join(args, " ") }
