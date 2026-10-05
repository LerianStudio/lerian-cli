package infracli

import (
	"strings"
)

// mascot is the wizard, at the size a terminal banner can carry: six lines, so it
// stands beside the wordmark rather than above it, and eleven columns, which is
// what is left of an 89-column terminal once the wordmark has had its 73.
//
// Drawn in the same half-block alphabet as the wordmark so the two read as one
// piece of art rather than as a logo with a sticker next to it. From the top: the
// point of the hat, its crown, its brim, the face under the brim, and the robe —
// with the staff down the left, its orb level with the eyes.
var mascot = []string{
	`    ▗▄▖    `,
	`   ▟███▙   `,
	` ▗▟█████▙▖ `,
	` ▀▀▜███▛▀▀ `,
	`◉   ◕‿◕    `,
	`╎  ╱▇▇▇╲   `,
}

// mascotGap is the space between the wordmark and the mascot.
const mascotGap = 2

// mascotWidth is measured from the art, for the same reason wordmarkWidth is: a
// hand-counted constant that drifts cuts the thing it was meant to protect.
func mascotWidth() int {
	widest := 0
	for _, line := range mascot {
		if w := displayWidth(line); w > widest {
			widest = w
		}
	}
	return widest
}

// bannerWidthWithMascot is what the two together need.
func bannerWidthWithMascot() int { return wordmarkWidth() + mascotGap + mascotWidth() }

// mascotColors paints the art by character.
//
// By character rather than by region because the alphabet already separates them:
// the hat is the only part drawn in full blocks, the robe the only part in ▇ and
// slashes, and the eyes, mouth, orb and staff are one rune each. A region map
// would be a second description of the same thing, free to disagree with it.
var mascotColors = map[rune]string{
	'▗': "\x1b[38;5;99m", '▄': "\x1b[38;5;99m", '▖': "\x1b[38;5;99m",
	'▟': "\x1b[38;5;99m", '█': "\x1b[38;5;99m", '▙': "\x1b[38;5;99m",
	'▀': "\x1b[38;5;141m", '▜': "\x1b[38;5;141m", '▛': "\x1b[38;5;141m",
	'◕': "\x1b[38;5;226m", // eyes
	'‿': "\x1b[38;5;120m", // mouth
	'◉': "\x1b[38;5;51m",  // the orb on the staff
	'╎': "\x1b[38;5;245m", // the staff
	'▇': "\x1b[38;5;61m", '╱': "\x1b[38;5;61m", '╲': "\x1b[38;5;61m",
}

// paintMascot colors one line of the art, or returns it unchanged where color is
// not wanted — a dumb terminal, NO_COLOR, a pipe.
//
// Runs of one color share an escape sequence. Not for the bytes: a per-character
// reset between two halves of the same block makes some terminals draw a seam
// down the middle of a solid shape.
func paintMascot(line string, colored bool) string {
	if !colored {
		return line
	}

	var out strings.Builder
	current := ""
	for _, r := range line {
		want := mascotColors[r]
		if want != current {
			if current != "" {
				out.WriteString("\x1b[0m")
			}
			out.WriteString(want)
			current = want
		}
		out.WriteRune(r)
	}
	if current != "" {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// besideWordmark lays the mascot out to the right of a wordmark line.
//
// The wordmark keeps its own left margin and its own width, so the mascot's
// column does not move as the lines of the wordmark change length — a mascot that
// slid left and right down the six rows would read as a rendering fault.
func besideWordmark(wordmarkLine, mascotLine string, colored bool) string {
	padding := wordmarkWidth() - wordmarkIndent - displayWidth(wordmarkLine)
	if padding < 0 {
		padding = 0
	}
	return wordmarkLine + strings.Repeat(" ", padding+mascotGap) + paintMascot(mascotLine, colored)
}

// bannerLine composes one row of the wide banner from the art, rather than from
// a rendered string.
//
// Composing instead of slicing is what makes the animation safe: a highlight that
// cut a finished line by rune count would cut through the mascot's color escapes,
// and half an escape sequence on screen is a line of garbage that survives until
// the next full redraw.
func bannerLine(index int, withMascot, colored bool, highlight func(string) string) string {
	line := wordmark[index]
	if highlight != nil {
		line = highlight(line)
	}
	if !withMascot {
		return line
	}
	return besideWordmark(line, mascot[index], colored)
}

// litWindow brightens a run of columns and leaves the rest alone. It is the
// moving part of the sweep.
func litWindow(start, width int) func(string) string {
	return func(line string) string {
		runes := []rune(line)
		from, to := start, start+width
		if from < 0 {
			from = 0
		}
		if to > len(runes) {
			to = len(runes)
		}
		if from >= to {
			return line
		}
		return string(runes[:from]) + "\x1b[1;97m" + string(runes[from:to]) + "\x1b[0m" + string(runes[to:])
	}
}
