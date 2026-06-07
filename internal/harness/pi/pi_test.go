package pi

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizePromotesPiMetadataEnvelope(t *testing.T) {
	payload := protocol.RawJSON(`{"event":{"input":{"command":"pwd"},"turnIndex":2},"metadata":{"session_id":"session-1","turn_id":"turn-2","cwd":"/tmp/project","model":"anthropic/claude","transcript_path":"/tmp/session-1.jsonl"}}`)
	env, err := (Mapper{}).Normalize("tool_call", payload, protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session-1" || env.TurnID != "turn-2" || env.CWD != "/tmp/project" || env.Model != "anthropic/claude" || env.TranscriptPath != "/tmp/session-1.jsonl" {
		t.Fatalf("metadata not promoted: %#v", env)
	}
	var typed map[string]interface{}
	if err := json.Unmarshal(env.Payload, &typed); err != nil {
		t.Fatal(err)
	}
	tool := typed["tool"].(map[string]interface{})
	if tool["command"] != "pwd" {
		t.Fatalf("payload should be typed tool payload, got %s", env.Payload)
	}
	if string(env.SourcePayload) != `{"input":{"command":"pwd"},"turnIndex":2}` {
		t.Fatalf("source payload should be unwrapped Pi event, got %s", env.SourcePayload)
	}
}

func TestNormalizeKeepsBarePiPayloadCompatible(t *testing.T) {
	payload := protocol.RawJSON(`{"input":{"command":"pwd"}}`)
	env, err := (Mapper{}).Normalize("tool_call", payload, protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	var typed map[string]interface{}
	if err := json.Unmarshal(env.Payload, &typed); err != nil {
		t.Fatal(err)
	}
	tool := typed["tool"].(map[string]interface{})
	if env.SessionID != "" || env.CWD != "" || tool["command"] != "pwd" {
		t.Fatalf("bare payload normalization changed: %#v", env)
	}
}

func TestTranslateNoneReturnsEmptyPassThrough(t *testing.T) {
	out, err := (Mapper{}).Translate("input", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorNone}})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "{}" {
		t.Fatalf("none decision should produce empty pass-through payload, got %s", out)
	}
}
func TestToolCallBlock(t *testing.T) {
	out, err := (Mapper{}).Translate("tool_call", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorBlock, Reason: "no"}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "return" {
		t.Fatalf("got %#v", got)
	}
}

func TestToolCallMutate(t *testing.T) {
	out, err := (Mapper{}).Translate("tool_call", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorTransform, UpdatedInput: protocol.Raw(map[string]interface{}{"command": "echo hi"})}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "mutate_and_return" || len(got.Mutations) != 1 {
		t.Fatalf("got %#v", got)
	}
}

func TestUserCommandUsesToolControlTranslation(t *testing.T) {
	out, err := (Mapper{}).Translate("user_bash", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorDeny, Reason: "no"}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "return" {
		t.Fatalf("got %#v", got)
	}
}

func TestCapabilityClassifiesSourceEvents(t *testing.T) {
	if got := (Mapper{}).Capability("user_bash"); got != core.CapabilityControlCapable {
		t.Fatalf("user_bash capability = %s", got)
	}
	if got := (Mapper{}).Capability("tool_call"); got != core.CapabilityControlCapable {
		t.Fatalf("tool_call capability = %s", got)
	}
	if got := (Mapper{}).Capability("turn_end"); got != core.CapabilityObserverOnly {
		t.Fatalf("turn_end capability = %s", got)
	}
}
