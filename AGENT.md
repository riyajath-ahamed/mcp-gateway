# AGENT.md — AI-assisted development guide

This file documents how AI coding assistants (Claude, Copilot, etc.) should
work with the `mcp-gateway` codebase. Read this before asking an AI to make
changes, write tests, or extend the gateway.

---

## Project map

```
cmd/gateway/main.go          ← entrypoint, wires middleware stack
internal/
  config/      config.go     ← YAML loader + env expansion + validation
  auth/        auth.go       ← API key, JWT middleware; GenerateAPIKey()
  registry/    registry.go   ← ServerState, capability aggregation, RouteForTool()
               test_helpers.go ← NewForTest() for unit tests
  router/      router.go     ← prefix/pattern routing + weighted round-robin
  proxy/       gateway.go    ← main http.Handler, JSON-RPC dispatch
  health/      checker.go    ← 30s health loop + circuit breaker
  transport/
    stdio.go                 ← subprocess MCP via JSON-RPC over stdin/stdout
    sse.go                   ← SSE-based MCP transport
  middleware/  middleware.go  ← rate limiter, recovery, request logger, CORS
  observability/ otel.go     ← OTel trace provider + Prometheus metrics
  admin/       admin.go      ← embedded dashboard (//go:embed dashboard.html)
               dashboard.html
pkg/sdk/       index.ts      ← TypeScript client SDK
```

---

## Key invariants

1. **`registry.ServerState` is goroutine-safe.** All mutations go through
   `RecordSuccess()` / `RecordFailure()` which hold `mu`. Never write fields
   directly outside those methods.

2. **`registry.NewForTest()` must be used in all unit tests.** Never perform
   real HTTP calls in tests under `internal/`. Use `httptest.NewServer` for
   backend mocks.

3. **Auth middleware bypasses `/health` and `/metrics` unconditionally.** Do
   not add auth checks to those paths anywhere else.

4. **Circuit breaker threshold comes from `ServerConfig.CircuitBreaker`.** Fall
   back to `3` when the field is nil. Keep this default consistent everywhere
   it appears (proxy, health checker).

5. **All JSON-RPC errors use negative codes.** Convention:
   - `-32700` parse error
   - `-32600` invalid request
   - `-32601` tool not found
   - `-32602` invalid params
   - `-32001` unauthorized
   - `-32002` backend error
   - `-32003` no healthy backend
   - `-32004` endpoint not found
   - `-32005` rate limit exceeded

---

## How to add a new MCP backend transport

1. Create `internal/transport/<name>.go` implementing at minimum:
   ```go
   func (t *XTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error)
   func (t *XTransport) ListTools(ctx context.Context) (json.RawMessage, error)
   func (t *XTransport) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error)
   ```
2. Register the transport string in `config.ServerConfig.Transport` validation.
3. Add a case to `registry.discoverTools()` that calls `ListTools`.
4. Add a case to `health.checkServer()` for health probing.
5. Write a unit test using a fake/in-process server.

---

## How to add a new auth type

1. Add an `AuthType` const in `internal/auth/auth.go`.
2. Add a `verify<Type>()` function.
3. Add a case in `verify()`.
4. Update `gateway.yaml` documentation (this file + README).
5. Add test cases to `internal/auth/auth_test.go`.

---

## How to add a new middleware

1. Write a function with signature:
   ```go
   func MyMiddleware(/* config */ ..., next http.Handler) http.Handler
   ```
   in `internal/middleware/middleware.go`.
2. Wire it in `cmd/gateway/main.go` (outermost = first in list = last to run).
3. Add tests to `internal/middleware/middleware_test.go`.

**Middleware stack order (outermost → innermost):**
```
Recovery → RateLimit → CORS → RequestLogger → Auth → mux
```

---

## Testing commands

```bash
# All tests
make test

# Single package
go test -v ./internal/router/...

# Benchmarks
go test -bench=. -benchmem ./internal/proxy/...

# Race detector (always on in CI)
go test -race ./...

# SDK tests
cd pkg/sdk && npm test
```

---

## Common AI prompts that work well with this codebase

- "Add OAuth 2.0 client credentials support to the auth middleware."
- "Implement retry-with-backoff in the proxy for transient backend errors."
- "Add a `tools/call` timeout per server using `ServerConfig.Timeout`."
- "Write a load test using `go test -bench` that measures p99 latency."
- "Extend the admin dashboard to show per-tool call counts."
- "Add Prometheus counter labels per backend server name."

---

## What NOT to ask AI to change without review

- `registry.ServerState.RecordFailure()` / `RecordSuccess()` — concurrency sensitive
- `auth.verifyJWT()` — security sensitive; prefer a real JWT library for prod
- `middleware.RateLimit()` bucket cleanup — off-by-one in time arithmetic breaks fairness
- `transport/stdio.go` readLoop — message ordering guarantees depend on the pending map

---

## Dependency policy

- **No new runtime dependencies** without discussion. The binary is intentionally lean.
- `gopkg.in/yaml.v2` — config parsing
- `github.com/google/uuid` — request ID generation
- `go.opentelemetry.io/otel/*` — tracing (optional, only active when endpoint is set)
- All other behaviour implemented in stdlib.

---

## Release checklist

- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` clean
- [ ] `cd pkg/sdk && npm test` passes
- [ ] `docker build .` succeeds
- [ ] `CHANGELOG.md` entry added
- [ ] Version bumped in `Makefile` `ldflags`
- [ ] `README.md` feature table updated
