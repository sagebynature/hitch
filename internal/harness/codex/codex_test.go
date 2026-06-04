package codex

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestMapPreToolUse(t *testing.T) {
	env, err := (Mapper{}).Map("PreToolUse", protocol.Raw(map[string]interface{}{"tool": "bash"}))
	if err != nil {
		t.Fatal(err)
	}
	if env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("got %s", env.HitchEventType)
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
