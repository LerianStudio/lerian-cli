package infra

import (
	"fmt"
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
		if l.Operation != "" {
			fmt.Fprintf(&advice, ", from a %s", strings.TrimPrefix(l.Operation, "OperationType"))
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
