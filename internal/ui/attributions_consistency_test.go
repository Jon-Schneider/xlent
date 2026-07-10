package ui

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// mainModule is xlent's own module path, which "go list -m all" reports but which
// must not appear in the third-party attribution list.
const mainModule = "github.com/Jon-Schneider/xlent"

// TestAttributionsMatchModuleGraph guards the release-checklist requirement (XL-11)
// that the in-app Help ▸ Attributions list — and therefore the THIRD_PARTY_NOTICES
// file generated from it — stays in lockstep with the modules Go actually resolves.
// It fails loudly whenever a dependency is added or removed without a matching edit
// to thirdPartyAttributions.
func TestAttributionsMatchModuleGraph(t *testing.T) {
	resolved := resolvedModules(t)

	listed := make(map[string]bool, len(thirdPartyAttributions))
	for _, a := range thirdPartyAttributions {
		if listed[a.module] {
			t.Errorf("duplicate attribution entry for %q", a.module)
		}
		listed[a.module] = true
	}

	var missing, extra []string
	for m := range resolved {
		if !listed[m] {
			missing = append(missing, m)
		}
	}
	for m := range listed {
		if !resolved[m] {
			extra = append(extra, m)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("dependencies present in the module graph but missing from thirdPartyAttributions "+
			"(add them, with a license file in internal/ui/licenses/):\n  %s",
			strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("attributions listed that are no longer in the module graph (remove them):\n  %s",
			strings.Join(extra, "\n  "))
	}
}

// resolvedModules returns every module in the build list except xlent itself,
// as reported by "go list -m all".
func resolvedModules(t *testing.T) map[string]bool {
	t.Helper()

	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}", "all")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -m all failed: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -m all failed: %v", err)
	}

	mods := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path := strings.TrimSpace(line)
		if path == "" || path == mainModule {
			continue
		}
		mods[path] = true
	}
	return mods
}
