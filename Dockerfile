# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN VERSION=$(cat VERSION 2>/dev/null || echo "dev") && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w -X github.com/configkits/mcp-gateway/internal/version.String=${VERSION}" \
    -o /bin/mcp-gateway \
    ./cmd/gateway

# ─── Final image ────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/mcp-gateway /mcp-gateway

EXPOSE 8080

ENTRYPOINT ["/mcp-gateway"]
CMD ["--config", "/etc/mcp-gateway/gateway.yaml"]
