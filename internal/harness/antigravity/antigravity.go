package antigravity

import (
	"encoding/json"

	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

var controlCapableEvents = map[string]struct{}{
	"PreToolUse":     {},
	"PostToolUse":    {},
	"PreInvocation":  {},
	"PostInvocation": {},
	"Stop":           {},
}

var knownSourceEvents = core.SourceEventSet(
	"PreToolUse",
	"PostToolUse",
	"PreInvocation",
	"PostInvocation",
	"Stop",
)

func (Mapper) KnownSourceEvents() map[string]struct{} {
	return knownSourceEvents
}

func (Mapper) Capability(sourceEventType string) core.SourceEventCapability {
	return core.CapabilityFromSet(sourceEventType, controlCapableEvents)
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := core.NewEnvelope(protocol.HarnessAntigravity, sourceEventType, sourcePayload, hitchEventType, core.ParseTypedPayload(protocol.HarnessAntigravity, sourceEventType, sourcePayload, hitchEventType))
	core.ApplySourceMetadata(&env, sourcePayload)

	// Extract common fields if present
	var src map[string]interface{}
	if err := json.Unmarshal(sourcePayload, &src); err == nil {
		if sessionID, ok := src["conversationId"].(string); ok && sessionID != "" {
			env.SessionID = sessionID
		}
		if transcriptPath, ok := src["transcriptPath"].(string); ok && transcriptPath != "" {
			env.TranscriptPath = transcriptPath
		}
	}

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
			out["decision"] = "deny"
			out["reason"] = d.Reason
		case protocol.BehaviorAllow, protocol.BehaviorTransform:
			out["decision"] = "allow"
			if d.Reason != "" {
				out["reason"] = d.Reason
			}
		}
	case "PostToolUse":
		// empty output {}
	case "PreInvocation", "PostInvocation":
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			out["injectSteps"] = []interface{}{
				map[string]interface{}{
					"ephemeralMessage": d.Context,
				},
			}
		}
		if sourceEventType == "PostInvocation" {
			if d.Behavior == protocol.BehaviorStop {
				out["terminationBehavior"] = "terminate"
			} else if d.Behavior == protocol.BehaviorContinue {
				out["terminationBehavior"] = "force_continue"
			}
		}
	case "Stop":
		if d.Behavior == protocol.BehaviorContinue {
			out["decision"] = "continue"
			out["reason"] = d.Reason
		} else {
			out["decision"] = ""
		}
	}
	return protocol.Raw(out), nil
}
