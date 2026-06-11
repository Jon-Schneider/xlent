package ui

import "charm.land/lipgloss/v2"

// The palette uses 256-color ANSI codes for broad terminal compatibility.
var (
	styleHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236"))

	// Header cell for the cursor's row/column, like Excel's highlight.
	styleHeaderActive = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("24")).
				Bold(true)

	styleCell = lipgloss.NewStyle()

	styleCellSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("238"))

	styleCursorCell = lipgloss.NewStyle().
			Reverse(true)

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
