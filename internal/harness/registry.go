package harness

import (
	"github.com/sagebynature/hitch/internal/harness/codex"
	"github.com/sagebynature/hitch/internal/harness/hermes"
	"github.com/sagebynature/hitch/internal/harness/omp"
	"github.com/sagebynature/hitch/internal/harness/opencode"
	"github.com/sagebynature/hitch/internal/harness/pi"
	"github.com/sagebynature/hitch/internal/harness/antigravity"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Registry struct {
	normalizers map[protocol.Harness]Normalizer
}

func NewRegistry(normalizers map[protocol.Harness]Normalizer) Registry {
	out := make(map[protocol.Harness]Normalizer, len(normalizers))
	for h, n := range normalizers {
		out[h] = n
	}
	return Registry{normalizers: out}
}

func DefaultRegistry() Registry {
	return NewRegistry(map[protocol.Harness]Normalizer{
		protocol.HarnessCodex:       codex.Mapper{},
		protocol.HarnessHermes:      hermes.Mapper{},
		protocol.HarnessPi:          pi.Mapper{},
		protocol.HarnessOMP:         omp.Mapper{},
		protocol.HarnessOpenCode:    opencode.Mapper{},
		protocol.HarnessAntigravity: antigravity.Mapper{},
	})
}

func (r Registry) Lookup(h protocol.Harness) (Normalizer, bool) {
	n, ok := r.normalizers[h]
	return n, ok
}
