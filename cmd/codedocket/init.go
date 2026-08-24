package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/Amaan-Khan14/codedocket"
	"os"
	"path/filepath"
)

func initCtx(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite existing store")
	noAgents := fs.Bool("no-agents", false, "do not touch AGENTS.md")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ctxDir, knowledgeFilePath, err := getDir()
	if err != nil {
		return err
	}
	repoDir := filepath.Dir(ctxDir)

	if _, err := os.Stat(knowledgeFilePath); err == nil {
		if !*force {
			// Refusal protects the store; onboarding is non-destructive
			// and still runs so existing projects get AGENTS.md.
			onboardAgents(repoDir, *noAgents)
			return fmt.Errorf("knowledge store already exists at %s (use --force to overwrite)", knowledgeFilePath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking store: %w", err)
	}

	legacyPath := filepath.Join(repoDir, ".ctx", "knowledge.json")
	if _, err := os.Stat(legacyPath); err == nil && !*force {
		onboardAgents(repoDir, *noAgents)
		return fmt.Errorf("legacy knowledge store already exists at %s; move it to %s or use --force to create a new CodeDocket store", legacyPath, knowledgeFilePath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking legacy store: %w", err)
	}

	store := codedocket.NewStore()

	if err := store.Save(knowledgeFilePath); err != nil {
		return fmt.Errorf("initializing knowledge store: %w", err)
	}
	// Sessions scratch is hygiene, not agent onboarding — always ensured,
	// never fatal (first note re-ensures lazily for pre-M6 stores).
	if err := codedocket.EnsureSessionsGitignore(ctxDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: sessions gitignore not written: %v\n", err)
	}

	fmt.Printf("initialized codedocket at %s\n", ctxDir)
	onboardAgents(repoDir, *noAgents)
	return nil

}

// onboardAgents ensures AGENTS.md carries the codedocket snippet. Failures warn but
// never fail init — the store is the critical artifact.
func onboardAgents(repoDir string, skip bool) {
	if skip {
		return
	}
	outcome, err := ensureAgentsSnippet(repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: AGENTS.md not updated: %v\n", err)
		return
	}
	if outcome != "present" {
		fmt.Printf("AGENTS.md %s with codedocket instructions\n", outcome)
	}
}

// getDir resolves .codedocket in the CURRENT directory only. init intentionally
// never walks up — init creates; codedocket.ResolveStore discovers.
func getDir() (string, string, error) {
	pwdDir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("getting current directory: %w", err)
	}
	ctxDir := filepath.Join(pwdDir, ".codedocket")
	return ctxDir, filepath.Join(ctxDir, "knowledge.json"), nil
}
