// examples/go/embedded/main.go
//
// Shows how to embed mcp-gateway into an existing Go HTTP server
// rather than running it as a standalone binary.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/configkits/mcp-gateway/internal/config"
	"github.com/configkits/mcp-gateway/internal/health"
	"github.com/configkits/mcp-gateway/internal/proxy"
	"github.com/configkits/mcp-gateway/internal/registry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg := &config.Config{
		Gateway: config.GatewayConfig{Port: 8080},
		Servers: []config.ServerConfig{
			{
				Name:      "my-tools",
				Transport: "http",
				URL:       "http://localhost:3001",
			},
		},
	}

	ctx := context.Background()

	reg := registry.New(cfg, logger)
	reg.Bootstrap(ctx)

	hc := health.NewChecker(reg, logger)
	go hc.Run(ctx, 0) // 0 uses the default 30s interval

	gateway := proxy.NewGateway(reg, cfg, logger)

	// Mount gateway alongside your own routes
	mux := http.NewServeMux()
	mux.Handle("/mcp/", gateway)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("my app"))
	})

	logger.Info("listening", "addr", ":8080")
	http.ListenAndServe(":8080", mux)
}
