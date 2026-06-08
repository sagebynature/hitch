package clientshim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sagebynature/hitch/internal/config"
)

func TestRunNormalizesEmptyStdin(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"event_id":"evt","normalized_event_id":"norm"}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	if err := Run(context.Background(), Options{Harness: "hermes", Event: "post_tool_call", URL: server.URL, Stdin: strings.NewReader(" \n\t"), Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("async dispatch wrote stdout: %q", stdout.String())
	}
	payload, ok := got["source_payload"].(map[string]any)
	if !ok || len(payload) != 0 {
		t.Fatalf("empty stdin was not normalized to object payload: %#v", got["source_payload"])
	}
	if got["source_event_type"] != "post_tool_call" || got["hitch_client_version"] == "" {
		t.Fatalf("missing source metadata: %#v", got)
	}
	if _, ok := got["native_payload"]; ok {
		t.Fatalf("old native payload key was emitted: %#v", got)
	}
}

func TestRunRejectsInvalidJSONWithoutStdout(t *testing.T) {
	var stdout bytes.Buffer
	err := Run(context.Background(), Options{Harness: "hermes", Event: "pre_tool_call", Sync: true, URL: "http://127.0.0.1:1", Stdin: strings.NewReader("{"), Stdout: &stdout})
	if err == nil || !strings.Contains(err.Error(), "stdin must be JSON") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid JSON wrote stdout: %q", stdout.String())
	}
}

func TestRunAsyncDispatchWritesNoStdout(t *testing.T) {
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
	if err := Run(context.Background(), Options{Harness: "hermes", Event: "post_tool_call", URL: server.URL, Stdin: strings.NewReader(`{"name":"Read"}`), Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("server did not receive async event")
	}
	if stdout.Len() != 0 {
		t.Fatalf("async dispatch wrote stdout: %q", stdout.String())
	}
}

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
		t.Fatalf("sync request mode not set: %#v", got)
	}
	if strings.TrimSpace(stdout.String()) != `{"action":"block","message":"policy"}` {
		t.Fatalf("unexpected sync stdout: %q", stdout.String())
	}
}

func TestRunSyncFailureReturnsNativeNoop(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		event   string
	}{
		{name: "Hermes", harness: "hermes", event: "pre_tool_call"},
		{name: "Codex", harness: "codex", event: "PreToolUse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := Run(context.Background(), Options{Harness: tt.harness, Event: tt.event, Sync: true, URL: "http://127.0.0.1:1", Stdin: strings.NewReader(`{"name":"Bash"}`), Stdout: &stdout}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(stdout.String()) != `{}` {
				t.Fatalf("unexpected no-op stdout: %q", stdout.String())
			}
		})
	}
}

func TestRunSyncInvalidOrEmptyResponseReturnsNativeNoop(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "server error", status: http.StatusInternalServerError, body: `{"error":"boom"}`},
		{name: "empty body", status: http.StatusOK, body: ``},
		{name: "invalid JSON", status: http.StatusOK, body: `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/events" {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			var stdout bytes.Buffer
			if err := Run(context.Background(), Options{Harness: "codex", Event: "PreToolUse", Sync: true, URL: server.URL, Stdin: strings.NewReader(`{"name":"Bash"}`), Stdout: &stdout}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(stdout.String()) != `{}` {
				t.Fatalf("unexpected no-op stdout: %q", stdout.String())
			}
		})
	}
}

func TestRunHonorsURL(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"event_id":"evt","normalized_event_id":"norm"}`))
	}))
	defer server.Close()

	if err := Run(context.Background(), Options{Harness: "hermes", Event: "post_tool_call", URL: server.URL, Stdin: strings.NewReader(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("configured URL was not used")
	}
}

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

func TestDefaultURLPrefersEnvOverConfig(t *testing.T) {
	t.Setenv("HITCH_URL", "http://url:1")
	if got := DefaultURL(); got != "http://url:1" {
		t.Fatalf("HITCH_URL should win, got %s", got)
	}
}

func TestDefaultURLUsesConfigWhenEnvAbsent(t *testing.T) {
	t.Setenv("HITCH_URL", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".config", "hitch", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(config.DefaultConfigTOML, "port = 8799", "port = 9876", 1)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := DefaultURL(); got != "http://127.0.0.1:9876" {
		t.Fatalf("config default URL was not used: %s", got)
	}
}

func TestDefaultURLIgnoresInvalidExtensions(t *testing.T) {
	t.Setenv("HITCH_URL", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".config", "hitch", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(config.DefaultConfigTOML, "port = 8799", "port = 9876", 1)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	extDir := filepath.Join(home, ".config", "hitch", "extensions", "broken")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "config.toml"), []byte(`
name = "broken"
entrypoint = "handler:handle"
kind = "observer"
hitch_events = ["tool.completed"]
payload = "hitch"
timeout_ms = 1000
surprise = true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := DefaultURL(); got != "http://127.0.0.1:9876" {
		t.Fatalf("config default URL was not used: %s", got)
	}
}
