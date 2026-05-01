package middleware_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/configkits/mcp-gateway/internal/middleware"
)

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

var panicHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	panic("test panic")
})

// ── Recovery ─────────────────────────────────────────────────────────

func TestRecovery_NoPanic(t *testing.T) {
	h := middleware.Recovery(logger, okHandler)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRecovery_Panic(t *testing.T) {
	h := middleware.Recovery(logger, panicHandler)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	w := httptest.NewRecorder()
	// Should not propagate the panic
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Rate limiter ─────────────────────────────────────────────────────

func TestRateLimit_AllowsUnderBurst(t *testing.T) {
	h := middleware.RateLimit(100, 5, okHandler) // burst of 5
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimit_BlocksOverBurst(t *testing.T) {
	h := middleware.RateLimit(1, 2, okHandler) // burst of 2, refill 1/s
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.RemoteAddr = "10.0.0.2:5678"

	statuses := make([]int, 5)
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		statuses[i] = w.Code
	}

	limited := 0
	for _, s := range statuses {
		if s == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Error("expected some requests to be rate limited")
	}
}

func TestRateLimit_DifferentIPsIndependent(t *testing.T) {
	h := middleware.RateLimit(1, 1, okHandler) // burst of 1

	makeReq := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}

	// First request from each IP should pass
	if makeReq("192.168.1.1") != http.StatusOK {
		t.Error("first request from IP1 should pass")
	}
	if makeReq("192.168.1.2") != http.StatusOK {
		t.Error("first request from IP2 should pass")
	}
}

// ── Request Logger ────────────────────────────────────────────────────

func TestRequestLogger_LogsStatus(t *testing.T) {
	h := middleware.RequestLogger(logger, okHandler)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── CORS ─────────────────────────────────────────────────────────────

func TestCORS_Wildcard(t *testing.T) {
	h := middleware.CORS([]string{"*"}, okHandler)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("CORS header: got %q, want %q", got, "http://localhost:3000")
	}
}

func TestCORS_Preflight(t *testing.T) {
	h := middleware.CORS([]string{"*"}, okHandler)
	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight: expected 204, got %d", w.Code)
	}
}
