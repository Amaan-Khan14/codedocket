package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	markerBegin = "<!-- codedocket:begin -->"
	markerEnd   = "<!-- codedocket:end -->"
)

// agentsSnippet is the onboarding text init writes into a target repo's
// AGENTS.md. It is load-bearing: it teaches every future agent session the
// explore-first / record discipline without human prompting. Edit with care;
// it must stay tool-agnostic (MCP tools, CLI, or raw JSON fallback).
var agentsSnippet = markerBegin + `
## Project knowledge (codedocket)

This repo accumulates project understanding — decisions, constraints, bugs,
assumptions, rationale — in ` + "`.codedocket/knowledge.json`" + `. It exists so you do not
re-derive what is already known.

**Read first.** Before planning or editing unfamiliar areas, query it:
- MCP tools: ` + "`codedocket_explore`" + ` (query for topics, paths for files you will touch,
  key for exact lookup, include_superseded for history).
- CLI: ` + "`codedocket explore --query <topic>`" + ` or ` + "`codedocket explore --path <paths>`" + `.
- Fallback: read ` + "`.codedocket/knowledge.json`" + ` directly — it is plain JSON.
Statuses matter: ` + "`superseded`" + ` = historical position, kept for history — do not
follow it; ` + "`disputed`" + ` = contested — weigh in or proceed carefully. Scope ` + "`.`" + `
means project-wide.

**Record what you learn.** When you discover something non-obvious — a
decision and its why, a constraint, a bug's root cause, an assumption —
record it so the next session does not rediscover it:
- MCP tools: ` + "`codedocket_record`" + `; CLI: ` + "`codedocket record --key K --kind K --statement S --scope P`" + `.
- Key: a stable dot.case slug naming the TOPIC, e.g. ` + "`storage.format`" + `.
  One decision per key.
- Kind: decision | constraint | bug | assumption | rationale | fact.
- Statement: the current position, 1-2 sentences.
- Scope: paths it applies to (` + "`.`" + ` for project-wide).
- Do not duplicate: explore first; same topic -> record again with the SAME
  key (re-observation strengthens it). A decision replacing an earlier one ->
  new key + ` + "`supersedes`" + `.
- ` + "`codedocket_dispute`" + ` flags knowledge as contested; it is not a correction mechanism —
  correct by recording.

**Capture without interrupting.** Structure on demand, not mid-execution:
- Mid-task, when you learn something non-obvious but are busy executing:
  ` + "`codedocket_note`" + ` (MCP) / ` + "`codedocket note \"one sentence\"`" + ` (CLI) —
  no key, no kind; it is reviewed at session end.
- When finishing (your client's Stop hook may insist): run
  ` + "`codedocket finalize`" + `, then for each proposal record it — ` + "`codedocket_record`" + `
  with the note text as evidence note and the session id shown — or skip it
  deliberately.
` + markerEnd + `
`

// ensureAgentsSnippet makes sure repoDir/AGENTS.md carries the codedocket snippet.
// Kept for the init path; equivalent to ensureMarkdownSnippet(dir, "AGENTS.md").
func ensureAgentsSnippet(repoDir string) (string, error) {
	return ensureMarkdownSnippet(repoDir, "AGENTS.md")
}

// ensureMarkdownSnippet makes sure dir/<name> carries the codedocket snippet.
// It never clobbers existing content outside its own marker block. Returns
// the outcome: "present", "created", "appended", or "updated" (the markers
// were present but held an older snippet, replaced in place — the upgrade
// path for every repo onboarded before a snippet revision).
func ensureMarkdownSnippet(dir, name string) (string, error) {
	path := filepath.Join(dir, name)

	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.WriteFile(path, []byte(agentsSnippet), 0o644); err != nil {
			return "", fmt.Errorf("creating AGENTS.md: %w", err)
		}
		return "created", nil
	case err != nil:
		return "", fmt.Errorf("reading AGENTS.md: %w", err)
	}

	if i := bytes.Index(data, []byte(markerBegin)); i >= 0 {
		j := bytes.Index(data[i:], []byte(markerEnd))
		if j < 0 {
			return "present", nil // malformed (begin without end): leave untouched
		}
		end := i + j + len(markerEnd)
		// The snippet constant ends with a newline after markerEnd; the
		// extracted block stops at the marker, so compare trimmed.
		want := bytes.TrimSuffix([]byte(agentsSnippet), []byte("\n"))
		if bytes.Equal(data[i:end], want) {
			return "present", nil
		}
		out := make([]byte, 0, len(data))
		out = append(out, data[:i]...)
		out = append(out, []byte(agentsSnippet)...)
		out = append(out, data[end:]...)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return "", fmt.Errorf("updating AGENTS.md snippet: %w", err)
		}
		return "updated", nil
	}

	sep := []byte("\n\n")
	if len(bytes.TrimSpace(data)) == 0 {
		sep = nil // empty file: snippet becomes the whole content
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("opening AGENTS.md: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(sep, []byte(agentsSnippet)...)); err != nil {
		return "", fmt.Errorf("appending to AGENTS.md: %w", err)
	}
	return "appended", nil
}
