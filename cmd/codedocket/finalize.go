package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	codedocket "github.com/Amaan-Khan14/codedocket"
)

// finalizeCtx is the consolidation moment (M6): render pending scratch as
// numbered proposals + git evidence + review instructions, then mark the
// session finalized. A report, not an error, when nothing is pending.
func finalizeCtx(args []string) error {
	fs := flag.NewFlagSet("finalize", flag.ExitOnError)
	session := fs.String("session", "", "specific session id to review")
	all := fs.Bool("all", false, "review every pending session")
	reopen := fs.Bool("reopen", false, "un-finalize a session (requires --session)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	knowledgePath, err := mustStore()
	if err != nil {
		return err
	}
	storeDir := filepath.Dir(knowledgePath)
	sessions, err := codedocket.ListSessions(storeDir)
	if err != nil {
		return err
	}

	if *reopen {
		return reopenSession(sessions, *session)
	}

	var targets []codedocket.SessionInfo
	switch {
	case *session != "":
		si, ok := findSession(sessions, *session)
		if !ok {
			return fmt.Errorf("unknown session %q", *session)
		}
		targets = append(targets, si)
	case *all:
		for _, s := range sessions {
			if !s.Finalized {
				targets = append(targets, s)
			}
		}
	default:
		// Newest pending (ids sort by embedded timestamp).
		for i := len(sessions) - 1; i >= 0; i-- {
			if !sessions[i].Finalized {
				targets = append(targets, sessions[i])
				break
			}
		}
	}

	if len(targets) == 0 {
		fmt.Println("no pending notes")
	}

	for _, s := range targets {
		notes, err := codedocket.LoadSessionNotes(storeDir, s.ID)
		if err != nil {
			return err
		}
		var since time.Time
		gitEvidence := ""
		if len(notes) > 0 {
			since = notes[0].At // never mtime — appends touch it
			gitEvidence = codedocket.GitEvidence(filepath.Dir(storeDir), since)
		}
		fmt.Print(codedocket.RenderSession(s.ID, notes, since, gitEvidence))
		if err := codedocket.MarkFinalized(s.Dir, len(notes), time.Now()); err != nil {
			return fmt.Errorf("marking %s finalized: %w", s.ID, err)
		}
		fmt.Printf("finalized %s (%d notes)\n\n", s.ID, len(notes))
	}

	// Safety net: surface other pending sessions the agent didn't just
	// review (stale captures, hook-less clients).
	if !*all {
		reviewed := make(map[string]bool)
		for _, s := range targets {
			reviewed[s.ID] = true
		}
		others := 0
		for _, s := range sessions {
			if !s.Finalized && !reviewed[s.ID] {
				others++
			}
		}
		if others > 0 {
			fmt.Printf("%d other pending session(s) — run 'codedocket finalize --all' to review them\n", others)
		}
	}

	// Follow-through footnote: finalized-but-zero-records join (read-only).
	if store, err := codedocket.Load(knowledgePath); err == nil {
		for _, s := range codedocket.UnreviewedSessions(store, sessions, codedocket.ScratchRetention, time.Now()) {
			fmt.Printf("⚠ %s: %d notes never recorded — re-review with 'codedocket finalize --session %s --reopen', or ignore if intentional\n",
				s.ID, s.Notes, s.ID)
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: footnote join skipped, store unreadable: %v\n", err)
	}

	// Retention pruning: finalized + older than the window.
	if pruned, err := codedocket.PruneFinalized(sessions, codedocket.ScratchRetention, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: prune incomplete: %v\n", err)
	} else if len(pruned) > 0 {
		fmt.Printf("pruned %d finalized session(s) older than %s\n", len(pruned), codedocket.ScratchRetention)
	}
	return nil
}

func findSession(sessions []codedocket.SessionInfo, id string) (codedocket.SessionInfo, bool) {
	for _, s := range sessions {
		if s.ID == id {
			return s, true
		}
	}
	return codedocket.SessionInfo{}, false
}

func reopenSession(sessions []codedocket.SessionInfo, id string) error {
	if id == "" {
		return fmt.Errorf("--reopen requires --session")
	}
	si, ok := findSession(sessions, id)
	if !ok {
		return fmt.Errorf("unknown session %q", id)
	}
	if !si.Finalized {
		fmt.Printf("session %s is already pending\n", id)
		return nil
	}
	if err := codedocket.ReopenSession(si.Dir); err != nil {
		return err
	}
	fmt.Printf("reopened %s; the next finalize will review its %d notes\n", id, si.Notes)
	return nil
}
