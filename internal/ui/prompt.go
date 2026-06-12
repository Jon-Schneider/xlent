package ui

// promptKind enumerates what the status-line prompt is asking. Submit
// behavior is dispatched on the kind in App.submitPrompt — no callbacks.
type promptKind int

const (
	promptNone promptKind = iota
	promptSaveAs
	promptOpen
	promptFind
	promptGoTo
	promptConfirmDirty // "Save changes before <pending>? (y/n)"
)

// pendingAction is what happens after a dirty-check prompt resolves (and
// after any save it triggers succeeds).
type pendingAction int

const (
	pendingNone pendingAction = iota
	pendingQuit
	pendingOpenPrompt
	pendingNew
)

// prompt is the one-line input that temporarily replaces the status bar.
type prompt struct {
	kind    promptKind
	label   string
	text    []rune
	pos     int
	pending pendingAction
}

func (p *prompt) active() bool {
	return p.kind != promptNone
}

func (p *prompt) isConfirm() bool {
	return p.kind == promptConfirmDirty
}

func (p *prompt) open(kind promptKind, label, initial string) {
	p.kind = kind
	p.label = label
	p.text = []rune(initial)
	p.pos = len(p.text)
}

func (p *prompt) close() {
	*p = prompt{}
}

func (p *prompt) String() string {
	return string(p.text)
}

func (p *prompt) insert(s string) {
	runes := []rune(s)
	p.text = append(p.text[:p.pos], append(runes, p.text[p.pos:]...)...)
	p.pos += len(runes)
}

func (p *prompt) backspace() {
	if p.pos > 0 {
		p.text = append(p.text[:p.pos-1], p.text[p.pos:]...)
		p.pos--
	}
}

func (p *prompt) left()  { p.pos = max(p.pos-1, 0) }
func (p *prompt) right() { p.pos = min(p.pos+1, len(p.text)) }
