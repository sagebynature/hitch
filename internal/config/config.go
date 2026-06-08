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

type SeedResult struct {
	Path    string
	Created bool
}

func SeedDefault(path string) (SeedResult, error) {
	if path == "" {
		path = DefaultPath
	}
	path = ExpandHome(path)
	if _, err := os.Stat(path); err == nil {
		return SeedResult{Path: path, Created: false}, nil
	} else if !os.IsNotExist(err) {
		return SeedResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return SeedResult{}, err
	}
	if err := os.WriteFile(path, []byte(DefaultConfigTOML), 0o644); err != nil {
		return SeedResult{}, err
	}
	return SeedResult{Path: path, Created: true}, nil
}

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
	Enabled bool   `toml:"enabled"`
	Level   string `toml:"level"`
	Format  string `toml:"format"`
}

type LogFile struct {
	Enabled    bool   `toml:"enabled"`
	Level      string `toml:"level"`
	Format     string `toml:"format"`
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

type SourceEventFilter struct {
	Harness         string `toml:"harness"`
	SourceEventType string `toml:"source_event_type"`
}

type HandlerConfig struct {
	Type         string              `toml:"type"`
	Extension    string              `toml:"extension"`
	Entrypoint   string              `toml:"entrypoint"`
	Command      []string            `toml:"command"`
	WorkingDir   string              `toml:"working_dir"`
	Events       []string            `toml:"events"`
	HitchEvents  []string            `toml:"hitch_events"`
	SourceEvents []SourceEventFilter `toml:"source_events"`
	Payload      string              `toml:"payload"`
	Kind         string              `toml:"kind"`
	TimeoutMS    int                 `toml:"timeout_ms"`
	OnError      string              `toml:"on_error"`
	OnTimeout    string              `toml:"on_timeout"`
	payloadSet      bool
	sourceEventsSet bool
}

type HarnessConfig struct {
	Codex    HarnessToggle `toml:"codex"`
	Hermes   HarnessToggle `toml:"hermes"`
	Pi       HarnessToggle `toml:"pi"`
	OMP      HarnessToggle `toml:"omp"`
	OpenCode HarnessToggle `toml:"opencode"`
}

type HarnessToggle struct {
	Enabled  bool                  `toml:"enabled"`
	EventMap map[string]EventTypes `toml:"event_map"`
}

type EventTypes []protocol.EventType

func (e *EventTypes) UnmarshalTOML(v interface{}) error {
	switch value := v.(type) {
	case string:
		*e = EventTypes{protocol.EventType(value)}
		return nil
	case []interface{}:
		out := make(EventTypes, 0, len(value))
		for _, item := range value {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("event map values must be strings, got %T", item)
			}
			out = append(out, protocol.EventType(s))
		}
		*e = out
		return nil
	default:
		return fmt.Errorf("event map value must be a string or list of strings, got %T", v)
	}
}

func Load(path string) (Config, error) {
	return LoadWithExtensionDir(path, DefaultExtensionDir())
}

func LoadWithoutExtensions(path string) (Config, error) {
	return loadFile(path, true)
}

func loadFile(path string, validate bool) (Config, error) {
	path = ExpandHome(path)
	baseDir := filepath.Dir(path)
	if absPath, err := filepath.Abs(path); err == nil {
		path = absPath
		baseDir = filepath.Dir(absPath)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	c, err := parseConfigBytes(b, validate)
	if err != nil {
		return Config{}, err
	}
	resolveHandlerWorkingDirs(&c, baseDir)
	return c, nil
}

func Parse(b []byte) (Config, error) {
	return parseConfigBytes(b, true)
}

func parseConfigBytes(b []byte, validate bool) (Config, error) {
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
	upgradeLegacyDefaultEventMaps(&c)
	for name, h := range c.Handlers {
		h.payloadSet = md.IsDefined("handlers", name, "payload")
		h.sourceEventsSet = md.IsDefined("handlers", name, "source_events")
		if err := normalizeHandlerConfig(name, &h); err != nil {
			return Config{}, err
		}
		c.Handlers[name] = h
	}
	if validate {
		if err := c.Validate(); err != nil {
			return Config{}, err
		}
	}
	return c, nil
}

func upgradeLegacyDefaultEventMaps(c *Config) {
	upgradeIfExact(&c.Harness.Pi.EventMap, "turn_end",
		EventTypes{protocol.EventTurnCompleted, protocol.EventTurnAssistantCompleted},
		EventTypes{protocol.EventTurnCompleted, protocol.EventTurnAssistantCompleted, protocol.EventLLMCompleted},
	)
	upgradeIfExact(&c.Harness.OMP.EventMap, "turn_end",
		EventTypes{protocol.EventTurnCompleted},
		EventTypes{protocol.EventTurnCompleted, protocol.EventTurnAssistantCompleted, protocol.EventLLMCompleted},
	)
	upgradeIfExact(&c.Harness.OpenCode.EventMap, "session.idle",
		EventTypes{protocol.EventTurnCompleted, protocol.EventTurnAssistantCompleted},
		EventTypes{protocol.EventTurnCompleted},
	)
	ensureEventMapEntry(&c.Harness.OpenCode.EventMap, "message.part.step-finish", EventTypes{protocol.EventLLMCompleted})
	ensureEventMapEntry(&c.Harness.OpenCode.EventMap, "message.part.text", EventTypes{protocol.EventTurnAssistantCompleted})
	if eventTypesEqual(c.Harness.OpenCode.EventMap["message.part.updated"], EventTypes{protocol.EventLLMCompleted}) {
		delete(c.Harness.OpenCode.EventMap, "message.part.updated")
	}
}

func upgradeIfExact(m *map[string]EventTypes, key string, oldValue, newValue EventTypes) {
	if !eventTypesEqual((*m)[key], oldValue) {
		return
	}
	ensureEventMap(m)
	(*m)[key] = newValue
}

func ensureEventMapEntry(m *map[string]EventTypes, key string, value EventTypes) {
	ensureEventMap(m)
	if _, ok := (*m)[key]; !ok {
		(*m)[key] = value
	}
}

func ensureEventMap(m *map[string]EventTypes) {
	if *m == nil {
		*m = map[string]EventTypes{}
	}
}

func eventTypesEqual(a, b EventTypes) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeHandlerConfig(name string, h *HandlerConfig) error {
	if h.Type == "" {
		h.Type = "shell"
	}
	if h.Payload == "" {
		h.Payload = "hitch"
	}
	if len(h.Events) != 0 && len(h.HitchEvents) != 0 && !stringMultisetsEqual(h.Events, h.HitchEvents) {
		return fmt.Errorf("handlers.%s cannot set conflicting events and hitch_events", name)
	}
	if len(h.HitchEvents) == 0 {
		h.HitchEvents = append([]string(nil), h.Events...)
	}
	if len(h.Events) == 0 {
		h.Events = append([]string(nil), h.HitchEvents...)
	}
	return nil
}

func stringMultisetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, s := range a {
		counts[s]++
	}
	for _, s := range b {
		n := counts[s]
		if n == 0 {
			return false
		}
		if n == 1 {
			delete(counts, s)
			continue
		}
		counts[s] = n - 1
	}
	return len(counts) == 0
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
	if err := validateLogLevel("log.level", c.Log.Level); err != nil {
		return err
	}
	if err := validateLogFormat("log.format", c.Log.Format); err != nil {
		return err
	}
	if c.Log.Stdout.Enabled {
		if err := validateOptionalLogLevel("log.stdout.level", c.Log.Stdout.Level); err != nil {
			return err
		}
		if err := validateOptionalLogFormat("log.stdout.format", c.Log.Stdout.Format); err != nil {
			return err
		}
	}
	if c.Log.File.Enabled {
		if err := validateOptionalLogLevel("log.file.level", c.Log.File.Level); err != nil {
			return err
		}
		if err := validateOptionalLogFormat("log.file.format", c.Log.File.Format); err != nil {
			return err
		}
		if c.Log.File.Path == "" {
			return errors.New("log.file.path is required when file logging is enabled")
		}
		if c.Log.File.MaxSizeMB <= 0 {
			return errors.New("log.file.max_size_mb must be positive")
		}
	}
	if c.Log.OTLP.Enabled {
		return errors.New("log.otlp is not implemented")
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
				return errors.New(`audit.backend "jsonl" is not implemented`)
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
	if err := validateHarnessEventMap("opencode", c.Harness.OpenCode.EventMap); err != nil {
		return err
	}
	for name, h := range c.Handlers {
		if name == "" {
			return errors.New("handler name cannot be empty")
		}
		switch h.Type {
		case "shell":
			if len(h.Command) == 0 || h.Command[0] == "" {
				return fmt.Errorf("handlers.%s.command is required", name)
			}
		case "native":
			if h.Extension == "" {
				return fmt.Errorf("handlers.%s.extension is required for native handlers", name)
			}
			if h.Entrypoint == "" {
				return fmt.Errorf("handlers.%s.entrypoint is required for native handlers", name)
			}
		default:
			return fmt.Errorf("handlers.%s.type must be shell or native", name)
		}
		if len(h.HitchEvents) == 0 {
			return fmt.Errorf("handlers.%s.hitch_events is required", name)
		}
		for _, e := range h.HitchEvents {
			if e != "*" && !protocol.IsValidEventType(protocol.EventType(e)) {
				return fmt.Errorf("handlers.%s references unknown event %q", name, e)
			}
		}
		switch h.Payload {
		case "hitch", "source":
		default:
			return fmt.Errorf("handlers.%s.payload must be hitch or source", name)
		}
		for _, f := range h.SourceEvents {
			if !protocol.IsValidHarness(protocol.Harness(f.Harness)) {
				return fmt.Errorf("handlers.%s source_events references unknown harness %q", name, f.Harness)
			}
			if f.SourceEventType == "" {
				return fmt.Errorf("handlers.%s source_events source_event_type is required", name)
			}
		}
		switch h.Kind {
		case "observer", "control":
		default:
			return fmt.Errorf("handlers.%s.kind must be observer or control", name)
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

func validateHarnessEventMap(harnessName string, eventMap map[string]EventTypes) error {
	for sourceEvent, hitchEvents := range eventMap {
		if sourceEvent == "" {
			return fmt.Errorf("harness.%s.event_map source event cannot be empty", harnessName)
		}
		if len(hitchEvents) == 0 {
			return fmt.Errorf("harness.%s.event_map.%s must map to at least one event", harnessName, sourceEvent)
		}
		seen := map[protocol.EventType]struct{}{}
		for _, hitchEvent := range hitchEvents {
			if !protocol.IsValidEventType(hitchEvent) {
				return fmt.Errorf("harness.%s.event_map.%s references unknown event %q", harnessName, sourceEvent, hitchEvent)
			}
			if _, ok := seen[hitchEvent]; ok {
				return fmt.Errorf("harness.%s.event_map.%s repeats event %q", harnessName, sourceEvent, hitchEvent)
			}
			seen[hitchEvent] = struct{}{}
		}
	}
	return nil
}

func validateOptionalLogLevel(key, value string) error {
	if value == "" {
		return nil
	}
	return validateLogLevel(key, value)
}

func validateLogLevel(key, value string) error {
	switch value {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid %s %q", key, value)
	}
}

func validateOptionalLogFormat(key, value string) error {
	if value == "" {
		return nil
	}
	return validateLogFormat(key, value)
}

func validateLogFormat(key, value string) error {
	switch value {
	case "json", "console":
		return nil
	default:
		return fmt.Errorf("invalid %s %q", key, value)
	}
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

func resolveHandlerWorkingDirs(c *Config, baseDir string) {
	for name, h := range c.Handlers {
		if h.WorkingDir == "" {
			continue
		}
		dir := ExpandHome(h.WorkingDir)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		h.WorkingDir = filepath.Clean(dir)
		c.Handlers[name] = h
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
