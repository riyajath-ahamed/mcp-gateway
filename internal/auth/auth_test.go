package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/configkits/mcp-gateway/internal/auth"
)

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
	v := auth.APIKeyValidator("X-Gateway-Key", "secret")
	handler := auth.Middleware(v, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	v := auth.APIKeyValidator("X-Gateway-Key", "secret")
	handler := auth.Middleware(v, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
