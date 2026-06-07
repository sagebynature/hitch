package app

import (
	"context"
	"testing"
	"time"
)

func TestServerAddr(t *testing.T) {
	got := ServerAddr("127.0.0.1", 8799)
	if got != "127.0.0.1:8799" {
		t.Fatalf("got %q", got)
	}
}

func TestShutdownTimeoutIsFiveSeconds(t *testing.T) {
	ctx, cancel := ShutdownContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("shutdown context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 4*time.Second || remaining > 5*time.Second {
		t.Fatalf("unexpected shutdown timeout %s", remaining)
	}
}

func TestResolveConfigPathDefaultsOnlyWhenFlagNotProvided(t *testing.T) {
	if got := resolveConfigPath("", false); got == "" {
		t.Fatal("empty unprovided config path did not default")
	}
	if got := resolveConfigPath("", true); got != "" {
		t.Fatalf("explicit empty config path was defaulted to %q", got)
	}
	if got := resolveConfigPath("/tmp/hitch.toml", true); got != "/tmp/hitch.toml" {
		t.Fatalf("explicit config path changed to %q", got)
	}
}
