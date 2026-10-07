package infra

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// StateLock is a lock somebody else — or a dead process — is holding on a
// root's state.
//
// Terraform reports this well and acts on it badly: the message is accurate, and
// the thing to do about it depends on a fact terraform cannot know, which is
// whether the holder is still alive. So this reads the message and says what to
// check.
type StateLock struct {
	ID        string
	Operation string
	Who       string
	Created   time.Time
	Path      string
}

var (
	lockFailure = regexp.MustCompile(`Error acquiring the state lock`)
	lockField   = func(name string) *regexp.Regexp {
		return regexp.MustCompile(`(?m)^\s*` + name + `:\s*(.+?)\s*$`)
	}
	lockID        = lockField("ID")
	lockOperation = lockField("Operation")
	lockWho       = lockField("Who")
	lockCreated   = lockField("Created")
	lockPath      = lockField("Path")
)

// ReadStateLock finds a lock report in terraform's output, or returns nil.
//
// Parsed from the human output because there is no other form of it: the error
// arrives as text through tfexec, and the fields below are the ones terraform
// has printed in that block for years.
func ReadStateLock(text string) *StateLock {
	if !lockFailure.MatchString(text) {
		return nil
	}

	lock := &StateLock{
		ID:        firstGroup(lockID, text),
		Operation: firstGroup(lockOperation, text),
		Who:       firstGroup(lockWho, text),
		Path:      firstGroup(lockPath, text),
	}
	if created := firstGroup(lockCreated, text); created != "" {
		// The format terraform prints: a Go time with a zone name after it.
		if at, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", created); err == nil {
			lock.Created = at
		}
	}
	// Without an ID there is no command to offer, and the rest is a description
	// of a problem with no handle on it. Terraform's own message is better.
	if lock.ID == "" {
		return nil
	}
	return lock
}

func firstGroup(pattern *regexp.Regexp, text string) string {
	if match := pattern.FindStringSubmatch(text); match != nil {
		return match[1]
	}
	return ""
}

// Advice is what to do, which depends on something this cannot see.
//
// It does not offer to run force-unlock. Releasing a lock while a write is in
// flight is how two terraforms end up writing one state, and the difference
// between "the process died" and "a colleague is applying right now" lives
// outside this machine. What it can do is say exactly which command, with the id
// filled in, and what to be sure of first — which is the part people skip when
// they go and look it up under time pressure.
func (l StateLock) Advice(dir string, now time.Time) string {
	var advice strings.Builder

	fmt.Fprintf(&advice, "The state is locked. Nothing is broken and nothing was written.\n")
	if l.Who != "" {
		fmt.Fprintf(&advice, "Held by %s", l.Who)
		if operation := l.operation(); operation != "" {
			fmt.Fprintf(&advice, ", from %s", operation)
		}
		if age := l.age(now); age != "" {
			fmt.Fprintf(&advice, ", %s", age)
		}
		fmt.Fprintf(&advice, ".\n")
	}

	fmt.Fprintf(&advice, "\nIf that run is still going — another terminal, CI, a colleague — wait for it.\n")
	fmt.Fprintf(&advice, "Breaking a lock while a write is in flight is how one state gets two writers.\n")
	fmt.Fprintf(&advice, "\nIf it is not (the usual case: interrupted, or the credentials expired\n")
	fmt.Fprintf(&advice, "mid-apply), release it and run again:\n\n")
	fmt.Fprintf(&advice, "  terraform -chdir=%s force-unlock %s\n", dir, l.ID)

	// Said because an interrupted apply is the common cause, and the lock is not
	// the only thing it can leave behind.
	if strings.Contains(l.Operation, "Apply") {
		fmt.Fprintf(&advice, "\nThat run was an apply, so check what it managed to do before trusting\n")
		fmt.Fprintf(&advice, "the next plan: terraform plan after the unlock says what the state now\n")
		fmt.Fprintf(&advice, "disagrees with. An errored.tfstate in the root means the write itself\n")
		fmt.Fprintf(&advice, "failed, and that one needs terraform state push.\n")
	}
	return advice.String()
}

// operation is terraform's OperationTypeApply as English: "an apply". Printed
// straight, it produced "from a Apply", which is the kind of seam that makes a
// message read as generated rather than written.
func (l StateLock) operation() string {
	name := strings.ToLower(strings.TrimPrefix(l.Operation, "OperationType"))
	switch name {
	case "":
		return ""
	case "apply":
		return "an apply"
	default:
		return "a " + name
	}
}

// Holder is who has it and for how long, in one line — the two facts the
// decision turns on.
func (l StateLock) Holder(now time.Time) string {
	who := l.Who
	if who == "" {
		who = "another run"
	}
	if operation := l.operation(); operation != "" {
		who += ", from " + operation
	}
	if age := l.age(now); age != "" {
		who += ", " + age
	}
	return who
}

// age is how long ago the lock was taken, in words, or empty when terraform did
// not say or the clock disagrees.
func (l StateLock) age(now time.Time) string {
	if l.Created.IsZero() {
		return ""
	}
	elapsed := now.Sub(l.Created)
	switch {
	case elapsed < 0:
		return ""
	case elapsed < time.Minute:
		return "seconds ago"
	case elapsed < time.Hour:
		return fmt.Sprintf("%d minute(s) ago", int(elapsed.Minutes()))
	default:
		return fmt.Sprintf("%.1f hour(s) ago", elapsed.Hours())
	}
}

// ForceUnlock releases a held lock on a root's state.
//
// Offered rather than done quietly, and never without being asked: this is the
// one terraform command whose damage is invisible at the moment it runs. If the
// holder is alive, nothing fails here — two writers simply proceed, and the
// state that loses the race is gone.
//
// -force because the confirmation has already happened, in a prompt that said
// who holds the lock and how old it is. Terraform's own y/n would be a second
// question about a decision already taken, and one that cannot be answered when
// this is not attached to a terminal.
func (c *CLI) ForceUnlock(ctx context.Context, unit Unit, lockID string) error {
	if strings.TrimSpace(lockID) == "" || strings.HasPrefix(lockID, "-") {
		return fmt.Errorf("infra: %q is not a lock id", lockID)
	}

	// #nosec G204 -- the binary is the one this CLI resolved and verified, every
	// flag is a literal, and the id is checked above for the only thing argv is
	// vulnerable to: a value that parses as an option. No shell is involved.
	command := exec.CommandContext(ctx, c.execPath, "force-unlock", "-force", lockID)
	command.Dir = unit.Dir

	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("infra: terraform force-unlock failed in %s: %w\n%s",
			unit.Name, err, strings.TrimSpace(string(output)))
	}
	return nil
}
