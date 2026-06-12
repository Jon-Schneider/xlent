# xlent — A Terminal Spreadsheet

`xlent` is a non-modal, Excel-style spreadsheet for the terminal. It opens, edits, and
saves `.xlsx` and `.csv` files, evaluates formulas live, and is driven by familiar
Excel keyboard shortcuts and the mouse — no vim grammar, no editing modes to learn.

## 1. Goals

- **Zero learning curve for Excel users.** Plain typing replaces a cell's content;
  arrows move; Ctrl+C/X/V/Z do what they always do. If you know Excel, you know `xlent`.
- **Faithful files.** Opening an `.xlsx` and saving it preserves everything `xlent`
  didn't touch: styles, column widths, number formats, other sheets.
- **Live formulas.** Edit a cell and every dependent cell updates immediately.
- **Pleasant to look at.** A clean grid, a Fresh-style mouse-driven menu bar, styled
  selection and headers, flicker-free redraws.

### Non-goals (v1)

- Charts, pivot tables, conditional formatting, data validation (preserved on
  round-trip, not editable).
- A cell-formatting UI (fonts, colors, borders). Existing formats are *displayed*
  where feasible and always *preserved*; creating new ones comes later.
- Array formulas / dynamic arrays.
- Collaboration, scripting, plugins.

## 2. Technology

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go | Best-in-class modules for both hard subsystems (xlsx, TUI); single static binary |
| TUI framework | [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) + Lip Gloss | Mouse support, kitty keyboard protocol (Ctrl+Shift+Arrow etc.), synchronized output (no tearing) |
| xlsx read/write | [excelize](https://github.com/qax-os/excelize) | Reads, writes, and edits in place while preserving untouched content |
| Formula evaluation | excelize `CalcCellValue` + our own dependency graph | Inherits 400+ battle-tested function implementations; we add incremental recalc |
| CSV | Go stdlib `encoding/csv` | Sufficient |
| Grid widget, undo, clipboard | Custom | No off-the-shelf spreadsheet grid exists in any TUI ecosystem |

## 3. Screen layout

```
┌ File  Edit  View  Help ──────────────────────────────────────────────┐  ← menu bar (mouse + F10)
│ B3            =SUM(B1:B2)                                            │  ← cell reference + formula bar
├──────┬───────────┬───────────┬───────────┬───────────┬───────────────┤
│      │     A     │     B     │     C     │     D     │      E        │
├──────┼───────────┼───────────┼───────────┼───────────┼───────────────┤
│   1  │ Revenue   │     1,200 │           │           │               │
│   2  │ Costs     │       800 │           │           │               │
│   3  │ Total     │ ▓▓▓2,000▓ │           │           │               │  ← selected cell highlighted
│   4  │           │           │           │           │               │
├──────┴───────────┴───────────┴───────────┴───────────┴───────────────┤
│ Sheet1 │ Sheet2 │                                                    │  ← sheet tabs
│ budget.xlsx [+]                 SUM=2,000          Ready             │  ← status bar
└───────────────────────────────────────────────────────────────────────┘
```

- **Menu bar:** Fresh-style. Click to open a dropdown; arrow keys + Enter navigate
  it; `F10` opens it from the keyboard. Menus list their shortcuts.
- **Formula bar:** shows the active cell's raw content (formula, not value) and the
  cell reference. During editing it mirrors the in-cell editor.
- **Grid:** sticky row/column headers, current selection styled distinctly from the
  active cell, formula-error cells (`#DIV/0!` etc.) styled as errors. Column widths
  honor widths read from the file.
- **Status bar:** filename + dirty indicator `[+]`, quick aggregates for the current
  selection (SUM/AVG/COUNT, like Excel's status bar), and mode hints.

## 4. Interaction model

### Non-modal editing

- Typing any printable character on a selected cell **starts a new edit, replacing
  the content** (Excel behavior). `=` starts a formula.
- `F2` edits the existing content with the cursor at the end.
- While editing: `Enter` commits and moves down; `Tab` commits and moves right;
  `Esc` cancels; arrow keys commit and move (Excel's quick-entry behavior), except
  after `F2`, where arrows move within the text.
- `Delete`/`Backspace` on a selection clears contents.

### Navigation & selection

| Keys | Action |
|---|---|
| Arrows | Move active cell |
| `Ctrl+Arrow` | Jump to edge of data region |
| `Shift+Arrow` | Extend selection |
| `Ctrl+Shift+Arrow` | Extend selection to edge of data region |
| `Home` / `Ctrl+Home` | Start of row / A1 |
| `PgUp` / `PgDn` | Page up / down |
| `Ctrl+A` | Select data region, then whole sheet |
| `Ctrl+PgUp` / `Ctrl+PgDn` | Previous / next sheet |

Mouse: click selects, drag extends, click+drag on headers selects whole
rows/columns, wheel scrolls, click on sheet tabs switches sheets.

### Commands

| Keys | Action |
|---|---|
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste (with reference adjustment, §5) |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo |
| `Ctrl+S` | Save (`Ctrl+Shift+S` save-as) |
| `Ctrl+O` | Open |
| `Ctrl+N` | New workbook |
| `Ctrl+Q` | Quit (prompts if dirty) |
| `F10` | Open menu bar |

Raw mode disables flow control (`IXON`) so `Ctrl+S`/`Ctrl+Q` reach the app, and
`Ctrl+C`/`Ctrl+Z` are received as keys, not signals.

### Terminal compatibility tiers

This is terminal physics, not a library limitation: the legacy input protocol
cannot represent some combos.

- **Tier 1 — kitty keyboard protocol** (Ghostty, kitty, iTerm2, WezTerm, foot):
  every shortcut above works. This is the primary target.
- **Tier 2 — legacy protocol** (macOS Terminal.app, others): `Ctrl+Shift+Arrow`
  and similar are indistinguishable from their unshifted forms. Fallbacks: `F8`
  toggles extend-selection mode (Excel has this too), and all commands remain
  reachable through the menu bar. Tier detection is automatic at startup.

Cmd-based macOS shortcuts are impossible in any terminal (the emulator consumes
them); `xlent` uses Windows-Excel-style Ctrl bindings everywhere.

## 5. Formulas

- **Syntax:** Excel A1 notation — cell refs (`B2`), ranges (`A1:B10`), absolute refs
  (`$A$1`), cross-sheet refs (`Sheet2!A1`), the usual operators, and function calls.
- **Evaluation:** the excelize workbook is the single source of truth for cell
  content. Evaluation delegates to excelize's `CalcCellValue` (400+ functions).
- **Incremental recalc:** `xlent` maintains its own dependency graph, built by parsing
  formula references on entry. Editing a cell dirties its transitive dependents
  only; those are re-evaluated in topological order. Cycles produce
  circular-reference errors rather than hangs.
- **Errors:** standard Excel error values (`#DIV/0!`, `#NAME?`, `#REF!`,
  `#VALUE!`, `#N/A`) rendered distinctly in the grid.
- **Tested core function set** (CI-verified against expected outputs; everything
  else excelize supports works on a best-effort basis):
  `SUM, AVERAGE, MIN, MAX, COUNT, COUNTA, COUNTIF, SUMIF, IF, IFERROR, AND, OR,
  NOT, ROUND, ROUNDUP, ROUNDDOWN, ABS, INT, MOD, SQRT, POWER, CONCATENATE, LEFT,
  RIGHT, MID, LEN, TRIM, UPPER, LOWER, TODAY, NOW, DATE, VLOOKUP, INDEX, MATCH`

### Clipboard semantics

- Internal copy/paste adjusts relative references the way Excel does (copy
  `=A1+1` from B1 to B3 → `=A3+1`); cut/paste does not adjust, and updates
  references *to* the moved range.
- Copies are also published to the system clipboard as TSV via OSC 52; pasting
  multi-cell TSV/CSV text from outside fills a range.

## 6. Undo / redo

Command-pattern over document mutations. Every user action produces one undoable
command (a multi-cell paste or delete is a single command, not one per cell).
Commands store inverse mutations (prior cell contents) and apply through the same
path as normal edits so the dependency graph and screen stay consistent. Unlimited
depth within memory limits; redo stack cleared on new edits. Undo history is
per-workbook, survives sheet switches, and is discarded on file close.

## 7. Files

- **xlsx:** loaded and saved via excelize against the same in-memory workbook, so
  untouched cells, styles, formats, defined names, and other sheets round-trip
  intact. Multi-sheet workbooks fully supported (tabs in UI).
- **csv:** loaded into a fresh single-sheet workbook; saving back to `.csv` writes
  computed values (formulas evaluated, as Excel does when saving CSV). Saving a
  CSV-opened file as `.xlsx` (and vice versa) is supported via save-as.
- Dirty tracking with `[+]` indicator; quitting or opening over a dirty workbook
  prompts Save / Discard / Cancel.
- `xlent path/to/file.xlsx` opens a file; bare `xlent` opens an empty workbook.

## 8. Architecture

```
cmd/xlent/             entry point, CLI args
internal/document/  workbook model: wraps excelize, owns all mutations,
                    dirty tracking, save/load (xlsx + csv adapters)
internal/engine/    formula reference parser, dependency graph,
                    incremental recalc orchestration, cycle detection
internal/undo/      command stack (operates on document mutations)
internal/ui/        Bubble Tea app: grid, menu bar, formula bar,
                    status bar, sheet tabs, dialogs, key/mouse handling,
                    terminal capability detection
internal/clipboard/ internal register + OSC 52 bridge, TSV encode/decode,
                    reference adjustment on paste
```

Rules of the road:

- **All mutations flow through `document`** — the UI never touches excelize
  directly. This is what keeps undo, recalc, and dirty tracking coherent.
- `engine` and `document` are UI-free and fully unit-testable; UI tests use Bubble
  Tea's test harness against golden screen renders.
- The grid renders only the visible viewport; cell storage is whatever excelize
  holds (sparse), so large sheets stay cheap.

## 9. Milestones

1. **Skeleton & grid** — Bubble Tea app, viewport grid rendering, keyboard
   navigation and selection, terminal tier detection. *Proves the rendering and
   input model.*
2. **Editing & CSV** — cell editor, formula bar, CSV open/save, clipboard, undo.
   *First genuinely useful build.*
3. **xlsx & formulas** — excelize integration, multi-sheet, dependency graph,
   incremental recalc, the tested function set. *The product's core promise.*
4. **Menu bar & polish** — mouse menus, dialogs (open/save-as/prompts), status-bar
   aggregates, style pass, error-state rendering.

## 10. Risks

| Risk | Mitigation |
|---|---|
| excelize `CalcCellValue` is officially work-in-progress; gaps or perf cliffs on exotic formulas | Pin a tested core function set in CI; treat the rest as best-effort; the dependency graph limits how much we re-evaluate |
| Recalc latency on large dependent chains | Incremental recalc + async evaluation with a "Calculating…" status; cached values render immediately |
| Tier 2 terminals degrade the shortcut set | Auto-detection, `F8` extend mode, menu bar as universal fallback; document Tier 1 terminals as recommended |
| OSC 52 clipboard size limits / terminal support varies | Internal register is the source of truth; OSC 52 is best-effort for interop |

---

## Appendix: original requirements

Preserved from the initial SPEC.md:

* Name it 'xlent'.
* Non-modal, Excel-like cell editing. Plain typing (no modifiers) starts editing/replaces cell content
* Excel-based modifier + key style keyboard shortcuts (for example copy/cut/paste as Ctrl+C/X/V) and cell selection via keyboard shortcuts w/arrow key.
* No vim grammar or editing mode
* Must support Spreadsheet formulas, navigation, selection ranges, undo (ctrl + z)
* Must support reading and writing xlsx and csv files
* As visually-pleasing as a TUI-based spreadsheet app can be
* A mouse-interactive menu bar like Fresh (https://github.com/sinelaw/fresh) has would be nice
* A statically-typed compiled language is a hard requirement
