package codex

import (
	"encoding/json"

	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := harness.NewEnvelope(protocol.HarnessCodex, sourceEventType, sourcePayload, hitchEventType, sourcePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	out := map[string]interface{}{}
	switch sourceEventType {
	case "PermissionRequest":
		if d.Behavior == protocol.BehaviorDeny || d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorStop {
			out["hookSpecificOutput"] = map[string]interface{}{"decision": map[string]interface{}{"behavior": "deny", "message": d.Reason}}
		} else if d.Behavior == protocol.BehaviorAllow {
			out["hookSpecificOutput"] = map[string]interface{}{"decision": map[string]interface{}{"behavior": "allow"}}
		}
	case "PreToolUse":
		if d.Behavior == protocol.BehaviorDeny || d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorStop {
			out["permissionDecision"] = "deny"
			out["permissionDecisionReason"] = d.Reason
		} else if d.Behavior == protocol.BehaviorAllow || d.Behavior == protocol.BehaviorTransform {
			out["permissionDecision"] = "allow"
			if len(d.UpdatedInput) != 0 {
				var v interface{}
				_ = json.Unmarshal(d.UpdatedInput, &v)
				out["updatedInput"] = v
			}
		}
	case "UserPromptSubmit", "SessionStart", "SubagentStart", "PostToolUse":
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			out["additionalContext"] = d.Context
		}
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorDeny {
			out["decision"] = "block"
			out["reason"] = d.Reason
		}
	case "SubagentStop":
		if d.Behavior == protocol.BehaviorContinue {
			out["continue"] = true
			out["reason"] = d.Reason
		}
	case "Stop", "PreCompact", "PostCompact":
		if d.Behavior == protocol.BehaviorStop || d.Behavior == protocol.BehaviorBlock {
			out["decision"] = "stop"
			out["reason"] = d.Reason
		}
	}
	return protocol.Raw(out), nil
}
