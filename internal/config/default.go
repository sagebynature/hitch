package config

import _ "embed"

// DefaultConfigTOML is the configuration seeded by the installer when
// ~/.config/hitch/config.toml does not exist.
//
//go:embed default.config.toml
var DefaultConfigTOML string
