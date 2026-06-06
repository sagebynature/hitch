# Hitch Serve Endpoint Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add request-level structured logging for `hitch serve` API endpoints without logging raw payloads.

**Architecture:** Keep the change local to `internal/api/server.go` and `internal/api/server_test.go`. Add a small request-summary helper that whitelists safe fields, emits `debug` logs for routine success paths, and emits `info` logs for sync success and failures. Existing `dispatch.Runner` logs remain responsible for per-handler execution details.

**Tech Stack:** Go `net/http`, standard-library `log/slog`, existing Hitch API server and tests.

---

## Source Spec

Implement `docs/superpowers/specs/2026-06-06-hitch-serve-endpoint-logging-design.md`.

Acceptance summary:

- `GET /v1/health` success logs at `debug`.
- `POST /v1/events` async success logs at `debug`.
- `POST /v1/events` sync success logs at `info`.
- request failures log at `info`.
- observer dispatch completion logs at `debug`.
- `GET /v1/events/:id` success logs at `debug`; failures log at `info`.
- logs include safe metadata and never raw payloads.

## File Structure

Modify:

- `internal/api/server.go`
  - add a small request log summary helper;
  - log health success;
  - log event request success and failure;
  - log sync completion;
  - log observer dispatch completion;
  - log event inspection success and failure.
- `internal/api/server_test.go`
  - add focused log-content tests;
  - preserve existing handler-invocation logging tests.

Do not modify:

- `internal/logging/logging.go`
- `internal/dispatch/dispatch.go`
- config schema files

---

### Task 1: Add API request log helper

**Files:**

- Modify: `internal/api/server.go`

- [ ] **Step 1: Add helper type near `EventResponse`**

Add this local helper in `internal/api/server.go` after the request/response structs:

```go
type apiRequestLog struct {
	started time.Time
	attrs   []any
}

func newAPIRequestLog(r *http.Request) apiRequestLog {
	return apiRequestLog{
		started: time.Now(),
		attrs: []any{
			"method", r.Method,
			"path", r.URL.Path,
		},
	}
}

func (l *apiRequestLog) add(key string, value any) {
	if value == nil {
		return
	}
	if s, ok := value.(string); ok && s == "" {
		return
	}
	l.attrs = append(l.attrs, key, value)
}

func (l *apiRequestLog) addEventRequest(mode string, req EventRequest) {
	l.add("mode", mode)
	l.add("harness", req.Harness)
	l.add("source_event_type", req.SourceEventType)
}

func (l *apiRequestLog) addEnvelope(env protocol.EventEnvelope, normalizedID string) {
	l.add("event_id", env.EventID)
	l.add("normalized_event_id", normalizedID)
	l.add("hitch_event_type", string(env.HitchEventType))
	l.add("session_id", env.SessionID)
	l.add("turn_id", env.TurnID)
	l.add("cwd", env.CWD)
	l.add("model", env.Model)
}

func (l apiRequestLog) emit(ctx context.Context, logger *slog.Logger, level slog.Level, msg string, status int, extra ...any) {
	attrs := append([]any{}, l.attrs...)
	attrs = append(attrs, "status", status, "duration_ms", time.Since(l.started).Milliseconds())
	attrs = append(attrs, extra...)
	logger.Log(ctx, level, msg, attrs...)
}
```

Run:

```sh
go test ./internal/api
```

Expected: PASS. The helper is unused but should compile.

---

### Task 2: Log health and inspection endpoints

**Files:**

- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Write failing health log test**

Add this test to `internal/api/server_test.go` near `TestHealthAndEventLifecycle`:

```go
func TestHealthLogsDebugSummary(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs bytes.Buffer
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
```

Run:

```sh
go test ./internal/api -run TestHealthLogsDebugSummary
```

Expected: FAIL because `handleHealth` does not log yet.

- [ ] **Step 2: Log health success**

Update `handleHealth` in `internal/api/server.go`:

```go
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	log := newAPIRequestLog(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	log.emit(r.Context(), s.log, slog.LevelDebug, "api request completed", http.StatusOK)
}
```

Run:

```sh
go test ./internal/api -run TestHealthLogsDebugSummary
```

Expected: PASS.

- [ ] **Step 3: Write failing inspection log tests**

Add this test pair near existing inspection tests:

```go
func TestInspectEventLogsSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs bytes.Buffer
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
```

Run:

```sh
go test ./internal/api -run TestInspectEventLogsSuccessAndFailure
```

Expected: FAIL because inspection endpoint does not log yet.

- [ ] **Step 4: Log inspection success and failure**

Update `handleGetEvent`:

```go
func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	log := newAPIRequestLog(r)
	id := strings.TrimPrefix(r.URL.Path, "/v1/events/")
	log.add("normalized_event_id", id)
	if id == "" {
		err := badRequest("missing id")
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusBadRequest, "error", err.Error())
		writeError(w, err)
		return
	}
	inspection, err := s.store.InspectEvent(r.Context(), id)
	if err != nil {
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusInternalServerError, "error", err.Error())
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inspection)
	log.emit(r.Context(), s.log, slog.LevelDebug, "api request completed", http.StatusOK)
}
```

Run:

```sh
go test ./internal/api -run 'TestHealthLogsDebugSummary|TestInspectEventLogsSuccessAndFailure'
```

Expected: PASS.

---

### Task 3: Log event endpoint success and failure

**Files:**

- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Refactor ingest to expose parsed request fields for logging**

Change `ingest` signature in `internal/api/server.go`:

```go
func (s *Server) ingest(ctx context.Context, r *http.Request) (EventResponse, protocol.EventEnvelope, EventRequest, string, error)
```

Update returns so every failure path returns the partially parsed `req` and parsed `mode` when available. Example:

```go
var req EventRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	return EventResponse{}, protocol.EventEnvelope{}, req, "", badRequest("invalid JSON: %v", err)
}
mode, err := normalizeRequestMode(req.Mode)
if err != nil {
	return EventResponse{}, protocol.EventEnvelope{}, req, strings.TrimSpace(req.Mode), err
}
```

For successful return:

```go
return EventResponse{EventID: env.EventID, NormalizedEventID: normalizedID}, env, req, mode, nil
```

Update `handleEvent` call site temporarily:

```go
resp, env, req, mode, err := s.ingest(r.Context(), r)
_ = req
```

Run:

```sh
go test ./internal/api
```

Expected: PASS after all return statements compile.

- [ ] **Step 2: Write failing async success log test**

Add this test in `internal/api/server_test.go`:

```go
func TestEventAsyncSuccessLogsDebugSummaryWithoutPayload(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs bytes.Buffer
	s := New(testConfig(), slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), st)
	body := `{"harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"secret-command","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test"},"hitch_client_version":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"DEBUG"`, `"msg":"api request completed"`, `"method":"POST"`, `"path":"/v1/events"`, `"mode":"async"`, `"harness":"codex"`, `"source_event_type":"PreToolUse"`, `"hitch_event_type":"tool.requested"`, `"session_id":"session_1"`, `"turn_id":"turn_1"`, `"cwd":"/tmp/hitch"`, `"model":"gpt-test"`, `"status":202`, `"duration_ms":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("async event log missing %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, "secret-command") {
		t.Fatalf("async event log leaked payload: %s", text)
	}
}
```

Run:

```sh
go test ./internal/api -run TestEventAsyncSuccessLogsDebugSummaryWithoutPayload
```

Expected: FAIL because event success is not logged yet.

- [ ] **Step 3: Write failing sync success log test**

Add this test:

```go
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

	var logs bytes.Buffer
	s := New(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), st)
	body := []byte(`{"mode":"sync","harness":"codex","source_event_type":"PreToolUse","source_payload":{"tool":"bash","session_id":"session_1","turn_id":"turn_1","cwd":"/tmp/hitch","model":"gpt-test"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync event code %d body %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, want := range []string{`"level":"INFO"`, `"msg":"api request completed"`, `"mode":"sync"`, `"harness":"codex"`, `"source_event_type":"PreToolUse"`, `"hitch_event_type":"tool.requested"`, `"session_id":"session_1"`, `"status":200`, `"control_handler_count":1`, `"native_response_bytes":`, `"duration_ms":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("sync event log missing %s:\n%s", want, text)
		}
	}
}
```

Run:

```sh
go test ./internal/api -run TestEventSyncSuccessLogsInfoSummary
```

Expected: FAIL because sync success is not logged yet.

- [ ] **Step 4: Write failing rejection log test**

Add this test:

```go
func TestEventFailureLogsInfoSummary(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var logs bytes.Buffer
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
```

Run:

```sh
go test ./internal/api -run TestEventFailureLogsInfoSummary
```

Expected: FAIL because rejection is not logged yet.

- [ ] **Step 5: Emit event request logs**

Update `handleEvent`:

```go
func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	log := newAPIRequestLog(r)
	resp, env, req, mode, err := s.ingest(r.Context(), r)
	log.addEventRequest(mode, req)
	if err != nil {
		status := statusForError(err)
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", status, "error", err.Error())
		writeError(w, err)
		return
	}
	log.addEnvelope(env, resp.NormalizedEventID)
	if mode == requestModeAsync {
		go s.dispatchObservers(context.Background(), resp.NormalizedEventID, env)
		writeJSON(w, http.StatusAccepted, resp)
		log.emit(r.Context(), s.log, slog.LevelDebug, "api request completed", http.StatusAccepted)
		return
	}
	s.handleSyncEvent(w, r, resp, env, log)
}
```

Add helper near `writeError`:

```go
func statusForError(err error) int {
	var he httpError
	if errors.As(err, &he) {
		return he.code
	}
	return http.StatusInternalServerError
}
```

Update `writeError` to reuse it:

```go
func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, statusForError(err), map[string]interface{}{"error": err.Error()})
}
```

Run:

```sh
go test ./internal/api -run 'TestEventAsyncSuccessLogsDebugSummaryWithoutPayload|TestEventFailureLogsInfoSummary'
```

Expected: PASS after the async and failure tests are satisfied.

- [ ] **Step 6: Log sync success and translation failure**

Change `handleSyncEvent` signature:

```go
func (s *Server) handleSyncEvent(w http.ResponseWriter, r *http.Request, resp EventResponse, env protocol.EventEnvelope, log apiRequestLog)
```

Update success path:

```go
result := s.runner.Dispatch(r.Context(), env, "control", 2*time.Second)
// existing persistence
native, err := runtime.normalizer.Translate(env.SourceEventType, result.Aggregate)
if err != nil {
	log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusInternalServerError, "control_handler_count", len(result.Invocations), "error", err.Error())
	writeError(w, err)
	return
}
// existing response write
log.emit(r.Context(), s.log, slog.LevelInfo, "api request completed", http.StatusOK, "control_handler_count", len(result.Invocations), "native_response_bytes", len(native))
```

Run:

```sh
go test ./internal/api -run TestEventSyncSuccessLogsInfoSummary
```

Expected: PASS.

---

### Task 4: Log observer dispatch summaries

**Files:**

- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Write failing observer summary test**

Add this test:

```go
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

	var logs bytes.Buffer
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
```

Run:

```sh
go test ./internal/api -run TestObserverDispatchLogsDebugSummary
```

Expected: FAIL because observer summary is not logged yet.

- [ ] **Step 2: Emit observer summary**

Update `dispatchObservers`:

```go
func (s *Server) dispatchObservers(ctx context.Context, normalizedID string, env protocol.EventEnvelope) {
	started := time.Now()
	result := s.runner.Dispatch(ctx, env, "observer", 0)
	for _, inv := range result.Invocations {
		_ = s.store.InsertHandlerInvocation(ctx, store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: normalizedID, HandlerName: inv.HandlerName, Kind: inv.Kind, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error})
	}
	attrs := []any{
		"normalized_event_id", normalizedID,
		"harness", string(env.Harness),
		"source_event_type", env.SourceEventType,
		"hitch_event_type", string(env.HitchEventType),
		"observer_handler_count", len(result.Invocations),
		"duration_ms", time.Since(started).Milliseconds(),
	}
	if env.SessionID != "" {
		attrs = append(attrs, "session_id", env.SessionID)
	}
	if env.TurnID != "" {
		attrs = append(attrs, "turn_id", env.TurnID)
	}
	if env.CWD != "" {
		attrs = append(attrs, "cwd", env.CWD)
	}
	if env.Model != "" {
		attrs = append(attrs, "model", env.Model)
	}
	s.log.Log(ctx, slog.LevelDebug, "observer dispatch completed", attrs...)
}
```

Run:

```sh
go test ./internal/api -run TestObserverDispatchLogsDebugSummary
```

Expected: PASS.

---

### Task 5: Final verification and cleanup

**Files:**

- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Run focused API tests**

Run:

```sh
go test ./internal/api
```

Expected: PASS.

- [ ] **Step 2: Run full Go test suite**

Run:

```sh
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run example smoke tests**

Run:

```sh
python3 examples/test_drive.py
python3 examples/test_payload_logger.py
```

Expected: PASS.

- [ ] **Step 4: Manual log smoke check**

Run Hitch with console JSON logging and a free port:

```sh
HITCH_TEST_DRIVE_PORT=8898 python3 examples/test_drive.py
```

Expected:

- example still passes;
- no raw command payload values appear in endpoint request summary logs;
- sync request summaries are visible at `info` if log level is `info` or lower;
- async request summaries require `debug` logging.

## Self-Review

- The plan covers every spec acceptance criterion.
- Log levels are explicit per endpoint path and outcome.
- Tests assert no payload leakage on event logs.
- The implementation stays in `internal/api/server.go` and `internal/api/server_test.go`.
- Existing handler invocation logging remains unchanged.
