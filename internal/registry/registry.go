package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/configkits/mcp-gateway/internal/config"
)

// MCPTool represents a tool discovered from a backend server.
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	ServerName  string          `json:"serverName"` // which backend owns this tool
}

// ServerState tracks runtime state for a backend server.
type ServerState struct {
	Config       config.ServerConfig
	Tools        []MCPTool
	Healthy      bool
	LastCheck    time.Time
	FailureCount int
	CircuitOpen  bool

	mu sync.RWMutex
}

func (s *ServerState) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Healthy && !s.CircuitOpen
}

func (s *ServerState) RecordFailure(threshold int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FailureCount++
	if threshold > 0 && s.FailureCount >= threshold {
		s.CircuitOpen = true
	}
	s.Healthy = false
}

func (s *ServerState) RecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FailureCount = 0
	s.CircuitOpen = false
	s.Healthy = true
	s.LastCheck = time.Now()
}

// Registry holds all registered backend servers and aggregated capabilities.
type Registry struct {
	servers []*ServerState
	logger  *slog.Logger
	mu      sync.RWMutex
}

// HealthSummary is returned by the /health endpoint.
type HealthSummary struct {
	AllHealthy bool
	Servers    []ServerHealth
}

type ServerHealth struct {
	Name        string `json:"name"`
	Healthy     bool   `json:"healthy"`
	CircuitOpen bool   `json:"circuitOpen"`
	FailureCount int   `json:"failureCount"`
	LastCheck   string `json:"lastCheck"`
}

func (h HealthSummary) JSON() []byte {
	type payload struct {
		Status  string         `json:"status"`
		Servers []ServerHealth `json:"servers"`
	}
	status := "healthy"
	if !h.AllHealthy {
		status = "degraded"
	}
	b, _ := json.Marshal(payload{Status: status, Servers: h.Servers})
	return b
}

func New(cfg *config.Config, logger *slog.Logger) *Registry {
	states := make([]*ServerState, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		sc := s // copy
		states = append(states, &ServerState{
			Config:  sc,
			Healthy: true,
		})
	}
	return &Registry{servers: states, logger: logger}
}

// Bootstrap connects to all backends and discovers their tools.
func (r *Registry) Bootstrap(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(r.servers))

	for _, s := range r.servers {
		wg.Add(1)
		go func(state *ServerState) {
			defer wg.Done()
			tools, err := r.discoverTools(ctx, state)
			if err != nil {
				r.logger.Warn("tool discovery failed",
					"server", state.Config.Name,
					"error", err,
				)
				state.RecordFailure(0)
				return
			}
			state.mu.Lock()
			state.Tools = tools
			state.Healthy = true
			state.mu.Unlock()
			r.logger.Info("registered server",
				"server", state.Config.Name,
				"tools", len(tools),
			)
		}(s)
	}
	wg.Wait()
	close(errs)

	totalTools := r.AggregatedTools()
	r.logger.Info("capability aggregation complete",
		"total_tools", len(totalTools),
		"servers", len(r.servers),
	)
	return nil
}

// discoverTools calls tools/list on a backend server.
func (r *Registry) discoverTools(ctx context.Context, state *ServerState) ([]MCPTool, error) {
	switch state.Config.Transport {
	case "http", "sse":
		return r.discoverHTTPTools(ctx, state)
	case "stdio":
		// STDIO discovery would spawn the subprocess and exchange JSON-RPC.
		// Stub: return empty and let the subprocess be managed separately.
		r.logger.Warn("stdio tool discovery not yet implemented, server registered with 0 tools",
			"server", state.Config.Name)
		return []MCPTool{}, nil
	default:
		return nil, fmt.Errorf("unknown transport: %s", state.Config.Transport)
	}
}

// discoverHTTPTools sends a JSON-RPC tools/list request to an HTTP MCP server.
func (r *Registry) discoverHTTPTools(ctx context.Context, state *ServerState) ([]MCPTool, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, state.Config.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	r.applyServerAuth(req, state.Config.Auth)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Result struct {
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("server error: %s", result.Error.Message)
	}

	tools := make([]MCPTool, 0, len(result.Result.Tools))
	for _, t := range result.Result.Tools {
		tools = append(tools, MCPTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			ServerName:  state.Config.Name,
		})
	}
	return tools, nil
}

func (r *Registry) applyServerAuth(req *http.Request, auth *config.ServerAuth) {
	if auth == nil {
		return
	}
	switch auth.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+auth.Token)
	case "api-key":
		header := auth.Header
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, auth.Token)
	case "basic":
		// Token is "user:pass" base64 encoded
		req.Header.Set("Authorization", "Basic "+auth.Token)
	}
}

// AggregatedTools returns all tools from all healthy servers.
func (r *Registry) AggregatedTools() []MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []MCPTool
	for _, s := range r.servers {
		s.mu.RLock()
		if s.Healthy {
			all = append(all, s.Tools...)
		}
		s.mu.RUnlock()
	}
	return all
}

// RouteForTool finds the server responsible for a given tool name.
func (r *Registry) RouteForTool(toolName string) (*ServerState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// First: check routing rules (prefix match)
	for _, s := range r.servers {
		if !s.IsAvailable() {
			continue
		}
		if s.Config.Routing != nil && s.Config.Routing.Prefix != "" {
			if strings.HasPrefix(toolName, s.Config.Routing.Prefix) {
				return s, nil
			}
		}
	}

	// Second: find by tool ownership
	for _, s := range r.servers {
		if !s.IsAvailable() {
			continue
		}
		s.mu.RLock()
		for _, t := range s.Tools {
			if t.Name == toolName {
				s.mu.RUnlock()
				return s, nil
			}
		}
		s.mu.RUnlock()
	}

	return nil, fmt.Errorf("no available server for tool: %s", toolName)
}

// Servers returns all server states (for health checks etc).
func (r *Registry) Servers() []*ServerState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.servers
}

func (r *Registry) HealthSummary() HealthSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	allHealthy := true
	shs := make([]ServerHealth, 0, len(r.servers))
	for _, s := range r.servers {
		s.mu.RLock()
		h := ServerHealth{
			Name:         s.Config.Name,
			Healthy:      s.Healthy,
			CircuitOpen:  s.CircuitOpen,
			FailureCount: s.FailureCount,
			LastCheck:    s.LastCheck.Format(time.RFC3339),
		}
		s.mu.RUnlock()
		if !h.Healthy {
			allHealthy = false
		}
		shs = append(shs, h)
	}
	return HealthSummary{AllHealthy: allHealthy, Servers: shs}
}
