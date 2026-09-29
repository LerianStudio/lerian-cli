package version

import (
	"fmt"
	"strings"
)

// field is one row of the version output, in the order it is printed.
type field struct {
	label string
	value func(Info) string
}

var fields = []field{
	{"version", func(i Info) string { return i.Version }},
	{"commit", func(i Info) string { return i.Commit }},
	{"built", func(i Info) string { return i.Date + " by " + i.BuiltBy }},
	{"go", func(i Info) string { return i.GoVersion + " · " + i.Platform }},
}

// Render lays the version out in two columns.
//
// String is left alone: it is the `key: value` form, and something somewhere is
// parsing it. This is the one for a person to read, and styled decides whether it
// may carry escape sequences — a version redirected into a file or a bug report
// must not.
func (i Info) Render(styled bool) string {
	width := 0
	for _, f := range fields {
		if len(f.label) > width {
			width = len(f.label)
		}
	}

	var out strings.Builder
	out.WriteString("\n")
	for _, f := range fields {
		label := fmt.Sprintf("%-*s", width, f.label)
		if styled {
			label = "\x1b[2m" + label + "\x1b[0m"
		}
		fmt.Fprintf(&out, "  %s  %s\n", label, f.value(i))
	}
	out.WriteString("\n")
	return out.String()
}
