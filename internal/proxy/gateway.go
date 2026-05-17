package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/configkits/mcp-gateway/internal/config"
	"github.com/configkits/mcp-gateway/internal/registry"
	"github.com/configkits/mcp-gateway/internal/router"
	"github.com/google/uuid"
)

// Gateway is the main MCP reverse proxy handler.
type Gateway struct {
	reg    *registry.Registry
	rtr    *router.Router
	cfg    *config.Config
	logger *slog.Logger
	client *http.Client
}

func NewGateway(reg *registry.Registry, rtr *router.Router, cfg *config.Config, logger *slog.Logger) *Gateway {
	return &Gateway{
		reg:    reg,
		rtr:    rtr,
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqID := uuid.New().String()[:8]
	start := time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Gateway-Request-ID", reqID)

	if !g.authenticate(w, r) {
		return
	}

	// Normalise path — strip /mcp prefix
	path := strings.TrimPrefix(r.URL.Path, "/mcp")
	path = strings.Trim(path, "/")

	switch {
	case r.Method == http.MethodGet && (path == "tools/list" || path == ""):
		g.handleToolsList(w, r, reqID)
	case r.Method == http.MethodPost:
		g.handleJSONRPC(w, r, reqID, start)
	case r.Method == http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	default:
		g.writeError(w, http.StatusMethodNotAllowed, -32600, "method not allowed")
	}
}

func (g *Gateway) handleToolsList(w http.ResponseWriter, _ *http.Request, reqID string) {
	tools := g.reg.AggregatedTools()
	g.logger.Info("tools/list", "request_id", reqID, "count", len(tools))
	json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"result":  map[string]any{"tools": tools},
	})
}

func (g *Gateway) handleJSONRPC(w http.ResponseWriter, r *http.Request, reqID string, start time.Time) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20)) // 4 MB max
	if err != nil {
		g.writeError(w, http.StatusBadRequest, -32700, "failed to read request body")
		return
	}

	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		g.writeError(w, http.StatusBadRequest, -32700, "invalid JSON-RPC request")
		return
	}

	g.logger.Debug("incoming request", "request_id", reqID, "method", req.Method)

	switch req.Method {
	case "tools/list":
		g.handleToolsList(w, r, reqID)
	case "tools/call":
		g.handleToolCall(w, r, req, reqID, start, body)
	default:
		g.forwardToFirst(w, r, body, reqID)
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (g *Gateway) handleToolCall(w http.ResponseWriter, r *http.Request, req jsonRPCRequest, reqID string, start time.Time, rawBody []byte) {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		g.writeError(w, http.StatusBadRequest, -32602, "invalid tools/call params")
		return
	}

	server, err := g.resolveServer(params.Name)
	if err != nil {
		g.logger.Warn("no route for tool", "tool", params.Name, "request_id", reqID)
		g.writeError(w, http.StatusNotFound, -32601, "tool not found: "+params.Name)
		return
	}

	g.logger.Info("routing tool call",
		"request_id", reqID,
		"tool", params.Name,
		"backend", server.Config.Name,
	)

	resp, err := g.forwardRequest(r, server, rawBody)
	if err != nil {
		threshold := 3
		if server.Config.CircuitBreaker != nil {
			threshold = server.Config.CircuitBreaker.Threshold
		}
		server.RecordFailure(threshold)
		g.logger.Error("backend error", "request_id", reqID, "backend", server.Config.Name, "error", err)
		g.writeError(w, http.StatusBadGateway, -32002, "backend error: "+err.Error())
		return
	}
	defer resp.Body.Close()
	server.RecordSuccess()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	g.logger.Info("request complete",
		"request_id", reqID,
		"tool", params.Name,
		"backend", server.Config.Name,
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// resolveServer finds the backend server for a tool name using:
// 1. Router rules (prefix/pattern) → 2. Capability ownership
func (g *Gateway) resolveServer(toolName string) (*registry.ServerState, error) {
	// Check router rules first (prefix, pattern)
	if g.rtr != nil {
		match, err := g.rtr.Route(toolName)
		if err != nil {
			return nil, err
		}
		if match != nil {
			for _, s := range g.reg.Servers() {
				if s.Config.Name == match.ServerName && s.IsAvailable() {
					return s, nil
				}
			}
		}
	}
	// Fall back to registry capability lookup
	return g.reg.RouteForTool(toolName)
}

func (g *Gateway) forwardToFirst(w http.ResponseWriter, r *http.Request, body []byte, reqID string) {
	for _, s := range g.reg.Servers() {
		if !s.IsAvailable() {
			continue
		}
		resp, err := g.forwardRequest(r, s, body)
		if err != nil {
			s.RecordFailure(3)
			continue
		}
		defer resp.Body.Close()
		s.RecordSuccess()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}
	g.writeError(w, http.StatusServiceUnavailable, -32003, "no healthy backend available")
}

func (g *Gateway) forwardRequest(r *http.Request, server *registry.ServerState, body []byte) (*http.Response, error) {
	targetURL := server.Config.URL + "/mcp"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set("X-Gateway-Server", server.Config.Name)

	for _, h := range []string{"Accept", "User-Agent"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	if server.Config.Auth != nil {
		switch server.Config.Auth.Type {
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+server.Config.Auth.Token)
		case "api-key":
			hdr := server.Config.Auth.Header
			if hdr == "" {
				hdr = "X-API-Key"
			}
			req.Header.Set(hdr, server.Config.Auth.Token)
		case "basic":
			req.Header.Set("Authorization", "Basic "+server.Config.Auth.Token)
		}
	}

	return g.client.Do(req)
}

func (g *Gateway) authenticate(w http.ResponseWriter, r *http.Request) bool {
	auth := g.cfg.Gateway.Auth
	if auth.Type == "" || auth.Type == "none" {
		return true
	}
	if auth.Type == "api-key" {
		if r.Header.Get(auth.Header) != auth.Secret {
			g.writeError(w, http.StatusUnauthorized, -32001, "unauthorized")
			return false
		}
	}
	return true
}

func (g *Gateway) writeError(w http.ResponseWriter, httpStatus, code int, message string) {
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"error":   map[string]any{"code": code, "message": message},
	})
}
