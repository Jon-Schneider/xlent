// Package ui implements the terminal interface for xl: the grid, menu bar,
// formula bar, status bar, and all keyboard/mouse handling.
package ui

import (
	tea "charm.land/bubbletea/v2"
)

// App is the root Bubble Tea model for xl.
type App struct {
	width  int
	height int

	// keyboardEnhanced reports whether the terminal supports the kitty
	// keyboard protocol (Tier 1 in the spec). When false, shortcuts like
	// Ctrl+Shift+Arrow are unavailable and fallbacks apply.
	keyboardEnhanced bool
}

// NewApp creates the root model. path is an optional workbook to open; an
// empty path starts with a blank workbook.
func NewApp(path string) (*App, error) {
	_ = path // wired up when the document package lands
	return &App{}, nil
}

func (a *App) Init() tea.Cmd {
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case tea.KeyboardEnhancementsMsg:
		a.keyboardEnhanced = true

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+q":
			return a, tea.Quit
		}
	}
	return a, nil
}

func (a *App) View() tea.View {
	v := tea.NewView("xl — press Ctrl+Q to quit")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "xl"
	return v
}
