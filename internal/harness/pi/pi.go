package pi

import (
	"encoding/json"
	"fmt"

	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

type AdapterResponse struct {
	AdapterAction string      `json:"adapter_action"`
	ReturnValue   interface{} `json:"return_value,omitempty"`
	Mutations     []Mutation  `json:"mutations,omitempty"`
}

type Mutation struct {
	Path  []string    `json:"path"`
	Value interface{} `json:"value"`
}

var eventMap = map[string]protocol.EventType{
	"input":                   protocol.EventTurnUserPrompt,
	"before_agent_start":      protocol.EventTurnStarted,
	"agent_start":             protocol.EventTurnStarted,
	"turn_start":              protocol.EventTurnStarted,
	"context":                 protocol.EventTurnStarted,
	"before_provider_request": protocol.EventTurnStarted,
	"tool_call":               protocol.EventToolRequested,
	"tool_result":             protocol.EventToolCompleted,
	"turn_end":                protocol.EventTurnCompleted,
	"agent_end":               protocol.EventTurnCompleted,
	"session_start":           protocol.EventSessionStarted,
	"session_shutdown":        protocol.EventSessionEnded,
	"session_before_switch":   protocol.EventSessionResumed,
	"session_before_fork":     protocol.EventSessionResumed,
	"session_before_compact":  protocol.EventSessionCompacted,
	"session_compact":         protocol.EventSessionCompacted,
	"user_bash":               protocol.EventToolRequested,
}

func (Mapper) Map(nativeEventType string, nativePayload protocol.RawJSON) (protocol.EventEnvelope, error) {
	eventType, ok := eventMap[nativeEventType]
	if !ok {
		return protocol.EventEnvelope{}, fmt.Errorf("unsupported pi event %q", nativeEventType)
	}
	env := harness.NewEnvelope(protocol.HarnessPi, nativeEventType, nativePayload, eventType, nativePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(nativeEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	return TranslateForHarness(nativeEventType, aggregate)
}

func TranslateForHarness(nativeEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	resp := AdapterResponse{AdapterAction: "noop"}
	switch nativeEventType {
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
	case "session_before_switch", "session_before_fork", "session_before_compact":
		if d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorStop {
			resp.AdapterAction = "return"
			resp.ReturnValue = map[string]interface{}{"cancel": true}
		}
	}
	return protocol.Raw(resp), nil
}
