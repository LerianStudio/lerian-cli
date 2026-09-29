package infracli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// wordmark is the block form, 46 columns wide and 6 lines tall.
var wordmark = []string{
	`██╗     ███████╗██████╗ ██╗ █████╗ ███╗   ██╗`,
	`██║     ██╔════╝██╔══██╗██║██╔══██╗████╗  ██║`,
	`██║     █████╗  ██████╔╝██║███████║██╔██╗ ██║`,
	`██║     ██╔══╝  ██╔══██╗██║██╔══██║██║╚██╗██║`,
	`███████╗███████╗██║  ██║██║██║  ██║██║ ╚████║`,
	`╚══════╝╚══════╝╚═╝  ╚═╝╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝`,
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

// revealStep is how long each line of the wordmark waits before the next one is
// drawn. Six lines at this rate is under a fifth of a second: long enough to read
// as an entrance, short enough that nobody waits for it twice.
const revealStep = 30 * time.Millisecond

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
	subtitle := "  lerian-cli · " + describeRelease(release)

	if width > 0 && width < wordmarkWidth() {
		return "\n  ◤ " + strings.TrimSpace(subtitle) + "\n\n"
	}

	margin := strings.Repeat(" ", wordmarkIndent)

	var banner strings.Builder
	banner.WriteString("\n")
	for _, line := range wordmark {
		banner.WriteString(margin + line + "\n")
	}
	banner.WriteString("\n" + subtitle + "\n\n")
	return banner.String()
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

// revealBanner draws the banner a line at a time.
//
// Every line is flushed as it is written, because the pause between them is the
// whole effect: buffered, the six lines arrive together after 180ms of nothing,
// which is not an entrance but a delay.
func revealBanner(out io.Writer, banner string, step time.Duration) {
	lines := strings.Split(banner, "\n")
	for index, line := range lines {
		if index == len(lines)-1 {
			fmt.Fprint(out, line)
			break
		}
		fmt.Fprintln(out, line)
		if strings.TrimSpace(line) != "" {
			time.Sleep(step)
		}
	}
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
