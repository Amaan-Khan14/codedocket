package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAgentsSnippetCreates(t *testing.T) {
	dir := t.TempDir()

	outcome, err := ensureAgentsSnippet(dir)
	if err != nil {
		t.Fatalf("ensureAgentsSnippet: %v", err)
	}
	if outcome != "created" {
		t.Fatalf("outcome = %q, want created", outcome)
	}

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{markerBegin, markerEnd, "codedocket_explore", "codedocket_record", "knowledge.json", "supersedes"} {
		if !strings.Contains(s, want) {
			t.Errorf("snippet missing %q", want)
		}
	}
}

func TestEnsureAgentsSnippetAppendsAndPreserves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	original := "# Existing team notes\n\n- use prettier\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, err := ensureAgentsSnippet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != "appended" {
		t.Fatalf("outcome = %q, want appended", outcome)
	}

	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.HasPrefix(s, original) {
		t.Errorf("existing content not preserved:\n%s", s)
	}
	if !strings.Contains(s, markerBegin) {
		t.Error("snippet not appended")
	}
}

func TestEnsureAgentsSnippetIdempotent(t *testing.T) {
	dir := t.TempDir()

	if _, err := ensureAgentsSnippet(dir); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))

	outcome, err := ensureAgentsSnippet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != "present" {
		t.Fatalf("second run outcome = %q, want present", outcome)
	}
	second, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(first) != string(second) {
		t.Fatal("second run modified the file")
	}
}

func TestEnsureAgentsSnippetEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureAgentsSnippet(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), markerBegin) {
		t.Errorf("empty file should become snippet-only, got prefix:\n%q", string(data)[:80])
	}
}

func TestEnsureAgentsSnippetReplacesStaleBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	// A repo onboarded before the v2 snippet: old block between markers,
	// human content on both sides.
	oldBlock := markerBegin + `
## Project knowledge (codedocket)

OLD v1 CONTENT — no note/finalize section.
` + markerEnd + "\n"
	before := "# Team notes\n\nKeep this.\n\n"
	after := "\n## Below the snippet\n\nAlso keep this.\n"
	if err := os.WriteFile(path, []byte(before+oldBlock+after), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, err := ensureAgentsSnippet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != "updated" {
		t.Fatalf("outcome = %q, want updated", outcome)
	}

	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.HasPrefix(s, before) || !strings.HasSuffix(s, after) {
		t.Fatalf("surrounding content not preserved:\n%q", s)
	}
	if !strings.Contains(s, "codedocket_note") || !strings.Contains(s, "codedocket finalize") {
		t.Error("upgraded snippet missing the v2 note/finalize section")
	}
	if strings.Contains(s, "OLD v1 CONTENT") {
		t.Error("stale snippet content survived the upgrade")
	}
	if strings.Count(s, markerBegin) != 1 || strings.Count(s, markerEnd) != 1 {
		t.Error("marker duplication")
	}

	// Second run on the upgraded file is a no-op.
	outcome2, err := ensureAgentsSnippet(dir)
	if err != nil || outcome2 != "present" {
		t.Fatalf("second run = %q, %v; want present", outcome2, err)
	}
	data2, _ := os.ReadFile(path)
	if string(data) != string(data2) {
		t.Fatal("second run modified the file")
	}
}

func TestEnsureAgentsSnippetMalformedMarkersUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	broken := "# Notes\n\n" + markerBegin + "\nunterminated block\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, err := ensureAgentsSnippet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != "present" {
		t.Fatalf("outcome = %q, want present (never corrupt malformed files)", outcome)
	}
	data, _ := os.ReadFile(path)
	if string(data) != broken {
		t.Fatal("malformed file must be left byte-identical")
	}
}
