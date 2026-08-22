package main

import (
	"errors"
	"fmt"
	"os"

	codedocket "github.com/Amaan-Khan14/codedocket"
)

// version is stamped by the release build (goreleaser ldflags); "dev" locally.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {

	case "init":

		if err := initCtx(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket init:", err)
			os.Exit(1)
		}

	case "record":
		if err := recordCtx(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket record:", err)
			os.Exit(1)
		}

	case "note":
		if err := noteCtx(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket note:", err)
			os.Exit(1)
		}

	case "finalize":
		if err := finalizeCtx(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket finalize:", err)
			os.Exit(1)
		}

	case "dispute":
		if err := disputeCtx(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket dispute:", err)
			os.Exit(1)
		}

	case "explore":
		if err := exploreCtx(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket explore:", err)
			os.Exit(1)
		}

	case "setup":
		if err := setupCtx(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket setup:", err)
			os.Exit(1)
		}

	case "serve":
		if err := serveCtx(); err != nil {
			fmt.Fprintln(os.Stderr, "codedocket serve:", err)
			os.Exit(1)
		}

	case "version":
		fmt.Println(version)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

// mustStore resolves the nearest .codedocket store walking up from cwd, with a
// friendly error when none exists.
func mustStore() (string, error) {
	_, knowledgePath, err := codedocket.ResolveStore()
	if errors.Is(err, codedocket.ErrNoStore) {
		return "", fmt.Errorf("no .codedocket store found walking up from cwd; run `codedocket init`")
	}
	return knowledgePath, err
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: codedocket <command> [args...]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  init      initialize a new knowledge store")
	fmt.Fprintln(os.Stderr, "  record    record a decision or observation")
	fmt.Fprintln(os.Stderr, "  note      capture a quick observation for later review")
	fmt.Fprintln(os.Stderr, "  finalize  review captured notes and record knowledge")
	fmt.Fprintln(os.Stderr, "  dispute   mark knowledge as disputed")
	fmt.Fprintln(os.Stderr, "  explore   explore project knowledge")
	fmt.Fprintln(os.Stderr, "  serve     run MCP stdio server")
	fmt.Fprintln(os.Stderr, "  setup     configure agent clients (MCP onboarding wizard)")
	fmt.Fprintln(os.Stderr, "  version   print version")
}
