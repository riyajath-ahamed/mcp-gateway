.PHONY: build test lint docker run clean generate-key

# ─── Go ─────────────────────────────────────────────────────────
build:
	go build -trimpath -ldflags="-s -w" -o bin/mcp-gateway ./cmd/gateway

test:
	go test -race -cover -count=1 ./...

test-verbose:
	go test -race -cover -count=1 -v ./...

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@latest"

# ─── Run locally ────────────────────────────────────────────────
run: build
	./bin/mcp-gateway --config gateway.yaml

run-dev:
	go run ./cmd/gateway --config gateway.yaml

# ─── API key generation ─────────────────────────────────────────
generate-key:
	@go run -mod=mod ./tools/genkey/main.go

# ─── TypeScript SDK ─────────────────────────────────────────────
sdk-build:
	cd pkg/sdk && npm ci && npm run build

sdk-test:
	cd pkg/sdk && npm run test

sdk-publish:
	cd pkg/sdk && npm publish --access public

# ─── Docker ─────────────────────────────────────────────────────
docker-build:
	docker build -t ghcr.io/configkits/mcp-gateway:latest .

docker-run: docker-build
	docker run --rm \
		-p 8080:8080 \
		-v $(PWD)/gateway.yaml:/etc/mcp-gateway/gateway.yaml:ro \
		-e GATEWAY_API_KEY=$(GATEWAY_API_KEY) \
		ghcr.io/configkits/mcp-gateway:latest

docker-compose-up:
	docker compose -f deploy/docker-compose.yml up --build

# ─── Observability stack (local dev) ────────────────────────────
otel-up:
	docker compose -f deploy/docker-compose.yml --profile observability up -d jaeger
	@echo "Jaeger UI: http://localhost:16686"

# ─── Cleanup ────────────────────────────────────────────────────
clean:
	rm -rf bin/ pkg/sdk/dist pkg/sdk/node_modules

# ─── Deploy ─────────────────────────────────────────────────────
deploy-fly:
	fly deploy --config deploy/fly.toml

deploy-railway:
	railway up
