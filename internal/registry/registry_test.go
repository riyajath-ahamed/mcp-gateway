package registry_test

import (
	"testing"

	"github.com/configkits/mcp-gateway/internal/config"
	"github.com/configkits/mcp-gateway/internal/registry"
	"log/slog"
	"os"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestRouteForTool_ByOwnership(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{Name: "server-a", Transport: "http", URL: "http://localhost:3001"},
			{Name: "server-b", Transport: "http", URL: "http://localhost:3002"},
		},
	}
	reg := registry.New(cfg, testLogger())

	// Manually inject tools (bypass network bootstrap)
	registry.InjectTools(reg, "server-a", []registry.MCPTool{
		{Name: "read_file", Description: "reads a file", ServerName: "server-a"},
		{Name: "write_file", Description: "writes a file", ServerName: "server-a"},
	})
	registry.InjectTools(reg, "server-b", []registry.MCPTool{
		{Name: "github_create_issue", Description: "creates an issue", ServerName: "server-b"},
	})

	s, err := reg.RouteForTool("read_file")
	if err != nil {
		t.Fatalf("expected route for read_file, got error: %v", err)
	}
	if s.Config.Name != "server-a" {
		t.Errorf("expected server-a, got %s", s.Config.Name)
	}

	s, err = reg.RouteForTool("github_create_issue")
	if err != nil {
		t.Fatalf("expected route for github_create_issue, got error: %v", err)
	}
	if s.Config.Name != "server-b" {
		t.Errorf("expected server-b, got %s", s.Config.Name)
	}
}

func TestRouteForTool_ByPrefix(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Name:      "github-mcp",
				Transport: "http",
				URL:       "http://localhost:3002",
				Routing:   &config.RoutingConfig{Prefix: "github_"},
			},
			{
				Name:      "filesystem-mcp",
				Transport: "http",
				URL:       "http://localhost:3001",
			},
		},
	}
	reg := registry.New(cfg, testLogger())
	registry.InjectTools(reg, "github-mcp", []registry.MCPTool{
		{Name: "github_create_pr", ServerName: "github-mcp"},
	})
	registry.InjectTools(reg, "filesystem-mcp", []registry.MCPTool{
		{Name: "read_file", ServerName: "filesystem-mcp"},
	})

	s, err := reg.RouteForTool("github_list_repos")
	if err != nil {
		t.Fatalf("expected prefix route for github_list_repos: %v", err)
	}
	if s.Config.Name != "github-mcp" {
		t.Errorf("expected github-mcp, got %s", s.Config.Name)
	}
}

func TestRouteForTool_NotFound(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{Name: "server-a", Transport: "http", URL: "http://localhost:3001"},
		},
	}
	reg := registry.New(cfg, testLogger())
	registry.InjectTools(reg, "server-a", []registry.MCPTool{
		{Name: "read_file", ServerName: "server-a"},
	})

	_, err := reg.RouteForTool("nonexistent_tool")
	if err == nil {
		t.Error("expected error for nonexistent tool, got nil")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Name:      "flaky-server",
				Transport: "http",
				URL:       "http://localhost:9999",
				CircuitBreaker: &config.CircuitBreakerConfig{Threshold: 3},
			},
		},
	}
	reg := registry.New(cfg, testLogger())
	registry.InjectTools(reg, "flaky-server", []registry.MCPTool{
		{Name: "some_tool", ServerName: "flaky-server"},
	})

	servers := reg.Servers()
	s := servers[0]

	// Should be routable before failures
	_, err := reg.RouteForTool("some_tool")
	if err != nil {
		t.Fatalf("expected tool to be routable before failures: %v", err)
	}

	// Record 3 failures — should open circuit
	s.RecordFailure(3)
	s.RecordFailure(3)
	s.RecordFailure(3)

	if !s.CircuitOpen {
		t.Error("expected circuit to be open after 3 failures")
	}

	// Should no longer be routable
	_, err = reg.RouteForTool("some_tool")
	if err == nil {
		t.Error("expected routing to fail with open circuit")
	}

	// Recovery: RecordSuccess closes circuit
	s.RecordSuccess()
	if s.CircuitOpen {
		t.Error("expected circuit to close after success")
	}
}

func TestAggregatedTools_OnlyHealthyServers(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{Name: "healthy", Transport: "http", URL: "http://localhost:3001"},
			{Name: "unhealthy", Transport: "http", URL: "http://localhost:3002"},
		},
	}
	reg := registry.New(cfg, testLogger())
	registry.InjectTools(reg, "healthy", []registry.MCPTool{
		{Name: "tool_a", ServerName: "healthy"},
	})
	registry.InjectTools(reg, "unhealthy", []registry.MCPTool{
		{Name: "tool_b", ServerName: "unhealthy"},
	})

	// Mark unhealthy server as failed
	for _, s := range reg.Servers() {
		if s.Config.Name == "unhealthy" {
			s.RecordFailure(1)
		}
	}

	tools := reg.AggregatedTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool from healthy server, got %d", len(tools))
	}
	if tools[0].Name != "tool_a" {
		t.Errorf("expected tool_a, got %s", tools[0].Name)
	}
}
