package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
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
	if len(inspection.HandlerInvocations) != 1 || inspection.HandlerInvocations[0].ID != "handler_1" || inspection.HandlerInvocations[0].InboundEventID != "in_1" {
		t.Fatalf("wrong handler invocations: %#v", inspection.HandlerInvocations)
	}
	hookKey := inspection.HandlerInvocations[0].HookKey
	if hookKey != "legacy:norm_1:h:control" || strings.Contains(hookKey, "handler_1") {
		t.Fatalf("legacy hook key = %q, want deterministic key without invocation id", hookKey)
	}
	if len(inspection.NativeResponses) != 1 || inspection.NativeResponses[0].ID != "resp_1" {
		t.Fatalf("wrong native responses: %#v", inspection.NativeResponses)
	}
}

func TestReserveHandlerInvocationPreventsDuplicateHookExecution(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_1", ReceivedAt: now, Harness: protocol.HarnessCodex, SourceEventType: "PostToolUse", SourcePayload: protocol.Raw(map[string]interface{}{"x": 1}), HitchEventType: protocol.EventToolCompleted, Payload: protocol.Raw(map[string]interface{}{"tool": "bash"})}
	if err := s.InsertInbound(ctx, InboundEvent{ID: "in_1", ReceivedAt: now, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload, HitchClientVersion: "test-client"}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertNormalized(ctx, NormalizedEvent{ID: "norm_1", InboundEventID: "in_1", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatal(err)
	}

	reservation := HandlerInvocationReservation{
		ID:                "handler_1",
		InboundEventID:    "in_1",
		NormalizedEventID: "norm_1",
		HandlerName:       "audit",
		Kind:              "observer",
		HookKey:           "codex:PostToolUse:tool.completed:observer",
		StartedAt:         now,
	}
	reserved, err := s.ReserveHandlerInvocation(ctx, reservation)
	if err != nil || !reserved {
		t.Fatalf("first reservation reserved=%v err=%v", reserved, err)
	}
	var status protocol.HandlerStatus
	var completedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT status, completed_at FROM handler_invocations WHERE id = ?`, "handler_1").Scan(&status, &completedAt); err != nil {
		t.Fatal(err)
	}
	if status != protocol.StatusReserved {
		t.Fatalf("reserved status = %q, want %q", status, protocol.StatusReserved)
	}
	if completedAt.Valid && completedAt.String != "" {
		t.Fatalf("reserved completed_at = %q, want empty", completedAt.String)
	}
	reservation.ID = "handler_2"
	reserved, err = s.ReserveHandlerInvocation(ctx, reservation)
	if err != nil {
		t.Fatal(err)
	}
	if reserved {
		t.Fatal("duplicate reservation succeeded")
	}

	if err := s.CompleteHandlerInvocation(ctx, HandlerInvocation{ID: "handler_1", CompletedAt: now.Add(time.Second), Status: protocol.StatusError, Error: "boom"}); err != nil {
		t.Fatalf("complete reservation: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM handler_invocations WHERE id = ?`, "handler_1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != protocol.StatusError {
		t.Fatalf("completed status = %q, want %q", status, protocol.StatusError)
	}
}

func TestReserveHandlerInvocationPrimaryKeyCollisionReturnsError(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_1", ReceivedAt: now, Harness: protocol.HarnessCodex, SourceEventType: "PostToolUse", SourcePayload: protocol.Raw(map[string]interface{}{"x": 1}), HitchEventType: protocol.EventToolCompleted, Payload: protocol.Raw(map[string]interface{}{"tool": "bash"})}
	if err := s.InsertInbound(ctx, InboundEvent{ID: "in_1", ReceivedAt: now, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertNormalized(ctx, NormalizedEvent{ID: "norm_1", InboundEventID: "in_1", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatal(err)
	}

	reservation := HandlerInvocationReservation{ID: "handler_1", InboundEventID: "in_1", NormalizedEventID: "norm_1", HandlerName: "audit", Kind: "observer", HookKey: "hook:one", StartedAt: now}
	reserved, err := s.ReserveHandlerInvocation(ctx, reservation)
	if err != nil || !reserved {
		t.Fatalf("first reservation reserved=%v err=%v", reserved, err)
	}
	reservation.HookKey = "hook:two"
	reserved, err = s.ReserveHandlerInvocation(ctx, reservation)
	if err == nil {
		t.Fatalf("primary key collision reserved=%v err=nil, want error", reserved)
	}
	if reserved {
		t.Fatal("primary key collision reported as reserved")
	}
}

func TestInsertHandlerInvocationLegacyReplayUsesDistinctHookKey(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_1", ReceivedAt: now, Harness: protocol.HarnessCodex, SourceEventType: "PostToolUse", SourcePayload: protocol.Raw(map[string]interface{}{"x": 1}), HitchEventType: protocol.EventToolCompleted, Payload: protocol.Raw(map[string]interface{}{"tool": "bash"})}
	if err := s.InsertInbound(ctx, InboundEvent{ID: "in_1", ReceivedAt: now, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertNormalized(ctx, NormalizedEvent{ID: "norm_1", InboundEventID: "in_1", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertHandlerInvocation(ctx, HandlerInvocation{ID: "handler_live", NormalizedEventID: "norm_1", HandlerName: "audit", Kind: "observer", StartedAt: now, CompletedAt: now, Status: protocol.StatusOK}); err != nil {
		t.Fatalf("insert live handler: %v", err)
	}
	if err := s.InsertHandlerInvocation(ctx, HandlerInvocation{ID: "handler_replay", NormalizedEventID: "norm_1", HandlerName: "audit", Kind: "observer", StartedAt: now.Add(time.Second), CompletedAt: now.Add(time.Second), Status: protocol.StatusOK, ReplaySourceID: "handler_live"}); err != nil {
		t.Fatalf("insert replay handler: %v", err)
	}

	inspection, err := s.InspectEvent(ctx, "norm_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.HandlerInvocations) != 2 {
		t.Fatalf("handler invocation count = %d, want 2: %#v", len(inspection.HandlerInvocations), inspection.HandlerInvocations)
	}
	if inspection.HandlerInvocations[0].HookKey != "legacy:norm_1:audit:observer" {
		t.Fatalf("live hook key = %q", inspection.HandlerInvocations[0].HookKey)
	}
	if inspection.HandlerInvocations[1].HookKey != "legacy:replay:handler_live:audit:observer" {
		t.Fatalf("replay hook key = %q", inspection.HandlerInvocations[1].HookKey)
	}
}

func TestOpenConfiguresBusyTimeoutAndWAL(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	assertSQLitePragmas(t, ctx, st.db)

	conn1, err := st.RawConn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Close()
	conn2, err := st.RawConn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	assertSQLitePragmas(t, ctx, conn1)
	assertSQLitePragmas(t, ctx, conn2)
}

type sqlitePragmaQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func assertSQLitePragmas(t *testing.T, ctx context.Context, q sqlitePragmaQuerier) {
	t.Helper()

	var busyTimeout int
	if err := q.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy_timeout = %d, want at least 5000", busyTimeout)
	}

	var journalMode string
	if err := q.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
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
	assertColumns(t, s, "handler_invocations",
		[]string{"id", "inbound_event_id", "normalized_event_id", "handler_name", "kind", "hook_key", "started_at", "completed_at", "status", "stdout", "stderr", "output_json", "decision_json", "error", "replay_source_id", "schema_version"},
		[]string{"handler_type", "payload_json"},
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
