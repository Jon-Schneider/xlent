package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestShiftMouseCaptureWritesDirectTerminalSequences(t *testing.T) {
	var output bytes.Buffer
	capture := newShiftMouseCapture(&output, false)

	if err := capture.enable(); err != nil {
		t.Fatal(err)
	}
	if err := capture.disable(); err != nil {
		t.Fatal(err)
	}

	want := requestShiftMouseCapture + releaseShiftMouseCapture
	if got := output.String(); got != want {
		t.Errorf("terminal output = %q, want %q", got, want)
	}
}

func TestShiftMouseCaptureUsesTmuxPassthrough(t *testing.T) {
	var output bytes.Buffer
	capture := newShiftMouseCapture(&output, true)

	if err := capture.enable(); err != nil {
		t.Fatal(err)
	}
	if err := capture.disable(); err != nil {
		t.Fatal(err)
	}

	want := "\x1bPtmux;\x1b\x1b[>1s\x1b\\" +
		"\x1bPtmux;\x1b\x1b[>0s\x1b\\"
	if got := output.String(); got != want {
		t.Errorf("tmux output = %q, want %q", got, want)
	}
}

func TestRunInteractiveProgramScopesShiftCaptureAroundProgram(t *testing.T) {
	var output bytes.Buffer
	program := &recordingInteractiveProgram{output: &output}
	capture := newShiftMouseCapture(&output, false)

	if err := runInteractiveProgram(program, capture); err != nil {
		t.Fatal(err)
	}

	want := requestShiftMouseCapture + "program" + releaseShiftMouseCapture
	if got := output.String(); got != want {
		t.Errorf("lifecycle output = %q, want %q", got, want)
	}
}

func TestRunInteractiveProgramReleasesShiftCaptureAfterProgramFailure(t *testing.T) {
	var output bytes.Buffer
	programFailure := errors.New("program failed")
	program := &recordingInteractiveProgram{output: &output, runErr: programFailure}
	capture := newShiftMouseCapture(&output, false)

	err := runInteractiveProgram(program, capture)
	if !errors.Is(err, programFailure) {
		t.Fatalf("error = %v, want program failure", err)
	}

	want := requestShiftMouseCapture + "program" + releaseShiftMouseCapture
	if got := output.String(); got != want {
		t.Errorf("failure lifecycle output = %q, want %q", got, want)
	}
}

func TestRunInteractiveProgramDoesNotStartWhenCaptureRequestFails(t *testing.T) {
	terminalFailure := errors.New("terminal unavailable")
	output := &failingTerminalWriter{failAt: 1, failure: terminalFailure}
	program := &recordingInteractiveProgram{}
	capture := newShiftMouseCapture(output, false)

	err := runInteractiveProgram(program, capture)
	if !errors.Is(err, terminalFailure) {
		t.Fatalf("error = %v, want terminal failure", err)
	}
	if program.ran {
		t.Error("program ran after the Shift-capture request failed")
	}
	if !strings.Contains(err.Error(), "request shift-modified mouse reporting") {
		t.Errorf("error = %q, want request context", err)
	}
}

func TestRunInteractiveProgramReportsCaptureReleaseFailure(t *testing.T) {
	terminalFailure := errors.New("terminal unavailable")
	output := &failingTerminalWriter{failAt: 2, failure: terminalFailure}
	program := &recordingInteractiveProgram{}
	capture := newShiftMouseCapture(output, false)

	err := runInteractiveProgram(program, capture)
	if !errors.Is(err, terminalFailure) {
		t.Fatalf("error = %v, want terminal failure", err)
	}
	if !program.ran {
		t.Error("program did not run after a successful Shift-capture request")
	}
	if !strings.Contains(err.Error(), "release shift-modified mouse reporting") {
		t.Errorf("error = %q, want release context", err)
	}
}

type recordingInteractiveProgram struct {
	output io.Writer
	runErr error
	ran    bool
}

func (program *recordingInteractiveProgram) Run() (tea.Model, error) {
	program.ran = true
	if program.output != nil {
		if _, err := io.WriteString(program.output, "program"); err != nil {
			return nil, err
		}
	}
	return nil, program.runErr
}

type failingTerminalWriter struct {
	writes  int
	failAt  int
	failure error
}

func (writer *failingTerminalWriter) Write(p []byte) (int, error) {
	writer.writes++
	if writer.writes == writer.failAt {
		return 0, writer.failure
	}
	return len(p), nil
}
