package ui

import (
	"fmt"
	"strings"
)

// ThirdPartyNotices renders the full third-party license notice document that
// ships in release archives. It draws from the same thirdPartyAttributions list
// and embedded license texts that power the in-app Help ▸ Attributions panel, so
// the distributed NOTICES file can never drift from what the application shows.
//
// The list mirrors xlent's resolved module graph (go list -m all), which includes
// modules used only when building or testing xlent and therefore not linked into
// the shipped binaries. The wording below is deliberately careful not to claim
// every listed module is redistributed.
func ThirdPartyNotices() string {
	var b strings.Builder
	b.WriteString("THIRD-PARTY SOFTWARE NOTICES\n")
	b.WriteString("============================\n\n")
	b.WriteString("xlent is built with the third-party Go modules listed below, portions of\n")
	b.WriteString("which are redistributed in xlent's binaries. Each module is provided under\n")
	b.WriteString("the license reproduced beneath it. This list mirrors xlent's resolved module\n")
	b.WriteString("graph and so also includes modules used only to build or test xlent.\n")

	for _, a := range thirdPartyAttributions {
		b.WriteString("\n")
		b.WriteString(strings.Repeat("-", 76))
		b.WriteString("\n")
		b.WriteString(a.module)
		b.WriteString(" (")
		b.WriteString(a.licenseID)
		b.WriteString(")\n")
		b.WriteString(strings.Repeat("-", 76))
		b.WriteString("\n\n")
		b.WriteString(strings.TrimRight(a.licenseText(), "\n"))
		b.WriteString("\n")
	}

	return b.String()
}

// ValidateAttributions verifies that every entry in thirdPartyAttributions
// resolves to real embedded license text rather than the degraded stub that
// licenseText() emits when a license file is missing. gen-notices and the test
// suite both call this so a missing or mistyped licenseID fails the build
// instead of silently shipping a legally-deficient NOTICES file.
func ValidateAttributions() error {
	var problems []string
	for _, a := range thirdPartyAttributions {
		for _, id := range licenseIDs(a.licenseID) {
			if _, err := licenseFS.ReadFile("licenses/" + id + ".txt"); err != nil {
				problems = append(problems, fmt.Sprintf("%s: no embedded license text for %q", a.module, id))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("unresolved attribution license texts:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}
