# codedocket — accumulated project context for coding sessions

`codedocket` gives a project a persistent, structured memory. Decisions, constraints,
bugs, assumptions, rationale, and facts are recorded as typed **knowledge
entries** in a single git-committed JSON file, so any future session — human or
agent — can ask *what does this project already know?* before acting.

The philosophy: a **dumb store with careful bookkeeping, driven by smart
callers**. The tool never infers, auto-resolves, or deletes anything; callers
(humans or coding agents) explicitly record, update, supersede, and dispute.

## Core ideas

- **Nothing is ever deleted.** Wrong or outdated entries are *superseded* by
  their replacement, or *disputed* to flag them for human review. History is
  preserved and re-surfaceable.
- **Fully deterministic merge.** Same store + same operation = same result.
  No inference, no auto-resolution, no fuzzy matching.
- **Git is the history of record.** The store is one indented JSON file with
  sorted keys and atomic writes, so diffs stay clean and reviewable.
- **Scoped knowledge.** Every entry names the paths it applies to (`scope`),
  so querying "I'm about to touch `internal/merge/`" surfaces exactly the
  constraints that govern that area. Scope `.` means project-wide.
- **Zero dependencies.** Go standard library only.

## Data model

Each **Knowledge** entry has:

| field | meaning |
|---|---|
| `key` | stable caller-chosen slug naming the *topic* (e.g. `merge.deterministic`); one position per key |
| `kind` | `decision` \| `constraint` \| `bug` \| `assumption` \| `rationale` \| `fact` |
| `statement` | the current position on the topic; updated in place as understanding evolves |
| `scope` | paths this knowledge applies to; `.` = project-global |
| `status` | `active` \| `superseded` \| `disputed` |
| `evidence` | observations backing the entry (session, note, timestamp); confidence is *derived* from evidence, never stored |

Supersession is recorded as a typed `supersedes` **edge**, so a query with
`--include-superseded` can show an old position side-by-side with the one that
replaced it.

## Installation

### Recommended: npm / npx

Run CodeDocket without cloning this repo or installing Go:

```sh
npx -y codedocket serve
```

Or install it globally:

```sh
npm install -g codedocket
codedocket setup
```

### From Go source

Requires Go 1.22+.

```sh
# Install directly from GitHub (recommended)
go install github.com/Amaan-Khan14/codedocket/cmd/codedocket@latest

# Or build from source
git clone https://github.com/Amaan-Khan14/codedocket.git
cd codedocket
go build -o codedocket ./cmd/codedocket
```

### For MCP server (AI agents)

Run the setup wizard to install `codedocket` to `~/.local/bin` and configure supported
agent clients:

```sh
codedocket setup
```

For non-interactive setup, pass the target clients and scope explicitly:

```sh
codedocket setup --clients codex,claude --scope global --yes
codedocket setup --clients opencode,cursor --scope project --skip-install
```

Supported clients:

| client | global config | project config | global instructions |
|---|---|---|---|
| opencode | `~/.config/opencode/opencode.json` | `opencode.json` | `~/.config/opencode/AGENTS.md` |
| Claude Code | `~/.claude.json` | `.mcp.json` | `~/.claude/CLAUDE.md` |
| Cursor | `~/.cursor/mcp.json` | `.cursor/mcp.json` | none |
| Codex CLI | `~/.codex/config.toml` | not supported in V1 | `~/.codex/AGENTS.md` |
| ZCode | `~/.zcode/cli/config.json` | `.zcode/config.json` | `~/.zcode/AGENTS.md` |
| Kiro | `~/.kiro/settings/mcp.json` | `.kiro/settings/mcp.json` | none |

You can also configure an MCP client manually:

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

## Usage

All commands operate on `.codedocket/knowledge.json` in the current directory.

### `codedocket init`

Initialize a knowledge store.

```sh
codedocket init            # creates .codedocket/knowledge.json; refuses if it exists
codedocket init --force    # overwrite an existing store
```

By default, `codedocket init` also ensures `AGENTS.md` contains the marker-delimited
project knowledge instructions. Use `--no-agents` to create only the store.

### `codedocket setup`

Configure agent clients to call `codedocket serve` over MCP and install the shared
instruction snippet where supported.

```sh
codedocket setup
codedocket setup --clients codex,claude --scope global --yes
codedocket setup --clients opencode --scope project --skip-install
```

Flags:

| flag | meaning |
|---|---|
| `--clients` | comma-separated clients: `opencode`, `claude`, `cursor`, `codex`, `zcode`, `kiro` |
| `--scope` | `global` or `project`; defaults to interactive selection, or `global` with `--yes` |
| `--skip-install` | do not copy the current binary to `~/.local/bin/codedocket` |
| `--yes` | use non-interactive defaults |

### `codedocket note`

Capture an observation mid-task with zero structure — one sentence, no key,
no kind. Notes land in gitignored per-session scratch
(`.codedocket/sessions/<id>/notes.json`) and are reviewed later via
`finalize`. Flags go before the text.

```sh
codedocket note --path internal/merge/ "merge never infers; conflicts are caller-explicit"
codedocket note "evidence count ranks but never qualifies"
```

Agents capture through the MCP tool `codedocket_note` instead.

### `codedocket finalize`

The consolidation moment: renders the pending session's notes as numbered
proposals plus capped git evidence and the review instructions — record each
proposal with `codedocket record` (passing the session id and the note text
as provenance) or skip it deliberately. Writes a mechanical `finalized.json`
marker; surfaces a `⚠` footnote for sessions that were finalized but produced
no records (the follow-through check); prunes finalized scratch older than 14
days. Never calls an LLM, never writes the store.

```sh
codedocket finalize                 # newest pending session
codedocket finalize --all           # every pending session (sweeps stale ones)
codedocket finalize --session ID --reopen   # un-finalize for re-review
```

### `codedocket hook stop`

The gate Stop hooks invoke — what `setup` registers for you. Silent allow
(exit 0, empty stdout) when nothing is pending; otherwise the client's block
shape with a reason the still-active agent receives as its next instruction.

```sh
codedocket hook stop --client zcode   # claude | codex | zcode | kiro
```

### `codedocket record`

Record knowledge, or add evidence to an existing key.

```sh
codedocket record --key storage.format --kind decision \
  --statement "Single JSON file, git-committed." \
  --scope .

# Confirming/refining an existing topic: record the same key again.
# The statement updates in place and evidence accumulates.

# Replacing an old decision: record the new key and supersede the old one.
codedocket record --key extract.smart-caller --kind decision \
  --statement "The calling agent performs extraction." \
  --scope mcp/ --supersedes extract.own-pipeline
```

Flags: `--key --kind --statement --scope a,b [--supersedes k1,k2] [--session] [--note]`
(`--scope` and `--supersedes` are comma-separated; `--session` defaults to `cli`).

### `codedocket dispute`

Flag an entry as contested when you believe it's wrong but can't yet replace
it. Disputed entries **stay visible** — that's the point: a human should weigh
in. To *correct* knowledge, use `record` instead (same key, or new key +
`--supersedes`).

```sh
codedocket dispute --key merge.conflicts --note "keys keep colliding"
```

### `codedocket explore`

Retrieve knowledge — the read path, and the actual point of the tool.

```sh
codedocket explore                                        # list everything (ranked)
codedocket explore --query "extraction pipeline"          # keyword search
codedocket explore --path internal/merge/merge.go         # knowledge scoped to a path
codedocket explore --kind decision                        # filter by kind
codedocket explore --key storage.format                   # exact lookup (even superseded)
codedocket explore --include-superseded                   # reveal historical positions
codedocket explore --json                                 # machine-readable output
```

Ranking is deterministic: scope matches outweigh keyword matches, evidence
count breaks ties, then recency, then key. Same store + same query = same
output, every time.

Example human output:

```
merge.deterministic  [decision]  (evidence: 2)
  Merge never infers; explicit only.
  scope: internal/  updated: 2026-08-06

merge.conflicts  [assumption, DISPUTED]  (evidence: 1)
  Conflicts will be rare at small scale.
  scope: internal/merge/  updated: 2026-08-05
```

## Project layout

```
├── cmd/codedocket/          # the codedocket CLI (flag-based subcommands, one binary)
│   └── main.go  init.go  setup.go  record.go  dispute.go  explore.go
├── types.go          # Knowledge / Evidence / Edge / Store / QueryOpts
├── store.go          # Load/Save: atomic writes, diff-stable JSON
├── merge.go          # Record/Dispute: the deterministic merge rules
├── query.go          # retrieval: filter → score → sort
└── *_test.go
```

The root package `codedocket` holds all core semantics; the CLI is a thin
presentation layer over it. Retrieval contract:

```go
func Query(s *Store, opts QueryOpts) []*Knowledge
```

## Roadmap

- **M1/M2 — done.** Core package (store, deterministic merge, query) and the
  `codedocket` CLI (`init`, `record`, `dispute`, `explore`), dogfooded on this repo's
  own `.codedocket/` store.
- **M3 — done.** `codedocket serve`: an MCP stdio server exposing `codedocket_record`,
  `codedocket_explore`, and `codedocket_dispute` as tools so coding agents can read and
  write project knowledge natively.
- **M4 — done.** Onboarding: `init` bootstraps an AGENTS.md snippet into the
  target repo, and `setup` configures supported agent clients for MCP.
- **M6 — done.** Memory consolidation: `note` (cheap mid-task capture into
  gitignored session scratch) → client **Stop hook** (`hook stop`, registered
  by `setup` for Claude Code, Codex, and ZCode) blocks the agent's exit and
  injects the finalize instruction while it is still in context → `finalize`
  renders notes + git evidence as numbered proposals (no LLM) → the agent
  records accepted ones via `record`. Worst case is delayed knowledge, never
  lost knowledge.

## Development

```sh
gofmt -l .        # must be clean
go vet ./...      # must be clean
go test ./...     # must be green
```
