package harness

import (
	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

type SourceEventCapability = core.SourceEventCapability

const (
	CapabilityObserverOnly   = core.CapabilityObserverOnly
	CapabilityControlCapable = core.CapabilityControlCapable
)

type Normalizer = core.Normalizer

func SourceEventSet(events ...string) map[string]struct{} {
	return core.SourceEventSet(events...)
}

func CapabilityFromSet(sourceEventType string, controlCapable map[string]struct{}) SourceEventCapability {
	return core.CapabilityFromSet(sourceEventType, controlCapable)
}

func NewEnvelope(h protocol.Harness, sourceEventType string, sourcePayload protocol.RawJSON, eventType protocol.EventType, payload protocol.RawJSON) protocol.EventEnvelope {
	return core.NewEnvelope(h, sourceEventType, sourcePayload, eventType, payload)
}
