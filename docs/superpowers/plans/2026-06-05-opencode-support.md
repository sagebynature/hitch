# OpenCode Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenCode as a first-class Hitch harness with normalized lifecycle events, sync control translation, and managed plugin installation.

**Architecture:** OpenCode support should mirror Pi/OMP: Hitch installs a generated TypeScript plugin into OpenCode's global plugin directory, the plugin forwards selected hook/event payloads to `/v1/dispatch-sync`, and Hitch normalizes those payloads behind a new `opencode` harness. The server remains config-driven: OpenCode source events must appear in `[harness.opencode.event_map]`, with low-noise defaults in `internal/config/default.config.toml` and the full catalog documented in `docs/events.md`.

**Tech Stack:** Go 1.x Hitch daemon/client, TOML config, generated TypeScript OpenCode plugin, OpenCode plugin hooks from `@opencode-ai/plugin`.

---

## Research Summary

Primary sources:

- OpenCode plugin docs: https://opencode.ai/docs/plugins/
- OpenCode SDK docs/event stream: https://opencode.ai/docs/sdk
- OpenCode config schema: https://opencode.ai/config.json
- OpenCode plugin type source: https://raw.githubusercontent.com/sst/opencode/dev/packages/plugin/src/index.ts
- OpenCode generated SDK event types: https://raw.githubusercontent.com/sst/opencode/dev/packages/sdk/js/src/gen/types.gen.ts

Observed OpenCode integration surface:

- Local plugins load automatically from `.opencode/plugins/` and `~/.config/opencode/plugins/`.
- Config supports npm plugin entries via `opencode.json` field `plugin`, but Hitch should use the global local plugin path for an idempotent managed install.
- Plugin functions export async functions returning hook objects.
- Generic `event({ event })` receives SDK server events, including `session.created`, `session.idle`, `session.compacted`, `session.error`, `message.updated`, `message.part.updated`, `permission.updated`, `permission.replied`, `file.edited`, `command.executed`, `todo.updated`, and `server.connected`.
- Typed control hooks include `chat.message`, `chat.params`, `chat.headers`, `command.execute.before`, `permission.ask`, `tool.execute.before`, `tool.execute.after`, `shell.env`, `experimental.session.compacting`, `experimental.compaction.autocontinue`, `experimental.text.complete`, and `tool.definition`.
- The docs list `permission.asked`, but the current SDK generated event type is `permission.updated`; support both names in docs/config catalog, and use `permission.ask` for the control hook.

Default event mapping decision:

| OpenCode source event | Hitch event | Default | Reason |
| --- | --- | --- | --- |
| `chat.message` | `turn.user_prompt` | Yes | Closest to Codex `UserPromptSubmit` and Pi/OMP `input`. |
| `tool.execute.before` | `tool.requested` | Yes | Closest to Codex `PreToolUse` and Pi/OMP `tool_call`. |
| `tool.execute.after` | `tool.completed` | Yes | Closest to Codex `PostToolUse` and Pi/OMP `tool_result`. |
| `permission.ask` | `tool.permission_requested` | Yes | Closest to Codex `PermissionRequest`. |
| `session.created` | `session.started` | Yes | Low-noise session lifecycle. |
| `session.idle` | `["turn.completed", "turn.assistant_completed"]` | Yes | OpenCode's documented notification example treats idle as session completion. Secondary row gives cross-harness assistant-complete audits. |
| `session.compacted` | `session.compacted` | Yes | Post-compaction lifecycle event. |
| `experimental.session.compacting` | `session.compacted` | Yes | Pre-compaction control hook. |
| `session.error` | `error.reported` | Yes | Native error lifecycle. |
| `command.executed` | `turn.user_prompt` | No | Useful command audit, but not a normal model turn and may be product-specific. |
| `chat.params`, `chat.headers` | `llm.requested` | No | LLM request mutation hooks; potentially high-volume/sensitive. |
| `experimental.text.complete` | `llm.completed` | No | Fine-grained text completion hook; opt-in. |
| `file.edited`, `file.watcher.updated` | `tool.completed` / `tool.progress` | No | File telemetry is useful but not lifecycle-critical. |
| `message.*`, `todo.updated`, `permission.updated`, `permission.replied`, `server.*`, `installation.*`, `lsp.*`, `tui.*`, `pty.*`, `vcs.*`, `tool.definition`, `shell.env`, `experimental.compaction.autocontinue` | See `docs/events.md` catalog | No | Keep defaults low-noise; catalog these for explicit opt-in where the existing Hitch taxonomy can represent them. |

Native response decision:

Create an OpenCode adapter response contract that the generated plugin owns:

```json
{
  "adapter_action": "noop | throw | set | append | inject_context",
  "path": ["args"],
  "value": {},
  "message": "blocked by policy"
}
```

- `noop`: do nothing.
- `throw`: throw `Error(message)` inside the OpenCode hook. Use for `block`, `deny`, and `stop` on blocking hooks.
- `set`: set `output[path...] = value`.
- `append`: append `value` to an array at `output[path...]`.
- `inject_context`: call `ctx.client.session.prompt({ path: { id: sessionID }, body: { noReply: true, parts: [{ type: "text", text: value }] } })` from the plugin when a hook has a `sessionID`.

Handlers can still bypass normalized translation with `decision.native_response`; Hitch returns that JSON directly for OpenCode, matching Codex/Hermes/Pi/OMP behavior.

## File Structure

Create:

- `internal/harness/opencode/opencode.go`: OpenCode normalizer and translator.
- `internal/harness/opencode/opencode_test.go`: normalization, metadata extraction, and native response translation tests.

Modify:

- `internal/protocol/protocol.go`: add `HarnessOpenCode` and make it valid.
- `internal/protocol/protocol_test.go`: add `opencode` validity coverage.
- `internal/config/config.go`: add `HarnessConfig.OpenCode` and validate its event map.
- `internal/config/default.config.toml`: add `[harness.opencode]` and default low-noise event map.
- `internal/config/config_test.go`: assert defaults parse and invalid OpenCode event-map values are rejected.
- `internal/api/server.go`: route `protocol.HarnessOpenCode` to `opencode.Mapper{}`.
- `internal/api/server_test.go`: add API ingest and sync dispatch tests for OpenCode.
- `internal/clientshim/clientshim.go`: add OpenCode native noop translation.
- `internal/clientshim/clientshim_test.go`: add OpenCode fail-open/noop coverage.
- `internal/install/install.go`: add OpenCode detection, planning, plugin install/uninstall, generated plugin content.
- `internal/install/install_test.go`: add OpenCode install, uninstall, idempotence, pinned URL, detection, and unknown-harness list coverage.
- `cmd/hitch-client/help.go`: include `opencode` in harness help text and examples.
- `cmd/hitch-client/main_test.go`: update help assertions.
- `docs/events.md`: add OpenCode source event catalog and native response behavior.
- `docs/configuration.md`, `docs/installation.md`, `docs/handler-development.md`, `docs/harness-contracts.md`, `docs/index.html`, `docs/docs/latest/index.html`, `README.md`: update supported harness lists and install/docs references.

---

### Task 1: Add OpenCode to protocol and config

**Files:**

- Modify: `internal/protocol/protocol.go`
- Modify: `internal/protocol/protocol_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/default.config.toml`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write protocol/config tests**

Add protocol coverage in `internal/protocol/protocol_test.go`:

```go
func TestOpenCodeHarnessIsValid(t *testing.T) {
	if !IsValidHarness(HarnessOpenCode) {
		t.Fatalf("opencode harness should be valid")
	}
}
```

Add config coverage in `internal/config/config_test.go`:

```go
func TestDefaultConfigIncludesOpenCodeHarness(t *testing.T) {
	cfg, err := Parse([]byte(DefaultConfigTOML))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Harness.OpenCode.Enabled {
		t.Fatal("harness.opencode should be enabled by default")
	}
	cases := map[string]protocol.EventType{
		"chat.message":                    protocol.EventTurnUserPrompt,
		"tool.execute.before":             protocol.EventToolRequested,
		"tool.execute.after":              protocol.EventToolCompleted,
		"permission.ask":                  protocol.EventToolPermissionRequest,
		"session.created":                 protocol.EventSessionStarted,
		"session.compacted":               protocol.EventSessionCompacted,
		"experimental.session.compacting": protocol.EventSessionCompacted,
		"session.error":                   protocol.EventErrorReported,
	}
	for source, want := range cases {
		got := cfg.Harness.OpenCode.EventMap[source]
		if len(got) == 0 || got[0] != want {
			t.Fatalf("opencode %s mapped to %#v, want first %s", source, got, want)
		}
	}
	idle := cfg.Harness.OpenCode.EventMap["session.idle"]
	if len(idle) != 2 || idle[0] != protocol.EventTurnCompleted || idle[1] != protocol.EventTurnAssistantCompleted {
		t.Fatalf("session.idle mapped to %#v", idle)
	}
}

func TestValidateRejectsInvalidOpenCodeEventMapValue(t *testing.T) {
	cfg, err := Parse([]byte(DefaultConfigTOML))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Harness.OpenCode.EventMap["chat.message"] = EventTypes{protocol.EventType("not.real")}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "harness.opencode.event_map.chat.message") {
		t.Fatalf("expected OpenCode event-map validation error, got %v", err)
	}
}
```


- [ ] **Step 2: Run tests and verify failure**

Run:

```sh
go test ./internal/protocol ./internal/config
```

Expected: FAIL because `HarnessOpenCode` and `HarnessConfig.OpenCode` do not exist.

- [ ] **Step 3: Add protocol/config implementation**

Modify `internal/protocol/protocol.go`:

```go
const (
	HarnessCodex    Harness = "codex"
	HarnessHermes   Harness = "hermes"
	HarnessPi       Harness = "pi"
	HarnessOMP      Harness = "omp"
	HarnessOpenCode Harness = "opencode"
)

var validHarnesses = map[Harness]struct{}{
	HarnessCodex: {}, HarnessHermes: {}, HarnessPi: {}, HarnessOMP: {}, HarnessOpenCode: {},
}
```

Modify `internal/config/config.go`:

```go
type HarnessConfig struct {
	Codex    HarnessToggle `toml:"codex"`
	Hermes   HarnessToggle `toml:"hermes"`
	Pi       HarnessToggle `toml:"pi"`
	OMP      HarnessToggle `toml:"omp"`
	OpenCode HarnessToggle `toml:"opencode"`
}
```

Add validation after OMP validation:

```go
if err := validateHarnessEventMap("opencode", c.Harness.OpenCode.EventMap); err != nil {
	return err
}
```

Modify `internal/config/default.config.toml` after the OMP section:

```toml
[harness.opencode]
enabled = true

[harness.opencode.event_map]
chat.message = "turn.user_prompt"
tool.execute.before = "tool.requested"
tool.execute.after = "tool.completed"
permission.ask = "tool.permission_requested"
session.created = "session.started"
session.idle = ["turn.completed", "turn.assistant_completed"]
session.compacted = "session.compacted"
experimental.session.compacting = "session.compacted"
session.error = "error.reported"
# Full source-event catalog and opt-in mappings are documented in docs/events.md.
```

- [ ] **Step 4: Run tests and verify pass**

Run:

```sh
go test ./internal/protocol ./internal/config
```

Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add internal/protocol/protocol.go internal/protocol/protocol_test.go internal/config/config.go internal/config/default.config.toml internal/config/config_test.go
git commit -m "feat: register opencode harness config"
```

### Task 2: Implement OpenCode normalizer and translator

**Files:**

- Create: `internal/harness/opencode/opencode.go`
- Create: `internal/harness/opencode/opencode_test.go`

- [ ] **Step 1: Write normalizer and translator tests**

Create `internal/harness/opencode/opencode_test.go`:

```go
package opencode

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizeUnwrapsPluginEnvelopeAndMetadata(t *testing.T) {
	source := protocol.Raw(map[string]interface{}{
		"event": map[string]interface{}{
			"input": map[string]interface{}{"tool": "bash", "sessionID": "sess_1", "callID": "call_1"},
			"output": map[string]interface{}{"args": map[string]interface{}{"command": "pwd"}},
		},
		"metadata": map[string]interface{}{
			"session_id":      "sess_1",
			"turn_id":         "msg_1",
			"cwd":             "/tmp/project",
			"model":           "anthropic/claude-sonnet-4",
			"transcript_path": "/tmp/opencode/session.json",
		},
	})

	env, err := Mapper{}.Normalize("tool.execute.before", source, protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.Harness != protocol.HarnessOpenCode || env.SourceEventType != "tool.execute.before" || env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	if env.SessionID != "sess_1" || env.TurnID != "msg_1" || env.CWD != "/tmp/project" || env.Model != "anthropic/claude-sonnet-4" || env.TranscriptPath != "/tmp/opencode/session.json" {
		t.Fatalf("metadata not copied: %#v", env)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["input"]; !ok {
		t.Fatalf("payload was not unwrapped: %#v", payload)
	}
}

func TestTranslateBlocksToolExecuteBeforeByThrowing(t *testing.T) {
	native, err := Mapper{}.Translate("tool.execute.before", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorBlock, Reason: "blocked"}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(native, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "throw" || got.Message != "blocked" {
		t.Fatalf("unexpected native response: %#v", got)
	}
}

func TestTranslateTransformsToolExecuteBeforeArgs(t *testing.T) {
	native, err := Mapper{}.Translate("tool.execute.before", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorTransform, UpdatedInput: protocol.Raw(map[string]interface{}{"command": "echo ok"})}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(native, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "set" || len(got.Path) != 1 || got.Path[0] != "args" {
		t.Fatalf("unexpected native response: %#v", got)
	}
}

func TestTranslatePermissionAskAllowAndDeny(t *testing.T) {
	allow, err := Mapper{}.Translate("permission.ask", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorAllow}})
	if err != nil {
		t.Fatal(err)
	}
	var allowResp AdapterResponse
	if err := json.Unmarshal(allow, &allowResp); err != nil {
		t.Fatal(err)
	}
	if allowResp.AdapterAction != "set" || allowResp.Value != "allow" {
		t.Fatalf("unexpected allow response: %#v", allowResp)
	}

	deny, err := Mapper{}.Translate("permission.ask", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorDeny, Reason: "no"}})
	if err != nil {
		t.Fatal(err)
	}
	var denyResp AdapterResponse
	if err := json.Unmarshal(deny, &denyResp); err != nil {
		t.Fatal(err)
	}
	if denyResp.AdapterAction != "set" || denyResp.Value != "deny" {
		t.Fatalf("unexpected deny response: %#v", denyResp)
	}
}

func TestTranslateInjectsChatMessageContext(t *testing.T) {
	native, err := Mapper{}.Translate("chat.message", protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorInjectContext, Context: "extra context"}})
	if err != nil {
		t.Fatal(err)
	}
	var got AdapterResponse
	if err := json.Unmarshal(native, &got); err != nil {
		t.Fatal(err)
	}
	if got.AdapterAction != "inject_context" || got.Value != "extra context" {
		t.Fatalf("unexpected native response: %#v", got)
	}
}

func TestTranslateUsesNativeResponseDirectly(t *testing.T) {
	want := protocol.Raw(map[string]interface{}{"adapter_action": "throw", "message": "native"})
	got, err := Mapper{}.Translate("tool.execute.before", protocol.AggregateDecision{Decision: protocol.Decision{NativeResponse: want}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```sh
go test ./internal/harness/opencode
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement mapper**

Create `internal/harness/opencode/opencode.go`:

```go
package opencode

import (
	"encoding/json"

	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

type pluginPayloadEnvelope struct {
	Event    protocol.RawJSON `json:"event"`
	Metadata pluginMetadata   `json:"metadata"`
}

type pluginMetadata struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	CWD            string `json:"cwd"`
	Model          string `json:"model"`
	TranscriptPath string `json:"transcript_path"`
}

type AdapterResponse struct {
	AdapterAction string      `json:"adapter_action"`
	Path          []string    `json:"path,omitempty"`
	Value         interface{} `json:"value,omitempty"`
	Message       string      `json:"message,omitempty"`
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	eventPayload, meta := unwrapSourcePayload(sourcePayload)
	env := harness.NewEnvelope(protocol.HarnessOpenCode, sourceEventType, sourcePayload, hitchEventType, eventPayload)
	env.SessionID = meta.SessionID
	env.TurnID = meta.TurnID
	env.CWD = meta.CWD
	env.Model = meta.Model
	env.TranscriptPath = meta.TranscriptPath
	if env.SessionID == "" && env.TurnID == "" && env.CWD == "" && env.Model == "" && env.TranscriptPath == "" {
		harness.ApplySourceMetadata(&env, eventPayload)
	}
	return env, protocol.ValidateEnvelope(env)
}

func unwrapSourcePayload(sourcePayload protocol.RawJSON) (protocol.RawJSON, pluginMetadata) {
	var wrapped pluginPayloadEnvelope
	if err := json.Unmarshal(sourcePayload, &wrapped); err != nil {
		return sourcePayload, pluginMetadata{}
	}
	if len(wrapped.Event) != 0 && json.Valid(wrapped.Event) {
		return wrapped.Event, wrapped.Metadata
	}
	return sourcePayload, pluginMetadata{}
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	resp := AdapterResponse{AdapterAction: "noop"}
	switch sourceEventType {
	case "tool.execute.before":
		if shouldThrow(d.Behavior) {
			resp.AdapterAction = "throw"
			resp.Message = d.Reason
		} else if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"args"}
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "tool.execute.after", "experimental.text.complete":
		if (d.Behavior == protocol.BehaviorReplaceResult || d.Behavior == protocol.BehaviorTransform) && len(d.UpdatedOutput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"output"}
			resp.Value = decodeJSON(d.UpdatedOutput)
		}
	case "permission.ask":
		if d.Behavior == protocol.BehaviorAllow {
			resp.AdapterAction = "set"
			resp.Path = []string{"status"}
			resp.Value = "allow"
		} else if d.Behavior == protocol.BehaviorDeny || d.Behavior == protocol.BehaviorBlock || d.Behavior == protocol.BehaviorStop {
			resp.AdapterAction = "set"
			resp.Path = []string{"status"}
			resp.Value = "deny"
		}
	case "chat.message", "command.execute.before":
		if shouldThrow(d.Behavior) {
			resp.AdapterAction = "throw"
			resp.Message = d.Reason
		} else if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			resp.AdapterAction = "inject_context"
			resp.Value = d.Context
		} else if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"parts"}
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "shell.env", "chat.params", "chat.headers", "tool.definition":
		if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "experimental.session.compacting":
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			resp.AdapterAction = "append"
			resp.Path = []string{"context"}
			resp.Value = d.Context
		} else if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"prompt"}
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "experimental.compaction.autocontinue":
		if d.Behavior == protocol.BehaviorStop || d.Behavior == protocol.BehaviorBlock {
			resp.AdapterAction = "set"
			resp.Path = []string{"enabled"}
			resp.Value = false
		}
	}
	return protocol.Raw(resp), nil
}

func shouldThrow(b protocol.DecisionBehavior) bool {
	return b == protocol.BehaviorBlock || b == protocol.BehaviorDeny || b == protocol.BehaviorStop
}

func decodeJSON(raw protocol.RawJSON) interface{} {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
```

- [ ] **Step 4: Run tests and verify pass**

Run:

```sh
go test ./internal/harness/opencode
```

Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add internal/harness/opencode/opencode.go internal/harness/opencode/opencode_test.go
git commit -m "feat: add opencode harness mapper"
```

### Task 3: Wire OpenCode into API and client shim

**Files:**

- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`
- Modify: `internal/clientshim/clientshim.go`
- Modify: `internal/clientshim/clientshim_test.go`

- [ ] **Step 1: Write API and client shim tests**

Add an API sync test in `internal/api/server_test.go` following the existing Codex/Hermes/Pi/OMP API test style:

```go
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
```


Add client shim coverage in `internal/clientshim/clientshim_test.go`:

```go
func TestNativeNoopOpenCodeReturnsNoopAdapterResponse(t *testing.T) {
	native := NativeNoop("opencode", "tool.execute.before")
	var got map[string]interface{}
	if err := json.Unmarshal(native, &got); err != nil {
		t.Fatal(err)
	}
	if got["adapter_action"] != "noop" {
		t.Fatalf("unexpected noop response: %#v", got)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```sh
go test ./internal/api ./internal/clientshim
```

Expected: FAIL because OpenCode is not wired into API/runtime/clientshim.

- [ ] **Step 3: Wire mapper**

Modify `internal/api/server.go` imports to include:

```go
"github.com/sagebynature/hitch/internal/harness/opencode"
```

Modify `buildHarnessRuntimes`:

```go
protocol.HarnessOpenCode: {normalizer: opencode.Mapper{}, eventMap: cfg.Harness.OpenCode.EventMap},
```

Modify `internal/clientshim/clientshim.go` imports to include:

```go
"github.com/sagebynature/hitch/internal/harness/opencode"
```

Modify `NativeNoop`:

```go
case protocol.HarnessOpenCode:
	native, _ := opencode.Mapper{}.Translate(sourceEventType, aggregate)
	return native
```

- [ ] **Step 4: Run tests and verify pass**

Run:

```sh
go test ./internal/api ./internal/clientshim
```

Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add internal/api/server.go internal/api/server_test.go internal/clientshim/clientshim.go internal/clientshim/clientshim_test.go
git commit -m "feat: route opencode events"
```

### Task 4: Add managed OpenCode plugin install/uninstall

**Files:**

- Modify: `internal/install/install.go`
- Modify: `internal/install/install_test.go`
- Modify: `cmd/hitch-client/help.go`
- Modify: `cmd/hitch-client/main_test.go`

- [ ] **Step 1: Write installer tests**

Add tests in `internal/install/install_test.go`:

```go
func TestApplyOpsInstallsOpenCodePluginIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	t.Setenv("PATH", addFakeCommand(t, "opencode"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "hitch.ts")
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, []byte("export const Existing = async () => ({})\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ops, err := plannedOps([]string{"opencode"}, false, "http://127.0.0.1:9000", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Action != "install_opencode_plugin" || ops[0].Path != pluginPath {
		t.Fatalf("unexpected ops: %#v", ops)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Managed by Hitch", "HITCH_PINNED_API_URL = \"http://127.0.0.1:9000\"", "export const HitchPlugin", "tool.execute.before", "permission.ask", "/v1/dispatch-sync"} {
		if !strings.Contains(string(first), want) {
			t.Fatalf("plugin missing %q:\n%s", want, first)
		}
	}
	backupMatches, err := filepath.Glob(filepath.Join(home, ".config", "hitch", "backups", "opencode-*.hitch.ts.bak"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backupMatches) != 1 {
		t.Fatalf("expected one OpenCode backup, got %#v", backupMatches)
	}

	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("second install changed managed OpenCode plugin")
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedOpenCodePlugin(t *testing.T) {
	t.Setenv("PATH", addFakeCommand(t, "opencode"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "hitch.ts")
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := opencodePluginContent("http://127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	ops, err := plannedOps([]string{"opencode"}, true, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Action != "uninstall_opencode_plugin" {
		t.Fatalf("unexpected ops: %#v", ops)
	}
	if err := applyOps(ops, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pluginPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed OpenCode plugin still exists or stat failed: %v", err)
	}
}

func TestOpenCodePluginContentAppliesThrowSetAppendAndInjectContext(t *testing.T) {
	content, err := opencodePluginContent("")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"function applyAdapterResponse", "adapter_action === \"throw\"", "adapter_action === \"set\"", "adapter_action === \"append\"", "adapter_action === \"inject_context\"", "client.session.prompt", "HITCH_DEFAULT_API_URL"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated plugin missing %q:\n%s", want, text)
		}
	}
}
```

Update help test expectations in `cmd/hitch-client/main_test.go` to include `opencode` wherever harness lists are asserted.

- [ ] **Step 2: Run tests and verify failure**

Run:

```sh
go test ./internal/install ./cmd/hitch-client
```

Expected: FAIL because OpenCode installer paths/functions do not exist.

- [ ] **Step 3: Add OpenCode installer implementation**

Modify `internal/install/install.go`.

Add source event catalog near the other event lists:

```go
var opencodeHookEvents = []string{
	"chat.message",
	"chat.params",
	"chat.headers",
	"command.execute.before",
	"command.executed",
	"permission.ask",
	"permission.asked",
	"permission.updated",
	"permission.replied",
	"tool.execute.before",
	"tool.execute.after",
	"tool.definition",
	"shell.env",
	"experimental.session.compacting",
	"experimental.compaction.autocontinue",
	"experimental.text.complete",
	"session.created",
	"session.updated",
	"session.deleted",
	"session.diff",
	"session.error",
	"session.idle",
	"session.status",
	"session.compacted",
	"message.updated",
	"message.removed",
	"message.part.updated",
	"message.part.removed",
	"file.edited",
	"file.watcher.updated",
	"todo.updated",
	"server.connected",
	"server.instance.disposed",
	"installation.updated",
	"installation.update-available",
	"lsp.client.diagnostics",
	"lsp.updated",
	"tui.prompt.append",
	"tui.command.execute",
	"tui.toast.show",
	"pty.created",
	"pty.updated",
	"pty.exited",
	"pty.deleted",
	"vcs.branch.updated",
}
```

Add harness spec:

```go
{Name: "opencode", Title: "OpenCode", Command: "opencode", ConfigPath: "~/.config/opencode/plugins/hitch.ts", Supported: true},
```

Add planned op case:

```go
case "opencode":
	action := "install_opencode_plugin"
	if uninstall {
		action = "uninstall_opencode_plugin"
	}
	ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: extensionURLReason(apiURL), AdapterURL: apiURL})
```

Add apply cases:

```go
case "install_opencode_plugin":
	return installOpenCodePlugin(op.Path, op.BackupPath, op.AdapterURL)
case "uninstall_opencode_plugin":
	return uninstallOpenCodePlugin(op.Path, op.BackupPath)
```

Add helpers:

```go
func opencodePluginInstalled(path string) bool {
	b, err := os.ReadFile(config.ExpandHome(path))
	return err == nil && bytes.Contains(b, []byte(piManagedExtensionMarker)) && bytes.Contains(b, []byte("HitchPlugin"))
}

func installOpenCodePlugin(path, backup, apiURL string) error {
	content, err := opencodePluginContent(apiURL)
	if err != nil {
		return err
	}
	return installExtensionContent(path, backup, content)
}

func uninstallOpenCodePlugin(path, backup string) error {
	return uninstallPiExtension(path, backup)
}

func opencodePluginContent(apiURL string) ([]byte, error) {
	return openCodePluginContent("opencode", "hitch-opencode-plugin", opencodeHookEvents, apiURL)
}
```

Add `openCodePluginContent` using the existing JSON literal helpers. The generated TypeScript must export `HitchPlugin` and include this logic:

```ts
export const HitchPlugin = async (ctx) => {
  const { client } = ctx
  return {
    event: async ({ event }) => {
      if (HITCH_EVENTS.includes(event.type)) {
        await dispatchToHitch(event.type, { event }, {}, ctx)
      }
    },
    "chat.message": async (input, output) => dispatchToHitch("chat.message", input, output, ctx),
    "chat.params": async (input, output) => dispatchToHitch("chat.params", input, output, ctx),
    "chat.headers": async (input, output) => dispatchToHitch("chat.headers", input, output, ctx),
    "command.execute.before": async (input, output) => dispatchToHitch("command.execute.before", input, output, ctx),
    "permission.ask": async (input, output) => dispatchToHitch("permission.ask", input, output, ctx),
    "tool.execute.before": async (input, output) => dispatchToHitch("tool.execute.before", input, output, ctx),
    "tool.execute.after": async (input, output) => dispatchToHitch("tool.execute.after", input, output, ctx),
    "tool.definition": async (input, output) => dispatchToHitch("tool.definition", input, output, ctx),
    "shell.env": async (input, output) => dispatchToHitch("shell.env", input, output, ctx),
    "experimental.session.compacting": async (input, output) => dispatchToHitch("experimental.session.compacting", input, output, ctx),
    "experimental.compaction.autocontinue": async (input, output) => dispatchToHitch("experimental.compaction.autocontinue", input, output, ctx),
    "experimental.text.complete": async (input, output) => dispatchToHitch("experimental.text.complete", input, output, ctx),
  }
}
```

The generated helper functions must:

- Resolve URL from pinned URL, `HITCH_URL`, then `http://127.0.0.1:8799`.
- Clone `input` and `output` with `JSON.parse(JSON.stringify(...))` before sending.
- Collect metadata: `session_id` from `input.sessionID`, `event.properties.sessionID`, or `ctx.project.id`; `cwd` from `ctx.directory`; `model` from `input.model` where available.
- POST `{harness:"opencode", source_event_type, source_payload:{event:{input, output}, metadata}, hitch_client_version}` to `/v1/dispatch-sync`.
- Fail open on fetch/JSON errors by returning without mutating OpenCode state.
- Apply adapter responses with `throw`, `set`, `append`, and `inject_context`.

- [ ] **Step 4: Update CLI help**

Modify `cmd/hitch-client/help.go` harness text from:

```text
source harness: codex, hermes, pi, omp
```

to:

```text
source harness: codex, hermes, pi, omp, opencode
```

Add examples:

```text
hitch-client install --only opencode --yes
hitch-client uninstall --only opencode --yes
```

- [ ] **Step 5: Run tests and verify pass**

Run:

```sh
go test ./internal/install ./cmd/hitch-client
```

Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add internal/install/install.go internal/install/install_test.go cmd/hitch-client/help.go cmd/hitch-client/main_test.go
git commit -m "feat: install opencode plugin"
```

### Task 5: Add OpenCode end-to-end lifecycle coverage

**Files:**

- Modify: `cmd/hitch/main_test.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Write OpenCode lifecycle E2E test**

Add a test in `cmd/hitch/main_test.go` next to `TestE2ECodexLifecycleHooksDispatchToNoopObserver`:

```go
func TestE2EOpenCodeLifecycleHooksDispatchToNoopObserver(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "events.sqlite")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := e2eNoopConfig(t, dbPath)
	cfg.Harness.OpenCode.EventMap = map[string]config.EventTypes{
		"chat.message":                    {protocol.EventTurnUserPrompt},
		"tool.execute.before":             {protocol.EventToolRequested},
		"tool.execute.after":              {protocol.EventToolCompleted},
		"permission.ask":                  {protocol.EventToolPermissionRequest},
		"session.created":                 {protocol.EventSessionStarted},
		"session.idle":                    {protocol.EventTurnCompleted, protocol.EventTurnAssistantCompleted},
		"experimental.session.compacting": {protocol.EventSessionCompacted},
		"session.compacted":               {protocol.EventSessionCompacted},
		"session.error":                   {protocol.EventErrorReported},
	}

	events := []struct {
		native string
		hitch  protocol.EventType
	}{
		{"chat.message", protocol.EventTurnUserPrompt},
		{"tool.execute.before", protocol.EventToolRequested},
		{"tool.execute.after", protocol.EventToolCompleted},
		{"permission.ask", protocol.EventToolPermissionRequest},
		{"session.created", protocol.EventSessionStarted},
		{"session.idle", protocol.EventTurnCompleted},
		{"experimental.session.compacting", protocol.EventSessionCompacted},
		{"session.compacted", protocol.EventSessionCompacted},
		{"session.error", protocol.EventErrorReported},
	}

	server := httptest.NewServer(api.New(cfg, slog.Default(), st).Handler())
	defer server.Close()
	client := api.Client{BaseURL: server.URL}

	for _, tc := range events {
		payload := protocol.RawJSON(`{"event":{"input":{"sessionID":"sess"},"output":{}},"metadata":{"session_id":"sess","cwd":"/tmp"}}`)
		resp, err := client.Dispatch(api.NewEventRequest("opencode", tc.native, payload))
		if err != nil {
			t.Fatalf("%s dispatch failed: %v", tc.native, err)
		}
		inspection, err := st.InspectEvent(ctx, resp.NormalizedEventID)
		if err != nil {
			t.Fatalf("%s inspection failed: %v", tc.native, err)
		}
		if inspection.Inbound.SourceEventType != tc.native || inspection.Normalized.HitchEventType != tc.hitch {
			t.Fatalf("%s was not persisted with expected mapping: %#v", tc.native, inspection)
		}
		if len(inspection.HandlerInvocations) != 1 || inspection.HandlerInvocations[0].HandlerName != "noop_observer" || inspection.HandlerInvocations[0].Status != protocol.StatusOK {
			t.Fatalf("%s noop observer invocation was not persisted: %#v", tc.native, inspection.HandlerInvocations)
		}
	}
	assistantCompleted := onlyInspection(t, ctx, st, protocol.EventTurnAssistantCompleted)
	if assistantCompleted.Inbound.SourceEventType != "session.idle" {
		t.Fatalf("session.idle did not produce secondary assistant-completed row: %#v", assistantCompleted)
	}
}
```


- [ ] **Step 2: Run test and verify failure or pass after prior tasks**

Run:

```sh
go test ./cmd/hitch -run TestE2EOpenCodeLifecycleHooksDispatchToNoopObserver -count=1
```

Expected after Tasks 1-4: PASS. If it fails, fix the OpenCode runtime path rather than weakening assertions.

- [ ] **Step 3: Run API package regression tests**

Run:

```sh
go test ./internal/api ./cmd/hitch
```

Expected: PASS.

- [ ] **Step 4: Commit**

```sh
git add cmd/hitch/main_test.go internal/api/server_test.go
git commit -m "test: cover opencode lifecycle dispatch"
```

### Task 6: Update docs and examples

**Files:**

- Modify: `docs/events.md`
- Modify: `docs/configuration.md`
- Modify: `docs/installation.md`
- Modify: `docs/handler-development.md`
- Modify: `docs/harness-contracts.md`
- Modify: `docs/index.html`
- Modify: `docs/docs/latest/index.html`
- Modify: `README.md`

- [ ] **Step 1: Update event docs**

In `docs/events.md`, change the opening sentence to include OpenCode:

```md
Hitch turns source hook payloads from Codex, Hermes, Pi, OMP, and OpenCode into one stable event envelope.
```

Add OpenCode to the normalized envelope harness list.

Add this secondary-row entry:

```md
| OpenCode `session.idle` | `["turn.completed", "turn.assistant_completed"]` |
```

Add section after OMP:

```md
## OpenCode source events

OpenCode uses a Hitch-managed TypeScript plugin installed at `~/.config/opencode/plugins/hitch.ts`. The plugin forwards both typed plugin hooks and generic SDK event-stream events to Hitch. Typed control hooks can apply Hitch's native response contract; generic event-stream events are observer-only unless a handler returns `decision.native_response` using the OpenCode adapter response shape.

OpenCode adapter response shape:

```json
{
  "adapter_action": "noop | throw | set | append | inject_context",
  "path": ["args"],
  "value": {},
  "message": "blocked by policy"
}
```

| OpenCode source event | Hitch event | Default | Normalized payload | Adapter response behavior |
| --- | --- | --- | --- | --- |
| `chat.message` | `turn.user_prompt` | Yes | `{input, output}` from the hook plus plugin metadata | `block`, `deny`, or `stop` throws; `inject_context` injects a no-reply context message into the session; `transform` can replace `output.parts` from `updated_input`. |
| `tool.execute.before` | `tool.requested` | Yes | Hook input/output | `block`, `deny`, or `stop` throws; `transform` replaces `output.args` from `updated_input`. |
| `tool.execute.after` | `tool.completed` | Yes | Hook input/output | `replace_result` or `transform` replaces `output.output` from `updated_output`. |
| `permission.ask` | `tool.permission_requested` | Yes | Hook input/output | `allow` sets `output.status` to `allow`; `deny`, `block`, or `stop` sets `output.status` to `deny`. |
| `session.created` | `session.started` | Yes | SDK event payload | Observer-only; native response is ignored unless supplied directly. |
| `session.idle` | `turn.completed`, `turn.assistant_completed` | Yes | SDK event payload | Primary `turn.completed` is dispatched live; secondary `turn.assistant_completed` is audit-only. |
| `session.compacted` | `session.compacted` | Yes | SDK event payload | Observer-only post-compaction lifecycle. |
| `experimental.session.compacting` | `session.compacted` | Yes | Hook input/output | `inject_context` appends to `output.context`; `transform` replaces `output.prompt` from `updated_input`. |
| `session.error` | `error.reported` | Yes | SDK event payload | Observer-only error lifecycle. |
| `command.execute.before` | `turn.user_prompt` | No | Hook input/output | `block`, `deny`, or `stop` throws; `inject_context` injects a no-reply context message. |
| `command.executed` | `turn.user_prompt` | No | SDK event payload | Observer-only command audit. |
| `chat.params`, `chat.headers` | `llm.requested` | No | Hook input/output | `transform` can replace the hook output from `updated_input` or `native_response`. |
| `experimental.text.complete` | `llm.completed` | No | Hook input/output | `replace_result` or `transform` replaces `output.output` from `updated_output`. |
| `shell.env` | `tool.requested` | No | Hook input/output | `transform` can replace `output.env` from `updated_input` or `native_response`. |
| `tool.definition` | `tool.requested` | No | Hook input/output | `transform` can replace the hook output from `updated_input` or `native_response`. |
| `permission.asked`, `permission.updated` | `tool.permission_requested` | No | SDK event payload | Observer-only compatibility names; use `permission.ask` for control. |
| `permission.replied` | `tool.permission_requested` | No | SDK event payload | Observer-only permission response audit. |
| `message.updated`, `message.removed`, `message.part.updated`, `message.part.removed` | `turn.assistant_completed` or `turn.assistant_started` | No | SDK event payload | High-volume message telemetry; configure explicitly when needed. |
| `file.edited` | `tool.completed` | No | SDK event payload | File mutation audit. |
| `file.watcher.updated` | `tool.progress` | No | SDK event payload | File watcher telemetry. |
| `todo.updated` | `turn.started` | No | SDK event payload | Task-state audit. |
| `server.connected`, `server.instance.disposed`, `installation.updated`, `installation.update-available`, `lsp.client.diagnostics`, `lsp.updated`, `tui.prompt.append`, `tui.command.execute`, `tui.toast.show`, `pty.created`, `pty.updated`, `pty.exited`, `pty.deleted`, `vcs.branch.updated` | Existing closest Hitch event by user config | No | SDK event payload | Product/runtime telemetry; leave unmapped by default unless a handler needs it. |
```

- [ ] **Step 2: Update install/config docs**

Update `docs/installation.md`:

- Supported harness lists: `Codex, Hermes, Pi, OMP, and OpenCode`.
- Install path: `~/.config/opencode/plugins/hitch.ts`.
- Hook coverage bullet: `OpenCode plugin installation covers OpenCode typed plugin hooks and SDK lifecycle events and is idempotent.`
- Example commands:

```sh
hitch-client install --only opencode --yes --json
hitch-client uninstall --only opencode --yes --json
```

Update `docs/configuration.md` example:

```toml
[harness.opencode.event_map]
chat.params = "llm.requested"
command.executed = "turn.user_prompt"
```

- [ ] **Step 3: Update handler/harness docs**

Update `docs/handler-development.md` common mappings:

```md
| Inspect or block a tool before it runs | `tool.requested` | Codex `PreToolUse`, Hermes `pre_tool_call`, Pi/OMP `tool_call`, OpenCode `tool.execute.before` |
| Inspect a completed tool result | `tool.completed` | Codex `PostToolUse`, Hermes `transform_tool_result`, Pi/OMP `tool_result`, OpenCode `tool.execute.after` |
| Add context to a user request | `turn.user_prompt` | Codex `UserPromptSubmit`, Hermes `pre_gateway_dispatch`, Pi/OMP `input`, OpenCode `chat.message` |
| Inspect or augment an LLM request | `llm.requested` | Hermes `pre_llm_call`; Pi/OMP `before_provider_request`; OpenCode `chat.params` or `chat.headers` when opted in |
| Run after model or agent turn completion | `turn.completed` | Codex `Stop`, Pi/OMP `turn_end`, OpenCode `session.idle` |
| Handle compaction lifecycle | `session.compacted` | Codex `PreCompact`, Pi `session_before_compact`, OMP `auto_compaction_start`, OpenCode `experimental.session.compacting` |
```

Update `docs/harness-contracts.md`:

```md
- OpenCode: installs a managed TypeScript plugin into `~/.config/opencode/plugins/hitch.ts`; forwards typed plugin hooks and selected SDK events to `/v1/dispatch-sync`; translates normalized decisions into plugin-owned adapter actions (`noop`, `throw`, `set`, `append`, `inject_context`); and fails open if Hitch is unavailable.
```

- [ ] **Step 4: Update README and generated docs HTML**

Update `README.md`, `docs/index.html`, and `docs/docs/latest/index.html` to include OpenCode in supported harness lists, quickstart install examples, and hook contract references. Preserve the existing site style and do not rewrite unrelated marketing copy.

- [ ] **Step 5: Run docs-relevant tests**

Run:

```sh
go test ./cmd/hitch ./cmd/hitch-client ./internal/config
```

Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add docs/events.md docs/configuration.md docs/installation.md docs/handler-development.md docs/harness-contracts.md docs/index.html docs/docs/latest/index.html README.md
git commit -m "docs: document opencode harness support"
```

### Task 7: Final verification

**Files:**

- All files modified by Tasks 1-6.

- [ ] **Step 1: Run focused package tests**

Run:

```sh
go test ./internal/protocol ./internal/config ./internal/harness/opencode ./internal/api ./internal/clientshim ./internal/install ./cmd/hitch ./cmd/hitch-client
```

Expected: PASS.

- [ ] **Step 2: Run all Go tests**

Run:

```sh
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Verify generated plugin content by inspection test**

Run:

```sh
go test ./internal/install -run 'TestApplyOpsInstallsOpenCodePluginIdempotentlyAndBacksUpExistingFile|TestOpenCodePluginContentAppliesThrowSetAppendAndInjectContext' -count=1
```

Expected: PASS.

- [ ] **Step 4: Verify OpenCode API behavior test**

Run:

```sh
go test ./internal/api -run TestDispatchSyncOpenCodeToolBeforeDenyTranslatesToThrow -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit verification-only fixes when they exist**

If Step 1, 2, 3, or 4 required fixes, commit the concrete changed files from the failing step. Example for an API translation fix:

```sh
git add internal/api/server.go internal/api/server_test.go internal/harness/opencode/opencode.go internal/harness/opencode/opencode_test.go
git commit -m "fix: stabilize opencode support"
```

If no files changed after verification, do not create an empty commit.

## Self-Review

Spec coverage:

- OpenCode lifecycle research is captured with source links and event names.
- Server support is covered by protocol/config/API/clientshim tasks.
- Native sync behavior is covered by the OpenCode mapper and plugin adapter response contract.
- Install/uninstall support is covered by managed plugin tasks.
- Docs and examples are covered after implementation tasks.
- Verification covers focused packages and full `go test ./...`.

Placeholder scan:

- No unresolved implementation placeholders are present.
- Event name `todo.updated` appears only as an OpenCode source event.

Type consistency:

- Harness name is consistently `opencode` in protocol, config, API, client shim, installer, docs, and generated plugin payloads.
- Adapter action names are consistently `noop`, `throw`, `set`, `append`, and `inject_context`.
- OpenCode source event names use typed hook names from the plugin interface and SDK event names from generated SDK types.
