// Command gen-notices writes the THIRD_PARTY_NOTICES.txt file that is bundled
// into release archives. It reuses the application's own attribution data so the
// distributed notices always match the in-app Help ▸ Attributions panel.
//
// Usage:
//
//	go run ./cmd/gen-notices [output-path]
//
// The output path defaults to THIRD_PARTY_NOTICES.txt in the current directory.
package main

import (
	"fmt"
	"os"

	"github.com/Jon-Schneider/xlent/internal/ui"
)

func main() {
	out := "THIRD_PARTY_NOTICES.txt"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	// Refuse to emit a notices file that would silently ship a degraded license
	// stub for any dependency; a missing license file must fail the release.
	if err := ui.ValidateAttributions(); err != nil {
		fmt.Fprintf(os.Stderr, "gen-notices: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(out, []byte(ui.ThirdPartyNotices()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen-notices: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "gen-notices: wrote %s\n", out)
}
