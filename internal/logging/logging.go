package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	sloglogrus "github.com/samber/slog-logrus/v2"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/sagebynature/hitch/internal/config"
)

func New(cfg config.LogConfig) (*slog.Logger, io.Closer, error) {
	var handlers []slog.Handler
	var closers []io.Closer

	if cfg.Stdout.Enabled {
		handlers = append(handlers, newHandler(os.Stdout, sinkLevel(cfg.Level, cfg.Stdout.Level), sinkFormat(cfg.Format, cfg.Stdout.Format), stdoutColorMode()))
	}
	if cfg.File.Enabled {
		path := config.ExpandHome(cfg.File.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, err
		}
		lj := &lumberjack.Logger{
			Filename:   path,
			MaxSize:    cfg.File.MaxSizeMB,
			MaxBackups: cfg.File.MaxBackups,
			MaxAge:     cfg.File.MaxAgeDays,
			Compress:   cfg.File.Compress,
		}
		handlers = append(handlers, newHandler(lj, sinkLevel(cfg.Level, cfg.File.Level), sinkFormat(cfg.Format, cfg.File.Format), consoleColorsDisabled))
		closers = append(closers, lj)
	}
	if len(handlers) == 0 {
		return nil, nil, errors.New("at least one log sink must be enabled")
	}

	return slog.New(fanoutHandler(handlers)), multiCloser(closers), nil
}

func sinkLevel(fallback, override string) string {
	if override != "" {
		return override
	}
	return fallback
}

func sinkFormat(fallback, override string) string {
	if override != "" {
		return override
	}
	return fallback
}

type consoleColorMode uint8

const (
	consoleColorsAuto consoleColorMode = iota
	consoleColorsDisabled
	consoleColorsForced
)

func stdoutColorMode() consoleColorMode {
	if os.Getenv("NO_COLOR") != "" {
		return consoleColorsDisabled
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") != "" {
		return consoleColorsForced
	}
	return consoleColorsAuto
}

func newHandler(out io.Writer, levelName, format string, colorMode consoleColorMode) slog.Handler {
	level := slogLevel(levelName)
	if format == "console" {
		return newLogrusHandler(out, level, colorMode)
	}
	return slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level})
}

func slogLevel(name string) slog.Level {
	switch name {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newLogrusHandler(out io.Writer, level slog.Level, colorMode consoleColorMode) slog.Handler {
	logger := logrus.New()
	logger.SetOutput(out)
	logger.SetLevel(logrusLevel(level))
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:             true,
		ForceColors:               colorMode == consoleColorsForced,
		DisableColors:             colorMode == consoleColorsDisabled,
		EnvironmentOverrideColors: true,
	})
	return sloglogrus.Option{Level: level, Logger: logger}.NewLogrusHandler()
}

func logrusLevel(level slog.Level) logrus.Level {
	switch {
	case level <= slog.LevelDebug:
		return logrus.DebugLevel
	case level <= slog.LevelInfo:
		return logrus.InfoLevel
	case level <= slog.LevelWarn:
		return logrus.WarnLevel
	default:
		return logrus.ErrorLevel
	}
}

type fanoutHandler []slog.Handler

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var first error
	for _, handler := range h {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanoutHandler, 0, len(h))
	for _, handler := range h {
		out = append(out, handler.WithAttrs(attrs))
	}
	return out
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	out := make(fanoutHandler, 0, len(h))
	for _, handler := range h {
		out = append(out, handler.WithGroup(name))
	}
	return out
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var first error
	for _, c := range m {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
