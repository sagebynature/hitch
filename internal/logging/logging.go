package logging

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sage-scm/hitch/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

func New(cfg config.LogConfig) (*slog.Logger, io.Closer, error) {
	var writers []io.Writer
	var closers []io.Closer

	if cfg.Stdout.Enabled {
		writers = append(writers, os.Stdout)
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
		writers = append(writers, lj)
		closers = append(closers, lj)
	}
	if len(writers) == 0 {
		return nil, nil, errors.New("at least one log sink must be enabled")
	}

	level := slog.LevelInfo
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	out := io.MultiWriter(writers...)
	var handler slog.Handler
	if cfg.Format == "console" {
		handler = slog.NewTextHandler(out, opts)
	} else {
		handler = slog.NewJSONHandler(out, opts)
	}
	return slog.New(handler), multiCloser(closers), nil
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
