package dispatch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/protocol"
)

const fastHandlerTimeoutMS = 5000

func testEnv() protocol.EventEnvelope {
	return protocol.EventEnvelope{HitchVersion: protocol.Version, EventID: "evt", ReceivedAt: time.Now().UTC(), Harness: protocol.HarnessCodex, SourceEventType: "PreToolUse", SourcePayload: protocol.Raw(map[string]interface{}{}), HitchEventType: protocol.EventToolRequested, Payload: protocol.Raw(map[string]interface{}{})}
}

func testRequest(kind string, deadline time.Duration) Request {
	return Request{Envelope: testEnv(), Kind: kind, InboundEventID: "inbound", NormalizedEventID: "normalized", TotalDeadline: deadline}
}

func script(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported in this test")
	}
	p := filepath.Join(t.TempDir(), "handler.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDispatchParsesHandlerResult(t *testing.T) {
	h := script(t, `echo '{"status":"ok","decision":{"behavior":"deny","reason":"no"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{"a": {Command: []string{h}, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS}})
	got := r.Dispatch(context.Background(), testRequest("control", 5*time.Second))
	if got.Aggregate.Decision.Behavior != protocol.BehaviorDeny {
		t.Fatalf("got %s", got.Aggregate.Decision.Behavior)
	}
	if len(got.Invocations) != 1 || got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("bad invocation: %#v", got.Invocations)
	}
}

func TestDispatchInvalidJSONIsError(t *testing.T) {
	h := script(t, `echo nope`)
	r := NewRunner(map[string]config.HandlerConfig{"a": {Command: []string{h}, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS}})
	got := r.Dispatch(context.Background(), testRequest("control", 5*time.Second))
	if got.Invocations[0].Status != protocol.StatusError {
		t.Fatalf("expected error: %#v", got.Invocations[0])
	}
}

func TestDispatchRunsHandlerInWorkingDir(t *testing.T) {
	dir := t.TempDir()
	h := script(t, `pwd > cwd.txt`)
	r := NewRunner(map[string]config.HandlerConfig{"a": {Command: []string{h}, WorkingDir: dir, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS}})
	got := r.Dispatch(context.Background(), testRequest("control", 5*time.Second))
	if got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("expected ok invocation: %#v", got.Invocations[0])
	}
	b, err := os.ReadFile(filepath.Join(dir, "cwd.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != dir {
		t.Fatalf("handler ran in %q, want %q", strings.TrimSpace(string(b)), dir)
	}
}

func TestDispatchTimeout(t *testing.T) {
	h := script(t, `sleep 1`)
	r := NewRunner(map[string]config.HandlerConfig{"a": {Command: []string{h}, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: 10}})
	got := r.Dispatch(context.Background(), testRequest("control", time.Second))
	if got.Invocations[0].Status != protocol.StatusTimeout {
		t.Fatalf("expected timeout: %#v", got.Invocations[0])
	}
}

func TestAggregationDeterministic(t *testing.T) {
	slowAllow := script(t, `sleep 0.05; echo '{"status":"ok","decision":{"behavior":"allow"}}'`)
	fastDeny := script(t, `echo '{"status":"ok","decision":{"behavior":"deny"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{
		"a_allow": {Command: []string{slowAllow}, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS},
		"b_deny":  {Command: []string{fastDeny}, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS},
	})
	got := r.Dispatch(context.Background(), testRequest("control", 5*time.Second))
	if got.Aggregate.Decision.Behavior != protocol.BehaviorDeny {
		t.Fatalf("deny should win: %#v", got.Aggregate.Decision)
	}
}

func TestContextConcatenation(t *testing.T) {
	h1 := script(t, `echo '{"status":"ok","decision":{"behavior":"inject_context","context":"one"}}'`)
	h2 := script(t, `echo '{"status":"ok","decision":{"behavior":"inject_context","context":"two"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{
		"a": {Command: []string{h1}, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS},
		"b": {Command: []string{h2}, HitchEvents: []string{"*"}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS},
	})
	got := r.Dispatch(context.Background(), testRequest("control", 5*time.Second))
	if got.Aggregate.Decision.Context != "one\n\ntwo" {
		t.Fatalf("bad context: %q", got.Aggregate.Decision.Context)
	}
}

func TestSourceEventFilterMatchesAndNonMatches(t *testing.T) {
	h := script(t, `echo '{"status":"ok","decision":{"behavior":"allow"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{
		"match": {Command: []string{h}, HitchEvents: []string{string(protocol.EventToolRequested)}, SourceEvents: []config.SourceEventFilter{{Harness: "codex", SourceEventType: "PreToolUse"}}, Kind: "control", TimeoutMS: fastHandlerTimeoutMS},
	})

	matched := r.Dispatch(context.Background(), testRequest("control", 5*time.Second))
	if len(matched.Invocations) != 1 || matched.Invocations[0].HandlerName != "match" {
		t.Fatalf("expected matching source filter invocation: %#v", matched.Invocations)
	}

	req := testRequest("control", 5*time.Second)
	req.Envelope.SourceEventType = "PostToolUse"
	notMatched := r.Dispatch(context.Background(), req)
	if len(notMatched.Invocations) != 0 || notMatched.Aggregate.Decision.Behavior != protocol.BehaviorNone {
		t.Fatalf("expected source filter non-match: %#v", notMatched)
	}
}

func TestAggregationIgnoresInternalReservedAndSkippedStatuses(t *testing.T) {
	got := aggregate(
		[]Invocation{
			{HandlerName: "reserved", Status: protocol.StatusReserved},
			{HandlerName: "skipped", Status: protocol.StatusSkipped},
			{HandlerName: "allow", Status: protocol.StatusOK, Result: protocol.HandlerResult{Status: protocol.StatusOK, Decision: &protocol.Decision{Behavior: protocol.BehaviorAllow}}},
		},
		nil,
		map[string]config.HandlerConfig{
			"reserved": {OnError: "fail_closed"},
			"skipped":  {OnError: "fail_closed"},
			"allow":    {},
		},
	)
	if got.Decision.Behavior != protocol.BehaviorAllow {
		t.Fatalf("internal statuses should not fail closed: %#v", got)
	}
}

func TestShellCommandPayloadIsPassedAsFirstScriptArg(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(map[string]config.HandlerConfig{
		"shell": {
			Command:     []string{"/bin/sh", "-c", `printf '%s' "$1" > "$CAPTURE_DIR/argv1.json"; printf '%s' "$0" > "$CAPTURE_DIR/argv0.txt"; echo '{"status":"ok","decision":{"behavior":"allow"}}'`},
			HitchEvents: []string{"*"},
			Kind:        "control",
			TimeoutMS:   fastHandlerTimeoutMS,
		},
	})
	req := testRequest("control", 5*time.Second)
	req.Envelope.Payload = protocol.RawJSON(`{ "tool" : { "name" : "Bash" } }`)

	t.Setenv("CAPTURE_DIR", dir)
	got := r.Dispatch(context.Background(), req)
	if len(got.Invocations) != 1 || got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("expected shell payload invocation: %#v", got.Invocations)
	}
	argv1, err := os.ReadFile(filepath.Join(dir, "argv1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv1) != `{"tool":{"name":"Bash"}}` {
		t.Fatalf("argv $1 payload = %s", argv1)
	}
	argv0, err := os.ReadFile(filepath.Join(dir, "argv0.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv0) == `{"tool":{"name":"Bash"}}` {
		t.Fatalf("payload was swallowed as $0")
	}
}

func TestShellCommandWithCustomArgZeroPassesPayloadAsFirstScriptArg(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(map[string]config.HandlerConfig{
		"shell": {
			Command:     []string{"/bin/sh", "-c", `printf '%s' "$1" > "$CAPTURE_DIR/argv1.json"; printf '%s' "$2" > "$CAPTURE_DIR/argv2.txt"; echo '{"status":"ok","decision":{"behavior":"allow"}}'`, "custom0"},
			HitchEvents: []string{"*"},
			Kind:        "control",
			TimeoutMS:   fastHandlerTimeoutMS,
		},
	})
	req := testRequest("control", 5*time.Second)
	req.Envelope.Payload = protocol.RawJSON(`{ "tool" : { "name" : "Bash" } }`)

	t.Setenv("CAPTURE_DIR", dir)
	got := r.Dispatch(context.Background(), req)
	if len(got.Invocations) != 1 || got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("expected shell payload invocation: %#v", got.Invocations)
	}
	argv1, err := os.ReadFile(filepath.Join(dir, "argv1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv1) != `{"tool":{"name":"Bash"}}` {
		t.Fatalf("argv $1 payload = %s", argv1)
	}
	argv2, err := os.ReadFile(filepath.Join(dir, "argv2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(argv2) != 0 {
		t.Fatalf("payload shifted to $2: %s", argv2)
	}
}

func TestBashLoginShellCommandPayloadIsPassedAsFirstScriptArg(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(map[string]config.HandlerConfig{
		"shell": {
			Command:     []string{"bash", "-lc", `printf '%s' "$1" > "$CAPTURE_DIR/argv1.json"; printf '%s' "$0" > "$CAPTURE_DIR/argv0.txt"; echo '{"status":"ok","decision":{"behavior":"allow"}}'`},
			HitchEvents: []string{"*"},
			Kind:        "control",
			TimeoutMS:   fastHandlerTimeoutMS,
		},
	})
	req := testRequest("control", 5*time.Second)
	req.Envelope.Payload = protocol.RawJSON(`{ "tool" : { "name" : "Bash" } }`)

	t.Setenv("CAPTURE_DIR", dir)
	got := r.Dispatch(context.Background(), req)
	if len(got.Invocations) != 1 || got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("expected bash -lc payload invocation: %#v", got.Invocations)
	}
	argv1, err := os.ReadFile(filepath.Join(dir, "argv1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv1) != `{"tool":{"name":"Bash"}}` {
		t.Fatalf("argv $1 payload = %s", argv1)
	}
	argv0, err := os.ReadFile(filepath.Join(dir, "argv0.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv0) == `{"tool":{"name":"Bash"}}` {
		t.Fatalf("payload was swallowed as $0")
	}
}

func TestBashLoginOptionBeforeCommandPayloadIsPassedAsFirstScriptArg(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(map[string]config.HandlerConfig{
		"shell": {
			Command:     []string{"bash", "-l", "-c", `printf '%s' "$1" > "$CAPTURE_DIR/argv1.json"; printf '%s' "$0" > "$CAPTURE_DIR/argv0.txt"; echo '{"status":"ok","decision":{"behavior":"allow"}}'`},
			HitchEvents: []string{"*"},
			Kind:        "control",
			TimeoutMS:   fastHandlerTimeoutMS,
		},
	})
	req := testRequest("control", 5*time.Second)
	req.Envelope.Payload = protocol.RawJSON(`{ "tool" : { "name" : "Bash" } }`)

	t.Setenv("CAPTURE_DIR", dir)
	got := r.Dispatch(context.Background(), req)
	if len(got.Invocations) != 1 || got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("expected bash -l -c payload invocation: %#v", got.Invocations)
	}
	argv1, err := os.ReadFile(filepath.Join(dir, "argv1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv1) != `{"tool":{"name":"Bash"}}` {
		t.Fatalf("argv $1 payload = %s", argv1)
	}
	argv0, err := os.ReadFile(filepath.Join(dir, "argv0.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv0) == `{"tool":{"name":"Bash"}}` {
		t.Fatalf("payload was swallowed as $0")
	}
}

func TestShellCommandScriptIndexScansOptionsUntilCommandMode(t *testing.T) {
	tests := []struct {
		name      string
		command   []string
		wantIndex int
		wantOK    bool
	}{
		{name: "long option before command mode", command: []string{"bash", "--norc", "-c", "script"}, wantIndex: 3, wantOK: true},
		{name: "long option without command mode", command: []string{"bash", "--norc", "script"}, wantOK: false},
		{name: "non option before command mode", command: []string{"bash", "script", "-c", "ignored"}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex, gotOK := shellCommandScriptIndex(tt.command)
			if gotIndex != tt.wantIndex || gotOK != tt.wantOK {
				t.Fatalf("shellCommandScriptIndex(%v) = (%d, %t), want (%d, %t)", tt.command, gotIndex, gotOK, tt.wantIndex, tt.wantOK)
			}
		})
	}
}

func TestIsShellCommandOptionOnlyMatchesShortOptionsWithC(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		{arg: "-c", want: true},
		{arg: "-lc", want: true},
		{arg: "--norc", want: false},
		{arg: "-l", want: false},
	}

	for _, tt := range tests {
		if got := isShellCommandOption(tt.arg); got != tt.want {
			t.Fatalf("isShellCommandOption(%q) = %t, want %t", tt.arg, got, tt.want)
		}
	}
}

func TestSourcePayloadSelectedForArgvAndContextIncludesEvent(t *testing.T) {
	dir := t.TempDir()
	h := script(t, `printf '%s' "$1" > "$CAPTURE_DIR/argv.json"
cat > "$CAPTURE_DIR/stdin.json"
echo '{"status":"ok","decision":{"behavior":"allow"}}'`)
	r := NewRunner(map[string]config.HandlerConfig{
		"source": {Command: []string{h}, HitchEvents: []string{"*"}, SourceEvents: []config.SourceEventFilter{{Harness: "codex", SourceEventType: "PreToolUse"}}, Payload: "source", Kind: "control", TimeoutMS: fastHandlerTimeoutMS},
	})
	req := testRequest("control", 5*time.Second)
	req.Envelope.SourcePayload = protocol.RawJSON(`{ "native" : true, "nested" : { "x" : 1 } }`)
	req.Envelope.Payload = protocol.RawJSON(`{"tool":{"name":"Bash"}}`)
	req.Envelope.SessionID = "session-1"
	req.Envelope.TurnID = "turn-1"
	req.InboundEventID = "in-1"
	req.NormalizedEventID = "norm-1"

	t.Setenv("CAPTURE_DIR", dir)
	got := r.Dispatch(context.Background(), req)
	if len(got.Invocations) != 1 || got.Invocations[0].Status != protocol.StatusOK {
		t.Fatalf("expected source payload invocation: %#v", got.Invocations)
	}
	argv, err := os.ReadFile(filepath.Join(dir, "argv.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv) != `{"native":true,"nested":{"x":1}}` {
		t.Fatalf("argv payload = %s", argv)
	}
	stdin, err := os.ReadFile(filepath.Join(dir, "stdin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var invCtx protocol.InvocationContext
	if err := json.Unmarshal(stdin, &invCtx); err != nil {
		t.Fatal(err)
	}
	if invCtx.HandlerName != "source" || invCtx.Kind != "control" || invCtx.InboundEventID != "in-1" || invCtx.NormalizedEventID != "norm-1" {
		t.Fatalf("bad invocation metadata: %#v", invCtx)
	}
	if invCtx.PayloadKind != "source" || string(invCtx.Payload) != `{"native":true,"nested":{"x":1}}` {
		t.Fatalf("bad selected payload: kind=%q payload=%s", invCtx.PayloadKind, invCtx.Payload)
	}
	if invCtx.Event.Harness != protocol.HarnessCodex || invCtx.Event.SourceEventType != "PreToolUse" || invCtx.Event.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("bad event metadata: %#v", invCtx.Event)
	}
	if string(invCtx.Event.SourcePayload) != `{"native":true,"nested":{"x":1}}` || string(invCtx.Event.Payload) != `{"tool":{"name":"Bash"}}` {
		t.Fatalf("event did not include both payloads: %#v", invCtx.Event)
	}
}
