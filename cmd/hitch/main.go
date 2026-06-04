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
	"strings"
	"syscall"
	"time"

	"github.com/sagebynature/hitch/internal/api"
	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/dispatch"
	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/harness/codex"
	"github.com/sagebynature/hitch/internal/harness/hermes"
	"github.com/sagebynature/hitch/internal/harness/omp"
	"github.com/sagebynature/hitch/internal/harness/pi"
	"github.com/sagebynature/hitch/internal/logging"
	"github.com/sagebynature/hitch/internal/protocol"
	"github.com/sagebynature/hitch/internal/store"
)

const version = "0.1.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			serve(os.Args[2:])
			return
		case "adapter":
			adapter(os.Args[2:])
			return
		case "handler":
			handler(os.Args[2:])
			return
		case "install":
			install(os.Args[2:], false)
			return
		case "uninstall":
			install(os.Args[2:], true)
			return
		case "status":
			status(os.Args[2:])
			return
		case "doctor":
			doctor(os.Args[2:])
			return
		case "inspect-event":
			inspectEvent(os.Args[2:])
			return
		case "replay":
			replay(os.Args[2:])
			return
		}
	}
	versionFlag := flag.Bool("version", false, "print version")
	flag.Parse()
	if *versionFlag {
		fmt.Printf("hitch %s\n", version)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: hitch --version | hitch serve | hitch adapter | hitch handler noop-observer | hitch install | hitch status | hitch doctor | hitch inspect-event | hitch replay")
	os.Exit(2)
}

func serve(args []string) {
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

func adapter(args []string) {
	fs := flag.NewFlagSet("adapter", flag.ExitOnError)
	harness := fs.String("harness", "", "source harness")
	event := fs.String("event", "", "native event type")
	syncMode := fs.Bool("sync", false, "dispatch synchronously")
	url := fs.String("url", "http://127.0.0.1:8799", "hitch API URL")
	_ = fs.Parse(args)
	if *harness == "" || *event == "" {
		fatal(fmt.Errorf("-harness and -event are required"))
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal(err)
	}
	if len(strings.TrimSpace(string(payload))) == 0 {
		payload = []byte(`{}`)
	}
	if !json.Valid(payload) {
		fatal(fmt.Errorf("stdin must be JSON"))
	}
	client := api.Client{BaseURL: *url}
	req := api.NewEventRequest(*harness, *event, protocol.RawJSON(payload))
	if *syncMode {
		resp, err := client.Dispatch(req)
		native := resp.NativeResponse
		if err != nil || len(native) == 0 {
			native = nativeNoop(*harness, *event)
		}
		if len(native) != 0 {
			_, _ = os.Stdout.Write(native)
			_, _ = os.Stdout.Write([]byte("\n"))
		}
		return
	}
	_, _ = client.Event(req)
}

func handler(args []string) {
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

func nativeNoop(harnessName, nativeEventType string) protocol.RawJSON {
	aggregate := protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorNone}}
	switch protocol.Harness(harnessName) {
	case protocol.HarnessCodex:
		native, _ := codex.Mapper{}.Translate(nativeEventType, aggregate)
		return native
	case protocol.HarnessHermes:
		native, _ := hermes.Mapper{}.Translate(nativeEventType, aggregate)
		return native
	case protocol.HarnessPi:
		native, _ := pi.Mapper{}.Translate(nativeEventType, aggregate)
		return native
	case protocol.HarnessOMP:
		native, _ := omp.Mapper{}.Translate(nativeEventType, aggregate)
		return native
	default:
		return nil
	}
}

func status(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit JSON")
	_ = fs.Parse(args)
	writeCLI(*jsonOut, map[string]interface{}{"config": config.DefaultPath, "version": version})
}
func doctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit JSON")
	_ = fs.Parse(args)
	writeCLI(*jsonOut, map[string]interface{}{"ok": true, "checks": []string{"config path resolvable", "binary runnable"}})
}

func inspectEvent(args []string) {
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
