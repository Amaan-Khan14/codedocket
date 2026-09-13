package codedocket

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func lockedStoreFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge.json")
	if err := NewStore().Save(path); err != nil {
		t.Fatal(err)
	}
	return path
}

// Update must return the no-store error instead of creating a stray lock
// file for a path whose store directory does not exist.
func TestUpdateWithoutStoreErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "knowledge.json")
	err := Update(path, func(*Store) error { return nil })
	if err == nil {
		t.Fatal("expected error for missing store, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(path), ".knowledge.lock")); statErr == nil {
		t.Fatal("lock file created for nonexistent store dir")
	}
}

// The regression test for the lost-update bug: N concurrent Update calls
// each record a unique key, and every key must survive. Before the store
// lock this was last-writer-wins — goroutines that loaded the same snapshot
// silently dropped each other's records on Save.
func TestUpdateConcurrentRecordsAllSurvive(t *testing.T) {
	path := lockedStoreFixture(t)
	const writers = 16

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("audit.writer-%02d", i)
			err := Update(path, func(s *Store) error {
				_, _, err := Record(s, RecordInput{
					Key:       key,
					Kind:      "fact",
					Statement: "concurrent writer probe",
					Scope:     []string{"."},
					Session:   fmt.Sprintf("writer-%02d", i),
				}, time.Now())
				return err
			})
			if err != nil {
				errs <- fmt.Errorf("writer %d: %w", i, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	final, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < writers; i++ {
		key := fmt.Sprintf("audit.writer-%02d", i)
		k, ok := final.Knowledge[key]
		if !ok {
			t.Fatalf("lost update: key %q missing from final store (%d entries)", key, len(final.Knowledge))
		}
		if k.Status != StatusActive {
			t.Fatalf("key %q has status %q, want active", key, k.Status)
		}
	}
}

// The lock file must never land in git: EnsureStoreGitignore ignores both
// the session scratch dir and .knowledge.lock, including on pre-existing
// gitignores that only cover the old entries.
func TestStoreGitignoreCoversLockFile(t *testing.T) {
	store := noteFixtureDir(t)

	if err := EnsureStoreGitignore(store); err != nil {
		t.Fatal(err)
	}
	gitignore, err := os.ReadFile(filepath.Join(store, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"sessions/", ".knowledge.lock"} {
		if !containsLine(string(gitignore), entry) {
			t.Fatalf(".gitignore missing %q:\n%s", entry, gitignore)
		}
	}
}

func containsLine(s, line string) bool {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}
