package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sagebynature/hitch/internal/api"
	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/protocol"
	"github.com/sagebynature/hitch/internal/store"
)

func init() {
	if os.Getenv("HITCH_TEST_NOOP_HANDLER") == "1" {
		noopObserverHandler()
		os.Exit(0)
	}
}

func runNoopObserverForTest(t *testing.T, input string) string {
	t.Helper()

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	_, _ = inW.WriteString(input)
	_ = inW.Close()
	os.Stdin = inR
	os.Stdout = outW
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		_ = inR.Close()
		_ = outR.Close()
	}()

	noopObserverHandler()
	_ = outW.Close()
	out, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func captureStdoutForTest(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = outW
	defer func() {
		os.Stdout = oldStdout
		_ = outR.Close()
	}()

	fn()
	_ = outW.Close()
	out, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRootHelpListsCommandsWithoutAdapter(t *testing.T) {
	var out strings.Builder
	printHitchHelp(&out)
	text := out.String()
	for _, want := range []string{"hitch <command>", "serve", "handler", "inspect-event", "Use hitch-client"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "adapter") {
		t.Fatalf("hitch help should not mention removed adapter command:\n%s", text)
	}
}

func TestSubcommandHelp(t *testing.T) {
	var out strings.Builder
	printHitchHelp(&out, "serve")
	text := out.String()
	for _, want := range []string{"Run the local Hitch API server", "--config", "hitch serve"} {
		if !strings.Contains(text, want) {
			t.Fatalf("serve help missing %q:\n%s", want, text)
		}
	}
}

func TestNoopObserverHandlerConsumesEnvelopeAndReturnsNone(t *testing.T) {
	out := runNoopObserverForTest(t, `{"hitch_version":"0.1.0","event_id":"evt","received_at":"2026-06-04T00:00:00Z","harness":"codex","source_event_type":"SessionStart","source_payload":{},"hitch_event_type":"session.started","payload":{}}`)

	var result protocol.HandlerResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("noop observer stdout is not handler JSON: %v; stdout=%q", err, out)
	}
	if result.Status != protocol.StatusOK {
		t.Fatalf("noop observer returned non-ok status: %#v", result)
	}
	if result.Decision == nil || result.Decision.Behavior != protocol.BehaviorNone {
		t.Fatalf("noop observer should not alter control flow: %#v", result.Decision)
	}
}

func TestHitchInstallSubcommandIsRejected(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "install", "--dry-run", "--json")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("hitch install unexpectedly succeeded: %s", out)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Fatalf("hitch install did not print daemon usage: %s", out)
	}
}

func TestHitchAdapterSubcommandIsRejected(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "adapter", "--help")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("hitch adapter unexpectedly succeeded: %s", out)
	}
	if strings.Contains(string(out), "Dispatch one source event") {
		t.Fatalf("hitch adapter exposed dispatch help after removal: %s", out)
	}
}

func TestInspectEventCLIReturnsAuditRecords(t *testing.T) {
	ctx := context.Background()
	dbPath, cfgPath, normalizedID := seedReplayFixture(t, ctx, "")

	out := captureStdoutForTest(t, func() {
		inspectEvent([]string{"-config", cfgPath, normalizedID})
	})

	var inspection store.EventInspection
	if err := json.Unmarshal([]byte(out), &inspection); err != nil {
		t.Fatalf("inspect-event output is not JSON: %v; output=%q", err, out)
	}
	if inspection.Inbound.ID == "" || inspection.Normalized.ID != normalizedID {
		t.Fatalf("inspect-event omitted event records from %s: %#v", dbPath, inspection)
	}
	if len(inspection.HandlerInvocations) != 1 || len(inspection.NativeResponses) != 1 {
		t.Fatalf("inspect-event omitted related records: %#v", inspection)
	}
}

func TestReplayDryRunDoesNotCreateInvocationAndReplayRecordsMetadata(t *testing.T) {
	ctx := context.Background()
	handlerJSON := `{"status":"ok","decision":{"behavior":"allow"}}`
	dbPath, cfgPath, normalizedID := seedReplayFixture(t, ctx, handlerJSON)

	_ = captureStdoutForTest(t, func() {
		replay([]string{"-config", cfgPath, "-dry-run", normalizedID})
	})
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := st.InspectEvent(ctx, normalizedID)
	_ = st.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.HandlerInvocations) != 1 {
		t.Fatalf("dry-run replay created handler invocation: %#v", inspection.HandlerInvocations)
	}

	_ = captureStdoutForTest(t, func() {
		replay([]string{"-config", cfgPath, normalizedID})
	})
	st, err = store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err = st.InspectEvent(ctx, normalizedID)
	_ = st.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.HandlerInvocations) != 2 {
		t.Fatalf("replay did not create exactly one new invocation: %#v", inspection.HandlerInvocations)
	}
	replayed := inspection.HandlerInvocations[1]
	if replayed.ReplaySourceID != normalizedID {
		t.Fatalf("replay invocation missing source id: %#v", replayed)
	}
	if replayed.Status != protocol.StatusOK {
		t.Fatalf("replay handler did not run successfully: %#v", replayed)
	}
}

func seedReplayFixture(t *testing.T, ctx context.Context, handlerJSON string) (string, string, string) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "events.sqlite")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC()
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_replay", ReceivedAt: now, Harness: protocol.HarnessCodex, SourceEventType: "PreToolUse", SourcePayload: protocol.Raw(map[string]interface{}{"tool": "bash"}), HitchEventType: protocol.EventToolRequested, Payload: protocol.Raw(map[string]interface{}{"tool": "bash"})}
	if err := st.InsertInbound(ctx, store.InboundEvent{ID: "in_replay", ReceivedAt: now, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload}); err != nil {
		t.Fatal(err)
	}
	normalizedID := "norm_replay"
	if err := st.InsertNormalized(ctx, store.NormalizedEvent{ID: normalizedID, InboundEventID: "in_replay", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertHandlerInvocation(ctx, store.HandlerInvocation{ID: "handler_original", NormalizedEventID: normalizedID, HandlerName: "original", Mode: "sync", StartedAt: now, CompletedAt: now, Status: protocol.StatusOK, Output: protocol.Raw(map[string]interface{}{"status": "ok"}), Decision: protocol.Raw(map[string]interface{}{"behavior": "none"})}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertNativeResponse(ctx, store.NativeResponse{ID: "native_original", NormalizedEventID: normalizedID, Harness: env.Harness, SourceEventType: env.SourceEventType, Response: protocol.Raw(map[string]interface{}{}), EmittedAt: now}); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.toml")
	handlerBlock := ""
	if handlerJSON != "" {
		handlerBlock = `
[handlers.replay]
command = ["/bin/sh", "-c", "printf '%s' \"$HANDLER_JSON\""]
events = ["tool.requested"]
mode = "sync"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
`
	}
	configText := `[server]
host = "127.0.0.1"
port = 8799
max_request_bytes = 1048576

[log]
level = "info"
format = "json"
include_native_payload = false

[log.stdout]
enabled = true

[log.file]
enabled = false
path = "` + filepath.ToSlash(filepath.Join(dir, "hitch.log")) + `"
max_size_mb = 100
max_backups = 10
max_age_days = 14
compress = true

[log.otlp]
enabled = false
endpoint = "http://127.0.0.1:4318"
protocol = "http/protobuf"

[audit]
enabled = true
backend = "sqlite"

[audit.sqlite]
path = "` + filepath.ToSlash(dbPath) + `"

[harness.codex]
enabled = true
[harness.hermes]
enabled = true
[harness.pi]
enabled = true
[harness.omp]
enabled = true
` + handlerBlock
	if err := os.WriteFile(cfgPath, []byte(configText), 0o644); err != nil {
		t.Fatal(err)
	}
	if handlerJSON != "" {
		t.Setenv("HANDLER_JSON", handlerJSON)
	}
	return dbPath, cfgPath, normalizedID
}

func TestE2ECodexLifecycleHooksDispatchToNoopObserver(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "events.sqlite")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := httptest.NewServer(api.New(e2eNoopConfig(t, dbPath), slog.Default(), st).Handler())
	defer server.Close()
	client := api.Client{BaseURL: server.URL}

	events := []struct {
		native string
		hitch  protocol.EventType
	}{
		{"SessionStart", protocol.EventSessionStarted},
		{"SubagentStart", protocol.EventSubagentStarted},
		{"UserPromptSubmit", protocol.EventTurnUserPrompt},
		{"PreToolUse", protocol.EventToolRequested},
		{"PermissionRequest", protocol.EventToolPermissionRequest},
		{"PostToolUse", protocol.EventToolCompleted},
		{"PreCompact", protocol.EventSessionCompacted},
		{"PostCompact", protocol.EventSessionCompacted},
		{"SubagentStop", protocol.EventSubagentCompleted},
		{"Stop", protocol.EventTurnCompleted},
	}
	for _, event := range events {
		resp, err := client.Dispatch(api.NewEventRequest("codex", event.native, codexLifecyclePayload(event.native)))
		if err != nil {
			t.Fatalf("%s dispatch failed: %v", event.native, err)
		}
		if string(resp.NativeResponse) != "{}" {
			t.Fatalf("%s noop observer should produce empty native response, got %s", event.native, string(resp.NativeResponse))
		}
		inspection, err := st.InspectEvent(ctx, resp.NormalizedEventID)
		if err != nil {
			t.Fatalf("%s inspection failed: %v", event.native, err)
		}
		if inspection.Inbound.SourceEventType != event.native || inspection.Normalized.HitchEventType != event.hitch {
			t.Fatalf("%s was not persisted with expected mapping: %#v", event.native, inspection)
		}
		if len(inspection.HandlerInvocations) != 1 || inspection.HandlerInvocations[0].HandlerName != "noop_observer" || inspection.HandlerInvocations[0].Status != protocol.StatusOK {
			t.Fatalf("%s noop observer invocation was not persisted: %#v", event.native, inspection.HandlerInvocations)
		}
		var decision protocol.Decision
		if err := json.Unmarshal(inspection.HandlerInvocations[0].Decision, &decision); err != nil {
			t.Fatalf("%s decision JSON was not persisted: %v", event.native, err)
		}
		if decision.Behavior != protocol.BehaviorNone {
			t.Fatalf("%s noop observer changed control flow: %#v", event.native, decision)
		}
		if len(inspection.NativeResponses) != 1 {
			t.Fatalf("%s native response was not persisted: %#v", event.native, inspection.NativeResponses)
		}
	}
}

func e2eConfig(t *testing.T, dbPath string) config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(`[server]
host = "127.0.0.1"
port = 8799
max_request_bytes = 1048576

[log]
level = "info"
format = "json"
include_native_payload = false

[log.stdout]
enabled = true

[log.file]
enabled = false
path = "` + filepath.ToSlash(filepath.Join(filepath.Dir(dbPath), "hitch.log")) + `"
max_size_mb = 100
max_backups = 10
max_age_days = 14
compress = true

[log.otlp]
enabled = false
endpoint = "http://127.0.0.1:4318"
protocol = "http/protobuf"

[audit]
enabled = true
backend = "sqlite"

[audit.sqlite]
path = "` + filepath.ToSlash(dbPath) + `"

[harness.codex]
enabled = true
[harness.hermes]
enabled = true
[harness.pi]
enabled = true
[harness.omp]
enabled = true

[handlers.policy]
command = ["/bin/sh", "-c", "printf '%s' \"$HANDLER_JSON\""]
events = ["tool.requested"]
mode = "sync"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func e2eNoopConfig(t *testing.T, dbPath string) config.Config {
	t.Helper()
	command := "HITCH_TEST_NOOP_HANDLER=1 " + shellQuote(os.Args[0])
	cfg, err := config.Parse([]byte(config.DefaultConfigTOML))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Audit.SQLite.Path = dbPath
	cfg.Log.File.Path = filepath.ToSlash(filepath.Join(filepath.Dir(dbPath), "hitch.log"))
	cfg.Handlers = map[string]config.HandlerConfig{
		"noop_observer": {
			Command:   []string{"/bin/sh", "-c", command},
			Events:    []string{"*"},
			Mode:      "sync",
			TimeoutMS: 5000,
			OnError:   "fail_open",
			OnTimeout: "fail_open",
		},
	}
	return cfg
}

func codexLifecyclePayload(event string) protocol.RawJSON {
	switch event {
	case "SessionStart":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"SessionStart","model":"test","permission_mode":"default","source":"startup"}`)
	case "SubagentStart":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"SubagentStart","model":"test","turn_id":"turn","agent_id":"agent","agent_type":"task","permission_mode":"default"}`)
	case "UserPromptSubmit":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"UserPromptSubmit","model":"test","turn_id":"turn","permission_mode":"default","prompt":"hello"}`)
	case "PreToolUse":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"PreToolUse","model":"test","turn_id":"turn","permission_mode":"default","tool_name":"Bash","tool_use_id":"tool","tool_input":{"command":"pwd"}}`)
	case "PermissionRequest":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"PermissionRequest","model":"test","turn_id":"turn","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"pwd","description":"approval"}}`)
	case "PostToolUse":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"PostToolUse","model":"test","turn_id":"turn","permission_mode":"default","tool_name":"Bash","tool_use_id":"tool","tool_input":{"command":"pwd"},"tool_response":{"stdout":"/tmp","exit_code":0}}`)
	case "PreCompact":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"PreCompact","model":"test","turn_id":"turn","trigger":"manual"}`)
	case "PostCompact":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"PostCompact","model":"test","turn_id":"turn","trigger":"manual"}`)
	case "SubagentStop":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"SubagentStop","model":"test","turn_id":"turn","permission_mode":"default","agent_id":"agent","agent_type":"task","agent_transcript_path":null,"stop_hook_active":false,"last_assistant_message":null}`)
	case "Stop":
		return protocol.RawJSON(`{"session_id":"session","transcript_path":null,"cwd":"/tmp","hook_event_name":"Stop","model":"test","turn_id":"turn","permission_mode":"default","stop_hook_active":false,"last_assistant_message":null}`)
	default:
		panic("missing Codex lifecycle payload")
	}
}

func onlyInspection(t *testing.T, ctx context.Context, st *store.Store, eventType protocol.EventType) store.EventInspection {
	t.Helper()
	id, err := st.LatestEventIDByType(ctx, eventType)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := st.InspectEvent(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return inspection
}
