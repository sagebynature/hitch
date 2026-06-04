package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
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
