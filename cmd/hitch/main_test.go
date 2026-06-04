package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
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

func runAdapterForTest(t *testing.T, args []string, input string) string {
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

	adapter(args)
	_ = outW.Close()
	out, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
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

func TestAdapterDispatchSyncPreservesNativePayloadAndPrintsNativeResponse(t *testing.T) {

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/dispatch-sync" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"event_id":"evt","normalized_event_id":"norm","aggregate":{"decision":{"behavior":"deny","reason":"policy"}},"native_response":{"permissionDecision":"deny","reason":"policy"}}`))
	}))
	defer server.Close()

	out := runAdapterForTest(t, []string{"-harness", "codex", "-event", "PreToolUse", "-sync", "-url", server.URL}, `{"tool":"bash","input":{"command":"pwd"}}`)

	var native map[string]any
	if err := json.Unmarshal([]byte(out), &native); err != nil {
		t.Fatalf("adapter stdout is not JSON: %v; stdout=%q", err, out)
	}
	if native["permissionDecision"] != "deny" || native["reason"] != "policy" {
		t.Fatalf("unexpected native response: %#v", native)
	}
	if got["harness"] != "codex" || got["native_event_type"] != "PreToolUse" {
		t.Fatalf("unexpected request metadata: %#v", got)
	}
	payload, ok := got["native_payload"].(map[string]any)
	if !ok {
		t.Fatalf("native_payload was not an object: %#v", got["native_payload"])
	}
	if payload["tool"] != "bash" {
		t.Fatalf("native payload was not preserved: %#v", payload)
	}
}

func TestAdapterAsyncPostsEventAndPrintsNothing(t *testing.T) {

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/v1/events" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"event_id":"evt","normalized_event_id":"norm"}`))
	}))
	defer server.Close()

	out := runAdapterForTest(t, []string{"-harness", "hermes", "-event", "post_tool_call", "-url", server.URL}, `{"name":"Read"}`)

	if out != "" {
		t.Fatalf("async adapter wrote stdout: %q", out)
	}
	if !called {
		t.Fatal("adapter did not post event")
	}
}

func TestAdapterFailsOpenWhenHitchIsUnreachable(t *testing.T) {

	out := runAdapterForTest(t, []string{"-harness", "hermes", "-event", "pre_tool_call", "-sync", "-url", "http://127.0.0.1:1"}, `{"name":"Bash"}`)

	var native map[string]any
	if err := json.Unmarshal([]byte(out), &native); err != nil {
		t.Fatalf("unreachable Hitch should emit native no-op JSON, got %q: %v", out, err)
	}
	if len(native) != 0 {
		t.Fatalf("unreachable Hitch should emit no-op response, got %#v", native)
	}
}

func TestNoopObserverHandlerConsumesEnvelopeAndReturnsNone(t *testing.T) {
	out := runNoopObserverForTest(t, `{"hitch_version":"0.1.0","event_id":"evt","received_at":"2026-06-04T00:00:00Z","harness":"codex","native_event_type":"SessionStart","native_payload":{},"hitch_event_type":"session.started","payload":{}}`)

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

func addFakeCommand(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func prepareInstallerTest(t *testing.T, harness string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HITCH_BINARY_PATH", filepath.Join(t.TempDir(), "hitch"))
	return addFakeCommand(t, harness)
}

func TestInstallDryRunPlansWithoutMutatingFilesystem(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(ops) != 2 || ops[0].Action != "seed_config" || ops[1].Action != "install_codex_hook" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if _, err := os.Stat(ops[0].Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run planning should not create config file: %v", err)
	}
	if _, err := os.Stat(ops[1].Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run planning should not create Codex hooks file: %v", err)
	}
}

func TestApplyOpsSeedsUserConfigWithoutOverwritingExistingFile(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	cfgPath := config.ExpandHome(config.DefaultPath)
	seeded, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(seeded) != config.DefaultConfigTOML {
		t.Fatalf("seeded config does not match default config")
	}
	if _, err := config.Parse(seeded); err != nil {
		t.Fatalf("seeded config is invalid: %v", err)
	}

	const existing = "user-owned config\n"
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != existing {
		t.Fatalf("installer overwrote existing user config: %q", current)
	}
}

func TestApplyOpsInstallsCodexHookIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[1]
	first, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range codexLifecycleEvents {
		needle := "adapter -harness codex -event " + event + " -sync"
		if !strings.Contains(string(first), needle) {
			t.Fatalf("codex %s hook was not installed: %s", event, first)
		}
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookOp.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("idempotent install should not create backup: %v", err)
	}

	existing := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"existing"}]}]}}` + "\n"
	if err := os.WriteFile(hookOp.Path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(hookOp.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != existing {
		t.Fatalf("backup did not preserve previous content: %q", backup)
	}
	current, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "existing") || !strings.Contains(string(current), "adapter -harness codex") {
		t.Fatalf("installed content did not preserve existing hook and add Hitch hook: %s", current)
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedCodexHook(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"codex"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(ops[1].Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current), "adapter -harness codex") {
		t.Fatalf("uninstall should remove managed Codex hook: %s", current)
	}

}

func TestApplyOpsInstallsHermesHooksIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "hermes")

	ops, err := plannedOps([]string{"hermes"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[0].Action != "seed_config" || ops[1].Action != "install_hermes_hook" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[1]
	first, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range hermesHookEvents {
		needle := "adapter -harness hermes -event " + event + " -sync"
		if !strings.Contains(string(first), needle) {
			t.Fatalf("hermes %s hook was not installed: %s", event, first)
		}
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookOp.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("idempotent Hermes install should not create backup: %v", err)
	}

	existing := "model: test\nhooks:\n  pre_tool_call:\n    - matcher: terminal\n      command: existing\nhooks_auto_accept: false\n"
	if err := os.WriteFile(hookOp.Path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(hookOp.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != existing {
		t.Fatalf("backup did not preserve previous Hermes config: %q", backup)
	}
	current, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "command: existing") || !strings.Contains(string(current), "adapter -harness hermes") || !strings.Contains(string(current), "hooks_auto_accept: false") {
		t.Fatalf("Hermes install did not preserve existing config and add Hitch hooks: %s", current)
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedHermesHooks(t *testing.T) {
	prepareInstallerTest(t, "hermes")

	ops, err := plannedOps([]string{"hermes"}, false)
	if err != nil {
		t.Fatal(err)
	}
	hookOp := ops[1]
	existing := "hooks:\n  pre_tool_call:\n    - matcher: terminal\n      command: existing\n"
	if err := os.MkdirAll(filepath.Dir(hookOp.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookOp.Path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"hermes"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current), "adapter -harness hermes") {
		t.Fatalf("uninstall should remove managed Hermes hooks: %s", current)
	}
	if !strings.Contains(string(current), "command: existing") {
		t.Fatalf("uninstall should preserve user Hermes hooks: %s", current)
	}
}

func TestDetectHarnessesReportsAvailabilityAndSupport(t *testing.T) {
	path := prepareInstallerTest(t, "codex")

	detections := detectHarnesses()
	var codex harnessDetection
	for _, d := range detections {
		if d.Harness == "codex" {
			codex = d
			break
		}
	}
	if !codex.Available || codex.BinaryPath != path || !codex.Supported {
		t.Fatalf("unexpected codex detection: %#v", codex)
	}
}

func TestPlannedOpsSkipsUnsupportedAvailableHarness(t *testing.T) {
	prepareInstallerTest(t, "pi")

	ops, err := plannedOps([]string{"pi"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[1].Action != "skip" || ops[1].Status != "skipped" {
		t.Fatalf("unexpected unsupported harness plan: %#v", ops)
	}
}

func TestPlannedOpsRejectsUnknownHarness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := plannedOps([]string{"unknown"}, false); err == nil {
		t.Fatal("expected unsupported harness error")
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
	env := protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt_replay", ReceivedAt: now, Harness: protocol.HarnessCodex, NativeEventType: "PreToolUse", NativePayload: protocol.Raw(map[string]interface{}{"tool": "bash"}), HitchEventType: protocol.EventToolRequested, Payload: protocol.Raw(map[string]interface{}{"tool": "bash"})}
	if err := st.InsertInbound(ctx, store.InboundEvent{ID: "in_replay", ReceivedAt: now, Harness: env.Harness, NativeEventType: env.NativeEventType, NativePayload: env.NativePayload}); err != nil {
		t.Fatal(err)
	}
	normalizedID := "norm_replay"
	if err := st.InsertNormalized(ctx, store.NormalizedEvent{ID: normalizedID, InboundEventID: "in_replay", HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertHandlerInvocation(ctx, store.HandlerInvocation{ID: "handler_original", NormalizedEventID: normalizedID, HandlerName: "original", Mode: "sync", StartedAt: now, CompletedAt: now, Status: protocol.StatusOK, Output: protocol.Raw(map[string]interface{}{"status": "ok"}), Decision: protocol.Raw(map[string]interface{}{"behavior": "none"})}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertNativeResponse(ctx, store.NativeResponse{ID: "native_original", NormalizedEventID: normalizedID, Harness: env.Harness, NativeEventType: env.NativeEventType, Response: protocol.Raw(map[string]interface{}{}), EmittedAt: now}); err != nil {
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

func TestE2ECodexAdapterDispatchesThroughPublicAPIAndPersists(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "events.sqlite")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	t.Setenv("HANDLER_JSON", `{"status":"ok","decision":{"behavior":"deny","reason":"policy"}}`)
	server := httptest.NewServer(api.New(e2eConfig(t, dbPath), slog.Default(), st).Handler())
	defer server.Close()

	out := runAdapterForTest(t, []string{"-harness", "codex", "-event", "PreToolUse", "-sync", "-url", server.URL}, `{"tool":"bash","input":{"command":"rm -rf /"}}`)

	var native map[string]any
	if err := json.Unmarshal([]byte(out), &native); err != nil {
		t.Fatalf("codex adapter stdout is not JSON: %v; output=%q", err, out)
	}
	if native["permissionDecision"] != "deny" || native["permissionDecisionReason"] != "policy" {
		t.Fatalf("codex deny was not translated to native response: %#v", native)
	}
	inspection := onlyInspection(t, ctx, st, protocol.EventToolRequested)
	if len(inspection.HandlerInvocations) != 1 || inspection.HandlerInvocations[0].Status != protocol.StatusOK {
		t.Fatalf("handler invocation was not persisted: %#v", inspection.HandlerInvocations)
	}
	if len(inspection.NativeResponses) != 1 {
		t.Fatalf("native response was not persisted: %#v", inspection.NativeResponses)
	}
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
		if inspection.Inbound.NativeEventType != event.native || inspection.Normalized.HitchEventType != event.hitch {
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

func TestE2EHermesAdapterDispatchesBlockThroughPublicAPI(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "events.sqlite")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	t.Setenv("HANDLER_JSON", `{"status":"ok","decision":{"behavior":"block","reason":"blocked"}}`)
	server := httptest.NewServer(api.New(e2eConfig(t, dbPath), slog.Default(), st).Handler())
	defer server.Close()

	out := runAdapterForTest(t, []string{"-harness", "hermes", "-event", "pre_tool_call", "-sync", "-url", server.URL}, `{"tool_name":"bash"}`)

	var native map[string]any
	if err := json.Unmarshal([]byte(out), &native); err != nil {
		t.Fatalf("hermes adapter stdout is not JSON: %v; output=%q", err, out)
	}
	if native["action"] != "block" || native["message"] != "blocked" {
		t.Fatalf("hermes block was not translated to native response: %#v", native)
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

func e2eNoopConfig(t *testing.T, dbPath string) config.Config {
	t.Helper()
	command := "HITCH_TEST_NOOP_HANDLER=1 " + shellQuote(os.Args[0])
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

[handlers.noop_observer]
command = ["/bin/sh", "-c", ` + tomlString(t, command) + `]
events = ["*"]
mode = "sync"
timeout_ms = 5000
on_error = "fail_open"
on_timeout = "fail_open"
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func tomlString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
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
