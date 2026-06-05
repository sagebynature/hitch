package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const Version = "0.1.0"

type Harness string

const (
	HarnessCodex  Harness = "codex"
	HarnessHermes Harness = "hermes"
	HarnessPi     Harness = "pi"
	HarnessOMP    Harness = "omp"
)

var validHarnesses = map[Harness]struct{}{
	HarnessCodex: {}, HarnessHermes: {}, HarnessPi: {}, HarnessOMP: {},
}

type EventType string

const (
	EventSessionStarted        EventType = "session.started"
	EventSessionResumed        EventType = "session.resumed"
	EventSessionEnded          EventType = "session.ended"
	EventSessionCompacted      EventType = "session.compacted"
	EventTurnStarted           EventType = "turn.started"
	EventTurnUserPrompt        EventType = "turn.user_prompt"
	EventTurnAssistantStarted  EventType = "turn.assistant_started"
	EventTurnCompleted         EventType = "turn.completed"
	EventToolRequested         EventType = "tool.requested"
	EventToolPermissionRequest EventType = "tool.permission_requested"
	EventToolCompleted         EventType = "tool.completed"
	EventSubagentStarted       EventType = "subagent.started"
	EventSubagentCompleted     EventType = "subagent.completed"
	EventErrorReported         EventType = "error.reported"
)

var validEventTypes = map[EventType]struct{}{
	EventSessionStarted: {}, EventSessionResumed: {}, EventSessionEnded: {}, EventSessionCompacted: {},
	EventTurnStarted: {}, EventTurnUserPrompt: {}, EventTurnAssistantStarted: {}, EventTurnCompleted: {},
	EventToolRequested: {}, EventToolPermissionRequest: {}, EventToolCompleted: {},
	EventSubagentStarted: {}, EventSubagentCompleted: {}, EventErrorReported: {},
}

type DecisionBehavior string

const (
	BehaviorNone          DecisionBehavior = "none"
	BehaviorAllow         DecisionBehavior = "allow"
	BehaviorDeny          DecisionBehavior = "deny"
	BehaviorBlock         DecisionBehavior = "block"
	BehaviorContinue      DecisionBehavior = "continue"
	BehaviorStop          DecisionBehavior = "stop"
	BehaviorTransform     DecisionBehavior = "transform"
	BehaviorReplaceResult DecisionBehavior = "replace_result"
	BehaviorInjectContext DecisionBehavior = "inject_context"
	BehaviorHandled       DecisionBehavior = "handled"
)

var validBehaviors = map[DecisionBehavior]struct{}{
	BehaviorNone: {}, BehaviorAllow: {}, BehaviorDeny: {}, BehaviorBlock: {}, BehaviorContinue: {}, BehaviorStop: {},
	BehaviorTransform: {}, BehaviorReplaceResult: {}, BehaviorInjectContext: {}, BehaviorHandled: {},
}

type HandlerStatus string

const (
	StatusOK      HandlerStatus = "ok"
	StatusError   HandlerStatus = "error"
	StatusTimeout HandlerStatus = "timeout"
)

var validStatuses = map[HandlerStatus]struct{}{
	StatusOK: {}, StatusError: {}, StatusTimeout: {},
}

type RawJSON = json.RawMessage

type EventEnvelope struct {
	HitchVersion    string    `json:"hitch_version"`
	EventID         string    `json:"event_id"`
	ReceivedAt      time.Time `json:"received_at"`
	Harness         Harness   `json:"harness"`
	HarnessVersion  string    `json:"harness_version,omitempty"`
	SourceEventType string    `json:"source_event_type"`
	SourcePayload   RawJSON   `json:"source_payload"`
	HitchEventType  EventType `json:"hitch_event_type"`
	SessionID       string    `json:"session_id,omitempty"`
	TurnID          string    `json:"turn_id,omitempty"`
	CWD             string    `json:"cwd,omitempty"`
	Model           string    `json:"model,omitempty"`
	TranscriptPath  string    `json:"transcript_path,omitempty"`
	Payload         RawJSON   `json:"payload"`
}

type Decision struct {
	Behavior       DecisionBehavior `json:"behavior"`
	Reason         string           `json:"reason,omitempty"`
	Context        string           `json:"context,omitempty"`
	UpdatedInput   RawJSON          `json:"updated_input,omitempty"`
	UpdatedOutput  RawJSON          `json:"updated_output,omitempty"`
	NativeResponse RawJSON          `json:"native_response,omitempty"`
}

type LogRecord struct {
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

type HandlerResult struct {
	Status   HandlerStatus      `json:"status"`
	Decision *Decision          `json:"decision,omitempty"`
	Logs     []LogRecord        `json:"logs,omitempty"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
}

type AggregateDecision struct {
	Decision       Decision        `json:"decision"`
	HandlerResults []HandlerResult `json:"handler_results,omitempty"`
	Errors         []string        `json:"errors,omitempty"`
}

func IsValidHarness(h Harness) bool           { _, ok := validHarnesses[h]; return ok }
func IsValidEventType(t EventType) bool       { _, ok := validEventTypes[t]; return ok }
func IsValidBehavior(b DecisionBehavior) bool { _, ok := validBehaviors[b]; return ok }
func IsValidStatus(s HandlerStatus) bool      { _, ok := validStatuses[s]; return ok }

func ValidateEnvelope(e EventEnvelope) error {
	if e.HitchVersion == "" {
		return errors.New("hitch_version is required")
	}
	if e.EventID == "" {
		return errors.New("event_id is required")
	}
	if e.ReceivedAt.IsZero() {
		return errors.New("received_at is required")
	}
	if !IsValidHarness(e.Harness) {
		return fmt.Errorf("unknown harness %q", e.Harness)
	}
	if e.SourceEventType == "" {
		return errors.New("source_event_type is required")
	}
	if len(e.SourcePayload) == 0 || !json.Valid(e.SourcePayload) {
		return errors.New("source_payload must be valid JSON")
	}
	if !IsValidEventType(e.HitchEventType) {
		return fmt.Errorf("unknown hitch_event_type %q", e.HitchEventType)
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.New("payload must be valid JSON")
	}
	return nil
}

func NormalizeHandlerResult(r *HandlerResult) error {
	if r.Status == "" {
		r.Status = StatusOK
	}
	if !IsValidStatus(r.Status) {
		return fmt.Errorf("unknown handler status %q", r.Status)
	}
	if r.Decision == nil {
		r.Decision = &Decision{Behavior: BehaviorNone}
		return nil
	}
	if r.Decision.Behavior == "" {
		r.Decision.Behavior = BehaviorNone
	}
	if !IsValidBehavior(r.Decision.Behavior) {
		return fmt.Errorf("unknown decision behavior %q", r.Decision.Behavior)
	}
	return nil
}

func Raw(v interface{}) RawJSON {
	b, err := json.Marshal(v)
	if err != nil {
		return RawJSON(`null`)
	}
	return b
}
