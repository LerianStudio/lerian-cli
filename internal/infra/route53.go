package infra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// HostedZone is a Route53 zone as the account reports it.
type HostedZone struct {
	// Name is the domain, with the trailing dot AWS returns stripped.
	Name string
	// ID is the bare zone id, without the /hostedzone/ prefix.
	ID string
	// Private marks a zone that only resolves inside a VPC. It matters because
	// the roles that want these ARNs — ExternalDNS and cert-manager — publish
	// records the public internet has to see.
	Private bool
}

// ARN is the form an IAM policy wants.
func (z HostedZone) ARN() string { return "arn:aws:route53:::hostedzone/" + z.ID }

// ZoneLister finds the Route53 zones an account holds. An interface so the
// prompt that uses it can be tested without credentials.
type ZoneLister interface {
	ListHostedZones(ctx context.Context, profile string) ([]HostedZone, error)
}

// CLIZones asks the AWS CLI, the same way everything else here does.
type CLIZones struct {
	// Binary is the aws executable. Empty means "aws" from PATH.
	Binary string
}

// ListHostedZones returns every zone in the account.
//
// Route53 is global, so no region is passed: a zone belongs to the account, not
// to a region, and naming one would be inventing a constraint the service does
// not have.
func (c CLIZones) ListHostedZones(ctx context.Context, profile string) ([]HostedZone, error) {
	binary := c.Binary
	if binary == "" {
		binary = "aws"
	}

	command := exec.CommandContext(ctx, binary, "route53", "list-hosted-zones",
		"--query", "HostedZones[].[Name,Id,Config.PrivateZone]", "--output", "text")
	command.Env = os.Environ()
	if profile != "" {
		command.Env = append(command.Env, "AWS_PROFILE="+profile)
	}

	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("aws route53 list-hosted-zones failed: %w\n%s",
			err, strings.TrimSpace(stderr.String()))
	}

	var zones []HostedZone
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		zones = append(zones, HostedZone{
			Name:    strings.TrimSuffix(fields[0], "."),
			ID:      strings.TrimPrefix(fields[1], "/hostedzone/"),
			Private: strings.EqualFold(fields[2], "true"),
		})
	}
	return zones, nil
}
