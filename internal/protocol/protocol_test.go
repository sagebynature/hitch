package protocol

import (
	"testing"
	"time"
)

func validEnvelope() EventEnvelope {
	return EventEnvelope{
		HitchVersion:    Version,
		EventID:         "evt_1",
		ReceivedAt:      time.Now().UTC(),
		Harness:         HarnessCodex,
		SourceEventType: "PreToolUse",
		SourcePayload:   Raw(map[string]interface{}{"hook_event_name": "PreToolUse"}),
		HitchEventType:  EventToolRequested,
		Payload:         Raw(map[string]interface{}{"tool": "bash"}),
	}
}

func TestValidateEnvelope(t *testing.T) {
	e := validEnvelope()
	if err := ValidateEnvelope(e); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}

	e.Harness = Harness("unknown")
	if err := ValidateEnvelope(e); err == nil {
		t.Fatal("unknown harness accepted")
	}

	e = validEnvelope()
	e.HitchEventType = EventType("unknown")
	if err := ValidateEnvelope(e); err == nil {
		t.Fatal("unknown event type accepted")
	}

	e = validEnvelope()
	e.SourcePayload = RawJSON(`{`)
	if err := ValidateEnvelope(e); err == nil {
		t.Fatal("invalid source payload accepted")
	}
}

func TestNewGranularEventTypesAreValid(t *testing.T) {
	for _, eventType := range []EventType{
		EventTurnAssistantCompleted,
		EventLLMRequested,
		EventLLMCompleted,
		EventToolProgress,
		EventRetryStarted,
		EventRetryCompleted,
	} {
		if !IsValidEventType(eventType) {
			t.Fatalf("event type %q should be valid", eventType)
		}
	}
}

func TestOpenCodeHarnessIsValid(t *testing.T) {
	if !IsValidHarness(HarnessOpenCode) {
		t.Fatalf("opencode harness should be valid")
	}
}

func TestNormalizeHandlerResult(t *testing.T) {
	r := HandlerResult{Status: StatusOK}
	if err := NormalizeHandlerResult(&r); err != nil {
		t.Fatalf("normalizing valid result: %v", err)
	}
	if r.Decision == nil || r.Decision.Behavior != BehaviorNone {
		t.Fatalf("missing decision did not default to none: %#v", r.Decision)
	}

	r = HandlerResult{Status: StatusOK, Decision: &Decision{Behavior: DecisionBehavior("bogus")}}
	if err := NormalizeHandlerResult(&r); err == nil {
		t.Fatal("invalid behavior accepted")
	}

	r = HandlerResult{Status: HandlerStatus("bogus")}
	if err := NormalizeHandlerResult(&r); err == nil {
		t.Fatal("invalid status accepted")
	}
}
