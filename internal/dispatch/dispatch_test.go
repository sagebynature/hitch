package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sage-scm/hitch/internal/config"
	"github.com/sage-scm/hitch/internal/protocol"
)

func testEnv() protocol.EventEnvelope {
	return protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt", ReceivedAt: time.Now().UTC(), Harness: protocol.HarnessCodex, NativeEventType: "PreToolUse", NativePayload: protocol.Raw(map[string]interface{}{}), HitchEventType: protocol.EventToolRequested, Payload: protocol.Raw(map[string]interface{}{})}
}

func script(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported in this test")
	}
	p := filepath.Join(t.TempDir(), "handler.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDispatchParsesHandlerResult(t *testing.T) {
	h := script(t, `echo '{"status":"ok","decision":{"behavior":"deny","reason":"no"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{"a": {Command: []string{h}, Events: []string{"*"}, Mode: "sync", TimeoutMS: 1000}})
	got := r.Dispatch(context.Background(), testEnv(), "sync", time.Second)
	if got.Aggregate.Decision.Behavior != protocol.BehaviorDeny {
		t.Fatalf("got %s", got.Aggregate.Decision.Behavior)
	}
	if len(got.Invocations) != 1 || got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("bad invocation: %#v", got.Invocations)
	}
}

func TestDispatchInvalidJSONIsError(t *testing.T) {
	h := script(t, `echo nope`)
	r := NewRunner(map[string]config.HandlerConfig{"a": {Command: []string{h}, Events: []string{"*"}, Mode: "sync", TimeoutMS: 1000}})
	got := r.Dispatch(context.Background(), testEnv(), "sync", time.Second)
	if got.Invocations[0].Status != protocol.StatusError {
		t.Fatalf("expected error: %#v", got.Invocations[0])
	}
}

func TestDispatchTimeout(t *testing.T) {
	h := script(t, `sleep 1`)
	r := NewRunner(map[string]config.HandlerConfig{"a": {Command: []string{h}, Events: []string{"*"}, Mode: "sync", TimeoutMS: 10}})
	got := r.Dispatch(context.Background(), testEnv(), "sync", time.Second)
	if got.Invocations[0].Status != protocol.StatusTimeout {
		t.Fatalf("expected timeout: %#v", got.Invocations[0])
	}
}

func TestAggregationDeterministic(t *testing.T) {
	slowAllow := script(t, `sleep 0.05; echo '{"status":"ok","decision":{"behavior":"allow"}}'`)
	fastDeny := script(t, `echo '{"status":"ok","decision":{"behavior":"deny"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{
		"a_allow": {Command: []string{slowAllow}, Events: []string{"*"}, Mode: "sync", TimeoutMS: 1000},
		"b_deny":  {Command: []string{fastDeny}, Events: []string{"*"}, Mode: "sync", TimeoutMS: 1000},
	})
	got := r.Dispatch(context.Background(), testEnv(), "sync", time.Second)
	if got.Aggregate.Decision.Behavior != protocol.BehaviorDeny {
		t.Fatalf("deny should win: %#v", got.Aggregate.Decision)
	}
}

func TestContextConcatenation(t *testing.T) {
	h1 := script(t, `echo '{"status":"ok","decision":{"behavior":"inject_context","context":"one"}}'`)
	h2 := script(t, `echo '{"status":"ok","decision":{"behavior":"inject_context","context":"two"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{
		"a": {Command: []string{h1}, Events: []string{"*"}, Mode: "sync", TimeoutMS: 1000},
		"b": {Command: []string{h2}, Events: []string{"*"}, Mode: "sync", TimeoutMS: 1000},
	})
	got := r.Dispatch(context.Background(), testEnv(), "sync", time.Second)
	if got.Aggregate.Decision.Context != "one\n\ntwo" {
		t.Fatalf("bad context: %q", got.Aggregate.Decision.Context)
	}
}
