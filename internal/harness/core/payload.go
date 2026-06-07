package core

import (
	"encoding/json"
	"strings"

	"github.com/sagebynature/hitch/internal/protocol"
)

// ParseTypedPayload converts harness-specific source payloads into Hitch event-type
// payloads. SourcePayload remains the harness escape hatch; Payload is the stable
// handler contract keyed by HitchEventType.
func ParseTypedPayload(h protocol.Harness, sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) protocol.RawJSON {
	var src map[string]interface{}
	if err := json.Unmarshal(sourcePayload, &src); err != nil || src == nil {
		return protocol.Raw(map[string]interface{}{"unparsed": true})
	}

	switch hitchEventType {
	case protocol.EventToolRequested:
		return protocol.Raw(map[string]interface{}{"tool": parseRequestedTool(h, sourceEventType, src)})
	case protocol.EventToolCompleted:
		return protocol.Raw(map[string]interface{}{"tool": parseCompletedTool(h, sourceEventType, src)})
	case protocol.EventTurnUserPrompt:
		return protocol.Raw(map[string]interface{}{"turn": parseUserPrompt(src)})
	case protocol.EventTurnAssistantCompleted:
		return protocol.Raw(map[string]interface{}{"turn": map[string]interface{}{"assistant": parseAssistantCompleted(h, sourceEventType, src)}})
	case protocol.EventTurnCompleted:
		return protocol.Raw(map[string]interface{}{"turn": parseTurnCompleted(h, sourceEventType, src)})
	case protocol.EventLLMCompleted:
		return protocol.Raw(map[string]interface{}{"llm": parseLLMCompleted(h, sourceEventType, src)})
	default:
		return protocol.Raw(map[string]interface{}{"unparsed": true})
	}
}

func parseRequestedTool(h protocol.Harness, sourceEventType string, src map[string]interface{}) map[string]interface{} {
	input := firstMap(src,
		[]string{"tool_input"},
		[]string{"input"},
		[]string{"output", "args"},
		[]string{"event", "input"},
	)
	command := firstString(
		valueAt(src, []string{"command"}),
		valueAt(src, []string{"cmd"}),
		valueAt(input, []string{"command"}),
		valueAt(input, []string{"cmd"}),
		valueAt(src, []string{"output", "args", "command"}),
	)
	name := firstString(
		valueAt(src, []string{"tool_name"}),
		valueAt(src, []string{"name"}),
		valueAt(src, []string{"toolName"}),
		valueAt(src, []string{"tool"}),
		valueAt(src, []string{"input", "tool"}),
		valueAt(src, []string{"event", "name"}),
	)
	tool := map[string]interface{}{}
	setIfString(tool, "name", name)
	setIfString(tool, "kind", toolKind(h, sourceEventType, name, command))
	setIfValue(tool, "input", input)
	setIfString(tool, "command", command)
	setIfString(tool, "call_id", firstString(
		valueAt(src, []string{"tool_call_id"}),
		valueAt(src, []string{"call_id"}),
		valueAt(src, []string{"id"}),
		valueAt(src, []string{"input", "callID"}),
		valueAt(src, []string{"input", "call_id"}),
	))
	return tool
}

func parseCompletedTool(h protocol.Harness, sourceEventType string, src map[string]interface{}) map[string]interface{} {
	input := firstMap(src,
		[]string{"tool_input"},
		[]string{"input"},
		[]string{"event", "input"},
	)
	output := firstValue(src,
		[]string{"output"},
		[]string{"result"},
		[]string{"tool_response"},
		[]string{"content"},
		[]string{"event", "output"},
	)
	name := firstString(
		valueAt(src, []string{"tool_name"}),
		valueAt(src, []string{"name"}),
		valueAt(src, []string{"toolName"}),
		valueAt(src, []string{"tool"}),
		valueAt(src, []string{"input", "tool"}),
		valueAt(src, []string{"event", "name"}),
	)
	tool := map[string]interface{}{}
	setIfString(tool, "name", name)
	setIfString(tool, "kind", toolKind(h, sourceEventType, name, firstString(valueAt(input, []string{"command"}))))
	setIfValue(tool, "input", input)
	setIfValue(tool, "output", output)
	if errValue, ok := firstPresent(src, []string{"isError"}, []string{"is_error"}, []string{"error"}); ok {
		tool["error"] = boolish(errValue)
	}
	setIfValue(tool, "exit_code", firstNumber(src, []string{"exit_code"}, []string{"exitCode"}, []string{"status"}))
	return tool
}

func parseUserPrompt(src map[string]interface{}) map[string]interface{} {
	turn := map[string]interface{}{}
	prompt := firstString(
		valueAt(src, []string{"prompt"}),
		valueAt(src, []string{"message"}),
		valueAt(src, []string{"text"}),
		valueAt(src, []string{"input"}),
		valueAt(src, []string{"output", "text"}),
		valueAt(src, []string{"extra", "user_message"}),
	)
	if prompt == "" {
		prompt = textFromParts(firstSlice(src,
			[]string{"parts"},
			[]string{"input", "parts"},
			[]string{"output", "parts"},
		))
	}
	setIfString(turn, "prompt", prompt)
	setIfValue(turn, "messages", firstSlice(src, []string{"messages"}, []string{"input", "messages"}, []string{"output", "messages"}))
	setIfString(turn, "command", firstString(valueAt(src, []string{"command"}), valueAt(src, []string{"cmd"})))
	return turn
}

func parseTurnCompleted(h protocol.Harness, sourceEventType string, src map[string]interface{}) map[string]interface{} {
	turn := map[string]interface{}{}
	setIfValue(turn, "index", firstNumber(src, []string{"turnIndex"}, []string{"turn_index"}))
	assistant := parseAssistantCompleted(h, sourceEventType, src)
	if firstString(valueAt(assistant, []string{"text"})) != "" || valueAt(assistant, []string{"content"}) != nil {
		turn["assistant"] = assistant
	}
	return turn
}

func parseAssistantCompleted(h protocol.Harness, sourceEventType string, src map[string]interface{}) map[string]interface{} {
	message := firstMap(src, []string{"message"}, []string{"event", "message"})
	part := firstMap(src, []string{"part"}, []string{"properties", "part"}, []string{"event", "properties", "part"})
	content := firstSlice(src, []string{"content"}, []string{"message", "content"}, []string{"event", "message", "content"})
	usageSource := firstMap(src, []string{"usage"}, []string{"message", "usage"}, []string{"event", "message", "usage"}, []string{"extra", "usage"})

	text := firstString(
		valueAt(src, []string{"last_assistant_message"}),
		valueAt(src, []string{"extra", "response_text"}),
		valueAt(src, []string{"response_text"}),
		valueAt(src, []string{"output", "text"}),
		valueAt(src, []string{"text"}),
		valueAt(part, []string{"text"}),
	)
	if text == "" {
		text = textFromParts(content)
	}

	assistant := map[string]interface{}{
		"usage": normalizeUsage(usageSource, h, sourceEventType, nil),
	}
	setIfString(assistant, "text", text)
	setIfValue(assistant, "content", content)
	setIfString(assistant, "finish_reason", firstString(
		valueAt(message, []string{"stopReason"}),
		valueAt(message, []string{"finish_reason"}),
		valueAt(message, []string{"finishReason"}),
		valueAt(src, []string{"stopReason"}),
		valueAt(src, []string{"finish_reason"}),
		valueAt(src, []string{"finishReason"}),
		valueAt(part, []string{"reason"}),
	))
	setIfString(assistant, "response_id", firstString(valueAt(message, []string{"responseId"}), valueAt(message, []string{"response_id"}), valueAt(src, []string{"responseId"}), valueAt(src, []string{"response_id"})))
	setIfString(assistant, "provider", firstString(valueAt(message, []string{"provider"}), valueAt(src, []string{"provider"}), valueAt(src, []string{"extra", "provider"})))
	setIfString(assistant, "model", firstString(valueAt(message, []string{"model"}), valueAt(src, []string{"model"}), valueAt(src, []string{"extra", "model"})))
	return assistant
}

func parseLLMCompleted(h protocol.Harness, sourceEventType string, src map[string]interface{}) map[string]interface{} {
	part := firstMap(src, []string{"part"}, []string{"properties", "part"}, []string{"event", "properties", "part"})
	usage := firstMap(src,
		[]string{"usage"},
		[]string{"message", "usage"},
		[]string{"properties", "message", "usage"},
		[]string{"output", "usage"},
		[]string{"response", "usage"},
		[]string{"extra", "usage"},
	)

	llm := map[string]interface{}{
		"usage": normalizeUsage(usage, h, sourceEventType, part),
	}
	setIfString(llm, "provider", firstString(valueAt(src, []string{"provider"}), valueAt(src, []string{"providerID"}), valueAt(src, []string{"message", "provider"}), valueAt(src, []string{"model", "providerID"}), valueAt(src, []string{"model", "provider"}), valueAt(src, []string{"extra", "provider"})))
	setIfString(llm, "model", firstString(valueAt(src, []string{"model"}), valueAt(src, []string{"message", "model"}), valueAt(src, []string{"model", "id"}), valueAt(src, []string{"model", "modelID"}), valueAt(src, []string{"extra", "model"})))
	setIfString(llm, "finish_reason", firstString(valueAt(part, []string{"reason"}), valueAt(src, []string{"reason"}), valueAt(src, []string{"finish_reason"}), valueAt(src, []string{"finishReason"}), valueAt(src, []string{"message", "stopReason"})))
	output := firstValue(src, []string{"output"}, []string{"result"}, []string{"response"}, []string{"text"}, []string{"extra", "response_text"}, []string{"properties", "part", "text"})
	if output == nil {
		if outputText := textFromParts(firstSlice(src, []string{"message", "content"}, []string{"event", "message", "content"})); outputText != "" {
			output = outputText
		}
	}
	setIfValue(llm, "output", output)
	setIfValue(llm, "duration_ms", firstNumber(src, []string{"duration_ms"}, []string{"durationMs"}, []string{"latency_ms"}, []string{"latencyMs"}))
	setIfString(llm, "request_id", firstString(valueAt(src, []string{"request_id"}), valueAt(src, []string{"requestId"}), valueAt(src, []string{"message", "responseId"}), valueAt(src, []string{"headers", "x-request-id"}), valueAt(src, []string{"headers", "request-id"})))
	return llm
}

func normalizeUsage(usage map[string]interface{}, h protocol.Harness, sourceEventType string, part map[string]interface{}) map[string]interface{} {
	tokensSource := firstNonEmptyMap(firstMap(usage, []string{"tokens"}), usage)
	if len(part) != 0 {
		tokensSource = firstNonEmptyMap(firstMap(part, []string{"tokens"}, []string{"usage", "tokens"}), tokensSource)
	}
	amount, amountOK := firstPresent(usage, []string{"cost"}, []string{"cost", "total"})
	if len(part) != 0 {
		if v, ok := firstPresent(part, []string{"cost"}, []string{"usage", "cost"}, []string{"usage", "cost", "total"}); ok {
			amount, amountOK = v, true
		}
	}
	return map[string]interface{}{
		"tokens": normalizeTokens(tokensSource),
		"cost": map[string]interface{}{
			"amount":    costAmountOrNil(amount, amountOK),
			"currency":  firstString(valueAt(usage, []string{"currency"}), "USD"),
			"source":    costSource(h, sourceEventType, part, amountOK),
			"estimated": false,
		},
	}
}

func normalizeTokens(src map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"input":       numberOrNil(firstPresent(src, []string{"input"}, []string{"input_tokens"}, []string{"inputTokens"}, []string{"prompt_tokens"}, []string{"promptTokens"})),
		"output":      numberOrNil(firstPresent(src, []string{"output"}, []string{"output_tokens"}, []string{"outputTokens"}, []string{"completion_tokens"}, []string{"completionTokens"})),
		"reasoning":   numberOrNil(firstPresent(src, []string{"reasoning"}, []string{"reasoning_tokens"}, []string{"reasoningTokens"})),
		"cache_read":  numberOrNil(firstPresent(src, []string{"cache_read"}, []string{"cacheRead"}, []string{"cache", "read"}, []string{"cacheReadInputTokens"})),
		"cache_write": numberOrNil(firstPresent(src, []string{"cache_write"}, []string{"cacheWrite"}, []string{"cache", "write"}, []string{"cacheWriteInputTokens"})),
		"total":       numberOrNil(firstPresent(src, []string{"total"}, []string{"total_tokens"}, []string{"totalTokens"})),
	}
}

func toolKind(h protocol.Harness, sourceEventType, name, command string) string {
	if sourceEventType == "user_python" {
		return "python"
	}
	if sourceEventType == "user_bash" || command != "" {
		return "shell"
	}
	lower := strings.ToLower(name)
	if lower == "bash" || lower == "shell" || lower == "sh" || lower == "zsh" {
		return "shell"
	}
	if h == protocol.HarnessOpenCode && lower == "bash" {
		return "shell"
	}
	if name != "" {
		return "tool"
	}
	return ""
}

func costSource(h protocol.Harness, sourceEventType string, part map[string]interface{}, hasAmount bool) string {
	if !hasAmount {
		return ""
	}
	if h == protocol.HarnessOpenCode && firstString(valueAt(part, []string{"type"})) == "step-finish" {
		return "opencode_step_finish"
	}
	if sourceEventType == "message_end" || sourceEventType == "turn_end" {
		return "harness_message_usage"
	}
	return "provider_usage"
}

func firstValue(src map[string]interface{}, paths ...[]string) interface{} {
	for _, path := range paths {
		if v, ok := firstPresent(src, path); ok {
			return v
		}
	}
	return nil
}

func firstPresent(src map[string]interface{}, paths ...[]string) (interface{}, bool) {
	for _, path := range paths {
		if v, ok := valueAtOK(src, path); ok {
			return v, true
		}
	}
	return nil, false
}

func firstString(values ...interface{}) string {
	for _, v := range values {
		switch s := v.(type) {
		case string:
			if s != "" {
				return s
			}
		case json.Number:
			return s.String()
		}
	}
	return ""
}

func firstMap(src map[string]interface{}, paths ...[]string) map[string]interface{} {
	for _, path := range paths {
		if v, ok := valueAtOK(src, path); ok {
			if m, ok := v.(map[string]interface{}); ok {
				return m
			}
		}
	}
	return nil
}

func firstNonEmptyMap(values ...map[string]interface{}) map[string]interface{} {
	for _, v := range values {
		if len(v) != 0 {
			return v
		}
	}
	return nil
}

func firstSlice(src map[string]interface{}, paths ...[]string) []interface{} {
	for _, path := range paths {
		if v, ok := valueAtOK(src, path); ok {
			if s, ok := v.([]interface{}); ok {
				return s
			}
		}
	}
	return nil
}

func firstNumber(src map[string]interface{}, paths ...[]string) interface{} {
	for _, path := range paths {
		if v, ok := valueAtOK(src, path); ok {
			if numberOrNil(v, true) != nil {
				return v
			}
		}
	}
	return nil
}

func valueAt(src map[string]interface{}, path []string) interface{} {
	v, _ := valueAtOK(src, path)
	return v
}

func valueAtOK(src map[string]interface{}, path []string) (interface{}, bool) {
	if src == nil || len(path) == 0 {
		return nil, false
	}
	var cur interface{} = src
	for _, key := range path {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setIfString(dst map[string]interface{}, key, value string) {
	if value != "" {
		dst[key] = value
	}
}

func setIfValue(dst map[string]interface{}, key string, value interface{}) {
	switch v := value.(type) {
	case nil:
		return
	case []interface{}:
		if v == nil {
			return
		}
	case map[string]interface{}:
		if v == nil {
			return
		}
	}
	dst[key] = value
}

func numberOrNil(args ...interface{}) interface{} {
	if len(args) == 0 {
		return nil
	}
	v := args[0]
	if len(args) > 1 {
		if ok, _ := args[1].(bool); !ok {
			return nil
		}
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return n
	case int64:
		return n
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	}
	return nil
}

func costAmountOrNil(v interface{}, ok bool) interface{} {
	if !ok {
		return nil
	}
	if n := numberOrNil(v, true); n != nil {
		return n
	}
	if m, ok := v.(map[string]interface{}); ok {
		return numberOrNil(firstPresent(m, []string{"total"}, []string{"amount"}))
	}
	return nil
}

func boolish(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b != "" && b != "false" && b != "0"
	default:
		return v != nil
	}
}

func textFromParts(parts []interface{}) string {

	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		m, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		text := firstString(valueAt(m, []string{"text"}))
		if text == "" {
			continue
		}
		b.WriteString(text)
	}
	return b.String()
}
