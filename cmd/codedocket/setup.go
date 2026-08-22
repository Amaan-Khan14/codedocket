package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// setupCtx is the onboarding wizard: detect agent clients, write MCP configs,
// drop the global instruction snippet, optionally self-install the binary.
// Interactive when stdin is a TTY; fully flag-driven otherwise.
func setupCtx(args []string) error {
	fl := flag.NewFlagSet("setup", flag.ExitOnError)
	clientsFlag := fl.String("clients", "", "comma-separated clients to configure (default: interactive select)")
	scope := fl.String("scope", "", "global | project (default: ask, or global with --yes)")
	skipInstall := fl.Bool("skip-install", false, "do not install codedocket to ~/.local/bin")
	yes := fl.Bool("yes", false, "non-interactive: defaults everywhere")
	if err := fl.Parse(args); err != nil {
		return err
	}

	interactive := isTTY(os.Stdin) && !*yes && *clientsFlag == "" && *scope == ""

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home dir: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}

	// --- select clients ---
	selected, err := selectClients(home, *clientsFlag, interactive)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println("nothing selected; no changes made")
		return nil
	}

	// --- install to PATH (so clients can launch the unary MCP server) ---
	binPath := ""
	if !*skipInstall {
		if interactive && !confirm("Install the codedocket binary to ~/.local/bin? [Y/n] ") {
			binPath = defaultBinPath(home)
		} else if p, err := installSelf(home); err != nil {
			fmt.Fprintf(os.Stderr, "warning: self-install failed (%v); configs will reference %s\n", err, defaultBinPath(home))
			binPath = defaultBinPath(home)
		} else {
			binPath = p
			fmt.Printf("Installed codedocket to %s\n", binPath)
		}
	} else {
		binPath = defaultBinPath(home)
	}

	// --- scope ---
	if *scope == "" {
		if interactive {
			*scope = ask("Apply agent configs to all your projects, or just this one? [all/this] ", "all")
			if strings.HasPrefix(*scope, "t") {
				*scope = "project"
			} else {
				*scope = "global"
			}
		} else {
			*scope = "global"
		}
	}
	if *scope != "global" && *scope != "project" {
		return fmt.Errorf("invalid scope %q: must be global or project", *scope)
	}

	// --- apply ---
	var report []string
	for _, c := range selected {
		if *scope == "global" {
			if c.GlobalConfig != "" {
				report = append(report, applyJSONOrTOML(c.Label, filepath.Join(home, c.GlobalConfig), c.merge, binPath))
			}
			if c.GlobalMD != "" {
				report = append(report, applyMarkdown(c.Label, filepath.Join(home, c.GlobalMD)))
			}
			if c.HookGlobal != "" && c.mergeHook != nil {
				report = append(report, applyJSONOrTOML(c.Label+" Stop hook", filepath.Join(home, c.HookGlobal), c.mergeHook, binPath))
			}
		} else {
			if c.ProjectConfig != "" {
				report = append(report, applyJSONOrTOML(c.Label, filepath.Join(cwd, c.ProjectConfig), c.merge, binPath))
			}
			if c.HookProject != "" && c.mergeHook != nil {
				report = append(report, applyJSONOrTOML(c.Label+" Stop hook", filepath.Join(cwd, c.HookProject), c.mergeHook, binPath))
			}
		}
	}
	if *scope == "project" {
		report = append(report, applyMarkdown("AGENTS.md", filepath.Join(cwd, "AGENTS.md")))
	}
	for _, line := range report {
		fmt.Println(line)
	}
	fmt.Println("\nDone! Restart your agents for MCP changes to take effect.")
	fmt.Println("Then: cd your-project && codedocket init")
	return nil
}

func selectClients(home, csv string, interactive bool) ([]clientDef, error) {
	if csv != "" {
		names := map[string]clientDef{}
		var known []string
		for _, c := range knownClients {
			names[c.Name] = c
			known = append(known, c.Name)
		}
		var out []clientDef
		for _, name := range strings.Split(csv, ",") {
			name = strings.TrimSpace(strings.ToLower(name))
			if c, ok := names[name]; ok {
				out = append(out, c)
			} else {
				return nil, fmt.Errorf("unknown client %q (known: %s)", name, strings.Join(known, ", "))
			}
		}
		return out, nil
	}

	detected := make([]bool, len(knownClients))
	fmt.Println("Which agents should codedocket configure?")
	for i, c := range knownClients {
		detected[i] = clientDetected(home, c)
		tag := "not found"
		if detected[i] {
			tag = "detected"
		}
		fmt.Printf("  %d. %-12s (%s)\n", i+1, c.Label, tag)
	}
	answer := ask(`Enter numbers (e.g. "1 4"), or "a" for all detected: `, "a")
	if answer == "a" {
		var out []clientDef
		for i, c := range knownClients {
			if detected[i] {
				out = append(out, c)
			}
		}
		if len(out) == 0 {
			fmt.Println("no known clients detected; configuring all")
			return knownClients, nil
		}
		return out, nil
	}
	var out []clientDef
	for _, tok := range strings.Fields(answer) {
		var n int
		if _, err := fmt.Sscanf(tok, "%d", &n); err != nil || n < 1 || n > len(knownClients) {
			return nil, fmt.Errorf("invalid selection %q", tok)
		}
		out = append(out, knownClients[n-1])
	}
	return out, nil
}

func clientDetected(home string, c clientDef) bool {
	for _, rel := range c.DetectDirs {
		if _, err := os.Stat(filepath.Join(home, rel)); err == nil {
			return true
		}
	}
	return false
}

// applyJSONOrTOML merges our codedocket entry into a client's MCP config file,
// backing up before any write. Reports codegraph-style Updated/Unchanged.
func applyJSONOrTOML(label, path string, merge func([]byte, string) ([]byte, bool, error), binPath string) string {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Sprintf("%s: FAILED %s (%v)", label, path, err)
	}
	merged, changed, err := merge(existing, binPath)
	if err != nil {
		return fmt.Sprintf("%s: FAILED %s (%v)", label, path, err)
	}
	if !changed {
		return fmt.Sprintf("%s: Unchanged %s", label, path)
	}
	if existing != nil {
		if err := os.WriteFile(path+".bak", existing, 0o644); err != nil {
			return fmt.Sprintf("%s: FAILED writing backup (%v)", label, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Sprintf("%s: FAILED %s (%v)", label, path, err)
	}
	if err := os.WriteFile(path, merged, 0o644); err != nil {
		return fmt.Sprintf("%s: FAILED %s (%v)", label, path, err)
	}
	return fmt.Sprintf("%s: Updated %s", label, path)
}

func applyMarkdown(label, path string) string {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Sprintf("%s: FAILED %s (%v)", label, path, err)
	}
	outcome, err := ensureMarkdownSnippet(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		return fmt.Sprintf("%s: FAILED %s (%v)", label, path, err)
	}
	if outcome == "present" {
		return fmt.Sprintf("%s: Unchanged %s", label, path)
	}
	return fmt.Sprintf("%s: Updated %s", label, path)
}

// installSelf copies the running binary to ~/.local/bin/codedocket.
func installSelf(home string) (string, error) {
	target := filepath.Join(home, ".local", "bin", "codedocket")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating executable: %w", err)
	}
	src, err := os.Open(execPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	if !strings.Contains(os.Getenv("PATH"), filepath.Dir(target)) {
		fmt.Fprintf(os.Stderr, "note: %s is not on your PATH (add: export PATH=$PATH:%s)\n", filepath.Dir(target), filepath.Dir(target))
	}
	return target, nil
}

// --- tiny interactive helpers (numbered/plain text: zero TUI deps) ---

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

var promptReader = bufio.NewReader(os.Stdin)

func ask(question, def string) string {
	fmt.Print(question)
	line, _ := promptReader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func confirm(question string) bool {
	answer := strings.ToLower(ask(question, "y"))
	return answer == "" || answer == "y" || answer == "yes"
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// defaultBinPath is the path configs should point at when we do not
// (re)install: the standard install location if it exists, else the actual
// executable path. Using the stable location keeps configs idempotent across runs.
func defaultBinPath(home string) string {
	p := filepath.Join(home, ".local", "bin", "codedocket")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if execPath, err := os.Executable(); err == nil {
		return execPath
	}
	// Last resort fallback
	return mustAbs(os.Args[0])
}
