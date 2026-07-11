# xlent

An Excel-style spreadsheet for the terminal. `xlent` opens, edits, and saves
`.xlsx`, `.xlsm`, `.xltm`, `.xltx`, and `.csv` files, evaluates formulas live,
and is driven by the keyboard shortcuts and mouse gestures you already know
from Excel — no modes, no new grammar to learn.

```
┌ File  Edit  Format  Data  Formulas  View  Help ──────────────────────┐
│ B3            =SUM(B1:B2)                                            │
├──────┬───────────┬───────────┬───────────┬───────────┬───────────────┤
│      │     A     │     B     │     C     │     D     │      E        │
├──────┼───────────┼───────────┼───────────┼───────────┼───────────────┤
│   1  │ Revenue   │     1,200 │           │           │               │
│   2  │ Costs     │       800 │           │           │               │
│   3  │ Total     │ ▓▓▓2,000▓ │           │           │               │
├──────┴───────────┴───────────┴───────────┴───────────┴───────────────┤
│ Sheet1 │ Sheet2 │                                                    │
│ budget.xlsx [+]                 SUM=2,000          Ready             │
└───────────────────────────────────────────────────────────────────────┘
```

## Install

### macOS (Homebrew)

```sh
brew tap jon-schneider/xlent
brew install --cask xlent
```

The cask installs a signed, notarized binary, so it runs without any Gatekeeper
prompt. Upgrade with `brew upgrade --cask xlent`.

### Prebuilt archives

Tagged releases provide ready-to-run archives for macOS, Linux, and Windows on
the [GitHub Releases page](https://github.com/Jon-Schneider/xlent/releases),
each with a `checksums.txt` for verification.

### From Go

With Go 1.26 or newer, install the latest release directly:

```sh
go install github.com/Jon-Schneider/xlent/cmd/xlent@latest
```

Then open a workbook, or run `xlent` with no arguments to start with a blank
workbook:

```sh
xlent budget.xlsx
xlent --help
```

To build from a source checkout, run `./build.sh`; it vets and tests the code
before writing the `xlent` binary at the repository root.

## Platform and terminal support

`xlent` supports macOS, Linux, and Windows in a modern ANSI-capable terminal.
Mouse support is recommended.

- **Tier 1 — kitty keyboard protocol:** Ghostty, kitty, iTerm2, WezTerm, and
  foot expose the full shortcut set, including `Ctrl+Shift+Arrow`.
- **Tier 2 — legacy terminal protocol:** macOS Terminal.app and other
  terminals without the kitty protocol cannot distinguish some modified key
  combinations (for example, `Ctrl+Shift+Arrow` from `Ctrl+Arrow`). `F8`
  toggles extend-selection mode as a fallback, and all commands remain
  available from the menu bar.

Terminal capability detection happens automatically at startup. macOS `Cmd`
shortcuts are consumed by terminal emulators, so `xlent` consistently uses
Windows-Excel-style `Ctrl` bindings.

## Features

- **Excel's editing model.** Typing replaces the cell, `F2` edits in place,
  `Enter`/`Tab`/arrows commit and move, `Esc` cancels. Formulas normalize on
  entry — `=sum(a1:a2` commits as `=SUM(A1:A2)`, with parens and string
  quotes auto-closed.
- **Live formulas.** A dependency graph recalculates exactly the affected
  cells on each edit; evaluation is delegated to excelize's 400+ function
  implementations. Cycles render as `#CIRC!` instead of hanging.
- **Point mode.** Mid-formula, arrow keys or mouse clicks pick cell and range
  references off the grid — including from other sheets (`Ctrl+PgUp/PgDn` or
  a tab click mid-edit), with sheet qualifiers quoted as needed.
- **Reference highlighting.** While a formula is edited, each reference is
  colored in the formula bar and the cells it targets get a matching tint.
- **Copy/cut/paste with Excel semantics.** Copying adjusts relative
  references; cutting drags references along with the move (`$`-anchors pin
  against copies, not moves). External paste via the terminal clipboard, TSV
  in and out. Paste Special variants paste values, formats, or a transposed
  block from the Edit menu.
- **Structure editing.** Insert/delete rows (`Ctrl++` / `Ctrl+-`) and
  columns; add, rename, and delete sheets. Renaming a sheet rewrites every
  formula that referenced it by name.
- **Formatting.** Number formats (currency, percent, date/time, text…) and
  bold/italic/underline (`Ctrl+B/I/U`) from the Format menu, persisted to the
  file as standard xlsx styles.
- **Data tools.** Sort a range ascending or descending; filter rows against a
  column (`Ctrl+Shift+L`, `Clear Filter` to restore); fill down/right
  (`Ctrl+D` / `Ctrl+R`) or fill a numeric series; create and remove Excel
  tables that auto-expand as you type below them. Data-validation rules already
  in a workbook are enforced on entry (authoring new rules is not yet
  supported).
- **Named ranges and freeze panes.** Define and delete workbook names from the
  Formulas menu; freeze/unfreeze panes and force a full `Recalculate All`
  (`F9`) from the View menu.
- **Find, Replace, and Go To.** `Ctrl+F` searches content and displayed values
  (`F3` repeats); `Ctrl+H` replaces across the sheet; `Ctrl+G` jumps to a cell,
  range, or `Sheet2!B3`.
- **Undo/redo for everything** (`Ctrl+Z` / `Ctrl+Y`): one user action is one
  undo step. Structural changes (rows, columns, sheets, formats) restore
  whole-workbook snapshots; plain edits replay.
- **Faithful round-trips.** `xlent` is designed to preserve workbook content it
  does not edit where supported by Excelize, including styles, widths, other
  sheets, and content it cannot display.
- **Mouse everywhere.** Click and drag to select, drag column edges in the
  header to resize, click tabs, scroll with the wheel, use the menus.

## Formula compatibility

`xlent` evaluates formulas through excelize, whose 400+ function
implementations cover the common Excel formula set. Formulas outside that set
are best-effort.

Dynamic-array formulas and array formulas are not evaluated. This includes
`FILTER`, `SORT`, `UNIQUE`, `SEQUENCE`, `XLOOKUP`, array constants such as
`{1,2;3,4}`, and legacy CSE array formulas. `xlent` flags these formulas in the
status bar and preserves them when saving, so an unsupported result is not
silently presented as current.

Structured references such as `Table[Column]` and `[@Column]` are tracked for
dependencies, but are not evaluated. Excel tables themselves can be created
and managed, and they round-trip and auto-expand normally.

## File support

`xlent` opens and saves `.xlsx`, `.xlsm`, `.xltm`, `.xltx`, and `.csv` files.
Macro-enabled and template workbooks round-trip through the same workbook
format. `xlent` never executes macros: it retains them as opaque workbook data
when saving. CSV files use one sheet, and saving to CSV writes the first
sheet's displayed values.

Encrypted workbooks that require an open password are not supported: `xlent`
does not prompt for or accept a password, so it cannot open them. Save an
unencrypted copy from a spreadsheet application before opening it in `xlent`.

## Keyboard reference

| Keys | Action |
|---|---|
| Arrows, `PgUp`/`PgDn`, `Home`, `Ctrl+Home` | Move around |
| `Ctrl+Arrow` / `Ctrl+Shift+Arrow` | Jump to data edge / extend to it |
| `Shift+Arrow`, `F8` | Extend selection |
| `Ctrl+A` | Select used range |
| `Ctrl+PgUp` / `Ctrl+PgDn` | Previous / next sheet (mid-formula: cross-sheet pointing) |
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo |
| `Ctrl++` / `Ctrl+-` | Insert / delete rows (columns via Edit menu) |
| `Ctrl+D` / `Ctrl+R` | Fill down / right (fill series via Data menu) |
| `Ctrl+Shift+L` | Toggle filter on the selected column |
| `Ctrl+B` / `Ctrl+I` / `Ctrl+U` | Bold / italic / underline |
| `Ctrl+F`, `F3`, `Ctrl+H`, `Ctrl+G` | Find, find next, replace, go to |
| `F9` | Recalculate all |
| `Ctrl+S` / `Ctrl+Shift+S`, `Ctrl+O`, `Ctrl+N` | Save / save as, open, new |
| `F2` | Edit cell in place |
| `F10` | Open the menu bar |
| `Ctrl+Q` / `Ctrl+W` | Quit |

## Architecture

```
cmd/xlent                  entry point
internal/ui             Bubble Tea app: grid, menus, prompts, editor, mouse
internal/document       Workbook: all mutations, recalc orchestration, I/O
internal/engine         references, formula rewriting, dependency graph
internal/undo           undo/redo stack (edit replay + snapshot commands)
internal/clipboard      copy/paste blocks, TSV encoding
```

The invariant that keeps it coherent: **every mutation flows through
`document.Workbook`** — the UI never touches the underlying file directly,
and undo replays through the same path, so the dependency graph and caches
can't drift from the file.

Built with [Bubble Tea v2](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss), and
[excelize](https://github.com/qax-os/excelize). See `SPEC.md` for the full
product definition.

## Development

```sh
./build.sh          # go vet + go test ./... + build
./build.sh --fast   # build only
```

## License

Licensed under the [Apache License, Version 2.0](LICENSE). Third-party
dependencies retain their own licenses; see the in-app Help → Attributions
screen for the full list.

Microsoft Excel is a trademark of the Microsoft group of companies. `xlent` is
an independent project and is not affiliated with, endorsed by, sponsored by,
or approved by Microsoft.
