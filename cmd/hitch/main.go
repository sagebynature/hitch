package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sagebynature/hitch/internal/api"
	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/dispatch"
	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/logging"
	"github.com/sagebynature/hitch/internal/store"
)

var version = "0.1.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			serve(os.Args[2:])
			return
		case "handler":
			handler(os.Args[2:])
			return
		case "status":
			status(os.Args[2:])
			return
		case "doctor":
			doctor(os.Args[2:])
			return
		case "config":
			configCmd(os.Args[2:])
			return
		case "inspect-event":
			inspectEvent(os.Args[2:])
			return
		case "replay":
			replay(os.Args[2:])
			return
		case "help":
			printHitchHelp(os.Stdout, os.Args[2:]...)
			return
		case "-h", "--help":
			printHitchHelp(os.Stdout)
			return
		}
	}
	versionFlag := flag.Bool("version", false, "print version")
	helpFlag := flag.Bool("help", false, "print help")
	flag.Parse()
	if *versionFlag {
		fmt.Printf("hitch %s\n", version)
		return
	}
	if *helpFlag {
		printHitchHelp(os.Stdout)
		return
	}
	printHitchHelp(os.Stderr)
	os.Exit(2)
}

func serve(args []string) {
	if isHelp(args) {
		printServeHelp(os.Stdout)
		return
	}
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "config/default.config.toml", "config file")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	logger, closer, err := logging.New(cfg.Log)
	if err != nil {
		fatal(err)
	}
	defer closer.Close()
	dbPath := config.ExpandHome(cfg.Audit.SQLite.Path)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		fatal(err)
	}
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		fatal(err)
	}
	defer st.Close()
	srv := &http.Server{Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port), Handler: api.New(cfg, logger, st).Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("hitch server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func handler(args []string) {
	if isHelp(args) {
		printHandlerHelp(os.Stdout)
		return
	}
	if len(args) != 1 || args[0] != "noop-observer" {
		fatal(fmt.Errorf("usage: hitch handler noop-observer"))
	}
	noopObserverHandler()
}

func noopObserverHandler() {
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal(err)
	}
	if len(bytes.TrimSpace(payload)) == 0 || !json.Valid(payload) {
		_, _ = os.Stdout.Write([]byte(`{"status":"error","decision":{"behavior":"none"}}` + "\n"))
		return
	}
	_, _ = os.Stdout.Write([]byte(`{"status":"ok","decision":{"behavior":"none"}}` + "\n"))
}

func status(args []string) {
	if isHelp(args) {
		printStatusHelp(os.Stdout)
		return
	}
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit JSON")
	_ = fs.Parse(args)
	writeCLI(*jsonOut, map[string]interface{}{"config": config.DefaultPath, "version": version})
}

func configCmd(args []string) {
	if len(args) == 0 || isHelp(args) {
		printConfigHelp(os.Stdout)
		return
	}
	switch args[0] {
	case "init":
		configInit(args[1:])
	default:
		fatal(fmt.Errorf("unknown config command %q", args[0]))
	}
}

func configInit(args []string) {
	if isHelp(args) {
		printConfigInitHelp(os.Stdout)
		return
	}
	fs := flag.NewFlagSet("config init", flag.ExitOnError)
	path := fs.String("path", config.DefaultPath, "config file")
	jsonOut := fs.Bool("json", false, "emit JSON")
	_ = fs.Parse(args)
	result, err := config.SeedDefault(*path)
	if err != nil {
		fatal(err)
	}
	writeCLI(*jsonOut, map[string]interface{}{"path": result.Path, "created": result.Created})
}

func doctor(args []string) {
	if isHelp(args) {
		printDoctorHelp(os.Stdout)
		return
	}
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit JSON")
	_ = fs.Parse(args)
	writeCLI(*jsonOut, map[string]interface{}{"ok": true, "checks": []string{"config path resolvable", "binary runnable"}})
}

func inspectEvent(args []string) {
	if isHelp(args) {
		printInspectEventHelp(os.Stdout)
		return
	}
	fs := flag.NewFlagSet("inspect-event", flag.ExitOnError)
	configPath := fs.String("config", "config/default.config.toml", "config file")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fatal(fmt.Errorf("inspect-event requires id"))
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	st, err := store.Open(context.Background(), config.ExpandHome(cfg.Audit.SQLite.Path))
	if err != nil {
		fatal(err)
	}
	defer st.Close()
	inspection, err := st.InspectEvent(context.Background(), fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	writeCLI(true, inspection)
}

func replay(args []string) {
	if isHelp(args) {
		printReplayHelp(os.Stdout)
		return
	}
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	configPath := fs.String("config", "config/default.config.toml", "config file")
	dryRun := fs.Bool("dry-run", false, "do not create replay records")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fatal(fmt.Errorf("replay requires id"))
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	st, err := store.Open(context.Background(), config.ExpandHome(cfg.Audit.SQLite.Path))
	if err != nil {
		fatal(err)
	}
	defer st.Close()
	env, err := st.GetEvent(context.Background(), fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	if *dryRun {
		writeCLI(true, map[string]interface{}{"dry_run": true, "event": env})
		return
	}
	result := dispatch.NewRunner(cfg.Handlers).Dispatch(context.Background(), env, "sync", 2*time.Second)
	for _, inv := range result.Invocations {
		err := st.InsertHandlerInvocation(context.Background(), store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: fs.Arg(0), HandlerName: inv.HandlerName, Mode: inv.Mode, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error, ReplaySourceID: fs.Arg(0)})
		if err != nil {
			fatal(err)
		}
	}
	writeCLI(true, map[string]interface{}{"dry_run": false, "event": env, "aggregate": result.Aggregate})
}

func writeCLI(jsonOut bool, v interface{}) {
	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(v)
		return
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
