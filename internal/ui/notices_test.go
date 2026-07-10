package ui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden files")

// TestValidateAttributionsResolves guards the compliance invariant that every
// attribution renders real embedded license text. If it fails, licenseText()
// would silently substitute a degraded stub and a release could ship a
// legally-deficient NOTICES file on an otherwise-green build.
func TestValidateAttributionsResolves(t *testing.T) {
	if err := ValidateAttributions(); err != nil {
		t.Fatal(err)
	}
}

// TestValidateAttributionsCatchesMissingLicense proves ValidateAttributions
// actually detects an unresolved license id, so the guard above cannot pass
// vacuously.
func TestValidateAttributionsCatchesMissingLicense(t *testing.T) {
	saved := thirdPartyAttributions
	t.Cleanup(func() { thirdPartyAttributions = saved })

	thirdPartyAttributions = []attribution{
		{"example.com/mod", "NoSuchLicense", "Copyright (c) nobody"},
	}
	if err := ValidateAttributions(); err == nil {
		t.Fatal("ValidateAttributions() = nil, want error for a missing license file")
	}
}

// TestThirdPartyNoticesGolden pins the exact rendered notices artifact against a
// committed golden file. Unlike a structural check, this catches ordering,
// separator, header-text, and copyright-splice regressions in the generator.
// Regenerate after an intentional change with:
//
//	go test ./internal/ui -run TestThirdPartyNoticesGolden -update
func TestThirdPartyNoticesGolden(t *testing.T) {
	got := ThirdPartyNotices()
	golden := filepath.Join("testdata", "notices.golden")

	if *updateGolden {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden file (run with -update to create it): %v", err)
	}

	if got != string(want) {
		t.Errorf("ThirdPartyNotices() differs from %s.\n"+
			"If this change is intentional, regenerate with:\n"+
			"  go test ./internal/ui -run TestThirdPartyNoticesGolden -update", golden)
	}
}

// TestThirdPartyNoticesNoTemplateLeak is a cheap independent guard: even if the
// golden file were regenerated carelessly, an unsubstituted template marker must
// never reach the distributed artifact.
func TestThirdPartyNoticesNoTemplateLeak(t *testing.T) {
	if out := ThirdPartyNotices(); strings.Contains(out, "{{COPYRIGHT}}") {
		t.Error("ThirdPartyNotices() contains an unsubstituted {{COPYRIGHT}} marker")
	}
}
