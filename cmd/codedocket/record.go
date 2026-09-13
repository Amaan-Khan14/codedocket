package main

import (
	"flag"
	"fmt"
	"github.com/Amaan-Khan14/codedocket"
	"strings"
	"time"
)

func recordCtx(args []string) error {
	fs := flag.NewFlagSet("record", flag.ExitOnError)

	key := fs.String("key", "", "stable key")
	kind := fs.String("kind", "", "decision kind")
	statement := fs.String("statement", "", "statement")
	scope := fs.String("scope", "", "comma-separated scopes")
	supersedes := fs.String("supersedes", "", "comma-separated keys")
	session := fs.String("session", "cli", "session id")
	note := fs.String("note", "", "optional note")

	fs.Usage = func() {
		fmt.Println("record --key <key> --kind <kind> --statement <statement> --scope <scope> [--supersedes <k1,k2>] [--session <session>] [--note <text>]")
	}

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("error parsing flags: %w", err)
	}

	if *key == "" {
		return fmt.Errorf("key is required")
	}
	if *kind == "" {
		return fmt.Errorf("kind is required")
	}
	if *statement == "" {
		return fmt.Errorf("statement is required")
	}
	if *scope == "" {
		return fmt.Errorf("scope is required")
	}

	knowledgeFilePath, err := mustStore()
	if err != nil {
		return err
	}

	in := codedocket.RecordInput{
		Key:        *key,
		Kind:       *kind,
		Statement:  *statement,
		Scope:      csv(*scope),
		Supersedes: csv(*supersedes),
		Session:    *session,
		Note:       *note,
	}

	// Load→record→save under the store lock so concurrent writers (other
	// MCP servers, another CLI) cannot drop this record's update.
	var created bool
	var evidence int
	err = codedocket.Update(knowledgeFilePath, func(store *codedocket.Store) error {
		k, c, err := codedocket.Record(store, in, time.Now())
		if err != nil {
			return err
		}
		created, evidence = c, len(k.Evidence)
		return nil
	})
	if err != nil {
		return err
	}

	if created {
		fmt.Printf("recorded %s\n", *key)
	} else {
		fmt.Printf("updated %s (evidence: %d)\n", *key, evidence)
	}

	return nil
}

func csv(s string) []string {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}
