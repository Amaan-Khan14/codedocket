# CodeDocket

Git-backed project memory for coding agents.

Coding agents are good at reading code, but they repeatedly lose the "why":
decisions, constraints, bug root causes, assumptions, and rationale from earlier
sessions. CodeDocket gives every repo a small, durable memory layer that agents
can read before acting and update when they learn something worth preserving.

Everything lives in `.codedocket/knowledge.json`, a plain JSON file you can
commit, diff, review, and carry with the project.

## Why It Exists

| Problem | CodeDocket's answer |
|---|---|
| Agents rediscover the same context every session | Store project knowledge once and retrieve it by topic or path |
| Important rationale gets buried in chat history | Commit it beside the code in `.codedocket/knowledge.json` |
| Agent memory can become fuzzy or unreviewable | Keep the store deterministic, explicit, and git-diffable |
| Wrong knowledge needs history, not deletion | Supersede or dispute entries while preserving evidence |
| Mid-task discoveries interrupt flow | Capture cheap notes now, consolidate them at the end |

## Quickstart

For MCP usage, install a stable `codedocket` binary first. Agent clients need a
command they can launch later, outside the shell that installed it.

```sh
npm install -g codedocket
codedocket setup
```

Then initialize project memory in any repo where you want agents to use it:

```sh
cd your-project
codedocket init
```

Restart your agent client. It will now have CodeDocket MCP tools available.

| MCP tool | When the agent should use it |
|---|---|
| `codedocket_explore` | Before planning or editing unfamiliar code |
| `codedocket_record` | When it learns a durable decision, constraint, bug root cause, assumption, rationale, or fact |
| `codedocket_dispute` | When existing knowledge looks wrong but should stay visible |
| `codedocket_note` | When it notices something mid-task but should not stop to structure it |

Manual MCP shape, if you prefer editing client config yourself:

```json
{
  "mcpServers": {
    "codedocket": {
      "command": "codedocket",
      "args": ["serve"]
    }
  }
}
```

## The Agent Workflow

| Moment | Command or MCP tool | What happens |
|---|---|---|
| Before editing unfamiliar code | `codedocket explore --path internal/foo/` or `codedocket_explore` | The agent sees relevant decisions, constraints, bugs, and rationale |
| When a fact becomes clear | `codedocket record ...` or `codedocket_record` | Structured knowledge is written to `.codedocket/knowledge.json` |
| When the agent is busy mid-task | `codedocket note "one sentence"` or `codedocket_note` | A cheap scratch note is saved without forcing structure immediately |
| At session end | `codedocket finalize` | Notes and git evidence become numbered proposals to record or skip |
| If knowledge is outdated | `codedocket record --supersedes old.key ...` | The replacement becomes active and the old position remains available |
| If knowledge is suspicious | `codedocket dispute --key some.key` | The entry stays visible with a disputed marker |

Example agent loop:

| Step | Agent action |
|---|---|
| 1 | Calls `codedocket_explore` with the paths it is about to touch |
| 2 | Uses the returned project knowledge while planning and editing |
| 3 | Calls `codedocket_note` for cheap mid-task observations |
| 4 | At the end, `codedocket finalize` turns notes into reviewable proposals |
| 5 | Calls `codedocket_record` for accepted proposals so future sessions inherit them |

## What Gets Stored

Each knowledge entry has a stable key, a type, a statement, a scope, a status,
and evidence.

| Field | Meaning |
|---|---|
| `key` | Caller-chosen topic slug, for example `merge.deterministic` |
| `kind` | `decision`, `constraint`, `bug`, `assumption`, `rationale`, or `fact` |
| `statement` | The current position on that topic |
| `scope` | Paths this knowledge applies to; `.` means project-wide |
| `status` | `active`, `superseded`, or `disputed` |
| `evidence` | Session notes and timestamps backing the entry |

CodeDocket does not infer truth, delete old positions, or silently resolve
conflicts. Callers explicitly record, refine, supersede, or dispute knowledge.

## Supported Agent Clients

| Client | MCP config | Project config | Instructions | Stop-hook finalize gate |
|---|---|---|---|---|
| Claude Code | `~/.claude.json` | `.mcp.json` | `~/.claude/CLAUDE.md` | yes |
| Codex CLI | `~/.codex/config.toml` | not supported in V1 | `~/.codex/AGENTS.md` | yes |
| ZCode | `~/.zcode/cli/config.json` | `.zcode/config.json` | `~/.zcode/AGENTS.md` | yes |
| opencode | `~/.config/opencode/opencode.json` | `opencode.json` | `~/.config/opencode/AGENTS.md` | no |
| Cursor | `~/.cursor/mcp.json` | `.cursor/mcp.json` | none | no |
| Kiro | `~/.kiro/settings/mcp.json` | `.kiro/settings/mcp.json` | none | deferred |

Manual MCP configuration:

```json
{
  "mcpServers": {
    "codedocket": {
      "command": "codedocket",
      "args": ["serve"]
    }
  }
}
```

## Installation

### npm / npx

```sh
npx -y codedocket serve
```

```sh
npm install -g codedocket
codedocket setup
```

The npm package downloads the matching native GitHub release binary during
install, so users do not need Go.

### Go

Requires Go 1.22 or newer.

```sh
go install github.com/Amaan-Khan14/codedocket/cmd/codedocket@latest
```

Or build from source:

```sh
git clone https://github.com/Amaan-Khan14/codedocket.git
cd codedocket
go build -o codedocket ./cmd/codedocket
```

## Command Reference

All commands operate on `.codedocket/knowledge.json` in the current directory.

### `codedocket init`

Initialize a knowledge store.

```sh
codedocket init
codedocket init --force
codedocket init --no-agents
```

By default, `codedocket init` also ensures `AGENTS.md` contains marker-delimited
project-memory instructions. Use `--no-agents` to create only the store.

### `codedocket setup`

Configure agent clients to call `codedocket serve` over MCP and install the
shared instruction snippet where supported.

```sh
codedocket setup
codedocket setup --clients codex,claude --scope global --yes
codedocket setup --clients opencode --scope project --skip-install
```

| Flag | Meaning |
|---|---|
| `--clients` | Comma-separated clients: `opencode`, `claude`, `cursor`, `codex`, `zcode`, `kiro` |
| `--scope` | `global` or `project`; defaults to interactive selection, or `global` with `--yes` |
| `--skip-install` | Do not copy the current binary to `~/.local/bin/codedocket` |
| `--yes` | Use non-interactive defaults |

### `codedocket explore`

Retrieve knowledge. This is the read path agents should call before touching
unfamiliar code.

```sh
codedocket explore
codedocket explore --query "extraction pipeline"
codedocket explore --path internal/merge/merge.go
codedocket explore --kind decision
codedocket explore --key storage.format
codedocket explore --include-superseded
codedocket explore --json
```

Ranking is deterministic: scope matches outweigh keyword matches, evidence
count breaks ties, then recency, then key. Same store plus same query gives the
same output.

### `codedocket record`

Record knowledge, or add evidence to an existing key.

```sh
codedocket record --key storage.format --kind decision \
  --statement "Single JSON file, git-committed." \
  --scope .

codedocket record --key extract.smart-caller --kind decision \
  --statement "The calling agent performs extraction." \
  --scope mcp/ --supersedes extract.own-pipeline
```

| Flag | Meaning |
|---|---|
| `--key` | Stable dot-case topic key |
| `--kind` | `decision`, `constraint`, `bug`, `assumption`, `rationale`, or `fact` |
| `--statement` | Current position, usually one or two sentences |
| `--scope` | Comma-separated paths; `.` means project-wide |
| `--supersedes` | Optional comma-separated keys replaced by this entry |
| `--session` | Optional provenance session; defaults to `cli` |
| `--note` | Optional evidence note |

Recording the same key again refines the statement in place and appends
evidence.

### `codedocket note`

Capture an observation mid-task with no key and no kind. Notes land in
gitignored per-session scratch under `.codedocket/sessions/<id>/notes.json` and
are reviewed later through `finalize`.

```sh
codedocket note --path internal/merge/ "merge never infers; conflicts are caller-explicit"
codedocket note "evidence count ranks but never qualifies"
```

Agents can capture through the MCP tool `codedocket_note`.

### `codedocket finalize`

Render pending session notes as numbered proposals plus capped git evidence.
The command never calls an LLM and never writes the knowledge store directly.
The agent or human records accepted proposals and skips the rest deliberately.

```sh
codedocket finalize
codedocket finalize --all
codedocket finalize --session ID --reopen
```

Finalized scratch older than the retention window is pruned; pending sessions
are never auto-pruned.

### `codedocket hook stop`

The Stop-hook gate registered by `setup`. It allows exit silently when nothing
is pending. If notes need review, it blocks the agent's stop and injects the
finalize instruction while the agent is still in context.

```sh
codedocket hook stop --client codex
codedocket hook stop --client claude
codedocket hook stop --client zcode
```

### `codedocket dispute`

Flag an entry as contested when you believe it is wrong but cannot yet replace
it. Disputed entries stay visible. To correct knowledge, use `record` instead.

```sh
codedocket dispute --key merge.conflicts --note "keys keep colliding"
```

## Design Principles

| Principle | Practical effect |
|---|---|
| Dumb store, smart caller | Humans or agents decide what to record; CodeDocket handles bookkeeping |
| Deterministic merge | Same store plus same operation gives the same result |
| Git as the history of record | Review project memory with normal code review tools |
| No deletion of knowledge | Superseded and disputed entries remain inspectable |
| Scoped retrieval | Path-specific queries surface relevant constraints before edits |
| Zero runtime dependencies | The Go binary uses the standard library only |

## Project Layout

```text
|-- cmd/codedocket/          # CLI, MCP server, setup, hooks
|-- types.go                 # Knowledge / Evidence / Edge / Store / QueryOpts
|-- store.go                 # Load/Save: atomic writes, diff-stable JSON
|-- merge.go                 # Record/Dispute: deterministic merge rules
|-- query.go                 # Retrieval: filter, score, sort
`-- *_test.go
```

The root package `codedocket` holds the core semantics; the CLI is a thin
presentation layer over it.

```go
func Query(s *Store, opts QueryOpts) []*Knowledge
```

## Roadmap

| Milestone | Status | Scope |
|---|---|---|
| M1/M2 | done | Core package, deterministic store, CLI commands, dogfooding on this repo |
| M3 | done | MCP stdio server exposing `codedocket_record`, `codedocket_explore`, and `codedocket_dispute` |
| M4 | done | Agent onboarding through `init`, `setup`, and shared instructions |
| M6 | done | Session notes, finalize proposals, Stop-hook gate for Claude Code, Codex, and ZCode |

## Development

```sh
gofmt -l .
go vet ./...
go test ./...
```
