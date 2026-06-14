package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// attribution is a single third-party dependency credit: the module import
// path and the license it ships under.
type attribution struct {
	module  string
	license string
}

// thirdPartyAttributions lists the open-source modules xlent depends on,
// grouped roughly by upstream project, with the license each is distributed
// under. Keep this in sync with go.mod when dependencies change.
var thirdPartyAttributions = []attribution{
	{"charm.land/bubbletea/v2", "MIT — © Charmbracelet, Inc."},
	{"charm.land/lipgloss/v2", "MIT — © Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/ansi", "MIT — © Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/term", "MIT — © Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/termios", "MIT — © Charmbracelet, Inc."},
	{"github.com/charmbracelet/x/windows", "MIT — © Charmbracelet, Inc."},
	{"github.com/charmbracelet/colorprofile", "MIT — © Charmbracelet, Inc."},
	{"github.com/charmbracelet/ultraviolet", "MIT — © Charmbracelet, Inc."},
	{"github.com/xuri/excelize/v2", "BSD-3-Clause — © xuri"},
	{"github.com/xuri/efp", "BSD-3-Clause — © xuri"},
	{"github.com/xuri/nfp", "BSD-3-Clause — © xuri"},
	{"github.com/clipperhouse/displaywidth", "MIT — © Clipperhouse"},
	{"github.com/clipperhouse/uax29/v2", "MIT — © Clipperhouse"},
	{"github.com/lucasb-eyer/go-colorful", "MIT — © Lucas Beyer"},
	{"github.com/mattn/go-runewidth", "MIT — © Yasuhiro Matsumoto"},
	{"github.com/muesli/cancelreader", "MIT — © Christian Muehlhaeuser"},
	{"github.com/richardlehane/mscfb", "Apache-2.0 — © Richard Lehane"},
	{"github.com/richardlehane/msoleps", "Apache-2.0 — © Richard Lehane"},
	{"github.com/rivo/uniseg", "MIT — © Oliver Kuederle"},
	{"github.com/tiendc/go-deepcopy", "MIT — © Tien Dang"},
	{"github.com/xo/terminfo", "MIT — © xo"},
	{"golang.org/x/crypto", "BSD-3-Clause — © The Go Authors"},
	{"golang.org/x/net", "BSD-3-Clause — © The Go Authors"},
	{"golang.org/x/sync", "BSD-3-Clause — © The Go Authors"},
	{"golang.org/x/sys", "BSD-3-Clause — © The Go Authors"},
	{"golang.org/x/text", "BSD-3-Clause — © The Go Authors"},
}

// attributionsOverlay is the open/scroll state of the Help ▸ Attributions
// panel. It renders as a centered, scrollable box on top of the grid.
type attributionsOverlay struct {
	open   bool
	scroll int
}

func (o *attributionsOverlay) openPanel() {
	o.open = true
	o.scroll = 0
}

func (o *attributionsOverlay) close() {
	o.open = false
	o.scroll = 0
}

// attributionLines renders the panel body as plain text lines (no border),
// one entry over two lines: the module path and its license beneath.
func attributionLines() []string {
	lines := []string{
		"xlent is built on these open-source projects.",
		"Thank you to their authors and maintainers.",
		"",
	}
	for _, a := range thirdPartyAttributions {
		lines = append(lines, a.module, "  "+a.license, "")
	}
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// handleAttributionsKey processes keys while the Attributions panel is open:
// Esc/Enter/q close it; the arrows and page keys scroll.
func (a *App) handleAttributionsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q", "ctrl+w":
		a.attributions.close()
	case "down", "j":
		a.attributions.scroll++
	case "up", "k":
		if a.attributions.scroll > 0 {
			a.attributions.scroll--
		}
	case "pgdown", " ":
		a.attributions.scroll += a.attributionsViewportHeight()
	case "pgup":
		a.attributions.scroll -= a.attributionsViewportHeight()
		if a.attributions.scroll < 0 {
			a.attributions.scroll = 0
		}
	case "home", "g":
		a.attributions.scroll = 0
	}
	a.clampAttributionsScroll()
	return a, nil
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

// clampAttributionsScroll keeps the scroll offset within the content so the
// last line can't be scrolled off the top of the viewport.
func (a *App) clampAttributionsScroll() {
	maxScroll := len(attributionLines()) - a.attributionsViewportHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.attributions.scroll > maxScroll {
		a.attributions.scroll = maxScroll
	}
	if a.attributions.scroll < 0 {
		a.attributions.scroll = 0
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
)

// overlayAttributions splices the Attributions panel, centered, on top of the
// already-rendered frame. It mirrors overlayDropdown's ANSI-safe approach.
func (a *App) overlayAttributions(content string) string {
	if !a.attributions.open {
		return content
	}
	a.clampAttributionsScroll()

	innerWidth := 56
	if a.width-8 < innerWidth {
		innerWidth = a.width - 8
	}
	if innerWidth < 10 {
		innerWidth = 10
	}

	body := attributionLines()
	vh := a.attributionsViewportHeight()
	if vh > len(body) {
		vh = len(body)
	}
	end := a.attributions.scroll + vh
	if end > len(body) {
		end = len(body)
	}
	visible := body[a.attributions.scroll:end]

	rows := make([]string, 0, vh+2)
	rows = append(rows, styleAttributionsTitle.Render(center("Third-Party Attributions", innerWidth)))
	rows = append(rows, "")
	for _, line := range visible {
		rows = append(rows, ansi.Truncate(line, innerWidth, "…"))
	}
	for len(rows) < vh+2 {
		rows = append(rows, "")
	}
	footer := "↑/↓ scroll · Esc to close"
	if a.attributions.scroll+vh < len(body) {
		footer = "↑/↓ scroll · more below · Esc to close"
	}
	rows = append(rows, "", styleAttributionsFooter.Render(center(footer, innerWidth)))

	box := strings.Split(styleAttributionsBox.Width(innerWidth).Render(strings.Join(rows, "\n")), "\n")

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

// center pads s with spaces so it sits in the middle of a width-wide field.
func center(s string, width int) string {
	w := ansi.StringWidth(s)
	if w >= width {
		return ansi.Truncate(s, width, "…")
	}
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-w-left)
}
