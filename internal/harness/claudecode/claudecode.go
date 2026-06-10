package claudecode

import (
	"encoding/json"

	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

var controlCapableEvents = map[string]struct{}{
	"SessionStart":      {},
	"UserPromptSubmit":  {},
	"PreToolUse":        {},
	"PermissionRequest": {},
	"PostToolUse":       {},
	"Stop":              {},
	"SubagentStart":     {},
	"SubagentStop":      {},
	"PreCompact":        {},
	"PostCompact":       {},
}

var knownSourceEvents = core.SourceEventSet(
	// Session lifecycle
	"SessionStart",
	"SessionEnd",
	"Setup",
	// Turn lifecycle
	"UserPromptSubmit",
	"UserPromptExpansion",
	"Stop",
	"StopFailure",
	// Tool lifecycle
	"PreToolUse",
	"PostToolUse",
	"PostToolUseFailure",
	"PostToolBatch",
	"PermissionRequest",
	"PermissionDenied",
	// Subagent
	"SubagentStart",
	"SubagentStop",
	// Context
	"PreCompact",
	"PostCompact",
	"InstructionsLoaded",
	// Reactive
	"Notification",
	"MessageDisplay",
	"CwdChanged",
	"FileChanged",
	"ConfigChange",
	// Advanced
	"Elicitation",
	"ElicitationResult",
	"WorktreeCreate",
	"WorktreeRemove",
	"TaskCreated",
	"TaskCompleted",
	"TeammateIdle",
)

func (Mapper) KnownSourceEvents() map[string]struct{} {
	return knownSourceEvents
}

func (Mapper) Capability(sourceEventType string) core.SourceEventCapability {
	return core.CapabilityFromSet(sourceEventType, controlCapableEvents)
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := core.NewEnvelope(protocol.HarnessClaudeCode, sourceEventType, sourcePayload, hitchEventType, core.ParseTypedPayload(protocol.HarnessClaudeCode, sourceEventType, sourcePayload, hitchEventType))
	core.ApplySourceMetadata(&env, sourcePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	out := map[string]interface{}{}
	switch sourceEventType {
	case "PreToolUse":
		switch d.Behavior {
		case protocol.BehaviorDeny, protocol.BehaviorBlock, protocol.BehaviorStop:
			out["hookSpecificOutput"] = map[string]interface{}{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": d.Reason,
			}
		case protocol.BehaviorAllow, protocol.BehaviorTransform:
			hso := map[string]interface{}{
				"hookEventName":      "PreToolUse",
				"permissionDecision": "allow",
			}
			if len(d.UpdatedInput) != 0 {
				var v interface{}
				_ = json.Unmarshal(d.UpdatedInput, &v)
				hso["updatedInput"] = v
			}
			out["hookSpecificOutput"] = hso
		}
	case "PermissionRequest":
		switch d.Behavior {
		case protocol.BehaviorDeny, protocol.BehaviorBlock, protocol.BehaviorStop:
			out["hookSpecificOutput"] = map[string]interface{}{
				"hookEventName": "PermissionRequest",
				"decision":      map[string]interface{}{"behavior": "deny"},
			}
		case protocol.BehaviorAllow:
			out["hookSpecificOutput"] = map[string]interface{}{
				"hookEventName": "PermissionRequest",
				"decision":      map[string]interface{}{"behavior": "allow"},
			}
		}
	case "UserPromptSubmit", "SessionStart":
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			out["additionalContext"] = d.Context
		}
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorDeny {
			out["decision"] = "block"
			out["reason"] = d.Reason
		}
	case "PostToolUse":
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			out["additionalContext"] = d.Context
		}
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorDeny {
			out["decision"] = "block"
			out["reason"] = d.Reason
		}
		if d.Behavior == protocol.BehaviorReplaceResult && len(d.UpdatedOutput) != 0 {
			var v interface{}
			if json.Unmarshal(d.UpdatedOutput, &v) == nil {
				if s, ok := v.(string); ok {
					out["updatedToolOutput"] = s
				} else {
					out["updatedToolOutput"] = string(d.UpdatedOutput)
				}
			}
		}
	case "Stop":
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorStop {
			out["decision"] = "block"
			out["reason"] = d.Reason
		}
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			out["additionalContext"] = d.Context
		}
	case "SubagentStart", "SubagentStop", "PreCompact", "PostCompact":
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorDeny || d.Behavior == protocol.BehaviorStop {
			out["decision"] = "block"
			out["reason"] = d.Reason
		}
	}
	return protocol.Raw(out), nil
}
