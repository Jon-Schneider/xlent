package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/Jon-Schneider/xlent/internal/ui"
)

// pseudoVersion matches Go module pseudo-versions (e.g.
// v0.0.0-20260710221620-f3c05c9c48f0). These are synthesized for untagged
// builds from a checkout; they carry the same revision we already print in
// parentheses, so we treat them as dev builds rather than a real release.
var pseudoVersion = regexp.MustCompile(`-[0-9]{14}-[0-9a-f]{12}`)

// version is the fallback build version used when the binary carries no module
// version (for example, a plain `go build` from a checkout). Release builds and
// `go install module@version` builds report their real version instead, via
// debug.ReadBuildInfo below.
var version = "dev"

const usageText = `Usage: xlent [flags] [file]

xlent is an Excel-style spreadsheet for the terminal. Pass a workbook path to
open it, or run with no arguments to start with a blank workbook.

Supported file types: .xlsx, .xlsm, .xltm, .xltx, .csv

Flags:
  -h, --help       show this help message and exit
  -v, --version    print version information and exit
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses CLI arguments and launches the app, returning a process exit code.
// It is split out from main so argument handling can be exercised in tests
// without spawning the terminal UI.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("xlent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usageText) }

	var showHelp, showVersion bool
	fs.BoolVar(&showHelp, "help", false, "show this help message and exit")
	fs.BoolVar(&showHelp, "h", false, "show this help message and exit (shorthand)")
	fs.BoolVar(&showVersion, "version", false, "print version information and exit")
	fs.BoolVar(&showVersion, "v", false, "print version information and exit (shorthand)")

	if err := fs.Parse(args); err != nil {
		// flag has already written the error and usage to stderr.
		return 2
	}

	if showHelp {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	if showVersion {
		fmt.Fprintln(stdout, versionString())
		return 0
	}

	rest := fs.Args()
	if len(rest) > 1 {
		fmt.Fprintln(stderr, "xlent: too many arguments; expected at most one file path")
		fmt.Fprint(stderr, usageText)
		return 2
	}

	var path string
	if len(rest) == 1 {
		path = rest[0]
	}

	app, err := ui.NewApp(path)
	if err != nil {
		fmt.Fprintf(stderr, "xlent: %v\n", err)
		return 1
	}

	program := tea.NewProgram(app, tea.WithOutput(stdout))
	capture := newShiftMouseCapture(stdout, os.Getenv("TMUX") != "")
	if err := runInteractiveProgram(program, capture); err != nil {
		fmt.Fprintf(stderr, "xlent: %v\n", err)
		return 1
	}
	return 0
}

// versionString reports the binary's version, enriched with the VCS revision
// and dirty state when the build embedded them.
func versionString() string {
	v := version
	revision, modified := "", ""

	if info, ok := debug.ReadBuildInfo(); ok {
		if mv := info.Main.Version; mv != "" && mv != "(devel)" && !pseudoVersion.MatchString(mv) {
			v = mv
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				modified = s.Value
			}
		}
	}

	if len(revision) > 7 {
		revision = revision[:7]
	}

	switch {
	case revision != "" && modified == "true":
		return fmt.Sprintf("xlent %s (%s, dirty)", v, revision)
	case revision != "":
		return fmt.Sprintf("xlent %s (%s)", v, revision)
	default:
		return "xlent " + v
	}
}
