package engine

import "testing"

func TestRenameSheetRefsRewritesQualifiedReferences(t *testing.T) {
	tests := []struct {
		formula string
		old     string
		new     string
		want    string
		changed bool
	}{
		{"=Data!A1*2", "Data", "Numbers", "=Numbers!A1*2", true},
		{"=SUM(Data!A1:B2)+data!C3", "Data", "Numbers", "=SUM(Numbers!A1:B2)+Numbers!C3", true},
		{"='Old Name'!A1", "Old Name", "Fresh", "=Fresh!A1", true},
		// A new name with a space must come out quoted.
		{"=Data!A1", "Data", "My Data", "='My Data'!A1", true},
		// Unqualified references and other sheets are untouched.
		{"=A1+Other!B2", "Data", "Numbers", "=A1+Other!B2", false},
		// Sheet-name text inside string literals must survive unscathed.
		{`=IF(Data!A1,"Data!A1","say ""Data""")`, "Data", "N", `=IF(N!A1,"Data!A1","say ""Data""")`, true},
	}
	for _, tt := range tests {
		got, changed := RenameSheetRefs(tt.formula, tt.old, tt.new)
		if changed != tt.changed {
			t.Errorf("RenameSheetRefs(%q) changed = %v, want %v", tt.formula, changed, tt.changed)
		}
		if changed && got != tt.want {
			t.Errorf("RenameSheetRefs(%q) = %q, want %q", tt.formula, got, tt.want)
		}
		if !changed && got != tt.formula {
			t.Errorf("RenameSheetRefs(%q) rewrote to %q without reporting a change", tt.formula, got)
		}
	}
}
