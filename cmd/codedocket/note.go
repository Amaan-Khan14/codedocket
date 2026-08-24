package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	codedocket "github.com/Amaan-Khan14/codedocket"
)

// noteCtx is the cheap-capture path (M6): one free-text argument, zero
// structure. Notes go to per-session scratch and are reviewed later via
// `codedocket finalize`.
func noteCtx(args []string) error {
	fs := flag.NewFlagSet("note", flag.ExitOnError)
	paths := fs.String("path", "", "comma-separated paths this note relates to")
	session := fs.String("session", "cli", "session name (groups notes until finalized)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: codedocket note [--path a,b] [--session name] \"text\" (flags before text)")
	}
	text := fs.Arg(0)
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("note text is required")
	}

	knowledgePath, err := mustStore()
	if err != nil {
		return err
	}
	storeDir := filepath.Dir(knowledgePath)

	if err := codedocket.EnsureSessionsGitignore(storeDir); err != nil {
		return fmt.Errorf("ensuring sessions gitignore: %w", err)
	}

	// Reuse the newest pending session for this name — many CLI
	// invocations, one logical working session.
	id, err := codedocket.FindPendingSession(storeDir, *session)
	if err != nil {
		return err
	}
	if id == "" {
		id = codedocket.NewSessionID(*session)
	}

	var notePaths []string
	if *paths != "" {
		for _, p := range strings.Split(*paths, ",") {
			if p = strings.TrimSpace(p); p != "" {
				notePaths = append(notePaths, p)
			}
		}
	}

	noteID, err := codedocket.AppendNote(storeDir, id, text, notePaths, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("noted #%d → %s\n", noteID, id)
	return nil
}
