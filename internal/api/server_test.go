package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/protocol"
	"github.com/sagebynature/hitch/internal/store"
)

func testConfig() config.Config {
	c, err := config.Parse([]byte(`
[server]
host = "127.0.0.1"
port = 8799
max_request_bytes = 1048576
[log]
level = "info"
format = "json"
[log.stdout]
enabled = true
[log.file]
enabled = false
path = "x"
max_size_mb = 1
[audit]
enabled = true
backend = "sqlite"
[audit.sqlite]
path = "x"
[harness.codex]
enabled = true
[harness.hermes]
enabled = true
[harness.pi]
enabled = true
[harness.omp]
enabled = true
`))
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

	body := `{"harness":"codex","harness_version":"","source_event_type":"PreToolUse","source_payload":{"tool":"bash"},"hitch_client_version":"test"}`
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
	body := []byte(`{"harness":"hermes","harness_version":"","source_event_type":"pre_tool_call","source_payload":{"tool_name":"bash"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/dispatch-sync", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dispatch code %d body %s", w.Code, w.Body.String())
	}
	var resp DispatchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.NativeResponse) == 0 {
		t.Fatalf("missing native response")
	}
	inspectReq := httptest.NewRequest(http.MethodGet, "/v1/events/"+resp.NormalizedEventID, nil)
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

func TestEventMapOverrideAndAddition(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	cfg.Harness.Codex.EventMap = map[string]protocol.EventType{
		"PreToolUse": protocol.EventToolPermissionRequest,
		"CustomHook": protocol.EventTurnStarted,
	}
	s := New(cfg, slog.Default(), st)

	overrideReq := httptest.NewRequest(http.MethodPost, "/v1/dispatch-sync", strings.NewReader(`{"harness":"codex","harness_version":"","source_event_type":"PreToolUse","source_payload":{"tool":"bash"},"hitch_client_version":"test"}`))
	overrideW := httptest.NewRecorder()
	s.Handler().ServeHTTP(overrideW, overrideReq)
	if overrideW.Code != http.StatusOK {
		t.Fatalf("override dispatch code %d body %s", overrideW.Code, overrideW.Body.String())
	}
	var overrideResp DispatchResponse
	if err := json.Unmarshal(overrideW.Body.Bytes(), &overrideResp); err != nil {
		t.Fatal(err)
	}
	inspection, err := st.InspectEvent(ctx, overrideResp.NormalizedEventID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Normalized.HitchEventType != protocol.EventToolPermissionRequest {
		t.Fatalf("override mapping ignored: %#v", inspection.Normalized)
	}

	addedReq := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"harness":"codex","harness_version":"","source_event_type":"CustomHook","source_payload":{},"hitch_client_version":"test"}`))
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
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"harness":"codex","harness_version":"","source_event_type":"CustomHook","source_payload":{},"hitch_client_version":"test"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsupported source event code %d body %s", w.Code, w.Body.String())
	}
}
