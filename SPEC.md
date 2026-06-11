How hard would it be to write a command-line spreadsheet app with the following requirements:

* Name it 'xl'.
* Non-modal, Excel-like cell editing. Plain typing (no modifiers) starts editing/replaces cell content
* Excel-based modifier + key style keyboard shortcuts (for example copy/cut/paste as Ctrl+C/X/V) and cell selection via keyboard shortcuts w/arrow key.
* No vim grammar or editing mode
* Must support Spreadsheet formulas, navigation, selection ranges, undo (ctrl + z)
* Must support reading and writing xlsx and csv files
* As visually-pleasing as a TUI-based spreadsheet app can be
* A mouse-interactive menu bar like Fresh (https://github.com/sinelaw/fresh) has would be nice
* Ideally written written it in Swift using xcodegen, but this is not a hard requirement. A statically-typed compiled language is a hard requirement