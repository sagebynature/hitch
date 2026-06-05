package pi

import (
	"encoding/json"

	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

type nativePayloadEnvelope struct {
	Event  protocol.RawJSON `json:"event"`
	Meta   nativeMetadata   `json:"metadata"`
	Legacy protocol.RawJSON `json:"payload"`
}

type nativeMetadata struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	CWD            string `json:"cwd"`
	Model          string `json:"model"`
	TranscriptPath string `json:"transcript_path"`
}

type AdapterResponse struct {
	AdapterAction string      `json:"adapter_action"`
	ReturnValue   interface{} `json:"return_value,omitempty"`
	Mutations     []Mutation  `json:"mutations,omitempty"`
}

type Mutation struct {
	Path  []string    `json:"path"`
	Value interface{} `json:"value"`
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	return NormalizeForHarness(protocol.HarnessPi, sourceEventType, sourcePayload, hitchEventType)
}

func NormalizeForHarness(h protocol.Harness, sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	eventPayload, meta := unwrapSourcePayload(sourcePayload)
	env := harness.NewEnvelope(h, sourceEventType, eventPayload, hitchEventType, eventPayload)
	env.SessionID = meta.SessionID
	env.TurnID = meta.TurnID
	env.CWD = meta.CWD
	env.Model = meta.Model
	env.TranscriptPath = meta.TranscriptPath
	if env.SessionID == "" && env.TurnID == "" && env.CWD == "" && env.Model == "" && env.TranscriptPath == "" {
		harness.ApplySourceMetadata(&env, eventPayload)
	}
	return env, protocol.ValidateEnvelope(env)
}

func unwrapSourcePayload(sourcePayload protocol.RawJSON) (protocol.RawJSON, nativeMetadata) {
	var wrapped nativePayloadEnvelope
	if err := json.Unmarshal(sourcePayload, &wrapped); err != nil {
		return sourcePayload, nativeMetadata{}
	}
	if len(wrapped.Event) != 0 && json.Valid(wrapped.Event) {
		return wrapped.Event, wrapped.Meta
	}
	if len(wrapped.Legacy) != 0 && json.Valid(wrapped.Legacy) {
		return wrapped.Legacy, wrapped.Meta
	}
	return sourcePayload, nativeMetadata{}
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	return TranslateForHarness(sourceEventType, aggregate)
}

func TranslateForHarness(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	resp := AdapterResponse{AdapterAction: "noop"}
	switch sourceEventType {
	case "input":
		switch d.Behavior {
		case protocol.BehaviorTransform:
			var input interface{}
			_ = json.Unmarshal(d.UpdatedInput, &input)
			resp.AdapterAction = "return"
			resp.ReturnValue = map[string]interface{}{"action": "transform", "text": input}
		case protocol.BehaviorHandled:
			resp.AdapterAction = "return"
			resp.ReturnValue = map[string]interface{}{"action": "handled"}
		case protocol.BehaviorContinue, protocol.BehaviorAllow:
			resp.AdapterAction = "return"
			resp.ReturnValue = map[string]interface{}{"action": "continue"}
		}
	case "tool_call":
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorDeny || d.Behavior == protocol.BehaviorStop {
			resp.AdapterAction = "return"
			resp.ReturnValue = map[string]interface{}{"block": true, "reason": d.Reason}
		} else if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			var v interface{}
			_ = json.Unmarshal(d.UpdatedInput, &v)
			resp.AdapterAction = "mutate_and_return"
			resp.Mutations = []Mutation{{Path: []string{"input"}, Value: v}}
		}
	case "tool_result":
		if (d.Behavior == protocol.BehaviorReplaceResult || d.Behavior == protocol.BehaviorTransform) && len(d.UpdatedOutput) != 0 {
			var v interface{}
			_ = json.Unmarshal(d.UpdatedOutput, &v)
			resp.AdapterAction = "return"
			resp.ReturnValue = v
		}
	case "context", "before_provider_request":
		if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			var v interface{}
			_ = json.Unmarshal(d.UpdatedInput, &v)
			resp.AdapterAction = "return"
			resp.ReturnValue = v
		}
	case "session_before_switch", "session_before_fork", "session_before_branch", "session_before_compact", "session_before_tree":
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorStop {
			resp.AdapterAction = "return"
			resp.ReturnValue = map[string]interface{}{"cancel": true}
		}
	}
	return protocol.Raw(resp), nil
}
