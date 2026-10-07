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
	fmt.Fprintf(out, "  $(eval $(ARGS):;@:)\n")
	fmt.Fprintf(out, "endif\n\n")
}

// writeMakeTargets is the work.
func writeMakeTargets(out *strings.Builder) {
	fmt.Fprintf(out, ".PHONY: plan apply destroy output init test check help require-root\n\n")

	fmt.Fprintf(out, "require-root:\n")
	fmt.Fprintf(out, "\t@test -n \"$(ROOT)\" || { echo '  which root? e.g. make plan %s dev'; "+
		"echo '  roots: $(ROOTS)'; exit 1; }\n", "<root>")
	fmt.Fprintf(out, "\t@test -d \"$(RESOLVED)\" || { echo \"  no such root: $(ROOT)\"; "+
		"echo '  roots: $(ROOTS)'; exit 1; }\n")
	fmt.Fprintf(out, "\t@test -f \"$(RESOLVED)/envs/$(ENV).tfvars\" || "+
		"{ echo \"  $(RESOLVED) has no envs/$(ENV).tfvars\"; exit 1; }\n\n")

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

	for _, verb := range []string{"plan", "apply", "destroy"} {
		fmt.Fprintf(out, "%s: init\n", verb)
		if verb != "plan" {
			// The same bar the CLI sets: these write, and a Makefile that applies
			// on one word is a Makefile somebody applies to the wrong environment.
			fmt.Fprintf(out, "\t@printf '  %s %%s in %%s. Type yes to continue: ' '$(RESOLVED)' '$(ENV)'; \\\n", verb)
			fmt.Fprintf(out, "\tread -r answer; [ \"$$answer\" = yes ] || { echo '  stopped.'; exit 1; }\n")
		}
		fmt.Fprintf(out, "\t$(TERRAFORM) -chdir=$(RESOLVED) %s -var-file=envs/$(ENV).tfvars\n\n", verb)
	}

	fmt.Fprintf(out, "output: init\n\t$(TERRAFORM) -chdir=$(RESOLVED) output\n\n")
	fmt.Fprintf(out, "test: require-root\n\t$(TERRAFORM) -chdir=$(RESOLVED) test\n\n")

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
