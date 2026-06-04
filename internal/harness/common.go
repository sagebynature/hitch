package harness

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/sage-scm/hitch/internal/protocol"
)

func NewEnvelope(h protocol.Harness, nativeEventType string, nativePayload protocol.RawJSON, eventType protocol.EventType, payload protocol.RawJSON) protocol.EventEnvelope {
	return protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: NewID("evt"), ReceivedAt: time.Now().UTC(), Harness: h, NativeEventType: nativeEventType, NativePayload: nativePayload, HitchEventType: eventType, Payload: payload}
}

func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
