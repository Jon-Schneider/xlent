package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	requestShiftMouseCapture = "\x1b[>1s"
	releaseShiftMouseCapture = "\x1b[>0s"
)

// interactiveProgram is the portion of Bubble Tea's program lifecycle needed
// to scope terminal modes around a running xlent session.
type interactiveProgram interface {
	Run() (tea.Model, error)
}

// shiftMouseCapture asks compatible terminals to include Shift in mouse
// reports. Terminals retain the right to reject the request through their own
// configuration, such as Ghostty's mouse-shift-capture=never setting.
type shiftMouseCapture struct {
	output     io.Writer
	insideTmux bool
}

func newShiftMouseCapture(output io.Writer, insideTmux bool) shiftMouseCapture {
	return shiftMouseCapture{output: output, insideTmux: insideTmux}
}

func (capture shiftMouseCapture) enable() error {
	return capture.write("request shift-modified mouse reporting", requestShiftMouseCapture)
}

func (capture shiftMouseCapture) disable() error {
	return capture.write("release shift-modified mouse reporting", releaseShiftMouseCapture)
}

func (capture shiftMouseCapture) write(operation, sequence string) error {
	if capture.insideTmux {
		sequence = tmuxPassthrough(sequence)
	}
	written, err := io.WriteString(capture.output, sequence)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if written != len(sequence) {
		return fmt.Errorf("%s: %w", operation, io.ErrShortWrite)
	}
	return nil
}

// tmuxPassthrough wraps a terminal sequence in tmux's DCS passthrough format.
// ESC bytes inside the payload must be doubled so tmux forwards one literal
// ESC byte to the outer terminal.
func tmuxPassthrough(sequence string) string {
	escaped := strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b")
	return "\x1bPtmux;" + escaped + "\x1b\\"
}

// runInteractiveProgram releases the Shift-capture request after Bubble Tea
// has restored its own mouse, alternate-screen, and raw-terminal modes.
func runInteractiveProgram(program interactiveProgram, capture shiftMouseCapture) (runErr error) {
	if err := capture.enable(); err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, capture.disable())
	}()

	_, runErr = program.Run()
	return runErr
}
