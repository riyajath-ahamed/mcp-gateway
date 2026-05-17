package auth_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/configkits/mcp-gateway/internal/auth"
)

var testLogger = slog.Default()

func TestAPIKeyValidator_Valid(t *testing.T) {
	v := auth.APIKeyValidator("X-Gateway-Key", "secret-key")
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Gateway-Key", "secret-key")
	if err := v(req); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestAPIKeyValidator_Missing(t *testing.T) {
	v := auth.APIKeyValidator("X-Gateway-Key", "secret-key")
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if err := v(req); err == nil {
		t.Error("expected error for missing key")
	}
}

func TestAPIKeyValidator_Wrong(t *testing.T) {
	v := auth.APIKeyValidator("X-Gateway-Key", "correct")
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Gateway-Key", "wrong")
	if err := v(req); err == nil {
		t.Error("expected error for wrong key")
	}
}

func TestNoopValidator_AlwaysPasses(t *testing.T) {
	v := auth.NoopValidator()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if err := v(req); err != nil {
		t.Errorf("noop should always pass, got: %v", err)
	}
}

func TestMiddleware_BlocksUnauthorized(t *testing.T) {
	cfg := auth.Config{Type: auth.AuthTypeAPIKey, Header: "X-Gateway-Key", Secret: "secret"}
	handler := auth.Middleware(cfg, testLogger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestMiddleware_AllowsAuthorized(t *testing.T) {
	cfg := auth.Config{Type: auth.AuthTypeAPIKey, Header: "X-Gateway-Key", Secret: "secret"}
	handler := auth.Middleware(cfg, testLogger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Gateway-Key", "secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key, err := auth.GenerateAPIKey(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != 64 {
		t.Errorf("expected 64-char hex string, got %d chars", len(key))
	}
}

func TestMiddleware_NoneType(t *testing.T) {
	cfg := auth.Config{Type: auth.AuthTypeNone}
	handler := auth.Middleware(cfg, testLogger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with auth type none, got %d", rr.Code)
	}
}
