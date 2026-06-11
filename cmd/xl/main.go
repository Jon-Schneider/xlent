package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Jon-Schneider/xl/internal/ui"
)

func main() {
	var path string
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	app, err := ui.NewApp(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xl: %v\n", err)
		os.Exit(1)
	}

	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "xl: %v\n", err)
		os.Exit(1)
	}
}
