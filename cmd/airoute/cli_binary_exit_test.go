package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBinaryExitStatusOnUnreachableGateway pins the end-to-end contract that the
// unit tests above cannot: main() must propagate a handler failure into a non-zero
// process exit status. At HEAD this failed - `airoute keys` printed a connection
// error and exited 0 - so the test is a regression guard for the wiring.
func TestBinaryExitStatusOnUnreachableGateway(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI binary; skipped with -short")
	}

	bin := filepath.Join(t.TempDir(), "airoute")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	run := func(args ...string) int {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "AIROUTE_ENDPOINT=http://127.0.0.1:1")
		err := cmd.Run()
		if err == nil {
			return 0
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %v: %v", args, err)
		}
		return exitErr.ExitCode()
	}

	for _, args := range [][]string{
		{"keys"},
		{"models"},
		{"users", "list"},
		{"skills"},
		{"mcp", "status"},
		{"status"},
	} {
		if code := run(args...); code == 0 {
			t.Fatalf("`airoute %s` against an unreachable gateway exited 0, want non-zero", args[0])
		}
	}
}
