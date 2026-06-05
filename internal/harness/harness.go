package harness

import "github.com/sagebynature/hitch/internal/protocol"

type Normalizer interface {
	Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error)
	Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error)
}
