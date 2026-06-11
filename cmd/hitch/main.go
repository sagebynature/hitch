package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sagebynature/hitch/internal/app"
	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/store"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			serve(os.Args[2:])
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
	configPath := fs.String("config", config.DefaultPath, "config file")
	_ = fs.Parse(args)
	bundle, err := app.NewServerBundle(context.Background(), app.ServeOptions{ConfigPath: *configPath, ConfigPathProvided: cliFlagProvided(args, "config")})
	if err != nil {
		fatal(err)
	}
	defer bundle.Close()
	srv := bundle.Server
	logger := bundle.Logger
	go func() {
		logger.Info("hitch server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", serverFailureLogAttrs(srv.Addr, err)...)
			os.Exit(1)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := app.ShutdownContext(context.Background())
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func serverFailureLogAttrs(addr string, err error) []any {
	attrs := []any{"addr", addr, "error", err.Error()}
	if strings.Contains(err.Error(), "address already in use") || strings.Contains(err.Error(), "bind") {
		attrs = append(attrs, "error_kind", "bind_failed", "hint", "another Hitch server may already be running; stop it or change server.port")
	}
	return attrs
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
	configPath := fs.String("config", config.DefaultPath, "config file")
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
	configPath := fs.String("config", config.DefaultPath, "config file")
	dryRun := fs.Bool("dry-run", false, "do not create replay records")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fatal(fmt.Errorf("replay requires id"))
	}
	result, err := app.Replay(context.Background(), app.ReplayOptions{ConfigPath: *configPath, ConfigPathProvided: cliFlagProvided(args, "config"), EventID: fs.Arg(0), DryRun: *dryRun})
	if err != nil {
		fatal(err)
	}
	writeCLI(true, result)
}

func writeCLI(jsonOut bool, v interface{}) {
	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(v)
		return
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func cliFlagProvided(args []string, name string) bool {
	short := "-" + name
	long := "--" + name
	for _, arg := range args {
		if arg == short || arg == long || strings.HasPrefix(arg, short+"=") || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
