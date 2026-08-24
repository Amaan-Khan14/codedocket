package codedocket

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func noteFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, ".codedocket")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	return store
}

func readNotes(t *testing.T, storeDir, sessionID string) sessionFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(storeDir, "sessions", sessionID, "notes.json"))
	if err != nil {
		t.Fatalf("reading notes file: %v", err)
	}
	var sf sessionFile
	if err := json.Unmarshal(b, &sf); err != nil {
		t.Fatalf("parsing notes file: %v", err)
	}
	return sf
}

func TestAppendNoteLazyCreation(t *testing.T) {
	store := noteFixtureDir(t)

	if _, err := os.Stat(filepath.Join(store, "sessions")); err == nil {
		t.Fatal("sessions dir must not exist before first note")
	}

	id, err := FindPendingSession(store, "cli")
	if err != nil || id != "" {
		t.Fatalf("FindPendingSession on empty store = %q, %v", id, err)
	}
	id = NewSessionID("cli")

	if n, err := AppendNote(store, id, "merge is deterministic", []string{"merge.go"}, time.Now()); err != nil || n != 1 {
		t.Fatalf("first note: id=%d err=%v", n, err)
	}

	sf := readNotes(t, store, id)
	if sf.Session != id || len(sf.Notes) != 1 {
		t.Fatalf("session file: %+v", sf)
	}
	if sf.Notes[0].ID != 1 || sf.Notes[0].Text != "merge is deterministic" {
		t.Fatalf("note content: %+v", sf.Notes[0])
	}
	if len(sf.Notes[0].Paths) != 1 || sf.Notes[0].Paths[0] != "merge.go" {
		t.Fatalf("note paths: %v", sf.Notes[0].Paths)
	}

	// gitignore ensured lazily alongside the first note (caller-driven here
	// via EnsureSessionsGitignore, matching the CLI/MCP call order)
	if err := EnsureSessionsGitignore(store); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(store, ".gitignore"))
	if !strings.Contains(string(b), "sessions/") {
		t.Fatalf("gitignore missing sessions/: %q", b)
	}
}

func TestAppendNoteSequentialIDs(t *testing.T) {
	store := noteFixtureDir(t)
	id := NewSessionID("cli")
	for i := 1; i <= 3; i++ {
		n, err := AppendNote(store, id, "note", nil, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if n != i {
			t.Fatalf("note %d got id %d", i, n)
		}
	}
	sf := readNotes(t, store, id)
	if len(sf.Notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(sf.Notes))
	}
}

func TestAppendNoteEmptyTextRejected(t *testing.T) {
	store := noteFixtureDir(t)
	for _, bad := range []string{"", "   ", "\t\n"} {
		if _, err := AppendNote(store, "cli-x", bad, nil, time.Now()); err == nil {
			t.Fatalf("empty text %q must be rejected", bad)
		}
	}
}

func TestFindPendingSessionReuseRule(t *testing.T) {
	store := noteFixtureDir(t)
	sessions := filepath.Join(store, "sessions")

	// Two pending sessions; newest lexicographically wins.
	old := "cli-20260101T000000Z"
	newer := "cli-20260201T000000Z"
	for _, id := range []string{old, newer} {
		if err := os.MkdirAll(filepath.Join(sessions, id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FindPendingSession(store, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Fatalf("expected newest pending %q, got %q", newer, got)
	}

	// Finalized newest is skipped; older pending takes over.
	if err := os.WriteFile(filepath.Join(sessions, newer, "finalized.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = FindPendingSession(store, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if got != old {
		t.Fatalf("expected fallback to %q, got %q", old, got)
	}

	// All finalized → none pending.
	if err := os.WriteFile(filepath.Join(sessions, old, "finalized.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = FindPendingSession(store, "cli")
	if err != nil || got != "" {
		t.Fatalf("expected no pending, got %q, %v", got, err)
	}

	// Different name prefix never matches.
	other := "zcode-20260301T000000Z"
	if err := os.MkdirAll(filepath.Join(sessions, other), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = FindPendingSession(store, "cli")
	if err != nil || got != "" {
		t.Fatalf("foreign prefix must not match, got %q, %v", got, err)
	}
}

func TestSanitizeSessionName(t *testing.T) {
	cases := map[string]string{
		"zcode":         "zcode",
		"Claude Code":   "Claude-Code",
		"../evil/path":  "..-evil-path", // separators collapsed; never "." or ".." once the -timestamp suffix lands
		"a  b":          "a-b",
		"kimi.kiro_1-2": "kimi.kiro_1-2",
	}
	for in, want := range cases {
		if got := sanitizeSessionName(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sanitizeSessionName("!!!"); got != "anon" {
		t.Errorf("empty sanitize must become anon, got %q", got)
	}
	for _, dangerous := range []string{"..", "../x", "/abs", "a/b"} {
		id := NewSessionID(dangerous)
		if strings.ContainsAny(id, "/\\") || id == "." || id == ".." || strings.HasPrefix(id, "/") {
			t.Errorf("NewSessionID(%q) produced unsafe id %q", dangerous, id)
		}
	}
}

func TestNewSessionIDFormat(t *testing.T) {
	id := NewSessionID("cli")
	if !strings.HasPrefix(id, "cli-") {
		t.Fatalf("missing prefix: %s", id)
	}
	ts := strings.TrimPrefix(id, "cli-")
	if len(ts) != 16 { // 20060102T150405Z
		t.Fatalf("timestamp %q is not compact-UTC format", ts)
	}
}

func TestEnsureSessionsGitignoreIdempotent(t *testing.T) {
	store := noteFixtureDir(t)

	if err := EnsureSessionsGitignore(store); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(store, ".gitignore"))

	if err := EnsureSessionsGitignore(store); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(store, ".gitignore"))
	if string(first) != string(second) {
		t.Fatalf("not idempotent:\n%q\n%q", first, second)
	}

	// Foreign content preserved, sessions/ appended once.
	foreign := []byte("*.lock\n")
	if err := os.WriteFile(filepath.Join(store, ".gitignore"), foreign, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSessionsGitignore(store); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(store, ".gitignore"))
	if !strings.Contains(string(got), "*.lock") || strings.Count(string(got), "sessions/") != 1 {
		t.Fatalf("foreign content mishandled: %q", got)
	}
}
