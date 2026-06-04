package hermes

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

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
