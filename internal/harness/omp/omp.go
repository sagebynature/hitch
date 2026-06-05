package omp

import (
	"github.com/sagebynature/hitch/internal/harness"
	piharness "github.com/sagebynature/hitch/internal/harness/pi"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := harness.NewEnvelope(protocol.HarnessOMP, sourceEventType, sourcePayload, hitchEventType, sourcePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	return piharness.TranslateForHarness(sourceEventType, aggregate)
}
