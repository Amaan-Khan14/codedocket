package main

import (
	"flag"
	"fmt"
	"github.com/Amaan-Khan14/codedocket"
	"time"
)

func disputeCtx(args []string) error {
	fs := flag.NewFlagSet("dispute", flag.ExitOnError)

	key := fs.String("key", "", "stable key")
	note := fs.String("note", "", "optional note")
	session := fs.String("session", "cli", "session id")

	fs.Usage = func() {
		fmt.Println("dispute --key <key> [--note <text>] [--session <session>]")
	}

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("error parsing flags: %w", err)
	}

	if *key == "" {
		return fmt.Errorf("key is required")
	}

	knowledgeFilePath, err := mustStore()
	if err != nil {
		return err
	}

	// Load→dispute→save under the store lock (same discipline as record).
	if err := codedocket.Update(knowledgeFilePath, func(store *codedocket.Store) error {
		_, err := codedocket.Dispute(store, *key, *session, *note, time.Now())
		return err
	}); err != nil {
		return err
	}

	fmt.Printf("disputed %s\n", *key)

	return nil
}
