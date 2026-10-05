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

// askForEnvironment asks which environment an account is being set up as.
//
// It used to be taken rather than asked: the first free slot, in order, silently.
// That is a decision about capacity and cost dressed as an implementation detail
// — set a production account up first in a fresh checkout and it got dev, which
// is db.t4g.micro, single-AZ, one day of backups and no deletion protection.
//
// Asked here, before init, because the answer decides three things at once: which
// envs/<env>.tfvars is written, which backend/<env>.hcl, and the name of the state
// bucket. After init they are all settled.
func askForEnvironment(ask *prompter, layout infra.Layout, account string) (string, error) {
	options := freeEnvironmentOptions(layout)
	if firstEnabled(options) < 0 || !options[firstEnabled(options)].selectable() {
		return "", errCheckoutFull(layout, account)
	}

	return ask.pick(
		"Which environment is this account?",
		"It picks the sizing the templates ship, and the name of the state bucket.",
		"--env", options, "")
}

// freeEnvironmentOptions lists the three, with the taken ones shown rather than
// hidden and not selectable.
//
// Distinct from init's own environmentOptions, which offers all three as valid:
// there, picking one that is already configured means reconfiguring that same
// account, which is a thing to do. Here a taken slot belongs to a DIFFERENT
// account, and a checkout holds one account per environment.
//
// Shown because their absence is the question somebody would ask next: a menu
// that silently offers two of three reads as a tool with opinions, while a row
// saying "taken by 905418424496" says a checkout holds one account per
// environment and that this one is spoken for.
func freeEnvironmentOptions(layout infra.Layout) []option {
	options := make([]option, 0, len(infra.Environments))
	for _, name := range infra.Environments {
		row := option{value: name, label: name, note: environmentNotes[name]}
		if config, err := infra.LoadEnvConfig(layout, name); err == nil {
			row.disabled = true
			row.note = "taken by account " + config.AccountID
		}
		options = append(options, row)
	}
	return options
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
