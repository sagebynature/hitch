package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
mode = "async"
timeout_ms = 1000

[handlers.security_gate]
command = ["hitch-handler-security"]
events = ["tool.requested", "tool.permission_requested"]
mode = "sync"
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
	if got := c.Handlers["security_gate"].Mode; got != "sync" {
		t.Fatalf("wrong mode: %s", got)
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
	if cfg.Harness.Codex.EventMap["CustomHook"] != "turn.started" {
		t.Fatalf("event map did not parse: %#v", cfg.Harness.Codex.EventMap)
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
	if cfg.Harness.Codex.EventMap["PreToolUse"] != "tool.requested" || cfg.Harness.Hermes.EventMap["pre_tool_call"] != "tool.requested" {
		t.Fatalf("default config omitted source event mappings: %#v", cfg.Harness)
	}
	if cfg.Harness.Hermes.EventMap["pre_llm_call"] != "llm.requested" || cfg.Harness.Hermes.EventMap["transform_llm_output"] != "llm.completed" {
		t.Fatalf("default config omitted LLM source event mappings: %#v", cfg.Harness.Hermes.EventMap)
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
	if cfg.Harness.OMP.EventMap["message_end"] != "turn.assistant_completed" {
		t.Fatalf("default OMP map should keep message_end as assistant completion: %#v", cfg.Harness.OMP.EventMap)
	}
	for _, excluded := range []string{"context", "before_provider_request", "tool_execution_update", "auto_retry_start", "message_start"} {
		if _, ok := cfg.Harness.OMP.EventMap[excluded]; ok {
			t.Fatalf("default OMP map should exclude noisy source event %q", excluded)
		}
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
mode = "async"
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
