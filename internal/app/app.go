package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sagebynature/hitch/internal/api"
	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/logging"
	"github.com/sagebynature/hitch/internal/store"
)

func ServerAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

func ShutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 5*time.Second)
}

type ServeOptions struct {
	ConfigPath         string
	ConfigPathProvided bool
}

type ServerBundle struct {
	Server *http.Server
	Logger *slog.Logger
	Close  func() error
}

func resolveConfigPath(path string, provided bool) string {
	if path == "" && !provided {
		return config.DefaultPath
	}
	return path
}

func NewServerBundle(ctx context.Context, opts ServeOptions) (*ServerBundle, error) {
	cfg, err := config.Load(resolveConfigPath(opts.ConfigPath, opts.ConfigPathProvided))
	if err != nil {
		return nil, err
	}
	logger, logCloser, err := logging.New(cfg.Log)
	if err != nil {
		return nil, err
	}
	dbPath := config.ExpandHome(cfg.Audit.SQLite.Path)
	if err := ensureDir(filepath.Dir(dbPath)); err != nil {
		_ = logCloser.Close()
		return nil, err
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		_ = logCloser.Close()
		return nil, err
	}
	srv := &http.Server{
		Addr:              ServerAddr(cfg.Server.Host, cfg.Server.Port),
		Handler:           api.New(cfg, logger, st).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &ServerBundle{
		Server: srv,
		Logger: logger,
		Close: func() error {
			storeErr := st.Close()
			logErr := logCloser.Close()
			if storeErr != nil {
				return storeErr
			}
			return logErr
		},
	}, nil
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
