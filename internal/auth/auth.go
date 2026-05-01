package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Validator is a function that validates an incoming request.
type Validator func(r *http.Request) error

// Middleware wraps an http.Handler with auth validation.
func Middleware(validator Validator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := validator(r); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":%q}}`, err.Error())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// APIKeyValidator returns a Validator that checks a header for a static API key.
func APIKeyValidator(header, secret string) Validator {
	if header == "" {
		header = "X-Gateway-Key"
	}
	secretBytes := []byte(secret)
	return func(r *http.Request) error {
		val := r.Header.Get(header)
		if val == "" {
			return fmt.Errorf("missing %s header", header)
		}
		if subtle.ConstantTimeCompare([]byte(val), secretBytes) != 1 {
			return fmt.Errorf("invalid API key")
		}
		return nil
	}
}

// JWTValidator is a placeholder for JWT Bearer validation.
// In production, integrate github.com/golang-jwt/jwt/v5.
func JWTValidator(signingSecret string) Validator {
	return func(r *http.Request) error {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return fmt.Errorf("missing Authorization header")
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			return fmt.Errorf("malformed Authorization header; expected: Bearer <token>")
		}
		// Full JWT verification with HMAC/RSA would go here.
		// Returning nil (valid) as a stub for wiring purposes.
		_ = time.Now() // suppress unused import
		return nil
	}
}

// NoopValidator allows all requests through (auth disabled).
func NoopValidator() Validator {
	return func(r *http.Request) error { return nil }
}
