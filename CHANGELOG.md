# Changelog

All notable changes to `mcp-gateway` are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added
- Initial public release of mcp-gateway

---

## [0.1.0-alpha] — 2026-04-13

### Added
- **Core gateway** — single Go binary proxying N MCP servers behind `:8080`
- **Capability aggregation** — on startup, discovers all tools from all backends,
  merges into unified manifest; clients see one `tools/list` response
- **Multi-transport support** — HTTP, SSE, and STDIO backends
- **Smart routing** — prefix rules, regex pattern rules, weighted round-robin
  fallback via `internal/router`
- **Circuit breaker** — per-server failure counter; opens after N failures,
  half-open probe on next health check interval
- **Health checker** — 30s background loop via `internal/health`
- **Auth middleware** — API key (header, constant-time compare), JWT (HS256 +
  exp check), passthrough; pluggable via `gateway.auth.type`
- **Rate limiter** — per-IP token-bucket (`rate_limit.requests_per_second` +
  `burst`)
- **CORS middleware** — configurable allowed origins
- **Panic recovery middleware** — catches panics, logs stack, returns 500
- **Request logger middleware** — structured JSON log per request
- **OpenTelemetry** — OTLP HTTP trace export via `OTEL_EXPORTER_OTLP_ENDPOINT`
- **Prometheus metrics** — `/metrics` endpoint (requests, success, error)
- **Admin dashboard** — embedded single-file HTML at `/admin/`; live server
  status, tool registry, request sparkline, log stream
- **TypeScript SDK** — `@configkits/mcp-gateway-client`; `listTools`,
  `callTool`, `callToolStream`, `health`
- **Docker image** — multi-stage build, distroless final image
- **Deploy templates** — Railway (`railway.toml`), Fly.io (`fly.toml`),
  Docker Compose
- **Test suite** — unit tests for config, auth, router, middleware, proxy;
  benchmarks for tools/list, tool call E2E, parallel throughput, router prefix
- **`AGENT.md`** — AI-assisted development guide

### Architecture decisions
- No external HTTP framework — stdlib `net/http` only
- Registry uses `sync.RWMutex` throughout; `ServerState` mutations are
  encapsulated in `RecordSuccess()` / `RecordFailure()`
- Admin dashboard embedded via `//go:embed` — zero runtime file dependencies
- Config supports `${ENV_VAR}` expansion before YAML parsing

### Known limitations (to be addressed in 0.2.0)
- JWT auth uses hand-rolled HS256; production users should swap in a
  well-audited JWT library
- STDIO transport tool discovery is a stub — tools must be pre-declared
- No persistent request log storage (log stream is in-memory in dashboard)
- OAuth 2.0 auth not yet implemented

---

## Roadmap

### 0.2.0 (planned)
- OAuth 2.0 client credentials auth
- Per-tool call timeout from `ServerConfig.Timeout`
- Retry with exponential backoff for transient backend errors
- Persistent SQLite request log for dashboard
- `mcp-gateway validate` CLI command to lint config
- Helm chart for Kubernetes deployment

### 0.3.0 (planned)
- Multi-region gateway federation
- Per-backend rate limiting
- gRPC transport support
- Web dashboard auth (separate admin secret)
- `@configkits/mcp-gateway-client` auto-type-generation from live manifest
