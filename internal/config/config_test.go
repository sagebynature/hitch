package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

const baseConfig = `
[server]
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
path = "~/.local/state/hitch/hitch.log"
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
path = "~/.local/share/hitch/events.sqlite"

[handlers.audit]
command = ["hitch-handler-audit"]
events = ["*"]
kind = "observer"
timeout_ms = 1000

[handlers.security_gate]
command = ["hitch-handler-security"]
events = ["tool.requested", "tool.permission_requested"]
kind = "control"
timeout_ms = 750
on_error = "fail_open"
on_timeout = "fail_open"

[harness.codex]
enabled = true
[harness.hermes]
enabled = true
[harness.pi]
enabled = true
[harness.omp]
enabled = true
`

func TestParseValidConfig(t *testing.T) {
	c, err := Parse([]byte(baseConfig))
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if c.Server.Port != 8799 {
		t.Fatalf("wrong port: %d", c.Server.Port)
	}
	if got := c.Handlers["security_gate"].Kind; got != "control" {
		t.Fatalf("wrong kind: %s", got)
	}
}

func TestParseAllowsPerSinkLogLevelAndFormat(t *testing.T) {
	text := strings.Replace(baseConfig, `[log.stdout]
enabled = true`, `[log.stdout]
enabled = true
level = "warn"
format = "console"`, 1)
	text = strings.Replace(text, `[log.file]
enabled = false`, `[log.file]
enabled = true
level = "debug"
format = "json"`, 1)
	cfg, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("per-sink log config rejected: %v", err)
	}
	if cfg.Log.Stdout.Level != "warn" || cfg.Log.Stdout.Format != "console" {
		t.Fatalf("per-sink stdout log config not parsed: %#v", cfg.Log.Stdout)
	}
	if cfg.Log.File.Level != "debug" || cfg.Log.File.Format != "json" {
		t.Fatalf("per-sink file log config not parsed: %#v", cfg.Log.File)
	}
}

func TestParseRejectsInvalidPerSinkLogLevelAndFormat(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{
			name: "stdout level",
			text: strings.Replace(baseConfig, `[log.stdout]
enabled = true`, `[log.stdout]
enabled = true
level = "trace"`, 1),
		},
		{
			name: "stdout format",
			text: strings.Replace(baseConfig, `[log.stdout]
enabled = true`, `[log.stdout]
enabled = true
format = "pretty"`, 1),
		},
		{
			name: "file level",
			text: strings.Replace(baseConfig, `[log.file]
enabled = false`, `[log.file]
enabled = true
level = "trace"`, 1),
		},
		{
			name: "file format",
			text: strings.Replace(baseConfig, `[log.file]
enabled = false`, `[log.file]
enabled = true
format = "pretty"`, 1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.text)); err == nil {
				t.Fatal("invalid per-sink log config accepted")
			}
		})
	}
}

func TestRejectsUnimplementedAuditJSONLBackend(t *testing.T) {
	cfg := strings.Replace(baseConfig, `backend = "sqlite"`, `backend = "jsonl"`, 1)
	_, err := Parse([]byte(cfg))
	if err == nil || !strings.Contains(err.Error(), `audit.backend "jsonl" is not implemented`) {
		t.Fatalf("expected jsonl implementation error, got %v", err)
	}
}

func TestRejectsUnimplementedOTLPLogging(t *testing.T) {
	cfg := strings.Replace(baseConfig, `enabled = false
endpoint = "http://127.0.0.1:4318"`, `enabled = true
endpoint = "http://127.0.0.1:4318"`, 1)
	_, err := Parse([]byte(cfg))
	if err == nil || !strings.Contains(err.Error(), `log.otlp is not implemented`) {
		t.Fatalf("expected otlp implementation error, got %v", err)
	}
}
func TestParseHarnessEventMap(t *testing.T) {
	cfg, err := Parse([]byte(baseConfig + `
[harness.codex.event_map]
CustomHook = "turn.started"
PreToolUse = "tool.permission_requested"
`))
	if err != nil {
		t.Fatalf("valid event map rejected: %v", err)
	}
	if got := cfg.Harness.Codex.EventMap["CustomHook"]; len(got) != 1 || got[0] != "turn.started" {
		t.Fatalf("event map scalar did not parse: %#v", cfg.Harness.Codex.EventMap)
	}
	if got := cfg.Harness.Codex.EventMap["PreToolUse"]; len(got) != 1 || got[0] != "tool.permission_requested" {
		t.Fatalf("event map scalar did not parse: %#v", cfg.Harness.Codex.EventMap)
	}
}

func TestParseHarnessEventMapList(t *testing.T) {
	cfg, err := Parse([]byte(baseConfig + `
[harness.codex.event_map]
Stop = ["turn.completed", "turn.assistant_completed"]
`))
	if err != nil {
		t.Fatalf("valid event map list rejected: %v", err)
	}
	got := cfg.Harness.Codex.EventMap["Stop"]
	if len(got) != 2 || got[0] != "turn.completed" || got[1] != "turn.assistant_completed" {
		t.Fatalf("event map list did not parse in order: %#v", got)
	}
}

func TestParseRejectsInvalidHarnessEventMap(t *testing.T) {
	_, err := Parse([]byte(baseConfig + `
[harness.codex.event_map]
BadHook = "not.real"
`))
	if err == nil {
		t.Fatal("invalid harness event map accepted")
	}
}

func TestParseRejectsInvalidHarnessEventMapList(t *testing.T) {
	_, err := Parse([]byte(baseConfig + `
[harness.codex.event_map]
Stop = ["turn.completed", "not.real"]
`))
	if err == nil {
		t.Fatal("invalid harness event map list accepted")
	}
}

func TestParseRejectsEmptyHarnessEventMapList(t *testing.T) {
	_, err := Parse([]byte(baseConfig + `
[harness.codex.event_map]
Stop = []
`))
	if err == nil {
		t.Fatal("empty harness event map list accepted")
	}
}

func TestParseRejectsDuplicateHarnessEventMapList(t *testing.T) {
	_, err := Parse([]byte(baseConfig + `
[harness.codex.event_map]
Stop = ["turn.completed", "turn.completed"]
`))
	if err == nil {
		t.Fatal("duplicate harness event map list accepted")
	}
}
func TestLoadResolvesHandlerWorkingDirRelativeToConfig(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "hitch.toml")
	text := strings.Replace(baseConfig, `command = ["hitch-handler-audit"]`, "command = [\"hitch-handler-audit\"]\nworking_dir = \"..\"", 1)
	if err := os.WriteFile(configPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Handlers["audit"].WorkingDir, dir; got != want {
		t.Fatalf("working_dir resolved to %q, want %q", got, want)
	}
}

func TestDefaultConfigTOMLMatchesEmbeddedConfigFile(t *testing.T) {
	b, err := os.ReadFile("default.config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != DefaultConfigTOML {
		t.Fatalf("embedded default config differs from internal/config/default.config.toml")
	}
	cfg, err := Parse([]byte(DefaultConfigTOML))
	if err != nil {
		t.Fatalf("embedded default config is invalid: %v", err)
	}
	if len(cfg.Handlers) != 0 {
		t.Fatalf("default config should not enable handlers: %#v", cfg.Handlers)
	}
	if got := cfg.Harness.Codex.EventMap["PreToolUse"]; len(got) != 1 || got[0] != "tool.requested" {
		t.Fatalf("default config omitted source event mappings: %#v", cfg.Harness)
	}
	if got := cfg.Harness.Hermes.EventMap["pre_tool_call"]; len(got) != 1 || got[0] != "tool.requested" {
		t.Fatalf("default config omitted source event mappings: %#v", cfg.Harness)
	}
	if got := cfg.Harness.Codex.EventMap["Stop"]; len(got) != 2 || got[0] != "turn.completed" || got[1] != "turn.assistant_completed" {
		t.Fatalf("default config should map Codex Stop to primary and assistant completion events: %#v", got)
	}
	if got := cfg.Harness.Hermes.EventMap["pre_llm_call"]; len(got) != 2 || got[0] != "llm.requested" || got[1] != "turn.user_prompt" {
		t.Fatalf("default config should map Hermes pre_llm_call to LLM and prompt events: %#v", got)
	}
	if got := cfg.Harness.Hermes.EventMap["transform_llm_output"]; len(got) != 2 || got[0] != "llm.completed" || got[1] != "turn.assistant_completed" {
		t.Fatalf("default config should map Hermes transform_llm_output to LLM and assistant completion events: %#v", got)
	}
	if got := cfg.Harness.Pi.EventMap["turn_end"]; len(got) != 3 || got[0] != "turn.completed" || got[1] != "turn.assistant_completed" || got[2] != "llm.completed" {
		t.Fatalf("default config should map Pi turn_end to turn and LLM completion events: %#v", got)
	}
	if got := cfg.Harness.OMP.EventMap["turn_end"]; len(got) != 3 || got[0] != "turn.completed" || got[1] != "turn.assistant_completed" || got[2] != "llm.completed" {
		t.Fatalf("default config should map OMP turn_end to turn and LLM completion events: %#v", got)
	}
	for _, excluded := range []string{"post_tool_call", "post_llm_call"} {
		if _, ok := cfg.Harness.Hermes.EventMap[excluded]; ok {
			t.Fatalf("default Hermes map should exclude noisy source event %q", excluded)
		}
	}
	for _, excluded := range []string{"context", "before_provider_request", "before_agent_start", "agent_start", "agent_end"} {
		if _, ok := cfg.Harness.Pi.EventMap[excluded]; ok {
			t.Fatalf("default Pi map should exclude noisy source event %q", excluded)
		}
	}
	if got := cfg.Harness.OMP.EventMap["message_end"]; len(got) != 1 || got[0] != "turn.assistant_completed" {
		t.Fatalf("default OMP map should keep message_end as assistant completion: %#v", cfg.Harness.OMP.EventMap)
	}
	for _, excluded := range []string{"context", "before_provider_request", "tool_execution_update", "auto_retry_start", "message_start"} {
		if _, ok := cfg.Harness.OMP.EventMap[excluded]; ok {
			t.Fatalf("default OMP map should exclude noisy source event %q", excluded)
		}
	}
}

func TestDefaultConfigIncludesOpenCodeHarness(t *testing.T) {
	cfg, err := Parse([]byte(DefaultConfigTOML))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Harness.OpenCode.Enabled {
		t.Fatal("harness.opencode should be enabled by default")
	}
	cases := map[string]protocol.EventType{
		"chat.message":                    protocol.EventTurnUserPrompt,
		"tool.execute.before":             protocol.EventToolRequested,
		"tool.execute.after":              protocol.EventToolCompleted,
		"permission.ask":                  protocol.EventToolPermissionRequest,
		"session.created":                 protocol.EventSessionStarted,
		"session.compacted":               protocol.EventSessionCompacted,
		"experimental.session.compacting": protocol.EventSessionCompacted,
		"session.error":                   protocol.EventErrorReported,
		"message.part.step-finish":        protocol.EventLLMCompleted,
		"message.part.text":               protocol.EventTurnAssistantCompleted,
	}
	for source, want := range cases {
		got := cfg.Harness.OpenCode.EventMap[source]
		if len(got) == 0 || got[0] != want {
			t.Fatalf("opencode %s mapped to %#v, want first %s", source, got, want)
		}
	}
	idle := cfg.Harness.OpenCode.EventMap["session.idle"]
	if len(idle) != 1 || idle[0] != protocol.EventTurnCompleted {
		t.Fatalf("session.idle mapped to %#v", idle)
	}
	if _, ok := cfg.Harness.OpenCode.EventMap["message.part.updated"]; ok {
		t.Fatalf("message.part.updated should be split by the OpenCode plugin before dispatch: %#v", cfg.Harness.OpenCode.EventMap)
	}
}

func TestValidateRejectsInvalidOpenCodeEventMapValue(t *testing.T) {
	cfg, err := Parse([]byte(DefaultConfigTOML))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Harness.OpenCode.EventMap["chat.message"] = EventTypes{protocol.EventType("not.real")}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "harness.opencode.event_map.chat.message") {
		t.Fatalf("expected OpenCode event-map validation error, got %v", err)
	}
}

func TestParseUpgradesLegacyDefaultEventMaps(t *testing.T) {
	legacy := DefaultConfigTOML
	legacy = strings.Replace(legacy, `turn_end = ["turn.completed", "turn.assistant_completed", "llm.completed"]`, `turn_end = ["turn.completed", "turn.assistant_completed"]`, 1)
	legacy = strings.Replace(legacy, `turn_end = ["turn.completed", "turn.assistant_completed", "llm.completed"]`, `turn_end = "turn.completed"`, 1)
	legacy = strings.Replace(legacy, `"session.idle" = "turn.completed"`, `"session.idle" = ["turn.completed", "turn.assistant_completed"]`, 1)
	legacy = strings.Replace(legacy, `"message.part.step-finish" = "llm.completed"
"message.part.text" = "turn.assistant_completed"
`, ``, 1)

	cfg, err := Parse([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Harness.Pi.EventMap["turn_end"]; len(got) != 3 || got[0] != protocol.EventTurnCompleted || got[1] != protocol.EventTurnAssistantCompleted || got[2] != protocol.EventLLMCompleted {
		t.Fatalf("legacy Pi turn_end was not upgraded: %#v", got)
	}
	if got := cfg.Harness.OMP.EventMap["turn_end"]; len(got) != 3 || got[0] != protocol.EventTurnCompleted || got[1] != protocol.EventTurnAssistantCompleted || got[2] != protocol.EventLLMCompleted {
		t.Fatalf("legacy OMP turn_end was not upgraded: %#v", got)
	}
	if got := cfg.Harness.OpenCode.EventMap["session.idle"]; len(got) != 1 || got[0] != protocol.EventTurnCompleted {
		t.Fatalf("legacy OpenCode session.idle was not upgraded: %#v", got)
	}
	if got := cfg.Harness.OpenCode.EventMap["message.part.step-finish"]; len(got) != 1 || got[0] != protocol.EventLLMCompleted {
		t.Fatalf("OpenCode step-finish synthetic mapping missing: %#v", got)
	}
	if got := cfg.Harness.OpenCode.EventMap["message.part.text"]; len(got) != 1 || got[0] != protocol.EventTurnAssistantCompleted {
		t.Fatalf("OpenCode text synthetic mapping missing: %#v", got)
	}
}

func TestSeedDefaultCreatesValidConfigWithoutOverwritingExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")

	result, err := SeedDefault(path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Path != path {
		t.Fatalf("unexpected seed result: %#v", result)
	}
	seeded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(seeded) != DefaultConfigTOML {
		t.Fatalf("seeded config does not match default config")
	}
	if _, err := Parse(seeded); err != nil {
		t.Fatalf("seeded config is invalid: %v", err)
	}

	const existing = "user-owned config\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = SeedDefault(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatalf("existing config should not be recreated: %#v", result)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != existing {
		t.Fatalf("seed overwrote existing user config: %q", current)
	}
}

func TestParseRejectsUnknownKey(t *testing.T) {
	_, err := Parse([]byte(baseConfig + "\nunknown = true\n"))
	if err == nil {
		t.Fatal("unknown key accepted")
	}
}

func TestParseRejectsInvalidEvent(t *testing.T) {
	bad := baseConfig + `
[handlers.bad]
command = ["x"]
events = ["not.real"]
kind = "observer"
timeout_ms = 1
`
	_, err := Parse([]byte(bad))
	if err == nil {
		t.Fatal("invalid event accepted")
	}
}

func TestParseRejectsInvalidPort(t *testing.T) {
	bad := []byte(baseConfig)
	bad = []byte(string(bad[:]) + "\n[server.extra]\n")
	_ = bad
	c, err := Parse([]byte(baseConfig))
	if err != nil {
		t.Fatal(err)
	}
	c.Server.Port = 70000
	if err := c.Validate(); err == nil {
		t.Fatal("invalid port accepted")
	}
}
