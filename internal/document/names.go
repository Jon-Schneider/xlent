package document

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/Jon-Schneider/xlent/internal/engine"
)

// DefinedNameInfo describes one workbook defined name for the UI.
type DefinedNameInfo struct {
	Name     string
	RefersTo string
	Scope    string
}

// loadNames rebuilds the name→range index from the workbook's defined names so
// formulas using a name depend on the cells it covers. Names whose target
// isn't a plain A1 range (formulas, constants, external refs) are indexed for
// listing but contribute no dependency.
func (w *Workbook) loadNames() {
	w.names = make(map[string]engine.Ref)
	for _, dn := range w.file.GetDefinedName() {
		refersTo := strings.TrimPrefix(dn.RefersTo, "=")
		ref, err := engine.ParseRef("", refersTo)
		if err != nil {
			continue
		}
		w.names[strings.ToUpper(dn.Name)] = ref
	}
}

// DefinedNames lists the workbook's defined names, sorted by name.
func (w *Workbook) DefinedNames() []DefinedNameInfo {
	defs := w.file.GetDefinedName()
	out := make([]DefinedNameInfo, 0, len(defs))
	for _, dn := range defs {
		out = append(out, DefinedNameInfo{Name: dn.Name, RefersTo: dn.RefersTo, Scope: dn.Scope})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SetDefinedName creates or updates a workbook-scoped defined name pointing at
// refersTo (e.g. "Sheet1!$A$1:$B$2"). An existing name of the same name is
// replaced. Derived state is rebuilt so dependencies on the name take effect.
func (w *Workbook) SetDefinedName(name, refersTo string) error {
	// excelize rejects a duplicate, so drop any existing definition first.
	_ = w.file.DeleteDefinedName(&excelize.DefinedName{Name: name})
	if err := w.file.SetDefinedName(&excelize.DefinedName{Name: name, RefersTo: refersTo}); err != nil {
		return fmt.Errorf("define name %q: %w", name, err)
	}
	w.resetDerivedState()
	return nil
}

// DeleteDefinedName removes a workbook-scoped defined name.
func (w *Workbook) DeleteDefinedName(name string) error {
	if err := w.file.DeleteDefinedName(&excelize.DefinedName{Name: name}); err != nil {
		return fmt.Errorf("delete name %q: %w", name, err)
	}
	w.resetDerivedState()
	return nil
}
