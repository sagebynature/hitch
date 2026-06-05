package omp

import (
	"github.com/sagebynature/hitch/internal/harness"
	piharness "github.com/sagebynature/hitch/internal/harness/pi"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

var defaultEventMap = map[string]protocol.EventType{
	"input":                 protocol.EventTurnUserPrompt,
	"before_agent_start":    protocol.EventTurnStarted,
	"turn_start":            protocol.EventTurnStarted,
	"tool_call":             protocol.EventToolRequested,
	"tool_result":           protocol.EventToolCompleted,
	"turn_end":              protocol.EventTurnCompleted,
	"auto_compaction_start": protocol.EventSessionCompacted,
	"todo_reminder":         protocol.EventTurnStarted,
}

func DefaultEventMap() map[string]protocol.EventType {
	out := make(map[string]protocol.EventType, len(defaultEventMap))
	for k, v := range defaultEventMap {
		out[k] = v
	}
	return out
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := harness.NewEnvelope(protocol.HarnessOMP, sourceEventType, sourcePayload, hitchEventType, sourcePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	return piharness.TranslateForHarness(sourceEventType, aggregate)
}
