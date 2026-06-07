package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/sagebynature/hitch/internal/protocol"
)

type SourceEventCapability string

const (
	CapabilityObserverOnly   SourceEventCapability = "observer_only"
	CapabilityControlCapable SourceEventCapability = "control_capable"
)

type Normalizer interface {
	Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error)
	Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error)
	Capability(sourceEventType string) SourceEventCapability
	KnownSourceEvents() map[string]struct{}
}

func SourceEventSet(events ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(events))
	for _, event := range events {
		out[event] = struct{}{}
	}
	return out
}

func CapabilityFromSet(sourceEventType string, controlCapable map[string]struct{}) SourceEventCapability {
	if _, ok := controlCapable[sourceEventType]; ok {
		return CapabilityControlCapable
	}
	return CapabilityObserverOnly
}

func NewEnvelope(h protocol.Harness, sourceEventType string, sourcePayload protocol.RawJSON, eventType protocol.EventType, payload protocol.RawJSON) protocol.EventEnvelope {
	return protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: NewID("evt"), ReceivedAt: time.Now().UTC(), Harness: h, SourceEventType: sourceEventType, SourcePayload: sourcePayload, HitchEventType: eventType, Payload: payload}
}

type SourceMetadata struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	CWD            string `json:"cwd"`
	Model          string `json:"model"`
	TranscriptPath string `json:"transcript_path"`
}

func ApplySourceMetadata(env *protocol.EventEnvelope, sourcePayload protocol.RawJSON) {
	meta := ExtractSourceMetadata(sourcePayload)
	env.SessionID = meta.SessionID
	env.TurnID = meta.TurnID
	env.CWD = meta.CWD
	env.Model = meta.Model
	env.TranscriptPath = meta.TranscriptPath
}

func ExtractSourceMetadata(sourcePayload protocol.RawJSON) SourceMetadata {
	var wrapped struct {
		SourceMetadata
		Extra struct {
			SourceMetadata
			TaskID string `json:"task_id"`
		} `json:"extra"`
	}
	_ = json.Unmarshal(sourcePayload, &wrapped)
	meta := wrapped.SourceMetadata
	if meta.TurnID == "" {
		meta.TurnID = wrapped.Extra.TurnID
	}
	if meta.Model == "" {
		meta.Model = wrapped.Extra.Model
	}
	if meta.SessionID == "" && wrapped.Extra.TaskID != "default" {
		meta.SessionID = wrapped.Extra.TaskID
	}
	return meta
}

func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
