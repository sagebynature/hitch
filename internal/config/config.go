package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sagebynature/hitch/internal/protocol"
)

const DefaultPath = "~/.config/hitch/config.toml"

type Config struct {
	Server   ServerConfig             `toml:"server"`
	Log      LogConfig                `toml:"log"`
	Audit    AuditConfig              `toml:"audit"`
	Handlers map[string]HandlerConfig `toml:"handlers"`
	Harness  HarnessConfig            `toml:"harness"`
}

type ServerConfig struct {
	Host            string `toml:"host"`
	Port            int    `toml:"port"`
	MaxRequestBytes int64  `toml:"max_request_bytes"`
}

type LogConfig struct {
	Level                string    `toml:"level"`
	Format               string    `toml:"format"`
	IncludeNativePayload bool      `toml:"include_native_payload"`
	Stdout               LogStdout `toml:"stdout"`
	File                 LogFile   `toml:"file"`
	OTLP                 LogOTLP   `toml:"otlp"`
}

type LogStdout struct {
	Enabled bool `toml:"enabled"`
}

type LogFile struct {
	Enabled    bool   `toml:"enabled"`
	Path       string `toml:"path"`
	MaxSizeMB  int    `toml:"max_size_mb"`
	MaxBackups int    `toml:"max_backups"`
	MaxAgeDays int    `toml:"max_age_days"`
	Compress   bool   `toml:"compress"`
}

type LogOTLP struct {
	Enabled  bool   `toml:"enabled"`
	Endpoint string `toml:"endpoint"`
	Protocol string `toml:"protocol"`
}

type AuditConfig struct {
	Enabled bool        `toml:"enabled"`
	Backend string      `toml:"backend"`
	SQLite  SQLiteAudit `toml:"sqlite"`
	JSONL   JSONLAudit  `toml:"jsonl"`
}

type SQLiteAudit struct {
	Path string `toml:"path"`
}
type JSONLAudit struct {
	Path string `toml:"path"`
}

type HandlerConfig struct {
	Command   []string `toml:"command"`
	Events    []string `toml:"events"`
	Mode      string   `toml:"mode"`
	TimeoutMS int      `toml:"timeout_ms"`
	OnError   string   `toml:"on_error"`
	OnTimeout string   `toml:"on_timeout"`
}

type HarnessConfig struct {
	Codex  HarnessToggle `toml:"codex"`
	Hermes HarnessToggle `toml:"hermes"`
	Pi     HarnessToggle `toml:"pi"`
	OMP    HarnessToggle `toml:"omp"`
}

type HarnessToggle struct {
	Enabled  bool                          `toml:"enabled"`
	EventMap map[string]protocol.EventType `toml:"event_map"`
}

func Load(path string) (Config, error) {
	path = ExpandHome(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(b)
}

func Parse(b []byte) (Config, error) {
	var c Config
	md, err := toml.Decode(string(b), &c)
	if err != nil {
		return Config{}, err
	}
	if undecoded := md.Undecoded(); len(undecoded) != 0 {
		return Config{}, fmt.Errorf("unknown config keys: %v", undecoded)
	}
	if c.Handlers == nil {
		c.Handlers = map[string]HandlerConfig{}
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	if c.Server.Host == "" {
		return errors.New("server.host is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port out of range: %d", c.Server.Port)
	}
	if c.Server.MaxRequestBytes <= 0 {
		return errors.New("server.max_request_bytes must be positive")
	}
	if c.Log.Level == "" {
		return errors.New("log.level is required")
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log.level %q", c.Log.Level)
	}
	switch c.Log.Format {
	case "json", "console":
	default:
		return fmt.Errorf("invalid log.format %q", c.Log.Format)
	}
	if c.Log.File.Enabled {
		if c.Log.File.Path == "" {
			return errors.New("log.file.path is required when file logging is enabled")
		}
		if c.Log.File.MaxSizeMB <= 0 {
			return errors.New("log.file.max_size_mb must be positive")
		}
	}
	if c.Log.OTLP.Enabled {
		if c.Log.OTLP.Endpoint == "" {
			return errors.New("log.otlp.endpoint is required when OTLP is enabled")
		}
		switch c.Log.OTLP.Protocol {
		case "http/protobuf", "grpc":
		default:
			return fmt.Errorf("invalid log.otlp.protocol %q", c.Log.OTLP.Protocol)
		}
	}
	if c.Audit.Enabled {
		if c.Audit.Backend == "" {
			return errors.New("audit.backend is required")
		}
		for _, b := range strings.Split(c.Audit.Backend, ",") {
			switch strings.TrimSpace(b) {
			case "sqlite":
				if c.Audit.SQLite.Path == "" {
					return errors.New("audit.sqlite.path is required")
				}
			case "jsonl":
				if c.Audit.JSONL.Path == "" {
					return errors.New("audit.jsonl.path is required")
				}
			default:
				return fmt.Errorf("invalid audit.backend %q", b)
			}
		}
	}
	if err := validateHarnessEventMap("codex", c.Harness.Codex.EventMap); err != nil {
		return err
	}
	if err := validateHarnessEventMap("hermes", c.Harness.Hermes.EventMap); err != nil {
		return err
	}
	if err := validateHarnessEventMap("pi", c.Harness.Pi.EventMap); err != nil {
		return err
	}
	if err := validateHarnessEventMap("omp", c.Harness.OMP.EventMap); err != nil {
		return err
	}
	for name, h := range c.Handlers {
		if name == "" {
			return errors.New("handler name cannot be empty")
		}
		if len(h.Command) == 0 || h.Command[0] == "" {
			return fmt.Errorf("handlers.%s.command is required", name)
		}
		if len(h.Events) == 0 {
			return fmt.Errorf("handlers.%s.events is required", name)
		}
		for _, e := range h.Events {
			if e != "*" && !protocol.IsValidEventType(protocol.EventType(e)) {
				return fmt.Errorf("handlers.%s references unknown event %q", name, e)
			}
		}
		switch h.Mode {
		case "async", "sync":
		default:
			return fmt.Errorf("handlers.%s.mode must be async or sync", name)
		}
		if h.TimeoutMS <= 0 {
			return fmt.Errorf("handlers.%s.timeout_ms must be positive", name)
		}
		if err := validatePolicy(name, "on_error", h.OnError); err != nil {
			return err
		}
		if err := validatePolicy(name, "on_timeout", h.OnTimeout); err != nil {
			return err
		}
	}
	return nil
}

func validateHarnessEventMap(harnessName string, eventMap map[string]protocol.EventType) error {
	for sourceEvent, hitchEvent := range eventMap {
		if sourceEvent == "" {
			return fmt.Errorf("harness.%s.event_map source event cannot be empty", harnessName)
		}
		if !protocol.IsValidEventType(hitchEvent) {
			return fmt.Errorf("harness.%s.event_map.%s references unknown event %q", harnessName, sourceEvent, hitchEvent)
		}
	}
	return nil
}

func validatePolicy(handler, key, value string) error {
	if value == "" {
		return nil
	}
	switch value {
	case "fail_open", "fail_closed", "native_default":
		return nil
	default:
		return fmt.Errorf("handlers.%s.%s has invalid policy %q", handler, key, value)
	}
}

func ExpandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
