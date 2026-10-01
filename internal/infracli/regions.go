package infracli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// typedRegionChoice is the row that takes a region this list has never heard of.
const typedRegionChoice = "\x00type-a-region"

// awsRegions is the commercial regions, in the order AWS documents them, with the
// place each one is — "sa-east-1" is not where most people know São Paulo to be,
// and a list of codes alone is a memory test.
//
// A copy, and copies age: AWS adds regions, and this file does not hear about it.
// That is why typedRegionChoice exists — the list is a convenience, never a gate.
// GovCloud and the China partitions are deliberately absent; they need their own
// credentials and endpoints, and somebody deploying there types the code.
var awsRegions = []struct{ code, place string }{
	{"us-east-1", "N. Virginia"},
	{"us-east-2", "Ohio"},
	{"us-west-1", "N. California"},
	{"us-west-2", "Oregon"},
	{"ca-central-1", "Canada, Central"},
	{"sa-east-1", "São Paulo"},
	{"eu-west-1", "Ireland"},
	{"eu-west-2", "London"},
	{"eu-west-3", "Paris"},
	{"eu-central-1", "Frankfurt"},
	{"eu-central-2", "Zurich"},
	{"eu-north-1", "Stockholm"},
	{"eu-south-1", "Milan"},
	{"eu-south-2", "Spain"},
	{"ap-south-1", "Mumbai"},
	{"ap-southeast-1", "Singapore"},
	{"ap-southeast-2", "Sydney"},
	{"ap-southeast-3", "Jakarta"},
	{"ap-northeast-1", "Tokyo"},
	{"ap-northeast-2", "Seoul"},
	{"ap-northeast-3", "Osaka"},
	{"ap-east-1", "Hong Kong"},
	{"me-south-1", "Bahrain"},
	{"me-central-1", "UAE"},
	{"il-central-1", "Tel Aviv"},
	{"af-south-1", "Cape Town"},
}

// regionOptions is the region list, opening on the one already known.
//
// known is whatever the run has been told so far, and source says where that came
// from — a flag the operator passed, or the profile's own region. Saying which
// matters: "already chosen" was on this row for a while, and it was not true. The
// region had been read off ~/.aws, and calling it a choice invites somebody to
// press enter believing they are confirming a decision they made earlier.
//
// It goes first because the cursor starts on the first row, and it is added even
// when this list has never heard of it: a region the operator named is a region
// that exists, whatever this file thinks.
func regionOptions(known, source string) []option {
	options := make([]option, 0, len(awsRegions)+2)

	if known != "" {
		note := placeOf(known) + source
		if source == "" {
			note = strings.TrimSuffix(placeOf(known), " · ")
		}
		options = append(options, option{value: known, label: known, note: note})
	}

	for _, region := range awsRegions {
		if region.code == known {
			continue
		}
		options = append(options, option{value: region.code, label: region.code, note: region.place})
	}

	return append(options, option{
		value: typedRegionChoice,
		label: "another region",
		note:  "type a code this list does not have — AWS adds regions, this copy ages",
	})
}

// placeOf names a region, and returns "" for one this list has never seen. The
// trailing separator is part of it so the caller can concatenate either way.
func placeOf(code string) string {
	for _, region := range awsRegions {
		if region.code == code {
			return region.place + " · "
		}
	}
	return ""
}

// askForRegion offers the list and falls through to typing when the operator
// wants one it does not have.
func askForRegion(ask *prompter, known, source string) (string, error) {
	chosen, err := ask.pick(
		"Which AWS region will the infrastructure be created in?",
		"Every resource lands here. Moving later means recreating them.",
		"--region", regionOptions(known, source), known)
	if err != nil {
		return "", err
	}
	if chosen != typedRegionChoice {
		return chosen, nil
	}

	typed, err := ask.ask(
		"Which AWS region will the infrastructure be created in?",
		"A region code, like eu-west-1.", "", "--region")
	if err != nil {
		return "", err
	}
	if err := validateRegion(typed); err != nil {
		return "", err
	}
	return typed, nil
}

// validateRegion checks the shape rather than the membership.
//
// The list of regions in this file ages; the shape of a region code does not, and
// a value that is not shaped like one — a typo, a pasted line, an account id —
// would otherwise reach the AWS API and fail there, several steps from the prompt
// that accepted it.
func validateRegion(code string) error {
	if !regionShape.MatchString(code) {
		return fmt.Errorf("%q is not an AWS region code\n"+
			"They look like us-east-1, eu-west-2 or sa-east-1: an area, a direction and a number.", code)
	}
	return nil
}

// regionShape is <area>-<direction>-<number>, which every region has looked like
// since the scheme was introduced.
var regionShape = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)

// typedAddressChoice is the row that takes an address other than the detected one.
const typedAddressChoice = "\x00type-an-address"

// egressOptions is the two answers there are once an address has been detected:
// use it, or give another.
//
// A blank line here asks somebody to retype what the detection just found, which
// is both work and a chance to mistype it. The escape is there because the
// detected address is what this machine appears to come from — a laptop behind a
// VPN, or a CI runner, may need to name something else.
func egressOptions(detected string) []option {
	return []option{
		{value: detected, label: detected, note: "what this machine appears to come from"},
		{value: typedAddressChoice, label: "another address", note: "type the address that should reach the API"},
	}
}

// profileRegion is the region a named profile declares in ~/.aws, or "" when it
// declares none or cannot be read.
//
// Only ever a suggestion: it is where the region list opens, so the operator sees
// the thing they most likely want without it being chosen for them.
func profileRegion(name string) string {
	if name == "" {
		return ""
	}
	profiles, err := infra.ListAWSProfiles()
	if err != nil {
		return ""
	}
	for _, profile := range profiles {
		if profile.Name == name {
			return profile.Region
		}
	}
	return ""
}

// regionFor settles the region for a profile that has just been chosen.
//
// A region already passed is the answer — somebody stated it. Otherwise it is
// asked for, with the profile's own region as the suggestion the list opens on.
//
// Not taken silently from the profile, which is what this used to do: the profile
// may have been configured for something else entirely, and infrastructure in a
// region nobody said out loud is discovered later, by the bill or by the latency.
func regionFor(ask *prompter, profile, passed, profileRegion string) (string, error) {
	if passed != "" {
		return passed, nil
	}
	source := ""
	if profileRegion != "" {
		source = "from the " + profile + " profile"
	}
	return askForRegion(ask, profileRegion, source)
}
