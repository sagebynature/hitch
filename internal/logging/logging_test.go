package logging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sagebynature/hitch/internal/config"
)

func TestNewFileLogger(t *testing.T) {
	dir := t.TempDir()
	cfg := config.LogConfig{
		Level: "info", Format: "json",
		File: config.LogFile{Enabled: true, Path: filepath.Join(dir, "hitch.log"), MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1},
	}
	logger, closer, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer closer.Close()
	logger.Info("hello", "native_payload", "should only appear if caller adds it")
	if _, err := os.Stat(filepath.Join(dir, "hitch.log")); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
}

func TestNewRejectsNoSink(t *testing.T) {
	_, _, err := New(config.LogConfig{Level: "info", Format: "json"})
	if err == nil {
		t.Fatal("logger without sink accepted")
	}
}
