package codedocket

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var finT0 = time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)

func finalizeFixture(t *testing.T, sessions ...SessionInfo) string {
	t.Helper()
	store := noteFixtureDir(t)
	for _, s := range sessions {
		dir := filepath.Join(store, "sessions", s.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if s.Notes > 0 {
			notes := make([]Note, s.Notes)
			for i := range notes {
				notes[i] = Note{ID: i + 1, At: finT0.Add(time.Duration(i) * time.Minute), Text: "note"}
			}
			if err := saveSessionFile(filepath.Join(dir, "notes.json"), &sessionFile{Session: s.ID, Notes: notes}); err != nil {
				t.Fatal(err)
			}
		}
		if s.Finalized {
			if err := MarkFinalized(dir, s.Notes, s.FinalizedAt); err != nil {
				t.Fatal(err)
			}
		}
	}
	return store
}

func TestListSessionsSortedWithState(t *testing.T) {
	store := finalizeFixture(t,
		SessionInfo{ID: "cli-20260102T000000Z", Notes: 2},
		SessionInfo{ID: "cli-20260101T000000Z", Notes: 1, Finalized: true, FinalizedAt: finT0},
	)
	got, err := ListSessions(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
	if got[0].ID != "cli-20260101T000000Z" || got[1].ID != "cli-20260102T000000Z" {
		t.Fatalf("not sorted ascending: %+v", got)
	}
	if got[0].Finalized != true || got[1].Finalized != false {
		t.Fatalf("finalized flags wrong: %+v", got)
	}
	if got[1].Notes != 2 || !got[1].FirstAt.Equal(finT0) {
		t.Fatalf("note metadata wrong: %+v", got[1])
	}
}

func TestMarkFinalizedAndReopen(t *testing.T) {
	store := finalizeFixture(t, SessionInfo{ID: "cli-x", Notes: 3})
	dir := filepath.Join(store, "sessions", "cli-x")

	at := finT0.Add(time.Hour)
	if err := MarkFinalized(dir, 3, at); err != nil {
		t.Fatal(err)
	}
	if !IsFinalized(dir) {
		t.Fatal("marker missing after MarkFinalized")
	}
	b, _ := os.ReadFile(filepath.Join(dir, finalizedFile))
	if !strings.Contains(string(b), `"notes": 3`) {
		t.Fatalf("marker content: %s", b)
	}

	if err := ReopenSession(dir); err != nil {
		t.Fatal(err)
	}
	if IsFinalized(dir) {
		t.Fatal("marker still present after reopen")
	}
	if err := ReopenSession(dir); err == nil {
		t.Fatal("reopening a pending session must error")
	}
}

func TestRenderSessionDeterministic(t *testing.T) {
	notes := []Note{
		{ID: 1, At: finT0, Text: "merge never infers", Paths: []string{"merge.go"}},
		{ID: 2, At: finT0.Add(time.Minute), Text: "evidence only ranks"},
	}
	git := "abc1234 fix merge\n 2 files changed"

	a := RenderSession("cli-20260822T100000Z", notes, finT0, git)
	b := RenderSession("cli-20260822T100000Z", notes, finT0, git)
	if a != b {
		t.Fatalf("renderer not deterministic:\n%q\n%q", a, b)
	}

	for _, want := range []string{
		"#1 [merge.go] merge never infers",
		"#2 evidence only ranks",
		"## git activity since 2026-08-22T10:00:00Z",
		"abc1234 fix merge",
		"--session cli-20260822T100000Z",
		"explore --query",
	} {
		if !strings.Contains(a, want) {
			t.Errorf("render missing %q", want)
		}
	}

	// Empty git evidence renders the absent-section form, deterministically.
	c := RenderSession("cli-20260822T100000Z", notes[:1], finT0, "")
	if !strings.Contains(c, "(none available)") {
		t.Errorf("empty git evidence must render absent section:\n%s", c)
	}
}

func TestUnreviewedSessionsJoin(t *testing.T) {
	now := finT0.Add(2 * 24 * time.Hour) // 2 days after finalization
	recent := SessionInfo{ID: "cli-recent", Notes: 2, Finalized: true, FinalizedAt: finT0.Add(time.Hour)}
	old := SessionInfo{ID: "cli-old", Notes: 2, Finalized: true, FinalizedAt: finT0.Add(-20 * 24 * time.Hour)}
	pending := SessionInfo{ID: "cli-pend", Notes: 1}
	empty := SessionInfo{ID: "cli-empty", Notes: 0, Finalized: true, FinalizedAt: finT0.Add(time.Hour)}
	undatable := SessionInfo{ID: "cli-undated", Notes: 1, Finalized: true} // zero FinalizedAt

	sessions := []SessionInfo{recent, old, pending, empty, undatable}

	t.Run("no evidence at all → only recent-with-notes flagged", func(t *testing.T) {
		store := NewStore()
		got := UnreviewedSessions(store, sessions, ScratchRetention, now)
		if len(got) != 1 || got[0].ID != recent.ID {
			t.Fatalf("expected only %s, got %+v", recent.ID, got)
		}
	})

	t.Run("evidence join suppresses the flag", func(t *testing.T) {
		store := NewStore()
		if _, _, err := Record(store, RecordInput{
			Key: "x.y", Kind: "fact", Statement: "s", Scope: []string{"."},
			Session: recent.ID,
		}, now); err != nil {
			t.Fatal(err)
		}
		got := UnreviewedSessions(store, sessions, ScratchRetention, now)
		if len(got) != 0 {
			t.Fatalf("evidence present must suppress footnote, got %+v", got)
		}
	})
}

func TestPruneFinalizedRequiresBothConditions(t *testing.T) {
	store := finalizeFixture(t,
		SessionInfo{ID: "pr-me", Notes: 1, Finalized: true, FinalizedAt: finT0.Add(-20 * 24 * time.Hour)},   // old + finalized → pruned
		SessionInfo{ID: "keep-new", Notes: 1, Finalized: true, FinalizedAt: finT0.Add(-2 * 24 * time.Hour)}, // recent finalized
		SessionInfo{ID: "keep-pend", Notes: 1}, // old pending never pruned (no marker)
	)
	sessions, err := ListSessions(store)
	if err != nil {
		t.Fatal(err)
	}

	removed, err := PruneFinalized(sessions, ScratchRetention, finT0)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "pr-me" {
		t.Fatalf("pruned = %v, want [pr-me]", removed)
	}
	if _, err := os.Stat(filepath.Join(store, "sessions", "keep-new")); err != nil {
		t.Fatal("recent finalized session must survive")
	}
	if _, err := os.Stat(filepath.Join(store, "sessions", "keep-pend")); err != nil {
		t.Fatal("pending session must never be pruned")
	}
}

func TestCapLines(t *testing.T) {
	in := strings.Repeat("line\n", 300)
	out := capLines(in, 200)
	if got := len(strings.Split(out, "\n")); got != 201 { // 200 lines + truncated marker
		t.Fatalf("capped line count = %d", got)
	}
	if !strings.Contains(out, "(truncated)") {
		t.Fatal("missing truncation marker")
	}
	small := "a\nb\n"
	if got := capLines(small, 200); got != "a\nb" {
		t.Fatalf("under-cap input must pass through trimmed: %q", got)
	}
}

func TestGitEvidenceNotARepo(t *testing.T) {
	if got := GitEvidence(t.TempDir(), finT0); got != "" {
		t.Fatalf("non-repo must yield empty evidence, got %q", got)
	}
}
