package registry

import (
	"log/slog"

	"github.com/configkits/mcp-gateway/internal/config"
)

// NewForTest creates a Registry pre-populated with the given tool sets,
// one slice per server in cfg.Servers. Used in unit tests to avoid
// real HTTP bootstrap calls.
func NewForTest(cfg *config.Config, logger *slog.Logger, toolSets [][]MCPTool) *Registry {
	states := make([]*ServerState, 0, len(cfg.Servers))
	for i, s := range cfg.Servers {
		sc := s
		state := &ServerState{
			Config:  sc,
			Healthy: true,
		}
		if i < len(toolSets) {
			state.Tools = toolSets[i]
		}
		states = append(states, state)
	}
	return &Registry{servers: states, logger: logger}
}
