// Package e2e exercises the compiled eolwhen binary as a subprocess. Unit
// tests in the main package call run() directly; these build the actual
// artifact and verify exit codes and stdout/stderr routing through a real
// os.Exec boundary. They stay off the network, so every case here is one
// that answers before endoflife.date is read.
//
// A separate process is also the only place a crash can be observed: a
// stack overflow is fatal, and nothing inside the process that meets one
// survives to report it.
package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "eolwhen-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: mkdir:", err)
		os.Exit(2)
	}
	binPath = filepath.Join(tmp, "eolwhen")
	out, buildErr := exec.Command("go", "build", "-o", binPath, "..").CombinedOutput()
	if buildErr != nil {
		fmt.Fprintf(os.Stderr, "TestMain: go build failed: %v\n%s", buildErr, out)
		os.RemoveAll(tmp)
		os.Exit(2)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

func runBin(t *testing.T, args ...string) result {
	t.Helper()
	return runBinWithin(t, 60*time.Second, args...)
}

// runBinWithin runs the binary and gives up after d, so that a file the
// readers could be talked into following round for ever fails the test
// instead of hanging it.
func runBinWithin(t *testing.T, d time.Duration, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, args...)
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%v did not finish within %s", args, d)
	}
	if err == nil {
		return result{stdout: so.String(), stderr: se.String(), exitCode: 0}
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("run: %v", err)
	}
	return result{stdout: so.String(), stderr: se.String(), exitCode: ee.ExitCode()}
}

func TestHelp(t *testing.T) {
	r := runBin(t, "--help")
	if r.exitCode != 0 || !strings.HasPrefix(r.stdout, "eolwhen — ") || r.stderr != "" {
		t.Errorf("--help = %+v", r)
	}
}

func TestVersion(t *testing.T) {
	r := runBin(t, "--version")
	if r.exitCode != 0 || strings.TrimSpace(r.stdout) == "" || r.stderr != "" {
		t.Errorf("--version = %+v", r)
	}
}

func TestInstructions(t *testing.T) {
	r := runBin(t, "--instructions")
	if r.exitCode != 0 || !strings.HasPrefix(r.stdout, "To find out whether") || strings.Count(r.stdout, "\n") != 1 {
		t.Errorf("--instructions = %+v", r)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"--nope"}, "unknown flag"},
		{"bad duration", []string{"--within", "1m"}, "minutes or months"},
		{"two directories", []string{"a", "b"}, "one directory per run"},
		{"missing directory", []string{filepath.Join(os.TempDir(), "eolwhen-not-here")}, "does not exist"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := runBin(t, tt.args...)
			if r.exitCode != 3 || r.stdout != "" || !strings.HasPrefix(r.stderr, "eolwhen: ") || !strings.Contains(r.stderr, tt.want) {
				t.Errorf("%v = %+v", tt.args, r)
			}
		})
	}
}

// TestNoDeclarations is the directory that declares nothing: the answer is
// exit 0 with the reason on stderr, not an error, because looping over many
// checkouts should not turn every quiet one into a failure.
func TestNoDeclarations(t *testing.T) {
	r := runBin(t, t.TempDir())
	if r.exitCode != 0 || r.stdout != "" || !strings.Contains(r.stderr, "no version declarations in") {
		t.Errorf("empty directory = %+v", r)
	}
}

// write puts a file in dir, making the directories above it.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestSelfReferringFiles runs the binary over files that refer to
// themselves. They are not configurations anyone means, but a directory
// being scanned is not trusted to be sensible, and the run has to end on its
// own either way.
//
// This is checked here rather than in a unit test because the failure it
// guards against is a stack overflow, which is fatal: no recover catches it
// and no timeout inside the process outlives it. Only a child process shows
// whether it happened, and only its output says so.
func TestSelfReferringFiles(t *testing.T) {
	for _, tt := range []struct{ name, path, body string }{
		{"a runner naming itself", ".github/workflows/ci.yml",
			"jobs:\n  test:\n    runs-on: &runner [*runner]\n"},
		{"a runner whose labels name it", ".github/workflows/ci.yml",
			"jobs:\n  test:\n    runs-on: &runner {labels: *runner}\n"},
		// No image anywhere, so nothing is declared and endoflife.date is
		// never asked: what is under test is whether the reading of the
		// file ends, which happens before any of that.
		{"a merge list holding itself", "compose.yml",
			"x-loop: &loop [*loop]\nservices:\n  app:\n    <<: *loop\n"},
		{"a template merging itself", "compose.yml",
			"x-self: &self\n  <<: *self\nservices:\n  app:\n    <<: *self\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, tt.path, tt.body)
			r := runBinWithin(t, 30*time.Second, dir)
			// A crash leaves the runtime's own words behind, and an exit
			// code that `eolwhen || [ $? = 2 ]` would have called success.
			for _, sign := range []string{"fatal error", "goroutine stack exceeds", "runtime.gopanic"} {
				if strings.Contains(r.stderr, sign) {
					t.Fatalf("crashed: stderr contains %q\n%s", sign, r.stderr)
				}
			}
			if r.exitCode != 0 {
				t.Errorf("exit = %d; want 0, nothing having been declared: %s", r.exitCode, r.stderr)
			}
		})
	}
}
