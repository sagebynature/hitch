package config

// DefaultConfigTOML is the configuration seeded by the installer when
// ~/.config/hitch/config.toml does not exist.
const DefaultConfigTOML = `[server]
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

[harness.codex]
enabled = true

[harness.hermes]
enabled = true

[harness.pi]
enabled = true

[harness.omp]
enabled = true
`
