package hermes

import (
	"encoding/json"

	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

var controlCapableEvents = map[string]struct{}{
	"pre_tool_call":             {},
	"pre_llm_call":              {},
	"transform_tool_result":     {},
	"transform_terminal_output": {},
	"transform_llm_output":      {},
	"pre_gateway_dispatch":      {},
}

var knownSourceEvents = harness.SourceEventSet(
	"pre_tool_call",
	"post_tool_call",
	"pre_llm_call",
	"post_llm_call",
	"on_session_start",
	"on_session_end",
	"subagent_stop",
	"transform_tool_result",
	"transform_terminal_output",
	"transform_llm_output",
	"pre_gateway_dispatch",
)

func (Mapper) KnownSourceEvents() map[string]struct{} {
	return knownSourceEvents
}

func (Mapper) Capability(sourceEventType string) harness.SourceEventCapability {
	return harness.CapabilityFromSet(sourceEventType, controlCapableEvents)
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := harness.NewEnvelope(protocol.HarnessHermes, sourceEventType, sourcePayload, hitchEventType, sourcePayload)
	harness.ApplySourceMetadata(&env, sourcePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	out := map[string]interface{}{}
	switch sourceEventType {
	case "pre_tool_call":
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorDeny || d.Behavior == protocol.BehaviorStop {
			out["action"] = "block"
			out["message"] = d.Reason
		}
	case "pre_llm_call":
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			out["context"] = d.Context
		}
	case "transform_tool_result", "transform_terminal_output", "transform_llm_output":
		if (d.Behavior == protocol.BehaviorReplaceResult || d.Behavior == protocol.BehaviorTransform) && len(d.UpdatedOutput) != 0 {
			out["result"] = rawValue(d.UpdatedOutput)
		}
	case "pre_gateway_dispatch":
		if d.Behavior == protocol.BehaviorHandled {
			out["action"] = "skip"
		} else if d.Behavior == protocol.BehaviorTransform {
			out["action"] = "rewrite"
			out["message"] = rawValue(d.UpdatedInput)
		} else if d.Behavior == protocol.BehaviorAllow {
			out["action"] = "allow"
		}
	}
	return protocol.Raw(out), nil
}

func rawValue(raw protocol.RawJSON) interface{} {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
