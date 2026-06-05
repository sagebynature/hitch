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

func TestNormalizeCopiesSourceMetadata(t *testing.T) {
	env, err := (Mapper{}).Normalize("pre_llm_call", protocol.Raw(map[string]interface{}{
		"session_id": "session_1",
		"cwd":        "/tmp/hitch",
	}), protocol.EventTurnStarted)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session_1" || env.CWD != "/tmp/hitch" {
		t.Fatalf("metadata not copied: %#v", env)
	}
}
