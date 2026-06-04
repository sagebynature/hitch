package omp

import (
	"fmt"

	"github.com/sage-scm/hitch/internal/harness"
	piharness "github.com/sage-scm/hitch/internal/harness/pi"
	"github.com/sage-scm/hitch/internal/protocol"
)

type Mapper struct{}

var eventMap = map[string]protocol.EventType{
	"input":                 protocol.EventTurnUserPrompt,
	"before_agent_start":    protocol.EventTurnStarted,
	"turn_start":            protocol.EventTurnStarted,
	"tool_call":             protocol.EventToolRequested,
	"tool_result":           protocol.EventToolCompleted,
	"turn_end":              protocol.EventTurnCompleted,
	"auto_compaction_start": protocol.EventSessionCompacted,
	"todo_reminder":         protocol.EventTurnStarted,
}

func (Mapper) Map(nativeEventType string, nativePayload protocol.RawJSON) (protocol.EventEnvelope, error) {
	eventType, ok := eventMap[nativeEventType]
	if !ok {
		return protocol.EventEnvelope{}, fmt.Errorf("unsupported omp event %q", nativeEventType)
	}
	env := harness.NewEnvelope(protocol.HarnessOMP, nativeEventType, nativePayload, eventType, nativePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(nativeEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	return piharness.TranslateForHarness(nativeEventType, aggregate)
}
