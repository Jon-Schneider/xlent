package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jon-Schneider/xl/internal/engine"
)

// refSpan is one cell/range reference found in formula text being edited:
// where it sits in the text (rune indexes) and what it points at.
type refSpan struct {
	start, end int // [start, end) rune span
	ref        engine.Ref
}

// refPalette pairs a formula-bar foreground with a grid-tint background, so
// the colored reference text and the cells it targets visibly match. Spans
// cycle through it in order, like Excel's colored formula references.
var refPalette = []struct {
	text lipgloss.Style
	cell lipgloss.Style
}{
	{lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("75")),
		lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("231"))},
	{lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("114")),
		lipgloss.NewStyle().Background(lipgloss.Color("22")).Foreground(lipgloss.Color("231"))},
	{lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("176")),
		lipgloss.NewStyle().Background(lipgloss.Color("53")).Foreground(lipgloss.Color("231"))},
	{lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("215")),
		lipgloss.NewStyle().Background(lipgloss.Color("94")).Foreground(lipgloss.Color("231"))},
}

// formulaRefSpans returns the references in the formula currently being
// edited, or nil when no formula edit is active. The editor text is
// un-normalized (lowercase names, maybe unbalanced), so this scans by hand
// rather than tokenizing with efp — efp tokens carry no source offsets.
func (a *App) formulaRefSpans() []refSpan {
	if !a.editor.active || len(a.editor.text) == 0 || a.editor.text[0] != '=' {
		return nil
	}
	return scanFormulaRefs(a.editor.text, a.editOrigin.sheet)
}

// scanFormulaRefs walks formula text, skipping string literals, collecting
// runs of reference-ish characters (including quoted sheet qualifiers) and
// keeping the ones engine.ParseRef accepts. Bare words like SUM or TRUE
// don't parse as cell references and fall out naturally.
func scanFormulaRefs(text []rune, defaultSheet string) []refSpan {
	var spans []refSpan
	i := 1 // skip the leading =
	for i < len(text) {
		switch {
		case text[i] == '"':
			// String literal: skip to its closing quote ("" escapes resolve
			// themselves — each quote just toggles once more).
			for i++; i < len(text) && text[i] != '"'; i++ {
			}
			i++

		case isRefRune(text[i]) || text[i] == '\'':
			j := i
			for j < len(text) {
				if text[j] == '\'' {
					k := j + 1
					for k < len(text) && text[k] != '\'' {
						k++
					}
					if k >= len(text) {
						break // unterminated quoted sheet name
					}
					j = k + 1
					continue
				}
				if !isRefRune(text[j]) {
					break
				}
				j++
			}
			if ref, err := engine.ParseRef(defaultSheet, string(text[i:j])); err == nil {
				spans = append(spans, refSpan{start: i, end: j, ref: ref})
			}
			i = max(j, i+1)

		default:
			i++
		}
	}
	return spans
}

// isRefRune reports whether r can appear in an unquoted reference token.
func isRefRune(r rune) bool {
	return r == '$' || r == ':' || r == '!' || r == '_' ||
		(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

// renderHighlightedFormula renders editor text with each reference span in
// its palette color, for the formula bar.
func renderHighlightedFormula(text []rune, spans []refSpan) string {
	var b strings.Builder
	pos := 0
	for i, s := range spans {
		if s.start > pos {
			b.WriteString(styleFormulaBar.Render(string(text[pos:s.start])))
		}
		b.WriteString(refPalette[i%len(refPalette)].text.Render(string(text[s.start:s.end])))
		pos = s.end
	}
	if pos < len(text) {
		b.WriteString(styleFormulaBar.Render(string(text[pos:])))
	}
	return b.String()
}

// refTintAt returns the grid tint for a cell covered by one of the formula's
// references on the displayed sheet, matching the formula bar's colors.
func refTintAt(spans []refSpan, sheet string, p position) (lipgloss.Style, bool) {
	for i, s := range spans {
		if strings.EqualFold(s.ref.Sheet, sheet) &&
			p.Col >= s.ref.MinCol && p.Col <= s.ref.MaxCol &&
			p.Row >= s.ref.MinRow && p.Row <= s.ref.MaxRow {
			return refPalette[i%len(refPalette)].cell, true
		}
	}
	return lipgloss.Style{}, false
}
