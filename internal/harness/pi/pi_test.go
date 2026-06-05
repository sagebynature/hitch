package pi

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestMapPromotesPiMetadataEnvelope(t *testing.T) {
	payload := protocol.RawJSON(`{"event":{"input":{"command":"pwd"},"turnIndex":2},"metadata":{"session_id":"session-1","turn_id":"turn-2","cwd":"/tmp/project","model":"anthropic/claude","transcript_path":"/tmp/session-1.jsonl"}}`)
	env, err := (Mapper{}).Map("tool_call", payload)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session-1" || env.TurnID != "turn-2" || env.CWD != "/tmp/project" || env.Model != "anthropic/claude" || env.TranscriptPath != "/tmp/session-1.jsonl" {
		t.Fatalf("metadata not promoted: %#v", env)
	}
	if string(env.Payload) != `{"input":{"command":"pwd"},"turnIndex":2}` {
		t.Fatalf("payload should be unwrapped Pi event, got %s", env.Payload)
	}
	if string(env.NativePayload) != `{"input":{"command":"pwd"},"turnIndex":2}` {
		t.Fatalf("native payload should be unwrapped Pi event, got %s", env.NativePayload)
	}
}

func TestMapKeepsBarePiPayloadCompatible(t *testing.T) {
	payload := protocol.RawJSON(`{"input":{"command":"pwd"}}`)
	env, err := (Mapper{}).Map("tool_call", payload)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "" || env.CWD != "" || string(env.Payload) != string(payload) {
		t.Fatalf("bare payload compatibility changed: %#v", env)
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
