package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// clientDef describes one known agent client: how to detect it, where its
// MCP config and global instruction file live, and how to merge our entry.
// Stop-hook registration (M6 Task 4) is a second artifact: HookGlobal/
// HookProject name the config file, mergeHook upserts the hook entry with
// the same presence rule as appendCodexTOML. "" / nil = no hook support.
type clientDef struct {
	Name          string // short lowercase name for --clients matching
	Label         string // display name
	GlobalConfig  string // path relative to home dir, "" if unsupported
	ProjectConfig string // path relative to project cwd, "" if unsupported
	GlobalMD      string // global instruction file relative to home, "" if none
	DetectDirs    []string
	merge         func(existing []byte, binPath string) ([]byte, bool, error)

	HookGlobal  string // Stop-hook config relative to home, "" if none
	HookProject string // Stop-hook config relative to cwd, "" if none
	mergeHook   func(existing []byte, binPath string) ([]byte, bool, error)
}

var knownClients = []clientDef{
	{
		Name:          "opencode",
		Label:         "opencode",
		GlobalConfig:  ".config/opencode/opencode.json",
		ProjectConfig: "opencode.json",
		GlobalMD:      ".config/opencode/AGENTS.md",
		DetectDirs:    []string{".config/opencode", ".opencode"},
		merge:         mergeOpencodeJSON,
	},
	{
		Name:          "claude",
		Label:         "Claude Code",
		GlobalConfig:  ".claude.json",
		ProjectConfig: ".mcp.json",
		GlobalMD:      ".claude/CLAUDE.md",
		DetectDirs:    []string{".claude", ".claude.json"},
		merge:         mergeMCPServersJSON,
		HookGlobal:    ".claude/settings.json",
		HookProject:   ".claude/settings.json",
		mergeHook:     mergeClaudeStopHookJSON,
	},
	{
		Name:          "cursor",
		Label:         "Cursor",
		GlobalConfig:  ".cursor/mcp.json",
		ProjectConfig: ".cursor/mcp.json",
		GlobalMD:      "",
		DetectDirs:    []string{".cursor"},
		merge:         mergeMCPServersJSON,
	},
	{
		Name:          "codex",
		Label:         "Codex CLI",
		GlobalConfig:  ".codex/config.toml",
		ProjectConfig: "", // TOML editing stays global-only in V1
		GlobalMD:      ".codex/AGENTS.md",
		DetectDirs:    []string{".codex"},
		merge:         appendCodexTOML,
		HookGlobal:    ".codex/config.toml", // same file as the MCP entry
		mergeHook:     appendCodexStopHookTOML,
	},
	{
		// ZCode nests servers under mcp.servers (not the common mcpServers
		// top level); config.json also holds hooks and plugin state, which
		// mergeNestedMap preserves untouched. Stop hooks live under
		// hooks.events.Stop and REQUIRE hooks.enabled=true; project-level
		// hook configs are ignored by ZCode, so hook registration is
		// global-only.
		Name:          "zcode",
		Label:         "ZCode",
		GlobalConfig:  ".zcode/cli/config.json",
		ProjectConfig: ".zcode/config.json",
		GlobalMD:      ".zcode/AGENTS.md",
		DetectDirs:    []string{".zcode"},
		merge:         mergeZcodeJSON,
		HookGlobal:    ".zcode/cli/config.json", // same file as the MCP entry
		mergeHook:     mergeZcodeStopHookJSON,
	},
	{
		Name:          "kiro",
		Label:         "Kiro",
		GlobalConfig:  ".kiro/settings/mcp.json",
		ProjectConfig: ".kiro/settings/mcp.json",
		GlobalMD:      "", // steering files are multi-file with frontmatter; out of scope V1
		DetectDirs:    []string{".kiro"},
		merge:         mergeMCPServersJSON,
		// No hook registration: Kiro hooks are IDE-managed markdown steering
		// files, not file-configurable (verified 2026-08-22). AGENTS.md
		// instruction covers the workflow instead.
	},
}

// mergeOpencodeJSON upserts mcp.codedocket into opencode's { "mcp": { ... } } map,
// preserving all unrelated keys. Whole-file rewrite with sorted keys; a
// .bak is made by the caller before writing.
func mergeOpencodeJSON(existing []byte, binPath string) ([]byte, bool, error) {
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(
		`{"type":"local","command":["`+binPath+`","serve"],"enabled":true}`), &want); err != nil {
		return nil, false, err // binPath escaping bug; unreachable for sane paths
	}
	return mergeNestedMap(existing, []string{"mcp", "codedocket"}, want)
}

// mergeMCPServersJSON upserts the { "mcpServers": { "codedocket": ... } } shape used
// by Claude Code, Cursor, and Kiro.
func mergeMCPServersJSON(existing []byte, binPath string) ([]byte, bool, error) {
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(
		`{"command":"`+binPath+`","args":["serve"]}`), &want); err != nil {
		return nil, false, err // binPath escaping bug; unreachable for sane paths
	}
	return mergeNestedMap(existing, []string{"mcpServers", "codedocket"}, want)
}

// mergeZcodeJSON upserts the { "mcp": { "servers": { "codedocket": ... } } } shape
// used by ZCode's .zcode/cli/config.json.
func mergeZcodeJSON(existing []byte, binPath string) ([]byte, bool, error) {
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(
		`{"command":"`+binPath+`","args":["serve"]}`), &want); err != nil {
		return nil, false, err
	}
	return mergeNestedMap(existing, []string{"mcp", "servers", "codedocket"}, want)
}

// mergeNestedMap upserts root[sections...] = want into arbitrary JSON, creating
// intermediate objects as needed. Idempotent: DeepEqual against the desired
// entry means "Unchanged".
func mergeNestedMap(existing []byte, sections []string, want map[string]interface{}) ([]byte, bool, error) {
	root := map[string]interface{}{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, false, fmt.Errorf("existing config is not valid JSON (leaving untouched): %w", err)
		}
	}
	cur := root
	for _, sec := range sections[:len(sections)-1] {
		next, _ := cur[sec].(map[string]interface{})
		if next == nil {
			next = map[string]interface{}{}
			cur[sec] = next
		}
		cur = next
	}
	last := sections[len(sections)-1]
	if reflect.DeepEqual(cur[last], want) {
		return existing, false, nil
	}
	cur[last] = want
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

// appendCodexTOML adds a [mcp_servers.codedocket] block if absent. No TOML parser:
// presence detection is the literal section header; anything non-trivial is
// deferred until a real need proves it.
func appendCodexTOML(existing []byte, binPath string) ([]byte, bool, error) {
	if bytes.Contains(existing, []byte("[mcp_servers.codedocket]")) {
		return existing, false, nil
	}
	block := fmt.Sprintf("[mcp_servers.codedocket]\ncommand = %q\nargs = [\"serve\"]\n", binPath)
	trimmed := bytes.TrimRight(existing, "\n")
	if len(bytes.TrimSpace(trimmed)) == 0 {
		return []byte(block), true, nil
	}
	return append(append(trimmed, '\n', '\n'), []byte(block)...), true, nil
}

// --- Stop-hook registration (M6 Task 4) ---
//
// Shapes verified against each client's docs 2026-08-22:
//   claude: settings.json  hooks.Stop[] groups, exec-form handlers
//   codex:  config.toml    [[hooks.Stop]] / [[hooks.Stop.hooks]], command string
//   zcode:  config.json    hooks.events.Stop[] groups + hooks.enabled=true
// Presence: JSON clients match a handler whose args are exactly
// ["hook","stop","--client",<name>] (path-independent, so reinstalls to a
// new binPath never duplicate); the Codex TOML form matches its literal
// command string.

// mergeClaudeStopHookJSON appends our matcher group to hooks.Stop[].
func mergeClaudeStopHookJSON(existing []byte, binPath string) ([]byte, bool, error) {
	group := map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": binPath,
				"args":    []string{"hook", "stop", "--client", "claude"},
			},
		},
	}
	return upsertStopHookArray(existing, []string{"hooks", "Stop"}, group, nil, "claude")
}

// mergeZcodeStopHookJSON appends our group to hooks.events.Stop[] and
// force-enables config-file hooks (they stay inert without
// hooks.enabled=true).
func mergeZcodeStopHookJSON(existing []byte, binPath string) ([]byte, bool, error) {
	group := map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "process",
				"command": binPath,
				"args":    []string{"hook", "stop", "--client", "zcode"},
				"enabled": true,
			},
		},
	}
	return upsertStopHookArray(existing, []string{"hooks", "events", "Stop"}, group,
		map[string]interface{}{"enabled": true}, "zcode")
}

// appendCodexStopHookTOML appends a [[hooks.Stop]] group with one command
// handler. Appending our own [[hooks.Stop]] header first guarantees the
// nested [[hooks.Stop.hooks]] attaches to our group, not a user's.
// Presence marker is the subcommand signature, not the binary name —
// dev builds are not named "codedocket" and a name-based literal would
// duplicate on every run.
func appendCodexStopHookTOML(existing []byte, binPath string) ([]byte, bool, error) {
	if bytes.Contains(existing, []byte("hook stop --client codex")) {
		return existing, false, nil
	}
	block := fmt.Sprintf("[[hooks.Stop]]\n[[hooks.Stop.hooks]]\ntype = \"command\"\ncommand = %q\ntimeout = 30\n",
		binPath+" hook stop --client codex")
	trimmed := bytes.TrimRight(existing, "\n")
	if len(bytes.TrimSpace(trimmed)) == 0 {
		return []byte(block), true, nil
	}
	return append(append(trimmed, '\n', '\n'), []byte(block)...), true, nil
}

// upsertStopHookArray idempotently appends one Stop group at
// root[sections...] (an array). Presence = an existing group carrying a
// handler with our exact args (hook stop --client <client>) — independent
// of the command path, so reinstalls to a new binPath don't duplicate.
// ensure (optional) sets key/values on the FIRST section object (e.g.
// zcode's hooks.enabled) when absent or different. Unrelated keys are
// preserved; output is MarshalIndent round-tripped like every other JSON
// merge here.
func upsertStopHookArray(existing []byte, sections []string, group map[string]interface{}, ensure map[string]interface{}, client string) ([]byte, bool, error) {
	root := map[string]interface{}{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, false, fmt.Errorf("existing config is not valid JSON (leaving untouched): %w", err)
		}
	}
	changed := false
	cur := root
	for i, sec := range sections[:len(sections)-1] {
		next, _ := cur[sec].(map[string]interface{})
		if next == nil {
			next = map[string]interface{}{}
			cur[sec] = next
			changed = true
		}
		if i == 0 && ensure != nil {
			for k, v := range ensure {
				if !reflect.DeepEqual(next[k], v) {
					next[k] = v
					changed = true
				}
			}
		}
		cur = next
	}
	last := sections[len(sections)-1]
	arr, _ := cur[last].([]interface{})
	if arr == nil {
		arr = []interface{}{}
		changed = true
	}
	// The command we want is the one in the group we would insert.
	wantCmd := ""
	if gh, ok := group["hooks"].([]interface{}); ok && len(gh) > 0 {
		if hm, ok := gh[0].(map[string]interface{}); ok {
			wantCmd, _ = hm["command"].(string)
		}
	}
	present := false
	for _, el := range arr {
		grp, ok := el.(map[string]interface{})
		if !ok {
			continue
		}
		hooks, _ := grp["hooks"].([]interface{})
		for _, h := range hooks {
			hm, ok := h.(map[string]interface{})
			if !ok || !ourHookArgs(hm, client) {
				continue
			}
			present = true
			// Update-in-place on binary drift (same semantics as the JSON
			// MCP merges): the stable ~/.local/bin path normally makes this
			// a no-op, but --skip-install dev builds move around.
			if hm["command"] != wantCmd {
				hm["command"] = wantCmd
				changed = true
			}
		}
	}
	if present && !changed {
		return existing, false, nil
	}
	if !present {
		arr = append(arr, group)
		cur[last] = arr
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

// hasStopHook reports whether a Stop array element is one of our groups:
// any handler whose args are exactly ["hook","stop","--client",client].
// JSON round-trips make args a []interface{} of strings.
func hasStopHook(el interface{}, client string) bool {
	group, ok := el.(map[string]interface{})
	if !ok {
		return false
	}
	hooks, _ := group["hooks"].([]interface{})
	for _, h := range hooks {
		hm, ok := h.(map[string]interface{})
		if ok && ourHookArgs(hm, client) {
			return true
		}
	}
	return false
}

// ourHookArgs matches a handler map carrying our exact argv.
func ourHookArgs(hm map[string]interface{}, client string) bool {
	args, _ := hm["args"].([]interface{})
	if len(args) != 4 {
		return false
	}
	s0, _ := args[0].(string)
	s1, _ := args[1].(string)
	s2, _ := args[2].(string)
	s3, _ := args[3].(string)
	return s0 == "hook" && s1 == "stop" && s2 == "--client" && s3 == client
}
