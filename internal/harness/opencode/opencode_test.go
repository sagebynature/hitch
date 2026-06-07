package opencode

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizeUnwrapsPluginEnvelopeAndMetadata(t *testing.T) {
	source := protocol.Raw(map[string]interface{}{
		"event": map[string]interface{}{
			"input":  map[string]interface{}{"tool": "bash", "sessionID": "sess_1", "callID": "call_1"},
			"output": map[string]interface{}{"args": map[string]interface{}{"command": "pwd"}},
		},
		"metadata": map[string]interface{}{
			"session_id":      "sess_1",
			"turn_id":         "msg_1",
			"cwd":             "/tmp/project",
			"model":           "anthropic/claude-sonnet-4",
			"transcript_path": "/tmp/opencode/session.json",
		},
	})

	env, err := Mapper{}.Normalize("tool.execute.before", source, protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.Harness != protocol.HarnessOpenCode || env.SourceEventType != "tool.execute.before" || env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	if env.SessionID != "sess_1" || env.TurnID != "msg_1" || env.CWD != "/tmp/project" || env.Model != "anthropic/claude-sonnet-4" || env.TranscriptPath != "/tmp/opencode/session.json" {
		t.Fatalf("metadata not copied: %#v", env)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["input"]; !ok {
		t.Fatalf("payload was not unwrapped: %#v", payload)
	}
}

func TestTranslateBlocksToolExecuteBeforeByThrowing(t *testing.T) {
	native, err := Mapper{}.Translate("tool.execute.before", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorBlock, Reason: "blocked"}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(native, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "throw" || got.Message != "blocked" {
		t.Fatalf("unexpected native response: %#v", got)
	}
}

func TestTranslateTransformsToolExecuteBeforeArgs(t *testing.T) {
	native, err := Mapper{}.Translate("tool.execute.before", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorTransform, UpdatedInput: protocol.Raw(map[string]interface{}{"command": "echo ok"})}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(native, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "set" || len(got.Path) != 1 || got.Path[0] != "args" {
		t.Fatalf("unexpected native response: %#v", got)
	}
}

func TestTranslatePermissionAskAllowAndDeny(t *testing.T) {
	allow, err := Mapper{}.Translate("permission.ask", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorAllow}})
	if err != nil {
		t.Fatal(err)
	}
	var allowResp AdapterResponse
	if err := json.Unmarshal(allow, &allowResp); err != nil {
		t.Fatal(err)
	}
	if allowResp.AdapterAction != "set" || allowResp.Value != "allow" {
		t.Fatalf("unexpected allow response: %#v", allowResp)
	}

	deny, err := Mapper{}.Translate("permission.ask", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorDeny, Reason: "no"}})
	if err != nil {
		t.Fatal(err)
	}
	var denyResp AdapterResponse
	if err := json.Unmarshal(deny, &denyResp); err != nil {
		t.Fatal(err)
	}
	if denyResp.AdapterAction != "set" || denyResp.Value != "deny" {
		t.Fatalf("unexpected deny response: %#v", denyResp)
	}
}

func TestTranslateInjectsChatMessageContext(t *testing.T) {
	native, err := Mapper{}.Translate("chat.message", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorInjectContext, Context: "extra context"}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(native, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "inject_context" || got.Value != "extra context" {
		t.Fatalf("unexpected native response: %#v", got)
	}
}

func TestTranslateUsesNativeResponseDirectly(t *testing.T) {
	want := protocol.Raw(map[string]interface{}{"adapter_action": "throw", "message": "native"})
	got, err := Mapper{}.Translate("tool.execute.before", protocol.AggregateDecision{Decision: protocol.Decision{NativeResponse: want}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestCapabilityClassifiesSourceEvents(t *testing.T) {
	if got := (Mapper{}).Capability("tool.execute.before"); got != harness.CapabilityControlCapable {
		t.Fatalf("tool.execute.before capability = %s", got)
	}
	if got := (Mapper{}).Capability("session.idle"); got != harness.CapabilityObserverOnly {
		t.Fatalf("session.idle capability = %s", got)
	}
}

func TestKnownSourceEventsIncludesOpenCodeSDKEvents(t *testing.T) {
	known := Mapper{}.KnownSourceEvents()
	for _, event := range []string{"session.updated", "session.deleted", "session.diff", "session.status", "permission.updated", "permission.replied", "message.updated", "message.part.updated", "file.edited", "server.connected", "pty.exited", "vcs.branch.updated"} {
		if _, ok := known[event]; !ok {
			t.Fatalf("expected %s to be catalog-known", event)
		}
	}
}
