package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/configkits/mcp-gateway/internal/config"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "gateway-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	return f.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	y := `
gateway:
  port: 9090
  auth:
    type: "api-key"
    header: "X-Key"
    secret: "supersecret"
servers:
  - name: "test-server"
    transport: "http"
    url: "http://localhost:3001"
observability:
  log_format: "json"
`
	cfg, err := config.Load(writeTemp(t, y))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Gateway.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Gateway.Port)
	}
	if len(cfg.Servers) != 1 || cfg.Servers[0].Name != "test-server" {
		t.Errorf("unexpected servers: %+v", cfg.Servers)
	}
}

func TestLoad_EnvExpansion(t *testing.T) {
	t.Setenv("TEST_TOKEN", "abc123")
	y := `
servers:
  - name: "env-server"
    transport: "http"
    url: "http://localhost:3001"
    auth:
      type: "bearer"
      token: "${TEST_TOKEN}"
`
	cfg, err := config.Load(writeTemp(t, y))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servers[0].Auth.Token != "abc123" {
		t.Errorf("expected abc123, got %q", cfg.Servers[0].Auth.Token)
	}
}

func TestLoad_DefaultPort(t *testing.T) {
	y := "servers:\n  - name: s\n    transport: http\n    url: http://localhost:3001\n"
	cfg, err := config.Load(writeTemp(t, y))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gateway.Addr() != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.Gateway.Addr())
	}
}

func TestLoad_NoServers(t *testing.T) {
	y := "gateway:\n  port: 8080\n"
	_, err := config.Load(writeTemp(t, y))
	if err == nil {
		t.Error("expected error for empty servers list")
	}
}

func TestLoad_DuplicateServerName(t *testing.T) {
	y := `
servers:
  - name: dup
    transport: http
    url: http://localhost:3001
  - name: dup
    transport: http
    url: http://localhost:3002
`
	_, err := config.Load(writeTemp(t, y))
	if err == nil {
		t.Error("expected error for duplicate server name")
	}
}

func TestLoad_MissingURL(t *testing.T) {
	y := "servers:\n  - name: no-url\n    transport: http\n"
	_, err := config.Load(writeTemp(t, y))
	if err == nil {
		t.Error("expected error for missing URL on http transport")
	}
}

func TestLoad_StdioMissingCommand(t *testing.T) {
	y := "servers:\n  - name: no-cmd\n    transport: stdio\n"
	_, err := config.Load(writeTemp(t, y))
	if err == nil {
		t.Error("expected error for stdio without command")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}
