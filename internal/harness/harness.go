package harness

import "github.com/sage-scm/hitch/internal/protocol"

type Mapper interface {
	Map(nativeEventType string, nativePayload protocol.RawJSON) (protocol.EventEnvelope, error)
	Translate(nativeEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error)
}
