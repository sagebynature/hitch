package harness

import (
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestDefaultRegistryContainsSupportedHarnesses(t *testing.T) {
	registry := DefaultRegistry()
	for _, h := range []protocol.Harness{
		protocol.HarnessCodex,
		protocol.HarnessHermes,
		protocol.HarnessPi,
		protocol.HarnessOMP,
		protocol.HarnessOpenCode,
	} {
		if _, ok := registry.Lookup(h); !ok {
			t.Fatalf("missing harness %s", h)
		}
	}
}

func TestRegistryRejectsUnknownHarness(t *testing.T) {
	registry := DefaultRegistry()
	if _, ok := registry.Lookup(protocol.Harness("unknown")); ok {
		t.Fatal("unknown harness was registered")
	}
}
