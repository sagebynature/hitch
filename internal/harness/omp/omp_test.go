package omp

import (
	"testing"

	"github.com/sage-scm/hitch/internal/protocol"
)

func TestMapOMPToolCall(t *testing.T) {
	env, err := (Mapper{}).Map("tool_call", protocol.Raw(map[string]interface{}{"toolName": "bash"}))
	if err != nil {
		t.Fatal(err)
	}
	if env.Harness != protocol.HarnessOMP || env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("bad env: %#v", env)
	}
}
