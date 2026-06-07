package core

import (
	"encoding/json"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func decodePayload(t *testing.T, raw protocol.RawJSON) map[string]interface{} {
	t.Helper()
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func nestedMap(t *testing.T, src map[string]interface{}, keys ...string) map[string]interface{} {
	t.Helper()
	cur := src
	for _, key := range keys {
		next, ok := cur[key].(map[string]interface{})
		if !ok {
			t.Fatalf("missing object path %v in %#v", keys, src)
		}
		cur = next
	}
	return cur
}

func TestParseToolRequestedAcrossHarnesses(t *testing.T) {
	tests := []struct {
		name       string
		harness    protocol.Harness
		sourceType string
		source     protocol.RawJSON
		toolName   string
		command    string
	}{
		{name: "codex", harness: protocol.HarnessCodex, sourceType: "PreToolUse", source: protocol.RawJSON(`{"tool_name":"Bash","tool_input":{"command":"pwd"}}`), toolName: "Bash", command: "pwd"},
		{name: "hermes", harness: protocol.HarnessHermes, sourceType: "pre_tool_call", source: protocol.RawJSON(`{"tool_name":"bash","input":{"command":"pwd"}}`), toolName: "bash", command: "pwd"},
		{name: "pi", harness: protocol.HarnessPi, sourceType: "tool_call", source: protocol.RawJSON(`{"name":"bash","input":{"command":"pwd"}}`), toolName: "bash", command: "pwd"},
		{name: "omp", harness: protocol.HarnessOMP, sourceType: "tool_call", source: protocol.RawJSON(`{"toolName":"bash","input":{"command":"pwd"}}`), toolName: "bash", command: "pwd"},
		{name: "opencode", harness: protocol.HarnessOpenCode, sourceType: "tool.execute.before", source: protocol.RawJSON(`{"input":{"tool":"bash","sessionID":"sess"},"output":{"args":{"command":"pwd"}}}`), toolName: "bash", command: "pwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := decodePayload(t, ParseTypedPayload(tt.harness, tt.sourceType, tt.source, protocol.EventToolRequested))
			tool := nestedMap(t, payload, "tool")
			if tool["name"] != tt.toolName || tool["command"] != tt.command || tool["kind"] != "shell" {
				t.Fatalf("bad tool payload: %#v", tool)
			}
			if _, ok := tool["input"].(map[string]interface{}); !ok {
				t.Fatalf("tool input missing: %#v", tool)
			}
		})
	}
}

func TestParseTurnUserPromptAcrossHarnesses(t *testing.T) {
	tests := []struct {
		name       string
		harness    protocol.Harness
		sourceType string
		source     protocol.RawJSON
		prompt     string
	}{
		{name: "codex", harness: protocol.HarnessCodex, sourceType: "UserPromptSubmit", source: protocol.RawJSON(`{"prompt":"Deploy this"}`), prompt: "Deploy this"},
		{name: "hermes", harness: protocol.HarnessHermes, sourceType: "pre_gateway_dispatch", source: protocol.RawJSON(`{"message":"rewrite: summarize"}`), prompt: "rewrite: summarize"},
		{name: "pi", harness: protocol.HarnessPi, sourceType: "input", source: protocol.RawJSON(`{"text":"hello"}`), prompt: "hello"},
		{name: "opencode", harness: protocol.HarnessOpenCode, sourceType: "chat.message", source: protocol.RawJSON(`{"output":{"parts":[{"type":"text","text":"hello"},{"type":"text","text":" world"}]}}`), prompt: "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := decodePayload(t, ParseTypedPayload(tt.harness, tt.sourceType, tt.source, protocol.EventTurnUserPrompt))
			turn := nestedMap(t, payload, "turn")
			if turn["prompt"] != tt.prompt {
				t.Fatalf("prompt = %#v, want %q; payload %#v", turn["prompt"], tt.prompt, turn)
			}
		})
	}
}

func TestParseAssistantCompletedAcrossHarnesses(t *testing.T) {
	tests := []struct {
		name       string
		harness    protocol.Harness
		sourceType string
		source     protocol.RawJSON
		text       string
		model      string
		provider   string
	}{
		{name: "codex stop", harness: protocol.HarnessCodex, sourceType: "Stop", source: protocol.RawJSON(`{"last_assistant_message":"HITCH_PROBE_CODEX_OK","model":"gpt-5.5"}`), text: "HITCH_PROBE_CODEX_OK", model: "gpt-5.5"},
		{name: "hermes transform output", harness: protocol.HarnessHermes, sourceType: "transform_llm_output", source: protocol.RawJSON(`{"extra":{"response_text":"HITCH_PROBE_HERMES_OK","model":"gpt-5.5"}}`), text: "HITCH_PROBE_HERMES_OK", model: "gpt-5.5"},
		{name: "pi turn end", harness: protocol.HarnessPi, sourceType: "turn_end", source: protocol.RawJSON(`{"message":{"content":[{"type":"text","text":"HITCH_PROBE_PI_OK"}],"provider":"openai-codex","model":"gpt-5.5","stopReason":"stop","responseId":"resp_pi","usage":{"input":428,"output":10,"cacheRead":0,"cacheWrite":0,"totalTokens":438,"cost":{"input":0.00214,"output":0.0003,"cacheRead":0,"cacheWrite":0,"total":0.00244}}}}`), text: "HITCH_PROBE_PI_OK", model: "gpt-5.5", provider: "openai-codex"},
		{name: "omp turn end", harness: protocol.HarnessOMP, sourceType: "turn_end", source: protocol.RawJSON(`{"message":{"content":[{"type":"thinking","thinking":""},{"type":"text","text":"HITCH_PROBE_OMP_OK"}],"provider":"openai-codex","model":"gpt-5.5","stopReason":"stop","responseId":"resp_omp","usage":{"input":12626,"output":32,"reasoningTokens":19,"cacheRead":0,"cacheWrite":0,"totalTokens":12658,"cost":{"total":0.06409}}}}`), text: "HITCH_PROBE_OMP_OK", model: "gpt-5.5", provider: "openai-codex"},
		{name: "opencode final text part", harness: protocol.HarnessOpenCode, sourceType: "message.part.text", source: protocol.RawJSON(`{"type":"message.part.updated","properties":{"sessionID":"sess","part":{"type":"text","text":"HITCH_PROBE_OPENCODE_OK","metadata":{"openai":{"phase":"final_answer"}}}}}`), text: "HITCH_PROBE_OPENCODE_OK"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := decodePayload(t, ParseTypedPayload(tt.harness, tt.sourceType, tt.source, protocol.EventTurnAssistantCompleted))
			assistant := nestedMap(t, payload, "turn", "assistant")
			if assistant["text"] != tt.text {
				t.Fatalf("assistant text = %#v, want %q; payload %#v", assistant["text"], tt.text, payload)
			}
			if tt.model != "" && assistant["model"] != tt.model {
				t.Fatalf("assistant model = %#v, want %q", assistant["model"], tt.model)
			}
			if tt.provider != "" && assistant["provider"] != tt.provider {
				t.Fatalf("assistant provider = %#v, want %q", assistant["provider"], tt.provider)
			}
		})
	}
}

func TestParseAssistantCompletedIncludesUsageAndCost(t *testing.T) {
	source := protocol.RawJSON(`{"message":{"content":[{"type":"text","text":"done"}],"usage":{"input":12,"output":3,"reasoningTokens":2,"cacheRead":4,"cacheWrite":1,"totalTokens":22,"cost":{"total":0.0123}}}}`)
	payload := decodePayload(t, ParseTypedPayload(protocol.HarnessPi, "turn_end", source, protocol.EventTurnAssistantCompleted))
	usage := nestedMap(t, payload, "turn", "assistant", "usage")
	tokens := nestedMap(t, usage, "tokens")
	if tokens["input"] != float64(12) || tokens["output"] != float64(3) || tokens["reasoning"] != float64(2) || tokens["cache_read"] != float64(4) || tokens["cache_write"] != float64(1) || tokens["total"] != float64(22) {
		t.Fatalf("bad assistant tokens: %#v", tokens)
	}
	cost := nestedMap(t, usage, "cost")
	if cost["amount"] != 0.0123 || cost["source"] != "harness_message_usage" || cost["estimated"] != false {
		t.Fatalf("bad assistant cost: %#v", cost)
	}
}

func TestParseOpenCodeStepFinishLLMCompletionCost(t *testing.T) {
	source := protocol.RawJSON(`{"type":"message.part.updated","properties":{"sessionID":"sess","messageID":"msg","part":{"type":"step-finish","reason":"stop","tokens":{"input":10,"output":3,"reasoning":2,"cache":{"read":4,"write":1},"total":20},"cost":0.00123}}}`)
	payload := decodePayload(t, ParseTypedPayload(protocol.HarnessOpenCode, "message.part.updated", source, protocol.EventLLMCompleted))
	llm := nestedMap(t, payload, "llm")
	if llm["finish_reason"] != "stop" {
		t.Fatalf("finish_reason = %#v", llm["finish_reason"])
	}
	usage := nestedMap(t, llm, "usage")
	tokens := nestedMap(t, usage, "tokens")
	if tokens["input"] != float64(10) || tokens["output"] != float64(3) || tokens["reasoning"] != float64(2) || tokens["cache_read"] != float64(4) || tokens["cache_write"] != float64(1) || tokens["total"] != float64(20) {
		t.Fatalf("bad tokens: %#v", tokens)
	}
	cost := nestedMap(t, usage, "cost")
	if cost["amount"] != 0.00123 || cost["currency"] != "USD" || cost["source"] != "opencode_step_finish" || cost["estimated"] != false {
		t.Fatalf("bad cost: %#v", cost)
	}
}

func TestParseLLMCompletionUsesNullMetricsWhenAbsent(t *testing.T) {
	payload := decodePayload(t, ParseTypedPayload(protocol.HarnessHermes, "transform_llm_output", protocol.RawJSON(`{"output":"done"}`), protocol.EventLLMCompleted))
	llm := nestedMap(t, payload, "llm")
	usage := nestedMap(t, llm, "usage")
	tokens := nestedMap(t, usage, "tokens")
	for _, key := range []string{"input", "output", "reasoning", "cache_read", "cache_write", "total"} {
		if tokens[key] != nil {
			t.Fatalf("token %s = %#v, want nil", key, tokens[key])
		}
	}
	cost := nestedMap(t, usage, "cost")
	if cost["amount"] != nil || cost["source"] != "" || cost["estimated"] != false {
		t.Fatalf("bad missing cost defaults: %#v", cost)
	}
}
