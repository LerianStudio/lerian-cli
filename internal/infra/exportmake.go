package infra

import (
	"fmt"
	"path/filepath"
	"strings"
)

// WriteMakefile is the front door of the exported repository.
//
// Running one root by hand is five arguments, two of which cannot be guessed:
// the backend file for the environment, and the state key for that root. People
// get those right the first time by copying them out of the README and wrong
// every time after.
//
// It DISCOVERS its roots rather than listing them. A generated list is accurate
// on the day of the export and wrong the first time somebody adds a service —
// and adding services is the point of handing this repository over. The shape is
// the contract instead: a root is a directory with an `envs/` in it, and its
// state key is its own path. Both hold for every root the templates ship and for
// every one added the same way.
func WriteMakefile(plan ExportPlan) string {
	var out strings.Builder

	fmt.Fprint(&out, bannerFor(""))
	fmt.Fprintf(&out, "# make plan eks dev     — or: make eks-plan ENV=dev\n")
	fmt.Fprintf(&out, "# make help             — every target, with what it does\n")
	fmt.Fprintf(&out, "#\n")
	fmt.Fprintf(&out, "# Roots are found, not listed: any directory with an envs/ in it is one,\n")
	fmt.Fprintf(&out, "# and its state key is its own path. Add a service the same way and it\n")
	fmt.Fprintf(&out, "# appears here with no change to this file.\n\n")

	fmt.Fprintf(&out, "ENV ?= dev\nTERRAFORM ?= terraform\n\n")

	writeMakeDiscovery(&out, plan)
	writeMakePositional(&out)
	writeMakeTargets(&out)
	writeMakeShortcuts(&out)
	writeMakeHelp(&out, plan)

	return out.String()
}

// writeMakeDiscovery finds the roots and works out what each one needs.
func writeMakeDiscovery(out *strings.Builder, plan ExportPlan) {
	fmt.Fprintf(out, "# Every directory holding an envs/ — excluding the module cache, which has\n")
	fmt.Fprintf(out, "# copies of other people's roots in it.\n")
	fmt.Fprintf(out, "ROOTS := $(sort $(patsubst ./%%/envs,%%,$(shell find . -type d -name envs "+
		"-not -path './.git/*' -not -path '*/.terraform/*' 2>/dev/null)))\n\n")

	// The prefix the state keys are relative to. With the templates' layout
	// dropped it is empty and a root's path is its key; when something sat
	// outside and the prefix stayed, it has to come off again here.
	prefix := ""
	if plan.Base != "" {
		prefix = ""
	} else if len(plan.Roots) > 0 {
		if cut := strings.LastIndex(filepath.ToSlash(plan.Roots[0].Path), "/"+keySuffixOf(plan.Roots[0])); cut > 0 {
			prefix = filepath.ToSlash(plan.Roots[0].Path)[:cut+1]
		}
	}
	fmt.Fprintf(out, "# The order the roots were applied in. Discovery cannot work this out —\n")
	fmt.Fprintf(out, "# nothing in a directory says the network comes before the cluster in it —\n")
	fmt.Fprintf(out, "# so it is written down. Anything found that is not in this list runs after\n")
	fmt.Fprintf(out, "# it, in alphabetical order.\n")
	fmt.Fprintf(out, "ORDER := %s\n", strings.Join(orderedPaths(plan), " "))
	fmt.Fprintf(out, "ordered = $(foreach o,$(ORDER),$(filter $(o),$(1))) $(sort $(filter-out $(ORDER),$(1)))\n")
	fmt.Fprintf(out, "reverse = $(if $(1),$(call reverse,$(wordlist 2,$(words $(1)),$(1))) $(firstword $(1)))\n\n")

	fmt.Fprintf(out, "# A root's state key is its path. KEY_PREFIX is what has to come off that\n")
	fmt.Fprintf(out, "# path first — empty when the roots sit at the top level, as they normally do.\n")
	fmt.Fprintf(out, "KEY_PREFIX := %s\n", prefix)
	fmt.Fprintf(out, "STATE_PREFIX := aws\n\n")

	fmt.Fprintf(out, "# Short names: the last directory, which is what people say out loud. A name\n")
	fmt.Fprintf(out, "# two roots would answer to is left out rather than given to one of them.\n")
	fmt.Fprintf(out, "$(foreach r,$(ROOTS),$(eval SHORT_$(notdir $(r)) += $(r)))\n")
	fmt.Fprintf(out, "SHORTS := $(sort $(foreach r,$(ROOTS),$(notdir $(r))))\n")
	fmt.Fprintf(out, "UNIQUE := $(foreach s,$(SHORTS),$(if $(filter 1,$(words $(SHORT_$(s)))),$(s)))\n")
	fmt.Fprintf(out, "$(foreach s,$(UNIQUE),$(eval root_$(s) := $(SHORT_$(s))))\n\n")

	fmt.Fprintf(out, "ROOT ?=\n")
	fmt.Fprintf(out, "RESOLVED = $(or $(root_$(ROOT)),$(ROOT))\n")
	fmt.Fprintf(out, "KEY = $(STATE_PREFIX)/$(patsubst $(KEY_PREFIX)%%,%%,$(RESOLVED))/terraform.tfstate\n")

	// Local state is a property of one root in the templates, and naming it is
	// honest: there is nothing in a directory that says "this one creates the
	// backend", so the Makefile cannot discover it the way it discovers the rest.
	fmt.Fprintf(out, "\n# The stack that creates the backend cannot keep its state in it, so it runs\n")
	fmt.Fprintf(out, "# on local state with a workspace per environment. Named rather than found:\n")
	fmt.Fprintf(out, "# nothing in a directory says which root that is.\n")
	fmt.Fprintf(out, "LOCAL_ROOTS := %s\n", strings.Join(localRoots(plan), " "))
	fmt.Fprintf(out, "LOCAL = $(filter $(RESOLVED),$(LOCAL_ROOTS))\n\n")
}

// keySuffixOf is the part of a state key after "aws/", which is the root's path
// as the templates see it.
func keySuffixOf(root ExportedRoot) string {
	return strings.TrimSuffix(strings.TrimPrefix(root.StateKey, "aws/"), "/terraform.tfstate")
}

// orderedPaths is the roots in the order the CLI applied them, which is the
// order a group has to run in: the network exists before the cluster in it.
func orderedPaths(plan ExportPlan) []string {
	paths := make([]string, 0, len(plan.Roots))
	for _, root := range plan.Roots {
		paths = append(paths, filepath.ToSlash(plan.Target(root.Path)))
	}
	return paths
}

// localRoots is the roots that run on local state, by their exported path.
func localRoots(plan ExportPlan) []string {
	var local []string
	for _, root := range plan.Roots {
		if root.Bootstrap {
			local = append(local, filepath.ToSlash(plan.Target(root.Path)))
		}
	}
	if len(local) == 0 {
		// None was exported, but one may be added later — and it will be called
		// this, because that is what the templates call it.
		local = append(local, "bootstrap")
	}
	return local
}

// writeMakePositional is what lets `make plan eks dev` work.
//
// The extra words of a command line are targets as far as make is concerned, so
// they need rules that do nothing. Those rules are created ONLY when the first
// goal is one of the verbs below — a blanket catch-all would swallow typos too,
// and `make pln eks dev` would sit there doing nothing instead of saying there
// is no such target.
func writeMakePositional(out *strings.Builder) {
	fmt.Fprintf(out, "VERBS := plan apply destroy output init test\n")
	fmt.Fprintf(out, "ifneq ($(filter $(firstword $(MAKECMDGOALS)),$(VERBS)),)\n")
	fmt.Fprintf(out, "  ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))\n")
	fmt.Fprintf(out, "  ifneq ($(word 1,$(ARGS)),)\n    ROOT := $(word 1,$(ARGS))\n  endif\n")
	fmt.Fprintf(out, "  ifneq ($(word 2,$(ARGS)),)\n    ENV := $(word 2,$(ARGS))\n  endif\n")
	fmt.Fprintf(out, "  # Do-nothing rules for the extra words — but never for a word that is\n")
	fmt.Fprintf(out, "  # already a target. `make plan eks dev plan` would otherwise redefine\n")
	fmt.Fprintf(out, "  # plan, and make warns about overriding it.\n")
	fmt.Fprintf(out, "  $(eval $(filter-out $(VERBS) check help,$(ARGS)):;@:)\n")
	fmt.Fprintf(out, "endif\n\n")
}

// writeMakeTargets is the work.
func writeMakeTargets(out *strings.Builder) {
	fmt.Fprintf(out, ".PHONY: plan apply destroy output init test check help require-root roots\n")
	fmt.Fprintf(out, ".PHONY: $(foreach v,plan apply destroy output test,one-$(v) group-$(v))\n\n")

	// HOLDS is the roots underneath a directory that is not one itself —
	// `infra-base` holds two, and answering "infra-base has no envs/dev.tfvars"
	// describes it as a broken root instead of as the parent of two working ones.
	// `all` is spelled out rather than left as the "." the dir of a top-level
	// root reduces to: `make plan . dev` is not something anybody would try.
	fmt.Fprintf(out, "HOLDS = $(if $(filter all,$(ROOT)),$(ROOTS),$(filter $(ROOT)/%%,$(ROOTS)))\n")
	fmt.Fprintf(out, "# Every directory holding roots, so `make roots` can name what the group\n")
	fmt.Fprintf(out, "# form accepts. A root is not a group, even when it sits under one.\n")
	fmt.Fprintf(out, "GROUPS := all $(sort $(filter-out . $(ROOTS),$(patsubst %%/,%%,$(dir $(ROOTS)))))\n\n")

	fmt.Fprintf(out, "require-root:\n")
	fmt.Fprintf(out, "\t@test -n \"$(ROOT)\" || { echo '  which root?  e.g. make plan $(firstword $(UNIQUE)) dev'; "+
		"$(MAKE) --no-print-directory roots; exit 1; }\n")
	fmt.Fprintf(out, "\t@test -z \"$(HOLDS)\" || "+
		"{ echo \"  $(ROOT) holds $(words $(HOLDS)) roots; this target takes one\"; exit 1; }\n")
	fmt.Fprintf(out, "\t@test -n \"$(filter $(RESOLVED),$(ROOTS))\" || "+
		"{ echo \"  no such root: $(ROOT)\"; $(MAKE) --no-print-directory roots; exit 1; }\n")
	fmt.Fprintf(out, "\t@test -f \"$(RESOLVED)/envs/$(ENV).tfvars\" || "+
		"{ echo \"  $(RESOLVED) has no envs/$(ENV).tfvars\"; "+
		"echo \"  it has: $$(ls $(RESOLVED)/envs 2>/dev/null | sed 's/[.]tfvars$$//' | tr '\\n' ' ')\"; "+
		"exit 1; }\n\n")

	// (3) The short names, because somebody who mistyped one is looking for the
	//     list of them — not for the paths, which is what they already saw.
	// roots is a target of its own because "what can I run this against" is the
	// first question somebody has, and reading it out of the error of a command
	// they had to guess at is not an answer.
	fmt.Fprintf(out, "roots:\n")
	fmt.Fprintf(out, "\t@echo '  in the order they are applied:'\n")
	fmt.Fprintf(out, "\t@for root in $(call ordered,$(ROOTS)); do \\\n")
	fmt.Fprintf(out, "\t\tshort=''; for s in $(UNIQUE); do "+
		"[ \"$$(basename $$root)\" = \"$$s\" ] && short=\"  ($$s)\"; done; \\\n")
	fmt.Fprintf(out, "\t\techo \"    $$root$$short\"; \\\n")
	fmt.Fprintf(out, "\tdone\n")
	fmt.Fprintf(out, "\t@echo ''\n")
	fmt.Fprintf(out, "\t@echo '  groups — these run every root under them, in order:'\n")
	fmt.Fprintf(out, "\t@for group in $(GROUPS); do \\\n")
	fmt.Fprintf(out, "\t\techo \"    $$group\"; \\\n")
	fmt.Fprintf(out, "\tdone\n\n")

	// init is its own target and also a prerequisite of everything else, because
	// -reconfigure is what stops a root initialized for one environment from
	// quietly using that environment's bucket when run for another.
	fmt.Fprintf(out, "init: require-root\n")
	fmt.Fprintf(out, "\t@if [ -n \"$(LOCAL)\" ]; then \\\n")
	fmt.Fprintf(out, "\t\t$(TERRAFORM) -chdir=$(RESOLVED) init -input=false >/dev/null && \\\n")
	fmt.Fprintf(out, "\t\t{ $(TERRAFORM) -chdir=$(RESOLVED) workspace select $(ENV) 2>/dev/null || \\\n")
	fmt.Fprintf(out, "\t\t  $(TERRAFORM) -chdir=$(RESOLVED) workspace new $(ENV) >/dev/null; }; \\\n")
	fmt.Fprintf(out, "\telse \\\n")
	fmt.Fprintf(out, "\t\ttest -f backend/$(ENV).hcl || { echo \"  no backend/$(ENV).hcl\"; exit 1; }; \\\n")
	fmt.Fprintf(out, "\t\t$(TERRAFORM) -chdir=$(RESOLVED) init -input=false -reconfigure \\\n")
	fmt.Fprintf(out, "\t\t\t-backend-config=$(CURDIR)/backend/$(ENV).hcl \\\n")
	fmt.Fprintf(out, "\t\t\t-backend-config=\"key=$(KEY)\" >/dev/null; \\\n")
	fmt.Fprintf(out, "\tfi\n\n")

	// Every verb takes either one root or a directory holding several. The group
	// form is what makes `make plan infra-base dev` mean "the whole of it" rather
	// than an error about a directory that is not a root.
	for _, verb := range []string{"plan", "apply", "destroy", "output", "test"} {
		fmt.Fprintf(out, "%s:\n", verb)
		// ROOT and ENV are passed on: a sub-make starts with none of this one's
		// variables, and a group-plan that cannot see ROOT works out that it holds
		// nothing and cheerfully does nothing.
		fmt.Fprintf(out, "\t@if [ -n \"$(HOLDS)\" ]; then "+
			"$(MAKE) --no-print-directory group-%s ROOT=\"$(ROOT)\" ENV=\"$(ENV)\"; "+
			"else $(MAKE) --no-print-directory one-%s ROOT=\"$(ROOT)\" ENV=\"$(ENV)\"; fi\n\n", verb, verb)
	}

	// The group confirms once, for all of them, and the children are told it has
	// happened. Asking per root turns one decision into three, and three prompts
	// in a row is a thing people answer without reading.
	for _, verb := range []string{"plan", "apply", "destroy", "output", "test"} {
		order := "$(call ordered,$(HOLDS))"
		if verb == "destroy" {
			// Backwards: the cluster goes before the network it sits in.
			order = "$(call reverse,$(call ordered,$(HOLDS)))"
		}

		fmt.Fprintf(out, "group-%s:\n", verb)
		if verb == "apply" || verb == "destroy" {
			fmt.Fprintf(out, "\t@test -n \"$(CONFIRMED)\" || { \\\n")
			fmt.Fprintf(out, "\t\tprintf '  %s these in %%s, in order:\\n' '$(ENV)'; \\\n", verb)
			fmt.Fprintf(out, "\t\tfor root in %s; do echo \"    $$root\"; done; \\\n", order)
			fmt.Fprintf(out, "\t\tprintf '  Type yes to continue: '; \\\n")
			fmt.Fprintf(out, "\t\tread -r answer; [ \"$$answer\" = yes ] || { echo '  stopped.'; exit 1; }; }\n")
		}
		fmt.Fprintf(out, "\t@for root in %s; do \\\n", order)
		fmt.Fprintf(out, "\t\tprintf '\\n==> %%s\\n' \"$$root\"; \\\n")
		fmt.Fprintf(out, "\t\t$(MAKE) --no-print-directory one-%s ROOT=$$root ENV=$(ENV) CONFIRMED=1 "+
			"|| exit 1; \\\n", verb)
		fmt.Fprintf(out, "\tdone\n\n")
	}

	for _, verb := range []string{"plan", "apply", "destroy"} {
		fmt.Fprintf(out, "one-%s: init\n", verb)
		if verb != "plan" {
			// The same bar the CLI sets: these write, and a Makefile that applies
			// on one word is a Makefile somebody applies to the wrong environment.
			fmt.Fprintf(out, "\t@test -n \"$(CONFIRMED)\" || { \\\n")
			fmt.Fprintf(out, "\t\tprintf '  %s %%s in %%s. Type yes to continue: ' '$(RESOLVED)' '$(ENV)'; \\\n", verb)
			fmt.Fprintf(out, "\t\tread -r answer; [ \"$$answer\" = yes ] || { echo '  stopped.'; exit 1; }; }\n")
		}
		fmt.Fprintf(out, "\t$(TERRAFORM) -chdir=$(RESOLVED) %s -var-file=envs/$(ENV).tfvars\n\n", verb)
	}

	fmt.Fprintf(out, "one-output: init\n\t$(TERRAFORM) -chdir=$(RESOLVED) output\n\n")
	fmt.Fprintf(out, "one-test: require-root\n\t$(TERRAFORM) -chdir=$(RESOLVED) test\n\n")

	// Both halves run, and the exit status comes at the end. Stopping at the
	// first meant a stray space in a .tfvars hid every validate behind it — and
	// those are the ones that say whether this estate still parses.
	fmt.Fprintf(out, "check:\n")
	fmt.Fprintf(out, "\t@failed=0; \\\n")
	fmt.Fprintf(out, "\t$(TERRAFORM) fmt -check -recursive . >/dev/null || { \\\n")
	fmt.Fprintf(out, "\t\techo '  formatting (cosmetic): terraform fmt -recursive .'; \\\n")
	fmt.Fprintf(out, "\t\t$(TERRAFORM) fmt -check -recursive . | sed 's/^/    /'; \\\n")
	fmt.Fprintf(out, "\t\tfailed=1; }; \\\n")
	fmt.Fprintf(out, "\tfor root in $(ROOTS); do \\\n")
	fmt.Fprintf(out, "\t\t$(TERRAFORM) -chdir=$$root init -backend=false -input=false >/dev/null && \\\n")
	fmt.Fprintf(out, "\t\t$(TERRAFORM) -chdir=$$root validate >/dev/null && echo \"  ok       $$root\" || \\\n")
	fmt.Fprintf(out, "\t\t{ echo \"  invalid  $$root\"; $(TERRAFORM) -chdir=$$root validate; failed=1; }; \\\n")
	fmt.Fprintf(out, "\tdone; \\\n")
	fmt.Fprintf(out, "\texit $$failed\n\n")
}

// writeMakeShortcuts gives each unambiguous root its own target, so tab-complete
// lists them. Generated by make from the roots it found, not written out here:
// a service added next month gets its shortcuts without this file changing.
func writeMakeShortcuts(out *strings.Builder) {
	fmt.Fprintf(out, "define shortcut\n")
	fmt.Fprintf(out, ".PHONY: $(1)-$(2)\n")
	fmt.Fprintf(out, "$(1)-$(2):\n")
	fmt.Fprintf(out, "\t@$$(MAKE) --no-print-directory $(2) ROOT=$(1) ENV=$$(ENV)\n")
	fmt.Fprintf(out, "endef\n")
	fmt.Fprintf(out, "$(foreach s,$(UNIQUE),$(foreach v,plan apply destroy output test,"+
		"$(eval $(call shortcut,$(s),$(v)))))\n\n")
}

// writeMakeHelp is the target somebody runs first.
func writeMakeHelp(out *strings.Builder, plan ExportPlan) {
	fmt.Fprintf(out, "help:\n")
	fmt.Fprintf(out, "\t@echo '  make plan <root> [env]      what would change (writes nothing)'\n")
	fmt.Fprintf(out, "\t@echo '  make apply <root> [env]     writes, after one confirmation'\n")
	fmt.Fprintf(out, "\t@echo '  make destroy <root> [env]   removes, after one confirmation'\n")
	fmt.Fprintf(out, "\t@echo '  make output <root> [env]    terraform output'\n")
	fmt.Fprintf(out, "\t@echo '  make test <root>            terraform test'\n")
	fmt.Fprintf(out, "\t@echo '  make check                  fmt and validate every root'\n")
	fmt.Fprintf(out, "\t@echo '  make roots                  what can be run, and in what order'\n")
	fmt.Fprintf(out, "\t@echo ''\n")
	fmt.Fprintf(out, "\t@echo '  <root> is one root, or a directory holding several:'\n")
	fmt.Fprintf(out, "\t@echo '    make plan infra-base dev  runs every root under it, in order'\n")
	fmt.Fprintf(out, "\t@echo '    make plan all dev         runs every root there is'\n")
	fmt.Fprintf(out, "\t@echo ''\n")
	fmt.Fprintf(out, "\t@echo '  env defaults to dev. These are the same:'\n")
	fmt.Fprintf(out, "\t@echo '    make plan %s stg'\n", firstShort(plan))
	fmt.Fprintf(out, "\t@echo '    make %s-plan ENV=stg'\n", firstShort(plan))
	fmt.Fprintf(out, "\t@echo '    make plan ROOT=%s ENV=stg'\n", firstPath(plan))
	fmt.Fprintf(out, "\t@echo ''\n")
	fmt.Fprintf(out, "\t@echo '  roots (found, not listed): $(ROOTS)'\n")
	fmt.Fprintf(out, "\t@echo '  a new service with an envs/ appears here on its own.'\n")
}

// firstShort and firstPath are the worked example in `make help`, taken from a
// root this repository actually has rather than invented.
func firstShort(plan ExportPlan) string {
	for _, root := range plan.Roots {
		if !root.Bootstrap {
			return filepath.Base(plan.Target(root.Path))
		}
	}
	if len(plan.Roots) > 0 {
		return filepath.Base(plan.Target(plan.Roots[0].Path))
	}
	return "<root>"
}

func firstPath(plan ExportPlan) string {
	if len(plan.Roots) > 0 {
		return filepath.ToSlash(plan.Target(plan.Roots[0].Path))
	}
	return "<root>"
}
