package codex

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizePreToolUse(t *testing.T) {
	env, err := (Mapper{}).Normalize("PreToolUse", protocol.Raw(map[string]interface{}{"tool": "bash"}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("got %s", env.HitchEventType)
	}
}

func TestNormalizeCopiesSourceMetadata(t *testing.T) {
	env, err := (Mapper{}).Normalize("PreToolUse", protocol.Raw(map[string]interface{}{
		"session_id":      "session_1",
		"turn_id":         "turn_1",
		"cwd":             "/tmp/hitch",
		"model":           "gpt-test",
		"transcript_path": "/tmp/transcript.jsonl",
	}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session_1" || env.TurnID != "turn_1" || env.CWD != "/tmp/hitch" || env.Model != "gpt-test" || env.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Fatalf("metadata not copied: %#v", env)
	}
}

func TestTranslatePermissionDeny(t *testing.T) {
	out, err := (Mapper{}).Translate("PermissionRequest", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorDeny, Reason: "no"}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["hookSpecificOutput"] == nil {
		t.Fatalf("missing hookSpecificOutput: %s", string(out))
	}
}

func TestCapabilityClassifiesSourceEvents(t *testing.T) {
	if got := (Mapper{}).Capability("PreToolUse"); got != core.CapabilityControlCapable {
		t.Fatalf("PreToolUse capability = %s", got)
	}
	if got := (Mapper{}).Capability("CustomObserver"); got != core.CapabilityObserverOnly {
		t.Fatalf("CustomObserver capability = %s", got)
	}
}
