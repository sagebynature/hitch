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
	cfg, err := loadFile(path)
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
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(root, name, "config.toml")
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var ext extensionFile
		if err := toml.Unmarshal(b, &ext); err != nil {
			return fmt.Errorf("extension %s config: %w", name, err)
		}
		if ext.Name == "" {
			ext.Name = name
		}
		if ext.Name != name {
			return fmt.Errorf("extension %s config name %q must match directory", name, ext.Name)
		}
		h := HandlerConfig{
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
	return nil
}
