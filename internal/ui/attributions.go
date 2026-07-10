package ui

import (
	"embed"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

//go:embed licenses/*.txt
var licenseFS embed.FS

// attribution is a single third-party dependency credit: the module import
// path, the SPDX-style license identifier it ships under (which also names
// the embedded license file), and the upstream copyright line.
type attribution struct {
	module    string
	licenseID string
	copyright string
}

// thirdPartyAttributions lists every module in xlent's resolved dependency
// graph, including dependencies used only by tests. Keep this in sync with
// go.mod when dependencies change; licenseID must match a file in licenses/.
var thirdPartyAttributions = []attribution{
	{"charm.land/bubbletea/v2", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"charm.land/lipgloss/v2", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/aymanbagabas/go-udiff", "BSD-3-Clause OR MIT", "Copyright (c) 2009 The Go Authors\nCopyright (c) 2023 Ayman Bagabas"},
	{"github.com/bits-and-blooms/bitset", "BSD-3-Clause", "Copyright (c) 2014 Will Fitzgerald"},
	{"github.com/charmbracelet/x/ansi", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/exp/golden", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/term", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/termios", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/windows", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/charmbracelet/colorprofile", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/charmbracelet/ultraviolet", "MIT", "Copyright (c) Charmbracelet, Inc."},
	{"github.com/xuri/excelize/v2", "BSD-3-Clause", "Copyright (c) 2016-2025 The excelize Authors"},
	{"github.com/xuri/efp", "BSD-3-Clause", "Copyright (c) 2017-2024 xuri"},
	{"github.com/xuri/nfp", "BSD-3-Clause", "Copyright (c) 2021-2024 xuri"},
	{"github.com/clipperhouse/displaywidth", "MIT", "Copyright (c) Clipperhouse"},
	{"github.com/clipperhouse/stringish", "MIT", "Copyright (c) Matt Sherman"},
	{"github.com/clipperhouse/uax29/v2", "MIT", "Copyright (c) Clipperhouse"},
	{"github.com/davecgh/go-spew", "ISC", "Copyright (c) 2012-2016 Dave Collins <dave@davec.name>"},
	{"github.com/lucasb-eyer/go-colorful", "MIT", "Copyright (c) 2013 Lucas Beyer"},
	{"github.com/mattn/go-runewidth", "MIT", "Copyright (c) 2016 Yasuhiro Matsumoto"},
	{"github.com/muesli/cancelreader", "MIT", "Copyright (c) 2022 Erik Geiser and Christian Muehlhaeuser"},
	{"github.com/pmezard/go-difflib", "BSD-3-Clause", "Copyright (c) 2013 Patrick Mezard"},
	{"github.com/richardlehane/mscfb", "Apache-2.0", "Copyright (c) 2014 Richard Lehane"},
	{"github.com/richardlehane/msoleps", "Apache-2.0", "Copyright (c) 2015 Richard Lehane"},
	{"github.com/rivo/uniseg", "MIT", "Copyright (c) 2019 Oliver Kuederle"},
	{"github.com/stretchr/testify", "MIT", "Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors"},
	{"github.com/tiendc/go-deepcopy", "MIT", "Copyright (c) 2024 Tien Dang"},
	{"github.com/xo/terminfo", "MIT", "Copyright (c) 2019 Kenneth Shaw"},
	{"golang.org/x/crypto", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/exp", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/image", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/mod", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/net", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/sync", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/sys", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/term", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/text", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"golang.org/x/tools", "BSD-3-Clause", "Copyright (c) 2009 The Go Authors"},
	{"gopkg.in/yaml.v3", "MIT AND Apache-2.0", "Copyright (c) 2006-2011 Kirill Simonov\nCopyright (c) 2011-2019 Canonical Ltd"},
}

// licenseText returns the full license for an entry, with the per-module
// copyright spliced into the embedded template. Modules that distribute code
// under more than one license show each full license. Missing files degrade to
// a short note rather than panicking.
func (a attribution) licenseText() string {
	licenseIDs := strings.NewReplacer(" OR ", "|", " AND ", "|").Replace(a.licenseID)
	var texts []string
	for _, licenseID := range strings.Split(licenseIDs, "|") {
		raw, err := licenseFS.ReadFile("licenses/" + licenseID + ".txt")
		if err != nil {
			texts = append(texts, a.copyright+"\n\nLicensed under "+licenseID+".")
			continue
		}
		texts = append(texts, strings.ReplaceAll(string(raw), "{{COPYRIGHT}}", a.copyright))
	}
	return strings.Join(texts, "\n\n")
}

// attributionsOverlay is the Help ▸ Attributions panel state. It has two
// modes: a selectable list of modules, and a detail view showing the full
// license text for the highlighted module.
type attributionsOverlay struct {
	open     bool
	selected int // highlighted module in list mode
	listTop  int // first visible module (list scroll offset)

	detail       bool // showing full license text for selected
	detailScroll int

	// Geometry captured during the last render, for mouse hit-testing.
	rowY0 int // screen y of the first visible module row
	rowsN int // number of module rows currently drawn
	boxX  int // left edge of the panel box
	boxW  int // panel box width
	boxY  int // top edge of the panel box
	boxH  int // panel box height
}

func (o *attributionsOverlay) openPanel() {
	*o = attributionsOverlay{open: true}
}

func (o *attributionsOverlay) close() {
	*o = attributionsOverlay{}
}

// openDetail switches the panel into license-text mode for the given module.
func (o *attributionsOverlay) openDetail(i int) {
	o.selected = i
	o.detail = true
	o.detailScroll = 0
}

// handleAttributionsKey routes keys while the panel is open. In list mode the
// arrows move the highlight and Enter opens the license; in detail mode the
// arrows scroll the text and Esc returns to the list.
func (a *App) handleAttributionsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.attributions.detail {
		switch msg.String() {
		case "esc", "backspace", "left", "h", "q":
			a.attributions.detail = false
		case "down", "j":
			a.attributions.detailScroll++
		case "up", "k":
			a.attributions.detailScroll--
		case "pgdown", " ":
			a.attributions.detailScroll += a.attributionsViewportHeight()
		case "pgup":
			a.attributions.detailScroll -= a.attributionsViewportHeight()
		case "home", "g":
			a.attributions.detailScroll = 0
		}
		return a, nil
	}

	switch msg.String() {
	case "esc", "q":
		a.attributions.close()
	case "enter", "right", "l", " ":
		a.attributions.openDetail(a.attributions.selected)
	case "down", "j":
		a.attributions.selected++
	case "up", "k":
		a.attributions.selected--
	case "pgdown":
		a.attributions.selected += a.attributionsViewportHeight()
	case "pgup":
		a.attributions.selected -= a.attributionsViewportHeight()
	case "home", "g":
		a.attributions.selected = 0
	case "end", "G":
		a.attributions.selected = len(thirdPartyAttributions) - 1
	}
	a.clampAttributionsSelection()
	return a, nil
}

// handleAttributionsMouse processes clicks while the panel is open: a click on
// a module row in list mode opens its license; a click outside the box closes
// the panel (in detail mode it steps back to the list).
func (a *App) handleAttributionsMouse(m tea.Mouse) {
	inBox := m.X >= a.attributions.boxX && m.X < a.attributions.boxX+a.attributions.boxW &&
		m.Y >= a.attributions.boxY && m.Y < a.attributions.boxY+a.attributions.boxH

	if a.attributions.detail {
		if !inBox {
			a.attributions.detail = false
		}
		return
	}
	if !inBox {
		a.attributions.close()
		return
	}
	row := m.Y - a.attributions.rowY0
	if row >= 0 && row < a.attributions.rowsN {
		a.attributions.openDetail(a.attributions.listTop + row)
	}
}

// scrollAttributionsWheel moves the highlight (list mode) or the license text
// (detail mode) by one notch per wheel event.
func (a *App) scrollAttributionsWheel(m tea.Mouse) {
	down := m.Button == tea.MouseWheelDown
	switch {
	case a.attributions.detail && down:
		a.attributions.detailScroll++
	case a.attributions.detail:
		a.attributions.detailScroll--
	case down:
		a.attributions.selected++
		a.clampAttributionsSelection()
	default:
		a.attributions.selected--
		a.clampAttributionsSelection()
	}
}

// attributionsViewportHeight is how many body lines fit in the panel for the
// current terminal height, leaving room for the border, title, and footer.
func (a *App) attributionsViewportHeight() int {
	h := a.height - 8 // top/bottom margin, border, title, footer
	if h < 3 {
		h = 3
	}
	return h
}

func (a *App) clampAttributionsSelection() {
	if a.attributions.selected < 0 {
		a.attributions.selected = 0
	}
	if max := len(thirdPartyAttributions) - 1; a.attributions.selected > max {
		a.attributions.selected = max
	}
	// Keep the highlight inside the visible window.
	vh := a.attributionsViewportHeight()
	if a.attributions.selected < a.attributions.listTop {
		a.attributions.listTop = a.attributions.selected
	}
	if a.attributions.selected >= a.attributions.listTop+vh {
		a.attributions.listTop = a.attributions.selected - vh + 1
	}
	if a.attributions.listTop < 0 {
		a.attributions.listTop = 0
	}
}

var (
	styleAttributionsBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("24")).
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("252")).
				Padding(0, 2)

	styleAttributionsTitle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("231")).
				Bold(true)

	styleAttributionsFooter = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("245"))

	styleAttributionsRow = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("252"))

	styleAttributionsRowActive = lipgloss.NewStyle().
					Background(lipgloss.Color("24")).
					Foreground(lipgloss.Color("231")).
					Bold(true)
)

// overlayAttributions splices the panel, centered, on top of the rendered
// frame. It mirrors overlayDropdown's ANSI-safe approach and records the box
// geometry so clicks can be mapped back to rows.
func (a *App) overlayAttributions(content string) string {
	if !a.attributions.open {
		return content
	}

	boxWidth := 64
	if a.width-8 < boxWidth {
		boxWidth = a.width - 8
	}
	if boxWidth < 20 {
		boxWidth = 20
	}
	// lipgloss Width counts the border (1 cell each side) and padding (2 cells
	// each side), so content is generated to the narrower text width to avoid
	// the box re-wrapping our pre-formatted lines.
	textWidth := boxWidth - 6

	var rows []string
	if a.attributions.detail {
		rows = a.renderAttributionsDetail(textWidth)
	} else {
		rows = a.renderAttributionsList(textWidth)
	}

	box := strings.Split(styleAttributionsBox.Width(boxWidth).Render(strings.Join(rows, "\n")), "\n")

	bg := strings.Split(content, "\n")
	boxW := ansi.StringWidth(box[0])
	x := (a.width - boxW) / 2
	if x < 0 {
		x = 0
	}
	y := (len(bg) - len(box)) / 2
	if y < 0 {
		y = 0
	}

	a.attributions.boxX, a.attributions.boxW = x, boxW
	a.attributions.boxY, a.attributions.boxH = y, len(box)
	// Module rows begin after the top border, the title line, and one blank.
	a.attributions.rowY0 = y + 3

	for i, bl := range box {
		ly := y + i
		if ly < 0 || ly >= len(bg) {
			continue
		}
		line := bg[ly]
		left := ansi.Truncate(line, x, "")
		if short := x - ansi.StringWidth(left); short > 0 {
			left += strings.Repeat(" ", short)
		}
		bg[ly] = left + bl + ansi.TruncateLeft(line, x+boxW, "")
	}
	return strings.Join(bg, "\n")
}

// renderAttributionsList builds the selectable module list (title, rows for
// the visible window, footer).
func (a *App) renderAttributionsList(innerWidth int) []string {
	a.clampAttributionsSelection()

	vh := a.attributionsViewportHeight()
	end := a.attributions.listTop + vh
	if end > len(thirdPartyAttributions) {
		end = len(thirdPartyAttributions)
	}
	visible := thirdPartyAttributions[a.attributions.listTop:end]
	a.attributions.rowsN = len(visible)

	rows := []string{styleAttributionsTitle.Render(center("Third-Party Attributions", innerWidth)), ""}
	for i, entry := range visible {
		idx := a.attributions.listTop + i
		text := attributionRow(entry, innerWidth)
		if idx == a.attributions.selected {
			rows = append(rows, styleAttributionsRowActive.Render(padTo(text, innerWidth)))
		} else {
			rows = append(rows, styleAttributionsRow.Render(padTo(text, innerWidth)))
		}
	}
	for len(rows) < vh+2 {
		rows = append(rows, "")
	}

	footer := "↑/↓ select · Enter view license · Esc close"
	if a.attributions.listTop+vh < len(thirdPartyAttributions) {
		footer = "↑/↓ select · Enter license · more below · Esc close"
	}
	rows = append(rows, "", styleAttributionsFooter.Render(center(footer, innerWidth)))
	return rows
}

// renderAttributionsDetail builds the full-license view for the selected
// module, scrolled to detailScroll.
func (a *App) renderAttributionsDetail(innerWidth int) []string {
	entry := thirdPartyAttributions[a.attributions.selected]
	body := wrapText(entry.licenseText(), innerWidth)

	vh := a.attributionsViewportHeight()
	maxScroll := len(body) - vh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.attributions.detailScroll > maxScroll {
		a.attributions.detailScroll = maxScroll
	}
	if a.attributions.detailScroll < 0 {
		a.attributions.detailScroll = 0
	}

	end := a.attributions.detailScroll + vh
	if end > len(body) {
		end = len(body)
	}
	visible := body[a.attributions.detailScroll:end]

	rows := []string{
		styleAttributionsTitle.Render(center(ansi.Truncate(entry.module, innerWidth, "…"), innerWidth)),
		styleAttributionsFooter.Render(center(entry.licenseID+" License", innerWidth)),
	}
	for _, line := range visible {
		rows = append(rows, ansi.Truncate(line, innerWidth, "…"))
	}
	for len(rows) < vh+2 {
		rows = append(rows, "")
	}

	footer := "↑/↓ scroll · Esc back"
	if a.attributions.detailScroll < maxScroll {
		footer = "↑/↓ scroll · more below · Esc back"
	}
	rows = append(rows, "", styleAttributionsFooter.Render(center(footer, innerWidth)))
	return rows
}

// attributionRow formats one list line: the module path on the left and its
// license id flush right, truncating the path if the line would overflow.
func attributionRow(entry attribution, width int) string {
	tag := entry.licenseID
	left := ansi.Truncate(entry.module, width-ansi.StringWidth(tag)-1, "…")
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(tag)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + tag
}

// wrapText wraps each source line to width, preserving the document's own line
// breaks and leading indentation so structured licenses stay readable.
func wrapText(s string, width int) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if ansi.StringWidth(line) <= width {
			out = append(out, line)
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		cur := ""
		for _, word := range strings.Fields(line) {
			switch {
			case cur == "":
				cur = indent + word
			case ansi.StringWidth(cur)+1+ansi.StringWidth(word) <= width:
				cur += " " + word
			default:
				out = append(out, cur)
				cur = indent + word
			}
		}
		out = append(out, cur)
	}
	return out
}

// center pads s with spaces so it sits in the middle of a width-wide field.
func center(s string, width int) string {
	w := ansi.StringWidth(s)
	if w >= width {
		return ansi.Truncate(s, width, "…")
	}
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-w-left)
}

// padTo right-pads s with spaces to exactly width (truncating if longer).
func padTo(s string, width int) string {
	w := ansi.StringWidth(s)
	if w >= width {
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}
