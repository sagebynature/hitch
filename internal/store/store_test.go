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
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_1", ReceivedAt: now, Harness: protocol.HarnessCodex, SourceEventType: "PreToolUse", SourcePayload: protocol.Raw(map[string]interface{}{"x": 1}), HitchEventType: protocol.EventToolRequested, SessionID: "session_1", CWD: "/tmp/hitch", Payload: protocol.Raw(map[string]interface{}{"tool": "bash"})}
	if err := s.InsertInbound(ctx, InboundEvent{ID: "in_1", ReceivedAt: now, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload, HitchClientVersion: "test-client"}); err != nil {
		t.Fatalf("insert inbound: %v", err)
	}
	if err := s.InsertNormalized(ctx, NormalizedEvent{ID: "norm_1", InboundEventID: "in_1", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatalf("insert normalized: %v", err)
	}
	if err := s.InsertHandlerInvocation(ctx, HandlerInvocation{ID: "handler_1", NormalizedEventID: "norm_1", HandlerName: "h", Kind: "control", StartedAt: now, CompletedAt: now, Status: protocol.StatusOK, Output: protocol.Raw(map[string]interface{}{"status": "ok"}), Decision: protocol.Raw(map[string]interface{}{"behavior": "none"})}); err != nil {
		t.Fatalf("insert handler: %v", err)
	}
	if err := s.InsertNativeResponse(ctx, NativeResponse{ID: "resp_1", NormalizedEventID: "norm_1", Response: protocol.Raw(map[string]interface{}{"ok": true}), EmittedAt: now}); err != nil {
		t.Fatalf("insert response: %v", err)
	}

	got, err := s.GetEvent(ctx, "norm_1")
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if got.EventID != env.EventID || got.HitchEventType != env.HitchEventType || got.SessionID != "session_1" || got.CWD != "/tmp/hitch" {
		t.Fatalf("wrong event: %#v", got)
	}

	inspection, err := s.InspectEvent(ctx, "norm_1")
	if err != nil {
		t.Fatalf("inspect event: %v", err)
	}
	if inspection.Inbound.ID != "in_1" || inspection.Normalized.ID != "norm_1" {
		t.Fatalf("wrong inspection event ids: %#v", inspection)
	}
	if inspection.Inbound.SourceEventType != "PreToolUse" || string(inspection.Inbound.SourcePayload) == "" || inspection.Inbound.HitchClientVersion != "test-client" {
		t.Fatalf("wrong source inspection fields: %#v", inspection.Inbound)
	}
	if len(inspection.HandlerInvocations) != 1 || inspection.HandlerInvocations[0].ID != "handler_1" {
		t.Fatalf("wrong handler invocations: %#v", inspection.HandlerInvocations)
	}
	if len(inspection.NativeResponses) != 1 || inspection.NativeResponses[0].ID != "resp_1" {
		t.Fatalf("wrong native responses: %#v", inspection.NativeResponses)
	}
}

func TestStoreSchemaMirrorsCurrentEventModels(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	assertColumns(t, s, "inbound_events",
		[]string{"id", "received_at", "harness", "source_event_type", "source_payload", "hitch_client_version", "request_headers", "schema_version"},
		[]string{"harness_version", "native_event_type", "native_payload_json", "source_payload_json", "source_adapter_version", "request_headers_json"},
	)
	assertColumns(t, s, "normalized_events",
		[]string{"id", "hitch_version", "event_id", "received_at", "harness", "source_event_type", "source_payload", "hitch_event_type", "session_id", "turn_id", "cwd", "model", "transcript_path", "payload", "inbound_event_id", "mapping_version", "schema_version"},
		[]string{"harness_version", "normalized_payload_json"},
	)
	assertColumns(t, s, "native_responses",
		[]string{"id", "normalized_event_id", "response_json", "emitted_at", "schema_version"},
		[]string{"harness", "source_event_type", "native_event_type"},
	)
}

func TestStoreMigrateResetsVersionMismatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	now := time.Now().UTC()
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_old", ReceivedAt: now, Harness: protocol.HarnessCodex, SourceEventType: "PreToolUse", SourcePayload: protocol.Raw(map[string]interface{}{}), HitchEventType: protocol.EventToolRequested, Payload: protocol.Raw(map[string]interface{}{})}
	if err := s.InsertInbound(ctx, InboundEvent{ID: "in_old", ReceivedAt: now, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload}); err != nil {
		t.Fatalf("insert inbound: %v", err)
	}
	if err := s.InsertNormalized(ctx, NormalizedEvent{ID: "norm_old", InboundEventID: "in_old", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatalf("insert normalized: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE schema_meta SET version = 2`); err != nil {
		t.Fatalf("downgrade schema version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	var version int
	if err := reopened.db.QueryRowContext(ctx, `SELECT version FROM schema_meta`).Scan(&version); err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}
	var count int
	if err := reopened.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM normalized_events`).Scan(&count); err != nil {
		t.Fatalf("count normalized events: %v", err)
	}
	if count != 0 {
		t.Fatalf("version mismatch should reset old event data, found %d normalized events", count)
	}
}

func assertColumns(t *testing.T, s *Store, table string, want []string, absent []string) {
	t.Helper()
	got := tableColumns(t, s, table)
	for _, column := range want {
		if !got[column] {
			t.Fatalf("%s missing column %s; got %#v", table, column, got)
		}
	}
	for _, column := range absent {
		if got[column] {
			t.Fatalf("%s still has obsolete column %s; got %#v", table, column, got)
		}
	}
}

func tableColumns(t *testing.T, s *Store, table string) map[string]bool {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(), `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("table info %s: %v", table, err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info %s: %v", table, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table info rows %s: %v", table, err)
	}
	return columns
}
