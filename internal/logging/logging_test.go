package logging

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestNewUsesPerSinkLevelAndFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hitch.log")
	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writePipe
	defer func() { os.Stdout = oldStdout }()

	cfg := config.LogConfig{
		Level:  "info",
		Format: "json",
		Stdout: config.LogStdout{Enabled: true, Level: "info", Format: "console"},
		File:   config.LogFile{Enabled: true, Level: "debug", Format: "json", Path: path, MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1},
	}
	logger, closer, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	logger.Debug("debug-visible-file", "component", "test")
	logger.Info("info-visible-both", "component", "test")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writePipe.Close(); err != nil {
		t.Fatal(err)
	}
	stdoutBytes, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatal(err)
	}
	stdout := string(stdoutBytes)
	if !strings.Contains(stdout, "msg=info-visible-both") || !strings.Contains(stdout, "component=test") {
		t.Fatalf("stdout did not use console format for info log: %q", stdout)
	}
	if strings.Contains(stdout, "debug-visible-file") || strings.Contains(stdout, `"msg":"info-visible-both"`) {
		t.Fatalf("stdout did not honor level/format override: %q", stdout)
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file := string(fileBytes)
	if !strings.Contains(file, `"msg":"debug-visible-file"`) || !strings.Contains(file, `"msg":"info-visible-both"`) {
		t.Fatalf("file did not use debug level JSON logs: %q", file)
	}
	if strings.Contains(file, "msg=info-visible-both") {
		t.Fatalf("file did not use JSON format: %q", file)
	}
}

func TestNewRejectsNoSink(t *testing.T) {
	_, _, err := New(config.LogConfig{Level: "info", Format: "json"})
	if err == nil {
		t.Fatal("logger without sink accepted")
	}
}
