package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// In-process drivers for the command functions. The subprocess tests in
// cli_test.go pin exit codes and flag-parse failures; these call
// *Ctx() directly so `go test -cover` sees the executed statements (the
// separately built subprocess binary is not instrumented). Validation
// failures that return errors are safe here; flag-parse errors would
// os.Exit via flag.ExitOnError and stay subprocess-only.

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	readErr := make(chan error, 1)
	outCh := make(chan string, 1)
	go func() {
		b, err := io.ReadAll(r)
		outCh <- string(b)
		readErr <- err
	}()

	fnErr := fn()
	w.Close()
	if err := <-readErr; err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return <-outCh, fnErr
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestInProcInitRecordExploreDispute(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	out, err := captureStdout(t, func() error { return initCtx(nil) })
	if err != nil || !strings.Contains(out, "initialized codedocket at") {
		t.Fatalf("init: out=%q err=%v", out, err)
	}
	if err := initCtx(nil); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second init must refuse: %v", err)
	}

	record := func(extra ...string) (string, error) {
		args := append([]string{"--key", "audit.inproc", "--kind", "fact",
			"--statement", "In-process flow probe.", "--scope", "."}, extra...)
		return captureStdout(t, func() error { return recordCtx(args) })
	}
	out, err = record()
	if err != nil || !strings.Contains(out, "recorded audit.inproc") {
		t.Fatalf("record: out=%q err=%v", out, err)
	}
	out, err = record()
	if err != nil || !strings.Contains(out, "updated audit.inproc (evidence: 2)") {
		t.Fatalf("re-record: out=%q err=%v", out, err)
	}

	out, err = captureStdout(t, func() error {
		return exploreCtx([]string{"--key", "audit.inproc"})
	})
	if err != nil || !strings.Contains(out, "In-process flow probe.") {
		t.Fatalf("explore: out=%q err=%v", out, err)
	}

	out, err = captureStdout(t, func() error {
		return exploreCtx([]string{"--key", "audit.inproc", "--json"})
	})
	if err != nil {
		t.Fatalf("explore --json: %v", err)
	}
	var results []map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(out), &results); jsonErr != nil || len(results) != 1 {
		t.Fatalf("explore --json: len=%d err=%v", len(results), jsonErr)
	}

	out, err = captureStdout(t, func() error {
		return disputeCtx([]string{"--key", "audit.inproc"})
	})
	if err != nil || !strings.Contains(out, "disputed audit.inproc") {
		t.Fatalf("dispute: out=%q err=%v", out, err)
	}
	out, _ = captureStdout(t, func() error {
		return exploreCtx([]string{"--key", "audit.inproc"})
	})
	if !strings.Contains(out, "DISPUTED") {
		t.Fatalf("disputed status not rendered: %q", out)
	}
}

func TestInProcNoteFinalizeHook(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if _, err := captureStdout(t, func() error { return initCtx([]string{"--no-agents"}) }); err != nil {
		t.Fatalf("init: %v", err)
	}

	out, err := captureStdout(t, func() error { return noteCtx([]string{"scratch for inproc"}) })
	if err != nil || !strings.Contains(out, "noted #1 → cli-") {
		t.Fatalf("note: out=%q err=%v", out, err)
	}

	out, err = captureStdout(t, func() error {
		return hookCtx([]string{"stop", "--client", "cursor"})
	})
	if err != nil || !strings.Contains(out, `"followup_message"`) {
		t.Fatalf("hook with pending: out=%q err=%v", out, err)
	}

	out, err = captureStdout(t, func() error { return finalizeCtx(nil) })
	if err != nil || !strings.Contains(out, "finalized cli-") {
		t.Fatalf("finalize: out=%q err=%v", out, err)
	}

	out, err = captureStdout(t, func() error { return finalizeCtx(nil) })
	if err != nil || !strings.Contains(out, "no pending notes") {
		t.Fatalf("finalize empty: out=%q err=%v", out, err)
	}

	out, err = captureStdout(t, func() error {
		return hookCtx([]string{"stop", "--client", "cursor"})
	})
	if err != nil || out != "" {
		t.Fatalf("hook after finalize must be silent: out=%q err=%v", out, err)
	}
	// NB: the unknown-client path calls os.Exit(2) inside hookCtx, so it is
	// asserted only by the subprocess tests in cli_test.go.
}
