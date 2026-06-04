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
	"testing"
	"time"

	"github.com/sage-scm/hitch/internal/api"
	"github.com/sage-scm/hitch/internal/config"
	"github.com/sage-scm/hitch/internal/protocol"
	"github.com/sage-scm/hitch/internal/store"
)

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

func TestInstallDryRunPlansWithoutMutatingFilesystem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ops, err := plannedOps([]string{"codex"}, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(ops) != 1 || ops[0]["action"] != "install" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if _, err := os.Stat(ops[0]["path"]); !os.IsNotExist(err) {
		t.Fatalf("dry-run planning should not create integration file: %v", err)
	}
}

func TestApplyOpsInstallsIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ops, err := plannedOps([]string{"pi"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(ops[0]["path"])
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ops[0]["backup_path"]); !os.IsNotExist(err) {
		t.Fatalf("idempotent install should not create backup: %v", err)
	}

	if err := os.WriteFile(ops[0]["path"], []byte("existing user content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(ops[0]["backup_path"])
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "existing user content\n" {
		t.Fatalf("backup did not preserve previous content: %q", backup)
	}
	current, err := os.ReadFile(ops[0]["path"])
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(first) {
		t.Fatalf("installed content changed unexpectedly: %q", current)
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedIntegrationFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ops, err := plannedOps([]string{"omp"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"omp"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ops[0]["path"]); !os.IsNotExist(err) {
		t.Fatalf("uninstall should remove managed file: %v", err)
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
