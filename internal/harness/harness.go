package harness

import "github.com/sagebynature/hitch/internal/protocol"

type SourceEventCapability string

const (
	CapabilityObserverOnly   SourceEventCapability = "observer_only"
	CapabilityControlCapable SourceEventCapability = "control_capable"
)

type Normalizer interface {
	Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error)
	Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error)
	Capability(sourceEventType string) SourceEventCapability
}

func CapabilityFromSet(sourceEventType string, controlCapable map[string]struct{}) SourceEventCapability {
	if _, ok := controlCapable[sourceEventType]; ok {
		return CapabilityControlCapable
	}
	return CapabilityObserverOnly
}
