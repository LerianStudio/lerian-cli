package infracli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// wordmark is the block form of the tool's name. Six lines, 71 columns, which
// fits a standard 80-column terminal with room to spare and gives way below that.
var wordmark = []string{
	`██╗     ███████╗██████╗ ██╗ █████╗ ███╗   ██╗        ██████╗██╗     ██╗`,
	`██║     ██╔════╝██╔══██╗██║██╔══██╗████╗  ██║       ██╔════╝██║     ██║`,
	`██║     █████╗  ██████╔╝██║███████║██╔██╗ ██║ █████╗██║     ██║     ██║`,
	`██║     ██╔══╝  ██╔══██╗██║██╔══██║██║╚██╗██║ ╚════╝██║     ██║     ██║`,
	`███████╗███████╗██║  ██║██║██║  ██║██║ ╚████║       ╚██████╗███████╗██║`,
	`╚══════╝╚══════╝╚═╝  ╚═╝╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝        ╚═════╝╚══════╝╚═╝`,
}

// wordmarkIndent is the left margin every line of the banner shares.
const wordmarkIndent = 2

// wordmarkWidth is measured rather than written down: a hand-counted constant
// that drifts from the art it describes cuts the wordmark off on exactly the
// widths it was meant to protect.
func wordmarkWidth() int {
	widest := 0
	for _, line := range wordmark {
		if w := displayWidth(line); w > widest {
			widest = w
		}
	}
	return widest + wordmarkIndent
}

// revealStep is how long each line of the wordmark holds before the next one
// arrives. Six lines at this rate, plus the rule drawing itself, is a fifth of a
// second: long enough to read as an entrance, short enough that nobody waits for
// it twice.
const revealStep = 25 * time.Millisecond

// Banner paints the wordmark, once, at the top of an interactive session.
//
// Nothing at all outside a terminal. The banner is decoration, and decoration in
// a pipe is damage: `lerian ... | grep` and a CI log both read every one of those
// lines as output, and the six that draw an L are six the reader has to skip.
//
// LERIAN_NO_BANNER suppresses it for anyone who has seen it enough times.
func Banner(out io.Writer, release string) {
	if !writerIsTerminal(out) {
		return
	}
	if _, quiet := os.LookupEnv("LERIAN_NO_BANNER"); quiet {
		return
	}

	painted := renderBanner(release, screenWidth(out))
	if animates(out) {
		revealBanner(out, painted, revealStep)
		return
	}
	fmt.Fprint(out, painted)
}

// renderBanner is the banner as text: the block wordmark where it fits, one line
// where it does not.
//
// A wrapped wordmark is not a wordmark, it is six broken lines, and the terminal
// decides where it wraps — so the narrow form is not a degraded banner, it is the
// correct one at that width.
func renderBanner(release string, width int) string {
	// The wordmark says the name, so the line under it only has the release left
	// to say. The narrow form has no wordmark, so it says both.
	if width > 0 && width < wordmarkWidth() {
		return "\n" + narrowBanner(release, width) + "\n"
	}
	subtitle := ruleWithRelease(describeRelease(release))

	margin := strings.Repeat(" ", wordmarkIndent)

	var banner strings.Builder
	banner.WriteString("\n")
	for _, line := range wordmark {
		banner.WriteString(margin + line + "\n")
	}
	banner.WriteString("\n" + subtitle + "\n\n")
	return banner.String()
}

// ruleWithRelease is the line under the wordmark: a rule the width of the art,
// with the release sitting at the end of it.
//
// Part of the banner rather than part of the animation. A flourish that only
// exists while it is being drawn is one nobody can screenshot, and one the static
// path — dumb terminals, NO_COLOR — never gets.
func ruleWithRelease(release string) string {
	rule := wordmarkWidth() - wordmarkIndent - displayWidth(release) - 2
	if rule < 1 {
		return "  " + release
	}
	return "  " + strings.Repeat("─", rule) + "  " + release
}

// narrowBanner is the one-line form, cut to the width it is given.
//
// It exists to avoid a wrapped wordmark, so a wrapped version of itself is the
// one thing it must not be: "◤ lerian-cli · development build" is 34 columns, and
// a 20-column terminal would fold it in half.
//
// It sheds in order — the release first, then the marker — because the name is
// the part worth keeping. Below even that, nothing: a fragment of a name is worse
// than a blank line where a banner would have been.
func narrowBanner(release string, width int) string {
	const marker = "  ◤ "

	full := marker + "lerian-cli · " + describeRelease(release)
	if displayWidth(full) <= width {
		return full
	}

	withMarker := marker + "lerian-cli"
	if displayWidth(withMarker) <= width {
		return withMarker
	}

	bare := "  lerian-cli"
	if displayWidth(bare) <= width {
		return bare
	}
	return ""
}

// describeRelease names the build. "dev" is what the version variable holds until
// a release stamps it, and printing it bare next to a separator reads as a
// version that failed to load rather than a build from somebody's machine.
func describeRelease(release string) string {
	switch release {
	case "", "dev":
		return "development build"
	default:
		return release
	}
}

// revealBanner draws the banner a line at a time, lighting each one as it
// arrives.
//
// Every write is flushed as it happens, because the pauses are the whole effect:
// buffered, the six lines arrive together after a fifth of a second of nothing,
// which is not an entrance but a delay.
//
// Where it ends is exactly the static banner — the animation is a way of arriving
// at it, not a second version of it. A test paints this onto a small terminal and
// compares the screen.
func revealBanner(out io.Writer, banner string, step time.Duration) {
	lines := strings.Split(banner, "\n")

	for index, line := range lines {
		// The split leaves a final empty element for the trailing newline. Writing
		// it as a line would add one the static banner does not have.
		if index == len(lines)-1 {
			fmt.Fprint(out, line)
			break
		}

		switch {
		case strings.Contains(line, "─"):
			drawAcross(out, line, step)
		case strings.TrimSpace(line) == "":
			fmt.Fprintln(out, line)
		default:
			glow(out, line, step)
		}
	}
}

// glow writes a line bright, holds it, and lets it settle as the next one
// arrives. Six lines of it reads as a wave running down the wordmark.
//
// The settled line is rewritten over the bright one from the start of the row,
// so what remains is the plain text — no escape sequence outlives the animation.
func glow(out io.Writer, line string, step time.Duration) {
	fmt.Fprint(out, "\x1b[1m"+line+"\x1b[0m")
	time.Sleep(step)
	fmt.Fprint(out, "\r"+line+"\n")
}

// drawAcross writes a line a few characters at a time, left to right, which draws
// the rule and types the release at the end of it.
func drawAcross(out io.Writer, line string, step time.Duration) {
	const chunk = 6

	runes := []rune(line)
	for start := 0; start < len(runes); start += chunk {
		end := start + chunk
		if end > len(runes) {
			end = len(runes)
		}
		fmt.Fprint(out, string(runes[start:end]))
		time.Sleep(step / chunk)
	}
	fmt.Fprint(out, "\n")
}

// animates reports whether this terminal should get the reveal.
//
// The same three conditions that gate color, because they are asking the same
// question: is there somebody watching this happen. A dumb terminal is an
// editor's shell, NO_COLOR is a stated preference for output without ornament,
// and neither wants six sleeps in front of the menu.
func animates(out io.Writer) bool {
	return newStyle(out).enabled
}
