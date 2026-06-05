package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prepareClientInstallTest(t *testing.T, harness string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	harnessPath := filepath.Join(dir, harness)
	if err := os.WriteFile(harnessPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	clientPath := filepath.Join(dir, "hitch-client")
	if err := os.WriteFile(clientPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("HITCH_CLIENT_BINARY_PATH", clientPath)
}

func TestRunInstallSubcommandDryRun(t *testing.T) {
	prepareClientInstallTest(t, "codex")

	var stdout bytes.Buffer
	if err := run([]string{"install", "--only", "codex", "--dry-run", "--json"}, strings.NewReader(`{}`), &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("installer output should use process stdout, got injected stdout %q", stdout.String())
	}
}

func TestRunUninstallSubcommandDryRun(t *testing.T) {
	prepareClientInstallTest(t, "hermes")

	if err := run([]string{"uninstall", "--only", "hermes", "--dry-run", "--json"}, strings.NewReader(`{}`), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestRunDispatchesSyncResponse(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/dispatch-sync" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"event_id":"evt","normalized_event_id":"norm","aggregate":{"decision":{"behavior":"allow"}},"native_response":{"permissionDecision":"allow"}}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err := run([]string{"-harness", "codex", "-event", "PreToolUse", "-sync", "-url", server.URL}, strings.NewReader(`{"tool":"bash"}`), &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != `{"permissionDecision":"allow"}` {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if got["harness"] != "codex" || got["source_event_type"] != "PreToolUse" || got["hitch_client_version"] == "" {
		t.Fatalf("unexpected request metadata: %#v", got)
	}
}

func TestRunAsyncPrintsNothing(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/v1/events" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"event_id":"evt","normalized_event_id":"norm"}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	if err := run([]string{"-harness", "hermes", "-event", "post_tool_call", "-url", server.URL}, strings.NewReader(`{}`), &stdout); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("server did not receive async event")
	}
	if stdout.Len() != 0 {
		t.Fatalf("async run wrote stdout: %q", stdout.String())
	}
}

func TestRunInvalidJSONReturnsErrorWithoutStdout(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"-harness", "hermes", "-event", "pre_tool_call", "-sync", "-url", "http://127.0.0.1:1"}, strings.NewReader("{"), &stdout)
	if err == nil || !strings.Contains(err.Error(), "stdin must be JSON") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid JSON wrote stdout: %q", stdout.String())
	}
}
