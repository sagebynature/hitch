package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
	"github.com/sagebynature/hitch/internal/store"
)

var _ eventStore = (*store.Store)(nil)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *lockedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func testConfig() config.Config {
	c, err := config.Parse([]byte(config.DefaultConfigTOML))
	if err != nil {
		panic(err)
	}
	return c
}

func TestHealthAndEvent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(testConfig(), slog.Default(), st)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health code %d", w.Code)
	}

	body := `{"harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test","transcript_path":"/tmp/transcript.jsonl"},"hitch_client_version":"test"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.NormalizedEventID == "" {
		t.Fatalf("missing id: %#v", resp)
	}
	inspection, err := st.InspectEvent(ctx, resp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Normalized.Envelope.SessionID != "session_1" || inspection.Normalized.Envelope.TurnID != "turn_1" || inspection.Normalized.Envelope.CWD != "/tmp/hitch" || inspection.Normalized.Envelope.Model != "gpt-test" || inspection.Normalized.Envelope.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Fatalf("metadata not persisted: %#v", inspection.Normalized.Envelope)
	}
}

func TestHealthLogsDebugSummary(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), st)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"DEBUG"`, `"msg":"api request completed"`, `"method":"GET"`, `"path":"/v1/health"`, `"status":200`, `"duration_ms":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("health log missing %s:\n%s", want, text)
		}
	}
}

func TestEventAsyncSuccessLogsDebugSummaryWithoutPayload(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), st)
	body := `{"harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"secret-command","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test"},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	text := logs.String()
	for _, want := range []string{`"level":"DEBUG"`, `"msg":"api request completed"`, `"method":"POST"`, `"path":"/v1/events"`, `"mode":"async"`, `"harness":"codex"`, `"source_event_type":"PreToolUse"`, `"event_id":"` + resp.EventID + `"`, `"normalized_event_id":"` + resp.NormalizedEventID + `"`, `"hitch_event_type":"tool.requested"`, `"session_id":"session_1"`, `"turn_id":"turn_1"`, `"cwd":"/tmp/hitch"`, `"model":"gpt-test"`, `"status":202`, `"duration_ms":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("async event log missing %s:\n%s", want, text)
		}
	}
	for _, leaked := range []string{"secret-command", "source_payload"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("async event log leaked payload %q: %s", leaked, text)
		}
	}
}

func TestEventSyncSuccessLogsInfoSummary(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Handlers = map[string]config.HandlerConfig{
		"allow_policy": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"allow"}}'`},
			Events:    []string{string(protocol.EventToolRequested)},
			Kind:      "control",
			TimeoutMS: 1000,
		},
	}

	var logs lockedBuffer
	s := New(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), st)
	body := []byte(`{"mode":"sync","harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync event code %d body %s", w.Code, w.Body.String())
	}
	eventID := w.Header().Get("X-Hitch-Event-ID")
	normalizedID := w.Header().Get("X-Hitch-Normalized-Event-ID")
	if eventID == "" || normalizedID == "" {
		t.Fatalf("missing sync response IDs: event=%q normalized=%q", eventID, normalizedID)
	}
	text := logs.String()
	for _, want := range []string{`"level":"INFO"`, `"msg":"api request completed"`, `"method":"POST"`, `"path":"/v1/events"`, `"mode":"sync"`, `"harness":"codex"`, `"source_event_type":"PreToolUse"`, `"event_id":"` + eventID + `"`, `"normalized_event_id":"` + normalizedID + `"`, `"hitch_event_type":"tool.requested"`, `"session_id":"session_1"`, `"status":200`, `"control_handler_count":1`, `"native_response_bytes":`, `"duration_ms":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("sync event log missing %s:\n%s", want, text)
		}
	}
}

func TestSyncSuccessLogsPassthroughOutcomeWhenNoControlHandlersRun(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, nil)), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"sync","harness":"omp","source_event_type":"tool_result","source_payload":{"name":"bash"},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	if !strings.Contains(text, `"sync_outcome":"passthrough"`) {
		t.Fatalf("missing passthrough sync outcome:\n%s", text)
	}
}

func TestOMPSyncNoControlDecisionReturnsEmptyPassThrough(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := New(testConfig(), slog.Default(), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"sync","harness":"omp","source_event_type":"input","source_payload":{"text":"hello"},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync code %d body %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "{}" {
		t.Fatalf("OMP no-control sync should return empty pass-through body, got %s", w.Body.String())
	}

	normalizedID := w.Header().Get("X-Hitch-Normalized-Event-ID")
	if normalizedID == "" {
		t.Fatal("missing normalized event header")
	}
	inspection, err := st.InspectEvent(ctx, normalizedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.HandlerInvocations) != 0 {
		t.Fatalf("no-control sync should not run handlers: %#v", inspection.HandlerInvocations)
	}
	if len(inspection.NativeResponses) != 1 || strings.TrimSpace(string(inspection.NativeResponses[0].Response)) != "{}" {
		t.Fatalf("native pass-through response should be empty object: %#v", inspection.NativeResponses)
	}
}

func TestSyncSuccessLogsPassthroughOutcomeWhenControlHandlersReturnNone(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Handlers = map[string]config.HandlerConfig{
		"noop_policy": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"none"}}'`},
			Events:    []string{string(protocol.EventToolRequested)},
			Kind:      "control",
			TimeoutMS: 1000,
		},
	}

	var logs lockedBuffer
	s := New(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"sync","harness":"omp","source_event_type":"tool_call","source_payload":{"name":"bash"},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	if !strings.Contains(text, `"sync_outcome":"passthrough"`) {
		t.Fatalf("missing passthrough sync outcome:\n%s", text)
	}
}

func TestSyncSuccessLogsHandlerDecisionOutcomeWhenControlHandlersRun(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Handlers = map[string]config.HandlerConfig{
		"allow_policy": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"allow"}}'`},
			Events:    []string{string(protocol.EventToolRequested)},
			Kind:      "control",
			TimeoutMS: 1000,
		},
	}

	var logs lockedBuffer
	s := New(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"sync","harness":"omp","source_event_type":"tool_call","source_payload":{"name":"bash"},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	if !strings.Contains(text, `"sync_outcome":"handler_decision"`) {
		t.Fatalf("missing handler_decision sync outcome:\n%s", text)
	}
}

func TestEventFailureLogsInfoSummary(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, nil)), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"later","harness":"codex","source_event_type":"PreToolUse","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"INFO"`, `"msg":"api request failed"`, `"method":"POST"`, `"path":"/v1/events"`, `"mode":"later"`, `"harness":"codex"`, `"source_event_type":"PreToolUse"`, `"status":400`, `"error":"mode must be async or sync"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("event failure log missing %s:\n%s", want, text)
		}
	}
}

func TestKnownUnmappedSourceEventLogsDebugIgnored(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"async","harness":"omp","source_event_type":"message_start","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("known-unmapped event should be ignored with 202, got %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"DEBUG"`, `"msg":"api request ignored"`, `"error_kind":"unmapped_source_event"`, `"harness":"omp"`, `"source_event_type":"message_start"`, `"status":202`} {
		if !strings.Contains(text, want) {
			t.Fatalf("known-unmapped log missing %s:\n%s", want, text)
		}
	}
}

func TestUnknownSourceEventLogsInfoFailure(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, nil)), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"async","harness":"omp","source_event_type":"surprise_new_event","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown event should remain 400, got %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"INFO"`, `"msg":"api request failed"`, `"error_kind":"unknown_source_event"`, `"harness":"omp"`, `"source_event_type":"surprise_new_event"`, `"status":400`} {
		if !strings.Contains(text, want) {
			t.Fatalf("unknown-event log missing %s:\n%s", want, text)
		}
	}
}

func TestStoreBusyFailuresLogErrorKind(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, nil)), st)

	conn, err := st.RawConn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN EXCLUSIVE`); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(ctx, `ROLLBACK`)

	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"async","harness":"omp","source_event_type":"turn_start","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected store busy 500, got %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	if !strings.Contains(text, `"error_kind":"store_busy"`) {
		t.Fatalf("missing store_busy error kind:\n%s", text)
	}
}

func TestEventObserverOnlySyncFailureLogsInfoSummary(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, nil)), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"sync","harness":"hermes","source_event_type":"on_session_end","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("observer-only sync code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"INFO"`, `"msg":"api request failed"`, `"mode":"sync"`, `"harness":"hermes"`, `"source_event_type":"on_session_end"`, `"status":400`, `"error":"hermes event \"on_session_end\" does not support sync dispatch"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("observer-only sync log missing %s:\n%s", want, text)
		}
	}
}

func TestInspectEventLogsSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs lockedBuffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), st)

	body := `{"harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash"},"hitch_client_version":"test"}`
	postReq := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	postW := httptest.NewRecorder()
	s.Handler().ServeHTTP(postW, postReq)
	if postW.Code != http.StatusAccepted {
		t.Fatalf("post code %d body %s", postW.Code, postW.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(postW.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	logs.Reset()
	inspectReq := httptest.NewRequest(http.MethodGet, "/v1/events/"+resp.NormalizedEventID, nil)
	inspectW := httptest.NewRecorder()
	s.Handler().ServeHTTP(inspectW, inspectReq)
	if inspectW.Code != http.StatusOK {
		t.Fatalf("inspect code %d body %s", inspectW.Code, inspectW.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"DEBUG"`, `"msg":"api request completed"`, `"method":"GET"`, `"path":"/v1/events/` + resp.NormalizedEventID + `"`, `"normalized_event_id":"` + resp.NormalizedEventID + `"`, `"status":200`} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspect success log missing %s:\n%s", want, text)
		}
	}

	logs.Reset()
	missingReq := httptest.NewRequest(http.MethodGet, "/v1/events/missing", nil)
	missingW := httptest.NewRecorder()
	s.Handler().ServeHTTP(missingW, missingReq)
	if missingW.Code != http.StatusInternalServerError {
		t.Fatalf("missing inspect code %d body %s", missingW.Code, missingW.Body.String())
	}
	text = logs.String()
	for _, want := range []string{`"level":"INFO"`, `"msg":"api request failed"`, `"method":"GET"`, `"path":"/v1/events/missing"`, `"normalized_event_id":"missing"`, `"status":500`, `"error":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspect failure log missing %s:\n%s", want, text)
		}
	}
}

func TestObserverDispatchLogsDebugSummary(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Handlers = map[string]config.HandlerConfig{
		"audit": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"none"}}'`},
			Events:    []string{string(protocol.EventToolRequested)},
			Kind:      "observer",
			TimeoutMS: 1000,
		},
	}

	var logs lockedBuffer
	s := New(cfg, slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), st)
	body := `{"harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash","session_id":"session_1"},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), `"msg":"observer dispatch completed"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	text := logs.String()
	for _, want := range []string{`"level":"DEBUG"`, `"msg":"observer dispatch completed"`, `"observer_handler_count":1`, `"harness":"codex"`, `"source_event_type":"PreToolUse"`, `"hitch_event_type":"tool.requested"`, `"session_id":"session_1"`, `"duration_ms":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("observer dispatch log missing %s:\n%s", want, text)
		}
	}
}

func TestDispatchSync(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	s := New(cfg, slog.Default(), st)
	body := []byte(`{"mode":"sync","harness":"hermes","source_event_type":"pre_tool_call","source_payload":{"tool_name":"bash"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dispatch code %d body %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "{}" {
		t.Fatalf("unexpected native response %s", w.Body.String())
	}
	normalizedID := w.Header().Get("X-Hitch-Normalized-Event-ID")
	if normalizedID == "" {
		t.Fatal("missing normalized event header")
	}
	inspectReq := httptest.NewRequest(http.MethodGet, "/v1/events/"+normalizedID, nil)
	inspectW := httptest.NewRecorder()
	s.Handler().ServeHTTP(inspectW, inspectReq)
	if inspectW.Code != http.StatusOK {
		t.Fatalf("inspect code %d body %s", inspectW.Code, inspectW.Body.String())
	}
	var inspection map[string]interface{}
	if err := json.Unmarshal(inspectW.Body.Bytes(), &inspection); err != nil {
		t.Fatal(err)
	}
	if inspection["inbound"] == nil || inspection["normalized"] == nil || inspection["native_responses"] == nil {
		t.Fatalf("incomplete inspection: %s", inspectW.Body.String())
	}
}

func TestEventRejectsInvalidMode(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := New(testConfig(), slog.Default(), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"later","harness":"codex","source_event_type":"PreToolUse","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode code %d body %s", w.Code, w.Body.String())
	}
}

func TestEventSyncModeRejectsObserverOnlySourceEvent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := New(testConfig(), slog.Default(), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"sync","harness":"hermes","source_event_type":"on_session_end","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("observer-only sync code %d body %s", w.Code, w.Body.String())
	}
}

func TestDispatchSyncWithDefaultConfigRunsNoHandlersAndPassesThrough(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := New(testConfig(), slog.Default(), st)
	body := []byte(`{"mode":"sync","harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dispatch code %d body %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "{}" {
		t.Fatalf("default sync dispatch should pass through, got %s", w.Body.String())
	}
	normalizedID := w.Header().Get("X-Hitch-Normalized-Event-ID")
	if normalizedID == "" {
		t.Fatal("missing normalized event header")
	}
	inspection, err := st.InspectEvent(ctx, normalizedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.HandlerInvocations) != 0 {
		t.Fatalf("default config should not run handlers: %#v", inspection.HandlerInvocations)
	}
	if len(inspection.NativeResponses) != 1 {
		t.Fatalf("native pass-through response was not persisted: %#v", inspection.NativeResponses)
	}
}

func TestDispatchSyncAlsoRunsAsyncObservers(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Handlers = map[string]config.HandlerConfig{
		"async_noop": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"none"}}'`},
			Events:    []string{"*"},
			Kind:      "observer",
			TimeoutMS: 1000,
			OnError:   "fail_open",
			OnTimeout: "fail_open",
		},
		"sync_noop": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"none"}}'`},
			Events:    []string{"*"},
			Kind:      "control",
			TimeoutMS: 1000,
			OnError:   "fail_open",
			OnTimeout: "fail_open",
		},
	}
	s := New(cfg, slog.Default(), st)
	body := []byte(`{"mode":"sync","harness":"omp","source_event_type":"tool_call","source_payload":{"event":{"name":"bash","input":{"command":"pwd"}}},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dispatch code %d body %s", w.Code, w.Body.String())
	}
	normalizedID := w.Header().Get("X-Hitch-Normalized-Event-ID")
	if normalizedID == "" {
		t.Fatal("missing normalized event header")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		inspection, err := st.InspectEvent(ctx, normalizedID)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]string{}
		for _, inv := range inspection.HandlerInvocations {
			seen[inv.HandlerName] = inv.Kind
		}
		if seen["sync_noop"] == "control" && seen["async_noop"] == "observer" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sync dispatch did not persist both control and observer handler invocations: %#v", inspection.HandlerInvocations)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDispatchSyncLogsHandlerInvocationDetails(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Handlers = map[string]config.HandlerConfig{
		"logger_check": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"allow"}}'`},
			Events:    []string{string(protocol.EventToolRequested)},
			Kind:      "control",
			TimeoutMS: 1000,
		},
	}
	var logs lockedBuffer
	s := New(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), st)
	body := []byte(`{"mode":"sync","harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dispatch code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{
		`"msg":"handler invocation starting"`,
		`"msg":"handler invocation completed"`,
		`"handler":"logger_check"`,
		`"kind":"control"`,
		`"harness":"codex"`,
		`"source_event_type":"PreToolUse"`,
		`"hitch_event_type":"tool.requested"`,
		`"session_id":"session_1"`,
		`"turn_id":"turn_1"`,
		`"cwd":"/tmp/hitch"`,
		`"model":"gpt-test"`,
		`"status":"ok"`,
		`"duration_ms":`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("handler invocation logs missing %s:\n%s", want, text)
		}
	}
}

func TestDispatchSyncOpenCodeToolBeforeDenyTranslatesToThrow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Harness.OpenCode.Enabled = true
	cfg.Harness.OpenCode.EventMap = map[string]config.EventTypes{"tool.execute.before": {protocol.EventToolRequested}}
	cfg.Handlers = map[string]config.HandlerConfig{
		"deny": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"behavior":"deny","reason":"no"}}'`},
			Events:    []string{string(protocol.EventToolRequested)},
			Kind:      "control",
			TimeoutMS: 1000,
			OnError:   "fail_open",
			OnTimeout: "fail_open",
		},
	}
	server := New(cfg, slog.Default(), st)
	body := `{"mode":"sync","harness":"opencode","source_event_type":"tool.execute.before","source_payload":{"event":{"input":{"tool":"bash","sessionID":"s1","callID":"c1"},"output":{"args":{"command":"pwd"}}},"metadata":{"session_id":"s1"}},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var native map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &native); err != nil {
		t.Fatal(err)
	}
	if native["adapter_action"] != "throw" || native["message"] != "no" {
		t.Fatalf("unexpected native response: %#v", native)
	}
}

func TestDispatchSyncNativeResponseOnlyPassesThrough(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Harness.OpenCode.Enabled = true
	cfg.Harness.OpenCode.EventMap = map[string]config.EventTypes{"tool.execute.before": {protocol.EventToolRequested}}
	cfg.Handlers = map[string]config.HandlerConfig{
		"native": {
			Command:   []string{"/bin/sh", "-c", `printf '%s' '{"status":"ok","decision":{"native_response":{"adapter_action":"throw","message":"native-only"}}}'`},
			Events:    []string{string(protocol.EventToolRequested)},
			Kind:      "control",
			TimeoutMS: 1000,
			OnError:   "fail_open",
			OnTimeout: "fail_open",
		},
	}
	var logs lockedBuffer
	server := New(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), st)
	body := `{"mode":"sync","harness":"opencode","source_event_type":"tool.execute.before","source_payload":{"event":{"input":{"tool":"bash","sessionID":"s1","callID":"c1"},"output":{"args":{"command":"pwd"}}},"metadata":{"session_id":"s1"}},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var native map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &native); err != nil {
		t.Fatal(err)
	}
	if native["adapter_action"] != "throw" || native["message"] != "native-only" {
		t.Fatalf("native_response-only decision was not passed through: %#v body=%s", native, rec.Body.String())
	}
	text := logs.String()
	if !strings.Contains(text, `"sync_outcome":"handler_decision"`) {
		t.Fatalf("native_response-only decision should be logged as handler-driven:\n%s", text)
	}
}

func TestEventMapOverrideAndAddition(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Harness.Codex.EventMap = map[string]config.EventTypes{
		"PreToolUse": {protocol.EventToolPermissionRequest},
		"CustomHook": {protocol.EventTurnStarted},
	}
	s := New(cfg, slog.Default(), st)

	overrideReq := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"mode":"sync","harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash"},"hitch_client_version":"test"}`))
	overrideW := httptest.NewRecorder()
	s.Handler().ServeHTTP(overrideW, overrideReq)
	if overrideW.Code != http.StatusOK {
		t.Fatalf("override dispatch code %d body %s", overrideW.Code, overrideW.Body.String())
	}
	normalizedID := overrideW.Header().Get("X-Hitch-Normalized-Event-ID")
	if normalizedID == "" {
		t.Fatal("missing normalized event header")
	}
	inspection, err := st.InspectEvent(ctx, normalizedID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Normalized.HitchEventType != protocol.EventToolPermissionRequest {
		t.Fatalf("override mapping ignored: %#v", inspection.Normalized)
	}

	addedReq := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"harness":"codex","source_event_type":"CustomHook","source_payload":{},"hitch_client_version":"test"}`))
	addedW := httptest.NewRecorder()
	s.Handler().ServeHTTP(addedW, addedReq)
	if addedW.Code != http.StatusAccepted {
		t.Fatalf("added event code %d body %s", addedW.Code, addedW.Body.String())
	}
	var addedResp EventResponse
	if err := json.Unmarshal(addedW.Body.Bytes(), &addedResp); err != nil {
		t.Fatal(err)
	}
	inspection, err = st.InspectEvent(ctx, addedResp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Inbound.SourceEventType != "CustomHook" || inspection.Normalized.HitchEventType != protocol.EventTurnStarted {
		t.Fatalf("added mapping ignored: %#v", inspection)
	}
}

func TestUnsupportedSourceEventRejected(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(testConfig(), slog.Default(), st)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"harness":"codex","source_event_type":"CustomHook","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsupported source event code %d body %s", w.Code, w.Body.String())
	}
}

func TestDefaultConfigIgnoresNoisySourceEventsButAllowsOptIn(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	defaultServer := New(testConfig(), slog.Default(), st)
	defaultReq := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"harness":"omp","source_event_type":"before_provider_request","source_payload":{"type":"before_provider_request","payload":{"model":"gpt-test"}},"hitch_client_version":"test"}`))
	defaultW := httptest.NewRecorder()
	defaultServer.Handler().ServeHTTP(defaultW, defaultReq)
	if defaultW.Code != http.StatusAccepted {
		t.Fatalf("default config should ignore excluded source event, got %d body %s", defaultW.Code, defaultW.Body.String())
	}
	var ignoredResp EventResponse
	if err := json.Unmarshal(defaultW.Body.Bytes(), &ignoredResp); err != nil {
		t.Fatal(err)
	}
	if ignoredResp.EventID != "" || ignoredResp.NormalizedEventID != "" {
		t.Fatalf("ignored event should not persist IDs: %#v", ignoredResp)
	}

	cfg := testConfig()
	cfg.Harness.OMP.EventMap["before_provider_request"] = config.EventTypes{protocol.EventLLMRequested}
	optInServer := New(cfg, slog.Default(), st)
	optInReq := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"harness":"omp","source_event_type":"before_provider_request","source_payload":{"type":"before_provider_request","payload":{"model":"gpt-test"}},"hitch_client_version":"test"}`))
	optInW := httptest.NewRecorder()
	optInServer.Handler().ServeHTTP(optInW, optInReq)
	if optInW.Code != http.StatusAccepted {
		t.Fatalf("opt-in config should accept source event, got %d body %s", optInW.Code, optInW.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(optInW.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	inspection, err := st.InspectEvent(ctx, resp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Normalized.HitchEventType != protocol.EventLLMRequested {
		t.Fatalf("opt-in mapping did not use llm.requested: %#v", inspection.Normalized)
	}
}

func TestIngestPersistsDerivedAssistantCompletionEvent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(testConfig(), slog.Default(), st)

	body := `{"harness":"pi","source_event_type":"turn_end","source_payload":{"event":{"type":"turn_end","turnIndex":3,"message":{"role":"assistant","content":[{"type":"text","text":"done"}]},"toolResults":[]},"metadata":{"session_id":"session_1","turn_id":"turn_3","cwd":"/tmp/hitch","model":"gpt-test","transcript_path":"/tmp/transcript.jsonl"}},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	primary, err := st.InspectEvent(ctx, resp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if primary.Normalized.HitchEventType != protocol.EventTurnCompleted {
		t.Fatalf("primary mapping changed: %#v", primary.Normalized)
	}
	derivedID, err := st.LatestEventIDByType(ctx, protocol.EventTurnAssistantCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if derivedID == resp.NormalizedEventID {
		t.Fatalf("derived event reused primary id %q", derivedID)
	}
	derived, err := st.InspectEvent(ctx, derivedID)
	if err != nil {
		t.Fatal(err)
	}
	if derived.Normalized.InboundEventID != primary.Normalized.InboundEventID {
		t.Fatalf("derived event not tied to same inbound event: primary=%#v derived=%#v", primary.Normalized, derived.Normalized)
	}
	if derived.Normalized.Envelope.HitchEventType != protocol.EventTurnAssistantCompleted {
		t.Fatalf("derived event has wrong type: %#v", derived.Normalized.Envelope)
	}
	if derived.Normalized.Envelope.SessionID != "session_1" || derived.Normalized.Envelope.TurnID != "turn_3" || derived.Normalized.Envelope.Model != "gpt-test" {
		t.Fatalf("derived event lost metadata: %#v", derived.Normalized.Envelope)
	}
	var assistantPayload map[string]interface{}
	if err := json.Unmarshal(derived.Normalized.Envelope.Payload, &assistantPayload); err != nil {
		t.Fatal(err)
	}
	assistant, ok := assistantPayload["turn"].(map[string]interface{})["assistant"].(map[string]interface{})
	if !ok || assistant["text"] != "done" {
		t.Fatalf("derived assistant payload missed final text: %s", derived.Normalized.Envelope.Payload)
	}
}

func TestIngestPersistsConfiguredSecondaryEvent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(testConfig(), slog.Default(), st)

	body := `{"harness":"hermes","source_event_type":"pre_llm_call","source_payload":{"session_id":"session_1","cwd":"/tmp/hitch","extra":{"turn_id":"turn_1","model":"gpt-test","user_message":"inspect this repo"}},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	primary, err := st.InspectEvent(ctx, resp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if primary.Normalized.HitchEventType != protocol.EventLLMRequested {
		t.Fatalf("primary mapping changed: %#v", primary.Normalized)
	}
	promptID, err := st.LatestEventIDByType(ctx, protocol.EventTurnUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := st.InspectEvent(ctx, promptID)
	if err != nil {
		t.Fatal(err)
	}
	if prompt.Normalized.InboundEventID != primary.Normalized.InboundEventID {
		t.Fatalf("secondary event not tied to same inbound event: primary=%#v secondary=%#v", primary.Normalized, prompt.Normalized)
	}
	if prompt.Normalized.Envelope.HitchEventType != protocol.EventTurnUserPrompt {
		t.Fatalf("secondary event has wrong type: %#v", prompt.Normalized.Envelope)
	}
	if prompt.Normalized.Envelope.TurnID != "turn_1" || prompt.Normalized.Envelope.Model != "gpt-test" {
		t.Fatalf("secondary event lost metadata: %#v", prompt.Normalized.Envelope)
	}
	var secondaryPayload map[string]interface{}
	if err := json.Unmarshal(prompt.Normalized.Envelope.Payload, &secondaryPayload); err != nil {
		t.Fatal(err)
	}
	turn, ok := secondaryPayload["turn"].(map[string]interface{})
	if !ok || turn["prompt"] != "inspect this repo" {
		t.Fatalf("secondary event payload was not reparsed for turn.user_prompt: %s", prompt.Normalized.Envelope.Payload)
	}
}

func TestIngestOpenCodeStepFinishPersistsLLMUsagePayload(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(testConfig(), slog.Default(), st)

	body := `{"harness":"opencode","source_event_type":"message.part.step-finish","source_payload":{"event":{"type":"message.part.updated","properties":{"sessionID":"sess_1","messageID":"msg_1","part":{"type":"step-finish","reason":"stop","tokens":{"input":10,"output":4,"reasoning":1,"cache":{"read":2,"write":3},"total":20},"cost":0.0025}}},"metadata":{"session_id":"sess_1","turn_id":"msg_1","cwd":"/tmp/hitch","model":"anthropic/claude-sonnet-4"}},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	inspection, err := st.InspectEvent(ctx, resp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Normalized.HitchEventType != protocol.EventLLMCompleted {
		t.Fatalf("unexpected event type: %#v", inspection.Normalized)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(inspection.Normalized.Envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	llm := payload["llm"].(map[string]interface{})
	usage := llm["usage"].(map[string]interface{})
	tokens := usage["tokens"].(map[string]interface{})
	cost := usage["cost"].(map[string]interface{})
	if llm["finish_reason"] != "stop" || tokens["input"] != float64(10) || tokens["cache_read"] != float64(2) || cost["amount"] != 0.0025 || cost["source"] != "opencode_step_finish" {
		t.Fatalf("bad llm usage payload: %s", inspection.Normalized.Envelope.Payload)
	}
}

type registryTestNormalizer struct{}

func (registryTestNormalizer) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := harness.NewEnvelope(protocol.HarnessCodex, sourceEventType, sourcePayload, hitchEventType, sourcePayload)
	env.SessionID = "registry-test"
	return env, protocol.ValidateEnvelope(env)
}

func (registryTestNormalizer) Translate(string, protocol.AggregateDecision) (protocol.RawJSON, error) {
	return nil, nil
}

func (registryTestNormalizer) Capability(string) harness.SourceEventCapability {
	return harness.CapabilityObserverOnly
}

func (registryTestNormalizer) KnownSourceEvents() map[string]struct{} {
	return map[string]struct{}{"registry_test_event": {}}
}

type registryWrongHarnessNormalizer struct {
	registryTestNormalizer
}

func (registryWrongHarnessNormalizer) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	env := harness.NewEnvelope(protocol.HarnessOMP, sourceEventType, sourcePayload, hitchEventType, sourcePayload)
	return env, protocol.ValidateEnvelope(env)
}

func (registryWrongHarnessNormalizer) Capability(string) harness.SourceEventCapability {
	return harness.CapabilityControlCapable
}

func TestNewWithHarnessRegistryUsesInjectedRegistry(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := testConfig()
	cfg.Harness.Codex.EventMap = map[string]config.EventTypes{
		"registry_test_event": {protocol.EventToolRequested},
	}
	registry := harness.NewRegistry(map[protocol.Harness]harness.Normalizer{
		protocol.HarnessCodex: registryTestNormalizer{},
	})
	s := NewWithHarnessRegistry(cfg, slog.Default(), st, registry)

	body := `{"harness":"codex","source_event_type":"registry_test_event","source_payload":{"tool":"bash"},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	var resp EventResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	inspection, err := st.InspectEvent(ctx, resp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Normalized.Envelope.SessionID != "registry-test" {
		t.Fatalf("server did not use injected registry normalizer: %#v", inspection.Normalized)
	}
}

func TestNewWithHarnessRegistryRejectsMismatchedNormalizerHarness(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := testConfig()
	cfg.Harness.Codex.EventMap = map[string]config.EventTypes{
		"registry_test_event": {protocol.EventToolRequested},
	}
	registry := harness.NewRegistry(map[protocol.Harness]harness.Normalizer{
		protocol.HarnessCodex: registryWrongHarnessNormalizer{},
	})
	s := NewWithHarnessRegistry(cfg, slog.Default(), st, registry)

	body := `{"mode":"sync","harness":"codex","source_event_type":"registry_test_event","source_payload":{"tool":"bash"},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `normalizer returned harness`) {
		t.Fatalf("unexpected body %s", w.Body.String())
	}
}

func TestSyncDispatchDoesNotRunSameObserverHookTwiceForInboundEvent(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig()
	logPath := filepath.Join(t.TempDir(), "count.txt")
	cfg.Handlers = map[string]config.HandlerConfig{
		"observer": {
			Command:   []string{"/bin/sh", "-c", "printf x >> " + shellQuote(logPath) + "; printf '%s' '{\"status\":\"ok\"}'"},
			Events:    []string{"*"},
			Kind:      "observer",
			TimeoutMS: 1000,
		},
	}
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(cfg, slog.Default(), st)
	body := `{"mode":"sync","harness":"codex","source_event_type":"PreToolUse","source_payload":{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"pwd"}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync request failed: %d %s", w.Code, w.Body.String())
	}
	eventID := w.Header().Get("X-Hitch-Normalized-Event-ID")
	env, err := st.GetEvent(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := st.InspectEvent(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	srv.dispatchObservers(ctx, inspection.Inbound.ID, eventID, env)
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "x" {
		t.Fatalf("observer ran more than once: %q", b)
	}
}
