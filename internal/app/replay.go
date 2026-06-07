package app

import (
	"context"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/dispatch"
	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
	"github.com/sagebynature/hitch/internal/store"
)

type ReplayOptions struct {
	ConfigPath         string
	ConfigPathProvided bool
	EventID            string
	DryRun             bool
}

type ReplayResult struct {
	DryRun    bool                        `json:"dry_run"`
	Event     protocol.EventEnvelope      `json:"event"`
	Aggregate *protocol.AggregateDecision `json:"aggregate,omitempty"`
}

func Replay(ctx context.Context, opts ReplayOptions) (ReplayResult, error) {
	cfg, err := config.Load(resolveConfigPath(opts.ConfigPath, opts.ConfigPathProvided))
	if err != nil {
		return ReplayResult{}, err
	}
	st, err := store.Open(ctx, config.ExpandHome(cfg.Audit.SQLite.Path))
	if err != nil {
		return ReplayResult{}, err
	}
	defer st.Close()
	env, err := st.GetEvent(ctx, opts.EventID)
	if err != nil {
		return ReplayResult{}, err
	}
	if opts.DryRun {
		return ReplayResult{DryRun: true, Event: env}, nil
	}
	result := dispatch.NewRunner(cfg.Handlers).Dispatch(ctx, env, "control", 2*time.Second)
	for _, inv := range result.Invocations {
		err := st.InsertHandlerInvocation(ctx, store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: opts.EventID, HandlerName: inv.HandlerName, Kind: inv.Kind, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error, ReplaySourceID: opts.EventID})
		if err != nil {
			return ReplayResult{}, err
		}
	}
	return ReplayResult{DryRun: false, Event: env, Aggregate: &result.Aggregate}, nil
}
