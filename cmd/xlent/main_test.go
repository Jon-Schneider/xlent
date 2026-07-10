package main

import (
	"bytes"
	"strings"
	"testing"
)

// runResult captures the outcome of invoking run with a set of arguments.
type runResult struct {
	code   int
	stdout string
	stderr string
}

func runArgs(t *testing.T, args ...string) runResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestHelpFlagPrintsUsageToStdout(t *testing.T) {
	for _, flag := range []string{"--help", "-help", "-h"} {
		result := runArgs(t, flag)
		if result.code != 0 {
			t.Errorf("%s: exit code = %d, want 0", flag, result.code)
		}
		if !strings.Contains(result.stdout, "Usage: xlent") {
			t.Errorf("%s: stdout missing usage text; got %q", flag, result.stdout)
		}
		if result.stderr != "" {
			t.Errorf("%s: stderr = %q, want empty", flag, result.stderr)
		}
	}
}

func TestVersionFlagPrintsVersionToStdout(t *testing.T) {
	for _, flag := range []string{"--version", "-version", "-v"} {
		result := runArgs(t, flag)
		if result.code != 0 {
			t.Errorf("%s: exit code = %d, want 0", flag, result.code)
		}
		if !strings.HasPrefix(result.stdout, "xlent ") {
			t.Errorf("%s: stdout = %q, want it to start with %q", flag, result.stdout, "xlent ")
		}
		if result.stderr != "" {
			t.Errorf("%s: stderr = %q, want empty", flag, result.stderr)
		}
	}
}

func TestUnknownFlagIsRejected(t *testing.T) {
	result := runArgs(t, "--bogus")
	if result.code != 2 {
		t.Errorf("exit code = %d, want 2", result.code)
	}
	if !strings.Contains(result.stderr, "bogus") {
		t.Errorf("stderr does not mention the unknown flag; got %q", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
}

func TestTooManyArgumentsIsRejected(t *testing.T) {
	result := runArgs(t, "one.xlsx", "two.xlsx")
	if result.code != 2 {
		t.Errorf("exit code = %d, want 2", result.code)
	}
	if !strings.Contains(result.stderr, "too many arguments") {
		t.Errorf("stderr missing 'too many arguments'; got %q", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
}

func TestVersionStringHasPrefix(t *testing.T) {
	if got := versionString(); !strings.HasPrefix(got, "xlent ") {
		t.Errorf("versionString() = %q, want it to start with %q", got, "xlent ")
	}
}
