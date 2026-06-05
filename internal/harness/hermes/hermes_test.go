package hermes

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizeCopiesSourceMetadata(t *testing.T) {
	env, err := (Mapper{}).Normalize("pre_llm_call", protocol.Raw(map[string]interface{}{
		"session_id": "session_1",
		"turn_id":    "turn_1",
		"cwd":        "/tmp/hitch",
	}), protocol.EventTurnStarted)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session_1" || env.TurnID != "turn_1" || env.CWD != "/tmp/hitch" {
		t.Fatalf("metadata not copied: %#v", env)
	}
}

func TestNormalizeUsesHermesTaskIDFallback(t *testing.T) {
	env, err := (Mapper{}).Normalize("pre_tool_call", protocol.Raw(map[string]interface{}{
		"session_id": "",
		"cwd":        "/tmp/hitch",
		"extra": map[string]interface{}{
			"task_id": "20260605_005702_b7deee",
		},
	}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "20260605_005702_b7deee" || env.CWD != "/tmp/hitch" {
		t.Fatalf("metadata not copied from fallback: %#v", env)
	}
}

func TestNormalizeCopiesHermesNestedTurnAndModelMetadata(t *testing.T) {
	env, err := (Mapper{}).Normalize("pre_tool_call", protocol.Raw(map[string]interface{}{
		"session_id": "session_1",
		"cwd":        "/tmp/hitch",
		"extra": map[string]interface{}{
			"turn_id": "turn_nested",
			"model":   "gpt-test",
		},
	}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session_1" || env.TurnID != "turn_nested" || env.Model != "gpt-test" {
		t.Fatalf("nested metadata not copied: %#v", env)
	}
}

func TestNormalizeIgnoresHermesDefaultTaskID(t *testing.T) {
	env, err := (Mapper{}).Normalize("transform_terminal_output", protocol.Raw(map[string]interface{}{
		"session_id": "",
		"cwd":        "/tmp/hitch",
		"extra": map[string]interface{}{
			"task_id": "default",
		},
	}), protocol.EventToolCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "" || env.CWD != "/tmp/hitch" {
		t.Fatalf("unexpected metadata fallback: %#v", env)
	}
}

func TestTranslatePreToolBlock(t *testing.T) {
	out, err := (Mapper{}).Translate("pre_tool_call", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorBlock, Reason: "no"}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["action"] != "block" {
		t.Fatalf("got %s", out)
	}
}

func TestTranslateGatewayRewriteUnwrapsJSONString(t *testing.T) {
	out, err := (Mapper{}).Translate("pre_gateway_dispatch", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorTransform, UpdatedInput: protocol.Raw("rewrite me")}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["action"] != "rewrite" || got["message"] != "rewrite me" {
		t.Fatalf("got %s", out)
	}
}

func TestTranslateTransformResultUnwrapsJSONString(t *testing.T) {
	out, err := (Mapper{}).Translate("transform_tool_result", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorReplaceResult, UpdatedOutput: protocol.Raw("new result")}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["result"] != "new result" {
		t.Fatalf("got %s", out)
	}
}
