package claudecode

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

func TestNormalizePreToolUse(t *testing.T) {
	env, err := (Mapper{}).Normalize("PreToolUse", protocol.Raw(map[string]interface{}{
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": "ls -la"},
	}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.HitchEventType != protocol.EventToolRequested {
		t.Fatalf("got %s", env.HitchEventType)
	}
	if env.Harness != protocol.HarnessClaudeCode {
		t.Fatalf("harness = %s", env.Harness)
	}
}

func TestNormalizeCopiesSourceMetadata(t *testing.T) {
	env, err := (Mapper{}).Normalize("PreToolUse", protocol.Raw(map[string]interface{}{
		"session_id":      "session_cc_1",
		"cwd":             "/home/user/project",
		"transcript_path": "/home/user/.claude/transcript.jsonl",
		"tool_name":       "Bash",
	}), protocol.EventToolRequested)
	if err != nil {
		t.Fatal(err)
	}
	if env.SessionID != "session_cc_1" {
		t.Fatalf("session_id = %q", env.SessionID)
	}
	if env.CWD != "/home/user/project" {
		t.Fatalf("cwd = %q", env.CWD)
	}
	if env.TranscriptPath != "/home/user/.claude/transcript.jsonl" {
		t.Fatalf("transcript_path = %q", env.TranscriptPath)
	}
}

func TestNormalizeUserPromptSubmit(t *testing.T) {
	env, err := (Mapper{}).Normalize("UserPromptSubmit", protocol.Raw(map[string]interface{}{
		"session_id": "s1",
		"prompt":     "fix the bug",
		"source":     "user_input",
	}), protocol.EventTurnUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if env.HitchEventType != protocol.EventTurnUserPrompt {
		t.Fatalf("got %s", env.HitchEventType)
	}
}

func TestNormalizeStop(t *testing.T) {
	env, err := (Mapper{}).Normalize("Stop", protocol.Raw(map[string]interface{}{
		"session_id": "s1",
	}), protocol.EventTurnCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if env.HitchEventType != protocol.EventTurnCompleted {
		t.Fatalf("got %s", env.HitchEventType)
	}
}

func TestCapabilityClassifiesSourceEvents(t *testing.T) {
	m := Mapper{}
	controlEvents := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "PostToolUse", "Stop", "SubagentStart", "SubagentStop", "PreCompact", "PostCompact"}
	for _, event := range controlEvents {
		if got := m.Capability(event); got != core.CapabilityControlCapable {
			t.Errorf("%s: capability = %s, want control_capable", event, got)
		}
	}
	observerEvents := []string{"SessionEnd", "StopFailure", "PostToolUseFailure", "Notification", "CwdChanged", "FileChanged"}
	for _, event := range observerEvents {
		if got := m.Capability(event); got != core.CapabilityObserverOnly {
			t.Errorf("%s: capability = %s, want observer_only", event, got)
		}
	}
}

func TestTranslatePreToolUseDeny(t *testing.T) {
	out, err := (Mapper{}).Translate("PreToolUse", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorDeny, Reason: "blocked by policy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	hso, ok := got["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %s", string(out))
	}
	if hso["permissionDecision"] != "deny" {
		t.Fatalf("permissionDecision = %v", hso["permissionDecision"])
	}
	if hso["permissionDecisionReason"] != "blocked by policy" {
		t.Fatalf("permissionDecisionReason = %v", hso["permissionDecisionReason"])
	}
	if hso["hookEventName"] != "PreToolUse" {
		t.Fatalf("hookEventName = %v", hso["hookEventName"])
	}
}

func TestTranslatePreToolUseAllow(t *testing.T) {
	out, err := (Mapper{}).Translate("PreToolUse", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorAllow},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	hso, ok := got["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %s", string(out))
	}
	if hso["permissionDecision"] != "allow" {
		t.Fatalf("permissionDecision = %v", hso["permissionDecision"])
	}
}

func TestTranslatePreToolUseTransform(t *testing.T) {
	out, err := (Mapper{}).Translate("PreToolUse", protocol.AggregateDecision{
		Decision: protocol.Decision{
			Behavior:     protocol.BehaviorTransform,
			UpdatedInput: protocol.Raw(map[string]interface{}{"command": "ls"}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	hso, ok := got["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %s", string(out))
	}
	if hso["permissionDecision"] != "allow" {
		t.Fatalf("permissionDecision = %v", hso["permissionDecision"])
	}
	if hso["updatedInput"] == nil {
		t.Fatal("missing updatedInput")
	}
}

func TestTranslatePermissionRequestDeny(t *testing.T) {
	out, err := (Mapper{}).Translate("PermissionRequest", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorDeny},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	hso, ok := got["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %s", string(out))
	}
	decision, ok := hso["decision"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing decision: %s", string(out))
	}
	if decision["behavior"] != "deny" {
		t.Fatalf("behavior = %v", decision["behavior"])
	}
}

func TestTranslatePermissionRequestAllow(t *testing.T) {
	out, err := (Mapper{}).Translate("PermissionRequest", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorAllow},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	hso, ok := got["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %s", string(out))
	}
	decision, ok := hso["decision"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing decision: %s", string(out))
	}
	if decision["behavior"] != "allow" {
		t.Fatalf("behavior = %v", decision["behavior"])
	}
}

func TestTranslateUserPromptBlock(t *testing.T) {
	out, err := (Mapper{}).Translate("UserPromptSubmit", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorBlock, Reason: "not allowed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["decision"] != "block" {
		t.Fatalf("decision = %v", got["decision"])
	}
	if got["reason"] != "not allowed" {
		t.Fatalf("reason = %v", got["reason"])
	}
}

func TestTranslateUserPromptInjectContext(t *testing.T) {
	out, err := (Mapper{}).Translate("UserPromptSubmit", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorInjectContext, Context: "extra context here"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["additionalContext"] != "extra context here" {
		t.Fatalf("additionalContext = %v", got["additionalContext"])
	}
}

func TestTranslateStopBlock(t *testing.T) {
	out, err := (Mapper{}).Translate("Stop", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorBlock, Reason: "task not complete"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["decision"] != "block" {
		t.Fatalf("decision = %v", got["decision"])
	}
}

func TestTranslatePostToolUseReplaceResult(t *testing.T) {
	out, err := (Mapper{}).Translate("PostToolUse", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorReplaceResult, UpdatedOutput: protocol.Raw("formatted output")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["updatedToolOutput"] != "formatted output" {
		t.Fatalf("updatedToolOutput = %v", got["updatedToolOutput"])
	}
}

func TestTranslateNoneReturnsEmpty(t *testing.T) {
	out, err := (Mapper{}).Translate("PreToolUse", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorNone},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty response, got %s", string(out))
	}
}

func TestTranslateNativeResponsePassthrough(t *testing.T) {
	native := protocol.Raw(map[string]interface{}{"custom": "response"})
	out, err := (Mapper{}).Translate("PreToolUse", protocol.AggregateDecision{
		Decision: protocol.Decision{Behavior: protocol.BehaviorDeny, NativeResponse: native},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["custom"] != "response" {
		t.Fatalf("expected native passthrough, got %s", string(out))
	}
}

func TestKnownSourceEventsCoversAllExpected(t *testing.T) {
	m := Mapper{}
	events := m.KnownSourceEvents()
	expected := []string{
		"SessionStart", "SessionEnd", "Setup",
		"UserPromptSubmit", "UserPromptExpansion", "Stop", "StopFailure",
		"PreToolUse", "PostToolUse", "PostToolUseFailure", "PostToolBatch", "PermissionRequest", "PermissionDenied",
		"SubagentStart", "SubagentStop",
		"PreCompact", "PostCompact", "InstructionsLoaded",
		"Notification", "MessageDisplay", "CwdChanged", "FileChanged", "ConfigChange",
		"Elicitation", "ElicitationResult", "WorktreeCreate", "WorktreeRemove",
		"TaskCreated", "TaskCompleted", "TeammateIdle",
	}
	for _, e := range expected {
		if _, ok := events[e]; !ok {
			t.Errorf("missing known source event %q", e)
		}
	}
}
