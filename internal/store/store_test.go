package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_1", ReceivedAt: now, Harness: protocol.HarnessCodex, NativeEventType: "PreToolUse", NativePayload: protocol.Raw(map[string]interface{}{"x": 1}), HitchEventType: protocol.EventToolRequested, Payload: protocol.Raw(map[string]interface{}{"tool": "bash"})}
	if err := s.InsertInbound(ctx, InboundEvent{ID: "in_1", ReceivedAt: now, Harness: env.Harness, NativeEventType: env.NativeEventType, NativePayload: env.NativePayload}); err != nil {
		t.Fatalf("insert inbound: %v", err)
	}
	if err := s.InsertNormalized(ctx, NormalizedEvent{ID: "norm_1", InboundEventID: "in_1", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatalf("insert normalized: %v", err)
	}
	if err := s.InsertHandlerInvocation(ctx, HandlerInvocation{ID: "handler_1", NormalizedEventID: "norm_1", HandlerName: "h", Mode: "sync", StartedAt: now, CompletedAt: now, Status: protocol.StatusOK, Output: protocol.Raw(map[string]interface{}{"status": "ok"}), Decision: protocol.Raw(map[string]interface{}{"behavior": "none"})}); err != nil {
		t.Fatalf("insert handler: %v", err)
	}
	if err := s.InsertNativeResponse(ctx, NativeResponse{ID: "resp_1", NormalizedEventID: "norm_1", Harness: env.Harness, NativeEventType: env.NativeEventType, Response: protocol.Raw(map[string]interface{}{"ok": true}), EmittedAt: now}); err != nil {
		t.Fatalf("insert response: %v", err)
	}

	got, err := s.GetEvent(ctx, "norm_1")
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if got.EventID != env.EventID || got.HitchEventType != env.HitchEventType {
		t.Fatalf("wrong event: %#v", got)
	}

	inspection, err := s.InspectEvent(ctx, "norm_1")
	if err != nil {
		t.Fatalf("inspect event: %v", err)
	}
	if inspection.Inbound.ID != "in_1" || inspection.Normalized.ID != "norm_1" {
		t.Fatalf("wrong inspection event ids: %#v", inspection)
	}
	if len(inspection.HandlerInvocations) != 1 || inspection.HandlerInvocations[0].ID != "handler_1" {
		t.Fatalf("wrong handler invocations: %#v", inspection.HandlerInvocations)
	}
	if len(inspection.NativeResponses) != 1 || inspection.NativeResponses[0].ID != "resp_1" {
		t.Fatalf("wrong native responses: %#v", inspection.NativeResponses)
	}
}
