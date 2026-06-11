package ui

// editMode distinguishes Excel's two editing behaviors: an edit started by
// typing (Enter mode — arrow keys commit and move) and one started with F2
// (Edit mode — left/right move within the text).
type editMode int

const (
	editModeReplace editMode = iota
	editModeInPlace
)

// editor is the in-cell text editor. It is intentionally dumb: all commit /
// cancel / navigation policy lives in the App's key handling.
type editor struct {
	active bool
	mode   editMode
	text   []rune
	pos    int
}

func (e *editor) start(initial string, mode editMode) {
	e.active = true
	e.mode = mode
	e.text = []rune(initial)
	e.pos = len(e.text)
}

func (e *editor) stop() {
	*e = editor{}
}

func (e *editor) String() string {
	return string(e.text)
}

func (e *editor) insert(s string) {
	runes := []rune(s)
	e.text = append(e.text[:e.pos], append(runes, e.text[e.pos:]...)...)
	e.pos += len(runes)
}

func (e *editor) backspace() {
	if e.pos > 0 {
		e.text = append(e.text[:e.pos-1], e.text[e.pos:]...)
		e.pos--
	}
}

func (e *editor) deleteForward() {
	if e.pos < len(e.text) {
		e.text = append(e.text[:e.pos], e.text[e.pos+1:]...)
	}
}

func (e *editor) left()  { e.pos = max(e.pos-1, 0) }
func (e *editor) right() { e.pos = min(e.pos+1, len(e.text)) }
func (e *editor) home()  { e.pos = 0 }
func (e *editor) end()   { e.pos = len(e.text) }

// window returns the visible slice of the editor text for a cell of width w,
// keeping the cursor in view, plus the cursor's offset within that slice.
func (e *editor) window(w int) (visible string, cursorOffset int) {
	if w < 1 {
		return "", 0
	}
	start := 0
	if e.pos > w-1 {
		start = e.pos - (w - 1)
	}
	end := min(start+w, len(e.text))
	return string(e.text[start:end]), e.pos - start
}
