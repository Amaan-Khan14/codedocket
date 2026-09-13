package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	codedocket "github.com/Amaan-Khan14/codedocket"
)

// hookCtx is the client hook entry point (M6 Task 3). `hook stop
// --client X` is what Stop hooks invoke: silent allow (exit 0, empty
// stdout) when nothing is pending; the client's block shape otherwise.
// Protocol bytes on stdout only; diagnostics on stderr. Our own errors
// never trap the agent — resolve/list failures allow with a warning
// (ZCode force-ends after 3 consecutive blocks; a broken hook must not
// spend them).
func hookCtx(args []string) error {
	if len(args) == 0 || args[0] != "stop" {
		return fmt.Errorf("usage: codedocket hook stop --client <%s>", strings.Join(codedocket.SupportedHookClients, "|"))
	}
	fs := flag.NewFlagSet("hook stop", flag.ExitOnError)
	client := fs.String("client", "", "client whose Stop-hook contract to speak")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *client == "" {
		return fmt.Errorf("--client is required (%s)", strings.Join(codedocket.SupportedHookClients, ", "))
	}

	knowledgePath, err := mustStore()
	if err != nil {
		// No store (or unreadable) → nothing can be pending → allow.
		fmt.Fprintf(os.Stderr, "codedocket hook: %v\n", err)
		return nil
	}
	sessions, err := codedocket.ListSessions(filepath.Dir(knowledgePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "codedocket hook: %v\n", err)
		return nil
	}
	var pending []codedocket.SessionInfo
	for _, s := range sessions {
		if !s.Finalized {
			pending = append(pending, s)
		}
	}

	out, err := codedocket.StopHookResponse(*client, pending)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if out != "" {
		fmt.Println(out)
	}
	return nil
}
