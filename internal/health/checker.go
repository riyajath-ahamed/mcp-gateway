package health

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/configkits/mcp-gateway/internal/registry"
)

// Checker runs periodic health checks against all backend servers.
type Checker struct {
	reg    *registry.Registry
	logger *slog.Logger
	client *http.Client
}

func NewChecker(reg *registry.Registry, logger *slog.Logger) *Checker {
	return &Checker{
		reg:    reg,
		logger: logger,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Run starts the health check loop, firing every interval.
// Pass 0 to use the default 30s interval.
func (c *Checker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	c.logger.Info("health check scheduler started", "interval", interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAll(ctx)
		}
	}
}

func (c *Checker) checkAll(ctx context.Context) {
	for _, s := range c.reg.Servers() {
		go c.checkServer(ctx, s)
	}
}

func (c *Checker) checkServer(ctx context.Context, s *registry.ServerState) {
	if s.CircuitOpen {
		c.logger.Info("circuit half-open probe", "server", s.Config.Name)
	}
	switch s.Config.Transport {
	case "http", "sse":
		c.checkHTTP(ctx, s)
	case "stdio":
		s.RecordSuccess()
	}
}

func (c *Checker) checkHTTP(ctx context.Context, s *registry.ServerState) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      "health-probe",
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Config.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		c.markUnhealthy(s, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		c.markUnhealthy(s, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		c.markUnhealthy(s, fmt.Errorf("HTTP %d", resp.StatusCode))
		return
	}

	s.RecordSuccess()
	c.logger.Debug("health check passed", "server", s.Config.Name)
}

func (c *Checker) markUnhealthy(s *registry.ServerState, err error) {
	threshold := 3
	if s.Config.CircuitBreaker != nil {
		threshold = s.Config.CircuitBreaker.Threshold
	}
	s.RecordFailure(threshold)
	if s.CircuitOpen {
		c.logger.Warn("circuit breaker opened", "server", s.Config.Name, "failures", s.FailureCount, "error", err)
	} else {
		c.logger.Warn("health check failed", "server", s.Config.Name, "failures", s.FailureCount, "error", err)
	}
}
