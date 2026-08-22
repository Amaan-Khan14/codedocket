package codedocket

import (
	"encoding/json"
	"fmt"
	"strings"
)

// hook stop (M6 Task 3): the Stop gate. Predicate: "≥ 1 pending session?"
// Translator: render allow/block in the client's exact wire shape. The
// hook gates, it never processes — all intelligence stays with the agent,
// which gets the reason as its next instruction while still in context.

// SupportedHookClients lists the clients whose Stop-hook contracts
// StopHookResponse can speak.
var SupportedHookClients = []string{"claude", "codex", "zcode", "kiro"}

type stopBlock struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// StopHookResponse returns the stdout a client's Stop hook should emit for
// the given pending sessions: "" (silent allow) when nothing is pending,
// the client's block shape otherwise. Shapes verified against each
// client's docs 2026-08-22 (claude/codex/zcode: decision+reason JSON;
// kiro: Agent Stop feeds plain-text stdout into the agent's context, no
// JSON decision contract). Unknown client → error.
func StopHookResponse(client string, pending []SessionInfo) (string, error) {
	if !supportedClient(client) {
		return "", fmt.Errorf("unsupported client %q (supported: claude, codex, zcode, kiro)", client)
	}
	if len(pending) == 0 {
		return "", nil // silence is required — noise on every stop gets the hook uninstalled
	}
	reason := StopBlockReason(client, pending)
	if client == "kiro" {
		return reason, nil
	}
	b, err := json.Marshal(stopBlock{Decision: "block", Reason: reason})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// StopBlockReason is the injected instruction, shared copy with the
// finalize review text (edit with care). It names the gated client's own
// newest pending session — falling back to the globally newest when the
// client has none (stale foreign captures still gate) — and mentions any
// others. Newest is by FirstAt, not id: ids sort by name first, so
// lexicographic order is wrong across clients.
func StopBlockReason(client string, pending []SessionInfo) string {
	target := newestPending(pending, client)
	if target == nil {
		target = newestPending(pending, "")
	}
	reason := fmt.Sprintf("codedocket: session %s has %d unreviewed %s. Run `codedocket finalize`, record accepted proposals with `codedocket record`, then finish.",
		target.ID, target.Notes, NotesWord(target.Notes))
	if others := len(pending) - 1; others > 0 {
		sessions := "sessions"
		if others == 1 {
			sessions = "session"
		}
		reason += fmt.Sprintf(" (%d other pending %s — `codedocket finalize --all`)", others, sessions)
	}
	return reason
}

// newestPending returns the session with the greatest FirstAt (ID breaks
// ties), restricted to prefix "<client>-" when client is non-empty.
func newestPending(pending []SessionInfo, client string) *SessionInfo {
	var best *SessionInfo
	for i := range pending {
		s := &pending[i]
		if client != "" && !strings.HasPrefix(s.ID, client+"-") {
			continue
		}
		if best == nil || s.FirstAt.After(best.FirstAt) ||
			(s.FirstAt.Equal(best.FirstAt) && s.ID > best.ID) {
			best = s
		}
	}
	return best
}

func supportedClient(client string) bool {
	for _, c := range SupportedHookClients {
		if c == client {
			return true
		}
	}
	return false
}
