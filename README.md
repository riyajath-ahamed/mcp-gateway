# mcp-gateway

> nginx for your AI agent tool layer

A single Go binary that proxies N MCP servers behind a unified Streamable HTTP endpoint-with capability aggregation, health checks, circuit breakers, pluggable auth, and full observability.

[![Go version](https://img.shields.io/badge/go-1.22-blue)](https://go.dev)
[![npm](https://img.shields.io/npm/v/@configkits/mcp-gateway-client)](https://npmjs.com/package/@configkits/mcp-gateway-client)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

---

## The Problem

Production AI systems connect to 5–15 MCP servers simultaneously-each with their own connection management, auth, and failure modes. Clients carry all this complexity themselves.

## The Solution

```
AI Client → mcp-gateway (:8080) → [filesystem-mcp, github-mcp, postgres-mcp, ...]
```

One endpoint. All tools. Auto-routed.

---

## Quickstart

```bash
# Install
go install github.com/configkits/mcp-gateway/cmd/gateway@latest

# Configure
cp gateway.yaml.example gateway.yaml
# edit gateway.yaml with your server URLs

# Run
mcp-gateway --config gateway.yaml
# ✓ registered server  filesystem-mcp  tools=12
# ✓ registered server  github-mcp      tools=31
# ✓ capability aggregation complete    total=51 tools
# ✓ gateway listening on :8080
```

### Docker

```bash
docker run -v $(pwd)/gateway.yaml:/etc/mcp-gateway/gateway.yaml \
  -p 8080:8080 \
  ghcr.io/configkits/mcp-gateway:latest
```

### One-click deploy

- [Deploy to Railway](https://railway.app/new/template?template=https://github.com/configkits/mcp-gateway)
- [Deploy to Fly.io](https://fly.io/docs/speedrun/) using `deploy/fly.toml`

---

## Configuration

See [`gateway.yaml`](gateway.yaml) for a fully annotated example.

```yaml
gateway:
  port: 8080
  auth:
    type: "api-key"
    header: "X-Gateway-Key"
    secret: "${GATEWAY_API_KEY}"

servers:
  - name: "github-mcp"
    transport: "http"
    url: "http://localhost:3002"
    auth:
      type: "bearer"
      token: "${GITHUB_MCP_TOKEN}"
    routing:
      prefix: "github_"
    circuit_breaker:
      threshold: 3
      timeout: "30s"
```

---

## TypeScript SDK

```bash
npm install @configkits/mcp-gateway-client
```

```typescript
import { MCPGatewayClient } from '@configkits/mcp-gateway-client'

const gateway = new MCPGatewayClient({
  url: 'https://gateway.yourapp.com',
  apiKey: process.env.MCP_GATEWAY_KEY,
})

// List all tools from all backends
const { tools } = await gateway.listTools()

// Call any tool-routing is automatic
const result = await gateway.callTool({
  name: 'github_create_issue',
  arguments: { title: 'fix: gateway routing', body: '...' }
})

// Streaming support
for await (const chunk of gateway.callToolStream(params)) {
  process.stdout.write(chunk)
}
```

---

## Architecture

```
┌─────────────────────┐
│   AI Client / Agent │
└──────────┬──────────┘
           │ Streamable HTTP
┌──────────▼──────────┐
│      mcp-gateway    │  capability aggregation
│  ┌─────────────────┐│  health checks + circuit breaker
│  │  server registry││  auth (api-key / jwt / oauth)
│  │  routing engine ││  OTel traces + metrics
│  └─────────────────┘│
└──┬──────┬───────┬───┘
   │      │       │
┌──▼──┐ ┌─▼──┐ ┌─▼──┐
│ fs  │ │ gh │ │ pg │  ... N backends
└─────┘ └────┘ └────┘
```

---

## Features

| Feature | Status |
|---------|--------|
| HTTP/SSE backend proxying | ✅ |
| STDIO backend support | ✅ |
| Capability aggregation | ✅ |
| Tool routing (prefix, pattern) | ✅ |
| Health checks | ✅ |
| Circuit breaker | ✅ |
| API key auth | ✅ |
| JWT auth | 🚧 |
| OAuth 2.0 | 🚧 |
| OpenTelemetry traces | ✅ |
| Prometheus metrics | ✅ |
| TypeScript SDK | ✅ |
| Docker image | ✅ |
| Railway template | ✅ |
| Fly.io template | ✅ |

---

## Observability

```bash
# Enable OTel tracing (Jaeger, Grafana Tempo, Honeycomb, Datadog...)
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# Prometheus metrics
curl http://localhost:8080/metrics

# Health endpoint
curl http://localhost:8080/health
# {"status":"healthy","servers":[{"name":"github-mcp","healthy":true,...}]}
```

---

## License

MIT-configkits/mcp-gateway
