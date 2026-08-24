package codedocket

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Session scratch capture (M6 Task 1). Notes are cheap, unstructured
// observations written during work and reviewed later via `finalize`.
// They live under <store>/sessions/<id>/notes.json — gitignored scratch,
// never part of knowledge.json. The only validation is non-empty text.

// Note is a single captured observation. IDs are sequential per session;
// capture order IS review order, so the array is never sorted.
type Note struct {
	ID    int       `json:"id"`
	At    time.Time `json:"at"`
	Text  string    `json:"text"`
	Paths []string  `json:"paths,omitempty"`
}

// sessionFile is the on-disk shape of a session's scratch file.
type sessionFile struct {
	Session string `json:"session"`
	Notes   []Note `json:"notes"`
}

// finalizedFile names the file whose presence marks a session as finalized
// (written by `finalize`; existence is all pending-detection needs).
const finalizedFile = "finalized.json"

// NewSessionID returns "<name>-<UTC compact timestamp>" — sortable and
// filesystem-safe. The timestamp is second-granular; two agents sharing a
// sanitized name within the same second collide (accepted V1 limitation).
func NewSessionID(name string) string {
	return sanitizeSessionName(name) + "-" + time.Now().UTC().Format("20060102T150405Z")
}

// sanitizeSessionName keeps [a-zA-Z0-9._-] and collapses everything else
// (spaces, path separators, unicode) into '-'. Empty results become "anon".
func sanitizeSessionName(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "anon"
	}
	return out
}

// FindPendingSession returns the newest (lexicographic max) pending session
// id with prefix "<name>-", or "" when none exists. Finalized sessions are
// skipped: their review already happened.
func FindPendingSession(storeDir, name string) (string, error) {
	prefix := sanitizeSessionName(name) + "-"
	entries, err := os.ReadDir(sessionsDir(storeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("listing sessions: %w", err)
	}
	best := ""
	for _, e := range entries {
		id := e.Name()
		if !e.IsDir() || !strings.HasPrefix(id, prefix) {
			continue
		}
		if best == "" || id > best {
			if isFinalized(filepath.Join(sessionsDir(storeDir), id)) {
				continue
			}
			best = id
		}
	}
	return best, nil
}

// AppendNote appends a note to session sessionID under storeDir, creating
// the session lazily on first use. Returns the note's sequential ID. The
// caller chooses the id policy: CLI reuses the newest pending session
// (FindPendingSession), MCP uses its initialize-time session id.
func AppendNote(storeDir, sessionID, text string, paths []string, at time.Time) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("note text is required")
	}
	dir := filepath.Join(sessionsDir(storeDir), sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("creating session dir: %w", err)
	}
	sf, err := loadSessionFile(filepath.Join(dir, "notes.json"), sessionID)
	if err != nil {
		return 0, err
	}
	n := Note{ID: len(sf.Notes) + 1, At: at, Text: text, Paths: paths}
	sf.Notes = append(sf.Notes, n)
	if err := saveSessionFile(filepath.Join(dir, "notes.json"), sf); err != nil {
		return 0, err
	}
	return n.ID, nil
}

// IsFinalized reports whether a session dir carries the finalized marker.
func IsFinalized(sessionDir string) bool {
	return isFinalized(sessionDir)
}

func isFinalized(sessionDir string) bool {
	_, err := os.Stat(filepath.Join(sessionDir, finalizedFile))
	return err == nil
}

// LoadSessionNotes returns the notes of one session (empty when none
// captured yet).
func LoadSessionNotes(storeDir, sessionID string) ([]Note, error) {
	sf, err := loadSessionFile(filepath.Join(sessionsDir(storeDir), sessionID, "notes.json"), sessionID)
	if err != nil {
		return nil, err
	}
	return sf.Notes, nil
}

// EnsureSessionsGitignore guarantees <storeDir>/.gitignore ignores sessions/.
// Idempotent: appends only when the entry is absent, never clobbers foreign
// content. Called by init and lazily by note (covers pre-M6 stores).
func EnsureSessionsGitignore(storeDir string) error {
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return fmt.Errorf("creating store dir: %w", err)
	}
	path := filepath.Join(storeDir, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == "sessions/" {
			return nil
		}
	}
	out := existing
	if len(out) > 0 && !strings.HasSuffix(string(out), "\n") {
		out = append(out, '\n')
	}
	out = append(out, []byte("sessions/\n")...)
	return os.WriteFile(path, out, 0o644)
}

// --- file plumbing (atomic, same discipline as the store) ---

func sessionsDir(storeDir string) string {
	return filepath.Join(storeDir, "sessions")
}

func loadSessionFile(path, sessionID string) (*sessionFile, error) {
	sf := &sessionFile{Session: sessionID}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return sf, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading notes: %w", err)
	}
	if err := json.Unmarshal(b, sf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if sf.Notes == nil {
		sf.Notes = []Note{}
	}
	return sf, nil
}

func saveSessionFile(path string, sf *sessionFile) error {
	b, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding notes: %w", err)
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".notes-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("closing temp file: %w", err)
	}
	_ = os.Chmod(tmp.Name(), 0o644)
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("replacing notes: %w", err)
	}
	return nil
}
