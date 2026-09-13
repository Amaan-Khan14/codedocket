package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The CLI is a thin main() over flag parsing and dispatch, so the meaningful
// tests here are subprocess-level: build the real binary once and drive the
// full init → record → explore → dispute → note → hook → finalize flows
// against temp dirs, asserting exit codes and stdout/stderr contracts.

var cliBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "codedocket-cli-test-*")
	if err != nil {
		panic(err)
	}
	// Windows exec refuses to launch a binary without the .exe extension,
	// and `go build -o` writes the name verbatim.
	cliBin = filepath.Join(dir, "codedocket-test-bin")
	if runtime.GOOS == "windows" {
		cliBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", cliBin, ".")
	var buildErr bytes.Buffer
	build.Stderr = &buildErr
	if err := build.Run(); err != nil {
		os.RemoveAll(dir)
		panic("building cli test binary: " + err.Error() + ": " + buildErr.String())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runCLI runs one command with dir as cwd. A non-zero exit is not fatal —
// several tests assert on error exits — so the exit code is returned.
func runCLI(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(cliBin, args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("running %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return outBuf.String(), errBuf.String(), code
}

func TestCLIVersionAndUsage(t *testing.T) {
	stdout, _, code := runCLI(t, t.TempDir(), "version")
	if code != 0 {
		t.Fatalf("version exit %d", code)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("version printed nothing")
	}

	_, stderr, code := runCLI(t, t.TempDir())
	if code != 1 || !strings.Contains(stderr, "usage: codedocket") {
		t.Fatalf("no args: exit=%d stderr=%q", code, stderr)
	}

	_, stderr, code = runCLI(t, t.TempDir(), "bogus")
	if code != 1 || !strings.Contains(stderr, "unknown command: bogus") {
		t.Fatalf("unknown command: exit=%d stderr=%q", code, stderr)
	}
}

func TestCLIInitCreatesStoreAndSnippet(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, code := runCLI(t, dir, "init")
	if code != 0 {
		t.Fatalf("init: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "initialized codedocket at") {
		t.Fatalf("init stdout: %q", stdout)
	}
	for _, name := range []string{"knowledge.json", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, ".codedocket", name)); err != nil {
			t.Errorf(".codedocket/%s missing: %v", name, err)
		}
	}
	gitignore, _ := os.ReadFile(filepath.Join(dir, ".codedocket", ".gitignore"))
	for _, entry := range []string{"sessions/", ".knowledge.lock"} {
		if !strings.Contains(string(gitignore), entry) {
			t.Errorf(".gitignore missing %q: %s", entry, gitignore)
		}
	}
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil || !strings.Contains(string(agents), "<!-- codedocket:begin -->") {
		t.Errorf("AGENTS.md snippet not installed: %v", err)
	}

	// Second init refuses (protecting the store) but stays non-destructive.
	_, stderr, code = runCLI(t, dir, "init")
	if code != 1 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("second init: exit=%d stderr=%q", code, stderr)
	}

	// --no-agents opts out of the snippet.
	dir2 := t.TempDir()
	if _, _, code := runCLI(t, dir2, "init", "--no-agents"); code != 0 {
		t.Fatalf("init --no-agents exited %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir2, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("--no-agents must not create AGENTS.md, stat err: %v", err)
	}
}

func TestCLIRecordExploreDisputeFlow(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := runCLI(t, dir, "init", "--no-agents"); code != 0 {
		t.Fatal("init failed")
	}

	// Flag validation errors.
	_, stderr, code := runCLI(t, dir, "record")
	if code == 0 || !strings.Contains(stderr, "key is required") {
		t.Fatalf("record without flags: exit=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runCLI(t, dir, "record", "--key", "audit.flow", "--kind", "bogus",
		"--statement", "s", "--scope", ".")
	if code == 0 || !strings.Contains(stderr, "invalid kind") {
		t.Fatalf("bad kind: exit=%d stderr=%q", code, stderr)
	}

	stdout, _, code := runCLI(t, dir, "record", "--key", "audit.flow", "--kind", "fact",
		"--statement", "Subprocess flow probe.", "--scope", ".")
	if code != 0 || !strings.Contains(stdout, "recorded audit.flow") {
		t.Fatalf("record: exit=%d stdout=%q", code, stdout)
	}

	// Same key again → evidence append, not a new entry.
	stdout, _, code = runCLI(t, dir, "record", "--key", "audit.flow", "--kind", "fact",
		"--statement", "Subprocess flow probe.", "--scope", ".")
	if code != 0 || !strings.Contains(stdout, "updated audit.flow (evidence: 2)") {
		t.Fatalf("re-record: exit=%d stdout=%q", code, stdout)
	}

	stdout, _, code = runCLI(t, dir, "explore", "--key", "audit.flow")
	if code != 0 || !strings.Contains(stdout, "Subprocess flow probe.") {
		t.Fatalf("explore: exit=%d stdout=%q", code, stdout)
	}

	stdout, _, code = runCLI(t, dir, "explore", "--key", "audit.flow", "--json")
	if code != 0 {
		t.Fatalf("explore --json: exit=%d", code)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("explore --json output not JSON: %v\n%s", err, stdout)
	}
	if len(results) != 1 {
		t.Fatalf("want exactly one result, got %d", len(results))
	}

	stdout, _, code = runCLI(t, dir, "dispute", "--key", "audit.flow")
	if code != 0 || !strings.Contains(stdout, "disputed audit.flow") {
		t.Fatalf("dispute: exit=%d stdout=%q", code, stdout)
	}
	stdout, _, code = runCLI(t, dir, "explore", "--key", "audit.flow")
	if code != 0 || !strings.Contains(stdout, "DISPUTED") {
		t.Fatalf("disputed entry not flagged in explore: %q", stdout)
	}

	// Missing store → friendly error, not a crash.
	_, stderr, code = runCLI(t, t.TempDir(), "explore")
	if code != 1 || !strings.Contains(stderr, "no .codedocket store found") {
		t.Fatalf("explore outside store: exit=%d stderr=%q", code, stderr)
	}
}

func TestCLINoteFinalizeHookLoop(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := runCLI(t, dir, "init", "--no-agents"); code != 0 {
		t.Fatal("init failed")
	}

	stdout, _, code := runCLI(t, dir, "note", "scratch observation for the flow test")
	if code != 0 || !strings.Contains(stdout, "noted #1 → cli-") {
		t.Fatalf("note: exit=%d stdout=%q", code, stdout)
	}

	// The Stop hook sees the pending session and blocks with the finalize
	// instruction — cursor wire shape (followup_message), exit 0.
	stdout, _, code = runCLI(t, dir, "hook", "stop", "--client", "cursor")
	if code != 0 || !strings.Contains(stdout, `"followup_message"`) ||
		!strings.Contains(stdout, "codedocket finalize") {
		t.Fatalf("hook with pending: exit=%d stdout=%q", code, stdout)
	}

	// Unknown client is rejected; hook outside a store allows silently.
	if _, _, code = runCLI(t, dir, "hook", "stop", "--client", "vscode"); code == 0 {
		t.Fatal("unknown client must fail")
	}
	stdout, _, code = runCLI(t, t.TempDir(), "hook", "stop", "--client", "cursor")
	if code != 0 || stdout != "" {
		t.Fatalf("hook without store must allow silently: exit=%d stdout=%q", code, stdout)
	}

	stdout, _, code = runCLI(t, dir, "finalize")
	if code != 0 || !strings.Contains(stdout, "finalized cli-") {
		t.Fatalf("finalize: exit=%d stdout=%q", code, stdout)
	}

	// Nothing pending → report, not error; hook allows silently again.
	stdout, _, code = runCLI(t, dir, "finalize")
	if code != 0 || !strings.Contains(stdout, "no pending notes") {
		t.Fatalf("finalize empty: exit=%d stdout=%q", code, stdout)
	}
	stdout, _, code = runCLI(t, dir, "hook", "stop", "--client", "cursor")
	if code != 0 || stdout != "" {
		t.Fatalf("hook after finalize must allow silently: exit=%d stdout=%q", code, stdout)
	}
}
