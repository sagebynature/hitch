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
