package codedocket

import (
	"strings"
	"testing"
	"time"
)

var hookT0 = time.Date(2026, 8, 22, 11, 0, 0, 0, time.UTC)

func pendingFixture(ids ...string) []SessionInfo {
	var out []SessionInfo
	for i, id := range ids {
		out = append(out, SessionInfo{
			ID:      id,
			Dir:     "/ignore/" + id,
			Notes:   2,
			FirstAt: hookT0.Add(time.Duration(i) * time.Hour), // distinct, ascending
		})
	}
	return out
}

func TestStopHookResponseShapes(t *testing.T) {
	pending := pendingFixture("cli-20260822T110000Z")

	// Exact literals, pinned from each client's docs (2026-08-22). Field
	// order and compact JSON are part of the contract.
	reason := "codedocket: session cli-20260822T110000Z has 2 unreviewed notes. Run `codedocket finalize`, record accepted proposals with `codedocket record`, then finish."
	jsonWant := `{"decision":"block","reason":"` + reason + `"}`

	for _, client := range []string{"claude", "codex", "zcode"} {
		got, err := StopHookResponse(client, pending)
		if err != nil {
			t.Fatalf("%s: %v", client, err)
		}
		if got != jsonWant {
			t.Errorf("%s shape mismatch:\n got: %s\nwant: %s", client, got, jsonWant)
		}
	}

	got, err := StopHookResponse("kiro", pending)
	if err != nil {
		t.Fatal(err)
	}
	if got != reason || strings.Contains(got, `"decision"`) {
		t.Errorf("kiro must emit plain text, got: %s", got)
	}
}

func TestStopHookResponseSilentAllow(t *testing.T) {
	for _, client := range SupportedHookClients {
		got, err := StopHookResponse(client, nil)
		if err != nil || got != "" {
			t.Errorf("%s: empty pending must be silent allow, got %q, %v", client, got, err)
		}
	}
}

func TestStopHookResponseUnknownClient(t *testing.T) {
	_, err := StopHookResponse("vscode", pendingFixture("x"))
	if err == nil {
		t.Fatal("unknown client must error")
	}
	for _, c := range SupportedHookClients {
		if !strings.Contains(err.Error(), c) {
			t.Errorf("error must list %s: %v", c, err)
		}
	}
}

func TestStopBlockReasonNewestAndOthers(t *testing.T) {
	pending := pendingFixture("cli-20260101T000000Z", "zcode-20260201T000000Z", "cli-20260301T000000Z")

	// The gated client's OWN newest session is named — even when another
	// client's id would sort later lexicographically (zcode-… > cli-…).
	reason := StopBlockReason("zcode", pending)
	if !strings.Contains(reason, "session zcode-20260201T000000Z has 2 unreviewed notes") {
		t.Errorf("reason must name the client's own newest session: %s", reason)
	}

	// No own session → global newest by FirstAt (March), not by id string
	// (which would pick zcode-February).
	reason = StopBlockReason("claude", pending)
	if !strings.Contains(reason, "session cli-20260301T000000Z") {
		t.Errorf("fallback must pick newest by FirstAt, not id order: %s", reason)
	}

	if !strings.Contains(reason, "2 other pending sessions") {
		t.Errorf("reason must count other pending sessions: %s", reason)
	}
	if !strings.Contains(reason, "codedocket finalize") {
		t.Errorf("reason must teach the fix: %s", reason)
	}
}

func TestStopBlockReasonPluralization(t *testing.T) {
	one := pendingFixture("cli-x")[0]
	one.Notes = 1
	reason := StopBlockReason("cli", []SessionInfo{one})
	if !strings.Contains(reason, "1 unreviewed note.") {
		t.Errorf("singular form wrong: %s", reason)
	}
	if strings.Contains(reason, "other pending") {
		t.Errorf("single session must not mention others: %s", reason)
	}
}
