package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

// ── Request Logger ────────────────────────────────────────────────────

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// RequestLogger logs every request with method, path, status, duration, and bytes.
func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", rec.bytes,
			"remote", r.RemoteAddr,
		)
	})
}

// ── Panic Recovery ────────────────────────────────────────────────────

// Recovery catches panics, logs the stack trace, and returns 500.
func Recovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				logger.Error("panic recovered",
					"panic", rec,
					"stack", string(stack),
					"path", r.URL.Path,
				)
				http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32000,"message":"internal server error"}}`,
					http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ── Token-bucket Rate Limiter ─────────────────────────────────────────

// bucket is a per-IP token bucket.
type bucket struct {
	tokens   float64
	lastSeen time.Time
	mu       sync.Mutex
}

type rateLimiter struct {
	buckets map[string]*bucket
	rps     float64
	burst   float64
	mu      sync.Mutex
	// background cleanup
	lastClean time.Time
}

// RateLimit enforces a per-IP token-bucket rate limit.
// rps = sustained requests per second; burst = max burst size.
func RateLimit(rps, burst float64, next http.Handler) http.Handler {
	rl := &rateLimiter{
		buckets:   make(map[string]*bucket),
		rps:       rps,
		burst:     burst,
		lastClean: time.Now(),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !rl.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32005,"message":"rate limit exceeded"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: rl.burst, lastSeen: time.Now()}
		rl.buckets[ip] = b
	}
	// Periodic cleanup of stale buckets (older than 5 minutes)
	if time.Since(rl.lastClean) > 5*time.Minute {
		for k, v := range rl.buckets {
			if time.Since(v.lastSeen) > 5*time.Minute {
				delete(rl.buckets, k)
			}
		}
		rl.lastClean = time.Now()
	}
	rl.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now

	// Refill tokens
	b.tokens += elapsed * rl.rps
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func extractIP(r *http.Request) string {
	// Respect X-Forwarded-For set by upstream proxies
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Strip port from RemoteAddr
	ip := r.RemoteAddr
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}

// ── CORS ──────────────────────────────────────────────────────────────

// CORS adds permissive CORS headers suitable for a local-dev gateway.
// In production, restrict AllowedOrigins via config.
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}
	wildcard := len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if wildcard || originSet[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Gateway-Key, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
