package config

import (
	"fmt"
	"os"
	"strings"
)

// Reset removes this tool's configuration, and returns what it removed.
//
// The point is a machine that has never run the CLI: the next invocation asks
// what it asked the first time — where the templates are, which account to deploy
// into — instead of reading an answer given months ago that may no longer be
// true.
//
// It takes the config file and nothing else. ~/.aws belongs to the AWS CLI and is
// read by every tool on the machine; a templates checkout is a git clone somebody
// made, possibly with work in it. Neither is this command's to delete, and a
// "reset" that took them would be an expensive surprise.
//
// A machine with nothing to remove is not an error: that is the state this
// produces.
func Reset() ([]string, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("cannot remove %s: %w", path, err)
	}
	return []string{path}, nil
}

// Describe is what this tool currently believes about the machine, in the order
// somebody would ask: where the answers live, then what they are.
func Describe() (string, error) {
	path, err := GetConfigPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Sprintf("  file     %s\n  state    nothing configured yet\n", path), nil
	}

	var out strings.Builder
	fmt.Fprintf(&out, "  file      %s\n", path)

	// A file too damaged to parse is the one somebody most needs to reset, so this
	// describes it rather than failing on it: the path is the part that matters,
	// and "cannot be read" is a truthful description of what is there.
	cfg, err := Load()
	if err != nil {
		fmt.Fprintf(&out, "  state     cannot be read: %v\n", err)
		return out.String(), nil
	}

	checkout := cfg.TemplatesCheckout
	if checkout == "" {
		checkout = "not recorded — discovered on each run"
	}
	fmt.Fprintf(&out, "  templates %s\n", checkout)

	profile := cfg.CurrentProfile
	if profile == "" {
		profile = "none"
	}
	fmt.Fprintf(&out, "  profile   %s\n", profile)

	// Named, never printed: a profile holds an API key.
	if len(cfg.Profiles) == 0 {
		fmt.Fprintf(&out, "  accounts  none — lerian auth login creates one\n")
		return out.String(), nil
	}
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	fmt.Fprintf(&out, "  accounts  %s\n", strings.Join(names, ", "))
	return out.String(), nil
}
