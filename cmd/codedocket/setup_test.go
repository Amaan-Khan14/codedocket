package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeOpencodeJSON(t *testing.T) {
	bin := "/home/u/.local/bin/codedocket"

	// empty file
	merged, changed, err := mergeOpencodeJSON(nil, bin)
	if err != nil || !changed {
		t.Fatalf("empty: changed=%v err=%v", changed, err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatal(err)
	}
	mcp := root["mcp"].(map[string]interface{})
	entry := mcp["codedocket"].(map[string]interface{})
	if entry["type"] != "local" || entry["enabled"] != true {
		t.Fatalf("entry: %+v", entry)
	}
	cmd := entry["command"].([]interface{})
	if cmd[0] != bin || cmd[1] != "serve" {
		t.Fatalf("command: %v", cmd)
	}

	// preserving unrelated keys
	existing := []byte(`{"model": "x", "mcp": {"codegraph": {"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
	merged2, changed2, err := mergeOpencodeJSON(existing, bin)
	if err != nil || !changed2 {
		t.Fatalf("preserve: changed=%v err=%v", changed2, err)
	}
	var root2 map[string]interface{}
	json.Unmarshal(merged2, &root2)
	if root2["model"] != "x" {
		t.Fatal("unrelated top-level key lost")
	}
	mcp2 := root2["mcp"].(map[string]interface{})
	if _, ok := mcp2["codegraph"]; !ok {
		t.Fatal("existing sibling server lost")
	}
	if _, ok := mcp2["codedocket"]; !ok {
		t.Fatal("codedocket not added")
	}

	// idempotent: merging the merged output reports unchanged
	merged3, changed3, err := mergeOpencodeJSON(merged2, bin)
	if err != nil {
		t.Fatal(err)
	}
	if changed3 {
		t.Fatalf("second merge should be Unchanged")
	}
	if string(merged3) != string(merged2) {
		t.Fatal("unchanged result differs from input")
	}

	// invalid JSON surfaces, nothing written
	if _, _, err := mergeOpencodeJSON([]byte(`{bad`), bin); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestMergeMCPServersJSON(t *testing.T) {
	bin := "/x/codedocket"
	merged, changed, err := mergeMCPServersJSON(nil, bin)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var root map[string]interface{}
	json.Unmarshal(merged, &root)
	servers := root["mcpServers"].(map[string]interface{})
	entry := servers["codedocket"].(map[string]interface{})
	if entry["command"] != bin || entry["args"].([]interface{})[0] != "serve" {
		t.Fatalf("entry: %+v", entry)
	}

	if _, changed, _ := mergeMCPServersJSON(merged, bin); changed {
		t.Fatal("second merge should be Unchanged")
	}
}

func TestMergeZcodeJSON(t *testing.T) {
	bin := "/x/codedocket"

	// empty file: creates the full mcp.servers.codedocket nesting
	merged, changed, err := mergeZcodeJSON(nil, bin)
	if err != nil || !changed {
		t.Fatalf("empty: changed=%v err=%v", changed, err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatal(err)
	}
	servers := root["mcp"].(map[string]interface{})["servers"].(map[string]interface{})
	entry := servers["codedocket"].(map[string]interface{})
	if entry["command"] != bin || entry["args"].([]interface{})[0] != "serve" {
		t.Fatalf("entry: %+v", entry)
	}

	// preserves sibling servers and unrelated config (hooks, plugin state)
	existing := []byte(`{
	  "hooks": {"enabled": true},
	  "plugins": {"marketplaces": []},
	  "mcp": {"servers": {"other": {"command": "other", "args": ["x"]}}}
	}`)
	merged2, changed2, err := mergeZcodeJSON(existing, bin)
	if err != nil || !changed2 {
		t.Fatalf("preserve: changed=%v err=%v", changed2, err)
	}
	var root2 map[string]interface{}
	json.Unmarshal(merged2, &root2)
	servers2 := root2["mcp"].(map[string]interface{})["servers"].(map[string]interface{})
	if _, ok := servers2["other"]; !ok {
		t.Fatal("existing sibling server lost")
	}
	if _, ok := servers2["codedocket"]; !ok {
		t.Fatal("codedocket not added")
	}
	if root2["hooks"] == nil || root2["plugins"] == nil {
		t.Fatal("unrelated config keys lost")
	}

	// idempotent
	if _, changed3, _ := mergeZcodeJSON(merged2, bin); changed3 {
		t.Fatal("second merge should be Unchanged")
	}
}

func TestAppendCodexTOML(t *testing.T) {
	bin := "/x/codedocket"

	out, changed, err := appendCodexTOML(nil, bin)
	if err != nil || !changed {
		t.Fatalf("empty: changed=%v err=%v", changed, err)
	}
	if !strings.HasPrefix(string(out), "[mcp_servers.codedocket]") {
		t.Fatalf("block placement:\n%s", out)
	}

	existing := []byte("[profiles.default]\nname = \"work\"\n")
	out2, changed2, _ := appendCodexTOML(existing, bin)
	if !changed2 {
		t.Fatal("expected change")
	}
	if !strings.Contains(string(out2), "[profiles.default]") || !strings.Contains(string(out2), "[mcp_servers.codedocket]") {
		t.Fatalf("existing TOML not preserved:\n%s", out2)
	}

	if _, changed3, _ := appendCodexTOML(out2, "/other/path/codedocket"); changed3 {
		t.Fatal("second append must be no-op regardless of binPath")
	}
}

func TestMergeClaudeStopHookJSON(t *testing.T) {
	bin := "/x/codedocket"

	merged, changed, err := mergeClaudeStopHookJSON(nil, bin)
	if err != nil || !changed {
		t.Fatalf("empty: changed=%v err=%v", changed, err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatal(err)
	}
	stop := root["hooks"].(map[string]interface{})["Stop"].([]interface{})
	group := stop[0].(map[string]interface{})
	h := group["hooks"].([]interface{})[0].(map[string]interface{})
	if h["command"] != bin || h["type"] != "command" {
		t.Fatalf("handler: %+v", h)
	}
	args := h["args"].([]interface{})
	if args[0] != "hook" || args[1] != "stop" || args[3] != "claude" {
		t.Fatalf("args: %v", args)
	}

	// preserves unrelated settings and a user's own Stop group
	existing := []byte(`{"model": "opus", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "lint"}]}]}}`)
	merged2, changed2, err := mergeClaudeStopHookJSON(existing, bin)
	if err != nil || !changed2 {
		t.Fatalf("preserve: changed=%v err=%v", changed2, err)
	}
	var root2 map[string]interface{}
	json.Unmarshal(merged2, &root2)
	if root2["model"] != "opus" {
		t.Fatal("unrelated setting lost")
	}
	stop2 := root2["hooks"].(map[string]interface{})["Stop"].([]interface{})
	if len(stop2) != 2 {
		t.Fatalf("expected user group + our group, got %d", len(stop2))
	}

	// idempotent
	if _, changed3, _ := mergeClaudeStopHookJSON(merged2, bin); changed3 {
		t.Fatal("second merge should be Unchanged")
	}
}

func TestMergeZcodeStopHookCoexistsWithMCP(t *testing.T) {
	bin := "/x/codedocket"

	// MCP entry first (the setup apply order), then the hook.
	withMCP, _, err := mergeZcodeJSON(nil, bin)
	if err != nil {
		t.Fatal(err)
	}
	merged, changed, err := mergeZcodeStopHookJSON(withMCP, bin)
	if err != nil || !changed {
		t.Fatalf("hook: changed=%v err=%v", changed, err)
	}

	var root map[string]interface{}
	json.Unmarshal(merged, &root)
	// MCP entry intact
	servers := root["mcp"].(map[string]interface{})["servers"].(map[string]interface{})
	if _, ok := servers["codedocket"]; !ok {
		t.Fatal("mcp entry lost when adding hook")
	}
	// hooks.enabled forced true
	hooks := root["hooks"].(map[string]interface{})
	if hooks["enabled"] != true {
		t.Fatalf("hooks.enabled = %v, want true", hooks["enabled"])
	}
	// our group under events.Stop with a process executor
	stop := hooks["events"].(map[string]interface{})["Stop"].([]interface{})
	group := stop[0].(map[string]interface{})
	h := group["hooks"].([]interface{})[0].(map[string]interface{})
	if h["type"] != "process" || h["command"] != bin || h["enabled"] != true {
		t.Fatalf("handler: %+v", h)
	}

	// idempotent even alongside the mcp entry (presence rule is hook-scoped)
	if _, changed2, _ := mergeZcodeStopHookJSON(merged, bin); changed2 {
		t.Fatal("second hook merge should be Unchanged")
	}

	// binary drift updates our handler's command in place (JSON MCP semantics)
	outDrift, changedDrift, err := mergeZcodeStopHookJSON(merged, "/new/path/cdt-dev")
	if err != nil || !changedDrift {
		t.Fatalf("drift: changed=%v err=%v", changedDrift, err)
	}
	var rootD map[string]interface{}
	json.Unmarshal(outDrift, &rootD)
	dHooks := rootD["hooks"].(map[string]interface{})["events"].(map[string]interface{})["Stop"].([]interface{})
	dHandler := dHooks[0].(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
	if dHandler["command"] != "/new/path/cdt-dev" {
		t.Fatalf("command not updated on drift: %v", dHandler["command"])
	}

	// enabled:false gets flipped (config-file hooks are inert without it)
	disabled := []byte(`{"hooks": {"enabled": false}}`)
	out, changed3, err := mergeZcodeStopHookJSON(disabled, bin)
	if err != nil || !changed3 {
		t.Fatalf("enable flip: changed=%v err=%v", changed3, err)
	}
	var root3 map[string]interface{}
	json.Unmarshal(out, &root3)
	if root3["hooks"].(map[string]interface{})["enabled"] != true {
		t.Fatal("enabled:false must be flipped to true")
	}
}

func TestAppendCodexStopHookTOML(t *testing.T) {
	bin := "/x/cdt-dev-build" // deliberately NOT named codedocket: presence must not depend on the binary name

	out, changed, err := appendCodexStopHookTOML(nil, bin)
	if err != nil || !changed {
		t.Fatalf("empty: changed=%v err=%v", changed, err)
	}
	s := string(out)
	for _, want := range []string{"[[hooks.Stop]]", "[[hooks.Stop.hooks]]", `type = "command"`, "hook stop --client codex", "timeout = 30"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}

	// preserves the MCP block that shares the file
	withMCP := []byte("[mcp_servers.codedocket]\ncommand = \"/x/codedocket\"\nargs = [\"serve\"]\n")
	out2, changed2, _ := appendCodexStopHookTOML(withMCP, bin)
	if !changed2 || !strings.Contains(string(out2), "[mcp_servers.codedocket]") {
		t.Fatalf("mcp block not preserved:\n%s", out2)
	}

	// idempotent — and never clobbers regardless of binPath drift
	if _, changed3, _ := appendCodexStopHookTOML(out2, "/other/codedocket"); changed3 {
		t.Fatal("second append must be no-op")
	}
}

func TestClientDetected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if clientDetected(home, knownClients[0]) {
		t.Fatal("opencode should not be detected in empty home")
	}
	mkdir(t, home, ".config/opencode")
	if !clientDetected(home, knownClients[0]) {
		t.Fatal("opencode should be detected once .config/opencode exists")
	}
}

func TestSelectClientsByName(t *testing.T) {
	got, err := selectClients(t.TempDir(), "opencode,claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Label != "opencode" || got[1].Label != "Claude Code" {
		t.Fatalf("selection: %+v", got)
	}
	if _, err := selectClients(t.TempDir(), "nope", false); err == nil {
		t.Fatal("unknown client must error")
	}

	// short names work for display-named clients, including zcode and kiro
	got2, err := selectClients(t.TempDir(), "codex,zcode,kiro", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 3 || got2[1].Label != "ZCode" || got2[2].Label != "Kiro" {
		t.Fatalf("new clients: %+v", got2)
	}
	// the error message must name every known client (no drift)
	_, err = selectClients(t.TempDir(), "nope", false)
	var known []string
	for _, c := range knownClients {
		known = append(known, c.Name)
	}
	for _, name := range known {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error %q missing known client %q", err, name)
		}
	}
}

func TestInstallBinarySamePathNoTruncation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "codedocket")
	payload := []byte("FAKE BINARY PAYLOAD — MUST SURVIVE")
	if err := os.WriteFile(target, payload, 0o755); err != nil {
		t.Fatal(err)
	}

	// Running setup from the installed location hits exactly this: source
	// and target are one file. O_TRUNC onto the open source would zero it.
	if err := installBinary(target, target); err != nil {
		t.Fatalf("same-path install: %v", err)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, payload) {
		t.Fatalf("same-path install corrupted the binary: %d bytes → %d bytes", len(payload), len(after))
	}
}

func mkdir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
}
