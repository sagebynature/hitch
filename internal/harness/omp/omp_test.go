package omp

import (
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizeOMPToolCall(t *testing.T) {
	env, err := (Mapper{}).Normalize("tool_call", protocol.Raw(map[string]interface{}{"toolName": "bash"}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.Harness != protocol.HarnessOMP || env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("bad env: %#v", env)
	}
}
