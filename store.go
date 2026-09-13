package codedocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Load reads knowledge.json from path.
func Load(path string) (*Store, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no knowledge store at %s (run `codedocket init`)", path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading store: %w", err)
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if s.Version < 1 || s.Version > CurrentVersion {
		return nil, fmt.Errorf("unsupported store version %d (this build understands up to %d)", s.Version, CurrentVersion)
	}
	if s.Knowledge == nil {
		s.Knowledge = map[string]*Knowledge{}
	}
	if s.Edges == nil {
		s.Edges = []Edge{}
	}
	return &s, nil
}

// Update applies mutate to the store and saves it, holding an exclusive
// advisory lock on <storeDir>/.knowledge.lock for the whole read-modify-write.
// It serializes writers across processes — each MCP client spawns its own
// `serve` process, and ad-hoc CLI calls race against them — so a record can
// no longer be silently dropped by last-writer-wins. Save alone stays
// lock-free; every Load→mutate→Save sequence must go through Update.
func Update(path string, mutate func(*Store) error) error {
	lock, err := acquireStoreLock(path)
	if err != nil {
		return err
	}
	defer lock.release()

	s, err := Load(path)
	if err != nil {
		return err
	}
	if err := mutate(s); err != nil {
		return err
	}
	return s.Save(path)
}

// storeLock owns the open lock file for one Update critical section.
type storeLock struct{ f *os.File }

// acquireStoreLock opens (creating if needed) the lock file next to the
// knowledge store and blocks until it holds the exclusive lock. The file is
// deliberately never unlinked: unlock-then-remove races with a waiter that
// still holds a fd on the removed inode while a third writer locks a fresh
// file, breaking mutual exclusion. A permanent .knowledge.lock is harmless —
// EnsureStoreGitignore keeps it out of git, and the OS releases the lock when
// the process exits, so crashed writers leave no stale lock behind.
func acquireStoreLock(knowledgePath string) (*storeLock, error) {
	lockPath := filepath.Join(filepath.Dir(knowledgePath), ".knowledge.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	if err := lockExclusive(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("locking %s: %w", lockPath, err)
	}
	return &storeLock{f: f}, nil
}

func (l *storeLock) release() {
	if l == nil || l.f == nil {
		return
	}
	_ = unlockExclusive(l.f)
	_ = l.f.Close()
}

// Save writes the store atomically: temp file in the target directory, then
// rename. JSON map keys marshal in sorted order, so output is diff-stable
// and git is the history of record. Atomicity here only prevents torn files —
// read-modify-write sequences must hold the store lock (see Update) or two
// writers will still lose each other's changes.
func (s *Store) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating store dir: %w", err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding store: %w", err)
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(dir, ".knowledge-*.tmp")
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
		return fmt.Errorf("replacing store: %w", err)
	}
	return nil
}

// ErrNoStore is returned by ResolveStore when no knowledge store is found
// walking up from the working directory.
var ErrNoStore = errors.New("no codedocket store found")

// ResolveStore walks up from the current directory (git-style) looking for
// .codedocket/knowledge.json, falling back to legacy .ctx/knowledge.json for
// pre-rename projects. The single resolver shared by the CLI and the MCP
// server. Note: `codedocket init` does NOT walk up — it creates in cwd only.
func ResolveStore() (storeDir, knowledgePath string, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("getting current directory: %w", err)
	}
	for {
		for _, name := range []string{".codedocket", ".ctx"} {
			storeDir = filepath.Join(dir, name)
			knowledgePath = filepath.Join(storeDir, "knowledge.json")

			info, err := os.Stat(knowledgePath)
			if err == nil && !info.IsDir() {
				return storeDir, knowledgePath, nil
			}
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", "", fmt.Errorf("checking %s: %w", knowledgePath, err)
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", ErrNoStore
		}
		dir = parent
	}
}
