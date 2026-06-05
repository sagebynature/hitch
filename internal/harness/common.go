package harness

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/sagebynature/hitch/internal/protocol"
)

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
			TaskID string `json:"task_id"`
		} `json:"extra"`
	}
	_ = json.Unmarshal(sourcePayload, &wrapped)
	meta := wrapped.SourceMetadata
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
