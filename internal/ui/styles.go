package ui

import "charm.land/lipgloss/v2"

// The palette uses 256-color ANSI codes for broad terminal compatibility.
var (
	gridLineColor         = lipgloss.Color("237")
	gridAlternateRowColor = lipgloss.Color("235")

	styleHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236"))

	// Header cell for the cursor's row/column, like Excel's highlight.
	styleHeaderActive = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("24")).
				Bold(true)

	// The source headings use a lighter blue while their selected rows or
	// columns are being reordered. This distinguishes an active drag from an
	// ordinary whole-axis selection.
	styleHeaderDragging = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("31")).
				Bold(true)

	styleInsertionLine = lipgloss.NewStyle().
				Foreground(lipgloss.Color("226")).
				Background(lipgloss.Color("236")).
				Bold(true)

	styleCell = lipgloss.NewStyle()

	styleGridLine = lipgloss.NewStyle().
			Foreground(gridLineColor)

	styleCellSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("238"))

	styleCursorCell = lipgloss.NewStyle().
			Reverse(true)

	// The range being picked while pointing a formula reference.
	stylePointedRef = lipgloss.NewStyle().
			Background(lipgloss.Color("29")).
			Foreground(lipgloss.Color("231"))

	styleErrorValue = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)

	styleFormulaBarRef = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("24")).
				Bold(true)

	styleFormulaBar = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252"))

	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("250"))

	styleStatusDirty = lipgloss.NewStyle().
				Background(lipgloss.Color("235")).
				Foreground(lipgloss.Color("214")).
				Bold(true)

	styleTab = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("248"))

	styleTabActive = lipgloss.NewStyle().
			Background(lipgloss.Color("24")).
			Foreground(lipgloss.Color("231")).
			Bold(true)

	styleTabBar = lipgloss.NewStyle().
			Background(lipgloss.Color("234"))
)
