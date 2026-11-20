package report

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/keelinfra/jenkins-bench/internal/console"
)

// The visual grammar of the terminal report, in one place.
//
// The report is a stack of sections. Each opens with a heading — its name,
// a hairline rule out to the page width, and a count at the right edge — and
// everything under a heading sits two columns in, so the eye can run down the
// left margin and find the next section without reading a word. Inside a
// section a record leads with a fixed-width label (a severity or a state), so
// the labels form a column of their own and the control IDs line up beside
// them. None of it depends on colour: the rules, the margin and the label
// column carry the structure in a pipe, a CI log and a file, and colour only
// reinforces it on a terminal.

// margin is the left margin of everything under a section heading.
const margin = "  "

// labelWidth fits the widest record label: MEDIUM and MANUAL.
const labelWidth = 6

// bodyIndent is where a record's text starts: margin, label column, gap.
var bodyIndent = strings.Repeat(" ", len(margin)+labelWidth+2)

// keyWidth is the width of the key column inside a record — "fix", "·" —
// including the gap after it.
const keyWidth = 5

// ruleChar draws section rules. barFull and barEmpty draw the score bar: a
// solid block and a light shade, which every monospace font a terminal ships
// with has, and which differ in density rather than in colour, so the bar
// still reads with colour off.
const (
	ruleChar = "─"
	barFull  = "█"
	barEmpty = "░"
)

// heading writes a section heading:
//
//	Failed ─────────────────────────────────────── 6 findings in 5 controls
//
// The title is bold in the given colour, the rule and the count dim. With no
// room for a rule of at least three characters the count is dropped, and then
// the rule; the title alone is never wider than the narrowest width allowed.
func heading(w io.Writer, p painter, width int, colour, title, meta string) {
	blank(w)
	tw, mw := runeWidth(title), runeWidth(meta)
	fill := width - tw - 2
	if meta != "" {
		fill -= mw + 1
	}
	if fill < 3 && meta != "" {
		meta, mw = "", 0
		fill = width - tw - 2
	}
	var b strings.Builder
	b.WriteString(p.paint(ansiBold+colour, title))
	if fill >= 3 {
		b.WriteString(" ")
		b.WriteString(p.paint(ansiDim, strings.Repeat(ruleChar, fill)))
		if meta != "" {
			b.WriteString(" ")
			b.WriteString(p.paint(ansiDim, meta))
		}
	}
	line(w, "%s", b.String())
}

// record writes the first line of a record — label, control ID, subject; the
// body follows through recordText and recordKey. The label is padded before it
// is painted, so its colour never leaks into the gap.
//
// The three stay on one line wherever they fit, so a grep for a job name
// returns how bad and which control along with it. A subject too long to fit
// beside the ID — a deeply foldered job — moves to its own line at the body
// indent rather than running past the edge.
func record(w io.Writer, p painter, width int, label string, labelColour func(string) string, id, subject string) {
	pad := strings.Repeat(" ", max(labelWidth-runeWidth(label), 0))
	head := margin + labelColour(label) + pad + "  " + p.paint(ansiCyan, id)
	if len(bodyIndent)+runeWidth(id)+2+runeWidth(subject) <= width {
		line(w, "%s  %s", head, p.paint(ansiBold, subject))
		return
	}
	line(w, "%s", head)
	line(w, "%s%s", bodyIndent, p.paint(ansiBold, subject))
}

// recordText writes wrapped prose at the record's body indent.
func recordText(w io.Writer, p painter, width int, colour, text string) {
	for _, l := range console.Wrap(text, width-len(bodyIndent)) {
		line(w, "%s%s", bodyIndent, p.paint(colour, l))
	}
}

// recordKey writes a keyed line inside a record — "fix  <text>" — with the
// text's continuations aligned under its first word.
func recordKey(w io.Writer, p painter, width int, key, keyColour, text, textColour string) {
	pad := strings.Repeat(" ", keyWidth-runeWidth(key))
	hang := bodyIndent + strings.Repeat(" ", keyWidth)
	for i, l := range console.Wrap(text, width-len(hang)) {
		if i == 0 {
			line(w, "%s%s%s%s", bodyIndent, p.paint(keyColour, key), pad, p.paint(textColour, l))
			continue
		}
		line(w, "%s%s", hang, p.paint(textColour, l))
	}
}

// indented writes wrapped prose at the section margin.
func indented(w io.Writer, p painter, width int, colour, text string) {
	for _, l := range console.Wrap(text, width-len(margin)) {
		line(w, "%s%s", margin, p.paint(colour, l))
	}
}

// scoreBar draws the score as a bar of n cells: solid in the score's own
// colour up to the value, a dim shade after it.
func scoreBar(p painter, colour string, value, n int) string {
	value = min(max(value, 0), 100)
	filled := value * n / 100
	return p.paint(colour, strings.Repeat(barFull, filled)) +
		p.paint(ansiDim, strings.Repeat(barEmpty, n-filled))
}

// brand renders the tool's name: a navy-on-amber badge on a terminal, the
// bare name everywhere else. The badge's padding is part of the badge, so
// without colour there is none to account for.
func brand(p painter) string {
	if !p.enabled {
		return "jenkins-bench"
	}
	return console.Brand() + " jenkins-bench " + ansiReset
}

// brandWidth is how many columns brand(p) occupies.
func brandWidth(p painter) int {
	if !p.enabled {
		return len("jenkins-bench")
	}
	return len(" jenkins-bench ")
}

func runeWidth(s string) int { return utf8.RuneCountInString(s) }

// countsLine is the four state counts, each in its own colour, separated by
// a dim dot.
func countsLine(p painter, passed, failed, manual, na int) (painted, plain string) {
	parts := []struct {
		colour, text string
	}{
		{ansiGreen, fmt.Sprintf("%d passed", passed)},
		{ansiRed, fmt.Sprintf("%d failed", failed)},
		{ansiYellow, fmt.Sprintf("%d manual", manual)},
		{ansiDim, fmt.Sprintf("%d n/a", na)},
	}
	var pp, pl []string
	for _, part := range parts {
		pp = append(pp, p.paint(part.colour, part.text))
		pl = append(pl, part.text)
	}
	return strings.Join(pp, p.paint(ansiDim, " · ")), strings.Join(pl, " · ")
}
