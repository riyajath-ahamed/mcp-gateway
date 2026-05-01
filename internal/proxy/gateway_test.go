package proxy_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/configkits/mcp-gateway/internal/config"
	"github.com/configkits/mcp-gateway/internal/proxy"
	"github.com/configkits/mcp-gateway/internal/registry"
)

func testGateway(t *testing.T, tools []registry.MCPTool) *proxy.Gateway {
	t.Helper()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{Port: 8080},
		Servers: []config.ServerConfig{
			{Name: "mock-server", Transport: "http", URL: "http://localhost:9999"},
		},
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reg := registry.New(cfg, logger)
	registry.InjectTools(reg, "mock-server", tools)
	return proxy.NewGateway(reg, cfg, logger)
}

func rpcBody(method string, params any) *bytes.Buffer {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	return bytes.NewBuffer(b)
}

func TestGateway_ToolsList(t *testing.T) {
	gw := testGateway(t, []registry.MCPTool{
		{Name: "read_file", ServerName: "mock-server"},
		{Name: "write_file", ServerName: "mock-server"},
	})
	req := httptest.NewRequest(http.MethodGet, "/mcp/tools/list", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result struct {
			Tools []registry.MCPTool `json:"tools"`
		} `json:"result"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Result.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(resp.Result.Tools))
	}
}

func TestGateway_ToolsList_ViaPost(t *testing.T) {
	gw := testGateway(t, []registry.MCPTool{{Name: "tool_a", ServerName: "mock-server"}})
	req := httptest.NewRequest(http.MethodPost, "/mcp", rpcBody("tools/list", map[string]any{}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGateway_ToolCall_UnknownTool(t *testing.T) {
	gw := testGateway(t, []registry.MCPTool{{Name: "read_file", ServerName: "mock-server"}})
	req := httptest.NewRequest(http.MethodPost, "/mcp", rpcBody("tools/call", map[string]any{"name": "ghost_tool"}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestGateway_InvalidJSON(t *testing.T) {
	gw := testGateway(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString("{not json}"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGateway_AuthBlocksWithoutKey(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: config.AuthConfig{Type: "api-key", Header: "X-Gateway-Key", Secret: "my-secret"},
		},
		Servers: []config.ServerConfig{{Name: "s", Transport: "http", URL: "http://localhost:9999"}},
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reg := registry.New(cfg, logger)
	gw := proxy.NewGateway(reg, cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/mcp/tools/list", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestGateway_AuthAllowsWithKey(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: config.AuthConfig{Type: "api-key", Header: "X-Gateway-Key", Secret: "my-secret"},
		},
		Servers: []config.ServerConfig{{Name: "s", Transport: "http", URL: "http://localhost:9999"}},
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reg := registry.New(cfg, logger)
	registry.InjectTools(reg, "s", []registry.MCPTool{{Name: "t", ServerName: "s"}})
	gw := proxy.NewGateway(reg, cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/mcp/tools/list", nil)
	req.Header.Set("X-Gateway-Key", "my-secret")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
