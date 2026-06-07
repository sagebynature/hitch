package harness

import (
	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

type SourceMetadata = core.SourceMetadata

func ApplySourceMetadata(env *protocol.EventEnvelope, sourcePayload protocol.RawJSON) {
	core.ApplySourceMetadata(env, sourcePayload)
}

func ExtractSourceMetadata(sourcePayload protocol.RawJSON) SourceMetadata {
	return core.ExtractSourceMetadata(sourcePayload)
}

func NewID(prefix string) string {
	return core.NewID(prefix)
}
