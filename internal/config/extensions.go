package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type extensionFile struct {
	Name         string              `toml:"name"`
	Entrypoint   string              `toml:"entrypoint"`
	Kind         string              `toml:"kind"`
	Events       []string            `toml:"events"`
	HitchEvents  []string            `toml:"hitch_events"`
	SourceEvents []SourceEventFilter `toml:"source_events"`
	Payload      string              `toml:"payload"`
	TimeoutMS    int                 `toml:"timeout_ms"`
	OnError      string              `toml:"on_error"`
	OnTimeout    string              `toml:"on_timeout"`
}

func DefaultExtensionDir() string {
	return ExpandHome("~/.config/hitch/extensions")
}

func LoadWithExtensionDir(path, extensionDir string) (Config, error) {
	cfg, err := loadFile(path, false)
	if err != nil {
		return Config{}, err
	}
	if err := mergeExtensions(&cfg, extensionDir); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func mergeExtensions(cfg *Config, root string) error {
	if root == "" {
		return nil
	}
	root = ExpandHome(root)
	resolvable := map[string]struct{}{}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return validateNativeExtensionReferences(cfg, resolvable)
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		configPath := filepath.Join(root, name, "config.toml")
		if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		resolvable[name] = struct{}{}

		if h, ok := cfg.Handlers[name]; ok && h.Type == "native" && h.Extension == "" {
			h.Extension = name
			cfg.Handlers[name] = h
		}

		_, sameNameExists := cfg.Handlers[name]
		referenced := referencesExtension(cfg, name)
		needsMerge := referenced && needsExtensionDefaults(cfg, name)
		if sameNameExists && !needsMerge {
			continue
		}
		if referenced && !needsMerge {
			continue
		}
		ext, err := readExtensionConfig(root, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if referenced {
			mergeExtensionDefaults(cfg, name, ext)
			continue
		}

		h := handlerFromExtension(ext)
		if err := normalizeHandlerConfig(ext.Name, &h); err != nil {
			return err
		}
		if h.Entrypoint == "" {
			return fmt.Errorf("extension %s entrypoint is required", ext.Name)
		}
		if _, exists := cfg.Handlers[ext.Name]; !exists {
			cfg.Handlers[ext.Name] = h
		}
	}
	return validateNativeExtensionReferences(cfg, resolvable)
}

func readExtensionConfig(root, name string) (extensionFile, error) {
	path := filepath.Join(root, name, "config.toml")
	b, err := os.ReadFile(path)
	if err != nil {
		return extensionFile{}, err
	}
	var ext extensionFile
	md, err := toml.Decode(string(b), &ext)
	if err != nil {
		return extensionFile{}, fmt.Errorf("extension %s config: %w", name, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) != 0 {
		return extensionFile{}, fmt.Errorf("extension %s config: unknown config keys: %v", name, undecoded)
	}
	if ext.Name == "" {
		ext.Name = name
	}
	if ext.Name != name {
		return extensionFile{}, fmt.Errorf("extension %s config name %q must match directory", name, ext.Name)
	}
	normalized := HandlerConfig{
		Events:       append([]string(nil), ext.Events...),
		HitchEvents:  append([]string(nil), ext.HitchEvents...),
		SourceEvents: append([]SourceEventFilter(nil), ext.SourceEvents...),
		Payload:      ext.Payload,
	}
	if err := normalizeHandlerConfig(ext.Name, &normalized); err != nil {
		return extensionFile{}, fmt.Errorf("extension %s config: %w", name, err)
	}
	ext.HitchEvents = normalized.HitchEvents
	ext.Payload = normalized.Payload
	return ext, nil
}

func validateNativeExtensionReferences(cfg *Config, resolvable map[string]struct{}) error {
	for name, h := range cfg.Handlers {
		if h.Type != "native" || h.Extension == "" {
			continue
		}
		if _, ok := resolvable[h.Extension]; !ok {
			return fmt.Errorf("handlers.%s.extension references unresolved extension %q", name, h.Extension)
		}
	}
	return nil
}

func referencesExtension(cfg *Config, extensionName string) bool {
	for _, h := range cfg.Handlers {
		if h.Type == "native" && h.Extension == extensionName {
			return true
		}
	}
	return false
}

func needsExtensionDefaults(cfg *Config, extensionName string) bool {
	for _, h := range cfg.Handlers {
		if h.Type != "native" || h.Extension != extensionName {
			continue
		}
		if h.Entrypoint == "" || h.Kind == "" || len(h.HitchEvents) == 0 ||
			(len(h.SourceEvents) == 0 && !h.sourceEventsSet) || !h.payloadSet ||
			h.TimeoutMS == 0 || h.OnError == "" || h.OnTimeout == "" {
			return true
		}
	}
	return false
}

func mergeExtensionDefaults(cfg *Config, extensionName string, ext extensionFile) {
	defaults := handlerFromExtension(ext)
	for name, h := range cfg.Handlers {
		if h.Type != "native" || h.Extension != extensionName {
			continue
		}
		if h.Entrypoint == "" {
			h.Entrypoint = defaults.Entrypoint
		}
		if h.Kind == "" {
			h.Kind = defaults.Kind
		}
		if len(h.HitchEvents) == 0 {
			h.HitchEvents = append([]string(nil), defaults.HitchEvents...)
		}
		if len(h.SourceEvents) == 0 && !h.sourceEventsSet {
			h.SourceEvents = append([]SourceEventFilter(nil), defaults.SourceEvents...)
		}
		if !h.payloadSet && defaults.Payload != "" {
			h.Payload = defaults.Payload
			h.payloadSet = true
		}
		if h.TimeoutMS == 0 {
			h.TimeoutMS = defaults.TimeoutMS
		}
		if h.OnError == "" {
			h.OnError = defaults.OnError
		}
		if h.OnTimeout == "" {
			h.OnTimeout = defaults.OnTimeout
		}
		cfg.Handlers[name] = h
	}
}

func handlerFromExtension(ext extensionFile) HandlerConfig {
	return HandlerConfig{
		Type:         "native",
		Extension:    ext.Name,
		Entrypoint:   ext.Entrypoint,
		Kind:         ext.Kind,
		HitchEvents:  append([]string(nil), ext.HitchEvents...),
		SourceEvents: append([]SourceEventFilter(nil), ext.SourceEvents...),
		Payload:      ext.Payload,
		TimeoutMS:    ext.TimeoutMS,
		OnError:      ext.OnError,
		OnTimeout:    ext.OnTimeout,
	}
}
