package pi

import (
	"encoding/json"
	"testing"

	"github.com/sage-scm/hitch/internal/protocol"
)

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
