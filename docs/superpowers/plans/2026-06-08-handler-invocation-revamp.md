# Handler Invocation Revamp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add source-aware handler routing, payload selection, native Python extensions through a Hitch SDK, and per-inbound-event handler dedupe.

**Architecture:** Keep Hitch's Go daemon as the router, auditor, and decision aggregator. Shell and native handlers both receive a versioned invocation context; native handlers run as isolated Python subprocesses through a small SDK runner. Store-level reservation enforces dedupe before execution.

**Tech Stack:** Go 1.24, TOML config parsing, SQLite audit store, Python 3 standard library SDK, existing Hitch HTTP and dispatch packages.

---

## File structure

- Modify `internal/config/config.go`: add handler type, payload mode, Hitch-event aliases, source-event filters, extension discovery merge, and validation.
- Create `internal/config/extensions.go`: discover `~/.config/hitch/extensions/<name>/config.toml`, validate native extension entries, and merge them into handler config.
- Modify `internal/config/config_test.go`: cover config compatibility, filters, payload mode, and extension discovery failures.
- Modify `internal/protocol/protocol.go`: add skipped status and invocation context structs used by shell and native handlers.
- Modify `internal/store/store.go`: add handler invocation `inbound_event_id` and `hook_key`, reservation/complete APIs, and schema version bump.
- Modify `internal/store/store_test.go`: cover reservation uniqueness and skipped rows.
- Modify `internal/dispatch/dispatch.go`: add source filtering, payload selection, invocation context construction, shell arg passing, native subprocess invocation, and skipped aggregation behavior.
- Modify `internal/dispatch/dispatch_test.go`: cover matching, payload args/stdin, native invocation, and skipped aggregation.
- Modify `internal/api/server.go`: pass inbound and normalized IDs into dispatch and use reservation-backed persistence.
- Modify `internal/api/server_test.go`: cover end-to-end dedupe and source-event filtering.
- Modify `internal/app/replay.go`: use replay-scoped dispatch request so replay can run handlers deliberately.
- Create `sdk/python/hitch_sdk/__init__.py`: Python dataclasses, result helpers, JSON serialization, and SDK runner.
- Create `examples/extensions/audit_logger/config.toml` and `examples/extensions/audit_logger/handler.py`: runnable native extension example.
- Update docs after behavior is tested: `docs/configuration.md`, `docs/handler-development.md`, `docs/handler-protocol.md`.

---

### Task 1: Config shape and validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Append these tests to `internal/config/config_test.go`:

```go
func TestParseHandlerConfigSupportsNewRoutingFields(t *testing.T) {
	text := baseConfig + `
[handlers.native_audit]
type = "native"
extension = "audit_logger"
hitch_events = ["tool.completed"]
source_events = [{ harness = "codex", source_event_type = "PostToolUse" }]
payload = "source"
kind = "observer"
timeout_ms = 500
`
	cfg, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	h := cfg.Handlers["native_audit"]
	if h.Type != "native" || h.Extension != "audit_logger" || h.Payload != "source" {
		t.Fatalf("new handler fields not parsed: %#v", h)
	}
	if len(h.HitchEvents) != 1 || h.HitchEvents[0] != "tool.completed" {
		t.Fatalf("hitch_events not parsed: %#v", h.HitchEvents)
	}
	if len(h.SourceEvents) != 1 || h.SourceEvents[0].Harness != "codex" || h.SourceEvents[0].SourceEventType != "PostToolUse" {
		t.Fatalf("source_events not parsed: %#v", h.SourceEvents)
	}
}

func TestParseHandlerConfigKeepsEventsAlias(t *testing.T) {
	cfg, err := Parse([]byte(baseConfig))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Handlers["security_gate"].HitchEvents; len(got) != 2 || got[0] != "tool.requested" || got[1] != "tool.permission_requested" {
		t.Fatalf("events alias was not copied to hitch_events: %#v", got)
	}
	if got := cfg.Handlers["security_gate"].Type; got != "shell" {
		t.Fatalf("default handler type = %q", got)
	}
	if got := cfg.Handlers["security_gate"].Payload; got != "hitch" {
		t.Fatalf("default payload = %q", got)
	}
}

func TestParseRejectsConflictingEventsAlias(t *testing.T) {
	bad := baseConfig + `
[handlers.conflict]
command = ["x"]
events = ["tool.requested"]
hitch_events = ["tool.completed"]
kind = "observer"
timeout_ms = 1
`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("conflicting events aliases accepted")
	}
}

func TestParseRejectsInvalidHandlerTypePayloadAndSourceFilter(t *testing.T) {
	cases := []string{
		`type = "worker"
command = ["x"]
hitch_events = ["tool.requested"]
kind = "observer"
timeout_ms = 1`,
		`type = "shell"
command = ["x"]
hitch_events = ["tool.requested"]
payload = "both"
kind = "observer"
timeout_ms = 1`,
		`type = "shell"
command = ["x"]
hitch_events = ["tool.requested"]
source_events = [{ harness = "unknown", source_event_type = "PreToolUse" }]
kind = "observer"
timeout_ms = 1`,
		`type = "shell"
command = ["x"]
hitch_events = ["tool.requested"]
source_events = [{ harness = "codex", source_event_type = "" }]
kind = "observer"
timeout_ms = 1`,
	}
	for i, handler := range cases {
		bad := baseConfig + "\n[handlers.bad_" + strconv.Itoa(i) + "]\n" + handler + "\n"
		if _, err := Parse([]byte(bad)); err == nil {
			t.Fatalf("invalid handler case %d accepted", i)
		}
	}
}
```

Add `strconv` to the test imports.

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
go test ./internal/config
```

Expected: FAIL because `HandlerConfig` has no `Type`, `Extension`, `HitchEvents`, `SourceEvents`, or `Payload` fields.

- [ ] **Step 3: Add config fields and normalization**

In `internal/config/config.go`, replace `HandlerConfig` with:

```go
type SourceEventFilter struct {
	Harness         string `toml:"harness"`
	SourceEventType string `toml:"source_event_type"`
}

type HandlerConfig struct {
	Type         string              `toml:"type"`
	Extension    string              `toml:"extension"`
	Command      []string            `toml:"command"`
	WorkingDir   string              `toml:"working_dir"`
	Events       []string            `toml:"events"`
	HitchEvents  []string            `toml:"hitch_events"`
	SourceEvents []SourceEventFilter `toml:"source_events"`
	Payload      string              `toml:"payload"`
	Kind         string              `toml:"kind"`
	TimeoutMS    int                 `toml:"timeout_ms"`
	OnError      string              `toml:"on_error"`
	OnTimeout    string              `toml:"on_timeout"`
}
```

Add this helper after `upgradeLegacyDefaultEventMaps`:

```go
func normalizeHandlerConfig(name string, h *HandlerConfig) error {
	if h.Type == "" {
		h.Type = "shell"
	}
	if h.Payload == "" {
		h.Payload = "hitch"
	}
	if len(h.Events) != 0 && len(h.HitchEvents) != 0 && !stringSlicesEqual(h.Events, h.HitchEvents) {
		return fmt.Errorf("handlers.%s cannot set conflicting events and hitch_events", name)
	}
	if len(h.HitchEvents) == 0 {
		h.HitchEvents = append([]string(nil), h.Events...)
	}
	return nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Call it from `Parse` after `c.Handlers` is initialized and before `c.Validate()`:

```go
for name, h := range c.Handlers {
	if err := normalizeHandlerConfig(name, &h); err != nil {
		return Config{}, err
	}
	c.Handlers[name] = h
}
```

- [ ] **Step 4: Update validation**

In `Validate`, replace handler command/event validation with checks against the canonical fields:

```go
switch h.Type {
case "shell":
	if len(h.Command) == 0 || h.Command[0] == "" {
		return fmt.Errorf("handlers.%s.command is required", name)
	}
case "native":
	if h.Extension == "" {
		return fmt.Errorf("handlers.%s.extension is required for native handlers", name)
	}
default:
	return fmt.Errorf("handlers.%s.type must be shell or native", name)
}
if len(h.HitchEvents) == 0 {
	return fmt.Errorf("handlers.%s.hitch_events is required", name)
}
for _, e := range h.HitchEvents {
	if e != "*" && !protocol.IsValidEventType(protocol.EventType(e)) {
		return fmt.Errorf("handlers.%s references unknown event %q", name, e)
	}
}
switch h.Payload {
case "hitch", "source":
default:
	return fmt.Errorf("handlers.%s.payload must be hitch or source", name)
}
for _, f := range h.SourceEvents {
	if !protocol.IsValidHarness(protocol.Harness(f.Harness)) {
		return fmt.Errorf("handlers.%s source_events references unknown harness %q", name, f.Harness)
	}
	if f.SourceEventType == "" {
		return fmt.Errorf("handlers.%s source_events source_event_type is required", name)
	}
}
```

- [ ] **Step 5: Run config tests and commit**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

Commit:

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add handler routing config"
```

---

### Task 2: Native extension discovery

**Files:**
- Create: `internal/config/extensions.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing extension discovery tests**

Append to `internal/config/config_test.go`:

```go
func TestLoadWithExtensionDirDiscoversNativeExtension(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "audit_logger")
	if err := os.MkdirAll(ext, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ext, "config.toml"), []byte(`
name = "audit_logger"
entrypoint = "handler:handle"
kind = "observer"
hitch_events = ["tool.completed"]
payload = "hitch"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(baseConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithExtensionDir(configPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	h := cfg.Handlers["audit_logger"]
	if h.Type != "native" || h.Extension != "audit_logger" || h.Entrypoint != "handler:handle" {
		t.Fatalf("extension not discovered: %#v", h)
	}
}

func TestLoadWithExtensionDirRejectsInvalidExtension(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "broken")
	if err := os.MkdirAll(ext, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ext, "config.toml"), []byte(`
name = "broken"
kind = "observer"
hitch_events = ["tool.completed"]
timeout_ms = 1000
`), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(baseConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithExtensionDir(configPath, dir); err == nil {
		t.Fatal("invalid extension accepted")
	}
}
```

This test introduces `Entrypoint`; add it to `HandlerConfig` in this task.

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
go test ./internal/config
```

Expected: FAIL because `LoadWithExtensionDir`, `Entrypoint`, and extension discovery do not exist.

- [ ] **Step 3: Add extension discovery code**

Create `internal/config/extensions.go`:

```go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type extensionFile struct {
	Name         string              `toml:"name"`
	Entrypoint   string              `toml:"entrypoint"`
	Kind         string              `toml:"kind"`
	HitchEvents  []string            `toml:"hitch_events"`
	SourceEvents []SourceEventFilter `toml:"source_events"`
	Payload      string              `toml:"payload"`
	TimeoutMS    int                 `toml:"timeout_ms"`
	OnError      string              `toml:"on_error"`
	OnTimeout    string              `toml:"on_timeout"`
}

func DefaultExtensionDir() string {
	return ExpandHome("~/.config/hitch/extensions")
}

func LoadWithExtensionDir(path, extensionDir string) (Config, error) {
	cfg, err := loadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := mergeExtensions(&cfg, extensionDir); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func mergeExtensions(cfg *Config, root string) error {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(root, name, "config.toml")
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var ext extensionFile
		if err := toml.Unmarshal(b, &ext); err != nil {
			return fmt.Errorf("extension %s config: %w", name, err)
		}
		if ext.Name == "" {
			ext.Name = name
		}
		if ext.Name != name {
			return fmt.Errorf("extension %s config name %q must match directory", name, ext.Name)
		}
		h := HandlerConfig{
			Type:         "native",
			Extension:    ext.Name,
			Entrypoint:   ext.Entrypoint,
			Kind:         ext.Kind,
			HitchEvents:  append([]string(nil), ext.HitchEvents...),
			SourceEvents: append([]SourceEventFilter(nil), ext.SourceEvents...),
			Payload:      ext.Payload,
			TimeoutMS:    ext.TimeoutMS,
			OnError:      ext.OnError,
			OnTimeout:    ext.OnTimeout,
		}
		if err := normalizeHandlerConfig(ext.Name, &h); err != nil {
			return err
		}
		if h.Entrypoint == "" {
			return fmt.Errorf("extension %s entrypoint is required", ext.Name)
		}
		if _, exists := cfg.Handlers[ext.Name]; !exists {
			cfg.Handlers[ext.Name] = h
		}
	}
	return nil
}
```

Add `Entrypoint string `toml:"entrypoint"`` to `HandlerConfig`.

Split current `Load` so both normal and extension-aware loading can share file parsing:

```go
func Load(path string) (Config, error) {
	return LoadWithExtensionDir(path, DefaultExtensionDir())
}

func loadFile(path string) (Config, error) {
	path = ExpandHome(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	c, err := Parse(b)
	if err != nil {
		return Config{}, err
	}
	resolveHandlerWorkingDirs(&c, filepath.Dir(path))
	return c, nil
}
```

- [ ] **Step 4: Validate native entrypoint**

In `Validate`, inside the `case "native"` block, add:

```go
if h.Entrypoint == "" {
	return fmt.Errorf("handlers.%s.entrypoint is required for native handlers", name)
}
```

- [ ] **Step 5: Run config tests and commit**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

Commit:

```bash
git add internal/config/config.go internal/config/extensions.go internal/config/config_test.go
git commit -m "feat: discover native handler extensions"
```

---

### Task 3: Protocol context and store reservation

**Files:**
- Modify: `internal/protocol/protocol.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing store tests**

Append to `internal/store/store_test.go`:

```go
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
	reservation.ID = "handler_2"
	reserved, err = s.ReserveHandlerInvocation(ctx, reservation)
	if err != nil {
		t.Fatal(err)
	}
	if reserved {
		t.Fatal("duplicate reservation succeeded")
	}
}
```

- [ ] **Step 2: Run store tests and verify failure**

Run:

```bash
go test ./internal/store
```

Expected: FAIL because reservation APIs and fields do not exist.

- [ ] **Step 3: Add protocol structs and skipped status**

In `internal/protocol/protocol.go`, add `StatusSkipped`:

```go
const (
	StatusOK      HandlerStatus = "ok"
	StatusError   HandlerStatus = "error"
	StatusTimeout HandlerStatus = "timeout"
	StatusSkipped HandlerStatus = "skipped"
)
```

Add it to `validStatuses`.

Add context types after `EventEnvelope`:

```go
type InvocationEvent struct {
	HitchVersion    string    `json:"hitch_version"`
	EventID         string    `json:"event_id"`
	ReceivedAt      time.Time `json:"received_at"`
	Harness         Harness   `json:"harness"`
	SourceEventType string    `json:"source_event_type"`
	SourcePayload   RawJSON   `json:"source_payload"`
	HitchEventType  EventType `json:"hitch_event_type"`
	SessionID       string    `json:"session_id,omitempty"`
	TurnID          string    `json:"turn_id,omitempty"`
	CWD             string    `json:"cwd,omitempty"`
	Model           string    `json:"model,omitempty"`
	TranscriptPath  string    `json:"transcript_path,omitempty"`
	Payload         RawJSON   `json:"payload"`
}

type InvocationContext struct {
	HitchVersion      string          `json:"hitch_version"`
	HandlerName       string          `json:"handler_name"`
	HandlerType       string          `json:"handler_type"`
	Kind              string          `json:"kind"`
	InboundEventID    string          `json:"inbound_event_id"`
	NormalizedEventID string          `json:"normalized_event_id"`
	PayloadKind       string          `json:"payload_kind"`
	Payload           RawJSON         `json:"payload"`
	Event             InvocationEvent `json:"event"`
}
```

- [ ] **Step 4: Add store fields and reservation APIs**

Bump `schemaVersion` to `6`.

In the `handler_invocations` schema, add:

```sql
  inbound_event_id TEXT NOT NULL REFERENCES inbound_events(id),
  hook_key TEXT NOT NULL,
```

Add a migration after table creation:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_handler_invocations_dedupe
ON handler_invocations(inbound_event_id, handler_name, hook_key);
```

Update `HandlerInvocation`:

```go
InboundEventID string `json:"inbound_event_id"`
HookKey        string `json:"hook_key"`
```

Add reservation type and methods:

```go
type HandlerInvocationReservation struct {
	ID                string
	InboundEventID    string
	NormalizedEventID string
	HandlerName       string
	Kind              string
	HookKey           string
	StartedAt         time.Time
}

func (s *Store) ReserveHandlerInvocation(ctx context.Context, r HandlerInvocationReservation) (bool, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO handler_invocations(id, inbound_event_id, normalized_event_id, handler_name, kind, hook_key, started_at, status, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, r.InboundEventID, r.NormalizedEventID, r.HandlerName, r.Kind, r.HookKey, r.StartedAt.Format(time.RFC3339Nano), protocol.StatusSkipped, schemaVersion)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) CompleteHandlerInvocation(ctx context.Context, h HandlerInvocation) error {
	completed := ""
	if !h.CompletedAt.IsZero() {
		completed = h.CompletedAt.Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE handler_invocations SET completed_at = ?, status = ?, stdout = ?, stderr = ?, output_json = ?, decision_json = ?, error = ?, replay_source_id = ? WHERE id = ?`, completed, h.Status, h.Stdout, h.Stderr, string(h.Output), string(h.Decision), h.Error, h.ReplaySourceID, h.ID)
	return err
}
```

Update `InsertHandlerInvocation` insert statement and `InspectEvent` select/scan to include the new fields.

- [ ] **Step 5: Run store tests and commit**

Run:

```bash
go test ./internal/protocol ./internal/store
```

Expected: PASS.

Commit:

```bash
git add internal/protocol/protocol.go internal/store/store.go internal/store/store_test.go
git commit -m "feat: reserve handler invocations"
```

---

### Task 4: Dispatch filtering and shell invocation context

**Files:**
- Modify: `internal/dispatch/dispatch.go`
- Modify: `internal/dispatch/dispatch_test.go`

- [ ] **Step 1: Write failing dispatch tests**

Append to `internal/dispatch/dispatch_test.go`:

```go
func TestDispatchMatchesSourceEventFilter(t *testing.T) {
	h := script(t, `echo '{"status":"ok","decision":{"behavior":"allow"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{
		"codex_only": {Type: "shell", Command: []string{h}, HitchEvents: []string{"*"}, SourceEvents: []config.SourceEventFilter{{Harness: "codex", SourceEventType: "PreToolUse"}}, Kind: "control", Payload: "hitch", TimeoutMS: fastHandlerTimeoutMS},
	})
	got := r.Dispatch(context.Background(), Request{Envelope: testEnv(), Kind: "control", InboundEventID: "in_1", NormalizedEventID: "norm_1", TotalDeadline: 5 * time.Second})
	if len(got.Invocations) != 1 {
		t.Fatalf("expected one match: %#v", got.Invocations)
	}
	env := testEnv()
	env.SourceEventType = "PostToolUse"
	got = r.Dispatch(context.Background(), Request{Envelope: env, Kind: "control", InboundEventID: "in_1", NormalizedEventID: "norm_2", TotalDeadline: 5 * time.Second})
	if len(got.Invocations) != 0 {
		t.Fatalf("unexpected source filter match: %#v", got.Invocations)
	}
}

func TestDispatchPassesSelectedPayloadArgAndContextStdin(t *testing.T) {
	dir := t.TempDir()
	h := script(t, `printf '%s' "$1" > arg.json; python3 -c 'import json,sys; data=json.load(sys.stdin); open("stdin-kind.txt","w").write(data["payload_kind"] + "|" + data["event"]["source_event_type"]); print("{\"status\":\"ok\"}")'`)
	env := testEnv()
	env.SourcePayload = protocol.Raw(map[string]interface{}{"native": "source"})
	env.Payload = protocol.Raw(map[string]interface{}{"normalized": "hitch"})
	r := NewRunner(map[string]config.HandlerConfig{
		"payload": {Type: "shell", Command: []string{h}, WorkingDir: dir, HitchEvents: []string{"*"}, Kind: "control", Payload: "source", TimeoutMS: fastHandlerTimeoutMS},
	})
	got := r.Dispatch(context.Background(), Request{Envelope: env, Kind: "control", InboundEventID: "in_1", NormalizedEventID: "norm_1", TotalDeadline: 5 * time.Second})
	if got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("bad invocation: %#v", got.Invocations[0])
	}
	arg, err := os.ReadFile(filepath.Join(dir, "arg.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arg), "source") || strings.Contains(string(arg), "hitch") {
		t.Fatalf("wrong selected payload arg: %s", arg)
	}
	stdinKind, err := os.ReadFile(filepath.Join(dir, "stdin-kind.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdinKind) != "source|PreToolUse" {
		t.Fatalf("wrong stdin context: %s", stdinKind)
	}
}
```

- [ ] **Step 2: Run dispatch tests and verify failure**

Run:

```bash
go test ./internal/dispatch
```

Expected: FAIL because `Request`, source filters, and payload context do not exist.

- [ ] **Step 3: Add dispatch request and matching**

In `internal/dispatch/dispatch.go`, add:

```go
type Request struct {
	Envelope          protocol.EventEnvelope
	Kind              string
	InboundEventID    string
	NormalizedEventID string
	ReplaySourceID    string
	TotalDeadline     time.Duration
}
```

Change `Dispatch` to accept `Request` and use `req.Envelope`, `req.Kind`, and `req.TotalDeadline`.

Replace `matchHandlers` with:

```go
func (r Runner) matchHandlers(env protocol.EventEnvelope, kind string) []string {
	names := make([]string, 0, len(r.Handlers))
	for name, h := range r.Handlers {
		if h.Kind != kind {
			continue
		}
		if !matchesHitchEvent(h.HitchEvents, env.HitchEventType) {
			continue
		}
		if !matchesSourceEvent(h.SourceEvents, env) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func matchesHitchEvent(events []string, event protocol.EventType) bool {
	for _, e := range events {
		if e == "*" || e == string(event) {
			return true
		}
	}
	return false
}

func matchesSourceEvent(filters []config.SourceEventFilter, env protocol.EventEnvelope) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f.Harness == string(env.Harness) && f.SourceEventType == env.SourceEventType {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Build invocation context and pass selected payload arg**

Add helpers:

```go
func selectedPayload(cfg config.HandlerConfig, env protocol.EventEnvelope) protocol.RawJSON {
	if cfg.Payload == "source" {
		return env.SourcePayload
	}
	return env.Payload
}

func invocationContext(name string, cfg config.HandlerConfig, env protocol.EventEnvelope, req Request) protocol.InvocationContext {
	return protocol.InvocationContext{
		HitchVersion:      protocol.Version,
		HandlerName:       name,
		HandlerType:       cfg.Type,
		Kind:              cfg.Kind,
		InboundEventID:    req.InboundEventID,
		NormalizedEventID: req.NormalizedEventID,
		PayloadKind:       cfg.Payload,
		Payload:           selectedPayload(cfg, env),
		Event: protocol.InvocationEvent{
			HitchVersion:    env.HitchVersion,
			EventID:         env.EventID,
			ReceivedAt:      env.ReceivedAt,
			Harness:         env.Harness,
			SourceEventType: env.SourceEventType,
			SourcePayload:   env.SourcePayload,
			HitchEventType:  env.HitchEventType,
			SessionID:       env.SessionID,
			TurnID:          env.TurnID,
			CWD:             env.CWD,
			Model:           env.Model,
			TranscriptPath:  env.TranscriptPath,
			Payload:         env.Payload,
		},
	}
}
```

Change `runHandler` to marshal `invocationContext` for stdin and append `string(selectedPayload(cfg, env))` to shell command args.

- [ ] **Step 5: Update existing dispatch tests for Request API**

Replace calls like:

```go
got := r.Dispatch(context.Background(), testEnv(), "control", 5*time.Second)
```

with:

```go
got := r.Dispatch(context.Background(), Request{Envelope: testEnv(), Kind: "control", InboundEventID: "in_1", NormalizedEventID: "norm_1", TotalDeadline: 5 * time.Second})
```

Update existing test handler configs from `Events` to `HitchEvents` where those tests construct `HandlerConfig` directly.

- [ ] **Step 6: Run dispatch tests and commit**

Run:

```bash
go test ./internal/dispatch
```

Expected: PASS.

Commit:

```bash
git add internal/dispatch/dispatch.go internal/dispatch/dispatch_test.go
git commit -m "feat: route handlers by source event"
```

---

### Task 5: Native Python SDK and native dispatch

**Files:**
- Create: `sdk/python/hitch_sdk/__init__.py`
- Modify: `internal/dispatch/dispatch.go`
- Modify: `internal/dispatch/dispatch_test.go`

- [ ] **Step 1: Write failing native dispatch test**

Append to `internal/dispatch/dispatch_test.go`:

```go
func TestDispatchRunsNativeExtensionWithContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "handler.py"), []byte(`
from hitch_sdk import HandlerResult

def handle(context):
    assert context.event.source_event_type == "PreToolUse"
    assert context.event.source_payload["native"] == "source"
    assert context.event.payload["normalized"] == "hitch"
    return HandlerResult.allow()
`), 0o644); err != nil {
		t.Fatal(err)
	}
	env := testEnv()
	env.SourcePayload = protocol.Raw(map[string]interface{}{"native": "source"})
	env.Payload = protocol.Raw(map[string]interface{}{"normalized": "hitch"})
	r := NewRunner(map[string]config.HandlerConfig{
		"native": {Type: "native", Extension: "native", Entrypoint: "handler:handle", WorkingDir: dir, HitchEvents: []string{"*"}, Kind: "control", Payload: "hitch", TimeoutMS: fastHandlerTimeoutMS},
	})
	got := r.Dispatch(context.Background(), Request{Envelope: env, Kind: "control", InboundEventID: "in_1", NormalizedEventID: "norm_1", TotalDeadline: 5 * time.Second})
	if got.Aggregate.Decision.Behavior != protocol.BehaviorAllow {
		t.Fatalf("native handler did not allow: %#v", got)
	}
}
```

- [ ] **Step 2: Run dispatch tests and verify failure**

Run:

```bash
go test ./internal/dispatch
```

Expected: FAIL because native execution and `hitch_sdk` do not exist.

- [ ] **Step 3: Create Python SDK**

Create `sdk/python/hitch_sdk/__init__.py`:

```python
from __future__ import annotations

import importlib
import json
import sys
from dataclasses import dataclass
from typing import Any, Callable


@dataclass(frozen=True)
class Event:
    hitch_version: str
    event_id: str
    received_at: str
    harness: str
    source_event_type: str
    source_payload: dict[str, Any]
    hitch_event_type: str
    payload: dict[str, Any]
    session_id: str | None = None
    turn_id: str | None = None
    cwd: str | None = None
    model: str | None = None
    transcript_path: str | None = None


@dataclass(frozen=True)
class Context:
    hitch_version: str
    handler_name: str
    handler_type: str
    kind: str
    inbound_event_id: str
    normalized_event_id: str
    payload_kind: str
    payload: dict[str, Any]
    event: Event

    @classmethod
    def from_json(cls, raw: bytes | str) -> "Context":
        data = json.loads(raw)
        event = Event(**data["event"])
        return cls(event=event, **{k: v for k, v in data.items() if k != "event"})

    @classmethod
    def from_stdin(cls) -> "Context":
        return cls.from_json(sys.stdin.read())


@dataclass(frozen=True)
class HandlerResult:
    status: str = "ok"
    decision: dict[str, Any] | None = None

    @staticmethod
    def none() -> "HandlerResult":
        return HandlerResult(decision={"behavior": "none"})

    @staticmethod
    def allow() -> "HandlerResult":
        return HandlerResult(decision={"behavior": "allow"})

    @staticmethod
    def deny(reason: str) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "deny", "reason": reason})

    @staticmethod
    def block(reason: str) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "block", "reason": reason})

    @staticmethod
    def inject_context(text: str) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "inject_context", "context": text})

    @staticmethod
    def transform(updated_input: Any) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "transform", "updated_input": updated_input})

    @staticmethod
    def replace_result(updated_output: Any) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "replace_result", "updated_output": updated_output})

    def to_json(self) -> str:
        body: dict[str, Any] = {"status": self.status}
        if self.decision is not None:
            body["decision"] = self.decision
        return json.dumps(body, separators=(",", ":"))


def emit_result(result: HandlerResult) -> None:
    sys.stdout.write(result.to_json())


def run(entrypoint: str) -> None:
    module_name, function_name = entrypoint.split(":", 1)
    module = importlib.import_module(module_name)
    func: Callable[[Context], HandlerResult] = getattr(module, function_name)
    result = func(Context.from_stdin())
    emit_result(result)
```

- [ ] **Step 4: Add native execution in dispatch**

In `runHandler`, branch by `cfg.Type`:

```go
switch cfg.Type {
case "native":
	cmd = exec.CommandContext(ctx, "python3", "-c", "import hitch_sdk, os; hitch_sdk.run(os.environ['HITCH_ENTRYPOINT'])")
	cmd.Env = append(cmd.Environ(), "HITCH_CHILD=1", "HITCH_ENTRYPOINT="+cfg.Entrypoint, "PYTHONPATH="+pythonPath(cfg.WorkingDir))
default:
	args := append([]string{}, cfg.Command[1:]...)
	args = append(args, string(selectedPayload(cfg, env)))
	cmd = exec.CommandContext(ctx, cfg.Command[0], args...)
	cmd.Env = append(cmd.Environ(), "HITCH_CHILD=1")
}
```

Add helper:

```go
func pythonPath(extensionDir string) string {
	parts := []string{filepath.Join("sdk", "python")}
	if extensionDir != "" {
		parts = append(parts, extensionDir)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}
```

Keep `cmd.Dir = cfg.WorkingDir` for native and shell when set.

- [ ] **Step 5: Run dispatch tests and commit**

Run:

```bash
go test ./internal/dispatch
```

Expected: PASS.

Commit:

```bash
git add sdk/python/hitch_sdk/__init__.py internal/dispatch/dispatch.go internal/dispatch/dispatch_test.go
git commit -m "feat: run native python handlers"
```

---

### Task 6: API and replay integration with reservation-backed dispatch

**Files:**
- Modify: `internal/dispatch/dispatch.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`
- Modify: `internal/app/replay.go`
- Modify: `cmd/hitch/main_test.go`

- [ ] **Step 1: Write failing API dedupe test**

Add to `internal/api/server_test.go`:

```go
func TestSyncDispatchDoesNotRunSameObserverHookTwiceForInboundEvent(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig()
	logPath := filepath.Join(t.TempDir(), "count.txt")
	cfg.Handlers = map[string]config.HandlerConfig{
		"observer": {Type: "shell", Command: []string{"/bin/sh", "-c", "printf x >> " + shellQuote(logPath) + "; printf '%s' '{\"status\":\"ok\"}'"}, HitchEvents: []string{"*"}, Kind: "observer", Payload: "hitch", TimeoutMS: 1000},
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
	srv.dispatchObservers(ctx, eventID, env)
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "x" {
		t.Fatalf("observer ran more than once: %q", b)
	}
}
```

Add this helper near the test if no shell quoting helper exists in `server_test.go`: `func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }`.

- [ ] **Step 2: Run API tests and verify failure**

Run:

```bash
go test ./internal/api
```

Expected: FAIL because server still calls the old dispatch API and does not reserve invocations.

- [ ] **Step 3: Add dispatch recorder interface**

In `internal/dispatch/dispatch.go`, add:

```go
type Recorder interface {
	ReserveHandlerInvocation(context.Context, Reservation) (bool, error)
	CompleteHandlerInvocation(context.Context, Invocation) error
}

type Reservation struct {
	ID                string
	InboundEventID    string
	NormalizedEventID string
	HandlerName       string
	Kind              string
	HookKey           string
	StartedAt         time.Time
}
```

Add `Recorder Recorder` to `Runner` and make `Dispatch` reserve before `runHandler` when `Recorder` is non-nil. If reservation returns false, return an `Invocation` with `StatusSkipped`, `HandlerName`, `Kind`, `StartedAt`, and `CompletedAt` set. Aggregation must ignore `StatusSkipped` like non-OK fail-open statuses.

- [ ] **Step 4: Adapt store to dispatch recorder**

In `internal/store/store.go`, add methods with dispatch-compatible signatures or adapter methods in `internal/api/server.go`. The direct store method should map dispatch reservation to store reservation:

```go
func (s *Store) ReserveHandlerInvocation(ctx context.Context, r dispatch.Reservation) (bool, error) {
	return s.reserveHandlerInvocation(ctx, HandlerInvocationReservation{
		ID:                r.ID,
		InboundEventID:    r.InboundEventID,
		NormalizedEventID: r.NormalizedEventID,
		HandlerName:       r.HandlerName,
		Kind:              r.Kind,
		HookKey:           r.HookKey,
		StartedAt:         r.StartedAt,
	})
}
```

If importing `dispatch` into `store` creates an import cycle, keep store signatures independent and define a small adapter in `internal/api/server.go`.

- [ ] **Step 5: Update server and replay call sites**

In `internal/api/server.go`, construct runner with recorder:

```go
runner := dispatch.NewRunnerWithLogger(cfg.Handlers, log)
runner.Recorder = st
```

Change sync dispatch to:

```go
result := s.runner.Dispatch(r.Context(), dispatch.Request{Envelope: env, Kind: "control", InboundEventID: resp.EventID, NormalizedEventID: resp.NormalizedEventID, TotalDeadline: 2 * time.Second})
```

Change observer dispatch to accept inbound ID or derive it from the envelope/request data passed by the caller. Use the persisted inbound ID, not the public event ID, when reserving. If `EventResponse.EventID` is the envelope event ID and not the store inbound row ID, extend `EventResponse` internally or return the inbound row ID from `ingest` so dispatch gets the true inbound ID.

In `internal/app/replay.go`, pass a replay-scoped inbound ID:

```go
result := dispatch.NewRunner(cfg.Handlers).Dispatch(ctx, dispatch.Request{Envelope: env, Kind: "control", InboundEventID: harness.NewID("replay"), NormalizedEventID: opts.EventID, ReplaySourceID: opts.EventID, TotalDeadline: 2 * time.Second})
```

- [ ] **Step 6: Run integration tests and commit**

Run:

```bash
go test ./internal/api ./internal/app ./cmd/hitch
```

Expected: PASS.

Commit:

```bash
git add internal/dispatch/dispatch.go internal/api/server.go internal/api/server_test.go internal/app/replay.go cmd/hitch/main_test.go internal/store/store.go
git commit -m "feat: dedupe handler execution by inbound event"
```

---

### Task 7: Examples and documentation after behavior passes

**Files:**
- Create: `examples/extensions/audit_logger/config.toml`
- Create: `examples/extensions/audit_logger/handler.py`
- Modify: `docs/configuration.md`
- Modify: `docs/handler-development.md`
- Modify: `docs/handler-protocol.md`
- Modify: `examples/payload-logger.config.toml`

- [ ] **Step 1: Add native extension example**

Create `examples/extensions/audit_logger/config.toml`:

```toml
name = "audit_logger"
entrypoint = "handler:handle"
kind = "observer"
hitch_events = ["tool.completed"]
payload = "hitch"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
```

Create `examples/extensions/audit_logger/handler.py`:

```python
from __future__ import annotations

from pathlib import Path

from hitch_sdk import Context, HandlerResult


def handle(context: Context) -> HandlerResult:
    path = Path("tmp/hitch-native-audit.log")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        f"{context.event.harness} {context.event.source_event_type} {context.event.hitch_event_type}\n",
        encoding="utf-8",
    )
    return HandlerResult.none()
```

- [ ] **Step 2: Update docs with exact behavior**

Add to `docs/configuration.md` handler section:

```markdown
Handlers support two invocation types: `shell` and `native`. Existing handlers default to `type = "shell"` and may keep using `events = ["tool.requested"]`; new configs should use `hitch_events = ["tool.requested"]`.

`source_events` narrows a handler to exact source hook pairs:

```toml
source_events = [{ harness = "codex", source_event_type = "PreToolUse" }]
```

`payload = "hitch"` passes Hitch's normalized payload as the primary payload. `payload = "source"` passes the preserved source payload as the primary payload. Both payloads are always available in the invocation context.
```

Add to `docs/handler-protocol.md`:

```markdown
Hitch writes an invocation context to handler stdin. Shell handlers also receive the selected primary payload as one compact JSON command-line argument. Native handlers receive `Context` through the Python SDK and return `HandlerResult`.

`status = "skipped"` means Hitch did not execute the handler because the same handler and hook key were already reserved for the inbound event. Skipped results do not affect decision aggregation.
```

Add to `docs/handler-development.md` a native example using `handle(context)` and the `audit_logger` extension path.

- [ ] **Step 3: Run docs-adjacent tests**

Run:

```bash
go test ./internal/config ./internal/dispatch ./internal/api
```

Expected: PASS.

- [ ] **Step 4: Commit examples and docs**

Commit:

```bash
git add examples/extensions/audit_logger/config.toml examples/extensions/audit_logger/handler.py docs/configuration.md docs/handler-development.md docs/handler-protocol.md examples/payload-logger.config.toml
git commit -m "docs: document native handler invocation"
```

---

### Task 8: End-to-end verification

**Files:**
- No planned code edits. Fix only failures caused by the previous tasks.

- [ ] **Step 1: Run focused package tests**

Run:

```bash
go test ./internal/config ./internal/protocol ./internal/store ./internal/dispatch ./internal/api ./internal/app ./cmd/hitch
```

Expected: PASS.

- [ ] **Step 2: Run example smoke tests**

Run:

```bash
python3 examples/test_payload_logger.py
```

Expected: script exits 0 and verifies payload log rows across configured harnesses.

- [ ] **Step 3: Run full Go test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Final commit if verification fixes were needed**

If Step 1, Step 2, or Step 3 required code changes, stage the revamp file set and commit the verification fix:

```bash
git add internal/config/config.go internal/protocol/protocol.go internal/store/store.go internal/dispatch/dispatch.go internal/api/server.go internal/app/replay.go cmd/hitch/main_test.go sdk/python/hitch_sdk/__init__.py docs/configuration.md docs/handler-development.md docs/handler-protocol.md examples/extensions/audit_logger/config.toml examples/extensions/audit_logger/handler.py
git commit -m "fix: complete handler invocation revamp"
```

If no files changed during verification, do not create an empty commit.
