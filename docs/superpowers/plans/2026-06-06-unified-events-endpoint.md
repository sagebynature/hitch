# Unified Events Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `/v1/dispatch-sync` with payload-driven `POST /v1/events` dispatch, rename handler `mode` to `kind`, and enforce source event capability so observer and control work stay separate.

**Architecture:** `POST /v1/events` becomes the single ingress path. Top-level request `mode` chooses async or sync dispatch, while handler `kind` chooses whether a handler observes or controls. Harness mappers expose source event capability (`observer_only` or `control_capable`) so sync requests are accepted only when the native source event can consume a response.

**Tech Stack:** Go `net/http`, TOML config via BurntSushi/toml, SQLite audit store, existing Hitch harness mappers, generated TypeScript adapters for Pi/OMP/OpenCode.

---

## Decision Source

Implement ADR-0002: `docs/adr/0002-unified-events-endpoint-and-dispatch-semantics.md`.

Core rules:

| Source event capability | Request `mode` | Control handlers | Observer handlers | Response |
| --- | --- | --- | --- | --- |
| `observer_only` | absent / `async` | No | Yes | `202` Hitch metadata JSON |
| `observer_only` | `sync` | No | No | `400` invalid request |
| `control_capable` | absent / `async` | No | Yes | `202` Hitch metadata JSON |
| `control_capable` | `sync` | Yes | Yes, after control dispatch | `200` harness-native JSON body |

## File Structure

Create:

- No new Go packages. Add small types/helpers to existing protocol/harness/API files.

Modify:

- `internal/protocol/protocol.go`: add request dispatch mode constants if protocol-level types are preferred.
- `internal/harness/harness.go`: add source event capability type and `Capability(sourceEventType string)` to `Normalizer`.
- `internal/harness/codex/codex.go`: classify Codex return-capable events as control-capable.
- `internal/harness/hermes/hermes.go`: classify Hermes return-capable events as control-capable.
- `internal/harness/pi/pi.go`: classify Pi source events as control-capable or observer-only.
- `internal/harness/omp/omp.go`: delegate OMP capability to Pi-compatible classifier with OMP-specific events included.
- `internal/harness/opencode/opencode.go`: classify OpenCode typed hooks as control-capable and SDK events as observer-only.
- `internal/config/config.go`: rename handler config field from `Mode`/`mode` to `Kind`/`kind`; validate `observer` and `control`.
- `internal/config/default.config.toml`: update sample handler-key expectations only if handler examples exist in defaults.
- `internal/config/config_test.go`: update validation tests for `kind`; add rejection of stale `mode` via unknown-key behavior.
- `internal/dispatch/dispatch.go`: filter handlers by kind and persist/log kind.
- `internal/dispatch/dispatch_test.go`: update async/sync handler terminology and tests.
- `internal/store/store.go`: rename handler invocation schema field from `mode` to `kind`; bump schema version.
- `internal/store/store_test.go`: update persisted inspection expectations.
- `internal/api/server.go`: remove `/v1/dispatch-sync`, add request `mode`, branch inside `/v1/events`, return native body directly for sync.
- `internal/api/server_test.go`: rewrite sync dispatch tests to call `/v1/events` with `mode:"sync"` and parse native body.
- `internal/api/client.go`: send `mode` on `EventRequest`; remove or rewrite dispatch wrapper.
- `internal/clientshim/clientshim.go`: always post to `/v1/events`; include request `mode`; treat sync body directly as native response.
- `internal/clientshim/clientshim_test.go`: update endpoint assertions and sync fail-open tests.
- `cmd/hitch-client/main.go`, `cmd/hitch-client/help.go`, `cmd/hitch-client/main_test.go`: preserve `-sync` CLI flag but document that it sets request `mode:"sync"`.
- `cmd/hitch/main.go`, `cmd/hitch/main_test.go`: update replay handler-kind fields and sample configs.
- `internal/install/install.go`, `internal/install/install_test.go`: update generated Pi/OMP/OpenCode adapters/plugins to post sync calls to `/v1/events` with `mode:"sync"`.
- `examples/test_drive.py`, `examples/test_payload_logger.py`, `examples/*.config.toml`: update endpoint and `kind` config.
- `README.md`, `docs/events.md`, `docs/handler-development.md`, `docs/harness-contracts.md`, `docs/installation.md`, `docs/walkthrough.md`, `docs/configuration.md`, `docs/index.html`, `docs/docs/latest/index.html`: update docs from two endpoints/handler mode to unified endpoint/request mode/handler kind.

---

### Task 1: Add request mode and handler kind config

**Files:**

- Modify: `internal/api/server.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/dispatch/dispatch.go`
- Modify: `internal/dispatch/dispatch_test.go`

- [ ] **Step 1: Write failing config tests for handler kind**

In `internal/config/config_test.go`, update existing handler-mode validation tests to use `kind`. Add this test if no equivalent exists:

```go
func TestHandlerKindValidation(t *testing.T) {
	base := DefaultConfigTOML + `
[handlers.audit]
command = ["/bin/echo", "ok"]
events = ["*"]
kind = "observer"
timeout_ms = 1000
`
	if _, err := Parse([]byte(base)); err != nil {
		t.Fatalf("observer handler kind should parse: %v", err)
	}

	control := strings.Replace(base, `kind = "observer"`, `kind = "control"`, 1)
	if _, err := Parse([]byte(control)); err != nil {
		t.Fatalf("control handler kind should parse: %v", err)
	}

	invalid := strings.Replace(base, `kind = "observer"`, `kind = "sync"`, 1)
	if _, err := Parse([]byte(invalid)); err == nil || !strings.Contains(err.Error(), "handlers.audit.kind must be observer or control") {
		t.Fatalf("expected invalid kind error, got %v", err)
	}
}

func TestHandlerModeKeyIsRejected(t *testing.T) {
	content := DefaultConfigTOML + `
[handlers.audit]
command = ["/bin/echo", "ok"]
events = ["*"]
mode = "async"
timeout_ms = 1000
`
	if _, err := Parse([]byte(content)); err == nil || !strings.Contains(err.Error(), "unknown config keys") {
		t.Fatalf("expected stale mode key to be rejected, got %v", err)
	}
}
```

Run:

```sh
go test ./internal/config
```

Expected: FAIL because `HandlerConfig` still uses `Mode` and validates `mode`.

- [ ] **Step 2: Rename handler config field**

In `internal/config/config.go`, change `HandlerConfig`:

```go
type HandlerConfig struct {
	Command    []string `toml:"command"`
	WorkingDir string   `toml:"working_dir"`
	Events     []string `toml:"events"`
	Kind       string   `toml:"kind"`
	TimeoutMS  int      `toml:"timeout_ms"`
	OnError    string   `toml:"on_error"`
	OnTimeout  string   `toml:"on_timeout"`
}
```

Replace handler mode validation:

```go
switch h.Kind {
case "observer", "control":
default:
	return fmt.Errorf("handlers.%s.kind must be observer or control", name)
}
```

Run:

```sh
go test ./internal/config
```

Expected: PASS.

- [ ] **Step 3: Rename dispatch filtering from mode to kind**

In `internal/dispatch/dispatch.go`, rename `Invocation.Mode` to `Invocation.Kind`, update `Dispatch` parameter name from `mode` to `kind`, and change `matchHandlers`:

```go
func (r Runner) Dispatch(ctx context.Context, env protocol.EventEnvelope, kind string, totalDeadline time.Duration) Result {
	selected := r.matchHandlers(env.HitchEventType, kind)
	// existing body
}

func (r Runner) matchHandlers(event protocol.EventType, kind string) []string {
	names := make([]string, 0, len(r.Handlers))
	for name, h := range r.Handlers {
		if h.Kind != kind {
			continue
		}
		for _, e := range h.Events {
			if e == "*" || e == string(event) {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	return names
}
```

Update log attributes to emit `kind` instead of `mode`:

```go
"kind", inv.Kind,
```

Run:

```sh
go test ./internal/dispatch
```

Expected: FAIL until tests are updated from `Mode`/`sync`/`async` to `Kind`/`control`/`observer`.

- [ ] **Step 4: Update dispatch tests**

In `internal/dispatch/dispatch_test.go`, replace handler config fields:

```go
Kind: "observer"
Kind: "control"
```

Replace dispatch calls:

```go
runner.Dispatch(ctx, env, "observer", 0)
runner.Dispatch(ctx, env, "control", 2*time.Second)
```

Replace invocation assertions from `inv.Mode` to `inv.Kind`.

Run:

```sh
go test ./internal/dispatch
```

Expected: PASS.

---

### Task 2: Add source event capability to harness mappers

**Files:**

- Modify: `internal/harness/harness.go`
- Modify: `internal/harness/codex/codex.go`
- Modify: `internal/harness/hermes/hermes.go`
- Modify: `internal/harness/pi/pi.go`
- Modify: `internal/harness/omp/omp.go`
- Modify: `internal/harness/opencode/opencode.go`
- Modify: `internal/harness/*/*_test.go`

- [ ] **Step 1: Add capability interface**

In `internal/harness/harness.go`, add:

```go
type SourceEventCapability string

const (
	CapabilityObserverOnly   SourceEventCapability = "observer_only"
	CapabilityControlCapable SourceEventCapability = "control_capable"
)

type Normalizer interface {
	Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error)
	Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error)
	Capability(sourceEventType string) SourceEventCapability
}

func CapabilityFromSet(sourceEventType string, controlCapable map[string]struct{}) SourceEventCapability {
	if _, ok := controlCapable[sourceEventType]; ok {
		return CapabilityControlCapable
	}
	return CapabilityObserverOnly
}
```

Run:

```sh
go test ./internal/harness/...
```

Expected: FAIL because mapper types do not implement `Capability` yet.

- [ ] **Step 2: Implement Codex capability**

In `internal/harness/codex/codex.go`, add:

```go
var controlCapableEvents = map[string]struct{}{
	"SessionStart":       {},
	"SubagentStart":      {},
	"UserPromptSubmit":   {},
	"PreToolUse":         {},
	"PermissionRequest":  {},
	"PostToolUse":        {},
	"PreCompact":         {},
	"PostCompact":        {},
	"SubagentStop":       {},
	"Stop":               {},
}

func (Mapper) Capability(sourceEventType string) harness.SourceEventCapability {
	return harness.CapabilityFromSet(sourceEventType, controlCapableEvents)
}
```

Add/adjust `internal/harness/codex/codex_test.go`:

```go
func TestCapabilityClassifiesCodexSourceEvents(t *testing.T) {
	if got := Mapper{}.Capability("PreToolUse"); got != harness.CapabilityControlCapable {
		t.Fatalf("PreToolUse capability = %s", got)
	}
	if got := Mapper{}.Capability("UnknownObserver"); got != harness.CapabilityObserverOnly {
		t.Fatalf("unknown capability = %s", got)
	}
}
```

- [ ] **Step 3: Implement Hermes capability**

In `internal/harness/hermes/hermes.go`, add:

```go
var controlCapableEvents = map[string]struct{}{
	"pre_tool_call":             {},
	"pre_llm_call":              {},
	"transform_tool_result":     {},
	"transform_terminal_output": {},
	"transform_llm_output":      {},
	"pre_gateway_dispatch":      {},
}

func (Mapper) Capability(sourceEventType string) harness.SourceEventCapability {
	return harness.CapabilityFromSet(sourceEventType, controlCapableEvents)
}
```

Add/adjust `internal/harness/hermes/hermes_test.go`:

```go
func TestCapabilityClassifiesHermesSourceEvents(t *testing.T) {
	if got := Mapper{}.Capability("pre_tool_call"); got != harness.CapabilityControlCapable {
		t.Fatalf("pre_tool_call capability = %s", got)
	}
	if got := Mapper{}.Capability("on_session_end"); got != harness.CapabilityObserverOnly {
		t.Fatalf("on_session_end capability = %s", got)
	}
}
```

- [ ] **Step 4: Implement Pi and OMP capability**

In `internal/harness/pi/pi.go`, add:

```go
var controlCapableEvents = map[string]struct{}{
	"input":                  {},
	"tool_call":              {},
	"tool_result":            {},
	"context":                {},
	"before_provider_request": {},
	"user_bash":              {},
	"user_python":            {},
	"session_before_switch":  {},
	"session_before_fork":    {},
	"session_before_branch":  {},
	"session_before_compact": {},
	"session_before_tree":    {},
	"session.compacting":     {},
}

func (Mapper) Capability(sourceEventType string) harness.SourceEventCapability {
	return CapabilityForHarness(sourceEventType)
}

func CapabilityForHarness(sourceEventType string) harness.SourceEventCapability {
	return harness.CapabilityFromSet(sourceEventType, controlCapableEvents)
}
```

In `internal/harness/omp/omp.go`, add:

```go
func (Mapper) Capability(sourceEventType string) harness.SourceEventCapability {
	return piharness.CapabilityForHarness(sourceEventType)
}
```

Add tests in `internal/harness/pi/pi_test.go` and `internal/harness/omp/omp_test.go` covering `tool_call` as control-capable and `turn_end` as observer-only.

- [ ] **Step 5: Implement OpenCode capability**

In `internal/harness/opencode/opencode.go`, add:

```go
var controlCapableEvents = map[string]struct{}{
	"chat.message":                         {},
	"tool.execute.before":                  {},
	"tool.execute.after":                   {},
	"permission.ask":                       {},
	"command.execute.before":               {},
	"chat.params":                          {},
	"chat.headers":                         {},
	"shell.env":                            {},
	"tool.definition":                      {},
	"experimental.session.compacting":      {},
	"experimental.compaction.autocontinue": {},
	"experimental.text.complete":           {},
}

func (Mapper) Capability(sourceEventType string) harness.SourceEventCapability {
	return harness.CapabilityFromSet(sourceEventType, controlCapableEvents)
}
```

Add/adjust `internal/harness/opencode/opencode_test.go`:

```go
func TestCapabilityClassifiesOpenCodeSourceEvents(t *testing.T) {
	if got := Mapper{}.Capability("tool.execute.before"); got != harness.CapabilityControlCapable {
		t.Fatalf("tool.execute.before capability = %s", got)
	}
	if got := Mapper{}.Capability("session.idle"); got != harness.CapabilityObserverOnly {
		t.Fatalf("session.idle capability = %s", got)
	}
}
```

Run:

```sh
go test ./internal/harness/...
```

Expected: PASS.

---

### Task 3: Unify server endpoint behavior

**Files:**

- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Write failing API tests for unified endpoint**

In `internal/api/server_test.go`, replace `TestDispatchSync` with a test that posts to `/v1/events` and expects native JSON directly:

```go
func TestEventSyncModeReturnsNativeResponseBody(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := New(testConfig(), slog.Default(), st)
	body := []byte(`{"mode":"sync","harness":"hermes","source_event_type":"pre_tool_call","source_payload":{"tool_name":"bash"},"hitch_client_version":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync event code %d body %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Hitch-Normalized-Event-ID") == "" {
		t.Fatalf("missing normalized event header")
	}
	var native map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &native); err != nil {
		t.Fatal(err)
	}
	if len(native) != 0 {
		t.Fatalf("default sync response should be native passthrough object, got %#v", native)
	}
}
```

Add invalid mode and observer-only sync rejection tests:

```go
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
```

Run:

```sh
go test ./internal/api
```

Expected: FAIL because `/v1/events` ignores `mode`, `/v1/dispatch-sync` still exists, and sync responses are wrapped.

- [ ] **Step 2: Add request mode parsing and route removal**

In `internal/api/server.go`, change `EventRequest`:

```go
type EventRequest struct {
	Mode               string           `json:"mode"`
	Harness            string           `json:"harness"`
	SourceEventType    string           `json:"source_event_type"`
	SourcePayload      protocol.RawJSON `json:"source_payload"`
	HitchClientVersion string           `json:"hitch_client_version"`
}
```

Add constants/helpers near request types:

```go
const (
	requestModeAsync = "async"
	requestModeSync  = "sync"
)

func normalizeRequestMode(mode string) (string, error) {
	switch strings.TrimSpace(mode) {
	case "", requestModeAsync:
		return requestModeAsync, nil
	case requestModeSync:
		return requestModeSync, nil
	default:
		return "", badRequest("mode must be async or sync")
	}
}
```

Remove this route from `New`:

```go
mux.HandleFunc("POST /v1/dispatch-sync", s.handleDispatchSync)
```

- [ ] **Step 3: Branch sync and async inside `handleEvent`**

Change `handleEvent` to decode once through a new helper. One minimal shape:

```go
func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	resp, env, mode, err := s.ingest(r.Context(), r)
	if err != nil {
		writeError(w, err)
		return
	}
	if mode == requestModeAsync {
		go s.dispatchObservers(context.Background(), resp.NormalizedEventID, env)
		writeJSON(w, http.StatusAccepted, resp)
		return
	}
	s.handleSyncEvent(w, r, resp, env)
}
```

Replace old `handleDispatchSync` with:

```go
func (s *Server) handleSyncEvent(w http.ResponseWriter, r *http.Request, resp EventResponse, env protocol.EventEnvelope) {
	result := s.runner.Dispatch(r.Context(), env, "control", 2*time.Second)
	for _, inv := range result.Invocations {
		_ = s.store.InsertHandlerInvocation(r.Context(), store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: resp.NormalizedEventID, HandlerName: inv.HandlerName, Kind: inv.Kind, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error})
	}
	go s.dispatchObservers(context.Background(), resp.NormalizedEventID, env)
	runtime := s.harnesses[env.Harness]
	native, err := runtime.normalizer.Translate(env.SourceEventType, result.Aggregate)
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.store.InsertNativeResponse(r.Context(), store.NativeResponse{ID: harness.NewID("nresp"), NormalizedEventID: resp.NormalizedEventID, Response: native, EmittedAt: time.Now().UTC()})
	w.Header().Set("X-Hitch-Event-ID", resp.EventID)
	w.Header().Set("X-Hitch-Normalized-Event-ID", resp.NormalizedEventID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(native)
}
```

Rename `dispatchAsync` to `dispatchObservers` and dispatch `observer`:

```go
func (s *Server) dispatchObservers(ctx context.Context, normalizedID string, env protocol.EventEnvelope) {
	result := s.runner.Dispatch(ctx, env, "observer", 0)
	// persist invocations with Kind
}
```

- [ ] **Step 4: Validate capability during ingest**

Refactor `ingest` to return mode and reject sync observer-only source events before persisting:

```go
func (s *Server) ingest(ctx context.Context, r *http.Request) (EventResponse, protocol.EventEnvelope, string, error) {
	var req EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, "", badRequest("invalid JSON: %v", err)
	}
	mode, err := normalizeRequestMode(req.Mode)
	if err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, "", err
	}
	// existing harness/source/payload validation
	if mode == requestModeSync && runtime.normalizer.Capability(req.SourceEventType) != harness.CapabilityControlCapable {
		return EventResponse{}, protocol.EventEnvelope{}, "", badRequest("%s event %q does not support sync dispatch", req.Harness, req.SourceEventType)
	}
	// existing normalize and persistence
	return EventResponse{EventID: env.EventID, NormalizedEventID: normalizedID}, env, mode, nil
}
```

Run:

```sh
go test ./internal/api
```

Expected: PASS after updating remaining tests in the file from `/v1/dispatch-sync` and `DispatchResponse` to `/v1/events` with `mode:"sync"` and native-body parsing.

---

### Task 4: Update store, replay, and API client structures

**Files:**

- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`
- Modify: `cmd/hitch/main.go`
- Modify: `cmd/hitch/main_test.go`
- Modify: `internal/api/client.go`

- [ ] **Step 1: Rename persisted handler invocation field**

In `internal/store/store.go`:

- bump `schemaVersion` by 1;
- change `handler_invocations.mode TEXT NOT NULL` to `kind TEXT NOT NULL`;
- rename `HandlerInvocation.Mode` to `HandlerInvocation.Kind` with JSON key `kind`;
- update insert/select SQL to use `kind`.

Expected struct field:

```go
type HandlerInvocation struct {
	ID                string                 `json:"id"`
	NormalizedEventID string                 `json:"normalized_event_id"`
	HandlerName       string                 `json:"handler_name"`
	Kind              string                 `json:"kind"`
	StartedAt         time.Time              `json:"started_at"`
	CompletedAt       time.Time              `json:"completed_at,omitempty"`
	Status            protocol.HandlerStatus `json:"status"`
	Stdout            string                 `json:"stdout,omitempty"`
	Stderr            string                 `json:"stderr,omitempty"`
	Output            protocol.RawJSON       `json:"output,omitempty"`
	Decision          protocol.RawJSON       `json:"decision,omitempty"`
	Error             string                 `json:"error,omitempty"`
	ReplaySourceID    string                 `json:"replay_source_id,omitempty"`
}
```

Run:

```sh
go test ./internal/store
```

Expected: FAIL until tests are updated from mode to kind.

- [ ] **Step 2: Update store tests and replay command**

In `internal/store/store_test.go`, replace `Mode: "sync"` with `Kind: "control"` and JSON assertions from `mode` to `kind`.

In `cmd/hitch/main.go`, update replay insertion:

```go
Kind: inv.Kind,
```

In `cmd/hitch/main_test.go`, replace inserted handler invocations and sample configs:

```go
Kind: "control"
kind = "control"
```

Run:

```sh
go test ./internal/store ./cmd/hitch
```

Expected: PASS after all replay/config references are updated.

- [ ] **Step 3: Update API client helper**

In `internal/api/client.go`, remove `DispatchResponse` usage or make `Dispatch` return raw native JSON. Minimal clean shape:

```go
func (c Client) Event(req EventRequest) (EventResponse, error) {
	req.Mode = requestModeAsync
	var out EventResponse
	err := c.post("/v1/events", req, &out)
	return out, err
}

func (c Client) Dispatch(req EventRequest) (protocol.RawJSON, error) {
	req.Mode = requestModeSync
	return c.postRaw("/v1/events", req)
}
```

Add `postRaw` that returns response bytes on 2xx and errors on non-2xx.

Run:

```sh
go test ./internal/api
```

Expected: PASS.

---

### Task 5: Update hitch-client fail-open sync behavior

**Files:**

- Modify: `internal/clientshim/clientshim.go`
- Modify: `internal/clientshim/clientshim_test.go`
- Modify: `cmd/hitch-client/help.go`
- Modify: `cmd/hitch-client/main_test.go`

- [ ] **Step 1: Write failing client shim tests**

In `internal/clientshim/clientshim_test.go`, update `TestRunSyncDispatchPrintsNativeResponse` to expect `/v1/events`, `mode:"sync"`, and direct native body:

```go
func TestRunSyncDispatchPrintsNativeResponse(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"action":"block","message":"policy"}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	if err := Run(context.Background(), Options{Harness: "hermes", Event: "pre_tool_call", Sync: true, URL: server.URL, Stdin: strings.NewReader(`{"name":"Bash"}`), Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	if got["mode"] != "sync" {
		t.Fatalf("sync request did not set mode: %#v", got)
	}
	if strings.TrimSpace(stdout.String()) != `{"action":"block","message":"policy"}` {
		t.Fatalf("unexpected sync stdout: %q", stdout.String())
	}
}
```

Update async tests to assert missing or `async` mode. Prefer explicit `mode:"async"` for clarity.

Run:

```sh
go test ./internal/clientshim
```

Expected: FAIL because client still posts `/v1/dispatch-sync` and expects `native_response` wrapper.

- [ ] **Step 2: Change shim request and raw response handling**

In `internal/clientshim/clientshim.go`, add `Mode` to request:

```go
type eventRequest struct {
	Mode               string           `json:"mode"`
	Harness            string           `json:"harness"`
	SourceEventType    string           `json:"source_event_type"`
	SourcePayload      protocol.RawJSON `json:"source_payload"`
	HitchClientVersion string           `json:"hitch_client_version"`
}
```

Set mode before posting:

```go
mode := "async"
if opts.Sync {
	mode = "sync"
}
req := eventRequest{Mode: mode, Harness: opts.Harness, SourceEventType: opts.Event, SourcePayload: protocol.RawJSON(payload), HitchClientVersion: protocol.Version}
```

For async:

```go
var resp eventResponse
_ = client.post(ctx, "/v1/events", req, &resp)
return nil
```

For sync:

```go
native, err := client.postRaw(ctx, "/v1/events", req)
if err != nil || len(bytes.TrimSpace(native)) == 0 || !json.Valid(native) {
	native = NativeNoop(opts.Harness, opts.Event)
}
```

Add `postRaw` to `httpClient`:

```go
func (c httpClient) postRaw(ctx context.Context, path string, req eventRequest) ([]byte, error) {
	h := c.http
	if h == nil {
		h = &http.Client{Timeout: 2 * time.Second}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := h.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hitch API %s: %s", resp.Status, string(body))
	}
	return body, nil
}
```

Run:

```sh
go test ./internal/clientshim ./cmd/hitch-client
```

Expected: PASS after help text/tests are updated.

---

### Task 6: Update generated adapters and examples

**Files:**

- Modify: `internal/install/install.go`
- Modify: `internal/install/install_test.go`
- Modify: `examples/test_drive.py`
- Modify: `examples/test_payload_logger.py`
- Modify: `examples/test-drive.config.toml`
- Modify: `examples/payload-logger.config.toml`

- [ ] **Step 1: Update generated adapter tests first**

In `internal/install/install_test.go`, replace assertions that generated Pi/OMP/OpenCode files contain `/v1/dispatch-sync`. Assert they contain:

```text
/v1/events
mode: "sync"
mode: "async"
```

For generated TypeScript objects, assert sync posts include a `mode: "sync"` field and observer posts include `mode: "async"`.

Run:

```sh
go test ./internal/install
```

Expected: FAIL because generated adapters still use `/v1/dispatch-sync`.

- [ ] **Step 2: Update generated TypeScript fetch calls**

In `internal/install/install.go`, update generated Pi/OMP/OpenCode sync fetch calls from:

```ts
await post('/v1/dispatch-sync', {
  harness: 'opencode',
  source_event_type: sourceEventType,
  source_payload: payload,
  hitch_client_version: HITCH_CLIENT_VERSION,
})
```

to:

```ts
await post('/v1/events', {
  mode: 'sync',
  harness: 'opencode',
  source_event_type: sourceEventType,
  source_payload: payload,
  hitch_client_version: HITCH_CLIENT_VERSION,
})
```

Update observer calls similarly with `mode: 'async'`.

Run:

```sh
go test ./internal/install
```

Expected: PASS.

- [ ] **Step 3: Update examples**

In `examples/test_drive.py` and `examples/test_payload_logger.py`, change sync request path to `/v1/events` and add `"mode": "sync"` to sync request JSON.

In `examples/test-drive.config.toml`:

```toml
kind = "control"
```

In `examples/payload-logger.config.toml`:

```toml
kind = "control"
kind = "observer"
```

Run:

```sh
go test ./cmd/hitch ./internal/config
python3 examples/test_drive.py
python3 examples/test_payload_logger.py
```

Expected: Go tests PASS. Python examples PASS when Hitch can start locally from the example scripts.

---

### Task 7: Update docs and generated docs HTML

**Files:**

- Modify: `README.md`
- Modify: `docs/events.md`
- Modify: `docs/handler-development.md`
- Modify: `docs/harness-contracts.md`
- Modify: `docs/installation.md`
- Modify: `docs/walkthrough.md`
- Modify: `docs/configuration.md`
- Modify: `docs/index.html`
- Modify: `docs/docs/latest/index.html`

- [ ] **Step 1: Update endpoint references**

Replace public endpoint lists with:

```text
GET  /v1/health
POST /v1/events
GET  /v1/events/:id
POST /v1/replay
```

Replace sync curl examples with:

```sh
curl -sS -X POST http://127.0.0.1:8799/v1/events \
  -H 'content-type: application/json' \
  -d '{
    "mode": "sync",
    "harness": "codex",
    "source_event_type": "PreToolUse",
    "source_payload": {
      "hook_event_name": "PreToolUse",
      "tool_name": "Bash",
      "tool_input": {"command":"pwd"}
    },
    "hitch_client_version": "manual"
  }'
```

- [ ] **Step 2: Update handler terminology**

In all docs and examples, replace handler config terminology:

```toml
kind = "observer"
kind = "control"
```

Use this explanation in `docs/handler-development.md`:

```md
Hitch separates three concepts:

- Request `mode`: top-level HTTP request field, `async` or `sync`; missing defaults to `async`.
- Handler `kind`: config field, `observer` or `control`.
- Source event capability: harness source-event contract, `observer_only` or `control_capable`.
```

Include the ADR dispatch matrix.

- [ ] **Step 3: Update harness contract wording**

Replace references to Pi/OMP/OpenCode posting return-capable callbacks to `/v1/dispatch-sync` with:

```md
Generated adapters post all callbacks to `/v1/events`. Observer callbacks send `mode:"async"`; return-capable callbacks send `mode:"sync"` and consume the response body as harness-native JSON.
```

- [ ] **Step 4: Verify no stale endpoint or handler-mode docs remain**

Use the search tool, not shell search, to verify these strings are gone from user-facing docs and implementation files except historical ADR/plan references:

```text
/v1/dispatch-sync
mode = "sync"
mode = "async"
```

Allowed references:

- `docs/adr/0001-hitch-architecture-and-technology-stack.md` as historical context.
- `docs/adr/0002-unified-events-endpoint-and-dispatch-semantics.md` as rationale.
- this implementation plan.

Run:

```sh
go test ./...
```

Expected: PASS.

---

### Task 8: Final verification

**Files:**

- All modified files above.

- [ ] **Step 1: Focused Go tests**

Run:

```sh
go test ./internal/config ./internal/harness/... ./internal/dispatch ./internal/store ./internal/api ./internal/clientshim ./internal/install ./cmd/hitch ./cmd/hitch-client
```

Expected: PASS.

- [ ] **Step 2: Full Go test suite**

Run:

```sh
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Example smoke tests**

Run:

```sh
python3 examples/test_drive.py
python3 examples/test_payload_logger.py
```

Expected: PASS. If a Python smoke test fails because it cannot start a local Hitch server, fix the integration break rather than skipping the test.

- [ ] **Step 4: Manual sync body check**

Start Hitch with an example config and send a sync event:

```sh
go run ./cmd/hitch serve --config examples/test-drive.config.toml
```

In another terminal:

```sh
curl -i -sS -X POST http://127.0.0.1:8799/v1/events \
  -H 'content-type: application/json' \
  -d '{"mode":"sync","harness":"hermes","source_event_type":"pre_tool_call","source_payload":{"tool_name":"bash"},"hitch_client_version":"manual"}'
```

Expected:

- HTTP status `200`.
- Response has `X-Hitch-Event-ID` and `X-Hitch-Normalized-Event-ID` headers.
- Body is harness-native JSON, not a Hitch wrapper; for default pass-through it is `{}`.

## Self-Review

- ADR-0002 requirements are covered: unified endpoint, request `mode`, handler `kind`, source event capability, sync native body, fail-open client behavior, removal of `/v1/dispatch-sync`.
- No compatibility branch is planned for `/v1/dispatch-sync`; clean cutover is intentional.
- Tests are behavior-focused: invalid mode, observer-only sync rejection, native body response, async observer after sync, and client fail-open parsing.
- Storage rename is included because Hitch is still in development and schema reset behavior already exists.
