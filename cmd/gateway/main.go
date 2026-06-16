package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/configkits/mcp-gateway/internal/admin"
	"github.com/configkits/mcp-gateway/internal/auth"
	"github.com/configkits/mcp-gateway/internal/config"
	"github.com/configkits/mcp-gateway/internal/health"
	"github.com/configkits/mcp-gateway/internal/middleware"
	"github.com/configkits/mcp-gateway/internal/observability"
	"github.com/configkits/mcp-gateway/internal/proxy"
	"github.com/configkits/mcp-gateway/internal/registry"
	"github.com/configkits/mcp-gateway/internal/router"
	"github.com/configkits/mcp-gateway/internal/version"
)

func main() {
	cfgPath  := flag.String("config", "gateway.yaml", "path to gateway config file")
	genKey   := flag.Bool("gen-key", false, "generate a random API key and exit")
	printVer := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *printVer {
		fmt.Printf("mcp-gateway %s\n", version.String)
		return
	}
	if *genKey {
		key, err := auth.GenerateAPIKey(32)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(key)
		return
	}

	// ── Logger ──────────────────────────────────────────────────────
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// ── Config ──────────────────────────────────────────────────────
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "error", err, "path", *cfgPath)
		os.Exit(1)
	}

	// ── OpenTelemetry ───────────────────────────────────────────────
	otelShutdown, err := observability.InitTracer(cfg.Observability.OTELEndpoint)
	if err != nil {
		slog.Warn("otel init failed, continuing without tracing", "error", err)
		otelShutdown = func(context.Context) error { return nil }
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Registry + bootstrap ────────────────────────────────────────
	reg := registry.New(cfg, logger)
	if err := reg.Bootstrap(ctx); err != nil {
		slog.Error("registry bootstrap failed", "error", err)
		os.Exit(1)
	}

	// ── Router ──────────────────────────────────────────────────────
	rtr, err := router.New(cfg.Servers)
	if err != nil {
		slog.Error("router init failed", "error", err)
		os.Exit(1)
	}

	// ── Health checker ──────────────────────────────────────────────
	hc := health.NewChecker(reg, logger)
	go hc.Run(ctx, 30*time.Second)

	// ── Handlers ────────────────────────────────────────────────────
	gateway := proxy.NewGateway(reg, rtr, cfg, logger)

	mux := http.NewServeMux()
	mux.Handle("/mcp", gateway)
	mux.Handle("/mcp/", gateway)
	mux.HandleFunc("/health", healthEndpoint(reg))
	mux.Handle("/admin/", http.StripPrefix("/admin", admin.Handler()))
	if cfg.Observability.MetricsPath != "" {
		mux.Handle(cfg.Observability.MetricsPath, observability.MetricsHandler())
	}

	// ── Middleware stack (outermost first) ──────────────────────────
	// Recovery → Rate limit → Request logging → Auth → mux
	var handler http.Handler = mux
	handler = auth.Middleware(auth.Config{
		Type:   auth.AuthType(cfg.Gateway.Auth.Type),
		Header: cfg.Gateway.Auth.Header,
		Secret: cfg.Gateway.Auth.Secret,
	}, logger, handler)
	handler = middleware.RequestLogger(logger, handler)
	if cfg.Gateway.RateLimit.RequestsPerSecond > 0 {
		handler = middleware.RateLimit(cfg.Gateway.RateLimit.RequestsPerSecond, cfg.Gateway.RateLimit.Burst, handler)
	}
	handler = middleware.Recovery(logger, handler)

	// ── Server ──────────────────────────────────────────────────────
	addr := cfg.Gateway.Addr()
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	slog.Info("gateway ready",
		"addr", addr,
		"servers", len(cfg.Servers),
		"tools", len(reg.AggregatedTools()),
		"version", version.String,
		"admin", addr+"/admin/",
	)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	_ = otelShutdown(context.Background())
}

func healthEndpoint(reg *registry.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := reg.HealthSummary()
		w.Header().Set("Content-Type", "application/json")
		if status.AllHealthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		w.Write(status.JSON())
	}
}
