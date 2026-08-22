package codedocket

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// finalize (M6 Task 2): the consolidation renderer. It gathers scratch
// notes + git evidence and renders numbered proposals plus one review
// instruction for the still-active agent. It is a pure renderer — never an
// LLM, never a writer of knowledge.json, never an improver of proposals.
// The only store access is the read-only follow-through join.

// ScratchRetention bounds how long finalized scratch survives before
// pruning. Pending sessions are never pruned (sessions.scratch-lifecycle).
const ScratchRetention = 14 * 24 * time.Hour

// marker is the on-disk shape of finalized.json. It certifies exactly one
// thing: "finalize rendered these proposals at <at>" (sessions.finalized-basis).
type marker struct {
	At    time.Time `json:"at"`
	Notes int       `json:"notes"`
}

// SessionInfo describes one session dir for listing, the footnote join,
// and pruning.
type SessionInfo struct {
	ID          string
	Dir         string
	Finalized   bool
	Notes       int
	FirstAt     time.Time // notes[0].At; zero when no notes
	FinalizedAt time.Time // marker at; zero when missing/unreadable
}

// ListSessions returns every session under storeDir sorted by ID
// (ascending; ids embed a sortable timestamp, so newest = last).
func ListSessions(storeDir string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(sessionsDir(storeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	var out []SessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		dir := filepath.Join(sessionsDir(storeDir), id)
		si := SessionInfo{ID: id, Dir: dir}
		if notes, err := LoadSessionNotes(storeDir, id); err == nil && len(notes) > 0 {
			si.Notes = len(notes)
			si.FirstAt = notes[0].At
		}
		if b, err := os.ReadFile(filepath.Join(dir, finalizedFile)); err == nil {
			var m marker
			if json.Unmarshal(b, &m) == nil {
				si.Finalized = true
				si.FinalizedAt = m.At
			} else {
				si.Finalized = true // unreadable marker still means finalized; undatable stays conservative
			}
		}
		out = append(out, si)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// MarkFinalized writes the finalized marker as finalize's last step for a
// session with n notes.
func MarkFinalized(sessionDir string, n int, at time.Time) error {
	b, err := json.MarshalIndent(marker{At: at, Notes: n}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(filepath.Join(sessionDir, finalizedFile), b, 0o644)
}

// ReopenSession deletes the finalized marker, returning the session to
// pending (the explicit un-finalize path for re-review).
func ReopenSession(sessionDir string) error {
	if err := os.Remove(filepath.Join(sessionDir, finalizedFile)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session is not finalized")
		}
		return err
	}
	return nil
}

// UnreviewedSessions returns finalized sessions that are within the
// retention window, hold at least one note, and produced zero store
// evidence for their id — the deterministic follow-through join
// (sessions.finalized-basis). No inference: both sides are facts already
// held. Undatable markers are conservatively skipped.
func UnreviewedSessions(store *Store, sessions []SessionInfo, retention time.Duration, now time.Time) []SessionInfo {
	var out []SessionInfo
	for _, s := range sessions {
		if !s.Finalized || s.FinalizedAt.IsZero() || s.Notes == 0 {
			continue
		}
		if now.Sub(s.FinalizedAt) > retention {
			continue
		}
		if evidenceForSession(store, s.ID) > 0 {
			continue
		}
		out = append(out, s)
	}
	return out
}

// PruneFinalized removes session dirs that are BOTH finalized AND older
// than the retention window (marker time). Pending sessions are never
// pruned; undatable markers are never pruned. Returns the removed ids.
func PruneFinalized(sessions []SessionInfo, retention time.Duration, now time.Time) ([]string, error) {
	var removed []string
	for _, s := range sessions {
		if !s.Finalized || s.FinalizedAt.IsZero() {
			continue
		}
		if now.Sub(s.FinalizedAt) <= retention {
			continue
		}
		if err := os.RemoveAll(s.Dir); err != nil {
			return removed, fmt.Errorf("pruning %s: %w", s.ID, err)
		}
		removed = append(removed, s.ID)
	}
	return removed, nil
}

func evidenceForSession(store *Store, sessionID string) int {
	if store == nil {
		return 0
	}
	n := 0
	for _, k := range store.Knowledge {
		for _, ev := range k.Evidence {
			if ev.Session == sessionID {
				n++
			}
		}
	}
	return n
}

// GitEvidence returns capped `git log --oneline --stat` output since the
// given time (the window starts at notes[0].At — file mtime lies because
// every append touches it). Not a git repo or git failure → "" (the
// section renders as absent; finalize never fails on this).
func GitEvidence(repoDir string, since time.Time) string {
	cmd := exec.Command("git", "-C", repoDir,
		"log", "--oneline", "--stat",
		"--since", since.UTC().Format(time.RFC3339),
		"-n", "30")
	out, err := cmd.Output() // stderr intentionally ignored
	if err != nil {
		return ""
	}
	return capLines(string(out), 200)
}

// capLines truncates s to at most max lines, marking the cut.
func capLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return strings.TrimRight(s, "\n")
	}
	return strings.Join(lines[:max], "\n") + "\n… (truncated)"
}

// RenderSession renders one session's review prompt. Pure: same inputs →
// identical bytes, every time. The copy is shared with the Stop-hook
// reason and the AGENTS.md snippet (Tasks 3/5) — edit with care.
func RenderSession(id string, notes []Note, since time.Time, gitEvidence string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "codedocket finalize — session %s (%d notes)\n\n", id, len(notes))

	b.WriteString("## proposals\n\n")
	for _, n := range notes {
		if len(n.Paths) > 0 {
			fmt.Fprintf(&b, "  #%d [%s] %s\n", n.ID, strings.Join(n.Paths, ", "), n.Text)
		} else {
			fmt.Fprintf(&b, "  #%d %s\n", n.ID, n.Text)
		}
	}
	if len(notes) == 0 {
		b.WriteString("  (none)\n")
	}

	b.WriteString("\n## git activity since " + since.UTC().Format(time.RFC3339) + "\n\n")
	if gitEvidence == "" {
		b.WriteString("  (none available)\n")
	} else {
		for _, line := range strings.Split(gitEvidence, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	b.WriteString(`
## review instructions

For each proposal above, decide: record it or skip it.

To record (judge and structure it yourself — the note text is raw):
  codedocket record --key <topic-slug> --kind <kind> \
    --statement "<your 1-2 sentence position>" --scope <paths> \
    --session ` + id + ` --note "<the note text>"
First run codedocket explore --query <topic> to avoid duplicates:
same key confirms/updates an entry; a new key with --supersedes replaces one.

Skip deliberately when a note is transient (build output, routine edits,
already-captured facts). Do not invent knowledge this session didn't
produce. The note text goes in --note — that provenance is what survives.
`)
	return b.String()
}
