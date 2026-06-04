package hermes

import (
	"fmt"

	"github.com/sage-scm/hitch/internal/harness"
	"github.com/sage-scm/hitch/internal/protocol"
)

type Mapper struct{}

var eventMap = map[string]protocol.EventType{
	"pre_tool_call":             protocol.EventToolRequested,
	"post_tool_call":            protocol.EventToolCompleted,
	"pre_llm_call":              protocol.EventTurnStarted,
	"post_llm_call":             protocol.EventTurnCompleted,
	"on_session_start":          protocol.EventSessionStarted,
	"on_session_end":            protocol.EventSessionEnded,
	"subagent_stop":             protocol.EventSubagentCompleted,
	"transform_tool_result":     protocol.EventToolCompleted,
	"transform_terminal_output": protocol.EventToolCompleted,
	"transform_llm_output":      protocol.EventTurnCompleted,
	"pre_gateway_dispatch":      protocol.EventTurnUserPrompt,
}

func (Mapper) Map(nativeEventType string, nativePayload protocol.RawJSON) (protocol.EventEnvelope, error) {
	eventType, ok := eventMap[nativeEventType]
	if !ok {
		return protocol.EventEnvelope{}, fmt.Errorf("unsupported hermes event %q", nativeEventType)
	}
	env := harness.NewEnvelope(protocol.HarnessHermes, nativeEventType, nativePayload, eventType, nativePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(nativeEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	out := map[string]interface{}{}
	switch nativeEventType {
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
			out["result"] = string(d.UpdatedOutput)
		}
	case "pre_gateway_dispatch":
		if d.Behavior == protocol.BehaviorHandled {
			out["action"] = "skip"
		} else if d.Behavior == protocol.BehaviorTransform {
			out["action"] = "rewrite"
			out["message"] = string(d.UpdatedInput)
		} else if d.Behavior == protocol.BehaviorAllow {
			out["action"] = "allow"
		}
	}
	return protocol.Raw(out), nil
}
