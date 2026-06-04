package config

import "testing"

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
