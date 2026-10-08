package infracli

import (
	"fmt"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// environmentNotes is what each environment's templates provision, in the words
// somebody choosing between them needs.
//
// Written here rather than read out of the tfvars at run time: parsing HCL to
// describe it would tie a menu to the shape of files that exist to be edited. The
// risk is that this drifts from what the templates actually ship, so a test reads
// the examples and fails when it does — the note is checked against the thing it
// describes, just not while somebody is waiting for a menu.
var environmentNotes = map[string]string{
	"dev": "smallest classes, single-AZ, short backups — cheapest",
	"stg": "production's shape, smaller",
	"prd": "largest classes, multi-AZ, long backups, deletion protection",
}

// environmentChoices is the three, each saying what it is FOR THIS ACCOUNT:
// already configured here, free, or held by somebody else.
//
// The state is on the row because it decides what happens next. "configured
// here" means the run goes straight to what to operate on; "free" means init
// runs first; "held by" means not this checkout, and a row that said only
// "production" would leave somebody picking it and then being refused.
func environmentChoices(layout infra.Layout, account string) []option {
	options := make([]option, 0, len(infra.Environments))
	for _, name := range infra.Environments {
		row := option{value: name, label: name, note: environmentNotes[name]}

		config, err := infra.LoadEnvConfig(layout, name)
		switch {
		case err != nil:
			row.note += "  ·  not set up here yet"
		case config.AccountID == account:
			row.note = "configured here" + backendNote(layout, name)
		default:
			row.disabled = true
			row.note = "another account holds it: " + config.AccountID
		}
		options = append(options, row)
	}
	return options
}

// backendNote says whether this environment has somewhere to keep state, which
// is the difference between a run that can do anything and one that can only
// bootstrap.
func backendNote(layout infra.Layout, environment string) string {
	if backendExists(layout, environment) {
		return "  ·  state backend ready"
	}
	return "  ·  no state backend yet"
}

// errCheckoutFull is the error for a checkout whose three environments are all
// spoken for.
func errCheckoutFull(layout infra.Layout, account string) error {
	return fmt.Errorf("this checkout already holds three accounts, which is all it can hold\n"+
		"Each account occupies one of backend/<env>.hcl and envs/<env>.tfvars, and the\n"+
		"templates provide three sets: %s.\n\n"+
		"Use a separate checkout for account %s, or free one of the three in\n%s",
		joinNames(infra.Environments), account, layout.RepoRel(layout.ConfigFile()))
}

// joinNames is "dev, stg and prd" — the list as a sentence, because this one is
// read in the middle of one.
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	out := ""
	for index, name := range names[:len(names)-1] {
		if index > 0 {
			out += ", "
		}
		out += name
	}
	return out + " and " + names[len(names)-1]
}
