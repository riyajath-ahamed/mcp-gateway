package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type AuthType string

const (
	AuthTypeAPIKey AuthType = "api-key"
	AuthTypeJWT    AuthType = "jwt"
	AuthTypeNone   AuthType = "none"
)

type Config struct {
	Type   AuthType
	Header string
	Secret string
}

// Validator is a function that validates an incoming request.
type Validator func(r *http.Request) error

// GenerateAPIKey generates a cryptographically random API key of n bytes,
// returned as a hex-encoded string.
func GenerateAPIKey(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating API key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Middleware builds a validator from the provided Config and wraps the handler
// with auth validation. Unrecognised auth types are treated as "none".
func Middleware(cfg Config, logger *slog.Logger, next http.Handler) http.Handler {
	var v Validator
	switch cfg.Type {
	case AuthTypeAPIKey:
		v = APIKeyValidator(cfg.Header, cfg.Secret)
	case AuthTypeJWT:
		v = JWTValidator(cfg.Secret)
	default:
		v = NoopValidator()
	}

	logger.Info("auth middleware configured", "type", string(cfg.Type))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := v(r); err != nil {
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
