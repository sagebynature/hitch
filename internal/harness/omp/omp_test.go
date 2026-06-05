package omp

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizeOMPToolCall(t *testing.T) {
	env, err := (Mapper{}).Normalize("tool_call", protocol.Raw(map[string]interface{}{"toolName": "bash"}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.Harness != protocol.HarnessOMP || env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("bad env: %#v", env)
	}
}

func TestNormalizeCopiesSourceMetadata(t *testing.T) {
	env, err := (Mapper{}).Normalize("pre_llm_call", protocol.Raw(map[string]interface{}{
		"session_id": "session_1",
		"cwd":        "/tmp/hitch",
	}), protocol.EventTurnStarted)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session_1" || env.CWD != "/tmp/hitch" {
		t.Fatalf("metadata not copied: %#v", env)
	}
}

func TestNormalizePromotesOMPExtensionMetadataEnvelope(t *testing.T) {
	payload := protocol.RawJSON(`{"event":{"input":{"command":"pwd"},"turnIndex":2},"metadata":{"session_id":"session-1","turn_id":"turn-2","cwd":"/tmp/project","model":"anthropic/claude","transcript_path":"/tmp/session-1.jsonl"}}`)
	env, err := (Mapper{}).Normalize("tool_call", payload, protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session-1" || env.TurnID != "turn-2" || env.CWD != "/tmp/project" || env.Model != "anthropic/claude" || env.TranscriptPath != "/tmp/session-1.jsonl" {
		t.Fatalf("metadata not promoted: %#v", env)
	}
	if string(env.Payload) != `{"input":{"command":"pwd"},"turnIndex":2}` {
		t.Fatalf("payload should be unwrapped OMP event, got %s", env.Payload)
	}
}

func TestTranslateOMPCancelableBranch(t *testing.T) {
	out, err := (Mapper{}).Translate("session_before_branch", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorBlock}})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		AdapterAction string         `json:"adapter_action"`
		ReturnValue   map[string]any `json:"return_value"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "return" || got.ReturnValue["cancel"] != true {
		t.Fatalf("expected cancel return, got %#v", got)
	}
}
