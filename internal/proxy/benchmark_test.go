package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/configkits/mcp-gateway/internal/config"
	"github.com/configkits/mcp-gateway/internal/proxy"
	"github.com/configkits/mcp-gateway/internal/registry"
	"github.com/configkits/mcp-gateway/internal/router"
)

func newBenchGateway(b *testing.B, backendURL string) *proxy.Gateway {
	b.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{Name: "bench-server", Transport: "http", URL: backendURL},
		},
	}
	rtr, _ := router.New(cfg.Servers)
	tools := []registry.MCPTool{
		{Name: "bench_tool", Description: "benchmark tool", ServerName: "bench-server"},
	}
	reg := registry.NewForTest(cfg, logger, [][]registry.MCPTool{tools})
	return proxy.NewGateway(reg, rtr, cfg, logger)
}

// BenchmarkToolsList measures tools/list throughput (registry read, no backend call).
func BenchmarkToolsList(b *testing.B) {
	gw := newBenchGateway(b, "http://unused")

	req := httptest.NewRequest(http.MethodGet, "/mcp/tools/list", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		gw.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status: %d", w.Code)
		}
	}
}

// BenchmarkToolCall_E2E measures full roundtrip including backend HTTP call.
func BenchmarkToolCall_E2E(b *testing.B) {
	// Stand up an in-process mock backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": "ok"}},
			},
		})
	}))
	defer backend.Close()

	gw := newBenchGateway(b, backend.URL)
	payload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": "bench_tool", "arguments": map[string]any{}},
	})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		gw.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status: %d", w.Code)
		}
	}
}

// BenchmarkToolCall_Parallel measures concurrent throughput.
func BenchmarkToolCall_Parallel(b *testing.B) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}},
		})
	}))
	defer backend.Close()

	gw := newBenchGateway(b, backend.URL)
	payload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1,
		"method": "tools/call",
		"params": map[string]any{"name": "bench_tool", "arguments": map[string]any{}},
	})

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			gw.ServeHTTP(w, req)
		}
	})
}

// BenchmarkRouter_Prefix benchmarks prefix-based routing resolution.
func BenchmarkRouter_Prefix(b *testing.B) {
	servers := []config.ServerConfig{
		{Name: "github", Transport: "http", URL: "http://unused", Routing: &config.RoutingConfig{Prefix: "github_"}},
		{Name: "fs", Transport: "http", URL: "http://unused", Routing: &config.RoutingConfig{Prefix: "fs_"}},
		{Name: "pg", Transport: "http", URL: "http://unused", Routing: &config.RoutingConfig{Prefix: "pg_"}},
	}
	rtr, _ := router.New(servers)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rtr.Route("github_create_issue")
	}
}

// BenchmarkRateLimit measures the per-IP token-bucket overhead.
func BenchmarkRateLimit(b *testing.B) {
	// Import middleware inline — tests in same module
	_ = os.Stderr // silence unused import
	b.Skip("run with: go test -bench=BenchmarkRateLimit ./internal/middleware/")
}
