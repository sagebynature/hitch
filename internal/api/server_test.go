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

func TestDispatchSync(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := testConfig()
	s := New(cfg, slog.Default(), st)
	body := []byte(`{"harness":"hermes","source_event_type":"pre_tool_call","source_payload":{"tool_name":"bash"},"hitch_client_version":"test"}`)
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
			Mode:      "sync",
			TimeoutMS: 1000,
		},
	}
	var logs bytes.Buffer
	s := New(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), st)
	body := []byte(`{"harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/dispatch-sync", bytes.NewReader(body))
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
		`"mode":"sync"`,
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
			Mode:      "sync",
			TimeoutMS: 1000,
			OnError:   "fail_open",
			OnTimeout: "fail_open",
		},
	}
	server := New(cfg, slog.Default(), st)
	body := `{"harness":"opencode","source_event_type":"tool.execute.before","source_payload":{"event":{"input":{"tool":"bash","sessionID":"s1","callID":"c1"},"output":{"args":{"command":"pwd"}}},"metadata":{"session_id":"s1"}},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/dispatch-sync", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var got DispatchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var native map[string]interface{}
	if err := json.Unmarshal(got.NativeResponse, &native); err != nil {
		t.Fatal(err)
	}
	if native["adapter_action"] != "throw" || native["message"] != "no" {
		t.Fatalf("unexpected native response: %#v", native)
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

	overrideReq := httptest.NewRequest(http.MethodPost, "/v1/dispatch-sync", strings.NewReader(`{"harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash"},"hitch_client_version":"test"}`))
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

func TestDefaultConfigExcludesNoisySourceEventsButAllowsOptIn(t *testing.T) {
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
	if defaultW.Code != http.StatusBadRequest {
		t.Fatalf("default config should reject excluded source event, got %d body %s", defaultW.Code, defaultW.Body.String())
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
}
